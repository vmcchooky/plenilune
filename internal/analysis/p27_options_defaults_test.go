package analysis

// Follow-up to the P2-7 finding (docs/integration/moonstorm.md §5): a
// present options object used to replace the defaults wholesale, so a
// partial object — say, only a deadline — zeroed every other field and
// failed OPTIONS_INVALID. Options are now defaulted per field: zero is
// never a valid explicit value for the bounded fields (validation has
// always required each ≥ 1), so zero means unset. MaxAnalyzed keeps
// its older rule — zero stays in the resolved struct and maxAnalyzed
// resolves it at use sites — because pre-P1-5 artifact echoes record
// max_analyzed as absent, and replay recompute must see that same echo.

import (
	"context"
	"errors"
	"testing"

	"plenilune/internal/packetset"
)

func TestPartialOptionsTakePerFieldDefaults(t *testing.T) {
	b, a := load(t, "redaction")
	r := run(t, b, a, func(req *Request) {
		req.Options = &Options{DeadlineMS: 45_000}
	})
	want := DefaultOptions()
	want.DeadlineMS = 45_000
	want.MaxAnalyzed = 0 // a present options object leaves max_analyzed unset
	if r.Request.Options != want {
		t.Fatalf("partial options echo:\n got %+v\nwant %+v", r.Request.Options, want)
	}
	// Against the nil-options run, exactly two echo fields may differ:
	// the named field, and max_analyzed (16 recorded vs unset).
	base := run(t, b, a, nil)
	got, ref := r.Request.Options, base.Request.Options
	if ref.DeadlineMS == got.DeadlineMS || ref.MaxAnalyzed != MaxAnalyzed || got.MaxAnalyzed != 0 {
		t.Fatalf("unexpected baseline echo: %+v vs %+v", got, ref)
	}
	got.DeadlineMS, got.MaxAnalyzed = ref.DeadlineMS, ref.MaxAnalyzed
	if got != ref {
		t.Fatalf("partial options changed more than the named field:\n got %+v\nwant %+v", r.Request.Options, ref)
	}
}

func TestEmptyOptionsEqualExplicitDefaultsWithMaxAnalyzedUnset(t *testing.T) {
	b, a := load(t, "redaction")
	empty := run(t, b, a, func(req *Request) { req.Options = &Options{} })
	if want := DefaultOptions(); empty.Request.Options.MaxAnalyzed != 0 ||
		empty.Request.Options.MaxClasses != want.MaxClasses ||
		empty.Request.Options.DeadlineMS != want.DeadlineMS ||
		empty.Request.Options.ACL != want.ACL {
		t.Fatalf("empty options must resolve to the defaults (max_analyzed unset): %+v", empty.Request.Options)
	}
	explicit := run(t, b, a, func(req *Request) {
		o := DefaultOptions()
		o.MaxAnalyzed = 0
		req.Options = &o
	})
	if empty.ContentSHA256 != explicit.ContentSHA256 || empty.SHA256 != explicit.SHA256 {
		t.Fatalf("empty options and the same options spelled out must be one artifact:\n%s / %s\n%s / %s",
			empty.ContentSHA256, empty.SHA256, explicit.ContentSHA256, explicit.SHA256)
	}
	// The nil-options request differs from {} only in the max_analyzed
	// echo (16 recorded vs unset) — the frozen-replay rule above. That
	// single echo field is also why their content digests differ.
	nilOpts := run(t, b, a, nil).Request.Options
	emptyOpts := empty.Request.Options
	emptyOpts.MaxAnalyzed = nilOpts.MaxAnalyzed
	if emptyOpts != nilOpts {
		t.Fatalf("nil and {} options must differ only in max_analyzed:\n got %+v\nwant %+v", empty.Request.Options, nilOpts)
	}
}

func TestNestedPartialOptionsFillLimits(t *testing.T) {
	b, a := load(t, "redaction")
	r := run(t, b, a, func(req *Request) {
		o := Options{}
		o.ACL.MaxWitnesses = 4 // default is 8
		req.Options = &o
	})
	echo := r.Request.Options
	if echo.ACL.MaxWitnesses != 4 {
		t.Fatalf("named nested field lost: %+v", echo.ACL)
	}
	if echo.ACL.Limits != packetset.Defaults() {
		t.Fatalf("absent limits must resolve to the defaults: %+v", echo.ACL.Limits)
	}
	if want := DefaultOptions(); echo.MaxClasses != want.MaxClasses || echo.DeadlineMS != want.DeadlineMS {
		t.Fatalf("absent scalar fields must resolve to the defaults: %+v", echo)
	}
}

func TestPartialOptionsStillRejectOutOfBounds(t *testing.T) {
	b, a := load(t, "redaction")
	overOps := Options{}
	overOps.ACL.Limits.MaxOperations = packetset.Defaults().MaxOperations + 1
	cases := map[string]Options{
		"deadline above ceiling":       {DeadlineMS: 120_001},
		"max classes negative":         {MaxClasses: -1},
		"max operations above ceiling": overOps,
	}
	for name, opts := range cases {
		o := opts
		_, err := Analyze(context.Background(), Request{
			Schema: RequestSchema, Before: Source{Text: b}, After: Source{Text: a}, Options: &o,
		})
		var e *Error
		if !errors.As(err, &e) || e.Code != "OPTIONS_INVALID" {
			t.Fatalf("%s: want OPTIONS_INVALID, got %v", name, err)
		}
	}
}

func TestPartialOptionsArtifactReplays(t *testing.T) {
	b, a := load(t, "redaction")
	r := run(t, b, a, func(req *Request) {
		req.Options = &Options{DeadlineMS: 45_000}
	})
	out, err := Replay(context.Background(), []byte(b), []byte(a), r)
	if err != nil || out.Status != "ANALYSIS_REPLAYED" {
		t.Fatalf("partial-options artifact replay: %+v %v", out, err)
	}
	if out.ContentSHA256 != r.ContentSHA256 {
		t.Fatalf("replay must surface the content digest: %+v vs %s", out, r.ContentSHA256)
	}
}

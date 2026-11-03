package audit

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/verify"
)

func legacyFixture(t *testing.T) ([]byte, Record) {
	t.Helper()
	source, err := os.ReadFile("testdata/legacy-v1.acl")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/legacy-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var r Record
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return source, r
}

func TestLegacyAuditFrozenReplay(t *testing.T) {
	source, r := legacyFixture(t)
	if r.Version != "acl-audit/v1" {
		t.Fatal("fixture version changed")
	}
	if r.SHA256 != "80dbe55cf391c8cec2a0663193295a784da47c10472e91cf4b18e60239356f82" {
		t.Fatal("frozen legacy seal changed")
	}
	if err := Check(r); err != nil {
		t.Fatal(err)
	}
	got, err := Replay(context.Background(), source, r)
	if err != nil || got.Status != "AUDIT_REPLAYED" || got.SHA256 != r.SHA256 {
		t.Fatalf("frozen replay: %+v %v", got, err)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["parser_version"]; ok {
		t.Fatal("legacy envelope acquired parser field")
	}
}

func TestAuditParserVersionPins(t *testing.T) {
	source, old := legacyFixture(t)
	current, err := Audit(context.Background(), source, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{string(source), "ip access-list standard X\n deny any\n permit host 192.0.2.7\n"} {
		r, err := Audit(context.Background(), []byte(input), verify.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(r)
		var wire map[string]any
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if r.Version != "acl-audit/v2" || wire["parser_version"] != ciscoacl.ParserVersion {
			t.Fatalf("new audit not pinned: %s", raw)
		}
		if got, err := Replay(context.Background(), []byte(input), r); err != nil || got.Status != "AUDIT_REPLAYED" {
			t.Fatalf("current replay: %+v %v", got, err)
		}
	}
	for _, tt := range []struct {
		record Record
		parser string
	}{{old, ciscoacl.LegacyParserVersion}, {old, ciscoacl.ParserVersion}, {current, ""}, {current, ciscoacl.LegacyParserVersion}, {current, "unknown"}} {
		raw, _ := json.Marshal(tt.record)
		var wire map[string]any
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		wire["parser_version"] = tt.parser
		raw, _ = json.Marshal(wire)
		var bad Record
		if err := json.Unmarshal(raw, &bad); err != nil {
			t.Fatal(err)
		}
		// Rehashing cannot legitimize a parser/version mismatch.
		if _, err := seal(bad); err == nil {
			t.Fatalf("unsupported parser pin sealed: %s/%q", bad.Version, tt.parser)
		}
		if _, err := Replay(context.Background(), source, bad); err == nil {
			t.Fatal("parser-tampered audit replayed")
		}
	}
}

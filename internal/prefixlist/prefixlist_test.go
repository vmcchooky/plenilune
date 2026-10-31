package prefixlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func lines(text string) []SourceLine {
	out := []SourceLine{}
	for i, l := range strings.Split(strings.TrimSpace(text), "\n") {
		out = append(out, SourceLine{Number: i + 1, Text: strings.TrimSpace(l)})
	}
	return out
}

func mustList(t *testing.T, name, text string) List {
	t.Helper()
	l, err := ParseLines(name, lines(text))
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	return l
}

func route(t *testing.T, s string) Route {
	t.Helper()
	var r Route
	if err := r.UnmarshalText([]byte(s)); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestParseDocumentedRangesAndSequence(t *testing.T) {
	l := mustList(t, "EDGE", `ip prefix-list EDGE description synthetic
ip prefix-list EDGE permit 10.0.0.0/8 le 24
ip prefix-list EDGE seq 3 deny 10.1.0.0/16 ge 20
ip prefix-list EDGE deny 0.0.0.0/0 ge 25 le 32
ip prefix-list EDGE permit 192.0.2.0/24`)
	got := []string{}
	for _, e := range l.Entries {
		got = append(got, fmt.Sprintf("%d:%s %d-%d", e.Seq, e.Action, e.Lo, e.Hi))
	}
	// Omitted seq = highest so far + 5: 5, explicit 3, then 10, 15.
	if strings.Join(got, ",") != "3:deny 20-32,5:permit 8-24,10:deny 25-32,15:permit 24-24" {
		t.Fatalf("entries %v", got)
	}
	for _, bad := range []struct{ text, code string }{
		{"ip prefix-list X permit 10.0.0.1/8", "PREFIX_LIST_HOST_BITS"},
		{"ip prefix-list X permit 10.0.0.0/8 ge 8", "PREFIX_LIST_RANGE"},
		{"ip prefix-list X permit 10.0.0.0/8 le 8", "PREFIX_LIST_RANGE"},
		{"ip prefix-list X permit 10.0.0.0/8 ge 24 le 16", "PREFIX_LIST_RANGE"},
		{"ip prefix-list X permit 10.0.0.0/8 ge 0", "PREFIX_LIST_RANGE"},
		{"ip prefix-list X permit 10.0.0.0/8 le 33", "PREFIX_LIST_RANGE"},
		{"ip prefix-list X permit 2001:db8::/32", "PREFIX_LIST_PREFIX"},
		{"ip prefix-list X seq 5 permit 10.0.0.0/8\nip prefix-list X seq 5 deny 10.0.0.0/8", "PREFIX_LIST_SEQUENCE"},
		{"ip prefix-list X description only", "PREFIX_LIST_EMPTY"},
		{"ip prefix-list X permit 10.0.0.0/8 eq 9", "PREFIX_LIST_UNSUPPORTED"},
	} {
		_, err := ParseLines("X", lines(bad.text))
		var e *Error
		if !errors.As(err, &e) || e.Code != bad.code {
			t.Fatalf("%q: got %v, want %s", bad.text, err, bad.code)
		}
	}
}

func TestEvaluateFirstMatchAndImplicitDeny(t *testing.T) {
	l := mustList(t, "P", `ip prefix-list P seq 10 deny 10.1.0.0/16 le 32
ip prefix-list P seq 20 permit 10.0.0.0/8 le 24`)
	for _, c := range []struct {
		route, action string
		implicit      bool
	}{{"10.1.2.0/24", "deny", false}, {"10.2.0.0/16", "permit", false}, {"10.2.3.0/25", "deny", true}, {"0.0.0.0/0", "deny", true}, {"10.0.0.0/8", "permit", false}} {
		d, err := l.Evaluate(route(t, c.route))
		if err != nil || d.Action != c.action || d.Implicit != c.implicit {
			t.Fatalf("%s: %+v %v", c.route, d, err)
		}
	}
	if _, err := l.Evaluate(Route{Network: 0x0A000001, Len: 24}); err == nil {
		t.Fatal("non-canonical route accepted")
	}
}

// Exhaustive oracle on a reduced route space: every route under 10.0.0.0/20
// at lengths 20..30 plus the covering prefix at other lengths is checked
// against the linear evaluator for class membership and deciding entries.
func TestCompareMatchesLinearEvaluatorExhaustively(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	gen := func() List {
		var b strings.Builder
		n := 1 + rng.Intn(6)
		for i := 0; i < n; i++ {
			length := 20 + rng.Intn(8)
			network := (uint32(10<<24) | uint32(rng.Intn(1<<12))<<8) & mask(uint8(length))
			act := []string{"permit", "deny"}[rng.Intn(2)]
			opt := ""
			switch rng.Intn(4) {
			case 1:
				opt = fmt.Sprintf(" ge %d", length+1+rng.Intn(31-length))
			case 2:
				opt = fmt.Sprintf(" le %d", length+1+rng.Intn(31-length))
			case 3:
				ge := length + 1 + rng.Intn(30-length)
				opt = fmt.Sprintf(" ge %d le %d", ge, ge+rng.Intn(31-ge))
			}
			fmt.Fprintf(&b, "ip prefix-list R seq %d %s %s%s\n", (i+1)*5, act, Route{Network: network, Len: uint8(length)}, opt)
		}
		if rng.Intn(3) == 0 {
			fmt.Fprintf(&b, "ip prefix-list R seq 900 permit 0.0.0.0/0 le %d\n", 20+rng.Intn(13))
		}
		return mustList(t, "R", b.String())
	}
	for seed := 0; seed < 80; seed++ {
		before, after := gen(), gen()
		ops := 0
		bp, _ := partition(context.Background(), before, &ops)
		ap, _ := partition(context.Background(), after, &ops)
		groups, _, stop, err := sweep(context.Background(), before, after, bp, ap, &ops)
		if err != nil || stop != "" {
			t.Fatalf("sweep: %v %s", err, stop)
		}
		report, err := Compare(context.Background(), before, after, DefaultMaxClasses)
		if err != nil || !report.Complete {
			t.Fatalf("seed %d: %v %+v", seed, err, report)
		}
		different := false
		check := func(r Route) {
			x, _ := before.Evaluate(r)
			y, _ := after.Evaluate(r)
			n := uint64(0)
			if r.Len > 0 {
				n = uint64(r.Network) >> uint(32-r.Len)
			}
			hits := 0
			for k, segs := range groups {
				for _, s := range segs {
					if s.Len == r.Len && n >= s.Lo && n <= s.Hi {
						hits++
						if !decidedBy(x, before, k.before) || !decidedBy(y, after, k.after) {
							t.Fatalf("seed %d route %s owners mismatch", seed, r)
						}
					}
				}
			}
			want := 0
			if x.Action != y.Action {
				want, different = 1, true
			}
			if hits != want {
				t.Fatalf("seed %d route %s: %d hits, want %d", seed, r, hits, want)
			}
		}
		for length := uint8(0); length <= 32; length++ {
			if length < 20 || length > 30 {
				check(Route{Network: uint32(10<<24) & mask(length), Len: length})
				continue
			}
			for n := uint64(0); n < 1<<uint(length-20); n++ {
				check(Route{Network: uint32(10<<24) | uint32(n<<uint(32-length)), Len: length})
			}
		}
		if different && report.Verdict != "DIFFERENT" {
			t.Fatalf("seed %d: grid differs but verdict %s", seed, report.Verdict)
		}
	}
}

func TestCompareClassesCountsAndWitness(t *testing.T) {
	before := mustList(t, "EDGE", `ip prefix-list EDGE seq 5 permit 10.0.0.0/8 le 24`)
	after := mustList(t, "EDGE", `ip prefix-list EDGE seq 5 deny 10.1.0.0/16 le 32
ip prefix-list EDGE seq 10 permit 10.0.0.0/8 le 24
ip prefix-list EDGE seq 15 permit 192.0.2.0/24`)
	r, err := Compare(context.Background(), before, after, DefaultMaxClasses)
	if err != nil || r.Verdict != "DIFFERENT" || len(r.Classes) != 2 {
		t.Fatalf("%v %+v", err, r)
	}
	denied := r.Classes[0]
	// Routes 10.1.0.0/16 through /24 that were permitted: 2^0+...+2^8 = 511.
	if denied.Effect != "newly_denied" || denied.Routes != 511 || denied.Witness.Route.String() != "10.1.0.0/16" || *denied.After.Seq != 5 {
		t.Fatalf("denied %+v", denied)
	}
	permitted := r.Classes[1]
	if permitted.Effect != "newly_permitted" || permitted.Routes != 1 || !permitted.Before.Implicit || permitted.Witness.Route.String() != "192.0.2.0/24" {
		t.Fatalf("permitted %+v", permitted)
	}
	same, err := Compare(context.Background(), before, mustList(t, "EDGE", "ip prefix-list EDGE seq 50 permit 10.0.0.0/8 le 24"), DefaultMaxClasses)
	if err != nil || same.Verdict != "EQUIVALENT" || len(same.Classes) != 0 {
		t.Fatalf("renumbered equivalent: %v %+v", err, same)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Compare(canceled, before, after, DefaultMaxClasses); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

// Byte slices marshal as base64 in Go; lengths must stay a JSON number array.
func TestClassLengthsSerializeAsNumbers(t *testing.T) {
	before := mustList(t, "EDGE", "ip prefix-list EDGE seq 5 permit 10.0.0.0/8 le 24")
	after := mustList(t, "EDGE", "ip prefix-list EDGE seq 5 permit 10.0.0.0/8 le 26")
	r, err := Compare(context.Background(), before, after, DefaultMaxClasses)
	if err != nil || len(r.Classes) != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	raw, err := json.Marshal(r.Classes[0])
	if err != nil || !strings.Contains(string(raw), `"lengths":[25,26]`) {
		t.Fatalf("lengths encoding: %s %v", raw, err)
	}
}

// The partition budget stops with no classes rather than a partial verdict.
func TestPartitionBudgetStops(t *testing.T) {
	l := mustList(t, "P", "ip prefix-list P seq 5 permit 10.0.0.0/8 le 32")
	ops := maxOperations
	_, err := partition(context.Background(), l, &ops)
	if !errors.Is(err, errBudget) {
		t.Fatalf("budget not enforced: %v", err)
	}
}

func TestCompareRejectsOversizedList(t *testing.T) {
	list := List{Name: "P", Entries: make([]Entry, MaxEntries+1)}
	for i := range list.Entries {
		list.Entries[i] = Entry{Seq: uint32(i + 1), Action: "permit", Lo: 8, Hi: 8}
	}
	if _, err := Compare(context.Background(), list, list, DefaultMaxClasses); err == nil {
		t.Fatal("oversized list accepted")
	}
}

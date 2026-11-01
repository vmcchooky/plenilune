package analysis

// P1-5: parallel multi-object analysis with a global budget and
// max_analyzed as an option. The acceptance properties are byte-level:
// the sealed artifact must not depend on the worker count or on
// GOMAXPROCS, the global ledger must stop admission deterministically
// in canonical order, and a deadline that lands mid-run must yield a
// marked partial artifact instead of the pre-P1-5 all-or-nothing error.

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// p15Pair renders one corpus ACL pair under the given name; mutate
// transforms the after rules.
func p15Pair(name string, n int, mutate func([]string) []string) (string, string) {
	base := p12CorpusACL(n)
	after := append([]string(nil), base...)
	if mutate != nil {
		after = mutate(after)
	}
	return p12Render(name, base), p12Render(name, after)
}

// p15SwapMid is the corpus generator's swap_adjacent: it swaps the two
// middle rules, which the engine proves equivalent at these sizes.
func p15SwapMid(rules []string) []string {
	mid := len(rules) / 2
	rules[mid], rules[mid+1] = rules[mid+1], rules[mid]
	return rules
}

func p15ChangePort(rules []string) []string {
	for i, rule := range rules {
		if strings.Contains(rule, " eq 443") {
			rules[i] = strings.Replace(rule, " eq 443", " eq 8443", 1)
			break
		}
	}
	return rules
}

// p15Mixed builds a two-sided config with one object of most statuses.
func p15Mixed() (string, string) {
	changedB, changedA := p15Pair("CHANGED", 60, p15ChangePort)
	swapB, swapA := p15Pair("SWAPPED", 100, p15SwapMid)
	sameB, _ := p15Pair("STABLE", 40, nil)
	_, addedA := p15Pair("NEWOBJ", 30, p15ChangePort)
	removedB, _ := p15Pair("GONE", 30, nil)
	plB := "ip prefix-list CUST seq 5 permit 203.0.113.0/24\nip prefix-list CUST seq 10 deny 0.0.0.0/0 le 32\n"
	plA := "ip prefix-list CUST seq 5 permit 203.0.113.0/24\nip prefix-list CUST seq 7 permit 198.51.100.0/24\nip prefix-list CUST seq 10 deny 0.0.0.0/0 le 32\n"
	before := changedB + swapB + sameB + removedB + plB
	after := changedA + swapA + sameB + addedA + plA
	return before, after
}

func p15Analyze(t *testing.T, before, after string, opts *Options, workers int) Result {
	t.Helper()
	req := Request{Schema: RequestSchema, Before: Source{Text: before}, After: Source{Text: after}, Options: opts}
	r, err := AnalyzeExec(context.Background(), req, ExecOptions{Workers: workers})
	if err != nil {
		t.Fatalf("analyze workers=%d: %v", workers, err)
	}
	return r
}

func TestP15WorkerCountInvariance(t *testing.T) {
	before, after := p15Mixed()
	want := map[string]string{
		"CHANGED": "changed", "SWAPPED": "equivalent", "STABLE": "unchanged",
		"NEWOBJ": "added", "GONE": "removed",
	}
	var sha string
	for _, workers := range []int{1, 2, 4, 7} {
		r := p15Analyze(t, before, after, nil, workers)
		if sha == "" {
			sha = r.SHA256
			if r.GlobalBudget != nil {
				t.Fatalf("unconstrained run recorded a global budget: %+v", r.GlobalBudget)
			}
			for name, status := range want {
				if o := object(t, r, DomainACL, name); o.Status != status {
					t.Fatalf("object %s status %q, want %q", name, o.Status, status)
				}
			}
			if o := object(t, r, DomainPrefixList, "CUST"); o.Status != "changed" {
				t.Fatalf("prefix object status %q", o.Status)
			}
		} else if r.SHA256 != sha {
			t.Fatalf("workers=%d digest %s differs from workers=1 digest %s", workers, r.SHA256, sha)
		}
	}
	// The digest must not depend on GOMAXPROCS either.
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	r := p15Analyze(t, before, after, nil, 0)
	if r.SHA256 != sha {
		t.Fatalf("GOMAXPROCS=1 digest %s differs from %s", r.SHA256, sha)
	}
	// And the parallel artifact replays exactly.
	base := p15Analyze(t, before, after, nil, 1)
	out, err := Replay(context.Background(), []byte(before), []byte(after), base)
	if err != nil || out.Status != "ANALYSIS_REPLAYED" || out.SHA256 != base.SHA256 {
		t.Fatalf("parallel artifact replay: %+v %v", out, err)
	}
}

func TestP15GlobalBudgetLedger(t *testing.T) {
	// Three equal-cost objects (same rules, equal-length names). Learn
	// one object's charge from an unconstrained run, then cap the global
	// operations budget at 1.5 charges: object one is recorded whole,
	// the rest of the run is bounded by the ledger.
	names := []string{"LEDGER-A", "LEDGER-B", "LEDGER-C"}
	var before, after strings.Builder
	for _, name := range names {
		b, a := p15Pair(name, 100, p15ChangePort)
		before.WriteString(b)
		after.WriteString(a)
	}
	full := p15Analyze(t, before.String(), after.String(), nil, 1)
	charge, _ := objectCharge(object(t, full, DomainACL, "LEDGER-A"))
	if charge <= 0 {
		t.Fatal("learned a zero charge")
	}
	limit := charge + charge/2
	opts := DefaultOptions()
	opts.ACL.Limits.MaxOperations = limit

	var sha string
	for _, workers := range []int{1, 4} {
		r := p15Analyze(t, before.String(), after.String(), &opts, workers)
		if sha == "" {
			sha = r.SHA256
		} else if r.SHA256 != sha {
			t.Fatalf("bounded run digest differs across workers: %s vs %s", r.SHA256, sha)
		}
	}
	r := p15Analyze(t, before.String(), after.String(), &opts, 4)
	if r.GlobalBudget == nil {
		t.Fatal("bounded run recorded no global budget")
	}
	if r.GlobalBudget.MaxOperations != limit || r.GlobalBudget.Operations < charge || r.GlobalBudget.Operations > limit {
		t.Fatalf("global budget record %+v (learned charge %d, limit %d)", r.GlobalBudget, charge, limit)
	}
	foundLimitation := false
	for _, l := range r.Limitations {
		foundLimitation = foundLimitation || l == "global_budget_exhausted"
	}
	if !foundLimitation {
		t.Fatalf("limitations %v lack global_budget_exhausted", r.Limitations)
	}
	first := object(t, r, DomainACL, "LEDGER-A")
	if first.Status != "changed" || first.ACL == nil || !first.ACL.SearchComplete || !first.ACL.Changes.Complete {
		t.Fatalf("first object was not recorded whole: %+v", first)
	}
	complete := 0
	recordedOps := 0
	for _, name := range names {
		o := object(t, r, DomainACL, name)
		if o.ACL != nil && o.ACL.SearchComplete && o.ACL.Changes.Complete {
			complete++
		}
		if o.ACL != nil {
			ops, _ := objectCharge(o)
			recordedOps += ops
		}
	}
	if complete != 1 {
		t.Fatalf("%d objects recorded complete under a 1.5-charge budget, want exactly 1", complete)
	}
	if recordedOps > limit {
		t.Fatalf("recorded objects charged %d operations over the %d budget", recordedOps, limit)
	}
	last := object(t, r, DomainACL, "LEDGER-C")
	if last.Status == "not_analyzed" && last.Reason != "global_budget" {
		t.Fatalf("last object not_analyzed for reason %q", last.Reason)
	}
	out, err := Replay(context.Background(), []byte(before.String()), []byte(after.String()), r)
	if err != nil || out.Status != "ANALYSIS_REPLAYED" || out.SHA256 != r.SHA256 {
		t.Fatalf("bounded artifact replay: %+v %v", out, err)
	}
}

func TestP15MaxAnalyzedOption(t *testing.T) {
	var before, after strings.Builder
	for _, name := range []string{"OPT-A", "OPT-B", "OPT-C", "OPT-D"} {
		b, a := p15Pair(name, 40, p15ChangePort)
		before.WriteString(b)
		after.WriteString(a)
	}
	opts := DefaultOptions()
	opts.MaxAnalyzed = 2
	r := p15Analyze(t, before.String(), after.String(), &opts, 4)
	if r.Summary.NotAnalyzed != 2 {
		t.Fatalf("summary %+v", r.Summary)
	}
	for _, name := range []string{"OPT-C", "OPT-D"} {
		if o := object(t, r, DomainACL, name); o.Status != "not_analyzed" || o.Reason != "object_limit" {
			t.Fatalf("object %s: %+v", name, o)
		}
	}
	out, err := Replay(context.Background(), []byte(before.String()), []byte(after.String()), r)
	if err != nil || out.Status != "ANALYSIS_REPLAYED" {
		t.Fatalf("max_analyzed artifact replay: %+v %v", out, err)
	}

	bad := DefaultOptions()
	bad.MaxAnalyzed = MaxAnalyzedLimit + 1
	req := Request{Schema: RequestSchema, Before: Source{Text: before.String()}, After: Source{Text: after.String()}, Options: &bad}
	var e *Error
	if _, err := Analyze(context.Background(), req); !errors.As(err, &e) || e.Code != "OPTIONS_INVALID" {
		t.Fatalf("max_analyzed over the limit accepted: %v", err)
	}
}

func TestP15DeadlinePartialArtifact(t *testing.T) {
	// One heavy object first (a 512-rule swap, hundreds of milliseconds
	// of engine work on any target machine) and two light ones behind
	// it, a single worker and a 50 ms deadline: the deadline lands
	// during the heavy comparison, so nothing completes and every
	// object must be marked, in a sealed artifact rather than an error.
	heavyB, heavyA := p15Pair("HEAVY", 512, p15SwapMid)
	light1B, light1A := p15Pair("LIGHT-A", 20, p15ChangePort)
	light2B, light2A := p15Pair("LIGHT-B", 20, p15ChangePort)
	before := heavyB + light1B + light2B
	after := heavyA + light1A + light2A
	opts := DefaultOptions()
	opts.DeadlineMS = 50
	req := Request{Schema: RequestSchema, Before: Source{Text: before}, After: Source{Text: after}, Options: &opts}
	r, err := AnalyzeExec(context.Background(), req, ExecOptions{Workers: 1})
	if err != nil {
		t.Fatalf("deadline mid-run returned an error instead of a partial artifact: %v", err)
	}
	if r.SHA256 == "" {
		t.Fatal("partial artifact is not sealed")
	}
	marked := 0
	for _, o := range r.Objects {
		if o.Status == "not_analyzed" && o.Reason == "deadline_exceeded" {
			marked++
			if o.ACL != nil || o.PrefixList != nil {
				t.Fatalf("deadline-marked object %s carries an engine payload", o.Name)
			}
		} else if o.ACL == nil && o.PrefixList == nil && o.Status != "unchanged" {
			t.Fatalf("object %s unmarked and unfinished: %+v", o.Name, o)
		}
	}
	if marked == 0 {
		t.Fatalf("no object marked deadline_exceeded: %+v", r.Objects)
	}
	foundLimitation := false
	for _, l := range r.Limitations {
		foundLimitation = foundLimitation || l == "analysis_deadline_exceeded"
	}
	if !foundLimitation {
		t.Fatalf("limitations %v lack analysis_deadline_exceeded", r.Limitations)
	}
	// A deadline-cut artifact replays as TIMING_SENSITIVE, never as a
	// mismatch and never as a full replay.
	out, err := Replay(context.Background(), []byte(before), []byte(after), r)
	if err != nil || out.Status != "TIMING_SENSITIVE" {
		t.Fatalf("deadline artifact replay: %+v %v", out, err)
	}
}

func BenchmarkP15AnalyzeWorkers(b *testing.B) {
	var before, after strings.Builder
	for i := 0; i < 16; i++ {
		b, a := p15Pair(fmt.Sprintf("BENCH-%02d", i), 100, p15ChangePort)
		before.WriteString(b)
		after.WriteString(a)
	}
	req := Request{Schema: RequestSchema, Before: Source{Text: before.String()}, After: Source{Text: after.String()}}
	for _, workers := range []int{1, 4} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := AnalyzeExec(context.Background(), req, ExecOptions{Workers: workers}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

package analysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"plenilune/internal/intent"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
)

func TestFrozenPrefixV01AnalysisReplay(t *testing.T) {
	for _, name := range []string{"analysis-routing-v01", "analysis-prefix-v01"} {
		t.Run(name, func(t *testing.T) {
			base := filepath.Join("..", "..", "testdata", "receipts")
			raw, err := os.ReadFile(filepath.Join(base, name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var r Result
			if err := json.Unmarshal(raw, &r); err != nil {
				t.Fatal(err)
			}
			var b, a []byte
			if name == "analysis-routing-v01" {
				x, y := load(t, "routing")
				b, a = []byte(x), []byte(y)
			} else {
				b, err = os.ReadFile(filepath.Join(base, name+"-before.cfg"))
				if err != nil {
					t.Fatal(err)
				}
				a, err = os.ReadFile(filepath.Join(base, name+"-after.cfg"))
				if err != nil {
					t.Fatal(err)
				}
				current := run(t, string(b), string(a), nil)
				if current.Objects[0].Status != "unsupported" {
					t.Fatal("new parser accepted legacy-only sequence")
				}
			}
			out, err := Replay(context.Background(), b, a, r)
			if err != nil || out.Status != "ANALYSIS_REPLAYED" || out.SHA256 != r.SHA256 {
				t.Fatalf("frozen replay: %+v %v", out, err)
			}
		})
	}
}

func TestAnalysisRejectsMixedUnknownOrRelabeledPrefixVersions(t *testing.T) {
	base := filepath.Join("..", "..", "testdata", "receipts")
	raw, err := os.ReadFile(filepath.Join(base, "analysis-prefix-v01.json"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(base, "analysis-prefix-v01-before.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(filepath.Join(base, "analysis-prefix-v01-after.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"mixed", "unknown", "relabel-current"} {
		t.Run(kind, func(t *testing.T) {
			var r Result
			if err := json.Unmarshal(raw, &r); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "mixed":
				r.Versions.PrefixListParser = currentVersions().PrefixListParser
			case "unknown":
				r.Versions.PrefixListEngine = "prefix-list-exact/v999"
			case "relabel-current":
				r.Versions = currentVersions()
			}
			r, err := Seal(r)
			if err != nil {
				t.Fatal(err)
			}
			if out, err := Replay(context.Background(), b, a, r); err == nil {
				t.Fatalf("version substitution passed: %+v", out)
			}
		})
	}
}

func timedRecord(t *testing.T, b, a string) Result {
	t.Helper()
	r := run(t, b, a, nil)
	for i := range r.Objects {
		if r.Objects[i].ACL != nil {
			x := r.Objects[i].ACL
			x.Verdict = "INCONCLUSIVE"
			x.SearchComplete = false
			x.Reason = "wall_clock"
			x.Witnesses = []verify.Witness{}
			x.Truncated = false
			x.Budget.TimingLimited = true
			x.Budget.StopReason = "wall_clock"
			r.Objects[i].Status = "inconclusive"
			break
		}
	}
	r.Summary = Summary{UnanalyzedChanges: r.Summary.UnanalyzedChanges}
	r.Limitations = []string{}
	summarize(&r)
	r, err := Seal(r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestTimingCannotExcuseUnrelatedTampering(t *testing.T) {
	b, a := load(t, "regression")
	b += "\nip prefix-list REPLAY seq 5 permit 203.0.113.0/24\n"
	a += "\nip prefix-list REPLAY seq 5 deny 203.0.113.0/24\n"
	base := timedRecord(t, b, a)
	if out, err := Replay(context.Background(), []byte(b), []byte(a), base); err != nil || out.Status != "TIMING_SENSITIVE" {
		t.Fatalf("valid limited replay: %+v %v", out, err)
	}
	cases := []struct {
		name string
		edit func(*Result)
	}{
		{"source metadata", func(r *Result) { v := r.Sources["before"]; v.Lines++; r.Sources["before"] = v }},
		{"coverage", func(r *Result) { v := r.Coverage["before"]; v.AnalyzedLines++; r.Coverage["before"] = v }},
		{"summary", func(r *Result) { r.Summary.Equivalent++ }},
		{"build identity", func(r *Result) { r.Tool.Commit = "another-build" }},
		{"prefix count", func(r *Result) {
			found := false
			for i := range r.Objects {
				if r.Objects[i].PrefixList != nil && len(r.Objects[i].PrefixList.Report.Classes) > 0 {
					r.Objects[i].PrefixList.Report.Classes[0].Routes++
					found = true
					break
				}
			}
			if !found {
				t.Fatal("fixture missing prefix change")
			}
		}},
		{"missing extent", func(r *Result) {
			for i := range r.Objects {
				if r.Objects[i].ACL != nil {
					r.Objects[i].Before = nil
					break
				}
			}
		}},
		{"false complete", func(r *Result) {
			for i := range r.Objects {
				if r.Objects[i].ACL != nil {
					r.Objects[i].ACL.SearchComplete = true
					r.Objects[i].ACL.Verdict = "EQUIVALENT"
					r.Objects[i].Status = "equivalent"
					break
				}
			}
		}},
		{"false class witness", func(r *Result) {
			for i := range r.Objects {
				if r.Objects[i].ACL != nil && len(r.Objects[i].ACL.Changes.Classes) > 0 {
					r.Objects[i].ACL.Changes.Classes[0].Witness.ID = "wit-fake"
					return
				}
			}
			t.Fatal("missing ACL change")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var r Result
			if err := json.Unmarshal(raw, &r); err != nil {
				t.Fatal(err)
			}
			tc.edit(&r)
			r, err := Seal(r)
			if err != nil {
				t.Fatal(err)
			}
			if out, err := Replay(context.Background(), []byte(b), []byte(a), r); err == nil {
				t.Fatalf("tamper excused by timing: %+v", out)
			}
		})
	}
}

func TestAnalysisDeadlineIntegerBoundPrecedesDurationConversion(t *testing.T) {
	for _, v := range []int{120001, int(^uint(0) >> 1)} {
		opts := DefaultOptions()
		opts.DeadlineMS = v
		_, err := Analyze(context.Background(), Request{Schema: RequestSchema, Before: Source{}, After: Source{}, Options: &opts})
		if err == nil {
			t.Fatal("overflowing or unsupported deadline accepted")
		}
		if e, ok := err.(*Error); !ok || e.Code != "OPTIONS_INVALID" {
			t.Fatalf("wrong deadline failure: %v", err)
		}
	}
}

// Inject the fresh timing state directly; this must not depend on machine speed.
func TestTimingProjectionRejectsFalseInsideAllowedLabel(t *testing.T) {
	b, a := load(t, "regression")
	r := run(t, b, a, func(req *Request) {
		req.Policies = []ObjectPolicy{{Domain: DomainACL, Name: "MGMT", Policy: repair.Policy{
			Schema: repair.PolicySchema, Scope: intent.Scope, OperatorAccepted: true, AllowedChanges: []intent.Region{},
			Conditions: intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_remain", Match: intent.Region{Protocol: "ip", SrcCIDR: "0.0.0.0/0", DstCIDR: "0.0.0.0/0"}}}},
		}}}
	})
	raw, _ := json.Marshal(r)
	var fresh Result
	if err := json.Unmarshal(raw, &fresh); err != nil {
		t.Fatal(err)
	}
	i := -1
	for j, o := range fresh.Objects {
		if o.Name == "MGMT" && o.ACL != nil && len(o.ACL.Changes.Classes) > 0 {
			i = j
			break
		}
	}
	if i < 0 {
		t.Fatal("fixture missing scoped ACL")
	}
	c := &fresh.Objects[i].ACL.Changes
	c.Complete = false
	c.Reason = "wall_clock"
	c.Budget.StopReason = "wall_clock"
	c.Budget.TimingLimited = true
	c.Classes = []verify.ChangeClass{}
	c.TotalClasses = 0
	c.ClassesTruncated = false
	if _, timed, err := replaySafety(context.Background(), []byte(b), []byte(a), r, fresh); err != nil || !timed {
		t.Fatalf("valid partial replay rejected: %v", err)
	}
	r.Objects[i].ACL.Changes.Classes[0].Scope = "inside_allowed"
	r.Objects[i].ACL.Changes.Classes[0].OutsideWitness = nil
	if _, _, err := replaySafety(context.Background(), []byte(b), []byte(a), r, fresh); err == nil {
		t.Fatal("outside packet relabeled inside passed partial replay")
	}
}

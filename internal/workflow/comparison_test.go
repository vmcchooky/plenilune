package workflow

import (
	"context"
	"encoding/json"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
	"plenilune/internal/verify"
	"testing"
)

var before = []byte("ip access-list extended DEMO\n permit tcp any any eq 22\n deny ip any any\n")
var after = []byte("ip access-list extended DEMO\n deny tcp host 192.0.2.10 any eq 22\n permit tcp any any eq 22\n deny ip any any\n")

func conditions() intent.Set {
	return intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.10/32", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
}

func artifact(t *testing.T) evidence.Comparison {
	t.Helper()
	s := conditions()
	c, err := Compare(context.Background(), before, after, &s, verify.DefaultOptions(), evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestComparisonRoundtripAndPredicateBinding(t *testing.T) {
	c := artifact(t)
	if c.Report.Verdict != "DIFFERENT" || c.Report.Obligations[0].Status != "VIOLATED" {
		t.Fatalf("wrong exact result: %+v", c.Report)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded evidence.Comparison
	if err := jsonio.Decode(b, evidence.MaxComparisonBytes, &decoded); err != nil {
		t.Fatal(err)
	}
	r, err := Replay(context.Background(), before, after, decoded)
	if err != nil || r.Status != "COMPARISON_REPLAYED" || r.RecheckedWitnesses < 2 {
		t.Fatalf("replay: %v %+v", err, r)
	}
	// A changed predicate invalidates the original receipt even if another
	// predicate might coincidentally receive the same outcome label.
	c.Conditions.Obligations[0].Match.DstPorts[0].Hi = 23
	if evidence.CheckComparison(c) == nil {
		t.Fatal("predicate definition excluded from digest")
	}
}

func TestReplayRejectsRehashedFalseResultAndChangedBytes(t *testing.T) {
	c := artifact(t)
	c.Report.Obligations[0].Status = "HOLDS"
	c.Report.Obligations[0].Witness = nil
	forged, err := evidence.SealComparison(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Replay(context.Background(), before, after, forged); err == nil {
		t.Fatal("rehashed false HOLDS accepted")
	}
	c = artifact(t)
	if _, err := Replay(context.Background(), append(append([]byte{}, before...), []byte("!\n")...), after, c); err == nil {
		t.Fatal("altered original bytes accepted")
	}
}

func TestDeterministicBudgetReplayAndTimingDisposition(t *testing.T) {
	opts := verify.DefaultOptions()
	opts.Limits.MaxOperations = 1
	c, err := Compare(context.Background(), before, after, nil, opts, evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Report.Verdict != "INCONCLUSIVE" || c.Report.Budget.StopReason != "operations" {
		t.Fatal("wrong logical budget outcome")
	}
	r, err := Replay(context.Background(), before, after, c)
	if err != nil || r.Status != "COMPARISON_REPLAYED" {
		t.Fatalf("logical budget must replay: %v %+v", err, r)
	}
	// This tests classifier behavior, not a claim that a real deadline ran.
	c.Report.Budget.TimingLimited = true
	c.Report.Budget.StopReason = "wall_clock"
	c.Report.Reason = "wall_clock"
	c, err = evidence.SealComparison(c)
	if err != nil {
		t.Fatal(err)
	}
	r, err = Replay(context.Background(), before, after, c)
	if err != nil || r.Status != "TIMING_SENSITIVE" {
		t.Fatalf("timing result incorrectly certified: %v %+v", err, r)
	}
}

func TestIndependentObligationWitnessWithEquivalentACLs(t *testing.T) {
	deny := []byte("ip access-list extended DEMO\n deny ip any any\n")
	s := conditions()
	c, err := Compare(context.Background(), deny, deny, &s, verify.DefaultOptions(), evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Report.Verdict != "EQUIVALENT" || c.Report.Obligations[0].Status != "VIOLATED" || c.Report.Obligations[0].Witness == nil {
		t.Fatal("obligation tied incorrectly to diff")
	}
	if _, err := Replay(context.Background(), deny, deny, c); err != nil {
		t.Fatal(err)
	}
}

package workflow

// P1-2 replay discipline: a v0.4 partial artifact replays deterministically
// (its stop is a budget dimension, not the clock), and pre-P1-2 artifacts
// stamped with the v0.3 engine keep replaying with no partial attached.

import (
	"context"
	"plenilune/internal/evidence"
	"plenilune/internal/verify"
	"testing"
)

var p12Before = []byte("ip access-list extended DEMO\n permit tcp any any eq 22\n permit tcp any any eq 80\n permit tcp any any range 8000 8099\n permit udp any any eq 53\n permit tcp any any eq 443\n deny ip any any\n")
var p12After = []byte("ip access-list extended DEMO\n permit tcp any any eq 2222\n permit tcp any any eq 8080\n permit tcp any any range 8000 8099\n permit udp any any eq 5353\n permit tcp any any eq 443\n deny ip any any\n")

func p12Tool() evidence.Tool {
	return evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"}
}

func TestP12PartialComparisonReplays(t *testing.T) {
	opts := verify.DefaultOptions()
	opts.Limits.MaxOperations = 150
	c, err := Compare(context.Background(), p12Before, p12After, nil, opts, p12Tool())
	if err != nil {
		t.Fatal(err)
	}
	if c.Report.Verdict != "DIFFERENT" || c.Report.SearchComplete || c.Report.Partial == nil {
		t.Fatalf("expected a partial DIFFERENT artifact: %+v", c.Report)
	}
	if len(c.Report.Partial.DecidedPairs) == 0 || len(c.Report.Witnesses) == 0 {
		t.Fatalf("partial artifact lost its proven part: %+v", c.Report.Partial)
	}
	r, err := Replay(context.Background(), p12Before, p12After, c)
	if err != nil || r.Status != "COMPARISON_REPLAYED" {
		t.Fatalf("partial replay: %v %+v", err, r)
	}
}

func TestP12PrePartialArtifactsStillReplay(t *testing.T) {
	// A completing v0.3 comparison.
	c, err := CompareEngine(context.Background(), before, after, nil, verify.DefaultOptions(), p12Tool(), verify.ResidualEngineVersion)
	if err != nil {
		t.Fatal(err)
	}
	if c.EngineVersion != verify.ResidualEngineVersion || c.Report.Partial != nil {
		t.Fatalf("v0.3 artifact shape changed: engine %s partial %+v", c.EngineVersion, c.Report.Partial)
	}
	if _, err := Replay(context.Background(), before, after, c); err != nil {
		t.Fatalf("v0.3 replay: %v", err)
	}
	// A stopped v0.3 comparison: all-or-nothing, no partial, still replays.
	opts := verify.DefaultOptions()
	opts.Limits.MaxOperations = 1
	c, err = CompareEngine(context.Background(), before, after, nil, opts, p12Tool(), verify.ResidualEngineVersion)
	if err != nil {
		t.Fatal(err)
	}
	if c.Report.Verdict != "INCONCLUSIVE" || c.Report.Partial != nil {
		t.Fatalf("v0.3 stopped shape changed: %+v", c.Report)
	}
	r, err := Replay(context.Background(), before, after, c)
	if err != nil || r.Status != "COMPARISON_REPLAYED" {
		t.Fatalf("v0.3 stopped replay: %v %+v", err, r)
	}
}

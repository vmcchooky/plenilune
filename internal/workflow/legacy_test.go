package workflow

import (
	"context"
	"os"
	"testing"

	"plenilune/internal/evidence"
	"plenilune/internal/jsonio"
)

// Frozen v0.1 receipts from the recorded backend case must keep replaying
// after the engine version bump.
func TestFrozenV01ComparisonReplays(t *testing.T) {
	before, err := os.ReadFile("../../testdata/acl/exact-before.acl")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile("../../testdata/acl/management.acl")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../testdata/receipts/comparison-v01.json")
	if err != nil {
		t.Fatal(err)
	}
	var c evidence.Comparison
	if err := jsonio.Decode(raw, evidence.MaxComparisonBytes, &c); err != nil {
		t.Fatal(err)
	}
	if c.EngineVersion != "acl-exact/v0.1" {
		t.Fatalf("fixture engine %s", c.EngineVersion)
	}
	r, err := Replay(context.Background(), before, after, c)
	if err != nil || r.Status != "COMPARISON_REPLAYED" || r.SHA256 != c.SHA256 {
		t.Fatalf("legacy replay: %v %+v", err, r)
	}
}

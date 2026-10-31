package repair

import (
	"context"
	"os"
	"testing"

	"plenilune/internal/jsonio"
)

// A frozen v0.1 repair receipt still rerenders and replays under v0.1 accounting.
func TestFrozenV01RepairReplays(t *testing.T) {
	before, err := os.ReadFile("../../testdata/acl/exact-before.acl")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile("../../testdata/acl/management.acl")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../testdata/receipts/repair-v01.json")
	if err != nil {
		t.Fatal(err)
	}
	var r Result
	if err := jsonio.Decode(raw, MaxJSONBytes, &r); err != nil {
		t.Fatal(err)
	}
	if r.Version != LegacyVersion {
		t.Fatalf("fixture version %s", r.Version)
	}
	result, err := Replay(context.Background(), before, after, r)
	if err != nil || result.Status != "REPAIR_REPLAYED" || result.SHA256 != r.SHA256 {
		t.Fatalf("legacy repair replay: %v %+v", err, result)
	}
}

package workflow

// P2-1 (docs/plan/04-phase-2-contract-integration.md): end-to-end, the
// same inputs compared by two builds (different tool commits) produce
// one content identity and two artifact identities, and replay surfaces
// both digests.

import (
	"context"
	"testing"

	"plenilune/internal/evidence"
	"plenilune/internal/verify"
)

func TestP21SameContentAcrossBuilds(t *testing.T) {
	ctx := context.Background()
	s := conditions()
	a, err := Compare(ctx, before, after, &s, verify.DefaultOptions(), evidence.Tool{Version: evidence.ToolVersion, Commit: "build-a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compare(ctx, before, after, &s, verify.DefaultOptions(), evidence.Tool{Version: evidence.ToolVersion, Commit: "build-b"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Schema != evidence.ComparisonSchema || a.ContentSHA256 == "" {
		t.Fatalf("new comparisons must be %s with a content digest", evidence.ComparisonSchema)
	}
	if a.ContentSHA256 != b.ContentSHA256 {
		t.Fatalf("content identity split across builds:\n%s\n%s", a.ContentSHA256, b.ContentSHA256)
	}
	if a.SHA256 == b.SHA256 {
		t.Fatal("artifact identity must cover the tool commit")
	}
	r, err := Replay(ctx, before, after, a)
	if err != nil || r.Status != "COMPARISON_REPLAYED" {
		t.Fatalf("replay: %v %+v", err, r)
	}
	if r.SHA256 != a.SHA256 || r.ContentSHA256 != a.ContentSHA256 {
		t.Fatalf("replay must surface both digests: %+v", r)
	}
}

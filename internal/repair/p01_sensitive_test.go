package repair

// P0-1 (docs/plan/02-phase-0-hardening.md): a repair receipt embeds the full
// candidate configuration text, so it is labeled sensitive. The label is
// handling metadata outside the sealed digest projection: receipts sealed
// before the label existed keep their digest and stay replayable.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"plenilune/internal/jsonio"
	"plenilune/internal/verify"
)

func TestP01CandidateReceiptIsLabeledSensitive(t *testing.T) {
	b, a, p, req := fixture(t)
	r, err := Verify(context.Background(), b, a, &p, req, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if r.Candidate == "" || !r.Sensitive {
		t.Fatalf("receipt with candidate must be labeled sensitive: %+v", r)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"sensitive":true`) {
		t.Fatalf("label missing on the wire: %s", data)
	}
	// The label is not evidence content: the identical receipt without it
	// (the pre-label wire shape) has the same digest, passes Check and
	// replays.
	legacy := r
	legacy.Sensitive = false
	d1, err := digest(r)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := digest(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 || d1 != r.SHA256 {
		t.Fatalf("label changed the sealed digest: %s vs %s vs %s", d1, d2, r.SHA256)
	}
	wire, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "sensitive") {
		t.Fatalf("unlabeled shape must omit the field: %s", wire)
	}
	var saved Result
	if err := jsonio.Decode(wire, MaxJSONBytes, &saved); err != nil {
		t.Fatal(err)
	}
	if err := Check(saved); err != nil {
		t.Fatalf("pre-label receipt fails check: %v", err)
	}
	result, err := Replay(context.Background(), b, a, saved)
	if err != nil || result.Status != "REPAIR_REPLAYED" {
		t.Fatalf("pre-label receipt replay: %v %+v", err, result)
	}
}

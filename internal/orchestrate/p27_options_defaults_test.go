package orchestrate

import (
	"context"
	"testing"

	"plenilune/internal/packetset"
	"plenilune/internal/verify"
)

// P2-7-fix-2: the session surface takes the same per-field defaulting
// as compare/repair — a partial Options no longer zeroes the limits
// and fails the whole session.
func TestSessionPartialOptionsTakeDefaults(t *testing.T) {
	in := request()
	in.Enabled = false
	in.Options = &verify.Options{MaxWitnesses: 3}
	out, err := Run(context.Background(), in, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	got := out.Comparison.Report.Options
	if got.MaxWitnesses != 3 || got.Limits != packetset.Defaults() {
		t.Fatalf("partial options not resolved per field: %+v", got)
	}
}

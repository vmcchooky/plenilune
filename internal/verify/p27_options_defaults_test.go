package verify

import (
	"testing"
	"time"

	"plenilune/internal/packetset"
)

// P2-7-fix-2: verify.Options takes the same per-field defaulting rule
// the analysis options family got in P2-7-fix. Zero is never a valid
// explicit value for any field (every validation floor is >= 1), so at
// a request boundary zero means "unset".

func TestOptionsWithDefaultsFillsEveryZeroField(t *testing.T) {
	if got := (Options{}).WithDefaults(); got != DefaultOptions() {
		t.Fatalf("empty options: got %+v, want defaults %+v", got, DefaultOptions())
	}
}

func TestOptionsWithDefaultsKeepsExplicitFields(t *testing.T) {
	in := Options{MaxWitnesses: 4, Limits: packetset.Limits{MaxBoxes: 10}}
	want := Options{MaxWitnesses: 4, Limits: packetset.Limits{MaxOperations: 2_000_000, MaxBoxes: 10, Timeout: 2 * time.Second}}
	if got := in.WithDefaults(); got != want {
		t.Fatalf("partial options: got %+v, want %+v", got, want)
	}
}

func TestOptionsWithDefaultsLeavesFullOptionsUnchanged(t *testing.T) {
	in := Options{MaxWitnesses: 64, Limits: packetset.Limits{MaxOperations: 7, MaxBoxes: 9, Timeout: time.Second}}
	if got := in.WithDefaults(); got != in {
		t.Fatalf("full options changed: got %+v, want %+v", got, in)
	}
}

// Defaulting replaces zero only: negative and over-cap values pass
// through untouched so the existing validation keeps rejecting them.
func TestOptionsWithDefaultsDoesNotHealOutOfBounds(t *testing.T) {
	in := Options{MaxWitnesses: -1, Limits: packetset.Limits{MaxOperations: 3_000_000, MaxBoxes: -5, Timeout: -time.Second}}
	if got := in.WithDefaults(); got != in {
		t.Fatalf("out-of-bounds values healed: got %+v, want %+v", got, in)
	}
}

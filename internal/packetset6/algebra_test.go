package packetset6

import (
	"context"
	"math/rand/v2"
	"testing"

	"plenilune/internal/packetset"
)

// Reduced-domain oracle: over a small synthetic subspace, box algebra must
// agree with brute-force point membership on every sampled packet. This is
// the independent check the 128-bit generalization stands on.

func testBudget(t *testing.T) *packetset.Budget {
	t.Helper()
	b, err := packetset.NewBudget(context.Background(), packetset.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// randCube draws a cube inside 2001:db8::/120 with a random wildcard over
// the low 16 bits, so random cubes overlap often.
func randCube(r *rand.Rand) Cube128 {
	base := [2]uint64{0x20010db800000000, 0}
	v := uint64(r.IntN(256))
	w := uint64(r.IntN(65536))
	return Cube128{
		Value:    [2]uint64{base[0], v &^ w},
		Wildcard: [2]uint64{0, w},
	}
}

func randInterval16(r *rand.Rand, max uint16) packetset.Interval {
	lo := uint16(r.IntN(int(max)))
	hi := lo + uint16(r.IntN(int(max-lo)))
	return packetset.Interval{Lo: lo, Hi: hi}
}

func randBox(r *rand.Rand) Box {
	proto := packetset.Protocol(58)
	if r.IntN(2) == 0 {
		proto = packetset.Protocol(6)
	}
	return Box{
		Protocols: proto,
		Src:       randCube(r),
		Dst:       randCube(r),
		SrcPort:   randInterval16(r, 64),
		DstPort:   randInterval16(r, 64),
		IcmpType:  randInterval16(r, 255),
	}
}

func randPacket(r *rand.Rand) Packet {
	proto := uint8(58)
	if r.IntN(2) == 0 {
		proto = 6
	}
	return Packet{
		Protocol: proto,
		Src:      [2]uint64{0x20010db800000000, uint64(r.IntN(256))},
		Dst:      [2]uint64{0x20010db800000000, uint64(r.IntN(256))},
		SrcPort:  uint16(r.IntN(64)),
		DstPort:  uint16(r.IntN(64)),
		IcmpType: uint8(r.IntN(256)),
	}
}

func TestAlgebraMatchesPointMembership(t *testing.T) {
	r := rand.New(rand.NewPCG(3307, 42))
	for iter := 0; iter < 300; iter++ {
		a := Region{randBox(r), randBox(r)}
		b := Region{randBox(r)}
		diff, err := Difference(a, b, testBudget(t))
		if err != nil {
			t.Fatalf("iter %d difference: %v", iter, err)
		}
		inter, err := Intersection(a, b, testBudget(t))
		if err != nil {
			t.Fatalf("iter %d intersection: %v", iter, err)
		}
		for s := 0; s < 40; s++ {
			p := randPacket(r)
			inA, inB := RegionContains(a, p), RegionContains(b, p)
			if got := RegionContains(diff, p); got != (inA && !inB) {
				t.Fatalf("iter %d packet %+v: difference membership %v, want %v (A=%v B=%v)", iter, p, got, inA && !inB, inA, inB)
			}
			if got := RegionContains(inter, p); got != (inA && inB) {
				t.Fatalf("iter %d packet %+v: intersection membership %v, want %v (A=%v B=%v)", iter, p, got, inA && inB, inA, inB)
			}
		}
	}
}

func TestDifferenceOfSingleBoxIsDisjoint(t *testing.T) {
	r := rand.New(rand.NewPCG(99, 7))
	for iter := 0; iter < 200; iter++ {
		a := Region{randBox(r)}
		b := Region{randBox(r)}
		diff, err := Difference(a, b, testBudget(t))
		if err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		for i := 0; i < len(diff); i++ {
			for j := i + 1; j < len(diff); j++ {
				if _, ok := Intersect(diff[i], diff[j]); ok {
					t.Fatalf("iter %d: difference slabs %d and %d overlap", iter, i, j)
				}
			}
		}
	}
}

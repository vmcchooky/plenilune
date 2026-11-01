package packetset

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// Fuzz target for the packet-set algebra (plan P1-6). Boxes are generated
// from the fuzz bytes over a reduced domain so membership can be checked
// exhaustively against the direct-bit oracle from algebra_test.go.
// Invariants (the algebra's documented contract): no panic, errors are
// only budget/invalid typed errors, output boxes are valid, for every
// sampled point membership of Intersection/Difference results matches the
// pointwise boolean combination of the operands, and disjoint inputs
// yield pairwise-disjoint outputs. Output ORDER is not part of the
// contract: the algebra emits in operand order and every production
// caller sorts with Sort itself (see internal/verify). A first version
// of this target asserted canonical sorting and the fuzzer produced a
// counterexample within seconds; the input is kept as a regression seed
// (testdata/fuzz/FuzzAlgebraInvariants/032225beb12da88c) and the target
// now asserts the documented contract instead.

type fuzzByteReader struct {
	data []byte
	pos  int
}

func (r *fuzzByteReader) next() byte {
	if r.pos >= len(r.data) {
		return 0
	}
	b := r.data[r.pos]
	r.pos++
	return b
}

func (r *fuzzByteReader) pick(n int) int { return int(r.next()) % n }

// fuzzBox builds a valid box whose cubes use few wildcard bits so the
// reduced-domain point grid below intersects it often.
func fuzzBox(r *fuzzByteReader) Box {
	cube := func() Cube {
		v := uint32(r.next())<<24 | uint32(r.next())<<16 | uint32(r.next())<<8 | uint32(r.next())
		w := uint32(0)
		switch r.pick(4) {
		case 0: // host
		case 1: // /24-ish
			w = 0xFF
		case 2: // /16-ish
			w = 0xFFFF
		case 3: // scattered low bits
			w = uint32(r.next()) | uint32(r.next())<<8
		}
		return Cube{Value: v &^ w, Wildcard: w}
	}
	interval := func() Interval {
		lo := uint16(r.pick(6)) * 100
		return Interval{Lo: lo, Hi: lo + uint16(r.pick(4))*100}
	}
	protos := Protocols{}
	for _, p := range []uint8{1, 6, 17, 47} {
		if r.pick(2) == 0 {
			protos[p/64] |= 1 << (p % 64)
		}
	}
	if protos == (Protocols{}) {
		protos = Protocol(6)
	}
	return Box{Protocols: protos, Src: cube(), Dst: cube(), SrcPort: interval(), DstPort: interval()}
}

func fuzzRegion(r *fuzzByteReader) Region {
	n := 1 + r.pick(3)
	out := make(Region, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fuzzBox(r))
	}
	return out
}

func regionHas(reg Region, p uint8, src, dst uint32, sp, dp uint16) bool {
	for _, b := range reg {
		if oracle(b, p, src, dst, sp, dp) {
			return true
		}
	}
	return false
}

// disjointRegion folds generated boxes into a pairwise-disjoint region
// using Difference itself; the membership checks below validate the
// construction independently through the oracle.
func disjointRegion(t *testing.T, r *fuzzByteReader, budget *Budget) Region {
	t.Helper()
	out := Region{}
	for i, n := 0, 1+r.pick(3); i < n; i++ {
		box := fuzzBox(r)
		pieces, err := Difference(Region{box}, out, budget)
		if err != nil {
			t.Fatalf("Difference while building disjoint region: %v", err)
		}
		out = append(out, pieces...)
	}
	return out
}

func assertPairwiseDisjoint(t *testing.T, reg Region) {
	t.Helper()
	for i := range reg {
		for j := i + 1; j < len(reg); j++ {
			if _, ok := Intersect(reg[i], reg[j]); ok {
				t.Fatalf("output boxes %d and %d overlap:\n%+v\n%+v", i, j, reg[i], reg[j])
			}
		}
	}
}

func FuzzAlgebraInvariants(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 0, 255, 255, 255, 255, 6, 22, 80, 187})
	f.Add([]byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120})
	f.Fuzz(func(t *testing.T, data []byte) {
		r := &fuzzByteReader{data: data}
		a, b := fuzzRegion(r), fuzzRegion(r)
		budget, err := NewBudget(context.Background(), Defaults())
		if err != nil {
			t.Fatal(err)
		}
		defer budget.Close()

		inter, err := Intersection(a, b, budget)
		if err != nil && !errors.Is(err, ErrLimit) && !errors.Is(err, ErrInvalid) {
			t.Fatalf("Intersection returned untyped error: %v", err)
		}
		diff, err := Difference(a, b, budget)
		if err != nil && !errors.Is(err, ErrLimit) && !errors.Is(err, ErrInvalid) {
			t.Fatalf("Difference returned untyped error: %v", err)
		}
		for _, reg := range []Region{inter, diff} {
			for _, box := range reg {
				if !valid(box) {
					t.Fatalf("algebra returned an invalid box: %+v", box)
				}
			}
		}
		if inter != nil && diff != nil {
			assertMembership(t, a, b, inter, diff)
		}

		// Disjoint phase: the documented contract promises pairwise
		// disjoint outputs for disjoint inputs.
		budget2, err := NewBudget(context.Background(), Defaults())
		if err != nil {
			t.Fatal(err)
		}
		defer budget2.Close()
		da, db := disjointRegion(t, r, budget2), disjointRegion(t, r, budget2)
		assertPairwiseDisjoint(t, da)
		assertPairwiseDisjoint(t, db)
		inter2, err := Intersection(da, db, budget2)
		if err != nil {
			t.Fatalf("Intersection of disjoint regions failed: %v", err)
		}
		diff2, err := Difference(da, db, budget2)
		if err != nil {
			t.Fatalf("Difference of disjoint regions failed: %v", err)
		}
		assertPairwiseDisjoint(t, inter2)
		assertPairwiseDisjoint(t, diff2)
		assertMembership(t, da, db, inter2, diff2)
	})
}

// assertMembership checks exhaustive membership accounting over a reduced
// point grid built from the operand cubes' fixed bits plus boundaries.
func assertMembership(t *testing.T, a, b, inter, diff Region) {
	t.Helper()
	srcs := map[uint32]bool{}
	dsts := map[uint32]bool{}
	ports := map[uint16]bool{0: true, 22: true, 80: true, 100: true, 200: true, 300: true, 65535: true}
	for _, reg := range []Region{a, b} {
		for _, box := range reg {
			srcs[box.Src.Value] = true
			srcs[box.Src.Value^box.Src.Wildcard] = true
			dsts[box.Dst.Value] = true
			dsts[box.Dst.Value^box.Dst.Wildcard] = true
			ports[box.SrcPort.Lo] = true
			ports[box.SrcPort.Hi] = true
			ports[box.DstPort.Lo] = true
			ports[box.DstPort.Hi] = true
		}
	}
	srcList := make([]uint32, 0, len(srcs))
	for v := range srcs {
		srcList = append(srcList, v)
	}
	dstList := make([]uint32, 0, len(dsts))
	for v := range dsts {
		dstList = append(dstList, v)
	}
	portList := make([]uint16, 0, len(ports))
	for v := range ports {
		portList = append(portList, v)
	}
	// Deterministic, bounded grid: sorted values, capped counts.
	slices.Sort(srcList)
	slices.Sort(dstList)
	slices.Sort(portList)
	if len(srcList) > 6 {
		srcList = srcList[:6]
	}
	if len(dstList) > 6 {
		dstList = dstList[:6]
	}
	if len(portList) > 8 {
		portList = portList[:8]
	}
	for _, proto := range []uint8{1, 6, 17, 47} {
		for _, src := range srcList {
			for _, dst := range dstList {
				for _, sp := range portList {
					for _, dp := range portList {
						inA := regionHas(a, proto, src, dst, sp, dp)
						inB := regionHas(b, proto, src, dst, sp, dp)
						if got := regionHas(inter, proto, src, dst, sp, dp); got != (inA && inB) {
							t.Fatalf("intersection membership wrong at proto=%d src=%08x dst=%08x %d->%d: got %v want %v", proto, src, dst, sp, dp, got, inA && inB)
						}
						if got := regionHas(diff, proto, src, dst, sp, dp); got != (inA && !inB) {
							t.Fatalf("difference membership wrong at proto=%d src=%08x dst=%08x %d->%d: got %v want %v", proto, src, dst, sp, dp, got, inA && !inB)
						}
					}
				}
			}
		}
	}
}

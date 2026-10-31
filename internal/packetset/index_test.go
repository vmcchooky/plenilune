package packetset

import (
	"context"
	"math/rand/v2"
	"reflect"
	"testing"
)

func randomIndexBox(r *rand.Rand) Box {
	cube := func() Cube {
		switch r.IntN(4) {
		case 0:
			return Cube{Wildcard: ^uint32(0)}
		case 1:
			w := ^uint32(0) >> uint(r.IntN(33))
			if r.IntN(2) == 0 {
				w = 0
			}
			v := (0xC0000200 | uint32(r.IntN(256))) &^ w
			return Cube{Value: v, Wildcard: w}
		case 2:
			// Non-contiguous wildcard inside a small documentation range.
			w := uint32(r.IntN(256)) & 0b10110101
			return Cube{Value: (0xC0000200 | uint32(r.IntN(256))) &^ w, Wildcard: w}
		default:
			w := ^uint32(0) >> uint(16+r.IntN(17))
			return Cube{Value: (0xC6336400 | uint32(r.IntN(1<<16))) &^ w, Wildcard: w}
		}
	}
	port := func() Interval {
		lo := uint16(r.IntN(70))
		return Interval{Lo: lo, Hi: lo + uint16(r.IntN(40))}
	}
	var protos Protocols
	switch r.IntN(3) {
	case 0:
		protos = Protocol(6)
	case 1:
		protos = Protocol(17)
	default:
		protos = AllProtocols()
	}
	return Box{Protocols: protos, Src: cube(), Dst: cube(), SrcPort: port(), DstPort: port()}
}

func budgetFor(t *testing.T) *Budget {
	t.Helper()
	b, err := NewBudget(context.Background(), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The index must not change set-operation output, only how many cuts are
// visited. Compare against the original full scan box-for-box, in order.
func TestIndexedOperationsMatchFullScan(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	for trial := 0; trial < 400; trial++ {
		a, b := Region{}, Region{}
		for i := r.IntN(6) + 1; i > 0; i-- {
			a = append(a, randomIndexBox(r))
		}
		for i := r.IntN(10); i > 0; i-- {
			b = append(b, randomIndexBox(r))
		}
		want, err := Difference(a, b, budgetFor(t))
		if err != nil {
			t.Fatal(err)
		}
		budget := budgetFor(t)
		idx, err := IndexOf(b, budget)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DifferenceIndexed(a, idx, budget)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d difference differs\nwant %v\n got %v", trial, want, got)
		}
		wantI, err := Intersection(a, b, budgetFor(t))
		if err != nil {
			t.Fatal(err)
		}
		gotI, err := IntersectionIndexed(a, idx, budgetFor(t))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotI, wantI) {
			t.Fatalf("trial %d intersection differs\nwant %v\n got %v", trial, wantI, gotI)
		}
	}
}

// Every cut that really intersects a box must be a candidate.
func TestIndexCandidatesAreComplete(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 5))
	idx := NewIndex()
	cuts := Region{}
	for i := 0; i < 300; i++ {
		cuts = append(cuts, randomIndexBox(r))
	}
	if err := idx.Add(cuts, budgetFor(t)); err != nil {
		t.Fatal(err)
	}
	for q := 0; q < 300; q++ {
		box := randomIndexBox(r)
		ids, err := idx.Candidates(box, budgetFor(t))
		if err != nil {
			t.Fatal(err)
		}
		seen := map[int]bool{}
		for i, id := range ids {
			if i > 0 && ids[i-1] >= id {
				t.Fatal("candidates are not strictly ascending")
			}
			seen[id] = true
		}
		for i, cut := range cuts {
			if _, ok := Intersect(box, cut); ok && !seen[i] {
				t.Fatalf("intersecting cut %d missing from candidates", i)
			}
		}
	}
}

// Index must not hide boxes from the workspace budget.
func TestIndexRespectsBoxBudget(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	region := Region{}
	for i := 0; i < 5; i++ {
		region = append(region, randomIndexBox(r))
	}
	b, err := NewBudget(context.Background(), Limits{MaxOperations: 100, MaxBoxes: 3, Timeout: 2e9})
	if err != nil {
		t.Fatal(err)
	}
	idx := NewIndex()
	if err := idx.Add(region, b); err == nil {
		t.Fatal("box budget not enforced")
	}
}

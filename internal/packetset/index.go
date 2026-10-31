package packetset

import (
	"math/bits"
	"slices"
)

// Index finds the cuts that may intersect a box. Each box is filed under the
// covering fixed-prefix of its source and destination cubes; two cubes can
// intersect only when one covering prefix contains the other. Candidates are a
// superset of intersecting cuts, returned in insertion order, so callers that
// apply them sequentially produce the same boxes as a scan of every cut.
type Index struct {
	boxes Region
	src   prefixIndex
	dst   prefixIndex
}

type prefixKey struct {
	bits  uint8
	value uint32
}

type prefixIndex struct {
	exact   map[prefixKey][]int
	byValue []valued
}

type valued struct {
	value uint32
	bits  uint8
	id    int
}

func NewIndex() *Index {
	return &Index{src: prefixIndex{exact: map[prefixKey][]int{}}, dst: prefixIndex{exact: map[prefixKey][]int{}}}
}

func (x *Index) Len() int { return len(x.boxes) }

func (x *Index) Box(i int) Box { return x.boxes[i] }

// Add validates and files boxes in order. One logical operation per box.
func (x *Index) Add(r Region, budget *Budget) error {
	if err := budget.CheckBoxes(len(x.boxes) + len(r)); err != nil {
		return err
	}
	for _, b := range r {
		if err := budget.CheckBoxes(len(x.boxes) + len(r)); err != nil {
			return err
		}
		if err := budget.Tick(1); err != nil {
			return err
		}
		if !valid(b) {
			return ErrInvalid
		}
		id := len(x.boxes)
		x.boxes = append(x.boxes, b)
		x.src.add(b.Src, id)
		x.dst.add(b.Dst, id)
	}
	return nil
}

func covering(c Cube) prefixKey {
	n := uint8(bits.LeadingZeros32(c.Wildcard))
	if c.Wildcard == 0 {
		n = 32
	}
	return prefixKey{bits: n, value: c.Value & prefixMask(n)}
}

func prefixMask(n uint8) uint32 {
	if n == 0 {
		return 0
	}
	return ^uint32(0) << (32 - n)
}

func (p *prefixIndex) add(c Cube, id int) {
	k := covering(c)
	p.exact[k] = append(p.exact[k], id)
	at, _ := slices.BinarySearchFunc(p.byValue, valued{value: k.value, id: id}, compareValued)
	p.byValue = slices.Insert(p.byValue, at, valued{value: k.value, bits: k.bits, id: id})
}

func compareValued(a, b valued) int {
	if a.value != b.value {
		if a.value < b.value {
			return -1
		}
		return 1
	}
	return a.id - b.id
}

// count reports how many candidates a query would produce without allocating.
func (p *prefixIndex) query(c Cube, out []int) []int {
	k := covering(c)
	for n := uint8(0); n <= k.bits; n++ {
		out = append(out, p.exact[prefixKey{bits: n, value: k.value & prefixMask(n)}]...)
	}
	hi := k.value | ^prefixMask(k.bits)
	at, _ := slices.BinarySearchFunc(p.byValue, valued{value: k.value}, compareValued)
	for ; at < len(p.byValue) && p.byValue[at].value <= hi; at++ {
		if p.byValue[at].bits > k.bits {
			out = append(out, p.byValue[at].id)
		}
	}
	return out
}

// Candidates returns cut indices, ascending, that may intersect b. It uses the
// more specific dimension; the choice affects only accounting, not results.
func (x *Index) Candidates(b Box, budget *Budget) ([]int, error) {
	if err := budget.Tick(1); err != nil {
		return nil, err
	}
	dim := &x.dst
	cube := b.Dst
	if covering(b.Src).bits > covering(b.Dst).bits {
		dim, cube = &x.src, b.Src
	}
	ids := dim.query(cube, nil)
	slices.Sort(ids)
	if err := budget.Tick(len(ids)); err != nil {
		return nil, err
	}
	return ids, nil
}

// DifferenceIndexed computes a\cuts with the same output boxes, in the same
// order, as Difference(a, cuts-in-insertion-order). Only accounting differs.
func DifferenceIndexed(a Region, cuts *Index, budget *Budget) (Region, error) {
	if err := budget.CheckBoxes(len(a) + cuts.Len()); err != nil {
		return nil, err
	}
	if err := validateRegion(a, budget); err != nil {
		return nil, err
	}
	out := Region{}
	for _, box := range a {
		ids, err := cuts.Candidates(box, budget)
		if err != nil {
			return nil, err
		}
		work := Region{box}
		for _, id := range ids {
			if len(work) == 0 {
				break
			}
			next := Region{}
			emit := func(piece Box) error {
				if err := budget.CheckBoxes(len(a) + cuts.Len() + len(out) + len(work) + len(next) + 1); err != nil {
					return err
				}
				next = append(next, piece)
				return nil
			}
			for _, w := range work {
				if err := subtract(w, cuts.boxes[id], budget, emit); err != nil {
					return nil, err
				}
			}
			work = next
		}
		out = append(out, work...)
	}
	return out, nil
}

// IntersectionIndexed returns the same boxes, in the same order, as
// Intersection(a, cuts-in-insertion-order).
func IntersectionIndexed(a Region, cuts *Index, budget *Budget) (Region, error) {
	if err := budget.CheckBoxes(len(a) + cuts.Len()); err != nil {
		return nil, err
	}
	if err := validateRegion(a, budget); err != nil {
		return nil, err
	}
	out := Region{}
	for _, x := range a {
		ids, err := cuts.Candidates(x, budget)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if z, ok := Intersect(x, cuts.boxes[id]); ok {
				if err := budget.CheckBoxes(len(a) + cuts.Len() + len(out) + 1); err != nil {
					return nil, err
				}
				out = append(out, z)
			}
		}
	}
	return out, nil
}

// IndexOf builds an index over r for one-shot set operations.
func IndexOf(r Region, budget *Budget) (*Index, error) {
	x := NewIndex()
	if err := x.Add(r, budget); err != nil {
		return nil, err
	}
	return x, nil
}

// Subtract emits a\cut as disjoint pieces using the package's fixed
// dimension order. Exposed for callers that track piece provenance.
func Subtract(a, cut Box, budget *Budget, emit func(Box) error) error {
	return subtract(a, cut, budget, emit)
}

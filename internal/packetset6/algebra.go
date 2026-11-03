package packetset6

import (
	"slices"

	"plenilune/internal/packetset"
)

func validateRegion(r Region, budget *packetset.Budget) error {
	for _, b := range r {
		if err := budget.Tick(1); err != nil {
			return err
		}
		if !valid(b) {
			return packetset.ErrInvalid
		}
	}
	return nil
}

// Difference computes A\B. If A is disjoint the output is disjoint. Neither
// input is mutated; no partial output is returned after a budget stop.
// Mirrors packetset.Difference exactly.
func Difference(a, b Region, budget *packetset.Budget) (Region, error) {
	if err := budget.CheckBoxes(len(a) + len(b)); err != nil {
		return nil, err
	}
	if err := validateRegion(a, budget); err != nil {
		return nil, err
	}
	if err := validateRegion(b, budget); err != nil {
		return nil, err
	}
	if err := budget.CheckBoxes(2*len(a) + len(b)); err != nil {
		return nil, err
	}
	work := slices.Clone(a)
	base := len(a) + len(b)
	for _, cut := range b {
		if len(work) == 0 {
			break
		}
		next := Region{}
		emit := func(box Box) error {
			if err := budget.CheckBoxes(base + len(work) + len(next) + 1); err != nil {
				return err
			}
			next = append(next, box)
			return nil
		}
		for _, box := range work {
			if err := subtract(box, cut, budget, emit); err != nil {
				return nil, err
			}
		}
		work = next
	}
	return work, nil
}

// Intersection computes every pair intersection; mirrors packetset.
func Intersection(a, b Region, budget *packetset.Budget) (Region, error) {
	if err := budget.CheckBoxes(len(a) + len(b)); err != nil {
		return nil, err
	}
	if err := validateRegion(a, budget); err != nil {
		return nil, err
	}
	if err := validateRegion(b, budget); err != nil {
		return nil, err
	}
	out := Region{}
	for _, x := range a {
		for _, y := range b {
			if err := budget.Tick(1); err != nil {
				return nil, err
			}
			if z, ok := Intersect(x, y); ok {
				if err := budget.CheckBoxes(len(a) + len(b) + len(out) + 1); err != nil {
					return nil, err
				}
				out = append(out, z)
			}
		}
	}
	return out, nil
}

// subtract partitions along dimensions in a fixed order: protocols, source,
// destination, source port, destination port, ICMPv6 type. Each emitted slab
// lies outside the intersection; the progressively restricted remainder
// enters the next dimension. The final remainder is the removed intersection.
func subtract(a, b Box, budget *packetset.Budget, emit func(Box) error) error {
	if err := budget.Tick(1); err != nil {
		return err
	}
	i, ok := Intersect(a, b)
	if !ok {
		return emit(a)
	}
	r := a
	var outside packetset.Protocols
	for k := range outside {
		outside[k] = r.Protocols[k] &^ i.Protocols[k]
	}
	if outside != (packetset.Protocols{}) {
		piece := r
		piece.Protocols = outside
		if err := emit(piece); err != nil {
			return err
		}
	}
	r.Protocols = i.Protocols
	if err := subtractAddress(&r, i.Src, true, budget, emit); err != nil {
		return err
	}
	if err := subtractAddress(&r, i.Dst, false, budget, emit); err != nil {
		return err
	}
	if err := subtractInterval(&r, i.SrcPort, dimSrcPort, budget, emit); err != nil {
		return err
	}
	if err := subtractInterval(&r, i.DstPort, dimDstPort, budget, emit); err != nil {
		return err
	}
	return subtractInterval(&r, i.IcmpType, dimIcmpType, budget, emit)
}

// subtractAddress splits one 128-bit cube dimension bit by bit: for every
// bit the working cube leaves free but the intersection fixes, emit the slab
// outside that bit, then fix the bit in the remainder. Both words are
// walked from the most significant bit down.
func subtractAddress(r *Box, intersection Cube128, source bool, budget *packetset.Budget, emit func(Box) error) error {
	c := &r.Dst
	if source {
		c = &r.Src
	}
	for w := 0; w < 2; w++ {
		for {
			split := c.Wildcard[w] &^ intersection.Wildcard[w]
			if split == 0 {
				break
			}
			if err := budget.Tick(1); err != nil {
				return err
			}
			bit := split & -split
			piece := *r
			outside := &piece.Dst
			if source {
				outside = &piece.Src
			}
			outside.Wildcard[w] &^= bit
			outside.Value[w] = (outside.Value[w] &^ bit) | ((intersection.Value[w] ^ bit) & bit)
			if err := emit(piece); err != nil {
				return err
			}
			c.Wildcard[w] &^= bit
			c.Value[w] = (c.Value[w] &^ bit) | (intersection.Value[w] & bit)
		}
	}
	return nil
}

type intervalDim int

const (
	dimSrcPort intervalDim = iota
	dimDstPort
	dimIcmpType
)

func intervalPtr(b *Box, dim intervalDim) *packetset.Interval {
	switch dim {
	case dimSrcPort:
		return &b.SrcPort
	case dimDstPort:
		return &b.DstPort
	default:
		return &b.IcmpType
	}
}

func subtractInterval(r *Box, intersection packetset.Interval, dim intervalDim, budget *packetset.Budget, emit func(Box) error) error {
	if err := budget.Tick(1); err != nil {
		return err
	}
	p := intervalPtr(r, dim)
	if p.Lo < intersection.Lo {
		piece := *r
		intervalPtr(&piece, dim).Hi = intersection.Lo - 1
		if err := emit(piece); err != nil {
			return err
		}
	}
	if p.Hi > intersection.Hi {
		piece := *r
		intervalPtr(&piece, dim).Lo = intersection.Hi + 1
		if err := emit(piece); err != nil {
			return err
		}
	}
	*p = intersection
	return nil
}

package packetset

import "slices"

func validateRegion(r Region, budget *Budget) error {
	for _, b := range r {
		if err := budget.Tick(1); err != nil {
			return err
		}
		if !valid(b) {
			return ErrInvalid
		}
	}
	return nil
}

// Difference computes A\B. If A is disjoint the output is disjoint. Neither
// input is mutated; no partial output is returned after a budget stop.
func Difference(a, b Region, budget *Budget) (Region, error) {
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

// Intersection computes every pair intersection. Disjoint input regions yield
// disjoint output. Non-disjoint input regions still have exact union semantics.
func Intersection(a, b Region, budget *Budget) (Region, error) {
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

// subtract partitions along dimensions in a fixed order. Each emitted slab
// lies outside the intersection; the progressively restricted remainder enters
// the next dimension. The final remainder is precisely the removed intersection.
func subtract(a, b Box, budget *Budget, emit func(Box) error) error {
	if err := budget.Tick(1); err != nil {
		return err
	}
	i, ok := Intersect(a, b)
	if !ok {
		return emit(a)
	}
	r := a
	var outside Protocols
	for k := range outside {
		outside[k] = r.Protocols[k] &^ i.Protocols[k]
	}
	if outside != (Protocols{}) {
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
	if err := subtractPort(&r, i.SrcPort, true, budget, emit); err != nil {
		return err
	}
	return subtractPort(&r, i.DstPort, false, budget, emit)
}

func subtractAddress(r *Box, intersection Cube, source bool, budget *Budget, emit func(Box) error) error {
	c := &r.Dst
	if source {
		c = &r.Src
	}
	for split := c.Wildcard &^ intersection.Wildcard; split != 0; split = c.Wildcard &^ intersection.Wildcard {
		if err := budget.Tick(1); err != nil {
			return err
		}
		bit := split & -split
		piece := *r
		outside := &piece.Dst
		if source {
			outside = &piece.Src
		}
		outside.Wildcard &^= bit
		outside.Value = (outside.Value &^ bit) | ((intersection.Value ^ bit) & bit)
		if err := emit(piece); err != nil {
			return err
		}
		c.Wildcard &^= bit
		c.Value = (c.Value &^ bit) | (intersection.Value & bit)
	}
	return nil
}

func subtractPort(r *Box, intersection Interval, source bool, budget *Budget, emit func(Box) error) error {
	if err := budget.Tick(1); err != nil {
		return err
	}
	p := &r.DstPort
	if source {
		p = &r.SrcPort
	}
	if p.Lo < intersection.Lo {
		piece := *r
		outside := &piece.DstPort
		if source {
			outside = &piece.SrcPort
		}
		outside.Hi = intersection.Lo - 1
		if err := emit(piece); err != nil {
			return err
		}
	}
	if p.Hi > intersection.Hi {
		piece := *r
		outside := &piece.DstPort
		if source {
			outside = &piece.SrcPort
		}
		outside.Lo = intersection.Hi + 1
		if err := emit(piece); err != nil {
			return err
		}
	}
	*p = intersection
	return nil
}

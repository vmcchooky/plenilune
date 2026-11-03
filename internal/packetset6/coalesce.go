package packetset6

import (
	"plenilune/internal/packetset"
)

// Coalesce reduces a region to a smaller region with the same union: it
// merges box pairs that differ in exactly one dimension by a sibling step
// — two cubes identical except one fixed bit (they merge into the parent
// cube), or two intervals that overlap or touch. Merging is exact, so
// membership is preserved.
//
// Implementation: grouping passes, not pair scans. For every bit of each
// address dimension, boxes are grouped by their parent key (that bit
// cleared in the value and set in the wildcard); two boxes landing in one
// group with complementary fixed values merge. Interval dimensions group
// by everything else and sweep by Lo. Each pass strictly reduces the box
// count or none; passes repeat to a fixpoint. Budget accounting is per
// box per sweep (the same granularity the algebra itself uses); the
// grouping maps keep wall time linear in the region size per sweep.
func Coalesce(r Region, budget *packetset.Budget) (Region, error) {
	work := append(Region{}, r...)
	for {
		before := len(work)
		var err error
		work, err = coalescePass(work, budget)
		if err != nil {
			return nil, err
		}
		if len(work) == before {
			Sort(work)
			return work, nil
		}
	}
}

func coalescePass(work Region, budget *packetset.Budget) (Region, error) {
	var err error
	if err = budget.Tick(len(work)); err != nil {
		return nil, err
	}
	if work, err = dedupe(work, budget); err != nil {
		return nil, err
	}
	for dim := 0; dim < 2; dim++ { // 0 = src, 1 = dst
		for bit := 0; bit < 128; bit++ {
			if work, err = mergeCubeBit(work, dim, bit); err != nil {
				return nil, err
			}
		}
	}
	for dim := 0; dim < 3; dim++ { // srcPort, dstPort, icmpType
		if work, err = mergeIntervalDim(work, dim, budget); err != nil {
			return nil, err
		}
	}
	return work, nil
}

func dedupe(work Region, budget *packetset.Budget) (Region, error) {
	seen := map[Box]bool{}
	out := work[:0]
	for _, b := range work {
		if err := budget.Tick(1); err != nil {
			return nil, err
		}
		if seen[b] {
			continue
		}
		seen[b] = true
		out = append(out, b)
	}
	return out, nil
}

type cubeKey struct {
	protocols        packetset.Protocols
	srcPort, dstPort packetset.Interval
	icmpType         packetset.Interval
	other            Cube128
	wildcard         [2]uint64
	parentValue      [2]uint64
}

// mergeCubeBit merges sibling cubes along one bit of one address
// dimension: boxes whose cubes agree everywhere except that bit (fixed in
// both, with opposite values) and whose wildcards are identical. Cost is
// accounted by the caller's per-sweep tick, not per box.
func mergeCubeBit(work Region, dim, bit int) (Region, error) {
	w, i := bit/64, uint(bit%64)
	groups := map[cubeKey]int{} // parent key -> slot in out
	out := Region{}
	for _, b := range work {
		c := b.Src
		other := b.Dst
		if dim == 1 {
			c, other = b.Dst, b.Src
		}
		if c.Wildcard[w]&(uint64(1)<<i) != 0 {
			out = append(out, b) // bit not fixed: cannot be a sibling here
			continue
		}
		parent := c.Value
		parent[w] &^= uint64(1) << i
		key := cubeKey{b.Protocols, b.SrcPort, b.DstPort, b.IcmpType, other, c.Wildcard, parent}
		if prev, ok := groups[key]; ok {
			// Complementary sibling found: widen the earlier box in place
			// into the parent cube and drop this one.
			merged := out[prev]
			mc := merged.Src
			if dim == 1 {
				mc = merged.Dst
			}
			mc.Value[w] &^= uint64(1) << i
			mc.Wildcard[w] |= uint64(1) << i
			if dim == 0 {
				merged.Src = mc
			} else {
				merged.Dst = mc
			}
			out[prev] = merged
			delete(groups, key)
			continue
		}
		groups[key] = len(out)
		out = append(out, b)
	}
	return out, nil
}

func mergeIntervalDim(work Region, dim int, budget *packetset.Budget) (Region, error) {
	type key struct {
		protocols packetset.Protocols
		src, dst  Cube128
		otherA    packetset.Interval
		otherB    packetset.Interval
	}
	get := func(b Box) packetset.Interval {
		switch dim {
		case 0:
			return b.SrcPort
		case 1:
			return b.DstPort
		default:
			return b.IcmpType
		}
	}
	set := func(b *Box, iv packetset.Interval) {
		switch dim {
		case 0:
			b.SrcPort = iv
		case 1:
			b.DstPort = iv
		default:
			b.IcmpType = iv
		}
	}
	others := func(b Box) (packetset.Interval, packetset.Interval) {
		switch dim {
		case 0:
			return b.DstPort, b.IcmpType
		case 1:
			return b.SrcPort, b.IcmpType
		default:
			return b.SrcPort, b.DstPort
		}
	}
	groups := map[key][]int{}
	for idx, b := range work {
		if err := budget.Tick(1); err != nil {
			return nil, err
		}
		oa, ob := others(b)
		k := key{b.Protocols, b.Src, b.Dst, oa, ob}
		groups[k] = append(groups[k], idx)
	}
	out := Region{}
	for _, idxs := range groups {
		boxes := make([]Box, len(idxs))
		for i, idx := range idxs {
			boxes[i] = work[idx]
		}
		// Sort by interval Lo and sweep-merge overlaps/touches.
		for i := 1; i < len(boxes); i++ {
			for j := i; j > 0 && get(boxes[j]).Lo < get(boxes[j-1]).Lo; j-- {
				boxes[j], boxes[j-1] = boxes[j-1], boxes[j]
			}
		}
		cur := boxes[0]
		for _, b := range boxes[1:] {
			civ, biv := get(cur), get(b)
			if uint32(civ.Hi)+1 >= uint32(biv.Lo) {
				set(&cur, packetset.Interval{Lo: civ.Lo, Hi: max(civ.Hi, biv.Hi)})
				continue
			}
			out = append(out, cur)
			cur = b
		}
		out = append(out, cur)
	}
	return out, nil
}

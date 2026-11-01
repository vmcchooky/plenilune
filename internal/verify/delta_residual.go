package verify

import (
	"slices"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
)

// semanticRuleEqual reports whether two rules match the same packets with
// the same action. Sequence numbers, logging flags and source positions are
// presentation, not semantics, and are ignored — this is the normalized rule
// representation the LCP alignment is defined over.
func semanticRuleEqual(a, b ciscoacl.Rule) bool {
	if a.Action != b.Action || a.Src != b.Src || a.Dst != b.Dst {
		return false
	}
	if (a.Protocol == nil) != (b.Protocol == nil) {
		return false
	}
	if a.Protocol != nil && *a.Protocol != *b.Protocol {
		return false
	}
	return slices.Equal(a.SrcPorts, b.SrcPorts) && slices.Equal(a.DstPorts, b.DstPorts)
}

// commonPrefixLen is the length of the longest prefix in which before and
// after hold semantically identical rules at the same positions.
func commonPrefixLen(before, after []ciscoacl.Rule) int {
	k := 0
	for k < len(before) && k < len(after) && semanticRuleEqual(before[k], after[k]) {
		k++
	}
	return k
}

// exactDeltaResidual is the v0.3 construction. The first k rules of the two
// ACLs are semantically identical, so the first rule matching any packet
// covered by that prefix is the same rule, with the same action, on both
// sides: no covered packet can be in the delta. The delta is therefore
// computed only from the residual rules, on the complement of the covered
// region (the union of the prefix rules' raw regions):
//
//   - each side's residual permit set is built with the covered index
//     preloaded by the prefix raw regions, in order, so every residual rule
//     sees exactly the cut sequence the v0.2 loop would have built for it
//     and its effective boxes are box-for-box v0.2's;
//   - residual work boxes lie outside the covered region, so no prefix rule
//     of the other side can decide any part of them; deciding them against
//     the other side's residual rules (plus implicit deny) yields exactly
//     the pieces v0.2 keeps from them.
//
// The returned delta's bp/ap hold the RESIDUAL permit sets only; lost and
// gained are the full delta. With no common prefix the construction
// degenerates to exactDeltaV2 unchanged.
func exactDeltaResidual(before, after ciscoacl.ACL, b *packetset.Budget) (delta, error) {
	k := commonPrefixLen(before.Rules, after.Rules)
	if k == 0 {
		return exactDeltaV2(before, after, b)
	}
	coveredSeq := make([]packetset.Region, 0, k)
	for i := 0; i < k; i++ {
		raw, err := ruleRegion(before.Rules[i], b)
		if err != nil {
			return delta{}, err
		}
		coveredSeq = append(coveredSeq, raw)
	}
	residualSet := func(rules []ciscoacl.Rule, held int) (packetset.Region, []int, error) {
		covered := packetset.NewIndex()
		for _, raw := range coveredSeq {
			if err := covered.Add(raw, b); err != nil {
				return nil, nil, err
			}
		}
		return taggedPermitSetOver(rules, covered, b, held)
	}
	bp, bTags, err := residualSet(before.Rules[k:], 0)
	if err != nil {
		return delta{}, err
	}
	ap, aTags, err := residualSet(after.Rules[k:], len(bp))
	if err != nil {
		return delta{}, err
	}
	d := delta{bp: bp, ap: ap}
	sides := []struct {
		work  packetset.Region
		tags  []int
		other ciscoacl.ACL
		lost  bool
	}{{bp, bTags, after, true}, {ap, aTags, before, false}}
	for _, side := range sides {
		if err := b.SetReserved(len(bp) + len(ap) + len(d.lost) + len(d.gained)); err != nil {
			return delta{}, err
		}
		rules, owner, err := rulesIndexOf(side.other.Rules[k:], b)
		if err != nil {
			return delta{}, err
		}
		boxes, deciders, sources, err := decideAgainst(side.work, side.other, rules, owner, b, len(bp)+len(ap)+len(d.lost)+len(d.gained))
		if err != nil {
			return delta{}, err
		}
		for i, box := range boxes {
			if actionOf(side.other, deciders[i]) == "permit" {
				continue
			}
			if side.lost {
				d.lost = append(d.lost, piece{box: box, before: side.tags[sources[i]], after: deciders[i]})
			} else {
				d.gained = append(d.gained, piece{box: box, before: deciders[i], after: side.tags[sources[i]]})
			}
		}
	}
	for _, ps := range [][]piece{d.lost, d.gained} {
		if err := b.Tick(1); err != nil {
			return delta{}, err
		}
		slices.SortFunc(ps, func(x, y piece) int { return packetset.CompareBoxes(x.box, y.box) })
	}
	return d, nil
}

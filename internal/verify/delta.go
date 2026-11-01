package verify

import (
	"slices"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
)

// piece is part of the exact difference with the rule that decides it on each
// side. Ordinal -1 means the implicit final deny.
type piece struct {
	box    packetset.Box
	before int
	after  int
}

type delta struct {
	bp, ap       packetset.Region
	lost, gained []piece
}

func (d delta) region(ps []piece) packetset.Region {
	out := make(packetset.Region, len(ps))
	for i, p := range ps {
		out[i] = p.box
	}
	return out
}

// rulesIndex files each rule's raw boxes in rule order with their ordinal.
func rulesIndex(acl ciscoacl.ACL, b *packetset.Budget) (*packetset.Index, []int, error) {
	return rulesIndexOf(acl.Rules, b)
}

// rulesIndexOf is rulesIndex over an explicit rule slice; owner entries are
// the rules' own ordinals, so a residual slice keeps global ordinals.
func rulesIndexOf(rules []ciscoacl.Rule, b *packetset.Budget) (*packetset.Index, []int, error) {
	idx, owner := packetset.NewIndex(), []int{}
	for _, rule := range rules {
		raw, err := ruleRegion(rule, b)
		if err != nil {
			return nil, nil, err
		}
		if err := idx.Add(raw, b); err != nil {
			return nil, nil, err
		}
		for range raw {
			owner = append(owner, rule.Ordinal)
		}
	}
	return idx, owner, nil
}

// taggedPermitSet returns the effective permit boxes of acl, sorted, with the
// permitting rule ordinal of each box. Boxes equal v0.1's permitSet output.
func taggedPermitSet(acl ciscoacl.ACL, b *packetset.Budget, held int) (packetset.Region, []int, error) {
	return taggedPermitSetOver(acl.Rules, packetset.NewIndex(), b, held)
}

// taggedPermitSetOver runs the taggedPermitSet loop over rules with the
// covered index as the caller prepared it. The output depends only on the
// sequence of raw regions the index was fed, in order, and on rules.
func taggedPermitSetOver(rules []ciscoacl.Rule, covered *packetset.Index, b *packetset.Budget, held int) (packetset.Region, []int, error) {
	type tagged struct {
		box  packetset.Box
		rule int
	}
	out := []tagged{}
	for _, rule := range rules {
		if err := b.SetReserved(held + len(out) + covered.Len()); err != nil {
			return nil, nil, err
		}
		raw, err := ruleRegion(rule, b)
		if err != nil {
			return nil, nil, err
		}
		if rule.Action == "permit" {
			effective, err := packetset.DifferenceIndexed(raw, covered, b)
			if err != nil {
				return nil, nil, err
			}
			if err := b.CheckBoxes(held + covered.Len() + len(raw) + len(out) + len(effective)); err != nil {
				return nil, nil, err
			}
			for _, box := range effective {
				out = append(out, tagged{box, rule.Ordinal})
			}
		}
		if err := covered.Add(raw, b); err != nil {
			return nil, nil, err
		}
	}
	if err := b.Tick(1); err != nil {
		return nil, nil, err
	}
	slices.SortFunc(out, func(x, y tagged) int { return packetset.CompareBoxes(x.box, y.box) })
	region, tags := make(packetset.Region, len(out)), make([]int, len(out))
	for i, t := range out {
		region[i], tags[i] = t.box, t.rule
	}
	return region, tags, nil
}

// decideAgainst splits every work box by the first rule of acl that matches
// it. Pieces are returned with the deciding ordinal, -1 for implicit deny,
// and the index of the originating work box. Rules not in the candidate set
// are disjoint from the work box and cannot decide any part of it.
func decideAgainst(work packetset.Region, acl ciscoacl.ACL, rules *packetset.Index, owner []int, b *packetset.Budget, held int) ([]packetset.Box, []int, []int, error) {
	boxes, deciders, sources := []packetset.Box{}, []int{}, []int{}
	for w, box := range work {
		ids, err := rules.Candidates(box, b)
		if err != nil {
			return nil, nil, nil, err
		}
		remaining := packetset.Region{box}
		for _, id := range ids {
			if len(remaining) == 0 {
				break
			}
			cut := rules.Box(id)
			next := packetset.Region{}
			for _, p := range remaining {
				if err := b.Tick(1); err != nil {
					return nil, nil, nil, err
				}
				inside, ok := packetset.Intersect(p, cut)
				if !ok {
					next = append(next, p)
					continue
				}
				if err := b.CheckBoxes(held + rules.Len() + len(boxes) + len(remaining) + len(next) + 1); err != nil {
					return nil, nil, nil, err
				}
				boxes, deciders, sources = append(boxes, inside), append(deciders, owner[id]), append(sources, w)
				if err := packetset.Subtract(p, cut, b, func(rest packetset.Box) error {
					if err := b.CheckBoxes(held + rules.Len() + len(boxes) + len(remaining) + len(next) + 1); err != nil {
						return err
					}
					next = append(next, rest)
					return nil
				}); err != nil {
					return nil, nil, nil, err
				}
			}
			remaining = next
		}
		for _, p := range remaining {
			boxes, deciders, sources = append(boxes, p), append(deciders, -1), append(sources, w)
		}
	}
	return boxes, deciders, sources, nil
}

func actionOf(acl ciscoacl.ACL, ordinal int) string {
	if ordinal < 0 {
		return "deny"
	}
	return acl.Rules[ordinal].Action
}

// exactDeltaV2 computes every packet whose decision differs, partitioned by
// the deciding rule on each side, building both full permit sets. It is the
// v0.2 construction, kept so v0.2 evidence replays with its own accounting.
// lost: before permits, after denies.
func exactDeltaV2(before, after ciscoacl.ACL, b *packetset.Budget) (delta, error) {
	bp, bTags, err := taggedPermitSet(before, b, 0)
	if err != nil {
		return delta{}, err
	}
	ap, aTags, err := taggedPermitSet(after, b, len(bp))
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
		rules, owner, err := rulesIndex(side.other, b)
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

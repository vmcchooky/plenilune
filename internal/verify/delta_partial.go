package verify

import (
	"errors"
	"slices"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
)

// deltaProgress is the internal progress record of the partial delta
// construction. It tracks how far the pipeline got before a budget stop
// so the caller can separate the decided part from the undecided
// remainder. Progress bookkeeping is plain variables: it consumes no
// budget, so a completing run's accounting is exactly v0.3's.
type deltaProgress struct {
	prefix    int // common prefix length k decided by alignment
	stage     string
	complete  bool // delta fully decided (a trailing tick may still have stopped the budget)
	atBefore  int  // before ordinal in flight at the stop, -1 when none
	atAfter   int  // after ordinal in flight at the stop, -1 when none
	undecided []UndecidedPart
}

// result converts the progress record into the wire partial result with
// the given decided difference pairs (already grouped and sorted).
func (p *deltaProgress) result(decided []RulePair, decidedTruncated bool) *PartialResult {
	out := &PartialResult{
		Stage:                 p.stage,
		PrefixPairs:           p.prefix,
		DecidedPairs:          decided,
		DecidedPairsTruncated: decidedTruncated,
		Undecided:             p.undecided,
	}
	if out.DecidedPairs == nil {
		out.DecidedPairs = []RulePair{}
	}
	if out.Undecided == nil {
		out.Undecided = []UndecidedPart{}
	}
	if p.atBefore >= 0 {
		v := p.atBefore
		out.BeforeRule = &v
	}
	if p.atAfter >= 0 {
		v := p.atAfter
		out.AfterRule = &v
	}
	return out
}

// groupDecidedPairs counts proven difference fragments per deciding rule
// pair, sorted by (before, after) with the implicit deny (-1) last, the
// same order change classes use. The listing is capped; the second
// return value records truncation.
func groupDecidedPairs(pieces []piece) ([]RulePair, bool) {
	type key struct{ before, after int }
	counts := map[key]int{}
	keys := []key{}
	for _, p := range pieces {
		k := key{p.before, p.after}
		if _, ok := counts[k]; !ok {
			keys = append(keys, k)
		}
		counts[k]++
	}
	order := func(n int) int {
		if n < 0 {
			return 1 << 30
		}
		return n
	}
	slices.SortFunc(keys, func(x, y key) int {
		if order(x.before) != order(y.before) {
			return order(x.before) - order(y.before)
		}
		return order(x.after) - order(y.after)
	})
	truncated := false
	if len(keys) > MaxPartialDecidedPairs {
		keys, truncated = keys[:MaxPartialDecidedPairs], true
	}
	out := make([]RulePair, 0, len(keys))
	for _, k := range keys {
		out = append(out, RulePair{Before: k.before, After: k.after, Pieces: counts[k]})
	}
	return out, truncated
}

// ordinalRange returns the inclusive ordinal range of rules[from:], or
// nil when the slice has no rules from that index.
func ordinalRange(rules []ciscoacl.Rule, from int) *[2]int {
	if from < 0 || from >= len(rules) {
		return nil
	}
	return &[2]int{rules[from].Ordinal, rules[len(rules)-1].Ordinal}
}

// taggedPermitSetOverPartial is taggedPermitSetOver with progress: on an
// error it returns the effective boxes of the rules fully processed
// before the failing one (stopIdx is that rule's index in rules) instead
// of discarding them. The budget calls on every path are exactly the
// original's, so a completing run accounts identically.
func taggedPermitSetOverPartial(rules []ciscoacl.Rule, covered *packetset.Index, b *packetset.Budget, held int) (packetset.Region, []int, int, error) {
	type tagged struct {
		box  packetset.Box
		rule int
	}
	out := []tagged{}
	snapshot := func() (packetset.Region, []int) {
		region, tags := make(packetset.Region, len(out)), make([]int, len(out))
		for i, t := range out {
			region[i], tags[i] = t.box, t.rule
		}
		return region, tags
	}
	for i, rule := range rules {
		mark := len(out)
		fail := func(err error) (packetset.Region, []int, int, error) {
			out = out[:mark]
			region, tags := snapshot()
			return region, tags, i, err
		}
		if err := b.SetReserved(held + len(out) + covered.Len()); err != nil {
			return fail(err)
		}
		raw, err := ruleRegion(rule, b)
		if err != nil {
			return fail(err)
		}
		if rule.Action == "permit" {
			effective, err := packetset.DifferenceIndexed(raw, covered, b)
			if err != nil {
				return fail(err)
			}
			if err := b.CheckBoxes(held + covered.Len() + len(raw) + len(out) + len(effective)); err != nil {
				return fail(err)
			}
			for _, box := range effective {
				out = append(out, tagged{box, rule.Ordinal})
			}
		}
		if err := covered.Add(raw, b); err != nil {
			return fail(err)
		}
	}
	if err := b.Tick(1); err != nil {
		// Every rule was processed; only the closing tick stopped. The
		// region is complete, but the budget is spent.
		region, tags := snapshot()
		return region, tags, len(rules), err
	}
	slices.SortFunc(out, func(x, y tagged) int { return packetset.CompareBoxes(x.box, y.box) })
	region, tags := snapshot()
	return region, tags, len(rules), nil
}

// decideAgainstPartial is decideAgainst with progress: on an error it
// returns the pieces of the work boxes fully decided before the failing
// one (done counts them) instead of discarding them. Every completed
// work box is classified exactly as the original classifies it: each of
// its pieces is either a proven difference fragment or decided by a
// same-action rule. Budget calls are exactly the original's.
func decideAgainstPartial(work packetset.Region, acl ciscoacl.ACL, rules *packetset.Index, owner []int, b *packetset.Budget, held int) ([]packetset.Box, []int, []int, int, error) {
	boxes, deciders, sources := []packetset.Box{}, []int{}, []int{}
	for w, box := range work {
		markB, markD, markS := len(boxes), len(deciders), len(sources)
		fail := func(err error) ([]packetset.Box, []int, []int, int, error) {
			return boxes[:markB], deciders[:markD], sources[:markS], w, err
		}
		ids, err := rules.Candidates(box, b)
		if err != nil {
			return fail(err)
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
					return fail(err)
				}
				inside, ok := packetset.Intersect(p, cut)
				if !ok {
					next = append(next, p)
					continue
				}
				if err := b.CheckBoxes(held + rules.Len() + len(boxes) + len(remaining) + len(next) + 1); err != nil {
					return fail(err)
				}
				boxes, deciders, sources = append(boxes, inside), append(deciders, owner[id]), append(sources, w)
				if err := packetset.Subtract(p, cut, b, func(rest packetset.Box) error {
					if err := b.CheckBoxes(held + rules.Len() + len(boxes) + len(remaining) + len(next) + 1); err != nil {
						return err
					}
					next = append(next, rest)
					return nil
				}); err != nil {
					return fail(err)
				}
			}
			remaining = next
		}
		for _, p := range remaining {
			boxes, deciders, sources = append(boxes, p), append(deciders, -1), append(sources, w)
		}
	}
	return boxes, deciders, sources, len(work), nil
}

// exactDeltaResidualPartial is the v0.4 construction: the same staged
// pipeline and budget accounting as exactDeltaResidual, run uniformly
// over the residual (at k=0 the stages are call-for-call the v0.2
// construction exactDeltaResidual delegates to). A non-budget error
// aborts exactly as v0.3's does. A budget stop returns the progress
// record and the pieces decided before the stop: lost/gained hold only
// proven difference fragments from fully decided work boxes, and the
// progress bounds everything not yet decided. Nothing here raises,
// skips or approximates: progress is observed, never manufactured.
func exactDeltaResidualPartial(before, after ciscoacl.ACL, b *packetset.Budget) (delta, *deltaProgress, error) {
	k := commonPrefixLen(before.Rules, after.Rules)
	prog := &deltaProgress{prefix: k, stage: PartialStagePrefix, atBefore: -1, atAfter: -1}
	residualUndecided := func() []UndecidedPart {
		return []UndecidedPart{{
			Direction:   "delta",
			BeforeRules: ordinalRange(before.Rules, k),
			AfterRules:  ordinalRange(after.Rules, k),
		}}
	}
	stop := func(stage string, err error) (delta, *deltaProgress, error) {
		prog.stage = stage
		if prog.undecided == nil {
			prog.undecided = residualUndecided()
		}
		return delta{}, prog, err
	}
	coveredSeq := make([]packetset.Region, 0, k)
	for i := 0; i < k; i++ {
		raw, err := ruleRegion(before.Rules[i], b)
		if err != nil {
			if !errors.Is(err, packetset.ErrLimit) {
				return delta{}, nil, err
			}
			return stop(PartialStagePrefix, err)
		}
		coveredSeq = append(coveredSeq, raw)
	}
	residualSet := func(rules []ciscoacl.Rule, held int, beforeSide bool) (packetset.Region, []int, error) {
		covered := packetset.NewIndex()
		for _, raw := range coveredSeq {
			if err := covered.Add(raw, b); err != nil {
				if len(rules) > 0 {
					if beforeSide {
						prog.atBefore = rules[0].Ordinal
					} else {
						prog.atAfter = rules[0].Ordinal
					}
				}
				return nil, nil, err
			}
		}
		region, tags, stopIdx, err := taggedPermitSetOverPartial(rules, covered, b, held)
		if err != nil && len(rules) > 0 {
			idx := stopIdx
			if idx >= len(rules) {
				idx = len(rules) - 1
			}
			if beforeSide {
				prog.atBefore = rules[idx].Ordinal
			} else {
				prog.atAfter = rules[idx].Ordinal
			}
		}
		return region, tags, err
	}
	bp, bTags, err := residualSet(before.Rules[k:], 0, true)
	if err != nil {
		if !errors.Is(err, packetset.ErrLimit) {
			return delta{}, nil, err
		}
		return stop(PartialStagePermitBefore, err)
	}
	ap, aTags, err := residualSet(after.Rules[k:], len(bp), false)
	if err != nil {
		if !errors.Is(err, packetset.ErrLimit) {
			return delta{}, nil, err
		}
		return stop(PartialStagePermitAfter, err)
	}
	d := delta{bp: bp, ap: ap}
	sides := []deltaSide{
		{work: bp, tags: bTags, other: after, lost: true, stage: PartialStageDecideLost},
		{work: ap, tags: aTags, other: before, lost: false, stage: PartialStageDecideGained},
	}
	for _, side := range sides {
		if err := b.SetReserved(len(bp) + len(ap) + len(d.lost) + len(d.gained)); err != nil {
			if !errors.Is(err, packetset.ErrLimit) {
				return delta{}, nil, err
			}
			return d, progWithStage(prog, side.stage, undecidedForSide(side, sides, k, before, after, 0)), err
		}
		rules, owner, err := rulesIndexOf(side.other.Rules[k:], b)
		if err != nil {
			if !errors.Is(err, packetset.ErrLimit) {
				return delta{}, nil, err
			}
			return d, progWithStage(prog, side.stage, undecidedForSide(side, sides, k, before, after, 0)), err
		}
		boxes, deciders, sources, done, err := decideAgainstPartial(side.work, side.other, rules, owner, b, len(bp)+len(ap)+len(d.lost)+len(d.gained))
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
		if err != nil {
			if !errors.Is(err, packetset.ErrLimit) {
				return delta{}, nil, err
			}
			sortPieces(d.lost)
			sortPieces(d.gained)
			return d, progWithStage(prog, side.stage, undecidedForSide(side, sides, k, before, after, done)), err
		}
	}
	for range [][]piece{d.lost, d.gained} {
		if err := b.Tick(1); err != nil {
			if !errors.Is(err, packetset.ErrLimit) {
				return delta{}, nil, err
			}
			// Both directions are fully decided; only the closing ticks
			// stopped the budget. The delta is complete — the stop lands
			// on whatever the caller does next.
			sortPieces(d.lost)
			sortPieces(d.gained)
			prog.complete = true
			return d, prog, err
		}
	}
	sortPieces(d.lost)
	sortPieces(d.gained)
	prog.complete = true
	return d, prog, nil
}

func sortPieces(ps []piece) {
	slices.SortFunc(ps, func(x, y piece) int { return packetset.CompareBoxes(x.box, y.box) })
}

// progWithStage stamps the stop stage and undecided bounds on prog.
func progWithStage(prog *deltaProgress, stage string, undecided []UndecidedPart) *deltaProgress {
	prog.stage = stage
	prog.undecided = undecided
	return prog
}

// deltaSide is one decide direction of the delta construction: a side's
// permit work boxes decided against the other ACL's residual rules.
type deltaSide struct {
	work  packetset.Region
	tags  []int
	other ciscoacl.ACL
	lost  bool
	stage string
}

// undecidedForSide bounds what a stop in one decide direction leaves
// undecided: the unprocessed work boxes of that direction plus, when the
// stopped direction is lost, the whole gained direction. sides is the
// fixed two-entry side table of exactDeltaResidualPartial; done counts
// the stopped direction's fully decided work boxes.
func undecidedForSide(side deltaSide, sides []deltaSide, k int, before, after ciscoacl.ACL, done int) []UndecidedPart {
	out := []UndecidedPart{}
	rest := side.work[done:]
	if len(rest) > 0 {
		part := UndecidedPart{WorkBoxes: len(rest)}
		if side.lost {
			part.Direction = "lost"
			lo, hi := side.tags[done], side.tags[done]
			for _, t := range side.tags[done:] {
				if t < lo {
					lo = t
				}
				if t > hi {
					hi = t
				}
			}
			part.BeforeRules = &[2]int{lo, hi}
			part.AfterRules = ordinalRange(after.Rules, k)
		} else {
			part.Direction = "gained"
			lo, hi := side.tags[done], side.tags[done]
			for _, t := range side.tags[done:] {
				if t < lo {
					lo = t
				}
				if t > hi {
					hi = t
				}
			}
			part.AfterRules = &[2]int{lo, hi}
			part.BeforeRules = ordinalRange(before.Rules, k)
		}
		out = append(out, part)
	}
	if side.lost {
		// The gained direction has not started: it is wholly undecided.
		out = append(out, UndecidedPart{
			Direction:   "gained",
			BeforeRules: ordinalRange(before.Rules, k),
			AfterRules:  ordinalRange(after.Rules, k),
			WorkBoxes:   len(sides[1].work),
		})
	}
	return out
}

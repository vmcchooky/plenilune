package verify

import (
	"context"
	"errors"
	"slices"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

// changesPartial is the change explanation of engine v0.4. Its completing
// path is Changes unchanged — same classes, same accounting. On a budget
// stop it keeps what was already decided (P1-2): classes fully formed and
// witness-checked before the stop stay listed, and Partial separates the
// decided rule pairs from the undecided remainder. A class is emitted
// only whole — witness, deciding refs and (when a policy is declared)
// scope classification — never partially formed. When the delta itself
// stopped and a policy scope is declared, classes are withheld as
// before (their scope cannot be computed without the stopped algebra),
// but the decided pairs are still recorded in Partial.
func changesPartial(ctx context.Context, before, after ciscoacl.ACL, allowed []intent.Region, opts ChangeOptions, version string, b *packetset.Budget) (ChangeReport, error) {
	r := ChangeReport{Schema: ChangeSchema, Engine: version, ScopeDeclared: allowed != nil, Classes: []ChangeClass{}, Options: opts}
	stop := func(cause error) (ChangeReport, error) {
		r.Budget = b.Stats()
		if !errors.Is(cause, packetset.ErrLimit) {
			return ChangeReport{}, cause
		}
		r.Complete, r.Reason = false, r.Budget.StopReason
		return r, nil
	}
	d, prog, err := exactDeltaResidualPartial(before, after, b)
	if err != nil {
		if !errors.Is(err, packetset.ErrLimit) {
			return ChangeReport{}, err
		}
		if !prog.complete {
			// The delta stopped: the decided content is the salvaged
			// fragments. Classes are formed from them only when no
			// policy scope needs classifying.
			decided, truncated := groupDecidedPairs(append(append([]piece{}, d.lost...), d.gained...))
			r.Partial = prog.result(decided, truncated)
			if allowed == nil {
				classes, cerr := salvageClasses(before, after, d, opts.MaxClasses)
				if cerr != nil {
					return ChangeReport{}, cerr
				}
				r.Classes = classes
			}
			return stop(err)
		}
		// The delta is fully decided; the budget died on its closing
		// ticks. Continue: the scope index or the class loop stops on
		// its own and is reported from there.
	}
	type key struct{ before, after int }
	groups := map[key][]packetset.Box{}
	keys := []key{}
	for _, p := range append(append([]piece(nil), d.lost...), d.gained...) {
		k := key{p.before, p.after}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], p.box)
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
	r.TotalClasses = len(keys)
	var allowedIdx *packetset.Index
	if allowed != nil {
		region := packetset.Region{}
		for _, a := range allowed {
			n, err := intent.Normalize(intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_remain", Match: a}}})
			if err != nil {
				return ChangeReport{}, err
			}
			x, err := conditionRegion(n.Obligations[0].Match, b)
			if err != nil {
				if !errors.Is(err, packetset.ErrLimit) {
					return ChangeReport{}, err
				}
				r.Partial = scopeIndexPartial(prog, before, after, d)
				return stop(err)
			}
			region = append(region, x...)
		}
		if allowedIdx, err = packetset.IndexOf(region, b); err != nil {
			if !errors.Is(err, packetset.ErrLimit) {
				return ChangeReport{}, err
			}
			r.Partial = scopeIndexPartial(prog, before, after, d)
			return stop(err)
		}
	}
	// classStop reports a budget stop inside the class loop at key index
	// idx: classes already emitted are decided; the remaining keys are
	// the undecided pairs, listed exactly.
	classStop := func(idx int, cause error) (ChangeReport, error) {
		decided := make([]RulePair, 0, len(r.Classes))
		for _, c := range r.Classes {
			decided = append(decided, RulePair{Before: derefOrdn(c.Before), After: derefOrdn(c.After), Pieces: c.Pieces})
		}
		undecided := []RulePair{}
		truncated := false
		for _, k := range keys[idx:] {
			if len(undecided) >= MaxPartialUndecidedPairs {
				truncated = true
				break
			}
			undecided = append(undecided, RulePair{Before: k.before, After: k.after, Pieces: len(groups[k])})
		}
		cp := &deltaProgress{prefix: prog.prefix, stage: PartialStageClasses, atBefore: -1, atAfter: -1, undecided: []UndecidedPart{}}
		r.Partial = cp.result(decided, false)
		r.Partial.UndecidedPairs = undecided
		r.Partial.UndecidedPairsTruncated = truncated
		return stop(cause)
	}
	for idx, k := range keys {
		if len(r.Classes) >= opts.MaxClasses {
			r.ClassesTruncated = true
			break
		}
		boxes := groups[k]
		packetset.Sort(boxes)
		if err := b.Tick(len(before.Rules) + len(after.Rules) + 1); err != nil {
			if !errors.Is(err, packetset.ErrLimit) {
				return ChangeReport{}, err
			}
			return classStop(idx, err)
		}
		w, err := witness(boxes[0], before, after, "change")
		if err != nil {
			return ChangeReport{}, err
		}
		if w.Before.Action == w.After.Action || !decidedBy(w.Before, k.before) || !decidedBy(w.After, k.after) {
			return ChangeReport{}, errors.New("change class failed independent evaluation")
		}
		c := ChangeClass{Before: ref(before, k.before), After: ref(after, k.after), Witness: w, Pieces: len(boxes), Scope: "undeclared"}
		c.Effect = "newly_denied"
		if c.After.Action == "permit" {
			c.Effect = "newly_permitted"
		}
		for i, box := range boxes {
			if i == maxExamples {
				c.ExamplesTruncated = true
				break
			}
			c.Examples = append(c.Examples, boxText(box))
		}
		if allowedIdx != nil {
			inside, err := packetset.IntersectionIndexed(boxes, allowedIdx, b)
			if err != nil {
				if !errors.Is(err, packetset.ErrLimit) {
					return ChangeReport{}, err
				}
				return classStop(idx, err)
			}
			outside, err := packetset.DifferenceIndexed(boxes, allowedIdx, b)
			if err != nil {
				if !errors.Is(err, packetset.ErrLimit) {
					return ChangeReport{}, err
				}
				return classStop(idx, err)
			}
			switch {
			case len(outside) == 0:
				c.Scope = "inside_allowed"
			case len(inside) == 0:
				c.Scope = "outside_allowed"
			default:
				c.Scope = "mixed"
			}
			if len(outside) > 0 {
				packetset.Sort(outside)
				ow, err := witness(outside[0], before, after, "change-outside")
				if err != nil {
					return ChangeReport{}, err
				}
				for _, a := range allowed {
					if regionContains(a, ow.Packet) {
						return ChangeReport{}, errors.New("outside witness lies in an allowed region")
					}
				}
				if ow.Before.Action == ow.After.Action {
					return ChangeReport{}, errors.New("outside witness failed independent evaluation")
				}
				c.OutsideWitness = &ow
			}
		}
		r.Classes = append(r.Classes, c)
	}
	if err := ctx.Err(); err != nil {
		return ChangeReport{}, err
	}
	r.Complete = true
	r.Budget = b.Stats()
	return r, nil
}

// scopeIndexPartial records a budget stop while building the allowed-
// change scope index: the delta is fully decided (all its pairs are
// decided differences) but no class could be scope-classified, so
// classes are withheld and the explanation stage is wholly undecided.
func scopeIndexPartial(prog *deltaProgress, before, after ciscoacl.ACL, d delta) *PartialResult {
	decided, truncated := groupDecidedPairs(append(append([]piece{}, d.lost...), d.gained...))
	sp := &deltaProgress{
		prefix: prog.prefix, stage: PartialStageScopeIndex, atBefore: -1, atAfter: -1,
		undecided: []UndecidedPart{{
			Direction:   "classes",
			BeforeRules: ordinalRange(before.Rules, prog.prefix),
			AfterRules:  ordinalRange(after.Rules, prog.prefix),
		}},
	}
	return sp.result(decided, truncated)
}

// salvageClasses forms whole change classes from the pieces a stopped
// delta had decided, without budget accounting (the budget is spent)
// and without scope classification — the caller uses it only when no
// policy scope is declared, where the completing path's scope is
// "undeclared" too. Witness generation and its independent checks are
// the completing path's.
func salvageClasses(before, after ciscoacl.ACL, d delta, maxClasses int) ([]ChangeClass, error) {
	type key struct{ before, after int }
	groups := map[key][]packetset.Box{}
	keys := []key{}
	for _, p := range append(append([]piece(nil), d.lost...), d.gained...) {
		k := key{p.before, p.after}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], p.box)
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
	out := []ChangeClass{}
	for _, k := range keys {
		if len(out) >= maxClasses {
			break
		}
		boxes := groups[k]
		packetset.Sort(boxes)
		w, err := witness(boxes[0], before, after, "change")
		if err != nil {
			return nil, err
		}
		if w.Before.Action == w.After.Action || !decidedBy(w.Before, k.before) || !decidedBy(w.After, k.after) {
			return nil, errors.New("change class failed independent evaluation")
		}
		c := ChangeClass{Before: ref(before, k.before), After: ref(after, k.after), Witness: w, Pieces: len(boxes), Scope: "undeclared"}
		c.Effect = "newly_denied"
		if c.After.Action == "permit" {
			c.Effect = "newly_permitted"
		}
		for i, box := range boxes {
			if i == maxExamples {
				c.ExamplesTruncated = true
				break
			}
			c.Examples = append(c.Examples, boxText(box))
		}
		out = append(out, c)
	}
	return out, nil
}

// derefOrdn returns the ordinal a rule ref names, -1 for the implicit deny.
func derefOrdn(r RuleRef) int {
	if r.Implicit || r.Ordinal == nil {
		return -1
	}
	return *r.Ordinal
}

package verify

import (
	"context"
	"errors"
	"net/netip"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

// comparePartial is engine v0.4. Its completing path is compareResidual
// unchanged — same stages, same budget accounting, same verdicts and
// obligation outcomes. The difference is only in what a budget stop
// returns: instead of discarding everything, the report keeps what the
// stopped search had already decided (P1-2). Difference witnesses proven
// before the stop stay in the report and can carry a DIFFERENT verdict
// (a proven difference needs no complete search, exactly as the legacy
// engine already allowed); the enumeration's incompleteness is recorded
// in Partial, SearchComplete stays false, and no obligation is decided
// by a stopped delta — obligations keep their independent statuses and
// are all INCONCLUSIVE with the stop reason when the delta stopped.
func comparePartial(ctx context.Context, before, after ciscoacl.ACL, conditions *intent.Set, opts Options, budget *packetset.Budget) (Report, error) {
	probe := ciscoacl.Packet{SrcIP: netip.IPv4Unspecified(), DstIP: netip.IPv4Unspecified()}
	if _, err := before.Evaluate(probe); err != nil {
		return Report{}, err
	}
	if _, err := after.Evaluate(probe); err != nil {
		return Report{}, err
	}
	if before.Name != after.Name || before.Kind != after.Kind {
		return Report{}, errors.New("ACL identity mismatch")
	}
	r := Report{Verdict: "INCONCLUSIVE", Witnesses: []Witness{}, Obligations: []ObligationResult{}, Options: opts}
	var obs []intent.Obligation
	if conditions != nil {
		n, err := intent.Normalize(*conditions)
		if err != nil {
			return Report{}, err
		}
		obs = n.Obligations
	}
	for _, ob := range obs {
		r.Obligations = append(r.Obligations, ObligationResult{ID: ob.ID, Status: "INCONCLUSIVE", Reason: "not_evaluated"})
	}
	finish := func(cause error) (Report, error) {
		r.Budget = budget.Stats()
		if cause != nil {
			if !errors.Is(cause, packetset.ErrLimit) {
				return Report{}, cause
			}
			r.Reason = r.Budget.StopReason
			for i := range r.Obligations {
				if r.Obligations[i].Reason == "not_evaluated" {
					r.Obligations[i].Reason = r.Reason
				}
			}
		}
		return r, nil
	}
	// partialFrom builds the report for a budget stop from the delta
	// pieces decided before it: witnesses are materialized from the
	// proven fragments (box-sorted, evaluator-checked, capped) without
	// touching the stopped budget, and Partial records the split.
	partialFrom := func(prog *deltaProgress, d delta, stage string) (Report, error) {
		pieces := append(append([]piece{}, d.lost...), d.gained...)
		decided, truncated := groupDecidedPairs(pieces)
		if stage != "" {
			prog.stage = stage
		}
		r.Partial = prog.result(decided, truncated)
		for _, region := range []packetset.Region{d.region(d.lost), d.region(d.gained)} {
			if err := appendSalvagedWitnesses(&r, region, before, after, opts.MaxWitnesses); err != nil {
				return Report{}, err
			}
		}
		if len(r.Witnesses) > 0 {
			r.Verdict = "DIFFERENT"
		}
		return finish(packetset.ErrLimit)
	}
	d, prog, err := exactDeltaResidualPartial(before, after, budget)
	if err != nil {
		if !errors.Is(err, packetset.ErrLimit) || prog == nil {
			return Report{}, err
		}
		if !prog.complete {
			// The search stopped inside the delta: only the salvaged
			// fragments are decided.
			return partialFrom(prog, d, "")
		}
		// The delta is fully decided; the budget died on its closing
		// ticks, before any witness accounting. The decided pairs are
		// the proof content and the stop is the witnesses stage.
		return partialFrom(prog, d, PartialStageWitnesses)
	}
	lost, gained := d.region(d.lost), d.region(d.gained)
	for _, region := range []packetset.Region{lost, gained} {
		if werr := appendWitnesses(&r, region, before, after, "diff", opts.MaxWitnesses, budget); werr != nil {
			if !errors.Is(werr, packetset.ErrLimit) {
				return Report{}, werr
			}
			// The delta is decided but its witness list stopped
			// mid-way: keep the witnesses materialized so far and
			// record the decided pairs in Partial.
			pieces := append(append([]piece{}, d.lost...), d.gained...)
			decided, truncated := groupDecidedPairs(pieces)
			prog.stage = PartialStageWitnesses
			prog.undecided = []UndecidedPart{}
			r.Partial = prog.result(decided, truncated)
			if len(r.Witnesses) > 0 {
				r.Verdict = "DIFFERENT"
			}
			return finish(werr)
		}
	}
	r.SearchComplete = true
	if len(lost)+len(gained) > 0 {
		r.Verdict = "DIFFERENT"
	} else {
		r.Verdict = "EQUIVALENT"
	}
	persistent := len(d.bp) + len(d.ap) + len(lost) + len(gained)
	for i, ob := range obs {
		if err := budget.SetReserved(persistent); err != nil {
			return finish(err)
		}
		region, err := conditionRegion(ob.Match, budget)
		if err != nil {
			return finish(err)
		}
		if err := budget.SetReserved(persistent + len(region)); err != nil {
			return finish(err)
		}
		var violation packetset.Region
		idx := engine{indexed: true}
		switch ob.Kind {
		case "must_permit":
			var cover packetset.Region
			cover, err = permitCover(after, region, budget)
			if err == nil {
				violation, err = idx.difference(region, cover, budget)
			}
		case "must_deny":
			var cover packetset.Region
			cover, err = permitCover(after, region, budget)
			if err == nil {
				violation, err = idx.intersection(region, cover, budget)
			}
		case "must_remain":
			violation, err = idx.intersection(region, lost, budget)
			if err == nil && len(violation) == 0 {
				violation, err = idx.intersection(region, gained, budget)
			}
		}
		if err != nil {
			return finish(err)
		}
		if len(violation) > 0 {
			packetset.Sort(violation)
			if err := budget.Tick(len(before.Rules) + len(after.Rules) + 1); err != nil {
				return finish(err)
			}
			w, err := witness(violation[0], before, after, ob.ID)
			if err != nil {
				return Report{}, err
			}
			valid := ob.Kind == "must_permit" && w.After.Action == "deny" || ob.Kind == "must_deny" && w.After.Action == "permit" || ob.Kind == "must_remain" && w.Before.Action != w.After.Action
			if !valid {
				return Report{}, errors.New("obligation witness failed independent evaluation")
			}
			r.Obligations[i] = ObligationResult{ID: ob.ID, Status: "VIOLATED", Witness: &w}
		} else {
			r.Obligations[i] = ObligationResult{ID: ob.ID, Status: "HOLDS"}
		}
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	return finish(nil)
}

// appendSalvagedWitnesses appends evaluator-checked difference witnesses
// from a region of proven difference fragments after the budget has
// stopped. It performs no budget accounting — the budget is already
// spent and its stats are final — but witness generation itself is the
// same independent evaluation the completing path performs.
func appendSalvagedWitnesses(r *Report, region packetset.Region, b, a ciscoacl.ACL, maxWitnesses int) error {
	packetset.Sort(region)
	for _, box := range region {
		if len(r.Witnesses) >= maxWitnesses {
			r.WitnessesTruncated = true
			break
		}
		w, err := witness(box, b, a, "diff")
		if err != nil {
			return err
		}
		if w.Before.Action == w.After.Action {
			return errors.New("difference witness failed independent evaluation")
		}
		r.Witnesses = append(r.Witnesses, w)
	}
	return nil
}

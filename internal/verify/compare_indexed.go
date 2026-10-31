package verify

import (
	"context"
	"errors"
	"net/netip"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

// compareIndexed is engine v0.2. The difference is decided against raw rules
// in first-match order instead of subtracting two fragmented permit sets.
func compareIndexed(ctx context.Context, before, after ciscoacl.ACL, conditions *intent.Set, opts Options, budget *packetset.Budget) (Report, error) {
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
	d, err := exactDelta(before, after, budget)
	if err != nil {
		return finish(err)
	}
	lost, gained := d.region(d.lost), d.region(d.gained)
	for _, region := range []packetset.Region{lost, gained} {
		if err := appendWitnesses(&r, region, before, after, "diff", opts.MaxWitnesses, budget); err != nil {
			return finish(err)
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
			violation, err = idx.difference(region, d.ap, budget)
		case "must_deny":
			violation, err = idx.intersection(region, d.ap, budget)
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
				return finish(err)
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

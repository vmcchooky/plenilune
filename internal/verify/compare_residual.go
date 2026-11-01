package verify

import (
	"context"
	"errors"
	"net/netip"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

// compareResidual is engine v0.3. The delta comes from the LCP-residual
// construction; verdicts and obligation outcomes are v0.2's. Obligations
// that v0.2 evaluates against the materialized after permit set are decided
// here against a permit cover (permitCover): the effective boxes of just
// the rules that can touch the condition region, fed to the same indexed
// difference/intersection v0.2 uses — the same violation sets, without
// materializing the global permit set. Obligation witnesses are still produced from a
// sorted violation region and re-evaluated independently; as with the
// v0.1→v0.2 step, their box representation may differ from the previous
// engine's while the packet decision they prove is identical.
func compareResidual(ctx context.Context, before, after ciscoacl.ACL, conditions *intent.Set, opts Options, budget *packetset.Budget) (Report, error) {
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
	d, err := exactDeltaResidual(before, after, budget)
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

// permitCover returns the effective permit boxes of the rules of acl whose
// raw region intersects region, with the same boxes taggedPermitSet would
// produce for them. Rules that cannot touch region are skipped: their
// effective regions are disjoint from region, so as cuts or as intersection
// partners against region they are exact no-ops, and the cover serves the
// must_permit/must_deny set operations exactly as the full permit set
// would — while the full set is never materialized. The cover boxes are
// pairwise disjoint, like the permit set's.
func permitCover(acl ciscoacl.ACL, region packetset.Region, b *packetset.Budget) (packetset.Region, error) {
	covered := packetset.NewIndex()
	out := packetset.Region{}
	for _, rule := range acl.Rules {
		raw, err := ruleRegion(rule, b)
		if err != nil {
			return nil, err
		}
		if rule.Action == "permit" {
			touches := false
			for _, rb := range raw {
				for _, wb := range region {
					if err := b.Tick(1); err != nil {
						return nil, err
					}
					if _, ok := packetset.Intersect(rb, wb); ok {
						touches = true
						break
					}
				}
				if touches {
					break
				}
			}
			if touches {
				effective, err := packetset.DifferenceIndexed(raw, covered, b)
				if err != nil {
					return nil, err
				}
				if err := b.CheckBoxes(covered.Len() + len(out) + len(effective)); err != nil {
					return nil, err
				}
				out = append(out, effective...)
			}
		}
		if err := covered.Add(raw, b); err != nil {
			return nil, err
		}
	}
	if err := b.Tick(1); err != nil {
		return nil, err
	}
	return out, nil
}

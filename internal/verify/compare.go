package verify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

func Compare(ctx context.Context, before, after ciscoacl.ACL, conditions *intent.Set, opts Options) (Report, error) {
	return CompareEngine(ctx, before, after, conditions, opts, EngineVersion)
}

// CompareEngine runs the named engine version. Replay passes the version
// recorded in evidence so older receipts keep their exact accounting.
func CompareEngine(ctx context.Context, before, after ciscoacl.ACL, conditions *intent.Set, opts Options, version string) (Report, error) {
	eng, err := engineFor(version)
	if err != nil {
		return Report{}, err
	}
	if opts.MaxWitnesses < 1 || opts.MaxWitnesses > 64 {
		return Report{}, errors.New("max witnesses must be 1 to 64")
	}
	budget, err := packetset.NewBudget(ctx, opts.Limits)
	if err != nil {
		return Report{}, err
	}
	defer budget.Close()
	return compareWithBudget(ctx, eng, before, after, conditions, opts, budget)
}

func compareWithBudget(ctx context.Context, eng engine, before, after ciscoacl.ACL, conditions *intent.Set, opts Options, budget *packetset.Budget) (Report, error) {
	if eng.partial {
		return comparePartial(ctx, before, after, conditions, opts, budget)
	}
	if eng.lcp {
		return compareResidual(ctx, before, after, conditions, opts, budget)
	}
	if eng.indexed {
		return compareIndexed(ctx, before, after, conditions, opts, budget)
	}
	return compareLegacy(ctx, eng, before, after, conditions, opts, budget)
}

func compareLegacy(ctx context.Context, eng engine, before, after ciscoacl.ACL, conditions *intent.Set, opts Options, budget *packetset.Budget) (Report, error) {
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
	bp, err := eng.permitSet(before, budget, 0)
	if err != nil {
		return finish(err)
	}
	ap, err := eng.permitSet(after, budget, len(bp))
	if err != nil {
		return finish(err)
	}
	if err := budget.SetReserved(0); err != nil {
		return finish(err)
	}
	lost, err := eng.difference(bp, ap, budget)
	if err != nil {
		return finish(err)
	}
	if err := appendWitnesses(&r, lost, before, after, "diff", opts.MaxWitnesses, budget); err != nil {
		return finish(err)
	}
	if len(r.Witnesses) > 0 {
		r.Verdict = "DIFFERENT"
	}
	if err := budget.SetReserved(len(lost)); err != nil {
		return finish(err)
	}
	gained, err := eng.difference(ap, bp, budget)
	if err != nil {
		return finish(err)
	}
	if err := appendWitnesses(&r, gained, before, after, "diff", opts.MaxWitnesses, budget); err != nil {
		return finish(err)
	}
	r.SearchComplete = true
	if len(r.Witnesses) > 0 {
		r.Verdict = "DIFFERENT"
	} else {
		r.Verdict = "EQUIVALENT"
	}
	persistent := len(bp) + len(ap) + len(lost) + len(gained)
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
		switch ob.Kind {
		case "must_permit":
			violation, err = eng.difference(region, ap, budget)
		case "must_deny":
			violation, err = eng.intersection(region, ap, budget)
		case "must_remain":
			violation, err = eng.intersection(region, lost, budget)
			if err == nil && len(violation) == 0 {
				violation, err = eng.intersection(region, gained, budget)
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

func (e engine) permitSet(acl ciscoacl.ACL, b *packetset.Budget, held int) (packetset.Region, error) {
	if e.indexed {
		region, _, err := taggedPermitSet(acl, b, held)
		return region, err
	}
	covered, permitted := packetset.Region{}, packetset.Region{}
	for _, rule := range acl.Rules {
		if err := b.SetReserved(held + len(permitted) + len(covered)); err != nil {
			return nil, err
		}
		raw, err := ruleRegion(rule, b)
		if err != nil {
			return nil, err
		}
		if err := b.SetReserved(held + len(permitted)); err != nil {
			return nil, err
		}
		if rule.Action == "permit" {
			effective, err := packetset.Difference(raw, covered, b)
			if err != nil {
				return nil, err
			}
			if err := b.CheckBoxes(len(covered) + len(raw) + len(effective)); err != nil {
				return nil, err
			}
			permitted = append(permitted, effective...)
		}
		if err := b.SetReserved(held); err != nil {
			return nil, err
		}
		if err := b.CheckBoxes(len(covered) + len(raw) + len(permitted)); err != nil {
			return nil, err
		}
		covered = append(covered, raw...)
	}
	if err := b.Tick(1); err != nil {
		return nil, err
	}
	packetset.Sort(permitted)
	if err := b.Tick(1); err != nil {
		return nil, err
	}
	return permitted, nil
}

func appendWitnesses(r *Report, region packetset.Region, b, a ciscoacl.ACL, label string, maxWitnesses int, budget *packetset.Budget) error {
	if err := budget.Tick(1); err != nil {
		return err
	}
	packetset.Sort(region)
	if err := budget.Tick(1); err != nil {
		return err
	}
	for _, box := range region {
		if len(r.Witnesses) >= maxWitnesses {
			r.WitnessesTruncated = true
			break
		}
		if err := budget.Tick(len(b.Rules) + len(a.Rules) + 1); err != nil {
			return err
		}
		w, err := witness(box, b, a, label)
		if err != nil {
			return err
		}
		if w.Before.Action == w.After.Action {
			return errors.New("difference witness failed independent evaluation")
		}
		r.Witnesses = append(r.Witnesses, w)
		r.Verdict = "DIFFERENT"
	}
	return nil
}

func witness(box packetset.Box, b, a ciscoacl.ACL, label string) (Witness, error) {
	proto, src, dst, sport, dport, ok := box.Witness()
	if !ok {
		return Witness{}, errors.New("no valid packet in proof region")
	}
	p := ciscoacl.Packet{Protocol: proto, SrcIP: address(src), DstIP: address(dst), SrcPort: sport, DstPort: dport}
	if !packetset.Contains(box, proto, src, dst, sport, dport) {
		return Witness{}, errors.New("witness outside residual")
	}
	x, err := b.Evaluate(p)
	if err != nil {
		return Witness{}, err
	}
	y, err := a.Evaluate(p)
	if err != nil {
		return Witness{}, err
	}
	w := Witness{Packet: p, Before: x, After: y}
	encoded, err := json.Marshal(struct {
		Label   string
		Witness Witness
	}{label, w})
	if err != nil {
		return Witness{}, err
	}
	sum := sha256.Sum256(encoded)
	w.ID = "wit-" + hex.EncodeToString(sum[:])
	return w, nil
}

func address(n uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
}

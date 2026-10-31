package verify

import (
	"context"
	"errors"
	"net/netip"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"strconv"
)

type PreservationReport struct {
	Status  string          `json:"status"`
	Reason  string          `json:"reason,omitempty"`
	Witness *Witness        `json:"witness,omitempty"`
	Budget  packetset.Stats `json:"budget"`
}

// PreserveOutside checks the entire symmetric difference, not just previously
// reported representative packets. allowed is a union; empty means no changes.
func PreserveOutside(ctx context.Context, before, candidate ciscoacl.ACL, allowed []intent.Region, opts Options) (PreservationReport, error) {
	return PreserveOutsideEngine(ctx, before, candidate, allowed, opts, EngineVersion)
}

// PreserveOutsideEngine runs the named engine version for receipt replay.
func PreserveOutsideEngine(ctx context.Context, before, candidate ciscoacl.ACL, allowed []intent.Region, opts Options, version string) (PreservationReport, error) {
	eng, err := engineFor(version)
	if err != nil {
		return PreservationReport{}, err
	}
	b, err := packetset.NewBudget(ctx, opts.Limits)
	if err != nil {
		return PreservationReport{}, err
	}
	defer b.Close()
	probe := ciscoacl.Packet{SrcIP: netip.IPv4Unspecified(), DstIP: netip.IPv4Unspecified()}
	if _, err := before.Evaluate(probe); err != nil {
		return PreservationReport{}, err
	}
	if _, err := candidate.Evaluate(probe); err != nil {
		return PreservationReport{}, err
	}
	if before.Name != candidate.Name || before.Kind != candidate.Kind {
		return PreservationReport{}, errors.New("ACL identity mismatch")
	}
	if len(allowed) > 12 {
		return PreservationReport{}, errors.New("too many allowed change regions")
	}
	r := PreservationReport{Status: "INCONCLUSIVE"}
	finish := func(cause error) (PreservationReport, error) {
		r.Budget = b.Stats()
		if cause != nil {
			if !errors.Is(cause, packetset.ErrLimit) {
				return PreservationReport{}, cause
			}
			r.Reason = r.Budget.StopReason
		}
		return r, nil
	}
	var changes [2]packetset.Region
	held := 0
	if eng.indexed {
		d, err := exactDelta(before, candidate, b)
		if err != nil {
			return finish(err)
		}
		changes = [2]packetset.Region{d.region(d.lost), d.region(d.gained)}
		held = len(d.bp) + len(d.ap)
	} else {
		bp, err := eng.permitSet(before, b, 0)
		if err != nil {
			return finish(err)
		}
		cp, err := eng.permitSet(candidate, b, len(bp))
		if err != nil {
			return finish(err)
		}
		held = len(bp) + len(cp)
		changes = [2]packetset.Region{bp, cp}
	}
	allowedSet := packetset.Region{}
	for i, region := range allowed {
		s, err := intent.Normalize(intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-" + strconv.Itoa(i+1), Kind: "must_remain", Match: region}}})
		if err != nil {
			return PreservationReport{}, err
		}
		if err := b.SetReserved(held + len(allowedSet)); err != nil {
			return finish(err)
		}
		x, err := conditionRegion(s.Obligations[0].Match, b)
		if err != nil {
			return finish(err)
		}
		if err := b.CheckBoxes(len(x)); err != nil {
			return finish(err)
		}
		allowedSet = append(allowedSet, x...)
	}
	pairs := [][2]packetset.Region{{changes[0], changes[1]}, {changes[1], changes[0]}}
	for k, pair := range pairs {
		if err := b.SetReserved(len(allowedSet)); err != nil {
			return finish(err)
		}
		var changed packetset.Region
		var err error
		if eng.indexed {
			changed = changes[k]
		} else if changed, err = eng.difference(pair[0], pair[1], b); err != nil {
			return finish(err)
		}
		if err := b.SetReserved(held); err != nil {
			return finish(err)
		}
		outside, err := eng.difference(changed, allowedSet, b)
		if err != nil {
			return finish(err)
		}
		if len(outside) == 0 {
			continue
		}
		if err := b.Tick(len(before.Rules) + len(candidate.Rules) + 2); err != nil {
			return finish(err)
		}
		packetset.Sort(outside)
		if err := b.Tick(1); err != nil {
			return finish(err)
		}
		w, err := witness(outside[0], before, candidate, "collateral")
		if err != nil {
			return PreservationReport{}, err
		}
		if w.Before.Action == w.After.Action {
			return PreservationReport{}, errors.New("collateral witness failed independent evaluation")
		}
		r.Status = "FAILED"
		r.Reason = "behavior_changed_outside_allowed_region"
		r.Witness = &w
		return finish(nil)
	}
	if err := ctx.Err(); err != nil {
		return PreservationReport{}, err
	}
	r.Status = "VERIFIED"
	return finish(nil)
}

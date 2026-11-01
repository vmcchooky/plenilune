package verify

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

// This file exposes the exact regions repair enumeration (P1-3) is built
// from: the full box region of each change class, the full violation
// region of each violated obligation, a single-rule rendering of a box,
// and the raw region of one rule. Everything here is a read-only view over
// the same constructions Compare and Changes already use; nothing here
// weakens a verdict — enumeration candidates still go through
// repair.Verify, which re-checks obligations and collateral exactly.

// ClassRegion pairs a change class from a complete Changes report with
// the exact, sorted box region the class covers. ChangeReport itself
// carries only bounded examples; enumeration needs the full region.
type ClassRegion struct {
	Class ChangeClass
	Boxes []packetset.Box
}

// ChangeClassRegions returns the exact region of every class in a
// complete Changes report. complete is false when the change search or
// the region pass stopped on budget; the returned report is the Changes
// report either way, and its Reason names the stop.
func ChangeClassRegions(ctx context.Context, before, after ciscoacl.ACL, allowed []intent.Region, opts ChangeOptions) ([]ClassRegion, ChangeReport, bool, error) {
	rep, err := Changes(ctx, before, after, allowed, opts)
	if err != nil || !rep.Complete {
		return nil, rep, false, err
	}
	eng, err := engineFor(EngineVersion)
	if err != nil {
		return nil, rep, false, err
	}
	b, err := packetset.NewBudget(ctx, opts.Limits)
	if err != nil {
		return nil, rep, false, err
	}
	defer b.Close()
	d, err := eng.delta(before, after, b)
	if err != nil {
		if errors.Is(err, packetset.ErrLimit) {
			return nil, rep, false, nil
		}
		return nil, rep, false, err
	}
	type key struct{ before, after int }
	groups := map[key][]packetset.Box{}
	for _, p := range append(append([]piece(nil), d.lost...), d.gained...) {
		k := key{p.before, p.after}
		groups[k] = append(groups[k], p.box)
	}
	out := make([]ClassRegion, 0, len(rep.Classes))
	for _, c := range rep.Classes {
		k := key{ordOf(c.Before), ordOf(c.After)}
		boxes, ok := groups[k]
		if !ok {
			return nil, rep, false, errors.New("change class has no delta region")
		}
		sorted := append([]packetset.Box(nil), boxes...)
		packetset.Sort(sorted)
		out = append(out, ClassRegion{Class: c, Boxes: sorted})
	}
	return out, rep, true, nil
}

func ordOf(r RuleRef) int {
	if r.Ordinal == nil {
		return -1
	}
	return *r.Ordinal
}

// maxViolationBoxes bounds the violation region materialized per
// obligation. Boxes past the cap are dropped in sorted order and the
// truncation is recorded; enumeration candidates built from a truncated
// region simply face the unchanged verification gate.
const maxViolationBoxes = 512

// ObligationRegion pairs a violated obligation with its exact violation
// region: the packets inside the obligation's match region whose outcome
// the obligation forbids, computed with the same permit-cover set
// operations the residual engine judges obligations by.
type ObligationRegion struct {
	Obligation intent.Obligation
	Boxes      []packetset.Box
	Truncated  bool
}

// ObligationViolationRegions returns the violation region of every
// obligation a complete comparison reports VIOLATED. complete is false
// when the comparison or a region pass stopped on budget.
func ObligationViolationRegions(ctx context.Context, before, after ciscoacl.ACL, conditions *intent.Set, opts Options) ([]ObligationRegion, Report, bool, error) {
	rep, err := Compare(ctx, before, after, conditions, opts)
	if err != nil || !rep.SearchComplete {
		return nil, rep, false, err
	}
	if conditions == nil {
		return nil, rep, true, nil
	}
	n, err := intent.Normalize(*conditions)
	if err != nil {
		return nil, rep, false, err
	}
	b, err := packetset.NewBudget(ctx, opts.Limits)
	if err != nil {
		return nil, rep, false, err
	}
	defer b.Close()
	eng, err := engineFor(EngineVersion)
	if err != nil {
		return nil, rep, false, err
	}
	idx := engine{indexed: true}
	var d *delta
	out := []ObligationRegion{}
	for i, res := range rep.Obligations {
		if res.Status != "VIOLATED" {
			continue
		}
		ob := n.Obligations[i]
		region, err := conditionRegion(ob.Match, b)
		if err != nil {
			if errors.Is(err, packetset.ErrLimit) {
				return nil, rep, false, nil
			}
			return nil, rep, false, err
		}
		var violation packetset.Region
		switch ob.Kind {
		case "must_permit", "must_deny":
			cover, err := permitCover(after, region, b)
			if err != nil {
				if errors.Is(err, packetset.ErrLimit) {
					return nil, rep, false, nil
				}
				return nil, rep, false, err
			}
			if ob.Kind == "must_permit" {
				violation, err = idx.difference(region, cover, b)
			} else {
				violation, err = idx.intersection(region, cover, b)
			}
		case "must_remain":
			if d == nil {
				dd, err := eng.delta(before, after, b)
				if err != nil {
					if errors.Is(err, packetset.ErrLimit) {
						return nil, rep, false, nil
					}
					return nil, rep, false, err
				}
				d = &dd
			}
			lost, err := idx.intersection(region, d.region(d.lost), b)
			if err != nil {
				if errors.Is(err, packetset.ErrLimit) {
					return nil, rep, false, nil
				}
				return nil, rep, false, err
			}
			gained, err := idx.intersection(region, d.region(d.gained), b)
			if err != nil {
				if errors.Is(err, packetset.ErrLimit) {
					return nil, rep, false, nil
				}
				return nil, rep, false, err
			}
			violation = append(lost, gained...)
		}
		if err != nil {
			if errors.Is(err, packetset.ErrLimit) {
				return nil, rep, false, nil
			}
			return nil, rep, false, err
		}
		if len(violation) == 0 {
			return nil, rep, false, errors.New("violated obligation has empty violation region")
		}
		packetset.Sort(violation)
		truncated := false
		if len(violation) > maxViolationBoxes {
			violation = violation[:maxViolationBoxes]
			truncated = true
		}
		out = append(out, ObligationRegion{Obligation: ob, Boxes: violation, Truncated: truncated})
	}
	return out, rep, true, nil
}

// RuleRegion returns the raw match region of one rule: the boxes the rule
// matches before any earlier rule shadows it. It is the region repair
// enumeration narrows when it rewrites a rule's predicate.
func RuleRegion(r ciscoacl.Rule, limits packetset.Limits) (packetset.Region, error) {
	b, err := packetset.NewBudget(context.Background(), limits)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	return ruleRegion(r, b)
}

// RenderBoxRule renders one exact box as a single supported ACL rule with
// the given action ("permit" or "deny"). It returns false when the box is
// not representable as one rule: a protocol set that is neither all
// protocols nor a single protocol, ports carried by a protocol that has
// none, or a rendering that does not parse back to exactly the same match
// set. The parse-back check compares match sets with the box algebra, so
// a true result is exact, never approximate.
func RenderBoxRule(action string, box packetset.Box) (string, bool) {
	if action != "permit" && action != "deny" {
		return "", false
	}
	var proto string
	single := -1
	if box.Protocols == packetset.AllProtocols() {
		proto = "ip"
	} else {
		count := 0
		for p := 0; p < 256; p++ {
			if box.Protocols[p/64]&(uint64(1)<<(p%64)) != 0 {
				count++
				single = p
			}
		}
		if count != 1 {
			return "", false
		}
		switch single {
		case 1:
			proto = "icmp"
		case 6:
			proto = "tcp"
		case 17:
			proto = "udp"
		default:
			proto = strconv.Itoa(single)
		}
		// Protocols without ports carry the zero interval in engine
		// boxes; anything else is left to the parse-back check below,
		// which compares match sets exactly and rejects the rendering.
	}
	var sb strings.Builder
	sb.WriteString(action)
	sb.WriteString(" ")
	sb.WriteString(proto)
	sb.WriteString(" ")
	sb.WriteString(renderCube(box.Src))
	if single == 6 || single == 17 {
		if tok := renderPortOp(box.SrcPort); tok != "" {
			sb.WriteString(" ")
			sb.WriteString(tok)
		}
	}
	sb.WriteString(" ")
	sb.WriteString(renderCube(box.Dst))
	if single == 6 || single == 17 {
		if tok := renderPortOp(box.DstPort); tok != "" {
			sb.WriteString(" ")
			sb.WriteString(tok)
		}
	}
	text := sb.String()
	parsed, err := ciscoacl.Parse([]byte("ip access-list extended RENDER\n " + text + "\n"))
	if err != nil || len(parsed.Rules) != 1 || parsed.Rules[0].Action != action {
		return "", false
	}
	b, err := packetset.NewBudget(context.Background(), packetset.Defaults())
	if err != nil {
		return "", false
	}
	defer b.Close()
	got, err := ruleRegion(parsed.Rules[0], b)
	if err != nil {
		return "", false
	}
	want := normalizeRegion(packetset.Region{box})
	got = normalizeRegion(got)
	extra, err := packetset.Difference(got, want, b)
	if err != nil || len(extra) != 0 {
		return "", false
	}
	missing, err := packetset.Difference(want, got, b)
	if err != nil || len(missing) != 0 {
		return "", false
	}
	return text, true
}

// normalizeRegion rewrites a region in the engine's box convention:
// TCP and UDP each stand alone with their port intervals, and every
// other protocol is grouped with zero port intervals (ports exist in
// the model only for TCP/UDP). Two regions describe the same packet set
// exactly when their normalizations are equal as box sets.
func normalizeRegion(r packetset.Region) packetset.Region {
	out := packetset.Region{}
	for _, b := range r {
		for _, p := range []uint8{6, 17} {
			if b.Protocols[p/64]&(uint64(1)<<(p%64)) != 0 {
				out = append(out, packetset.Box{Protocols: packetset.Protocol(p), Src: b.Src, Dst: b.Dst, SrcPort: b.SrcPort, DstPort: b.DstPort})
			}
		}
		rest := b.Protocols
		rest[0] &^= (uint64(1) << 6) | (uint64(1) << 17)
		if rest != (packetset.Protocols{}) {
			out = append(out, packetset.Box{Protocols: rest, Src: b.Src, Dst: b.Dst})
		}
	}
	packetset.Sort(out)
	return out
}

func renderCube(c packetset.Cube) string {
	addr := address(c.Value).String()
	if c.Wildcard == ^uint32(0) {
		return "any"
	}
	if c.Wildcard == 0 {
		return "host " + addr
	}
	return fmt.Sprintf("%s %s", addr, address(c.Wildcard).String())
}

func renderPortOp(i packetset.Interval) string {
	switch {
	case i.Lo == 0 && i.Hi == 65535:
		return ""
	case i.Lo == i.Hi:
		return "eq " + strconv.Itoa(int(i.Lo))
	default:
		return fmt.Sprintf("range %d %d", i.Lo, i.Hi)
	}
}

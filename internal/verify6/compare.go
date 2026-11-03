// Package verify6 is the exact comparison engine for IPv6 ACLs, built on
// packetset6. The decision function of an IPv6 ACL is fully determined by
// its permit set (every packet not permitted is denied, explicitly or by
// the documented implicit tail), so two ACLs are equivalent exactly when
// their permit regions coincide. Budget exhaustion yields INCONCLUSIVE,
// never a guessed verdict.
package verify6

import (
	"context"
	"fmt"
	"net/netip"

	"plenilune/internal/ciscoacl6"
	"plenilune/internal/packetset"
	"plenilune/internal/packetset6"
)

// EngineVersion identifies this comparison construction.
const EngineVersion = "acl6-exact/v0.1"

type Options struct {
	Limits packetset.Limits
}

func DefaultOptions() Options { return Options{Limits: packetset.Defaults()} }

// Packet is a distinguishing packet in wire form.
type Packet struct {
	Protocol uint8      `json:"protocol"`
	SrcIP    netip.Addr `json:"src_ip"`
	DstIP    netip.Addr `json:"dst_ip"`
	SrcPort  uint16     `json:"src_port"`
	DstPort  uint16     `json:"dst_port"`
	IcmpType uint8      `json:"icmp_type,omitempty"`
}

type Decision struct {
	Action string `json:"action"`
}

type Witness struct {
	Packet Packet   `json:"packet"`
	Before Decision `json:"before"`
	After  Decision `json:"after"`
}

type Comparison struct {
	Version        string    `json:"version"`
	Verdict        string    `json:"verdict"` // EQUIVALENT | DIFFERENT | INCONCLUSIVE
	SearchComplete bool      `json:"search_complete"`
	Witnesses      []Witness `json:"witnesses,omitempty"`
}

func cubeOf(a ciscoacl6.Address128) packetset6.Cube128 {
	return packetset6.Cube128{Value: a.Value, Wildcard: a.Wildcard}
}

func intervalsOf(xs []ciscoacl6.Interval) []packetset.Interval {
	out := make([]packetset.Interval, len(xs))
	for i, x := range xs {
		out[i] = packetset.Interval{Lo: x.Lo, Hi: x.Hi}
	}
	return out
}

// ruleRegion compiles one rule (explicit or implicit-tail synthetic) into
// its box region.
func ruleRegion(r ciscoacl6.Rule, b *packetset.Budget) (packetset6.Region, error) {
	proto := packetset.AllProtocols()
	if r.Protocol != nil {
		proto = packetset.Protocol(*r.Protocol)
	}
	src, dst := cubeOf(r.Src), cubeOf(r.Dst)
	out := packetset6.Region{}
	emit := func(box packetset6.Box) error {
		if err := b.Tick(1); err != nil {
			return err
		}
		if err := b.CheckBoxes(len(out) + 1); err != nil {
			return err
		}
		out = append(out, box)
		return nil
	}
	for _, p := range []uint8{6, 17} {
		if proto[p/64]&(uint64(1)<<(p%64)) == 0 {
			continue
		}
		for _, s := range intervalsOf(r.SrcPorts) {
			for _, d := range intervalsOf(r.DstPorts) {
				if err := emit(packetset6.Box{Protocols: packetset.Protocol(p), Src: src, Dst: dst, SrcPort: s, DstPort: d, IcmpType: packetset6.FullIcmpType()}); err != nil {
					return nil, err
				}
			}
		}
	}
	if proto[58/64]&(uint64(1)<<(58%64)) != 0 {
		types := packetset6.FullIcmpType()
		if r.IcmpType != nil {
			types = packetset.Interval{Lo: uint16(*r.IcmpType), Hi: uint16(*r.IcmpType)}
		}
		if err := emit(packetset6.Box{Protocols: packetset.Protocol(58), Src: src, Dst: dst, IcmpType: types}); err != nil {
			return nil, err
		}
	}
	rest := proto
	rest[0] &^= (uint64(1) << 6) | (uint64(1) << 17) | (uint64(1) << 58)
	if rest != (packetset.Protocols{}) {
		if err := emit(packetset6.Box{Protocols: rest, Src: src, Dst: dst, IcmpType: packetset6.FullIcmpType()}); err != nil {
			return nil, err
		}
	}
	packetset6.Sort(out)
	return out, nil
}

// ndRegion is the neighbor-discovery subspace the documented implicit
// tail permits: ICMPv6 (58), any to any, type 136 (NA) or 135 (NS).
func ndRegion() packetset6.Region {
	any := packetset6.Cube128{Wildcard: [2]uint64{^uint64(0), ^uint64(0)}}
	mk := func(t uint16) packetset6.Box {
		return packetset6.Box{Protocols: packetset.Protocol(58), Src: any, Dst: any, IcmpType: packetset.Interval{Lo: t, Hi: t}}
	}
	return packetset6.Region{mk(ciscoacl6.IcmpNeighborAdvertisement), mk(ciscoacl6.IcmpNeighborSolicitation)}
}

// aclSets computes one ACL's explicit decision structure: the effective
// permit region E (each permit rule's region minus everything earlier
// rules decided) and the decided region D (the union of all explicit rule
// regions). The implicit tail is NOT materialized here: an IPv6 ACL's full
// permit set is E ∪ (ND ∖ D), and materializing ND ∖ D (a wide region
// minus narrow holes, intrinsically ~250 boxes) is left to the layered
// difference in Compare, where every minuend stays explicit-sized.
func aclSets(a ciscoacl6.ACL, b *packetset.Budget) (permit, decided packetset6.Region, err error) {
	for _, r := range a.Rules {
		region, err := ruleRegion(r, b)
		if err != nil {
			return nil, nil, err
		}
		eff, err := packetset6.Difference(region, decided, b)
		if err != nil {
			return nil, nil, err
		}
		if r.Action == "permit" {
			permit = append(permit, eff...)
		}
		decided = append(decided, region...)
	}
	if decided, err = packetset6.Coalesce(decided, b); err != nil {
		return nil, nil, err
	}
	if permit, err = packetset6.Coalesce(permit, b); err != nil {
		return nil, nil, err
	}
	return permit, decided, nil
}

// permitDifference computes Pa ∖ Pb for permit sets Pa = Ea ∪ (ND ∖ Da)
// and Pb = Eb ∪ (ND ∖ Db), decomposed so no intermediate region is a
// wide-minus-narrow complement:
//
//	Ea ∖ Pb            = (Ea ∖ Eb) ∖ (ND ∖ Db)     (minuend stays explicit)
//	(ND ∖ Da) ∖ Pb     = (ND ∩ Db) ∖ (Da ∪ Eb)     (ditto)
func permitDifference(ea, da, eb, db packetset6.Region, b *packetset.Budget) (packetset6.Region, error) {
	tailB, err := packetset6.Difference(ndRegion(), db, b)
	if err != nil {
		return nil, err
	}
	part1, err := packetset6.Difference(ea, eb, b)
	if err != nil {
		return nil, err
	}
	if part1, err = packetset6.Difference(part1, tailB, b); err != nil {
		return nil, err
	}
	ndInB, err := packetset6.Intersection(ndRegion(), db, b)
	if err != nil {
		return nil, err
	}
	cutters := append(append(packetset6.Region{}, da...), eb...)
	part2, err := packetset6.Difference(ndInB, cutters, b)
	if err != nil {
		return nil, err
	}
	out := append(part1, part2...)
	return packetset6.Coalesce(out, b)
}

func wordsAddr(w [2]uint64) netip.Addr {
	var b [16]byte
	for i, x := range w {
		for j := 0; j < 8; j++ {
			b[i*8+j] = byte(x >> (56 - 8*j))
		}
	}
	return netip.AddrFrom16(b)
}

// witnessPacket picks a representative packet from a box.
func witnessPacket(box packetset6.Box) Packet {
	var p Packet
	for proto := 0; proto < 256; proto++ {
		if box.Protocols[proto/64]&(uint64(1)<<(proto%64)) != 0 {
			p.Protocol = uint8(proto)
			break
		}
	}
	p.SrcIP = wordsAddr(box.Src.Value)
	p.DstIP = wordsAddr(box.Dst.Value)
	p.SrcPort = box.SrcPort.Lo
	p.DstPort = box.DstPort.Lo
	if p.Protocol == 58 {
		p.IcmpType = uint8(box.IcmpType.Lo)
	}
	return p
}

func toPacket6(p Packet) ciscoacl6.Packet {
	return ciscoacl6.Packet{Protocol: p.Protocol, SrcIP: p.SrcIP, DstIP: p.DstIP, SrcPort: p.SrcPort, DstPort: p.DstPort, IcmpType: p.IcmpType}
}

// Compare decides whether two IPv6 ACLs have the same decision function.
func Compare(ctx context.Context, before, after ciscoacl6.ACL, opts Options) (Comparison, error) {
	budget, err := packetset.NewBudget(ctx, opts.Limits)
	if err != nil {
		return Comparison{}, err
	}
	defer budget.Close()
	out := Comparison{Version: EngineVersion, Verdict: "INCONCLUSIVE"}
	permitBefore, decidedBefore, err := aclSets(before, budget)
	if err != nil {
		return out, nil // budget stop or invalid input: inconclusive, never a guess
	}
	permitAfter, decidedAfter, err := aclSets(after, budget)
	if err != nil {
		return out, nil
	}
	onlyBefore, err := permitDifference(permitBefore, decidedBefore, permitAfter, decidedAfter, budget)
	if err != nil {
		return out, nil
	}
	onlyAfter, err := permitDifference(permitAfter, decidedAfter, permitBefore, decidedBefore, budget)
	if err != nil {
		return out, nil
	}
	out.SearchComplete = true
	if len(onlyBefore) == 0 && len(onlyAfter) == 0 {
		out.Verdict = "EQUIVALENT"
		return out, nil
	}
	out.Verdict = "DIFFERENT"
	for _, side := range []struct {
		region packetset6.Region
	}{{onlyBefore}, {onlyAfter}} {
		if len(side.region) == 0 {
			continue
		}
		wp := witnessPacket(side.region[0])
		bd, err := before.Evaluate(toPacket6(wp))
		if err != nil {
			return Comparison{}, fmt.Errorf("witness validation (before): %w", err)
		}
		ad, err := after.Evaluate(toPacket6(wp))
		if err != nil {
			return Comparison{}, fmt.Errorf("witness validation (after): %w", err)
		}
		if bd.Action == ad.Action {
			return Comparison{}, fmt.Errorf("witness packet does not distinguish the ACLs (%s vs %s)", bd.Action, ad.Action)
		}
		out.Witnesses = append(out.Witnesses, Witness{Packet: wp, Before: Decision{Action: bd.Action}, After: Decision{Action: ad.Action}})
	}
	return out, nil
}

package verify

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

// ChangeSchema identifies the ACL behavior-change explanation. It is separate
// from plenilune.evidence/v1 so existing comparison receipts are unchanged.
const ChangeSchema = "plenilune.acl-changes/v1"

const (
	DefaultMaxClasses = 32
	MaxClasses        = 128
	maxExamples       = 3
)

// RuleRef names the rule that decides a change class on one side.
type RuleRef struct {
	Action   string `json:"action"`
	Ordinal  *int   `json:"ordinal,omitempty"`
	Line     *int   `json:"line,omitempty"`
	Implicit bool   `json:"implicit"`
}

// BoxText is an exact rendering of one disjoint box of a class region.
type BoxText struct {
	Protocol string `json:"protocol"`
	Src      string `json:"src"`
	Dst      string `json:"dst"`
	SrcPorts string `json:"src_ports"`
	DstPorts string `json:"dst_ports"`
}

// ChangeClass is every packet decided by one (before rule, after rule) pair
// whose actions differ. Pieces counts boxes in this representation, not
// packets; the witness is one evaluator-checked packet from the class.
type ChangeClass struct {
	Effect            string    `json:"effect"`
	Before            RuleRef   `json:"before"`
	After             RuleRef   `json:"after"`
	Witness           Witness   `json:"witness"`
	Pieces            int       `json:"pieces"`
	Examples          []BoxText `json:"examples"`
	ExamplesTruncated bool      `json:"examples_truncated"`
	Scope             string    `json:"scope"`
	OutsideWitness    *Witness  `json:"outside_witness,omitempty"`
}

type ChangeOptions struct {
	Limits     packetset.Limits `json:"limits"`
	MaxClasses int              `json:"max_classes"`
}

func DefaultChangeOptions() ChangeOptions {
	return ChangeOptions{Limits: packetset.Defaults(), MaxClasses: DefaultMaxClasses}
}

// ChangeReport explains the exact difference. Complete=false never means
// "no change". Engines before acl-exact/v0.4 withhold classes entirely on
// a budget stop; v0.4+ reports the decided part (Partial) and lists only
// classes that were fully decided and witness-checked before the stop.
type ChangeReport struct {
	Schema           string          `json:"schema"`
	Engine           string          `json:"engine"`
	Complete         bool            `json:"complete"`
	Reason           string          `json:"reason,omitempty"`
	ScopeDeclared    bool            `json:"scope_declared"`
	Classes          []ChangeClass   `json:"classes"`
	ClassesTruncated bool            `json:"classes_truncated"`
	TotalClasses     int             `json:"total_classes"`
	Budget           packetset.Stats `json:"budget"`
	Options          ChangeOptions   `json:"options"`
	// Partial is present only when a v0.4+ engine stopped on budget; it
	// separates the decided rule pairs from the undecided remainder.
	Partial *PartialResult `json:"partial,omitempty"`
}

// Changes partitions the exact behavior difference by deciding rules. allowed
// nil means no accepted scope was supplied; [] means no change is allowed.
func Changes(ctx context.Context, before, after ciscoacl.ACL, allowed []intent.Region, opts ChangeOptions) (ChangeReport, error) {
	return ChangesEngine(ctx, before, after, allowed, opts, EngineVersion)
}

// ChangesEngine runs the named engine version, mirroring CompareEngine:
// replay passes the version recorded in evidence so older change reports
// keep their exact accounting.
func ChangesEngine(ctx context.Context, before, after ciscoacl.ACL, allowed []intent.Region, opts ChangeOptions, version string) (ChangeReport, error) {
	eng, err := engineFor(version)
	if err != nil {
		return ChangeReport{}, err
	}
	if opts.MaxClasses < 1 || opts.MaxClasses > MaxClasses {
		return ChangeReport{}, errors.New("max classes must be 1 to 128")
	}
	if len(allowed) > 12 {
		return ChangeReport{}, errors.New("too many allowed change regions")
	}
	if before.Name != after.Name || before.Kind != after.Kind {
		return ChangeReport{}, errors.New("ACL identity mismatch")
	}
	probe := ciscoacl.Packet{SrcIP: netip.IPv4Unspecified(), DstIP: netip.IPv4Unspecified()}
	if _, err := before.Evaluate(probe); err != nil {
		return ChangeReport{}, err
	}
	if _, err := after.Evaluate(probe); err != nil {
		return ChangeReport{}, err
	}
	b, err := packetset.NewBudget(ctx, opts.Limits)
	if err != nil {
		return ChangeReport{}, err
	}
	defer b.Close()
	if eng.partial {
		return changesPartial(ctx, before, after, allowed, opts, version, b)
	}
	r := ChangeReport{Schema: ChangeSchema, Engine: version, ScopeDeclared: allowed != nil, Classes: []ChangeClass{}, Options: opts}
	finish := func(cause error) (ChangeReport, error) {
		r.Budget = b.Stats()
		if cause != nil {
			if !errors.Is(cause, packetset.ErrLimit) {
				return ChangeReport{}, cause
			}
			r.Complete, r.Reason, r.Classes, r.TotalClasses, r.ClassesTruncated = false, r.Budget.StopReason, []ChangeClass{}, 0, false
		}
		return r, nil
	}
	d, err := eng.delta(before, after, b)
	if err != nil {
		return finish(err)
	}
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
				return finish(err)
			}
			region = append(region, x...)
		}
		if allowedIdx, err = packetset.IndexOf(region, b); err != nil {
			return finish(err)
		}
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
	for _, k := range keys {
		if len(r.Classes) >= opts.MaxClasses {
			r.ClassesTruncated = true
			break
		}
		boxes := groups[k]
		packetset.Sort(boxes)
		if err := b.Tick(len(before.Rules) + len(after.Rules) + 1); err != nil {
			return finish(err)
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
				return finish(err)
			}
			outside, err := packetset.DifferenceIndexed(boxes, allowedIdx, b)
			if err != nil {
				return finish(err)
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
	return finish(nil)
}

func decidedBy(d ciscoacl.Decision, ordinal int) bool {
	if ordinal < 0 {
		return d.Implicit
	}
	return !d.Implicit && d.RuleOrdinal != nil && *d.RuleOrdinal == ordinal
}

func ref(acl ciscoacl.ACL, ordinal int) RuleRef {
	if ordinal < 0 {
		return RuleRef{Action: "deny", Implicit: true}
	}
	o, line := ordinal, acl.Rules[ordinal].Line
	return RuleRef{Action: acl.Rules[ordinal].Action, Ordinal: &o, Line: &line}
}

func boxText(b packetset.Box) BoxText {
	t := BoxText{Protocol: protocolText(b.Protocols), Src: cubeText(b.Src), Dst: cubeText(b.Dst), SrcPorts: portText(b.SrcPort), DstPorts: portText(b.DstPort)}
	// Ports exist in the model only for TCP/UDP; other protocols carry zero.
	if b.Protocols[0]&((uint64(1)<<6)|(uint64(1)<<17)) == 0 {
		t.SrcPorts, t.DstPorts = "n/a", "n/a"
	}
	return t
}

func protocolText(p packetset.Protocols) string {
	if p == packetset.AllProtocols() {
		return "ip"
	}
	others := packetset.AllProtocols()
	others[0] &^= (uint64(1) << 6) | (uint64(1) << 17)
	if p == others {
		return "ip-except-tcp-udp"
	}
	names := map[int]string{1: "icmp", 6: "tcp", 17: "udp"}
	out := ""
	for n := 0; n < 256; n++ {
		if p[n/64]&(uint64(1)<<(n%64)) == 0 {
			continue
		}
		if out != "" {
			out += ","
		}
		if name, ok := names[n]; ok {
			out += name
		} else {
			out += fmt.Sprint(n)
		}
	}
	return out
}

func cubeText(c packetset.Cube) string {
	addr := address(c.Value).String()
	if c.Wildcard == ^uint32(0) {
		return "any"
	}
	if c.Wildcard&(c.Wildcard+1) == 0 {
		bits := 32
		for w := c.Wildcard; w != 0; w >>= 1 {
			bits--
		}
		return fmt.Sprintf("%s/%d", addr, bits)
	}
	return addr + " wildcard " + address(c.Wildcard).String()
}

func portText(i packetset.Interval) string {
	switch {
	case i.Lo == 0 && i.Hi == 65535:
		return "any"
	case i.Lo == i.Hi:
		return fmt.Sprint(i.Lo)
	}
	return fmt.Sprintf("%d-%d", i.Lo, i.Hi)
}

// regionContains is a direct predicate check, independent of box algebra.
func regionContains(r intent.Region, p ciscoacl.Packet) bool {
	if r.Protocol != "ip" {
		n := map[string]uint8{"tcp": 6, "udp": 17, "icmp": 1}[r.Protocol]
		if _, named := map[string]bool{"tcp": true, "udp": true, "icmp": true}[r.Protocol]; !named {
			var v int
			if _, err := fmt.Sscanf(r.Protocol, "%d", &v); err != nil || v < 0 || v > 255 {
				return false
			}
			n = uint8(v)
		}
		if n != p.Protocol {
			return false
		}
	}
	in := func(cidr string, excludes []string, a netip.Addr) bool {
		pfx, err := netip.ParsePrefix(cidr)
		if err != nil || !pfx.Masked().Contains(a) {
			return false
		}
		for _, x := range excludes {
			e, err := netip.ParsePrefix(x)
			if err != nil || e.Masked().Contains(a) {
				return false
			}
		}
		return true
	}
	ports := func(xs []intent.Interval, v uint16) bool {
		if len(xs) == 0 {
			return true
		}
		for _, x := range xs {
			if v >= x.Lo && v <= x.Hi {
				return true
			}
		}
		return false
	}
	return in(r.SrcCIDR, r.ExcludeSrc, p.SrcIP) && in(r.DstCIDR, r.ExcludeDst, p.DstIP) && ports(r.SrcPorts, p.SrcPort) && ports(r.DstPorts, p.DstPort)
}

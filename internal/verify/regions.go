package verify

import (
	"encoding/binary"
	"net/netip"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"strconv"
)

func ruleRegion(r ciscoacl.Rule, b *packetset.Budget) (packetset.Region, error) {
	proto := packetset.AllProtocols()
	if r.Protocol != nil {
		proto = packetset.Protocol(*r.Protocol)
	}
	src := packetset.Cube{Value: r.Src.Value, Wildcard: r.Src.Wildcard}
	dst := packetset.Cube{Value: r.Dst.Value, Wildcard: r.Dst.Wildcard}
	out := packetset.Region{}
	for _, p := range []uint8{6, 17} {
		if proto[p/64]&(uint64(1)<<(p%64)) == 0 {
			continue
		}
		for _, s := range r.SrcPorts {
			for _, d := range r.DstPorts {
				if err := b.Tick(1); err != nil {
					return nil, err
				}
				if err := b.CheckBoxes(len(out) + 1); err != nil {
					return nil, err
				}
				out = append(out, packetset.Box{Protocols: packetset.Protocol(p), Src: src, Dst: dst, SrcPort: packetset.Interval{Lo: s.Lo, Hi: s.Hi}, DstPort: packetset.Interval{Lo: d.Lo, Hi: d.Hi}})
			}
		}
	}
	proto[0] &^= (uint64(1) << 6) | (uint64(1) << 17)
	if proto != (packetset.Protocols{}) {
		if err := b.Tick(1); err != nil {
			return nil, err
		}
		if err := b.CheckBoxes(len(out) + 1); err != nil {
			return nil, err
		}
		out = append(out, packetset.Box{Protocols: proto, Src: src, Dst: dst, SrcPort: packetset.Interval{}, DstPort: packetset.Interval{}})
	}
	packetset.Sort(out)
	return out, nil
}

func conditionRegion(r intent.Region, b *packetset.Budget) (packetset.Region, error) {
	proto := packetset.AllProtocols()
	if r.Protocol != "ip" {
		p := uint8(0)
		switch r.Protocol {
		case "tcp":
			p = 6
		case "udp":
			p = 17
		case "icmp":
			p = 1
		default:
			n, err := strconv.ParseUint(r.Protocol, 10, 8)
			if err != nil {
				return nil, err
			}
			p = uint8(n)
		}
		proto = packetset.Protocol(p)
	}
	src, err := prefixCube(r.SrcCIDR)
	if err != nil {
		return nil, err
	}
	dst, err := prefixCube(r.DstCIDR)
	if err != nil {
		return nil, err
	}
	sports, dports := r.SrcPorts, r.DstPorts
	if len(sports) == 0 {
		sports = []intent.Interval{{Lo: 0, Hi: 65535}}
	}
	if len(dports) == 0 {
		dports = []intent.Interval{{Lo: 0, Hi: 65535}}
	}
	out := packetset.Region{}
	for p := 0; p < 256; p++ {
		if proto[p/64]&(uint64(1)<<uint(p%64)) == 0 {
			continue
		}
		if p != 6 && p != 17 {
			if err := b.Tick(1); err != nil {
				return nil, err
			}
			if err := b.CheckBoxes(len(out) + 1); err != nil {
				return nil, err
			}
			out = append(out, packetset.Box{Protocols: packetset.Protocol(uint8(p)), Src: src, Dst: dst})
			continue
		}
		for _, s := range sports {
			for _, d := range dports {
				if err := b.Tick(1); err != nil {
					return nil, err
				}
				if err := b.CheckBoxes(len(out) + 1); err != nil {
					return nil, err
				}
				out = append(out, packetset.Box{Protocols: packetset.Protocol(uint8(p)), Src: src, Dst: dst, SrcPort: packetset.Interval{Lo: s.Lo, Hi: s.Hi}, DstPort: packetset.Interval{Lo: d.Lo, Hi: d.Hi}})
			}
		}
	}
	for _, side := range []struct {
		prefixes []string
		source   bool
	}{{r.ExcludeSrc, true}, {r.ExcludeDst, false}} {
		for _, s := range side.prefixes {
			cube, err := prefixCube(s)
			if err != nil {
				return nil, err
			}
			cut := packetset.Box{Protocols: packetset.AllProtocols(), Src: packetset.Cube{Wildcard: ^uint32(0)}, Dst: packetset.Cube{Wildcard: ^uint32(0)}, SrcPort: packetset.Interval{Lo: 0, Hi: 65535}, DstPort: packetset.Interval{Lo: 0, Hi: 65535}}
			if side.source {
				cut.Src = cube
			} else {
				cut.Dst = cube
			}
			out, err = packetset.Difference(out, packetset.Region{cut}, b)
			if err != nil {
				return nil, err
			}
		}
	}
	packetset.Sort(out)
	return out, nil
}

func prefixCube(text string) (packetset.Cube, error) {
	p, err := netip.ParsePrefix(text)
	if err != nil {
		return packetset.Cube{}, err
	}
	a := p.Masked().Addr().As4()
	return packetset.Cube{Value: binary.BigEndian.Uint32(a[:]), Wildcard: ^uint32(0) >> p.Bits()}, nil
}

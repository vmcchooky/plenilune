package packetset

import (
	"cmp"
	"math/bits"
	"slices"
)

type Cube struct {
	Value    uint32 `json:"value"`
	Wildcard uint32 `json:"wildcard"`
}
type Interval struct {
	Lo uint16 `json:"lo"`
	Hi uint16 `json:"hi"`
}
type Protocols [4]uint64
type Box struct {
	Protocols Protocols `json:"protocols"`
	Src       Cube      `json:"src"`
	Dst       Cube      `json:"dst"`
	SrcPort   Interval  `json:"src_port"`
	DstPort   Interval  `json:"dst_port"`
}
type Region []Box

func AllProtocols() Protocols    { return Protocols{^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0)} }
func Protocol(p uint8) Protocols { var out Protocols; out[p/64] = uint64(1) << (p % 64); return out }

func valid(b Box) bool {
	return b.Protocols != (Protocols{}) && b.Src.Value&b.Src.Wildcard == 0 && b.Dst.Value&b.Dst.Wildcard == 0 && b.SrcPort.Lo <= b.SrcPort.Hi && b.DstPort.Lo <= b.DstPort.Hi
}

func intersectCube(a, b Cube) (Cube, bool) {
	if (a.Value^b.Value)&^(a.Wildcard|b.Wildcard) != 0 {
		return Cube{}, false
	}
	return Cube{Value: a.Value | b.Value, Wildcard: a.Wildcard & b.Wildcard}, true
}

func Intersect(a, b Box) (Box, bool) {
	if !valid(a) || !valid(b) {
		return Box{}, false
	}
	var out Box
	for i := range out.Protocols {
		out.Protocols[i] = a.Protocols[i] & b.Protocols[i]
	}
	if out.Protocols == (Protocols{}) {
		return Box{}, false
	}
	var ok bool
	if out.Src, ok = intersectCube(a.Src, b.Src); !ok {
		return Box{}, false
	}
	if out.Dst, ok = intersectCube(a.Dst, b.Dst); !ok {
		return Box{}, false
	}
	out.SrcPort = Interval{Lo: max(a.SrcPort.Lo, b.SrcPort.Lo), Hi: min(a.SrcPort.Hi, b.SrcPort.Hi)}
	out.DstPort = Interval{Lo: max(a.DstPort.Lo, b.DstPort.Lo), Hi: min(a.DstPort.Hi, b.DstPort.Hi)}
	if out.SrcPort.Lo > out.SrcPort.Hi || out.DstPort.Lo > out.DstPort.Hi {
		return Box{}, false
	}
	return out, true
}

func Contains(b Box, proto uint8, src, dst uint32, sport, dport uint16) bool {
	return valid(b) && b.Protocols[proto/64]&(uint64(1)<<(proto%64)) != 0 && src&^b.Src.Wildcard == b.Src.Value && dst&^b.Dst.Wildcard == b.Dst.Value && sport >= b.SrcPort.Lo && sport <= b.SrcPort.Hi && dport >= b.DstPort.Lo && dport <= b.DstPort.Hi
}

// Witness chooses the lowest supported tuple in the complete-header domain.
// Non-TCP/UDP protocols have zero ports. Callers still re-evaluate witnesses.
func (b Box) Witness() (proto uint8, src, dst uint32, sport, dport uint16, ok bool) {
	if !valid(b) {
		return
	}
	for word, mask := range b.Protocols {
		for mask != 0 {
			p := uint8(word*64 + bits.TrailingZeros64(mask))
			mask &= mask - 1
			if p != 6 && p != 17 && (b.SrcPort.Lo != 0 || b.DstPort.Lo != 0) {
				continue
			}
			return p, b.Src.Value, b.Dst.Value, b.SrcPort.Lo, b.DstPort.Lo, true
		}
	}
	return
}

func Sort(r Region) {
	slices.SortFunc(r, CompareBoxes)
}

// CompareBoxes is the canonical deterministic box order used by Sort.
func CompareBoxes(a, b Box) int {
	if n := cmp.Compare(firstProtocol(a.Protocols), firstProtocol(b.Protocols)); n != 0 {
		return n
	}
	for i := range a.Protocols {
		if n := cmp.Compare(a.Protocols[i], b.Protocols[i]); n != 0 {
			return n
		}
	}
	for _, p := range [][2]uint32{{a.Src.Value, b.Src.Value}, {a.Src.Wildcard, b.Src.Wildcard}, {a.Dst.Value, b.Dst.Value}, {a.Dst.Wildcard, b.Dst.Wildcard}, {uint32(a.SrcPort.Lo), uint32(b.SrcPort.Lo)}, {uint32(a.SrcPort.Hi), uint32(b.SrcPort.Hi)}, {uint32(a.DstPort.Lo), uint32(b.DstPort.Lo)}, {uint32(a.DstPort.Hi), uint32(b.DstPort.Hi)}} {
		if n := cmp.Compare(p[0], p[1]); n != 0 {
			return n
		}
	}
	return 0
}

func firstProtocol(ps Protocols) int {
	for i, word := range ps {
		if word != 0 {
			return i*64 + bits.TrailingZeros64(word)
		}
	}
	return 256
}

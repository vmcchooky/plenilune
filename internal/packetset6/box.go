// Package packetset6 is the 128-bit counterpart of packetset: exact box
// algebra over IPv6 packet space. It mirrors packetset's constructions and
// budget discipline dimension for dimension, with two changes: address
// cubes are 128-bit (value/wildcard word pairs), and boxes carry an
// ICMPv6 type interval — the dimension IPv6 ACL semantics need for the
// documented implicit neighbor-discovery permits. The type interval is
// meaningful only for protocol 58; region construction keeps it at the
// full range for every other protocol so the dimension stays neutral.
package packetset6

import (
	"cmp"
	"slices"

	"plenilune/internal/packetset"
)

// Cube128 is a 128-bit wildcard cube. Word 0 is the most significant half.
type Cube128 struct {
	Value    [2]uint64 `json:"value"`
	Wildcard [2]uint64 `json:"wildcard"`
}

// Box is one cell of IPv6 packet space: a protocol set, source and
// destination cubes, port intervals, and an ICMPv6 type interval.
type Box struct {
	Protocols packetset.Protocols `json:"protocols"`
	Src       Cube128             `json:"src"`
	Dst       Cube128             `json:"dst"`
	SrcPort   packetset.Interval  `json:"src_port"`
	DstPort   packetset.Interval  `json:"dst_port"`
	IcmpType  packetset.Interval  `json:"icmp_type"`
}

type Region []Box

// Packet is a fully specified IPv6 packet for point-membership checks.
// IcmpType is consulted only when Protocol is 58.
type Packet struct {
	Protocol uint8
	Src      [2]uint64
	Dst      [2]uint64
	SrcPort  uint16
	DstPort  uint16
	IcmpType uint8
}

func FullIcmpType() packetset.Interval { return packetset.Interval{Lo: 0, Hi: 255} }

func validCube(c Cube128) bool {
	return c.Value[0]&c.Wildcard[0] == 0 && c.Value[1]&c.Wildcard[1] == 0
}

func valid(b Box) bool {
	return b.Protocols != (packetset.Protocols{}) && validCube(b.Src) && validCube(b.Dst) &&
		b.SrcPort.Lo <= b.SrcPort.Hi && b.DstPort.Lo <= b.DstPort.Hi &&
		b.IcmpType.Lo <= b.IcmpType.Hi && b.IcmpType.Hi <= 255
}

func cubeContains(c Cube128, v [2]uint64) bool {
	return v[0]&^c.Wildcard[0] == c.Value[0] && v[1]&^c.Wildcard[1] == c.Value[1]
}

// Contains reports whether the packet is a member of the box.
func (b Box) Contains(p Packet) bool {
	if b.Protocols[p.Protocol/64]&(uint64(1)<<(p.Protocol%64)) == 0 {
		return false
	}
	if !cubeContains(b.Src, p.Src) || !cubeContains(b.Dst, p.Dst) {
		return false
	}
	if p.SrcPort < b.SrcPort.Lo || p.SrcPort > b.SrcPort.Hi || p.DstPort < b.DstPort.Lo || p.DstPort > b.DstPort.Hi {
		return false
	}
	if p.Protocol == 58 && (uint16(p.IcmpType) < b.IcmpType.Lo || uint16(p.IcmpType) > b.IcmpType.Hi) {
		return false
	}
	return true
}

// RegionContains reports whether any box of the region contains the packet.
func RegionContains(r Region, p Packet) bool {
	for _, b := range r {
		if b.Contains(p) {
			return true
		}
	}
	return false
}

func intersectCube(a, b Cube128) (Cube128, bool) {
	var out Cube128
	for w := 0; w < 2; w++ {
		if (a.Value[w]^b.Value[w])&^(a.Wildcard[w]|b.Wildcard[w]) != 0 {
			return Cube128{}, false
		}
		out.Value[w] = a.Value[w] | b.Value[w]
		out.Wildcard[w] = a.Wildcard[w] & b.Wildcard[w]
	}
	return out, true
}

func intersectInterval(a, b packetset.Interval) (packetset.Interval, bool) {
	out := packetset.Interval{Lo: max(a.Lo, b.Lo), Hi: min(a.Hi, b.Hi)}
	return out, out.Lo <= out.Hi
}

// Intersect returns the intersection of two boxes, or false when disjoint.
func Intersect(a, b Box) (Box, bool) {
	if !valid(a) || !valid(b) {
		return Box{}, false
	}
	var out Box
	for i := range out.Protocols {
		out.Protocols[i] = a.Protocols[i] & b.Protocols[i]
	}
	if out.Protocols == (packetset.Protocols{}) {
		return Box{}, false
	}
	var ok bool
	if out.Src, ok = intersectCube(a.Src, b.Src); !ok {
		return Box{}, false
	}
	if out.Dst, ok = intersectCube(a.Dst, b.Dst); !ok {
		return Box{}, false
	}
	if out.SrcPort, ok = intersectInterval(a.SrcPort, b.SrcPort); !ok {
		return Box{}, false
	}
	if out.DstPort, ok = intersectInterval(a.DstPort, b.DstPort); !ok {
		return Box{}, false
	}
	if out.IcmpType, ok = intersectInterval(a.IcmpType, b.IcmpType); !ok {
		return Box{}, false
	}
	return out, true
}

func compareCube(a, b Cube128) int {
	for w := 0; w < 2; w++ {
		if c := cmp.Compare(a.Value[w], b.Value[w]); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Wildcard[w], b.Wildcard[w]); c != 0 {
			return c
		}
	}
	return 0
}

func compareInterval(a, b packetset.Interval) int {
	if c := cmp.Compare(a.Lo, b.Lo); c != 0 {
		return c
	}
	return cmp.Compare(a.Hi, b.Hi)
}

// CompareBoxes is a total order over boxes, for canonical region sorting.
func CompareBoxes(a, b Box) int {
	for i := range a.Protocols {
		if c := cmp.Compare(a.Protocols[i], b.Protocols[i]); c != 0 {
			return c
		}
	}
	if c := compareCube(a.Src, b.Src); c != 0 {
		return c
	}
	if c := compareCube(a.Dst, b.Dst); c != 0 {
		return c
	}
	if c := compareInterval(a.SrcPort, b.SrcPort); c != 0 {
		return c
	}
	if c := compareInterval(a.DstPort, b.DstPort); c != 0 {
		return c
	}
	return compareInterval(a.IcmpType, b.IcmpType)
}

// Sort orders a region canonically.
func Sort(r Region) { slices.SortFunc(r, CompareBoxes) }

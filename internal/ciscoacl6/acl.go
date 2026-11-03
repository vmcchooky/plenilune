// Package ciscoacl6 parses one bounded named IPv6 ACL and evaluates a
// single complete IPv6 packet. It is the IPv6 counterpart of ciscoacl,
// with the semantics Cisco documents for IOS IPv6 ACLs — including the
// implicit tail every IPv6 ACL carries: permit icmp any any nd-na,
// permit icmp any any nd-ns, then deny ipv6 any any (Cisco IOS IPv6
// command reference). It does not model interface binding, extension
// headers, or reachability.
package ciscoacl6

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// ParserVersion versions the IPv6 ACL parser independently of the IPv4
// parser line.
const ParserVersion = "cisco-acl6/v0.1"

const (
	MaxInputBytes = 2 << 20
	MaxRules      = 512
	MaxLines      = 10000
	MaxLineBytes  = 8192
	// MaxPortIntervals mirrors ciscoacl's cap on one rule's port set.
	MaxPortIntervals = 128
)

// ICMPv6 types modeled by this profile (IOS names in parse.go).
const (
	IcmpDestinationUnreachable = 1
	IcmpPacketTooBig           = 2
	IcmpTimeExceeded           = 3
	IcmpParameterProblem       = 4
	IcmpEchoRequest            = 128
	IcmpEchoReply              = 129
	IcmpRouterSolicitation     = 133
	IcmpRouterAdvertisement    = 134
	IcmpNeighborSolicitation   = 135
	IcmpNeighborAdvertisement  = 136
)

// Error deliberately excludes source text; errors must not leak a rule.
type Error struct {
	Code    string `json:"code"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s at line %d: %s", e.Code, e.Line, e.Message) }

func fail(code string, line int, message string) error {
	return &Error{Code: code, Line: line, Message: message}
}

// Address128 is a 128-bit wildcard cube: normalized fixed bits plus the
// wildcard (1 = ignore). Word 0 is the most significant half. Parsers
// produce prefix cubes (contiguous wildcard tails), but the type — like
// the IPv4 model — carries general wildcard semantics.
type Address128 struct {
	Value    [2]uint64 `json:"value"`
	Wildcard [2]uint64 `json:"wildcard"`
}

type Interval struct {
	Lo uint16 `json:"lo"`
	Hi uint16 `json:"hi"`
}

type Rule struct {
	Ordinal  int        `json:"ordinal"`
	Line     int        `json:"line"`
	Sequence *uint32    `json:"sequence,omitempty"`
	Action   string     `json:"action"`
	Protocol *uint8     `json:"protocol"` // null means all IPv6 payloads ("ipv6")
	Src      Address128 `json:"src"`
	Dst      Address128 `json:"dst"`
	SrcPorts []Interval `json:"src_ports"`
	DstPorts []Interval `json:"dst_ports"`
	IcmpType *uint8     `json:"icmp_type,omitempty"` // only for protocol 58; null means any type
	Logging  string     `json:"logging,omitempty"`
}

type ACL struct {
	Name     string   `json:"name"`
	Rules    []Rule   `json:"rules"`
	Warnings []string `json:"warnings"`
}

type Packet struct {
	Protocol uint8      `json:"protocol"`
	SrcIP    netip.Addr `json:"src_ip"`
	DstIP    netip.Addr `json:"dst_ip"`
	SrcPort  uint16     `json:"src_port"`
	DstPort  uint16     `json:"dst_port"`
	IcmpType uint8      `json:"icmp_type"` // consulted only when Protocol is 58
}

type Decision struct {
	Action      string `json:"action"`
	RuleOrdinal *int   `json:"rule_ordinal,omitempty"`
	Line        *int   `json:"line,omitempty"`
	Implicit    bool   `json:"implicit"`
}

func (p Packet) Validate() error {
	if !p.SrcIP.Is6() || p.SrcIP.Is4In6() || !p.DstIP.Is6() || p.DstIP.Is4In6() {
		return fail("PACKET_INVALID", 0, "source and destination must be IPv6 addresses")
	}
	if p.Protocol != 6 && p.Protocol != 17 && (p.SrcPort != 0 || p.DstPort != 0) {
		return fail("PACKET_INVALID", 0, "ports must be zero for protocols other than TCP or UDP")
	}
	return nil
}

func ipv6Words(a netip.Addr) [2]uint64 {
	b := a.As16()
	return [2]uint64{binary.BigEndian.Uint64(b[:8]), binary.BigEndian.Uint64(b[8:])}
}

func matchAddr(a Address128, v [2]uint64) bool {
	return v[0]&^a.Wildcard[0] == a.Value[0] && v[1]&^a.Wildcard[1] == a.Value[1]
}

func validPorts(xs []Interval) bool {
	if len(xs) == 0 || len(xs) > MaxPortIntervals {
		return false
	}
	for i, x := range xs {
		if x.Lo > x.Hi || i > 0 && uint32(xs[i-1].Hi)+1 >= uint32(x.Lo) {
			return false
		}
	}
	return true
}

func fullPorts(xs []Interval) bool { return len(xs) == 1 && xs[0] == (Interval{0, 65535}) }

func containsPort(xs []Interval, port uint16) bool {
	for _, x := range xs {
		if port >= x.Lo && port <= x.Hi {
			return true
		}
	}
	return false
}

// Evaluate scans explicit rules directly, then applies the documented
// implicit tail: ICMPv6 neighbor advertisement (136) and neighbor
// solicitation (135) are permitted; everything else is denied.
func (a ACL) Evaluate(p Packet) (Decision, error) {
	if err := p.Validate(); err != nil {
		return Decision{}, err
	}
	if !aclName.MatchString(a.Name) || len(a.Rules) == 0 || len(a.Rules) > MaxRules {
		return Decision{}, fail("ACL_INVALID", 0, "invalid ACL identity or rule count")
	}
	previousLine := 0
	for i, r := range a.Rules {
		if r.Ordinal != i || r.Line < 1 || r.Line > MaxLines || r.Line <= previousLine {
			return Decision{}, fail("ACL_INVALID", 0, "invalid rule source binding")
		}
		previousLine = r.Line
		if r.Action != "permit" && r.Action != "deny" ||
			r.Src.Value[0]&r.Src.Wildcard[0] != 0 || r.Src.Value[1]&r.Src.Wildcard[1] != 0 ||
			r.Dst.Value[0]&r.Dst.Wildcard[0] != 0 || r.Dst.Value[1]&r.Dst.Wildcard[1] != 0 ||
			!validPorts(r.SrcPorts) || !validPorts(r.DstPorts) {
			return Decision{}, fail("ACL_INVALID", r.Line, "invalid normalized rule")
		}
		if r.Protocol == nil || *r.Protocol != 58 {
			if r.IcmpType != nil {
				return Decision{}, fail("ACL_INVALID", r.Line, "ICMP type constraint requires ICMPv6")
			}
		}
		if (r.Protocol == nil || *r.Protocol != 6 && *r.Protocol != 17) && (!fullPorts(r.SrcPorts) || !fullPorts(r.DstPorts)) {
			return Decision{}, fail("ACL_INVALID", r.Line, "port constraint requires TCP or UDP")
		}
		if r.Protocol != nil && *r.Protocol == 58 && (!fullPorts(r.SrcPorts) || !fullPorts(r.DstPorts)) {
			return Decision{}, fail("ACL_INVALID", r.Line, "ICMPv6 rules cannot carry port constraints")
		}
	}
	src, dst := ipv6Words(p.SrcIP), ipv6Words(p.DstIP)
	for _, r := range a.Rules {
		if r.Protocol != nil && *r.Protocol != p.Protocol {
			continue
		}
		if !matchAddr(r.Src, src) || !matchAddr(r.Dst, dst) {
			continue
		}
		if p.Protocol == 6 || p.Protocol == 17 {
			if !containsPort(r.SrcPorts, p.SrcPort) || !containsPort(r.DstPorts, p.DstPort) {
				continue
			}
		}
		if p.Protocol == 58 && r.IcmpType != nil && *r.IcmpType != p.IcmpType {
			continue
		}
		// Other payloads match on protocol + addresses only.
		ordinal, line := r.Ordinal, r.Line
		return Decision{Action: r.Action, RuleOrdinal: &ordinal, Line: &line}, nil
	}
	// Implicit tail (documented for every IPv6 ACL): neighbor discovery is
	// permitted, everything else is denied.
	if p.Protocol == 58 && (p.IcmpType == IcmpNeighborAdvertisement || p.IcmpType == IcmpNeighborSolicitation) {
		return Decision{Action: "permit", Implicit: true}, nil
	}
	return Decision{Action: "deny", Implicit: true}, nil
}

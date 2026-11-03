// Package ciscoacl parses one bounded IPv4 ACL and evaluates a single
// complete IPv4 packet. It does not model interface binding or network reachability.
package ciscoacl

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

const (
	ParserVersion         = "cisco-acl/v0.4"
	PreviousParserVersion = "cisco-acl/v0.3"
	LegacyParserVersion   = "cisco-acl/v0.2"
	MaxInputBytes         = 2 << 20
	MaxRules              = 512
	MaxLines              = 10000
	MaxLineBytes          = 8192
	// MaxPortIntervals bounds one rule's port interval set. Parsers before
	// v0.4 produced at most 2 intervals (a single port operator); v0.4
	// service object-groups resolve to merged interval sets, so the DTO
	// validation cap rises to 128. Group-free parser output is unchanged.
	MaxPortIntervals = 128
)

// Error deliberately excludes source text; CLI errors must not leak a rule.
type Error struct {
	Code    string `json:"code"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s at line %d: %s", e.Code, e.Line, e.Message) }

func fail(code string, line int, message string) error {
	return &Error{Code: code, Line: line, Message: message}
}

type Interval struct {
	Lo uint16 `json:"lo"`
	Hi uint16 `json:"hi"`
}

// Address stores normalized fixed bits and the Cisco wildcard (1 = ignore).
type Address struct {
	Value    uint32 `json:"value"`
	Wildcard uint32 `json:"wildcard"`
}

type Rule struct {
	Ordinal  int     `json:"ordinal"`            // evaluation index; standard may differ from physical order
	Line     int     `json:"line"`               // original physical source line, 1-based
	Sequence *uint32 `json:"sequence,omitempty"` // standard: effective sequence, including implicit numbering
	Action   string  `json:"action"`
	Protocol *uint8  `json:"protocol"` // null means all IPv4 protocols
	Src      Address `json:"src"`
	Dst      Address `json:"dst"`
	// SrcSet and DstSet are the v0.4 object-group union form: when non-empty
	// the set is authoritative and the matching single Address must be the
	// zero value. Members are canonical: sorted by (Value, Wildcard) with
	// exact duplicates removed. See docs/semantics/object-groups-v04.md.
	SrcSet   []Address  `json:"src_set,omitempty"`
	DstSet   []Address  `json:"dst_set,omitempty"`
	SrcPorts []Interval `json:"src_ports"`
	DstPorts []Interval `json:"dst_ports"`
	Logging  string     `json:"logging,omitempty"`
}

type ACL struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Rules    []Rule   `json:"rules"`
	Warnings []string `json:"warnings"`
}

type Packet struct {
	Protocol uint8      `json:"protocol"`
	SrcIP    netip.Addr `json:"src_ip"`
	DstIP    netip.Addr `json:"dst_ip"`
	SrcPort  uint16     `json:"src_port"`
	DstPort  uint16     `json:"dst_port"`
}

type Decision struct {
	Action      string `json:"action"`
	RuleOrdinal *int   `json:"rule_ordinal,omitempty"`
	Line        *int   `json:"line,omitempty"`
	Implicit    bool   `json:"implicit"`
}

// Validate limits packets to complete IPv4 headers. Fragment and TCP state are
// intentionally absent from this point-evaluation model.
func (p Packet) Validate() error {
	if !p.SrcIP.Is4() || !p.DstIP.Is4() {
		return fail("PACKET_INVALID", 0, "source and destination must be IPv4 addresses")
	}
	if p.Protocol != 6 && p.Protocol != 17 && (p.SrcPort != 0 || p.DstPort != 0) {
		return fail("PACKET_INVALID", 0, "ports must be zero for protocols other than TCP or UDP")
	}
	return nil
}

// Evaluate scans rules directly, independently of any future region algebra.
// A deny decision is not a statement about end-to-end network reachability.
func (a ACL) Evaluate(p Packet) (Decision, error) {
	if err := p.Validate(); err != nil {
		return Decision{}, err
	}
	if !SupportedKind(a.Kind, ParserVersion) || !aclName.MatchString(a.Name) || len(a.Rules) == 0 || len(a.Rules) > MaxRules {
		return Decision{}, fail("ACL_INVALID", 0, "invalid ACL identity or rule count")
	}
	if a.Kind == "standard" && !validStandardName(a.Name) {
		return Decision{}, fail("ACL_INVALID", 0, "invalid standard ACL identity")
	}
	// Validate the complete supplied IR before making any decision, including
	// unreachable rules. Exported DTOs can be constructed outside the parser.
	previousLine := 0
	var seenLines map[int]bool
	var seenHosts map[uint32]bool
	var seenSequences map[uint32]bool
	if a.Kind == "standard" {
		seenLines = make(map[int]bool, len(a.Rules))
		seenHosts = make(map[uint32]bool)
		seenSequences = make(map[uint32]bool)
	}
	var lastNetworkSequence uint32
	haveNetwork := false
	for i, r := range a.Rules {
		if r.Ordinal != i || r.Line < 1 || r.Line > MaxLines || seenLines[r.Line] || a.Kind == "named-extended" && r.Line <= previousLine {
			return Decision{}, fail("ACL_INVALID", 0, "invalid rule source binding")
		}
		if a.Kind == "standard" {
			seenLines[r.Line] = true
		}
		previousLine = r.Line
		if r.Action != "permit" && r.Action != "deny" || r.Src.Value&r.Src.Wildcard != 0 || r.Dst.Value&r.Dst.Wildcard != 0 ||
			!validPorts(r.SrcPorts) || !validPorts(r.DstPorts) || !validAddressSet(r.SrcSet, r.Src) || !validAddressSet(r.DstSet, r.Dst) {
			return Decision{}, fail("ACL_INVALID", r.Line, "invalid normalized rule")
		}
		if a.Kind == "standard" && (len(r.SrcSet) > 0 || len(r.DstSet) > 0) {
			return Decision{}, fail("ACL_INVALID", r.Line, "standard rules cannot carry address sets")
		}
		if (r.Protocol == nil || *r.Protocol != 6 && *r.Protocol != 17) && (!fullPorts(r.SrcPorts) || !fullPorts(r.DstPorts)) {
			return Decision{}, fail("ACL_INVALID", r.Line, "port constraint requires TCP or UDP")
		}
		if a.Kind == "standard" {
			if r.Protocol != nil || r.Dst != (Address{Wildcard: ^uint32(0)}) || !fullPorts(r.SrcPorts) || !fullPorts(r.DstPorts) || r.Sequence == nil || *r.Sequence == 0 || *r.Sequence > maxStandardSequence || seenSequences[*r.Sequence] || r.Logging != "" && r.Logging != "log" && r.Logging != "log-input" {
				return Decision{}, fail("ACL_INVALID", r.Line, "invalid standard source-only rule")
			}
			seenSequences[*r.Sequence] = true
			if r.Src.Wildcard == 0 {
				if haveNetwork || seenHosts[r.Src.Value] {
					return Decision{}, fail("ACL_INVALID", r.Line, "invalid standard host order or duplicate host")
				}
				seenHosts[r.Src.Value] = true
			} else {
				if haveNetwork && *r.Sequence <= lastNetworkSequence {
					return Decision{}, fail("ACL_INVALID", r.Line, "standard network sequences must increase")
				}
				haveNetwork, lastNetworkSequence = true, *r.Sequence
			}
		}
	}
	src, dst := ipv4(p.SrcIP), ipv4(p.DstIP)
	for _, r := range a.Rules {
		if (r.Protocol == nil || *r.Protocol == p.Protocol) && matchAddress(r.SrcSet, r.Src, src) &&
			matchAddress(r.DstSet, r.Dst, dst) && containsPort(r.SrcPorts, p.SrcPort) && containsPort(r.DstPorts, p.DstPort) {
			ordinal, line := r.Ordinal, r.Line
			return Decision{Action: r.Action, RuleOrdinal: &ordinal, Line: &line}, nil
		}
	}
	return Decision{Action: "deny", Implicit: true}, nil
}

func ipv4(a netip.Addr) uint32 { b := a.As4(); return binary.BigEndian.Uint32(b[:]) }

// validAddressSet enforces the union-form contract: a non-empty set is
// authoritative, so the single Address must be zero; members must be
// canonical (normalized, strictly increasing by (Value, Wildcard)).
func validAddressSet(set []Address, single Address) bool {
	if len(set) == 0 {
		return true
	}
	if single != (Address{}) || len(set) > MaxGroupMembers {
		return false
	}
	for i, a := range set {
		if a.Value&a.Wildcard != 0 {
			return false
		}
		if i > 0 {
			prev := set[i-1]
			if prev.Value > a.Value || prev.Value == a.Value && prev.Wildcard >= a.Wildcard {
				return false
			}
		}
	}
	return true
}

func matchAddress(set []Address, single Address, v uint32) bool {
	if len(set) == 0 {
		return v & ^single.Wildcard == single.Value
	}
	for _, a := range set {
		if v & ^a.Wildcard == a.Value {
			return true
		}
	}
	return false
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

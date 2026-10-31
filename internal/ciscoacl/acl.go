// Package ciscoacl parses one bounded named extended ACL and evaluates a single
// complete IPv4 packet. It does not model interface binding or network reachability.
package ciscoacl

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

const (
	ParserVersion = "cisco-acl/v0.2"
	MaxInputBytes = 2 << 20
	MaxRules      = 512
	MaxLines      = 10000
	MaxLineBytes  = 8192
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
	Ordinal  int        `json:"ordinal"`
	Line     int        `json:"line"`
	Sequence *uint32    `json:"sequence,omitempty"`
	Action   string     `json:"action"`
	Protocol *uint8     `json:"protocol"` // null means all IPv4 protocols
	Src      Address    `json:"src"`
	Dst      Address    `json:"dst"`
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
	if a.Kind != "named-extended" || !aclName.MatchString(a.Name) || len(a.Rules) == 0 || len(a.Rules) > MaxRules {
		return Decision{}, fail("ACL_INVALID", 0, "invalid ACL identity or rule count")
	}
	// Validate the complete supplied IR before making any decision, including
	// unreachable rules. Exported DTOs can be constructed outside the parser.
	previousLine := 0
	for i, r := range a.Rules {
		if r.Ordinal != i || r.Line < 1 || r.Line > MaxLines || r.Line <= previousLine {
			return Decision{}, fail("ACL_INVALID", 0, "invalid rule source binding")
		}
		previousLine = r.Line
		if r.Action != "permit" && r.Action != "deny" || r.Src.Value&r.Src.Wildcard != 0 || r.Dst.Value&r.Dst.Wildcard != 0 ||
			!validPorts(r.SrcPorts) || !validPorts(r.DstPorts) {
			return Decision{}, fail("ACL_INVALID", r.Line, "invalid normalized rule")
		}
		if (r.Protocol == nil || *r.Protocol != 6 && *r.Protocol != 17) && (!fullPorts(r.SrcPorts) || !fullPorts(r.DstPorts)) {
			return Decision{}, fail("ACL_INVALID", r.Line, "port constraint requires TCP or UDP")
		}
	}
	src, dst := ipv4(p.SrcIP), ipv4(p.DstIP)
	for _, r := range a.Rules {
		if (r.Protocol == nil || *r.Protocol == p.Protocol) && src & ^r.Src.Wildcard == r.Src.Value &&
			dst & ^r.Dst.Wildcard == r.Dst.Value && containsPort(r.SrcPorts, p.SrcPort) && containsPort(r.DstPorts, p.DstPort) {
			ordinal, line := r.Ordinal, r.Line
			return Decision{Action: r.Action, RuleOrdinal: &ordinal, Line: &line}, nil
		}
	}
	return Decision{Action: "deny", Implicit: true}, nil
}

func ipv4(a netip.Addr) uint32 { b := a.As4(); return binary.BigEndian.Uint32(b[:]) }

func validPorts(xs []Interval) bool {
	if len(xs) == 0 || len(xs) > 2 {
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

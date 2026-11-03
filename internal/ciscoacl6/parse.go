package ciscoacl6

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var aclName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

// Parse accepts one document holding exactly one named IPv6 ACL.
func Parse(input []byte) (ACL, error) {
	var zero ACL
	if len(input) > MaxInputBytes {
		return zero, fail("INPUT_TOO_LARGE", 0, "ACL exceeds byte limit")
	}
	if !utf8.Valid(input) {
		return zero, fail("PARSE_ENCODING", 0, "input must be UTF-8")
	}
	text := strings.ReplaceAll(string(input), "\r\n", "\n")
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return zero, fail("PARSE_ENCODING", 0, "unsupported control character")
		}
	}
	logicalLines := strings.Count(text, "\n")
	if text != "" && !strings.HasSuffix(text, "\n") {
		logicalLines++
	}
	if logicalLines > MaxLines {
		return zero, fail("PARSE_LIMIT", 0, "ACL exceeds line limit")
	}
	lines := strings.Split(text, "\n")
	a := ACL{Rules: []Rule{}, Warnings: []string{}}
	seen, closed := false, false
	var lastSeq uint32
	var haveSeq, haveRuleSeq, haveUnsequencedRule bool
	for i, line := range lines {
		n := i + 1
		if len(line) > MaxLineBytes {
			return zero, fail("PARSE_LIMIT", n, "line exceeds byte limit")
		}
		t := strings.Trim(line, " \t")
		if t == "" {
			continue
		}
		indented := line[0] == ' ' || line[0] == '\t'
		if t == "!" {
			if seen && !indented {
				closed = true
			}
			continue
		}
		if !indented && strings.EqualFold(t, "end") {
			if !seen {
				return zero, fail("PARSE_DOCUMENT", n, "end appears before ACL")
			}
			closed = true
			continue
		}
		f := strings.FieldsFunc(t, func(r rune) bool { return r == ' ' || r == '\t' })
		if len(f) >= 2 && strings.EqualFold(f[0], "ipv6") && strings.EqualFold(f[1], "access-list") {
			if seen {
				return zero, fail("PARSE_MULTIPLE_ACLS", n, "exactly one ACL is required")
			}
			if indented || len(f) != 3 || !aclName.MatchString(f[2]) {
				return zero, fail("PARSE_HEADER", n, "expected named IPv6 ACL header")
			}
			a.Name, seen = f[2], true
			continue
		}
		if !seen || closed || !indented {
			return zero, fail("PARSE_DOCUMENT", n, "only one ACL block is accepted")
		}
		var seq *uint32
		if len(f) > 0 && decimal(f[0]) {
			value, err := strconv.ParseUint(f[0], 10, 32)
			if err != nil || value == 0 {
				return zero, fail("PARSE_SEQUENCE_ORDER", n, "invalid sequence")
			}
			v := uint32(value)
			if haveSeq && v <= lastSeq {
				return zero, fail("PARSE_SEQUENCE_ORDER", n, "sequence numbers must increase")
			}
			lastSeq, haveSeq, seq = v, true, &v
			f = f[1:]
		}
		if len(f) == 0 {
			return zero, fail("PARSE_INVALID_TOKEN", n, "rule is missing")
		}
		if strings.EqualFold(f[0], "remark") {
			if len(f) < 2 {
				return zero, fail("PARSE_INVALID_TOKEN", n, "remark text is missing")
			}
			continue
		}
		if len(a.Rules) >= MaxRules {
			return zero, fail("PARSE_LIMIT", n, "ACL exceeds rule limit")
		}
		r, err := parseRule(f, n)
		if err != nil {
			return zero, err
		}
		if seq == nil {
			haveUnsequencedRule = true
		} else {
			haveRuleSeq = true
		}
		r.Ordinal, r.Sequence = len(a.Rules), seq
		a.Rules = append(a.Rules, r)
	}
	if !seen {
		return zero, fail("PARSE_NOT_FOUND", 0, "ACL header is missing")
	}
	if len(a.Rules) == 0 {
		return zero, fail("PARSE_EMPTY_ACL", 0, "ACL must contain at least one executable rule")
	}
	if haveRuleSeq && haveUnsequencedRule {
		a.Warnings = append(a.Warnings, "sequence_partial")
	}
	return a, nil
}

var icmpTypeNames = map[string]uint8{
	"destination-unreachable": IcmpDestinationUnreachable,
	"packet-too-big":          IcmpPacketTooBig,
	"time-exceeded":           IcmpTimeExceeded,
	"parameter-problem":       IcmpParameterProblem,
	"echo-request":            IcmpEchoRequest,
	"echo-reply":              IcmpEchoReply,
	"nd-rs":                   IcmpRouterSolicitation,
	"router-solicitation":     IcmpRouterSolicitation,
	"nd-ra":                   IcmpRouterAdvertisement,
	"router-advertisement":    IcmpRouterAdvertisement,
	"nd-ns":                   IcmpNeighborSolicitation,
	"neighbor-solicitation":   IcmpNeighborSolicitation,
	"nd-na":                   IcmpNeighborAdvertisement,
	"neighbor-advertisement":  IcmpNeighborAdvertisement,
}

func parseRule(f []string, line int) (Rule, error) {
	r := Rule{Line: line, SrcPorts: []Interval{{0, 65535}}, DstPorts: []Interval{{0, 65535}}}
	bad := func(code, msg string) (Rule, error) { return Rule{}, fail(code, line, msg) }
	if len(f) < 4 {
		return bad("PARSE_INVALID_TOKEN", "rule is incomplete")
	}
	r.Action = strings.ToLower(f[0])
	if r.Action != "permit" && r.Action != "deny" {
		return bad("PARSE_UNSUPPORTED", "unsupported rule action")
	}
	proto := strings.ToLower(f[1])
	if proto != "ipv6" {
		var p uint8
		switch proto {
		case "tcp":
			p = 6
		case "udp":
			p = 17
		case "icmp":
			p = 58 // ICMPv6 in IPv6 ACLs
		default:
			if !decimal(proto) {
				return bad("PARSE_UNSUPPORTED", "unsupported protocol name")
			}
			x, err := strconv.ParseUint(proto, 10, 8)
			if err != nil {
				return bad("PARSE_UNSUPPORTED", "protocol must be 0 through 255")
			}
			p = uint8(x)
		}
		r.Protocol = &p
	}
	pos := 2
	var err error
	if r.Src, pos, err = parseOperand(f, pos, line); err != nil {
		return Rule{}, err
	}
	if r.Protocol != nil && *r.Protocol == 58 {
		// ICMPv6 rules take no source-side qualifier in this profile.
	} else if pos < len(f) && isPortOp(f[pos]) {
		if r.Protocol == nil || *r.Protocol != 6 && *r.Protocol != 17 {
			return bad("PARSE_PORT_NOT_ALLOWED", "port constraint requires TCP or UDP")
		}
		if r.SrcPorts, pos, err = parsePorts(f, pos, line); err != nil {
			return Rule{}, err
		}
	}
	if r.Dst, pos, err = parseOperand(f, pos, line); err != nil {
		return Rule{}, err
	}
	if pos < len(f) {
		if r.Protocol != nil && *r.Protocol == 58 {
			if t, ok := icmpTypeNames[strings.ToLower(f[pos])]; ok {
				r.IcmpType = &t
				pos++
			} else if isPortOp(f[pos]) {
				return bad("PARSE_PORT_NOT_ALLOWED", "ICMPv6 rules match a message type, not ports")
			}
		} else if isPortOp(f[pos]) {
			if r.Protocol == nil || *r.Protocol != 6 && *r.Protocol != 17 {
				return bad("PARSE_PORT_NOT_ALLOWED", "port constraint requires TCP or UDP")
			}
			if r.DstPorts, pos, err = parsePorts(f, pos, line); err != nil {
				return Rule{}, err
			}
		}
	}
	if pos < len(f) {
		option := strings.ToLower(f[pos])
		if pos != len(f)-1 || option != "log" && option != "log-input" {
			return bad("PARSE_UNSUPPORTED", "unsupported rule option or trailing token")
		}
		r.Logging = option
	}
	return r, nil
}

// parseOperand parses one IPv6 address operand: "any", "host <addr>", or
// "<addr>/<prefix-len>". IPv6 ACLs have no wildcard-mask form.
func parseOperand(f []string, pos, line int) (Address128, int, error) {
	bad := func() (Address128, int, error) {
		return Address128{}, pos, fail("PARSE_INVALID_IP", line, "expected IPv6 prefix, host, or any")
	}
	if pos >= len(f) {
		return bad()
	}
	if strings.EqualFold(f[pos], "any") {
		return Address128{Wildcard: [2]uint64{^uint64(0), ^uint64(0)}}, pos + 1, nil
	}
	if strings.EqualFold(f[pos], "host") {
		if pos+1 >= len(f) {
			return bad()
		}
		addr, err := netip.ParseAddr(f[pos+1])
		if err != nil || !addr.Is6() || addr.Is4In6() || addr.Zone() != "" {
			return bad()
		}
		return Address128{Value: ipv6Words(addr)}, pos + 2, nil
	}
	prefix, err := netip.ParsePrefix(f[pos])
	if err != nil || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
		return bad()
	}
	prefix = prefix.Masked()
	v := ipv6Words(prefix.Addr())
	bits := prefix.Bits()
	var w [2]uint64
	for i := bits; i < 128; i++ {
		w[i/64] |= uint64(1) << (63 - i%64)
	}
	return Address128{Value: v, Wildcard: w}, pos + 1, nil
}

func decimal(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isPortOp(s string) bool {
	switch strings.ToLower(s) {
	case "eq", "neq", "lt", "gt", "range":
		return true
	}
	return false
}

func parsePorts(f []string, pos, line int) ([]Interval, int, error) {
	bad := func() ([]Interval, int, error) {
		return nil, pos, fail("PARSE_INVALID_PORT", line, "expected numeric nonempty port constraint")
	}
	op := strings.ToLower(f[pos])
	pos++
	if pos >= len(f) || !decimal(f[pos]) {
		return bad()
	}
	v, err := strconv.ParseUint(f[pos], 10, 16)
	if err != nil {
		return bad()
	}
	p := uint16(v)
	pos++
	switch op {
	case "eq":
		return []Interval{{p, p}}, pos, nil
	case "neq":
		out := []Interval{}
		if p > 0 {
			out = append(out, Interval{0, p - 1})
		}
		if p < 65535 {
			out = append(out, Interval{p + 1, 65535})
		}
		return out, pos, nil
	case "lt":
		if p == 0 {
			return bad()
		}
		return []Interval{{0, p - 1}}, pos, nil
	case "gt":
		if p == 65535 {
			return bad()
		}
		return []Interval{{p + 1, 65535}}, pos, nil
	case "range":
		if pos >= len(f) || !decimal(f[pos]) {
			return bad()
		}
		hi, err := strconv.ParseUint(f[pos], 10, 16)
		if err != nil || hi < v {
			return bad()
		}
		return []Interval{{p, uint16(hi)}}, pos + 1, nil
	}
	return bad()
}

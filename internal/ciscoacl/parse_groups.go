package ciscoacl

import (
	"net/netip"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// parseExtendedV4 is the cisco-acl/v0.4 named-extended parser: the v0.3
// document grammar plus object-group definitions and references.
// Group-free documents parse exactly as parseLegacy does. Definitions may
// appear before or after the ACL block; references resolve after the whole
// document is read, so definition order does not matter.
func parseExtendedV4(input []byte) (ACL, error) {
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
	groups, rest, err := extractGroups(lines)
	if err != nil {
		return zero, err
	}

	a := ACL{Kind: "named-extended", Rules: []Rule{}, Warnings: []string{}}
	seen, closed := false, false
	var lastSeq uint32
	var haveSeq, haveRuleSeq, haveUnsequencedRule bool
	for i, line := range rest {
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
		if len(f) >= 2 && strings.EqualFold(f[0], "ip") && strings.EqualFold(f[1], "access-list") {
			if seen {
				return zero, fail("PARSE_MULTIPLE_ACLS", n, "exactly one ACL is required")
			}
			if indented || len(f) != 4 || !strings.EqualFold(f[2], "extended") || !aclName.MatchString(f[3]) {
				return zero, fail("PARSE_HEADER", n, "expected named extended ACL header")
			}
			a.Name, seen = f[3], true
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
		r, err := parseRuleV4(f, n, groups)
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

// extractGroups pulls object-group blocks out of the document. Consumed
// lines are blanked in the returned slice so line numbers are preserved.
// A block is a non-indented "object-group ..." header followed by indented
// member lines; it ends at the first non-indented line.
func extractGroups(lines []string) (*groupTable, []string, error) {
	groups := newGroupTable()
	rest := append([]string(nil), lines...)
	i := 0
	for i < len(lines) {
		line := lines[i]
		t := strings.Trim(line, " \t")
		if t == "" || line[0] == ' ' || line[0] == '\t' {
			i++
			continue
		}
		f := strings.FieldsFunc(t, func(r rune) bool { return r == ' ' || r == '\t' })
		if len(f) == 0 || !strings.EqualFold(f[0], "object-group") {
			i++
			continue
		}
		headerLine := i + 1
		if len(groups.network)+len(groups.service) >= MaxGroups {
			return nil, nil, fail("PARSE_GROUP_LIMIT", headerLine, "too many object-group definitions")
		}
		kind, name, svc, err := parseGroupHeader(f, headerLine)
		if err != nil {
			return nil, nil, err
		}
		rest[i] = ""
		i++
		for i < len(lines) {
			ml := lines[i]
			mt := strings.Trim(ml, " \t")
			if mt == "" {
				i++
				continue
			}
			if ml[0] != ' ' && ml[0] != '\t' {
				break
			}
			mf := strings.FieldsFunc(mt, func(r rune) bool { return r == ' ' || r == '\t' })
			if kind == "network" {
				if err := parseNetMember(svc.net, mf, i+1); err != nil {
					return nil, nil, err
				}
			} else {
				if err := parseSvcMember(svc.svc, mf, i+1); err != nil {
					return nil, nil, err
				}
			}
			rest[i] = ""
			i++
		}
		switch kind {
		case "network":
			if _, dup := groups.network[name]; dup {
				return nil, nil, fail("PARSE_GROUP_DUPLICATE", headerLine, "duplicate network object-group")
			}
			if len(svc.net.members) == 0 {
				return nil, nil, fail("PARSE_GROUP_EMPTY", headerLine, "network object-group has no members")
			}
			groups.network[name] = svc.net
		case "service":
			if _, dup := groups.service[name]; dup {
				return nil, nil, fail("PARSE_GROUP_DUPLICATE", headerLine, "duplicate service object-group")
			}
			if len(svc.svc.members) == 0 {
				return nil, nil, fail("PARSE_GROUP_EMPTY", headerLine, "service object-group has no members")
			}
			groups.service[name] = svc.svc
		}
	}
	return groups, rest, nil
}

type groupSink struct {
	net *networkGroup
	svc *serviceGroup
}

func parseGroupHeader(f []string, line int) (string, string, groupSink, error) {
	bad := func(msg string) (string, string, groupSink, error) {
		return "", "", groupSink{}, fail("PARSE_GROUP_HEADER", line, msg)
	}
	if len(f) < 3 {
		return bad("object-group header is incomplete")
	}
	kind := strings.ToLower(f[1])
	name := f[2]
	if !groupNameOK(name) {
		return bad("invalid object-group name")
	}
	switch kind {
	case "network":
		if len(f) != 3 {
			return bad("network object-group header takes no extra tokens")
		}
		return "network", name, groupSink{net: &networkGroup{name: name, line: line}}, nil
	case "service":
		if len(f) != 4 {
			return bad("service object-group header requires a protocol")
		}
		g := &serviceGroup{name: name, line: line}
		switch strings.ToLower(f[3]) {
		case "tcp":
			g.tcp = true
		case "udp":
			g.udp = true
		case "tcp-udp":
			g.tcp, g.udp = true, true
		default:
			return bad("service object-group protocol must be tcp, udp, or tcp-udp")
		}
		return "service", name, groupSink{svc: g}, nil
	}
	return bad("unsupported object-group type (protocol groups are out of scope)")
}

func parseNetMember(g *networkGroup, f []string, line int) error {
	bad := func() error { return fail("PARSE_GROUP_MEMBER", line, "invalid network object-group member") }
	if len(f) == 0 {
		return bad()
	}
	if strings.EqualFold(f[0], "group-object") {
		if len(f) != 2 || !groupNameOK(f[1]) {
			return bad()
		}
		g.members = append(g.members, netMember{group: f[1], line: line})
		return nil
	}
	if strings.EqualFold(f[0], "host") {
		if len(f) != 2 {
			return bad()
		}
		addr, err := netip.ParseAddr(f[1])
		if err != nil || !addr.Is4() {
			return bad()
		}
		g.members = append(g.members, netMember{addr: Address{Value: ipv4(addr)}, line: line})
		return nil
	}
	if len(f) != 2 {
		return bad()
	}
	addr, err := netip.ParseAddr(f[0])
	if err != nil || !addr.Is4() {
		return bad()
	}
	mask, err := netip.ParseAddr(f[1])
	if err != nil || !mask.Is4() {
		return bad()
	}
	// Network group members carry a subnet mask, not a wildcard mask.
	m := ipv4(mask)
	g.members = append(g.members, netMember{addr: Address{Value: ipv4(addr) & m, Wildcard: ^m}, line: line})
	return nil
}

func parseSvcMember(g *serviceGroup, f []string, line int) error {
	bad := func() error { return fail("PARSE_GROUP_MEMBER", line, "invalid service object-group member") }
	if len(f) == 0 {
		return bad()
	}
	if strings.EqualFold(f[0], "group-object") {
		if len(f) != 2 || !groupNameOK(f[1]) {
			return bad()
		}
		g.members = append(g.members, svcMember{group: f[1], line: line})
		return nil
	}
	if !strings.EqualFold(f[0], "port-object") || len(f) < 3 {
		return bad()
	}
	ivs, consumed, err := parsePorts(f, 1, line)
	if err != nil || consumed != len(f) {
		return bad()
	}
	g.members = append(g.members, svcMember{intervals: ivs, line: line})
	return nil
}

func parseRuleV4(f []string, line int, groups *groupTable) (Rule, error) {
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
	if proto != "ip" {
		var p uint8
		switch proto {
		case "tcp":
			p = 6
		case "udp":
			p = 17
		case "icmp":
			p = 1
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
	if pos, err = parseAddrSpec(f, pos, line, groups, &r.Src, &r.SrcSet); err != nil {
		return Rule{}, err
	}
	if pos, err = parseServiceSpec(f, pos, line, groups, r.Protocol, &r.SrcPorts); err != nil {
		return Rule{}, err
	}
	if pos, err = parseAddrSpec(f, pos, line, groups, &r.Dst, &r.DstSet); err != nil {
		return Rule{}, err
	}
	if pos, err = parseServiceSpec(f, pos, line, groups, r.Protocol, &r.DstPorts); err != nil {
		return Rule{}, err
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

// parseAddrSpec parses one address position: a plain address (as
// parseAddress) or "object-group NAME" resolving to a member set.
func parseAddrSpec(f []string, pos, line int, groups *groupTable, single *Address, set *[]Address) (int, error) {
	if pos < len(f) && strings.EqualFold(f[pos], "object-group") {
		if pos+1 >= len(f) || !groupNameOK(f[pos+1]) {
			return pos, fail("PARSE_GROUP_MEMBER", line, "object-group reference is incomplete")
		}
		members, err := groups.resolveNetwork(f[pos+1], line)
		if err != nil {
			return pos, err
		}
		*set = members
		return pos + 2, nil
	}
	addr, next, err := parseAddress(f, pos, line)
	if err != nil {
		return pos, err
	}
	*single = addr
	return next, nil
}

// parseServiceSpec parses one optional port position: a port operator
// (as parsePorts) or "object-group NAME" resolving to a port set. When
// neither is present the position is left at the full-range default.
func parseServiceSpec(f []string, pos, line int, groups *groupTable, proto *uint8, ports *[]Interval) (int, error) {
	if pos >= len(f) {
		return pos, nil
	}
	if strings.EqualFold(f[pos], "object-group") {
		if pos+1 >= len(f) || !groupNameOK(f[pos+1]) {
			return pos, fail("PARSE_GROUP_MEMBER", line, "object-group reference is incomplete")
		}
		// Disambiguation (as in IOS): an object-group token in a port
		// position is a service reference only when the name resolves to a
		// service group; a network group of that name belongs to the
		// following address position and is left for parseAddrSpec.
		if _, isService := groups.service[f[pos+1]]; !isService {
			if _, isNetwork := groups.network[f[pos+1]]; isNetwork {
				return pos, nil
			}
		}
		if proto == nil || *proto != 6 && *proto != 17 {
			return pos, fail("PARSE_PORT_NOT_ALLOWED", line, "service object-group requires TCP or UDP")
		}
		ivs, err := groups.resolveService(f[pos+1], *proto, line)
		if err != nil {
			return pos, err
		}
		*ports = ivs
		return pos + 2, nil
	}
	if isPortOp(f[pos]) {
		if proto == nil || *proto != 6 && *proto != 17 {
			return pos, fail("PARSE_PORT_NOT_ALLOWED", line, "port constraint requires TCP or UDP")
		}
		ivs, next, err := parsePorts(f, pos, line)
		if err != nil {
			return pos, err
		}
		*ports = ivs
		return next, nil
	}
	return pos, nil
}

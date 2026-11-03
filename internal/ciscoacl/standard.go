package ciscoacl

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxStandardSequence uint32 = 2147483647

func SupportedParser(version string) bool {
	return version == ParserVersion || version == LegacyParserVersion
}

func SupportedKind(kind, parser string) bool {
	return SupportedParser(parser) && (kind == "named-extended" || parser == ParserVersion && kind == "standard")
}

// Parse accepts one ACL-only document. Standard ACLs use the IOS router profile:
// exact source hosts precede wildcard networks; networks follow sequence order.
// Rule.Ordinal is the evaluation index; Rule.Line is the physical source line.
// References: Cisco IOS XE16.12 sec-acl-seq-num, Sequence Numbering Behavior:
// https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/sec_data_acl/configuration/xe-16-12/sec-data-acl-xe-16-12-book/sec-acl-seq-num.html
// Numbered ranges and source syntax: Cisco Configure IP Access Lists, ID23602:
// https://www.cisco.com/c/en/us/support/docs/security/ios-firewall/23602-confaccesslists.html
func Parse(input []byte) (ACL, error) { return ParseVersion(input, ParserVersion) }

func ParseVersion(input []byte, version string) (ACL, error) {
	if !SupportedParser(version) {
		return ACL{}, fail("PARSE_UNSUPPORTED", 0, "unsupported ACL parser version")
	}
	if len(input) > MaxInputBytes {
		return ACL{}, fail("INPUT_TOO_LARGE", 0, "ACL exceeds byte limit")
	}
	if version == LegacyParserVersion {
		return parseLegacy(input)
	}
	// Dispatch from the first meaningful line only. The selected parser validates
	// every line, including unsupported tails and control characters.
	for _, line := range strings.Split(string(input), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || len(f) == 1 && f[0] == "!" {
			continue
		}
		if strings.EqualFold(f[0], "access-list") || len(f) >= 3 && strings.EqualFold(f[0], "ip") && strings.EqualFold(f[1], "access-list") && strings.EqualFold(f[2], "standard") {
			return parseStandard(input)
		}
		break
	}
	return parseLegacy(input)
}

func validStandardName(name string) bool {
	if !aclName.MatchString(name) {
		return false
	}
	if !decimal(name) {
		return true
	}
	n, err := strconv.ParseUint(name, 10, 32)
	return err == nil && strconv.FormatUint(n, 10) == name && (n >= 1 && n <= 99 || n >= 1300 && n <= 1999)
}

func parseStandard(input []byte) (ACL, error) {
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
	a := ACL{Kind: "standard", Rules: []Rule{}, Warnings: []string{}}
	seen, numbered, closed := false, false, false
	sequences := map[uint32]bool{}
	hosts := map[uint32]bool{}
	var highest uint32
	for i, line := range strings.Split(text, "\n") {
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
		if closed {
			return zero, fail("PARSE_DOCUMENT", n, "content follows closed ACL")
		}
		if len(f) >= 2 && strings.EqualFold(f[0], "ip") && strings.EqualFold(f[1], "access-list") {
			if seen {
				return zero, fail("PARSE_MULTIPLE_ACLS", n, "exactly one ACL is required")
			}
			if indented || len(f) != 4 || !strings.EqualFold(f[2], "standard") || !validStandardName(f[3]) {
				return zero, fail("PARSE_HEADER", n, "expected supported standard ACL header")
			}
			a.Name, seen = f[3], true
			continue
		}
		if strings.EqualFold(f[0], "access-list") {
			if indented || len(f) < 3 || !decimal(f[1]) || !validStandardName(f[1]) {
				return zero, fail("PARSE_HEADER", n, "expected standard numbered ACL")
			}
			if seen && (!numbered || f[1] != a.Name) {
				return zero, fail("PARSE_MULTIPLE_ACLS", n, "exactly one ACL is required")
			}
			a.Name, seen, numbered = f[1], true, true
			f = f[2:]
		} else if !seen || numbered || !indented {
			return zero, fail("PARSE_DOCUMENT", n, "only one standard ACL is accepted")
		}
		var seq uint32
		if !numbered && decimal(f[0]) {
			value, err := strconv.ParseUint(f[0], 10, 32)
			if err != nil || value == 0 || value > uint64(maxStandardSequence) {
				return zero, fail("PARSE_SEQUENCE_ORDER", n, "invalid standard sequence")
			}
			seq, f = uint32(value), f[1:]
		} else {
			if highest > maxStandardSequence-10 {
				return zero, fail("PARSE_SEQUENCE_ORDER", n, "implicit standard sequence overflows")
			}
			seq = highest + 10
		}
		if sequences[seq] {
			return zero, fail("PARSE_SEQUENCE_ORDER", n, "duplicate standard sequence")
		}
		sequences[seq] = true
		if seq > highest {
			highest = seq
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
		r, err := parseStandardRule(f, n)
		if err != nil {
			return zero, err
		}
		if r.Src.Wildcard == 0 {
			if hosts[r.Src.Value] {
				return zero, fail("PARSE_UNSUPPORTED", n, "duplicate exact-host definitions are not supported")
			}
			hosts[r.Src.Value] = true
		}
		value := seq
		// Standard sequence is effective metadata, not evidence that the source
		// carried a numeric token. Implicit entries (including remarks) reserve
		// the next highest sequence plus 10; remarks are not executable rules.
		r.Sequence = &value
		a.Rules = append(a.Rules, r)
	}
	if !seen {
		return zero, fail("PARSE_NOT_FOUND", 0, "ACL header is missing")
	}
	if len(a.Rules) == 0 {
		return zero, fail("PARSE_EMPTY_ACL", 0, "ACL must contain at least one executable rule")
	}
	// Hosts are disjoint, so their source order is deterministic without modeling
	// Cisco's hash-bucket display order. Network precedence is sequence based.
	sort.SliceStable(a.Rules, func(i, j int) bool {
		x, y := a.Rules[i], a.Rules[j]
		if (x.Src.Wildcard == 0) != (y.Src.Wildcard == 0) {
			return x.Src.Wildcard == 0
		}
		if x.Src.Wildcard == 0 {
			return false
		}
		return *x.Sequence < *y.Sequence
	})
	for i := range a.Rules {
		a.Rules[i].Ordinal = i
	}
	return a, nil
}

func parseStandardRule(f []string, line int) (Rule, error) {
	r := Rule{Line: line, SrcPorts: []Interval{{0, 65535}}, DstPorts: []Interval{{0, 65535}}, Dst: Address{Wildcard: ^uint32(0)}}
	if len(f) < 2 {
		return Rule{}, fail("PARSE_INVALID_TOKEN", line, "standard rule is incomplete")
	}
	r.Action = strings.ToLower(f[0])
	if r.Action != "permit" && r.Action != "deny" {
		return Rule{}, fail("PARSE_UNSUPPORTED", line, "unsupported standard rule action")
	}
	pos := 1
	if strings.EqualFold(f[pos], "any") {
		r.Src, pos = Address{Wildcard: ^uint32(0)}, pos+1
	} else if strings.EqualFold(f[pos], "host") {
		var err error
		r.Src, pos, err = parseAddress(f, pos, line)
		if err != nil {
			return Rule{}, err
		}
	} else {
		addr, err := netip.ParseAddr(f[pos])
		if err != nil || !addr.Is4() {
			return Rule{}, fail("PARSE_INVALID_IP", line, "expected IPv4 source")
		}
		r.Src, pos = Address{Value: ipv4(addr)}, pos+1
		if pos < len(f) && f[pos] != "log" && f[pos] != "log-input" {
			if mask, err := netip.ParseAddr(f[pos]); err == nil && mask.Is4() {
				r.Src.Wildcard = ipv4(mask)
				r.Src.Value &= ^r.Src.Wildcard
				pos++
			}
		}
	}
	if pos < len(f) {
		option := strings.ToLower(f[pos])
		if pos != len(f)-1 || option != "log" && option != "log-input" {
			return Rule{}, fail("PARSE_UNSUPPORTED", line, "unsupported standard rule option or trailing token")
		}
		r.Logging = option
	}
	return r, nil
}

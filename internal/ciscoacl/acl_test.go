package ciscoacl

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"testing"
)

func packet(src, dst string, sport, dport uint16) Packet {
	return Packet{Protocol: 6, SrcIP: netip.MustParseAddr(src), DstIP: netip.MustParseAddr(dst), SrcPort: sport, DstPort: dport}
}

func mustParse(t *testing.T, text string) ACL {
	t.Helper()
	a, err := Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestFirstMatchTraceAndImplicitDeny(t *testing.T) {
	a := mustParse(t, "ip access-list extended MGMT\n 10 remark management only\n 20 deny tcp host 192.0.2.10 any eq 22 log-input\n 30 permit tcp 192.0.2.0 0.0.0.255 any eq 22\n!\nend\n")
	for _, tt := range []struct {
		src           string
		port          uint16
		action        string
		line, ordinal int
		implicit      bool
	}{
		{"192.0.2.10", 22, "deny", 3, 0, false},
		{"192.0.2.11", 22, "permit", 4, 1, false},
		{"192.0.3.11", 22, "deny", 0, 0, true},
		{"192.0.2.11", 443, "deny", 0, 0, true},
	} {
		d, err := a.Evaluate(packet(tt.src, "198.51.100.1", 40000, tt.port))
		if err != nil {
			t.Fatal(err)
		}
		if d.Action != tt.action || d.Implicit != tt.implicit {
			t.Fatalf("%+v: %+v", tt, d)
		}
		if tt.implicit {
			if d.Line != nil || d.RuleOrdinal != nil {
				t.Fatalf("implicit deny has explicit trace: %+v", d)
			}
		} else if d.Line == nil || *d.Line != tt.line || d.RuleOrdinal == nil || *d.RuleOrdinal != tt.ordinal {
			t.Fatalf("trace: %+v", d)
		}
	}
	if a.Rules[0].Logging != "log-input" {
		t.Fatal("logging metadata lost")
	}
}

func TestNoncontiguousWildcardAgainstDirectMask(t *testing.T) {
	// Exhaust every 8-bit mask/address pair, using a direct byte-level oracle.
	// Higher octets are fixed and the low octet is intentionally not CIDR-only.
	for wildcard := 0; wildcard < 256; wildcard++ {
		a := mustParse(t, fmt.Sprintf("ip access-list extended X\n permit ip 10.0.0.85 0.0.0.%d any\n", wildcard))
		for value := 0; value < 256; value++ {
			p := packet(fmt.Sprintf("10.0.0.%d", value), "198.51.100.1", 0, 0)
			p.Protocol = 47
			d, err := a.Evaluate(p)
			if err != nil {
				t.Fatal(err)
			}
			want := ((byte(value) ^ byte(85)) & ^byte(wildcard)) == 0
			if (d.Action == "permit") != want {
				t.Fatalf("mask=%d value=%d got=%+v", wildcard, value, d)
			}
		}
	}
	// Do not normalize away bits that must remain fixed in another octet.
	a := mustParse(t, "ip access-list extended X\n permit ip 10.0.0.0 0.255.0.255 any\n")
	d, err := a.Evaluate(packet("10.8.1.9", "198.51.100.1", 0, 0))
	if err != nil || d.Action != "deny" {
		t.Fatalf("cross-octet match: %+v %v", d, err)
	}
}

func TestPortOperatorsExhaustive(t *testing.T) {
	for _, tt := range []struct {
		op      string
		matches func(uint16) bool
	}{
		{"eq 22", func(p uint16) bool { return p == 22 }},
		{"neq 22", func(p uint16) bool { return p != 22 }},
		{"lt 22", func(p uint16) bool { return p < 22 }},
		{"gt 22", func(p uint16) bool { return p > 22 }},
		{"range 20 24", func(p uint16) bool { return p >= 20 && p <= 24 }},
		{"neq 0", func(p uint16) bool { return p != 0 }},
		{"neq 65535", func(p uint16) bool { return p != 65535 }},
	} {
		a := mustParse(t, "ip access-list extended X\n permit tcp any any "+tt.op+"\n")
		for value := 0; value <= 65535; value++ {
			d, err := a.Evaluate(packet("192.0.2.1", "198.51.100.1", 40000, uint16(value)))
			if err != nil || (d.Action == "permit") != tt.matches(uint16(value)) {
				t.Fatalf("%s port=%d: %+v %v", tt.op, value, d, err)
			}
		}
	}
}

func TestSourcePortAndProtocol(t *testing.T) {
	a := mustParse(t, "ip access-list extended X\n permit 6 any eq 1024 any range 80 90\n permit udp any any eq 53\n permit 0 host 192.0.2.1 host 198.51.100.1\n")
	for _, tt := range []struct {
		proto    uint8
		src, dst uint16
		want     string
	}{
		{6, 1024, 80, "permit"}, {6, 1025, 80, "deny"}, {6, 1024, 91, "deny"},
		{17, 1024, 53, "permit"}, {6, 1024, 53, "deny"}, {0, 0, 0, "permit"}, {255, 0, 0, "deny"},
	} {
		p := packet("192.0.2.1", "198.51.100.1", tt.src, tt.dst)
		p.Protocol = tt.proto
		d, err := a.Evaluate(p)
		if err != nil || d.Action != tt.want {
			t.Fatalf("%+v: %+v %v", tt, d, err)
		}
	}
}

func TestRejectUnsupportedAndMalformedDocuments(t *testing.T) {
	for _, tt := range []struct{ name, input, code string }{
		{"missing", "", "PARSE_NOT_FOUND"},
		{"empty ACL", "ip access-list extended X\n remark no executable rules\n", "PARSE_EMPTY_ACL"},
		{"full config", "hostname router\nip access-list extended X\n permit ip any any\n", "PARSE_DOCUMENT"},
		{"opaque fake ACL", "banner motd ^C\nip access-list extended X\n permit ip any any\n^C\n", "PARSE_DOCUMENT"},
		{"two blocks", "ip access-list extended X\n permit ip any any\nip access-list extended Y\n deny ip any any\n", "PARSE_MULTIPLE_ACLS"},
		{"standard", "ip access-list standard X\n permit any\n", "PARSE_HEADER"},
		{"unindented", "ip access-list extended X\npermit ip any any\n", "PARSE_DOCUMENT"},
		{"after close", "ip access-list extended X\n!\n permit ip any any\n", "PARSE_DOCUMENT"},
		{"incomplete", "ip access-list extended X\n permit tcp any\n", "PARSE_INVALID_TOKEN"},
		{"trailing", "ip access-list extended X\n permit tcp any any eq 22 established\n", "PARSE_UNSUPPORTED"},
		{"log duplicate", "ip access-list extended X\n permit ip any any log log-input\n", "PARSE_UNSUPPORTED"},
		{"port name", "ip access-list extended X\n permit tcp any any eq ssh\n", "PARSE_INVALID_PORT"},
		{"ipv6", "ip access-list extended X\n permit tcp host ::1 any\n", "PARSE_INVALID_IP"},
		{"leading zero IP", "ip access-list extended X\n permit ip host 010.0.0.1 any\n", "PARSE_INVALID_IP"},
		{"bad wildcard", "ip access-list extended X\n permit ip 10.0.0.0 0.0.0.256 any\n", "PARSE_INVALID_IP"},
		{"port on ip", "ip access-list extended X\n permit ip any any eq 22\n", "PARSE_PORT_NOT_ALLOWED"},
		{"lt zero", "ip access-list extended X\n permit tcp any any lt 0\n", "PARSE_INVALID_PORT"},
		{"gt max", "ip access-list extended X\n permit tcp any any gt 65535\n", "PARSE_INVALID_PORT"},
		{"range reverse", "ip access-list extended X\n permit tcp any any range 90 80\n", "PARSE_INVALID_PORT"},
		{"negative", "ip access-list extended X\n permit tcp any any eq -1\n", "PARSE_INVALID_PORT"},
		{"overflow", "ip access-list extended X\n permit tcp any any eq 65536\n", "PARSE_INVALID_PORT"},
		{"sequence duplicate", "ip access-list extended X\n 10 permit ip any any\n 10 deny ip any any\n", "PARSE_SEQUENCE_ORDER"},
		{"seq zero", "ip access-list extended X\n 0 permit ip any any\n", "PARSE_SEQUENCE_ORDER"},
		{"seq alone", "ip access-list extended X\n 10\n", "PARSE_INVALID_TOKEN"},
		{"protocol name", "ip access-list extended X\n permit gre any any\n", "PARSE_UNSUPPORTED"},
		{"protocol overflow", "ip access-list extended X\n permit 256 any any\n", "PARSE_UNSUPPORTED"},
		{"NUL", "ip access-list extended X\n\x00", "PARSE_ENCODING"},
		{"C1 control", "ip access-list extended X\n remark \u0085\n", "PARSE_ENCODING"},
		{"unicode separator", "ip\u00a0access-list extended X\n permit ip any any\n", "PARSE_DOCUMENT"},
		{"UTF8", string([]byte{0xff}), "PARSE_ENCODING"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, err := Parse([]byte(tt.input))
			var typed *Error
			if !errors.As(err, &typed) || typed.Code != tt.code {
				t.Fatalf("want %s, got %v", tt.code, err)
			}
			if !reflect.DeepEqual(a, ACL{}) {
				t.Fatal("error returned partial ACL")
			}
		})
	}
}

func TestLimitsAndMixedSequences(t *testing.T) {
	for _, text := range []string{
		strings.Repeat(" ", MaxInputBytes+1),
		"ip access-list extended X\n" + strings.Repeat(" permit ip any any\n", MaxRules+1),
		strings.Repeat("\n", MaxLines+1),
		"ip access-list extended X\n remark " + strings.Repeat("x", MaxLineBytes),
	} {
		if _, err := Parse([]byte(text)); err == nil {
			t.Fatal("limit accepted")
		}
	}
	a := mustParse(t, "!\r\nip access-list extended X\r\n 10 permit ip any any\r\n deny ip any any\r\nend\r\n")
	if !reflect.DeepEqual(a.Warnings, []string{"sequence_partial"}) {
		t.Fatalf("warnings: %v", a.Warnings)
	}
}

func TestLogicalLineLimitWithAndWithoutTerminator(t *testing.T) {
	input := "ip access-list extended X\n" + strings.Repeat(" remark padding\n", MaxLines-2) + " permit ip any any\n"
	for _, text := range []string{input, strings.TrimSuffix(input, "\n"), strings.ReplaceAll(input, "\n", "\r\n")} {
		a := mustParse(t, text)
		if a.Rules[0].Line != MaxLines {
			t.Fatalf("last executable line = %d, want %d", a.Rules[0].Line, MaxLines)
		}
	}
	for _, text := range []string{input + "\n", input + " remark extra"} {
		_, err := Parse([]byte(text))
		var typed *Error
		if !errors.As(err, &typed) || typed.Code != "PARSE_LIMIT" {
			t.Fatalf("expected line-limit error, got %v", err)
		}
	}
}

func TestSequenceWarningUsesExecutableRulesOnly(t *testing.T) {
	for _, tt := range []struct {
		name, entries string
		partial       bool
	}{
		{"unsequenced remark", " remark note\n 10 permit ip any any\n", false},
		{"sequenced remark", " 10 remark note\n permit ip any any\n", false},
		{"mixed remarks", " 10 remark note\n remark another note\n 20 permit ip any any\n", false},
		{"mixed executable rules", " 10 remark note\n 20 permit tcp any any eq 22\n deny ip any any\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := mustParse(t, "ip access-list extended X\n"+tt.entries)
			want := []string{}
			if tt.partial {
				want = []string{"sequence_partial"}
			}
			if !reflect.DeepEqual(a.Warnings, want) {
				t.Fatalf("warnings: %v, want %v", a.Warnings, want)
			}
		})
	}
	// Remarks still participate in monotonic sequence validation.
	for _, entries := range []string{
		" 20 remark note\n 10 permit ip any any\n",
		" 10 permit ip any any\n 10 remark duplicate sequence\n",
	} {
		_, err := Parse([]byte("ip access-list extended X\n" + entries))
		var typed *Error
		if !errors.As(err, &typed) || typed.Code != "PARSE_SEQUENCE_ORDER" {
			t.Fatalf("expected sequence-order error, got %v", err)
		}
	}
}

func TestICMPNameMatchesNumericProtocol(t *testing.T) {
	named := mustParse(t, "ip access-list extended X\n permit icmp any any\n")
	numeric := mustParse(t, "ip access-list extended X\n permit 1 any any\n")
	if !reflect.DeepEqual(named, numeric) {
		t.Fatal("icmp alias changed normalized rules")
	}
	p := packet("192.0.2.1", "198.51.100.1", 0, 0)
	p.Protocol = 1
	d, err := named.Evaluate(p)
	if err != nil || d.Action != "permit" {
		t.Fatalf("ICMP evaluation: %+v %v", d, err)
	}
	for _, entry := range []string{"permit icmp any any echo", "permit icmp any any eq 22"} {
		if _, err := Parse([]byte("ip access-list extended X\n " + entry + "\n")); err == nil {
			t.Fatal("unsupported ICMP constraint accepted")
		}
	}
}

func TestDemoFixtureActuallyDemonstratesFirstMatch(t *testing.T) {
	config, err := os.ReadFile("../../testdata/acl/management.acl")
	if err != nil {
		t.Fatal(err)
	}
	packetJSON, err := os.ReadFile("../../testdata/packet.json")
	if err != nil {
		t.Fatal(err)
	}
	var p Packet
	if err := json.Unmarshal(packetJSON, &p); err != nil {
		t.Fatal(err)
	}
	a, err := Parse(config)
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Evaluate(p)
	if err != nil || d.Action != "deny" || d.Implicit || d.RuleOrdinal == nil || *d.RuleOrdinal != 0 {
		t.Fatalf("fixture should hit leading SSH deny: %+v %v", d, err)
	}
	line := " 20 deny tcp host 192.0.2.10 any eq 22 log\n"
	normalized := strings.ReplaceAll(string(config), "\r\n", "\n")
	if strings.Count(normalized, line) != 1 {
		t.Fatal("expected one targeted deny in fixture")
	}
	withoutDeny, err := Parse([]byte(strings.Replace(normalized, line, "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	d, err = withoutDeny.Evaluate(p)
	if err != nil || d.Action != "permit" || d.Implicit {
		t.Fatalf("removing leading deny must expose broader permit: %+v %v", d, err)
	}
}

func TestInvalidPacketAndUnreachableIRAreRejected(t *testing.T) {
	if _, err := (ACL{Name: "X", Kind: "named-extended"}).Evaluate(packet("192.0.2.1", "198.51.100.1", 0, 0)); err == nil {
		t.Fatal("empty constructed ACL accepted")
	}
	a := mustParse(t, "ip access-list extended X\n permit ip any any\n deny tcp any any eq 22\n")
	for _, p := range []Packet{
		{}, {Protocol: 6, SrcIP: netip.MustParseAddr("::ffff:192.0.2.1"), DstIP: netip.MustParseAddr("198.51.100.1")},
		{Protocol: 1, SrcIP: netip.MustParseAddr("192.0.2.1"), DstIP: netip.MustParseAddr("198.51.100.1"), DstPort: 22},
	} {
		if _, err := a.Evaluate(p); err == nil {
			t.Fatal("invalid packet accepted")
		}
	}
	a.Rules[1].Action = "unsupported"
	if _, err := a.Evaluate(packet("192.0.2.1", "198.51.100.1", 40000, 22)); err == nil {
		t.Fatal("invalid unreachable rule accepted")
	}
}

func TestInvalidSourceBindingsAreRejectedBeforeDecision(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*ACL)
	}{
		{"first ordinal", func(a *ACL) { a.Rules[0].Ordinal = 1 }},
		{"duplicate ordinal", func(a *ACL) { a.Rules[1].Ordinal = 0 }},
		{"negative ordinal", func(a *ACL) { a.Rules[1].Ordinal = -1 }},
		{"missing line", func(a *ACL) { a.Rules[1].Line = 0 }},
		{"negative line", func(a *ACL) { a.Rules[1].Line = -1 }},
		{"duplicate line", func(a *ACL) { a.Rules[1].Line = a.Rules[0].Line }},
		{"decreasing line", func(a *ACL) { a.Rules[1].Line = 1 }},
		{"line exceeds input limit", func(a *ACL) { a.Rules[1].Line = MaxLines + 1 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := mustParse(t, "ip access-list extended X\n permit ip any any\n deny tcp any any eq 22\n")
			tt.mutate(&a)
			d, err := a.Evaluate(packet("192.0.2.1", "198.51.100.1", 40000, 22))
			var typed *Error
			if !errors.As(err, &typed) || typed.Code != "ACL_INVALID" || d != (Decision{}) {
				t.Fatalf("invalid source binding produced a decision: %+v %v", d, err)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, text := range []string{"ip access-list extended X\n permit tcp any any eq 22\n", "ip access-list extended X\n 10 deny ip any any\n", "banner motd ^C\n"} {
		f.Add([]byte(text))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		a, err := Parse(data)
		if err != nil {
			if !reflect.DeepEqual(a, ACL{}) {
				t.Fatal("partial ACL")
			}
			return
		}
		if _, err := a.Evaluate(packet("192.0.2.1", "198.51.100.1", 40000, 22)); err != nil {
			t.Fatalf("parser emitted invalid IR: %v", err)
		}
	})
}

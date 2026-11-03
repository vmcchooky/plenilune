package ciscoacl

import (
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"testing"
)

// P3-5 corpus is registered before parser implementation. These expectations
// pin the IOS router host-hash-first profile, not a universal Cisco platform.
func TestStandardCorpus(t *testing.T) {
	var cases []struct {
		Name, Source, Identity, Error string
		Lines                         []int
		Sequences                     []uint32
	}
	raw, err := os.ReadFile("testdata/standard-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			a, err := Parse([]byte(tt.Source))
			if tt.Error != "" {
				var parseErr *Error
				if !errors.As(err, &parseErr) || parseErr.Code != tt.Error {
					t.Fatalf("want %s, got %v", tt.Error, err)
				}
				if !reflect.DeepEqual(a, ACL{}) {
					t.Fatal("failed parse returned partial ACL")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if a.Kind != "standard" || a.Name != tt.Identity || len(a.Rules) != len(tt.Lines) {
				t.Fatalf("identity/count: %+v", a)
			}
			for i, r := range a.Rules {
				if r.Ordinal != i || r.Line != tt.Lines[i] || r.Sequence == nil || *r.Sequence != tt.Sequences[i] {
					t.Fatalf("rule %d source/effective binding: %+v", i, r)
				}
				if r.Protocol != nil || r.Dst != (Address{Wildcard: ^uint32(0)}) || !fullPorts(r.SrcPorts) || !fullPorts(r.DstPorts) {
					t.Fatalf("standard rule constrains another field: %+v", r)
				}
			}
			if _, err := a.Evaluate(packet("192.0.2.7", "198.51.100.3", 0, 0)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParserVersionDispatch(t *testing.T) {
	for _, input := range []string{
		"ip access-list extended X\n permit ip any any\n",
		"ip access-list extended X\n 10 remark first\n 20 permit tcp host 192.0.2.7 any eq 22 log\n deny ip any any\n!\nend\n",
		"ip access-list extended X\r\n permit icmp any any\r\n",
	} {
		old, oldErr := ParseVersion([]byte(input), LegacyParserVersion)
		current, err := ParseVersion([]byte(input), ParserVersion)
		if oldErr != nil || err != nil || !reflect.DeepEqual(old, current) {
			t.Fatalf("extended DTO changed: old=%+v current=%+v errors=%v/%v", old, current, oldErr, err)
		}
	}
	if ParserVersion != "cisco-acl/v0.3" || LegacyParserVersion != "cisco-acl/v0.2" {
		t.Fatal("unexpected parser identity")
	}
	for _, version := range []string{ParserVersion, LegacyParserVersion, "unknown", ""} {
		if SupportedParser(version) != (version == ParserVersion || version == LegacyParserVersion) {
			t.Fatalf("parser support: %q", version)
		}
		for _, kind := range []string{"standard", "named-extended", "numbered", "unknown", ""} {
			want := (version == ParserVersion || version == LegacyParserVersion) && (kind == "named-extended" || version == ParserVersion && kind == "standard")
			if SupportedKind(kind, version) != want {
				t.Fatalf("kind support: %q/%q", kind, version)
			}
		}
	}
	for _, input := range []string{"ip access-list standard X\n permit any\n", "access-list 1 permit any\n"} {
		if _, err := ParseVersion([]byte(input), LegacyParserVersion); err == nil {
			t.Fatal("legacy accepted standard syntax")
		}
		if _, err := ParseVersion([]byte(input), "unknown"); err == nil {
			t.Fatal("unknown version accepted syntax")
		}
	}
}

func TestStandardBoundsMasksAndIRValidation(t *testing.T) {
	for _, name := range []string{"1", "99", "1300", "1999", "NAME"} {
		input := "ip access-list standard " + name + "\r\n permit 192.0.2.85 0.0.0.170\r\n"
		a := mustParse(t, input)
		if a.Rules[0].Src != (Address{Value: 0xc0000255, Wildcard: 0xaa}) {
			t.Fatalf("wildcard normalization: %+v", a.Rules[0].Src)
		}
	}
	for _, name := range []string{"0", "100", "1299", "2000", "01", "4294967296"} {
		if _, err := Parse([]byte("ip access-list standard " + name + "\n permit any\n")); err == nil {
			t.Fatalf("accepted invalid numeric name %s", name)
		}
	}
	for _, input := range []string{
		"ip access-list standard X\n" + strings.Repeat(" permit any\n", MaxRules+1),
		"ip access-list standard X\n" + strings.Repeat("\n", MaxLines),
		"ip access-list standard X\n remark " + strings.Repeat("x", MaxLineBytes),
		"access-list 1 permit any\n" + string([]byte{0xff}),
		"access-list 1 permit any\n remark \u0085\n",
		strings.Repeat("x", MaxInputBytes+1),
	} {
		if a, err := Parse([]byte(input)); err == nil || !reflect.DeepEqual(a, ACL{}) {
			t.Fatalf("invalid bounded source returned ACL: %+v %v", a, err)
		}
	}
	valid := "ip access-list standard X\n deny 192.0.2.0 0.0.0.255\n permit 192.0.2.7\n"
	for _, mutate := range []func(*ACL){
		func(a *ACL) { a.Rules[0].Line = a.Rules[1].Line },
		func(a *ACL) { a.Rules[0].Ordinal = 1 },
		func(a *ACL) {
			a.Rules[0], a.Rules[1] = a.Rules[1], a.Rules[0]
			a.Rules[0].Ordinal, a.Rules[1].Ordinal = 0, 1
		},
		func(a *ACL) { p := uint8(6); a.Rules[0].Protocol = &p },
		func(a *ACL) { a.Rules[0].Dst = Address{} },
		func(a *ACL) { a.Rules[0].SrcPorts = []Interval{{22, 22}} },
		func(a *ACL) { a.Rules[0].Sequence = nil },
		func(a *ACL) { n := uint32(2147483648); a.Rules[0].Sequence = &n },
		func(a *ACL) { a.Rules[0].Sequence = a.Rules[1].Sequence },
		func(a *ACL) { a.Rules[0].Logging = "unsupported" },
	} {
		a := mustParse(t, valid)
		mutate(&a)
		if _, err := a.Evaluate(packet("192.0.2.7", "198.51.100.1", 0, 0)); err == nil {
			t.Fatalf("accepted malformed IR: %+v", a)
		}
	}
	a := mustParse(t, "ip access-list standard X\n 10 permit any\n 20 deny 192.0.2.0 0.0.0.255\n")
	a.Rules[0].Sequence, a.Rules[1].Sequence = a.Rules[1].Sequence, a.Rules[0].Sequence
	if _, err := a.Evaluate(packet("192.0.2.7", "198.51.100.1", 0, 0)); err == nil {
		t.Fatal("accepted noncanonical network sequence order")
	}
}

func TestStandardHostPriorityAndImplicitDeny(t *testing.T) {
	a := mustParse(t, "ip access-list standard X\n 10 deny 192.0.2.0 0.0.0.255\n 20 permit 192.0.2.7\n")
	for _, proto := range []uint8{0, 1, 6, 17, 47, 255} {
		for _, source := range []string{"192.0.2.7", "192.0.2.8", "203.0.113.1"} {
			p := Packet{Protocol: proto, SrcIP: netip.MustParseAddr(source), DstIP: netip.MustParseAddr("198.51.100.9")}
			if proto == 6 || proto == 17 {
				p.SrcPort, p.DstPort = 65535, 443
			}
			d, err := a.Evaluate(p)
			if err != nil {
				t.Fatal(err)
			}
			if source == "192.0.2.7" {
				if d.Action != "permit" || d.Implicit || d.Line == nil || *d.Line != 3 || d.RuleOrdinal == nil || *d.RuleOrdinal != 0 {
					t.Fatalf("host trace: %+v", d)
				}
			} else if source == "192.0.2.8" {
				if d.Action != "deny" || d.Implicit || d.Line == nil || *d.Line != 2 {
					t.Fatalf("network trace: %+v", d)
				}
			} else if d.Action != "deny" || !d.Implicit {
				t.Fatalf("implicit deny: %+v", d)
			}
		}
	}
}

package ciscoacl

import (
	"net/netip"
	"strings"
	"testing"
)

const groupDoc = `object-group network SRCNET
 host 192.0.2.10
 192.0.2.0 255.255.255.0
 group-object EXTRA
object-group network EXTRA
 host 198.51.100.7
object-group service WEB tcp
 port-object eq 80
 port-object eq 443
 port-object range 8000 8009
ip access-list extended G1
 permit tcp object-group SRCNET any object-group WEB
 deny tcp any any eq 22
 permit ip any any
!
`

func TestParseGroupsBasic(t *testing.T) {
	a, err := Parse([]byte(groupDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(a.Rules) != 3 {
		t.Fatalf("rules = %d, want 3 (groups must not become rules)", len(a.Rules))
	}
	r := a.Rules[0]
	// SRCNET resolves to: host .10, subnet 192.0.2.0/24 (mask converted to
	// wildcard), plus nested EXTRA's host — canonical sorted order, and the
	// /24 subsumes .10 only as a member (no pruning, exact dedup only).
	want := []Address{
		{Value: ipv4(netip.MustParseAddr("192.0.2.0")), Wildcard: 0xFF},
		{Value: ipv4(netip.MustParseAddr("192.0.2.10")), Wildcard: 0},
		{Value: ipv4(netip.MustParseAddr("198.51.100.7")), Wildcard: 0},
	}
	if len(r.SrcSet) != len(want) {
		t.Fatalf("src set = %+v, want %+v", r.SrcSet, want)
	}
	for i := range want {
		if r.SrcSet[i] != want[i] {
			t.Fatalf("src set[%d] = %+v, want %+v", i, r.SrcSet[i], want[i])
		}
	}
	if r.Src != (Address{}) {
		t.Fatalf("single src must stay zero when a set is present: %+v", r.Src)
	}
	wantPorts := []Interval{{80, 80}, {443, 443}, {8000, 8009}}
	if len(r.DstPorts) != len(wantPorts) {
		t.Fatalf("dst ports = %+v, want %+v", r.DstPorts, wantPorts)
	}
	for i := range wantPorts {
		if r.DstPorts[i] != wantPorts[i] {
			t.Fatalf("dst ports[%d] = %+v, want %+v", i, r.DstPorts[i], wantPorts[i])
		}
	}
}

func TestParseGroupsEvaluate(t *testing.T) {
	a, err := Parse([]byte(groupDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pkt := func(src, dst string, proto uint8, dport uint16) Packet {
		return Packet{Protocol: proto, SrcIP: netip.MustParseAddr(src), DstIP: netip.MustParseAddr(dst), DstPort: dport}
	}
	cases := []struct {
		p    Packet
		want string
	}{
		{pkt("192.0.2.10", "203.0.113.1", 6, 443), "permit"},    // group host member
		{pkt("192.0.2.99", "203.0.113.1", 6, 80), "permit"},     // group subnet member
		{pkt("198.51.100.7", "203.0.113.1", 6, 8005), "permit"}, // nested group member
		{pkt("192.0.3.10", "203.0.113.1", 6, 443), "permit"},    // not in group -> falls to permit ip
		{pkt("192.0.2.10", "203.0.113.1", 6, 25), "permit"},     // in group, port outside service group -> falls to permit ip
		{pkt("192.0.2.10", "203.0.113.1", 6, 22), "deny"},       // service group excludes 22 -> deny rule decides
		{pkt("203.0.113.8", "203.0.113.1", 6, 22), "deny"},      // deny rule reachable from any source
	}
	for i, c := range cases {
		d, err := a.Evaluate(c.p)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if d.Action != c.want {
			t.Fatalf("case %d: action = %q, want %q (packet %+v)", i, d.Action, c.want, c.p)
		}
	}
	// The deny rule (ordinal 1) must be reachable: tcp any->any:22 whose
	// source is outside every rule-1 path is decided by rule 3 (permit ip),
	// because rule 1 requires the service group. Use a packet that rule 1
	// cannot take and check the deciding ordinal for a group hit instead.
	d, err := a.Evaluate(pkt("198.51.100.7", "203.0.113.9", 6, 443))
	if err != nil || d.RuleOrdinal == nil || *d.RuleOrdinal != 0 {
		t.Fatalf("nested member decision = %+v, %v; want ordinal 0", d, err)
	}
}

func TestParseGroupsErrors(t *testing.T) {
	docs := map[string]string{
		"undefined": "ip access-list extended X\n permit tcp object-group NOPE any eq 80\n",
		"cycle": `object-group network A
 group-object B
object-group network B
 group-object A
ip access-list extended X
 permit tcp object-group A any eq 80
`,
		"duplicate": `object-group network A
 host 192.0.2.1
object-group network A
 host 192.0.2.2
ip access-list extended X
 permit ip any any
`,
		"empty": `object-group network A
ip access-list extended X
 permit ip any any
`,
		"protocol mismatch": `object-group service TCPSVC tcp
 port-object eq 80
ip access-list extended X
 permit udp any any object-group TCPSVC
`,
		"service on ip rule": `object-group service S tcp
 port-object eq 80
ip access-list extended X
 permit ip any any object-group S
`,
	}
	for name, doc := range docs {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Fatalf("%s: expected parse error", name)
		} else if ce, ok := err.(*Error); !ok || !strings.HasPrefix(ce.Code, "PARSE_") {
			t.Fatalf("%s: unexpected error %v", name, err)
		}
	}
	// Group grammar is v0.4-only: the same document under v0.3 must fail.
	if _, err := ParseVersion([]byte(groupDoc), PreviousParserVersion); err == nil {
		t.Fatal("v0.3 accepted object-group syntax")
	}
}

func TestParseGroupsDefinedAfterACL(t *testing.T) {
	doc := `ip access-list extended LATE
 permit tcp object-group N any eq 443
 deny ip any any
object-group network N
 host 192.0.2.5
`
	a, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(a.Rules[0].SrcSet) != 1 {
		t.Fatalf("late-defined group did not resolve: %+v", a.Rules[0])
	}
}

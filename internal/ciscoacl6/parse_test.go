package ciscoacl6

import (
	"net/netip"
	"testing"
)

const doc6 = `ipv6 access-list EDGE6
 10 permit tcp 2001:db8:1::/48 any eq 443
 20 deny udp any host 2001:db8::53 eq 53
 30 permit icmp any any echo-request
 40 deny ipv6 2001:db8:bad::/48 any
`

func TestParseIPv6Basics(t *testing.T) {
	a, err := Parse([]byte(doc6))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(a.Rules) != 4 {
		t.Fatalf("rules = %d, want 4", len(a.Rules))
	}
	r := a.Rules[0]
	// 2001:db8:1::/48: hi word fixed in its top 48 bits.
	want := Address128{
		Value:    [2]uint64{0x20010db800010000, 0},
		Wildcard: [2]uint64{0x000000000000FFFF, ^uint64(0)},
	}
	if r.Src != want {
		t.Fatalf("src cube = %+v, want %+v", r.Src, want)
	}
	if a.Rules[2].IcmpType == nil || *a.Rules[2].IcmpType != IcmpEchoRequest {
		t.Fatalf("icmp rule type = %v, want echo-request", a.Rules[2].IcmpType)
	}
	if a.Rules[3].Protocol != nil {
		t.Fatalf("ipv6 protocol must be nil (all), got %v", *a.Rules[3].Protocol)
	}
}

func TestEvaluateIPv6AndImplicitTail(t *testing.T) {
	a, err := Parse([]byte(doc6))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pkt := func(proto uint8, src, dst string, dport uint16, icmp uint8) Packet {
		return Packet{Protocol: proto, SrcIP: netip.MustParseAddr(src), DstIP: netip.MustParseAddr(dst), DstPort: dport, IcmpType: icmp}
	}
	cases := []struct {
		p        Packet
		want     string
		implicit bool
	}{
		{pkt(6, "2001:db8:1::5", "2001:db8:2::9", 443, 0), "permit", false},
		{pkt(6, "2001:db8:1::5", "2001:db8:2::9", 80, 0), "deny", true},  // falls through to implicit deny
		{pkt(17, "2001:db8::1", "2001:db8::53", 53, 0), "deny", false},   // explicit udp deny
		{pkt(58, "2001:db8::1", "2001:db8::2", 0, 128), "permit", false}, // explicit echo-request
		{pkt(58, "2001:db8::1", "2001:db8::2", 0, 135), "permit", true},  // implicit nd-ns
		{pkt(58, "2001:db8::1", "2001:db8::2", 0, 136), "permit", true},  // implicit nd-na
		{pkt(58, "2001:db8::1", "2001:db8::2", 0, 2), "deny", true},      // packet-too-big is NOT implicit
		{pkt(6, "2001:db8:bad::1", "2001:db8::2", 443, 0), "deny", false},
	}
	for i, c := range cases {
		d, err := a.Evaluate(c.p)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if d.Action != c.want || d.Implicit != c.implicit {
			t.Fatalf("case %d: decision = %+v, want action %q implicit %v", i, d, c.want, c.implicit)
		}
	}
}

func TestParseIPv6Errors(t *testing.T) {
	docs := map[string]string{
		"ipv4 wildcard form": "ipv6 access-list X\n permit tcp 2001:db8::1 0.0.0.255 any\n",
		"object group":       "ipv6 access-list X\n permit tcp object-group G any eq 80\n",
		"icmp with ports":    "ipv6 access-list X\n permit icmp any any eq 8\n",
		"port on ipv6 proto": "ipv6 access-list X\n permit ipv6 any any eq 80\n",
		"unknown icmp type":  "ipv6 access-list X\n permit icmp any any mld-query\n",
		"two acls":           "ipv6 access-list X\n permit ipv6 any any\nipv6 access-list Y\n permit ipv6 any any\n",
		"ipv4 prefix":        "ipv6 access-list X\n permit tcp 192.0.2.0/24 any\n",
	}
	for name, doc := range docs {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Fatalf("%s: expected parse error", name)
		}
	}
}

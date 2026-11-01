package ciscoacl

import (
	"encoding/json"
	"net/netip"
	"reflect"
	"testing"
)

// Fuzz support for the ACL parser targets (plan P1-6). The parser target
// itself is FuzzParse in acl_test.go; this file carries its packet
// generator and the JSON round-trip target.

func fuzzPacket(data []byte, i int) Packet {
	b := func(off int, def byte) byte {
		if i+off < len(data) {
			return data[i+off]
		}
		return def
	}
	proto := []uint8{6, 17, 1, 47}[b(0, 6)%4]
	p := Packet{
		Protocol: proto,
		SrcIP:    netip.AddrFrom4([4]byte{b(1, 192), b(2, 0), b(3, 2), b(4, 10)}),
		DstIP:    netip.AddrFrom4([4]byte{b(5, 198), b(6, 51), b(7, 100), b(8, 10)}),
	}
	if proto == 6 || proto == 17 {
		p.SrcPort = uint16(b(9, 0))<<8 | uint16(b(10, 0))
		p.DstPort = uint16(b(11, 0))<<8 | uint16(b(12, 22))
	}
	return p
}

// FuzzACLJSONRoundTrip is the parse -> print -> re-parse round trip over the
// wire form this project actually prints: the ACL JSON DTO (evidence and the
// parse CLI embed it; there is no canonical Cisco-text printer in the
// codebase). A round trip must preserve the DTO exactly and keep every
// packet decision identical.
func FuzzACLJSONRoundTrip(f *testing.F) {
	f.Add([]byte("ip access-list extended MGMT\n 10 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 20 deny ip any any\n"))
	f.Add([]byte("ip access-list extended MGMT\n 20 deny tcp host 192.0.2.10 any eq 22 log\n 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 40 deny ip any any\n!\n"))
	f.Add([]byte("ip access-list extended X\n permit tcp any any neq 23\n permit udp any any range 8000 8099\n deny ip any any log-input\n"))
	f.Add([]byte("ip access-list extended X\n permit icmp any any\n permit 47 host 192.0.2.1 any\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		a, err := Parse(data)
		if err != nil {
			return
		}
		raw, err := json.Marshal(a)
		if err != nil {
			t.Fatalf("Marshal(parsed ACL) failed: %v", err)
		}
		var back ACL
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("Unmarshal(printed ACL) failed: %v", err)
		}
		if !reflect.DeepEqual(a, back) {
			t.Fatalf("JSON round trip changed the ACL:\n%+v\n%+v", a, back)
		}
		for i := 0; i+16 <= len(data)+16; i += 16 {
			p := fuzzPacket(data, i)
			d1, err1 := a.Evaluate(p)
			d2, err2 := back.Evaluate(p)
			if err1 != nil || err2 != nil || d1.Action != d2.Action ||
				(d1.RuleOrdinal == nil) != (d2.RuleOrdinal == nil) ||
				(d1.RuleOrdinal != nil && *d1.RuleOrdinal != *d2.RuleOrdinal) {
				t.Fatalf("decision changed across round trip for %v: %v/%v vs %v/%v", p, d1, err1, d2, err2)
			}
		}
	})
}

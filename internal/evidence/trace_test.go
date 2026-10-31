package evidence

import (
	"net/netip"
	"plenilune/internal/ciscoacl"
	"testing"
)

func TestTraceBindsPacketResultSourceAndScope(t *testing.T) {
	p := ciscoacl.Packet{Protocol: 6, SrcIP: netip.MustParseAddr("192.0.2.1"), DstIP: netip.MustParseAddr("198.51.100.1"), SrcPort: 40000, DstPort: 22}
	x, err := SealTrace(ConfigDigest([]byte("acl")), p, ciscoacl.Decision{Action: "deny", Implicit: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckIntegrity(x); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Trace){
		func(x *Trace) { x.Packet.DstPort = 23 },
		func(x *Trace) { x.ConfigSHA256 = ConfigDigest([]byte("different")) },
		func(x *Trace) { x.Decision.Action = "permit" },
		func(x *Trace) { x.Scope = "network-wide" },
	} {
		y := x
		mutate(&y)
		if CheckIntegrity(y) == nil {
			t.Fatal("accepted modified trace")
		}
	}
}

func TestTraceBindsExplicitRuleSource(t *testing.T) {
	p := ciscoacl.Packet{Protocol: 6, SrcIP: netip.MustParseAddr("192.0.2.1"), DstIP: netip.MustParseAddr("198.51.100.1"), SrcPort: 40000, DstPort: 22}
	ordinal, line := 0, 2
	x, err := SealTrace(ConfigDigest([]byte("acl")), p, ciscoacl.Decision{Action: "permit", RuleOrdinal: &ordinal, Line: &line})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Trace){
		func(x *Trace) { n := 3; x.Decision.Line = &n },
		func(x *Trace) { n := 1; x.Decision.RuleOrdinal = &n },
		func(x *Trace) { x.Packet.SrcIP = netip.MustParseAddr("192.0.2.2") },
		func(x *Trace) { x.Packet.DstIP = netip.MustParseAddr("198.51.100.2") },
		func(x *Trace) { x.Packet.Protocol = 17 },
		func(x *Trace) { x.Packet.SrcPort = 40001 },
	} {
		y := x
		mutate(&y)
		if CheckIntegrity(y) == nil {
			t.Fatal("modified source or tuple retained valid digest")
		}
	}
}

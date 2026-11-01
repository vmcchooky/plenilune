package verify

import (
	"context"
	"strings"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

func boxOf(t *testing.T, proto string, src, dst string, sport, dport packetset.Interval) packetset.Box {
	t.Helper()
	p := packetset.AllProtocols()
	switch proto {
	case "ip":
	case "tcp":
		p = packetset.Protocol(6)
	case "udp":
		p = packetset.Protocol(17)
	case "icmp":
		p = packetset.Protocol(1)
	default:
		n := 0
		for _, c := range proto {
			n = n*10 + int(c-'0')
		}
		p = packetset.Protocol(uint8(n))
	}
	cube := func(s string) packetset.Cube {
		if s == "any" {
			return packetset.Cube{Wildcard: ^uint32(0)}
		}
		c, err := prefixCube(s)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	return packetset.Box{Protocols: p, Src: cube(src), Dst: cube(dst), SrcPort: sport, DstPort: dport}
}

func TestRenderBoxRuleRoundTrips(t *testing.T) {
	full := packetset.Interval{Lo: 0, Hi: 65535}
	cases := []struct {
		name string
		box  packetset.Box
		want string
	}{
		{"tcp host to host eq", boxOf(t, "tcp", "192.0.2.10/32", "198.51.100.10/32", full, packetset.Interval{Lo: 22, Hi: 22}), "permit tcp host 192.0.2.10 host 198.51.100.10 eq 22"},
		{"udp subnet range", boxOf(t, "udp", "192.0.2.0/24", "198.51.100.0/25", packetset.Interval{Lo: 1024, Hi: 2048}, packetset.Interval{Lo: 53, Hi: 53}), "permit udp 192.0.2.0 0.0.0.255 range 1024 2048 198.51.100.0 0.0.0.127 eq 53"},
		{"icmp any", boxOf(t, "icmp", "any", "198.51.100.10/32", packetset.Interval{}, packetset.Interval{}), "deny icmp any host 198.51.100.10"},
		{"ip any any", boxOf(t, "ip", "any", "any", full, full), "permit ip any any"},
	}
	for _, tc := range cases {
		action := "permit"
		if strings.HasPrefix(tc.want, "deny") {
			action = "deny"
		}
		got, ok := RenderBoxRule(action, tc.box)
		if !ok || got != tc.want {
			t.Fatalf("%s: got %q ok=%v want %q", tc.name, got, ok, tc.want)
		}
	}
}

func TestRenderBoxRuleRejectsUnrepresentable(t *testing.T) {
	full := packetset.Interval{Lo: 0, Hi: 65535}
	// All protocols except TCP: not one rule.
	p := packetset.AllProtocols()
	p[0] &^= uint64(1) << 6
	if _, ok := RenderBoxRule("permit", packetset.Box{Protocols: p, Src: packetset.Cube{Wildcard: ^uint32(0)}, Dst: packetset.Cube{Wildcard: ^uint32(0)}, SrcPort: full, DstPort: full}); ok {
		t.Fatal("rendered a multi-protocol box")
	}
	// TCP+UDP together: not one rule.
	p2 := packetset.Protocol(6)
	p2[0] |= uint64(1) << 17
	if _, ok := RenderBoxRule("permit", packetset.Box{Protocols: p2, Src: packetset.Cube{Wildcard: ^uint32(0)}, Dst: packetset.Cube{Wildcard: ^uint32(0)}, SrcPort: full, DstPort: full}); ok {
		t.Fatal("rendered a tcp+udp box")
	}
	// Bad action.
	if _, ok := RenderBoxRule("inspect", boxOf(t, "tcp", "any", "any", full, full)); ok {
		t.Fatal("rendered a bad action")
	}
}

const (
	p13Before = "ip access-list extended MGMT\n 10 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 20 deny ip any any\n"
	p13After  = "ip access-list extended MGMT\n 10 remark synthetic documentation addresses only\n 20 deny tcp host 192.0.2.10 any eq 22 log\n 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 40 deny ip any any\n!\n"
)

func p13Set(t *testing.T, kind string) intent.Set {
	t.Helper()
	return intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: kind, Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
}

func TestClassAndViolationRegionsOnFixtureShape(t *testing.T) {
	b, err := ciscoacl.Parse([]byte(p13Before))
	if err != nil {
		t.Fatal(err)
	}
	a, err := ciscoacl.Parse([]byte(p13After))
	if err != nil {
		t.Fatal(err)
	}
	classes, rep, complete, err := ChangeClassRegions(context.Background(), b, a, []intent.Region{}, ChangeOptions{Limits: packetset.Defaults(), MaxClasses: MaxClasses})
	if err != nil || !complete || !rep.Complete {
		t.Fatalf("classes: err=%v complete=%v rep=%+v", err, complete, rep)
	}
	if len(classes) != 1 {
		t.Fatalf("classes = %d, want 1", len(classes))
	}
	c := classes[0]
	if c.Class.Scope != "outside_allowed" || c.Class.Effect != "newly_denied" || ordOf(c.Class.Before) != 0 || ordOf(c.Class.After) != 0 {
		t.Fatalf("class: %+v", c.Class)
	}
	wantBox := boxOf(t, "tcp", "192.0.2.10/32", "198.51.100.10/32", packetset.Interval{Lo: 0, Hi: 65535}, packetset.Interval{Lo: 22, Hi: 22})
	if len(c.Boxes) != 1 || c.Boxes[0] != wantBox {
		t.Fatalf("class boxes: %+v want %+v", c.Boxes, wantBox)
	}
	set := p13Set(t, "must_permit")
	violations, comp, complete, err := ObligationViolationRegions(context.Background(), b, a, &set, DefaultOptions())
	if err != nil || !complete || !comp.SearchComplete {
		t.Fatalf("violations: err=%v complete=%v", err, complete)
	}
	if len(violations) != 1 || violations[0].Obligation.ID != "OB-1" || len(violations[0].Boxes) != 1 || violations[0].Boxes[0] != wantBox {
		t.Fatalf("violations: %+v", violations)
	}
	// must_remain over the same region is violated by the same packets.
	set = p13Set(t, "must_remain")
	violations, _, complete, err = ObligationViolationRegions(context.Background(), b, a, &set, DefaultOptions())
	if err != nil || !complete || len(violations) != 1 || len(violations[0].Boxes) != 1 || violations[0].Boxes[0] != wantBox {
		t.Fatalf("must_remain violations: err=%v complete=%v %+v", err, complete, violations)
	}
	// must_deny over a region the after ACL permits: violation region is
	// the permitted part of the region.
	set = p13Set(t, "must_deny")
	violations, _, complete, err = ObligationViolationRegions(context.Background(), b, a, &set, DefaultOptions())
	if err != nil || !complete {
		t.Fatalf("must_deny: err=%v complete=%v", err, complete)
	}
	if len(violations) != 1 || len(violations[0].Boxes) == 0 {
		t.Fatalf("must_deny violations: %+v", violations)
	}
	// The denied-by-rule-0 host is carved out of the permitted region by
	// the cover, so the violation set is the /24 minus host 192.0.2.10.
	// Compare sets with the algebra, not box counts.
	regionBox := boxOf(t, "tcp", "192.0.2.0/24", "198.51.100.10/32", packetset.Interval{Lo: 0, Hi: 65535}, packetset.Interval{Lo: 22, Hi: 22})
	bud, err := packetset.NewBudget(context.Background(), packetset.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer bud.Close()
	want, err := packetset.Difference(packetset.Region{regionBox}, packetset.Region{wantBox}, bud)
	if err != nil {
		t.Fatal(err)
	}
	got := violations[0].Boxes
	extra, err := packetset.Difference(got, want, bud)
	if err != nil || len(extra) != 0 {
		t.Fatalf("must_deny violation has extra boxes: err=%v extra=%v", err, extra)
	}
	missing, err := packetset.Difference(want, got, bud)
	if err != nil || len(missing) != 0 {
		t.Fatalf("must_deny violation misses boxes: err=%v missing=%v", err, missing)
	}
}

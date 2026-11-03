package verify6

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"testing"

	"plenilune/internal/ciscoacl6"
)

// Grid oracle: random ACL pairs over a tiny subspace (2001:db8::/124 —
// 16 addresses), with rule operands drawn from prefix boundaries that the
// packet grid resolves completely. The engine verdict must agree with
// brute-force Evaluate over every grid packet.

var gridPrefixes = []string{
	"2001:db8::/124", "2001:db8::/125", "2001:db8::8/125",
	"2001:db8::/126", "2001:db8::4/126", "2001:db8::8/126", "2001:db8::c/126",
	"2001:db8::1/128", "2001:db8::5/128", "2001:db8::a/128", "2001:db8::f/128",
}

func randRule6(r *rand.Rand) string {
	action := "permit"
	if r.IntN(3) == 0 {
		action = "deny"
	}
	src := gridPrefixes[r.IntN(len(gridPrefixes))]
	dst := gridPrefixes[r.IntN(len(gridPrefixes))]
	switch r.IntN(4) {
	case 0:
		return fmt.Sprintf("%s tcp %s %s eq %d", action, src, dst, []int{53, 443, 8080}[r.IntN(3)])
	case 1:
		return fmt.Sprintf("%s udp %s %s range 100 199", action, src, dst)
	case 2:
		typ := []string{"echo-request", "nd-ns", "nd-na", "packet-too-big"}[r.IntN(4)]
		return fmt.Sprintf("%s icmp %s %s %s", action, src, dst, typ)
	default:
		return fmt.Sprintf("%s ipv6 %s %s", action, src, dst)
	}
}

func randACL6(r *rand.Rand, name string) ciscoacl6.ACL {
	text := "ipv6 access-list " + name + "\n"
	n := 1 + r.IntN(4)
	for i := 0; i < n; i++ {
		text += " " + randRule6(r) + "\n"
	}
	a, err := ciscoacl6.Parse([]byte(text))
	if err != nil {
		panic(err)
	}
	return a
}

func gridPackets() []ciscoacl6.Packet {
	out := []ciscoacl6.Packet{}
	for s := 0; s < 16; s++ {
		for d := 0; d < 16; d++ {
			src := netip.MustParseAddr(fmt.Sprintf("2001:db8::%x", s))
			dst := netip.MustParseAddr(fmt.Sprintf("2001:db8::%x", d))
			for _, proto := range []uint8{6, 17, 58, 47} {
				ports := []uint16{0}
				if proto == 6 || proto == 17 {
					ports = []uint16{53, 150, 443, 8080}
				}
				for _, port := range ports {
					for _, typ := range []uint8{1, 2, 128, 135, 136} {
						out = append(out, ciscoacl6.Packet{Protocol: proto, SrcIP: src, DstIP: dst, DstPort: port, IcmpType: typ})
					}
				}
			}
		}
	}
	return out
}

func TestCompareAgainstGridOracle(t *testing.T) {
	packets := gridPackets()
	r := rand.New(rand.NewPCG(777, 13))
	for iter := 0; iter < 120; iter++ {
		before := randACL6(r, "G")
		after := randACL6(r, "G")
		cmp, err := Compare(context.Background(), before, after, DefaultOptions())
		if err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		if !cmp.SearchComplete {
			t.Fatalf("iter %d: search incomplete on a tiny grid corpus", iter)
		}
		gridDifferent := false
		for _, p := range packets {
			bd, err := before.Evaluate(p)
			if err != nil {
				t.Fatalf("iter %d before evaluate: %v", iter, err)
			}
			ad, err := after.Evaluate(p)
			if err != nil {
				t.Fatalf("iter %d after evaluate: %v", iter, err)
			}
			if bd.Action != ad.Action {
				gridDifferent = true
				break
			}
		}
		want := "EQUIVALENT"
		if gridDifferent {
			want = "DIFFERENT"
		}
		if cmp.Verdict != want {
			t.Fatalf("iter %d: verdict %s, grid oracle wants %s\nbefore=%+v\nafter=%+v", iter, cmp.Verdict, want, before, after)
		}
		if want == "DIFFERENT" && len(cmp.Witnesses) == 0 {
			t.Fatalf("iter %d: DIFFERENT without a witness", iter)
		}
	}
}

func TestCompareImplicitTailEquivalence(t *testing.T) {
	// An ACL that explicitly permits ND and one that does not are
	// EQUIVALENT: the implicit tail permits ND in both. An engine that
	// ignored the implicit tail would call these DIFFERENT.
	withExplicit, err := ciscoacl6.Parse([]byte("ipv6 access-list A\n permit icmp any any nd-ns\n permit icmp any any nd-na\n deny tcp any any eq 22\n"))
	if err != nil {
		t.Fatal(err)
	}
	without, err := ciscoacl6.Parse([]byte("ipv6 access-list A\n deny tcp any any eq 22\n"))
	if err != nil {
		t.Fatal(err)
	}
	cmp, err := Compare(context.Background(), withExplicit, without, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Verdict != "EQUIVALENT" {
		t.Fatalf("verdict %s, want EQUIVALENT (implicit ND tail)", cmp.Verdict)
	}
}

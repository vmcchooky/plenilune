package verify

import (
	"context"
	"fmt"
	"math/rand"
	"net/netip"
	"strings"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

func smallACL(t *testing.T, rng *rand.Rand, rules int) ciscoacl.ACL {
	var s strings.Builder
	for i := 0; i < rules; i++ {
		action := "deny"
		if rng.Intn(2) == 0 {
			action = "permit"
		}
		switch rng.Intn(5) {
		case 0:
			fmt.Fprintf(&s, " %s icmp 192.0.2.%d 0.0.0.%d any\n", action, rng.Intn(4), rng.Intn(4))
		case 1:
			fmt.Fprintf(&s, " %s ip any 198.51.100.%d 0.0.0.%d\n", action, rng.Intn(4), rng.Intn(4))
		default:
			proto := []string{"tcp", "udp"}[rng.Intn(2)]
			lo, hi := rng.Intn(4), rng.Intn(4)
			if lo > hi {
				lo, hi = hi, lo
			}
			fmt.Fprintf(&s, " %s %s 192.0.2.%d 0.0.0.%d 198.51.100.%d 0.0.0.%d range %d %d\n", action, proto, rng.Intn(4), rng.Intn(4), rng.Intn(4), rng.Intn(4), lo, hi)
		}
	}
	return parse(t, s.String())
}

func ordinalOf(d ciscoacl.Decision) int {
	if d.Implicit {
		return -1
	}
	return *d.RuleOrdinal
}

// Every packet in the reduced grid lands in exactly the delta piece whose
// deciding rules match the linear evaluator; equal decisions land nowhere.
func TestChangePiecesMatchLinearEvaluatorExhaustively(t *testing.T) {
	rng := rand.New(rand.NewSource(4242))
	for seed := 0; seed < 60; seed++ {
		b, a := smallACL(t, rng, 2+rng.Intn(6)), smallACL(t, rng, 2+rng.Intn(6))
		budget, err := packetset.NewBudget(context.Background(), packetset.Defaults())
		if err != nil {
			t.Fatal(err)
		}
		d, err := exactDelta(b, a, budget)
		if err != nil {
			t.Fatal(err)
		}
		pieces := append(append([]piece(nil), d.lost...), d.gained...)
		for _, proto := range []uint8{1, 6, 17, 47} {
			for src := 0; src < 4; src++ {
				for dst := 0; dst < 4; dst++ {
					ports := []int{0}
					if proto == 6 || proto == 17 {
						ports = []int{0, 1, 2, 3, 4}
					}
					for _, dp := range ports {
						p := ciscoacl.Packet{Protocol: proto, SrcIP: netip.AddrFrom4([4]byte{192, 0, 2, byte(src)}), DstIP: netip.AddrFrom4([4]byte{198, 51, 100, byte(dst)}), DstPort: uint16(dp)}
						x, _ := b.Evaluate(p)
						y, _ := a.Evaluate(p)
						s, dd := uint32(0xC0000200|src), uint32(0xC6336400|dst)
						hits := 0
						for _, pc := range pieces {
							if packetset.Contains(pc.box, proto, s, dd, 0, uint16(dp)) {
								hits++
								if pc.before != ordinalOf(x) || pc.after != ordinalOf(y) {
									t.Fatalf("seed %d: packet %+v in piece (%d,%d), evaluator (%d,%d)", seed, p, pc.before, pc.after, ordinalOf(x), ordinalOf(y))
								}
							}
						}
						want := 0
						if x.Action != y.Action {
							want = 1
						}
						if hits != want {
							t.Fatalf("seed %d: packet %+v in %d pieces, want %d (before %s after %s)", seed, p, hits, want, x.Action, y.Action)
						}
					}
				}
			}
		}
	}
}

func TestChangesReportVerifiedClassesAndScope(t *testing.T) {
	before := parse(t, " permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443\n")
	after := parse(t, " deny tcp host 192.0.2.10 any eq 22\n permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443\n permit udp any host 198.51.100.53 eq 53\n")
	r, err := Changes(context.Background(), before, after, nil, DefaultChangeOptions())
	if err != nil || !r.Complete || r.ScopeDeclared || len(r.Classes) != 2 {
		t.Fatalf("changes: %v %+v", err, r)
	}
	denied, permitted := r.Classes[0], r.Classes[1]
	if denied.Effect != "newly_denied" || *denied.Before.Ordinal != 0 || *denied.After.Ordinal != 0 || denied.Witness.Packet.SrcIP.String() != "192.0.2.10" || denied.Witness.Packet.DstPort != 22 {
		t.Fatalf("denied class %+v", denied)
	}
	if permitted.Effect != "newly_permitted" || !permitted.Before.Implicit || *permitted.After.Ordinal != 3 || permitted.Witness.Packet.Protocol != 17 || permitted.Witness.Packet.DstPort != 53 {
		t.Fatalf("permitted class %+v", permitted)
	}
	if denied.Scope != "undeclared" || denied.Examples[0].Src != "192.0.2.10/32" || denied.Examples[0].DstPorts != "22" {
		t.Fatalf("class text %+v", denied.Examples)
	}
	allowed := []intent.Region{{Protocol: "tcp", SrcCIDR: "192.0.2.10/32", DstCIDR: "0.0.0.0/0", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}
	scoped, err := Changes(context.Background(), before, after, allowed, DefaultChangeOptions())
	if err != nil || !scoped.ScopeDeclared || scoped.Classes[0].Scope != "inside_allowed" || scoped.Classes[1].Scope != "outside_allowed" || scoped.Classes[1].OutsideWitness == nil {
		t.Fatalf("scoped: %v %+v", err, scoped)
	}
	none, err := Changes(context.Background(), before, before, []intent.Region{}, DefaultChangeOptions())
	if err != nil || !none.Complete || len(none.Classes) != 0 || !none.ScopeDeclared {
		t.Fatalf("identical: %v %+v", err, none)
	}
	tiny := DefaultChangeOptions()
	tiny.Limits.MaxOperations = 3
	stopped, err := Changes(context.Background(), before, after, nil, tiny)
	if err != nil || stopped.Complete || len(stopped.Classes) != 0 || stopped.Reason != "operations" {
		t.Fatalf("budget stop must withhold classes: %v %+v", err, stopped)
	}
	limited := DefaultChangeOptions()
	limited.MaxClasses = 1
	cut, err := Changes(context.Background(), before, after, nil, limited)
	if err != nil || len(cut.Classes) != 1 || !cut.ClassesTruncated || cut.TotalClasses != 2 {
		t.Fatalf("truncation: %v %+v", err, cut)
	}
}

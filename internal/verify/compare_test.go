package verify

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/netip"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"reflect"
	"strings"
	"testing"
	"time"
)

func parse(t *testing.T, rules string) ciscoacl.ACL {
	t.Helper()
	a, err := ciscoacl.Parse([]byte("ip access-list extended DEMO\n" + rules))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestExactCasesAndIndependentWitnesses(t *testing.T) {
	for _, tc := range []struct{ name, before, after, verdict string }{
		{"implicit", " permit tcp any any eq 22\n deny ip any any\n", " permit tcp any any eq 22\n", "EQUIVALENT"},
		{"reorder", " permit tcp any any eq 22\n deny ip any any\n", " deny ip any any\n permit tcp any any eq 22\n", "DIFFERENT"},
		{"noncontiguous", " permit tcp 10.0.0.0 0.255.0.255 any neq 22\n", " permit tcp 10.0.0.0 0.0.255.255 any neq 22\n", "DIFFERENT"},
		{"portzero", " permit tcp any any neq 22\n", " permit tcp any any range 23 65535\n", "DIFFERENT"},
		{"nonTCP", " deny ip any any\n", " permit ip any any\n", "DIFFERENT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, a := parse(t, tc.before), parse(t, tc.after)
			r, err := Compare(context.Background(), b, a, nil, DefaultOptions())
			if err != nil {
				t.Fatal(err)
			}
			if r.Verdict != tc.verdict || !r.SearchComplete {
				t.Fatalf("unexpected report: %+v", r)
			}
			for _, w := range r.Witnesses {
				x, err := b.Evaluate(w.Packet)
				if err != nil {
					t.Fatal(err)
				}
				y, err := a.Evaluate(w.Packet)
				if err != nil {
					t.Fatal(err)
				}
				if x.Action == y.Action || !reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
					t.Fatal("invalid witness")
				}
			}
			if tc.name == "nonTCP" && (r.Witnesses[0].Packet.Protocol != 0 || r.Witnesses[0].Packet.SrcPort != 0 || r.Witnesses[0].Packet.DstPort != 0) {
				t.Fatal("invalid non-TCP witness domain")
			}
			r2, err := Compare(context.Background(), b, a, nil, DefaultOptions())
			if err != nil || !reflect.DeepEqual(r, r2) {
				t.Fatal("comparison is not deterministic")
			}
		})
	}
}

func condition(kind string) intent.Set {
	return intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: kind, Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.0/24", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
}

func TestObligationsIndependentOfGlobalDifference(t *testing.T) {
	deny, allow := parse(t, " deny ip any any\n"), parse(t, " permit tcp any any eq 22\n")
	for _, tc := range []struct {
		kind            string
		b, a            ciscoacl.ACL
		verdict, status string
	}{
		{"must_permit", deny, deny, "EQUIVALENT", "VIOLATED"},
		{"must_deny", allow, allow, "EQUIVALENT", "VIOLATED"},
		{"must_remain", deny, allow, "DIFFERENT", "VIOLATED"},
		{"must_permit", deny, allow, "DIFFERENT", "HOLDS"},
		{"must_deny", deny, deny, "EQUIVALENT", "HOLDS"},
	} {
		s := condition(tc.kind)
		r, err := Compare(context.Background(), tc.b, tc.a, &s, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if r.Verdict != tc.verdict || len(r.Obligations) != 1 || r.Obligations[0].Status != tc.status {
			t.Fatalf("unexpected obligation: %+v", r)
		}
		if tc.status == "VIOLATED" && r.Obligations[0].Witness == nil {
			t.Fatal("missing independent witness")
		}
	}
}

func TestExcludeRegionsAndProtocolDomain(t *testing.T) {
	b := parse(t, " deny ip any any\n")
	a := parse(t, " permit tcp 192.0.2.0 0.0.0.127 any eq 22\n")
	s := condition("must_deny")
	s.Obligations[0].Match.ExcludeSrc = []string{"192.0.2.0/25"}
	r, err := Compare(context.Background(), b, a, &s, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if r.Obligations[0].Status != "HOLDS" {
		t.Fatal("excluded addresses incorrectly included")
	}
	s.Obligations[0].Match.Protocol = "ip"
	s.Obligations[0].Match.DstPorts = nil
	r, err = Compare(context.Background(), b, a, &s, DefaultOptions())
	if err != nil || r.Obligations[0].Status != "HOLDS" {
		t.Fatalf("all-protocol domain: %v %+v", err, r)
	}
}

func TestReducedDomainOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(1717))
	makeACL := func() ciscoacl.ACL {
		var s strings.Builder
		for i := 0; i < 6; i++ {
			action := "deny"
			if rng.Intn(2) == 0 {
				action = "permit"
			}
			proto := "tcp"
			if rng.Intn(2) == 0 {
				proto = "udp"
			}
			lo, hi := rng.Intn(4), rng.Intn(4)
			if lo > hi {
				lo, hi = hi, lo
			}
			wild := rng.Intn(4)
			fmt.Fprintf(&s, " %s %s 192.0.2.%d 0.0.0.%d 198.51.100.%d 0.0.0.%d range %d %d\n", action, proto, rng.Intn(4), wild, rng.Intn(4), rng.Intn(4), lo, hi)
		}
		return parse(t, s.String())
	}
	for seed := 0; seed < 40; seed++ {
		b, a := makeACL(), makeACL()
		different := false
		// Rules constrain both address low bytes to 0..3 and destination ports
		// to 0..3. Source ports are unconstrained and cannot alter decisions.
		for _, proto := range []uint8{6, 17} {
			for src := 0; src < 4; src++ {
				for dst := 0; dst < 4; dst++ {
					for dp := 0; dp < 4; dp++ {
						p := ciscoacl.Packet{Protocol: proto, SrcIP: netip.AddrFrom4([4]byte{192, 0, 2, byte(src)}), DstIP: netip.AddrFrom4([4]byte{198, 51, 100, byte(dst)}), DstPort: uint16(dp)}
						x, err := b.Evaluate(p)
						if err != nil {
							t.Fatal(err)
						}
						y, err := a.Evaluate(p)
						if err != nil {
							t.Fatal(err)
						}
						if x.Action != y.Action {
							different = true
						}
					}
				}
			}
		}
		r, err := Compare(context.Background(), b, a, nil, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if !r.SearchComplete || (r.Verdict == "DIFFERENT") != different {
			t.Fatalf("oracle mismatch case %d: %+v", seed, r)
		}
	}
}

func TestBudgetAndCancellation(t *testing.T) {
	b := parse(t, " permit tcp any any eq 22\n")
	opts := DefaultOptions()
	opts.Limits.MaxOperations = 1
	r, err := Compare(context.Background(), b, b, nil, opts)
	if err != nil || r.Verdict != "INCONCLUSIVE" || r.SearchComplete || r.Budget.StopReason != "operations" {
		t.Fatalf("budget result: %v %+v", err, r)
	}
	opts = DefaultOptions()
	now := time.Unix(1, 0)
	budget, createErr := packetset.NewBudgetWithClock(context.Background(), opts.Limits, func() time.Time { return now })
	if createErr != nil {
		t.Fatal(createErr)
	}
	defer budget.Close()
	now = now.Add(opts.Limits.Timeout)
	for _, version := range []string{EngineVersion, LegacyEngineVersion} {
		eng, _ := engineFor(version)
		r, err = compareWithBudget(context.Background(), eng, b, b, nil, opts, budget)
		if err != nil || r.Verdict != "INCONCLUSIVE" || !r.Budget.TimingLimited {
			t.Fatalf("%s timing result: %v %+v", version, err, r)
		}
	}
	if _, err := CompareEngine(context.Background(), b, b, nil, DefaultOptions(), "acl-exact/v9"); err == nil {
		t.Fatal("unknown engine version accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Compare(ctx, b, b, nil, DefaultOptions()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	bad := b
	bad.Name = "different"
	if _, err := Compare(context.Background(), b, bad, nil, DefaultOptions()); err == nil {
		t.Fatal("identity mismatch accepted")
	}
}

package verify

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
)

// Fuzz targets for the verifier (plan P1-6): a differential target pitting
// the point evaluator against an independent brute-force scan over exact
// rule regions on a reduced domain, and an end-to-end compare target.
// Budget stops may yield INCONCLUSIVE; panics, wrong witnesses and verdicts
// that contradict re-evaluation are failures.

type fuzzGen struct {
	data []byte
	pos  int
}

func (g *fuzzGen) next() byte {
	if g.pos >= len(g.data) {
		return 0
	}
	b := g.data[g.pos]
	g.pos++
	return b
}

func (g *fuzzGen) pick(n int) int { return int(g.next()) % n }

var fuzzHosts = []string{"192.0.2.0", "192.0.2.1", "192.0.2.5", "192.0.2.15", "192.0.2.16", "192.0.2.100", "198.51.100.1"}

// fuzzGenACL renders a small ACL (2-7 rules) over a reduced domain from the
// byte stream. Every rendering is valid parser input by construction.
func fuzzGenACL(g *fuzzGen) string {
	addr := func() string {
		switch g.pick(4) {
		case 0:
			return "any"
		case 1:
			return "host " + fuzzHosts[g.pick(len(fuzzHosts))]
		case 2:
			return "192.0.2.0 0.0.0.15"
		default:
			return "192.0.2.0 0.0.0.255"
		}
	}
	ports := func() string {
		switch g.pick(6) {
		case 0:
			return ""
		case 1:
			return " eq 22"
		case 2:
			return " eq 80"
		case 3:
			return " range 0 1023"
		case 4:
			return " gt 1023"
		default:
			return " neq 53"
		}
	}
	n := 2 + g.pick(6)
	var sb strings.Builder
	sb.WriteString("ip access-list extended FUZZ\n")
	for i := 0; i < n; i++ {
		action := "permit"
		if g.pick(3) == 0 {
			action = "deny"
		}
		proto := []string{"ip", "tcp", "udp", "icmp", "47"}[g.pick(5)]
		fmt.Fprintf(&sb, " %d %s %s %s", (i+1)*10, action, proto, addr())
		if proto == "tcp" || proto == "udp" {
			sb.WriteString(ports())
		}
		fmt.Fprintf(&sb, " %s", addr())
		if proto == "tcp" || proto == "udp" {
			sb.WriteString(ports())
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func fuzzParse(t *testing.T, text string) ciscoacl.ACL {
	t.Helper()
	a, err := ciscoacl.Parse([]byte(text))
	if err != nil {
		t.Fatalf("generated ACL does not parse: %v\n%s", err, text)
	}
	return a
}

// bruteDecision is the independent oracle: the first rule whose exact match
// region contains the packet decides; otherwise the implicit deny decides.
func bruteDecision(t *testing.T, a ciscoacl.ACL, p ciscoacl.Packet) ciscoacl.Decision {
	t.Helper()
	src := p.SrcIP.As4()
	dst := p.DstIP.As4()
	s := uint32(src[0])<<24 | uint32(src[1])<<16 | uint32(src[2])<<8 | uint32(src[3])
	d := uint32(dst[0])<<24 | uint32(dst[1])<<16 | uint32(dst[2])<<8 | uint32(dst[3])
	for i, r := range a.Rules {
		region, err := RuleRegion(r, packetset.Defaults())
		if err != nil {
			t.Fatalf("RuleRegion(rule %d) failed: %v", i, err)
		}
		for _, box := range region {
			if packetset.Contains(box, p.Protocol, s, d, p.SrcPort, p.DstPort) {
				ord := i
				return ciscoacl.Decision{Action: r.Action, RuleOrdinal: &ord}
			}
		}
	}
	return ciscoacl.Decision{Action: "deny", Implicit: true}
}

func FuzzEvaluateVsRegionBruteForce(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24})
	f.Add([]byte{})
	f.Add([]byte{200, 100, 50, 25, 12, 6, 3, 1, 0, 255, 254, 253, 128, 64, 32, 16, 8, 4, 2, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		a := fuzzParse(t, fuzzGenACL(&fuzzGen{data: data}))
		ports := []uint16{0, 22, 53, 80, 443, 1024, 8080}
		for _, proto := range []uint8{6, 17, 1, 47} {
			sports, dports := []uint16{0}, []uint16{0}
			if proto == 6 || proto == 17 {
				sports, dports = ports, ports
			}
			for _, sh := range fuzzHosts {
				for _, dh := range fuzzHosts {
					for _, sp := range sports {
						for _, dp := range dports {
							p := ciscoacl.Packet{Protocol: proto, SrcIP: netip.MustParseAddr(sh), DstIP: netip.MustParseAddr(dh), SrcPort: sp, DstPort: dp}
							got, err := a.Evaluate(p)
							if err != nil {
								t.Fatalf("Evaluate(%v) failed: %v", p, err)
							}
							want := bruteDecision(t, a, p)
							if got.Action != want.Action || got.Implicit != want.Implicit ||
								(got.RuleOrdinal == nil) != (want.RuleOrdinal == nil) ||
								(got.RuleOrdinal != nil && *got.RuleOrdinal != *want.RuleOrdinal) {
								t.Fatalf("decision mismatch for %v:\n evaluator: %+v\n brute force: %+v\nACL:\n%+v", p, got, want, a)
							}
						}
					}
				}
			}
		}
	})
}

func FuzzCompareSmallPairs(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32})
	f.Add([]byte{})
	f.Add([]byte{42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42})
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &fuzzGen{data: data}
		before := fuzzParse(t, fuzzGenACL(g))
		after := fuzzParse(t, fuzzGenACL(g))
		opts := Options{Limits: packetset.Limits{MaxOperations: 200_000, MaxBoxes: 20_000, Timeout: 2 * time.Second}, MaxWitnesses: 4}
		ctx := context.Background()

		same, err := Compare(ctx, before, before, nil, opts)
		if err != nil {
			t.Fatalf("Compare(identical) failed: %v", err)
		}
		if !same.SearchComplete || same.Verdict != "EQUIVALENT" {
			t.Fatalf("identical ACLs must be EQUIVALENT and complete, got %s (complete=%v, reason=%q)", same.Verdict, same.SearchComplete, same.Reason)
		}

		r, err := Compare(ctx, before, after, nil, opts)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		switch r.Verdict {
		case "EQUIVALENT", "DIFFERENT", "INCONCLUSIVE":
		default:
			t.Fatalf("unknown verdict %q", r.Verdict)
		}
		if !r.SearchComplete {
			return // budget stop: INCONCLUSIVE/partial is allowed, nothing to recheck
		}
		if r.Verdict == "EQUIVALENT" && len(r.Witnesses) != 0 {
			t.Fatalf("EQUIVALENT report carries %d witnesses", len(r.Witnesses))
		}
		if r.Verdict == "DIFFERENT" {
			if len(r.Witnesses) == 0 {
				t.Fatalf("complete DIFFERENT report carries no witness")
			}
			for _, w := range r.Witnesses {
				db, err := before.Evaluate(w.Packet)
				if err != nil {
					t.Fatalf("witness packet does not evaluate on before: %v", err)
				}
				da, err := after.Evaluate(w.Packet)
				if err != nil {
					t.Fatalf("witness packet does not evaluate on after: %v", err)
				}
				if db.Action != w.Before.Action || da.Action != w.After.Action {
					t.Fatalf("witness decisions do not re-evaluate: recorded %s/%s, actual %s/%s", w.Before.Action, w.After.Action, db.Action, da.Action)
				}
				if db.Action == da.Action {
					t.Fatalf("witness packet %v does not differ: both %s", w.Packet, db.Action)
				}
			}
		}
	})
}

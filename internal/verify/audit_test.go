package verify

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/netip"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
	"reflect"
	"strings"
	"testing"
)

func auditFindings(r AuditReport, kind string) []AuditFinding {
	out := []AuditFinding{}
	for _, f := range r.Findings {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

// checkAuditWitnesses re-validates every witness in the report against
// the linear evaluator and the rule regions, independently of the
// audit's own construction: an unreachable witness packet must lie in
// its rule's match and be decided earlier; a relation witness packet
// must lie in both rules' matches and carry the evaluator's decision.
func checkAuditWitnesses(t *testing.T, acl ciscoacl.ACL, r AuditReport) {
	t.Helper()
	b, err := packetset.NewBudget(context.Background(), packetset.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	raws := make([]packetset.Region, len(acl.Rules))
	for i, rule := range acl.Rules {
		raw, err := ruleRegion(rule, b)
		if err != nil {
			t.Fatal(err)
		}
		raws[i] = raw
	}
	for _, f := range r.Findings {
		if f.Kind != AuditUnreachable || f.Witness == nil {
			continue
		}
		w := f.Witness
		if !regionHasPacket(raws[f.Rule], w.Packet) {
			t.Fatalf("unreachable witness outside rule %d match: %+v", f.Rule, w)
		}
		d, err := acl.Evaluate(w.Packet)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d, w.Decision) || d.Implicit || d.RuleOrdinal == nil || *d.RuleOrdinal >= f.Rule {
			t.Fatalf("unreachable witness not decided earlier: %+v", w)
		}
	}
	for _, rel := range r.Relations {
		if rel.Witness == nil {
			t.Fatalf("relation without witness: %+v", rel)
		}
		w := rel.Witness
		if !regionHasPacket(raws[rel.Earlier], w.Packet) || !regionHasPacket(raws[rel.Later], w.Packet) {
			t.Fatalf("relation witness outside a match: %+v", w)
		}
		d, err := acl.Evaluate(w.Packet)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d, w.Decision) {
			t.Fatalf("relation witness decision not the evaluator's: %+v", w)
		}
	}
}

func TestAuditUnreachableExact(t *testing.T) {
	acl := parse(t, " permit tcp any any eq 22\n permit tcp any any eq 22\n permit tcp 192.0.2.0 0.0.0.127 any eq 80\n permit tcp 192.0.2.128 0.0.0.127 any eq 80\n permit tcp 192.0.2.0 0.0.0.255 any eq 80\n permit udp any any eq 53\n")
	r, err := AuditACL(context.Background(), acl, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete || !r.RelationsComplete {
		t.Fatalf("audit not complete: %+v", r)
	}
	un := auditFindings(r, AuditUnreachable)
	if len(un) != 2 || un[0].Rule != 1 || un[1].Rule != 4 {
		t.Fatalf("unreachable findings: %+v", un)
	}
	// Rule 1 is an exact duplicate; rule 4 is covered only by the
	// union of rules 2 and 3 — neither covers it alone.
	if un[0].Witness == nil || un[0].Witness.Decision.RuleOrdinal == nil || *un[0].Witness.Decision.RuleOrdinal != 0 {
		t.Fatalf("duplicate witness: %+v", un[0])
	}
	checkAuditWitnesses(t, acl, r)
	// The other side of the same shapes: rule 0 is redundant (its
	// duplicate decides identically), and rules 2 and 3 are each
	// redundant (rule 4 decides their whole effective region with
	// the same action, even though rule 4 is itself unreachable).
	red := auditFindings(r, AuditRedundant)
	if len(red) != 3 || red[0].Rule != 0 || red[1].Rule != 2 || red[2].Rule != 3 {
		t.Fatalf("redundant findings: %+v", red)
	}
	if !reflect.DeepEqual(red[0].CoveredBy, []int{1}) || !reflect.DeepEqual(red[1].CoveredBy, []int{4}) || !reflect.DeepEqual(red[2].CoveredBy, []int{4}) {
		t.Fatalf("redundant covered-by: %+v", red)
	}
}

func TestAuditUnreachableByWildcardAndPortRange(t *testing.T) {
	acl := parse(t, " permit tcp 10.0.0.0 0.255.0.255 any eq 80\n permit tcp host 10.2.0.9 any eq 80\n deny tcp any any range 1 1024\n deny tcp any any eq 80\n")
	r, err := AuditACL(context.Background(), acl, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	un := auditFindings(r, AuditUnreachable)
	if len(un) != 2 || un[0].Rule != 1 || un[1].Rule != 3 {
		t.Fatalf("unreachable findings: %+v", un)
	}
	checkAuditWitnesses(t, acl, r)
}

func TestAuditRedundantExact(t *testing.T) {
	acl := parse(t, " permit tcp host 192.0.2.10 any eq 22\n permit tcp any any eq 22\n deny tcp any any eq 23\n")
	r, err := AuditACL(context.Background(), acl, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	red := auditFindings(r, AuditRedundant)
	if len(red) != 2 || red[0].Rule != 0 || red[1].Rule != 2 {
		t.Fatalf("redundant findings: %+v", red)
	}
	if !reflect.DeepEqual(red[0].CoveredBy, []int{1}) || red[0].ImplicitDeny {
		t.Fatalf("covered-by evidence: %+v", red[0])
	}
	// Rule 2 is decided by nothing later; its packets fall through to
	// the implicit deny, which denies them identically.
	if len(red[1].CoveredBy) != 0 || !red[1].ImplicitDeny {
		t.Fatalf("fallthrough evidence: %+v", red[1])
	}
}

func TestAuditFinalDenyIsRedundantInModel(t *testing.T) {
	acl := parse(t, " permit tcp any any eq 22\n deny ip any any\n")
	r, err := AuditACL(context.Background(), acl, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	red := auditFindings(r, AuditRedundant)
	if len(red) != 1 || red[0].Rule != 1 || !red[0].ImplicitDeny {
		t.Fatalf("final deny finding: %+v", r.Findings)
	}
}

func TestAuditRedundantTrapNoFalsePositive(t *testing.T) {
	// Rule 0's whole match is covered by rule 2's raw region with the
	// same action — but rule 1 decides the host part first with the
	// other action, so removing rule 0 WOULD change behavior. The
	// naive coverage formula would flag rule 0; the exact
	// post-removal replay must not.
	acl := parse(t, " permit tcp 192.0.2.0 0.0.0.255 any eq 80\n deny tcp 192.0.2.0 0.0.1.255 any eq 80\n permit tcp any any eq 80\n")
	r, err := AuditACL(context.Background(), acl, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 0 {
		t.Fatalf("false finding: %+v", r.Findings)
	}
	checkAuditWitnesses(t, acl, r)
}

func TestAuditPartialCoverIsNotRedundant(t *testing.T) {
	acl := parse(t, " permit tcp 192.0.2.0 0.0.0.255 any eq 80\n permit tcp 192.0.2.0 0.0.0.127 any eq 80\n")
	r, err := AuditACL(context.Background(), acl, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(auditFindings(r, AuditRedundant)) != 0 {
		t.Fatalf("false redundant finding: %+v", r.Findings)
	}
	un := auditFindings(r, AuditUnreachable)
	if len(un) != 1 || un[0].Rule != 1 {
		t.Fatalf("unreachable findings: %+v", un)
	}
}

func TestAuditRelationsExact(t *testing.T) {
	gen := parse(t, " permit tcp host 192.0.2.10 any eq 22\n deny tcp 192.0.2.0 0.0.0.255 any eq 22\n")
	r, err := AuditACL(context.Background(), gen, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Relations) != 1 || r.Relations[0].Kind != AuditGeneralization || r.Relations[0].Earlier != 0 || r.Relations[0].Later != 1 {
		t.Fatalf("generalization: %+v", r.Relations)
	}
	if r.Relations[0].Witness.Decision.RuleOrdinal == nil || *r.Relations[0].Witness.Decision.RuleOrdinal != 0 {
		t.Fatalf("generalization witness not decided by the exception rule: %+v", r.Relations[0].Witness)
	}
	checkAuditWitnesses(t, gen, r)

	// Partial overlap needs non-aligned wildcards: rule 0 matches
	// last octets 0-127, rule 1 matches the even last octets — the
	// overlap is the even octets below 128, and each rule also has
	// packets the other never matches.
	corr := parse(t, " permit tcp 192.0.2.0 0.0.0.127 any eq 80\n deny tcp 192.0.2.0 0.0.0.254 any eq 80\n")
	r, err = AuditACL(context.Background(), corr, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Relations) != 1 || r.Relations[0].Kind != AuditCorrelation {
		t.Fatalf("correlation: %+v", r.Relations)
	}
	checkAuditWitnesses(t, corr, r)

	// Same-action nesting is redundancy territory, not a relation.
	same := parse(t, " permit tcp host 192.0.2.10 any eq 22\n permit tcp any any eq 22\n")
	r, err = AuditACL(context.Background(), same, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Relations) != 0 {
		t.Fatalf("same-action relation claimed: %+v", r.Relations)
	}

	// Equal matches with different actions: the later rule is
	// unreachable; no relation is claimed on top of the finding.
	eq := parse(t, " permit tcp any any eq 22\n deny tcp any any eq 22\n")
	r, err = AuditACL(context.Background(), eq, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Relations) != 0 {
		t.Fatalf("equal-match relation claimed: %+v", r.Relations)
	}
	if un := auditFindings(r, AuditUnreachable); len(un) != 1 || un[0].Rule != 1 {
		t.Fatalf("equal-match unreachable: %+v", un)
	}
	checkAuditWitnesses(t, eq, r)
}

func TestAuditBudgetIncomplete(t *testing.T) {
	acl := parse(t, " permit tcp any any eq 22\n permit tcp any any eq 80\n permit tcp any any eq 443\n deny ip any any\n")
	opts := DefaultOptions()
	opts.Limits.MaxOperations = 5
	r, err := AuditACL(context.Background(), acl, opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || r.Reason != "operations" || r.Stage != AuditStageEffective || r.StageRule == nil {
		t.Fatalf("incomplete shape: %+v", r)
	}
	if r.RelationsComplete || len(r.Relations) != 0 {
		t.Fatalf("relations claimed on stopped audit: %+v", r)
	}
	for _, f := range r.Findings {
		if f.Kind != AuditUnreachable || f.Rule > *r.StageRule {
			t.Fatalf("finding past the stop: %+v (stage rule %d)", f, *r.StageRule)
		}
	}
	r2, err := AuditACL(context.Background(), acl, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, r2) {
		t.Fatal("incomplete audit not deterministic")
	}
}

func TestAuditDeterministic(t *testing.T) {
	acl := parse(t, " permit tcp host 192.0.2.10 any eq 22\n deny tcp 192.0.2.0 0.0.0.255 any eq 22\n permit tcp any any eq 80\n deny ip any any\n")
	first, err := AuditACL(context.Background(), acl, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		next, err := AuditACL(context.Background(), acl, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatal("audit report differs across runs")
		}
	}
	if !first.Complete || len(first.Findings) == 0 || len(first.Relations) == 0 {
		t.Fatalf("fixture should exercise findings and relations: %+v", first)
	}
}

func TestAuditCancellation(t *testing.T) {
	acl := parse(t, " permit tcp any any eq 22\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := AuditACL(ctx, acl, DefaultOptions()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

// TestAuditReducedDomainOracle cross-checks the audit against brute
// force over a reduced packet domain, in the style of
// TestReducedDomainOracle. Generated rules only test the low two bits
// of the last address octet (values and wildcards in 0..3) and
// destination-port sets inside 0..3, so the packet space partitions
// into finitely many classes represented by the grid: address low
// bytes and ports in 0..3 plus the representative 4 (any higher value
// is rejected by every generated cube's care bits and lies outside
// every generated port set). Brute force decides shadowing,
// redundancy (by actually removing the rule and re-evaluating) and
// the pair relations from the linear evaluator alone.
func TestAuditReducedDomainOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(4242))
	addr := func(base string) string {
		return fmt.Sprintf("%s.%d 0.0.0.%d", base, rng.Intn(4), rng.Intn(4))
	}
	makeLines := func() []string {
		lines := []string{}
		for i := 0; i < 6; i++ {
			action := "deny"
			if rng.Intn(2) == 0 {
				action = "permit"
			}
			proto := []string{"tcp", "udp", "icmp", "ip"}[rng.Intn(4)]
			var sb strings.Builder
			fmt.Fprintf(&sb, " %s %s %s %s", action, proto, addr("192.0.2"), addr("198.51.100"))
			if proto == "tcp" || proto == "udp" {
				switch rng.Intn(3) {
				case 1:
					fmt.Fprintf(&sb, " eq %d", rng.Intn(4))
				case 2:
					lo, hi := rng.Intn(4), rng.Intn(4)
					if lo > hi {
						lo, hi = hi, lo
					}
					fmt.Fprintf(&sb, " range %d %d", lo, hi)
				}
			}
			lines = append(lines, sb.String())
		}
		return lines
	}
	build := func(lines []string) ciscoacl.ACL {
		return parse(t, strings.Join(lines, "\n")+"\n")
	}
	type gridPacket struct {
		p       ciscoacl.Packet
		decider int
		action  string
	}
	for seed := 0; seed < 30; seed++ {
		lines := makeLines()
		acl := build(lines)
		n := len(lines)
		singles := make([]ciscoacl.ACL, n)
		for i := range lines {
			singles[i] = build(lines[i : i+1])
		}
		grid := []gridPacket{}
		for _, proto := range []uint8{1, 6, 17} {
			for s := 0; s <= 4; s++ {
				for d := 0; d <= 4; d++ {
					ports := []uint16{0}
					if proto == 6 || proto == 17 {
						ports = []uint16{0, 1, 2, 3, 4}
					}
					for _, dp := range ports {
						p := ciscoacl.Packet{Protocol: proto, SrcIP: netip.AddrFrom4([4]byte{192, 0, 2, byte(s)}), DstIP: netip.AddrFrom4([4]byte{198, 51, 100, byte(d)}), DstPort: dp}
						dec, err := acl.Evaluate(p)
						if err != nil {
							t.Fatal(err)
						}
						decider := -1
						if !dec.Implicit && dec.RuleOrdinal != nil {
							decider = *dec.RuleOrdinal
						}
						grid = append(grid, gridPacket{p, decider, dec.Action})
					}
				}
			}
		}
		match := make([][]bool, n)
		for i := 0; i < n; i++ {
			match[i] = make([]bool, len(grid))
			for k, g := range grid {
				dec, err := singles[i].Evaluate(g.p)
				if err != nil {
					t.Fatal(err)
				}
				match[i][k] = !dec.Implicit
			}
		}
		r, err := AuditACL(context.Background(), acl, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if !r.Complete {
			t.Fatalf("seed %d: audit incomplete on reduced domain: %+v", seed, r)
		}
		gotUnreachable := map[int]bool{}
		gotRedundant := map[int]bool{}
		for _, f := range r.Findings {
			switch f.Kind {
			case AuditUnreachable:
				gotUnreachable[f.Rule] = true
			case AuditRedundant:
				gotRedundant[f.Rule] = true
			default:
				t.Fatalf("seed %d: unknown finding kind %q", seed, f.Kind)
			}
		}
		for i := 0; i < n; i++ {
			effEmpty := true
			for _, g := range grid {
				if g.decider == i {
					effEmpty = false
					break
				}
			}
			if gotUnreachable[i] != effEmpty {
				t.Fatalf("seed %d rule %d: audit unreachable=%v, brute force=%v\n%s", seed, i, gotUnreachable[i], effEmpty, strings.Join(lines, "\n"))
			}
			// Brute-force redundancy: remove the rule, re-parse, and
			// compare every grid packet's action.
			rest := append(append([]string{}, lines[:i]...), lines[i+1:]...)
			removed := build(rest)
			redundant := true
			for _, g := range grid {
				dec, err := removed.Evaluate(g.p)
				if err != nil {
					t.Fatal(err)
				}
				if dec.Action != g.action {
					redundant = false
					break
				}
			}
			wantRedundant := redundant && !effEmpty
			if gotRedundant[i] != wantRedundant {
				t.Fatalf("seed %d rule %d: audit redundant=%v, brute force=%v\n%s", seed, i, gotRedundant[i], wantRedundant, strings.Join(lines, "\n"))
			}
		}
		type relKey struct {
			kind string
			i, j int
		}
		wantRelations := map[relKey]bool{}
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if acl.Rules[i].Action == acl.Rules[j].Action {
					continue
				}
				inter, subIJ, subJI := false, true, true
				for k := range grid {
					if match[i][k] && match[j][k] {
						inter = true
					}
					if match[i][k] && !match[j][k] {
						subIJ = false
					}
					if match[j][k] && !match[i][k] {
						subJI = false
					}
				}
				switch {
				case subIJ && !subJI:
					wantRelations[relKey{AuditGeneralization, i, j}] = true
				case inter && !subIJ && !subJI:
					wantRelations[relKey{AuditCorrelation, i, j}] = true
				}
			}
		}
		gotRelations := map[relKey]bool{}
		for _, rel := range r.Relations {
			gotRelations[relKey{rel.Kind, rel.Earlier, rel.Later}] = true
		}
		if !reflect.DeepEqual(gotRelations, wantRelations) {
			t.Fatalf("seed %d: relations audit=%v brute=%v\n%s", seed, gotRelations, wantRelations, strings.Join(lines, "\n"))
		}
		checkAuditWitnesses(t, acl, r)
	}
}

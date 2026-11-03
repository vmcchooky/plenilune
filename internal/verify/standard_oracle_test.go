package verify_test

import (
	"context"
	"fmt"
	"math/rand"
	"net/netip"
	"strings"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/verify"
)

// Independent reduced-domain model: only the low four source bits vary; every
// modeled rule fixes all higher bits to 192.0.2.0/28. Outside it, implicit deny
// is universal. Expected answers never use production parsing/canonicalization.
type stdOracleRule struct {
	address, wildcard byte
	permit            bool
}
type stdOracleDoc struct {
	rules         []stdOracleRule
	text          string
	line, ordinal []int // model-rule index -> physical line / evaluation index
	byOrdinal     []int
}

func stdOracleDecision(rules []stdOracleRule, source byte, skip int) (bool, int) {
	for index, rule := range rules {
		if index != skip && rule.wildcard == 0 && source == rule.address {
			return rule.permit, index
		}
	}
	for index, rule := range rules { // model list is the declared sequence order
		if index != skip && rule.wildcard != 0 && (source^rule.address) & ^rule.wildcard == 0 {
			return rule.permit, index
		}
	}
	return false, -1
}

func stdOracleSource(t *testing.T, random *rand.Rand, style int) stdOracleDoc {
	t.Helper()
	var rules []stdOracleRule
	hosts := random.Perm(16)
	for i := 0; i < 6; i++ {
		mask := []byte{1, 2, 3, 5, 7, 15}[random.Intn(6)]
		address := byte(random.Intn(16))
		if i%2 == 0 {
			mask, address = 0, byte(hosts[i/2])
		}
		rules = append(rules, stdOracleRule{address, mask, random.Intn(2) == 0})
	}
	return stdOracleRender(random, style, rules)
}

func stdOracleRender(random *rand.Rand, style int, rules []stdOracleRule) stdOracleDoc {
	model := stdOracleDoc{rules: append([]stdOracleRule(nil), rules...), line: make([]int, len(rules)), ordinal: make([]int, len(rules))}
	physical := random.Perm(6)
	if style == 2 {
		physical = []int{0, 1, 2, 3, 4, 5}
	}
	var text strings.Builder
	if style != 2 {
		text.WriteString("ip access-list standard 7\n")
	}
	for position, i := range physical {
		rule := model.rules[i]
		action := "deny"
		if rule.permit {
			action = "permit"
		}
		operand := fmt.Sprintf("192.0.2.%d 0.0.0.%d", rule.address, rule.wildcard)
		if rule.wildcard == 0 && i%3 == 0 {
			operand = fmt.Sprintf("host 192.0.2.%d", rule.address)
		}
		if rule.wildcard == 0 && i%3 == 1 {
			operand = fmt.Sprintf("192.0.2.%d", rule.address)
		}
		if style == 2 {
			fmt.Fprintf(&text, "access-list 7 %s %s\n", action, operand)
			model.line[i] = position + 1
		} else {
			fmt.Fprintf(&text, " %d %s %s\n", 10*(i+1), action, operand)
			model.line[i] = position + 2
		}
	}
	// Exact hosts are disjoint. Their stable display order follows physical
	// occurrence; network order follows declared sequences, not physical layout.
	for _, i := range physical {
		if model.rules[i].wildcard == 0 {
			model.byOrdinal = append(model.byOrdinal, i)
		}
	}
	for i, rule := range model.rules {
		if rule.wildcard != 0 {
			model.byOrdinal = append(model.byOrdinal, i)
		}
	}
	for ordinal, index := range model.byOrdinal {
		model.ordinal[index] = ordinal
	}
	model.text = text.String()
	return model
}

func stdOracleCheckDecision(t *testing.T, model stdOracleDoc, packet ciscoacl.Packet, actual ciscoacl.Decision) {
	t.Helper()
	address := packet.SrcIP.As4()
	permit, index := false, -1
	if address[0] == 192 && address[1] == 0 && address[2] == 2 && address[3] < 16 {
		permit, index = stdOracleDecision(model.rules, address[3], -1)
	}
	if (actual.Action == "permit") != permit || actual.Implicit != (index < 0) {
		t.Fatalf("oracle decision mismatch: packet=%+v decision=%+v", packet, actual)
	}
	if index < 0 {
		if actual.Line != nil || actual.RuleOrdinal != nil {
			t.Fatal("implicit decision carries a source reference")
		}
	} else if actual.Line == nil || *actual.Line != model.line[index] || actual.RuleOrdinal == nil || *actual.RuleOrdinal != model.ordinal[index] {
		t.Fatalf("oracle source binding mismatch: model index=%d line=%d ordinal=%d got=%+v", index, model.line[index], model.ordinal[index], actual)
	}
}

func TestStandardIndependentReducedDomainOracle(t *testing.T) {
	random := rand.New(rand.NewSource(35003))
	ctx := context.Background()
	for trial := 0; trial < 72; trial++ {
		before, after := stdOracleSource(t, random, trial%3), stdOracleSource(t, random, (trial+1)%3)
		if trial%9 == 0 {
			after = stdOracleRender(random, (trial+1)%3, before.rules)
		}
		if trial%9 == 1 || trial%9 == 2 {
			for i := range after.rules {
				after.rules[i].permit = trial%9 == 1
			}
			after.rules[5].address, after.rules[5].wildcard = 0, 15
			after = stdOracleRender(random, (trial+1)%3, after.rules)
		}
		b, err := ciscoacl.Parse([]byte(before.text))
		if err != nil {
			t.Fatal(err)
		}
		a, err := ciscoacl.Parse([]byte(after.text))
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range []string{"192.0.2.16", "192.0.3.1", "203.0.113.1"} {
			packet := ciscoacl.Packet{Protocol: 6, SrcIP: netip.MustParseAddr(source), DstIP: netip.MustParseAddr("198.51.100.1")}
			for _, side := range []struct {
				model stdOracleDoc
				acl   ciscoacl.ACL
			}{{before, b}, {after, a}} {
				decision, err := side.acl.Evaluate(packet)
				if err != nil {
					t.Fatal(err)
				}
				stdOracleCheckDecision(t, side.model, packet, decision)
			}
		}
		different, allPermit, allDeny := false, true, true
		pairs := map[[2]int]bool{}
		for src := byte(0); src < 16; src++ {
			bp, bi := stdOracleDecision(before.rules, src, -1)
			ap, ai := stdOracleDecision(after.rules, src, -1)
			different, allPermit, allDeny = different || bp != ap, allPermit && ap, allDeny && !ap
			if bp != ap {
				bo, ao := -1, -1
				if bi >= 0 {
					bo = before.ordinal[bi]
				}
				if ai >= 0 {
					ao = after.ordinal[ai]
				}
				pairs[[2]int{bo, ao}] = true
			}
			for _, protocol := range []uint8{0, 1, 6, 17, 47, 255} {
				for _, dst := range []byte{1, 200} {
					ports := []uint16{0}
					if protocol == 6 || protocol == 17 {
						ports = []uint16{0, 22, 65535}
					}
					for _, port := range ports {
						packet := ciscoacl.Packet{Protocol: protocol, SrcIP: netip.AddrFrom4([4]byte{192, 0, 2, src}), DstIP: netip.AddrFrom4([4]byte{198, 51, 100, dst}), SrcPort: port, DstPort: port}
						bd, e := b.Evaluate(packet)
						if e != nil {
							t.Fatal(e)
						}
						stdOracleCheckDecision(t, before, packet, bd)
						ad, e := a.Evaluate(packet)
						if e != nil {
							t.Fatal(e)
						}
						stdOracleCheckDecision(t, after, packet, ad)
					}
				}
			}
		}
		set := intent.Set{Schema: intent.Schema, Scope: intent.Scope}
		for i, kind := range []string{"must_permit", "must_deny", "must_remain"} {
			set.Obligations = append(set.Obligations, intent.Obligation{ID: fmt.Sprintf("OB-%d", i+1), Kind: kind,
				Match: intent.Region{Protocol: "ip", SrcCIDR: "192.0.2.0/28", DstCIDR: "0.0.0.0/0"}})
		}
		report, err := verify.Compare(ctx, b, a, &set, verify.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if !report.SearchComplete || (report.Verdict == "DIFFERENT") != different || (report.Verdict == "EQUIVALENT") == different {
			t.Fatalf("trial %d verdict=%s want different=%t", trial, report.Verdict, different)
		}
		for i, holds := range []bool{allPermit, allDeny, !different} {
			if (report.Obligations[i].Status == "HOLDS") != holds || report.Obligations[i].Status == "INCONCLUSIVE" {
				t.Fatalf("trial %d obligation %d: %+v", trial, i, report.Obligations[i])
			}
		}
		checkWitness := func(w verify.Witness) {
			stdOracleCheckDecision(t, before, w.Packet, w.Before)
			stdOracleCheckDecision(t, after, w.Packet, w.After)
		}
		for _, w := range report.Witnesses {
			checkWitness(w)
		}
		for _, result := range report.Obligations {
			if result.Witness != nil {
				checkWitness(*result.Witness)
			}
		}
		changes, err := verify.Changes(ctx, b, a, nil, verify.DefaultChangeOptions())
		if err != nil {
			t.Fatal(err)
		}
		if !changes.Complete || changes.ClassesTruncated || len(changes.Classes) != len(pairs) {
			t.Fatalf("trial %d class coverage=%d expected=%d complete=%t", trial, len(changes.Classes), len(pairs), changes.Complete)
		}
		for _, class := range changes.Classes {
			checkWitness(class.Witness)
			bo, ao := -1, -1
			if class.Before.Ordinal != nil {
				bo = *class.Before.Ordinal
			}
			if class.After.Ordinal != nil {
				ao = *class.After.Ordinal
			}
			if !pairs[[2]int{bo, ao}] {
				t.Fatalf("trial %d unproved class pair %d/%d", trial, bo, ao)
			}
			delete(pairs, [2]int{bo, ao})
		}
		if len(pairs) != 0 {
			t.Fatal("missing independently expected class")
		}
		stdOracleAudit(t, after, a)
		if trial < 12 {
			stdOraclePartial(t, before, after, b, a)
		}
	}
}

func stdOracleAudit(t *testing.T, model stdOracleDoc, acl ciscoacl.ACL) {
	t.Helper()
	report, err := verify.AuditACL(context.Background(), acl, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete {
		t.Fatal("small standard audit did not complete")
	}
	findings := map[[2]int]bool{}
	for _, f := range report.Findings {
		kind := 0
		if f.Kind == verify.AuditRedundant {
			kind = 1
		}
		findings[[2]int{f.Rule, kind}] = true
		if f.Line != model.line[model.byOrdinal[f.Rule]] {
			t.Fatal("audit line is not physical source line")
		}
		if f.Witness != nil {
			stdOracleCheckDecision(t, model, f.Witness.Packet, f.Witness.Decision)
		}
	}
	for index := range model.rules {
		reachable, redundant := false, true
		for src := byte(0); src < 16; src++ {
			original, deciding := stdOracleDecision(model.rules, src, -1)
			removed, _ := stdOracleDecision(model.rules, src, index)
			reachable, redundant = reachable || deciding == index, redundant && original == removed
		}
		ordinal := model.ordinal[index]
		if findings[[2]int{ordinal, 0}] != !reachable || findings[[2]int{ordinal, 1}] != (reachable && redundant) {
			t.Fatalf("audit ordinal %d reachable=%t redundant=%t findings=%v", ordinal, reachable, redundant, findings)
		}
	}
}

func stdOraclePartial(t *testing.T, before, after stdOracleDoc, b, a ciscoacl.ACL) {
	t.Helper()
	for _, operations := range []int{1, 10, 50, 150} {
		opts := verify.DefaultOptions()
		opts.Limits.MaxOperations = operations
		report, err := verify.Compare(context.Background(), b, a, nil, opts)
		if err != nil {
			t.Fatal(err)
		}
		if report.Budget.StopReason != "" && (report.SearchComplete || report.Verdict == "EQUIVALENT") {
			t.Fatal("budget stop upgraded to equivalence")
		}
		for _, w := range report.Witnesses {
			stdOracleCheckDecision(t, before, w.Packet, w.Before)
			stdOracleCheckDecision(t, after, w.Packet, w.After)
		}
		if report.Partial != nil {
			for _, part := range report.Partial.Undecided {
				for side, span := range []*[2]int{part.BeforeRules, part.AfterRules} {
					limit := len(b.Rules)
					if side == 1 {
						limit = len(a.Rules)
					}
					if span != nil && (span[0] < 0 || span[1] < span[0] || span[1] >= limit) {
						t.Fatal("partial range is not a canonical ordinal range")
					}
				}
			}
		}
	}
}

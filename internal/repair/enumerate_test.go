package repair

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/patch"
	"plenilune/internal/verify"
)

func findCandidate(e Enumeration, transform, target string) *Candidate {
	for i := range e.Candidates {
		if e.Candidates[i].Transform == transform && e.Candidates[i].Target == target {
			return &e.Candidates[i]
		}
	}
	return nil
}

func anyVerified(e Enumeration) bool {
	for _, c := range e.Candidates {
		if c.Result.Status == "VERIFIED" {
			return true
		}
	}
	return false
}

func TestEnumerateFixtureCase(t *testing.T) {
	b, a, p, knownGood := fixture(t)
	e, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !e.Complete || e.Targets != 2 {
		t.Fatalf("enumeration: complete=%v targets=%d", e.Complete, e.Targets)
	}
	if err := CheckEnumeration(e); err != nil {
		t.Fatal(err)
	}
	if !e.Sensitive {
		t.Fatal("enumeration embedding candidate text must be labeled sensitive")
	}
	// The hand-authored repair from the pre-enumeration flow (remove the
	// denying rule) is exactly one enumerated remove candidate — the
	// class and obligation transforms generate the same request, and
	// deduplication keeps the first (class) entry — and it verifies.
	rm := findCandidate(e, TransformRemoveRule, "class:0:0")
	if rm == nil || rm.Result.Status != "VERIFIED" {
		t.Fatalf("remove candidate: %+v", rm)
	}
	if !reflect.DeepEqual(rm.Request.Edits, knownGood.Edits) {
		t.Fatalf("enumerated remove differs from hand-authored repair: %+v vs %+v", rm.Request.Edits, knownGood.Edits)
	}
	// Inserting a permit exception for exactly the violation region,
	// immediately before the denying rule, also verifies.
	ins := findCandidate(e, TransformInsertException, "class:0:0")
	if ins == nil || ins.Result.Status != "VERIFIED" {
		t.Fatalf("insert candidate: %+v", ins)
	}
	if !strings.Contains(ins.Request.Edits[0].Content, "permit tcp host 192.0.2.10 host 198.51.100.10 eq 22") {
		t.Fatalf("insert content: %q", ins.Request.Edits[0].Content)
	}
	// Flipping the denying rule to a broad permit breaks collateral
	// (it permits SSH to every destination, not just the managed host):
	// shown as REJECTED with its reason, never as a success.
	flip := findCandidate(e, TransformFlipRule, "class:0:0")
	if flip == nil || flip.Result.Status != "REJECTED" || flip.Result.Reason != "collateral_changed" {
		t.Fatalf("flip candidate: %+v", flip)
	}
	if !anyVerified(e) {
		t.Fatal("no verified candidate in a fixable case")
	}
}

func TestEnumerateDeterministicAcrossRunsAndProcs(t *testing.T) {
	b, a, p, _ := fixture(t)
	base, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	baseJSON, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		got, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if got.SHA256 != base.SHA256 {
			t.Fatalf("run %d digest differs", i)
		}
		gotJSON, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotJSON) != string(baseJSON) {
			t.Fatalf("run %d record bytes differ", i)
		}
	}
	old := runtime.GOMAXPROCS(1)
	got, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions())
	runtime.GOMAXPROCS(old)
	if err != nil || got.SHA256 != base.SHA256 {
		t.Fatalf("GOMAXPROCS=1 differs: %v", err)
	}
}

func TestEnumerateExternalProposalsFoldIntoFrame(t *testing.T) {
	b, a, p, knownGood := fixture(t)
	// The known-good proposal is byte-identical to an enumerated
	// candidate: deduplication keeps the engine-enumerated entry, so a
	// proposal gains nothing by being submitted.
	e, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions(), knownGood, knownGood)
	if err != nil {
		t.Fatal(err)
	}
	externals := 0
	for _, c := range e.Candidates {
		if c.Transform == TransformExternal {
			externals++
		}
	}
	if externals != 0 {
		t.Fatalf("duplicate proposal survived dedup: %d external candidates", externals)
	}
	// A distinct proposal enters the same frame once: deduplicated,
	// verified by the same gate, sorted by the same key.
	afterHash := evidence.ConfigDigest(a)
	flip := patch.Request{Schema: patch.Schema, Scope: intent.Scope, BaseBeforeSHA256: evidence.ConfigDigest(b), BaseAfterSHA256: afterHash, ObligationsDigest: knownGood.ObligationsDigest, Edits: []patch.Edit{{Op: "replace", Ref: &patch.RuleRef{BaseAfterSHA256: afterHash, Ordinal: 2, Line: 5}, Content: "deny tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22"}}}
	e2, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions(), flip, flip)
	if err != nil {
		t.Fatal(err)
	}
	ext := findCandidate(e2, TransformExternal, "external")
	if ext == nil {
		t.Fatal("distinct external proposal missing")
	}
	if err := CheckEnumeration(e2); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, c := range e2.Candidates {
		if c.Transform == TransformExternal {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("external candidates = %d, want 1 after dedup", count)
	}
}

func TestEnumerateNoVerifiedWithoutAcceptance(t *testing.T) {
	b, a, p, _ := fixture(t)
	p.OperatorAccepted = false
	e, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Candidates) == 0 {
		t.Fatal("no candidates generated for an unaccepted policy")
	}
	reachedAcceptance := false
	for _, c := range e.Candidates {
		if c.Result.Status == "VERIFIED" {
			t.Fatal("candidate VERIFIED under an unaccepted policy")
		}
		if c.Result.Reason == "policy_not_accepted" {
			reachedAcceptance = true
		}
	}
	// Candidates that pass every other check must land on the acceptance
	// gate and stop there; candidates failing earlier checks keep their
	// earlier reasons (the gate order is Verify's, unchanged).
	if !reachedAcceptance {
		t.Fatal("no candidate reached the acceptance gate")
	}
}

func TestEnumeratePolicyMissing(t *testing.T) {
	b, a, _, _ := fixture(t)
	e, err := Enumerate(context.Background(), b, a, nil, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !e.Complete || len(e.Candidates) == 0 {
		t.Fatalf("enumeration: complete=%v candidates=%d", e.Complete, len(e.Candidates))
	}
	for _, c := range e.Candidates {
		if c.Result.Status != "UNVERIFIED" || c.Result.Reason != "policy_missing" {
			t.Fatalf("candidate without policy: %+v", c.Result)
		}
	}
}

func TestEnumerateNothingToRepair(t *testing.T) {
	b, _, p, _ := fixture(t)
	e, err := Enumerate(context.Background(), b, b, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !e.Complete || e.Targets != 0 || len(e.Candidates) != 0 {
		t.Fatalf("enumeration: %+v", e)
	}
	if err := CheckEnumeration(e); err != nil {
		t.Fatal(err)
	}
}

func TestEnumerateContradictoryObligationsRejectEverything(t *testing.T) {
	before := []byte("ip access-list extended T\n 10 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 20 deny ip any any\n")
	after := []byte("ip access-list extended T\n 10 permit tcp any host 198.51.100.10 eq 22\n 20 deny ip any any\n")
	set := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{
		{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}},
		{ID: "OB-2", Kind: "must_deny", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.10/32", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}},
	}}
	// OB-1 forces 192.0.2.10 to be permitted and OB-2 forces it to be
	// denied: no candidate can satisfy both, whatever the transform.
	p := Policy{Schema: PolicySchema, Scope: intent.Scope, OperatorAccepted: true, Conditions: set, AllowedChanges: []intent.Region{}}
	e, err := Enumerate(context.Background(), before, after, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Candidates) == 0 {
		t.Fatal("no candidates generated for a violated obligation")
	}
	if anyVerified(e) {
		t.Fatal("a candidate verified under contradictory obligations")
	}
}

func TestEnumerateReorderSwap(t *testing.T) {
	// Before: the broad permit decides SSH to the managed host (the
	// obligation holds); the host deny below it only governs SSH to
	// other destinations. After swaps the two rules, so the host deny
	// now shadows the permit for the managed host.
	before := []byte("ip access-list extended SW\n 10 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 20 deny tcp host 192.0.2.10 any eq 22\n 30 deny ip any any\n")
	after := []byte("ip access-list extended SW\n 10 deny tcp host 192.0.2.10 any eq 22\n 20 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 30 deny ip any any\n")
	_, _, p, _ := fixture(t)
	e, err := Enumerate(context.Background(), before, after, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	re := findCandidate(e, TransformReorderSwap, "class:0:0")
	if re == nil {
		t.Fatal("no reorder candidate for a pure rule swap")
	}
	if re.Result.Status != "VERIFIED" {
		t.Fatalf("reorder candidate: %+v", re.Result)
	}
	if len(re.Request.Edits) != 2 {
		t.Fatalf("reorder edits = %d, want remove+add", len(re.Request.Edits))
	}
}

func TestEnumerateIncompleteOnStarvedBudget(t *testing.T) {
	b, a, p, _ := fixture(t)
	opts := verify.Options{Limits: packetset.Limits{MaxOperations: 5, MaxBoxes: 5, Timeout: time.Second}, MaxWitnesses: 8}
	e, err := Enumerate(context.Background(), b, a, &p, opts)
	if err != nil {
		t.Fatal(err)
	}
	if e.Complete || e.Reason == "" || len(e.Candidates) != 0 {
		t.Fatalf("enumeration: %+v", e)
	}
	if err := CheckEnumeration(e); err != nil {
		t.Fatal(err)
	}
}

func TestReplayEnumeration(t *testing.T) {
	b, a, p, _ := fixture(t)
	e, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	res, err := ReplayEnumeration(context.Background(), b, a, &p, e)
	if err != nil || res.Status != "ENUMERATION_REPLAYED" || res.SHA256 != e.SHA256 {
		t.Fatalf("replay: %v %+v", err, res)
	}
	tampered := e
	tampered.Targets++
	if _, err := ReplayEnumeration(context.Background(), b, a, &p, tampered); err == nil {
		t.Fatal("tampered enumeration replayed")
	}
	if _, err := ReplayEnumeration(context.Background(), b, b, &p, e); err == nil {
		t.Fatal("enumeration replayed against different inputs")
	}
}

// Differential against the pre-P1-3 flow: the old behavior was a human
// (or AI) authoring one patch and calling Verify. For seeded mutations
// whose hand-authored inverse patch verifies, the deterministic
// enumeration must also surface at least one VERIFIED candidate.
func TestEnumerateSeededDifferential(t *testing.T) {
	rules := []string{
		"permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22",
		"deny tcp host 192.0.2.10 any eq 23",
		"permit udp 192.0.2.0 0.0.0.255 host 198.51.100.53 eq 53",
		"deny ip any any",
	}
	render := func(rs []string) []byte {
		var sb strings.Builder
		sb.WriteString("ip access-list extended T\n")
		for i, r := range rs {
			fmt.Fprintf(&sb, " %d %s\n", (i+1)*10, r)
		}
		return []byte(sb.String())
	}
	before := render(rules)
	set := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{
		{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}},
	}}
	_, digest, err := intent.Digest(set)
	if err != nil {
		t.Fatal(err)
	}
	policy := Policy{Schema: PolicySchema, Scope: intent.Scope, OperatorAccepted: true, Conditions: set, AllowedChanges: []intent.Region{}}
	rng := rand.New(rand.NewPCG(42, 7))
	checked, skipped := 0, 0
	for i := 0; i < 24; i++ {
		k := rng.IntN(3)
		afterRules := append([]string(nil), rules...)
		var manual patch.Request
		afterHash := ""
		build := func() []byte {
			after := render(afterRules)
			afterHash = evidence.ConfigDigest(after)
			return after
		}
		var after []byte
		if i%2 == 0 {
			// Flip rule k's action; the manual repair flips it back.
			flipped := afterRules[k]
			if strings.HasPrefix(flipped, "permit") {
				flipped = "deny" + flipped[len("permit"):]
			} else {
				flipped = "permit" + flipped[len("deny"):]
			}
			afterRules[k] = flipped
			after = build()
			parsed, err := ciscoacl.Parse(after)
			if err != nil {
				t.Fatal(err)
			}
			r := parsed.Rules[k]
			manual = patch.Request{Schema: patch.Schema, Scope: intent.Scope, BaseBeforeSHA256: evidence.ConfigDigest(before), BaseAfterSHA256: afterHash, ObligationsDigest: digest, Edits: []patch.Edit{{Op: "replace", Ref: &patch.RuleRef{BaseAfterSHA256: afterHash, Ordinal: r.Ordinal, Line: r.Line}, Content: rules[k]}}}
		} else {
			// Remove rule k; the manual repair re-inserts it.
			removed := afterRules[k]
			afterRules = append(afterRules[:k], afterRules[k+1:]...)
			after = build()
			parsed, err := ciscoacl.Parse(after)
			if err != nil {
				t.Fatal(err)
			}
			if k < len(parsed.Rules) {
				r := parsed.Rules[k]
				manual = patch.Request{Schema: patch.Schema, Scope: intent.Scope, BaseBeforeSHA256: evidence.ConfigDigest(before), BaseAfterSHA256: afterHash, ObligationsDigest: digest, Edits: []patch.Edit{{Op: "add_before", Ref: &patch.RuleRef{BaseAfterSHA256: afterHash, Ordinal: r.Ordinal, Line: r.Line}, Content: removed}}}
			} else {
				manual = patch.Request{Schema: patch.Schema, Scope: intent.Scope, BaseBeforeSHA256: evidence.ConfigDigest(before), BaseAfterSHA256: afterHash, ObligationsDigest: digest, Edits: []patch.Edit{{Op: "append", Content: removed}}}
			}
		}
		mr, err := Verify(context.Background(), before, after, &policy, manual, verify.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		e, err := Enumerate(context.Background(), before, after, &policy, verify.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		e2, err := Enumerate(context.Background(), before, after, &policy, verify.DefaultOptions())
		if err != nil || e2.SHA256 != e.SHA256 {
			t.Fatalf("case %d: enumeration not deterministic", i)
		}
		if mr.Status != "VERIFIED" || e.Targets == 0 {
			// Either the mutation is not repairable by hand either, or
			// it changed no behavior and violated no obligation (for
			// example removing a deny rule whose packets fall through
			// to the final deny): enumeration correctly has no targets,
			// and a trivially-verifying no-op repair proves nothing.
			skipped++
			continue
		}
		checked++
		if !anyVerified(e) {
			t.Fatalf("case %d (rule %d): manual repair verifies but enumeration found none; targets=%d candidates=%d", i, k, e.Targets, len(e.Candidates))
		}
	}
	if checked < 8 {
		t.Fatalf("differential exercised only %d meaningful cases", checked)
	}
	t.Logf("meaningful manual-verified cases: %d, skipped: %d", checked, skipped)
}

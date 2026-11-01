package cli

import (
	"encoding/json"

	"testing"

	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/patch"
	"plenilune/internal/repair"
)

func enumerateFixture(t *testing.T) (string, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	beforeText := "ip access-list extended DEMO\n permit tcp any any eq 22\n deny ip any any\n"
	afterText := "ip access-list extended DEMO\n deny tcp host 192.0.2.10 any eq 22\n permit tcp any any eq 22\n deny ip any any\n"
	b := put(t, dir, "before.acl", beforeText)
	a := put(t, dir, "after.acl", afterText)
	s := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.10/32", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
	_, hash, err := intent.Digest(s)
	if err != nil {
		t.Fatal(err)
	}
	p := repair.Policy{Schema: repair.PolicySchema, Scope: intent.Scope, OperatorAccepted: true, Conditions: s, AllowedChanges: []intent.Region{}}
	pb, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	policyPath := put(t, dir, "policy.json", string(pb))
	afterHash := evidence.ConfigDigest([]byte(afterText))
	q := patch.Request{Schema: patch.Schema, Scope: intent.Scope, BaseBeforeSHA256: evidence.ConfigDigest([]byte(beforeText)), BaseAfterSHA256: afterHash, ObligationsDigest: hash, Edits: []patch.Edit{{Op: "remove", Ref: &patch.RuleRef{BaseAfterSHA256: afterHash, Ordinal: 0, Line: 2}}}}
	qb, err := json.Marshal([]patch.Request{q})
	if err != nil {
		t.Fatal(err)
	}
	proposalsPath := put(t, dir, "proposals.json", string(qb))
	return b, a, policyPath, proposalsPath
}

func TestRepairEnumerateCLI(t *testing.T) {
	b, a, policyPath, proposalsPath := enumerateFixture(t)
	code, body, errout := invoke("repair-enumerate", "--before", b, "--after", a, "--policy", policyPath)
	if code != 0 || errout != "" {
		t.Fatalf("enumerate: %d %s", code, errout)
	}
	var e repair.Enumeration
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	if !e.Complete || len(e.Candidates) == 0 {
		t.Fatalf("enumeration: %+v", e)
	}
	if err := repair.CheckEnumeration(e); err != nil {
		t.Fatal(err)
	}
	verified := false
	for _, c := range e.Candidates {
		if c.Result.Status == "VERIFIED" {
			verified = true
		}
	}
	if !verified {
		t.Fatal("no verified candidate in the CLI enumeration")
	}
	// The same run with the hand-authored fix supplied as a proposal
	// produces the identical record: the proposal deduplicates against
	// the enumerated candidate and cannot change the outcome.
	code2, body2, errout2 := invoke("repair-enumerate", "--before", b, "--after", a, "--policy", policyPath, "--proposals", proposalsPath)
	if code2 != 0 || errout2 != "" || body2 != body {
		t.Fatalf("enumerate with proposals: %d %s (record differs: %v)", code2, errout2, body2 != body)
	}
}

func TestRepairEnumerateCLIUnacceptedPolicy(t *testing.T) {
	b, a, _, _ := enumerateFixture(t)
	// The same policy without operator acceptance.
	s := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.10/32", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
	p := repair.Policy{Schema: repair.PolicySchema, Scope: intent.Scope, OperatorAccepted: false, Conditions: s, AllowedChanges: []intent.Region{}}
	pb, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	unaccepted := put(t, t.TempDir(), "policy.json", string(pb))
	code, body, _ := invoke("repair-enumerate", "--before", b, "--after", a, "--policy", unaccepted)
	if code != 1 {
		t.Fatalf("unaccepted enumerate exit = %d, want 1 (complete, none verified)", code)
	}
	var e repair.Enumeration
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	for _, c := range e.Candidates {
		if c.Result.Status == "VERIFIED" {
			t.Fatal("a candidate verified under an unaccepted policy")
		}
	}
}

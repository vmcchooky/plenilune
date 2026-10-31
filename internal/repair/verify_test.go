package repair

import (
	"context"
	"encoding/json"
	"os"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
	"plenilune/internal/patch"
	"plenilune/internal/verify"
	"strings"
	"testing"
)

func fixture(t *testing.T) ([]byte, []byte, Policy, patch.Request) {
	t.Helper()
	before := []byte("ip access-list extended MGMT\n 10 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 20 deny ip any any\n")
	after, err := os.ReadFile("../../testdata/acl/management.acl")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../testdata/obligations.json")
	if err != nil {
		t.Fatal(err)
	}
	var set intent.Set
	if err := jsonio.Decode(raw, intent.MaxJSONBytes, &set); err != nil {
		t.Fatal(err)
	}
	set, h, err := intent.Digest(set)
	if err != nil {
		t.Fatal(err)
	}
	p := Policy{Schema: PolicySchema, Scope: intent.Scope, OperatorAccepted: true, Conditions: set, AllowedChanges: []intent.Region{}}
	afterHash := evidence.ConfigDigest(after)
	req := patch.Request{Schema: patch.Schema, Scope: intent.Scope, BaseBeforeSHA256: evidence.ConfigDigest(before), BaseAfterSHA256: afterHash, ObligationsDigest: h, Edits: []patch.Edit{{Op: "remove", Ref: &patch.RuleRef{BaseAfterSHA256: afterHash, Ordinal: 0, Line: 3}}}}
	return before, after, p, req
}

func TestVerifiedRepairAndOfflineRerenderReplay(t *testing.T) {
	b, a, p, req := fixture(t)
	r, err := Verify(context.Background(), b, a, &p, req, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "VERIFIED" || r.Collateral.Status != "VERIFIED" || r.Comparison == nil || r.Comparison.Report.Obligations[0].Status != "HOLDS" {
		t.Fatalf("repair: %+v", r)
	}
	if !strings.Contains(r.Candidate, "remark synthetic") || strings.Contains(r.Candidate, "deny tcp host 192.0.2.10") {
		t.Fatal("candidate source rendering wrong")
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var saved Result
	if err := jsonio.Decode(data, MaxJSONBytes, &saved); err != nil {
		t.Fatal(err)
	}
	result, err := Replay(context.Background(), b, a, saved)
	if err != nil || result.Status != "REPAIR_REPLAYED" {
		t.Fatalf("replay: %v %+v", err, result)
	}
	incomplete := saved
	incomplete.SHA256 = ""
	incomplete.Comparison.Report.SearchComplete = false
	if _, err := seal(incomplete); err == nil {
		t.Fatal("sealed a verified receipt with an incomplete comparison")
	}
}

func TestMissingAcceptanceOrAllowedScopeCannotVerify(t *testing.T) {
	b, a, p, req := fixture(t)
	p.OperatorAccepted = false
	r, err := Verify(context.Background(), b, a, &p, req, verify.DefaultOptions())
	if err != nil || r.Status != "UNVERIFIED" || r.Reason != "policy_not_accepted" {
		t.Fatalf("unaccepted: %v %+v", err, r)
	}
	p.OperatorAccepted = true
	p.AllowedChanges = nil
	r, err = Verify(context.Background(), b, a, &p, req, verify.DefaultOptions())
	if err != nil || r.Status != "UNVERIFIED" || r.Collateral.Status != "UNCHECKED" {
		t.Fatalf("undeclared scope: %v %+v", err, r)
	}
	r, err = Verify(context.Background(), b, a, nil, req, verify.DefaultOptions())
	if err != nil || r.Status != "UNVERIFIED" || r.Reason != "policy_missing" {
		t.Fatalf("missing policy: %v %+v", err, r)
	}
}

func TestPinAndUnmetObligationReject(t *testing.T) {
	b, a, p, req := fixture(t)
	req.ObligationsDigest = strings.Repeat("0", 64)
	r, err := Verify(context.Background(), b, a, &p, req, verify.DefaultOptions())
	if err != nil || r.Status != "REJECTED" || r.Candidate != "" {
		t.Fatal("mismatched policy accepted")
	}
	b, a, p, req = fixture(t)
	req.Edits[0].Ref.Ordinal = 2
	req.Edits[0].Ref.Line = 5
	r, err = Verify(context.Background(), b, a, &p, req, verify.DefaultOptions())
	if err != nil || r.Status != "REJECTED" || r.Reason != "obligations_violated" {
		t.Fatalf("failed condition accepted: %v %+v", err, r)
	}
}

func TestTimingDispositionDoesNotExcuseCandidateTampering(t *testing.T) {
	b, a, p, req := fixture(t)
	r, err := Verify(context.Background(), b, a, &p, req, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	// A comment edit leaves rule/source decisions unchanged. Even a recomputed
	// receipt plus timing-sensitive classification cannot excuse different bytes
	// from the ones the original patch actually renders.
	r.Candidate = strings.Replace(r.Candidate, "remark synthetic documentation addresses only", "remark altered documentation", 1)
	r.CandidateSHA256 = evidence.ConfigDigest([]byte(r.Candidate))
	r.Comparison.After.SHA256 = r.CandidateSHA256
	c, err := evidence.SealComparison(*r.Comparison)
	if err != nil {
		t.Fatal(err)
	}
	r.Comparison = &c
	r.Status = "UNVERIFIED"
	r.Reason = "collateral_inconclusive"
	r.Collateral.Status = "INCONCLUSIVE"
	r.Collateral.Stats.TimingLimited = true
	r.Collateral.Stats.StopReason = "wall_clock"
	r, err = seal(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Replay(context.Background(), b, a, r); err == nil {
		t.Fatal("timing classification hid rerender mismatch")
	}
}

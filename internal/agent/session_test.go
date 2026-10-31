package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"plenilune/internal/ai"
)

func TestSessionCompletesWithEvidenceBackedClaim(t *testing.T) {
	engine := &fakeEngine{}
	provider := scripted(
		contentResponse(proposal),
		toolResponse(toolCall("c1", "get_evidence", getEvidenceCall)),
		contentResponse(finalHolds),
	)
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())

	if res.Status != StatusCompleted || res.Reason != "" {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if res.Schema != ResultSchema {
		t.Fatalf("schema = %q", res.Schema)
	}
	if res.Obligations == nil || res.Obligations.OperatorAccepted || res.Obligations.Source != "ai_proposed_validated" {
		t.Fatalf("accepted obligations = %+v", res.Obligations)
	}
	if res.Obligations.SHA256 != engine.lastDigest {
		t.Fatalf("digest = %s, engine = %s", res.Obligations.SHA256, engine.lastDigest)
	}
	if len(res.Checks) != 1 || res.Checks[0].Status != ObligationHolds || res.Checks[0].EvidenceIDs[0] != "EV-HOLDS" {
		t.Fatalf("checks = %+v", res.Checks)
	}
	if len(res.Claims) != 1 || !res.Claims[0].AIGenerated || res.Claims[0].EvidenceIDs[0] != "EV-HOLDS" {
		t.Fatalf("claims = %+v", res.Claims)
	}
	if engine.checkCalls != 1 || engine.evidenceCalls != 1 {
		t.Fatalf("engine calls = check %d evidence %d", engine.checkCalls, engine.evidenceCalls)
	}
	if provider.count() != 3 {
		t.Fatalf("provider calls = %d", provider.count())
	}
	if res.Provenance.Requests != 3 || res.Provenance.ToolCalls != 2 || res.Provenance.Usage.TotalTokens != 45 {
		t.Fatalf("provenance = %+v", res.Provenance)
	}
	if len(res.Provenance.RequestSHA256) != 3 || len(res.Provenance.ResponseSHA256) != 3 {
		t.Fatalf("hashes = %+v", res.Provenance)
	}
	first := provider.request(0)
	if len(first.Tools) != 4 || first.ResponseFormat == nil || first.ResponseFormat.Type != "json_schema" {
		t.Fatalf("tools/format = %+v", first)
	}
	if !strings.Contains(first.Messages[0].Content, "untrusted data") {
		t.Fatal("system prompt is missing the untrusted-data rule")
	}
	if !strings.Contains(first.Messages[1].Content, "Allow only 10.20.5.0/24") {
		t.Fatal("payload is missing intent")
	}
	second := provider.request(1)
	if got := second.Messages[len(second.Messages)-1]; got.Role != "user" || !strings.Contains(got.Content, "EV-HOLDS") {
		t.Fatalf("check observation = %+v", got)
	}
	third := provider.request(2)
	if got := third.Messages[len(third.Messages)-1]; got.Role != "tool" || got.ToolCallID != "c1" {
		t.Fatalf("tool result = %+v", got)
	}
}

func TestSessionAcceptsDirectCheckToolCall(t *testing.T) {
	engine := &fakeEngine{}
	args := `{"obligations":[` + oneObligation + `]}`
	provider := scripted(
		toolResponse(toolCall("c1", "check_obligations", args)),
		contentResponse(finalHolds),
	)
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted {
		t.Fatalf("status = %s/%s", res.Status, res.Reason)
	}
	if engine.checkCalls != 1 || provider.count() != 2 || res.Provenance.ToolCalls != 1 {
		t.Fatalf("calls = engine %d provider %d tools %d", engine.checkCalls, provider.count(), res.Provenance.ToolCalls)
	}
}

func TestSessionRepairLoopVerifiesPatch(t *testing.T) {
	engine := &fakeEngine{checkStatuses: map[string]string{"OB-1": ObligationViolated}}
	provider := scripted(
		contentResponse(proposal),
		toolResponse(toolCall("c1", "get_evidence", `{"ids":["EV-VIOLATION"]}`)),
		toolResponse(toolCall("c2", "verify_patch", appendPatchCall)),
		contentResponse(finalRepair),
	)
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted || res.Repair == nil || res.Repair.Status != PatchVerified {
		t.Fatalf("result = %s/%s repair=%+v", res.Status, res.Reason, res.Repair)
	}
	if res.Repair.Attempts != 1 || res.Repair.CollateralStatus != CollateralVerified {
		t.Fatalf("repair = %+v", res.Repair)
	}
	if engine.patchCalls != 1 || provider.count() != 4 || res.Provenance.ToolCalls != 3 {
		t.Fatalf("calls = patch %d provider %d tools %d", engine.patchCalls, provider.count(), res.Provenance.ToolCalls)
	}
	patch := engine.lastPatch
	if patch.BaseBeforeSHA256 != testBefore || patch.BaseAfterSHA256 != testAfter ||
		patch.ObligationsDigest != res.Obligations.SHA256 || patch.Scope != "ipv4-unfragmented-or-initial-with-complete-l4/v1" {
		t.Fatalf("patch pins = %+v", patch)
	}
	if len(patch.Edits) != 1 || patch.Edits[0].Op != "append" || patch.Edits[0].Ref != nil {
		t.Fatalf("patch edits = %+v", patch.Edits)
	}
	if len(res.Claims) != 1 || res.Claims[0].EvidenceIDs[0] != "EV-PATCH" {
		t.Fatalf("claims = %+v", res.Claims)
	}
}

func TestSessionClarificationStopsBeforeChecks(t *testing.T) {
	engine := &fakeEngine{}
	provider := scripted(contentResponse(clarification))
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusClarificationNeeded || len(res.Questions) != 2 {
		t.Fatalf("result = %s questions=%v", res.Status, res.Questions)
	}
	if engine.checkCalls != 0 || provider.count() != 1 {
		t.Fatalf("unexpected calls: engine %d provider %d", engine.checkCalls, provider.count())
	}
}

func TestSessionRetriesTransientProviderErrorOnce(t *testing.T) {
	engine := &fakeEngine{}
	provider := &fakeProvider{fn: func(call int, _ ai.Request) (ai.Response, error) {
		switch call {
		case 1:
			return ai.Response{}, &ai.StatusError{Status: 429, Code: "rate_limited", Retriable: true}
		case 2:
			return contentResponse(proposal), nil
		default:
			return contentResponse(finalHolds), nil
		}
	}}
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted || provider.count() != 3 {
		t.Fatalf("result = %s calls=%d", res.Status, provider.count())
	}
	if res.Provenance.Retries != 1 || res.Provenance.Requests != 3 {
		t.Fatalf("provenance = %+v", res.Provenance)
	}
}

func TestSessionDoesNotRetryAuthError(t *testing.T) {
	provider := &fakeProvider{fn: func(int, ai.Request) (ai.Response, error) {
		return ai.Response{}, &ai.StatusError{Status: 401, Code: "invalid_api_key"}
	}}
	s := newSession(t, provider, &fakeEngine{})
	res := runSession(t, s, testInput())
	if res.Status != StatusUnavailable || res.Reason != "provider_auth_failed" {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if provider.count() != 1 {
		t.Fatalf("provider calls = %d", provider.count())
	}
}

func TestSessionModelBudgetStopsToolLoop(t *testing.T) {
	engine := &fakeEngine{}
	provider := &fakeProvider{fn: func(call int, _ ai.Request) (ai.Response, error) {
		if call == 1 {
			return contentResponse(proposal), nil
		}
		return toolResponse(toolCall(fmt.Sprintf("c%d", call), "get_evidence", `{"ids":["EV-HOLDS"]}`)), nil
	}}
	s := newSession(t, provider, engine, func(c *Config) { c.MaxModelRequests = 3 })
	res := runSession(t, s, testInput())
	if res.Status != StatusPartial || res.Reason != "model_budget_exhausted" {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if provider.count() != 3 || res.Provenance.Requests != 3 {
		t.Fatalf("provider = %d requests=%d", provider.count(), res.Provenance.Requests)
	}
	if len(res.Checks) != 1 || res.Checks[0].Status != ObligationHolds {
		t.Fatalf("checks = %+v", res.Checks)
	}
}

func TestSessionDisablesToolsAtBudgetAndFinishes(t *testing.T) {
	engine := &fakeEngine{}
	provider := scripted(
		contentResponse(proposal),
		toolResponse(toolCall("c1", "get_evidence", getEvidenceCall)),
		contentResponse(finalHolds),
	)
	s := newSession(t, provider, engine, func(c *Config) { c.MaxToolCalls = 2 })
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted || res.Provenance.ToolCalls != 2 {
		t.Fatalf("result = %s tools=%d", res.Status, res.Provenance.ToolCalls)
	}
	third := provider.request(2)
	if len(third.Tools) != 0 {
		t.Fatalf("tools were still offered: %d", len(third.Tools))
	}
	found := false
	for _, m := range third.Messages {
		if strings.Contains(m.Content, "tool_budget_exhausted") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing tool budget instruction")
	}
}

func TestSessionStopsWhenModelCallsToolsAfterExhaustion(t *testing.T) {
	engine := &fakeEngine{}
	provider := &fakeProvider{fn: func(call int, _ ai.Request) (ai.Response, error) {
		if call == 1 {
			return contentResponse(proposal), nil
		}
		return toolResponse(toolCall(fmt.Sprintf("c%d", call), "get_evidence", `{"ids":["EV-HOLDS"]}`)), nil
	}}
	s := newSession(t, provider, engine, func(c *Config) { c.MaxToolCalls = 2 })
	res := runSession(t, s, testInput())
	if res.Status != StatusPartial || res.Reason != "tool_budget_exhausted" {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if provider.count() != 3 || res.Provenance.ToolCalls != 2 {
		t.Fatalf("provider = %d tools=%d", provider.count(), res.Provenance.ToolCalls)
	}
}

func TestSessionRepairProposalCap(t *testing.T) {
	engine := &fakeEngine{
		checkStatuses: map[string]string{"OB-1": ObligationViolated},
		patchStatus:   PatchRejected,
		patchEvidence: []string{"EV-REJECT"},
	}
	provider := scripted(
		contentResponse(proposal),
		toolResponse(toolCall("c1", "get_evidence", `{"ids":["EV-VIOLATION"]}`)),
		toolResponse(toolCall("c2", "verify_patch", appendPatchCall)),
		toolResponse(toolCall("c3", "verify_patch", appendPatchCall)),
		toolResponse(toolCall("c4", "verify_patch", appendPatchCall)),
		contentResponse(finalEmpty),
	)
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusPartial || res.Reason != "repair_unverified" {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if res.Repair == nil || res.Repair.Attempts != 2 || res.Repair.Status != PatchRejected {
		t.Fatalf("repair = %+v", res.Repair)
	}
	if engine.patchCalls != 2 || res.Provenance.RepairProposals != 2 {
		t.Fatalf("patch calls = %d proposals=%d", engine.patchCalls, res.Provenance.RepairProposals)
	}
}

func TestSessionDowngradesInconsistentVerifiedPatch(t *testing.T) {
	engine := &fakeEngine{
		checkStatuses:   map[string]string{"OB-1": ObligationViolated},
		patchCollateral: CollateralUnchecked,
	}
	provider := scripted(
		contentResponse(proposal),
		toolResponse(toolCall("c1", "get_evidence", `{"ids":["EV-VIOLATION"]}`)),
		toolResponse(toolCall("c2", "verify_patch", appendPatchCall)),
		contentResponse(finalEmpty),
	)
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Repair == nil || res.Repair.Status != PatchUnverified {
		t.Fatalf("repair = %+v", res.Repair)
	}
	if res.Status != StatusPartial || res.Reason != "repair_unverified" {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
}

func TestSessionDropsClaimsWithUnknownEvidence(t *testing.T) {
	badFinal := `{"status":"ok","obligations":[],"questions":[],"claims":[{"text":"Fabricated claim.","obligation_ids":["OB-1"],"evidence_ids":["EV-UNKNOWN"]}],"summary":"x"}`
	provider := scripted(contentResponse(proposal), contentResponse(badFinal))
	s := newSession(t, provider, &fakeEngine{})
	res := runSession(t, s, testInput())
	if res.Status != StatusPartial || res.Reason != "claims_dropped" {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if len(res.Claims) != 0 || res.DroppedClaims != 1 {
		t.Fatalf("claims = %+v dropped=%d", res.Claims, res.DroppedClaims)
	}
}

func TestSessionDropsClaimsWithUnknownObligation(t *testing.T) {
	badFinal := `{"status":"ok","obligations":[],"questions":[],"claims":[{"text":"Known witness, unknown condition.","obligation_ids":["OB-9"],"evidence_ids":["EV-HOLDS"]}],"summary":"x"}`
	provider := scripted(contentResponse(proposal), contentResponse(badFinal))
	s := newSession(t, provider, &fakeEngine{})
	res := runSession(t, s, testInput())
	if res.Status != StatusPartial || res.DroppedClaims != 1 || len(res.Claims) != 0 {
		t.Fatalf("result = %s claims=%+v dropped=%d", res.Status, res.Claims, res.DroppedClaims)
	}
}

func TestSessionCorrectsInvalidOutput(t *testing.T) {
	bad := strings.Replace(proposal, `"status":"ok"`, `"status":"ok","verdict":"EQUIVALENT"`, 1)
	provider := scripted(contentResponse(bad), contentResponse(proposal), contentResponse(finalHolds))
	s := newSession(t, provider, &fakeEngine{})
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted || provider.count() != 3 {
		t.Fatalf("result = %s calls=%d", res.Status, provider.count())
	}
	second := provider.request(1)
	last := second.Messages[len(second.Messages)-1]
	if last.Role != "user" || !strings.Contains(last.Content, "invalid_output") {
		t.Fatalf("correction = %+v", last)
	}
}

func TestSessionCorrectsTooManyObligations(t *testing.T) {
	provider := scripted(contentResponse(proposal13), contentResponse(proposal), contentResponse(finalHolds))
	s := newSession(t, provider, &fakeEngine{})
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted || provider.count() != 3 {
		t.Fatalf("result = %s calls=%d", res.Status, provider.count())
	}
}

func TestSessionRejectsClarificationAfterFreeze(t *testing.T) {
	provider := scripted(contentResponse(proposal), contentResponse(clarification), contentResponse(finalHolds))
	s := newSession(t, provider, &fakeEngine{})
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted || provider.count() != 3 {
		t.Fatalf("result = %s calls=%d", res.Status, provider.count())
	}
	third := provider.request(2)
	last := third.Messages[len(third.Messages)-1]
	if last.Role != "user" || !strings.Contains(last.Content, "accepted obligation set") {
		t.Fatalf("correction = %+v", last)
	}
}

func TestSessionListChangesProvidesEvidence(t *testing.T) {
	engine := &fakeEngine{}
	final := `{"status":"ok","obligations":[],"questions":[],"claims":[{"text":"A change summary exists.","obligation_ids":[],"evidence_ids":["EV-CHANGE"]}],"summary":"x"}`
	provider := scripted(
		toolResponse(toolCall("c1", "list_changes", `{"limit":5}`)),
		contentResponse(proposal),
		contentResponse(final),
	)
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted || engine.changesCalls != 1 {
		t.Fatalf("result = %s changes=%d", res.Status, engine.changesCalls)
	}
	if len(res.Claims) != 1 || res.Claims[0].EvidenceIDs[0] != "EV-CHANGE" {
		t.Fatalf("claims = %+v", res.Claims)
	}
}

func TestSessionToolArgumentValidation(t *testing.T) {
	engine := &fakeEngine{}
	badEvidence := `{"ids":["EV-HOLDS"],"extra":true}`
	provider := scripted(
		toolResponse(toolCall("c1", "get_evidence", badEvidence)),
		toolResponse(toolCall("c2", "get_evidence", `{"ids":["EV-NOPE"]}`)),
		contentResponse(proposal),
		contentResponse(finalHolds),
	)
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusCompleted {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if engine.evidenceCalls != 0 {
		t.Fatalf("engine evidence calls = %d", engine.evidenceCalls)
	}
}

func TestSessionEngineFailureIsNotAVerdict(t *testing.T) {
	engine := &fakeEngine{checkErr: errors.New("engine unavailable")}
	provider := scripted(contentResponse(proposal), contentResponse(finalEmpty))
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusPartial || res.Reason != "checks_unavailable" {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if len(res.Checks) != 0 || res.Obligations == nil {
		t.Fatalf("result = %+v", res)
	}
}

func TestSessionRejectsEngineDigestMismatch(t *testing.T) {
	engine := &fakeEngine{checkOverride: &ObligationCheck{
		ObligationsDigest: "deadbeef",
		Results:           []ObligationResult{{ID: "OB-1", Status: ObligationHolds, EvidenceIDs: []string{"EV-HOLDS"}}},
	}}
	provider := scripted(contentResponse(proposal), contentResponse(finalEmpty))
	s := newSession(t, provider, engine)
	res := runSession(t, s, testInput())
	if res.Status != StatusPartial || res.Reason != "checks_unavailable" || len(res.Checks) != 0 {
		t.Fatalf("result = %s/%s checks=%+v", res.Status, res.Reason, res.Checks)
	}
}

func TestSessionCanceledByDeadline(t *testing.T) {
	provider := blockingProvider{}
	s := newSession(t, provider, &fakeEngine{}, func(c *Config) {
		c.SessionDeadline = 30 * time.Millisecond
	})
	start := time.Now()
	res := runSession(t, s, testInput())
	if res.Status != StatusCanceled {
		t.Fatalf("status = %s/%s", res.Status, res.Reason)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cancel took %s", elapsed)
	}
}

type blockingProvider struct{}

func (blockingProvider) Complete(ctx context.Context, _ ai.Request) (ai.Response, error) {
	<-ctx.Done()
	return ai.Response{}, ctx.Err()
}

func TestSessionCanceledBeforeStart(t *testing.T) {
	provider := scripted(contentResponse(proposal))
	s := newSession(t, provider, &fakeEngine{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := s.Run(ctx, testInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusCanceled || provider.count() != 0 {
		t.Fatalf("result = %s calls=%d", res.Status, provider.count())
	}
}

func TestSessionDisabledAndNotConfigured(t *testing.T) {
	disabled, err := New(Config{Enabled: false})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := disabled.Run(context.Background(), testInput())
	if err != nil || res.Status != StatusDisabled {
		t.Fatalf("disabled = %s/%v", res.Status, err)
	}

	noProvider, err := New(Config{Enabled: true, Engine: &fakeEngine{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err = noProvider.Run(context.Background(), testInput())
	if err != nil || res.Status != StatusNotConfigured || res.Reason != "provider_not_configured" {
		t.Fatalf("no provider = %s/%s/%v", res.Status, res.Reason, err)
	}

	noEngine, err := New(Config{Enabled: true, Provider: scripted(contentResponse(proposal))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err = noEngine.Run(context.Background(), testInput())
	if err != nil || res.Status != StatusNotConfigured || res.Reason != "engine_not_configured" {
		t.Fatalf("no engine = %s/%s/%v", res.Status, res.Reason, err)
	}
}

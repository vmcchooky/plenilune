package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"plenilune/internal/ai"
)

// P1-7 raised the AI proposal budget. These tests pin the new defaults,
// prove the raise changes outcomes only at the proposal layer (a draft
// that verifies on attempt 3 or 6 now completes; under the pre-P1-7
// caps it could not), and prove every raised cap is still a hard cap:
// a model that never produces a verifiable draft is stopped at exactly
// MaxRepairProposals engine verifications, never more.

// attemptEngine is a fakeEngine whose VerifyPatch rejects until the
// verifyOn-th attempt (1-based), which it verifies. verifyOn == 0 means
// no attempt ever verifies.
type attemptEngine struct {
	fakeEngine
	verifyOn   int
	patchCalls int
}

func (e *attemptEngine) VerifyPatch(_ context.Context, req PatchCheckRequest) (PatchCheckResult, error) {
	e.patchCalls++
	if e.verifyOn > 0 && e.patchCalls == e.verifyOn {
		return PatchCheckResult{
			Status:            PatchVerified,
			ObligationsDigest: req.ObligationsDigest,
			ObligationResults: []ObligationResult{{ID: "OB-1", Status: ObligationHolds, EvidenceIDs: []string{"EV-PATCH"}}},
			CollateralStatus:  CollateralVerified,
			EvidenceIDs:       []string{"EV-PATCH"},
		}, nil
	}
	return PatchCheckResult{
		Status:            PatchRejected,
		ObligationsDigest: req.ObligationsDigest,
		ObligationResults: []ObligationResult{{ID: "OB-1", Status: ObligationViolated, EvidenceIDs: []string{"EV-REJECT"}}},
		CollateralStatus:  CollateralFailed,
		EvidenceIDs:       []string{"EV-REJECT"},
	}, nil
}

// drafterProvider plays a persistent repair drafter: it proposes the
// obligation set, then keeps submitting verify_patch drafts until the
// engine verifies one or the local repair budget reports exhaustion,
// then it returns its final JSON. It never sees a verdict it could
// influence: statuses in tool observations are engine-owned.
func drafterProvider() *fakeProvider {
	return &fakeProvider{fn: func(call int, req ai.Request) (ai.Response, error) {
		if call == 1 {
			return contentResponse(proposal), nil
		}
		// A verified draft anywhere in the transcript ends the repair
		// loop (later verify_patch calls only earn already_verified).
		latestTool := ""
		for _, m := range req.Messages {
			if m.Role != "tool" {
				continue
			}
			if strings.Contains(m.Content, `"status":"VERIFIED"`) {
				return contentResponse(finalRepair), nil
			}
			latestTool = m.Content
		}
		if strings.Contains(latestTool, "repair_budget_exhausted") {
			return contentResponse(finalEmpty), nil
		}
		return toolResponse(toolCall("draft", "verify_patch", appendPatchCall)), nil
	}}
}

func runDraftScenario(t *testing.T, verifyOn int, mutate ...func(*Config)) (Result, *attemptEngine, time.Duration) {
	t.Helper()
	engine := &attemptEngine{
		fakeEngine: fakeEngine{checkStatuses: map[string]string{"OB-1": ObligationViolated}},
		verifyOn:   verifyOn,
	}
	s := newSession(t, drafterProvider(), engine, mutate...)
	start := time.Now()
	res := runSession(t, s, testInput())
	return res, engine, time.Since(start)
}

// pinPreP17Caps restores the pre-P1-7 session budget as an explicit
// configuration, for before/after measurement on the same corpus.
func pinPreP17Caps(c *Config) {
	c.MaxModelRequests = 8
	c.MaxToolCalls = 12
	c.MaxRepairProposals = 2
	c.SessionDeadline = 60 * time.Second
}

func TestP17SessionBudgetCorpus(t *testing.T) {
	type outcome struct {
		verified bool
		attempts int
	}
	// verifyOn -> expected outcome under (pre-P1-7 caps, P1-7 defaults).
	expect := map[int][2]outcome{
		1: {{true, 1}, {true, 1}},
		3: {{false, 2}, {true, 3}},
		6: {{false, 2}, {true, 6}},
		0: {{false, 2}, {false, 6}},
	}
	for _, verifyOn := range []int{1, 3, 6, 0} {
		for i, cfg := range []struct {
			name   string
			mutate []func(*Config)
		}{
			{"pre-p1-7", []func(*Config){pinPreP17Caps}},
			{"p1-7-defaults", nil},
		} {
			res, engine, wall := runDraftScenario(t, verifyOn, cfg.mutate...)
			want := expect[verifyOn][i]
			verified := res.Repair != nil && res.Repair.Status == PatchVerified
			if verified != want.verified {
				t.Fatalf("verifyOn=%d %s: verified=%v, want %v (result %s/%s)", verifyOn, cfg.name, verified, want.verified, res.Status, res.Reason)
			}
			if engine.patchCalls != want.attempts {
				t.Fatalf("verifyOn=%d %s: engine patch calls=%d, want %d", verifyOn, cfg.name, engine.patchCalls, want.attempts)
			}
			if res.Provenance.RepairProposals != want.attempts {
				t.Fatalf("verifyOn=%d %s: provenance proposals=%d, want %d", verifyOn, cfg.name, res.Provenance.RepairProposals, want.attempts)
			}
			if want.verified && res.Status != StatusCompleted {
				t.Fatalf("verifyOn=%d %s: status=%s, want completed", verifyOn, cfg.name, res.Status)
			}
			if !want.verified && (res.Status != StatusPartial || res.Reason != "repair_unverified") {
				t.Fatalf("verifyOn=%d %s: status=%s/%s, want partial/repair_unverified", verifyOn, cfg.name, res.Status, res.Reason)
			}
			t.Logf("corpus verify_on=%d config=%s verified=%v attempts=%d requests=%d tools=%d tokens=%d wall_ms=%d",
				verifyOn, cfg.name, verified, engine.patchCalls,
				res.Provenance.Requests, res.Provenance.ToolCalls, res.Provenance.Usage.TotalTokens, wall.Milliseconds())
		}
	}
}

func TestP17DefaultBudgets(t *testing.T) {
	s, err := New(Config{Enabled: true, Model: "muse-spark-1.3-contributor"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := SessionBudgets{
		MaxModelRequests:     16,
		MaxToolCalls:         24,
		MaxRepairProposals:   6,
		MaxOutputCorrections: 2,
		SessionDeadlineMS:    120_000,
	}
	if got := s.cfg.budgets(); got != want {
		t.Fatalf("default budgets = %+v, want %+v", got, want)
	}
	// The budgets are recorded in the result provenance even when the
	// session never runs (AI disabled), so a stored record shows the
	// caps it was produced under.
	off, err := New(Config{Enabled: false})
	if err != nil {
		t.Fatalf("New disabled: %v", err)
	}
	res, err := off.Run(context.Background(), testInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusDisabled || res.Provenance.Budgets != want {
		t.Fatalf("disabled result = %s budgets=%+v", res.Status, res.Provenance.Budgets)
	}
}

func TestP17BudgetsRecordedInProvenance(t *testing.T) {
	res, _, _ := runDraftScenario(t, 1, func(c *Config) { c.MaxRepairProposals = 4 })
	if res.Provenance.Budgets.MaxRepairProposals != 4 {
		t.Fatalf("recorded proposal budget = %d, want the configured 4", res.Provenance.Budgets.MaxRepairProposals)
	}
	if res.Provenance.Budgets.MaxModelRequests != DefaultMaxModelRequests ||
		res.Provenance.Budgets.MaxToolCalls != DefaultMaxToolCalls ||
		res.Provenance.Budgets.SessionDeadlineMS != DefaultSessionDeadline.Milliseconds() {
		t.Fatalf("recorded budgets = %+v, want configured caps with defaults elsewhere", res.Provenance.Budgets)
	}
}

func TestP17ProposalCeilingValidation(t *testing.T) {
	if _, err := New(Config{MaxRepairProposals: 16}); err != nil {
		t.Fatalf("proposal cap 16 rejected: %v", err)
	}
	if _, err := New(Config{MaxRepairProposals: 17}); err == nil {
		t.Fatal("proposal cap 17 accepted; the ceiling must stay a hard bound")
	}
	if _, err := New(Config{MaxRepairProposals: -1}); err == nil {
		t.Fatal("negative proposal cap accepted")
	}
}

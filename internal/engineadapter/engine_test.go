package engineadapter

import (
	"context"
	"encoding/json"
	"errors"
	"plenilune/internal/agent"
	"plenilune/internal/ai"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
	"strings"
	"sync"
	"testing"
)

func fixturePolicy() repair.Policy {
	return repair.Policy{Schema: repair.PolicySchema, Scope: intent.Scope, OperatorAccepted: true, Conditions: intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.10/32", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}, AllowedChanges: []intent.Region{}}
}

func fixtureSources() ([]byte, []byte) {
	return []byte("ip access-list extended MGMT\n 10 remark synthetic comment\n 20 permit tcp any any eq 22\n 30 deny ip any any\n"), []byte("ip access-list extended MGMT\n 10 deny tcp host 192.0.2.10 any eq 22\n 20 permit tcp any any eq 22\n 30 deny ip any any\n")
}

func newEngine(t *testing.T, policy *repair.Policy) *Engine {
	t.Helper()
	b, a := fixtureSources()
	e, err := New(context.Background(), b, a, policy, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func request(t *testing.T, e *Engine, set intent.Set) agent.PatchCheckRequest {
	t.Helper()
	_, digest, err := intent.Digest(set)
	if err != nil {
		t.Fatal(err)
	}
	return agent.PatchCheckRequest{BaseBeforeSHA256: e.beforeHash, BaseAfterSHA256: e.afterHash, ObligationsDigest: digest, Scope: intent.Scope, Edits: []agent.Edit{{Op: "remove", Ref: &agent.RuleRef{BaseAfterSHA256: e.afterHash, Ordinal: 0, Line: 2}}}}
}

func TestRealObligationEvidenceAndVerifiedRepair(t *testing.T) {
	p := fixturePolicy()
	e := newEngine(t, &p)
	check, err := e.CheckObligations(context.Background(), p.Conditions)
	if err != nil {
		t.Fatal(err)
	}
	if len(check.Results) != 1 || check.Results[0].Status != agent.ObligationViolated || len(check.Results[0].EvidenceIDs) != 1 {
		t.Fatalf("unexpected exact check: %+v", check)
	}
	ids := check.Results[0].EvidenceIDs
	if len(ids[0]) > 64 {
		t.Fatal("engine witness ID does not fit agent port")
	}
	batch, err := e.GetEvidence(context.Background(), agent.EvidenceRequest{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Records) != 1 || batch.Records[0].Kind != "obligation_witness" || !strings.Contains(batch.Records[0].Summary, "before permit") || !strings.Contains(batch.Records[0].Summary, "after deny") {
		t.Fatalf("not a real witness: %+v", batch)
	}
	result, err := e.VerifyPatch(context.Background(), request(t, e, p.Conditions))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agent.PatchVerified || result.CollateralStatus != agent.CollateralVerified || result.ObligationResults[0].Status != agent.ObligationHolds || len(result.EvidenceIDs) == 0 {
		t.Fatalf("repair: %+v", result)
	}
	proofs, err := e.Repairs()
	if err != nil || len(proofs) != 1 || proofs[0].Comparison == nil || proofs[0].Comparison.Report.Obligations[0].Status != "HOLDS" {
		t.Fatalf("missing full repair proof: %v %+v", err, proofs)
	}
	if strings.Contains(proofs[0].Candidate, "deny tcp host 192.0.2.10") {
		t.Fatal("candidate did not apply actual edit")
	}
	if _, err := e.GetEvidence(context.Background(), agent.EvidenceRequest{IDs: []string{"invented"}}); err == nil {
		t.Fatal("unknown evidence accepted")
	}
}

func TestPolicyIsFrozenAndInputsAndSnapshotsOwned(t *testing.T) {
	p := fixturePolicy()
	before, after := fixtureSources()
	e, err := New(context.Background(), before, after, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	p.Conditions.Obligations[0].Match.DstPorts[0].Lo = 23
	before[0] = 'X'
	after[0] = 'X'
	input, err := e.Input("Allow the management host to use SSH.")
	if err != nil {
		t.Fatal(err)
	}
	if input.Before[0] != "permit tcp any any eq 22" || strings.Contains(strings.Join(input.Before, " "), "remark") {
		t.Fatal("provider excerpt includes source wrapper/remark")
	}
	input.Before[0] = "mutated"
	if len(input.Changes) > 0 && input.Changes[0].Ordinal != nil {
		*input.Changes[0].Ordinal = 999
	}
	input2, err := e.Input("Allow SSH.")
	if err != nil || input2.Before[0] == "mutated" {
		t.Fatal("input snapshot aliases engine")
	}
	if input.AcceptedConditions == nil {
		t.Fatal("accepted policy missing from contextual input")
	}
	if !input.RepairAvailable {
		t.Fatal("accepted policy did not enable repair")
	}
	input.AcceptedConditions.Obligations[0].Match.DstPorts[0].Lo = 99
	input3, err := e.Input("Allow SSH.")
	if err != nil || input3.AcceptedConditions == nil || input3.AcceptedConditions.Obligations[0].Match.DstPorts[0].Lo != 22 {
		t.Fatal("accepted context aliases policy")
	}
	snapshot, err := e.Comparison()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Conditions.Obligations[0].Match.DstPorts[0].Lo = 23
	if len(snapshot.Report.Witnesses) > 0 && snapshot.Report.Witnesses[0].Before.Line != nil {
		*snapshot.Report.Witnesses[0].Before.Line = 999
	}
	snapshot2, err := e.Comparison()
	if err != nil || snapshot2.Conditions.Obligations[0].Match.DstPorts[0].Lo != 22 {
		t.Fatal("comparison snapshot aliases policy")
	}
	other := fixturePolicy().Conditions
	other.Obligations[0].Kind = "must_deny"
	if _, err := e.CheckObligations(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	result, err := e.VerifyPatch(context.Background(), request(t, e, other))
	if err != nil || result.Status != agent.PatchRejected || result.Reason != "frozen_obligations_mismatch" {
		t.Fatalf("model changed frozen policy: %v %+v", err, result)
	}
	if e.policy.Conditions.Obligations[0].Kind != "must_permit" {
		t.Fatal("checking a proposal overwrote policy")
	}
}

func TestMissingAcceptanceAndPinMismatchNeverVerify(t *testing.T) {
	for _, policy := range []*repair.Policy{nil, func() *repair.Policy { p := fixturePolicy(); p.OperatorAccepted = false; return &p }()} {
		e := newEngine(t, policy)
		set := fixturePolicy().Conditions
		if _, err := e.CheckObligations(context.Background(), set); err != nil {
			t.Fatal(err)
		}
		_, digest, err := intent.Digest(set)
		if err != nil {
			t.Fatal(err)
		}
		if e.OperatorAccepted(e.beforeHash, e.afterHash, digest) {
			t.Fatal("missing operator acceptance acquired authority")
		}
		in, err := e.Input("Allow SSH.")
		if err != nil || in.AcceptedConditions != nil || in.RepairAvailable {
			t.Fatal("unaccepted policy leaked into accepted context")
		}
		result, err := e.VerifyPatch(context.Background(), request(t, e, set))
		if err != nil || result.Status != agent.PatchUnverified || result.CollateralStatus != agent.CollateralUnchecked {
			t.Fatalf("unaccepted policy verified: %v %+v", err, result)
		}
	}
	p := fixturePolicy()
	e := newEngine(t, &p)
	_, digest, err := intent.Digest(p.Conditions)
	if err != nil {
		t.Fatal(err)
	}
	if !e.OperatorAccepted(e.beforeHash, e.afterHash, digest) {
		t.Fatal("matching frozen policy not recognized")
	}
	if e.OperatorAccepted(strings.Repeat("0", 64), e.afterHash, digest) || e.OperatorAccepted(e.beforeHash, strings.Repeat("0", 64), digest) || e.OperatorAccepted(e.beforeHash, e.afterHash, strings.Repeat("0", 64)) {
		t.Fatal("wrong input/policy acquired authority")
	}
	r := request(t, e, p.Conditions)
	r.BaseAfterSHA256 = strings.Repeat("0", 64)
	result, err := e.VerifyPatch(context.Background(), r)
	if err != nil || result.Status != agent.PatchRejected {
		t.Fatalf("wrong ticket verified: %v %+v", err, result)
	}
}

func TestIncompleteCollateralMappingAndDirectEditBounds(t *testing.T) {
	for _, status := range []string{agent.CollateralVerified, agent.CollateralFailed, agent.CollateralUnchecked} {
		mapped, err := agentCollateral(status)
		if err != nil || mapped != status {
			t.Fatalf("changed known collateral status: %s %v", mapped, err)
		}
	}
	if mapped, err := agentCollateral("INCONCLUSIVE"); err != nil || mapped != agent.CollateralUnchecked || mapped == agent.CollateralVerified {
		t.Fatalf("incomplete collateral promoted: %s %v", mapped, err)
	}
	if _, err := agentCollateral("invented"); err == nil {
		t.Fatal("unknown collateral result accepted")
	}
	p := fixturePolicy()
	e := newEngine(t, &p)
	for _, count := range []int{0, 6} {
		r := request(t, e, p.Conditions)
		r.Edits = make([]agent.Edit, count)
		result, err := e.VerifyPatch(context.Background(), r)
		if err != nil || result.Status != agent.PatchRejected || result.Reason != "invalid_edit_count" {
			t.Fatalf("unbounded edits accepted: %v %+v", err, result)
		}
	}
}

func TestChangeRefsPaginationAndConcurrentSnapshots(t *testing.T) {
	p := fixturePolicy()
	e := newEngine(t, &p)
	page, err := e.ListChanges(context.Background(), agent.ListChangesRequest{Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("changes: %v %+v", err, page)
	}
	item := page.Items[0]
	if item.Side != "after" || item.Ordinal == nil || *item.Ordinal != 0 || item.Line == nil || *item.Line != 2 {
		t.Fatalf("wrong actual source ref: %+v", item)
	}
	*item.Ordinal = 99
	page2, err := e.ListChanges(context.Background(), agent.ListChangesRequest{IDs: []string{item.ID}, Limit: 1})
	if err != nil || *page2.Items[0].Ordinal != 0 {
		t.Fatal("page aliases immutable source refs")
	}
	if _, err := e.ListChanges(context.Background(), agent.ListChangesRequest{Cursor: "another-ticket:0", Limit: 1}); err == nil {
		t.Fatal("foreign cursor accepted")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.CheckObligations(context.Background(), p.Conditions); err != nil {
				t.Error(err)
			}
			if _, err := e.Comparison(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestEarlyInputLimitsAndRecordCapacity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(ctx, nil, nil, nil, verify.DefaultOptions()); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled constructor did not stop")
	}
	if _, err := New(context.Background(), make([]byte, ciscoacl.MaxInputBytes+1), nil, nil, verify.DefaultOptions()); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatal("oversized input reached parse/clone path")
	}
	e := newEngine(t, nil)
	e.mu.Lock()
	defer e.mu.Unlock()
	for len(e.records) < maxRecords {
		n := len(e.records)
		rec := agent.EvidenceRecord{ID: shortID("ev", string(rune(n))), Kind: "receipt", Summary: "bounded"}
		if err := e.saveRecords([]agent.EvidenceRecord{rec}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.saveRecords([]agent.EvidenceRecord{{ID: "extra", Kind: "receipt", Summary: "bounded"}}); err == nil || len(e.records) != maxRecords {
		t.Fatal("record limit was not atomic")
	}
}

type scriptedProvider struct {
	step int
	call func(int, ai.Request) (ai.Response, error)
}

// This wrapper deliberately exposes only the proposal/check port, not the
// optional operator-policy authority of its underlying real engine.
type proposalOnlyEngine struct{ agent.Engine }

func TestAcceptedInputNeedsMatchingEngineAuthorityBeforeProviderCall(t *testing.T) {
	p := fixturePolicy()
	e := newEngine(t, &p)
	for _, tc := range []struct {
		name   string
		engine agent.Engine
		mutate func(*agent.Input)
	}{
		{"missing_authority", proposalOnlyEngine{e}, func(*agent.Input) {}},
		{"different_input", e, func(in *agent.Input) { in.BaseAfterSHA256 = strings.Repeat("0", 64) }},
		{"different_conditions", e, func(in *agent.Input) { in.AcceptedConditions.Obligations[0].Kind = "must_deny" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &scriptedProvider{call: func(int, ai.Request) (ai.Response, error) { return ai.Response{}, errors.New("must not call provider") }}
			session, err := agent.New(agent.Config{Enabled: true, Provider: provider, Engine: tc.engine})
			if err != nil {
				t.Fatal(err)
			}
			in, err := e.Input("Allow SSH.")
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&in)
			if _, err := session.Run(context.Background(), in); err == nil {
				t.Fatal("unbound accepted context was trusted")
			}
			if provider.step != 0 {
				t.Fatal("unbound operator policy was sent to provider")
			}
		})
	}
}

func (p *scriptedProvider) Complete(_ context.Context, req ai.Request) (ai.Response, error) {
	p.step++
	return p.call(p.step, req)
}

func TestRepairAvailabilityFlagWithoutAcceptanceHasNoAuthority(t *testing.T) {
	e := newEngine(t, nil)
	set := fixturePolicy().Conditions
	in, err := e.Input("Allow SSH.")
	if err != nil {
		t.Fatal(err)
	}
	if in.RepairAvailable || in.AcceptedConditions != nil {
		t.Fatal("engine without accepted policy reported repair availability")
	}
	// A caller can flip the provider-facing hint, but the engine must still
	// refuse the proposal because it owns acceptance.
	in.RepairAvailable = true
	proposal, err := json.Marshal(struct {
		Status      string              `json:"status"`
		Obligations []intent.Obligation `json:"obligations"`
		Questions   []string            `json:"questions"`
		Claims      []agent.Claim       `json:"claims"`
		Summary     string              `json:"summary"`
	}{"ok", set.Obligations, []string{}, []agent.Claim{}, ""})
	if err != nil {
		t.Fatal(err)
	}
	patchArgs, err := json.Marshal(struct {
		Edits     []agent.Edit `json:"edits"`
		Rationale string       `json:"rationale"`
	}{request(t, e, set).Edits, "Attempt repair without operator acceptance."})
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{call: func(step int, req ai.Request) (ai.Response, error) {
		response := ai.Response{Model: "scripted-no-live-meta", FinishReason: "stop", RequestSHA256: strings.Repeat("1", 64), ResponseSHA256: strings.Repeat("2", 64)}
		switch step {
		case 1:
			response.Content = string(proposal)
		case 2:
			response.FinishReason = "tool_calls"
			response.ToolCalls = []ai.ToolCall{{ID: "patch-1", Type: "function", Function: ai.FunctionCall{Name: "verify_patch", Arguments: string(patchArgs)}}}
		case 3:
			response.Content = `{"status":"ok","obligations":[],"questions":[],"claims":[],"summary":""}`
		default:
			return ai.Response{}, errors.New("unexpected provider request")
		}
		return response, nil
	}}
	session, err := agent.New(agent.Config{Enabled: true, Provider: provider, Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agent.StatusPartial || result.Repair == nil ||
		result.Repair.Status != agent.PatchUnverified || result.Repair.Reason != "operator_policy_not_accepted" {
		t.Fatalf("forged availability granted repair authority: %+v", result)
	}
	if provider.step != 3 {
		t.Fatalf("unexpected provider calls: %d", provider.step)
	}
}

func TestAgentToolLoopUsesRealEngineAndRealRepair(t *testing.T) {
	p := fixturePolicy()
	e := newEngine(t, &p)
	checked, err := e.CheckObligations(context.Background(), p.Conditions)
	if err != nil {
		t.Fatal(err)
	}
	witnessID := checked.Results[0].EvidenceIDs[0]
	proposal, err := json.Marshal(struct {
		Status      string              `json:"status"`
		Obligations []intent.Obligation `json:"obligations"`
		Questions   []string            `json:"questions"`
		Claims      []agent.Claim       `json:"claims"`
		Summary     string              `json:"summary"`
	}{"ok", p.Conditions.Obligations, []string{}, []agent.Claim{}, ""})
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{call: func(step int, req ai.Request) (ai.Response, error) {
		response := ai.Response{Model: "scripted-no-live-meta", FinishReason: "stop", RequestSHA256: strings.Repeat("1", 64), ResponseSHA256: strings.Repeat("2", 64)}
		switch step {
		case 1:
			response.Content = string(proposal)
		case 2:
			response.FinishReason = "tool_calls"
			response.ToolCalls = []ai.ToolCall{{ID: "get-1", Type: "function", Function: ai.FunctionCall{Name: "get_evidence", Arguments: `{"ids":["` + witnessID + `"]}`}}}
		case 3:
			args, err := json.Marshal(struct {
				Edits     []agent.Edit `json:"edits"`
				Rationale string       `json:"rationale"`
			}{request(t, e, p.Conditions).Edits, "Remove the shadowing deny."})
			if err != nil {
				return ai.Response{}, err
			}
			response.FinishReason = "tool_calls"
			response.ToolCalls = []ai.ToolCall{{ID: "patch-1", Type: "function", Function: ai.FunctionCall{Name: "verify_patch", Arguments: string(args)}}}
		case 4:
			last := req.Messages[len(req.Messages)-1]
			var observation struct {
				Result agent.PatchCheckResult `json:"result"`
			}
			if err := json.Unmarshal([]byte(last.Content), &observation); err != nil {
				return ai.Response{}, err
			}
			if observation.Result.Status != agent.PatchVerified || len(observation.Result.EvidenceIDs) == 0 {
				return ai.Response{}, errors.New("real repair was not verified")
			}
			final, err := json.Marshal(struct {
				Status      string              `json:"status"`
				Obligations []intent.Obligation `json:"obligations"`
				Questions   []string            `json:"questions"`
				Claims      []struct {
					Text          string   `json:"text"`
					ObligationIDs []string `json:"obligation_ids"`
					EvidenceIDs   []string `json:"evidence_ids"`
				} `json:"claims"`
				Summary string `json:"summary"`
			}{Status: "ok", Obligations: []intent.Obligation{}, Questions: []string{}, Claims: []struct {
				Text          string   `json:"text"`
				ObligationIDs []string `json:"obligation_ids"`
				EvidenceIDs   []string `json:"evidence_ids"`
			}{{"The deterministic engine verified the repair.", []string{"OB-1"}, observation.Result.EvidenceIDs}}, Summary: "Scripted provider selected tools; real engine checked candidate."})
			if err != nil {
				return ai.Response{}, err
			}
			response.Content = string(final)
		default:
			return ai.Response{}, errors.New("unexpected provider request")
		}
		return response, nil
	}}
	session, err := agent.New(agent.Config{Enabled: true, Provider: provider, Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	in, err := e.Input("Allow this management host to SSH; preserve all other behavior.")
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agent.StatusCompleted || result.Repair == nil || result.Repair.Status != agent.PatchVerified || len(result.Claims) != 1 {
		t.Fatalf("real integration did not complete: %+v", result)
	}
	if result.Obligations == nil || !result.Obligations.OperatorAccepted {
		t.Fatal("session did not preserve engine-owned operator acceptance")
	}
	if provider.step != 4 {
		t.Fatalf("unexpected tool loop steps: %d", provider.step)
	}
	proofs, err := e.Repairs()
	if err != nil || len(proofs) != 1 || proofs[0].Comparison == nil || evidence.CheckComparison(*proofs[0].Comparison) != nil {
		t.Fatalf("missing deterministic receipt: %v", err)
	}
}

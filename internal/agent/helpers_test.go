package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"plenilune/internal/ai"
	"plenilune/internal/intent"
)

var (
	testBefore = strings.Repeat("b", 64)
	testAfter  = strings.Repeat("a", 64)
)

func testInput() Input {
	return Input{
		Intent:           "Allow only 10.20.5.0/24 to reach RDP on 10.30.0.0/24.",
		ACLName:          "MGMT",
		Before:           []string{"permit tcp 10.20.5.0 0.0.0.255 10.30.0.0 0.0.0.255 eq 3389", "deny ip any any"},
		After:            []string{"permit tcp 10.20.0.0 0.0.255.255 10.30.0.0 0.0.0.255 eq 3389", "deny ip any any"},
		BaseBeforeSHA256: testBefore,
		BaseAfterSHA256:  testAfter,
	}
}

const oneObligation = `{"id":"OB-1","kind":"must_deny","match":{"protocol":"tcp","src_cidr":"10.20.5.0/24","dst_cidr":"10.30.0.0/24","dst_ports":[{"lo":3389,"hi":3389}]}}`

const proposal = `{"status":"ok","obligations":[` + oneObligation + `],"questions":[],"claims":[],"summary":""}`

var proposal13 = `{"status":"ok","obligations":[
` + strings.Repeat(`{"id":"OB-1","kind":"must_deny","match":{"protocol":"tcp","src_cidr":"any","dst_cidr":"any"}},`, 12) + `
{"id":"OB-13","kind":"must_deny","match":{"protocol":"tcp","src_cidr":"any","dst_cidr":"any"}}],"questions":[],"claims":[],"summary":""}`

const clarification = `{"status":"clarification_needed","obligations":[],"questions":["Which source subnet is the enclosing universe?","Should other RDP sources be denied?"],"claims":[],"summary":""}`

const finalHolds = `{"status":"ok","obligations":[],"questions":[],"claims":[{"text":"The verifier witness supports the stated condition.","obligation_ids":["OB-1"],"evidence_ids":["EV-HOLDS"]}],"summary":"Synthetic model summary."}`

const finalRepair = `{"status":"ok","obligations":[],"questions":[],"claims":[{"text":"The repair candidate is verified by the engine.","obligation_ids":["OB-1"],"evidence_ids":["EV-PATCH"]}],"summary":"Repair verified by the deterministic engine."}`

const finalEmpty = `{"status":"ok","obligations":[],"questions":[],"claims":[],"summary":"No claims."}`

const getEvidenceCall = `{"ids":["EV-HOLDS"]}`

const appendPatchCall = `{"edits":[{"op":"append","content":"deny tcp 10.20.0.0 0.0.255.255 10.30.0.0 0.0.0.255 eq 3389"}],"rationale":"Restore the missing deny."}`

type fakeProvider struct {
	mu       sync.Mutex
	calls    int
	fn       func(call int, req ai.Request) (ai.Response, error)
	requests []ai.Request
}

func (p *fakeProvider) Complete(ctx context.Context, req ai.Request) (ai.Response, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.requests = append(p.requests, req)
	fn := p.fn
	p.mu.Unlock()
	if fn == nil {
		return ai.Response{}, errors.New("unexpected model request")
	}
	return fn(call, req)
}

func (p *fakeProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *fakeProvider) request(index int) ai.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[index]
}

func scripted(responses ...ai.Response) *fakeProvider {
	return &fakeProvider{fn: func(call int, _ ai.Request) (ai.Response, error) {
		if call > len(responses) {
			return ai.Response{}, errors.New("script exhausted")
		}
		return responses[call-1], nil
	}}
}

func contentResponse(content string) ai.Response {
	return ai.Response{
		Model:          "muse-spark-1.3",
		Content:        content,
		FinishReason:   "stop",
		Usage:          ai.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		RequestSHA256:  strings.Repeat("1", 64),
		ResponseSHA256: strings.Repeat("2", 64),
	}
}

func toolResponse(calls ...ai.ToolCall) ai.Response {
	return ai.Response{
		Model:          "muse-spark-1.3",
		ToolCalls:      calls,
		FinishReason:   "tool_calls",
		Usage:          ai.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		RequestSHA256:  strings.Repeat("3", 64),
		ResponseSHA256: strings.Repeat("4", 64),
	}
}

func toolCall(id, name, args string) ai.ToolCall {
	return ai.ToolCall{ID: id, Type: "function", Function: ai.FunctionCall{Name: name, Arguments: args}}
}

type fakeEngine struct {
	checkStatuses map[string]string
	checkEvidence map[string][]string
	checkErr      error
	checkOverride *ObligationCheck
	checkCalls    int
	lastSet       intent.Set
	lastDigest    string

	evidence      map[string]EvidenceRecord
	evidenceErr   error
	evidenceCalls int

	changes      ChangesPage
	changesErr   error
	changesCalls int

	patchStatus     string
	patchCollateral string
	patchEvidence   []string
	patchErr        error
	patchCalls      int
	lastPatch       PatchCheckRequest
	patchOverride   *PatchCheckResult
}

func (f *fakeEngine) CheckObligations(_ context.Context, set intent.Set) (ObligationCheck, error) {
	f.checkCalls++
	f.lastSet = set
	_, digest, err := intent.Digest(set)
	if err != nil {
		return ObligationCheck{}, err
	}
	f.lastDigest = digest
	if f.checkErr != nil {
		return ObligationCheck{}, f.checkErr
	}
	if f.checkOverride != nil {
		return *f.checkOverride, nil
	}
	results := make([]ObligationResult, 0, len(set.Obligations))
	for _, ob := range set.Obligations {
		status := f.checkStatuses[ob.ID]
		if status == "" {
			status = ObligationHolds
		}
		evidence := f.checkEvidence[ob.ID]
		if evidence == nil {
			switch status {
			case ObligationHolds:
				evidence = []string{"EV-HOLDS"}
			case ObligationViolated:
				evidence = []string{"EV-VIOLATION"}
			}
		}
		results = append(results, ObligationResult{ID: ob.ID, Status: status, EvidenceIDs: evidence})
	}
	return ObligationCheck{ObligationsDigest: digest, Results: results}, nil
}

func (f *fakeEngine) GetEvidence(_ context.Context, req EvidenceRequest) (EvidenceBatch, error) {
	f.evidenceCalls++
	if f.evidenceErr != nil {
		return EvidenceBatch{}, f.evidenceErr
	}
	batch := EvidenceBatch{}
	for _, id := range req.IDs {
		rec, ok := f.evidence[id]
		if !ok {
			rec = EvidenceRecord{ID: id, Kind: "witness", Summary: "synthetic witness detail"}
		}
		batch.Records = append(batch.Records, rec)
	}
	return batch, nil
}

func (f *fakeEngine) ListChanges(_ context.Context, req ListChangesRequest) (ChangesPage, error) {
	f.changesCalls++
	if f.changesErr != nil {
		return ChangesPage{}, f.changesErr
	}
	if len(f.changes.Items) > 0 || len(f.changes.EvidenceIDs) > 0 {
		return f.changes, nil
	}
	return ChangesPage{
		Items:       []Change{{ID: "D-1", Kind: "added", Side: "after", Line: intp(3), Summary: "added permit rule"}},
		EvidenceIDs: []string{"EV-CHANGE"},
		Truncated:   false,
	}, nil
}

func (f *fakeEngine) VerifyPatch(_ context.Context, req PatchCheckRequest) (PatchCheckResult, error) {
	f.patchCalls++
	f.lastPatch = req
	if f.patchErr != nil {
		return PatchCheckResult{}, f.patchErr
	}
	if f.patchOverride != nil {
		override := *f.patchOverride
		return override, nil
	}
	status := f.patchStatus
	if status == "" {
		status = PatchVerified
	}
	collateral := f.patchCollateral
	if collateral == "" {
		collateral = CollateralVerified
	}
	evidence := f.patchEvidence
	if evidence == nil {
		evidence = []string{"EV-PATCH"}
	}
	results := make([]ObligationResult, 0, len(f.lastSet.Obligations))
	for _, ob := range f.lastSet.Obligations {
		results = append(results, ObligationResult{ID: ob.ID, Status: ObligationHolds, EvidenceIDs: []string{"EV-PATCH"}})
	}
	return PatchCheckResult{
		Status:            status,
		ObligationsDigest: req.ObligationsDigest,
		ObligationResults: results,
		CollateralStatus:  collateral,
		EvidenceIDs:       evidence,
	}, nil
}

func intp(n int) *int { return &n }

func newSession(t *testing.T, provider ai.Completer, engine Engine, mutate ...func(*Config)) *Session {
	t.Helper()
	cfg := Config{Enabled: true, Provider: provider, Engine: engine, Model: "muse-spark-1.3", RetryBackoff: 0}
	for _, m := range mutate {
		m(&cfg)
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func runSession(t *testing.T, s *Session, in Input) Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := s.Run(ctx, in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

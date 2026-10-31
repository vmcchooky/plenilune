package orchestrate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"plenilune/internal/agent"
	"plenilune/internal/ai"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/repair"
	"strings"
	"sync"
	"testing"
)

type scripted struct {
	step int
	call func(int, ai.Request) (ai.Response, error)
}

func (p *scripted) Complete(_ context.Context, r ai.Request) (ai.Response, error) {
	p.step++
	return p.call(p.step, r)
}

func request() Request {
	s := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
	return Request{Before: "ip access-list extended DEMO\n permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n deny ip any any\n", After: "ip access-list extended DEMO\n deny tcp host 192.0.2.10 any eq 22\n permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n deny ip any any\n", Intent: "Allow SSH from 192.0.2.0/24 to 198.51.100.10, restore the original behavior.", Policy: &repair.Policy{Schema: repair.PolicySchema, Scope: intent.Scope, OperatorAccepted: true, Conditions: s, AllowedChanges: []intent.Region{}}, Enabled: true}
}

func TestFullSessionHasRealRepairAndSeparateAIArtifact(t *testing.T) {
	in := request()
	provider := &scripted{call: func(step int, r ai.Request) (ai.Response, error) {
		out := ai.Response{Model: "scripted-not-live-meta", FinishReason: "tool_calls"}
		switch step {
		case 1:
			if len(r.Messages) < 2 {
				t.Fatal("missing model context")
			}
			var payload struct {
				Accepted *intent.Set `json:"accepted_conditions"`
			}
			if err := json.Unmarshal([]byte(r.Messages[1].Content), &payload); err != nil || payload.Accepted == nil {
				t.Fatal("operator policy absent from model context")
			}
			b, err := json.Marshal(struct {
				Obligations []intent.Obligation `json:"obligations"`
			}{in.Policy.Conditions.Obligations})
			if err != nil {
				return out, err
			}
			out.ToolCalls = []ai.ToolCall{{ID: "check-1", Type: "function", Function: ai.FunctionCall{Name: "check_obligations", Arguments: string(b)}}}
		case 2:
			b, err := json.Marshal(struct {
				Edits     []agent.Edit `json:"edits"`
				Rationale string       `json:"rationale"`
			}{[]agent.Edit{{Op: "remove", Ref: &agent.RuleRef{BaseAfterSHA256: evidence.ConfigDigest([]byte(in.After)), Ordinal: 0, Line: 2}}}, "Remove the shadowing deny."})
			if err != nil {
				return out, err
			}
			out.ToolCalls = []ai.ToolCall{{ID: "patch-1", Type: "function", Function: ai.FunctionCall{Name: "verify_patch", Arguments: string(b)}}}
		case 3:
			out.FinishReason = "stop"
			out.Content = `{"status":"ok","obligations":[],"questions":[],"claims":[],"summary":"Repair checked by the deterministic engine."}`
		default:
			return out, errors.New("unexpected provider call")
		}
		return out, nil
	}}
	out, err := Run(context.Background(), in, provider, "scripted-not-live-meta")
	if err != nil {
		t.Fatal(err)
	}
	if out.AI.Status != agent.StatusCompleted || out.AI.Obligations == nil || !out.AI.Obligations.OperatorAccepted || len(out.Repairs) != 1 || out.Repairs[0].Status != "VERIFIED" {
		t.Fatalf("session not integrated: %+v", out)
	}
	if err := repair.Check(out.Repairs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := repair.Replay(context.Background(), []byte(in.Before), []byte(in.After), out.Repairs[0]); err != nil {
		t.Fatal(err)
	}
	before := out.Repairs[0].SHA256
	out.AI.Summary = "different AI text"
	if out.Repairs[0].SHA256 != before {
		t.Fatal("AI metadata changed deterministic proof")
	}
}

func TestDisabledAndMissingProviderRemainTruthful(t *testing.T) {
	in := request()
	in.Enabled = false
	provider := &scripted{call: func(int, ai.Request) (ai.Response, error) {
		t.Fatal("disabled AI called provider")
		return ai.Response{}, nil
	}}
	out, err := Run(context.Background(), in, provider, "")
	if err != nil || out.AI.Status != agent.StatusDisabled || provider.step != 0 {
		t.Fatalf("disabled: %v %+v", err, out)
	}
	in.Enabled = true
	out, err = Run(context.Background(), in, nil, "")
	if err != nil || out.AI.Status != agent.StatusNotConfigured || len(out.Repairs) != 0 || out.Comparison.Report.Verdict != "DIFFERENT" {
		t.Fatalf("missing key/provider pretended runtime: %v %+v", err, out)
	}
}

func TestOfflineDoesNotApplyProviderExcerptLimits(t *testing.T) {
	in := request()
	name := strings.Repeat("a", 100)
	in.Before = strings.Replace(in.Before, "DEMO", name, 1)
	in.After = strings.Replace(in.After, "DEMO", name, 1)
	for _, enabled := range []bool{false, true} {
		in.Enabled = enabled
		r, err := Run(context.Background(), in, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if r.Comparison.Before.Name != name || r.Comparison.Report.Verdict != "DIFFERENT" {
			t.Fatal("offline core comparison was constrained by provider")
		}
		if !enabled && r.AI.Status != agent.StatusDisabled || enabled && r.AI.Status != agent.StatusNotConfigured {
			t.Fatal("incorrect provider state")
		}
	}
}

func TestHTTPProviderAdapterAndRealRepairEndToEnd(t *testing.T) {
	in := request()
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer synthetic-test-key" {
			t.Error("provider wire routing/auth mismatch")
			http.Error(w, "bad request", 400)
			return
		}
		var wire struct {
			Input []json.RawMessage `json:"input"`
			Tools []json.RawMessage `json:"tools"`
			Text  struct {
				Format json.RawMessage `json:"format"`
			} `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil || len(wire.Tools) != 4 || len(wire.Text.Format) == 0 || len(wire.Input) == 0 {
			t.Errorf("Responses request missing contract: %v", err)
			http.Error(w, "bad request", 400)
			return
		}
		message := ai.Message{Role: "assistant"}
		switch calls {
		case 1:
			b, err := json.Marshal(struct {
				Status      string              `json:"status"`
				Obligations []intent.Obligation `json:"obligations"`
				Questions   []string            `json:"questions"`
				Claims      []agent.Claim       `json:"claims"`
				Summary     string              `json:"summary"`
			}{"ok", in.Policy.Conditions.Obligations, []string{}, []agent.Claim{}, ""})
			if err != nil {
				t.Error(err)
				http.Error(w, "encode", 500)
				return
			}
			message.Content = string(b)
		case 2:
			b, err := json.Marshal(struct {
				Edits     []agent.Edit `json:"edits"`
				Rationale string       `json:"rationale"`
			}{[]agent.Edit{{Op: "remove", Ref: &agent.RuleRef{BaseAfterSHA256: evidence.ConfigDigest([]byte(in.After)), Ordinal: 0, Line: 2}}}, "Restore original SSH behavior."})
			if err != nil {
				t.Error(err)
				http.Error(w, "encode", 500)
				return
			}
			message.ToolCalls = []ai.ToolCall{{ID: "repair-http", Type: "function", Function: ai.FunctionCall{Name: "verify_patch", Arguments: string(b)}}}
		case 3:
			message.Content = `{"status":"ok","obligations":[],"questions":[],"claims":[],"summary":"The engine checked the repair."}`
		default:
			t.Error("unexpected extra HTTP provider request")
			http.Error(w, "unexpected", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		output := []any{}
		if message.Content != "" {
			output = append(output, map[string]any{"type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": message.Content}}})
		}
		for _, call := range message.ToolCalls {
			output = append(output, map[string]any{"type": "function_call", "status": "completed", "call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments})
		}
		response := map[string]any{"id": "synthetic-http-response", "model": "scripted-http-not-live-meta", "status": "completed", "output": output}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := ai.New(ai.Config{APIKey: "synthetic-test-key", BaseURL: server.URL, Model: "scripted-http-not-live-meta"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), in, client, "scripted-http-not-live-meta")
	if err != nil {
		t.Fatal(err)
	}
	if result.AI.Status != agent.StatusCompleted || len(result.Repairs) != 1 || result.Repairs[0].Status != "VERIFIED" || len(result.AI.Provenance.RequestSHA256) != 3 {
		t.Fatalf("HTTP-backed repair loop incomplete: %+v", result.AI)
	}
	for _, h := range result.AI.Provenance.RequestSHA256 {
		if len(h) != 64 {
			t.Fatal("HTTP request provenance missing")
		}
	}
}

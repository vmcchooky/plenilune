package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestResponsesStatelessReasoningAndToolPairReplay(t *testing.T) {
	calls := 0
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if string(body["store"]) != "false" || string(body["stream"]) != "false" || string(body["include"]) != `["reasoning.encrypted_content"]` || body["messages"] != nil {
			t.Fatalf("wrong Responses flags")
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"id":"resp1","status":"completed","output":[{"id":"rs1","type":"reasoning","encrypted_content":"opaque-secret-state"},{"type":"function_call","status":"completed","call_id":"call1","name":"get_evidence","arguments":"{}"}]}`))
			return
		}
		var items []struct {
			Type      string          `json:"type"`
			CallID    string          `json:"call_id"`
			Encrypted string          `json:"encrypted_content"`
			Summary   json.RawMessage `json:"summary"`
		}
		if err := json.Unmarshal(body["input"], &items); err != nil {
			t.Fatal(err)
		}
		if len(items) != 4 || items[1].Type != "reasoning" || items[1].Encrypted != "opaque-secret-state" || string(items[1].Summary) != "[]" || items[2].Type != "function_call" || items[3].Type != "function_call_output" || items[2].CallID != items[3].CallID {
			t.Fatalf("reasoning/tool pairing not preserved: %+v", items)
		}
		_, _ = w.Write([]byte(`{"id":"resp2","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"done"}]}]}`))
	})
	first, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "check"}}, MaxTokens: testTokens})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), Request{MaxTokens: testTokens, Messages: []Message{
		{Role: "user", Content: "check"},
		{Role: "assistant", ToolCalls: first.ToolCalls, ReplayItems: first.ReplayItems},
		{Role: "tool", ToolCallID: "call1", Content: `{"ok":true}`},
	}})
	if err != nil || calls != 2 {
		t.Fatalf("replay: %v, calls %d", err, calls)
	}
	encoded, _ := json.Marshal(Message{Role: "assistant", ReplayItems: first.ReplayItems})
	if strings.Contains(string(encoded), "opaque-secret-state") {
		t.Fatal("opaque provider state exported")
	}
}

func TestResponsesTerminalStatesAndModelBinding(t *testing.T) {
	cases := []struct {
		name, body, finish string
		bad                bool
	}{
		{"incomplete text", `{"id":"r","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","status":"incomplete","content":[{"type":"output_text","text":"partial"}]}]}`, "length", false},
		{"incomplete tool", `{"id":"r","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"function_call","status":"completed","call_id":"c","name":"get_evidence","arguments":"{}"}]}`, "", true},
		{"failed", `{"id":"r","status":"failed","output":[]}`, "", true},
		{"wrong model", `{"id":"r","model":"muse-wonder","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"x"}]}]}`, "", true},
		{"refusal", `{"id":"r","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"refusal"}]}]}`, "content_filter", false},
		{"refusal plus tool", `{"id":"r","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"refusal"}]},{"type":"function_call","status":"completed","call_id":"c","name":"get_evidence","arguments":"{}"}]}`, "content_filter", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := decodeResponses([]byte(tc.body), DefaultModel)
			if (err != nil) != tc.bad || err == nil && r.FinishReason != tc.finish {
				t.Fatalf("response %s, err %v", r.FinishReason, err)
			}
		})
	}
}

func TestModelOverrideNeverReachesProvider(t *testing.T) {
	client, _ := newTestClient(t, func(http.ResponseWriter, *http.Request) { t.Fatal("model override reached provider") })
	_, err := client.Complete(context.Background(), Request{Model: "muse-wonder", Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens})
	if err == nil {
		t.Fatal("model override was accepted")
	}
	if _, err := New(Config{APIKey: testKey, Model: "muse-wonder"}); err == nil {
		t.Fatal("public provider allowed non-contributor model")
	}
}

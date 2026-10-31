package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testKey = "sk-test-secret-value"
const testTokens = 3000

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(Config{APIKey: testKey, BaseURL: server.URL, Model: DefaultModel})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client, server
}

func TestCompleteWireShapeAndParse(t *testing.T) {
	var received map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/responses" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testKey {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"resp_1","model":"muse-spark-1.3-contributor",
			"status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"status\":\"ok\"}"}]}],
			"usage":{"input_tokens":12,"output_tokens":4,"total_tokens":16}
		}`)
	})
	temp := 0.0
	resp, err := client.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "usr"},
		},
		Tools: []Tool{{Type: "function", Function: ToolFunction{
			Name: "check_obligations", Description: "check", Parameters: json.RawMessage(`{"type":"object"}`),
		}}},
		ResponseFormat: &ResponseFormat{Type: "json_schema", JSONSchema: &JSONSchema{
			Name: "Envelope", Schema: json.RawMessage(`{"type":"object"}`),
		}},
		MaxTokens:   1200,
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != `{"status":"ok"}` || resp.Model != "muse-spark-1.3-contributor" || resp.FinishReason != "stop" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Usage.PromptTokens != 12 || resp.Usage.CompletionTokens != 4 || resp.Usage.TotalTokens != 16 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if len(resp.RequestSHA256) != 64 || len(resp.ResponseSHA256) != 64 {
		t.Fatalf("missing hashes: %+v", resp)
	}
	if received["model"] != "muse-spark-1.3-contributor" || received["tool_choice"] != "auto" {
		t.Fatalf("request fields = %v", received)
	}
	tools, ok := received["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v", received["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "check_obligations" {
		t.Fatalf("tool shape = %v", tool)
	}
	format := received["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("response_format = %v", format)
	}
	if _, ok := format["schema"].(map[string]any); !ok {
		t.Fatalf("json_schema shape = %v", format)
	}
	if received["max_output_tokens"].(float64) != 1200 || received["temperature"].(float64) != 0 {
		t.Fatalf("generation params = %v", received)
	}
}

func TestCompleteParsesToolCalls(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"resp_tool","status":"completed","output":[
			{"type":"function_call","status":"completed","call_id":"call_1","name":"get_evidence","arguments":"{\"ids\":[\"E-1\"]}"}
		],"usage":{"input_tokens":10,"output_tokens":5}}`)
	})
	resp, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_1" ||
		resp.ToolCalls[0].Function.Name != "get_evidence" {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %q", resp.FinishReason)
	}
}

func TestCompleteProviderErrors(t *testing.T) {
	tests := []struct {
		status    int
		body      string
		retriable bool
	}{
		{401, `{"error":{"message":"bad key ` + testKey + `","type":"auth","code":"invalid_api_key"}}`, false},
		{403, `{"error":{"message":"forbidden","code":"forbidden"}}`, false},
		{400, `{"error":{"message":"schema rejected","code":"invalid_request"}}`, false},
		{429, `{"error":{"message":"slow down","code":"rate_limited"}}`, true},
		{500, `{"error":{"message":"upstream","code":"server_error"}}`, true},
		{503, `not json`, true},
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens})
			if err == nil {
				t.Fatal("expected error")
			}
			var se *StatusError
			if !errors.As(err, &se) {
				t.Fatalf("error type = %T (%v)", err, err)
			}
			if se.Status != tc.status || se.Retriable != tc.retriable {
				t.Fatalf("status error = %+v", se)
			}
			if strings.Contains(err.Error(), testKey) {
				t.Fatalf("error leaked key: %v", err)
			}
			if len(se.RequestSHA256) != 64 {
				t.Fatalf("missing request hash")
			}
		})
	}
}

func TestCompleteRejectsMalformedResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
		code string
	}{
		{"invalid json", `not json`, "invalid_response_json"},
		{"empty output", `{"id":"r","status":"completed","output":[]}`, "invalid_response"},
		{"duplicate tool id", `{"id":"r","status":"completed","output":[
			{"type":"function_call","status":"completed","call_id":"a","name":"get_evidence","arguments":"{}"},
			{"type":"function_call","status":"completed","call_id":"a","name":"get_evidence","arguments":"{}"}]}`, "invalid_response"},
		{"bad arguments", `{"id":"r","status":"completed","output":[
			{"type":"function_call","status":"completed","call_id":"a","name":"get_evidence","arguments":"{"}]}`, "invalid_response"},
		{"bad tool name", `{"id":"r","status":"completed","output":[
			{"type":"function_call","status":"completed","call_id":"a","name":"Bad-Name","arguments":"{}"}]}`, "invalid_response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens})
			var te *TransportError
			if !errors.As(err, &te) || te.Code != tc.code {
				t.Fatalf("error = %v, want code %s", err, tc.code)
			}
		})
	}
}

func TestCompleteResponseTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 100))
	}))
	defer server.Close()
	client, err := New(Config{APIKey: testKey, BaseURL: server.URL, MaxResponseBytes: 64})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens})
	var te *TransportError
	if !errors.As(err, &te) || te.Code != "response_too_large" {
		t.Fatalf("error = %v", err)
	}
}

func TestCompleteRequestTooLarge(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()
	client, err := New(Config{APIKey: testKey, BaseURL: server.URL, MaxRequestBytes: 128})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: strings.Repeat("a", 1000)}}, MaxTokens: testTokens})
	var te *TransportError
	if !errors.As(err, &te) || te.Code != "request_too_large" {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("oversized request reached the provider")
	}
}

func TestCompleteDoesNotFollowRedirects(t *testing.T) {
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls++
	}))
	defer target.Close()
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	})
	_, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens})
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusFound {
		t.Fatalf("error = %v", err)
	}
	if targetCalls != 0 {
		t.Fatalf("redirect was followed %d times", targetCalls)
	}
}

func TestCompleteCancelAndTimeout(t *testing.T) {
	release := make(chan struct{})
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := client.Complete(ctx, Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestCompleteInputValidation(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	long := strings.Repeat("x", 5000)
	cases := []struct {
		name string
		req  Request
	}{
		{"no messages", Request{MaxTokens: testTokens}},
		{"bad role", Request{Messages: []Message{{Role: "developer", Content: "x"}}, MaxTokens: testTokens}},
		{"tool without id", Request{Messages: []Message{{Role: "tool", Content: "x"}}, MaxTokens: testTokens}},
		{"bad tool", Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens, Tools: []Tool{{Type: "function"}}}},
		{"too many tools", Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens, Tools: make([]Tool, 9)}},
		{"long description", Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens, Tools: []Tool{{Type: "function",
			Function: ToolFunction{Name: "ok", Description: long, Parameters: json.RawMessage(`{}`)}}}}},
		{"bad response format", Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens,
			ResponseFormat: &ResponseFormat{Type: "json_object"}}},
		{"missing max tokens", Request{Messages: []Message{{Role: "user", Content: "x"}}}},
		{"max tokens", Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: 5000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := client.Complete(context.Background(), tc.req); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	temp := 3.0
	if _, err := client.Complete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: testTokens, Temperature: &temp,
	}); err == nil {
		t.Fatal("expected temperature error")
	}
}

func TestMissingMaxTokensNeverReachesProvider(t *testing.T) {
	called := false
	client, _ := newTestClient(t, func(http.ResponseWriter, *http.Request) { called = true })
	_, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}})
	if err == nil || called {
		t.Fatalf("missing max_tokens was not rejected before the network: err=%v called=%t", err, called)
	}
	if !strings.Contains(err.Error(), "max_tokens") {
		t.Fatalf("error does not name max_tokens: %v", err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	if _, err := ConfigFromEnv(func(string) string { return "" }); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error = %v", err)
	}
	env := map[string]string{"META_MODEL_API_KEY": testKey}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.Model != DefaultModel || cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("defaults = %+v", cfg)
	}
	if strings.Contains(cfg.String(), testKey) {
		t.Fatal("config string leaked the key")
	}
	env["PLENILUNE_META_BASE_URL"] = "http://api.meta.ai/v1"
	if _, err := ConfigFromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("expected https enforcement")
	}
	env["PLENILUNE_META_BASE_URL"] = "http://127.0.0.1:8080/v1"
	if _, err := ConfigFromEnv(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("loopback should be allowed: %v", err)
	}
}

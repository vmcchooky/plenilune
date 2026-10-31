package ai

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// This test spends real Meta Model API quota. It is opt-in, prints no key and
// no raw prompt/response. Run: PLENILUNE_LIVE_META=1 go test -run TestLiveMetaStructuredOutput ./internal/ai
func TestLiveMetaStructuredOutput(t *testing.T) {
	if os.Getenv("PLENILUNE_LIVE_META") != "1" {
		t.Skip("set PLENILUNE_LIVE_META=1 and META_MODEL_API_KEY to run the live smoke")
	}
	cfg, err := ConfigFromEnv(os.Getenv)
	if err != nil {
		t.Fatalf("live smoke is enabled but not configured: %v", err)
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	temp := 0.0
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := client.Complete(ctx, Request{
		Messages: []Message{
			{Role: "system", Content: "Return only the requested JSON object. Do not add prose."},
			{Role: "user", Content: `Return {"ok":true,"model":"<your model id>"} exactly.`},
		},
		ResponseFormat: &ResponseFormat{Type: "json_schema", JSONSchema: &JSONSchema{
			Name:   "SmokeResult",
			Schema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["ok","model"],"properties":{"ok":{"type":"boolean"},"model":{"type":"string"}}}`),
		}},
		MaxTokens:   64,
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("live Meta smoke failed: %v", err)
	}
	var out struct {
		OK    bool   `json:"ok"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
		t.Fatalf("structured content did not parse: %v", err)
	}
	if !out.OK {
		t.Fatal("model did not return ok=true")
	}
	if len(resp.RequestSHA256) != 64 || len(resp.ResponseSHA256) != 64 {
		t.Fatal("missing request/response hashes")
	}
	t.Logf("live meta ok configured=%s reported=%s prompt_tokens=%d completion_tokens=%d elapsed_ms=%d",
		cfg.Model, resp.Model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, time.Since(start).Milliseconds())
}

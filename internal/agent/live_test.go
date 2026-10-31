package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"plenilune/internal/ai"
)

// The engine is synthetic here: this test exercises the live Meta provider and
// tool-loop wire behavior only. It is not evidence about verifier or repair
// correctness. Opt in with PLENILUNE_LIVE_META_AGENT=1 plus META_MODEL_API_KEY.
func TestLiveMetaAgentSession(t *testing.T) {
	if os.Getenv("PLENILUNE_LIVE_META_AGENT") != "1" {
		t.Skip("set PLENILUNE_LIVE_META_AGENT=1 and META_MODEL_API_KEY to run the live agent smoke")
	}
	cfg, err := ai.ConfigFromEnv(os.Getenv)
	if err != nil {
		t.Fatalf("live agent smoke is enabled but not configured: %v", err)
	}
	client, err := ai.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	engine := &fakeEngine{}
	session, err := New(Config{
		Enabled:      true,
		Provider:     client,
		Engine:       engine,
		Model:        cfg.Model,
		RetryBackoff: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := session.Run(ctx, testInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Provenance.Requests == 0 {
		t.Fatal("live agent made no model request")
	}
	if res.Status == StatusUnavailable || res.Status == StatusCanceled {
		t.Fatalf("live agent did not complete: %s/%s", res.Status, res.Reason)
	}
	t.Logf("live agent status=%s requests=%d tool_calls=%d repairs=%d checks=%d",
		res.Status, res.Provenance.Requests, res.Provenance.ToolCalls, res.Provenance.RepairProposals, len(res.Checks))
}

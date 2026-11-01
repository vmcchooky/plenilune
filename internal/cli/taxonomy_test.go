package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"plenilune/internal/errtax"
	"plenilune/internal/evidence"
	"plenilune/internal/jsonio"
)

// P2-3 acceptance: every stage of the unified taxonomy has at least
// one representative error produced through the real CLI surface,
// carrying code + stage + retryable (and field/line where the stage
// supports them). Exit codes are the pre-taxonomy ones.

func decodeErrEnvelope(t *testing.T, stderr string) errtax.Envelope {
	t.Helper()
	var env errtax.Envelope
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr is not an error envelope: %q: %v", stderr, err)
	}
	if env.Code == "" || env.Stage == "" || env.Message == "" {
		t.Fatalf("envelope missing code/stage/message: %q", stderr)
	}
	return env
}

func TestTaxonomyCLIRepresentatives(t *testing.T) {
	dir := t.TempDir()
	goodACL := "ip access-list extended DEMO\n permit tcp host 192.0.2.10 host 198.51.100.20 eq 22\n deny ip any any\n"
	before := put(t, dir, "before.cfg", goodACL)
	after := put(t, dir, "after.cfg", goodACL)

	t.Run("request/USAGE", func(t *testing.T) {
		code, _, stderr := invoke("bogus-command")
		env := decodeErrEnvelope(t, stderr)
		if code != 2 || env.Code != "USAGE" || env.Stage != errtax.StageRequest || env.Retryable {
			t.Fatalf("exit %d envelope %+v", code, env)
		}
	})

	t.Run("request/CANCELED is retryable", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out, errOut bytes.Buffer
		code := Run(ctx, []string{"parse", "--config", before}, &out, &errOut)
		env := decodeErrEnvelope(t, errOut.String())
		if code != 2 || env.Code != "CANCELED" || env.Stage != errtax.StageRequest || !env.Retryable {
			t.Fatalf("exit %d envelope %+v", code, env)
		}
	})

	t.Run("parse detail carries stage and line", func(t *testing.T) {
		bad := put(t, dir, "bad.cfg", "ip access-list extended DEMO\n permit tcp any any eq banana\n")
		code, _, stderr := invoke("parse", "--config", bad)
		env := decodeErrEnvelope(t, stderr)
		if code != 2 || !strings.HasPrefix(env.Code, "PARSE_") || env.Stage != errtax.StageParse || env.Retryable {
			t.Fatalf("exit %d envelope %+v", code, env)
		}
		if env.Line != 2 {
			t.Fatalf("parse envelope line = %d, want 2 (%+v)", env.Line, env)
		}
	})

	t.Run("validate/REQUEST_SCHEMA carries field", func(t *testing.T) {
		req := put(t, dir, "req.json", `{"schema":"plenilune.analysis-request/v999","before":{"text":""},"after":{"text":""}}`)
		code, _, stderr := invoke("analyze", "--request", req)
		env := decodeErrEnvelope(t, stderr)
		if code != 2 || env.Code != "REQUEST_SCHEMA" || env.Stage != errtax.StageValidate || env.Field != "schema" || env.Retryable {
			t.Fatalf("exit %d envelope %+v", code, env)
		}
	})

	t.Run("engine/COMPARISON_FAILED", func(t *testing.T) {
		code, _, stderr := invoke("diff", "--before", before, "--after", after, "--max-operations", "3000000")
		env := decodeErrEnvelope(t, stderr)
		if code != 2 || env.Code != "COMPARISON_FAILED" || env.Stage != errtax.StageEngine || env.Retryable {
			t.Fatalf("exit %d envelope %+v", code, env)
		}
	})

	t.Run("provider/AI_MODEL_UNSUPPORTED", func(t *testing.T) {
		t.Setenv("META_MODEL_API_KEY", "test-not-used")
		t.Setenv("PLENILUNE_META_MODEL", "some-other-model")
		code, _, stderr := invoke("assist", "--before", before, "--after", after, "--intent", "Keep denied.", "--ai", "on")
		env := decodeErrEnvelope(t, stderr)
		if code != 2 || env.Code != "AI_MODEL_UNSUPPORTED" || env.Stage != errtax.StageProvider || env.Retryable {
			t.Fatalf("exit %d envelope %+v", code, env)
		}
	})
}

func TestTaxonomyCLIReplayMismatch(t *testing.T) {
	dir := t.TempDir()
	config := put(t, dir, "acl.cfg", "ip access-list extended DEMO\n permit tcp host 192.0.2.10 host 198.51.100.20 eq 22\n deny ip any any\n")
	packet := put(t, dir, "packet.json", `{"protocol":6,"src_ip":"192.0.2.10","dst_ip":"198.51.100.20","src_port":40000,"dst_port":22}`)
	code, body, errout := invoke("trace", "--config", config, "--packet", packet)
	if code != 0 || errout != "" {
		t.Fatalf("trace: %d %s", code, errout)
	}
	var trace evidence.Trace
	if err := jsonio.Decode([]byte(body), evidence.MaxTraceBytes, &trace); err != nil {
		t.Fatal(err)
	}
	// A self-consistently rehashed false decision: the seal verifies,
	// semantic replay must refuse it at the replay stage.
	trace.Decision.Action = "deny"
	tampered, err := evidence.SealTrace(trace.ConfigSHA256, trace.Packet, trace.Decision)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	proof := put(t, dir, "trace.json", string(raw))
	code, _, stderr := invoke("verify", "--config", config, "--evidence", proof)
	env := decodeErrEnvelope(t, stderr)
	if code != 2 || env.Code != "EVIDENCE_MISMATCH" || env.Stage != errtax.StageReplay || env.Retryable {
		t.Fatalf("exit %d envelope %+v", code, env)
	}
}

// Budget exhaustion is a marked domain outcome, never an unmarked
// partial success: a compare stopped by the operations budget is
// INCONCLUSIVE with search_complete=false, not EQUIVALENT.
func TestTaxonomyCLIBudgetStopIsMarkedInconclusive(t *testing.T) {
	dir := t.TempDir()
	acl := "ip access-list extended DEMO\n permit tcp host 192.0.2.10 host 198.51.100.20 eq 22\n deny ip any any\n"
	before := put(t, dir, "before.cfg", acl)
	after := put(t, dir, "after.cfg", acl)
	code, body, _ := invoke("diff", "--before", before, "--after", after, "--max-operations", "1")
	if code != 2 {
		t.Fatalf("budget-stopped diff exit %d, want 2; body %s", code, body)
	}
	var artifact evidence.Comparison
	if err := json.Unmarshal([]byte(body), &artifact); err != nil {
		t.Fatalf("diff output is not a comparison artifact: %v", err)
	}
	if artifact.Report.Verdict != "INCONCLUSIVE" || artifact.Report.SearchComplete {
		t.Fatalf("budget stop masquerades as success: verdict %q search_complete %v", artifact.Report.Verdict, artifact.Report.SearchComplete)
	}
}

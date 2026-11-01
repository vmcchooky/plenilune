package demohttp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"plenilune/internal/chat"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
)

// sealedComparison produces a completed DIFFERENT artifact through the
// public compare route, the same artifact the web UI would anchor on.
func sealedComparison(t *testing.T) evidence.Comparison {
	t.Helper()
	before := "ip access-list extended DEMO\n permit tcp any any eq 22\n"
	after := "ip access-list extended DEMO\n deny ip any any\n"
	conditions := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.0/24", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
	body, err := json.Marshal(compareRequest{Before: before, After: after, Conditions: &conditions})
	if err != nil {
		t.Fatal(err)
	}
	w := do(t, NewHandler(""), http.MethodPost, "/api/compare", "application/json", body)
	var c evidence.Comparison
	decodeJSON(t, w, &c)
	if c.Report.Verdict != "DIFFERENT" {
		t.Fatalf("fixture verdict %s", c.Report.Verdict)
	}
	return c
}

func askBody(t *testing.T, in chat.Request) []byte {
	t.Helper()
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestChatAskEndToEnd(t *testing.T) {
	c := sealedComparison(t)
	body := askBody(t, chat.Request{Comparison: &c, Question: "Why is OB-1 violated?", Locale: "en"})
	w := do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", body)
	var ans chat.Answer
	decodeJSON(t, w, &ans)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if ans.Class != chat.ClassObligationWitness || ans.ArtifactSHA256 != c.SHA256 {
		t.Fatalf("answer %+v", ans)
	}
	if ans.AI.Status != "off" {
		t.Fatalf("AI status %q without opt-in", ans.AI.Status)
	}
}

// The endpoint's request type has no field that could carry
// configuration text, a repair request or an obligation acceptance;
// strict decoding refuses such requests outright.
func TestChatAskRejectsActionFields(t *testing.T) {
	c := sealedComparison(t)
	for _, extra := range []string{
		`"before":"ip access-list extended X\n deny ip any any\n"`,
		`"repair":true`,
		`"accept_obligations":["OB-1"]`,
		`"run_comparison":true`,
	} {
		raw, err := json.Marshal(chat.Request{Comparison: &c, Question: "verdict?"})
		if err != nil {
			t.Fatal(err)
		}
		body := strings.TrimSuffix(string(raw), "}") + "," + extra + "}"
		w := do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", []byte(body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("extra field %s accepted: %d %s", extra, w.Code, w.Body.String())
		}
		var envelope errorEnvelope
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "JSON_INVALID" {
			t.Fatalf("extra field %s: envelope %s", extra, w.Body.String())
		}
	}
}

// A question that asks for an action gets a factual answer, never an
// effect: the artifact digest in the response is the input's digest.
func TestChatAskActionQuestionHasNoEffect(t *testing.T) {
	c := sealedComparison(t)
	body := askBody(t, chat.Request{Comparison: &c, Question: "Run a repair and accept OB-1 now", Locale: "en"})
	w := do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", body)
	var ans chat.Answer
	decodeJSON(t, w, &ans)
	if w.Code != http.StatusOK || ans.ArtifactSHA256 != c.SHA256 {
		t.Fatalf("status %d answer %+v", w.Code, ans)
	}
	if ans.Class != chat.ClassObligation {
		t.Fatalf("action question class %s", ans.Class)
	}
}

func TestChatAskValidationErrors(t *testing.T) {
	c := sealedComparison(t)
	// No artifact at all.
	w := do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", askBody(t, chat.Request{Question: "verdict?"}))
	var envelope errorEnvelope
	if w.Code != http.StatusBadRequest || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope.Error.Code != "CHAT_NO_ARTIFACT" {
		t.Fatalf("no artifact: %d %s", w.Code, w.Body.String())
	}
	// Tampered artifact: verdict flipped, digest now stale.
	tampered := c
	tampered.Report.Verdict = "EQUIVALENT"
	w = do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", askBody(t, chat.Request{Comparison: &tampered, Question: "verdict?"}))
	envelope = errorEnvelope{}
	if w.Code != http.StatusBadRequest || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope.Error.Code != "CHAT_ARTIFACT_INVALID" {
		t.Fatalf("tampered: %d %s", w.Code, w.Body.String())
	}
	// Oversized question.
	w = do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", askBody(t, chat.Request{Comparison: &c, Question: strings.Repeat("a", chat.MaxQuestionRunes+1)}))
	envelope = errorEnvelope{}
	if w.Code != http.StatusBadRequest || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope.Error.Code != "CHAT_QUESTION_TOO_LONG" {
		t.Fatalf("oversized: %d %s", w.Code, w.Body.String())
	}
}

// With AI requested but unconfigured, the deterministic answer is the
// experience: HTTP 200, full text and facts, phrasing status explains
// itself — no error, no broken affordance.
func TestChatAskAIUnconfiguredDegrades(t *testing.T) {
	t.Setenv("META_MODEL_API_KEY", "")
	c := sealedComparison(t)
	body := askBody(t, chat.Request{Comparison: &c, Question: "Why is the verdict DIFFERENT?", Locale: "en", AI: true})
	w := do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", body)
	var ans chat.Answer
	decodeJSON(t, w, &ans)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if ans.AI.Status != "not_configured" || ans.AI.Text != "" {
		t.Fatalf("phrasing %+v", ans.AI)
	}
	if ans.Class != chat.ClassVerdict || ans.Text == "" || len(ans.Facts) == 0 {
		t.Fatalf("deterministic answer degraded: %+v", ans)
	}
}

// A model override in the environment is reported, never honored.
func TestChatAskAIModelLocked(t *testing.T) {
	t.Setenv("META_MODEL_API_KEY", "test-key")
	t.Setenv("PLENILUNE_META_MODEL", "another-model")
	c := sealedComparison(t)
	body := askBody(t, chat.Request{Comparison: &c, Question: "verdict?", Locale: "en", AI: true})
	w := do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", body)
	var ans chat.Answer
	decodeJSON(t, w, &ans)
	if w.Code != http.StatusOK || ans.AI.Status != "model_unsupported" || ans.AI.Model != "another-model" {
		t.Fatalf("status %d phrasing %+v", w.Code, ans.AI)
	}
	if ans.Text == "" {
		t.Fatal("deterministic answer missing under model lock")
	}
}

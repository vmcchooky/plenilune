package demohttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/orchestrate"
	"plenilune/internal/patch"
	"plenilune/internal/repair"
	"plenilune/internal/workflow"
	"testing"
)

func TestRepairHTTPActualProofAndReplay(t *testing.T) {
	b := "ip access-list extended DEMO\n permit tcp any any eq 22\n deny ip any any\n"
	a := "ip access-list extended DEMO\n deny tcp host 192.0.2.10 any eq 22\n permit tcp any any eq 22\n deny ip any any\n"
	s := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.10/32", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
	s, h, err := intent.Digest(s)
	if err != nil {
		t.Fatal(err)
	}
	p := repair.Policy{Schema: repair.PolicySchema, Scope: intent.Scope, OperatorAccepted: true, Conditions: s, AllowedChanges: []intent.Region{}}
	req := patch.Request{Schema: patch.Schema, Scope: intent.Scope, BaseBeforeSHA256: evidence.ConfigDigest([]byte(b)), BaseAfterSHA256: evidence.ConfigDigest([]byte(a)), ObligationsDigest: h, Edits: []patch.Edit{{Op: "remove", Ref: &patch.RuleRef{BaseAfterSHA256: evidence.ConfigDigest([]byte(a)), Ordinal: 0, Line: 2}}}}
	body, err := json.Marshal(struct {
		Before string        `json:"before"`
		After  string        `json:"after"`
		Policy repair.Policy `json:"policy"`
		Patch  patch.Request `json:"patch"`
	}{b, a, p, req})
	if err != nil {
		t.Fatal(err)
	}
	w := do(t, NewHandler(""), http.MethodPost, "/api/repair/verify", "application/json", body)
	var proof repair.Result
	if err := json.Unmarshal(w.Body.Bytes(), &proof); err != nil || w.Code != 200 || proof.Status != "VERIFIED" {
		t.Fatalf("repair: %d %s", w.Code, w.Body.String())
	}
	body, err = json.Marshal(struct {
		Before   string        `json:"before"`
		After    string        `json:"after"`
		Evidence repair.Result `json:"evidence"`
	}{b, a, proof})
	if err != nil {
		t.Fatal(err)
	}
	w = do(t, NewHandler(""), http.MethodPost, "/api/repair/replay", "application/json", body)
	var replay workflow.ReplayResult
	if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil || w.Code != 200 || replay.Status != "REPAIR_REPLAYED" {
		t.Fatalf("replay: %d %s", w.Code, w.Body.String())
	}
}

func TestAIHTTPDisabledNeverReadsInvalidProviderConfig(t *testing.T) {
	t.Setenv("META_MODEL_API_KEY", "test-must-not-be-used")
	t.Setenv("PLENILUNE_META_BASE_URL", "not-a-url")
	in := orchestrate.Request{Before: "ip access-list extended DEMO\n deny ip any any\n", After: "ip access-list extended DEMO\n deny ip any any\n", Enabled: false}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	w := do(t, NewHandler(""), http.MethodPost, "/api/agent/session", "application/json", b)
	var result orchestrate.Result
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.AI.Status != "disabled" {
		t.Fatalf("AI-off read provider config: %d %s", w.Code, w.Body.String())
	}
}

func TestAIRejectsModelOutsideMuseSpark(t *testing.T) {
	t.Setenv("META_MODEL_API_KEY", "test-key")
	t.Setenv("PLENILUNE_META_MODEL", "another-model")
	in := orchestrate.Request{Before: "ip access-list extended DEMO\n deny ip any any\n", After: "ip access-list extended DEMO\n deny ip any any\n", Intent: "Keep the ACL unchanged.", Enabled: true}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/agent/session", bytes.NewReader(b))
	r.Host = "127.0.0.1:8080"
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewHandler("").ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected model guard to reject: %d %s", w.Code, w.Body.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "AI_MODEL_UNSUPPORTED" {
		t.Fatalf("unexpected model guard error: %s", w.Body.String())
	}
}

func TestAIOriginAndMissingProvider(t *testing.T) {
	t.Setenv("META_MODEL_API_KEY", "")
	in := orchestrate.Request{Before: "ip access-list extended DEMO\n deny ip any any\n", After: "ip access-list extended DEMO\n deny ip any any\n", Intent: "Keep SSH denied.", Enabled: true}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host, origin string
		status       int
	}{{"evil.example", "", 403}, {"127.0.0.1:8080", "https://evil.example", 403}, {"127.0.0.1:8080", "http://127.0.0.1:5173", 200}} {
		r := httptest.NewRequest(http.MethodPost, "/api/agent/session", bytes.NewReader(b))
		r.Host = tc.host
		r.Header.Set("Content-Type", "application/json")
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		NewHandler("").ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("origin check: %s %s: %d %s", tc.host, tc.origin, w.Code, w.Body.String())
		}
		if tc.status == 200 {
			var result orchestrate.Result
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.AI.Status != "not_configured" {
				t.Fatal("missing key not reported")
			}
		}
	}
}

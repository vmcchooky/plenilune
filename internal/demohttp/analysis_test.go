package demohttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"plenilune/internal/analysis"
)

func configPair(t *testing.T, name string) (string, string) {
	t.Helper()
	read := func(side string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", name, side+".cfg"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return read("before"), read("after")
}

func postJSON(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	req.Host = "127.0.0.1:8080" // the API guard only accepts loopback authorities
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewHandler("").ServeHTTP(rec, req)
	return rec
}

func TestAnalyzeHTTPInventoryAndReplay(t *testing.T) {
	before, after := configPair(t, "routing")
	rec := postJSON(t, "/api/analyze", analysis.Request{Schema: analysis.RequestSchema, Before: analysis.Source{Text: before}, After: analysis.Source{Text: after}})
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze HTTP %d %s", rec.Code, rec.Body.String())
	}
	var result analysis.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != analysis.Schema || result.Summary.Changed != 1 || result.SHA256 == "" {
		t.Fatalf("result %+v", result.Summary)
	}
	replay := postJSON(t, "/api/analyze/replay", map[string]any{"before": before, "after": after, "evidence": result})
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body.String(), "ANALYSIS_REPLAYED") {
		t.Fatalf("replay HTTP %d %s", replay.Code, replay.Body.String())
	}
	mismatch := postJSON(t, "/api/analyze/replay", map[string]any{"before": before + "\n", "after": after, "evidence": result})
	if mismatch.Code != http.StatusBadRequest || !strings.Contains(mismatch.Body.String(), "EVIDENCE_MISMATCH") {
		t.Fatalf("mismatch HTTP %d %s", mismatch.Code, mismatch.Body.String())
	}
}

func TestAnalyzeHTTPRejectsInvalidRequests(t *testing.T) {
	for name, body := range map[string]string{
		"schema":  `{"schema":"x","before":{"text":""},"after":{"text":""}}`,
		"unknown": `{"schema":"plenilune.analysis-request/v1","before":{"text":""},"after":{"text":""},"extra":1}`,
		"policy":  `{"schema":"plenilune.analysis-request/v1","before":{"text":""},"after":{"text":""},"policies":[{"domain":"prefix-list","name":"X","policy":{}}]}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/analyze", strings.NewReader(body))
		req.Host = "127.0.0.1:8080" // the API guard only accepts loopback authorities
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		NewHandler("").ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code"`) {
			t.Fatalf("%s: HTTP %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

package demohttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"plenilune/internal/analysis"
	"plenilune/internal/chat"
	"plenilune/internal/errtax"
)

// P2-3 acceptance on the HTTP surface: error responses carry the
// unified taxonomy fields (code + stage + retryable, field/line
// where the stage supports them) inside the existing
// {"error": ...} envelope, with unchanged status codes and codes.

func TestTaxonomyHTTPChatStrictDecodeKeepsJSONInvalid(t *testing.T) {
	c := sealedComparison(t)
	raw, err := json.Marshal(chat.Request{Comparison: &c, Question: "verdict?"})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSuffix(string(raw), "}") + `,"before":"ip access-list extended X\n deny ip any any\n"}`
	w := do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", []byte(body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w)
	if e.Code != "JSON_INVALID" || e.Stage != errtax.StageRequest || e.Retryable {
		t.Fatalf("strict-decode error = %+v, want JSON_INVALID/request/non-retryable", e)
	}
}

func TestTaxonomyHTTPChatArtifactInvalidIsReplayStage(t *testing.T) {
	c := sealedComparison(t)
	c.Tool.Commit = "tampered" // breaks the seal without touching the payload shape
	body := askBody(t, chat.Request{Comparison: &c, Question: "verdict?"})
	w := do(t, NewHandler(""), http.MethodPost, "/api/chat/ask", "application/json", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w)
	if e.Code != "CHAT_ARTIFACT_INVALID" || e.Stage != errtax.StageReplay || e.Retryable {
		t.Fatalf("artifact error = %+v, want CHAT_ARTIFACT_INVALID/replay/non-retryable", e)
	}
}

func TestTaxonomyHTTPParseDetailCarriesStageAndLine(t *testing.T) {
	body, err := json.Marshal(parseRequest{
		Before: "ip access-list extended DEMO\n permit tcp any any eq banana\n",
		After:  "ip access-list extended DEMO\n deny ip any any\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	w := do(t, NewHandler(""), http.MethodPost, "/api/parse", "application/json", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w)
	if !strings.HasPrefix(e.Code, "PARSE_") || e.Stage != errtax.StageParse || e.Retryable {
		t.Fatalf("parse error = %+v, want PARSE_*/parse/non-retryable", e)
	}
	if e.Line != 2 || e.Side != "before" {
		t.Fatalf("parse error position = line %d side %q, want 2/before", e.Line, e.Side)
	}
}

// A request whose deadline has already passed when the analysis
// starts fails as a budget-stage error — it never returns a partial
// artifact masquerading as a result.
func TestTaxonomyHTTPAnalyzeDeadlineExceededIsBudgetStage(t *testing.T) {
	body, err := json.Marshal(analysis.Request{
		Schema: analysis.RequestSchema,
		Before: analysis.Source{Text: "ip access-list extended DEMO\n permit ip any any\n"},
		After:  analysis.Source{Text: "ip access-list extended DEMO\n permit ip any any\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", strings.NewReader(string(body)))
	req.Host = "127.0.0.1:8080" // the API guard only accepts loopback authorities
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	NewHandler("").ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	e := decodeError(t, rec)
	if e.Code != "DEADLINE_EXCEEDED" || e.Stage != errtax.StageBudget || !e.Retryable {
		t.Fatalf("deadline error = %+v, want DEADLINE_EXCEEDED/budget/retryable", e)
	}
}

// The concurrency guard's 429 is the canonical transient error: its
// envelope must say retryable=true so a caller knows to back off
// instead of treating the rejection as a contract failure. The
// harness mirrors the guard's own overload test.
func TestTaxonomyHTTPServerBusyIsRetryable(t *testing.T) {
	sem := make(chan struct{}, maxConcurrentAnalyses)
	entered := make(chan struct{}, maxConcurrentAnalyses)
	release := make(chan struct{})
	handler := limitConcurrent(sem, func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusOK)
	})
	done := make(chan *httptest.ResponseRecorder, maxConcurrentAnalyses)
	for i := 0; i < maxConcurrentAnalyses; i++ {
		go func() {
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(http.MethodPost, "/api/analyze", nil))
			done <- rec
		}()
	}
	for i := 0; i < maxConcurrentAnalyses; i++ {
		<-entered // both slots are now held
	}
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/api/analyze", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("overload status = %d, want 429", rec.Code)
	}
	e := decodeError(t, rec)
	if e.Code != "SERVER_BUSY" || e.Stage != errtax.StageRequest || !e.Retryable {
		t.Fatalf("overload error = %+v, want SERVER_BUSY/request/retryable", e)
	}
	close(release)
	for i := 0; i < maxConcurrentAnalyses; i++ {
		<-done
	}
}

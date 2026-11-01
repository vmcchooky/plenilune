package demohttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"plenilune/internal/analysis"
)

func guardRequest(t *testing.T, handler http.Handler, method, path, host string, headers map[string]string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	req.Host = host
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func parseBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(parseRequest{Before: beforeACL, After: afterACL})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestP02ForeignHostRejectedOnNonAIRoutes(t *testing.T) {
	handler := NewHandler("")
	analyzeBody, err := json.Marshal(analysis.Request{Schema: analysis.RequestSchema, Before: analysis.Source{Text: beforeACL}, After: analysis.Source{Text: afterACL}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, host string
		body             []byte
	}{
		{"parse foreign host", "/api/parse", "evil.example", parseBody(t)},
		{"parse rebound host with port", "/api/parse", "evil.example:8080", parseBody(t)},
		{"parse non-loopback ip", "/api/parse", "192.0.2.1:8080", parseBody(t)},
		{"parse garbage port", "/api/parse", "127.0.0.1:abc", parseBody(t)},
		{"parse port out of range", "/api/parse", "127.0.0.1:99999", parseBody(t)},
		{"analyze foreign host", "/api/analyze", "evil.example", analyzeBody},
		{"health foreign host", "/api/health", "evil.example", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodPost
			if tc.body == nil {
				method = http.MethodGet
			}
			rec := guardRequest(t, handler, method, tc.path, tc.host, nil, tc.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
			}
			if got := decodeError(t, rec).Code; got != "LOCAL_ORIGIN_REQUIRED" {
				t.Fatalf("code = %q, want LOCAL_ORIGIN_REQUIRED", got)
			}
		})
	}
}

func TestP02CrossSiteOriginAndFetchMetadataRejected(t *testing.T) {
	handler := NewHandler("")
	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{"cross-site origin", map[string]string{"Origin": "https://evil.example"}},
		{"origin with userinfo", map[string]string{"Origin": "http://user@127.0.0.1:8080"}},
		{"non-http origin scheme", map[string]string{"Origin": "file://127.0.0.1"}},
		{"sec-fetch-site cross-site", map[string]string{"Sec-Fetch-Site": "cross-site"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := guardRequest(t, handler, http.MethodPost, "/api/parse", "127.0.0.1:8080", tc.headers, parseBody(t))
			if rec.Code != http.StatusForbidden || decodeError(t, rec).Code != "LOCAL_ORIGIN_REQUIRED" {
				t.Fatalf("status = %d, body = %s, want 403 LOCAL_ORIGIN_REQUIRED", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestP02LoopbackRequestsStillWork(t *testing.T) {
	handler := NewHandler("")
	for _, tc := range []struct {
		name    string
		host    string
		headers map[string]string
	}{
		{"no origin", "127.0.0.1:8080", nil},
		{"localhost host", "localhost:8080", nil},
		{"ipv6 loopback host", "[::1]:8080", nil},
		{"same origin", "127.0.0.1:8080", map[string]string{"Origin": "http://127.0.0.1:8080"}},
		{"dev ui on another loopback port", "127.0.0.1:8080", map[string]string{"Origin": "http://localhost:5173"}},
		{"sec-fetch same-origin", "127.0.0.1:8080", map[string]string{"Sec-Fetch-Site": "same-origin"}},
		{"sec-fetch same-site", "127.0.0.1:8080", map[string]string{"Sec-Fetch-Site": "same-site"}},
		{"sec-fetch none", "127.0.0.1:8080", map[string]string{"Sec-Fetch-Site": "none"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := guardRequest(t, handler, http.MethodPost, "/api/parse", tc.host, tc.headers, parseBody(t))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
	// Health without any browser headers (curl, probes) keeps working.
	rec := guardRequest(t, handler, http.MethodGet, "/api/health", "127.0.0.1:8080", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}
	// The static UI is outside the API guard's scope.
	rec = guardRequest(t, handler, http.MethodGet, "/", "anything.example", nil, nil)
	if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != "UI_NOT_BUILT" {
		t.Fatalf("static status = %d, body = %s, want the unguarded UI_NOT_BUILT 404", rec.Code, rec.Body.String())
	}
}

func TestP02HandlerForAddrRequiresListenPort(t *testing.T) {
	handler := NewHandlerForAddr("", "127.0.0.1:8080")
	for _, tc := range []struct {
		host string
		want int
	}{
		{"127.0.0.1:8080", http.StatusOK},
		{"localhost:8080", http.StatusOK},
		{"[::1]:8080", http.StatusOK},
		{"127.0.0.1:9999", http.StatusForbidden},
		{"127.0.0.1", http.StatusForbidden}, // no port means 80, not the listen port
		{"evil.example:8080", http.StatusForbidden},
	} {
		rec := guardRequest(t, handler, http.MethodGet, "/api/health", tc.host, nil, nil)
		if rec.Code != tc.want {
			t.Fatalf("host %q: status = %d, want %d", tc.host, rec.Code, tc.want)
		}
	}
}

func TestP02ConcurrencyCapRejectsBeyondLimit(t *testing.T) {
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
	if got := decodeError(t, rec).Code; got != "SERVER_BUSY" {
		t.Fatalf("overload code = %q, want SERVER_BUSY", got)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}

	close(release)
	for i := 0; i < maxConcurrentAnalyses; i++ {
		if rec := <-done; rec.Code != http.StatusOK {
			t.Fatalf("held request status = %d, want 200", rec.Code)
		}
	}
	// A slot is free again: the next request is admitted.
	rec = httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/api/analyze", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("post-release status = %d, want 200", rec.Code)
	}
}

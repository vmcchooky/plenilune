package demohttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
)

const beforeACL = `ip access-list extended MGMT
 10 remark synthetic documentation addresses only
 20 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22
 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443
 40 deny ip any any
!
`

const afterACL = `ip access-list extended MGMT
 10 remark synthetic documentation addresses only
 20 deny tcp host 192.0.2.10 any eq 22 log
 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22
 40 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443
 50 deny ip any any
!
`

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func do(t *testing.T, handler http.Handler, method, target, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(string(body)))
	req.Host = "127.0.0.1:8080" // the API guard only accepts loopback authorities
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) apiError {
	t.Helper()
	var envelope errorEnvelope
	decodeJSON(t, rec, &envelope)
	return envelope.Error
}

func TestParseSuccessReturnsNormalizedACLAndOriginalDigest(t *testing.T) {
	aclBytes := fixture(t, filepath.Join("acl", "management.acl"))
	body, err := json.Marshal(parseRequest{Before: string(aclBytes), After: string(aclBytes)})
	if err != nil {
		t.Fatal(err)
	}
	rec := do(t, NewHandler(""), http.MethodPost, "/api/parse", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var parsed parseResponse
	decodeJSON(t, rec, &parsed)
	if parsed.Before.ACL.Name != "MGMT" || parsed.After.ACL.Name != "MGMT" {
		t.Fatalf("expected MGMT on both sides, got %q / %q", parsed.Before.ACL.Name, parsed.After.ACL.Name)
	}
	if len(parsed.Before.ACL.Rules) != 3 || len(parsed.After.ACL.Rules) != 3 {
		t.Fatalf("rule counts = %d / %d, want 3 / 3", len(parsed.Before.ACL.Rules), len(parsed.After.ACL.Rules))
	}
	if got, want := parsed.Before.SHA256, evidence.ConfigDigest(aclBytes); got != want {
		t.Fatalf("before digest = %q, want %q", got, want)
	}
	if parsed.Before.ACL.Rules[0].Line != 3 || parsed.Before.ACL.Rules[0].Action != "deny" {
		t.Fatalf("first rule = %+v, want deny at source line 3", parsed.Before.ACL.Rules[0])
	}
}

func TestParseUnsupportedObjectGroupFailsClosed(t *testing.T) {
	after := strings.Replace(afterACL, "deny ip any any", "deny ip object-group ADMIN any", 1)
	body, err := json.Marshal(parseRequest{Before: beforeACL, After: after})
	if err != nil {
		t.Fatal(err)
	}
	rec := do(t, NewHandler(""), http.MethodPost, "/api/parse", "application/json", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	apiErr := decodeError(t, rec)
	if apiErr.Code != "PARSE_INVALID_IP" || apiErr.Side != "after" || apiErr.Line != 6 {
		t.Fatalf("error = %+v, want PARSE_INVALID_IP after line 6", apiErr)
	}
	if strings.Contains(rec.Body.String(), "object-group ADMIN") {
		t.Fatalf("error response leaked the rejected source line: %s", rec.Body.String())
	}
}

func TestParseRejectsAmbiguousJSON(t *testing.T) {
	handler := NewHandler("")
	valid := `{"before":"x","after":"y"}`
	cases := []struct {
		name string
		body string
		want string
	}{
		{"malformed", `{"before":`, "JSON_INVALID"},
		{"unknown field", `{"before":"x","after":"y","extra":true}`, "JSON_INVALID"},
		{"duplicate key", `{"before":"x","before":"z","after":"y"}`, "JSON_INVALID"},
		{"wrong case", `{"Before":"x","after":"y"}`, "JSON_INVALID"},
		{"trailing value", valid + ` {}`, "JSON_INVALID"},
		{"null side", `{"before":null,"after":"y"}`, "PARSE_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, handler, http.MethodPost, "/api/parse", "application/json", []byte(tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if got := decodeError(t, rec).Code; got != tc.want {
				t.Fatalf("code = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestContentTypeGate(t *testing.T) {
	handler := NewHandler("")
	validBody := []byte(`{"before":"x","after":"y"}`)
	for _, tc := range []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{"json", "application/json", http.StatusBadRequest}, // body is JSON but not an ACL
		{"json with charset", "application/json; charset=utf-8", http.StatusBadRequest},
		{"json with valid parameters", "application/json; charset=utf-8; profile=demo", http.StatusBadRequest},
		{"jsonp", "application/jsonp", http.StatusUnsupportedMediaType},
		{"json patch suffix", "application/json-patch+json", http.StatusUnsupportedMediaType},
		{"malformed parameter", "application/json; charset=", http.StatusUnsupportedMediaType},
		{"text", "text/plain", http.StatusUnsupportedMediaType},
		{"missing", "", http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, handler, http.MethodPost, "/api/parse", tc.contentType, validBody)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if rec.Code == http.StatusUnsupportedMediaType && decodeError(t, rec).Code != "CONTENT_TYPE" {
				t.Fatalf("unexpected error: %s", rec.Body.String())
			}
		})
	}
}

func TestNormalizeSuccessMatchesCoreDigest(t *testing.T) {
	obligations := fixture(t, "obligations.json")
	rec := do(t, NewHandler(""), http.MethodPost, "/api/obligations/normalize", "application/json", obligations)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var normalized normalizeResponse
	decodeJSON(t, rec, &normalized)
	var input intent.Set
	if err := json.Unmarshal(obligations, &input); err != nil {
		t.Fatal(err)
	}
	wantSet, wantDigest, err := intent.Digest(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized.Set, wantSet) {
		t.Fatalf("normalized set = %+v, want %+v", normalized.Set, wantSet)
	}
	if normalized.SHA256 != wantDigest || len(normalized.SHA256) != 64 {
		t.Fatalf("digest = %q, want %q", normalized.SHA256, wantDigest)
	}
	if got := normalized.Set.Obligations[0].Match.DstPorts; !reflect.DeepEqual(got, []intent.Interval{{Lo: 22, Hi: 22}}) {
		t.Fatalf("dst ports = %+v", got)
	}
}

func TestNormalizeRejectsInvalidAndOversizedPayloads(t *testing.T) {
	handler := NewHandler("")
	valid := fixture(t, "obligations.json")
	var set intent.Set
	if err := json.Unmarshal(valid, &set); err != nil {
		t.Fatal(err)
	}
	empty, _ := json.Marshal(intent.Set{Schema: set.Schema, Scope: set.Scope, Obligations: []intent.Obligation{}})
	padded := bytesWithSpaces(valid, intent.MaxJSONBytes+1)
	for _, tc := range []struct {
		name string
		body []byte
		want string
	}{
		{"empty set", empty, "OBLIGATIONS_INVALID"},
		{"duplicate key", []byte(`{"schema":"plenilune.obligations/v1","schema":"x","scope":"y","obligations":[]}`), "JSON_INVALID"},
		{"oversized", padded, "JSON_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, handler, http.MethodPost, "/api/obligations/normalize", "application/json", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if got := decodeError(t, rec).Code; got != tc.want {
				t.Fatalf("code = %q, want %q (%s)", got, tc.want, rec.Body.String())
			}
		})
	}
}

func bytesWithSpaces(base []byte, minLen int) []byte {
	out := make([]byte, 0, len(base)+minLen)
	out = append(out, base...)
	for len(out) < minLen {
		out = append(out, ' ')
	}
	return out
}

func TestTraceSealsRuntimePointDecisions(t *testing.T) {
	var packet ciscoacl.Packet
	if err := json.Unmarshal(fixture(t, "packet.json"), &packet); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler("")
	for _, tc := range []struct {
		name       string
		config     []byte
		wantAction string
		wantLine   int
	}{
		{"before", []byte(beforeACL), "permit", 3},
		{"after", []byte(afterACL), "deny", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(traceRequest{Config: string(tc.config), Packet: packet})
			if err != nil {
				t.Fatal(err)
			}
			rec := do(t, handler, http.MethodPost, "/api/trace", "application/json", body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var trace evidence.Trace
			decodeJSON(t, rec, &trace)
			if err := evidence.CheckIntegrity(trace); err != nil {
				t.Fatalf("trace integrity: %v", err)
			}
			if trace.Decision.Action != tc.wantAction || trace.Decision.Line == nil || *trace.Decision.Line != tc.wantLine {
				t.Fatalf("decision = %+v, want %s at line %d", trace.Decision, tc.wantAction, tc.wantLine)
			}
			if trace.Decision.RuleOrdinal == nil || *trace.Decision.RuleOrdinal != 0 || trace.Decision.Implicit {
				t.Fatalf("decision source = %+v, want explicit ordinal 0", trace.Decision)
			}
			if got, want := trace.ConfigSHA256, evidence.ConfigDigest(tc.config); got != want {
				t.Fatalf("config digest = %q, want %q", got, want)
			}
			if len(trace.SHA256) != 64 {
				t.Fatalf("trace digest length = %d", len(trace.SHA256))
			}
		})
	}
}

func TestTraceRejectsInvalidPacketConfigAndOversizedBody(t *testing.T) {
	packet := json.RawMessage(fixture(t, "packet.json"))
	handler := NewHandler("")
	valid := []byte(`{"config":` + quoteJSON(beforeACL) + `,"packet":` + string(packet) + `}`)
	if rec := do(t, handler, http.MethodPost, "/api/trace", "application/json", valid); rec.Code != http.StatusOK {
		t.Fatalf("baseline trace status = %d, body = %s", rec.Code, rec.Body.String())
	}

	nonTCP := []byte(`{"config":` + quoteJSON(beforeACL) + `,"packet":{"protocol":1,"src_ip":"192.0.2.10","dst_ip":"198.51.100.10","src_port":40000,"dst_port":22}}`)
	rec := do(t, handler, http.MethodPost, "/api/trace", "application/json", nonTCP)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "ports must be zero") {
		t.Fatalf("non-TCP ports status = %d, body = %s", rec.Code, rec.Body.String())
	}

	invalidConfig := []byte(`{"config":"not an acl","packet":` + string(packet) + `}`)
	rec = do(t, handler, http.MethodPost, "/api/trace", "application/json", invalidConfig)
	if rec.Code != http.StatusBadRequest || decodeError(t, rec).Side != "config" {
		t.Fatalf("invalid config status = %d, body = %s", rec.Code, rec.Body.String())
	}

	oversized := []byte(`{"config":` + quoteJSON(strings.Repeat("x", maxTraceBody+10)) + `,"packet":` + string(packet) + `}`)
	rec = do(t, handler, http.MethodPost, "/api/trace", "application/json", oversized)
	if rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != "JSON_INVALID" {
		t.Fatalf("oversized trace status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestParseBodyLimit(t *testing.T) {
	body := []byte(`{"before":"` + strings.Repeat("x", maxParseBody+10) + `","after":"x"}`)
	rec := do(t, NewHandler(""), http.MethodPost, "/api/parse", "application/json", body)
	if rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != "JSON_INVALID" {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHealthPublishesRuntimeAndFixtureBoundaries(t *testing.T) {
	rec := do(t, NewHandler(""), http.MethodGet, "/api/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var health struct {
		Service string            `json:"service"`
		Runtime map[string]string `json:"runtime"`
	}
	decodeJSON(t, rec, &health)
	if health.Service != "plenilune-demo" {
		t.Fatalf("service = %q", health.Service)
	}
	want := map[string]string{
		"parse": "available", "trace": "available", "obligations": "available",
		"comparison": "available", "repair": "available", "ai": "available_opt_in",
		"analysis": "available",
	}
	if !reflect.DeepEqual(health.Runtime, want) {
		t.Fatalf("runtime = %+v, want %+v", health.Runtime, want)
	}
}

func TestSecurityHeadersAndStaticNotBuilt(t *testing.T) {
	handler := NewHandler("")
	rec := do(t, handler, http.MethodGet, "/api/health", "", nil)
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff = %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("referrer policy = %q", got)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("csp = %q", csp)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}

	rec = do(t, handler, http.MethodGet, "/", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("static status = %d, want 404", rec.Code)
	}
	if got := decodeError(t, rec).Code; got != "UI_NOT_BUILT" {
		t.Fatalf("static error = %q", got)
	}
}

func TestMethodsOutsideAPIAreRejected(t *testing.T) {
	rec := do(t, NewHandler(""), http.MethodPost, "/api/health", "application/json", []byte(`{}`))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

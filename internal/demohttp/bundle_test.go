package demohttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"plenilune/internal/bundle"
)

func postBundleExport(t *testing.T, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler("")
	req := httptest.NewRequest(http.MethodPost, "/api/bundle/export", bytes.NewReader(body))
	req.Host = "127.0.0.1:8080" // the API guard only accepts loopback authorities
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// Acceptance 4: the server export is byte-identical to the shared
// builder's manifest for the same case — the exact bytes the CLI
// exporter writes for it.
func TestBundleExportHTTPMatchesSharedBuilder(t *testing.T) {
	repair := "{\"schema\":\"plenilune.repair/v1\"}\n"
	report := "# Report\n"
	files := map[string][]byte{
		bundle.RoleBefore:     []byte("ip access-list extended DEMO\n permit tcp any any eq 22\n"),
		bundle.RoleAfter:      []byte("ip access-list extended DEMO\n deny ip any any\n"),
		bundle.RoleComparison: []byte("{\"schema\":\"plenilune.evidence/v2\"}\n"),
		bundle.RoleRepair:     []byte(repair),
		bundle.RoleReport:     []byte(report),
	}
	body, err := json.Marshal(bundleExportRequest{
		Before:     string(files[bundle.RoleBefore]),
		After:      string(files[bundle.RoleAfter]),
		Comparison: string(files[bundle.RoleComparison]),
		Repair:     &repair,
		Report:     &report,
	})
	if err != nil {
		t.Fatal(err)
	}
	w := postBundleExport(t, body)
	if w.Code != http.StatusOK {
		t.Fatalf("bundle export: %d %s", w.Code, w.Body.String())
	}
	want, err := bundle.Build(files)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := bundle.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w.Body.Bytes(), wantBytes) {
		t.Fatalf("server manifest differs from shared builder:\n%s\n---\n%s", w.Body.Bytes(), wantBytes)
	}
}

// A case missing a required part is a typed BUNDLE_INVALID at the
// validate stage, naming the role — the same envelope discipline as
// every other endpoint (P2-3).
func TestBundleExportHTTPMissingComparison(t *testing.T) {
	body, err := json.Marshal(bundleExportRequest{Before: "b", After: "a", Comparison: ""})
	if err != nil {
		t.Fatal(err)
	}
	w := postBundleExport(t, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing comparison: %d %s", w.Code, w.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "BUNDLE_INVALID" || env.Error.Stage != "validate" || env.Error.Field != bundle.RoleComparison || env.Error.Retryable {
		t.Fatalf("envelope = %+v", env.Error)
	}
}

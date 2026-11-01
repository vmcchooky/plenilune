package demohttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/workflow"
	"testing"
)

func TestExactComparisonHTTPAndReplay(t *testing.T) {
	before := "ip access-list extended DEMO\n permit tcp any any eq 22\n"
	after := "ip access-list extended DEMO\n deny ip any any\n"
	conditions := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.0/24", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
	body, err := json.Marshal(compareRequest{Before: before, After: after, Conditions: &conditions})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler("")
	req := httptest.NewRequest(http.MethodPost, "/api/compare", bytes.NewReader(body))
	req.Host = "127.0.0.1:8080" // the API guard only accepts loopback authorities
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("compare: %d %s", w.Code, w.Body.String())
	}
	var c evidence.Comparison
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.Report.Verdict != "DIFFERENT" || c.Report.Obligations[0].Status != "VIOLATED" || c.Report.Obligations[0].Witness == nil {
		t.Fatal("endpoint did not return exact engine proof")
	}
	body, err = json.Marshal(struct {
		Before   string              `json:"before"`
		After    string              `json:"after"`
		Evidence evidence.Comparison `json:"evidence"`
	}{before, after, c})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/compare/replay", bytes.NewReader(body))
	req.Host = "127.0.0.1:8080" // the API guard only accepts loopback authorities
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var result workflow.ReplayResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Status != "COMPARISON_REPLAYED" {
		t.Fatalf("replay: %d %s", w.Code, w.Body.String())
	}
}

func TestCompareServedFromSessionCache(t *testing.T) {
	// A name no other test uses: the package cache is process-wide.
	before := "ip access-list extended CACHEP15\n permit tcp any any eq 22\n"
	after := "ip access-list extended CACHEP15\n deny ip any any\n"
	body, err := json.Marshal(compareRequest{Before: before, After: after})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler("")
	post := func() string {
		req := httptest.NewRequest(http.MethodPost, "/api/compare", bytes.NewReader(body))
		req.Host = "127.0.0.1:8080"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("compare: %d %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	hitsBefore, _, _ := compareCache.Stats()
	first := post()
	second := post()
	if first != second {
		t.Fatal("cached compare response differs from the computed one")
	}
	hitsAfter, _, _ := compareCache.Stats()
	if hitsAfter != hitsBefore+1 {
		t.Fatalf("second identical compare did not hit the cache: hits %d -> %d", hitsBefore, hitsAfter)
	}
}

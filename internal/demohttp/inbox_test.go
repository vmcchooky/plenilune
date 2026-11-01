package demohttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"plenilune/internal/analysis"
	"plenilune/internal/inbox"
	"plenilune/internal/intent"
)

// The trigger store is package-level like the compare cache, so each
// test swaps in a fresh store and uses object names no other test
// touches; other tests' analyses then cannot pollute these inboxes.
func freshInbox(t *testing.T) {
	t.Helper()
	old := verdictInbox
	verdictInbox = inbox.NewStore()
	t.Cleanup(func() { verdictInbox = old })
}

func doReq(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	req.Host = "127.0.0.1:8080" // the API guard only accepts loopback authorities
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewHandler("").ServeHTTP(rec, req)
	return rec
}

func getInbox(t *testing.T) inboxDocument {
	t.Helper()
	rec := doReq(t, http.MethodGet, "/api/inbox", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/inbox: %d %s", rec.Code, rec.Body.String())
	}
	var doc inboxDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func putWatches(t *testing.T, watches []inbox.Watch) *httptest.ResponseRecorder {
	t.Helper()
	return doReq(t, http.MethodPut, "/api/watches", watchesDocument{Watches: watches})
}

func analyzePair(t *testing.T, before, after string) analysis.Result {
	t.Helper()
	rec := postJSON(t, "/api/analyze", analysis.Request{
		Schema: analysis.RequestSchema,
		Before: analysis.Source{Text: before},
		After:  analysis.Source{Text: after},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze: %d %s", rec.Code, rec.Body.String())
	}
	var result analysis.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

const (
	pinCfgBase = "hostname PIN28\n!\nip access-list extended PIN28A\n 10 permit tcp 192.0.2.0 0.0.0.255 any eq 22\n 20 deny ip any any\n!\nend\n"
	// Same behavior, different text (a remark): the engine runs and
	// returns EQUIVALENT.
	pinCfgRemark = "hostname PIN28\n!\nip access-list extended PIN28A\n 10 permit tcp 192.0.2.0 0.0.0.255 any eq 22\n 15 remark behavior unchanged\n 20 deny ip any any\n!\nend\n"
	// Port 22 becomes 443: DIFFERENT.
	pinCfgChanged = "hostname PIN28\n!\nip access-list extended PIN28A\n 10 permit tcp 192.0.2.0 0.0.0.255 any eq 443\n 20 deny ip any any\n!\nend\n"
)

// Acceptance 1 + 2 over HTTP: a watched object's verdict change
// across two completed analyses produces exactly one inbox entry with
// the before/after verdicts and the second artifact's content digest;
// the baseline run and a repeat of the same verdict produce none.
func TestInboxVerdictChangeOverHTTP(t *testing.T) {
	freshInbox(t)
	if rec := putWatches(t, []inbox.Watch{{Name: "PIN28A", Conditions: []string{inbox.ConditionVerdictChanged}}}); rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/watches: %d %s", rec.Code, rec.Body.String())
	}
	analyzePair(t, pinCfgBase, pinCfgRemark) // baseline: EQUIVALENT
	if doc := getInbox(t); len(doc.Entries) != 0 || doc.Unread != 0 {
		t.Fatalf("baseline analysis filled the inbox: %+v", doc)
	}
	second := analyzePair(t, pinCfgBase, pinCfgChanged) // DIFFERENT
	doc := getInbox(t)
	if len(doc.Entries) != 1 || doc.Unread != 1 {
		t.Fatalf("inbox after verdict change: %+v", doc)
	}
	e := doc.Entries[0]
	if e.Name != "PIN28A" || e.Domain != inbox.DomainACL || e.Condition != inbox.ConditionVerdictChanged {
		t.Fatalf("entry identity %+v", e)
	}
	if e.PreviousVerdict != "EQUIVALENT" || e.Verdict != "DIFFERENT" {
		t.Fatalf("entry verdicts %+v", e)
	}
	if e.ArtifactKind != inbox.ArtifactAnalysis || e.ContentSHA256 != second.ContentSHA256 || e.ContentSHA256 == "" {
		t.Fatalf("entry artifact link %+v vs analysis content %q", e, second.ContentSHA256)
	}
	// Same verdict again: no second entry.
	analyzePair(t, pinCfgRemark, pinCfgChanged)
	if doc := getInbox(t); len(doc.Entries) != 1 {
		t.Fatalf("repeat verdict added entries: %+v", doc.Entries)
	}
}

// Acceptance 3 over HTTP: an obligation failure fires through the
// compare flow even though the comparison verdict itself stays
// EQUIVALENT on both runs.
func TestInboxObligationFailureOverHTTP(t *testing.T) {
	freshInbox(t)
	if rec := putWatches(t, []inbox.Watch{{Name: "PIN28B", Conditions: []string{inbox.ConditionObligationFailed}}}); rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/watches: %d %s", rec.Code, rec.Body.String())
	}
	acl := "ip access-list extended PIN28B\n permit tcp any any eq 22\n deny ip any any\n"
	region := intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.0/24", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}
	compare := func(kind string) {
		conditions := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: kind, Match: region}}}
		rec := postJSON(t, "/api/compare", compareRequest{Before: acl, After: acl, Conditions: &conditions})
		if rec.Code != http.StatusOK {
			t.Fatalf("compare %s: %d %s", kind, rec.Code, rec.Body.String())
		}
	}
	compare("must_permit") // OB-1 holds; verdict EQUIVALENT
	if doc := getInbox(t); len(doc.Entries) != 0 {
		t.Fatalf("holding obligation filled the inbox: %+v", doc.Entries)
	}
	compare("must_deny") // OB-1 violated; verdict still EQUIVALENT
	doc := getInbox(t)
	if len(doc.Entries) != 1 {
		t.Fatalf("inbox after obligation failure: %+v", doc.Entries)
	}
	e := doc.Entries[0]
	if e.Condition != inbox.ConditionObligationFailed || e.Verdict != "EQUIVALENT" || e.ArtifactKind != inbox.ArtifactComparison {
		t.Fatalf("entry %+v", e)
	}
	if len(e.ViolatedObligations) != 1 || e.ViolatedObligations[0] != "OB-1" {
		t.Fatalf("violated obligations %+v", e)
	}
}

// Acceptance 4 over HTTP: watches round-trip through the API, the
// validation failures are typed taxonomy errors, and the read flow
// persists across calls.
func TestWatchesAndInboxReadOverHTTP(t *testing.T) {
	freshInbox(t)
	// Invalid watch: typed WATCHES_INVALID in the validate stage.
	rec := putWatches(t, []inbox.Watch{{Name: "PIN28C", Conditions: []string{"exploded"}}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"WATCHES_INVALID"`) || !strings.Contains(rec.Body.String(), `"stage":"validate"`) || !strings.Contains(rec.Body.String(), `"field":"watches"`) {
		t.Fatalf("invalid watch: %d %s", rec.Code, rec.Body.String())
	}
	// Valid set round-trips, normalized (domain defaulted).
	if rec := putWatches(t, []inbox.Watch{{Name: "PIN28C", Conditions: []string{inbox.ConditionAnyDifferent}}}); rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/watches: %d %s", rec.Code, rec.Body.String())
	}
	rec = doReq(t, http.MethodGet, "/api/watches", nil)
	var doc watchesDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil || len(doc.Watches) != 1 || doc.Watches[0].Domain != inbox.DomainACL {
		t.Fatalf("GET /api/watches: %d %s", rec.Code, rec.Body.String())
	}
	// Unknown entry id: typed INBOX_INVALID.
	rec = doReq(t, http.MethodPost, "/api/inbox/read", inboxReadRequest{ID: 4242})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"INBOX_INVALID"`) {
		t.Fatalf("unknown read id: %d %s", rec.Code, rec.Body.String())
	}
	// Fire one entry, then read it and confirm the state persists.
	acl := "ip access-list extended PIN28C\n permit tcp any any eq 22\n"
	changed := "ip access-list extended PIN28C\n deny ip any any\n"
	if rec := postJSON(t, "/api/compare", compareRequest{Before: acl, After: changed}); rec.Code != http.StatusOK {
		t.Fatalf("compare: %d %s", rec.Code, rec.Body.String())
	}
	doc2 := getInbox(t)
	if len(doc2.Entries) != 1 || doc2.Unread != 1 {
		t.Fatalf("inbox %+v", doc2)
	}
	id := doc2.Entries[0].ID
	rec = doReq(t, http.MethodPost, "/api/inbox/read", inboxReadRequest{ID: id})
	if rec.Code != http.StatusOK {
		t.Fatalf("mark read: %d %s", rec.Code, rec.Body.String())
	}
	doc3 := getInbox(t)
	if len(doc3.Entries) != 1 || !doc3.Entries[0].Read || doc3.Unread != 0 {
		t.Fatalf("read state did not persist: %+v", doc3)
	}
}

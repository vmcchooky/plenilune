package demohttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/patch"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
)

// P2-7-fix-2: a partial options object on /api/compare resolves field
// by field (zero = unset) instead of replacing the defaults wholesale
// and failing on the zeroed limits.
func TestComparePartialOptionsTakeDefaults(t *testing.T) {
	post := func(name string, opts *verify.Options) evidence.Comparison {
		before := "ip access-list extended " + name + "\n permit tcp any any eq 22\n"
		after := "ip access-list extended " + name + "\n deny ip any any\n"
		body, err := json.Marshal(compareRequest{Before: before, After: after, Options: opts})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/compare", bytes.NewReader(body))
		req.Host = "127.0.0.1:8080"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler("").ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("compare %s: %d %s", name, w.Code, w.Body.String())
		}
		var c evidence.Comparison
		if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	partial := post("OPTPART", &verify.Options{MaxWitnesses: 4})
	if partial.Report.Options.MaxWitnesses != 4 || partial.Report.Options.Limits != packetset.Defaults() {
		t.Fatalf("partial options not resolved per field: %+v", partial.Report.Options)
	}
	full := verify.DefaultOptions()
	full.MaxWitnesses = 4
	explicit := post("OPTEXPL", &full)
	if explicit.Report.Options != partial.Report.Options || explicit.Report.Verdict != partial.Report.Verdict {
		t.Fatalf("partial and explicit-full disagree: %+v vs %+v", partial.Report.Options, explicit.Report.Options)
	}
}

// The same rule on /api/repair/verify: partial options resolve instead
// of failing as REPAIR_INPUT_INVALID on the zeroed limits.
func TestRepairPartialOptionsTakeDefaults(t *testing.T) {
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
		Before  string          `json:"before"`
		After   string          `json:"after"`
		Policy  repair.Policy   `json:"policy"`
		Patch   patch.Request   `json:"patch"`
		Options *verify.Options `json:"options"`
	}{b, a, p, req, &verify.Options{MaxWitnesses: 2}})
	if err != nil {
		t.Fatal(err)
	}
	w := do(t, NewHandler(""), http.MethodPost, "/api/repair/verify", "application/json", body)
	var proof repair.Result
	if err := json.Unmarshal(w.Body.Bytes(), &proof); err != nil || w.Code != 200 || proof.Status != "VERIFIED" {
		t.Fatalf("repair: %d %s", w.Code, w.Body.String())
	}
	if proof.Options.MaxWitnesses != 2 || proof.Options.Limits != packetset.Defaults() {
		t.Fatalf("partial options not resolved per field: %+v", proof.Options)
	}
}

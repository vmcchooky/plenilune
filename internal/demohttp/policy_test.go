package demohttp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"plenilune/internal/intent"
	"plenilune/internal/repair"
)

func previewPolicyFixture() repair.Policy {
	r := intent.Region{
		Protocol: "006", SrcCIDR: "192.0.2.7/24", DstCIDR: "198.51.100.10/32",
		ExcludeSrc: []string{"192.0.2.200/32", "192.0.2.199/32", "192.0.2.200/32"},
		DstPorts:   []intent.Interval{{Lo: 23, Hi: 23}, {Lo: 22, Hi: 22}},
	}
	return repair.Policy{
		Schema: repair.PolicySchema, Scope: intent.Scope,
		Conditions: intent.Set{Schema: intent.Schema, Scope: intent.Scope,
			Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: r}}},
		AllowedChanges: []intent.Region{r, r},
	}
}

func postPolicyPreview(t *testing.T, policy repair.Policy) policyNormalizeResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{"policy": policy})
	if err != nil {
		t.Fatal(err)
	}
	w := do(t, NewHandler(""), http.MethodPost, "/api/policy/normalize", "application/json", body)
	if w.Code != http.StatusOK {
		t.Fatalf("preview status = %d: %s", w.Code, w.Body.String())
	}
	var out policyNormalizeResponse
	decodeJSON(t, w, &out)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("preview must not be cached")
	}
	return out
}

func TestPolicyNormalizeCanonicalFullPreview(t *testing.T) {
	out := postPolicyPreview(t, previewPolicyFixture())
	condition := out.Policy.Conditions.Obligations[0].Match
	if condition.Protocol != "tcp" || condition.SrcCIDR != "192.0.2.0/24" {
		t.Fatalf("not canonical: %+v", condition)
	}
	if !reflect.DeepEqual(condition.ExcludeSrc, []string{"192.0.2.199/32", "192.0.2.200/32"}) ||
		!reflect.DeepEqual(condition.DstPorts, []intent.Interval{{Lo: 22, Hi: 23}}) {
		t.Fatalf("excludes or intervals are not canonical: %+v", condition)
	}
	if len(out.Policy.AllowedChanges) != 1 || !reflect.DeepEqual(condition, out.Policy.AllowedChanges[0]) {
		t.Fatalf("allowed scope not normalized/deduplicated with conditions: %+v", out.Policy.AllowedChanges)
	}
	if out.Policy.OperatorAccepted {
		t.Fatal("normalization must not approve the preview")
	}
	assertDigest := func(value any, got string) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got != hex.EncodeToString(sum[:]) {
			t.Fatalf("digest does not bind returned canonical value: %s", got)
		}
	}
	assertDigest(out.Policy, out.PolicySHA256)
	assertDigest(out.Policy.Conditions, out.ObligationsDigest)
	if again := postPolicyPreview(t, out.Policy); !reflect.DeepEqual(out, again) {
		t.Fatal("canonical preview is not idempotent")
	}
}

func TestPolicyNormalizeAcceptanceAndAllowedScopeHaveDistinctIdentities(t *testing.T) {
	p := previewPolicyFixture()
	p.AllowedChanges = nil
	undeclared := postPolicyPreview(t, p)
	p.AllowedChanges = []intent.Region{}
	empty := postPolicyPreview(t, p)
	if undeclared.Policy.AllowedChanges != nil || empty.Policy.AllowedChanges == nil || len(empty.Policy.AllowedChanges) != 0 {
		t.Fatal("nil scope and explicit empty scope were conflated")
	}
	if undeclared.PolicySHA256 == empty.PolicySHA256 || undeclared.ObligationsDigest != empty.ObligationsDigest {
		t.Fatal("allowed scope must change full identity, not condition identity")
	}
	p.OperatorAccepted = true
	accepted := postPolicyPreview(t, p)
	if !accepted.Policy.OperatorAccepted || accepted.PolicySHA256 == empty.PolicySHA256 || accepted.ObligationsDigest != empty.ObligationsDigest {
		t.Fatal("acceptance must be preserved and bind only full-policy identity")
	}
	accepted.Policy.OperatorAccepted = false
	if !reflect.DeepEqual(accepted.Policy, empty.Policy) {
		t.Fatal("acceptance normalization unexpectedly changed definitions/scope")
	}
}

func TestPolicyNormalizeRejectsInvalidAndAmbiguousInput(t *testing.T) {
	body, err := json.Marshal(map[string]any{"policy": previewPolicyFixture()})
	if err != nil {
		t.Fatal(err)
	}
	valid := string(body)
	for _, tc := range []struct {
		name, body, code string
	}{
		{"missing", `{}`, "POLICY_INPUT_INVALID"},
		{"null", `{"policy":null}`, "POLICY_INPUT_INVALID"},
		{"schema", strings.Replace(valid, repair.PolicySchema, "invalid-secret-policy", 1), "POLICY_INPUT_INVALID"},
		{"invalid region", strings.Replace(valid, "192.0.2.7/24", "not-a-cidr-secret", 1), "POLICY_INPUT_INVALID"},
		{"unknown wrapper", `{"policy":null,"extra":1}`, "JSON_INVALID"},
		{"duplicate escaped key", `{"policy":null,"\u0070olicy":null}`, "JSON_INVALID"},
		{"wrong field spelling", strings.Replace(valid, `"scope":`, `"Scope":`, 1), "JSON_INVALID"},
		{"nested unknown field", strings.Replace(valid, `"operator_accepted":false`, `"operator_accepted":false,"secret":1`, 1), "JSON_INVALID"},
		{"trailing value", valid + ` {}`, "JSON_INVALID"},
		{"oversized", strings.Repeat(" ", maxParseBody+1), "JSON_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, NewHandler(""), http.MethodPost, "/api/policy/normalize", "application/json", []byte(tc.body))
			if w.Code != http.StatusBadRequest || decodeError(t, w).Code != tc.code {
				t.Fatalf("unexpected error %d: %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("rejected policy content leaked into error")
			}
		})
	}
}

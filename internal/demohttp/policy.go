package demohttp

import (
	"net/http"
	"plenilune/internal/intent"
	"plenilune/internal/repair"
)

type policyNormalizeResponse struct {
	Policy            repair.Policy `json:"policy"`
	PolicySHA256      string        `json:"policy_sha256"`
	ObligationsDigest string        `json:"obligations_digest"`
}

// Normalization describes precisely what a caller is about to accept. It does
// not approve a policy or identify the caller; the acceptance bit is preserved.
func handlePolicyNormalize(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Policy *repair.Policy `json:"policy"`
	}
	if !decodeRequest(w, r, maxParseBody, &in) {
		return
	}
	invalid := func() {
		writeError(w, http.StatusBadRequest, apiError{Code: "POLICY_INPUT_INVALID", Message: "invalid policy schema, conditions or allowed change regions"})
	}
	if in.Policy == nil {
		invalid()
		return
	}
	policy, err := repair.NormalizePolicy(*in.Policy)
	if err != nil {
		invalid()
		return
	}
	policyDigest, err := repair.PolicyDigest(policy)
	if err != nil {
		invalid()
		return
	}
	_, conditionsDigest, err := intent.Digest(policy.Conditions)
	if err != nil {
		invalid()
		return
	}
	writeJSON(w, http.StatusOK, policyNormalizeResponse{
		Policy: policy, PolicySHA256: policyDigest, ObligationsDigest: conditionsDigest,
	})
}

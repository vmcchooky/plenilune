package demohttp

import (
	"net/http"
	"plenilune/internal/patch"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
)

func handleRepair(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Before  string          `json:"before"`
		After   string          `json:"after"`
		Policy  *repair.Policy  `json:"policy"`
		Patch   patch.Request   `json:"patch"`
		Options *verify.Options `json:"options,omitempty"`
	}
	if !decodeRequest(w, r, maxParseBody, &in) {
		return
	}
	opts := verify.DefaultOptions()
	if in.Options != nil {
		opts = *in.Options
	}
	result, err := repair.Verify(r.Context(), []byte(in.Before), []byte(in.After), in.Policy, in.Patch, opts)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "REPAIR_INPUT_INVALID", Message: "invalid repair inputs, policy or options"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleRepairReplay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Before   string        `json:"before"`
		After    string        `json:"after"`
		Evidence repair.Result `json:"evidence"`
	}
	if !decodeRequest(w, r, maxParseBody+repair.MaxJSONBytes, &in) {
		return
	}
	result, err := repair.Replay(r.Context(), []byte(in.Before), []byte(in.After), in.Evidence)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "EVIDENCE_MISMATCH", Message: "repair replay failed"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

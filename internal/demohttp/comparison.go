package demohttp

import (
	"errors"
	"net/http"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/verify"
	"plenilune/internal/workflow"
)

type compareRequest struct {
	Before     string          `json:"before"`
	After      string          `json:"after"`
	Conditions *intent.Set     `json:"conditions"`
	Options    *verify.Options `json:"options,omitempty"`
}

// compareCache memoizes sealed comparisons for the life of the demo
// server process (P1-5): reopening an object the UI already compared
// (useReview.openObject) is served from the cache instead of recomputed.
// It is an internal optimization only — a hit is the exact sealed
// artifact a fresh computation produced for the same inputs.
var compareCache = workflow.NewCompareCache(128)

func handleCompare(w http.ResponseWriter, r *http.Request) {
	var in compareRequest
	if !decodeRequest(w, r, maxParseBody, &in) {
		return
	}
	opts := verify.DefaultOptions()
	if in.Options != nil {
		opts = *in.Options
	}
	c, err := compareCache.Compare(r.Context(), []byte(in.Before), []byte(in.After), in.Conditions, opts, evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"})
	if err != nil {
		comparisonError(w, err)
		return
	}
	// EQUIVALENT, DIFFERENT and INCONCLUSIVE are domain outcomes, all HTTP 200.
	writeJSON(w, http.StatusOK, c)
}

func handleComparisonReplay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Before   string              `json:"before"`
		After    string              `json:"after"`
		Evidence evidence.Comparison `json:"evidence"`
	}
	if !decodeRequest(w, r, maxParseBody+evidence.MaxComparisonBytes, &in) {
		return
	}
	result, err := workflow.Replay(r.Context(), []byte(in.Before), []byte(in.After), in.Evidence)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "EVIDENCE_MISMATCH", Message: "comparison replay failed"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func comparisonError(w http.ResponseWriter, err error) {
	var domain *workflow.Error
	if errors.As(err, &domain) {
		writeError(w, http.StatusBadRequest, apiError{Code: domain.Code, Message: domain.Message, Side: domain.Side, Line: domain.Line})
		return
	}
	writeError(w, http.StatusBadRequest, apiError{Code: "COMPARISON_INVALID", Message: "invalid comparison input, conditions or options"})
}

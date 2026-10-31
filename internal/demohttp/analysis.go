package demohttp

import (
	"errors"
	"net/http"

	"plenilune/internal/analysis"
)

const (
	maxAnalysisBody = 2*analysis.MaxSourceBytes + 2<<20
	maxReplayBody   = maxAnalysisBody + analysis.MaxResultBytes
)

func analysisError(w http.ResponseWriter, err error) {
	var e *analysis.Error
	if errors.As(err, &e) {
		writeError(w, http.StatusBadRequest, apiError{Code: e.Code, Message: e.Message})
		return
	}
	writeError(w, http.StatusBadRequest, apiError{Code: "ANALYSIS_FAILED", Message: "analysis failed"})
}

// handleAnalyze reviews two configuration versions; object statuses are
// domain outcomes in HTTP 200, invalid requests are HTTP 400.
func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	var in analysis.Request
	if !decodeRequest(w, r, maxAnalysisBody, &in) {
		return
	}
	result, err := analysis.Analyze(r.Context(), in)
	if err != nil {
		analysisError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleAnalyzeReplay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Before   string          `json:"before"`
		After    string          `json:"after"`
		Evidence analysis.Result `json:"evidence"`
	}
	if !decodeRequest(w, r, maxReplayBody, &in) {
		return
	}
	result, err := analysis.Replay(r.Context(), []byte(in.Before), []byte(in.After), in.Evidence)
	if err != nil {
		analysisError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

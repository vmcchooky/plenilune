// Package demohttp exposes the deterministic foundation to the local demo UI.
package demohttp

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
)

const (
	maxParseBody = 8 << 20
	maxTraceBody = 4 << 20
)

type parseRequest struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

type parsedACL struct {
	ACL    ciscoacl.ACL `json:"acl"`
	SHA256 string       `json:"sha256"`
}

type parseResponse struct {
	Before parsedACL `json:"before"`
	After  parsedACL `json:"after"`
}

type traceRequest struct {
	Config string          `json:"config"`
	Packet ciscoacl.Packet `json:"packet"`
}

type normalizeResponse struct {
	Set    intent.Set `json:"set"`
	SHA256 string     `json:"sha256"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	Side    string `json:"side,omitempty"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

// NewHandler serves the built UI and same-origin localhost API. Exact comparison
// and repair are offline; only an explicitly enabled AI session uses a provider.
func NewHandler(webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("POST /api/parse", handleParse)
	mux.HandleFunc("POST /api/analyze", handleAnalyze)
	mux.HandleFunc("POST /api/analyze/replay", handleAnalyzeReplay)
	mux.HandleFunc("POST /api/obligations/normalize", handleNormalize)
	mux.HandleFunc("POST /api/policy/normalize", handlePolicyNormalize)
	mux.HandleFunc("POST /api/trace", handleTrace)
	mux.HandleFunc("POST /api/compare", handleCompare)
	mux.HandleFunc("POST /api/compare/replay", handleComparisonReplay)
	mux.HandleFunc("POST /api/repair/verify", handleRepair)
	mux.HandleFunc("POST /api/repair/replay", handleRepairReplay)
	mux.HandleFunc("POST /api/agent/session", handleAgentSession)
	mux.Handle("GET /", staticHandler(webDir))
	return securityHeaders(mux)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "plenilune-demo",
		"runtime": map[string]string{
			"parse":       "available",
			"trace":       "available",
			"obligations": "available",
			"comparison":  "available",
			"ai":          "available_opt_in",
			"repair":      "available",
			"analysis":    "available",
		},
	})
}

func handleParse(w http.ResponseWriter, r *http.Request) {
	var in parseRequest
	if !decodeRequest(w, r, maxParseBody, &in) {
		return
	}
	before, err := parseSide(in.Before)
	if err != nil {
		writeDomainError(w, "before", err)
		return
	}
	after, err := parseSide(in.After)
	if err != nil {
		writeDomainError(w, "after", err)
		return
	}
	writeJSON(w, http.StatusOK, parseResponse{Before: before, After: after})
}

func parseSide(input string) (parsedACL, error) {
	acl, err := ciscoacl.Parse([]byte(input))
	if err != nil {
		return parsedACL{}, err
	}
	return parsedACL{ACL: acl, SHA256: evidence.ConfigDigest([]byte(input))}, nil
}

func handleNormalize(w http.ResponseWriter, r *http.Request) {
	var in intent.Set
	if !decodeRequest(w, r, intent.MaxJSONBytes, &in) {
		return
	}
	set, digest, err := intent.Digest(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "OBLIGATIONS_INVALID", Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, normalizeResponse{Set: set, SHA256: digest})
}

func handleTrace(w http.ResponseWriter, r *http.Request) {
	var in traceRequest
	if !decodeRequest(w, r, maxTraceBody, &in) {
		return
	}
	acl, err := ciscoacl.Parse([]byte(in.Config))
	if err != nil {
		writeDomainError(w, "config", err)
		return
	}
	decision, err := acl.Evaluate(in.Packet)
	if err != nil {
		writeDomainError(w, "packet", err)
		return
	}
	trace, err := evidence.SealTrace(evidence.ConfigDigest([]byte(in.Config)), in.Packet, decision)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "TRACE_INVALID", Message: "could not seal packet trace"})
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

func decodeRequest(w http.ResponseWriter, r *http.Request, limit int, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, apiError{Code: "CONTENT_TYPE", Message: "expected application/json"})
		return false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "INPUT_READ", Message: "could not read request"})
		return false
	}
	if err := jsonio.Decode(body, limit, dst); err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "JSON_INVALID", Message: err.Error()})
		return false
	}
	return true
}

func writeDomainError(w http.ResponseWriter, side string, err error) {
	out := apiError{Code: "INPUT_INVALID", Message: "input validation failed", Side: side}
	var aclErr *ciscoacl.Error
	if errors.As(err, &aclErr) {
		out.Code, out.Message, out.Line = aclErr.Code, aclErr.Message, aclErr.Line
	}
	writeError(w, http.StatusBadRequest, out)
}

func writeError(w http.ResponseWriter, status int, err apiError) {
	writeJSON(w, status, errorEnvelope{Error: err})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "could not encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write(append(data, '\n')); err != nil {
		log.Printf("demo response write failed: %v", err)
	}
}

func staticHandler(webDir string) http.Handler {
	files := http.FileServer(http.Dir(webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if webDir == "" {
			writeError(w, http.StatusNotFound, apiError{Code: "UI_NOT_BUILT", Message: "build web/dist or pass -web"})
			return
		}
		files.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

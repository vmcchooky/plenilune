package demohttp

import (
	"errors"
	"net/http"
	"net/netip"
	"os"
	"plenilune/internal/ai"
	"plenilune/internal/orchestrate"
)

func handleAgentSession(w http.ResponseWriter, r *http.Request) {
	var in orchestrate.Request
	if !decodeRequest(w, r, maxParseBody, &in) {
		return
	}
	var provider ai.Completer
	model := ""
	if in.Enabled {
		if !localAIRequest(r) {
			writeError(w, http.StatusForbidden, apiError{Code: "LOCAL_ORIGIN_REQUIRED", Message: "AI sessions require a local host and origin"})
			return
		}
		cfg, err := ai.ConfigFromEnv(os.Getenv)
		if err != nil && !errors.Is(err, ai.ErrNotConfigured) {
			writeError(w, http.StatusBadRequest, apiError{Code: "AI_CONFIG_INVALID", Message: "invalid server Meta configuration"})
			return
		}
		if err == nil {
			// This demo is qualified for one model only; an operator override must
			// not silently select another provider model or a fallback.
			if cfg.Model != ai.DefaultModel {
				writeError(w, http.StatusBadRequest, apiError{Code: "AI_MODEL_UNSUPPORTED", Message: "only muse-spark-1.3-contributor is enabled"})
				return
			}
			client, err := ai.New(cfg)
			if err != nil {
				writeError(w, http.StatusBadRequest, apiError{Code: "AI_CONFIG_INVALID", Message: "invalid server Meta configuration"})
				return
			}
			provider, model = client, cfg.Model
		}
	}
	result, err := orchestrate.Run(r.Context(), in, provider, model)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "SESSION_INPUT_INVALID", Message: "invalid session input, policy or options"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// localAIRequest is the AI session's own second check, kept alongside the
// shared localOnly guard: an enabled AI session spends a provider key, so it
// re-verifies the loopback host and origin before any provider configuration
// is read.
func localAIRequest(r *http.Request) bool {
	host, _ := splitAuthority(r.Host)
	if !loopbackHost(host) {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return loopbackOrigin(origin)
}
func loopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

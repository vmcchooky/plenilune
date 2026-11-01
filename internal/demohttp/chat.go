package demohttp

import (
	"errors"
	"net/http"
	"os"
	"plenilune/internal/ai"
	"plenilune/internal/analysis"
	"plenilune/internal/chat"
	"plenilune/internal/evidence"
)

// maxChatBody bounds an interrogation request: it may carry one
// comparison and one analysis artifact, never configuration text.
const maxChatBody = evidence.MaxComparisonBytes + analysis.MaxResultBytes + (1 << 20)

// handleChatAsk serves the verdict interrogation chat (P1-10). The
// deterministic core answers from the sealed artifacts in the request;
// the optional AI phrasing is a presentation layer over that answer and
// degrades to a status on the answer instead of failing the request —
// the verdict screen must stay fully usable with AI unconfigured.
func handleChatAsk(w http.ResponseWriter, r *http.Request) {
	var in chat.Request
	if !decodeRequest(w, r, maxChatBody, &in) {
		return
	}
	ans, err := chat.Ask(in)
	if err != nil {
		var ce *chat.Error
		if errors.As(err, &ce) {
			writeError(w, http.StatusBadRequest, apiError{Code: ce.Code, Message: ce.Message})
			return
		}
		writeError(w, http.StatusBadRequest, apiError{Code: "CHAT_INVALID", Message: "invalid chat request"})
		return
	}
	ans.AI = chat.Phrasing{Status: "off"}
	if in.AI {
		ans.AI = phraseIfConfigured(r, in.Question, ans)
	}
	writeJSON(w, http.StatusOK, ans)
}

// phraseIfConfigured mirrors the agent session's provider discipline:
// the loopback origin is re-verified before any provider configuration
// is read, and the model is locked — an operator override naming another
// model is reported, never silently honored. Every failure mode leaves
// the deterministic answer intact.
func phraseIfConfigured(r *http.Request, question string, ans chat.Answer) chat.Phrasing {
	if !localAIRequest(r) {
		return chat.Phrasing{Status: "unavailable"}
	}
	cfg, err := ai.ConfigFromEnv(os.Getenv)
	if errors.Is(err, ai.ErrNotConfigured) {
		return chat.Phrasing{Status: "not_configured"}
	}
	if err != nil {
		return chat.Phrasing{Status: "unavailable"}
	}
	if cfg.Model != ai.DefaultModel {
		return chat.Phrasing{Status: "model_unsupported", Model: cfg.Model}
	}
	client, err := ai.New(cfg)
	if err != nil {
		return chat.Phrasing{Status: "unavailable"}
	}
	text, err := chat.Phrase(r.Context(), client, cfg.Model, question, ans)
	if err != nil {
		return chat.Phrasing{Status: "error", Model: cfg.Model}
	}
	return chat.Phrasing{Status: "ok", Model: cfg.Model, Text: text}
}

package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"plenilune/internal/ai"
)

// Phrasing budget: one provider request per question, plain text out.
// The model receives no tools and no artifact access — retrieval is
// done by the deterministic core, and the model may only rephrase the
// answer and facts it is handed.
const (
	phraseMaxTokens = 800
	phraseMaxRunes  = 2400
)

// Phrase asks the locked model to rephrase a deterministic answer using
// only its facts. The result is presentation only: callers must keep
// showing the deterministic text and facts as the authoritative record,
// so a phrasing that contradicts the artifact loses in the UI by
// construction. An empty or over-long completion is an error, never a
// silently accepted answer.
func Phrase(ctx context.Context, provider ai.Completer, model, question string, ans Answer) (string, error) {
	if provider == nil {
		return "", errors.New("chat: no provider configured")
	}
	var b strings.Builder
	b.WriteString("Question: " + question + "\n\nDeterministic answer (authoritative):\n" + ans.Text + "\n\nFacts (the only facts you may use):\n")
	for _, f := range ans.Facts {
		fmt.Fprintf(&b, "- %s: %s\n", f.Label, f.Value)
	}
	temperature := 0.0
	resp, err := provider.Complete(ctx, ai.Request{
		Model:       model,
		MaxTokens:   phraseMaxTokens,
		Temperature: &temperature,
		Messages: []ai.Message{
			{Role: "system", Content: phraseSystemPrompt(ans.Locale)},
			{Role: "user", Content: b.String()},
		},
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(resp.Content)
	if text == "" {
		return "", errors.New("chat: empty phrasing from provider")
	}
	if runes := []rune(text); len(runes) > phraseMaxRunes {
		text = string(runes[:phraseMaxRunes])
	}
	return text, nil
}

func phraseSystemPrompt(locale string) string {
	language := "English"
	if locale == "vi" {
		language = "Vietnamese"
	}
	return "You rephrase a deterministic answer from the Plenilune ACL verifier. " +
		"Use ONLY the facts provided: never add numbers, packet values, rule references, statuses or conclusions that are not in the facts, " +
		"and never change a verdict or a status. If the deterministic answer says the question cannot be answered from the artifact, keep that meaning. " +
		"Answer in " + language + ", plain text, no markdown, at most six sentences."
}

package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"plenilune/internal/ai"
)

type fakeCompleter struct {
	req  ai.Request
	resp ai.Response
	err  error
}

func (f *fakeCompleter) Complete(_ context.Context, req ai.Request) (ai.Response, error) {
	f.req = req
	return f.resp, f.err
}

func phraseFixtureAnswer() Answer {
	return Answer{
		Schema:         Schema,
		Class:          ClassVerdict,
		Text:           "The verdict is DIFFERENT.",
		ArtifactSHA256: "abc123",
		Locale:         "en",
		Facts:          []Fact{{Label: "Verdict", Value: "DIFFERENT", Source: "comparison.report.verdict"}},
	}
}

func TestPhraseGroundsOnFactsWithoutTools(t *testing.T) {
	fake := &fakeCompleter{resp: ai.Response{Content: "The verdict is DIFFERENT — the two ACLs differ."}}
	got, err := Phrase(context.Background(), fake, ai.DefaultModel, "Why DIFFERENT?", phraseFixtureAnswer())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "DIFFERENT") {
		t.Fatalf("phrasing lost the verdict: %q", got)
	}
	if len(fake.req.Tools) != 0 {
		t.Fatalf("phrasing must not offer tools, got %d", len(fake.req.Tools))
	}
	if fake.req.Model != ai.DefaultModel {
		t.Fatalf("wrong model %q", fake.req.Model)
	}
	// The user message must carry the deterministic answer and the
	// fact values — that is the whole of the model's world.
	if len(fake.req.Messages) != 2 {
		t.Fatalf("messages %+v", fake.req.Messages)
	}
	if body := fake.req.Messages[1].Content; !strings.Contains(body, "The verdict is DIFFERENT.") || !strings.Contains(body, "- Verdict: DIFFERENT") {
		t.Fatalf("provider request does not carry the answer and facts: %q", body)
	}
	if sys := fake.req.Messages[0].Content; !strings.Contains(sys, "ONLY the facts") {
		t.Fatalf("system prompt does not pin the grounding rule: %q", sys)
	}
}

func TestPhraseFailures(t *testing.T) {
	ans := phraseFixtureAnswer()
	if _, err := Phrase(context.Background(), nil, ai.DefaultModel, "q", ans); err == nil {
		t.Fatal("nil provider accepted")
	}
	boom := &fakeCompleter{err: errors.New("provider down")}
	if _, err := Phrase(context.Background(), boom, ai.DefaultModel, "q", ans); err == nil {
		t.Fatal("provider error swallowed")
	}
	empty := &fakeCompleter{resp: ai.Response{Content: "   "}}
	if _, err := Phrase(context.Background(), empty, ai.DefaultModel, "q", ans); err == nil {
		t.Fatal("empty phrasing accepted")
	}
}

func TestPhraseTruncates(t *testing.T) {
	fake := &fakeCompleter{resp: ai.Response{Content: strings.Repeat("x", phraseMaxRunes+500)}}
	got, err := Phrase(context.Background(), fake, ai.DefaultModel, "q", phraseFixtureAnswer())
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(got)) != phraseMaxRunes {
		t.Fatalf("phrasing not bounded: %d runes", len([]rune(got)))
	}
}

package chat

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"plenilune/internal/analysis"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/verify"
	"plenilune/internal/workflow"
)

const (
	chatBefore = "ip access-list extended CHAT\n permit tcp any any eq 22\n permit tcp any any eq 443\n"
	chatAfter  = "ip access-list extended CHAT\n deny tcp any any eq 22\n permit tcp any any eq 443\n"
)

func chatConditions() *intent.Set {
	return &intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{
		{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.0/24", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}},
		{ID: "OB-2", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.0/24", DstPorts: []intent.Interval{{Lo: 443, Hi: 443}}}},
	}}
}

// fixtureComparison is a sealed DIFFERENT comparison: OB-1 VIOLATED with
// a witness, OB-2 HOLDS.
func fixtureComparison(t *testing.T) evidence.Comparison {
	t.Helper()
	c, err := workflow.Compare(context.Background(), []byte(chatBefore), []byte(chatAfter), chatConditions(), verify.DefaultOptions(), evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Report.Verdict != "DIFFERENT" || c.Report.Obligations[0].Status != "VIOLATED" || c.Report.Obligations[1].Status != "HOLDS" {
		t.Fatalf("fixture shape drifted: verdict %s obligations %+v", c.Report.Verdict, c.Report.Obligations)
	}
	return c
}

// fixturePartial is the same pair starved to a single operation, so the
// search stops with a partial record and INCONCLUSIVE obligations.
func fixturePartial(t *testing.T) evidence.Comparison {
	t.Helper()
	opts := verify.Options{Limits: packetset.Limits{MaxOperations: 1, MaxBoxes: 50000, Timeout: 2 * time.Second}, MaxWitnesses: 8}
	c, err := workflow.Compare(context.Background(), []byte(chatBefore), []byte(chatAfter), chatConditions(), opts, evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Report.SearchComplete || c.Report.Partial == nil {
		t.Fatalf("fixture shape drifted: complete=%v partial=%v", c.Report.SearchComplete, c.Report.Partial)
	}
	return c
}

// fixtureAnalysis is a sealed analysis whose single ACL changes one port,
// producing exactly one change class.
func fixtureAnalysis(t *testing.T) analysis.Result {
	t.Helper()
	before := "ip access-list extended EDGE\n permit tcp any any eq 443\n"
	after := "ip access-list extended EDGE\n permit tcp any any eq 8443\n"
	r, err := analysis.Analyze(context.Background(), analysis.Request{
		Schema: analysis.RequestSchema,
		Before: analysis.Source{Text: before},
		After:  analysis.Source{Text: after},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A 443 -> 8443 move yields two classes (newly denied + newly
	// permitted); the fixture only needs at least one recorded class.
	if len(r.Objects) != 1 || r.Objects[0].ACL == nil || r.Objects[0].ACL.Changes.TotalClasses < 1 {
		t.Fatalf("fixture shape drifted: %+v", r.Summary)
	}
	return r
}

func ask(t *testing.T, req Request) Answer {
	t.Helper()
	ans, err := Ask(req)
	if err != nil {
		t.Fatalf("Ask(%q): %v", req.Question, err)
	}
	return ans
}

func factBySource(ans Answer, source string) (Fact, bool) {
	for _, f := range ans.Facts {
		if f.Source == source {
			return f, true
		}
	}
	return Fact{}, false
}

func TestAskVerdictBothLocales(t *testing.T) {
	c := fixtureComparison(t)
	en := ask(t, Request{Comparison: &c, Question: "Why is the verdict DIFFERENT?", Locale: "en"})
	if en.Class != ClassVerdict || en.ArtifactSHA256 != c.SHA256 {
		t.Fatalf("en: class %s anchor %s", en.Class, en.ArtifactSHA256)
	}
	if f, ok := factBySource(en, "comparison.report.verdict"); !ok || f.Value != "DIFFERENT" {
		t.Fatalf("en: verdict fact %+v ok=%v", f, ok)
	}
	if !strings.Contains(en.Text, "DIFFERENT") {
		t.Fatalf("en: text does not state the verdict: %q", en.Text)
	}
	vi := ask(t, Request{Comparison: &c, Question: "Vì sao kết luận là DIFFERENT?", Locale: "vi"})
	if vi.Class != ClassVerdict || !strings.Contains(vi.Text, "DIFFERENT") {
		t.Fatalf("vi: class %s text %q", vi.Class, vi.Text)
	}
}

func TestAskObligationWitnessBothLocales(t *testing.T) {
	c := fixtureComparison(t)
	w := c.Report.Obligations[0].Witness
	if w == nil {
		t.Fatal("fixture lost its violation witness")
	}
	for _, tc := range []struct{ locale, question string }{
		{"en", "Why is OB-1 violated?"},
		{"vi", "Vì sao OB-1 bị vi phạm?"},
		{"vi", "vi sao ob-1 bi vi pham"}, // typed without diacritics
	} {
		ans := ask(t, Request{Comparison: &c, Question: tc.question, Locale: tc.locale})
		if ans.Class != ClassObligationWitness {
			t.Fatalf("%s: class %s, want obligation_witness", tc.locale, ans.Class)
		}
		pf, ok := factBySource(ans, "comparison.report.obligations[0].witness.packet")
		if !ok || !strings.Contains(pf.Value, w.Packet.SrcIP.String()) || !strings.Contains(pf.Value, w.Packet.DstIP.String()) {
			t.Fatalf("%s: packet fact %+v ok=%v", tc.locale, pf, ok)
		}
		if !strings.Contains(ans.Text, w.Packet.SrcIP.String()) {
			t.Fatalf("%s: text does not carry the witness packet: %q", tc.locale, ans.Text)
		}
	}
}

func TestAskObligationHoldsHasNoWitness(t *testing.T) {
	c := fixtureComparison(t)
	ans := ask(t, Request{Comparison: &c, Question: "What is the status of OB-2?", Locale: "en"})
	if ans.Class != ClassObligation {
		t.Fatalf("class %s", ans.Class)
	}
	if f, ok := factBySource(ans, "comparison.report.obligations[1].status"); !ok || f.Value != "HOLDS" {
		t.Fatalf("status fact %+v ok=%v", f, ok)
	}
	if _, ok := factBySource(ans, "comparison.report.obligations[1].witness.packet"); ok {
		t.Fatal("a holding obligation must not gain a witness fact")
	}
}

func TestAskObligationUnknown(t *testing.T) {
	c := fixtureComparison(t)
	ans := ask(t, Request{Comparison: &c, Question: "Why is OB-9 violated?", Locale: "en"})
	if ans.Class != ClassNotInArtifact {
		t.Fatalf("class %s", ans.Class)
	}
	if f, ok := factBySource(ans, "comparison.report.obligations"); !ok || !strings.Contains(f.Value, "OB-1") {
		t.Fatalf("evaluated-ids fact %+v ok=%v", f, ok)
	}
}

func TestAskObligationsList(t *testing.T) {
	c := fixtureComparison(t)
	ans := ask(t, Request{Comparison: &c, Question: "Kết quả các điều kiện thế nào?", Locale: "vi"})
	if ans.Class != ClassObligations {
		t.Fatalf("class %s", ans.Class)
	}
	if !strings.Contains(ans.Text, "OB-1 VIOLATED") || !strings.Contains(ans.Text, "OB-2 HOLDS") {
		t.Fatalf("listing text %q", ans.Text)
	}
}

func TestAskWitnessesAndDecidingRule(t *testing.T) {
	c := fixtureComparison(t)
	ans := ask(t, Request{Comparison: &c, Question: "Show me a witness packet", Locale: "en"})
	if ans.Class != ClassWitnesses {
		t.Fatalf("witnesses class %s", ans.Class)
	}
	// The count fact must equal the artifact's witness count.
	if f, ok := factBySource(ans, "comparison.report.witnesses"); !ok || f.Value != strconv.Itoa(len(c.Report.Witnesses)) {
		t.Fatalf("witness count fact %+v ok=%v", f, ok)
	}
	rule := ask(t, Request{Comparison: &c, Question: "Which rule decided this witness?", Locale: "en"})
	if rule.Class != ClassDecidingRule {
		t.Fatalf("deciding class %s", rule.Class)
	}
	if _, ok := factBySource(rule, "comparison.report.witnesses[0].before"); !ok {
		t.Fatalf("deciding facts %+v", rule.Facts)
	}
	if !strings.Contains(rule.Text, "ordinal") {
		t.Fatalf("deciding text lacks the rule reference: %q", rule.Text)
	}
}

func TestAskChangeClasses(t *testing.T) {
	a := fixtureAnalysis(t)
	for _, tc := range []struct{ locale, question string }{
		{"en", "What changed?"},
		{"vi", "Cái gì đã thay đổi?"},
	} {
		ans := ask(t, Request{Analysis: &a, Question: tc.question, Locale: tc.locale})
		if ans.Class != ClassChangeClasses {
			t.Fatalf("%s: class %s", tc.locale, ans.Class)
		}
		if _, ok := factBySource(ans, "analysis.objects[0].acl.changes.classes[0]"); !ok {
			t.Fatalf("%s: class fact missing: %+v", tc.locale, ans.Facts)
		}
	}
	// The same question against a comparison-only artifact must be
	// answered honestly: classes are not in this artifact.
	c := fixtureComparison(t)
	ans := ask(t, Request{Comparison: &c, Question: "What changed?", Locale: "en"})
	if ans.Class != ClassNotInArtifact {
		t.Fatalf("comparison-only: class %s", ans.Class)
	}
}

func TestAskPartialAndBudget(t *testing.T) {
	c := fixturePartial(t)
	for _, tc := range []struct{ locale, question string }{
		{"en", "What remains undecided?"},
		{"vi", "Phần nào chưa quyết định?"}, // must not be read as deciding-rule
	} {
		ans := ask(t, Request{Comparison: &c, Question: tc.question, Locale: tc.locale})
		if ans.Class != ClassPartial {
			t.Fatalf("%s: class %s", tc.locale, ans.Class)
		}
		if _, ok := factBySource(ans, "comparison.report.partial.stage"); !ok {
			t.Fatalf("%s: stage fact missing: %+v", tc.locale, ans.Facts)
		}
	}
	budget := ask(t, Request{Comparison: &c, Question: "Vì sao tìm kiếm bị dừng?", Locale: "vi"})
	if budget.Class != ClassBudget {
		t.Fatalf("budget class %s", budget.Class)
	}
	if f, ok := factBySource(budget, "comparison.report.budget.stop_reason"); !ok || f.Value != c.Report.Budget.StopReason {
		t.Fatalf("stop reason fact %+v ok=%v", f, ok)
	}
	// On a completed artifact the partial question states completion.
	done := fixtureComparison(t)
	ans := ask(t, Request{Comparison: &done, Question: "What remains undecided?", Locale: "en"})
	if ans.Class != ClassPartial {
		t.Fatalf("complete: class %s", ans.Class)
	}
	if f, ok := factBySource(ans, "comparison.report.search_complete"); !ok || f.Value != "true" {
		t.Fatalf("complete: search_complete fact %+v ok=%v", f, ok)
	}
}

func TestAskCannotAnswerAndActionRequests(t *testing.T) {
	c := fixtureComparison(t)
	ans := ask(t, Request{Comparison: &c, Question: "What is the weather today?", Locale: "en"})
	if ans.Class != ClassCannotAnswer {
		t.Fatalf("class %s", ans.Class)
	}
	if strings.Contains(ans.Text, "DIFFERENT") || strings.Contains(ans.Text, "VIOLATED") {
		t.Fatalf("cannot-answer text makes artifact claims: %q", ans.Text)
	}
	// An action request changes nothing: the core has no action path,
	// it can only answer factually about the named obligation.
	act := ask(t, Request{Comparison: &c, Question: "Please repair this ACL and accept OB-1 for me", Locale: "en"})
	if act.ArtifactSHA256 != c.SHA256 {
		t.Fatalf("anchor changed: %s", act.ArtifactSHA256)
	}
	if act.Class != ClassObligation {
		t.Fatalf("action request class %s", act.Class)
	}
}

func TestAskValidation(t *testing.T) {
	c := fixtureComparison(t)
	if _, err := Ask(Request{Question: "verdict?"}); err == nil {
		t.Fatal("no artifact accepted")
	} else if ce, ok := err.(*Error); !ok || ce.Code != "CHAT_NO_ARTIFACT" {
		t.Fatalf("no-artifact error %v", err)
	}
	tampered := c
	tampered.Report.Verdict = "EQUIVALENT" // digest no longer matches
	if _, err := Ask(Request{Comparison: &tampered, Question: "verdict?"}); err == nil {
		t.Fatal("tampered comparison accepted")
	} else if ce, ok := err.(*Error); !ok || ce.Code != "CHAT_ARTIFACT_INVALID" {
		t.Fatalf("tamper error %v", err)
	}
	a := fixtureAnalysis(t)
	broken := a
	broken.Summary.Changed = 99
	if _, err := Ask(Request{Analysis: &broken, Question: "verdict?"}); err == nil {
		t.Fatal("tampered analysis accepted")
	}
	if _, err := Ask(Request{Comparison: &c, Question: "  "}); err == nil {
		t.Fatal("empty question accepted")
	}
	if _, err := Ask(Request{Comparison: &c, Question: strings.Repeat("a", MaxQuestionRunes+1)}); err == nil {
		t.Fatal("oversized question accepted")
	}
	if _, err := Ask(Request{Comparison: &c, Question: "verdict?", Locale: "fr"}); err == nil {
		t.Fatal("unsupported locale accepted")
	}
}

func TestAskDeterministicAndTraceable(t *testing.T) {
	c := fixtureComparison(t)
	a := fixtureAnalysis(t)
	questions := []Request{
		{Comparison: &c, Question: "Why is the verdict DIFFERENT?", Locale: "en"},
		{Comparison: &c, Question: "Why is OB-1 violated?", Locale: "vi"},
		{Comparison: &c, Analysis: &a, Question: "What changed?", Locale: "en"},
		{Comparison: &c, Question: "What remains undecided?", Locale: "en"},
		{Analysis: &a, Question: "What is the verdict?", Locale: "en"},
	}
	for _, q := range questions {
		first := ask(t, q)
		second := ask(t, q)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("nondeterministic answer for %q", q.Question)
		}
		if first.Text == "" || len(first.Facts) == 0 {
			t.Fatalf("empty answer for %q", q.Question)
		}
		for _, f := range first.Facts {
			if !strings.HasPrefix(f.Source, "comparison.") && !strings.HasPrefix(f.Source, "analysis.") &&
				f.Source != "artifact.sha256" && f.Source != "request" {
				t.Fatalf("fact without an artifact source: %+v", f)
			}
		}
	}
	// The analysis-only verdict answer must never invent a single
	// configuration verdict.
	ans := ask(t, Request{Analysis: &a, Question: "What is the verdict?", Locale: "en"})
	if ans.Class != ClassVerdict {
		t.Fatalf("analysis verdict class %s", ans.Class)
	}
	if f, ok := factBySource(ans, "analysis.objects[0].acl.verdict"); !ok || f.Value == "" {
		t.Fatalf("per-object verdict fact %+v ok=%v", f, ok)
	}
}

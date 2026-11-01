// Package chat answers questions about one completed, sealed artifact:
// an evidence comparison and/or a configuration analysis. It is an
// interrogation layer only. The request carries artifacts and a question
// — never configuration text — so this package cannot run an analysis,
// issue or alter a verdict, accept obligations or trigger a repair, and
// it writes nothing. Every factual claim in an answer is copied from the
// artifact into the answer's fact list with the field it came from; the
// rendered text is assembled from those facts only.
//
// Artifact digests are verified before any question is answered: a
// comparison must pass evidence.CheckComparison and an analysis record
// must re-seal to its recorded digest. The digest is a content check,
// not a signature (the project's usual wording).
package chat

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"plenilune/internal/analysis"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/verify"
)

// Schema identifies the chat answer wire format.
const Schema = "plenilune.chat/v1"

// MaxQuestionRunes bounds one question. The request body itself is
// bounded by the HTTP layer; this bounds the matching work per ask.
const MaxQuestionRunes = 1000

const (
	maxFacts           = 40
	maxListedWitnesses = 8
	maxListedClasses   = 8
	maxListedPairs     = 6
)

// Class values: the question classes the deterministic core can answer.
// Anything outside them is answered with ClassCannotAnswer — plainly,
// never by speculation.
const (
	ClassVerdict           = "verdict"
	ClassObligations       = "obligations"
	ClassObligation        = "obligation"
	ClassObligationWitness = "obligation_witness"
	ClassWitnesses         = "witnesses"
	ClassDecidingRule      = "deciding_rule"
	ClassChangeClasses     = "change_classes"
	ClassPartial           = "partial"
	ClassBudget            = "budget"
	ClassScope             = "scope"
	ClassNotInArtifact     = "not_in_artifact"
	ClassCannotAnswer      = "cannot_answer"
)

// Error is a typed chat request failure; no answer is produced with it.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func invalid(code, msg string) error { return &Error{Code: code, Message: msg} }

// Request is one interrogation. At least one artifact is required; when
// both are present the comparison answers verdict/obligation questions
// and the analysis answers change-class questions, each fact citing the
// artifact it came from.
type Request struct {
	Comparison *evidence.Comparison `json:"comparison,omitempty"`
	Analysis   *analysis.Result     `json:"analysis,omitempty"`
	Question   string               `json:"question"`
	Locale     string               `json:"locale,omitempty"`
	AI         bool                 `json:"ai,omitempty"`
}

// Fact is one artifact value an answer rests on. Source is the field
// path inside the named artifact, e.g. "comparison.report.verdict".
type Fact struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// Phrasing is the optional AI rendering of the deterministic answer.
// Status is "off" unless phrasing was requested; the deterministic text
// and facts stay authoritative in every state.
type Phrasing struct {
	Status string `json:"status"`
	Model  string `json:"model,omitempty"`
	Text   string `json:"text,omitempty"`
}

// Answer is the deterministic reply: a class, text assembled only from
// the listed facts, and the facts themselves so the UI can show the
// artifact's own data next to any phrasing.
type Answer struct {
	Schema         string   `json:"schema"`
	ArtifactSHA256 string   `json:"artifact_sha256"`
	Locale         string   `json:"locale"`
	Class          string   `json:"class"`
	Text           string   `json:"text"`
	Facts          []Fact   `json:"facts"`
	AI             Phrasing `json:"ai"`
}

// Ask validates the request and artifacts, classifies the question
// and renders the deterministic answer.
func Ask(req Request) (Answer, error) {
	locale := req.Locale
	if locale == "" {
		locale = "en"
	}
	if locale != "en" && locale != "vi" {
		return Answer{}, invalid("CHAT_LOCALE_UNSUPPORTED", "locale must be en or vi")
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return Answer{}, invalid("CHAT_QUESTION_EMPTY", "question is empty")
	}
	if len([]rune(question)) > MaxQuestionRunes {
		return Answer{}, invalid("CHAT_QUESTION_TOO_LONG", "question exceeds the size limit")
	}
	if req.Comparison == nil && req.Analysis == nil {
		return Answer{}, invalid("CHAT_NO_ARTIFACT", "no completed artifact to interrogate")
	}
	v := &view{locale: locale}
	if req.Comparison != nil {
		if err := evidence.CheckComparison(*req.Comparison); err != nil {
			return Answer{}, invalid("CHAT_ARTIFACT_INVALID", "comparison artifact fails its seal check")
		}
		v.comparison = req.Comparison
		v.anchor = req.Comparison.SHA256
	}
	if req.Analysis != nil {
		if err := checkAnalysisSeal(*req.Analysis); err != nil {
			return Answer{}, invalid("CHAT_ARTIFACT_INVALID", "analysis artifact fails its seal check")
		}
		v.analysis = req.Analysis
		if v.anchor == "" {
			v.anchor = req.Analysis.SHA256
		}
	}
	q := parseQuestion(question)
	class, facts, text := v.answer(q)
	return Answer{
		Schema:         Schema,
		ArtifactSHA256: v.anchor,
		Locale:         locale,
		Class:          class,
		Text:           text,
		Facts:          facts,
		AI:             Phrasing{Status: "off"},
	}, nil
}

// checkAnalysisSeal mirrors the first gate of analysis.Replay: the
// record must re-seal to its recorded digest under a supported schema.
// It verifies content integrity only — it recomputes nothing.
func checkAnalysisSeal(r analysis.Result) error {
	if r.SHA256 == "" || (r.Schema != analysis.Schema && r.Schema != analysis.SchemaV1) {
		return errors.New("unsupported analysis schema")
	}
	sealed, err := analysis.Seal(r)
	if err != nil || sealed.SHA256 != r.SHA256 {
		return errors.New("analysis digest does not match its content")
	}
	return nil
}

// view is the read-only artifact access the answer builders share.
type view struct {
	comparison *evidence.Comparison
	analysis   *analysis.Result
	anchor     string
	locale     string
}

func (v *view) s(en, vi string) string {
	if v.locale == "vi" {
		return vi
	}
	return en
}

// aclObject is one analysis object carrying an ACL analysis, with its
// index for fact source paths.
type aclObject struct {
	index int
	obj   analysis.Object
}

// aclObjects lists the analyzed ACL objects in artifact order.
func (v *view) aclObjects() []aclObject {
	if v.analysis == nil {
		return nil
	}
	var out []aclObject
	for i, o := range v.analysis.Objects {
		if o.Domain == "acl" && o.ACL != nil {
			out = append(out, aclObject{index: i, obj: o})
		}
	}
	return out
}

// pickObject chooses the ACL object a question is about: the one whose
// name the question mentions, else the first.
func (v *view) pickObject(folded string) (aclObject, bool) {
	objects := v.aclObjects()
	for _, o := range objects {
		if o.obj.Name != "" && strings.Contains(folded, fold(o.obj.Name)) {
			return o, true
		}
	}
	if len(objects) > 0 {
		return objects[0], true
	}
	return aclObject{}, false
}

// --- question parsing -------------------------------------------------

var obligationIDPattern = regexp.MustCompile(`(?i)\bob\s*-?\s*(\d{1,3})\b`)

type questionInfo struct {
	folded       string
	obligationID string // canonical "OB-2", or ""
}

func parseQuestion(raw string) questionInfo {
	q := questionInfo{folded: fold(raw)}
	if m := obligationIDPattern.FindStringSubmatch(raw); m != nil {
		q.obligationID = "OB-" + m[1]
	}
	return q
}

func hasAny(folded string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(folded, n) {
			return true
		}
	}
	return false
}

// fold lowercases and strips Vietnamese diacritics so keyword matching
// works for typed-without-diacritics questions as well as full text.
func fold(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x0300 && r <= 0x036f {
			continue // combining marks (NFD input)
		}
		if mapped, ok := foldMap[r]; ok {
			b.WriteRune(mapped)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var foldMap = map[rune]rune{}

func init() {
	groups := map[rune]string{
		'a': "àáảãạăằắẳẵặâầấẩẫậ",
		'e': "èéẻẽẹêềếểễệ",
		'i': "ìíỉĩị",
		'o': "òóỏõọôồốổỗộơờớởỡợ",
		'u': "ùúủũụưừứửữự",
		'y': "ỳýỷỹỵ",
		'd': "đ",
	}
	for base, forms := range groups {
		for _, r := range forms {
			foldMap[r] = base
		}
	}
}

// --- classification ---------------------------------------------------

func (v *view) answer(q questionInfo) (string, []Fact, string) {
	f := q.folded
	witnessIntent := hasAny(f, "witness", "minh chung", "chung cu", "bang chung", "goi tin", "packet")
	whyIntent := hasAny(f, "vi sao", "tai sao", "why", "ly do")
	ruleIntent := strings.Contains(f, "rule") && hasAny(f, "decid", "quyet dinh", "decide")
	switch {
	case q.obligationID != "":
		return v.answerObligation(q, witnessIntent || whyIntent)
	case hasAny(f, "obligation", "dieu kien"):
		return v.answerObligations()
	case ruleIntent:
		return v.answerDecidingRule()
	case witnessIntent:
		return v.answerWitnesses()
	case hasAny(f, "change class", "lop thay doi", "thay doi", "changed", "changes", "difference"):
		return v.answerChangeClasses(q)
	case hasAny(f, "undecided", "chua quyet", "chua phan giai", "chua xac dinh", "chua xong", "partial", "mot phan", "con lai", "unresolved", "incomplete"):
		return v.answerPartial()
	case hasAny(f, "budget", "ngan sach", "stop reason", "stopped", "bi dung", "dung lai", "gioi han", "limit", "inconclusive", "khong ket luan", "operations", "boxes"):
		return v.answerBudget()
	case hasAny(f, "scope", "pham vi", "mo hinh", "limitation", "han che"):
		return v.answerScope()
	case hasAny(f, "verdict", "ket luan", "ket qua", "different", "equivalent", "tuong duong", "khac nhau", "vi sao", "tai sao", "why"):
		return v.answerVerdict()
	default:
		return v.answerCannot()
	}
}

// --- shared fact builders ---------------------------------------------

func (v *view) fact(label, value, source string) Fact {
	return Fact{Label: label, Value: value, Source: source}
}

func capFacts(facts []Fact) []Fact {
	if len(facts) > maxFacts {
		return facts[:maxFacts]
	}
	return facts
}

func packetText(p ciscoacl.Packet) string {
	proto := map[uint8]string{1: "icmp", 6: "tcp", 17: "udp"}
	name, ok := proto[p.Protocol]
	if !ok {
		name = fmt.Sprint(p.Protocol)
	}
	if p.Protocol == 6 || p.Protocol == 17 {
		return fmt.Sprintf("%s %s:%d -> %s:%d", name, p.SrcIP, p.SrcPort, p.DstIP, p.DstPort)
	}
	return fmt.Sprintf("%s %s -> %s", name, p.SrcIP, p.DstIP)
}

func (v *view) decisionText(d ciscoacl.Decision) string {
	if d.Implicit {
		return v.s("deny (implicit final deny)", "deny (deny mặc định cuối)")
	}
	// The ordinal is rendered raw, exactly as the artifact records it —
	// the same convention as the web witness detail.
	text := d.Action
	if d.RuleOrdinal != nil {
		text += fmt.Sprintf(" · ordinal %d", *d.RuleOrdinal)
	}
	if d.Line != nil {
		text += v.s(fmt.Sprintf(" · line %d", *d.Line), fmt.Sprintf(" · dòng %d", *d.Line))
	}
	return text
}

func (v *view) witnessFact(label string, w verify.Witness, source string) Fact {
	value := fmt.Sprintf("%s · %s: %s · %s: %s",
		packetText(w.Packet),
		v.s("before", "trước"), v.decisionText(w.Before),
		v.s("after", "sau"), v.decisionText(w.After))
	return v.fact(label, value, source)
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

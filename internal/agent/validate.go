package agent

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"plenilune/internal/ai"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
)

const (
	maxIntentBytes     = 2048
	maxRulesPerSide    = 64
	maxRuleBytes       = 2048
	maxExcerptBytes    = 32 << 10
	maxChanges         = 64
	maxChangeSummary   = 512
	maxEvidenceRefs    = 512
	maxEvidenceSummary = 1024
)

var (
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	tokenPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
)

// InputError is a caller contract violation. Messages never echo rule text.
type InputError struct {
	Code    string
	Message string
}

func (e *InputError) Error() string { return e.Message }

func inputError(message string) error {
	return &InputError{Code: "invalid_input", Message: message}
}

func validateInput(in Input) error {
	if in.Intent == "" || !utf8.ValidString(in.Intent) || len(in.Intent) > maxIntentBytes {
		return inputError("intent must be non-empty UTF-8 within the size limit")
	}
	if hasUnsafeControl(in.Intent) {
		return inputError("intent contains unsupported control characters")
	}
	if len([]rune(in.ACLName)) > 64 || hasControl(in.ACLName) {
		return inputError("ACL name is outside the allowed limits")
	}
	if err := validateRules(in.Before); err != nil {
		return err
	}
	if err := validateRules(in.After); err != nil {
		return err
	}
	if len(in.Before) == 0 && len(in.After) == 0 {
		return inputError("at least one ACL excerpt side is required")
	}
	if len(in.Changes) > maxChanges {
		return inputError("too many change summaries")
	}
	seen := make(map[string]bool, len(in.Changes))
	for _, c := range in.Changes {
		if !tokenPattern.MatchString(c.ID) || seen[c.ID] {
			return inputError("change IDs must be valid and unique")
		}
		seen[c.ID] = true
		if len(c.Kind) > 64 || len(c.Side) > 64 || hasControl(c.Kind+c.Side) ||
			len(c.Summary) > maxChangeSummary || hasUnsafeControl(c.Summary) || looksSensitive(c.Summary) {
			return inputError("change summary is outside the allowed limits")
		}
		if c.Ordinal != nil && *c.Ordinal < 0 || c.Line != nil && *c.Line < 1 {
			return inputError("change source refs are invalid")
		}
	}
	if (in.BaseBeforeSHA256 == "") != (in.BaseAfterSHA256 == "") {
		return inputError("both base digests must be provided together")
	}
	if in.BaseBeforeSHA256 != "" && (!digestPattern.MatchString(in.BaseBeforeSHA256) || !digestPattern.MatchString(in.BaseAfterSHA256)) {
		return inputError("base digests must be lowercase SHA-256 hex")
	}
	return nil
}

func validateRules(rules []string) error {
	if len(rules) > maxRulesPerSide {
		return inputError("ACL excerpt has too many rules")
	}
	total := 0
	for _, rule := range rules {
		if rule == "" || !utf8.ValidString(rule) || len(rule) > maxRuleBytes || hasControl(rule) || looksSensitive(rule) {
			return inputError("ACL excerpt line is outside the allowed limits")
		}
		if !canonicalRule(rule) {
			return inputError("ACL excerpt lines must be canonical permit/deny rules without headers, remarks or sequence prefixes")
		}
		total += len(rule)
	}
	if total > maxExcerptBytes {
		return inputError("ACL excerpt exceeds the size limit")
	}
	return nil
}

func validateObligations(in []intent.Obligation) (intent.Set, string, error) {
	set, digest, err := intent.Digest(intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: in})
	if err != nil {
		return intent.Set{}, "", jsonio.Invalid(err.Error())
	}
	return set, digest, nil
}

type modelOutput struct {
	Status      string              `json:"status"`
	Obligations []intent.Obligation `json:"obligations"`
	Questions   []string            `json:"questions"`
	Claims      []modelClaim        `json:"claims"`
	Summary     string              `json:"summary"`
}

// UnmarshalJSON requires every envelope field; missing and null are not zero.
func (m *modelOutput) UnmarshalJSON(data []byte) error {
	var in struct {
		Status      *string              `json:"status"`
		Obligations *[]intent.Obligation `json:"obligations"`
		Questions   *[]string            `json:"questions"`
		Claims      *[]modelClaim        `json:"claims"`
		Summary     *string              `json:"summary"`
	}
	if err := jsonio.Decode(data, ai.MaxContentBytes, &in); err != nil {
		return err
	}
	if in.Status == nil || in.Obligations == nil || in.Questions == nil || in.Claims == nil || in.Summary == nil {
		return jsonio.Invalid("model output requires status, obligations, questions, claims and summary")
	}
	*m = modelOutput{
		Status:      *in.Status,
		Obligations: *in.Obligations,
		Questions:   *in.Questions,
		Claims:      *in.Claims,
		Summary:     *in.Summary,
	}
	return nil
}

type modelClaim struct {
	Text          string   `json:"text"`
	ObligationIDs []string `json:"obligation_ids"`
	EvidenceIDs   []string `json:"evidence_ids"`
}

func parseModelOutput(content string) (*modelOutput, error) {
	if content == "" || len(content) > ai.MaxContentBytes {
		return nil, jsonio.Invalid("model output is empty or too large")
	}
	var out modelOutput
	if err := jsonio.Decode([]byte(content), ai.MaxContentBytes, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func validateStructure(out *modelOutput) error {
	if out.Status != "ok" && out.Status != "clarification_needed" {
		return jsonio.Invalid("status must be ok or clarification_needed")
	}
	if len(out.Obligations) > 12 {
		return jsonio.Invalid("too many obligations")
	}
	if len(out.Questions) > 3 {
		return jsonio.Invalid("too many clarification questions")
	}
	for _, q := range out.Questions {
		if strings.TrimSpace(q) == "" || len([]rune(q)) > 300 || hasUnsafeControl(q) {
			return jsonio.Invalid("clarification question is outside the allowed limits")
		}
	}
	if len(out.Claims) > 8 {
		return jsonio.Invalid("too many claims")
	}
	for _, c := range out.Claims {
		if len([]rune(c.Text)) > 500 || len(c.ObligationIDs) > 12 || len(c.EvidenceIDs) > 8 {
			return jsonio.Invalid("claim is outside the allowed limits")
		}
		for _, id := range c.ObligationIDs {
			if len(id) > 64 || hasControl(id) {
				return jsonio.Invalid("claim obligation ID is invalid")
			}
		}
		for _, id := range c.EvidenceIDs {
			if !evidenceID(id) {
				return jsonio.Invalid("claim evidence ID is invalid")
			}
		}
	}
	if len([]rune(out.Summary)) > 2000 || hasUnsafeControl(out.Summary) {
		return jsonio.Invalid("summary is outside the allowed limits")
	}
	return nil
}

func validateClarification(out *modelOutput) error {
	if err := validateStructure(out); err != nil {
		return err
	}
	if len(out.Obligations) != 0 {
		return jsonio.Invalid("clarification must not carry obligations")
	}
	if len(out.Claims) != 0 {
		return jsonio.Invalid("clarification must not carry claims")
	}
	if len(out.Questions) == 0 {
		return jsonio.Invalid("clarification requires at least one question")
	}
	return nil
}

func (r *runner) applyFinal(out *modelOutput) error {
	if err := validateStructure(out); err != nil {
		return err
	}
	if len(out.Questions) != 0 {
		return jsonio.Invalid("questions are only allowed for clarification")
	}
	if len(out.Obligations) > 0 {
		_, digest, err := validateObligations(out.Obligations)
		if err != nil {
			return err
		}
		if digest != r.accepted.digest {
			return jsonio.Invalid("final obligations must repeat the accepted set or be empty")
		}
	}
	claims, dropped := r.claimsFrom(out.Claims)
	r.claims = claims
	r.dropped += dropped
	r.final = out
	return nil
}

func (r *runner) claimsFrom(in []modelClaim) ([]Claim, int) {
	out := make([]Claim, 0, len(in))
	dropped := 0
	for _, c := range in {
		text := strings.TrimSpace(c.Text)
		if text == "" || len([]rune(text)) > 500 || hasUnsafeControl(text) || len(c.EvidenceIDs) == 0 {
			dropped++
			continue
		}
		evidenceIDs, ok := dedupeKnown(c.EvidenceIDs, func(id string) bool { return r.known[id] })
		if !ok {
			dropped++
			continue
		}
		obligationIDs, ok := dedupeKnown(c.ObligationIDs, func(id string) bool { return r.acceptedIDs[id] })
		if !ok {
			dropped++
			continue
		}
		out = append(out, Claim{
			Text:          text,
			ObligationIDs: obligationIDs,
			EvidenceIDs:   evidenceIDs,
			AIGenerated:   true,
		})
	}
	return out, dropped
}

func dedupeKnown(ids []string, known func(string) bool) ([]string, bool) {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !known(id) {
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, true
}

func (r *runner) validateObligationResults(set intent.Set, in []ObligationResult) ([]ObligationResult, error) {
	if len(in) != len(set.Obligations) {
		return nil, jsonio.Invalid("engine returned the wrong number of obligation results")
	}
	byID := make(map[string]ObligationResult, len(in))
	for _, res := range in {
		if !r.acceptedIDs[res.ID] {
			return nil, jsonio.Invalid("engine returned an unknown obligation ID")
		}
		if _, dup := byID[res.ID]; dup {
			return nil, jsonio.Invalid("engine returned a duplicate obligation result")
		}
		switch res.Status {
		case ObligationHolds, ObligationViolated, ObligationInconclusive:
		default:
			return nil, jsonio.Invalid("engine returned an unsupported obligation status")
		}
		if len(res.EvidenceIDs) > 8 {
			return nil, jsonio.Invalid("obligation result has too many evidence IDs")
		}
		if len(res.Reason) > 512 || hasUnsafeControl(res.Reason) || looksSensitive(res.Reason) {
			return nil, jsonio.Invalid("obligation reason is outside display limits")
		}
		if res.Status == ObligationViolated && len(res.EvidenceIDs) == 0 {
			return nil, jsonio.Invalid("violated obligation requires witness evidence")
		}
		byID[res.ID] = res
	}
	ordered := make([]ObligationResult, 0, len(set.Obligations))
	for _, ob := range set.Obligations {
		res, ok := byID[ob.ID]
		if !ok {
			return nil, jsonio.Invalid("engine omitted an accepted obligation")
		}
		ordered = append(ordered, res)
	}
	return ordered, nil
}

func (r *runner) registerResults(results []ObligationResult) error {
	for _, res := range results {
		if err := r.registerEvidence(res.EvidenceIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateChangesPage(page ChangesPage, limit int) error {
	if len(page.Items) > limit || len(page.Items) > 20 {
		return jsonio.Invalid("engine returned too many changes")
	}
	if len(page.NextCursor) > 64 || hasControl(page.NextCursor) {
		return jsonio.Invalid("engine returned an invalid cursor")
	}
	if len(page.EvidenceIDs) > 32 {
		return jsonio.Invalid("engine returned too many change evidence IDs")
	}
	seen := make(map[string]bool, len(page.Items))
	for _, c := range page.Items {
		if !tokenPattern.MatchString(c.ID) || seen[c.ID] {
			return jsonio.Invalid("engine returned invalid or duplicate change IDs")
		}
		seen[c.ID] = true
		if c.Kind == "" || len(c.Kind) > 64 || len(c.Side) > 64 || hasControl(c.Kind+c.Side) ||
			len(c.Summary) > maxChangeSummary || hasUnsafeControl(c.Summary) || looksSensitive(c.Summary) {
			return jsonio.Invalid("engine change summary is outside display limits")
		}
		if c.Ordinal != nil && *c.Ordinal < 0 || c.Line != nil && *c.Line < 1 {
			return jsonio.Invalid("engine change refs are invalid")
		}
	}
	return nil
}

func (r *runner) registerEvidence(ids []string) error {
	for _, id := range ids {
		if !evidenceID(id) {
			return jsonio.Invalid("engine returned an invalid evidence ID")
		}
		if !r.known[id] && len(r.known) >= maxEvidenceRefs {
			return jsonio.Invalid("evidence reference capacity exceeded")
		}
		r.known[id] = true
	}
	return nil
}

func evidenceID(id string) bool { return tokenPattern.MatchString(id) }

func canonicalRule(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(lower, "permit ") || strings.HasPrefix(lower, "deny ")
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func hasUnsafeControl(s string) bool {
	for _, r := range s {
		if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
			return true
		}
	}
	return false
}

var sensitiveMarkers = []string{
	"password", "secret", "community", "snmp-server", "banner ", "username ",
	"tacacs", "radius", "authentication-key",
}

func looksSensitive(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range sensitiveMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

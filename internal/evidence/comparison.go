package evidence

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/verify"
)

// ComparisonSchema is the current evidence schema (P2-1): the artifact
// carries two digests — content_sha256 over the semantic payload and the
// whole-envelope sha256 (the artifact digest). See CONTRACTS §5.
const ComparisonSchema = "plenilune.evidence/v2"

// ComparisonSchemaV1 is the frozen pre-dual-digest schema. New comparisons
// are never emitted as v1; v1 artifacts remain checkable and replayable
// under the original single-digest rules (see CheckComparison).
const ComparisonSchemaV1 = "plenilune.evidence/v1"
const ToolVersion = "0.2.0-dev"
const MaxComparisonBytes = 4 << 20

type Tool struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

type Input struct {
	SHA256 string `json:"sha256"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
}

// Comparison binds definitions, scope and a verifier-owned report. The
// digests are content receipts, not signatures or a certification of intent
// fidelity. Two identities are stamped (P2-1): ContentSHA256 is the digest
// of the semantic payload only (inputs, definitions, scope, parser/engine
// versions, options and results — the Report carries the options), so the
// same result produced by a different build keeps one content identity;
// SHA256 remains the whole-envelope digest (the artifact digest), computed
// over every field including the tool provenance and the content digest
// itself, and identifies this exact artifact. Frozen v1 artifacts carry
// only the envelope digest, in the same field.
type Comparison struct {
	Schema        string        `json:"schema"`
	Tool          Tool          `json:"tool"`
	ParserVersion string        `json:"parser_version"`
	EngineVersion string        `json:"engine_version"`
	Scope         string        `json:"scope"`
	Before        Input         `json:"before"`
	After         Input         `json:"after"`
	Conditions    *intent.Set   `json:"conditions"`
	Report        verify.Report `json:"report"`
	ContentSHA256 string        `json:"content_sha256,omitempty"`
	SHA256        string        `json:"sha256,omitempty"`
}

// SealComparison stamps the current engine unless the caller names the
// supported engine that actually produced the report, and seals under
// the current (v2) schema: both digests.
func SealComparison(c Comparison) (Comparison, error) {
	return SealComparisonAs(c, ComparisonSchema)
}

// SealComparisonAs seals a comparison in the shape of the named evidence
// schema. The current schema stamps both digests; the frozen v1 schema
// stamps only the envelope digest under the original rules and must not
// carry a content digest. The v1 shape exists so replay paths that embed
// comparisons (repair receipts) can recompute a recorded artifact in its
// own shape; new comparisons are only ever sealed by SealComparison.
func SealComparisonAs(c Comparison, schema string) (Comparison, error) {
	if schema != ComparisonSchema && schema != ComparisonSchemaV1 {
		return Comparison{}, errors.New("unsupported comparison schema")
	}
	if schema == ComparisonSchemaV1 && c.ContentSHA256 != "" {
		return Comparison{}, errors.New("v1 comparison must not carry a content digest")
	}
	if c.EngineVersion == "" {
		c.EngineVersion = verify.EngineVersion
	}
	c.Schema, c.ParserVersion, c.Scope = schema, ciscoacl.ParserVersion, intent.Scope
	if c.Conditions != nil {
		n, err := intent.Normalize(*c.Conditions)
		if err != nil {
			return Comparison{}, err
		}
		c.Conditions = &n
	}
	if err := validateComparison(c); err != nil {
		return Comparison{}, err
	}
	if schema == ComparisonSchema {
		content, err := comparisonContentDigest(c)
		if err != nil {
			return Comparison{}, err
		}
		c.ContentSHA256 = content
	}
	h, err := comparisonDigest(c)
	if err != nil {
		return Comparison{}, err
	}
	c.SHA256 = h
	return c, nil
}

// CheckComparison verifies a recorded comparison under the rules of its
// own schema version. A v2 artifact must carry a content digest that
// recomputes from its semantic payload AND an envelope digest that
// recomputes over the whole artifact, so a payload tamper is caught even
// when the forger recomputes the envelope over a forged content claim.
// A frozen v1 artifact is checked under the original rules: envelope
// digest only, and it must not carry a content digest.
func CheckComparison(c Comparison) error {
	if err := validateComparison(c); err != nil {
		return err
	}
	h, err := comparisonDigest(c)
	if err != nil {
		return err
	}
	if c.SHA256 != h {
		return errors.New("comparison digest mismatch")
	}
	if c.Schema == ComparisonSchema {
		content, err := comparisonContentDigest(c)
		if err != nil {
			return err
		}
		if c.ContentSHA256 != content {
			return errors.New("comparison content digest mismatch")
		}
	}
	return nil
}

// comparisonContent is the fixed-order semantic projection a v2 content
// digest is taken over: inputs, definitions, scope, the parser/engine
// versions and the report (options and results). Build provenance — the
// Tool identity — and both digest fields are excluded by construction.
type comparisonContent struct {
	Schema        string        `json:"schema"`
	ParserVersion string        `json:"parser_version"`
	EngineVersion string        `json:"engine_version"`
	Scope         string        `json:"scope"`
	Before        Input         `json:"before"`
	After         Input         `json:"after"`
	Conditions    *intent.Set   `json:"conditions"`
	Report        verify.Report `json:"report"`
}

func comparisonContentDigest(c Comparison) (string, error) {
	p := comparisonContent{Schema: c.Schema, ParserVersion: c.ParserVersion, EngineVersion: c.EngineVersion, Scope: c.Scope, Before: c.Before, After: c.After, Conditions: c.Conditions, Report: c.Report}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	if len(b) > MaxComparisonBytes {
		return "", errors.New("comparison exceeds output limit")
	}
	return ConfigDigest(b), nil
}

// validatePartial checks the structural invariants of a partial result
// (P1-2): it may only ride on a budget-stopped, incomplete search whose
// stop reason it matches; its decided pairs are sorted, unique and carry
// at least one proven fragment each; its undecided parts only bound
// regions without characterizing them. Soundness of the decided content
// itself is enforced by replay, which recomputes the report.
func validatePartial(r verify.Report) error {
	p := r.Partial
	if r.SearchComplete || r.Reason == "" || r.Reason != r.Budget.StopReason {
		return errors.New("partial result requires a stopped incomplete search")
	}
	switch p.Stage {
	case verify.PartialStagePrefix, verify.PartialStagePermitBefore, verify.PartialStagePermitAfter,
		verify.PartialStageDecideLost, verify.PartialStageDecideGained, verify.PartialStageWitnesses,
		verify.PartialStageScopeIndex, verify.PartialStageClasses:
	default:
		return errors.New("unknown partial stage")
	}
	if p.PrefixPairs < 0 || (p.BeforeRule != nil && *p.BeforeRule < 0) || (p.AfterRule != nil && *p.AfterRule < 0) {
		return errors.New("invalid partial progress cursor")
	}
	if len(p.DecidedPairs) > verify.MaxPartialDecidedPairs || len(p.UndecidedPairs) > verify.MaxPartialUndecidedPairs {
		return errors.New("partial pair listing exceeds its cap")
	}
	// The engine raises a truncated flag only when the listing actually
	// hit its cap, and a stopped delta always leaves an undecided
	// remainder; only the witnesses stage (delta fully decided) may have
	// an empty remainder.
	if (p.DecidedPairsTruncated && len(p.DecidedPairs) != verify.MaxPartialDecidedPairs) ||
		(p.UndecidedPairsTruncated && len(p.UndecidedPairs) != verify.MaxPartialUndecidedPairs) {
		return errors.New("partial pair truncated flag contradicts its listing")
	}
	if len(p.Undecided) == 0 && p.Stage != verify.PartialStageWitnesses {
		return errors.New("partial result names no undecided remainder")
	}
	for _, pairs := range [][]verify.RulePair{p.DecidedPairs, p.UndecidedPairs} {
		for i, pair := range pairs {
			if pair.Before < -1 || pair.After < -1 || pair.Pieces < 1 {
				return errors.New("invalid partial rule pair")
			}
			if i > 0 {
				prev := pairs[i-1]
				if pairOrder(prev.Before) > pairOrder(pair.Before) || pairOrder(prev.Before) == pairOrder(pair.Before) && pairOrder(prev.After) >= pairOrder(pair.After) {
					return errors.New("partial rule pairs must be sorted and unique")
				}
			}
		}
	}
	for _, part := range p.Undecided {
		switch part.Direction {
		case "lost", "gained", "delta", "classes":
		default:
			return errors.New("unknown partial undecided direction")
		}
		for _, rg := range []*[2]int{part.BeforeRules, part.AfterRules} {
			if rg != nil && (rg[0] < 0 || rg[1] < rg[0]) {
				return errors.New("invalid partial undecided rule range")
			}
		}
		if part.WorkBoxes < 0 {
			return errors.New("invalid partial undecided work count")
		}
	}
	return nil
}

// pairOrder sorts the implicit deny (-1) after every explicit ordinal,
// matching the change-class pair order.
func pairOrder(n int) int {
	if n < 0 {
		return 1 << 30
	}
	return n
}

func comparisonDigest(c Comparison) (string, error) {
	c.SHA256 = ""
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	if len(b) > MaxComparisonBytes {
		return "", errors.New("comparison exceeds output limit")
	}
	return ConfigDigest(b), nil
}

func validateComparison(c Comparison) error {
	if c.Schema != ComparisonSchema && c.Schema != ComparisonSchemaV1 || c.ParserVersion != ciscoacl.ParserVersion || !verify.SupportedEngine(c.EngineVersion) || c.Scope != intent.Scope || c.Tool.Version != ToolVersion || c.Tool.Commit == "" {
		return errors.New("unsupported comparison schema or version")
	}
	if c.Schema == ComparisonSchemaV1 && c.ContentSHA256 != "" {
		return errors.New("v1 comparison must not carry a content digest")
	}
	for _, in := range []Input{c.Before, c.After} {
		h, err := hex.DecodeString(in.SHA256)
		if err != nil || len(h) != 32 || hex.EncodeToString(h) != in.SHA256 || in.Name == "" || in.Kind != "named-extended" {
			return errors.New("invalid comparison input identity")
		}
	}
	if c.Before.Name != c.After.Name || c.Before.Kind != c.After.Kind {
		return errors.New("comparison identity mismatch")
	}
	r := c.Report
	limits, defaults := r.Options.Limits, packetset.Defaults()
	if r.Options.MaxWitnesses < 1 || r.Options.MaxWitnesses > 64 || len(r.Witnesses) > r.Options.MaxWitnesses || limits.MaxOperations < 1 || limits.MaxOperations > defaults.MaxOperations || limits.MaxBoxes < 1 || limits.MaxBoxes > defaults.MaxBoxes || limits.Timeout <= 0 || limits.Timeout > defaults.Timeout {
		return errors.New("invalid comparison budget options")
	}
	if r.Budget.Operations < 0 || r.Budget.Operations > limits.MaxOperations || r.Budget.PeakBoxes < 0 || r.Budget.PeakBoxes > limits.MaxBoxes {
		return errors.New("invalid comparison budget statistics")
	}
	switch r.Budget.StopReason {
	case "", "operations", "boxes":
		if r.Budget.TimingLimited {
			return errors.New("inconsistent timing classification")
		}
	case "wall_clock", "context_deadline":
		if !r.Budget.TimingLimited {
			return errors.New("missing timing classification")
		}
	default:
		return errors.New("unknown comparison stop reason")
	}
	if !r.SearchComplete && r.Budget.StopReason == "" {
		return errors.New("incomplete search requires stop reason")
	}
	switch r.Verdict {
	case "EQUIVALENT":
		if !r.SearchComplete || len(r.Witnesses) != 0 {
			return errors.New("invalid equivalence result")
		}
	case "DIFFERENT":
		if len(r.Witnesses) == 0 {
			return errors.New("difference requires witness")
		}
	case "INCONCLUSIVE":
		if r.SearchComplete || len(r.Witnesses) != 0 {
			return errors.New("invalid inconclusive result")
		}
	default:
		return errors.New("invalid comparison verdict")
	}
	if r.Partial != nil {
		if err := validatePartial(r); err != nil {
			return err
		}
	}
	var conditions []intent.Obligation
	if c.Conditions != nil {
		n, err := intent.Normalize(*c.Conditions)
		if err != nil {
			return err
		}
		a, err := json.Marshal(n)
		if err != nil {
			return err
		}
		b, err := json.Marshal(c.Conditions)
		if err != nil {
			return err
		}
		if string(a) != string(b) {
			return errors.New("condition definitions must be normalized")
		}
		conditions = n.Obligations
	}
	if len(r.Obligations) != len(conditions) {
		return errors.New("obligation results do not cover definitions")
	}
	for i, result := range r.Obligations {
		if result.ID != conditions[i].ID {
			return errors.New("obligation result identity mismatch")
		}
		switch result.Status {
		case "HOLDS", "INCONCLUSIVE":
			if result.Witness != nil {
				return errors.New("unexpected obligation witness")
			}
		case "VIOLATED":
			if result.Witness == nil {
				return errors.New("violation requires independent witness")
			}
		default:
			return errors.New("invalid obligation outcome")
		}
	}
	return nil
}

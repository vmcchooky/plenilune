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

const ComparisonSchema = "plenilune.evidence/v1"
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

// Comparison binds definitions, scope and a verifier-owned report. The digest
// is a content receipt, not a signature or a certification of intent fidelity.
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
	SHA256        string        `json:"sha256,omitempty"`
}

// SealComparison stamps the current engine unless the caller names the
// supported engine that actually produced the report.
func SealComparison(c Comparison) (Comparison, error) {
	if c.EngineVersion == "" {
		c.EngineVersion = verify.EngineVersion
	}
	c.Schema, c.ParserVersion, c.Scope = ComparisonSchema, ciscoacl.ParserVersion, intent.Scope
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
	h, err := comparisonDigest(c)
	if err != nil {
		return Comparison{}, err
	}
	c.SHA256 = h
	return c, nil
}

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
	return nil
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
	if c.Schema != ComparisonSchema || c.ParserVersion != ciscoacl.ParserVersion || !verify.SupportedEngine(c.EngineVersion) || c.Scope != intent.Scope || c.Tool.Version != ToolVersion || c.Tool.Commit == "" {
		return errors.New("unsupported comparison schema or version")
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

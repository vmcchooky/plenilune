package agent

import "plenilune/internal/jsonio"

func (r *RuleRef) UnmarshalJSON(data []byte) error {
	var in struct {
		BaseAfterSHA256 *string `json:"base_after_sha256"`
		Ordinal         *int    `json:"ordinal"`
		Line            *int    `json:"line"`
	}
	if err := jsonio.Decode(data, MaxReferenceBytes, &in); err != nil {
		return err
	}
	if in.BaseAfterSHA256 == nil || in.Ordinal == nil || in.Line == nil {
		return jsonio.Invalid("rule reference requires base digest, ordinal and line")
	}
	*r = RuleRef{BaseAfterSHA256: *in.BaseAfterSHA256, Ordinal: *in.Ordinal, Line: *in.Line}
	return nil
}

const MaxReferenceBytes = 4096

// Package patch applies bounded edits to an immutable ACL source in memory.
// Applying a patch does not verify its obligations or collateral effects.
package patch

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
)

const Schema = "plenilune.patch/v1"
const MaxEdits = 5
const MaxContentBytes = 2048

type RuleRef struct {
	BaseAfterSHA256 string `json:"base_after_sha256"`
	Ordinal         int    `json:"ordinal"`
	Line            int    `json:"line"`
}

// UnmarshalJSON distinguishes the valid first ordinal (0) from an omitted or
// null ordinal. Every component of a source-qualified reference is required.
func (r *RuleRef) UnmarshalJSON(data []byte) error {
	var in struct {
		BaseAfterSHA256 *string `json:"base_after_sha256"`
		Ordinal         *int    `json:"ordinal"`
		Line            *int    `json:"line"`
	}
	if err := jsonio.Decode(data, 4096, &in); err != nil {
		return err
	}
	if in.BaseAfterSHA256 == nil || in.Ordinal == nil || in.Line == nil {
		return jsonio.Invalid("rule reference requires base digest, ordinal and line")
	}
	*r = RuleRef{BaseAfterSHA256: *in.BaseAfterSHA256, Ordinal: *in.Ordinal, Line: *in.Line}
	return nil
}

type Edit struct {
	Op      string   `json:"op"`
	Ref     *RuleRef `json:"ref,omitempty"`
	Content string   `json:"content,omitempty"`
}

type Request struct {
	Schema            string `json:"schema"`
	BaseBeforeSHA256  string `json:"base_before_sha256"`
	BaseAfterSHA256   string `json:"base_after_sha256"`
	ObligationsDigest string `json:"obligations_digest"`
	Scope             string `json:"scope"`
	Edits             []Edit `json:"edits"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string        { return e.Code + ": " + e.Message }
func fail(code, message string) error { return &Error{Code: code, Message: message} }

// Apply resolves every reference against baseAfter before rendering any edit.
// It returns no candidate on error and never modifies the supplied bytes.
func Apply(baseAfter []byte, req Request) ([]byte, error) {
	if req.Schema != Schema || req.Scope != intent.Scope {
		return nil, fail("PATCH_METADATA_INVALID", "unsupported patch schema or scope")
	}
	if !digest(req.BaseBeforeSHA256) || !digest(req.BaseAfterSHA256) || !digest(req.ObligationsDigest) {
		return nil, fail("PATCH_METADATA_INVALID", "patch requires canonical SHA-256 pins")
	}
	if len(baseAfter) > ciscoacl.MaxInputBytes {
		return nil, fail("PATCH_LIMIT", "base source exceeds size limit")
	}
	h := sha256.Sum256(baseAfter)
	if hex.EncodeToString(h[:]) != req.BaseAfterSHA256 {
		return nil, fail("PATCH_BASE_MISMATCH", "patch does not match base source")
	}
	if len(req.Edits) < 1 || len(req.Edits) > MaxEdits {
		return nil, fail("PATCH_EDITS_INVALID", "patch requires one to five edits")
	}
	base, err := ciscoacl.Parse(baseAfter)
	if err != nil {
		return nil, fail("PATCH_BASE_INVALID", "base source is not a supported ACL")
	}
	byLine := make(map[int]Edit, len(req.Edits))
	appends := make([]string, 0, len(req.Edits))
	for _, edit := range req.Edits {
		switch edit.Op {
		case "add_before", "replace", "append":
			if err := validateContent(edit.Content); err != nil {
				return nil, err
			}
		case "remove":
			if edit.Content != "" {
				return nil, fail("PATCH_CONTENT_INVALID", "remove must not contain replacement content")
			}
		default:
			return nil, fail("PATCH_EDITS_INVALID", "unsupported patch operation")
		}
		if edit.Op == "append" {
			if edit.Ref != nil {
				return nil, fail("PATCH_REF_INVALID", "append must not contain a rule reference")
			}
			appends = append(appends, strings.Trim(edit.Content, " "))
			continue
		}
		ref := edit.Ref
		if ref == nil || ref.BaseAfterSHA256 != req.BaseAfterSHA256 || ref.Ordinal < 0 || ref.Ordinal >= len(base.Rules) || base.Rules[ref.Ordinal].Line != ref.Line {
			return nil, fail("PATCH_REF_INVALID", "rule reference does not match immutable base")
		}
		if _, exists := byLine[ref.Line]; exists {
			return nil, fail("PATCH_REF_CONFLICT", "multiple edits target the same base rule")
		}
		byLine[ref.Line] = edit
	}
	lines, eol := splitLines(string(baseAfter))
	lastRule := base.Rules[len(base.Rules)-1].Line
	out := make([]sourceLine, 0, len(lines)+len(req.Edits))
	for i, line := range lines {
		if edit, exists := byLine[i+1]; exists {
			switch edit.Op {
			case "add_before":
				out = append(out, sourceLine{text: " " + strings.Trim(edit.Content, " "), eol: eol})
				out = append(out, line)
			case "replace":
				out = append(out, sourceLine{text: indent(line.text) + strings.Trim(edit.Content, " "), eol: line.eol})
			case "remove": // Deliberately omit only the referenced executable line.
			}
		} else {
			out = append(out, line)
		}
		if i+1 == lastRule && len(appends) > 0 {
			if len(out) > 0 && out[len(out)-1].eol == "" {
				out[len(out)-1].eol = eol
			}
			for j, content := range appends {
				ending := eol
				if i == len(lines)-1 && j == len(appends)-1 && line.eol == "" {
					ending = ""
				}
				out = append(out, sourceLine{text: " " + content, eol: ending})
			}
		}
	}
	if len(out) > ciscoacl.MaxLines {
		return nil, fail("PATCH_LIMIT", "candidate exceeds line limit")
	}
	renumber(out)
	var candidate strings.Builder
	for _, line := range out {
		if len(line.text) > ciscoacl.MaxLineBytes || candidate.Len()+len(line.text)+len(line.eol) > ciscoacl.MaxInputBytes {
			return nil, fail("PATCH_LIMIT", "candidate exceeds source limits")
		}
		candidate.WriteString(line.text)
		candidate.WriteString(line.eol)
	}
	data := []byte(candidate.String())
	after, err := ciscoacl.Parse(data)
	if err != nil || after.Name != base.Name || after.Kind != base.Kind {
		return nil, fail("PATCH_CANDIDATE_INVALID", "candidate is not a supported ACL with the same identity")
	}
	return data, nil
}

func digest(text string) bool {
	if len(text) != sha256.Size*2 {
		return false
	}
	b, err := hex.DecodeString(text)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == text
}

func validateContent(content string) error {
	if len(content) == 0 || len(content) > MaxContentBytes || !utf8.ValidString(content) {
		return fail("PATCH_CONTENT_INVALID", "rule content exceeds allowed encoding or size")
	}
	for _, r := range content {
		if unicode.IsControl(r) {
			return fail("PATCH_CONTENT_INVALID", "rule content must be one line without control characters")
		}
	}
	body := strings.Trim(content, " ")
	fields := strings.Fields(body)
	if len(fields) == 0 || !strings.EqualFold(fields[0], "permit") && !strings.EqualFold(fields[0], "deny") {
		return fail("PATCH_CONTENT_INVALID", "content must be an unsequenced permit or deny rule")
	}
	a, err := ciscoacl.Parse([]byte("ip access-list extended PATCH\n " + body + "\n"))
	if err != nil || len(a.Rules) != 1 {
		return fail("PATCH_CONTENT_INVALID", "content is outside the supported rule grammar")
	}
	return nil
}

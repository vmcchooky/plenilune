package prefixlist

import (
	"strings"
	"testing"
)

// Regression for the FuzzParsePrefixList crasher of 2026-11-02: the
// legacy parser accepted a line whose text carries a bare \r (Fields
// tokenizes past it) and returned a list that failed its own
// Validate, breaking the parse-success-implies-valid invariant the
// current parser gets from validating its result. The legacy parser
// now enforces the source-line rules on the way in.
func TestParseLinesLegacyRejectsMalformedSourceLines(t *testing.T) {
	good := SourceLine{Number: 1, Text: "ip prefix-list 0 deny 0.0.0.0/0"}
	cases := map[string][]SourceLine{
		"bare carriage return": {{Number: 1, Text: "ip prefix-list 0 denY\r0.0.0.0/0"}},
		"embedded newline":     {{Number: 1, Text: "ip prefix-list 0 deny 0.0.0.0/0\n"}},
		"unnumbered line":      {{Number: 0, Text: good.Text}},
		"duplicate numbers":    {good, good},
	}
	for name, lines := range cases {
		if _, err := ParseLinesVersion("0", lines, LegacyParserVersion); err == nil || !strings.Contains(err.Error(), "PREFIX_LIST_SOURCE") {
			t.Errorf("%s: legacy parse = %v, want PREFIX_LIST_SOURCE refusal", name, err)
		}
	}
	// The current parser refuses the crasher line as well (via its
	// end-of-parse Validate), so both versions agree it is unparseable.
	if _, err := ParseLinesVersion("0", cases["bare carriage return"], ParserVersion); err == nil || !strings.Contains(err.Error(), "PREFIX_LIST_SOURCE") {
		t.Errorf("current parse of crasher line = %v, want PREFIX_LIST_SOURCE refusal", err)
	}
	// The same line without the \r parses under both versions and
	// validates: the refusal is about the source line, not the entry.
	clean := []SourceLine{{Number: 1, Text: "ip prefix-list 0 deny 0.0.0.0/0"}}
	for _, version := range []string{ParserVersion, LegacyParserVersion} {
		l, err := ParseLinesVersion("0", clean, version)
		if err != nil {
			t.Fatalf("%s: clean line refused: %v", version, err)
		}
		if err := l.Validate(); err != nil {
			t.Fatalf("%s: clean parse fails Validate: %v", version, err)
		}
	}
}

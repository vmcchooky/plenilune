package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuleReferenceRequiresExplicitFirstOrdinal(t *testing.T) {
	h := strings.Repeat("a", 64)
	for _, s := range []string{`{"base_after_sha256":"` + h + `","line":3}`, `{"base_after_sha256":"` + h + `","ordinal":null,"line":3}`, `{"base_after_sha256":"` + h + `","ordinal":0,"Line":3}`} {
		var ref RuleRef
		if json.Unmarshal([]byte(s), &ref) == nil {
			t.Fatal("accepted missing/null/alias rule ref")
		}
	}
	var ref RuleRef
	if err := json.Unmarshal([]byte(`{"base_after_sha256":"`+h+`","ordinal":0,"line":3}`), &ref); err != nil || ref.Ordinal != 0 {
		t.Fatalf("rejected explicit first ordinal: %v", err)
	}
}

package cli

import (
	"encoding/json"
	"plenilune/internal/evidence"
	"strings"
	"testing"
)

func TestDiffCLIExitAndFullReplay(t *testing.T) {
	dir := t.TempDir()
	b := put(t, dir, "before.acl", "ip access-list extended DEMO\n permit tcp any any eq 22\n")
	a := put(t, dir, "after.acl", "ip access-list extended DEMO\n deny ip any any\n")
	code, body, errout := invoke("diff", "--before", b, "--after", a)
	if code != 1 || errout != "" {
		t.Fatalf("diff: %d %s", code, errout)
	}
	var c evidence.Comparison
	if err := json.Unmarshal([]byte(body), &c); err != nil || c.Report.Verdict != "DIFFERENT" {
		t.Fatalf("invalid full evidence: %v", err)
	}
	path := put(t, dir, "comparison.json", body)
	if code, out, errout := invoke("replay", "--before", b, "--after", a, "--evidence", path); code != 0 || !strings.Contains(out, "COMPARISON_REPLAYED") {
		t.Fatalf("replay: %d %s", code, errout)
	}
	if code, _, errout := invoke("diff", "--before", b, "--after", b); code != 0 {
		t.Fatalf("equivalent: %d %s", code, errout)
	}
	if code, out, errout := invoke("diff", "--before", b, "--after", a, "--max-operations", "1"); code != 2 || errout != "" || !strings.Contains(out, "INCONCLUSIVE") {
		t.Fatalf("budget: %d %s", code, errout)
	}
}

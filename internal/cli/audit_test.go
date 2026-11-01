package cli

import (
	"encoding/json"
	"testing"

	"plenilune/internal/audit"
	"plenilune/internal/verify"
)

const auditFixture = "ip access-list extended DEMO\n permit tcp any any eq 22\n deny tcp host 192.0.2.10 any eq 22\n deny ip any any\n"

func TestAuditCLI(t *testing.T) {
	dir := t.TempDir()
	cfg := put(t, dir, "config.acl", auditFixture)
	code, body, errout := invoke("audit", "--config", cfg)
	if code != 0 || errout != "" {
		t.Fatalf("audit: %d %s", code, errout)
	}
	var rec audit.Record
	if err := json.Unmarshal([]byte(body), &rec); err != nil {
		t.Fatal(err)
	}
	if err := audit.Check(rec); err != nil {
		t.Fatal(err)
	}
	if !rec.Report.Complete {
		t.Fatalf("record incomplete: %+v", rec.Report)
	}
	// The host deny is shadowed by the opening permit (unreachable);
	// the final deny is redundant in the model (implicit fallthrough).
	kinds := map[int]string{}
	for _, f := range rec.Report.Findings {
		kinds[f.Rule] = f.Kind
	}
	if kinds[1] != verify.AuditUnreachable || kinds[2] != verify.AuditRedundant {
		t.Fatalf("findings: %+v", rec.Report.Findings)
	}

	// Replay the record the CLI just printed.
	ev := put(t, dir, "audit.json", body)
	code, body, errout = invoke("audit-replay", "--config", cfg, "--evidence", ev)
	if code != 0 || errout != "" {
		t.Fatalf("audit-replay: %d %s", code, errout)
	}
	var res struct {
		Status string `json:"status"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != "AUDIT_REPLAYED" || res.SHA256 != rec.SHA256 {
		t.Fatalf("replay result: %+v", res)
	}
}

func TestAuditCLIIncompleteAndErrors(t *testing.T) {
	dir := t.TempDir()
	cfg := put(t, dir, "config.acl", auditFixture)
	code, body, _ := invoke("audit", "--config", cfg, "--max-operations", "5")
	if code != 2 {
		t.Fatalf("incomplete audit exit: %d", code)
	}
	var rec audit.Record
	if err := json.Unmarshal([]byte(body), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Report.Complete || rec.Report.Reason != "operations" {
		t.Fatalf("incomplete record: %+v", rec.Report)
	}
	if err := audit.Check(rec); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := invoke("audit", "--config", dir+"/missing.acl"); code != 2 {
		t.Fatalf("missing file exit: %d", code)
	}
	bad := put(t, dir, "bad.acl", "not an acl\n")
	if code, _, errout := invoke("audit", "--config", bad); code != 2 || errout == "" {
		t.Fatalf("bad acl: %d %q", code, errout)
	}
	if code, _, _ := invoke("audit"); code != 2 {
		t.Fatalf("missing flag exit: %d", code)
	}
}

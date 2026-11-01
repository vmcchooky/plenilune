package audit

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"plenilune/internal/verify"
)

const fixture = "ip access-list extended DEMO\n permit tcp host 192.0.2.10 any eq 22\n deny tcp 192.0.2.0 0.0.0.255 any eq 22\n permit tcp any any eq 22\n deny ip any any\n"

func TestAuditRecordSealCheckReplay(t *testing.T) {
	rec, err := Audit(context.Background(), []byte(fixture), verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Report.Complete || len(rec.Report.Findings) == 0 || len(rec.Report.Relations) == 0 {
		t.Fatalf("fixture record: %+v", rec.Report)
	}
	if err := Check(rec); err != nil {
		t.Fatal(err)
	}
	res, err := Replay(context.Background(), []byte(fixture), rec)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "AUDIT_REPLAYED" || res.SHA256 != rec.SHA256 {
		t.Fatalf("replay: %+v", res)
	}
	// Tampering with a finding breaks the seal.
	bad := rec
	bad.Report.Findings = append([]verify.AuditFinding{}, rec.Report.Findings...)
	if bad.Report.Findings[0].Kind == verify.AuditRedundant {
		bad.Report.Findings[0].Kind = verify.AuditUnreachable
	} else {
		bad.Report.Findings[0].Kind = verify.AuditRedundant
	}
	if err := Check(bad); err == nil {
		t.Fatal("tampered record passed Check")
	}
	if _, err := Replay(context.Background(), []byte(fixture), bad); err == nil {
		t.Fatal("tampered record replayed")
	}
	// The record is bound to the exact input bytes.
	other := strings.Replace(fixture, "eq 22", "eq 23", 1)
	if _, err := Replay(context.Background(), []byte(other), rec); err == nil {
		t.Fatal("record replayed against different input")
	}
}

func TestAuditRecordDeterministicAcrossRunsAndProcs(t *testing.T) {
	first, err := Audit(context.Background(), []byte(fixture), verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		next, err := Audit(context.Background(), []byte(fixture), verify.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if next.SHA256 != first.SHA256 {
			t.Fatal("audit record differs across runs")
		}
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	one, err := Audit(context.Background(), []byte(fixture), verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if one.SHA256 != first.SHA256 {
		t.Fatal("audit record differs under GOMAXPROCS=1")
	}
}

func TestAuditIncompleteRecordSealsAndReplays(t *testing.T) {
	opts := verify.DefaultOptions()
	opts.Limits.MaxOperations = 5
	rec, err := Audit(context.Background(), []byte(fixture), opts)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Report.Complete || rec.Report.Stage != verify.AuditStageEffective || rec.Report.Reason != "operations" {
		t.Fatalf("incomplete record: %+v", rec.Report)
	}
	if err := Check(rec); err != nil {
		t.Fatal(err)
	}
	res, err := Replay(context.Background(), []byte(fixture), rec)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "AUDIT_REPLAYED" {
		t.Fatalf("incomplete replay: %+v", res)
	}
}

func TestAuditRejectsBadInput(t *testing.T) {
	if _, err := Audit(context.Background(), []byte("not an acl\n"), verify.DefaultOptions()); err == nil {
		t.Fatal("garbage input accepted")
	}
	if _, err := Audit(context.Background(), []byte(fixture), verify.Options{}); err == nil {
		t.Fatal("zero options accepted")
	}
}

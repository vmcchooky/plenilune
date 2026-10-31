package cli

import (
	"encoding/json"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/patch"
	"plenilune/internal/repair"
	"strings"
	"testing"
)

func TestRepairCLIAndOfflineReplay(t *testing.T) {
	dir := t.TempDir()
	b := put(t, dir, "before.acl", "ip access-list extended DEMO\n permit tcp any any eq 22\n deny ip any any\n")
	after := "ip access-list extended DEMO\n deny tcp host 192.0.2.10 any eq 22\n permit tcp any any eq 22\n deny ip any any\n"
	a := put(t, dir, "after.acl", after)
	s := intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.10/32", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}}
	_, hash, err := intent.Digest(s)
	if err != nil {
		t.Fatal(err)
	}
	p := repair.Policy{Schema: repair.PolicySchema, Scope: intent.Scope, OperatorAccepted: true, Conditions: s, AllowedChanges: []intent.Region{}}
	pb, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	policyPath := put(t, dir, "policy.json", string(pb))
	q := patch.Request{Schema: patch.Schema, Scope: intent.Scope, BaseBeforeSHA256: evidence.ConfigDigest([]byte("ip access-list extended DEMO\n permit tcp any any eq 22\n deny ip any any\n")), BaseAfterSHA256: evidence.ConfigDigest([]byte(after)), ObligationsDigest: hash, Edits: []patch.Edit{{Op: "remove", Ref: &patch.RuleRef{BaseAfterSHA256: evidence.ConfigDigest([]byte(after)), Ordinal: 0, Line: 2}}}}
	qb, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	patchPath := put(t, dir, "patch.json", string(qb))
	code, body, errout := invoke("repair", "--before", b, "--after", a, "--policy", policyPath, "--patch", patchPath)
	if code != 0 || errout != "" {
		t.Fatalf("repair: %d %s", code, errout)
	}
	var proof repair.Result
	if err := json.Unmarshal([]byte(body), &proof); err != nil || proof.Status != "VERIFIED" {
		t.Fatal("missing verified receipt")
	}
	artifact := put(t, dir, "repair.json", body)
	if code, out, errout := invoke("repair-replay", "--before", b, "--after", a, "--evidence", artifact); code != 0 || !strings.Contains(out, "REPAIR_REPLAYED") {
		t.Fatalf("replay: %d %s", code, errout)
	}
	p.OperatorAccepted = false
	pb, err = json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	put(t, dir, "policy.json", string(pb))
	if code, out, _ := invoke("repair", "--before", b, "--after", a, "--policy", policyPath, "--patch", patchPath); code != 2 || !strings.Contains(out, "UNVERIFIED") {
		t.Fatal("unaccepted policy verified")
	}
}

func TestAssistOffAndMissingKeyNeverPretendLiveMeta(t *testing.T) {
	dir := t.TempDir()
	b := put(t, dir, "acl.cfg", "ip access-list extended DEMO\n deny ip any any\n")
	t.Setenv("META_MODEL_API_KEY", "test-not-used")
	t.Setenv("PLENILUNE_META_BASE_URL", "invalid-url")
	if code, out, errout := invoke("assist", "--before", b, "--after", b, "--ai", "off"); code != 0 || !strings.Contains(out, `"status": "disabled"`) {
		t.Fatalf("AI-off read config: %d %s", code, errout)
	}
	t.Setenv("META_MODEL_API_KEY", "")
	if code, out, errout := invoke("assist", "--before", b, "--after", b, "--intent", "Keep denied.", "--ai", "on"); code != 2 || !strings.Contains(out, "not_configured") {
		t.Fatalf("missing key: %d %s", code, errout)
	}
}

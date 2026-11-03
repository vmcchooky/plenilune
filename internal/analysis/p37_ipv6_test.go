package analysis

import (
	"context"
	"testing"
)

// P3-7 end-to-end: IPv6 ACL objects are analyzed by the current tool,
// sealed, and replayed; runs recorded without a v6 parser identity (the
// pre-P3-7 shape) keep IPv6 unsupported byte-for-byte.

const p37Before = `ipv6 access-list EDGE6
 permit tcp 2001:db8:1::/48 any eq 443
 deny ipv6 any any
`

const p37After = `ipv6 access-list EDGE6
 permit tcp 2001:db8:1::/48 any eq 443
 permit tcp 2001:db8:2::/48 any eq 22
 deny ipv6 any any
`

func TestP37IPv6ObjectAnalyzedAndReplayed(t *testing.T) {
	r := run(t, p37Before, p37After, nil)
	obj := object(t, r, DomainACL, "EDGE6")
	if obj.Kind != "ipv6" || obj.Status != "changed" || obj.ACL6 == nil {
		t.Fatalf("ipv6 object = %+v", obj)
	}
	if obj.ACL6.Verdict != "DIFFERENT" || !obj.ACL6.SearchComplete || len(obj.ACL6.Witnesses) == 0 {
		t.Fatalf("acl6 payload = %+v", obj.ACL6)
	}
	w := obj.ACL6.Witnesses[0]
	if w.Before.Action != "deny" || w.After.Action != "permit" {
		t.Fatalf("witness = %+v, want deny -> permit", w)
	}
	if r.Versions.ACL6Parser != "cisco-acl6/v0.1" || r.Versions.ACL6Engine != "acl6-exact/v0.1" {
		t.Fatalf("versions = %+v", r.Versions)
	}
	out, err := Replay(context.Background(), []byte(p37Before), []byte(p37After), r)
	if err != nil || out.Status != "ANALYSIS_REPLAYED" || out.SHA256 != r.SHA256 {
		t.Fatalf("replay: %+v %v", out, err)
	}
}

func TestP37IPv6EquivalentAndImplicitTail(t *testing.T) {
	before := "ipv6 access-list ND1\n permit icmp any any nd-ns\n permit icmp any any nd-na\n deny tcp any any eq 22\n"
	after := "ipv6 access-list ND1\n deny tcp any any eq 22\n"
	r := run(t, before, after, nil)
	obj := object(t, r, DomainACL, "ND1")
	if obj.Status != "equivalent" || obj.ACL6 == nil || obj.ACL6.Verdict != "EQUIVALENT" {
		t.Fatalf("nd object = %+v", obj)
	}
}

func TestP37PreP37VersionsKeepIPv6Unsupported(t *testing.T) {
	versions := currentVersions()
	versions.ACL6Parser, versions.ACL6Engine = "", ""
	r, err := analyzeVersion(context.Background(), Request{Schema: RequestSchema, Before: Source{Text: p37Before}, After: Source{Text: p37After}}, versions)
	if err != nil {
		t.Fatal(err)
	}
	obj := object(t, r, DomainACL, "EDGE6")
	if obj.Status != "unsupported" || obj.ACL6 != nil {
		t.Fatalf("pre-P3-7 shape changed: %+v", obj)
	}
}

package patch

import (
	"bytes"
	"net/netip"
	"testing"

	"plenilune/internal/ciscoacl"
)

func standardAction(t *testing.T, source []byte, ip string) string {
	t.Helper()
	acl := parse(t, source)
	decision, err := acl.Evaluate(ciscoacl.Packet{Protocol: 6, SrcIP: netip.MustParseAddr(ip), DstIP: netip.MustParseAddr("198.51.100.1"), DstPort: 22})
	if err != nil {
		t.Fatal(err)
	}
	return decision.Action
}

func TestStandardNamedEditsPreserveSequenceOrderAndPhysicalComments(t *testing.T) {
	base := []byte("ip access-list standard TEST\r\n 30 deny 192.0.2.0 0.0.0.255\r\n 10 permit 192.0.2.128 0.0.0.127\r\n 20 permit host 192.0.2.1\r\n 40 remark kept exactly\r\n!\r\n")
	for _, tc := range []struct {
		name       string
		edit       Edit
		low, extra string
	}{
		{"remove host", Edit{Op: "remove", Ref: ref(base, 0, 4)}, "deny", "deny"},
		{"replace host", Edit{Op: "replace", Ref: ref(base, 0, 4), Content: "deny host 192.0.2.1"}, "deny", "deny"},
		{"insert network", Edit{Op: "add_before", Ref: ref(base, 2, 2), Content: "permit 192.0.2.0 0.0.0.127"}, "permit", "deny"},
		{"append host", Edit{Op: "append", Content: "permit host 203.0.113.7"}, "deny", "permit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate, err := Apply(base, request(base, tc.edit))
			if err != nil {
				t.Fatal(err)
			}
			if standardAction(t, candidate, "192.0.2.200") != "permit" || standardAction(t, candidate, "192.0.2.2") != tc.low || standardAction(t, candidate, "203.0.113.7") != tc.extra {
				t.Fatalf("wrong network ordering after %s: %s", tc.name, candidate)
			}
			if !bytes.Contains(candidate, []byte("remark kept exactly\r\n!\r\n")) || bytes.Count(candidate, []byte("\r\n")) != bytes.Count(candidate, []byte("\n")) {
				t.Fatal("comments/endings changed")
			}
		})
	}
}

func TestStandardNumberedEditGrammarAndLastPhysicalRule(t *testing.T) {
	base := []byte("!\r\naccess-list 7 deny 192.0.2.0 0.0.0.255\r\naccess-list 7 permit host 192.0.2.1\r\n!\r\n")
	for _, tc := range []struct {
		name       string
		edit       Edit
		ip, action string
	}{
		{"append", Edit{Op: "append", Content: "permit host 203.0.113.7"}, "203.0.113.7", "permit"},
		{"replace", Edit{Op: "replace", Ref: ref(base, 0, 3), Content: "deny host 192.0.2.1"}, "192.0.2.1", "deny"},
		{"insert", Edit{Op: "add_before", Ref: ref(base, 1, 2), Content: "permit 192.0.2.0 0.0.0.127"}, "192.0.2.2", "permit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate, err := Apply(base, request(base, tc.edit))
			if err != nil {
				t.Fatal(err)
			}
			if standardAction(t, candidate, tc.ip) != tc.action {
				t.Fatalf("wrong numbered edit: %s", candidate)
			}
			if !bytes.HasPrefix(candidate, []byte("!\r\n")) || !bytes.HasSuffix(candidate, []byte("!\r\n")) {
				t.Fatal("comments moved")
			}
			if tc.name == "append" && !bytes.Contains(candidate, []byte("access-list 7 permit host 192.0.2.1\r\naccess-list 7 permit host 203.0.113.7\r\n!")) {
				t.Fatal("append used last evaluation rule instead of last physical line")
			}
		})
	}
}

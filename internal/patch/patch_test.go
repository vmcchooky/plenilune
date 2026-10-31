package patch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
)

func sha(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
func request(base []byte, edits ...Edit) Request {
	return Request{Schema: Schema, BaseBeforeSHA256: sha([]byte("before")), BaseAfterSHA256: sha(base), ObligationsDigest: sha([]byte("conditions")), Scope: intent.Scope, Edits: edits}
}
func ref(base []byte, ordinal, line int) *RuleRef {
	return &RuleRef{BaseAfterSHA256: sha(base), Ordinal: ordinal, Line: line}
}
func parse(t *testing.T, data []byte) ciscoacl.ACL {
	t.Helper()
	a, err := ciscoacl.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestFixtureRemoveDenyChangesPacketAndPreservesSource(t *testing.T) {
	base, err := os.ReadFile("../../testdata/acl/management.acl")
	if err != nil {
		t.Fatal(err)
	}
	original := bytes.Clone(base)
	before := parse(t, base)
	p := ciscoacl.Packet{Protocol: 6, SrcIP: netip.MustParseAddr("192.0.2.10"), DstIP: netip.MustParseAddr("198.51.100.10"), SrcPort: 40000, DstPort: 22}
	d, err := before.Evaluate(p)
	if err != nil || d.Action != "deny" {
		t.Fatalf("fixture before: %+v %v", d, err)
	}
	data, err := Apply(base, request(base, Edit{Op: "remove", Ref: ref(base, 0, before.Rules[0].Line)}))
	if err != nil {
		t.Fatal(err)
	}
	after := parse(t, data)
	d, err = after.Evaluate(p)
	if err != nil || d.Action != "permit" {
		t.Fatalf("candidate packet: %+v %v", d, err)
	}
	if !bytes.Equal(base, original) {
		t.Fatal("Apply mutated source bytes")
	}
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.Contains(data, []byte("10 remark synthetic documentation addresses only")) || !bytes.HasSuffix(normalized, []byte("!\n")) {
		t.Fatalf("remark/wrapper lost: %s", data)
	}
	if len(after.Rules) != len(before.Rules)-1 {
		t.Fatal("removed wrong executable rule count")
	}
}

func TestImmutableRefsAndAllOperations(t *testing.T) {
	base := []byte("ip access-list extended DEMO\n 7 remark before\n 10 deny tcp host 192.0.2.1 any eq 22\n remark between\n 20 deny udp any any eq 53\n 30 permit ip any any\n 40 remark trailing\n!\nend\n")
	data, err := Apply(base, request(base,
		Edit{Op: "remove", Ref: ref(base, 0, 3)},
		Edit{Op: "replace", Ref: ref(base, 1, 5), Content: "permit udp any any eq 53"},
		Edit{Op: "add_before", Ref: ref(base, 2, 6), Content: "deny tcp any any eq 443"},
		Edit{Op: "append", Content: "deny ip any any"},
	))
	if err != nil {
		t.Fatal(err)
	}
	want := "ip access-list extended DEMO\n 10 remark before\n remark between\n 20 permit udp any any eq 53\n 30 deny tcp any any eq 443\n 40 permit ip any any\n 50 deny ip any any\n 60 remark trailing\n!\nend\n"
	if string(data) != want {
		t.Fatalf("candidate source mismatch\ngot: %q\nwant:%q", data, want)
	}
	a := parse(t, data)
	if len(a.Rules) != 4 || a.Rules[0].Line != 4 {
		t.Fatal("candidate source mapping wrong")
	}
}

func TestLineEndingsAndSourceEnd(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		for _, suffix := range []string{"", "!", "end"} {
			for _, final := range []bool{false, true} {
				text := "ip access-list extended DEMO" + eol + "\tpermit tcp any any eq 22"
				if suffix != "" {
					text += eol + suffix
				}
				if final {
					text += eol
				}
				base := []byte(text)
				data, err := Apply(base, request(base, Edit{Op: "append", Content: "deny ip any any"}))
				if err != nil {
					t.Fatalf("eol%q suffix%q final%v: %v", eol, suffix, final, err)
				}
				if !bytes.Contains(data, []byte("\t10 permit tcp any any eq 22"+eol+" 20 deny ip any any")) {
					t.Fatalf("wrong insertion/indent: %q", data)
				}
				if eol == "\r\n" && strings.Contains(strings.ReplaceAll(string(data), "\r\n", ""), "\n") {
					t.Fatal("CRLF candidate contains introduced LF")
				}
				if bytes.HasSuffix(data, []byte(eol)) != final {
					t.Fatalf("final newline policy changed: %q", data)
				}
				if suffix != "" && !bytes.Contains(data, []byte(eol+suffix)) {
					t.Fatal("trailing wrapper lost")
				}
			}
		}
	}
}

func TestRejectInvalidPatchWithoutPartialCandidate(t *testing.T) {
	base := []byte("ip access-list extended DEMO\n deny tcp any any eq 22\n permit ip any any\n")
	valid := func() Request { return request(base, Edit{Op: "remove", Ref: ref(base, 0, 2)}) }
	cases := []func(*Request){
		func(r *Request) { r.Schema = "wrong" },
		func(r *Request) { r.Scope = "network-wide" },
		func(r *Request) { r.BaseBeforeSHA256 = "bad" },
		func(r *Request) { r.ObligationsDigest = strings.Repeat("A", 64) },
		func(r *Request) { r.BaseAfterSHA256 = sha([]byte("other")) },
		func(r *Request) { r.Edits = nil },
		func(r *Request) {
			for len(r.Edits) <= MaxEdits {
				r.Edits = append(r.Edits, Edit{Op: "append", Content: "permit ip any any"})
			}
		},
		func(r *Request) { r.Edits[0].Op = "move" },
		func(r *Request) { r.Edits[0].Ref = nil },
		func(r *Request) { r.Edits[0].Ref.Ordinal = -1 },
		func(r *Request) { r.Edits[0].Ref.Ordinal = 2 },
		func(r *Request) { r.Edits[0].Ref.Line = 3 },
		func(r *Request) { r.Edits[0].Ref.BaseAfterSHA256 = sha([]byte("other")) },
		func(r *Request) { r.Edits[0].Content = " " },
		func(r *Request) {
			r.Edits = append(r.Edits, Edit{Op: "add_before", Ref: ref(base, 0, 2), Content: "permit ip any any"})
		},
		func(r *Request) { r.Edits = []Edit{{Op: "append", Ref: ref(base, 0, 2), Content: "permit ip any any"}} },
	}
	for i, mutate := range cases {
		r := valid()
		mutate(&r)
		data, err := Apply(base, r)
		var domain *Error
		if err == nil || data != nil || !errors.As(err, &domain) {
			t.Fatalf("case %d accepted or returned partial data: %v", i, err)
		}
	}
}

func TestRejectUnsafeContentAndMalformedCandidate(t *testing.T) {
	base := []byte("ip access-list extended DEMO\n permit ip any any\n")
	for _, content := range []string{"", "remark SECRET", "10 permit ip any any", "ip access-list extended EVIL", "permit ip any any\n deny ip any any", "permit ip any any\r", "permit\ttcp any any eq 22", "permit tcp any any established", strings.Repeat("x", MaxContentBytes+1), string([]byte{0xff})} {
		data, err := Apply(base, request(base, Edit{Op: "replace", Ref: ref(base, 0, 2), Content: content}))
		if err == nil || data != nil {
			t.Fatalf("accepted unsupported content %q", content)
		}
		if strings.Contains(err.Error(), "SECRET") {
			t.Fatal("content leaked in error")
		}
	}
	if data, err := Apply(base, request(base, Edit{Op: "remove", Ref: ref(base, 0, 2)})); err == nil || data != nil {
		t.Fatal("accepted empty resulting ACL")
	}
}

func TestStaleBaseAndDeterministicCandidate(t *testing.T) {
	base := []byte("ip access-list extended DEMO\n permit ip any any\n")
	r := request(base, Edit{Op: "replace", Ref: ref(base, 0, 2), Content: "deny ip any any"})
	a, err := Apply(base, r)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Apply(base, r)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("same patch did not render deterministic source")
	}
	if data, err := Apply(a, r); err == nil || data != nil {
		t.Fatal("stale base replay accepted")
	}
}

func TestCandidateLimits(t *testing.T) {
	base := []byte("ip access-list extended DEMO\n" + strings.Repeat(" permit ip any any\n", ciscoacl.MaxRules))
	if data, err := Apply(base, request(base, Edit{Op: "append", Content: "deny ip any any"})); err == nil || data != nil {
		t.Fatal("candidate exceeded rule limit")
	}
	base = []byte("ip access-list extended DEMO\n permit ip any any\n" + strings.Repeat("\n", ciscoacl.MaxLines-2))
	if data, err := Apply(base, request(base, Edit{Op: "append", Content: "deny ip any any"})); err == nil || data != nil {
		t.Fatal("candidate exceeded line limit")
	}
}

func TestRuleRefJSONRequiresCompleteTuple(t *testing.T) {
	hash := sha([]byte("base"))
	for _, body := range []string{
		`{"base_after_sha256":"` + hash + `","line":2}`,
		`{"base_after_sha256":"` + hash + `","ordinal":null,"line":2}`,
		`{"base_after_sha256":"` + hash + `","ordinal":0}`,
		`{"base_after_sha256":"` + hash + `","ordinal":0,"line":null}`,
		`{"ordinal":0,"line":2}`,
		`{"base_after_sha256":null,"ordinal":0,"line":2}`,
		`{"base_after_sha256":"` + hash + `","ordinal":0,"Ordinal":1,"line":2}`,
	} {
		var r RuleRef
		if err := json.Unmarshal([]byte(body), &r); err == nil {
			t.Fatalf("incomplete/ambiguous reference accepted: %s", body)
		}
	}
	var r RuleRef
	if err := json.Unmarshal([]byte(`{"base_after_sha256":"`+hash+`","ordinal":0,"line":2}`), &r); err != nil || r.Ordinal != 0 || r.Line != 2 {
		t.Fatalf("valid first rule rejected: %+v %v", r, err)
	}
}

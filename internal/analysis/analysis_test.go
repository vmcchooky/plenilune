package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/repair"
)

func load(t *testing.T, name string) (string, string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", name, "before.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", name, "after.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b), string(a)
}

func run(t *testing.T, before, after string, mutate func(*Request)) Result {
	t.Helper()
	req := Request{Schema: RequestSchema, Before: Source{Text: before}, After: Source{Text: after}}
	if mutate != nil {
		mutate(&req)
	}
	r, err := Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	return r
}

func object(t *testing.T, r Result, domain, name string) Object {
	t.Helper()
	for _, o := range r.Objects {
		if o.Domain == domain && o.Name == name {
			return o
		}
	}
	t.Fatalf("object %s/%s missing", domain, name)
	return Object{}
}

func TestRefactorChangesTextButNotBehavior(t *testing.T) {
	b, a := load(t, "refactor")
	r := run(t, b, a, nil)
	acl := object(t, r, DomainACL, "MGMT")
	if acl.Status != "equivalent" || acl.ACL.Verdict != "EQUIVALENT" || !acl.ACL.Changes.Complete || len(acl.ACL.Changes.Classes) != 0 {
		t.Fatalf("acl %+v", acl)
	}
	pl := object(t, r, DomainPrefixList, "CUSTOMER")
	if pl.Status != "equivalent" || pl.PrefixList.Report.Verdict != "EQUIVALENT" {
		t.Fatalf("prefix-list %+v", pl)
	}
	if len(r.UnanalyzedChanges) != 0 || r.Summary.Equivalent != 2 {
		t.Fatalf("summary %+v changes %+v", r.Summary, r.UnanalyzedChanges)
	}
	if len(acl.References["before"]) != 1 || acl.References["before"][0].Text != "ip access-group MGMT in" || len(pl.References["after"]) != 1 {
		t.Fatalf("references %+v %+v", acl.References, pl.References)
	}
	// Engine input is the exact block, mapped back to source lines.
	if acl.Before.Lines[0] != [2]int{9, 14} || *acl.Before.ByteStart <= 0 || b[*acl.Before.ByteStart:*acl.Before.ByteEnd] != strings.Join(strings.Split(b, "\n")[8:14], "\n")+"\n" {
		t.Fatalf("extent %+v", acl.Before)
	}
}

func TestIntendedChangeInsideDeclaredScope(t *testing.T) {
	b, a := load(t, "intended")
	plain := run(t, b, a, nil)
	acl := object(t, plain, DomainACL, "MGMT")
	if acl.Status != "changed" || len(acl.ACL.Changes.Classes) != 1 {
		t.Fatalf("acl %+v", acl.ACL)
	}
	c := acl.ACL.Changes.Classes[0]
	if c.Effect != "newly_permitted" || c.Scope != "undeclared" || c.Witness.Packet.Protocol != 17 || c.Witness.Packet.DstPort != 161 || c.Witness.Packet.SrcIP.String() != "192.0.2.50" || *c.Before.Line != 6 || *c.After.Line != 6 {
		t.Fatalf("class %+v", c)
	}
	policy := repair.Policy{
		Schema: repair.PolicySchema, Scope: intent.Scope, OperatorAccepted: true,
		Conditions:     intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-1", Kind: "must_permit", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}}}},
		AllowedChanges: []intent.Region{{Protocol: "udp", SrcCIDR: "192.0.2.50/32", DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 161, Hi: 161}}}},
	}
	scoped := run(t, b, a, func(r *Request) {
		r.Select = []ObjectRef{{Domain: DomainACL, Name: "MGMT"}}
		r.Policies = []ObjectPolicy{{Domain: DomainACL, Name: "MGMT", Policy: policy}}
	})
	sc := object(t, scoped, DomainACL, "MGMT").ACL
	if sc.PolicySHA256 == "" || !sc.Changes.ScopeDeclared || sc.Changes.Classes[0].Scope != "inside_allowed" {
		t.Fatalf("scoped %+v", sc)
	}
	if pl := object(t, scoped, DomainPrefixList, "CUSTOMER"); pl.Status != "unchanged" {
		t.Fatalf("unselected unchanged list %+v", pl)
	}
	policy.OperatorAccepted = false
	_, err := Analyze(context.Background(), Request{Schema: RequestSchema, Before: Source{Text: b}, After: Source{Text: a}, Policies: []ObjectPolicy{{Domain: DomainACL, Name: "MGMT", Policy: policy}}})
	var e *Error
	if !errors.As(err, &e) || e.Code != "POLICY_NOT_ACCEPTED" {
		t.Fatalf("unaccepted policy: %v", err)
	}
}

func TestRegressionLostAndUnintendedAccess(t *testing.T) {
	b, a := load(t, "regression")
	acl := object(t, run(t, b, a, nil), DomainACL, "MGMT").ACL
	if acl.Verdict != "DIFFERENT" || len(acl.Changes.Classes) != 2 {
		t.Fatalf("acl %+v", acl)
	}
	lost, opened := acl.Changes.Classes[0], acl.Changes.Classes[1]
	if lost.Effect != "newly_denied" || lost.Witness.Packet.SrcIP.String() != "192.0.2.10" || lost.Witness.Packet.DstPort != 22 || *lost.After.Ordinal != 0 {
		t.Fatalf("lost %+v", lost)
	}
	if opened.Effect != "newly_permitted" || opened.Witness.Packet.DstPort != 3389 || opened.Examples[0].Src != "any" || opened.Examples[0].DstPorts != "3389" {
		t.Fatalf("opened %+v", opened)
	}
}

func TestRoutingPrefixListChangeIsExact(t *testing.T) {
	b, a := load(t, "routing")
	pl := object(t, run(t, b, a, nil), DomainPrefixList, "CUSTOMER")
	rep := pl.PrefixList.Report
	if pl.Status != "changed" || len(rep.Classes) != 1 {
		t.Fatalf("prefix-list %+v", rep)
	}
	c := rep.Classes[0]
	if c.Effect != "newly_permitted" || c.Routes != 3 || c.Witness.Route.String() != "203.0.113.0/25" || *c.Before.Seq != 10 || *c.After.Seq != 5 {
		t.Fatalf("class %+v", c)
	}
	if pl.After.Lines[0] != [2]int{16, 18} {
		t.Fatalf("lines %+v", pl.After.Lines)
	}
}

func TestUnsupportedAndUnanalyzedAreExplicit(t *testing.T) {
	b, a := load(t, "unsupported")
	r := run(t, b, a, nil)
	acl := object(t, r, DomainACL, "MGMT")
	if acl.Status != "unsupported" || acl.ACL != nil || len(acl.Errors) != 1 || acl.Errors[0].Side != "after" || acl.Errors[0].Line != 15 {
		t.Fatalf("acl %+v", acl)
	}
	std := object(t, r, DomainACL, "NTP-PEERS")
	if std.Status != "unsupported" || std.Before != nil || std.Errors[0].Code != "UNSUPPORTED_KIND" {
		t.Fatalf("standard %+v", std)
	}
	v6 := object(t, r, DomainPrefixList, "V6-CUSTOMER")
	if v6.Kind != "ipv6" || v6.Status != "unsupported" {
		t.Fatalf("v6 %+v", v6)
	}
	got := map[string]string{}
	for _, c := range r.UnanalyzedChanges {
		got[c.Header] = c.Change
	}
	if got["object-group network WEB"] != "added" || got["interface GigabitEthernet0/1"] != "modified" || len(got) != 2 {
		t.Fatalf("unanalyzed %+v", r.UnanalyzedChanges)
	}
	limits := strings.Join(r.Limitations, ",")
	if !strings.Contains(limits, "unanalyzed_configuration_present") || !strings.Contains(limits, "unsupported_objects_present") {
		t.Fatalf("limitations %v", r.Limitations)
	}
	if r.Coverage["after"].UnanalyzedLines == 0 || r.Coverage["after"].UnsupportedLines == 0 {
		t.Fatalf("coverage %+v", r.Coverage["after"])
	}
}

func TestStandaloneACLKeepsWholeDocumentDigest(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "testdata", "acl", "management.acl"))
	if err != nil {
		t.Fatal(err)
	}
	r := run(t, string(doc), string(doc), nil)
	o := r.Objects[0]
	if len(r.Objects) != 1 || o.Status != "unchanged" || *o.Before.ByteStart != 0 || *o.Before.ByteEnd != len(doc) || o.Before.SHA256 != digest(doc) {
		t.Fatalf("standalone %+v", o)
	}
}

func TestCRLFBlockIsExactSourceBytes(t *testing.T) {
	b, a := load(t, "regression")
	b, a = strings.ReplaceAll(b, "\n", "\r\n"), strings.ReplaceAll(a, "\n", "\r\n")
	o := object(t, run(t, b, a, nil), DomainACL, "MGMT")
	block := a[*o.After.ByteStart:*o.After.ByteEnd]
	if !strings.HasPrefix(block, "ip access-list extended MGMT\r\n") || !strings.HasSuffix(block, "log\r\n") || o.After.SHA256 != digest([]byte(block)) || o.Status != "changed" {
		t.Fatalf("crlf block %q %+v", block, o.After)
	}
}

func TestDuplicateAndNumberedDefinitionsAreNotGuessed(t *testing.T) {
	cfg := "ip access-list extended A\n permit ip any any\nip access-list extended A\n deny ip any any\naccess-list 101 permit ip any any\naccess-list 101 deny ip any any\n"
	r := run(t, cfg, cfg, nil)
	a, n := object(t, r, DomainACL, "A"), object(t, r, DomainACL, "101")
	if a.Status != "unsupported" || a.Errors[0].Code != "DUPLICATE_DEFINITION" || a.Before.ByteStart != nil || a.Before.SHA256 == "" || n.Status != "unsupported" || n.Kind != "numbered" || n.Before.Lines[0] != [2]int{5, 6} {
		t.Fatalf("dup %+v numbered %+v", a, n)
	}
}

func TestReplayDetectsTamperingAndSourceChanges(t *testing.T) {
	b, a := load(t, "routing")
	r := run(t, b, a, nil)
	ok, err := Replay(context.Background(), []byte(b), []byte(a), r)
	if err != nil || ok.Status != "ANALYSIS_REPLAYED" || ok.SHA256 != r.SHA256 || ok.RecheckedWitnesses == 0 {
		t.Fatalf("replay %v %+v", err, ok)
	}
	var e *Error
	if _, err := Replay(context.Background(), []byte(b+"\n"), []byte(a), r); !errors.As(err, &e) || e.Code != "EVIDENCE_MISMATCH" {
		t.Fatalf("source change accepted: %v", err)
	}
	tampered := r
	tampered.Objects = append([]Object(nil), r.Objects...)
	tampered.Objects[0].Status = "equivalent"
	if _, err := Replay(context.Background(), []byte(b), []byte(a), tampered); !errors.As(err, &e) || e.Code != "EVIDENCE_MISMATCH" {
		t.Fatalf("tampered status accepted: %v", err)
	}
	resealed, _ := Seal(tampered)
	if _, err := Replay(context.Background(), []byte(b), []byte(a), resealed); !errors.As(err, &e) || e.Code != "EVIDENCE_MISMATCH" {
		t.Fatalf("resealed false result accepted: %v", err)
	}
	raw, _ := json.Marshal(r)
	var round Result
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if again, err := Replay(context.Background(), []byte(b), []byte(a), round); err != nil || again.Status != "ANALYSIS_REPLAYED" {
		t.Fatalf("round-trip replay %v %+v", err, again)
	}
}

func TestPrefixListTextChangeIsNotReportedUnchanged(t *testing.T) {
	before := "hostname R\n!\nip prefix-list P seq 5 permit 10.0.0.0/8 le 24\n"
	after := "hostname R\n!\nip prefix-list P seq 5  permit 10.0.0.0/8 le 24\n"
	r := run(t, before, after, nil)
	o := object(t, r, DomainPrefixList, "P")
	if o.Status != "equivalent" || o.PrefixList == nil || o.PrefixList.Report.Verdict != "EQUIVALENT" {
		t.Fatalf("text-only change status %+v", o)
	}
}

func TestInvalidACLLimitsAreRejectedBeforeAnyVerdict(t *testing.T) {
	before := "hostname R\n!\nip prefix-list P seq 5 permit 10.0.0.0/8\n"
	bad := DefaultOptions()
	bad.ACL.Limits = packetset.Limits{}
	_, err := Analyze(context.Background(), Request{Schema: RequestSchema, Before: Source{Text: before}, After: Source{Text: before}, Options: &bad})
	var e *Error
	if !errors.As(err, &e) || e.Code != "OPTIONS_INVALID" {
		t.Fatalf("invalid limits accepted: %v", err)
	}
}

func TestOversizedLineCountIsRejectedEarly(t *testing.T) {
	text := strings.Repeat("\n", MaxSourceLines+1)
	_, err := Analyze(context.Background(), Request{Schema: RequestSchema, Before: Source{Text: text}, After: Source{Text: "hostname R\n"}})
	var e *Error
	if !errors.As(err, &e) || e.Code != "INPUT_TOO_LARGE" {
		t.Fatalf("line limit: %v", err)
	}
}

func TestSelectMustNameAnInventoriedObject(t *testing.T) {
	b, a := load(t, "regression")
	_, err := Analyze(context.Background(), Request{Schema: RequestSchema, Before: Source{Text: b}, After: Source{Text: a}, Select: []ObjectRef{{Domain: DomainACL, Name: "NOPE"}}})
	var e *Error
	if !errors.As(err, &e) || e.Code != "SELECT_INVALID" {
		t.Fatalf("missing selection accepted: %v", err)
	}
}

func TestRequestValidationAndCancellation(t *testing.T) {
	b, a := load(t, "regression")
	var e *Error
	for name, mutate := range map[string]func(*Request){
		"schema":  func(r *Request) { r.Schema = "x" },
		"options": func(r *Request) { o := DefaultOptions(); o.MaxClasses = 0; r.Options = &o },
		"select":  func(r *Request) { r.Select = []ObjectRef{{Domain: "bgp", Name: "x"}} },
		"ref":     func(r *Request) { r.Before.Ref = "bad\nref" },
		"size":    func(r *Request) { r.After.Text = strings.Repeat("x", MaxSourceBytes+1) },
	} {
		req := Request{Schema: RequestSchema, Before: Source{Text: b}, After: Source{Text: a}}
		mutate(&req)
		if _, err := Analyze(context.Background(), req); !errors.As(err, &e) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Analyze(ctx, Request{Schema: RequestSchema, Before: Source{Text: b}, After: Source{Text: a}}); !errors.As(err, &e) || e.Code != "CANCELED" {
		t.Fatalf("cancel: %v", err)
	}
}

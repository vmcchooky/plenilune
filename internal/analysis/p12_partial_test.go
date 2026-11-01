package analysis

// P1-2 at the analysis surface: a budget-stopped ACL object keeps its
// partial explanation inside the sealed analysis artifact (object status
// stays "inconclusive" — the partial never upgrades it), the artifact
// replays, and an artifact produced with the v0.3 engine pinned carries
// no partial and still replays.

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"plenilune/internal/verify"
)

// p12CorpusACL mirrors the verify measurement corpus generator exactly
// (same PCG stream), so the 512-rule swap below is the plan's motivating
// case, not a lookalike.
func p12CorpusACL(n int) []string {
	r := rand.New(rand.NewPCG(uint64(n), uint64(n)^0x9e3779b97f4a7c15))
	services := []string{"eq 22", "eq 443", "eq 80", "range 8000 8099", "eq 53", "neq 23", "gt 1023", "eq 3389"}
	lines := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		action := "permit"
		if r.IntN(5) == 0 {
			action = "deny"
		}
		var src string
		switch r.IntN(4) {
		case 0:
			src = fmt.Sprintf("host 192.0.2.%d", r.IntN(254)+1)
		case 1:
			src = fmt.Sprintf("192.0.2.%d 0.0.0.15", r.IntN(16)*16)
		case 2:
			src = fmt.Sprintf("198.51.%d.0 0.0.0.255", r.IntN(64))
		default:
			src = "any"
		}
		dst := fmt.Sprintf("host 203.0.113.%d", r.IntN(254)+1)
		if r.IntN(3) == 0 {
			dst = fmt.Sprintf("203.0.113.%d 0.0.0.31", r.IntN(8)*32)
		}
		switch r.IntN(6) {
		case 0:
			lines = append(lines, fmt.Sprintf("%s icmp %s %s", action, src, dst))
		case 1:
			lines = append(lines, fmt.Sprintf("%s udp %s %s %s", action, src, dst, services[r.IntN(len(services))]))
		default:
			lines = append(lines, fmt.Sprintf("%s tcp %s %s %s", action, src, dst, services[r.IntN(len(services))]))
		}
	}
	return append(lines, "deny ip any any")
}

func p12Render(name string, rules []string) string {
	var b strings.Builder
	b.WriteString("ip access-list extended " + name + "\n")
	for i, rule := range rules {
		fmt.Fprintf(&b, " %d %s\n", (i+1)*10, rule)
	}
	b.WriteString("!\n")
	return b.String()
}

func p12Swap512() (string, string) {
	base := p12CorpusACL(512)
	swapped := append([]string(nil), base...)
	mid := len(swapped) / 2
	swapped[mid], swapped[mid+1] = swapped[mid+1], swapped[mid]
	return p12Render("CORPUS", base), p12Render("CORPUS", swapped)
}

func TestP12AnalysisCarriesPartial(t *testing.T) {
	before, after := p12Swap512()
	r := run(t, before, after, nil)
	acl := object(t, r, DomainACL, "CORPUS")
	if acl.Status != "inconclusive" || acl.ACL == nil {
		t.Fatalf("object status %q acl %+v", acl.Status, acl.ACL)
	}
	if acl.ACL.Verdict != "INCONCLUSIVE" || acl.ACL.SearchComplete {
		t.Fatalf("acl verdict %q complete %v", acl.ACL.Verdict, acl.ACL.SearchComplete)
	}
	p := acl.ACL.Partial
	if p == nil || p.Stage != verify.PartialStagePermitBefore || p.PrefixPairs != 256 {
		t.Fatalf("acl partial %+v", p)
	}
	if acl.ACL.Changes.Partial == nil || acl.ACL.Changes.Complete {
		t.Fatalf("changes partial %+v complete %v", acl.ACL.Changes.Partial, acl.ACL.Changes.Complete)
	}
	out, err := Replay(context.Background(), []byte(before), []byte(after), r)
	if err != nil || out.Status != "ANALYSIS_REPLAYED" || out.SHA256 != r.SHA256 {
		t.Fatalf("partial analysis replay: %+v %v", out, err)
	}
}

func TestP12AnalysisPinnedV03HasNoPartialAndReplays(t *testing.T) {
	before, after := p12Swap512()
	versions := currentVersions()
	versions.ACLEngine = verify.ResidualEngineVersion
	req := Request{Schema: RequestSchema, Before: Source{Text: before}, After: Source{Text: after}}
	r, err := analyzeVersion(context.Background(), req, versions)
	if err != nil {
		t.Fatal(err)
	}
	acl := object(t, r, DomainACL, "CORPUS")
	if acl.ACL == nil || acl.ACL.Partial != nil || acl.ACL.Changes.Partial != nil {
		t.Fatalf("v0.3-pinned analysis gained a partial: %+v", acl.ACL)
	}
	if acl.Status != "inconclusive" {
		t.Fatalf("v0.3-pinned status %q", acl.Status)
	}
	out, err := Replay(context.Background(), []byte(before), []byte(after), r)
	if err != nil || out.Status != "ANALYSIS_REPLAYED" || out.SHA256 != r.SHA256 {
		t.Fatalf("v0.3-pinned analysis replay: %+v %v", out, err)
	}
}

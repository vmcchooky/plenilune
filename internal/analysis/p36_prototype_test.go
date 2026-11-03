package analysis

// P3-6 PROTOTYPE — measurement only, not a product test.
//
// Plan 05, P3-6 (object-groups) requires a data-driven design decision:
// unify group membership in the IR (union) vs textual expansion. Today's
// parser accepts only expanded text, so this prototype measures what
// expansion costs in the CURRENT engine: a logical rule
// "permit tcp <srcGroup> <dstGroup> eq <port>" expands to
// |srcGroup| x |dstGroup| physical rules. The "after" text models the
// most common real edit — one host added to the source group of the
// first logical rule — so the pair is not trivially identical.
//
// Recorded per case: physical rule counts, Analyze wall time, object
// status, verdict, search completeness, or the intake error when the
// expanded form exceeds hard caps.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func p36Expand(nSrc, nDst, k int, extraSrcFirst bool) (string, int) {
	var b strings.Builder
	b.WriteString("ip access-list extended P36X\n")
	seq := 0
	rules := 0
	line := func(s string) {
		seq += 10
		rules++
		fmt.Fprintf(&b, " %d %s\n", seq, s)
	}
	for i := 0; i < k; i++ {
		ns := nSrc
		if i == 0 && extraSrcFirst {
			ns++
		}
		for s := 1; s <= ns; s++ {
			for d := 1; d <= nDst; d++ {
				line(fmt.Sprintf("permit tcp host 192.0.2.%d host 198.51.100.%d eq %d", s, d, 443+i))
			}
		}
	}
	line("deny ip any any")
	b.WriteString("!\n")
	return b.String(), rules
}

func TestP36PrototypeExpansionCost(t *testing.T) {
	cases := []struct{ nSrc, nDst, k int }{
		{4, 4, 5},   // 80 physical rules
		{8, 8, 5},   // 320
		{16, 16, 2}, // 512 = MaxRules, exactly at the cap
		{16, 16, 5}, // 1280: a modest real group ACL, over the cap
		{32, 8, 5},  // 1280: wide source group only
	}
	for _, c := range cases {
		before, nBefore := p36Expand(c.nSrc, c.nDst, c.k, false)
		after, nAfter := p36Expand(c.nSrc, c.nDst, c.k, true)
		req := Request{Schema: RequestSchema, Before: Source{Text: before}, After: Source{Text: after}}
		start := time.Now()
		r, err := Analyze(context.Background(), req)
		dur := time.Since(start)
		if err != nil {
			t.Logf("groups %dx%d x%d rules: before=%d after=%d -> ANALYZE ERROR after %v: %v",
				c.nSrc, c.nDst, c.k, nBefore, nAfter, dur.Round(time.Millisecond), err)
			continue
		}
		obj := object(t, r, DomainACL, "P36X")
		verdict, complete := "", false
		if obj.ACL != nil {
			verdict = obj.ACL.Verdict
			complete = obj.ACL.SearchComplete
		}
		t.Logf("groups %dx%d x%d rules: before=%d after=%d -> status=%s verdict=%s complete=%v in %v",
			c.nSrc, c.nDst, c.k, nBefore, nAfter, obj.Status, verdict, complete, dur.Round(time.Millisecond))
	}
}

package analysis

// P3-6 cross-checks: the object-group (union) form must agree with the
// expanded form wherever the expanded form is analyzable at all, and must
// stay analyzable at realistic group sizes where expansion exceeds
// MaxRules and comes back "unsupported" (see p36_prototype_test.go).

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// p36GroupForm renders the same logical ACL as p36Expand, but with
// object-groups: k rules total regardless of group sizes.
func p36GroupForm(nSrc, nDst, k int, extraSrc bool) string {
	var b strings.Builder
	b.WriteString("object-group network SRCS\n")
	n := nSrc
	if extraSrc {
		n++
	}
	for s := 1; s <= n; s++ {
		fmt.Fprintf(&b, " host 192.0.2.%d\n", s)
	}
	b.WriteString("object-group network DSTS\n")
	for d := 1; d <= nDst; d++ {
		fmt.Fprintf(&b, " host 198.51.100.%d\n", d)
	}
	b.WriteString("ip access-list extended P36X\n")
	for i := 0; i < k; i++ {
		fmt.Fprintf(&b, " %d permit tcp object-group SRCS object-group DSTS eq %d\n", (i+1)*10, 443+i)
	}
	fmt.Fprintf(&b, " %d deny ip any any\n!\n", (k+1)*10)
	return b.String()
}

func TestP36GroupFormMatchesExpansion(t *testing.T) {
	for _, c := range []struct{ nSrc, nDst, k int }{{4, 4, 5}, {8, 8, 5}} {
		eb, _ := p36Expand(c.nSrc, c.nDst, c.k, false)
		ea, _ := p36Expand(c.nSrc, c.nDst, c.k, true)
		er := run(t, eb, ea, nil)
		eo := object(t, er, DomainACL, "P36X")

		gb, ga := p36GroupForm(c.nSrc, c.nDst, c.k, false), p36GroupForm(c.nSrc, c.nDst, c.k, true)
		gr := run(t, gb, ga, nil)
		go_ := object(t, gr, DomainACL, "P36X")

		if eo.Status != go_.Status || eo.ACL == nil || go_.ACL == nil || eo.ACL.Verdict != go_.ACL.Verdict {
			t.Fatalf("groups %dx%d x%d: expanded status=%s verdict=%v, group status=%s verdict=%v",
				c.nSrc, c.nDst, c.k, eo.Status, eo.ACL, go_.Status, go_.ACL)
		}
		if go_.ACL.Verdict != "DIFFERENT" || !go_.ACL.SearchComplete {
			t.Fatalf("groups %dx%d x%d: group form verdict=%s complete=%v, want DIFFERENT/complete",
				c.nSrc, c.nDst, c.k, go_.ACL.Verdict, go_.ACL.SearchComplete)
		}
	}
}

func TestP36GroupFormBeyondExpansionWall(t *testing.T) {
	// The case expansion cannot represent at all (1281 physical rules ->
	// unsupported in the prototype): 5 logical rules with 16x16 groups.
	before := p36GroupForm(16, 16, 5, false)
	after := p36GroupForm(16, 16, 5, true)
	start := time.Now()
	r, err := Analyze(context.Background(), Request{Schema: RequestSchema, Before: Source{Text: before}, After: Source{Text: after}})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	obj := object(t, r, DomainACL, "P36X")
	if obj.ACL == nil || obj.ACL.Verdict != "DIFFERENT" || !obj.ACL.SearchComplete {
		t.Fatalf("group form 16x16 x5: status=%s acl=%+v", obj.Status, obj.ACL)
	}
	t.Logf("group form 16x16 x5 (5 logical rules): DIFFERENT complete in %v", time.Since(start).Round(time.Millisecond))
}

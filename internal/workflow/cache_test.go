package workflow

// P1-5 comparison cache: a hit must be the exact sealed artifact a
// fresh computation produces, and changing any single keyed input —
// before bytes, after bytes, conditions, options — must be a miss.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/verify"
)

const cacheACLBefore = "ip access-list extended EDGE\n 10 permit tcp 192.0.2.0 0.0.0.255 any eq 443\n 20 deny ip any any\n"
const cacheACLAfter = "ip access-list extended EDGE\n 10 permit tcp 192.0.2.0 0.0.0.255 any eq 8443\n 20 deny ip any any\n"

func cacheTool() evidence.Tool {
	return evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"}
}

func TestCompareCacheHitIsByteIdentical(t *testing.T) {
	ctx := context.Background()
	c := NewCompareCache(4)
	opts := verify.DefaultOptions()
	first, err := c.Compare(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), nil, opts, cacheTool())
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := Compare(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), nil, opts, cacheTool())
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Compare(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), nil, opts, cacheTool())
	if err != nil {
		t.Fatal(err)
	}
	fb, _ := json.Marshal(fresh)
	sb, _ := json.Marshal(second)
	if string(fb) != string(sb) || first.SHA256 != second.SHA256 {
		t.Fatal("cache hit differs from a fresh computation")
	}
	hits, misses, entries := c.Stats()
	if hits != 1 || misses != 1 || entries != 1 {
		t.Fatalf("stats hits=%d misses=%d entries=%d", hits, misses, entries)
	}
	// The cached artifact still replays independently of the cache.
	out, err := Replay(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), second)
	if err != nil || out.Status != "COMPARISON_REPLAYED" {
		t.Fatalf("cached artifact replay: %+v %v", out, err)
	}
}

func TestCompareCacheKeyCoversEveryInput(t *testing.T) {
	ctx := context.Background()
	c := NewCompareCache(16)
	opts := verify.DefaultOptions()
	conds := &intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{
		{ID: "OB-1", Kind: "must_deny", Match: intent.Region{Protocol: "tcp", SrcCIDR: "192.0.2.0/24", DstCIDR: "0.0.0.0/0", DstPorts: []intent.Interval{{Lo: 8443, Hi: 8443}}}},
	}}
	base := []struct {
		name string
		call func() (evidence.Comparison, error)
	}{
		{"baseline", func() (evidence.Comparison, error) {
			return c.Compare(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), conds, opts, cacheTool())
		}},
		{"before changed", func() (evidence.Comparison, error) {
			return c.Compare(ctx, []byte(strings.Replace(cacheACLBefore, "eq 443", "eq 444", 1)), []byte(cacheACLAfter), conds, opts, cacheTool())
		}},
		{"after changed", func() (evidence.Comparison, error) {
			return c.Compare(ctx, []byte(cacheACLBefore), []byte(strings.Replace(cacheACLAfter, "eq 8443", "eq 8444", 1)), conds, opts, cacheTool())
		}},
		{"conditions dropped", func() (evidence.Comparison, error) {
			return c.Compare(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), nil, opts, cacheTool())
		}},
		{"options changed", func() (evidence.Comparison, error) {
			o := opts
			o.MaxWitnesses = 3
			return c.Compare(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), conds, o, cacheTool())
		}},
	}
	seen := map[string]string{}
	for _, tc := range base {
		got, err := tc.call()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if prev, dup := seen[got.SHA256]; dup {
			t.Fatalf("%s produced the same artifact as %s — a keyed input was ignored", tc.name, prev)
		}
		seen[got.SHA256] = tc.name
	}
	hits, misses, _ := c.Stats()
	if hits != 0 || misses != len(base) {
		t.Fatalf("every mutated input must miss: hits=%d misses=%d", hits, misses)
	}
	// Repeating the baseline is a hit.
	baseline, err := base[0].call()
	if err != nil {
		t.Fatal(err)
	}
	hits, _, _ = c.Stats()
	if hits != 1 {
		t.Fatalf("baseline repeat: hits=%d", hits)
	}
	// Since P2-1 the key is the input-side content projection: changing
	// ONLY the tool commit — build provenance, excluded from content
	// identity — is also a hit and returns the artifact exactly as the
	// producing computation sealed it. (Before P2-1 the tool identity
	// was keyed and this call missed with a different artifact digest.)
	provenance, err := c.Compare(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), conds, opts, evidence.Tool{Version: evidence.ToolVersion, Commit: "other"})
	if err != nil {
		t.Fatal(err)
	}
	pb, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if string(pb) != string(bb) {
		t.Fatalf("commit-only change must return the cached artifact byte-identical:\n%s\n%s", pb, bb)
	}
	hits, misses, _ = c.Stats()
	if hits != 2 || misses != len(base) {
		t.Fatalf("commit-only change must hit: hits=%d misses=%d", hits, misses)
	}
}

func TestCompareCacheEvictsFIFOAndSkipsTiming(t *testing.T) {
	ctx := context.Background()
	c := NewCompareCache(2)
	opts := verify.DefaultOptions()
	mk := func(port string) (string, string) {
		return "ip access-list extended E" + port + "\n 10 permit tcp any any eq " + port + "\n",
			"ip access-list extended E" + port + "\n 10 deny tcp any any eq " + port + "\n"
	}
	for _, port := range []string{"80", "81", "82"} {
		b, a := mk(port)
		if _, err := c.Compare(ctx, []byte(b), []byte(a), nil, opts, cacheTool()); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, entries := c.Stats(); entries != 2 {
		t.Fatalf("entries=%d over capacity 2", entries)
	}
	// The oldest entry was evicted: it misses again.
	b, a := mk("80")
	before, _, _ := c.Stats()
	if _, err := c.Compare(ctx, []byte(b), []byte(a), nil, opts, cacheTool()); err != nil {
		t.Fatal(err)
	}
	hits, _, _ := c.Stats()
	if hits != before {
		t.Fatal("evicted entry still hits")
	}

	// A timing-limited result is never stored. Force the timing stop
	// with a workload that cannot finish inside the limit on any
	// machine: the 512-rule cacheHeavyPair comparison needs ~500k
	// operations (tens of milliseconds even on fast hardware) against
	// a 2 ms wall-clock limit, so the budget's wall_clock stop lands
	// mid-run at an enforcement point. The original version raced a
	// 1 ns limit against the tiny pair above: on a fast machine the
	// whole 76-operation comparison completed before the deadline was
	// observed (Windows gate for PR #19: failed 10/10,
	// TimingLimited=false). A pre-expired context is not a substitute:
	// by design it returns an error, not a timing-limited result.
	heavyBefore, heavyAfter := cacheHeavyPair()
	timing := verify.DefaultOptions()
	timing.Limits.Timeout = 2 * time.Millisecond
	c2 := NewCompareCache(4)
	for i := 0; i < 2; i++ {
		r, err := c2.Compare(ctx, []byte(heavyBefore), []byte(heavyAfter), nil, timing, cacheTool())
		if err != nil {
			t.Fatal(err)
		}
		if !r.Report.Budget.TimingLimited {
			t.Fatalf("expected a timing-limited result, got %+v", r.Report.Budget)
		}
	}
	hits, misses, entries := c2.Stats()
	if hits != 0 || misses != 2 || entries != 0 {
		t.Fatalf("timing-limited results must not be cached: hits=%d misses=%d entries=%d", hits, misses, entries)
	}
}

func TestCompareCacheConcurrent(t *testing.T) {
	ctx := context.Background()
	c := NewCompareCache(8)
	opts := verify.DefaultOptions()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if _, err := c.Compare(ctx, []byte(cacheACLBefore), []byte(cacheACLAfter), nil, opts, cacheTool()); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if _, _, entries := c.Stats(); entries != 1 {
		t.Fatalf("entries=%d", entries)
	}
}

// cacheHeavyPair builds a 512-rule ACL pair (one middle rule's port
// range shifted) whose exact comparison needs ~500k operations: the
// overlapping port ranges fragment the packet sets, so the comparison
// takes tens of milliseconds on any target machine. It exists so the
// timing segment of TestCompareCacheEvictsFIFOAndSkipsTiming can
// force a mid-run wall-clock stop without racing machine speed.
func cacheHeavyPair() (string, string) {
	mk := func(shift int) string {
		var b strings.Builder
		b.WriteString("ip access-list extended HEAVY\n")
		for i := 0; i < 512; i++ {
			action := "permit"
			if i%5 == 4 {
				action = "deny"
			}
			lo := i * 37 % 60000
			if i == 256 {
				lo += shift
			}
			fmt.Fprintf(&b, " %d %s tcp any any range %d %d\n", (i+1)*10, action, lo, lo+997)
		}
		return b.String()
	}
	return mk(0), mk(500)
}

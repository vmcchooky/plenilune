package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/verify"
)

// CompareCache memoizes sealed comparisons for one process (the demo
// server's session). It is an internal optimization only: a hit returns
// the exact sealed artifact a fresh Compare produced for the same key,
// evidence is still created once per distinct input and replays
// independently of the cache.
//
// The key is the input-side content projection (P2-1): it binds exactly
// the inputs that determine the comparison's content_sha256 — both
// configuration byte digests, the conditions as supplied, the full
// options, and the parser and engine versions. Changing any one of them
// is a miss by construction. Build provenance is NOT keyed: neither the
// tool commit (the provenance P2-1 removes from content identity) nor
// the tool version (a release label — the computation's semantics are
// pinned by the parser/engine versions, and the version is validated
// against the producing build at seal time). A hit therefore returns
// the artifact as the producing computation sealed it, tool provenance
// included; callers identify it by its content digest. Results whose
// search stopped on a timing limit are never stored: their content
// depends on machine load, so they are recomputed (and replay as
// TIMING_SENSITIVE) instead of being frozen into the cache.
type CompareCache struct {
	mu       sync.Mutex
	capacity int
	order    []string // FIFO, oldest first
	items    map[string]evidence.Comparison
	hits     int
	misses   int
}

// NewCompareCache returns a cache holding at most capacity results
// (FIFO eviction). A capacity <= 0 selects the default of 128.
func NewCompareCache(capacity int) *CompareCache {
	if capacity <= 0 {
		capacity = 128
	}
	return &CompareCache{capacity: capacity, items: map[string]evidence.Comparison{}}
}

type compareKey struct {
	BeforeSHA  string         `json:"before_sha256"`
	AfterSHA   string         `json:"after_sha256"`
	Conditions *intent.Set    `json:"conditions"`
	Options    verify.Options `json:"options"`
	Parser     string         `json:"parser"`
	Engine     string         `json:"engine"`
}

func cacheKey(before, after []byte, conditions *intent.Set, opts verify.Options, engine string) (string, error) {
	k := compareKey{
		BeforeSHA:  evidence.ConfigDigest(before),
		AfterSHA:   evidence.ConfigDigest(after),
		Conditions: conditions,
		Options:    opts,
		Parser:     ciscoacl.ParserVersion,
		Engine:     engine,
	}
	b, err := json.Marshal(k)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Compare returns the cached sealed comparison for the exact inputs, or
// computes, seals and (unless timing-limited) stores it.
func (c *CompareCache) Compare(ctx context.Context, before, after []byte, conditions *intent.Set, opts verify.Options, tool evidence.Tool) (evidence.Comparison, error) {
	key, err := cacheKey(before, after, conditions, opts, verify.EngineVersion)
	if err != nil {
		return Compare(ctx, before, after, conditions, opts, tool)
	}
	c.mu.Lock()
	if hit, ok := c.items[key]; ok {
		c.hits++
		c.mu.Unlock()
		return hit, nil
	}
	c.misses++
	c.mu.Unlock()

	fresh, err := Compare(ctx, before, after, conditions, opts, tool)
	if err != nil {
		return evidence.Comparison{}, err
	}
	if fresh.Report.Budget.TimingLimited {
		return fresh, nil
	}
	c.mu.Lock()
	if _, ok := c.items[key]; !ok {
		for len(c.order) >= c.capacity {
			delete(c.items, c.order[0])
			c.order = c.order[1:]
		}
		c.order = append(c.order, key)
		c.items[key] = fresh
	}
	c.mu.Unlock()
	return fresh, nil
}

// Stats reports cumulative hits, misses and the current entry count.
// A lookup that cannot be keyed falls back to Compare and is not
// counted.
func (c *CompareCache) Stats() (hits, misses, entries int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, len(c.items)
}

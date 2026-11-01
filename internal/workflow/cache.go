package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

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
// The key binds every input of the computation: both configuration byte
// digests, the conditions as supplied, the full options, the engine
// version and the tool identity. Changing any one of them is a miss by
// construction. Results whose search stopped on a timing limit are
// never stored: their content depends on machine load, so they are
// recomputed (and replay as TIMING_SENSITIVE) instead of being frozen
// into the cache.
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
	Engine     string         `json:"engine"`
	Tool       evidence.Tool  `json:"tool"`
}

func cacheKey(before, after []byte, conditions *intent.Set, opts verify.Options, tool evidence.Tool, engine string) (string, error) {
	k := compareKey{
		BeforeSHA:  evidence.ConfigDigest(before),
		AfterSHA:   evidence.ConfigDigest(after),
		Conditions: conditions,
		Options:    opts,
		Engine:     engine,
		Tool:       tool,
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
	key, err := cacheKey(before, after, conditions, opts, tool, verify.EngineVersion)
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

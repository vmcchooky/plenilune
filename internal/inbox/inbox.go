// Package inbox is the trigger engine and verdict inbox of the local
// demo server (P2-8): the event-driven entry of the fast flow. A user
// pins watched objects — an ACL or prefix-list by name — with trigger
// conditions; whenever an analysis or comparison completes in the
// server, the store evaluates the result for each watched object
// against the object's previously recorded outcome and records an
// inbox entry for every condition that fires. The inbox is therefore
// the list of results that need attention, each entry pointing at the
// sealed artifact that produced it (by content digest). Everything
// else — the deep review flow — stays one click away from the entry's
// artifact.
//
// Scope discipline (plan §P2-8 as scoped for this item): there is no
// daemon, scheduler or filesystem watcher here, and no new verdict
// semantics. Evaluation is synchronous at analysis/comparison
// completion inside the server process, and the store lives in server
// memory for the process lifetime — the same posture as the compare
// cache (P1-5). Watches, per-object outcome state and inbox entries
// reset when the server restarts; nothing is written to disk.
//
// Verdict discipline: an INCONCLUSIVE result (including a
// budget-stopped partial result, whose verdict token is INCONCLUSIVE)
// is recorded as the object's state like any other outcome, but it is
// never treated as a verdict change: the verdict_changed condition
// fires only between two decided verdicts (EQUIVALENT / DIFFERENT),
// and any_different fires only on a decided DIFFERENT. Budget
// exhaustion is a fact about the search, not about the configuration,
// so it must not page the user as a behavior change.
package inbox

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Trigger conditions a watch can carry. A watch fires an inbox entry
// per condition that matches; conditions are stored deduplicated in
// this canonical order.
const (
	ConditionVerdictChanged   = "verdict_changed"
	ConditionAnyDifferent     = "any_different"
	ConditionObligationFailed = "obligation_failed"
)

var conditionOrder = []string{ConditionVerdictChanged, ConditionAnyDifferent, ConditionObligationFailed}

// Artifact kinds an observation can come from.
const (
	ArtifactAnalysis   = "analysis"
	ArtifactComparison = "comparison"
)

// Object domains, matching the analysis artifact's domain tokens.
const (
	DomainACL        = "acl"
	DomainPrefixList = "prefix-list"
)

// Decided verdict tokens. INCONCLUSIVE is recorded as state but is
// never a decided verdict (see the package comment).
const (
	VerdictEquivalent   = "EQUIVALENT"
	VerdictDifferent    = "DIFFERENT"
	VerdictInconclusive = "INCONCLUSIVE"
)

// Store bounds: server memory is finite, so the inbox keeps at most
// MaxEntries entries (oldest dropped first) and outcome state for at
// most MaxTrackedObjects objects (least recently first-seen evicted).
// MaxWatches bounds the pinned set a single local user can configure.
const (
	MaxWatches        = 64
	MaxEntries        = 256
	MaxTrackedObjects = 512
	maxNameBytes      = 128 // matches the analysis request name bound
)

// Watch pins one object (by domain + name) to a set of trigger
// conditions. Domain is DomainACL unless the caller names the
// prefix-list domain explicitly.
type Watch struct {
	Domain     string   `json:"domain"`
	Name       string   `json:"name"`
	Conditions []string `json:"conditions"`
}

// Observation is one object's outcome from one completed analysis or
// comparison, fed to the store at completion time. ViolatedObligations
// is meaningful only when ObligationsEvaluated is true (the comparison
// flow evaluates obligations; the analysis flow does not, so an
// analysis observation must not clear a recorded violation set).
type Observation struct {
	Domain               string
	Name                 string
	Verdict              string // EQUIVALENT, DIFFERENT or INCONCLUSIVE
	ObligationsEvaluated bool
	ViolatedObligations  []string // obligation IDs with status VIOLATED
	ArtifactKind         string   // ArtifactAnalysis or ArtifactComparison
	ContentSHA256        string   // content digest of the producing artifact
	SHA256               string   // artifact (envelope) digest of the producing artifact
}

// Entry is one fired trigger: the inbox record. PreviousVerdict and
// Verdict bracket a verdict_changed firing; ViolatedObligations names
// the newly violated obligations of an obligation_failed firing.
// ContentSHA256 identifies the artifact to open. Stale is computed at
// read time: it is true when a newer outcome for the same object has
// been recorded since, so this entry no longer describes the object's
// latest artifact (the display-level stale mark of plan §P2-8; the
// full stale-decision invalidation is P3-4).
type Entry struct {
	ID                  int64     `json:"id"`
	Domain              string    `json:"domain"`
	Name                string    `json:"name"`
	Condition           string    `json:"condition"`
	PreviousVerdict     string    `json:"previous_verdict,omitempty"`
	Verdict             string    `json:"verdict,omitempty"`
	ViolatedObligations []string  `json:"violated_obligations,omitempty"`
	ArtifactKind        string    `json:"artifact_kind"`
	ContentSHA256       string    `json:"content_sha256"`
	SHA256              string    `json:"sha256,omitempty"`
	Time                time.Time `json:"time"`
	Read                bool      `json:"read"`
	Stale               bool      `json:"stale"`

	seq int64 // observation sequence that produced this entry
}

type objectKey struct {
	domain string
	name   string
}

// objectState is the last recorded outcome of one object: the baseline
// the next observation is judged against.
type objectState struct {
	verdict       string
	violated      map[string]bool
	contentSHA256 string
	seq           int64
}

// Store is the trigger engine + inbox state. The zero value is not
// usable; build one with NewStore. Safe for concurrent use.
type Store struct {
	mu       sync.Mutex
	watches  []Watch
	watchSet map[objectKey]Watch
	last     map[objectKey]objectState
	seen     []objectKey // first-seen order, for state eviction
	entries  []Entry     // append order (oldest first)
	nextID   int64
	seq      int64
	now      func() time.Time
}

// NewStore returns an empty store: no watches, no recorded outcomes,
// no entries.
func NewStore() *Store {
	return &Store{
		watchSet: map[objectKey]Watch{},
		last:     map[objectKey]objectState{},
		nextID:   1,
		now:      time.Now,
	}
}

// ValidateWatches normalizes a watch list (domain default, condition
// dedup in canonical order) and rejects anything the store cannot
// honor: too many watches, an empty or over-long name, an unknown
// domain or condition, an empty condition set, or a duplicate object.
func ValidateWatches(in []Watch) ([]Watch, error) {
	if len(in) > MaxWatches {
		return nil, fmt.Errorf("at most %d watches", MaxWatches)
	}
	out := make([]Watch, 0, len(in))
	seen := map[objectKey]bool{}
	for _, w := range in {
		w.Name = strings.TrimSpace(w.Name)
		if w.Name == "" || len(w.Name) > maxNameBytes {
			return nil, errors.New("watch name must be 1..128 bytes")
		}
		if w.Domain == "" {
			w.Domain = DomainACL
		}
		if w.Domain != DomainACL && w.Domain != DomainPrefixList {
			return nil, fmt.Errorf("unsupported watch domain %q", w.Domain)
		}
		if len(w.Conditions) == 0 {
			return nil, fmt.Errorf("watch %s/%s needs at least one condition", w.Domain, w.Name)
		}
		set := map[string]bool{}
		for _, c := range w.Conditions {
			if !knownCondition(c) {
				return nil, fmt.Errorf("unknown trigger condition %q", c)
			}
			set[c] = true
		}
		w.Conditions = nil
		for _, c := range conditionOrder {
			if set[c] {
				w.Conditions = append(w.Conditions, c)
			}
		}
		key := objectKey{w.Domain, w.Name}
		if seen[key] {
			return nil, fmt.Errorf("duplicate watch for %s/%s", w.Domain, w.Name)
		}
		seen[key] = true
		out = append(out, w)
	}
	return out, nil
}

func knownCondition(c string) bool {
	for _, k := range conditionOrder {
		if c == k {
			return true
		}
	}
	return false
}

// SetWatches replaces the whole pinned set atomically: an invalid list
// changes nothing. Recorded outcomes are kept — a watch pinned after
// analyses have run judges the next result against the last recorded
// outcome of that object.
func (s *Store) SetWatches(in []Watch) error {
	normalized, err := ValidateWatches(in)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watches = normalized
	s.watchSet = make(map[objectKey]Watch, len(normalized))
	for _, w := range normalized {
		s.watchSet[objectKey{w.Domain, w.Name}] = w
	}
	return nil
}

// Watches returns the current pinned set, in the order it was set.
func (s *Store) Watches() []Watch {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Watch, len(s.watches))
	copy(out, s.watches)
	return out
}

// decided reports whether a verdict token is a decided behavior
// verdict. INCONCLUSIVE — complete or budget-stopped partial alike —
// is not decided and never fires a verdict condition.
func decided(verdict string) bool {
	return verdict == VerdictEquivalent || verdict == VerdictDifferent
}

// Observe records one completed outcome and returns the inbox entries
// it fired (usually none). The outcome always becomes the object's
// recorded state, watched or not, so baselines and stale marks stay
// current; only a pinned object can fire.
func (s *Store) Observe(o Observation) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	key := objectKey{o.Domain, o.Name}
	prev, seen := s.last[key]

	state := objectState{verdict: o.Verdict, violated: prev.violated, contentSHA256: o.ContentSHA256, seq: s.seq}
	if o.ObligationsEvaluated {
		state.violated = make(map[string]bool, len(o.ViolatedObligations))
		for _, id := range o.ViolatedObligations {
			state.violated[id] = true
		}
	}

	var fired []Entry
	if w, watched := s.watchSet[key]; watched {
		for _, condition := range w.Conditions {
			switch condition {
			case ConditionVerdictChanged:
				// Only a decided-to-decided transition is a verdict
				// change: a previous outcome must exist, and neither
				// side may be INCONCLUSIVE.
				if seen && decided(prev.verdict) && decided(o.Verdict) && prev.verdict != o.Verdict {
					fired = append(fired, s.appendLocked(key, o, condition, prev.verdict, nil))
				}
			case ConditionAnyDifferent:
				// A decided DIFFERENT the user has not already been
				// told about: the identical artifact observed again
				// (a cache hit, a re-run of the same inputs) does not
				// fire twice.
				if decided(o.Verdict) && o.Verdict == VerdictDifferent && !(seen && prev.contentSHA256 == o.ContentSHA256) {
					fired = append(fired, s.appendLocked(key, o, condition, "", nil))
				}
			case ConditionObligationFailed:
				if o.ObligationsEvaluated {
					var newly []string
					for id := range state.violated {
						if !prev.violated[id] {
							newly = append(newly, id)
						}
					}
					if len(newly) > 0 {
						sort.Strings(newly)
						fired = append(fired, s.appendLocked(key, o, condition, "", newly))
					}
				}
			}
		}
	}

	if !seen {
		s.seen = append(s.seen, key)
		if len(s.seen) > MaxTrackedObjects {
			delete(s.last, s.seen[0])
			s.seen = s.seen[1:]
		}
	}
	s.last[key] = state
	return fired
}

// appendLocked stamps and stores one entry. Callers hold s.mu.
func (s *Store) appendLocked(key objectKey, o Observation, condition, previousVerdict string, violated []string) Entry {
	e := Entry{
		ID:                  s.nextID,
		Domain:              key.domain,
		Name:                key.name,
		Condition:           condition,
		PreviousVerdict:     previousVerdict,
		Verdict:             o.Verdict,
		ViolatedObligations: violated,
		ArtifactKind:        o.ArtifactKind,
		ContentSHA256:       o.ContentSHA256,
		SHA256:              o.SHA256,
		Time:                s.now(),
		seq:                 s.seq,
	}
	s.nextID++
	s.entries = append(s.entries, e)
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[len(s.entries)-MaxEntries:]
	}
	return e
}

// Entries returns the inbox newest-first, with the stale mark computed
// against the latest recorded outcome of each entry's object.
func (s *Store) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.entries))
	for i := len(s.entries) - 1; i >= 0; i-- {
		e := s.entries[i]
		if st, ok := s.last[objectKey{e.Domain, e.Name}]; ok && st.seq > e.seq {
			e.Stale = true
		}
		out = append(out, e)
	}
	return out
}

// MarkRead flags one entry as read and reports whether the id exists.
func (s *Store) MarkRead(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].ID == id {
			s.entries[i].Read = true
			return true
		}
	}
	return false
}

// MarkAllRead flags every entry as read and returns how many were
// unread.
func (s *Store) MarkAllRead() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for i := range s.entries {
		if !s.entries[i].Read {
			s.entries[i].Read = true
			n++
		}
	}
	return n
}

// Unread counts the entries not yet marked read.
func (s *Store) Unread() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.entries {
		if !e.Read {
			n++
		}
	}
	return n
}

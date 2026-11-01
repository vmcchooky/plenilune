package inbox

import (
	"fmt"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	base := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	tick := 0
	s.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Minute)
	}
	return s
}

func mustWatch(t *testing.T, s *Store, watches ...Watch) {
	t.Helper()
	if err := s.SetWatches(watches); err != nil {
		t.Fatal(err)
	}
}

func analysisObs(name, verdict, digest string) Observation {
	return Observation{Domain: DomainACL, Name: name, Verdict: verdict, ArtifactKind: ArtifactAnalysis, ContentSHA256: digest, SHA256: "env-" + digest}
}

func comparisonObs(name, verdict, digest string, violated ...string) Observation {
	return Observation{Domain: DomainACL, Name: name, Verdict: verdict, ObligationsEvaluated: true, ViolatedObligations: violated, ArtifactKind: ArtifactComparison, ContentSHA256: digest, SHA256: "env-" + digest}
}

// Acceptance 1: two sequential analyses of the same watched object
// with a changed verdict produce exactly one inbox entry carrying the
// before/after verdicts and the producing artifact's content digest.
func TestVerdictChangeFiresOnceWithBeforeAfter(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "EDGE", Conditions: []string{ConditionVerdictChanged}})

	if fired := s.Observe(analysisObs("EDGE", VerdictEquivalent, "c1")); len(fired) != 0 {
		t.Fatalf("baseline observation fired %v", fired)
	}
	fired := s.Observe(analysisObs("EDGE", VerdictDifferent, "c2"))
	if len(fired) != 1 {
		t.Fatalf("verdict change fired %d entries, want 1", len(fired))
	}
	e := fired[0]
	if e.Condition != ConditionVerdictChanged || e.PreviousVerdict != VerdictEquivalent || e.Verdict != VerdictDifferent {
		t.Fatalf("entry %+v", e)
	}
	if e.Domain != DomainACL || e.Name != "EDGE" || e.ArtifactKind != ArtifactAnalysis || e.ContentSHA256 != "c2" || e.SHA256 != "env-c2" {
		t.Fatalf("entry identity %+v", e)
	}
	if e.Read || e.Stale || e.Time.IsZero() {
		t.Fatalf("fresh entry flags %+v", e)
	}
	entries := s.Entries()
	if len(entries) != 1 || entries[0].ID != e.ID {
		t.Fatalf("inbox %+v", entries)
	}
}

// Acceptance 2 (first half): the same verdict twice fires nothing.
func TestSameVerdictTwiceFiresNothing(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "EDGE", Conditions: []string{ConditionVerdictChanged, ConditionAnyDifferent}})
	s.Observe(analysisObs("EDGE", VerdictEquivalent, "c1"))
	if fired := s.Observe(analysisObs("EDGE", VerdictEquivalent, "c2")); len(fired) != 0 {
		t.Fatalf("unchanged verdict fired %v", fired)
	}
	if len(s.Entries()) != 0 {
		t.Fatalf("inbox not empty: %+v", s.Entries())
	}
}

// Acceptance 2 (second half): an unwatched object changing verdict
// fires nothing — but its outcome is still recorded, so pinning it
// later judges the next result against that baseline.
func TestUnwatchedObjectFiresNothingButRecordsBaseline(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "OTHER", Conditions: []string{ConditionVerdictChanged}})
	s.Observe(analysisObs("EDGE", VerdictEquivalent, "c1"))
	if fired := s.Observe(analysisObs("EDGE", VerdictDifferent, "c2")); len(fired) != 0 {
		t.Fatalf("unwatched object fired %v", fired)
	}
	mustWatch(t, s,
		Watch{Name: "OTHER", Conditions: []string{ConditionVerdictChanged}},
		Watch{Name: "EDGE", Conditions: []string{ConditionVerdictChanged}},
	)
	if fired := s.Observe(analysisObs("EDGE", VerdictEquivalent, "c3")); len(fired) != 1 || fired[0].PreviousVerdict != VerdictDifferent {
		t.Fatalf("late-pinned watch did not use the recorded baseline: %v", fired)
	}
}

// Acceptance 3: an obligation failure fires even when the top-line
// verdict is unchanged, names the newly violated obligations, and
// does not re-fire while the same obligations stay violated.
func TestObligationFailureFiresWithUnchangedVerdict(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "EDGE", Conditions: []string{ConditionObligationFailed}})

	if fired := s.Observe(comparisonObs("EDGE", VerdictEquivalent, "c1")); len(fired) != 0 {
		t.Fatalf("clean comparison fired %v", fired)
	}
	fired := s.Observe(comparisonObs("EDGE", VerdictEquivalent, "c2", "OB-2", "OB-1"))
	if len(fired) != 1 {
		t.Fatalf("obligation failure fired %d entries, want 1", len(fired))
	}
	e := fired[0]
	if e.Condition != ConditionObligationFailed || e.Verdict != VerdictEquivalent || e.ArtifactKind != ArtifactComparison {
		t.Fatalf("entry %+v", e)
	}
	if len(e.ViolatedObligations) != 2 || e.ViolatedObligations[0] != "OB-1" || e.ViolatedObligations[1] != "OB-2" {
		t.Fatalf("violated obligations not sorted/named: %v", e.ViolatedObligations)
	}
	// Still violated, nothing new: no second entry.
	if fired := s.Observe(comparisonObs("EDGE", VerdictEquivalent, "c3", "OB-1", "OB-2")); len(fired) != 0 {
		t.Fatalf("standing violation re-fired %v", fired)
	}
	// One more obligation breaks: fires again naming only the new one.
	fired = s.Observe(comparisonObs("EDGE", VerdictEquivalent, "c4", "OB-1", "OB-2", "OB-3"))
	if len(fired) != 1 || len(fired[0].ViolatedObligations) != 1 || fired[0].ViolatedObligations[0] != "OB-3" {
		t.Fatalf("new violation did not fire alone: %v", fired)
	}
	// An analysis observation carries no obligation evaluation: it
	// must not clear the recorded violation set.
	s.Observe(analysisObs("EDGE", VerdictEquivalent, "c5"))
	if fired := s.Observe(comparisonObs("EDGE", VerdictEquivalent, "c6", "OB-1", "OB-2", "OB-3")); len(fired) != 0 {
		t.Fatalf("analysis observation cleared the violation baseline: %v", fired)
	}
}

// any_different fires for each new DIFFERENT artifact, not for the
// same artifact observed again (cache hit / identical re-run).
func TestAnyDifferentFiresOncePerArtifact(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "EDGE", Conditions: []string{ConditionAnyDifferent}})
	if fired := s.Observe(analysisObs("EDGE", VerdictDifferent, "c1")); len(fired) != 1 {
		t.Fatalf("first DIFFERENT fired %v", fired)
	}
	if fired := s.Observe(analysisObs("EDGE", VerdictDifferent, "c1")); len(fired) != 0 {
		t.Fatalf("same artifact re-fired %v", fired)
	}
	if fired := s.Observe(analysisObs("EDGE", VerdictDifferent, "c2")); len(fired) != 1 {
		t.Fatalf("new DIFFERENT artifact fired %v", fired)
	}
	if fired := s.Observe(analysisObs("EDGE", VerdictEquivalent, "c3")); len(fired) != 0 {
		t.Fatalf("EQUIVALENT fired any_different %v", fired)
	}
}

// INCONCLUSIVE discipline: it is recorded as the object's state, but
// no verdict condition treats a transition to or from it as a verdict
// change. any_different still fires on a decided DIFFERENT that
// follows an inconclusive run.
func TestInconclusiveIsRecordedStateNotVerdictChange(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "EDGE", Conditions: []string{ConditionVerdictChanged, ConditionAnyDifferent}})
	s.Observe(analysisObs("EDGE", VerdictEquivalent, "c1"))
	if fired := s.Observe(analysisObs("EDGE", VerdictInconclusive, "c2")); len(fired) != 0 {
		t.Fatalf("EQUIVALENT -> INCONCLUSIVE fired %v", fired)
	}
	fired := s.Observe(analysisObs("EDGE", VerdictDifferent, "c3"))
	if len(fired) != 1 || fired[0].Condition != ConditionAnyDifferent {
		t.Fatalf("INCONCLUSIVE -> DIFFERENT fired %v, want only any_different", fired)
	}
	if fired := s.Observe(analysisObs("EDGE", VerdictEquivalent, "c4")); len(fired) != 1 || fired[0].Condition != ConditionVerdictChanged || fired[0].PreviousVerdict != VerdictDifferent {
		t.Fatalf("decided-to-decided change after inconclusive fired %v", fired)
	}
}

// Acceptance 4 (store level): entries come back newest-first, the
// read flow persists across calls, and an entry goes stale once a
// newer outcome for its object is recorded.
func TestEntriesNewestFirstReadFlowAndStale(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "EDGE", Conditions: []string{ConditionAnyDifferent}})
	first := s.Observe(analysisObs("EDGE", VerdictDifferent, "c1"))[0]
	second := s.Observe(analysisObs("EDGE", VerdictDifferent, "c2"))[0]

	entries := s.Entries()
	if len(entries) != 2 || entries[0].ID != second.ID || entries[1].ID != first.ID {
		t.Fatalf("entries not newest-first: %+v", entries)
	}
	if !entries[1].Stale {
		t.Fatalf("older entry not marked stale: %+v", entries[1])
	}
	if entries[0].Stale {
		t.Fatalf("latest entry marked stale: %+v", entries[0])
	}
	if s.Unread() != 2 {
		t.Fatalf("unread = %d, want 2", s.Unread())
	}
	if !s.MarkRead(first.ID) {
		t.Fatal("MarkRead lost a known id")
	}
	if s.MarkRead(9999) {
		t.Fatal("MarkRead accepted an unknown id")
	}
	entries = s.Entries()
	if !entries[1].Read || entries[0].Read {
		t.Fatalf("read flags did not persist: %+v", entries)
	}
	if n := s.MarkAllRead(); n != 1 || s.Unread() != 0 {
		t.Fatalf("MarkAllRead = %d, unread = %d", n, s.Unread())
	}
	// A later observation of the object stales the newest entry too.
	s.Observe(analysisObs("EDGE", VerdictEquivalent, "c3"))
	if entries := s.Entries(); !entries[0].Stale {
		t.Fatalf("entry not stale after a newer outcome: %+v", entries[0])
	}
}

func TestValidateWatches(t *testing.T) {
	got, err := ValidateWatches([]Watch{{Name: " EDGE ", Conditions: []string{ConditionObligationFailed, ConditionVerdictChanged, ConditionVerdictChanged}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "EDGE" || got[0].Domain != DomainACL {
		t.Fatalf("normalization %+v", got)
	}
	if len(got[0].Conditions) != 2 || got[0].Conditions[0] != ConditionVerdictChanged || got[0].Conditions[1] != ConditionObligationFailed {
		t.Fatalf("conditions not canonicalized: %v", got[0].Conditions)
	}
	for name, in := range map[string][]Watch{
		"empty name":    {{Name: " ", Conditions: []string{ConditionAnyDifferent}}},
		"bad domain":    {{Domain: "router", Name: "X", Conditions: []string{ConditionAnyDifferent}}},
		"no condition":  {{Name: "X"}},
		"bad condition": {{Name: "X", Conditions: []string{"exploded"}}},
		"duplicate": {
			{Name: "X", Conditions: []string{ConditionAnyDifferent}},
			{Name: "X", Conditions: []string{ConditionVerdictChanged}},
		},
	} {
		if _, err := ValidateWatches(in); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	tooMany := make([]Watch, MaxWatches+1)
	for i := range tooMany {
		tooMany[i] = Watch{Name: fmt.Sprintf("W%d", i), Conditions: []string{ConditionAnyDifferent}}
	}
	if _, err := ValidateWatches(tooMany); err == nil {
		t.Fatal("watch cap not enforced")
	}
	// The same name in two domains is two different objects.
	if _, err := ValidateWatches([]Watch{
		{Domain: DomainACL, Name: "X", Conditions: []string{ConditionAnyDifferent}},
		{Domain: DomainPrefixList, Name: "X", Conditions: []string{ConditionAnyDifferent}},
	}); err != nil {
		t.Fatalf("cross-domain names rejected: %v", err)
	}
}

func TestSetWatchesInvalidKeepsPrevious(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "EDGE", Conditions: []string{ConditionVerdictChanged}})
	if err := s.SetWatches([]Watch{{Name: "", Conditions: []string{ConditionVerdictChanged}}}); err == nil {
		t.Fatal("invalid list accepted")
	}
	if got := s.Watches(); len(got) != 1 || got[0].Name != "EDGE" {
		t.Fatalf("previous watches replaced by an invalid list: %+v", got)
	}
}

// The inbox is bounded: beyond MaxEntries the oldest entries drop.
func TestEntriesAreBounded(t *testing.T) {
	s := testStore(t)
	mustWatch(t, s, Watch{Name: "EDGE", Conditions: []string{ConditionAnyDifferent}})
	for i := 0; i < MaxEntries+10; i++ {
		s.Observe(analysisObs("EDGE", VerdictDifferent, fmt.Sprintf("c%d", i)))
	}
	entries := s.Entries()
	if len(entries) != MaxEntries {
		t.Fatalf("entries = %d, want cap %d", len(entries), MaxEntries)
	}
	if entries[0].ContentSHA256 != fmt.Sprintf("c%d", MaxEntries+9) {
		t.Fatalf("newest entry wrong: %+v", entries[0])
	}
}

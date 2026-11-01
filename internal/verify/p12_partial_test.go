package verify

// P1-2 partial INCONCLUSIVE results: when the budget stops a v0.4 search,
// the report must keep what was proven (identical prefix pairs, decided
// rule pairs with evaluator-checked witnesses) and bound the rest as
// undecided — never characterizing the remainder. Cases that complete
// must be identical to v0.3, including budget accounting.

import (
	"context"
	"math/rand/v2"
	"reflect"
	"testing"

	"plenilune/internal/ciscoacl"
)

func p12CorpusPair(t *testing.T, n int, name string) (ciscoacl.ACL, ciscoacl.ACL) {
	t.Helper()
	for _, c := range p11Cases(n) {
		if c.name == name {
			before, err := ciscoacl.Parse(render("CORPUS", c.before))
			if err != nil {
				t.Fatal(err)
			}
			after, err := ciscoacl.Parse(render("CORPUS", c.after))
			if err != nil {
				t.Fatal(err)
			}
			return before, after
		}
	}
	t.Fatalf("corpus case %s@%d not found", name, n)
	return ciscoacl.ACL{}, ciscoacl.ACL{}
}

func decisionOrdinal(d ciscoacl.Decision) int {
	if d.Implicit || d.RuleOrdinal == nil {
		return -1
	}
	return *d.RuleOrdinal
}

// reevaluateWitnesses checks every witness in the report against the
// independent linear evaluator: the packet must really flip its decision.
func reevaluateWitnesses(t *testing.T, before, after ciscoacl.ACL, witnesses []Witness) {
	t.Helper()
	for _, w := range witnesses {
		x, ex := before.Evaluate(w.Packet)
		y, ey := after.Evaluate(w.Packet)
		if ex != nil || ey != nil || x.Action == y.Action ||
			!reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
			t.Fatalf("witness failed re-evaluation: %+v (before %+v, after %+v)", w, x, y)
		}
	}
}

func decidedPairSet(p *PartialResult) map[[2]int]int {
	out := map[[2]int]int{}
	if p != nil {
		for _, pair := range p.DecidedPairs {
			out[[2]int{pair.Before, pair.After}] = pair.Pieces
		}
	}
	return out
}

// The two motivating cases from the plan: on v0.3 both stop all-or-nothing
// (stop=boxes inside the before permit set). On v0.4 the proven part is
// the identical leading prefix; the residual is one bounded undecided
// region. The proven part is exactly the prefix — the fragmentation that
// stops the run happens before any residual pair is decided, and the
// model must not manufacture more.
func TestP12MotivatingCasesProduceBoundedPartials(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefix int
	}{
		{"swap_adjacent", 256},
		{"change_port", 13},
	} {
		before, after := p12CorpusPair(t, 512, tc.name)

		old, err := CompareEngine(context.Background(), before, after, nil, DefaultOptions(), ResidualEngineVersion)
		if err != nil {
			t.Fatal(err)
		}
		if old.SearchComplete || old.Verdict != "INCONCLUSIVE" || old.Partial != nil {
			t.Fatalf("%s: v0.3 shape changed: %+v", tc.name, old)
		}

		rep, err := Compare(context.Background(), before, after, nil, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if rep.Verdict != "INCONCLUSIVE" || rep.SearchComplete || rep.Reason != "boxes" {
			t.Fatalf("%s: verdict %s complete %v reason %q", tc.name, rep.Verdict, rep.SearchComplete, rep.Reason)
		}
		if rep.Budget.Operations != old.Budget.Operations || rep.Budget.PeakBoxes != old.Budget.PeakBoxes {
			t.Fatalf("%s: partial machinery changed budget accounting: v0.4 %d ops/%d boxes vs v0.3 %d/%d",
				tc.name, rep.Budget.Operations, rep.Budget.PeakBoxes, old.Budget.Operations, old.Budget.PeakBoxes)
		}
		p := rep.Partial
		if p == nil {
			t.Fatalf("%s: no partial explanation", tc.name)
		}
		if p.Stage != PartialStagePermitBefore || p.PrefixPairs != tc.prefix {
			t.Fatalf("%s: partial stage %q prefix %d, want permit_before/%d", tc.name, p.Stage, p.PrefixPairs, tc.prefix)
		}
		if len(p.DecidedPairs) != 0 || len(rep.Witnesses) != 0 {
			t.Fatalf("%s: nothing in the residual was decided, got pairs %v witnesses %d", tc.name, p.DecidedPairs, len(rep.Witnesses))
		}
		wantRange := [2]int{tc.prefix, 511}
		if len(p.Undecided) != 1 || p.Undecided[0].Direction != "delta" ||
			p.Undecided[0].BeforeRules == nil || *p.Undecided[0].BeforeRules != wantRange ||
			p.Undecided[0].AfterRules == nil || *p.Undecided[0].AfterRules != wantRange {
			t.Fatalf("%s: undecided remainder not the bounded residual: %+v", tc.name, p.Undecided)
		}

		ch, err := Changes(context.Background(), before, after, nil, DefaultChangeOptions())
		if err != nil {
			t.Fatal(err)
		}
		if ch.Complete || len(ch.Classes) != 0 || ch.Partial == nil ||
			ch.Partial.Stage != PartialStagePermitBefore || ch.Partial.PrefixPairs != tc.prefix {
			t.Fatalf("%s: changes partial shape wrong: complete %v classes %d partial %+v", tc.name, ch.Complete, len(ch.Classes), ch.Partial)
		}

		// Identical-prefix-only partial at a small size: the prefix proof
		// survives a boxes cap that stops the residual immediately.
		if tc.name == "change_port" {
			small, smallAfter := p12CorpusPair(t, 100, tc.name)
			opts := DefaultOptions()
			opts.Limits.MaxBoxes = 1000
			rep100, err := Compare(context.Background(), small, smallAfter, nil, opts)
			if err != nil {
				t.Fatal(err)
			}
			if rep100.Partial == nil || rep100.Partial.PrefixPairs != 12 ||
				rep100.Partial.Stage != PartialStagePermitBefore || rep100.Verdict != "INCONCLUSIVE" {
				t.Fatalf("change_port@100 cap: partial %+v verdict %s", rep100.Partial, rep100.Verdict)
			}
		}
	}
}

// Stops inside the decide stages: fragments decided before the stop are
// proven (their witnesses re-evaluate), the direction that never finished
// stays undecided, and a proven difference still yields DIFFERENT —
// marked partial, never a plain verdict.
func TestP12DecideStagePartials(t *testing.T) {
	before, after := p12CorpusPair(t, 100, "change_port")

	// Reference: the completing run's change classes bound what any
	// partial may claim — decided pairs and piece counts come from it.
	ref, err := Changes(context.Background(), before, after, nil, DefaultChangeOptions())
	if err != nil || !ref.Complete {
		t.Fatalf("reference changes: %v complete %v", err, ref.Complete)
	}
	refPieces := map[[2]int]int{}
	for _, c := range ref.Classes {
		refPieces[[2]int{derefOrdn(c.Before), derefOrdn(c.After)}] += c.Pieces
	}

	// Stop during decide_lost: nothing decided yet.
	opts := DefaultOptions()
	opts.Limits.MaxOperations = 40_000
	rep, err := Compare(context.Background(), before, after, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != "INCONCLUSIVE" || rep.Partial == nil || rep.Partial.Stage != PartialStageDecideLost {
		t.Fatalf("decide_lost stop: verdict %s partial %+v", rep.Verdict, rep.Partial)
	}
	if len(rep.Partial.DecidedPairs) != 0 || len(rep.Witnesses) != 0 {
		t.Fatalf("decide_lost stop decided something: %+v", rep.Partial)
	}
	if len(rep.Partial.Undecided) != 2 ||
		rep.Partial.Undecided[0].Direction != "lost" || rep.Partial.Undecided[1].Direction != "gained" {
		t.Fatalf("decide_lost undecided shape: %+v", rep.Partial.Undecided)
	}

	// Stop during decide_gained: the lost direction was fully decided and
	// its witnesses prove DIFFERENT, but the search is incomplete.
	opts.Limits.MaxOperations = 60_000
	rep, err = Compare(context.Background(), before, after, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != "DIFFERENT" || rep.SearchComplete || rep.Partial == nil ||
		rep.Partial.Stage != PartialStageDecideGained || rep.Reason != "operations" {
		t.Fatalf("decide_gained stop: verdict %s complete %v partial %+v", rep.Verdict, rep.SearchComplete, rep.Partial)
	}
	if len(rep.Witnesses) == 0 {
		t.Fatal("decide_gained stop: proven difference lost its witnesses")
	}
	reevaluateWitnesses(t, before, after, rep.Witnesses)
	decided := decidedPairSet(rep.Partial)
	for _, w := range rep.Witnesses {
		key := [2]int{decisionOrdinal(w.Before), decisionOrdinal(w.After)}
		if _, ok := decided[key]; !ok {
			t.Fatalf("witness pair %v not among decided pairs %v", key, decided)
		}
	}
	for key, pieces := range decided {
		if limit, ok := refPieces[key]; !ok || pieces > limit {
			t.Fatalf("decided pair %v claims %d pieces, reference allows %d (present %v)", key, pieces, limit, ok)
		}
	}
	if len(rep.Partial.Undecided) == 0 {
		t.Fatal("decide_gained stop: undecided remainder missing")
	}

	// Obligations on a stopped delta are never evaluated: all report
	// INCONCLUSIVE with the comparison's stop reason.
	rng := rand.New(rand.NewPCG(0x0B11, 0xCA7))
	conds := diffObligations(rng, 2)
	opts.Limits.MaxOperations = 40_000
	rep, err = Compare(context.Background(), before, after, conds, opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Partial == nil {
		t.Fatal("obligation run lost its partial")
	}
	for _, ob := range rep.Obligations {
		if ob.Status != "INCONCLUSIVE" || ob.Reason != rep.Reason {
			t.Fatalf("obligation %s on stopped delta: %+v", ob.ID, ob)
		}
	}
}

// A stop inside the Changes class loop keeps the classes already formed
// (exactly the reference run's leading classes) and names the remaining
// pairs as undecided with their full fragment counts. A stop inside the
// delta keeps the classes formable from decided fragments.
func TestP12ChangesPartials(t *testing.T) {
	before, after := p12CorpusPair(t, 250, "change_port")
	ref, err := Changes(context.Background(), before, after, nil, DefaultChangeOptions())
	if err != nil || !ref.Complete || ref.TotalClasses != 2 {
		t.Fatalf("reference: %v complete %v total %d", err, ref.Complete, ref.TotalClasses)
	}

	opts := DefaultChangeOptions()
	opts.Limits.MaxOperations = 447_650
	ch, err := Changes(context.Background(), before, after, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Complete || ch.Partial == nil || ch.Partial.Stage != PartialStageClasses {
		t.Fatalf("class-loop stop: complete %v partial %+v", ch.Complete, ch.Partial)
	}
	if ch.TotalClasses != ref.TotalClasses || len(ch.Classes) != 1 ||
		!reflect.DeepEqual(ch.Classes[0], ref.Classes[0]) {
		t.Fatalf("class-loop stop: emitted classes are not the reference prefix: %+v vs %+v", ch.Classes, ref.Classes)
	}
	rest := ref.Classes[1]
	wantUndecided := []RulePair{{Before: derefOrdn(rest.Before), After: derefOrdn(rest.After), Pieces: rest.Pieces}}
	if !reflect.DeepEqual(ch.Partial.UndecidedPairs, wantUndecided) {
		t.Fatalf("class-loop stop: undecided pairs %+v, want %+v", ch.Partial.UndecidedPairs, wantUndecided)
	}

	// Delta stop with salvageable classes (no declared scope).
	opts.Limits.MaxOperations = 446_000
	ch, err = Changes(context.Background(), before, after, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Complete || ch.Partial == nil || ch.Partial.Stage != PartialStageDecideGained {
		t.Fatalf("delta stop: complete %v partial %+v", ch.Complete, ch.Partial)
	}
	if len(ch.Classes) == 0 {
		t.Fatal("delta stop: decided fragments formed no class")
	}
	decided := decidedPairSet(ch.Partial)
	for _, c := range ch.Classes {
		key := [2]int{derefOrdn(c.Before), derefOrdn(c.After)}
		pieces, ok := decided[key]
		if !ok || c.Pieces > pieces {
			t.Fatalf("salvaged class pair %v pieces %d not backed by decided pairs %v", key, c.Pieces, decided)
		}
		reevaluateWitnesses(t, before, after, []Witness{c.Witness})
	}
}

// Differential discipline, mirroring P1-1: on the same 600 seeded cases,
// where v0.3 completes, v0.4 must produce the identical report (partial
// absent, budget accounting identical); where v0.3 stops deterministically
// inside the delta, v0.4 must carry a partial whose witnesses re-evaluate
// and whose decided pairs are consistent; obligation-stage stops keep
// the v0.3 shape exactly.
func TestP12DifferentialV04AgainstV03(t *testing.T) {
	rng := rand.New(rand.NewPCG(0xD1FF, 0x5EED))
	const cases = 600
	var completed, deltaStopped, legacyStopped, timingSkips int
	for c := 0; c < cases; c++ {
		n := 1 + rng.IntN(14)
		base := diffLines(rng, n)
		var afterLines []string
		switch c % 5 {
		case 0:
			afterLines = diffLines(rng, 1+rng.IntN(14))
		case 1:
			k := rng.IntN(n + 1)
			afterLines = append(append([]string(nil), base[:k]...), diffLines(rng, rng.IntN(6))...)
		case 2:
			afterLines = append([]string(nil), base...)
		case 3:
			afterLines = append([]string{diffRuleLine(rng)}, base...)
		case 4:
			k := rng.IntN(n)
			afterLines = append([]string(nil), base...)
			afterLines[k] = diffRuleLine(rng)
		}
		if len(afterLines) == 0 {
			afterLines = []string{diffRuleLine(rng)}
		}
		before := diffACL(t, base)
		after := diffACL(t, afterLines)
		conds := diffObligations(rng, rng.IntN(4))
		opts := DefaultOptions()

		old, err := CompareEngine(context.Background(), before, after, conds, opts, ResidualEngineVersion)
		if err != nil {
			t.Fatalf("case %d: v0.3 error: %v", c, err)
		}
		got, err := CompareEngine(context.Background(), before, after, conds, opts, EngineVersion)
		if err != nil {
			t.Fatalf("case %d: v0.4 error: %v", c, err)
		}
		if old.Budget.TimingLimited || got.Budget.TimingLimited {
			timingSkips++
			reevaluateWitnesses(t, before, after, got.Witnesses)
			continue
		}
		if old.SearchComplete {
			completed++
			if got.Partial != nil || !reflect.DeepEqual(got, old) {
				t.Fatalf("case %d: completing case diverged\nv0.4: %+v\nv0.3: %+v", c, got, old)
			}
			continue
		}
		// v0.3 stopped deterministically: identical accounting means v0.4
		// stops at the same place.
		if got.SearchComplete || got.Budget.Operations != old.Budget.Operations || got.Budget.PeakBoxes != old.Budget.PeakBoxes {
			t.Fatalf("case %d: stop accounting diverged: v0.4 complete %v %d/%d vs v0.3 %d/%d",
				c, got.SearchComplete, got.Budget.Operations, got.Budget.PeakBoxes, old.Budget.Operations, old.Budget.PeakBoxes)
		}
		reevaluateWitnesses(t, before, after, got.Witnesses)
		if got.Partial == nil {
			// Obligation-stage stop: the delta completed, so the report
			// keeps the v0.3 shape (no partial) and must match it fully.
			legacyStopped++
			if !reflect.DeepEqual(got, old) {
				t.Fatalf("case %d: obligation-stage stop diverged\nv0.4: %+v\nv0.3: %+v", c, got, old)
			}
			continue
		}
		deltaStopped++
		if got.Verdict != old.Verdict && !(old.Verdict == "INCONCLUSIVE" && got.Verdict == "DIFFERENT") {
			t.Fatalf("case %d: partial verdict %s vs v0.3 %s (only a proven DIFFERENT may refine INCONCLUSIVE)", c, got.Verdict, old.Verdict)
		}
		decided := decidedPairSet(got.Partial)
		for _, w := range got.Witnesses {
			key := [2]int{decisionOrdinal(w.Before), decisionOrdinal(w.After)}
			if _, ok := decided[key]; !ok {
				t.Fatalf("case %d: witness pair %v not decided: %+v", c, key, got.Partial)
			}
		}
		if len(got.Partial.Undecided) == 0 {
			t.Fatalf("case %d: partial without an undecided remainder: %+v", c, got.Partial)
		}

		// Changes on the same pair: completing runs identical; stopped
		// runs carry a partial whose classes are backed by decided pairs.
		oldCh, err := ChangesEngine(context.Background(), before, after, nil, DefaultChangeOptions(), ResidualEngineVersion)
		if err != nil {
			t.Fatalf("case %d: v0.3 changes error: %v", c, err)
		}
		gotCh, err := ChangesEngine(context.Background(), before, after, nil, DefaultChangeOptions(), EngineVersion)
		if err != nil {
			t.Fatalf("case %d: v0.4 changes error: %v", c, err)
		}
		if oldCh.Budget.TimingLimited || gotCh.Budget.TimingLimited {
			continue
		}
		if oldCh.Complete {
			if gotCh.Partial != nil || !reflect.DeepEqual(gotCh, oldCh) {
				t.Fatalf("case %d: completing changes diverged\nv0.4: %+v\nv0.3: %+v", c, gotCh, oldCh)
			}
			continue
		}
		if gotCh.Partial == nil {
			t.Fatalf("case %d: v0.4 changes stopped without a partial: %+v", c, gotCh)
		}
		chDecided := decidedPairSet(gotCh.Partial)
		for _, cl := range gotCh.Classes {
			key := [2]int{derefOrdn(cl.Before), derefOrdn(cl.After)}
			if _, ok := chDecided[key]; !ok {
				t.Fatalf("case %d: changes class pair %v not decided: %+v", c, key, gotCh.Partial)
			}
			reevaluateWitnesses(t, before, after, []Witness{cl.Witness})
		}
	}
	t.Logf("cases=%d completed=%d deltaStopped=%d legacyStopped=%d timingSkips=%d",
		cases, completed, deltaStopped, legacyStopped, timingSkips)
	if completed == 0 || deltaStopped == 0 {
		t.Fatalf("differential did not exercise both paths: completed %d deltaStopped %d", completed, deltaStopped)
	}
}

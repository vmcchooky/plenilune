package verify

// P1-1 gate criterion 5: differential equivalence of engine v0.3
// (acl-exact/v0.3, LCP-residual) against engine v0.2 (acl-exact/v0.2, the
// previously shipping construction) on a randomized corpus of small ACL
// pairs with obligations. Wherever v0.2 completes, v0.3 must produce the
// same verdict and the same obligation statuses; the diff witnesses and the
// change classes must be identical (they are computed from the same
// box-for-box delta). Obligation witnesses are compared by independent
// re-evaluation, per the project precedent that a witness's box
// representation may differ between engine versions while the packet
// decision it proves is identical.

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
)

var diffProtos = []string{"ip", "tcp", "udp", "icmp"}

func diffRuleLine(r *rand.Rand) string {
	action := "deny"
	if r.IntN(2) == 0 {
		action = "permit"
	}
	proto := diffProtos[r.IntN(len(diffProtos))]
	addr := func() string {
		switch r.IntN(4) {
		case 0:
			return "any"
		case 1:
			return fmt.Sprintf("host 192.0.2.%d", r.IntN(16))
		case 2:
			return fmt.Sprintf("192.0.2.%d 0.0.0.3", r.IntN(4)*4)
		default:
			return fmt.Sprintf("198.51.100.%d 0.0.0.7", r.IntN(4)*8)
		}
	}
	line := action + " " + proto + " " + addr() + " " + addr()
	if proto == "tcp" || proto == "udp" {
		switch r.IntN(5) {
		case 0:
			// no port constraint
		case 1:
			line += fmt.Sprintf(" eq %d", []int{22, 80, 443, 53}[r.IntN(4)])
		case 2:
			lo := 8000 + r.IntN(3)*10
			line += fmt.Sprintf(" range %d %d", lo, lo+5)
		case 3:
			line += " gt 1023"
		case 4:
			line += fmt.Sprintf(" neq %d", []int{22, 23, 80}[r.IntN(3)])
		}
	}
	return line
}

func diffLines(r *rand.Rand, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = diffRuleLine(r)
	}
	return out
}

func diffACL(t *testing.T, lines []string) ciscoacl.ACL {
	t.Helper()
	var s strings.Builder
	for _, l := range lines {
		s.WriteString(" " + l + "\n")
	}
	return parse(t, s.String())
}

func diffObligations(r *rand.Rand, n int) *intent.Set {
	if n == 0 {
		return nil
	}
	kinds := []string{"must_permit", "must_deny", "must_remain"}
	set := intent.Set{Schema: intent.Schema, Scope: intent.Scope}
	for i := 0; i < n; i++ {
		ob := intent.Obligation{
			ID:   fmt.Sprintf("OB-%d", i+1),
			Kind: kinds[r.IntN(len(kinds))],
			Match: intent.Region{
				Protocol: diffProtos[r.IntN(len(diffProtos))],
				SrcCIDR:  []string{"192.0.2.0/28", "192.0.2.4/30", "0.0.0.0/0"}[r.IntN(3)],
				DstCIDR:  []string{"198.51.100.0/28", "198.51.100.8/29", "0.0.0.0/0"}[r.IntN(3)],
			},
		}
		if ob.Match.Protocol == "tcp" || ob.Match.Protocol == "udp" {
			if r.IntN(2) == 0 {
				ob.Match.DstPorts = []intent.Interval{{Lo: 443, Hi: 443}}
			}
		}
		set.Obligations = append(set.Obligations, ob)
	}
	return &set
}

// violationValid re-evaluates an obligation witness packet with the linear
// evaluator and checks it genuinely proves the violation.
func violationValid(kind string, w *Witness, before, after ciscoacl.ACL) bool {
	if w == nil {
		return false
	}
	x, err1 := before.Evaluate(w.Packet)
	y, err2 := after.Evaluate(w.Packet)
	if err1 != nil || err2 != nil || !reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
		return false
	}
	switch kind {
	case "must_permit":
		return w.After.Action == "deny"
	case "must_deny":
		return w.After.Action == "permit"
	case "must_remain":
		return w.Before.Action != w.After.Action
	}
	return false
}

func TestP11DifferentialV03AgainstV02(t *testing.T) {
	rng := rand.New(rand.NewPCG(0xD1FF, 0x5EED))
	const cases = 600
	var sharedPrefix, headInsert, identical, witnessReprDiff, v02Incomplete, v02ObligationStopped, timingSkips int
	for c := 0; c < cases; c++ {
		n := 1 + rng.IntN(14)
		base := diffLines(rng, n)
		var afterLines []string
		switch c % 5 {
		case 0: // independent pair (usually no common prefix)
			afterLines = diffLines(rng, 1+rng.IntN(14))
		case 1: // shared prefix, independent tails
			k := rng.IntN(n + 1)
			afterLines = append(append([]string(nil), base[:k]...), diffLines(rng, rng.IntN(6))...)
			if k > 0 {
				sharedPrefix++
			}
		case 2: // identical
			afterLines = append([]string(nil), base...)
			identical++
		case 3: // insert at head: no common prefix, long common suffix
			afterLines = append([]string{diffRuleLine(rng)}, base...)
			headInsert++
		case 4: // shared prefix plus one mutated tail rule
			k := rng.IntN(n)
			afterLines = append([]string(nil), base...)
			afterLines[k] = diffRuleLine(rng)
			if k > 0 {
				sharedPrefix++
			}
		}
		if len(afterLines) == 0 {
			afterLines = []string{diffRuleLine(rng)}
		}
		before := diffACL(t, base)
		after := diffACL(t, afterLines)
		conds := diffObligations(rng, rng.IntN(4))
		opts := DefaultOptions()

		old, err := CompareEngine(context.Background(), before, after, conds, opts, IndexedEngineVersion)
		if err != nil {
			t.Fatalf("case %d: v0.2 error: %v", c, err)
		}
		got, err := CompareEngine(context.Background(), before, after, conds, opts, EngineVersion)
		if err != nil {
			t.Fatalf("case %d: v0.3 error: %v", c, err)
		}
		if old.Budget.TimingLimited || got.Budget.TimingLimited {
			// A wall-clock stop is a property of the machine and build
			// (e.g. -race slowdowns), not of the engines' deterministic
			// work; op/box stops below remain binding. Witnesses are still
			// re-evaluated before skipping.
			timingSkips++
			for _, w := range got.Witnesses {
				x, ex := before.Evaluate(w.Packet)
				y, ey := after.Evaluate(w.Packet)
				if ex != nil || ey != nil || x.Action == y.Action || !reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
					t.Fatalf("case %d: v0.3 witness failed re-evaluation in a timing-limited case: %+v", c, w)
				}
			}
			continue
		}
		if !old.SearchComplete {
			// The gate binds only where the full path completes. v0.3 may
			// legitimately complete here; any witness it returns must still
			// prove a real decision difference under independent evaluation.
			v02Incomplete++
			for _, w := range got.Witnesses {
				x, ex := before.Evaluate(w.Packet)
				y, ey := after.Evaluate(w.Packet)
				if ex != nil || ey != nil || x.Action == y.Action || !reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
					t.Fatalf("case %d: v0.3 witness failed re-evaluation where v0.2 was incomplete: %+v", c, w)
				}
			}
			continue
		}
		if !got.SearchComplete || got.Verdict != old.Verdict {
			t.Fatalf("case %d: verdict v0.3 %s (complete %v) vs v0.2 %s", c, got.Verdict, got.SearchComplete, old.Verdict)
		}
		if got.WitnessesTruncated != old.WitnessesTruncated || !reflect.DeepEqual(got.Witnesses, old.Witnesses) {
			t.Fatalf("case %d: diff witnesses differ\nv0.3: %+v\nv0.2: %+v", c, got.Witnesses, old.Witnesses)
		}
		if len(got.Obligations) != len(old.Obligations) {
			t.Fatalf("case %d: obligation count %d vs %d", c, len(got.Obligations), len(old.Obligations))
		}
		var kinds []string
		if conds != nil {
			norm, err := intent.Normalize(*conds)
			if err != nil {
				t.Fatal(err)
			}
			for _, ob := range norm.Obligations {
				kinds = append(kinds, ob.Kind)
			}
		}
		for i := range old.Obligations {
			o, g := old.Obligations[i], got.Obligations[i]
			if o.ID != g.ID {
				t.Fatalf("case %d obligation %s: id mismatch %s", c, o.ID, g.ID)
			}
			if o.Status == "INCONCLUSIVE" {
				// v0.2's obligation evaluation was itself budget-stopped
				// (status INCONCLUSIVE with a stop reason); the gate binds
				// only where v0.2 decided the obligation. v0.3 deciding it
				// exactly is an improvement, and its witness is still
				// required to prove the violation below.
				v02ObligationStopped++
				if g.Status == "VIOLATED" && !violationValid(kinds[i], g.Witness, before, after) {
					t.Fatalf("case %d obligation %s: v0.3 witness failed re-evaluation: %+v", c, o.ID, g.Witness)
				}
				continue
			}
			if o.Status != g.Status {
				t.Fatalf("case %d obligation %s: status v0.3 %s vs v0.2 %s", c, o.ID, g.Status, o.Status)
			}
			if g.Status == "VIOLATED" {
				if !violationValid(kinds[i], g.Witness, before, after) {
					t.Fatalf("case %d obligation %s: v0.3 witness failed re-evaluation: %+v", c, o.ID, g.Witness)
				}
				if o.Witness != nil && g.Witness != nil && !reflect.DeepEqual(g.Witness.Packet, o.Witness.Packet) {
					witnessReprDiff++
				}
			}
		}

		// Change classes derive from the same delta and must agree exactly,
		// including class witness packets.
		oldCh, err := ChangesEngine(context.Background(), before, after, nil, DefaultChangeOptions(), IndexedEngineVersion)
		if err != nil {
			t.Fatalf("case %d: v0.2 changes error: %v", c, err)
		}
		if !oldCh.Complete || oldCh.Budget.TimingLimited {
			continue // Same gate rule as the comparison: equality binds on deterministic completion.
		}
		gotCh, err := ChangesEngine(context.Background(), before, after, nil, DefaultChangeOptions(), EngineVersion)
		if err != nil {
			t.Fatalf("case %d: v0.3 changes error: %v", c, err)
		}
		if !gotCh.Complete {
			if gotCh.Budget.TimingLimited {
				continue
			}
			t.Fatalf("case %d: v0.3 changes incomplete where v0.2 completed: %+v", c, gotCh)
		}
		if gotCh.TotalClasses != oldCh.TotalClasses || len(gotCh.Classes) != len(oldCh.Classes) {
			t.Fatalf("case %d: change classes %d/%d vs %d/%d", c, len(gotCh.Classes), gotCh.TotalClasses, len(oldCh.Classes), oldCh.TotalClasses)
		}
		for i := range oldCh.Classes {
			o, g := oldCh.Classes[i], gotCh.Classes[i]
			if o.Effect != g.Effect || !reflect.DeepEqual(o.Before, g.Before) || !reflect.DeepEqual(o.After, g.After) ||
				o.Pieces != g.Pieces || !reflect.DeepEqual(o.Witness, g.Witness) || !reflect.DeepEqual(o.Examples, g.Examples) {
				t.Fatalf("case %d: change class %d differs\nv0.3: %+v\nv0.2: %+v", c, i, g, o)
			}
		}
	}
	t.Logf("cases=%d sharedPrefix=%d headInsert=%d identical=%d v02Incomplete=%d v02ObligationStopped=%d timingSkips=%d obligationWitnessPacketDiffs=%d",
		cases, sharedPrefix, headInsert, identical, v02Incomplete, v02ObligationStopped, timingSkips, witnessReprDiff)
}

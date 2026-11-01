package repair

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/patch"
	"plenilune/internal/verify"
)

// P1-7 regression: flooding the enumeration's external (proposal)
// source cannot move the deterministic frame. The selection — the
// first VERIFIED candidate in the frame's order — is the same
// engine-enumerated candidate as without any externals; duplicates of
// it submitted as proposals are deduplicated away in favor of the
// engine entry; a verifying proposal that needs more edits ranks below
// it by the shared sort key; and no volume of invalid proposals makes
// any of them verify. Proposal volume is a proposal-layer budget
// concern only: adjudication (Verify, ordering, dedup) does not bend.
func TestP17ExternalFloodCannotMoveSelection(t *testing.T) {
	ctx := context.Background()
	b, a, p, knownGood := fixture(t)
	base, err := Enumerate(ctx, b, a, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	firstVerified := func(e Enumeration) *Candidate {
		for i := range e.Candidates {
			if e.Candidates[i].Result.Status == "VERIFIED" {
				return &e.Candidates[i]
			}
		}
		return nil
	}
	baseSel := firstVerified(base)
	if baseSel == nil {
		t.Fatal("fixture enumeration has no verified candidate")
	}
	baseSelDigest, err := requestDigest(baseSel.Request)
	if err != nil {
		t.Fatal(err)
	}
	engineKeys := func(e Enumeration) []string {
		keys := []string{}
		for _, c := range e.Candidates {
			if c.Transform == TransformExternal {
				continue
			}
			d, err := requestDigest(c.Request)
			if err != nil {
				t.Fatal(err)
			}
			keys = append(keys, c.Transform+"|"+c.Target+"|"+c.Result.Status+"|"+d)
		}
		return keys
	}

	afterHash := evidence.ConfigDigest(a)
	mkReq := func(edits ...patch.Edit) patch.Request {
		return patch.Request{Schema: patch.Schema, Scope: intent.Scope,
			BaseBeforeSHA256: evidence.ConfigDigest(b), BaseAfterSHA256: afterHash,
			ObligationsDigest: knownGood.ObligationsDigest, Edits: edits}
	}
	flood := []patch.Request{}
	// The winning engine candidate, resubmitted as a proposal 50 times.
	for i := 0; i < 50; i++ {
		flood = append(flood, knownGood)
	}
	// 200 distinct junk proposals (each submitted twice): broad permits
	// appended after the final deny are unreachable, so the candidate
	// behaves exactly like the un-repaired after and the violated
	// obligation still fails. None of them may verify.
	const junk = 200
	for port := 2000; port < 2000+junk; port++ {
		req := mkReq(patch.Edit{Op: "append", Content: fmt.Sprintf("permit tcp any any eq %d", port)})
		flood = append(flood, req, req)
	}
	// One distinct proposal that does verify — removing the denying
	// rule (as the winner does) plus the behavior-neutral explicit
	// final deny — but needs two edits, so the shared sort key ranks it
	// below every one-edit verified candidate.
	twoEdit := mkReq(
		patch.Edit{Op: "remove", Ref: &patch.RuleRef{BaseAfterSHA256: afterHash, Ordinal: 0, Line: 3}},
		patch.Edit{Op: "remove", Ref: &patch.RuleRef{BaseAfterSHA256: afterHash, Ordinal: 2, Line: 5}},
	)
	flood = append(flood, twoEdit)

	got, err := Enumerate(ctx, b, a, &p, verify.DefaultOptions(), flood...)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckEnumeration(got); err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates) != len(base.Candidates)+junk+1 {
		t.Fatalf("candidates = %d, want baseline %d + %d unique junk + 1 two-edit", len(got.Candidates), len(base.Candidates), junk)
	}

	// Selection is byte-identical to the no-flood selection.
	sel := firstVerified(got)
	if sel == nil {
		t.Fatal("flooded enumeration lost its verified candidate")
	}
	selDigest, err := requestDigest(sel.Request)
	if err != nil {
		t.Fatal(err)
	}
	if selDigest != baseSelDigest || sel.Transform != baseSel.Transform {
		t.Fatalf("selection moved under flood: %s/%s, want %s/%s", sel.Transform, selDigest, baseSel.Transform, baseSelDigest)
	}

	// Engine-enumerated candidates keep their exact relative order and
	// statuses; externals may only interleave by the shared sort key.
	if !reflect.DeepEqual(engineKeys(base), engineKeys(got)) {
		t.Fatal("engine candidate order or status changed under external flood")
	}

	// The duplicated winner survives exactly once, as the engine entry.
	winnerCount := 0
	for _, c := range got.Candidates {
		d, err := requestDigest(c.Request)
		if err != nil {
			t.Fatal(err)
		}
		if d == baseSelDigest {
			winnerCount++
			if c.Transform == TransformExternal {
				t.Fatal("duplicate proposal displaced the engine-enumerated entry")
			}
		}
	}
	if winnerCount != 1 {
		t.Fatalf("winner digest appears %d times, want 1", winnerCount)
	}

	// Every junk proposal is present exactly once and none verified;
	// the two-edit proposal verified but ranks below the selection.
	externals, junkVerified := 0, 0
	var twoEditCand *Candidate
	for i := range got.Candidates {
		c := &got.Candidates[i]
		if c.Transform != TransformExternal {
			continue
		}
		externals++
		if len(c.Request.Edits) == 2 {
			twoEditCand = c
			continue
		}
		if c.Result.Status == "VERIFIED" {
			junkVerified++
		}
	}
	if externals != junk+1 {
		t.Fatalf("external candidates = %d, want %d", externals, junk+1)
	}
	if junkVerified != 0 {
		t.Fatalf("%d junk proposals verified under flood", junkVerified)
	}
	if twoEditCand == nil || twoEditCand.Result.Status != "VERIFIED" {
		t.Fatalf("two-edit proposal = %+v, want a VERIFIED external candidate", twoEditCand)
	}
	if twoEditCand.Index <= sel.Index {
		t.Fatalf("two-edit proposal index %d ranks at or above the one-edit selection %d", twoEditCand.Index, sel.Index)
	}

	// The flooded record replays from its recorded proposals alone —
	// no live source is consulted.
	res, err := ReplayEnumeration(ctx, b, a, &p, got)
	if err != nil || res.Status != "ENUMERATION_REPLAYED" || res.SHA256 != got.SHA256 {
		t.Fatalf("flooded replay: %v %+v", err, res)
	}
}

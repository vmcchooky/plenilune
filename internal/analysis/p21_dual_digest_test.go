package analysis

// P2-1 (docs/plan/04-phase-2-contract-integration.md): an analysis artifact
// carries two digests — content_sha256 over the semantic payload (sources
// by content, request echo, versions, objects, coverage, unanalyzed
// changes, summary, global budget, limitations) and the whole-envelope
// sha256 (the artifact digest, tool provenance included). The same result
// produced by a different build — or labeled with different caller Ref
// metadata — keeps one content identity. Frozen v1/v2 artifacts keep
// replaying under their original single-digest rules.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"plenilune/internal/jsonio"
)

func TestP21ContentDigestIgnoresProvenance(t *testing.T) {
	b, a := load(t, "redaction")
	r := run(t, b, a, nil)
	if r.Schema != Schema || r.ContentSHA256 == "" {
		t.Fatalf("new analysis must be %s with a content digest: %s %q", Schema, r.Schema, r.ContentSHA256)
	}
	// A different build commit: same content, different envelope.
	other := r
	other.Tool.Commit = "another-build"
	sealed, err := Seal(other)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ContentSHA256 != r.ContentSHA256 {
		t.Fatalf("content identity must not depend on the build commit:\n%s\n%s", sealed.ContentSHA256, r.ContentSHA256)
	}
	if sealed.SHA256 == r.SHA256 {
		t.Fatal("artifact (envelope) digest must cover the tool provenance")
	}
	// A different caller Ref label: caller metadata, not result identity —
	// same content, different envelope.
	relabeled := r
	relabeled.Sources = map[string]SourceInfo{}
	for side, s := range r.Sources {
		s.Ref = "caller-ticket-42"
		relabeled.Sources[side] = s
	}
	sealed, err = Seal(relabeled)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ContentSHA256 != r.ContentSHA256 {
		t.Fatal("content identity must not depend on source Ref labels")
	}
	if sealed.SHA256 == r.SHA256 {
		t.Fatal("artifact (envelope) digest must cover the recorded Ref labels")
	}
	// Replay surfaces both digests.
	ok, err := Replay(context.Background(), []byte(b), []byte(a), r)
	if err != nil || ok.Status != "ANALYSIS_REPLAYED" {
		t.Fatalf("replay: %v %+v", err, ok)
	}
	if ok.SHA256 != r.SHA256 || ok.ContentSHA256 != r.ContentSHA256 {
		t.Fatalf("replay must surface both digests: %+v", ok)
	}
}

func TestP21TamperAndForgedContentRejected(t *testing.T) {
	b, a := load(t, "redaction")
	r := run(t, b, a, nil)
	var e *Error
	// Payload tamper: rejected at the seal gate.
	tampered := r
	tampered.Summary.Objects++
	if _, err := Replay(context.Background(), []byte(b), []byte(a), tampered); !errors.As(err, &e) || e.Code != "EVIDENCE_MISMATCH" {
		t.Fatalf("tampered payload accepted: %v", err)
	}
	// A forged content claim under a consistently recomputed envelope is
	// still rejected: the content digest is recomputed from the payload,
	// never trusted from the record.
	forged := r
	forged.ContentSHA256 = strings.Repeat("0", 64)
	forged.SHA256 = ""
	raw, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	forged.SHA256 = digest(raw)
	if _, err := Replay(context.Background(), []byte(b), []byte(a), forged); !errors.As(err, &e) || e.Code != "EVIDENCE_MISMATCH" {
		t.Fatalf("forged content claim accepted: %v", err)
	}
	// A frozen shape must not grow a content digest.
	frozen := r
	frozen.Schema = SchemaV2
	if _, err := Seal(frozen); err == nil {
		t.Fatal("v2-shaped artifact carrying a content digest sealed")
	}
}

func TestP21ReplayAcceptsFrozenV2Artifact(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "analysis-v2-artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Sanity: the fixture really is a pre-dual-digest v2 artifact, sealed
	// by the v2 code (generated and frozen with P2-1).
	if !strings.Contains(string(raw), `"schema":"plenilune.analysis/v2"`) || strings.Contains(string(raw), "content_sha256") {
		t.Fatal("fixture is not a raw v2 artifact")
	}
	var r Result
	if err := jsonio.Decode(raw, MaxResultBytes, &r); err != nil {
		t.Fatalf("strict decode of v2 artifact: %v", err)
	}
	b, a := load(t, "redaction")
	ok, err := Replay(context.Background(), []byte(b), []byte(a), r)
	if err != nil || ok.Status != "ANALYSIS_REPLAYED" || ok.SHA256 != r.SHA256 || ok.ContentSHA256 != "" {
		t.Fatalf("v2 replay: %v %+v", err, ok)
	}
	// The same sources analyzed today emit only the dual-digest v3 shape.
	fresh := run(t, b, a, nil)
	if fresh.Schema != Schema || fresh.ContentSHA256 == "" {
		t.Fatalf("fresh schema %q content %q", fresh.Schema, fresh.ContentSHA256)
	}
}

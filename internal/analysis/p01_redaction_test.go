package analysis

// P0-1 (docs/plan/02-phase-0-hardening.md): analysis artifacts must not carry
// raw configuration text. Section changes keep a redacted header kind, a line
// count and a section digest; references keep line/kind/name only. Artifacts
// recorded under the frozen v1 schema stay replayable from their sources.

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

var p01Secrets = []string{
	"BEFORE-COMMUNITY-SECRET", "AFTER-COMMUNITY-SECRET",
	"$1$BEFORESECRET$", "$1$AFTERSECRET$",
	"privilege 15 secret", "snmp-server community BEFORE", "snmp-server community AFTER",
}

func findKey(t *testing.T, v any, key, path string) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if k == key {
				t.Fatalf("artifact carries forbidden key %q at %s", key, path)
			}
			findKey(t, val, key, path+"/"+k)
		}
	case []any:
		for _, val := range x {
			findKey(t, val, key, path)
		}
	}
}

func TestP01ArtifactCarriesNoRawSecretText(t *testing.T) {
	b, a := load(t, "redaction")
	r := run(t, b, a, nil)
	// The literal pin names the current schema: v2 when P0-1 landed, v3
	// since the P2-1 dual-digest bump. The redaction assertions below are
	// the invariant; the pin just keeps "current" explicit.
	if r.Schema != Schema || Schema != "plenilune.analysis/v3" {
		t.Fatalf("schema %q", r.Schema)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p01Secrets {
		if strings.Contains(string(raw), s) {
			t.Fatalf("artifact leaks %q", s)
		}
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	findKey(t, generic, "header", "")
	findKey(t, generic, "text", "")

	byKind := map[string]SectionChange{}
	for _, c := range r.UnanalyzedChanges {
		if c.Header != "" {
			t.Fatalf("v2 section change carries raw header: %+v", c)
		}
		if len(c.SectionSHA256) != 64 {
			t.Fatalf("section change lacks digest: %+v", c)
		}
		byKind[c.Change+"/"+c.HeaderKind] = c
	}
	removed := byKind["removed/snmp-server community"]
	if removed.Lines != 1 || removed.SectionSHA256 != digest([]byte("snmp-server community BEFORE-COMMUNITY-SECRET ro")) {
		t.Fatalf("removed snmp-server change %+v", removed)
	}
	if _, ok := byKind["added/snmp-server community"]; !ok {
		t.Fatalf("added snmp-server change missing: %+v", r.UnanalyzedChanges)
	}
	if _, ok := byKind["removed/username netops"]; !ok {
		t.Fatalf("username change must keep at most two header tokens: %+v", r.UnanalyzedChanges)
	}
	modified, ok := byKind["modified/interface GigabitEthernet0/1"]
	if !ok || modified.Lines != 4 || modified.BeforeLine != 7 || modified.AfterLine != 7 {
		t.Fatalf("modified interface change %+v", r.UnanalyzedChanges)
	}

	edge := object(t, r, DomainACL, "EDGE")
	refs := edge.References["after"]
	if len(refs) != 1 || refs[0].Kind != "access-group" || refs[0].Name != "EDGE" || refs[0].Line != 10 || refs[0].Text != "" {
		t.Fatalf("references %+v", edge.References)
	}
}

func TestP01ReplayV2ArtifactAndTamperDetection(t *testing.T) {
	b, a := load(t, "redaction")
	r := run(t, b, a, nil)
	ok, err := Replay(context.Background(), []byte(b), []byte(a), r)
	if err != nil || ok.Status != "ANALYSIS_REPLAYED" {
		t.Fatalf("replay: %v %+v", err, ok)
	}
	tampered := r
	tampered.UnanalyzedChanges = append([]SectionChange(nil), r.UnanalyzedChanges...)
	tampered.UnanalyzedChanges[0].HeaderKind = "interface GigabitEthernet9/9"
	var e *Error
	if _, err := Replay(context.Background(), []byte(b), []byte(a), tampered); !errors.As(err, &e) || e.Code != "EVIDENCE_MISMATCH" {
		t.Fatalf("tampered header_kind accepted: %v", err)
	}
	// An artifact relabeled under an unknown schema version is still
	// rejected: the schema is pinned, never guessed at replay (CONTRACTS
	// section 4). (The unknown version moves with the current schema —
	// this segment named v3 until P2-1 made v3 current.)
	future := r
	future.Schema = "plenilune.analysis/v4"
	if _, err := Replay(context.Background(), []byte(b), []byte(a), future); !errors.As(err, &e) || e.Code != "EVIDENCE_MISMATCH" {
		t.Fatalf("unknown schema accepted: %v", err)
	}
}

func TestP01ReplayAcceptsFrozenV1Artifact(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "analysis-v1-artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Sanity: the fixture really is a pre-redaction v1 artifact, sealed by
	// the v1 code over sources that contain secret-looking lines.
	if !strings.Contains(string(raw), `"schema":"plenilune.analysis/v1"`) || !strings.Contains(string(raw), "BEFORE-COMMUNITY-SECRET") {
		t.Fatal("fixture is not a raw v1 artifact")
	}
	var r Result
	if err := jsonio.Decode(raw, MaxResultBytes, &r); err != nil {
		t.Fatalf("strict decode of v1 artifact: %v", err)
	}
	b, a := load(t, "redaction")
	ok, err := Replay(context.Background(), []byte(b), []byte(a), r)
	if err != nil || ok.Status != "ANALYSIS_REPLAYED" || ok.SHA256 != r.SHA256 {
		t.Fatalf("v1 replay: %v %+v", err, ok)
	}
	// The same sources analyzed today emit only the redacted v2 shape.
	fresh := run(t, b, a, nil)
	if fresh.Schema != Schema {
		t.Fatalf("fresh schema %q", fresh.Schema)
	}
}

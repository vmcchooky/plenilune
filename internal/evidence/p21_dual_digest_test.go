package evidence

// P2-1 (docs/plan/04-phase-2-contract-integration.md): a comparison carries
// two digests — content_sha256 over the semantic payload (inputs,
// definitions, scope, parser/engine versions, options and results) and the
// whole-envelope sha256 (the artifact digest, provenance included). The
// same result from a different build keeps one content identity; a frozen
// v1 artifact keeps checking under the original single-digest rules.

import (
	"strings"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/verify"
)

func p21Comparison(commit string) Comparison {
	sha := strings.Repeat("a", 64)
	return Comparison{
		Tool:          Tool{Version: ToolVersion, Commit: commit},
		EngineVersion: verify.EngineVersion,
		Before:        Input{SHA256: sha, Name: "DEMO", Kind: "named-extended"},
		After:         Input{SHA256: sha, Name: "DEMO", Kind: "named-extended"},
		Report: verify.Report{
			Verdict:        "EQUIVALENT",
			SearchComplete: true,
			Witnesses:      []verify.Witness{},
			Options:        verify.DefaultOptions(),
			Budget:         packetset.Stats{Operations: 10, PeakBoxes: 2},
		},
	}
}

func TestP21ContentDigestIgnoresBuildCommit(t *testing.T) {
	a, err := SealComparison(p21Comparison("commit-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := SealComparison(p21Comparison("commit-b"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Schema != ComparisonSchema || a.ContentSHA256 == "" {
		t.Fatalf("new comparisons must be %s with a content digest: %+v", ComparisonSchema, a)
	}
	if a.ContentSHA256 != b.ContentSHA256 {
		t.Fatalf("content identity must not depend on the build commit:\n%s\n%s", a.ContentSHA256, b.ContentSHA256)
	}
	if a.SHA256 == b.SHA256 {
		t.Fatal("artifact (envelope) digest must cover the tool provenance")
	}
	if err := CheckComparison(a); err != nil {
		t.Fatal(err)
	}
	if err := CheckComparison(b); err != nil {
		t.Fatal(err)
	}
}

func TestP21TamperBreaksContentAndEnvelope(t *testing.T) {
	c, err := SealComparison(p21Comparison("commit-a"))
	if err != nil {
		t.Fatal(err)
	}
	// Payload tamper without rehashing: both checks reject.
	tampered := c
	tampered.Report.Budget.Operations = 11
	if err := CheckComparison(tampered); err == nil {
		t.Fatal("tampered payload accepted")
	}
	// A forger who recomputes the envelope over a forged content claim is
	// still caught: the content digest is recomputed from the payload,
	// never trusted from the artifact.
	forged := c
	forged.ContentSHA256 = strings.Repeat("0", 64)
	h, err := comparisonDigest(forged)
	if err != nil {
		t.Fatal(err)
	}
	forged.SHA256 = h
	if err := CheckComparison(forged); err == nil {
		t.Fatal("forged content claim with a consistent envelope accepted")
	}
	// Payload tamper rehashed through the public seal changes content too:
	// the tampered result is a different content identity.
	other, err := SealComparison(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if other.ContentSHA256 == c.ContentSHA256 {
		t.Fatal("payload tamper kept the content digest")
	}
}

func TestP21FrozenV1ChecksUnderOldRules(t *testing.T) {
	// Rebuild a genuine v1 artifact: same payload, frozen schema, no
	// content field, envelope digest over the v1 shape.
	v1 := p21Comparison("unversioned")
	v1.Schema = ComparisonSchemaV1
	v1.ParserVersion, v1.Scope = ciscoacl.ParserVersion, intent.Scope
	h, err := comparisonDigest(v1)
	if err != nil {
		t.Fatal(err)
	}
	v1.SHA256 = h
	if err := CheckComparison(v1); err != nil {
		t.Fatalf("frozen v1 rejected: %v", err)
	}
	// A v1 artifact that grows a content digest is not a v1 artifact.
	v1.ContentSHA256 = strings.Repeat("b", 64)
	if err := CheckComparison(v1); err == nil {
		t.Fatal("v1 artifact carrying a content digest accepted")
	}
	// An unknown schema is rejected, never guessed.
	v1.ContentSHA256 = ""
	v1.Schema = "plenilune.evidence/v3"
	if err := CheckComparison(v1); err == nil {
		t.Fatal("unknown evidence schema accepted")
	}
}

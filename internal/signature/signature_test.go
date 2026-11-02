package signature

// P2-5 (docs/plan/04-phase-2-contract-integration.md): detached
// Ed25519 signatures bind a sealed artifact's P2-1 digest pair. The
// tests pin the acceptance of the plan item: verify passes with the
// right key, fails with the wrong key or tampered content, and no
// private key material ever appears in an envelope. All keys are
// ephemeral and all artifacts synthetic.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"plenilune/internal/analysis"
	"plenilune/internal/evidence"
	"plenilune/internal/packetset"
	"plenilune/internal/verify"
)

func newKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a signature.Error", err)
	}
	return se.Code
}

// sealedComparison fabricates a sealed evidence v2 artifact with the
// given build commit and input digest, following the fabrication
// pattern of the evidence package's own dual-digest tests.
func sealedComparison(t *testing.T, commit, inputSHA string) []byte {
	t.Helper()
	c := evidence.Comparison{
		Tool:          evidence.Tool{Version: evidence.ToolVersion, Commit: commit},
		EngineVersion: verify.EngineVersion,
		Before:        evidence.Input{SHA256: inputSHA, Name: "DEMO", Kind: "named-extended"},
		After:         evidence.Input{SHA256: inputSHA, Name: "DEMO", Kind: "named-extended"},
		Report: verify.Report{
			Verdict:        "EQUIVALENT",
			SearchComplete: true,
			Witnesses:      []verify.Witness{},
			Options:        verify.DefaultOptions(),
			Budget:         packetset.Stats{Operations: 10, PeakBoxes: 2},
		},
	}
	sealed, err := evidence.SealComparison(c)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(sealed)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func sealedAnalysis(t *testing.T) []byte {
	t.Helper()
	r, err := analysis.Analyze(context.Background(), analysis.Request{
		Schema: analysis.RequestSchema,
		Before: analysis.Source{Text: "ip access-list extended EDGE\n permit tcp any any eq 443\n"},
		After:  analysis.Source{Text: "ip access-list extended EDGE\n permit tcp any any eq 443\n permit udp any any eq 53\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSignVerifyComparisonRoundTrip(t *testing.T) {
	priv := newKey(t)
	artifact := sealedComparison(t, "commit-a", strings.Repeat("a", 64))
	env, err := Sign(priv, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if env.Schema != Schema || env.ArtifactSchema != evidence.ComparisonSchema {
		t.Fatalf("envelope identity: %+v", env)
	}
	var recorded evidence.Comparison
	if err := json.Unmarshal(artifact, &recorded); err != nil {
		t.Fatal(err)
	}
	// BOTH digests are bound, and they are the artifact's own pair.
	if env.ContentSHA256 != recorded.ContentSHA256 || env.ArtifactSHA256 != recorded.SHA256 {
		t.Fatalf("envelope binds %s / %s, artifact carries %s / %s",
			env.ContentSHA256, env.ArtifactSHA256, recorded.ContentSHA256, recorded.SHA256)
	}
	pub := priv.Public().(ed25519.PublicKey)
	if env.Fingerprint != Fingerprint(pub) || env.PublicKey != EncodePublicKey(pub) {
		t.Fatal("envelope key material does not match the signing key")
	}
	// The fingerprint definition is contractual: hex SHA-256 of the
	// raw public key bytes.
	sum := sha256.Sum256(pub)
	if env.Fingerprint != hex.EncodeToString(sum[:]) {
		t.Fatal("fingerprint is not the SHA-256 of the raw public key")
	}
	// The canonical signed string is contractual and domain-separated.
	want := "plenilune.signature/v1\n" + evidence.ComparisonSchema + "\n" + env.ContentSHA256 + "\n" + env.ArtifactSHA256 + "\n"
	if got := string(Message(env.ArtifactSchema, env.ContentSHA256, env.ArtifactSHA256)); got != want {
		t.Fatalf("canonical message:\n%q\nwant:\n%q", got, want)
	}
	if !ed25519.Verify(pub, Message(env.ArtifactSchema, env.ContentSHA256, env.ArtifactSHA256), mustB64(t, env.Signature)) {
		t.Fatal("raw Ed25519 verification of the envelope signature failed")
	}
	res, err := Verify(artifact, env, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusValid || res.ContentSHA256 != env.ContentSHA256 || res.ArtifactSHA256 != env.ArtifactSHA256 || res.Fingerprint != env.Fingerprint {
		t.Fatalf("verify result: %+v", res)
	}
	// A correct pin also verifies; the envelope survives a
	// marshal/parse round trip byte-stably in meaning.
	if _, err := Verify(artifact, env, env.Fingerprint); err != nil {
		t.Fatal(err)
	}
	raw, err := Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != env {
		t.Fatalf("envelope round trip: %+v vs %+v", parsed, env)
	}
	if _, err := Verify(artifact, parsed, ""); err != nil {
		t.Fatal(err)
	}
}

func TestSignVerifyAnalysisRoundTrip(t *testing.T) {
	priv := newKey(t)
	artifact := sealedAnalysis(t)
	env, err := Sign(priv, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if env.ArtifactSchema != analysis.Schema {
		t.Fatalf("envelope artifact schema: %+v", env)
	}
	var recorded analysis.Result
	if err := json.Unmarshal(artifact, &recorded); err != nil {
		t.Fatal(err)
	}
	if env.ContentSHA256 != recorded.ContentSHA256 || env.ArtifactSHA256 != recorded.SHA256 {
		t.Fatal("envelope does not bind the analysis digest pair")
	}
	res, err := Verify(artifact, env, "")
	if err != nil || res.Status != StatusValid {
		t.Fatalf("verify: %+v %v", res, err)
	}
}

// Acceptance: both digest fields are actually bound — mutating
// either recorded digest in the envelope breaks verification even
// though the signature and the artifact are untouched.
func TestBothDigestsAreBound(t *testing.T) {
	priv := newKey(t)
	artifact := sealedComparison(t, "commit-a", strings.Repeat("a", 64))
	env, err := Sign(priv, artifact)
	if err != nil {
		t.Fatal(err)
	}
	flip := func(s string) string {
		if s[0] == '0' {
			return "1" + s[1:]
		}
		return "0" + s[1:]
	}
	mutated := env
	mutated.ContentSHA256 = flip(mutated.ContentSHA256)
	if _, err := Verify(artifact, mutated, ""); codeOf(t, err) != "SIGNATURE_DIGEST_MISMATCH" {
		t.Fatalf("mutated content digest: %v", err)
	}
	mutated = env
	mutated.ArtifactSHA256 = flip(mutated.ArtifactSHA256)
	if _, err := Verify(artifact, mutated, ""); codeOf(t, err) != "SIGNATURE_DIGEST_MISMATCH" {
		t.Fatalf("mutated artifact digest: %v", err)
	}
}

// Acceptance: tampering with the artifact after signing — semantic
// or provenance-only — fails with the digest-mismatch code, as does
// presenting one artifact with another artifact's envelope.
func TestVerifyRejectsTamperedOrSwappedArtifact(t *testing.T) {
	priv := newKey(t)
	artifact := sealedComparison(t, "commit-a", strings.Repeat("a", 64))
	env, err := Sign(priv, artifact)
	if err != nil {
		t.Fatal(err)
	}
	// Provenance-only change: same content digest, different
	// artifact digest — the pair no longer matches the envelope.
	otherBuild := sealedComparison(t, "commit-b", strings.Repeat("a", 64))
	if _, err := Verify(otherBuild, env, ""); codeOf(t, err) != "SIGNATURE_DIGEST_MISMATCH" {
		t.Fatalf("different build of the same result: %v", err)
	}
	// Different inputs entirely.
	otherInputs := sealedComparison(t, "commit-a", strings.Repeat("b", 64))
	if _, err := Verify(otherInputs, env, ""); codeOf(t, err) != "SIGNATURE_DIGEST_MISMATCH" {
		t.Fatalf("different artifact: %v", err)
	}
	// An analysis artifact under a comparison envelope.
	if _, err := Verify(sealedAnalysis(t), env, ""); codeOf(t, err) != "SIGNATURE_DIGEST_MISMATCH" {
		t.Fatalf("cross-kind artifact: %v", err)
	}
}

// Acceptance: the wrong key fails — both as a signature that does
// not verify under the envelope's key and as a pinned fingerprint
// naming a different signer.
func TestVerifyRejectsWrongKey(t *testing.T) {
	privA, privB := newKey(t), newKey(t)
	artifact := sealedComparison(t, "commit-a", strings.Repeat("a", 64))
	envA, err := Sign(privA, artifact)
	if err != nil {
		t.Fatal(err)
	}
	envB, err := Sign(privB, artifact)
	if err != nil {
		t.Fatal(err)
	}
	swapped := envA
	swapped.Signature = envB.Signature
	if _, err := Verify(artifact, swapped, ""); codeOf(t, err) != "SIGNATURE_MISMATCH" {
		t.Fatalf("signature from another key: %v", err)
	}
	fpB := Fingerprint(privB.Public().(ed25519.PublicKey))
	if _, err := Verify(artifact, envA, fpB); codeOf(t, err) != "SIGNATURE_MISMATCH" {
		t.Fatalf("pinned fingerprint of another signer: %v", err)
	}
	// The genuine second signature verifies against its own envelope.
	if _, err := Verify(artifact, envB, fpB); err != nil {
		t.Fatal(err)
	}
}

// An artifact whose own recorded digest was edited after sealing
// fails its seal check even when a signature exists over the true
// digests: the presented file is not the sealed artifact.
func TestVerifyRejectsArtifactFailingOwnSeal(t *testing.T) {
	priv := newKey(t)
	artifact := sealedComparison(t, "commit-a", strings.Repeat("a", 64))
	env, err := Sign(priv, artifact)
	if err != nil {
		t.Fatal(err)
	}
	var c evidence.Comparison
	if err := json.Unmarshal(artifact, &c); err != nil {
		t.Fatal(err)
	}
	c.SHA256 = strings.Repeat("0", 64)
	broken, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(broken, env, ""); codeOf(t, err) != "EVIDENCE_MISMATCH" {
		t.Fatalf("artifact with edited digest field: %v", err)
	}
	if _, err := Sign(priv, broken); codeOf(t, err) != "EVIDENCE_MISMATCH" {
		t.Fatalf("signing an artifact that does not check out: %v", err)
	}
}

// Frozen artifacts carry no content digest, so there is no pair to
// bind: signing refuses rather than downgrading to one digest.
func TestSignRefusesFrozenArtifacts(t *testing.T) {
	priv := newKey(t)
	c := evidence.Comparison{
		Tool:          evidence.Tool{Version: evidence.ToolVersion, Commit: "commit-a"},
		EngineVersion: verify.EngineVersion,
		Before:        evidence.Input{SHA256: strings.Repeat("a", 64), Name: "DEMO", Kind: "named-extended"},
		After:         evidence.Input{SHA256: strings.Repeat("a", 64), Name: "DEMO", Kind: "named-extended"},
		Report: verify.Report{
			Verdict:        "EQUIVALENT",
			SearchComplete: true,
			Witnesses:      []verify.Witness{},
			Options:        verify.DefaultOptions(),
			Budget:         packetset.Stats{Operations: 10, PeakBoxes: 2},
		},
	}
	v1, err := evidence.SealComparisonAs(c, evidence.ComparisonSchemaV1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(priv, data); codeOf(t, err) != "SIGN_ARTIFACT_UNSUPPORTED" {
		t.Fatalf("signing frozen v1 evidence: %v", err)
	}
	if _, _, _, err := ArtifactDigests(data); codeOf(t, err) != "SIGN_ARTIFACT_UNSUPPORTED" {
		t.Fatalf("digests of frozen v1 evidence: %v", err)
	}
}

// An artifact with its digests stripped was never sealed; signing
// it would bind nothing, so it is refused too.
func TestSignRefusesUnsealedArtifact(t *testing.T) {
	priv := newKey(t)
	artifact := sealedComparison(t, "commit-a", strings.Repeat("a", 64))
	var c evidence.Comparison
	if err := json.Unmarshal(artifact, &c); err != nil {
		t.Fatal(err)
	}
	c.ContentSHA256, c.SHA256 = "", ""
	stripped, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(priv, stripped); codeOf(t, err) != "EVIDENCE_MISMATCH" {
		t.Fatalf("signing an unsealed artifact: %v", err)
	}
}

func TestParseRejectsMalformedEnvelopes(t *testing.T) {
	priv := newKey(t)
	artifact := sealedComparison(t, "commit-a", strings.Repeat("a", 64))
	env, err := Sign(priv, artifact)
	if err != nil {
		t.Fatal(err)
	}
	other := newKey(t)
	cases := map[string]func(*Envelope){
		"wrong envelope schema":  func(e *Envelope) { e.Schema = "plenilune.signature/v2" },
		"unsupported artifact":   func(e *Envelope) { e.ArtifactSchema = evidence.ComparisonSchemaV1 },
		"malformed digest":       func(e *Envelope) { e.ContentSHA256 = "zz" },
		"fingerprint not of key": func(e *Envelope) { e.Fingerprint = Fingerprint(other.Public().(ed25519.PublicKey)) },
		"short signature":        func(e *Envelope) { e.Signature = base64.StdEncoding.EncodeToString([]byte("short")) },
		"short public key":       func(e *Envelope) { e.PublicKey = base64.StdEncoding.EncodeToString([]byte("short")) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := env
			mutate(&bad)
			raw, err := Marshal(bad)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Parse(raw); codeOf(t, err) != "SIGNATURE_INVALID" {
				t.Fatalf("Parse: %v", err)
			}
			if _, err := Verify(artifact, bad, ""); codeOf(t, err) != "SIGNATURE_INVALID" {
				t.Fatalf("Verify: %v", err)
			}
		})
	}
	if _, err := Parse([]byte("not json")); codeOf(t, err) != "SIGNATURE_INVALID" {
		t.Fatalf("Parse of garbage: %v", err)
	}
	extra, err := Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	extra = append(extra[:len(extra)-2], []byte(",\n  \"extra\": 1\n}\n")...)
	if _, err := Parse(extra); codeOf(t, err) != "SIGNATURE_INVALID" {
		t.Fatalf("Parse must reject unknown fields: %v", err)
	}
}

func TestKeyMaterialForms(t *testing.T) {
	priv := newKey(t)
	got, err := ParsePrivateKey(EncodePrivateKey(priv))
	if err != nil || !got.Equal(priv) {
		t.Fatalf("private key round trip: %v", err)
	}
	seedForm, err := ParsePrivateKey(base64.StdEncoding.EncodeToString(priv.Seed()))
	if err != nil || !seedForm.Equal(priv) {
		t.Fatalf("seed form: %v", err)
	}
	withNewline, err := ParsePrivateKey(EncodePrivateKey(priv) + "\n")
	if err != nil || !withNewline.Equal(priv) {
		t.Fatalf("trailing newline: %v", err)
	}
	for _, bad := range []string{"", "not base64!!", base64.StdEncoding.EncodeToString([]byte("ten bytes!"))} {
		if _, err := ParsePrivateKey(bad); codeOf(t, err) != "SIGN_KEY_INVALID" {
			t.Fatalf("ParsePrivateKey(%q): %v", bad, err)
		}
	}
	pub := priv.Public().(ed25519.PublicKey)
	gotPub, err := ParsePublicKey(EncodePublicKey(pub))
	if err != nil || !gotPub.Equal(pub) {
		t.Fatalf("public key round trip: %v", err)
	}
	if _, err := ParsePublicKey(EncodePrivateKey(priv)); codeOf(t, err) != "SIGN_KEY_INVALID" {
		t.Fatalf("private key as public key: %v", err)
	}
}

// Acceptance: the envelope — the only signing output that travels —
// contains no private key material in any encoding the package uses.
func TestEnvelopeCarriesNoPrivateKeyMaterial(t *testing.T) {
	priv := newKey(t)
	env, err := Sign(priv, sealedComparison(t, "commit-a", strings.Repeat("a", 64)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{EncodePrivateKey(priv), base64.StdEncoding.EncodeToString(priv.Seed())} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("envelope contains private key material")
		}
	}
}

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

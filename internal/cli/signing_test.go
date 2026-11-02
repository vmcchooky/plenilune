package cli

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"plenilune/internal/evidence"
	"plenilune/internal/jsonio"
	"plenilune/internal/signature"
)

// keygenOut is the sign-keygen stdout document.
type keygenOut struct {
	Status      string `json:"status"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
}

// makeSigningKey runs sign-keygen into dir and returns the key path
// and the printed public material.
func makeSigningKey(t *testing.T, dir string) (string, keygenOut) {
	t.Helper()
	keyPath := filepath.Join(dir, "signing.key")
	code, out, errout := invoke("sign-keygen", "--out", keyPath)
	if code != 0 || errout != "" {
		t.Fatalf("sign-keygen: %d %s", code, errout)
	}
	var kg keygenOut
	if err := json.Unmarshal([]byte(out), &kg); err != nil {
		t.Fatal(err)
	}
	if kg.Status != "KEY_GENERATED" || len(kg.Fingerprint) != 64 || kg.PublicKey == "" {
		t.Fatalf("keygen output: %s", out)
	}
	return keyPath, kg
}

// diffArtifact produces a sealed comparison artifact file for a
// synthetic pair and returns (before, after, artifact) paths.
func diffArtifact(t *testing.T, dir string) (string, string, string) {
	t.Helper()
	b := put(t, dir, "before.acl", "ip access-list extended DEMO\n permit tcp any any eq 22\n")
	a := put(t, dir, "after.acl", "ip access-list extended DEMO\n deny ip any any\n")
	code, body, errout := invoke("diff", "--before", b, "--after", a)
	if code != 1 || errout != "" {
		t.Fatalf("diff: %d %s", code, errout)
	}
	return b, a, put(t, dir, "comparison.json", body)
}

func signArtifact(t *testing.T, keyPath, artifactPath, sigPath string) signature.Envelope {
	t.Helper()
	code, out, errout := invoke("sign", "--key", keyPath, "--evidence", artifactPath, "--out", sigPath)
	if code != 0 || errout != "" {
		t.Fatalf("sign: %d %s", code, errout)
	}
	var env signature.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatal(err)
	}
	fileEnv, err := signature.Parse(onDisk)
	if err != nil {
		t.Fatal(err)
	}
	if fileEnv != env {
		t.Fatalf("envelope on disk differs from stdout:\n%s", onDisk)
	}
	return env
}

// Acceptance: the private key lands in a 0600 file and nowhere else
// — keygen prints only the fingerprint and public key, and a second
// keygen refuses to overwrite the key.
func TestSignKeygenWritesPrivateKey0600AndPrintsOnlyPublic(t *testing.T) {
	dir := t.TempDir()
	keyPath, kg := makeSigningKey(t, dir)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("key file permissions %v, want 0600", info.Mode().Perm())
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	keyText := strings.TrimSpace(string(keyBytes))
	if _, err := signature.ParsePrivateKey(keyText); err != nil {
		t.Fatalf("key file does not hold a private key: %v", err)
	}
	if strings.Contains(kg.Fingerprint+kg.PublicKey, keyText) || keyText == kg.PublicKey {
		t.Fatal("keygen printed private key material")
	}
	code, _, errout := invoke("sign-keygen", "--out", keyPath)
	if code != 2 || !strings.Contains(errout, `"code":"SIGN_KEY_INVALID"`) {
		t.Fatalf("keygen over an existing key: %d %s", code, errout)
	}
	after, err := os.ReadFile(keyPath)
	if err != nil || string(after) != string(keyBytes) {
		t.Fatal("refused keygen modified the existing key file")
	}
}

// Acceptance 1 (CLI): sign a comparison artifact, verify it —
// bare, with the correct fingerprint pin and with the public key
// file — and confirm the pair the envelope binds is the artifact's.
func TestSignVerifyComparisonRoundTrip(t *testing.T) {
	dir := t.TempDir()
	_, _, artifactPath := diffArtifact(t, dir)
	keyPath, kg := makeSigningKey(t, dir)
	sigPath := filepath.Join(dir, "comparison.sig.json")
	env := signArtifact(t, keyPath, artifactPath, sigPath)
	var art evidence.Comparison
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonio.Decode(data, evidence.MaxComparisonBytes, &art); err != nil {
		t.Fatal(err)
	}
	if env.ContentSHA256 != art.ContentSHA256 || env.ArtifactSHA256 != art.SHA256 || env.Fingerprint != kg.Fingerprint {
		t.Fatalf("envelope %+v does not bind artifact %s / %s", env, art.ContentSHA256, art.SHA256)
	}
	code, out, errout := invoke("verify-signature", "--evidence", artifactPath, "--signature", sigPath)
	if code != 0 || errout != "" || !strings.Contains(out, `"status": "SIGNATURE_VALID"`) || !strings.Contains(out, kg.Fingerprint) {
		t.Fatalf("verify-signature: %d %s %s", code, out, errout)
	}
	if code, _, _ := invoke("verify-signature", "--evidence", artifactPath, "--signature", sigPath, "--fingerprint", kg.Fingerprint); code != 0 {
		t.Fatalf("verify with correct fingerprint pin: %d", code)
	}
	pubPath := put(t, dir, "signing.pub", kg.PublicKey+"\n")
	if code, _, errout := invoke("verify-signature", "--evidence", artifactPath, "--signature", sigPath, "--pubkey", pubPath); code != 0 {
		t.Fatalf("verify with public key file: %s", errout)
	}
	// The envelope that travels carries no private key material.
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	sigBytes, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sigBytes), strings.TrimSpace(string(keyBytes))) {
		t.Fatal("signature file contains the private key")
	}
}

// Acceptance 1 (CLI): the same round trip for an analysis artifact.
func TestSignVerifyAnalysisRoundTrip(t *testing.T) {
	dir := t.TempDir()
	b := put(t, dir, "before.cfg", "ip access-list extended EDGE\n permit tcp any any eq 443\n")
	a := put(t, dir, "after.cfg", "ip access-list extended EDGE\n permit tcp any any eq 443\n permit udp any any eq 53\n")
	code, body, errout := invoke("analyze", "--before", b, "--after", a)
	if code != 0 || errout != "" {
		t.Fatalf("analyze: %d %s", code, errout)
	}
	artifactPath := put(t, dir, "analysis.json", body)
	keyPath, _ := makeSigningKey(t, dir)
	sigPath := filepath.Join(dir, "analysis.sig.json")
	env := signArtifact(t, keyPath, artifactPath, sigPath)
	if !strings.HasSuffix(env.ArtifactSchema, "/v3") {
		t.Fatalf("envelope artifact schema: %+v", env)
	}
	code, out, errout := invoke("verify-signature", "--evidence", artifactPath, "--signature", sigPath)
	if code != 0 || !strings.Contains(out, `"status": "SIGNATURE_VALID"`) {
		t.Fatalf("verify-signature: %d %s %s", code, out, errout)
	}
}

// Acceptance 2+3 (CLI): a tampered artifact and another artifact
// presented under the first one's envelope both fail with the
// digest-mismatch code; a wrong fingerprint pin fails as a
// signature mismatch.
func TestVerifySignatureRejectsTamperedAndSwapped(t *testing.T) {
	dir := t.TempDir()
	_, _, artifactPath := diffArtifact(t, dir)
	keyPath, _ := makeSigningKey(t, dir)
	sigPath := filepath.Join(dir, "comparison.sig.json")
	signArtifact(t, keyPath, artifactPath, sigPath)

	// Tamper with provenance only (build commit) and leave the
	// recorded digests stale: the recomputed pair moves away from
	// the envelope.
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	var art evidence.Comparison
	if err := jsonio.Decode(data, evidence.MaxComparisonBytes, &art); err != nil {
		t.Fatal(err)
	}
	art.Tool.Commit = "tampered-build"
	tamperedBytes, err := json.Marshal(art)
	if err != nil {
		t.Fatal(err)
	}
	tamperedPath := put(t, dir, "tampered.json", string(tamperedBytes))
	code, _, errout := invoke("verify-signature", "--evidence", tamperedPath, "--signature", sigPath)
	if code != 2 || !strings.Contains(errout, `"code":"SIGNATURE_DIGEST_MISMATCH"`) {
		t.Fatalf("tampered artifact: %d %s", code, errout)
	}

	// A different artifact under this envelope. (A second run of
	// the same diff would produce the identical artifact — the
	// tool is deterministic — so the pair itself must differ.)
	otherDir := t.TempDir()
	ob := put(t, otherDir, "before.acl", "ip access-list extended DEMO\n permit tcp any any eq 22\n")
	oa := put(t, otherDir, "after.acl", "ip access-list extended DEMO\n permit tcp any any eq 80\n")
	ocode, obody, oerrout := invoke("diff", "--before", ob, "--after", oa)
	if ocode != 1 || oerrout != "" {
		t.Fatalf("other diff: %d %s", ocode, oerrout)
	}
	otherPath := put(t, otherDir, "comparison.json", obody)
	code, _, errout = invoke("verify-signature", "--evidence", otherPath, "--signature", sigPath)
	if code != 2 || !strings.Contains(errout, `"code":"SIGNATURE_DIGEST_MISMATCH"`) {
		t.Fatalf("swapped artifact: %d %s", code, errout)
	}

	// Wrong signer pin.
	code, _, errout = invoke("verify-signature", "--evidence", artifactPath, "--signature", sigPath,
		"--fingerprint", strings.Repeat("0", 64))
	if code != 2 || !strings.Contains(errout, `"code":"SIGNATURE_MISMATCH"`) {
		t.Fatalf("wrong fingerprint pin: %d %s", code, errout)
	}
}

// Malformed inputs across all three commands stay typed.
func TestSigningCommandsRejectMalformedInputs(t *testing.T) {
	dir := t.TempDir()
	_, _, artifactPath := diffArtifact(t, dir)
	keyPath, _ := makeSigningKey(t, dir)

	badKey := put(t, dir, "bad.key", "not a key\n")
	code, _, errout := invoke("sign", "--key", badKey, "--evidence", artifactPath)
	if code != 2 || !strings.Contains(errout, `"code":"SIGN_KEY_INVALID"`) {
		t.Fatalf("sign with garbage key: %d %s", code, errout)
	}

	sigPath := filepath.Join(dir, "comparison.sig.json")
	signArtifact(t, keyPath, artifactPath, sigPath)
	badSig := put(t, dir, "bad.sig.json", "{}")
	code, _, errout = invoke("verify-signature", "--evidence", artifactPath, "--signature", badSig)
	if code != 2 || !strings.Contains(errout, `"code":"SIGNATURE_INVALID"`) {
		t.Fatalf("verify with empty envelope: %d %s", code, errout)
	}

	// Corrupt the signature bytes of a real envelope.
	env := signArtifact(t, keyPath, artifactPath, filepath.Join(dir, "ok.sig.json"))
	env.Signature = base64.StdEncoding.EncodeToString(make([]byte, 64))
	raw, err := signature.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	corruptPath := put(t, dir, "corrupt.sig.json", string(raw))
	code, _, errout = invoke("verify-signature", "--evidence", artifactPath, "--signature", corruptPath)
	if code != 2 || !strings.Contains(errout, `"code":"SIGNATURE_MISMATCH"`) {
		t.Fatalf("verify with zeroed signature: %d %s", code, errout)
	}
}

// Artifacts without the digest pair (here a trace) cannot be
// signed: there is no content digest to bind.
func TestSignRefusesArtifactWithoutDigestPair(t *testing.T) {
	dir := t.TempDir()
	config := put(t, dir, "acl.cfg", "ip access-list extended DEMO\n deny ip any any\n")
	packet := put(t, dir, "packet.json", `{"protocol":6,"src_ip":"192.0.2.10","dst_ip":"198.51.100.20","src_port":40000,"dst_port":22}`)
	code, body, errout := invoke("trace", "--config", config, "--packet", packet)
	if code != 0 || errout != "" {
		t.Fatalf("trace: %d %s", code, errout)
	}
	tracePath := put(t, dir, "trace.json", body)
	keyPath, _ := makeSigningKey(t, dir)
	code, _, errout = invoke("sign", "--key", keyPath, "--evidence", tracePath)
	if code != 2 || !strings.Contains(errout, `"code":"SIGN_ARTIFACT_UNSUPPORTED"`) {
		t.Fatalf("signing a trace: %d %s", code, errout)
	}
}

// Acceptance 4 — verdict independence, composed at the CLI surface
// where a caller would meet both features. Replay has no signature
// input and its outcome is asserted unchanged across three states
// of the signature world: no envelope at all, a valid envelope, and
// an envelope whose signature is invalid. verify-signature reports
// the invalid state on its own surface; it cannot reach into
// replay. (Structurally: no replay code path imports the signature
// package — the CLI dispatch below is the only composition point,
// and it keeps the two strictly separate.)
func TestSignatureNeverGatesReplay(t *testing.T) {
	dir := t.TempDir()
	before, after, artifactPath := diffArtifact(t, dir)

	// Absent signature: replay matches.
	code, out, errout := invoke("replay", "--before", before, "--after", after, "--evidence", artifactPath)
	if code != 0 || !strings.Contains(out, "COMPARISON_REPLAYED") {
		t.Fatalf("replay without signature: %d %s %s", code, out, errout)
	}

	// Invalid signature: verify-signature says so, replay is
	// unmoved.
	keyPath, _ := makeSigningKey(t, dir)
	sigPath := filepath.Join(dir, "comparison.sig.json")
	env := signArtifact(t, keyPath, artifactPath, sigPath)
	env.Signature = base64.StdEncoding.EncodeToString(make([]byte, 64))
	raw, err := signature.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	put(t, dir, "comparison.sig.json", string(raw))
	code, _, errout = invoke("verify-signature", "--evidence", artifactPath, "--signature", sigPath)
	if code == 0 || !strings.Contains(errout, `"code":"SIGNATURE_MISMATCH"`) {
		t.Fatalf("verify of corrupted envelope: %d %s", code, errout)
	}
	code, out, errout = invoke("replay", "--before", before, "--after", after, "--evidence", artifactPath)
	if code != 0 || !strings.Contains(out, "COMPARISON_REPLAYED") {
		t.Fatalf("replay after invalid signature: %d %s %s", code, out, errout)
	}

	// Same independence on the analysis pair: a wrong signer pin
	// fails verification while analyze-replay still matches.
	b := put(t, dir, "before.cfg", "ip access-list extended EDGE\n permit tcp any any eq 443\n")
	a := put(t, dir, "after.cfg", "ip access-list extended EDGE\n permit tcp any any eq 443\n permit udp any any eq 53\n")
	code, body, errout := invoke("analyze", "--before", b, "--after", a)
	if code != 0 || errout != "" {
		t.Fatalf("analyze: %d %s", code, errout)
	}
	analysisPath := put(t, dir, "analysis.json", body)
	analysisSig := filepath.Join(dir, "analysis.sig.json")
	signArtifact(t, keyPath, analysisPath, analysisSig)
	code, _, errout = invoke("verify-signature", "--evidence", analysisPath, "--signature", analysisSig,
		"--fingerprint", strings.Repeat("0", 64))
	if code == 0 || !strings.Contains(errout, `"code":"SIGNATURE_MISMATCH"`) {
		t.Fatalf("verify analysis with wrong pin: %d %s", code, errout)
	}
	code, out, errout = invoke("analyze-replay", "--before", b, "--after", a, "--evidence", analysisPath)
	if code != 0 || !strings.Contains(out, "ANALYSIS_REPLAYED") {
		t.Fatalf("analyze-replay after failed verification: %d %s %s", code, out, errout)
	}
}

// Package signature implements detached Ed25519 signatures over
// evidence (plenilune.signature/v1, P2-5). A signature envelope binds
// ONE sealed artifact by its P2-1 digest pair — content_sha256 (the
// semantic payload) and the whole-envelope artifact digest — plus the
// signer's public key. Both digests are always bound together:
// signing only the content digest would let a signature travel to a
// different printing of the same result, and signing only the
// envelope digest would bind build provenance instead of the result.
//
// A signature proves exactly one thing: the holder of the private key
// signed these digests. It does not prove the signer's identity, that
// anyone approved the change, that the analysis is correct, or when
// the signing happened — the envelope deliberately carries no
// timestamp and no identity claim. Verification therefore never
// feeds a verdict, a seal check or a replay outcome: it is reported
// alongside them, and an artifact with an invalid or absent signature
// replays exactly as one with a valid signature. The private key
// never enters this package's outputs: keys are generated and held by
// the signer (a local user standalone; MoonStorm when integrated),
// and only public keys and signatures appear in envelopes.
//
// Only artifacts that carry the digest pair can be signed: evidence
// comparisons at plenilune.evidence/v2 and analysis artifacts at
// plenilune.analysis/v3. Frozen artifacts (evidence v1, analysis
// v1/v2) have no content digest to bind, so signing them is refused
// rather than silently downgraded to a single-digest signature; they
// keep replaying under their original rules, unsigned.
//
// This package is pure: it works on artifact bytes and key material.
// File I/O (key files, artifact files) belongs to the surfaces.
package signature

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"plenilune/internal/analysis"
	"plenilune/internal/evidence"
	"plenilune/internal/jsonio"
)

// Schema is the signature envelope schema id.
const Schema = "plenilune.signature/v1"

// MaxEnvelopeBytes bounds an envelope: three short base64 fields and
// schema/digest strings, so 16 KiB is generous and still refuses
// absurd inputs before allocation.
const MaxEnvelopeBytes = 16 << 10

// StatusValid is the only success status a verification reports.
const StatusValid = "SIGNATURE_VALID"

// Envelope is the detached signature document. It is not part of the
// artifact it binds and never modifies it; it can be stored and
// transported separately, exactly like a bundle manifest.
type Envelope struct {
	Schema         string `json:"schema"`
	ArtifactSchema string `json:"artifact_schema"`
	ContentSHA256  string `json:"content_sha256"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	PublicKey      string `json:"public_key"`
	Fingerprint    string `json:"fingerprint"`
	Signature      string `json:"signature"`
}

// Result is a successful verification: the artifact identity the
// signature was checked against (recomputed from the artifact, never
// copied out of the envelope) and the signer's fingerprint.
type Result struct {
	Status         string `json:"status"`
	ArtifactSchema string `json:"artifact_schema"`
	ContentSHA256  string `json:"content_sha256"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	Fingerprint    string `json:"fingerprint"`
}

// Error is the signature package's typed error. Code is a published
// taxonomy code (registered in errtax and capabilities).
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func typed(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Message is the canonical byte string a signature covers:
// domain-separated by the envelope schema, then the artifact schema
// and both digests, one field per line. Signing digest strings (not
// raw file bytes) is what binds the signature to the artifact's
// identities instead of to one printing of it; the artifact schema
// line keeps a signature from being reinterpreted across artifact
// kinds. The exact layout is contractual (CONTRACTS §4) — changing
// it invalidates every existing signature.
func Message(artifactSchema, contentSHA256, artifactSHA256 string) []byte {
	return []byte(Schema + "\n" + artifactSchema + "\n" + contentSHA256 + "\n" + artifactSHA256 + "\n")
}

// Fingerprint identifies a public key: the SHA-256 of the raw public
// key bytes, hex-encoded — the same digest shape the rest of the
// tool prints, so a fingerprint can be pinned and compared by eye.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// EncodePrivateKey renders key material as one base64 line, the
// private key file format. The output is secret: callers write it to
// a 0600 file and never print it.
func EncodePrivateKey(priv ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(priv)
}

// EncodePublicKey renders a public key as one base64 line, the
// public key file format accepted by verify.
func EncodePublicKey(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}

func decodeKeyText(text string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(trimASCII(text))
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func trimASCII(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// ParsePrivateKey reads private key material in the key file format:
// base64 of the 64-byte Ed25519 private key, or of its 32-byte seed.
// Anything else is SIGN_KEY_INVALID. The parsed key is the only place
// private material exists in this package; nothing here logs or
// serializes it.
func ParsePrivateKey(text string) (ed25519.PrivateKey, error) {
	raw, err := decodeKeyText(text)
	if err != nil {
		return nil, typed("SIGN_KEY_INVALID", "private key file is not base64 key material")
	}
	switch len(raw) {
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	}
	return nil, typed("SIGN_KEY_INVALID", "private key file must hold a %d-byte Ed25519 private key or its %d-byte seed, got %d bytes", ed25519.PrivateKeySize, ed25519.SeedSize, len(raw))
}

// ParsePublicKey reads public key material in the key file format:
// base64 of the 32-byte Ed25519 public key.
func ParsePublicKey(text string) (ed25519.PublicKey, error) {
	raw, err := decodeKeyText(text)
	if err != nil {
		return nil, typed("SIGN_KEY_INVALID", "public key file is not base64 key material")
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, typed("SIGN_KEY_INVALID", "public key file must hold a %d-byte Ed25519 public key, got %d bytes", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// identity is one artifact's digest pair, recorded and recomputed.
// The recomputed pair is the truth: it comes from re-running the
// owning package's seal machinery over the payload, never from
// trusting the digest fields printed in the artifact.
type identity struct {
	schema          string
	recordedContent string
	recordedDigest  string
	content         string
	digest          string
}

// identify recomputes the digest pair of a signable artifact. An
// artifact whose schema carries no content digest, or that is not an
// evidence/analysis artifact at all, is SIGN_ARTIFACT_UNSUPPORTED.
// An artifact that fails its own seal machinery is EVIDENCE_MISMATCH.
// Strict-decode failures are returned unwrapped so the surface can
// map them like other malformed JSON input.
func identify(data []byte) (identity, error) {
	schema := sniffSchema(data)
	switch schema {
	case evidence.ComparisonSchema:
		var c evidence.Comparison
		if err := jsonio.Decode(data, evidence.MaxComparisonBytes, &c); err != nil {
			return identity{}, err
		}
		sealed, err := evidence.SealComparison(c)
		if err != nil {
			return identity{}, typed("EVIDENCE_MISMATCH", "comparison artifact does not pass its own seal check")
		}
		return identity{schema: schema, recordedContent: c.ContentSHA256, recordedDigest: c.SHA256, content: sealed.ContentSHA256, digest: sealed.SHA256}, nil
	case analysis.Schema:
		var r analysis.Result
		if err := jsonio.Decode(data, analysis.MaxResultBytes, &r); err != nil {
			return identity{}, err
		}
		sealed, err := analysis.Seal(r)
		if err != nil {
			return identity{}, typed("EVIDENCE_MISMATCH", "analysis artifact does not pass its own seal check")
		}
		return identity{schema: schema, recordedContent: r.ContentSHA256, recordedDigest: r.SHA256, content: sealed.ContentSHA256, digest: sealed.SHA256}, nil
	case "":
		return identity{}, errors.New("artifact has no schema; not a Plenilune artifact")
	default:
		return identity{}, typed("SIGN_ARTIFACT_UNSUPPORTED", "artifact schema %q carries no content digest to sign; only %s and %s artifacts can be signed", schema, evidence.ComparisonSchema, analysis.Schema)
	}
}

// sniffSchema leniently reads only the schema field to dispatch an
// artifact to its owning package. The artifact itself is then
// strictly decoded and fully verified by that owner; this sniff
// never trusts the bytes.
func sniffSchema(data []byte) string {
	var probe struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return ""
	}
	return probe.Schema
}

// ArtifactDigests recomputes the digest pair of a signable artifact
// from its payload. The artifact's own recorded digests must equal
// the recomputed pair; an artifact that does not check out against
// itself is EVIDENCE_MISMATCH and has no trustworthy identity.
func ArtifactDigests(data []byte) (artifactSchema, contentSHA256, artifactSHA256 string, err error) {
	id, err := identify(data)
	if err != nil {
		return "", "", "", err
	}
	if id.recordedContent != id.content || id.recordedDigest != id.digest {
		return "", "", "", typed("EVIDENCE_MISMATCH", "artifact digests do not recompute from its payload")
	}
	return id.schema, id.content, id.digest, nil
}

// Sign produces the detached envelope binding an artifact's digest
// pair to priv's key. The artifact must be sealed and check out
// against itself first (ArtifactDigests): Plenilune does not sign
// artifacts whose digests do not recompute.
func Sign(priv ed25519.PrivateKey, artifact []byte) (Envelope, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return Envelope{}, typed("SIGN_KEY_INVALID", "private key has the wrong size")
	}
	schema, content, digest, err := ArtifactDigests(artifact)
	if err != nil {
		return Envelope{}, err
	}
	pub := priv.Public().(ed25519.PublicKey)
	sig := ed25519.Sign(priv, Message(schema, content, digest))
	return Envelope{
		Schema:         Schema,
		ArtifactSchema: schema,
		ContentSHA256:  content,
		ArtifactSHA256: digest,
		PublicKey:      EncodePublicKey(pub),
		Fingerprint:    Fingerprint(pub),
		Signature:      base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// BuildBytes returns the canonical envelope encoding: indented JSON
// with a trailing newline — the bytes surfaces write to a signature
// file.
func Marshal(env Envelope) ([]byte, error) {
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Parse strictly decodes an envelope and validates it fully: schema,
// a signable artifact schema, well-formed digests, a public key whose
// fingerprint matches the recorded fingerprint, and a signature of
// the right size. A malformed envelope is SIGNATURE_INVALID.
func Parse(data []byte) (Envelope, error) {
	var env Envelope
	if err := jsonio.Decode(data, MaxEnvelopeBytes, &env); err != nil {
		return Envelope{}, typed("SIGNATURE_INVALID", "signature envelope is not valid %s JSON", Schema)
	}
	if err := validateEnvelope(env); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

func validateEnvelope(env Envelope) error {
	if env.Schema != Schema {
		return typed("SIGNATURE_INVALID", "signature envelope schema is %q, want %s", env.Schema, Schema)
	}
	if env.ArtifactSchema != evidence.ComparisonSchema && env.ArtifactSchema != analysis.Schema {
		return typed("SIGNATURE_INVALID", "signature envelope binds unsupported artifact schema %q", env.ArtifactSchema)
	}
	if !isDigest(env.ContentSHA256) || !isDigest(env.ArtifactSHA256) {
		return typed("SIGNATURE_INVALID", "signature envelope carries a malformed digest")
	}
	pub, err := base64.StdEncoding.DecodeString(env.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return typed("SIGNATURE_INVALID", "signature envelope carries a malformed public key")
	}
	if env.Fingerprint != Fingerprint(ed25519.PublicKey(pub)) {
		return typed("SIGNATURE_INVALID", "signature envelope fingerprint does not match its public key")
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return typed("SIGNATURE_INVALID", "signature envelope carries a malformed signature")
	}
	return nil
}

func isDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Verify checks an envelope against an artifact. The artifact's
// digests are recomputed from its payload — the envelope's printed
// digests are claims, checked against that truth, never trusted:
//
//   - envelope pair ≠ recomputed pair → SIGNATURE_DIGEST_MISMATCH
//     (tampered artifact, or the envelope of a different artifact);
//   - the artifact's own recorded digests ≠ recomputed pair →
//     EVIDENCE_MISMATCH (the artifact fails its own seal check);
//   - the Ed25519 signature does not verify under the envelope's
//     public key, or a non-empty pinnedFingerprint names a different
//     signer → SIGNATURE_MISMATCH.
//
// A valid result attests key possession for those digests only; it
// says nothing about the verdict inside the artifact, and no caller
// may use it to gate one.
func Verify(artifact []byte, env Envelope, pinnedFingerprint string) (Result, error) {
	if err := validateEnvelope(env); err != nil {
		return Result{}, err
	}
	id, err := identify(artifact)
	if err != nil {
		return Result{}, err
	}
	if id.schema != env.ArtifactSchema || id.content != env.ContentSHA256 || id.digest != env.ArtifactSHA256 {
		return Result{}, typed("SIGNATURE_DIGEST_MISMATCH", "artifact digests do not match the digests the signature binds")
	}
	if id.recordedContent != id.content || id.recordedDigest != id.digest {
		return Result{}, typed("EVIDENCE_MISMATCH", "artifact digests do not recompute from its payload")
	}
	pub, _ := base64.StdEncoding.DecodeString(env.PublicKey)
	sig, _ := base64.StdEncoding.DecodeString(env.Signature)
	if !ed25519.Verify(ed25519.PublicKey(pub), Message(env.ArtifactSchema, env.ContentSHA256, env.ArtifactSHA256), sig) {
		return Result{}, typed("SIGNATURE_MISMATCH", "signature does not verify for the public key in the envelope")
	}
	if pinnedFingerprint != "" && pinnedFingerprint != env.Fingerprint {
		return Result{}, typed("SIGNATURE_MISMATCH", "signer fingerprint %s does not match the pinned fingerprint", env.Fingerprint)
	}
	return Result{
		Status:         StatusValid,
		ArtifactSchema: id.schema,
		ContentSHA256:  id.content,
		ArtifactSHA256: id.digest,
		Fingerprint:    env.Fingerprint,
	}, nil
}

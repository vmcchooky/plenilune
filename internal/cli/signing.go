package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"flag"
	"io"
	"os"

	"plenilune/internal/analysis"
	"plenilune/internal/errtax"
	"plenilune/internal/signature"
)

// Evidence signing commands (P2-5). sign-keygen creates an Ed25519
// keypair and writes ONLY the private key to a caller-named file
// (0600, never overwritten); the fingerprint and public key go to
// stdout so the signer can publish them. sign produces a detached
// plenilune.signature/v1 envelope binding a sealed artifact's
// content_sha256 AND artifact sha256; verify-signature recomputes
// the artifact's digests from its payload and checks the envelope
// against them. Key material lives with the signer — standalone a
// local user, with MoonStorm the MoonStorm side, which is why verify
// needs only the envelope plus an optional pinned fingerprint or
// public key. No command here reads a verdict, and no replay command
// consults a signature: verification is reported alongside replay,
// never instead of it and never as its gate.

// maxKeyFileBytes bounds a key file read: base64 of a 64-byte key
// plus whitespace is far below this.
const maxKeyFileBytes = 4096

// failSignature emits a signature.Error as the shared taxonomy
// envelope; anything else is a malformed-JSON failure, mapped like
// the other strict decode failures (bundle-replay precedent).
func failSignature(stderr io.Writer, err error) int {
	var se *signature.Error
	if errors.As(err, &se) {
		return outputError(stderr, errtax.For(se.Code, se.Message))
	}
	return fail(stderr, "INVALID_JSON", err.Error())
}

func runSignKeygen(ctx context.Context, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet("sign-keygen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var outPath string
	fs.StringVar(&outPath, "out", "", "file that receives the new private key (must not exist)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid sign-keygen arguments")
	}
	if fs.NArg() != 0 || outPath == "" {
		return fail(stderr, "USAGE", "sign-keygen requires --out")
	}
	if ctx.Err() != nil {
		return fail(stderr, "CANCELED", "operation canceled")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fail(stderr, "OUTPUT_FAILED", "cannot generate signing key")
	}
	// O_EXCL: an existing file is never overwritten — silently
	// replacing a private key would orphan every signature it made.
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fail(stderr, "SIGN_KEY_INVALID", "key file already exists; refusing to overwrite a private key")
		}
		return fail(stderr, "OUTPUT_FAILED", "cannot create key file")
	}
	_, writeErr := io.WriteString(f, signature.EncodePrivateKey(priv)+"\n")
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return fail(stderr, "OUTPUT_FAILED", "cannot write key file")
	}
	// Only public material is printed: the fingerprint a verifier
	// pins and the public key a verifier may load. The private key
	// exists solely inside the file just written.
	return output(out, stderr, struct {
		Status      string `json:"status"`
		Fingerprint string `json:"fingerprint"`
		PublicKey   string `json:"public_key"`
	}{"KEY_GENERATED", signature.Fingerprint(pub), signature.EncodePublicKey(pub)})
}

func runSign(ctx context.Context, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var keyPath, evidencePath, outPath string
	fs.StringVar(&keyPath, "key", "", "private key file written by sign-keygen")
	fs.StringVar(&evidencePath, "evidence", "", "sealed evidence (v2) or analysis (v3) artifact file")
	fs.StringVar(&outPath, "out", "", "optional file that also receives the signature envelope")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid sign arguments")
	}
	if fs.NArg() != 0 || keyPath == "" || evidencePath == "" {
		return fail(stderr, "USAGE", "sign requires --key and --evidence")
	}
	keyData, err := read(ctx, keyPath, maxKeyFileBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	priv, err := signature.ParsePrivateKey(string(keyData))
	if err != nil {
		return failSignature(stderr, err)
	}
	data, err := read(ctx, evidencePath, analysis.MaxResultBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	env, err := signature.Sign(priv, data)
	if err != nil {
		return failSignature(stderr, err)
	}
	if outPath != "" {
		b, err := signature.Marshal(env)
		if err != nil {
			return fail(stderr, "OUTPUT_FAILED", "cannot encode signature envelope")
		}
		if err := os.WriteFile(outPath, b, 0o600); err != nil {
			return fail(stderr, "OUTPUT_FAILED", "cannot write signature file")
		}
	}
	return output(out, stderr, env)
}

func runVerifySignature(ctx context.Context, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify-signature", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var evidencePath, signaturePath, fingerprintFlag, pubkeyPath string
	fs.StringVar(&evidencePath, "evidence", "", "sealed evidence (v2) or analysis (v3) artifact file")
	fs.StringVar(&signaturePath, "signature", "", "signature envelope file written by sign")
	fs.StringVar(&fingerprintFlag, "fingerprint", "", "optional pinned signer fingerprint (hex SHA-256 of the public key)")
	fs.StringVar(&pubkeyPath, "pubkey", "", "optional file holding the signer's base64 public key; the envelope key must match it")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid verify-signature arguments")
	}
	if fs.NArg() != 0 || evidencePath == "" || signaturePath == "" {
		return fail(stderr, "USAGE", "verify-signature requires --evidence and --signature")
	}
	sigData, err := read(ctx, signaturePath, signature.MaxEnvelopeBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	env, err := signature.Parse(sigData)
	if err != nil {
		return failSignature(stderr, err)
	}
	pinned := fingerprintFlag
	if pubkeyPath != "" {
		pubData, err := read(ctx, pubkeyPath, maxKeyFileBytes)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		pub, err := signature.ParsePublicKey(string(pubData))
		if err != nil {
			return failSignature(stderr, err)
		}
		fp := signature.Fingerprint(pub)
		if pinned != "" && pinned != fp {
			return outputError(stderr, errtax.For("SIGNATURE_MISMATCH", "pinned fingerprint does not match the public key file"))
		}
		pinned = fp
	}
	data, err := read(ctx, evidencePath, analysis.MaxResultBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	res, err := signature.Verify(data, env, pinned)
	if err != nil {
		return failSignature(stderr, err)
	}
	return output(out, stderr, res)
}

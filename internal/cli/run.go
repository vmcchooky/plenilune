// Package cli owns local I/O and orchestration. Provider access requires assist --ai on.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/errtax"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
	"reflect"
	"runtime"
	"runtime/debug"
)

const Version = evidence.ToolVersion

const help = `Plenilune — semantic diff and change-intent verification for network configuration

Commands:
  analyze --before FILE --after FILE [--select domain:name,...]
  analyze --request FILE|-          Inventory two configuration versions, compare
                                     supported objects (acl, prefix-list) and emit
                                     plenilune.analysis/v3; - reads stdin
  analyze-replay --before FILE --after FILE --evidence FILE
  analyze-replay --request FILE|-   Recompute an analysis artifact offline
  version                            Print tool, build and parser versions
  capabilities [--json]              Print the capabilities document
                                     (plenilune.capabilities/v1): supported schemas,
                                     domains, limits, options, features, error codes.
                                     Callers check compatibility BEFORE submitting work
  parse --config FILE                Parse one named extended ACL-only file
  trace --config FILE --packet FILE  Evaluate one IPv4 packet; emit trace JSON
  verify --config FILE --evidence FILE
                                     Check trace digest and replay the packet
  obligations --file FILE            Validate/normalize conditions and hash them
  diff --before FILE --after FILE [--obligations FILE]
                                     Exact comparison and full evidence JSON
  diff --brief --out FILE --before FILE --after FILE [--obligations FILE]
                                     Write the full evidence JSON to FILE and print
                                     a five-line summary: verdict, object status,
                                     obligations, limitations, artifact path
  replay --before FILE --after FILE --evidence FILE
                                     Replay full comparison evidence offline
  repair --before FILE --after FILE --policy FILE --patch FILE
                                     Verify an in-memory patch and collateral
  repair-replay --before FILE --after FILE --evidence FILE
                                     Rerender and replay repair evidence offline
  repair-enumerate --before FILE --after FILE [--policy FILE] [--proposals FILE]
                                     Deterministically enumerate repair candidates
                                     from change classes and obligation violations;
                                     every candidate carries its repair receipt.
                                     Optional proposals FILE (JSON array of patch
                                     requests, e.g. AI proposals) folds into the
                                     same deterministic order
  bundle-export --out DIR --before FILE --after FILE --comparison FILE [--repair FILE] [--report FILE] [--instructions FILE]
                                     Export one review case as a bundle directory:
                                     the case files under canonical names plus a
                                     plenilune.bundle/v1 manifest (SHA-256 per
                                     file). The manifest is an integrity index;
                                     the receipts inside the files stay the proof
  bundle-replay --dir DIR            Verify a bundle directory against its
                                     manifest, then replay the comparison/analysis
                                     and repair artifacts inside; exits 0 when
                                     everything replays, 2 on a bundle integrity
                                     failure or a timing-sensitive replay
  sign-keygen --out FILE             Generate an Ed25519 signing key: the
                                     private key is written to FILE (0600,
                                     never overwritten); only the fingerprint
                                     and public key are printed
  sign --key FILE --evidence FILE [--out FILE]
                                     Detached signature (plenilune.signature/v1)
                                     binding BOTH digests of a sealed evidence
                                     (v2) or analysis (v3) artifact:
                                     content_sha256 and the artifact sha256
  verify-signature --evidence FILE --signature FILE [--fingerprint HEX] [--pubkey FILE]
                                     Verify a detached signature: the artifact
                                     digests are recomputed from the payload,
                                     never trusted from the envelope; exits 0
                                     SIGNATURE_VALID. A signature attests key
                                     possession only — it never changes a
                                     verdict or a replay outcome
  audit --config FILE                Exact audit of one ACL: unreachable and
                                     redundant rules (exact findings) plus
                                     generalization/correlation relations
                                     (heuristic layer), as a sealed record;
                                     report-only, never edits the config
  audit-replay --config FILE --evidence FILE
                                     Recompute an audit record offline
  assist --before FILE --after FILE [--policy FILE] --intent TEXT --ai off|on
                                     Real engine tool loop; AI defaults off

Only assist --ai on calls Meta with ACL excerpts/intent; all other commands are offline.
Analyze exits 0 when an artifact is produced (any object status); analyze-replay
exits 0 ANALYSIS_REPLAYED, 2 TIMING_SENSITIVE/error. Errors are JSON on stderr.
Diff exits: 0 EQUIVALENT, 1 DIFFERENT, 2 INCONCLUSIVE/error.
Diff --brief exits: 0 EQUIVALENT, 1 DIFFERENT, 2 INCONCLUSIVE, 3 error;
brief errors are a single line on stderr, and --out is required with --brief.
Repair exits: 0 VERIFIED, 1 REJECTED, 2 UNVERIFIED/error.
Repair-enumerate exits: 0 complete with a VERIFIED candidate, 1 complete
with none, 2 incomplete/error. Audit exits: 0 complete, 2 incomplete/error;
audit-replay exits 0 AUDIT_REPLAYED, 2 TIMING_SENSITIVE/error.
Other commands: 0/2.
Use --help after any command for its flags.
`

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx.Err() != nil {
		return fail(stderr, "CANCELED", "operation canceled")
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		if _, err := io.WriteString(stdout, help); err != nil {
			return fail(stderr, "OUTPUT_FAILED", "cannot write output")
		}
		return 0
	}
	cmd := args[0]
	if cmd == "analyze" || cmd == "analyze-replay" {
		return runAnalysis(ctx, cmd, args[1:], stdinReader, stdout, stderr)
	}
	if cmd == "diff" || cmd == "replay" {
		return runComparison(ctx, cmd, args[1:], stdout, stderr)
	}
	if cmd == "repair" || cmd == "repair-replay" || cmd == "assist" {
		return runRepair(ctx, cmd, args[1:], stdout, stderr)
	}
	if cmd == "repair-enumerate" {
		return runRepairEnumerate(ctx, args[1:], stdout, stderr)
	}
	if cmd == "bundle-export" {
		return runBundleExport(ctx, args[1:], stdout, stderr)
	}
	if cmd == "bundle-replay" {
		return runBundleReplay(ctx, args[1:], stdout, stderr)
	}
	if cmd == "capabilities" {
		return runCapabilities(args[1:], stdout, stderr)
	}
	if cmd == "sign-keygen" {
		return runSignKeygen(ctx, args[1:], stdout, stderr)
	}
	if cmd == "sign" {
		return runSign(ctx, args[1:], stdout, stderr)
	}
	if cmd == "verify-signature" {
		return runVerifySignature(ctx, args[1:], stdout, stderr)
	}
	if cmd == "audit" {
		return runAudit(ctx, args[1:], stdout, stderr)
	}
	if cmd == "audit-replay" {
		return runAuditReplay(ctx, args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath, packetPath, evidencePath, filePath string
	switch cmd {
	case "parse":
		fs.StringVar(&configPath, "config", "", "ACL-only input file")
	case "trace":
		fs.StringVar(&configPath, "config", "", "ACL-only input file")
		fs.StringVar(&packetPath, "packet", "", "packet JSON file")
	case "verify":
		fs.StringVar(&configPath, "config", "", "original ACL-only input file")
		fs.StringVar(&evidencePath, "evidence", "", "trace JSON file")
	case "obligations":
		fs.StringVar(&filePath, "file", "", "structured condition JSON file")
	case "version":
	default:
		return fail(stderr, "USAGE", "unknown command; use --help")
	}
	fs.Usage = func() { fs.SetOutput(stdout); fs.PrintDefaults() }
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.Usage()
			return 0
		}
		return fail(stderr, "USAGE", "invalid arguments; use --help")
	}
	if fs.NArg() != 0 {
		return fail(stderr, "USAGE", "unexpected positional argument")
	}
	if cmd == "version" {
		return output(stdout, stderr, buildVersion())
	}
	if cmd == "obligations" {
		if filePath == "" {
			return fail(stderr, "USAGE", "--file is required")
		}
		data, err := read(ctx, filePath, intent.MaxJSONBytes)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		var s intent.Set
		if err := jsonio.Decode(data, intent.MaxJSONBytes, &s); err != nil {
			var domain *jsonio.ValidationError
			if errors.As(err, &domain) {
				return fail(stderr, "INVALID_OBLIGATION", domain.Error())
			}
			return fail(stderr, "INVALID_JSON", err.Error())
		}
		n, h, err := intent.Digest(s)
		if err != nil {
			return fail(stderr, "INVALID_OBLIGATION", err.Error())
		}
		return output(stdout, stderr, struct {
			Status     string     `json:"status"`
			Conditions intent.Set `json:"conditions"`
			SHA256     string     `json:"sha256"`
		}{"VALIDATED_NOT_EVALUATED", n, h})
	}
	if configPath == "" || (cmd == "trace" && packetPath == "") || (cmd == "verify" && evidencePath == "") {
		return fail(stderr, "USAGE", "required input flag is missing")
	}
	config, err := read(ctx, configPath, ciscoacl.MaxInputBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	acl, err := ciscoacl.Parse(config)
	if err != nil {
		var pe *ciscoacl.Error
		if errors.As(err, &pe) {
			return outputError(stderr, pe)
		}
		return fail(stderr, "PARSE_FAILED", "ACL parse failed")
	}
	if ctx.Err() != nil {
		return fail(stderr, "CANCELED", "operation canceled")
	}
	if cmd == "parse" {
		return output(stdout, stderr, acl)
	}
	var p ciscoacl.Packet
	if cmd == "trace" {
		data, err := read(ctx, packetPath, 4096)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		p, err = decodePacket(data)
		if err != nil {
			return fail(stderr, "INVALID_PACKET", err.Error())
		}
	} else {
		data, err := read(ctx, evidencePath, evidence.MaxTraceBytes)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		var t evidence.Trace
		if err := jsonio.Decode(data, evidence.MaxTraceBytes, &t); err != nil {
			return fail(stderr, "INVALID_JSON", err.Error())
		}
		if err := evidence.CheckIntegrity(t); err != nil {
			return fail(stderr, "EVIDENCE_MISMATCH", err.Error())
		}
		if t.ConfigSHA256 != evidence.ConfigDigest(config) {
			return fail(stderr, "EVIDENCE_MISMATCH", "configuration bytes do not match trace")
		}
		d, err := acl.Evaluate(t.Packet)
		if err != nil || !reflect.DeepEqual(d, t.Decision) {
			return fail(stderr, "EVIDENCE_MISMATCH", "replayed decision does not match trace")
		}
		return output(stdout, stderr, struct {
			Status string `json:"status"`
			SHA256 string `json:"sha256"`
			Scope  string `json:"scope"`
		}{"TRACE_REPLAYED", t.SHA256, t.Scope})
	}
	d, err := acl.Evaluate(p)
	if err != nil {
		return fail(stderr, "INVALID_PACKET", "packet is outside supported scope")
	}
	t, err := evidence.SealTrace(evidence.ConfigDigest(config), p, d)
	if err != nil {
		return fail(stderr, "EVIDENCE_FAILED", "cannot seal trace")
	}
	return output(stdout, stderr, t)
}

func read(ctx context.Context, path string, limit int) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, errors.New("operation canceled")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open input file")
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		if closeErr := f.Close(); closeErr != nil {
			return nil, errors.New("cannot close input")
		}
		return nil, errors.New("input must be a regular file")
	}
	data, readErr := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.New("cannot read input file")
	}
	if len(data) > limit {
		return nil, errors.New("input exceeds size limit")
	}
	if ctx.Err() != nil {
		return nil, errors.New("operation canceled")
	}
	return data, nil
}

func decodePacket(data []byte) (ciscoacl.Packet, error) {
	var p ciscoacl.Packet
	if err := jsonio.Decode(data, 4096, &p); err != nil {
		return ciscoacl.Packet{}, err
	}
	return p, nil
}

func output(out, stderr io.Writer, value any) int {
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	if err := e.Encode(value); err != nil {
		return fail(stderr, "OUTPUT_FAILED", "cannot write output")
	}
	return 0
}

func outputError(stderr io.Writer, value any) int {
	// Typed domain errors carrying a published code are emitted as
	// the shared taxonomy envelope (code/stage/message/retryable plus
	// line/side when known); anything else keeps its own shape.
	if err, ok := value.(error); ok {
		if env, adapted := errtax.FromError(err); adapted {
			value = env
		}
	}
	if err := json.NewEncoder(stderr).Encode(value); err != nil {
		return 2
	}
	return 2
}

func fail(stderr io.Writer, code, message string) int {
	return outputError(stderr, errtax.For(code, message))
}

func buildVersion() any {
	revision := "unversioned"
	modified := false
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
			if s.Key == "vcs.modified" {
				modified = s.Value == "true"
			}
		}
	}
	return struct {
		Tool     string `json:"tool"`
		Version  string `json:"version"`
		Commit   string `json:"commit"`
		Modified bool   `json:"modified"`
		Go       string `json:"go"`
		Parser   string `json:"parser"`
	}{"plenilune", Version, revision, modified, runtime.Version(), ciscoacl.ParserVersion}
}

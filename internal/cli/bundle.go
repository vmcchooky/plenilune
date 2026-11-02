package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"

	"plenilune/internal/analysis"
	"plenilune/internal/bundle"
	"plenilune/internal/errtax"
	"plenilune/internal/evidence"
	"plenilune/internal/jsonio"
	"plenilune/internal/repair"
	"plenilune/internal/workflow"
)

// Review bundle commands (P2-4). bundle-export writes one review
// case as a directory of files under canonical names plus
// bundle-manifest.json (plenilune.bundle/v1), the integrity index
// over the SHA-256 of every file; bundle-replay verifies a bundle
// directory against its manifest — any flipped byte or missing
// listed file is a typed bundle error naming the role — and then
// hands the verified artifacts to the existing replay machinery.
// The bundle wraps files; it never modifies the sealed artifacts
// inside them.

// maxBundleViewBytes bounds the free-text view files (report,
// instructions) a bundle export accepts.
const maxBundleViewBytes = 4 << 20

func bundleFileCap(role string) int {
	switch role {
	case bundle.RoleBefore, bundle.RoleAfter:
		return analysis.MaxSourceBytes
	case bundle.RoleComparison:
		return analysis.MaxResultBytes
	case bundle.RoleRepair:
		return repair.MaxJSONBytes
	default:
		return maxBundleViewBytes
	}
}

// failBundle emits a bundle.Error as the shared taxonomy envelope,
// carrying the failing role in the envelope's field slot so callers
// can map the failure without parsing the message.
func failBundle(stderr io.Writer, err error) int {
	var be *bundle.Error
	if errors.As(err, &be) {
		env := errtax.For(be.Code, be.Message)
		if be.Role != "" {
			env.Field = be.Role
		}
		return outputError(stderr, env)
	}
	return fail(stderr, "BUNDLE_INVALID", err.Error())
}

func runBundleExport(ctx context.Context, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet("bundle-export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var outDir string
	paths := map[string]*string{}
	fs.StringVar(&outDir, "out", "", "directory that receives the bundle (created if needed)")
	for _, role := range []string{bundle.RoleBefore, bundle.RoleAfter, bundle.RoleComparison, bundle.RoleRepair, bundle.RoleReport, bundle.RoleInstructions} {
		p := fs.String(role, "", "input file for the "+role+" role")
		paths[role] = p
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid bundle-export arguments")
	}
	if fs.NArg() != 0 || outDir == "" || *paths[bundle.RoleBefore] == "" || *paths[bundle.RoleAfter] == "" || *paths[bundle.RoleComparison] == "" {
		return fail(stderr, "USAGE", "bundle-export requires --out, --before, --after and --comparison")
	}
	files := map[string][]byte{}
	for role, p := range paths {
		if *p == "" {
			continue
		}
		data, err := read(ctx, *p, bundleFileCap(role))
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		files[role] = data
	}
	manifest, err := bundle.Build(files)
	if err != nil {
		return failBundle(stderr, err)
	}
	manifestBytes, err := bundle.Marshal(manifest)
	if err != nil {
		return fail(stderr, "OUTPUT_FAILED", "cannot encode bundle manifest")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fail(stderr, "OUTPUT_FAILED", "cannot create bundle directory")
	}
	// Files first, manifest last: a directory whose export was
	// interrupted never carries an index claiming files that are
	// not there yet.
	for _, entry := range manifest.Files {
		if err := os.WriteFile(filepath.Join(outDir, filepath.FromSlash(entry.Path)), files[entry.Role], 0o600); err != nil {
			return fail(stderr, "OUTPUT_FAILED", "cannot write bundle file "+entry.Path)
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, bundle.ManifestName), manifestBytes, 0o600); err != nil {
		return fail(stderr, "OUTPUT_FAILED", "cannot write bundle manifest")
	}
	return output(out, stderr, manifest)
}

// bundleReplayOutput is the bundle-replay result: the primary
// artifact's replay (comparison or analysis) and, when the bundle
// carries one, the repair receipt's replay. Status is the primary
// status, demoted to TIMING_SENSITIVE when the repair replay was
// timing-sensitive but the primary matched — the bundle as a whole
// has then not fully replayed.
type bundleReplayOutput struct {
	Status     string                 `json:"status"`
	Comparison *workflow.ReplayResult `json:"comparison,omitempty"`
	Analysis   *analysis.ReplayResult `json:"analysis,omitempty"`
	Repair     *workflow.ReplayResult `json:"repair,omitempty"`
}

func runBundleReplay(ctx context.Context, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet("bundle-replay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var dir string
	fs.StringVar(&dir, "dir", "", "bundle directory written by bundle-export")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid bundle-replay arguments")
	}
	if fs.NArg() != 0 || dir == "" {
		return fail(stderr, "USAGE", "bundle-replay requires --dir")
	}
	raw, err := readBundleFile(filepath.Join(dir, bundle.ManifestName), bundle.MaxManifestBytes, "")
	if err != nil {
		return failBundle(stderr, err)
	}
	manifest, err := bundle.Parse(raw)
	if err != nil {
		return failBundle(stderr, err)
	}
	files := map[string][]byte{}
	for _, entry := range manifest.Files {
		data, err := readBundleFile(filepath.Join(dir, filepath.FromSlash(entry.Path)), bundleFileCap(entry.Role), entry.Role)
		if err != nil {
			return failBundle(stderr, err)
		}
		files[entry.Role] = data
	}
	if err := manifest.Verify(files); err != nil {
		return failBundle(stderr, err)
	}
	before, after := files[bundle.RoleBefore], files[bundle.RoleAfter]
	result := bundleReplayOutput{}
	switch schema := artifactSchema(files[bundle.RoleComparison]); schema {
	case evidence.ComparisonSchema, evidence.ComparisonSchemaV1:
		var artifact evidence.Comparison
		if err := jsonio.Decode(files[bundle.RoleComparison], evidence.MaxComparisonBytes, &artifact); err != nil {
			return fail(stderr, "INVALID_JSON", err.Error())
		}
		res, err := workflow.Replay(ctx, before, after, artifact)
		if err != nil {
			return fail(stderr, "EVIDENCE_MISMATCH", "comparison replay failed")
		}
		result.Comparison, result.Status = &res, res.Status
	case analysis.Schema, analysis.SchemaV2, analysis.SchemaV1:
		var artifact analysis.Result
		if err := jsonio.Decode(files[bundle.RoleComparison], analysis.MaxResultBytes, &artifact); err != nil {
			return fail(stderr, "INVALID_JSON", err.Error())
		}
		res, err := analysis.Replay(ctx, before, after, artifact)
		if err != nil {
			return analysisFailure(stderr, err)
		}
		result.Analysis, result.Status = &res, res.Status
	default:
		return fail(stderr, "EVIDENCE_MISMATCH", "bundle comparison file is not a supported evidence or analysis artifact")
	}
	if repairBytes, ok := files[bundle.RoleRepair]; ok {
		var receipt repair.Result
		if err := jsonio.Decode(repairBytes, repair.MaxJSONBytes, &receipt); err != nil {
			return fail(stderr, "INVALID_JSON", err.Error())
		}
		res, err := repair.Replay(ctx, before, after, receipt)
		if err != nil {
			return fail(stderr, "EVIDENCE_MISMATCH", err.Error())
		}
		result.Repair = &res
		if res.Status == "TIMING_SENSITIVE" && (result.Status == "COMPARISON_REPLAYED" || result.Status == "ANALYSIS_REPLAYED") {
			result.Status = "TIMING_SENSITIVE"
		}
	}
	if code := output(out, stderr, result); code != 0 {
		return code
	}
	matched := result.Status == "COMPARISON_REPLAYED" || result.Status == "ANALYSIS_REPLAYED"
	if result.Repair != nil && result.Repair.Status != "REPAIR_REPLAYED" {
		matched = false
	}
	if !matched {
		return 2
	}
	return 0
}

// artifactSchema leniently reads only the schema field of an
// artifact to dispatch it to its owning replay. The artifact itself
// is then strictly decoded and fully verified by that owner; this
// sniff never trusts the bytes.
func artifactSchema(data []byte) string {
	var probe struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return ""
	}
	return probe.Schema
}

// readBundleFile reads one bundle file with a byte cap. A missing
// file is the typed BUNDLE_FILE_MISSING naming the role (empty role
// for the manifest itself, whose absence makes the directory not a
// bundle at all); an oversized file is BUNDLE_INVALID.
func readBundleFile(name string, limit int, role string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if role == "" {
				return nil, &bundle.Error{Code: "BUNDLE_INVALID", Message: "bundle manifest " + bundle.ManifestName + " not found"}
			}
			return nil, &bundle.Error{Code: "BUNDLE_FILE_MISSING", Message: "bundle file for role \"" + role + "\" is missing", Role: role}
		}
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.New("cannot read bundle file")
	}
	if len(data) > limit {
		return nil, &bundle.Error{Code: "BUNDLE_INVALID", Message: "bundle file exceeds the size limit for its role", Role: role}
	}
	return data, nil
}

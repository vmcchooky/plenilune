package capabilities

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"plenilune/internal/agent"
	"plenilune/internal/analysis"
	"plenilune/internal/audit"
	"plenilune/internal/bundle"
	"plenilune/internal/chat"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/packetset"
	"plenilune/internal/prefixlist"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
)

var update = flag.Bool("update", false, "rewrite the snapshot golden file")

// TestSnapshot pins the full JSON shape of the v1 document. Any
// deliberate change to the published surface (a schema bump, a moved
// cap, a new feature) must update the golden file in the same commit,
// which makes the contract change visible in review.
func TestSnapshot(t *testing.T) {
	doc := BuildWithTool(evidence.ToolVersion, "snapshot-commit")
	got, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "capabilities.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("capabilities document drifted from testdata/capabilities.json; if deliberate, rerun with -update and record the change\ngot:\n%s", got)
	}
}

// TestSchemasMatchOwningConstants guards the builder against ever
// hardcoding a schema string: the document must be assembled from
// the constants of the packages that own each schema.
func TestSchemasMatchOwningConstants(t *testing.T) {
	doc := BuildWithTool("v", "c")
	byID := map[string]SchemaSupport{}
	for _, s := range doc.Schemas {
		byID[s.ID] = s
	}
	want := map[string]SchemaSupport{
		"plenilune.evidence":                {Current: evidence.ComparisonSchema, Supported: []string{evidence.ComparisonSchemaV1, evidence.ComparisonSchema}},
		"plenilune.analysis":                {Current: analysis.Schema, Supported: []string{analysis.SchemaV1, analysis.SchemaV2, analysis.Schema}},
		"plenilune.analysis-request":        {Current: analysis.RequestSchema, Supported: []string{analysis.RequestSchema}},
		"plenilune.analysis-replay-request": {Current: analysis.ReplayRequestSchema, Supported: []string{analysis.ReplayRequestSchema}},
		"plenilune.repair":                  {Current: repair.Schema, Supported: []string{repair.Schema}},
		"plenilune.repair-enumeration":      {Current: repair.EnumerationSchema, Supported: []string{repair.EnumerationSchema}},
		"plenilune.policy":                  {Current: repair.PolicySchema, Supported: []string{repair.PolicySchema}},
		"plenilune.audit":                   {Current: audit.Schema, Supported: []string{audit.Schema}},
		"plenilune.bundle":                  {Current: bundle.Schema, Supported: []string{bundle.Schema}},
		"plenilune.chat":                    {Current: chat.Schema, Supported: []string{chat.Schema}},
		"plenilune.ai":                      {Current: agent.ResultSchema, Supported: []string{agent.ResultSchema}},
		"plenilune.trace":                   {Current: evidence.TraceSchema, Supported: []string{evidence.TraceSchema}},
	}
	if len(byID) != len(want) {
		t.Fatalf("document lists %d schemas, test expects %d", len(byID), len(want))
	}
	for id, w := range want {
		got, ok := byID[id]
		if !ok {
			t.Fatalf("document is missing schema %s", id)
		}
		if got.Current != w.Current || strings.Join(got.Supported, ",") != strings.Join(w.Supported, ",") {
			t.Fatalf("schema %s = %+v, want current %s supported %v", id, got, w.Current, w.Supported)
		}
	}
}

// TestLimitsAndOptionsMatchCodeDefaults re-derives the published
// numbers from the owning packages' defaults, so a moved cap in the
// engine cannot leave stale numbers in the document.
func TestLimitsAndOptionsMatchCodeDefaults(t *testing.T) {
	doc := BuildWithTool("v", "c")
	pd := packetset.Defaults()
	ad := analysis.DefaultOptions()
	if doc.Limits.ACL.MaxRules != ciscoacl.MaxRules || doc.Limits.ACL.MaxInputBytes != ciscoacl.MaxInputBytes {
		t.Fatal("ACL parser bounds drifted")
	}
	if doc.Limits.ACL.MaxOperations != (Bound{Default: int64(pd.MaxOperations), Max: int64(pd.MaxOperations)}) ||
		doc.Limits.ACL.MaxBoxes != (Bound{Default: int64(pd.MaxBoxes), Max: int64(pd.MaxBoxes)}) ||
		doc.Limits.ACL.TimeoutNS.Default != int64(pd.Timeout) {
		t.Fatal("packetset limits drifted")
	}
	if doc.Limits.PrefixList.MaxEntries != prefixlist.MaxEntries {
		t.Fatal("prefix-list bound drifted")
	}
	if doc.Limits.Analysis.MaxAnalyzed != (Bound{Default: int64(analysis.MaxAnalyzed), Max: int64(analysis.MaxAnalyzedLimit)}) ||
		doc.Limits.Analysis.MaxClasses != (Bound{Default: int64(verify.DefaultMaxClasses), Max: int64(verify.MaxClasses)}) {
		t.Fatal("analysis limits drifted")
	}
	opts := map[string]int64{}
	for _, o := range doc.Options {
		opts[o.Name] = o.Default
	}
	wantOpts := map[string]int64{
		"acl.limits.max_operations": int64(ad.ACL.Limits.MaxOperations),
		"acl.limits.max_boxes":      int64(ad.ACL.Limits.MaxBoxes),
		"acl.limits.timeout_ns":     int64(ad.ACL.Limits.Timeout),
		"acl.max_witnesses":         int64(ad.ACL.MaxWitnesses),
		"max_classes":               int64(ad.MaxClasses),
		"deadline_ms":               int64(ad.DeadlineMS),
		"max_analyzed":              int64(ad.MaxAnalyzed),
	}
	if len(opts) != len(wantOpts) {
		t.Fatalf("options surface has %d entries, want %d", len(opts), len(wantOpts))
	}
	for name, want := range wantOpts {
		if opts[name] != want {
			t.Fatalf("option %s default = %d, want %d (analysis.DefaultOptions)", name, opts[name], want)
		}
	}
	for _, d := range doc.Domains {
		switch d.ID {
		case "acl":
			if d.ParserVersion != ciscoacl.ParserVersion || d.EngineVersion != verify.EngineVersion {
				t.Fatal("acl domain versions drifted")
			}
		case "prefix-list":
			if d.ParserVersion != prefixlist.ParserVersion || d.EngineVersion != prefixlist.EngineVersion {
				t.Fatal("prefix-list domain versions drifted")
			}
		default:
			t.Fatalf("unknown domain %q", d.ID)
		}
	}
}

// TestErrorCodesAreEmitted cross-checks the one curated list that
// cannot be derived from constants: every published error code must
// appear as a string literal in non-test Go source somewhere under
// cmd/ or internal/ outside this package — i.e. some real code path
// can actually emit it.
func TestErrorCodesAreEmitted(t *testing.T) {
	if !slices.IsSorted(ErrorCodes) {
		t.Fatal("ErrorCodes must stay sorted")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	var corpus strings.Builder
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if filepath.Base(path) == "capabilities" && filepath.Base(filepath.Dir(path)) == "internal" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			corpus.Write(data)
			corpus.WriteByte('\n')
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	text := corpus.String()
	for _, code := range ErrorCodes {
		if !strings.Contains(text, "\""+code+"\"") {
			t.Fatalf("published error code %s is not emitted as a literal anywhere in non-test source", code)
		}
	}
}

// TestFeaturesPinned keeps the curated feature list identical
// between the exported list and the built document, sorted, and
// containing every feature constant this package defines.
func TestFeaturesPinned(t *testing.T) {
	if !slices.IsSorted(Features) {
		t.Fatal("Features must stay sorted")
	}
	doc := BuildWithTool("v", "c")
	if strings.Join(doc.Features, ",") != strings.Join(Features, ",") {
		t.Fatal("document features differ from the Features list")
	}
	for _, f := range []string{FeatureRepairEnumeration, FeatureAudit, FeatureInterrogationChat, FeatureAIProposals, FeatureCompareCache, FeaturePartialResults, FeatureDualDigest} {
		if !contains(doc.Features, f) {
			t.Fatalf("feature %s missing from document", f)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

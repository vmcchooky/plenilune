package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"plenilune/internal/bundle"
	"plenilune/internal/evidence"
)

// exportSyntheticCase runs diff on a synthetic pair, writes the
// comparison to disk, then bundle-exports the case. It returns the
// bundle directory and the comparison bytes.
func exportSyntheticCase(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	b := put(t, dir, "before-src.acl", "ip access-list extended DEMO\n permit tcp any any eq 22\n")
	a := put(t, dir, "after-src.acl", "ip access-list extended DEMO\n deny ip any any\n")
	code, body, errout := invoke("diff", "--before", b, "--after", a)
	if code != 1 || errout != "" {
		t.Fatalf("diff: %d %s", code, errout)
	}
	comp := put(t, dir, "evidence.json", body)
	outDir := filepath.Join(dir, "case-bundle")
	code, manifestOut, errout := invoke("bundle-export", "--out", outDir,
		"--before", b, "--after", a, "--comparison", comp)
	if code != 0 || errout != "" {
		t.Fatalf("bundle-export: %d %s", code, errout)
	}
	var m bundle.Manifest
	if err := json.Unmarshal([]byte(manifestOut), &m); err != nil || m.Schema != bundle.Schema {
		t.Fatalf("export printed an invalid manifest: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(outDir, bundle.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	want, err := bundle.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(want) {
		t.Fatalf("manifest on disk differs from printed manifest:\n%s\n---\n%s", onDisk, want)
	}
	return outDir, []byte(body)
}

// Acceptance 1: a fresh load from the bundle directory alone —
// sources and comparison come only from the bundle — replays to
// the status the artifact itself carries.
func TestBundleExportReplayRoundTrip(t *testing.T) {
	dir, comparisonBytes := exportSyntheticCase(t)
	var original evidence.Comparison
	if err := json.Unmarshal(comparisonBytes, &original); err != nil {
		t.Fatal(err)
	}
	code, out, errout := invoke("bundle-replay", "--dir", dir)
	if code != 0 || errout != "" {
		t.Fatalf("bundle-replay: %d %s", code, errout)
	}
	if !strings.Contains(out, `"status": "COMPARISON_REPLAYED"`) {
		t.Fatalf("bundle replay output: %s", out)
	}
	if !strings.Contains(out, original.SHA256) {
		t.Fatalf("bundle replay did not return the artifact digest: %s", out)
	}
}

// Acceptance 2: flipping one byte in a bundled file fails the load
// with the typed bundle error naming that file's role.
func TestBundleReplayDetectsFlippedByte(t *testing.T) {
	dir, _ := exportSyntheticCase(t)
	target := filepath.Join(dir, "comparison.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	data[10] ^= 0x01
	if err := os.WriteFile(target, data, 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errout := invoke("bundle-replay", "--dir", dir)
	if code != 2 {
		t.Fatalf("tampered bundle replay exit = %d, want 2", code)
	}
	if !strings.Contains(errout, `"code":"BUNDLE_DIGEST_MISMATCH"`) || !strings.Contains(errout, `"field":"comparison"`) {
		t.Fatalf("tampered bundle error does not name the role: %s", errout)
	}
}

// Acceptance 3: removing a required file, and removing an optional
// file the manifest still lists, are both typed integrity failures.
func TestBundleReplayMissingFiles(t *testing.T) {
	dir, _ := exportSyntheticCase(t)
	if err := os.Remove(filepath.Join(dir, "after.acl")); err != nil {
		t.Fatal(err)
	}
	code, _, errout := invoke("bundle-replay", "--dir", dir)
	if code != 2 || !strings.Contains(errout, `"code":"BUNDLE_FILE_MISSING"`) || !strings.Contains(errout, `"field":"after"`) {
		t.Fatalf("missing required file: %d %s", code, errout)
	}

	// Optional-but-listed: export with a report, then delete it.
	dir2 := t.TempDir()
	b := put(t, dir2, "b.acl", "ip access-list extended DEMO\n permit tcp any any eq 22\n")
	a := put(t, dir2, "a.acl", "ip access-list extended DEMO\n deny ip any any\n")
	_, body, _ := invoke("diff", "--before", b, "--after", a)
	comp := put(t, dir2, "evidence.json", body)
	report := put(t, dir2, "report-src.md", "# Report\n")
	outDir := filepath.Join(dir2, "with-report")
	if code, _, errout := invoke("bundle-export", "--out", outDir,
		"--before", b, "--after", a, "--comparison", comp, "--report", report); code != 0 {
		t.Fatalf("export with report: %d %s", code, errout)
	}
	if code, _, errout := invoke("bundle-replay", "--dir", outDir); code != 0 {
		t.Fatalf("replay with report present: %d %s", code, errout)
	}
	if err := os.Remove(filepath.Join(outDir, "report.md")); err != nil {
		t.Fatal(err)
	}
	code, _, errout = invoke("bundle-replay", "--dir", outDir)
	if code != 2 || !strings.Contains(errout, `"code":"BUNDLE_FILE_MISSING"`) || !strings.Contains(errout, `"field":"report"`) {
		t.Fatalf("missing listed optional file: %d %s", code, errout)
	}
}

// An analysis artifact in the comparison role dispatches to the
// analysis replay, exactly as analyze-replay does.
func TestBundleReplayAnalysisArtifact(t *testing.T) {
	dir := t.TempDir()
	b := put(t, dir, "b.acl", "ip access-list extended EDGE\n permit tcp any any eq 443\n")
	a := put(t, dir, "a.acl", "ip access-list extended EDGE\n permit tcp any any eq 443\n permit udp any any eq 53\n")
	code, body, errout := invoke("analyze", "--before", b, "--after", a)
	if code != 0 || errout != "" {
		t.Fatalf("analyze: %d %s", code, errout)
	}
	comp := put(t, dir, "analysis.json", body)
	outDir := filepath.Join(dir, "analysis-bundle")
	if code, _, errout := invoke("bundle-export", "--out", outDir,
		"--before", b, "--after", a, "--comparison", comp); code != 0 {
		t.Fatalf("export analysis bundle: %d %s", code, errout)
	}
	code, out, errout := invoke("bundle-replay", "--dir", outDir)
	if code != 0 || !strings.Contains(out, `"status": "ANALYSIS_REPLAYED"`) {
		t.Fatalf("analysis bundle replay: %d %s %s", code, out, errout)
	}
}

// Acceptance 5 (CLI surface): a hand-written manifest whose entry
// escapes the bundle directory is rejected before any file is read.
func TestBundleReplayRejectsPathEscapeManifest(t *testing.T) {
	dir := t.TempDir()
	d := bundle.Digest([]byte("x"))
	entries := `{"role":"before","path":"../secret.acl","sha256":"` + d + `"},` +
		`{"role":"after","path":"after.acl","sha256":"` + d + `"},` +
		`{"role":"comparison","path":"comparison.json","sha256":"` + d + `"}`
	put(t, dir, bundle.ManifestName, `{"schema":"`+bundle.Schema+`","files":[`+entries+"]}")
	code, _, errout := invoke("bundle-replay", "--dir", dir)
	if code != 2 || !strings.Contains(errout, `"code":"BUNDLE_INVALID"`) {
		t.Fatalf("path escape manifest: %d %s", code, errout)
	}
}

// A directory without a manifest is not a bundle.
func TestBundleReplayWithoutManifest(t *testing.T) {
	code, _, errout := invoke("bundle-replay", "--dir", t.TempDir())
	if code != 2 || !strings.Contains(errout, `"code":"BUNDLE_INVALID"`) {
		t.Fatalf("missing manifest: %d %s", code, errout)
	}
}

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"plenilune/internal/analysis"
)

func writeTemp(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnalyzeCLIFileAndStdinReplay(t *testing.T) {
	before, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", "regression", "before.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", "regression", "after.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	beforePath, afterPath := writeTemp(t, "before.cfg", string(before)), writeTemp(t, "after.cfg", string(after))
	var out, errOut bytes.Buffer
	if code := Run(t.Context(), []string{"analyze", "--before", beforePath, "--after", afterPath}, &out, &errOut); code != 0 {
		t.Fatalf("analyze exit %d: %s", code, errOut.String())
	}
	var result analysis.Result
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != analysis.Schema || result.Summary.Changed != 1 || result.SHA256 == "" {
		t.Fatalf("artifact %+v", result.Summary)
	}
	evidence := writeTemp(t, "analysis.json", out.String())
	out.Reset()
	errOut.Reset()
	if code := Run(t.Context(), []string{"analyze-replay", "--before", beforePath, "--after", afterPath, "--evidence", evidence}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "ANALYSIS_REPLAYED") {
		t.Fatalf("replay exit %d %s %s", code, out.String(), errOut.String())
	}

	request, _ := json.Marshal(analysis.Request{Schema: analysis.RequestSchema, Before: analysis.Source{Text: string(before), Ref: "artifact://x@1"}, After: analysis.Source{Text: string(after), Ref: "artifact://x@2"}})
	old := stdinReader
	stdinReader = bytes.NewReader(request)
	defer func() { stdinReader = old }()
	out.Reset()
	errOut.Reset()
	if code := Run(t.Context(), []string{"analyze", "--request", "-"}, &out, &errOut); code != 0 {
		t.Fatalf("stdin analyze exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "artifact://x@1") {
		t.Fatal("caller ref not echoed")
	}
}

func TestAnalyzeCLIUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"analyze"},
		{"analyze", "--before", "only.cfg"},
		{"analyze", "--before", "a.cfg", "--after", "b.cfg", "--request", "-"},
		{"analyze-replay", "--before", "a.cfg", "--after", "b.cfg"},
	} {
		var out, errOut bytes.Buffer
		if code := Run(t.Context(), args, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), `"code"`) || out.Len() != 0 {
			t.Fatalf("%v: exit %d out %q err %q", args, code, out.String(), errOut.String())
		}
	}
}

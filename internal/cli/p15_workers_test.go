package cli

// P1-5: the analyze --workers flag is an execution detail — the
// artifact bytes must not depend on it — and out-of-range values are
// usage errors.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeWorkersFlagArtifactInvariant(t *testing.T) {
	before, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", "regression", "before.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", "regression", "after.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	beforePath, afterPath := writeTemp(t, "before.cfg", string(before)), writeTemp(t, "after.cfg", string(after))
	var reference string
	for _, workers := range []string{"0", "1", "4"} {
		var out, errOut bytes.Buffer
		code := Run(t.Context(), []string{"analyze", "--before", beforePath, "--after", afterPath, "--workers", workers}, &out, &errOut)
		if code != 0 {
			t.Fatalf("workers=%s exit %d: %s", workers, code, errOut.String())
		}
		if reference == "" {
			reference = out.String()
		} else if out.String() != reference {
			t.Fatalf("workers=%s changed the artifact bytes", workers)
		}
	}
	for _, workers := range []string{"-1", "65"} {
		var out, errOut bytes.Buffer
		if code := Run(t.Context(), []string{"analyze", "--before", beforePath, "--after", afterPath, "--workers", workers}, &out, &errOut); code != 2 {
			t.Fatalf("workers=%s exit %d, want usage error 2", workers, code)
		}
	}
}

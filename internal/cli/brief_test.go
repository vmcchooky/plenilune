package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"plenilune/internal/evidence"
	"plenilune/internal/jsonio"
	"strings"
	"testing"
)

const (
	briefGoodACL = "ip access-list extended DEMO\n permit tcp host 192.0.2.10 host 198.51.100.10 eq 22\n deny ip any any\n"
	briefDenyACL = "ip access-list extended DEMO\n deny ip any any\n"
	briefWideACL = "ip access-list extended DEMO\n permit tcp any any eq 22\n"
)

// briefArgs builds a diff --brief invocation in dir and returns the args and
// the artifact path. Extra args are appended last so budget flags can follow.
func briefArgs(t *testing.T, dir, before, after string, extra ...string) ([]string, string) {
	t.Helper()
	b := put(t, dir, "before.acl", before)
	a := put(t, dir, "after.acl", after)
	out := filepath.Join(dir, "artifact.json")
	args := []string{"diff", "--brief", "--out", out, "--before", b, "--after", a}
	return append(args, extra...), out
}

func TestBriefGoldenFormats(t *testing.T) {
	obligations, err := os.ReadFile(filepath.Join("testdata", "obligations-brief.json"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name        string
		before      string
		after       string
		obligations bool
		extra       []string
		golden      string
		wantCode    int
		wantVerdict string
	}{
		{"equivalent obligations hold", briefGoodACL, briefGoodACL, true, nil, "brief-equivalent.golden", 0, "EQUIVALENT"},
		{"different obligation violated", briefGoodACL, briefDenyACL, true, nil, "brief-different-violated.golden", 1, "DIFFERENT"},
		{"different no obligations", briefWideACL, briefDenyACL, false, nil, "brief-different-no-obligations.golden", 1, "DIFFERENT"},
		{"inconclusive budget exhausted", briefWideACL, briefDenyACL, false, []string{"--max-operations", "1"}, "brief-inconclusive.golden", 2, "INCONCLUSIVE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			extra := tc.extra
			if tc.obligations {
				cond := put(t, dir, "obligations.json", string(obligations))
				extra = append(append([]string{}, extra...), "--obligations", cond)
			}
			args, outPath := briefArgs(t, dir, tc.before, tc.after, extra...)
			code, stdout, stderr := invoke(args...)
			if code != tc.wantCode {
				t.Fatalf("exit %d, want %d (stderr %q)", code, tc.wantCode, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr not empty: %q", stderr)
			}
			if strings.Contains(stdout, "\x1b") {
				t.Fatalf("brief output contains ANSI escapes: %q", stdout)
			}
			if n := strings.Count(stdout, "\n"); n != 5 {
				t.Fatalf("brief output has %d lines, want the fixed budget of 5: %q", n, stdout)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.golden))
			if err != nil {
				t.Fatal(err)
			}
			got := strings.ReplaceAll(stdout, outPath, "ARTIFACT_PATH")
			if got != string(want) {
				t.Fatalf("brief output mismatch\ngot:\n%s\nwant:\n%s", got, want)
			}
			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("artifact file not written: %v", err)
			}
			var artifact evidence.Comparison
			if err := jsonio.Decode(data, evidence.MaxComparisonBytes, &artifact); err != nil {
				t.Fatalf("artifact file is not a comparison: %v", err)
			}
			if artifact.Report.Verdict != tc.wantVerdict {
				t.Fatalf("artifact verdict %q, want %q", artifact.Report.Verdict, tc.wantVerdict)
			}
		})
	}
}

func TestBriefArtifactBytesMatchFullOutput(t *testing.T) {
	dir := t.TempDir()
	b := put(t, dir, "before.acl", briefGoodACL)
	a := put(t, dir, "after.acl", briefDenyACL)
	code, full, stderr := invoke("diff", "--before", b, "--after", a)
	if code != 1 || stderr != "" {
		t.Fatalf("full diff: %d %s", code, stderr)
	}
	args, outPath := briefArgs(t, dir, briefGoodACL, briefDenyACL)
	code, brief, stderr := invoke(args...)
	if code != 1 || stderr != "" {
		t.Fatalf("brief diff: %d %s", code, stderr)
	}
	if strings.Contains(brief, "{") {
		t.Fatalf("brief stdout carries JSON: %q", brief)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte(full)) {
		t.Fatal("brief artifact bytes differ from the default diff output")
	}
}

func TestBriefErrorContract(t *testing.T) {
	dir := t.TempDir()
	b := put(t, dir, "before.acl", briefWideACL)
	a := put(t, dir, "after.acl", briefDenyACL)
	out := filepath.Join(dir, "artifact.json")
	singleLine := func(t *testing.T, code int, stdout, stderr string) {
		t.Helper()
		if code != 3 {
			t.Fatalf("exit %d, want 3", code)
		}
		if stdout != "" {
			t.Fatalf("stdout not empty on error: %q", stdout)
		}
		if !strings.HasPrefix(stderr, "error: ") || strings.Count(stderr, "\n") != 1 {
			t.Fatalf("brief error is not one line: %q", stderr)
		}
	}
	t.Run("missing out is a brief usage error", func(t *testing.T) {
		code, stdout, stderr := invoke("diff", "--brief", "--before", b, "--after", a)
		singleLine(t, code, stdout, stderr)
		if stderr != "error: USAGE --brief requires --out FILE for the full evidence artifact\n" {
			t.Fatalf("unexpected error line: %q", stderr)
		}
	})
	t.Run("missing input file", func(t *testing.T) {
		code, stdout, stderr := invoke("diff", "--brief", "--out", out, "--before", filepath.Join(dir, "nope.acl"), "--after", a)
		singleLine(t, code, stdout, stderr)
		if !strings.Contains(stderr, "INPUT_FAILED") {
			t.Fatalf("unexpected error line: %q", stderr)
		}
	})
	t.Run("unknown flag", func(t *testing.T) {
		code, stdout, stderr := invoke("diff", "--brief", "--bogus")
		singleLine(t, code, stdout, stderr)
	})
	t.Run("acl parse error keeps its typed code", func(t *testing.T) {
		bad := put(t, dir, "bad.acl", "ip access-list extended DEMO\n permit tcp any any eq 99999\n")
		code, stdout, stderr := invoke("diff", "--brief", "--out", out, "--before", bad, "--after", a)
		singleLine(t, code, stdout, stderr)
		if !strings.Contains(stderr, "PARSE_") {
			t.Fatalf("typed parse code lost: %q", stderr)
		}
	})
	t.Run("invalid obligations", func(t *testing.T) {
		cond := put(t, dir, "cond.json", "{not json")
		code, stdout, stderr := invoke("diff", "--brief", "--out", out, "--before", b, "--after", a, "--obligations", cond)
		singleLine(t, code, stdout, stderr)
		if !strings.Contains(stderr, "INVALID_OBLIGATION") {
			t.Fatalf("unexpected error line: %q", stderr)
		}
	})
	t.Run("out without brief stays the classic contract", func(t *testing.T) {
		code, stdout, stderr := invoke("diff", "--out", out, "--before", b, "--after", a)
		if code != 2 || stdout != "" || !strings.Contains(stderr, `"code":"USAGE"`) {
			t.Fatalf("non-brief --out rejection changed: %d %q %q", code, stdout, stderr)
		}
	})
	t.Run("no artifact file left behind on error", func(t *testing.T) {
		missing := filepath.Join(dir, "never.json")
		if code, _, _ := invoke("diff", "--brief", "--out", missing, "--before", filepath.Join(dir, "nope.acl"), "--after", a); code != 3 {
			t.Fatalf("exit %d, want 3", code)
		}
		if _, err := os.Stat(missing); !os.IsNotExist(err) {
			t.Fatal("artifact file exists after a failed brief run")
		}
	})
}

func TestBriefHelpDocumentsExitCodes(t *testing.T) {
	code, stdout, _ := invoke()
	if code != 0 || !strings.Contains(stdout, "Diff --brief exits: 0 EQUIVALENT, 1 DIFFERENT, 2 INCONCLUSIVE, 3 error") {
		t.Fatal("top-level help does not document the brief exit codes")
	}
	code, stdout, _ = invoke("diff", "--help")
	if code != 0 || !strings.Contains(stdout, "-brief") || !strings.Contains(stdout, "-out") {
		t.Fatal("diff flag help does not document --brief/--out")
	}
}

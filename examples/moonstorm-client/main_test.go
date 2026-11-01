package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"plenilune/internal/cli"
)

// realDocument produces the tool's actual capabilities document
// in-process, exactly as the shipped binary would print it, by
// invoking the CLI the way internal/cli's own tests do — no binary
// is executed, so the gate tests cannot flake on build state.
func realDocument(t *testing.T) document {
	t.Helper()
	var out, errb bytes.Buffer
	if code := cli.Run(context.Background(), []string{"capabilities", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("capabilities exit %d: %s", code, errb.String())
	}
	var doc document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode capabilities document: %v", err)
	}
	return doc
}

func TestGateAcceptsTheRealDocument(t *testing.T) {
	if reasons := gate(realDocument(t)); len(reasons) > 0 {
		t.Fatalf("real document refused: %v", reasons)
	}
}

func TestGateRefusesWrongSchemaVersions(t *testing.T) {
	for _, id := range []string{"plenilune.evidence", "plenilune.analysis"} {
		doc := realDocument(t)
		for i := range doc.Schemas {
			if doc.Schemas[i].ID == id {
				doc.Schemas[i].Current = id + "/v0"
			}
		}
		reasons := gate(doc)
		if len(reasons) != 1 || !strings.Contains(reasons[0], id) {
			t.Fatalf("%s downgraded: reasons = %v, want one naming the schema", id, reasons)
		}
	}
}

func TestGateRefusesMissingFeature(t *testing.T) {
	doc := realDocument(t)
	kept := doc.Features[:0]
	for _, f := range doc.Features {
		if f != "audit" {
			kept = append(kept, f)
		}
	}
	doc.Features = kept
	reasons := gate(doc)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "audit") {
		t.Fatalf("reasons = %v, want one naming the missing feature", reasons)
	}
}

func TestGateRefusesUnknownCapabilitiesSchema(t *testing.T) {
	doc := realDocument(t)
	doc.Schema = "plenilune.capabilities/v99"
	if reasons := gate(doc); len(reasons) == 0 {
		t.Fatal("unknown capabilities schema accepted")
	}
}

// --- P2-7: the full job loop, exercised in-process --------------
//
// script adapts the in-process CLI (cli.Run, the same entry the
// shipped binary's main calls) to the client's invoke seam, so the
// loop tests run against real artifacts without executing a built
// binary. The one transport difference: the client sends requests
// on stdin ("--request -"), which the in-process CLI reads from a
// package-level reader the test cannot replace, so the adapter
// writes the payload to a temp file and rewrites the argument —
// the request bytes, the command and the code path are otherwise
// identical. Hooks let a test tamper with one call's stdin/stdout
// or override a call outright.
type script struct {
	t          *testing.T
	calls      []string
	stdinHook  func(cmd string, stdin []byte) []byte
	stdoutHook func(cmd string, stdout []byte) []byte
	override   func(cmd string) (stdout, stderr []byte, code int, ok bool)
}

func (s *script) invoke(args []string, stdin []byte) ([]byte, []byte, int) {
	cmd := args[0]
	s.calls = append(s.calls, cmd)
	if s.override != nil {
		if stdout, stderr, code, ok := s.override(cmd); ok {
			return stdout, stderr, code
		}
	}
	if s.stdinHook != nil {
		stdin = s.stdinHook(cmd, stdin)
	}
	realArgs := append([]string(nil), args...)
	for i := 0; i+1 < len(realArgs); i++ {
		if realArgs[i] == "--request" && realArgs[i+1] == "-" {
			path := filepath.Join(s.t.TempDir(), "request.json")
			if err := os.WriteFile(path, stdin, 0o600); err != nil {
				s.t.Fatalf("write request file: %v", err)
			}
			realArgs[i+1] = path
		}
	}
	var out, errb bytes.Buffer
	code := cli.Run(context.Background(), realArgs, &out, &errb)
	stdout := out.Bytes()
	if s.stdoutHook != nil {
		stdout = s.stdoutHook(cmd, stdout)
	}
	return stdout, errb.Bytes(), code
}

func (s *script) called(cmd string) bool {
	for _, c := range s.calls {
		if c == cmd {
			return true
		}
	}
	return false
}

func configFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", "regression", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func fixtureJob(t *testing.T) *job {
	t.Helper()
	return &job{
		before:     source{Text: configFixture(t, "before.cfg"), Ref: "artifact://synthetic/edge-r1@v41"},
		after:      source{Text: configFixture(t, "after.cfg"), Ref: "artifact://synthetic/edge-r1@v42"},
		deadlineMS: 30000,
	}
}

// flipDigest returns the JSON with the named digest field's first
// hex digit changed — a well-formed but wrong digest claim. It
// tolerates the pretty-printed spacing the CLI emits.
func flipDigest(t *testing.T, data []byte, field string) []byte {
	t.Helper()
	key := []byte(`"` + field + `":`)
	i := bytes.Index(data, key)
	if i < 0 {
		t.Fatalf("field %s not found in %s", field, data)
	}
	out := append([]byte(nil), data...)
	pos := i + len(key)
	for pos < len(out) && (out[pos] == ' ' || out[pos] == '"') {
		pos++
	}
	if pos >= len(out) {
		t.Fatalf("field %s has no value in %s", field, data)
	}
	if out[pos] == '0' {
		out[pos] = '1'
	} else {
		out[pos] = '0'
	}
	return out
}

func TestLoopCompletesOnRealFixtures(t *testing.T) {
	s := &script{t: t}
	var out bytes.Buffer
	if code := execute(s.invoke, fixtureJob(t), &out); code != 0 {
		t.Fatalf("loop exit %d, want 0:\n%s", code, out.String())
	}
	text := out.String()
	for _, want := range []string{
		"COMPATIBLE:",
		"ARTIFACT: schema plenilune.analysis/v3",
		"OBJECT acl MGMT changed",
		"OBJECT prefix-list CUSTOMER unchanged",
		"SUMMARY: objects=2 changed=1",
		"DIGESTS: content_sha256=",
		"CACHE HIT: identical re-run reproduced content_sha256",
		"REPLAY: ANALYSIS_REPLAYED",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("loop output missing %q:\n%s", want, text)
		}
	}
}

func TestLoopGateOnlyWithoutJob(t *testing.T) {
	s := &script{t: t}
	var out bytes.Buffer
	if code := execute(s.invoke, nil, &out); code != 0 {
		t.Fatalf("gate-only exit %d, want 0:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "COMPATIBLE:") {
		t.Errorf("missing COMPATIBLE line:\n%s", out.String())
	}
	if s.called("analyze") || s.called("analyze-replay") {
		t.Errorf("gate-only run invoked the loop: calls %v", s.calls)
	}
}

func TestLoopGateRefusalSkipsAnalyze(t *testing.T) {
	s := &script{t: t}
	s.stdoutHook = func(cmd string, stdout []byte) []byte {
		if cmd != "capabilities" {
			return stdout
		}
		var doc document
		if err := json.Unmarshal(stdout, &doc); err != nil {
			t.Fatalf("decode capabilities: %v", err)
		}
		for i := range doc.Schemas {
			if doc.Schemas[i].ID == "plenilune.analysis" {
				doc.Schemas[i].Current = "plenilune.analysis/v0"
			}
		}
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("re-encode capabilities: %v", err)
		}
		return data
	}
	var out bytes.Buffer
	if code := execute(s.invoke, fixtureJob(t), &out); code != 3 {
		t.Fatalf("loop exit %d, want 3 (gate refusal):\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "REFUSED: schema plenilune.analysis is plenilune.analysis/v0") {
		t.Errorf("missing refusal reason:\n%s", out.String())
	}
	if s.called("analyze") || s.called("analyze-replay") {
		t.Errorf("loop proceeded after gate refusal: calls %v", s.calls)
	}
}

func TestLoopMapsAnalyzeTypedError(t *testing.T) {
	s := &script{t: t}
	s.override = func(cmd string) ([]byte, []byte, int, bool) {
		if cmd == "analyze" {
			return nil, []byte(`{"code":"DEADLINE_EXCEEDED","stage":"budget","message":"analysis deadline exceeded","retryable":true}`), 2, true
		}
		return nil, nil, 0, false
	}
	var out bytes.Buffer
	if code := execute(s.invoke, fixtureJob(t), &out); code != 4 {
		t.Fatalf("loop exit %d, want 4 (analyze failed):\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "ANALYZE FAILED: DEADLINE_EXCEEDED (stage budget, retryable true)") {
		t.Errorf("typed error not mapped by code/stage:\n%s", out.String())
	}
}

func TestLoopRejectsTamperedArtifactAtReplay(t *testing.T) {
	s := &script{t: t}
	s.stdoutHook = func(cmd string, stdout []byte) []byte {
		if cmd == "analyze" {
			return flipDigest(t, stdout, "content_sha256")
		}
		return stdout
	}
	var out bytes.Buffer
	if code := execute(s.invoke, fixtureJob(t), &out); code != 6 {
		t.Fatalf("loop exit %d, want 6 (replay did not confirm):\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "REPLAY FAILED: EVIDENCE_MISMATCH") {
		t.Errorf("tampered artifact not caught by replay:\n%s", out.String())
	}
}

func TestLoopDetectsReplayDigestMismatch(t *testing.T) {
	s := &script{t: t}
	s.stdoutHook = func(cmd string, stdout []byte) []byte {
		if cmd != "analyze-replay" {
			return stdout
		}
		return flipDigest(t, stdout, "sha256")
	}
	var out bytes.Buffer
	if code := execute(s.invoke, fixtureJob(t), &out); code != 5 {
		t.Fatalf("loop exit %d, want 5 (digest check failed):\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "REPLAY MISMATCH:") {
		t.Errorf("replay digest mismatch not reported:\n%s", out.String())
	}
}

func TestLoopReportsTimingSensitiveAsNotFinal(t *testing.T) {
	s := &script{t: t}
	s.stdoutHook = func(cmd string, stdout []byte) []byte {
		if cmd == "analyze-replay" {
			return bytes.Replace(stdout, []byte("ANALYSIS_REPLAYED"), []byte("TIMING_SENSITIVE"), 1)
		}
		return stdout
	}
	var out bytes.Buffer
	if code := execute(s.invoke, fixtureJob(t), &out); code != 6 {
		t.Fatalf("loop exit %d, want 6 (timing sensitive):\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "TIMING_SENSITIVE") || !strings.Contains(out.String(), "NOT final") {
		t.Errorf("timing-sensitive result not reported as not-final:\n%s", out.String())
	}
}

// Command moonstorm-client is the sample external caller for the
// Plenilune contract — the shape a future MoonStorm adapter takes.
// The full integrator contract it implements is specified in
// docs/integration/moonstorm.md.
//
// The client runs the documented job loop:
//
//  1. Gate: ask the binary for its capabilities document
//     (`plenilune capabilities --json`) and refuse the job early,
//     with named reasons, when the tool's schema versions or
//     features do not match what the caller requires — before any
//     configuration changes hands.
//  2. Analyze: send one analysis request (`plenilune analyze
//     --request -`, request JSON on stdin so decrypted configuration
//     never has to touch the caller's disk) and store the returned
//     artifact under its content digest.
//  3. Idempotency: repeat the identical request; an identical
//     content_sha256 is the caller's cache hit — the stored artifact
//     is reused and nothing is re-stored. A different content digest
//     is reported, never silently merged (a timing-limited run is
//     the usual cause).
//  4. Replay: verify the stored artifact with
//     `plenilune analyze-replay --request -`. ANALYSIS_REPLAYED
//     with both digests matching the stored pair is the only
//     outcome the caller treats as confirmed; TIMING_SENSITIVE is
//     reported as not-final, never upgraded.
//
// With no configuration flags the client stops after the gate (the
// P2-2 behavior). The client depends only on the binary and the
// JSON contract — no Plenilune packages, no browser, no UI.
// Standard library only.
//
// Usage:
//
//	moonstorm-client [-plenilune PATH] [-before FILE] [-after FILE]
//	                 [-before-ref LABEL] [-after-ref LABEL]
//	                 [-deadline-ms N]
//
// The binary path comes from -plenilune, else $PLENILUNE_BIN, else
// "plenilune" on PATH. Exit codes:
//
//	0  gate passed (and, with configurations, the loop completed
//	   with a replay-confirmed artifact)
//	2  the capabilities call itself failed, or a usage/input error
//	3  incompatible (gate refusal reasons printed)
//	4  the analyze call failed (typed error printed)
//	5  the artifact or the replay result failed the client's own
//	   digest checks
//	6  replay did not confirm the artifact (typed error, or
//	   TIMING_SENSITIVE — the result is not final)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"time"
)

// document is the client's own view of plenilune.capabilities/v1 —
// an external caller decodes the fields it gates on and ignores the
// rest (the v1 contract is additive-only).
type document struct {
	Schema string `json:"schema"`
	Tool   struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	} `json:"tool"`
	Schemas []struct {
		ID        string   `json:"id"`
		Current   string   `json:"current"`
		Supported []string `json:"supported"`
	} `json:"schemas"`
	Features []string `json:"features"`
}

const capabilitiesSchema = "plenilune.capabilities/v1"

// requiredSchemas pins the artifact schema versions this caller
// writes and reads. A tool whose current version differs is refused
// before any configuration is sent.
var requiredSchemas = map[string]string{
	"plenilune.evidence": "plenilune.evidence/v2",
	"plenilune.analysis": "plenilune.analysis/v3",
}

// requiredFeatures are the build features the caller's workflow
// relies on (content-digest dedup, the repair enumerator, the exact
// audit).
var requiredFeatures = []string{"audit", "dual_digest", "repair_enumeration"}

// gate returns the list of incompatibility reasons; empty means the
// caller may proceed. It is a pure function of the document so it
// can be unit-tested without executing a binary.
func gate(doc document) []string {
	var reasons []string
	if doc.Schema != capabilitiesSchema {
		reasons = append(reasons, fmt.Sprintf("capabilities schema %q is not supported (want %q)", doc.Schema, capabilitiesSchema))
	}
	byID := map[string]string{}
	for _, s := range doc.Schemas {
		byID[s.ID] = s.Current
	}
	ids := make([]string, 0, len(requiredSchemas))
	for id := range requiredSchemas {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		want := requiredSchemas[id]
		got, ok := byID[id]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("tool does not publish schema %s", id))
		} else if got != want {
			reasons = append(reasons, fmt.Sprintf("schema %s is %s, this caller requires %s", id, got, want))
		}
	}
	have := map[string]bool{}
	for _, f := range doc.Features {
		have[f] = true
	}
	for _, f := range requiredFeatures {
		if !have[f] {
			reasons = append(reasons, fmt.Sprintf("tool lacks required feature %q", f))
		}
	}
	return reasons
}

// invoke is the subprocess seam: one call to the binary with
// arguments and an optional stdin payload. Production wires it to
// execRunner; tests wire it to an in-process adapter so the loop
// can be exercised against real artifacts without executing a
// built binary.
type invoke func(args []string, stdin []byte) (stdout, stderr []byte, code int)

// execRunner runs the real binary. The caller's hard timeout is
// deliberately far above the largest analysis deadline the
// contract allows (120s): deadlines inside the job belong to the
// request, this cap only keeps a wedged process from hanging the
// caller forever. Killing the process yields no artifact, which
// the caller records as an inconclusive job — never a verdict.
func execRunner(binary string) invoke {
	return func(args []string, stdin []byte) ([]byte, []byte, int) {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		err := cmd.Run()
		if err == nil {
			return out.Bytes(), errb.Bytes(), 0
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out.Bytes(), errb.Bytes(), exitErr.ExitCode()
		}
		return out.Bytes(), errb.Bytes(), -1
	}
}

// typedError is the client's view of the P2-3 error envelope the
// CLI writes to stderr on failure. Callers map on Code/Stage and
// the Retryable flag — never on the message text.
type typedError struct {
	Code      string `json:"code"`
	Stage     string `json:"stage"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e *typedError) describe() string {
	return fmt.Sprintf("%s (stage %s, retryable %t): %s", e.Code, e.Stage, e.Retryable, e.Message)
}

// callError reports a failed subprocess call: the exit code plus
// the typed envelope when the tool produced one.
type callError struct {
	code  int
	typed *typedError
	raw   string
}

func failCall(code int, stderr []byte) *callError {
	ce := &callError{code: code, raw: string(bytes.TrimSpace(stderr))}
	var te typedError
	if err := json.Unmarshal(stderr, &te); err == nil && te.Code != "" {
		ce.typed = &te
	}
	return ce
}

func (e *callError) describe() string {
	if e.typed != nil {
		return e.typed.describe()
	}
	if e.raw != "" {
		return fmt.Sprintf("exit %d: %s", e.code, e.raw)
	}
	return fmt.Sprintf("exit %d", e.code)
}

// artifact is the client's view of plenilune.analysis/v3: the
// fields the caller reports and keys on. The caller never
// recomputes the seal itself — replay is the verification
// primitive — but it does check the artifact carries the digest
// pair the dedup and replay steps key on.
type artifact struct {
	Schema   string `json:"schema"`
	Versions struct {
		ACLParser        string `json:"acl_parser"`
		ACLEngine        string `json:"acl_engine"`
		PrefixListParser string `json:"prefix_list_parser"`
		PrefixListEngine string `json:"prefix_list_engine"`
	} `json:"versions"`
	Objects []struct {
		Domain string `json:"domain"`
		Kind   string `json:"kind"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"objects"`
	Summary struct {
		Objects           int `json:"objects"`
		Changed           int `json:"changed"`
		Equivalent        int `json:"equivalent"`
		Unchanged         int `json:"unchanged"`
		Added             int `json:"added"`
		Removed           int `json:"removed"`
		Unsupported       int `json:"unsupported"`
		Inconclusive      int `json:"inconclusive"`
		NotAnalyzed       int `json:"not_analyzed"`
		UnanalyzedChanges int `json:"unanalyzed_changes"`
	} `json:"summary"`
	Limitations   []string `json:"limitations"`
	ContentSHA256 string   `json:"content_sha256"`
	SHA256        string   `json:"sha256"`
}

func isHexDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// checkArtifactShape returns the problems with an artifact the
// caller is about to trust as a job result: wrong schema (the gate
// pinned the current one) or a missing/malformed digest pair.
func checkArtifactShape(a artifact) []string {
	var problems []string
	if want := requiredSchemas["plenilune.analysis"]; a.Schema != want {
		problems = append(problems, fmt.Sprintf("artifact schema is %q, this caller requires %q", a.Schema, want))
	}
	if !isHexDigest(a.ContentSHA256) {
		problems = append(problems, "artifact carries no valid content_sha256 — no content identity to dedup or sign")
	}
	if !isHexDigest(a.SHA256) {
		problems = append(problems, "artifact carries no valid sha256 (artifact digest)")
	}
	return problems
}

// source is one side of an analysis/replay request. Ref is an
// opaque caller correlation label, never identity.
type source struct {
	Text string `json:"text"`
	Ref  string `json:"ref,omitempty"`
}

// analysisRequest is the plenilune.analysis-request/v1 document.
// The options block is all-or-nothing on the wire: a present block
// replaces the defaults wholesale, so a partial block (say, only a
// deadline) leaves the remaining fields zero and is rejected with
// OPTIONS_INVALID. The client therefore always sends the complete
// budget profile below — the values the current contract publishes
// as defaults in capabilities (options list), with the job's own
// deadline substituted. The profile is part of the job's identity:
// the artifact echoes the effective options in its request echo,
// inside the content projection, so the same configurations under
// a different profile are a different result with a different
// content digest.
type analysisRequest struct {
	Schema  string `json:"schema"`
	Before  source `json:"before"`
	After   source `json:"after"`
	Options struct {
		ACL struct {
			Limits struct {
				MaxOperations int `json:"max_operations"`
				MaxBoxes      int `json:"max_boxes"`
				TimeoutNS     int `json:"timeout_ns"`
			} `json:"limits"`
			MaxWitnesses int `json:"max_witnesses"`
		} `json:"acl"`
		MaxClasses int `json:"max_classes"`
		DeadlineMS int `json:"deadline_ms"`
	} `json:"options"`
}

const (
	analysisRequestSchema = "plenilune.analysis-request/v1"
	replayRequestSchema   = "plenilune.analysis-replay-request/v1"
)

func buildAnalysisRequest(before, after source, deadlineMS int) []byte {
	var req analysisRequest
	req.Schema = analysisRequestSchema
	req.Before = before
	req.After = after
	req.Options.ACL.Limits.MaxOperations = 2000000
	req.Options.ACL.Limits.MaxBoxes = 50000
	req.Options.ACL.Limits.TimeoutNS = 2000000000
	req.Options.ACL.MaxWitnesses = 8
	req.Options.MaxClasses = 32
	req.Options.DeadlineMS = deadlineMS
	data, _ := json.Marshal(req)
	return data
}

func callAnalyze(inv invoke, request []byte) (artifact, []byte, *callError) {
	stdout, stderr, code := inv([]string{"analyze", "--request", "-"}, request)
	if code != 0 {
		return artifact{}, nil, failCall(code, stderr)
	}
	var art artifact
	if err := json.Unmarshal(stdout, &art); err != nil {
		return artifact{}, nil, &callError{code: code, raw: "analyze output is not valid JSON: " + err.Error()}
	}
	return art, stdout, nil
}

// replayResult is the client's view of the analyze-replay output.
type replayResult struct {
	Status             string `json:"status"`
	SHA256             string `json:"sha256"`
	ContentSHA256      string `json:"content_sha256"`
	RecheckedWitnesses int    `json:"rechecked_witnesses"`
}

func callReplay(inv invoke, before, after source, artifactRaw []byte) (replayResult, *callError) {
	// The evidence is embedded as raw JSON so its bytes reach the
	// tool exactly as produced — re-marshalling through a generic
	// value could reorder or drop fields the strict decoder and
	// the seal check insist on.
	req := []byte(`{"schema":"` + replayRequestSchema + `","before":`)
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	req = append(req, b...)
	req = append(req, []byte(`,"after":`)...)
	req = append(req, a...)
	req = append(req, []byte(`,"evidence":`)...)
	req = append(req, artifactRaw...)
	req = append(req, '}')
	stdout, stderr, code := inv([]string{"analyze-replay", "--request", "-"}, req)
	// A TIMING_SENSITIVE replay still returns its result document
	// on stdout, with exit code 2: decode it before treating the
	// call as failed, so the caller can report "not final" instead
	// of a generic failure.
	var res replayResult
	if len(stdout) > 0 && json.Unmarshal(stdout, &res) == nil && res.Status != "" {
		return res, nil
	}
	if code != 0 {
		return replayResult{}, failCall(code, stderr)
	}
	return replayResult{}, &callError{code: code, raw: "analyze-replay returned no result document"}
}

// job describes one unit of caller work: a pair of configurations
// the caller already holds (MoonStorm keeps custody of the bytes;
// this sample reads them from files) plus caller labels and the
// per-job deadline. The budget profile is part of the job's
// identity: the options are inside the artifact's content
// projection, so the same configurations under a different
// deadline produce a different content digest.
type job struct {
	before, after source
	deadlineMS    int
}

// execute runs the gate and, when the job carries configurations,
// the full loop. All report lines go to out; refusal/failure
// reasons go to out as well so a single stream tells the whole
// story of the job.
func execute(inv invoke, j *job, out io.Writer) int {
	stdout, stderr, code := inv([]string{"capabilities", "--json"}, nil)
	if code != 0 {
		fmt.Fprintf(out, "capabilities call failed: %s\n", failCall(code, stderr).describe())
		return 2
	}
	var doc document
	if err := json.Unmarshal(stdout, &doc); err != nil {
		fmt.Fprintf(out, "capabilities document is not valid JSON: %v\n", err)
		return 2
	}
	if reasons := gate(doc); len(reasons) > 0 {
		for _, r := range reasons {
			fmt.Fprintf(out, "REFUSED: %s\n", r)
		}
		return 3
	}
	fmt.Fprintf(out, "COMPATIBLE: plenilune %s (commit %s) — evidence %s, analysis %s, required features present\n",
		doc.Tool.Version, doc.Tool.Commit,
		requiredSchemas["plenilune.evidence"], requiredSchemas["plenilune.analysis"])
	if j == nil {
		return 0
	}

	request := buildAnalysisRequest(j.before, j.after, j.deadlineMS)
	art, raw, callErr := callAnalyze(inv, request)
	if callErr != nil {
		fmt.Fprintf(out, "ANALYZE FAILED: %s\n", callErr.describe())
		return 4
	}
	if problems := checkArtifactShape(art); len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(out, "ARTIFACT REJECTED: %s\n", p)
		}
		return 5
	}
	fmt.Fprintf(out, "ARTIFACT: schema %s (acl engine %s, prefix-list engine %s)\n",
		art.Schema, art.Versions.ACLEngine, art.Versions.PrefixListEngine)
	for _, o := range art.Objects {
		fmt.Fprintf(out, "OBJECT %s %s %s\n", o.Domain, o.Name, o.Status)
	}
	s := art.Summary
	fmt.Fprintf(out, "SUMMARY: objects=%d changed=%d equivalent=%d unchanged=%d added=%d removed=%d unsupported=%d inconclusive=%d not_analyzed=%d\n",
		s.Objects, s.Changed, s.Equivalent, s.Unchanged, s.Added, s.Removed, s.Unsupported, s.Inconclusive, s.NotAnalyzed)
	fmt.Fprintf(out, "DIGESTS: content_sha256=%s artifact_sha256=%s\n", art.ContentSHA256, art.SHA256)

	// Idempotency: the identical request must reproduce the same
	// content identity. The caller keys its store on
	// content_sha256, so equality here is the cache hit — the
	// first artifact is kept and nothing is stored twice. A
	// different digest means the two runs are different results
	// (a timing-limited run is the usual cause); report both and
	// keep the first, but do not claim the hit.
	again, _, callErr := callAnalyze(inv, request)
	if callErr != nil {
		fmt.Fprintf(out, "ANALYZE FAILED: %s\n", callErr.describe())
		return 4
	}
	if again.ContentSHA256 == art.ContentSHA256 && again.SHA256 == art.SHA256 {
		fmt.Fprintf(out, "CACHE HIT: identical re-run reproduced content_sha256 %s — stored artifact reused\n", art.ContentSHA256)
	} else {
		fmt.Fprintf(out, "NO CACHE HIT: re-run produced content_sha256 %s, first run %s — keeping the first artifact; a timing-limited run is the usual cause\n",
			again.ContentSHA256, art.ContentSHA256)
	}

	res, callErr := callReplay(inv, j.before, j.after, raw)
	if callErr != nil {
		fmt.Fprintf(out, "REPLAY FAILED: %s\n", callErr.describe())
		return 6
	}
	switch res.Status {
	case "ANALYSIS_REPLAYED":
		if res.SHA256 != art.SHA256 || res.ContentSHA256 != art.ContentSHA256 {
			fmt.Fprintf(out, "REPLAY MISMATCH: replay confirmed sha256=%s content_sha256=%s, stored artifact has sha256=%s content_sha256=%s\n",
				res.SHA256, res.ContentSHA256, art.SHA256, art.ContentSHA256)
			return 5
		}
		fmt.Fprintf(out, "REPLAY: ANALYSIS_REPLAYED (rechecked_witnesses=%d) — artifact confirmed under both digests\n", res.RecheckedWitnesses)
		return 0
	case "TIMING_SENSITIVE":
		fmt.Fprintf(out, "REPLAY: TIMING_SENSITIVE — the result depends on run timing and is NOT final; do not cache it as a completed job\n")
		return 6
	default:
		fmt.Fprintf(out, "REPLAY FAILED: unexpected replay status %q\n", res.Status)
		return 6
	}
}

func main() {
	binary := flag.String("plenilune", "", "path to the plenilune binary (default: $PLENILUNE_BIN or plenilune on PATH)")
	beforePath := flag.String("before", "", "file holding the before configuration (omit both configuration flags to run the gate only)")
	afterPath := flag.String("after", "", "file holding the after configuration")
	beforeRef := flag.String("before-ref", "", "opaque caller label for the before configuration")
	afterRef := flag.String("after-ref", "", "opaque caller label for the after configuration")
	deadlineMS := flag.Int("deadline-ms", 30000, "per-job analysis deadline in milliseconds (contract range 1..120000)")
	flag.Parse()
	path := *binary
	if path == "" {
		path = os.Getenv("PLENILUNE_BIN")
	}
	if path == "" {
		path = "plenilune"
	}
	inv := execRunner(path)
	if (*beforePath == "") != (*afterPath == "") {
		fmt.Fprintln(os.Stderr, "-before and -after must be given together")
		os.Exit(2)
	}
	if *beforePath == "" {
		os.Exit(execute(inv, nil, os.Stdout))
	}
	before, err := os.ReadFile(*beforePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read before configuration: %v\n", err)
		os.Exit(2)
	}
	after, err := os.ReadFile(*afterPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read after configuration: %v\n", err)
		os.Exit(2)
	}
	j := &job{
		before:     source{Text: string(before), Ref: *beforeRef},
		after:      source{Text: string(after), Ref: *afterRef},
		deadlineMS: *deadlineMS,
	}
	os.Exit(execute(inv, j, os.Stdout))
}

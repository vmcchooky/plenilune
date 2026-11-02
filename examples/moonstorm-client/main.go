// Command moonstorm-client is the sample external caller for the
// Plenilune contract — the shape a future MoonStorm adapter takes.
//
// This part (P2-2) implements only the compatibility gate: before
// any configuration changes hands, the client asks the binary for
// its capabilities document (`plenilune capabilities --json`) and
// refuses the job early, with named reasons, when the tool's schema
// versions or features do not match what the caller requires. On a
// match it stops: the analyze/replay loop extends this same client
// in P2-7.
//
// The client depends only on the binary and the JSON contract — no
// Plenilune packages, no browser, no UI. Standard library only.
//
// Usage:
//
//	moonstorm-client [-plenilune PATH]
//
// The binary path comes from -plenilune, else $PLENILUNE_BIN, else
// "plenilune" on PATH. Exit codes: 0 compatible, 2 the capabilities
// call itself failed, 3 incompatible (reasons printed).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
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

func run(binary string, stdout, stderr *bytes.Buffer) int {
	cmd := exec.Command(binary, "capabilities", "--json")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "capabilities call failed: %v\n%s", err, stderr.String())
		return 2
	}
	var doc document
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		fmt.Fprintf(os.Stderr, "capabilities document is not valid JSON: %v\n", err)
		return 2
	}
	if reasons := gate(doc); len(reasons) > 0 {
		for _, r := range reasons {
			fmt.Printf("REFUSED: %s\n", r)
		}
		return 3
	}
	fmt.Printf("COMPATIBLE: plenilune %s (commit %s) — evidence %s, analysis %s, required features present\n",
		doc.Tool.Version, doc.Tool.Commit,
		requiredSchemas["plenilune.evidence"], requiredSchemas["plenilune.analysis"])
	return 0
}

func main() {
	binary := flag.String("plenilune", "", "path to the plenilune binary (default: $PLENILUNE_BIN or plenilune on PATH)")
	flag.Parse()
	path := *binary
	if path == "" {
		path = os.Getenv("PLENILUNE_BIN")
	}
	if path == "" {
		path = "plenilune"
	}
	var stdout, stderr bytes.Buffer
	os.Exit(run(path, &stdout, &stderr))
}

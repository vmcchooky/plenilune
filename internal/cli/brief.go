package cli

import (
	"fmt"
	"io"
	"strings"

	"plenilune/internal/evidence"
)

// Brief mode (diff --brief --out FILE) is a fixed-budget view of the same
// comparison artifact the default diff prints in full: the artifact JSON is
// written byte-identically to the caller-named file and stdout carries five
// stable, grep-able lines. Exit codes in brief mode are 0 EQUIVALENT,
// 1 DIFFERENT, 2 INCONCLUSIVE and 3 for any usage/parse/IO error; errors are
// one plain line on stderr instead of the usual JSON error document.

// wantsBrief reports whether the raw argument list requests brief mode. It
// scans the unparsed args so that even a flag-parse failure can be reported
// in brief form when the caller asked for it.
func wantsBrief(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--brief" || a == "-brief" {
			return true
		}
		for _, prefix := range []string{"--brief=", "-brief="} {
			if v, ok := strings.CutPrefix(a, prefix); ok {
				switch v {
				case "0", "f", "F", "false", "FALSE", "False":
					return false
				}
				return true
			}
		}
	}
	return false
}

// briefFail writes the single-line brief error and returns the brief error
// exit code. Messages are flattened so the output is always exactly one line.
func briefFail(stderr io.Writer, code, message string) int {
	message = strings.Join(strings.Fields(message), " ")
	if _, err := fmt.Fprintf(stderr, "error: %s %s\n", code, message); err != nil {
		return 3
	}
	return 3
}

// briefLines renders the five-line summary from the sealed artifact alone:
// verdict, the compared object's status line, the obligations listing, the
// modeled-scope limitations line and the artifact path. An INCONCLUSIVE
// verdict always carries its stop reason; when no conditions were supplied
// the obligations line says none were evaluated rather than implying a pass.
// A partial result (P1-2: the budget stopped the search) is marked on the
// verdict line itself — partial=true, the stop reason, and how much was
// decided (prefix pairs decided identically, rule pairs proven different) —
// so the line never reads as a plain complete verdict.
func briefLines(c evidence.Comparison, artifactPath string) string {
	r := c.Report
	var b strings.Builder
	reason := r.Reason
	if reason == "" {
		reason = r.Budget.StopReason
	}
	if reason == "" {
		reason = "unspecified"
	}
	switch {
	case r.Verdict == "INCONCLUSIVE":
		fmt.Fprintf(&b, "verdict: INCONCLUSIVE reason=%s", reason)
		if r.Partial != nil {
			fmt.Fprintf(&b, " partial=true stage=%s prefix_pairs=%d decided_pairs=%d", r.Partial.Stage, r.Partial.PrefixPairs, len(r.Partial.DecidedPairs))
		}
		b.WriteString("\n")
	case r.Verdict == "DIFFERENT" && r.Partial != nil:
		fmt.Fprintf(&b, "verdict: DIFFERENT partial=true reason=%s prefix_pairs=%d decided_pairs=%d\n", reason, r.Partial.PrefixPairs, len(r.Partial.DecidedPairs))
	default:
		fmt.Fprintf(&b, "verdict: %s\n", r.Verdict)
	}
	fmt.Fprintf(&b, "object: acl %s %s", c.Before.Name, r.Verdict)
	if r.Verdict == "DIFFERENT" {
		fmt.Fprintf(&b, " witnesses=%d", len(r.Witnesses))
		if r.WitnessesTruncated {
			b.WriteString(" witnesses_truncated=true")
		}
	}
	b.WriteString("\n")
	if len(r.Obligations) == 0 {
		b.WriteString("obligations: none evaluated\n")
	} else {
		parts := make([]string, len(r.Obligations))
		for i, ob := range r.Obligations {
			parts[i] = ob.ID + " " + ob.Status
		}
		fmt.Fprintf(&b, "obligations: %s\n", strings.Join(parts, ", "))
	}
	fmt.Fprintf(&b, "limitations: scope=%s\n", c.Scope)
	fmt.Fprintf(&b, "artifact: %s\n", artifactPath)
	return b.String()
}

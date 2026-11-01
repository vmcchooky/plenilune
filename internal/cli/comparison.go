package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
	"plenilune/internal/verify"
	"plenilune/internal/workflow"
	"runtime/debug"
)

func runComparison(ctx context.Context, cmd string, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var beforePath, afterPath, conditionsPath, artifactPath, outPath string
	var briefFlag bool
	fs.StringVar(&beforePath, "before", "", "original before ACL-only file")
	fs.StringVar(&afterPath, "after", "", "original after ACL-only file")
	opts := verify.DefaultOptions()
	if cmd == "diff" {
		fs.StringVar(&conditionsPath, "obligations", "", "operator-provided condition set")
		fs.BoolVar(&briefFlag, "brief", false, "print the fixed five-line summary; full evidence goes to --out")
		fs.StringVar(&outPath, "out", "", "file that receives the full evidence JSON (required with --brief)")
		fs.IntVar(&opts.Limits.MaxOperations, "max-operations", opts.Limits.MaxOperations, "logical operation cap (may lower default)")
		fs.IntVar(&opts.Limits.MaxBoxes, "max-boxes", opts.Limits.MaxBoxes, "workspace box cap (may lower default)")
		fs.DurationVar(&opts.Limits.Timeout, "timeout", opts.Limits.Timeout, "safety deadline (maximum 2s)")
		fs.IntVar(&opts.MaxWitnesses, "max-witnesses", opts.MaxWitnesses, "maximum reported witness packets, 1..64")
	} else {
		fs.StringVar(&artifactPath, "evidence", "", "full comparison evidence JSON")
	}
	briefScan := cmd == "diff" && wantsBrief(args)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		if briefScan {
			return briefFail(stderr, "USAGE", "invalid comparison arguments")
		}
		return fail(stderr, "USAGE", "invalid comparison arguments")
	}
	brief := cmd == "diff" && briefFlag
	failX := func(code, message string) int {
		if brief {
			return briefFail(stderr, code, message)
		}
		return fail(stderr, code, message)
	}
	if fs.NArg() != 0 || beforePath == "" || afterPath == "" || cmd == "replay" && artifactPath == "" {
		return failX("USAGE", "required comparison input is missing")
	}
	if cmd == "diff" {
		if brief && outPath == "" {
			return failX("USAGE", "--brief requires --out FILE for the full evidence artifact")
		}
		if !brief && outPath != "" {
			return failX("USAGE", "--out requires --brief")
		}
	}
	before, err := read(ctx, beforePath, ciscoacl.MaxInputBytes)
	if err != nil {
		return failX("INPUT_FAILED", err.Error())
	}
	after, err := read(ctx, afterPath, ciscoacl.MaxInputBytes)
	if err != nil {
		return failX("INPUT_FAILED", err.Error())
	}
	if cmd == "replay" {
		data, err := read(ctx, artifactPath, evidence.MaxComparisonBytes)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		var artifact evidence.Comparison
		if err := jsonio.Decode(data, evidence.MaxComparisonBytes, &artifact); err != nil {
			return fail(stderr, "INVALID_JSON", err.Error())
		}
		result, err := workflow.Replay(ctx, before, after, artifact)
		if err != nil {
			return fail(stderr, "EVIDENCE_MISMATCH", "comparison replay failed")
		}
		if code := output(out, stderr, result); code != 0 {
			return code
		}
		if result.Status == "TIMING_SENSITIVE" {
			return 2
		}
		return 0
	}
	var conditions *intent.Set
	if conditionsPath != "" {
		data, err := read(ctx, conditionsPath, intent.MaxJSONBytes)
		if err != nil {
			return failX("INPUT_FAILED", err.Error())
		}
		var s intent.Set
		if err := jsonio.Decode(data, intent.MaxJSONBytes, &s); err != nil {
			return failX("INVALID_OBLIGATION", err.Error())
		}
		n, err := intent.Normalize(s)
		if err != nil {
			return failX("INVALID_OBLIGATION", err.Error())
		}
		conditions = &n
	}
	artifact, err := workflow.Compare(ctx, before, after, conditions, opts, comparisonTool())
	if err != nil {
		var domain *workflow.Error
		if errors.As(err, &domain) {
			if brief {
				return briefFail(stderr, domain.Code, domain.Message)
			}
			return outputError(stderr, domain)
		}
		if ctx.Err() != nil {
			return failX("CANCELED", "operation canceled")
		}
		return failX("COMPARISON_FAILED", "invalid comparison options or engine input")
	}
	if brief {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(artifact); err != nil {
			return briefFail(stderr, "OUTPUT_FAILED", "cannot encode evidence")
		}
		if err := os.WriteFile(outPath, buf.Bytes(), 0o600); err != nil {
			return briefFail(stderr, "OUTPUT_FAILED", "cannot write artifact file")
		}
		if _, err := io.WriteString(out, briefLines(artifact, outPath)); err != nil {
			return briefFail(stderr, "OUTPUT_FAILED", "cannot write output")
		}
	} else if code := output(out, stderr, artifact); code != 0 {
		return code
	}
	switch artifact.Report.Verdict {
	case "EQUIVALENT":
		return 0
	case "DIFFERENT":
		return 1
	default:
		return 2
	}
}

func comparisonTool() evidence.Tool {
	commit := "unversioned"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				commit = setting.Value
			}
		}
	}
	return evidence.Tool{Version: evidence.ToolVersion, Commit: commit}
}

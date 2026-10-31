package cli

import (
	"context"
	"errors"
	"flag"
	"io"
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
	var beforePath, afterPath, conditionsPath, artifactPath string
	fs.StringVar(&beforePath, "before", "", "original before ACL-only file")
	fs.StringVar(&afterPath, "after", "", "original after ACL-only file")
	opts := verify.DefaultOptions()
	if cmd == "diff" {
		fs.StringVar(&conditionsPath, "obligations", "", "operator-provided condition set")
		fs.IntVar(&opts.Limits.MaxOperations, "max-operations", opts.Limits.MaxOperations, "logical operation cap (may lower default)")
		fs.IntVar(&opts.Limits.MaxBoxes, "max-boxes", opts.Limits.MaxBoxes, "workspace box cap (may lower default)")
		fs.DurationVar(&opts.Limits.Timeout, "timeout", opts.Limits.Timeout, "safety deadline (maximum 2s)")
		fs.IntVar(&opts.MaxWitnesses, "max-witnesses", opts.MaxWitnesses, "maximum reported witness packets, 1..64")
	} else {
		fs.StringVar(&artifactPath, "evidence", "", "full comparison evidence JSON")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid comparison arguments")
	}
	if fs.NArg() != 0 || beforePath == "" || afterPath == "" || cmd == "replay" && artifactPath == "" {
		return fail(stderr, "USAGE", "required comparison input is missing")
	}
	before, err := read(ctx, beforePath, ciscoacl.MaxInputBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	after, err := read(ctx, afterPath, ciscoacl.MaxInputBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
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
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		var s intent.Set
		if err := jsonio.Decode(data, intent.MaxJSONBytes, &s); err != nil {
			return fail(stderr, "INVALID_OBLIGATION", err.Error())
		}
		n, err := intent.Normalize(s)
		if err != nil {
			return fail(stderr, "INVALID_OBLIGATION", err.Error())
		}
		conditions = &n
	}
	artifact, err := workflow.Compare(ctx, before, after, conditions, opts, comparisonTool())
	if err != nil {
		var domain *workflow.Error
		if errors.As(err, &domain) {
			return outputError(stderr, domain)
		}
		if ctx.Err() != nil {
			return fail(stderr, "CANCELED", "operation canceled")
		}
		return fail(stderr, "COMPARISON_FAILED", "invalid comparison options or engine input")
	}
	if code := output(out, stderr, artifact); code != 0 {
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

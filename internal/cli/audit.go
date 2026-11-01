package cli

import (
	"context"
	"errors"
	"flag"
	"io"

	"plenilune/internal/audit"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/jsonio"
	"plenilune/internal/verify"
)

// runAudit is the offline display surface for the exact rule audit
// (P1-4): it prints the sealed audit record for one ACL-only file and
// never modifies the configuration. Exit codes: 0 when the audit
// completed (findings or not — a completed audit with dead rules is a
// successful audit), 2 when the audit is incomplete or the input
// fails. Budget flags may only lower the defaults, as with diff.
func runAudit(ctx context.Context, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath string
	opts := verify.DefaultOptions()
	fs.StringVar(&configPath, "config", "", "ACL-only input file")
	fs.IntVar(&opts.Limits.MaxOperations, "max-operations", opts.Limits.MaxOperations, "logical operation cap (may lower default)")
	fs.IntVar(&opts.Limits.MaxBoxes, "max-boxes", opts.Limits.MaxBoxes, "workspace box cap (may lower default)")
	fs.DurationVar(&opts.Limits.Timeout, "timeout", opts.Limits.Timeout, "safety deadline (maximum 2s)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid audit arguments")
	}
	if fs.NArg() != 0 || configPath == "" {
		return fail(stderr, "USAGE", "required audit inputs are missing")
	}
	data, err := read(ctx, configPath, ciscoacl.MaxInputBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	rec, err := audit.Audit(ctx, data, opts)
	if err != nil {
		var pe *ciscoacl.Error
		if errors.As(err, &pe) {
			return outputError(stderr, pe)
		}
		return fail(stderr, "AUDIT_INPUT_INVALID", "invalid audit inputs or options")
	}
	if code := output(out, stderr, rec); code != 0 {
		return code
	}
	if !rec.Report.Complete {
		return 2
	}
	return 0
}

// runAuditReplay recomputes an audit record offline from the original
// configuration and reports AUDIT_REPLAYED or TIMING_SENSITIVE,
// mirroring analyze-replay. Exit 0 only on AUDIT_REPLAYED.
func runAuditReplay(ctx context.Context, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet("audit-replay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath, evidencePath string
	fs.StringVar(&configPath, "config", "", "original ACL-only input file")
	fs.StringVar(&evidencePath, "evidence", "", "audit record JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid audit-replay arguments")
	}
	if fs.NArg() != 0 || configPath == "" || evidencePath == "" {
		return fail(stderr, "USAGE", "required audit-replay inputs are missing")
	}
	data, err := read(ctx, configPath, ciscoacl.MaxInputBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	raw, err := read(ctx, evidencePath, audit.MaxAuditBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	var rec audit.Record
	if err := jsonio.Decode(raw, audit.MaxAuditBytes, &rec); err != nil {
		return fail(stderr, "INVALID_JSON", err.Error())
	}
	res, err := audit.Replay(ctx, data, rec)
	if err != nil {
		var pe *ciscoacl.Error
		if errors.As(err, &pe) {
			return outputError(stderr, pe)
		}
		return fail(stderr, "EVIDENCE_MISMATCH", err.Error())
	}
	if code := output(out, stderr, struct {
		Status string `json:"status"`
		SHA256 string `json:"sha256"`
	}{res.Status, res.SHA256}); code != 0 {
		return code
	}
	if res.Status != "AUDIT_REPLAYED" {
		return 2
	}
	return 0
}

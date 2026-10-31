package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"plenilune/internal/agent"
	"plenilune/internal/ai"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
	"plenilune/internal/orchestrate"
	"plenilune/internal/patch"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
)

func runRepair(ctx context.Context, cmd string, args []string, out, stderr io.Writer) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var beforePath, afterPath, policyPath, patchPath, artifactPath, intentText, mode string
	fs.StringVar(&beforePath, "before", "", "original before ACL")
	fs.StringVar(&afterPath, "after", "", "original after ACL")
	if cmd == "repair-replay" {
		fs.StringVar(&artifactPath, "evidence", "", "repair evidence JSON")
	} else {
		fs.StringVar(&policyPath, "policy", "", "operator policy JSON")
	}
	if cmd == "repair" {
		fs.StringVar(&patchPath, "patch", "", "pinned patch JSON")
	}
	if cmd == "assist" {
		fs.StringVar(&intentText, "intent", "", "operator change intent")
		fs.StringVar(&mode, "ai", "off", "off or on; on sends bounded ACL excerpts and intent to Meta")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid repair/session arguments")
	}
	if fs.NArg() != 0 || beforePath == "" || afterPath == "" || cmd == "repair" && (policyPath == "" || patchPath == "") || cmd == "repair-replay" && artifactPath == "" {
		return fail(stderr, "USAGE", "required repair/session inputs are missing")
	}
	if cmd == "assist" && (mode != "off" && mode != "on") {
		return fail(stderr, "USAGE", "--ai must be off or on")
	}
	b, err := read(ctx, beforePath, ciscoacl.MaxInputBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	a, err := read(ctx, afterPath, ciscoacl.MaxInputBytes)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	if cmd == "repair-replay" {
		data, err := read(ctx, artifactPath, repair.MaxJSONBytes)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		var r repair.Result
		if err := jsonio.Decode(data, repair.MaxJSONBytes, &r); err != nil {
			return fail(stderr, "INVALID_JSON", err.Error())
		}
		result, err := repair.Replay(ctx, b, a, r)
		if err != nil {
			return fail(stderr, "EVIDENCE_MISMATCH", "repair replay failed")
		}
		if code := output(out, stderr, result); code != 0 {
			return code
		}
		if result.Status != "REPAIR_REPLAYED" {
			return 2
		}
		return 0
	}
	var policy *repair.Policy
	if policyPath != "" {
		data, err := read(ctx, policyPath, intent.MaxJSONBytes)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		var p repair.Policy
		if err := jsonio.Decode(data, intent.MaxJSONBytes, &p); err != nil {
			return fail(stderr, "INVALID_POLICY", err.Error())
		}
		n, err := repair.NormalizePolicy(p)
		if err != nil {
			return fail(stderr, "INVALID_POLICY", "invalid policy conditions or scope")
		}
		policy = &n
	}
	if cmd == "assist" {
		var provider ai.Completer
		model := ""
		if mode == "on" {
			cfg, err := ai.ConfigFromEnv(os.Getenv)
			if err != nil && !errors.Is(err, ai.ErrNotConfigured) {
				return fail(stderr, "AI_CONFIG_INVALID", "invalid server Meta configuration")
			}
			if err == nil {
				// This CLI is qualified for one model only, matching the demo HTTP
				// guard. A loopback base URL must not widen the model allowlist.
				if cfg.Model != ai.DefaultModel {
					return fail(stderr, "AI_MODEL_UNSUPPORTED", "only muse-spark-1.3-contributor is enabled")
				}
				client, err := ai.New(cfg)
				if err != nil {
					return fail(stderr, "AI_CONFIG_INVALID", "invalid Meta configuration")
				}
				provider, model = client, cfg.Model
			}
		}
		r, err := orchestrate.Run(ctx, orchestrate.Request{Before: string(b), After: string(a), Intent: intentText, Policy: policy, Enabled: mode == "on"}, provider, model)
		if err != nil {
			return fail(stderr, "SESSION_INPUT_INVALID", "invalid session inputs or policy")
		}
		if code := output(out, stderr, r); code != 0 {
			return code
		}
		if r.AI.Status == agent.StatusCompleted || r.AI.Status == agent.StatusDisabled {
			return 0
		}
		return 2
	}
	data, err := read(ctx, patchPath, 16<<10)
	if err != nil {
		return fail(stderr, "INPUT_FAILED", err.Error())
	}
	var req patch.Request
	if err := jsonio.Decode(data, 16<<10, &req); err != nil {
		return fail(stderr, "INVALID_PATCH", err.Error())
	}
	r, err := repair.Verify(ctx, b, a, policy, req, verify.DefaultOptions())
	if err != nil {
		return fail(stderr, "REPAIR_INPUT_INVALID", "invalid repair inputs or options")
	}
	if code := output(out, stderr, r); code != 0 {
		return code
	}
	switch r.Status {
	case "VERIFIED":
		return 0
	case "REJECTED":
		return 1
	default:
		return 2
	}
}

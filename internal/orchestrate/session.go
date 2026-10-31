// Package orchestrate joins provider orchestration to the deterministic core.
// AI artifacts are sidecars; comparison and repair receipts keep their own hashes.
package orchestrate

import (
	"context"
	"plenilune/internal/agent"
	"plenilune/internal/ai"
	"plenilune/internal/engineadapter"
	"plenilune/internal/evidence"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
)

type Request struct {
	Before  string          `json:"before"`
	After   string          `json:"after"`
	Intent  string          `json:"intent"`
	Policy  *repair.Policy  `json:"policy"`
	Enabled bool            `json:"enabled"`
	Options *verify.Options `json:"options,omitempty"`
}

type Result struct {
	Schema     string              `json:"schema"`
	Comparison evidence.Comparison `json:"comparison"`
	AI         agent.Result        `json:"ai"`
	Repairs    []repair.Result     `json:"repairs"`
}

func Run(ctx context.Context, in Request, provider ai.Completer, model string) (Result, error) {
	opts := verify.DefaultOptions()
	if in.Options != nil {
		opts = *in.Options
	}
	engine, err := engineadapter.New(ctx, []byte(in.Before), []byte(in.After), in.Policy, opts)
	if err != nil {
		return Result{}, err
	}
	var input agent.Input
	// Provider-facing excerpt limits must not constrain offline comparison or
	// a truthful missing-provider response; Session.Run short-circuits both.
	if in.Enabled && provider != nil {
		input, err = engine.Input(in.Intent)
		if err != nil {
			return Result{}, err
		}
	}
	session, err := agent.New(agent.Config{Enabled: in.Enabled, Provider: provider, Engine: engine, Model: model})
	if err != nil {
		return Result{}, err
	}
	result, err := session.Run(ctx, input)
	if err != nil {
		return Result{}, err
	}
	comparison, err := engine.Comparison()
	if err != nil {
		return Result{}, err
	}
	repairs, err := engine.Repairs()
	if err != nil {
		return Result{}, err
	}
	return Result{Schema: "plenilune.session/v1", Comparison: comparison, AI: result, Repairs: repairs}, nil
}

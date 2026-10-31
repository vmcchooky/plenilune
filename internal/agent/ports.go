// Package agent owns the Meta tool loop, budgets and evidence-reference
// validation. It never produces ACL verdicts: the injected Engine does.
package agent

import (
	"context"
	"plenilune/internal/ai"
	"plenilune/internal/intent"
	"time"
)

const (
	ProviderName  = "meta-model-api"
	PromptVersion = "meta-orchestrator/v3"
	ResultSchema  = "plenilune.ai/v1"

	DefaultMaxModelRequests     = 8
	DefaultMaxToolCalls         = 12
	DefaultMaxRepairProposals   = 2
	DefaultMaxOutputCorrections = 2
	DefaultSessionDeadline      = 60 * time.Second
	DefaultRetryBackoff         = 250 * time.Millisecond
)

// Status is the session outcome. It is deliberately independent of any ACL
// verdict; checks and repair results come from the Engine.
type Status string

const (
	StatusDisabled            Status = "disabled"
	StatusNotConfigured       Status = "not_configured"
	StatusUnavailable         Status = "unavailable"
	StatusClarificationNeeded Status = "clarification_needed"
	StatusPartial             Status = "partial"
	StatusCompleted           Status = "completed"
	StatusCanceled            Status = "canceled"
)

// Input is the bounded change ticket the caller sends to the model. Intent and
// ACL excerpts are untrusted data; they are not executed or interpreted as
// instructions by this package.
type Input struct {
	Intent    string
	ACLName   string
	Before    []string
	After     []string
	Changes   []Change
	Truncated bool
	// Context supplied by orchestration; acceptance is independently checked
	// against the engine's frozen policy authority, never inferred from JSON.
	AcceptedConditions *intent.Set
	// RepairAvailable tells the tool registry whether accepted policy exists.
	// It is caller-supplied, not model-supplied, and is not sent in the payload.
	RepairAvailable bool

	// Base digests and scope are supplied by the deterministic caller. They are
	// pinned into every patch request; the model cannot change them.
	BaseBeforeSHA256 string
	BaseAfterSHA256  string
}

// PolicyAuthority is optional so proposal-only engine implementations remain
// usable. It binds operator acceptance to the exact inputs and definition hash.
type PolicyAuthority interface {
	OperatorAccepted(beforeSHA, afterSHA, conditionsDigest string) bool
}

// Engine is implemented by the exact-verifier adapter. It owns witnesses,
// digests, allowed-change policy and all verdicts. It must be read-only except
// for computing an in-memory candidate patch.
type Engine interface {
	ListChanges(ctx context.Context, req ListChangesRequest) (ChangesPage, error)
	CheckObligations(ctx context.Context, set intent.Set) (ObligationCheck, error)
	GetEvidence(ctx context.Context, req EvidenceRequest) (EvidenceBatch, error)
	VerifyPatch(ctx context.Context, req PatchCheckRequest) (PatchCheckResult, error)
}

type ListChangesRequest struct {
	IDs    []string
	Cursor string
	Limit  int
}

type Change struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Side    string `json:"side"`
	Ordinal *int   `json:"ordinal,omitempty"`
	Line    *int   `json:"line,omitempty"`
	Summary string `json:"summary"`
}

type ChangesPage struct {
	Items       []Change `json:"items"`
	NextCursor  string   `json:"next_cursor,omitempty"`
	Truncated   bool     `json:"truncated"`
	EvidenceIDs []string `json:"evidence_ids"`
}

// ObligationStatus values are verifier-owned. The model never emits them.
const (
	ObligationHolds        = "HOLDS"
	ObligationViolated     = "VIOLATED"
	ObligationInconclusive = "INCONCLUSIVE"
)

type ObligationCheck struct {
	ObligationsDigest string             `json:"obligations_digest"`
	Results           []ObligationResult `json:"results"`
}

type ObligationResult struct {
	ID          string   `json:"id"`
	Status      string   `json:"status"`
	EvidenceIDs []string `json:"evidence_ids"`
	Reason      string   `json:"reason,omitempty"`
}

type EvidenceRequest struct {
	IDs []string
}

type EvidenceBatch struct {
	Records []EvidenceRecord `json:"records"`
}

type EvidenceRecord struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

// RuleRef is pinned to one immutable base-after artifact by digest, ordinal
// and source line. It is not a durable rule identifier.
type RuleRef struct {
	BaseAfterSHA256 string `json:"base_after_sha256"`
	Ordinal         int    `json:"ordinal"`
	Line            int    `json:"line"`
}

type Edit struct {
	Op      string   `json:"op"` // add_before, append, remove, replace
	Ref     *RuleRef `json:"ref,omitempty"`
	Content string   `json:"content,omitempty"`
}

// PatchCheckRequest carries the caller's pins plus model-authored edits. The
// Engine owns the frozen allowed-change region and re-verifies the base.
type PatchCheckRequest struct {
	Edits             []Edit `json:"edits"`
	Rationale         string `json:"rationale,omitempty"`
	BaseBeforeSHA256  string `json:"base_before_sha256"`
	BaseAfterSHA256   string `json:"base_after_sha256"`
	ObligationsDigest string `json:"obligations_digest"`
	Scope             string `json:"scope"`
}

const (
	PatchVerified       = "VERIFIED"
	PatchRejected       = "REJECTED"
	PatchUnverified     = "UNVERIFIED"
	CollateralVerified  = "VERIFIED"
	CollateralFailed    = "FAILED"
	CollateralUnchecked = "UNCHECKED"
)

type PatchCheckResult struct {
	Status            string             `json:"status"`
	ObligationsDigest string             `json:"obligations_digest"`
	ObligationResults []ObligationResult `json:"obligation_results"`
	CollateralStatus  string             `json:"collateral_status"`
	EvidenceIDs       []string           `json:"evidence_ids"`
	Reason            string             `json:"reason,omitempty"`
}

type AcceptedObligations struct {
	Set              intent.Set `json:"set"`
	SHA256           string     `json:"sha256"`
	Source           string     `json:"source"`
	OperatorAccepted bool       `json:"operator_accepted"`
}

type Repair struct {
	Attempts          int                `json:"attempts"`
	Status            string             `json:"status"`
	Edits             []Edit             `json:"edits,omitempty"`
	Rationale         string             `json:"rationale,omitempty"`
	ObligationsDigest string             `json:"obligations_digest,omitempty"`
	Obligations       []ObligationResult `json:"obligations,omitempty"`
	CollateralStatus  string             `json:"collateral_status,omitempty"`
	EvidenceIDs       []string           `json:"evidence_ids,omitempty"`
	Reason            string             `json:"reason,omitempty"`
}

type Claim struct {
	Text          string   `json:"text"`
	ObligationIDs []string `json:"obligation_ids,omitempty"`
	EvidenceIDs   []string `json:"evidence_ids"`
	AIGenerated   bool     `json:"ai_generated"`
}

type Provenance struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	PromptVersion   string   `json:"prompt_version"`
	Requests        int      `json:"requests"`
	Retries         int      `json:"retries"`
	ToolCalls       int      `json:"tool_calls"`
	RepairProposals int      `json:"repair_proposals"`
	Usage           ai.Usage `json:"usage"`
	RequestSHA256   []string `json:"request_sha256"`
	ResponseSHA256  []string `json:"response_sha256"`
	ElapsedMS       int64    `json:"elapsed_ms"`
}

type Result struct {
	Schema        string               `json:"schema"`
	Status        Status               `json:"status"`
	Reason        string               `json:"reason,omitempty"`
	Obligations   *AcceptedObligations `json:"obligations,omitempty"`
	Checks        []ObligationResult   `json:"checks,omitempty"`
	Repair        *Repair              `json:"repair,omitempty"`
	Claims        []Claim              `json:"claims,omitempty"`
	DroppedClaims int                  `json:"dropped_claims,omitempty"`
	Questions     []string             `json:"questions,omitempty"`
	Summary       string               `json:"summary,omitempty"`
	Provenance    Provenance           `json:"provenance"`
}

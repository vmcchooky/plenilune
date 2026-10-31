package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"plenilune/internal/ai"
	"plenilune/internal/intent"
)

// Reasoning tokens count toward max_output_tokens. The previous 1200-token
// budget was exhausted before a usable final JSON in a live UI session.
const outputTokensPerRequest = 3000

type Config struct {
	Enabled              bool
	Provider             ai.Completer
	Engine               Engine
	Model                string
	PromptVersion        string
	MaxModelRequests     int
	MaxToolCalls         int
	MaxRepairProposals   int
	MaxOutputCorrections int
	SessionDeadline      time.Duration
	RetryBackoff         time.Duration

	// PromptOnlyJSON omits response_format and relies on the system prompt plus
	// the strict local decoder. Use it only if a live smoke shows the provider
	// rejects tools and structured output in the same request.
	PromptOnlyJSON bool
}

type Session struct {
	cfg       Config
	tools     []ai.Tool
	toolNames map[string]bool
}

func New(cfg Config) (*Session, error) {
	if cfg.Model == "" {
		cfg.Model = ai.DefaultModel
	}
	if cfg.PromptVersion == "" {
		cfg.PromptVersion = PromptVersion
	}
	if cfg.MaxModelRequests == 0 {
		cfg.MaxModelRequests = DefaultMaxModelRequests
	}
	if cfg.MaxToolCalls == 0 {
		cfg.MaxToolCalls = DefaultMaxToolCalls
	}
	if cfg.MaxRepairProposals == 0 {
		cfg.MaxRepairProposals = DefaultMaxRepairProposals
	}
	if cfg.MaxOutputCorrections == 0 {
		cfg.MaxOutputCorrections = DefaultMaxOutputCorrections
	}
	if cfg.SessionDeadline <= 0 {
		cfg.SessionDeadline = DefaultSessionDeadline
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = DefaultRetryBackoff
	}
	if cfg.RetryBackoff < 0 {
		return nil, errors.New("retry backoff must not be negative")
	}
	if cfg.MaxModelRequests < 1 || cfg.MaxModelRequests > 32 ||
		cfg.MaxToolCalls < 1 || cfg.MaxToolCalls > 64 ||
		cfg.MaxRepairProposals < 0 || cfg.MaxRepairProposals > 8 ||
		cfg.MaxOutputCorrections < 1 || cfg.MaxOutputCorrections > 8 {
		return nil, errors.New("invalid session budget")
	}
	tools := toolDefinitions()
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		names[t.Function.Name] = true
	}
	return &Session{cfg: cfg, tools: tools, toolNames: names}, nil
}

type acceptedState struct {
	set    intent.Set
	digest string
	check  *ObligationCheck
}

type runner struct {
	session *Session
	ctx     context.Context
	in      Input

	messages    []ai.Message
	accepted    *acceptedState
	known       map[string]bool
	acceptedIDs map[string]bool
	checks      []ObligationResult
	repair      *Repair
	questions   []string
	final       *modelOutput
	claims      []Claim
	dropped     int

	requests    int
	retries     int
	toolCalls   int
	proposals   int
	corrections int
	usage       ai.Usage
	reqHashes   []string
	respHash    []string
	modelName   string

	toolsDisabled bool
	toolNoteAdded bool
}

func (s *Session) Run(ctx context.Context, in Input) (Result, error) {
	start := time.Now()
	base := Provenance{
		Provider:      ProviderName,
		Model:         s.cfg.Model,
		PromptVersion: s.cfg.PromptVersion,
	}
	if !s.cfg.Enabled {
		base.ElapsedMS = time.Since(start).Milliseconds()
		return Result{Schema: ResultSchema, Status: StatusDisabled, Reason: "ai_disabled", Provenance: base}, nil
	}
	if s.cfg.Provider == nil {
		base.ElapsedMS = time.Since(start).Milliseconds()
		return Result{Schema: ResultSchema, Status: StatusNotConfigured, Reason: "provider_not_configured", Provenance: base}, nil
	}
	if s.cfg.Engine == nil {
		base.ElapsedMS = time.Since(start).Milliseconds()
		return Result{Schema: ResultSchema, Status: StatusNotConfigured, Reason: "engine_not_configured", Provenance: base}, nil
	}
	if err := validateInput(in); err != nil {
		return Result{}, err
	}
	if in.AcceptedConditions != nil {
		n, h, err := intent.Digest(*in.AcceptedConditions)
		if err != nil {
			return Result{}, inputError("accepted conditions are invalid")
		}
		authority, ok := s.cfg.Engine.(PolicyAuthority)
		if !ok || !authority.OperatorAccepted(in.BaseBeforeSHA256, in.BaseAfterSHA256, h) {
			return Result{}, inputError("accepted conditions are not bound to the engine policy")
		}
		in.AcceptedConditions = &n
	}
	if ctx.Err() != nil {
		base.ElapsedMS = time.Since(start).Milliseconds()
		return Result{Schema: ResultSchema, Status: StatusCanceled, Reason: "canceled", Provenance: base}, nil
	}
	user, err := buildPayload(in)
	if err != nil {
		return Result{}, errors.New("cannot build model input")
	}
	runCtx, cancel := context.WithTimeout(ctx, s.cfg.SessionDeadline)
	defer cancel()
	r := &runner{
		session:     s,
		ctx:         runCtx,
		in:          in,
		messages:    []ai.Message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: string(user)}},
		known:       make(map[string]bool),
		acceptedIDs: make(map[string]bool),
	}
	result := r.loop()
	result.Provenance = r.provenance(start)
	return result, nil
}

func (r *runner) loop() Result {
	for {
		if r.ctx.Err() != nil {
			return r.result(StatusCanceled, "canceled")
		}
		if r.requests >= r.session.cfg.MaxModelRequests {
			return r.limitResult("model_budget_exhausted")
		}
		resp, err := r.modelCall()
		if err != nil {
			if r.ctx.Err() != nil {
				return r.result(StatusCanceled, "canceled")
			}
			if errors.Is(err, errModelBudget) {
				return r.limitResult("model_budget_exhausted")
			}
			return r.limitResult(providerReason(err))
		}
		switch resp.FinishReason {
		case "length":
			if !r.correctTruncated() {
				return r.limitResult("invalid_model_output")
			}
			continue
		case "content_filter":
			return r.limitResult("provider_refused")
		}
		if len(resp.ToolCalls) > 0 {
			if r.toolsDisabled {
				return r.limitResult("tool_budget_exhausted")
			}
			r.messages = append(r.messages, assistantMessage(resp))
			for _, call := range resp.ToolCalls {
				content, toolErr := r.executeTool(call)
				if toolErr != nil {
					if r.ctx.Err() != nil {
						return r.result(StatusCanceled, "canceled")
					}
					return r.limitResult("tool_failed")
				}
				r.messages = append(r.messages, ai.Message{Role: "tool", ToolCallID: call.ID, Content: string(content)})
			}
			if r.toolsDisabled && !r.toolNoteAdded {
				r.messages = append(r.messages, ai.Message{
					Role:    "user",
					Content: `{"error":{"code":"tool_budget_exhausted"},"instruction":"Return the final JSON object now; do not request more tools."}`,
				})
				r.toolNoteAdded = true
			}
			continue
		}

		out, err := parseModelOutput(resp.Content)
		if err != nil {
			if !r.correct(resp.Content, "invalid_output", err.Error()) {
				return r.limitResult("invalid_model_output")
			}
			continue
		}
		if out.Status == "clarification_needed" {
			if r.accepted != nil {
				if !r.correct(resp.Content, "invalid_output", "clarification must precede the accepted obligation set") {
					return r.limitResult("invalid_model_output")
				}
				continue
			}
			if err := validateClarification(out); err != nil {
				if !r.correct(resp.Content, "invalid_output", err.Error()) {
					return r.limitResult("invalid_model_output")
				}
				continue
			}
			r.final = out
			r.questions = append([]string(nil), out.Questions...)
			return r.result(StatusClarificationNeeded, "clarification_requested")
		}
		if out.Status != "ok" {
			if !r.correct(resp.Content, "invalid_output", "status must be ok or clarification_needed") {
				return r.limitResult("invalid_model_output")
			}
			continue
		}

		if r.accepted == nil {
			if len(out.Obligations) == 0 {
				if !r.correct(resp.Content, "invalid_output", "propose obligations or ask a clarification before finishing") {
					return r.limitResult("invalid_model_output")
				}
				continue
			}
			set, digest, err := validateObligations(out.Obligations)
			if err != nil {
				if !r.correct(resp.Content, "invalid_obligations", err.Error()) {
					return r.limitResult("invalid_model_output")
				}
				continue
			}
			if !r.matchesOperatorPolicy(digest) {
				if !r.correct(resp.Content, "operator_policy_mismatch", "repeat the operator-accepted conditions without changing them") {
					return r.limitResult("operator_policy_mismatch")
				}
				continue
			}
			r.messages = append(r.messages, assistantMessage(resp))
			r.accept(set, digest)
			content, checkErr := r.checkAccepted()
			if checkErr != nil {
				if r.ctx.Err() != nil {
					return r.result(StatusCanceled, "canceled")
				}
				return r.limitResult("tool_failed")
			}
			r.messages = append(r.messages, ai.Message{Role: "user", Content: string(content)})
			continue
		}

		if err := r.applyFinal(out); err != nil {
			if !r.correct(resp.Content, "invalid_output", err.Error()) {
				return r.limitResult("invalid_model_output")
			}
			continue
		}
		return r.finish()
	}
}

var errModelBudget = errors.New("model request budget exhausted")

func providerReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "provider_timeout"
	}
	var se *ai.StatusError
	if errors.As(err, &se) {
		switch {
		case se.Status == 401 || se.Status == 403:
			return "provider_auth_failed"
		case se.Status == 429:
			return "provider_rate_limited"
		case se.Status >= 500:
			return "provider_unavailable"
		default:
			return "provider_rejected"
		}
	}
	var te *ai.TransportError
	if errors.As(err, &te) {
		switch te.Code {
		case "timeout":
			return "provider_timeout"
		case "request_too_large":
			return "provider_request_too_large"
		case "response_too_large":
			return "provider_response_too_large"
		case "invalid_response", "invalid_response_json", "content_too_large":
			return "provider_invalid_response"
		}
	}
	return "provider_unavailable"
}

func (r *runner) toolsForRequest() []ai.Tool {
	if r.toolsDisabled {
		return nil
	}
	// Repairs are meaningful only after the operator accepted the policy in the
	// caller. Offering verify_patch before that produces unverified candidates
	// and an avoidable partial session.
	if r.in.RepairAvailable {
		return r.session.tools
	}
	tools := make([]ai.Tool, 0, len(r.session.tools))
	for _, tool := range r.session.tools {
		if tool.Function.Name != "verify_patch" {
			tools = append(tools, tool)
		}
	}
	return tools
}

func (r *runner) modelCall() (ai.Response, error) {
	req := ai.Request{
		Model:     r.session.cfg.Model,
		Messages:  r.messages,
		Tools:     r.toolsForRequest(),
		MaxTokens: outputTokensPerRequest,
	}
	if !r.session.cfg.PromptOnlyJSON {
		req.ResponseFormat = &ai.ResponseFormat{
			Type:       "json_schema",
			JSONSchema: &ai.JSONSchema{Name: "PleniluneMetaOutput", Schema: outputSchema()},
		}
	}
	if r.toolsDisabled {
		req.Tools = nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		if r.requests >= r.session.cfg.MaxModelRequests {
			return ai.Response{}, errModelBudget
		}
		if r.ctx.Err() != nil {
			return ai.Response{}, r.ctx.Err()
		}
		r.requests++
		resp, err := r.session.cfg.Provider.Complete(r.ctx, req)
		if err == nil {
			r.recordResponse(resp)
			return resp, nil
		}
		if r.ctx.Err() != nil {
			return ai.Response{}, r.ctx.Err()
		}
		if !ai.Retriable(err) || attempt == 1 || r.requests >= r.session.cfg.MaxModelRequests {
			return ai.Response{}, err
		}
		r.retries++
		if r.session.cfg.RetryBackoff > 0 {
			timer := time.NewTimer(r.session.cfg.RetryBackoff)
			select {
			case <-r.ctx.Done():
				timer.Stop()
				return ai.Response{}, r.ctx.Err()
			case <-timer.C:
			}
		}
	}
	return ai.Response{}, errors.New("model request retry exhausted")
}

func (r *runner) recordResponse(resp ai.Response) {
	r.usage.PromptTokens += resp.Usage.PromptTokens
	r.usage.CompletionTokens += resp.Usage.CompletionTokens
	r.usage.TotalTokens += resp.Usage.TotalTokens
	if resp.RequestSHA256 != "" {
		r.reqHashes = append(r.reqHashes, resp.RequestSHA256)
	}
	if resp.ResponseSHA256 != "" {
		r.respHash = append(r.respHash, resp.ResponseSHA256)
	}
	if resp.Model != "" {
		r.modelName = resp.Model
	}
}

func assistantMessage(resp ai.Response) ai.Message {
	return ai.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls, ReplayItems: resp.ReplayItems}
}

func (r *runner) accept(set intent.Set, digest string) {
	r.accepted = &acceptedState{set: set, digest: digest}
	for _, ob := range set.Obligations {
		r.acceptedIDs[ob.ID] = true
	}
}

func (r *runner) correct(content, code, message string) bool {
	if r.corrections >= r.session.cfg.MaxOutputCorrections {
		return false
	}
	r.corrections++
	if content != "" {
		r.messages = append(r.messages, ai.Message{Role: "assistant", Content: content})
	}
	correction, err := json.Marshal(struct {
		Error       toolProblem `json:"error"`
		Instruction string      `json:"instruction"`
	}{
		Error:       toolProblem{Code: code, Message: message},
		Instruction: "Correct it and return only the required JSON object or a valid tool call.",
	})
	if err != nil {
		return false
	}
	r.messages = append(r.messages, ai.Message{Role: "user", Content: string(correction)})
	return true
}

// correctTruncated starts a fresh, tools-off final turn instead of echoing the
// truncated assistant payload back into the context. Replaying a full truncated
// payload on every correction tends to recreate the same truncation.
func (r *runner) correctTruncated() bool {
	if r.corrections >= r.session.cfg.MaxOutputCorrections {
		return false
	}
	r.corrections++
	r.toolsDisabled = true
	correction, err := json.Marshal(struct {
		Error       toolProblem `json:"error"`
		Instruction string      `json:"instruction"`
	}{
		Error:       toolProblem{Code: "invalid_output", Message: fmt.Sprintf("model output was truncated by the %d-token limit", outputTokensPerRequest)},
		Instruction: "Do not repeat or continue truncated text and do not call tools. Return only the smallest valid final JSON object now; keep summary under 200 characters.",
	})
	if err != nil {
		return false
	}
	r.messages = append(r.messages, ai.Message{Role: "user", Content: string(correction)})
	return true
}

func (r *runner) finish() Result {
	allHold := len(r.checks) > 0
	for _, c := range r.checks {
		if c.Status != ObligationHolds {
			allHold = false
			break
		}
	}
	verified := r.repair != nil && r.repair.Status == PatchVerified
	switch {
	case r.dropped > 0:
		return r.result(StatusPartial, "claims_dropped")
	case verified:
		return r.result(StatusCompleted, "")
	case len(r.checks) == 0:
		return r.result(StatusPartial, "checks_unavailable")
	case allHold:
		return r.result(StatusCompleted, "")
	default:
		if r.hasViolation() {
			return r.result(StatusPartial, "repair_unverified")
		}
		return r.result(StatusPartial, "obligations_not_held")
	}
}

func (r *runner) hasViolation() bool {
	for _, c := range r.checks {
		if c.Status == ObligationViolated {
			return true
		}
	}
	return false
}

func (r *runner) limitResult(reason string) Result {
	if r.ctx.Err() != nil {
		return r.result(StatusCanceled, "canceled")
	}
	if r.accepted == nil {
		return r.result(StatusUnavailable, reason)
	}
	return r.result(StatusPartial, reason)
}

func (r *runner) result(status Status, reason string) Result {
	out := Result{
		Schema:        ResultSchema,
		Status:        status,
		Reason:        reason,
		Checks:        append([]ObligationResult(nil), r.checks...),
		Repair:        cloneRepair(r.repair),
		Claims:        append([]Claim(nil), r.claims...),
		DroppedClaims: r.dropped,
		Questions:     append([]string(nil), r.questions...),
		Summary:       r.summary(),
	}
	if r.accepted != nil {
		out.Obligations = &AcceptedObligations{
			Set:              r.accepted.set,
			SHA256:           r.accepted.digest,
			Source:           "ai_proposed_validated",
			OperatorAccepted: r.operatorAccepted(r.accepted.digest),
		}
		if out.Obligations.OperatorAccepted {
			out.Obligations.Source = "operator_accepted"
		}
	}
	return out
}

func (r *runner) operatorAccepted(digest string) bool {
	authority, ok := r.session.cfg.Engine.(PolicyAuthority)
	return ok && authority.OperatorAccepted(r.in.BaseBeforeSHA256, r.in.BaseAfterSHA256, digest)
}
func (r *runner) matchesOperatorPolicy(digest string) bool {
	if r.in.AcceptedConditions == nil {
		return true
	}
	_, expected, err := intent.Digest(*r.in.AcceptedConditions)
	return err == nil && digest == expected && r.operatorAccepted(digest)
}

func (r *runner) summary() string {
	if r.final == nil {
		return ""
	}
	return strings.TrimSpace(r.final.Summary)
}

func (r *runner) provenance(start time.Time) Provenance {
	model := r.modelName
	if model == "" {
		model = r.session.cfg.Model
	}
	return Provenance{
		Provider:        ProviderName,
		Model:           model,
		PromptVersion:   r.session.cfg.PromptVersion,
		Requests:        r.requests,
		Retries:         r.retries,
		ToolCalls:       r.toolCalls,
		RepairProposals: r.proposals,
		Usage:           r.usage,
		RequestSHA256:   append([]string(nil), r.reqHashes...),
		ResponseSHA256:  append([]string(nil), r.respHash...),
		ElapsedMS:       time.Since(start).Milliseconds(),
	}
}

func cloneRepair(in *Repair) *Repair {
	if in == nil {
		return nil
	}
	out := *in
	out.Edits = append([]Edit(nil), in.Edits...)
	out.Obligations = append([]ObligationResult(nil), in.Obligations...)
	out.EvidenceIDs = append([]string(nil), in.EvidenceIDs...)
	return &out
}

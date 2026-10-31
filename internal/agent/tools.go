package agent

import (
	"encoding/json"
	"strconv"
	"strings"

	"plenilune/internal/ai"
	"plenilune/internal/intent"
	"plenilune/internal/jsonio"
)

const maxObservationBytes = 16 << 10

type observation struct {
	Tool   string       `json:"tool"`
	Result any          `json:"result,omitempty"`
	Error  *toolProblem `json:"error,omitempty"`
	Note   string       `json:"note,omitempty"`
}

type toolProblem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func observe(tool string, result any, note string) []byte {
	body, err := json.Marshal(observation{Tool: tool, Result: result, Note: note})
	if err != nil || len(body) > maxObservationBytes {
		return errorObservation(tool, "result_too_large", "tool result exceeds the observation limit")
	}
	return body
}

func errorObservation(tool, code, message string) []byte {
	body, err := json.Marshal(observation{Tool: tool, Error: &toolProblem{Code: code, Message: message}})
	if err != nil {
		return []byte(`{"error":{"code":"internal","message":"tool observation failed"}}`)
	}
	return body
}

func (r *runner) takeTool() bool {
	if r.toolCalls >= r.session.cfg.MaxToolCalls {
		r.toolsDisabled = true
		return false
	}
	r.toolCalls++
	if r.toolCalls >= r.session.cfg.MaxToolCalls {
		r.toolsDisabled = true
	}
	return true
}

// executeTool returns a bounded observation for the model. The error is
// reserved for context cancellation; domain and argument failures become
// observations so the model can correct itself within the session budget.
func (r *runner) executeTool(call ai.ToolCall) ([]byte, error) {
	if !r.session.toolNames[call.Function.Name] {
		return errorObservation(call.Function.Name, "unknown_tool", "tool is not available"), nil
	}
	if !r.takeTool() {
		return errorObservation(call.Function.Name, "tool_budget_exhausted", "local tool budget is exhausted"), nil
	}
	switch call.Function.Name {
	case "list_changes":
		return r.listChanges(call.Function.Arguments)
	case "check_obligations":
		return r.checkTool(call.Function.Arguments)
	case "get_evidence":
		return r.getEvidence(call.Function.Arguments)
	case "verify_patch":
		return r.verifyPatch(call.Function.Arguments)
	default:
		return errorObservation(call.Function.Name, "unknown_tool", "tool is not available"), nil
	}
}

func (r *runner) listChanges(args string) ([]byte, error) {
	var in struct {
		IDs    []string `json:"ids"`
		Cursor string   `json:"cursor"`
		Limit  int      `json:"limit"`
	}
	if err := decodeToolArgs(args, &in); err != nil {
		return errorObservation("list_changes", "invalid_arguments", err.Error()), nil
	}
	if len(in.IDs) > 16 || len(in.Cursor) > 64 || in.Limit < 0 || in.Limit > 20 {
		return errorObservation("list_changes", "invalid_arguments", "ids, cursor or limit is outside the allowed range"), nil
	}
	seen := make(map[string]bool, len(in.IDs))
	for _, id := range in.IDs {
		if !tokenPattern.MatchString(id) || seen[id] {
			return errorObservation("list_changes", "invalid_arguments", "change IDs must be valid and unique"), nil
		}
		seen[id] = true
	}
	limit := in.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < len(in.IDs) {
		limit = len(in.IDs)
	}
	page, err := r.session.cfg.Engine.ListChanges(r.ctx, ListChangesRequest{IDs: append([]string(nil), in.IDs...), Cursor: in.Cursor, Limit: limit})
	if err != nil {
		if r.ctx.Err() != nil {
			return nil, r.ctx.Err()
		}
		return errorObservation("list_changes", "tool_failed", "engine could not list changes"), nil
	}
	if err := validateChangesPage(page, limit); err != nil {
		return errorObservation("list_changes", "invalid_tool_result", err.Error()), nil
	}
	if len(in.IDs) > 0 {
		for _, item := range page.Items {
			if !seen[item.ID] {
				return errorObservation("list_changes", "invalid_tool_result", "engine returned a change outside the requested IDs"), nil
			}
		}
	}
	if err := r.registerEvidence(page.EvidenceIDs); err != nil {
		return errorObservation("list_changes", "invalid_tool_result", err.Error()), nil
	}
	return observe("list_changes", page, "Typed change summaries and source refs. Evidence IDs are verifier-owned."), nil
}

func (r *runner) checkTool(args string) ([]byte, error) {
	var in struct {
		Obligations []intent.Obligation `json:"obligations"`
	}
	if err := decodeToolArgs(args, &in); err != nil {
		return errorObservation("check_obligations", "invalid_arguments", err.Error()), nil
	}
	set, digest, err := validateObligations(in.Obligations)
	if err != nil {
		return errorObservation("check_obligations", "invalid_obligations", err.Error()), nil
	}
	if !r.matchesOperatorPolicy(digest) {
		return errorObservation("check_obligations", "operator_policy_mismatch", "operator-accepted conditions are frozen"), nil
	}
	if r.accepted != nil && r.accepted.digest != digest {
		return errorObservation("check_obligations", "obligations_frozen", "accepted obligations are frozen for this session"), nil
	}
	if r.accepted == nil {
		r.accept(set, digest)
	}
	if r.accepted.check != nil {
		return r.checkObservation(), nil
	}
	return r.runCheck()
}

func (r *runner) checkAccepted() ([]byte, error) {
	if !r.takeTool() {
		return errorObservation("check_obligations", "tool_budget_exhausted", "local tool budget is exhausted"), nil
	}
	if r.accepted == nil {
		return errorObservation("check_obligations", "obligations_missing", "no accepted obligation set"), nil
	}
	if r.accepted.check != nil {
		return r.checkObservation(), nil
	}
	return r.runCheck()
}

func (r *runner) runCheck() ([]byte, error) {
	check, err := r.session.cfg.Engine.CheckObligations(r.ctx, r.accepted.set)
	if err != nil {
		if r.ctx.Err() != nil {
			return nil, r.ctx.Err()
		}
		return errorObservation("check_obligations", "tool_failed", "engine could not check obligations"), nil
	}
	results, err := r.validateObligationResults(r.accepted.set, check.Results)
	if err != nil {
		return errorObservation("check_obligations", "invalid_tool_result", err.Error()), nil
	}
	if check.ObligationsDigest != r.accepted.digest {
		return errorObservation("check_obligations", "invalid_tool_result", "engine digest does not match the accepted obligation set"), nil
	}
	if err := r.registerResults(results); err != nil {
		return errorObservation("check_obligations", "invalid_tool_result", err.Error()), nil
	}
	checked := ObligationCheck{ObligationsDigest: check.ObligationsDigest, Results: results}
	r.accepted.check = &checked
	r.checks = append([]ObligationResult(nil), results...)
	return r.checkObservation(), nil
}

func (r *runner) checkObservation() []byte {
	return observe("check_obligations", r.accepted.check, "Statuses are verifier-owned. Call get_evidence for witness detail before proposing a repair.")
}

func (r *runner) getEvidence(args string) ([]byte, error) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if err := decodeToolArgs(args, &in); err != nil {
		return errorObservation("get_evidence", "invalid_arguments", err.Error()), nil
	}
	if len(in.IDs) == 0 || len(in.IDs) > 8 {
		return errorObservation("get_evidence", "invalid_arguments", "request 1 to 8 evidence IDs"), nil
	}
	seen := make(map[string]bool, len(in.IDs))
	for _, id := range in.IDs {
		if !evidenceID(id) || !r.known[id] {
			return errorObservation("get_evidence", "unknown_evidence", "evidence ID was not returned by an earlier tool"), nil
		}
		if seen[id] {
			return errorObservation("get_evidence", "invalid_arguments", "evidence IDs must be unique"), nil
		}
		seen[id] = true
	}
	batch, err := r.session.cfg.Engine.GetEvidence(r.ctx, EvidenceRequest{IDs: append([]string(nil), in.IDs...)})
	if err != nil {
		if r.ctx.Err() != nil {
			return nil, r.ctx.Err()
		}
		return errorObservation("get_evidence", "tool_failed", "engine could not read evidence"), nil
	}
	if len(batch.Records) > len(in.IDs) {
		return errorObservation("get_evidence", "invalid_tool_result", "engine returned more records than requested"), nil
	}
	seenRecord := make(map[string]bool, len(batch.Records))
	for _, rec := range batch.Records {
		if !seen[rec.ID] || seenRecord[rec.ID] {
			return errorObservation("get_evidence", "invalid_tool_result", "engine returned an unexpected evidence record"), nil
		}
		seenRecord[rec.ID] = true
		if rec.Kind == "" || len(rec.Kind) > 64 || len(rec.Summary) > maxEvidenceSummary || hasControl(rec.Summary) || looksSensitive(rec.Summary) {
			return errorObservation("get_evidence", "invalid_tool_result", "evidence record is outside the display limits"), nil
		}
	}
	return observe("get_evidence", batch, "Bounded verifier-owned detail. Do not cite IDs that are not present here."), nil
}

var editOps = map[string]bool{"add_before": true, "append": true, "remove": true, "replace": true}

func (r *runner) verifyPatch(args string) ([]byte, error) {
	var in struct {
		Edits     []Edit `json:"edits"`
		Rationale string `json:"rationale"`
	}
	if err := decodeToolArgs(args, &in); err != nil {
		return errorObservation("verify_patch", "invalid_arguments", err.Error()), nil
	}
	if r.accepted == nil || r.accepted.check == nil {
		return errorObservation("verify_patch", "obligations_not_checked", "check the accepted obligations before proposing a repair"), nil
	}
	if r.in.BaseBeforeSHA256 == "" || r.in.BaseAfterSHA256 == "" {
		return errorObservation("verify_patch", "repair_not_available", "repair pins are not configured for this session"), nil
	}
	if !r.hasViolation() {
		return errorObservation("verify_patch", "no_violation", "no VIOLATED obligation requires a repair"), nil
	}
	if r.repair != nil && r.repair.Status == PatchVerified {
		return errorObservation("verify_patch", "already_verified", "a repair candidate has already been verified"), nil
	}
	if r.proposals >= r.session.cfg.MaxRepairProposals {
		return errorObservation("verify_patch", "repair_budget_exhausted", "repair proposal budget is exhausted"), nil
	}
	if err := r.validatePatchArgs(in.Edits, in.Rationale); err != nil {
		return errorObservation("verify_patch", "invalid_arguments", err.Error()), nil
	}
	r.proposals++
	req := PatchCheckRequest{
		Edits:             in.Edits,
		Rationale:         strings.TrimSpace(in.Rationale),
		BaseBeforeSHA256:  r.in.BaseBeforeSHA256,
		BaseAfterSHA256:   r.in.BaseAfterSHA256,
		ObligationsDigest: r.accepted.digest,
		Scope:             intent.Scope,
	}
	result, err := r.session.cfg.Engine.VerifyPatch(r.ctx, req)
	if err != nil {
		if r.ctx.Err() != nil {
			return nil, r.ctx.Err()
		}
		r.recordFailedRepair(req, "engine_unavailable")
		return errorObservation("verify_patch", "tool_failed", "engine could not verify the repair candidate"), nil
	}
	normalized, err := r.validatePatchResult(result)
	if err != nil {
		r.recordFailedRepair(req, "invalid_tool_result")
		return errorObservation("verify_patch", "invalid_tool_result", err.Error()), nil
	}
	r.repair = &Repair{
		Attempts:          r.proposals,
		Status:            normalized.Status,
		Edits:             append([]Edit(nil), in.Edits...),
		Rationale:         req.Rationale,
		ObligationsDigest: normalized.ObligationsDigest,
		Obligations:       append([]ObligationResult(nil), normalized.ObligationResults...),
		CollateralStatus:  normalized.CollateralStatus,
		EvidenceIDs:       append([]string(nil), normalized.EvidenceIDs...),
		Reason:            normalized.Reason,
	}
	note := "The verifier, not the model, owns this status. VERIFIED requires every accepted obligation to HOLD and collateral to be verified."
	return observe("verify_patch", normalized, note), nil
}

func (r *runner) recordFailedRepair(req PatchCheckRequest, reason string) {
	r.repair = &Repair{
		Attempts:          r.proposals,
		Status:            PatchUnverified,
		Edits:             append([]Edit(nil), req.Edits...),
		Rationale:         req.Rationale,
		ObligationsDigest: req.ObligationsDigest,
		Reason:            reason,
	}
}

func (r *runner) validatePatchArgs(edits []Edit, rationale string) error {
	if len(edits) == 0 || len(edits) > 5 {
		return jsonio.Invalid("expected 1 to 5 edits")
	}
	if len([]rune(rationale)) > 500 || hasControl(rationale) {
		return jsonio.Invalid("rationale is outside the allowed limits")
	}
	refs := make(map[string]bool)
	for _, e := range edits {
		if !editOps[e.Op] {
			return jsonio.Invalid("unsupported edit operation")
		}
		needsRef := e.Op != "append"
		if needsRef != (e.Ref != nil) {
			return jsonio.Invalid("this edit operation has a ref mismatch")
		}
		needsContent := e.Op != "remove"
		if needsContent != (e.Content != "") {
			return jsonio.Invalid("this edit operation has a content mismatch")
		}
		if e.Ref != nil {
			if e.Ref.BaseAfterSHA256 != r.in.BaseAfterSHA256 {
				return jsonio.Invalid("edit ref is pinned to a different base-after digest")
			}
			if e.Ref.Ordinal < 0 || e.Ref.Line < 1 {
				return jsonio.Invalid("edit ref has invalid ordinal or line")
			}
			key := e.Ref.BaseAfterSHA256 + ":" + strconv.Itoa(e.Ref.Ordinal) + ":" + strconv.Itoa(e.Ref.Line)
			if refs[key] {
				return jsonio.Invalid("duplicate or conflicting edit refs")
			}
			refs[key] = true
		}
		if e.Content != "" {
			if len(e.Content) > 1024 || hasControl(e.Content) || strings.TrimSpace(e.Content) != e.Content ||
				looksSensitive(e.Content) || !canonicalRule(e.Content) {
				return jsonio.Invalid("edit content must be one supported ACL rule line")
			}
		}
	}
	return nil
}

func (r *runner) validatePatchResult(result PatchCheckResult) (PatchCheckResult, error) {
	switch result.Status {
	case PatchVerified, PatchRejected, PatchUnverified:
	default:
		return PatchCheckResult{}, jsonio.Invalid("engine returned an unsupported patch status")
	}
	switch result.CollateralStatus {
	case CollateralVerified, CollateralFailed, CollateralUnchecked:
	default:
		return PatchCheckResult{}, jsonio.Invalid("engine returned an unsupported collateral status")
	}
	if result.ObligationsDigest != r.accepted.digest {
		return PatchCheckResult{}, jsonio.Invalid("engine patch digest does not match accepted obligations")
	}
	results, err := r.validateObligationResults(r.accepted.set, result.ObligationResults)
	if err != nil {
		return PatchCheckResult{}, err
	}
	if len(result.Reason) > 512 || hasControl(result.Reason) || looksSensitive(result.Reason) {
		return PatchCheckResult{}, jsonio.Invalid("engine patch reason is outside display limits")
	}
	if len(result.EvidenceIDs) > 32 {
		return PatchCheckResult{}, jsonio.Invalid("engine returned too many patch evidence IDs")
	}
	if err := r.registerResults(results); err != nil {
		return PatchCheckResult{}, err
	}
	if err := r.registerEvidence(result.EvidenceIDs); err != nil {
		return PatchCheckResult{}, err
	}
	result.ObligationResults = results
	if result.Status == PatchVerified {
		allHold := true
		for _, c := range results {
			if c.Status != ObligationHolds {
				allHold = false
				break
			}
		}
		if !allHold || result.CollateralStatus != CollateralVerified || len(result.EvidenceIDs) == 0 {
			result.Status = PatchUnverified
			result.Reason = "engine verification result is internally inconsistent"
		}
	}
	return result, nil
}

func decodeToolArgs(args string, dst any) error {
	return jsonio.Decode([]byte(args), ai.MaxToolArgumentBytes, dst)
}

// Package engineadapter connects the AI tool ports to the real deterministic
// engine. Policy and input ownership stay here, outside provider-controlled data.
package engineadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"plenilune/internal/agent"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/patch"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
	"plenilune/internal/workflow"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const maxRecords = 512

// Engine is one immutable before/after ticket with an optional frozen policy.
// Records and snapshots are bounded and protected for concurrent tool callers.
type Engine struct {
	before, after         []byte
	bACL, aACL            ciscoacl.ACL
	beforeHash, afterHash string
	policy                *repair.Policy
	opts                  verify.Options
	changes               []agent.Change
	initialEvidence       []string
	mu                    sync.Mutex
	records               map[string]agent.EvidenceRecord
	checks                map[string]agent.ObligationCheck
	comparison            evidence.Comparison
	repairs               []repair.Result
}

var _ agent.Engine = (*Engine)(nil)

func New(ctx context.Context, before, after []byte, policy *repair.Policy, opts verify.Options) (*Engine, error) {
	if ctx == nil {
		return nil, errors.New("engine context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(before) > ciscoacl.MaxInputBytes || len(after) > ciscoacl.MaxInputBytes {
		return nil, errors.New("engine input exceeds ACL byte limit")
	}
	e := &Engine{before: slices.Clone(before), after: slices.Clone(after), opts: opts, records: map[string]agent.EvidenceRecord{}, checks: map[string]agent.ObligationCheck{}, repairs: []repair.Result{}}
	var conditions *intent.Set
	if policy != nil {
		n, err := repair.NormalizePolicy(*policy)
		if err != nil {
			return nil, err
		}
		owned, err := clone(n)
		if err != nil {
			return nil, err
		}
		e.policy = &owned
		conditions = &owned.Conditions
	}
	b, err := ciscoacl.Parse(e.before)
	if err != nil {
		return nil, err
	}
	a, err := ciscoacl.Parse(e.after)
	if err != nil {
		return nil, err
	}
	e.bACL, e.aACL = b, a
	e.beforeHash, e.afterHash = evidence.ConfigDigest(e.before), evidence.ConfigDigest(e.after)
	c, err := workflow.Compare(ctx, e.before, e.after, conditions, opts, evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"})
	if err != nil {
		return nil, err
	}
	e.comparison = c
	records, ids := comparisonRecords(c)
	if conditions != nil {
		_, digest, err := intent.Digest(*conditions)
		if err != nil {
			return nil, err
		}
		check, obRecords := obligationRecords(c.Report.Obligations, digest, c.SHA256)
		e.checks[digest] = check
		records = append(records, obRecords...)
	}
	if err := e.saveRecords(records); err != nil {
		return nil, err
	}
	e.initialEvidence = ids
	e.changes, err = sourceChanges(b, a, e.beforeHash, e.afterHash, c.Report.Verdict)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) ListChanges(ctx context.Context, req agent.ListChangesRequest) (agent.ChangesPage, error) {
	if err := ctx.Err(); err != nil {
		return agent.ChangesPage{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 20 || len(req.IDs) > 16 || limit < len(req.IDs) || len(req.Cursor) > 64 {
		return agent.ChangesPage{}, errors.New("invalid change page bounds")
	}
	items := e.changes
	if len(req.IDs) > 0 {
		if req.Cursor != "" {
			return agent.ChangesPage{}, errors.New("IDs and cursor cannot be combined")
		}
		wanted := map[string]bool{}
		for _, id := range req.IDs {
			if wanted[id] {
				return agent.ChangesPage{}, errors.New("duplicate change ID")
			}
			wanted[id] = true
		}
		items = []agent.Change{}
		for _, item := range e.changes {
			if wanted[item.ID] {
				items = append(items, item)
				delete(wanted, item.ID)
			}
		}
		if len(wanted) > 0 {
			return agent.ChangesPage{}, errors.New("unknown change ID")
		}
	}
	start := 0
	prefix := shortID("cur", e.beforeHash+e.afterHash)[:36] + ":"
	if req.Cursor != "" {
		if !strings.HasPrefix(req.Cursor, prefix) {
			return agent.ChangesPage{}, errors.New("cursor belongs to another ticket")
		}
		suffix := strings.TrimPrefix(req.Cursor, prefix)
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 0 || n > len(items) || strconv.Itoa(n) != suffix {
			return agent.ChangesPage{}, errors.New("invalid cursor")
		}
		start = n
	}
	end := min(start+limit, len(items))
	page := agent.ChangesPage{Items: []agent.Change{}, EvidenceIDs: slices.Clone(e.initialEvidence), Truncated: end < len(items)}
	for _, item := range items[start:end] {
		copy := item
		if item.Ordinal != nil {
			n := *item.Ordinal
			copy.Ordinal = &n
		}
		if item.Line != nil {
			n := *item.Line
			copy.Line = &n
		}
		page.Items = append(page.Items, copy)
	}
	if page.Truncated {
		page.NextCursor = prefix + strconv.Itoa(end)
	}
	return page, nil
}

func (e *Engine) CheckObligations(ctx context.Context, set intent.Set) (agent.ObligationCheck, error) {
	n, digest, err := intent.Digest(set)
	if err != nil {
		return agent.ObligationCheck{}, err
	}
	c, err := workflow.Compare(ctx, e.before, e.after, &n, e.opts, evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"})
	if err != nil {
		return agent.ObligationCheck{}, err
	}
	check, records := obligationRecords(c.Report.Obligations, digest, c.SHA256)
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.saveRecords(records); err != nil {
		return agent.ObligationCheck{}, err
	}
	e.checks[digest] = check
	e.comparison = c
	return clone(check)
}

func (e *Engine) GetEvidence(ctx context.Context, req agent.EvidenceRequest) (agent.EvidenceBatch, error) {
	if err := ctx.Err(); err != nil {
		return agent.EvidenceBatch{}, err
	}
	if len(req.IDs) < 1 || len(req.IDs) > 8 {
		return agent.EvidenceBatch{}, errors.New("request 1 to 8 evidence records")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := agent.EvidenceBatch{Records: []agent.EvidenceRecord{}}
	seen := map[string]bool{}
	for _, id := range req.IDs {
		if seen[id] {
			return agent.EvidenceBatch{}, errors.New("duplicate evidence ID")
		}
		seen[id] = true
		record, ok := e.records[id]
		if !ok {
			return agent.EvidenceBatch{}, errors.New("unknown evidence ID")
		}
		out.Records = append(out.Records, record)
	}
	return out, nil
}

func (e *Engine) VerifyPatch(ctx context.Context, req agent.PatchCheckRequest) (agent.PatchCheckResult, error) {
	if err := ctx.Err(); err != nil {
		return agent.PatchCheckResult{}, err
	}
	e.mu.Lock()
	baseCheck, ok := e.checks[req.ObligationsDigest]
	e.mu.Unlock()
	if !ok {
		return agent.PatchCheckResult{}, errors.New("conditions were not checked for this ticket")
	}
	baseCheck, err := clone(baseCheck)
	if err != nil {
		return agent.PatchCheckResult{}, err
	}
	unverified := func(status, reason string) (agent.PatchCheckResult, error) {
		return agent.PatchCheckResult{Status: status, ObligationsDigest: req.ObligationsDigest, ObligationResults: baseCheck.Results, CollateralStatus: agent.CollateralUnchecked, EvidenceIDs: []string{}, Reason: reason}, nil
	}
	if req.BaseBeforeSHA256 != e.beforeHash || req.BaseAfterSHA256 != e.afterHash || req.Scope != intent.Scope {
		return unverified(agent.PatchRejected, "request_pins_mismatch")
	}
	if e.policy == nil || !e.policy.OperatorAccepted {
		return unverified(agent.PatchUnverified, "operator_policy_not_accepted")
	}
	_, frozenDigest, err := intent.Digest(e.policy.Conditions)
	if err != nil {
		return agent.PatchCheckResult{}, err
	}
	if frozenDigest != req.ObligationsDigest {
		return unverified(agent.PatchRejected, "frozen_obligations_mismatch")
	}
	if len(req.Edits) < 1 || len(req.Edits) > patch.MaxEdits {
		return unverified(agent.PatchRejected, "invalid_edit_count")
	}
	request := patch.Request{Schema: patch.Schema, BaseBeforeSHA256: e.beforeHash, BaseAfterSHA256: e.afterHash, ObligationsDigest: req.ObligationsDigest, Scope: intent.Scope, Edits: make([]patch.Edit, len(req.Edits))}
	for i, edit := range req.Edits {
		request.Edits[i] = patch.Edit{Op: edit.Op, Content: edit.Content}
		if edit.Ref != nil {
			request.Edits[i].Ref = &patch.RuleRef{BaseAfterSHA256: edit.Ref.BaseAfterSHA256, Ordinal: edit.Ref.Ordinal, Line: edit.Ref.Line}
		}
	}
	result, err := repair.Verify(ctx, e.before, e.after, e.policy, request, e.opts)
	if err != nil {
		return agent.PatchCheckResult{}, err
	}
	mappedCollateral, err := agentCollateral(result.Collateral.Status)
	if err != nil {
		return agent.PatchCheckResult{}, err
	}
	check := baseCheck
	records := []agent.EvidenceRecord{}
	if result.Comparison != nil {
		check, records = obligationRecords(result.Comparison.Report.Obligations, req.ObligationsDigest, result.Comparison.SHA256)
	}
	receipt := agent.EvidenceRecord{ID: shortID("ev", "repair:"+result.SHA256), Kind: "repair_receipt", Summary: fmt.Sprintf("Repair receipt %s; status %s; collateral %s; candidate digest %s.", result.SHA256, result.Status, result.Collateral.Status, result.CandidateSHA256)}
	records = append(records, receipt)
	ids := []string{receipt.ID}
	if result.Collateral.Witness != nil {
		record := witnessRecord("collateral_witness", result.SHA256, *result.Collateral.Witness)
		records = append(records, record)
		ids = append(ids, record.ID)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.repairs) >= 8 {
		return agent.PatchCheckResult{}, errors.New("repair record capacity exceeded")
	}
	if err := e.saveRecords(records); err != nil {
		return agent.PatchCheckResult{}, err
	}
	owned, err := clone(result)
	if err != nil {
		return agent.PatchCheckResult{}, err
	}
	e.repairs = append(e.repairs, owned)
	return agent.PatchCheckResult{Status: result.Status, ObligationsDigest: req.ObligationsDigest, ObligationResults: check.Results, CollateralStatus: mappedCollateral, EvidenceIDs: ids, Reason: result.Reason}, nil
}

// The AI port has no INCONCLUSIVE collateral enum. Preserve that full core
// receipt while exposing UNCHECKED so an incomplete proof cannot be promoted.
func agentCollateral(status string) (string, error) {
	switch status {
	case agent.CollateralVerified, agent.CollateralFailed, agent.CollateralUnchecked:
		return status, nil
	case "INCONCLUSIVE":
		return agent.CollateralUnchecked, nil
	default:
		return "", errors.New("unsupported collateral result")
	}
}

func (e *Engine) Comparison() (evidence.Comparison, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return clone(e.comparison)
}
func (e *Engine) Repairs() ([]repair.Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return clone(e.repairs)
}

// OperatorAccepted answers from constructor-owned state, never from a model
// field. Context sent to a provider cannot confer policy authority.
func (e *Engine) OperatorAccepted(beforeSHA, afterSHA, conditionsDigest string) bool {
	if e.policy == nil || !e.policy.OperatorAccepted || beforeSHA != e.beforeHash || afterSHA != e.afterHash {
		return false
	}
	_, digest, err := intent.Digest(e.policy.Conditions)
	return err == nil && conditionsDigest == digest
}

// Input prepares the bounded provider-facing excerpt without headers, remarks
// or sequence numbers. It never claims an excerpt is the complete input.
func (e *Engine) Input(intentText string) (agent.Input, error) {
	b, bt, err := excerpt(e.before, e.bACL)
	if err != nil {
		return agent.Input{}, err
	}
	a, at, err := excerpt(e.after, e.aACL)
	if err != nil {
		return agent.Input{}, err
	}
	if len(e.aACL.Name) > 64 {
		return agent.Input{}, errors.New("ACL name exceeds agent input limit")
	}
	changes := e.changes
	if len(changes) > 64 {
		changes = changes[:64]
	}
	owned, err := clone(changes)
	if err != nil {
		return agent.Input{}, err
	}
	in := agent.Input{Intent: intentText, ACLName: e.aACL.Name, Before: b, After: a, Changes: owned, Truncated: bt || at || len(e.changes) > 64, BaseBeforeSHA256: e.beforeHash, BaseAfterSHA256: e.afterHash}
	if e.policy != nil && e.policy.OperatorAccepted {
		_, digest, err := intent.Digest(e.policy.Conditions)
		if err != nil {
			return agent.Input{}, err
		}
		if e.OperatorAccepted(e.beforeHash, e.afterHash, digest) {
			conditions, err := clone(e.policy.Conditions)
			if err != nil {
				return agent.Input{}, err
			}
			in.AcceptedConditions = &conditions
		}
	}
	return in, nil
}

func (e *Engine) saveRecords(records []agent.EvidenceRecord) error {
	// Caller holds mu, or owns the unpublished engine during construction.
	newIDs := map[string]bool{}
	for _, record := range records {
		if len(record.ID) > 64 || len(record.Summary) > 1024 {
			return errors.New("evidence record exceeds agent limits")
		}
		if _, exists := e.records[record.ID]; !exists {
			newIDs[record.ID] = true
		}
	}
	if len(e.records)+len(newIDs) > maxRecords {
		return errors.New("evidence reference capacity exceeded")
	}
	for _, record := range records {
		e.records[record.ID] = record
	}
	return nil
}

func clone[T any](in T) (T, error) {
	var out T
	b, err := json.Marshal(in)
	if err != nil {
		return out, errors.New("cannot copy engine artifact")
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, errors.New("cannot copy engine artifact")
	}
	return out, nil
}

func shortID(prefix, key string) string {
	sum := sha256.Sum256([]byte(key))
	return prefix + "-" + hex.EncodeToString(sum[:])[:60]
}

func comparisonRecords(c evidence.Comparison) ([]agent.EvidenceRecord, []string) {
	record := agent.EvidenceRecord{ID: shortID("ev", "comparison:"+c.SHA256), Kind: "comparison_receipt", Summary: fmt.Sprintf("Exact comparison receipt %s; verdict %s; search complete %t.", c.SHA256, c.Report.Verdict, c.Report.SearchComplete)}
	records := []agent.EvidenceRecord{record}
	ids := []string{record.ID}
	for _, w := range c.Report.Witnesses {
		rec := witnessRecord("difference_witness", c.SHA256, w)
		records = append(records, rec)
		if len(ids) < 32 {
			ids = append(ids, rec.ID)
		}
	}
	return records, ids
}

func obligationRecords(results []verify.ObligationResult, digest, receipt string) (agent.ObligationCheck, []agent.EvidenceRecord) {
	check := agent.ObligationCheck{ObligationsDigest: digest, Results: []agent.ObligationResult{}}
	records := []agent.EvidenceRecord{}
	for _, result := range results {
		record := agent.EvidenceRecord{ID: shortID("ev", "obligation:"+receipt+":"+result.ID), Kind: "obligation_receipt", Summary: fmt.Sprintf("Condition %s result %s; exact comparison receipt %s.", result.ID, result.Status, receipt)}
		if result.Witness != nil {
			record = witnessRecord("obligation_witness", receipt+":"+result.ID, *result.Witness)
		}
		records = append(records, record)
		check.Results = append(check.Results, agent.ObligationResult{ID: result.ID, Status: result.Status, EvidenceIDs: []string{record.ID}, Reason: result.Reason})
	}
	return check, records
}

func witnessRecord(kind, receipt string, w verify.Witness) agent.EvidenceRecord {
	return agent.EvidenceRecord{ID: shortID("ev", kind+":"+receipt+":"+w.ID), Kind: kind, Summary: fmt.Sprintf("Witness %s; protocol %d; %s:%d to %s:%d; before %s ordinal %s line %s; after %s ordinal %s line %s; receipt %s.", w.ID, w.Packet.Protocol, w.Packet.SrcIP, w.Packet.SrcPort, w.Packet.DstIP, w.Packet.DstPort, w.Before.Action, refNumber(w.Before.RuleOrdinal), refNumber(w.Before.Line), w.After.Action, refNumber(w.After.RuleOrdinal), refNumber(w.After.Line), receipt)}
}
func refNumber(n *int) string {
	if n == nil {
		return "implicit"
	}
	return strconv.Itoa(*n)
}

func excerpt(source []byte, acl ciscoacl.ACL) ([]string, bool, error) {
	lines := strings.Split(strings.ReplaceAll(string(source), "\r\n", "\n"), "\n")
	out := []string{}
	total := 0
	for _, rule := range acl.Rules {
		f := strings.Fields(lines[rule.Line-1])
		if rule.Sequence != nil {
			f = f[1:]
		}
		line := strings.Join(f, " ")
		if len(line) > 2048 {
			return nil, false, errors.New("rule exceeds agent excerpt line limit")
		}
		if len(out) == 64 || total+len(line) > 32<<10 {
			return out, true, nil
		}
		out = append(out, line)
		total += len(line)
	}
	return out, false, nil
}

func sourceChanges(b, a ciscoacl.ACL, bh, ah, verdict string) ([]agent.Change, error) {
	fingerprint := func(r ciscoacl.Rule) (string, error) {
		r.Ordinal = 0
		r.Line = 0
		r.Sequence = nil
		data, err := json.Marshal(r)
		if err != nil {
			return "", errors.New("cannot fingerprint source rule")
		}
		return string(data), nil
	}
	before, after := map[string]int{}, map[string]int{}
	for _, r := range b.Rules {
		key, err := fingerprint(r)
		if err != nil {
			return nil, err
		}
		before[key]++
	}
	for _, r := range a.Rules {
		key, err := fingerprint(r)
		if err != nil {
			return nil, err
		}
		after[key]++
	}
	out := []agent.Change{}
	add := func(side, kind, hash string, r ciscoacl.Rule) {
		ordinal, line := r.Ordinal, r.Line
		out = append(out, agent.Change{ID: shortID("ch", side+":"+hash+":"+strconv.Itoa(ordinal)), Kind: kind, Side: side, Ordinal: &ordinal, Line: &line, Summary: fmt.Sprintf("%s source rule ordinal %d line %d action %s; base digest %s.", side, ordinal, line, r.Action, hash)})
	}
	for _, r := range b.Rules {
		key, err := fingerprint(r)
		if err != nil {
			return nil, err
		}
		if after[key] > 0 {
			after[key]--
		} else {
			add("before", "rule_removed", bh, r)
		}
	}
	for _, r := range a.Rules {
		key, err := fingerprint(r)
		if err != nil {
			return nil, err
		}
		if before[key] > 0 {
			before[key]--
		} else {
			add("after", "rule_added", ah, r)
		}
	}
	// Reordering can change first-match behavior without an added/removed rule.
	// Supply honest source-context refs, not an invented semantic diagnosis.
	if len(out) == 0 && verdict == "DIFFERENT" {
		for _, r := range a.Rules {
			add("after", "source_context", ah, r)
		}
	}
	return out, nil
}

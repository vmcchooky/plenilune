package repair

import (
	"context"
	"encoding/json"
	"errors"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/patch"
	"plenilune/internal/verify"
	"plenilune/internal/workflow"
)

type Collateral struct {
	Status  string          `json:"status"`
	Reason  string          `json:"reason,omitempty"`
	Witness *verify.Witness `json:"witness,omitempty"`
	Stats   packetset.Stats `json:"stats"`
}

type Result struct {
	Schema          string         `json:"schema"`
	Version         string         `json:"version"`
	Status          string         `json:"status"`
	Reason          string         `json:"reason,omitempty"`
	BeforeSHA256    string         `json:"before_sha256"`
	BaseAfterSHA256 string         `json:"base_after_sha256"`
	Policy          *Policy        `json:"policy"`
	PolicySHA256    string         `json:"policy_sha256,omitempty"`
	Request         patch.Request  `json:"request"`
	Options         verify.Options `json:"options"`
	Candidate       string         `json:"candidate,omitempty"`
	CandidateSHA256 string         `json:"candidate_sha256,omitempty"`
	// Sensitive marks a receipt whose candidate field carries full
	// configuration text, so stores and reports must handle it as sensitive.
	// It is a handling label, not evidence content: digest() excludes it, so
	// receipts sealed before the label existed keep their digest and stay
	// replayable.
	Sensitive         bool                 `json:"sensitive,omitempty"`
	Conditions        intent.Set           `json:"conditions"`
	ObligationsDigest string               `json:"obligations_digest,omitempty"`
	Comparison        *evidence.Comparison `json:"comparison,omitempty"`
	Collateral        Collateral           `json:"collateral"`
	SHA256            string               `json:"sha256,omitempty"`
}

func Verify(ctx context.Context, before, after []byte, policy *Policy, request patch.Request, opts verify.Options) (Result, error) {
	return verifyVersion(ctx, before, after, policy, request, opts, Version, evidence.ComparisonSchema)
}

// verifyVersion recomputes a receipt of the given repair version. The
// evidence schema names the shape the embedded candidate comparison is
// sealed in: fresh verification uses the current evidence schema, while
// Replay passes the recorded comparison's own schema so a receipt from
// an earlier evidence era recomputes byte-identically (P2-1).
func verifyVersion(ctx context.Context, before, after []byte, policy *Policy, request patch.Request, opts verify.Options, version string, evidenceSchema string) (Result, error) {
	engine, err := engineForRepair(version)
	if err != nil {
		return Result{}, err
	}
	if len(before) > ciscoacl.MaxInputBytes || len(after) > ciscoacl.MaxInputBytes {
		return Result{}, errors.New("ACL input exceeds limit")
	}
	if len(request.Edits) < 1 || len(request.Edits) > patch.MaxEdits {
		return Result{}, errors.New("patch requires one to five edits")
	}
	if len(request.Schema) > 128 || len(request.Scope) > 128 || len(request.BaseBeforeSHA256) != 64 || len(request.BaseAfterSHA256) != 64 || len(request.ObligationsDigest) != 64 {
		return Result{}, errors.New("patch metadata exceeds contract bounds")
	}
	for _, edit := range request.Edits {
		if len(edit.Op) > 32 || len(edit.Content) > patch.MaxContentBytes || edit.Ref != nil && len(edit.Ref.BaseAfterSHA256) != 64 {
			return Result{}, errors.New("patch edit exceeds contract bounds")
		}
	}
	if opts.MaxWitnesses < 1 || opts.MaxWitnesses > 64 {
		return Result{}, errors.New("invalid witness limit")
	}
	b, err := packetset.NewBudget(ctx, opts.Limits)
	if err != nil {
		return Result{}, err
	}
	b.Close()
	request = cloneRequest(request)
	r := Result{Schema: Schema, Version: version, Status: "UNVERIFIED", BeforeSHA256: evidence.ConfigDigest(before), BaseAfterSHA256: evidence.ConfigDigest(after), Request: request, Options: opts, Collateral: Collateral{Status: "UNCHECKED"}}
	finish := func(status, reason string) (Result, error) { r.Status, r.Reason = status, reason; return seal(r) }
	if policy != nil {
		n, err := NormalizePolicy(*policy)
		if err != nil {
			return Result{}, err
		}
		r.Policy = &n
		r.Conditions = n.Conditions
		r.PolicySHA256, err = PolicyDigest(n)
		if err != nil {
			return Result{}, err
		}
		_, r.ObligationsDigest, err = intent.Digest(n.Conditions)
		if err != nil {
			return Result{}, err
		}
	}
	if request.BaseBeforeSHA256 != r.BeforeSHA256 || request.BaseAfterSHA256 != r.BaseAfterSHA256 {
		return finish("REJECTED", "base_digest_mismatch")
	}
	if r.Policy != nil && request.ObligationsDigest != r.ObligationsDigest {
		return finish("REJECTED", "obligations_digest_mismatch")
	}
	candidate, err := patch.Apply(after, request)
	if err != nil {
		var invalid *patch.Error
		if errors.As(err, &invalid) {
			return finish("REJECTED", invalid.Code)
		}
		return Result{}, errors.New("candidate rendering failed")
	}
	r.Candidate, r.CandidateSHA256 = string(candidate), evidence.ConfigDigest(candidate)
	r.Sensitive = true
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if r.Policy == nil {
		return finish("UNVERIFIED", "policy_missing")
	}
	comparison, err := workflow.CompareEngineSchema(ctx, before, candidate, &r.Conditions, opts, evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"}, engine, evidenceSchema)
	if err != nil {
		return Result{}, err
	}
	r.Comparison = &comparison
	// A budget-stopped or timing-limited comparison cannot establish the
	// complete candidate behavior, even if the obligations seen so far hold.
	if !comparison.Report.SearchComplete || comparison.Report.Budget.StopReason != "" || comparison.Report.Budget.TimingLimited {
		return finish("UNVERIFIED", "comparison_incomplete")
	}
	for _, ob := range comparison.Report.Obligations {
		if ob.Status == "VIOLATED" {
			return finish("REJECTED", "obligations_violated")
		}
	}
	for _, ob := range comparison.Report.Obligations {
		if ob.Status != "HOLDS" {
			return finish("UNVERIFIED", "obligations_inconclusive")
		}
	}
	if r.Policy.AllowedChanges == nil {
		return finish("UNVERIFIED", "allowed_change_region_missing")
	}
	bACL, err := ciscoacl.Parse(before)
	if err != nil {
		return Result{}, err
	}
	cACL, err := ciscoacl.Parse(candidate)
	if err != nil {
		return Result{}, err
	}
	preservation, err := verify.PreserveOutsideEngine(ctx, bACL, cACL, r.Policy.AllowedChanges, opts, engine)
	if err != nil {
		return Result{}, err
	}
	r.Collateral = Collateral{Status: preservation.Status, Reason: preservation.Reason, Witness: preservation.Witness, Stats: preservation.Budget}
	if preservation.Status == "FAILED" {
		return finish("REJECTED", "collateral_changed")
	}
	if preservation.Status != "VERIFIED" {
		return finish("UNVERIFIED", "collateral_inconclusive")
	}
	if !r.Policy.OperatorAccepted {
		return finish("UNVERIFIED", "policy_not_accepted")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return finish("VERIFIED", "")
}

func cloneRequest(r patch.Request) patch.Request {
	edits := make([]patch.Edit, len(r.Edits))
	copy(edits, r.Edits)
	for i := range edits {
		if edits[i].Ref != nil {
			x := *edits[i].Ref
			edits[i].Ref = &x
		}
	}
	r.Edits = edits
	return r
}

func seal(r Result) (Result, error) {
	if err := validate(r); err != nil {
		return Result{}, err
	}
	h, err := digest(r)
	if err != nil {
		return Result{}, err
	}
	r.SHA256 = h
	return r, nil
}
func digest(r Result) (string, error) {
	r.SHA256 = ""
	r.Sensitive = false // handling label only; never part of the sealed projection
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	if len(b) > MaxJSONBytes {
		return "", errors.New("repair evidence exceeds limit")
	}
	return evidence.ConfigDigest(b), nil
}
func Check(r Result) error {
	if err := validate(r); err != nil {
		return err
	}
	h, err := digest(r)
	if err != nil {
		return err
	}
	if h != r.SHA256 {
		return errors.New("repair evidence digest mismatch")
	}
	return nil
}
func validate(r Result) error {
	engine, err := engineForRepair(r.Version)
	if r.Schema != Schema || err != nil {
		return errors.New("unsupported repair evidence schema/version")
	}
	if r.Status != "VERIFIED" && r.Status != "REJECTED" && r.Status != "UNVERIFIED" {
		return errors.New("invalid repair status")
	}
	d := packetset.Defaults()
	l := r.Options.Limits
	if r.Options.MaxWitnesses < 1 || r.Options.MaxWitnesses > 64 || l.MaxOperations < 1 || l.MaxOperations > d.MaxOperations || l.MaxBoxes < 1 || l.MaxBoxes > d.MaxBoxes || l.Timeout <= 0 || l.Timeout > d.Timeout {
		return errors.New("invalid repair options")
	}
	stats := r.Collateral.Stats
	if stats.Operations < 0 || stats.PeakBoxes < 0 || stats.Operations > r.Options.Limits.MaxOperations || stats.PeakBoxes > r.Options.Limits.MaxBoxes {
		return errors.New("invalid collateral budget statistics")
	}
	switch stats.StopReason {
	case "", "operations", "boxes":
		if stats.TimingLimited {
			return errors.New("inconsistent collateral timing")
		}
	case "wall_clock", "context_deadline":
		if !stats.TimingLimited {
			return errors.New("missing collateral timing")
		}
	default:
		return errors.New("invalid collateral stop reason")
	}
	switch r.Collateral.Status {
	case "VERIFIED":
		if r.Collateral.Witness != nil || stats.StopReason != "" || stats.TimingLimited {
			return errors.New("incomplete collateral proof")
		}
	case "FAILED":
		if r.Collateral.Witness == nil {
			return errors.New("collateral failure requires witness")
		}
	case "UNCHECKED":
		if r.Collateral.Witness != nil || stats != (packetset.Stats{}) {
			return errors.New("unchecked collateral contains proof data")
		}
	case "INCONCLUSIVE":
		if r.Collateral.Witness != nil || stats.StopReason == "" {
			return errors.New("inconclusive collateral requires stop reason")
		}
	default:
		return errors.New("invalid collateral status")
	}
	if r.Candidate != "" && evidence.ConfigDigest([]byte(r.Candidate)) != r.CandidateSHA256 {
		return errors.New("candidate bytes do not match digest")
	}
	if r.Policy != nil {
		h, err := PolicyDigest(*r.Policy)
		if err != nil {
			return err
		}
		if h != r.PolicySHA256 {
			return errors.New("policy digest mismatch")
		}
		_, conditionsHash, err := intent.Digest(r.Policy.Conditions)
		if err != nil {
			return err
		}
		_, resultHash, err := intent.Digest(r.Conditions)
		if err != nil || conditionsHash != resultHash || conditionsHash != r.ObligationsDigest {
			return errors.New("repair definitions do not match frozen policy")
		}
	}
	if r.Comparison != nil {
		if err := evidence.CheckComparison(*r.Comparison); err != nil {
			return err
		}
		if r.Comparison.EngineVersion != engine || !r.Comparison.Report.SearchComplete || r.Comparison.Report.Budget.StopReason != "" || r.Comparison.Report.Budget.TimingLimited {
			return errors.New("candidate comparison is incomplete or from another engine")
		}
		if r.Comparison.Before.SHA256 != r.BeforeSHA256 || r.Comparison.After.SHA256 != r.CandidateSHA256 || r.Comparison.Conditions == nil {
			return errors.New("candidate comparison input binding mismatch")
		}
		_, h, err := intent.Digest(*r.Comparison.Conditions)
		if err != nil || h != r.ObligationsDigest {
			return errors.New("candidate comparison policy mismatch")
		}
	}
	if r.Status == "VERIFIED" {
		if r.Policy == nil || !r.Policy.OperatorAccepted || r.Policy.AllowedChanges == nil || r.Candidate == "" || r.Comparison == nil || r.Collateral.Status != "VERIFIED" || r.Collateral.Witness != nil {
			return errors.New("verified repair lacks accepted complete proof")
		}
		if r.Request.BaseBeforeSHA256 != r.BeforeSHA256 || r.Request.BaseAfterSHA256 != r.BaseAfterSHA256 || r.Request.ObligationsDigest != r.ObligationsDigest {
			return errors.New("verified repair pin mismatch")
		}
		if len(r.Comparison.Report.Obligations) != len(r.Conditions.Obligations) {
			return errors.New("incomplete repair obligations")
		}
		for _, ob := range r.Comparison.Report.Obligations {
			if ob.Status != "HOLDS" {
				return errors.New("verified repair has unmet obligation")
			}
		}
	}
	return nil
}

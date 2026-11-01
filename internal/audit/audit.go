// Package audit owns the sealed rule-audit record (P1-4): the exact
// single-ACL diagnosis computed by verify.AuditACL, bound to the exact
// configuration bytes it was computed from and sealed with the
// project's usual SHA-256 digest. The seal is a content check, not a
// signature or proof of authorship — the same wording as every other
// Plenilune artifact.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/verify"
	"plenilune/internal/workflow"
)

// Schema and Version identify the audit record and its scheme. The
// record embeds a verify.AuditReport produced by the current ACL
// engine; the engine version is pinned in the record and replay
// dispatches on it like every other engine-bound artifact.
const Schema = "plenilune.audit/v1"
const Version = "acl-audit/v1"

// MaxAuditBytes bounds the sealed audit record on decode. A record
// carries one small witness per finding or relation and no
// configuration text, so the bound is generous but finite.
const MaxAuditBytes = 64 << 20

// Record is the sealed audit of one ACL-only configuration. Report is
// the engine's audit report verbatim; the remaining fields bind it to
// the exact input bytes and the ACL identity. A record with
// Report.Complete false is an honest partial diagnosis: it names the
// stage and rule where the budget stopped and claims nothing about
// the unexamined rules.
type Record struct {
	Schema       string             `json:"schema"`
	Version      string             `json:"version"`
	Engine       string             `json:"engine"`
	Scope        string             `json:"scope"`
	ConfigSHA256 string             `json:"config_sha256"`
	Name         string             `json:"name"`
	Kind         string             `json:"kind"`
	Report       verify.AuditReport `json:"report"`
	SHA256       string             `json:"sha256,omitempty"`
}

// Audit parses one ACL-only configuration and returns its sealed
// audit record. A budget-stopped audit is a record, not an error: the
// report inside carries Complete=false with the stop stage and rule.
func Audit(ctx context.Context, config []byte, opts verify.Options) (Record, error) {
	if len(config) > ciscoacl.MaxInputBytes {
		return Record{}, errors.New("ACL input exceeds limit")
	}
	acl, err := ciscoacl.Parse(config)
	if err != nil {
		return Record{}, err
	}
	rep, err := verify.AuditACL(ctx, acl, opts)
	if err != nil {
		return Record{}, err
	}
	rec := Record{Schema: Schema, Version: Version, Engine: verify.EngineVersion, Scope: intent.Scope, ConfigSHA256: evidence.ConfigDigest(config), Name: acl.Name, Kind: acl.Kind, Report: rep}
	return seal(rec)
}

func seal(r Record) (Record, error) {
	if err := validate(r); err != nil {
		return Record{}, err
	}
	h, err := digest(r)
	if err != nil {
		return Record{}, err
	}
	r.SHA256 = h
	return r, nil
}

func digest(r Record) (string, error) {
	r.SHA256 = ""
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	if len(b) > MaxAuditBytes {
		return "", errors.New("audit record exceeds limit")
	}
	return evidence.ConfigDigest(b), nil
}

// Check verifies the record's seal and internal consistency: schema,
// version, engine and scope pins, the digest, and the structural
// invariants of the embedded report (finding order and shape,
// relation order, and agreement between the completeness flags, the
// stop stage and the budget stop reason). Check does not re-run the
// audit; Replay does.
func Check(r Record) error {
	if err := validate(r); err != nil {
		return err
	}
	h, err := digest(r)
	if err != nil {
		return err
	}
	if h != r.SHA256 {
		return errors.New("audit digest mismatch")
	}
	return nil
}

func validate(r Record) error {
	if r.Schema != Schema || r.Version != Version {
		return errors.New("unsupported audit schema/version")
	}
	if r.Engine != verify.EngineVersion || !verify.SupportedEngine(r.Engine) {
		return errors.New("unsupported audit engine version")
	}
	if r.Scope != intent.Scope {
		return errors.New("unsupported audit scope")
	}
	rep := r.Report
	if rep.Rules < 1 || rep.Rules > ciscoacl.MaxRules {
		return errors.New("audit rule count out of range")
	}
	if rep.Complete {
		if rep.Reason != "" || rep.Stage != "" || rep.StageRule != nil || rep.StageRule2 != nil || !rep.RelationsComplete {
			return errors.New("complete audit carries stop residue")
		}
	} else {
		if rep.Reason == "" || rep.Reason != rep.Budget.StopReason {
			return errors.New("incomplete audit reason does not match its budget stop")
		}
		switch rep.Stage {
		case verify.AuditStageEffective, verify.AuditStageRedundancy:
			if rep.StageRule == nil || *rep.StageRule < 0 || *rep.StageRule >= rep.Rules {
				return errors.New("incomplete audit stage cursor out of range")
			}
			if len(rep.Relations) != 0 || rep.RelationsComplete {
				return errors.New("audit stopped before relations but carries them")
			}
		case verify.AuditStageRelations:
			// Findings are complete at this stage; the relations
			// are partial by definition of the stop.
			if rep.RelationsComplete {
				return errors.New("audit stopped in relations but claims them complete")
			}
		default:
			return errors.New("incomplete audit carries no valid stop stage")
		}
	}
	rank := map[string]int{verify.AuditUnreachable: 0, verify.AuditRedundant: 1}
	seenFinding := map[int]bool{}
	for i, f := range rep.Findings {
		rk, ok := rank[f.Kind]
		if !ok {
			return errors.New("audit finding kind unsupported")
		}
		if f.Rule < 0 || f.Rule >= rep.Rules || seenFinding[f.Rule] {
			return errors.New("audit finding rule out of range or duplicated")
		}
		seenFinding[f.Rule] = true
		if i > 0 {
			prev := rep.Findings[i-1]
			if rank[prev.Kind] > rk || rank[prev.Kind] == rk && prev.Rule > f.Rule {
				return errors.New("audit findings out of order")
			}
		}
		switch f.Kind {
		case verify.AuditUnreachable:
			if f.EffectiveBoxes != 0 {
				return errors.New("unreachable finding with nonempty effective region")
			}
		case verify.AuditRedundant:
			if f.EffectiveBoxes < 1 {
				return errors.New("redundant finding with empty effective region")
			}
			for k, c := range f.CoveredBy {
				if c <= f.Rule || c >= rep.Rules || k > 0 && c <= f.CoveredBy[k-1] {
					return errors.New("redundant covered-by list invalid")
				}
			}
			if f.ImplicitDeny && f.Action != "deny" {
				return errors.New("implicit-deny fallthrough on a non-deny rule")
			}
		}
	}
	for i, rel := range rep.Relations {
		if rel.Kind != verify.AuditGeneralization && rel.Kind != verify.AuditCorrelation {
			return errors.New("audit relation kind unsupported")
		}
		if rel.Earlier < 0 || rel.Earlier >= rel.Later || rel.Later >= rep.Rules {
			return errors.New("audit relation pair out of range")
		}
		if rel.EarlierAction == rel.LaterAction {
			return errors.New("audit relation between same-action rules")
		}
		if i > 0 {
			prev := rep.Relations[i-1]
			if prev.Earlier > rel.Earlier || prev.Earlier == rel.Earlier && prev.Later >= rel.Later {
				return errors.New("audit relations out of order")
			}
		}
	}
	return nil
}

// Replay re-runs the audit from the original configuration bytes and
// requires the same sealed record, mirroring the other replay paths:
// byte-level agreement first, and a TIMING_SENSITIVE disposition —
// never a pass — when only timing-limited budget accounting differs.
func Replay(ctx context.Context, config []byte, r Record) (workflow.ReplayResult, error) {
	if err := ctx.Err(); err != nil {
		return workflow.ReplayResult{}, err
	}
	if err := Check(r); err != nil {
		return workflow.ReplayResult{}, err
	}
	if len(config) > ciscoacl.MaxInputBytes {
		return workflow.ReplayResult{}, errors.New("ACL input exceeds limit")
	}
	if evidence.ConfigDigest(config) != r.ConfigSHA256 {
		return workflow.ReplayResult{}, errors.New("audit original input mismatch")
	}
	acl, err := ciscoacl.Parse(config)
	if err != nil {
		return workflow.ReplayResult{}, err
	}
	if acl.Name != r.Name || acl.Kind != r.Kind {
		return workflow.ReplayResult{}, errors.New("audit ACL identity mismatch")
	}
	fresh, err := Audit(ctx, config, r.Report.Options)
	if err != nil {
		return workflow.ReplayResult{}, err
	}
	if fresh.SHA256 == r.SHA256 {
		return workflow.ReplayResult{Status: "AUDIT_REPLAYED", SHA256: r.SHA256}, nil
	}
	// The only tolerated divergence is timing: identical findings,
	// relations and completeness with at least one side timing-limited
	// in its budget accounting.
	a, b := r.Report, fresh.Report
	timing := a.Budget.TimingLimited || b.Budget.TimingLimited
	a.Budget, b.Budget = packetset.Stats{}, packetset.Stats{}
	if timing && reflect.DeepEqual(a, b) {
		return workflow.ReplayResult{Status: "TIMING_SENSITIVE", SHA256: r.SHA256}, nil
	}
	return workflow.ReplayResult{}, errors.New("replayed audit differs")
}

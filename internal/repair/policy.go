// Package repair freezes operator policy, renders in-memory patches and checks
// candidate obligations and collateral with the exact engine.
package repair

import (
	"encoding/json"
	"errors"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/verify"
	"slices"
	"strconv"
	"strings"
)

const PolicySchema = "plenilune.policy/v1"
const Schema = "plenilune.repair/v1"

// Version is stamped on new receipts. Each repair version is bound to the ACL
// engine whose accounting its embedded comparison and collateral record.
const Version = "acl-repair/v0.2"
const LegacyVersion = "acl-repair/v0.1"

func engineForRepair(version string) (string, error) {
	switch version {
	case Version:
		return verify.EngineVersion, nil
	case LegacyVersion:
		return verify.LegacyEngineVersion, nil
	}
	return "", errors.New("unsupported repair evidence version")
}

const MaxJSONBytes = 8 << 20

type Policy struct {
	Schema           string     `json:"schema"`
	Scope            string     `json:"scope"`
	OperatorAccepted bool       `json:"operator_accepted"`
	Conditions       intent.Set `json:"conditions"`
	// nil means undeclared. A non-nil empty slice explicitly permits no changes.
	AllowedChanges []intent.Region `json:"allowed_changes"`
}

func NormalizePolicy(p Policy) (Policy, error) {
	if p.Schema != PolicySchema || p.Scope != intent.Scope {
		return Policy{}, errors.New("unsupported policy schema or scope")
	}
	n, err := intent.Normalize(p.Conditions)
	if err != nil {
		return Policy{}, err
	}
	out := Policy{Schema: PolicySchema, Scope: intent.Scope, OperatorAccepted: p.OperatorAccepted, Conditions: n}
	if len(p.AllowedChanges) > 12 {
		return Policy{}, errors.New("too many allowed change regions")
	}
	if p.AllowedChanges != nil {
		out.AllowedChanges = make([]intent.Region, 0, len(p.AllowedChanges))
		for i, r := range p.AllowedChanges {
			s, err := intent.Normalize(intent.Set{Schema: intent.Schema, Scope: intent.Scope, Obligations: []intent.Obligation{{ID: "OB-" + strconv.Itoa(i+1), Kind: "must_remain", Match: r}}})
			if err != nil {
				return Policy{}, err
			}
			out.AllowedChanges = append(out.AllowedChanges, s.Obligations[0].Match)
		}
		// Predicate structs contain no unsupported JSON values; error handling is
		// outside the comparator so it cannot hide a serialization failure.
		type keyed struct {
			key    string
			region intent.Region
		}
		keys := make([]keyed, 0, len(out.AllowedChanges))
		for _, r := range out.AllowedChanges {
			b, err := json.Marshal(r)
			if err != nil {
				return Policy{}, err
			}
			keys = append(keys, keyed{string(b), r})
		}
		slices.SortFunc(keys, func(a, b keyed) int { return strings.Compare(a.key, b.key) })
		out.AllowedChanges = out.AllowedChanges[:0]
		last := ""
		for _, k := range keys {
			if k.key != last {
				out.AllowedChanges = append(out.AllowedChanges, k.region)
				last = k.key
			}
		}
	}
	return out, nil
}

func PolicyDigest(p Policy) (string, error) {
	n, err := NormalizePolicy(p)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	return evidence.ConfigDigest(b), nil
}

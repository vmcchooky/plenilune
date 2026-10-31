// Package evidence binds typed trace inputs and results to a reproducible digest.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
)

const TraceSchema = "plenilune.trace/v1"
const MaxTraceBytes = 64 << 10

// Trace is point-evaluation evidence, not a whole-ACL equivalence certificate.
// Its digest detects changes against a known digest, not author authenticity.
type Trace struct {
	Schema        string            `json:"schema"`
	Scope         string            `json:"scope"`
	ParserVersion string            `json:"parser_version"`
	ConfigSHA256  string            `json:"config_sha256"`
	Packet        ciscoacl.Packet   `json:"packet"`
	Decision      ciscoacl.Decision `json:"decision"`
	SHA256        string            `json:"sha256"`
}

func ConfigDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SealTrace accepts a caller-computed decision; replay belongs to orchestration.
func SealTrace(configSHA string, packet ciscoacl.Packet, decision ciscoacl.Decision) (Trace, error) {
	t := Trace{Schema: TraceSchema, Scope: intent.Scope, ParserVersion: ciscoacl.ParserVersion, ConfigSHA256: configSHA, Packet: packet, Decision: decision}
	if err := validate(t); err != nil {
		return Trace{}, err
	}
	h, err := digest(t)
	if err != nil {
		return Trace{}, err
	}
	t.SHA256 = h
	return t, nil
}

func CheckIntegrity(t Trace) error {
	if err := validate(t); err != nil {
		return err
	}
	h, err := digest(t)
	if err != nil {
		return err
	}
	if t.SHA256 != h {
		return errors.New("trace digest mismatch")
	}
	return nil
}

func validate(t Trace) error {
	if t.Schema != TraceSchema || t.Scope != intent.Scope || t.ParserVersion != ciscoacl.ParserVersion {
		return errors.New("unsupported trace schema, scope or parser")
	}
	h, err := hex.DecodeString(t.ConfigSHA256)
	if err != nil || len(h) != sha256.Size || hex.EncodeToString(h) != t.ConfigSHA256 {
		return errors.New("invalid configuration digest")
	}
	if err := t.Packet.Validate(); err != nil {
		return errors.New("invalid trace packet")
	}
	d := t.Decision
	if d.Action != "permit" && d.Action != "deny" {
		return errors.New("invalid trace action")
	}
	if d.Implicit {
		if d.Action != "deny" || d.RuleOrdinal != nil || d.Line != nil {
			return errors.New("invalid implicit deny trace")
		}
	} else if d.RuleOrdinal == nil || d.Line == nil || *d.RuleOrdinal < 0 || *d.Line < 1 {
		return errors.New("explicit trace requires rule source")
	}
	return nil
}

func digest(t Trace) (string, error) {
	// SHA256 is omitted from this fixed-order v1 projection, not serialized empty.
	p := struct {
		Schema        string            `json:"schema"`
		Scope         string            `json:"scope"`
		ParserVersion string            `json:"parser_version"`
		ConfigSHA256  string            `json:"config_sha256"`
		Packet        ciscoacl.Packet   `json:"packet"`
		Decision      ciscoacl.Decision `json:"decision"`
	}{t.Schema, t.Scope, t.ParserVersion, t.ConfigSHA256, t.Packet, t.Decision}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return ConfigDigest(b), nil
}

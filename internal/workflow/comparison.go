// Package workflow orchestrates pure local comparison and semantic replay.
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/verify"
	"reflect"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Side    string `json:"side,omitempty"`
	Line    int    `json:"line,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func Compare(ctx context.Context, before, after []byte, conditions *intent.Set, opts verify.Options, tool evidence.Tool) (evidence.Comparison, error) {
	return CompareEngine(ctx, before, after, conditions, opts, tool, verify.EngineVersion)
}

// CompareEngine seals a comparison produced by the named engine version.
func CompareEngine(ctx context.Context, before, after []byte, conditions *intent.Set, opts verify.Options, tool evidence.Tool, engine string) (evidence.Comparison, error) {
	b, a, err := parsePair(before, after)
	if err != nil {
		return evidence.Comparison{}, err
	}
	r, err := verify.CompareEngine(ctx, b, a, conditions, opts, engine)
	if err != nil {
		return evidence.Comparison{}, err
	}
	return evidence.SealComparison(evidence.Comparison{Tool: tool, EngineVersion: engine, Before: input(before, b), After: input(after, a), Conditions: conditions, Report: r})
}

type ReplayResult struct {
	Status             string `json:"status"`
	SHA256             string `json:"sha256"`
	RecheckedWitnesses int    `json:"rechecked_witnesses"`
}

func Replay(ctx context.Context, before, after []byte, c evidence.Comparison) (ReplayResult, error) {
	if err := ctx.Err(); err != nil {
		return ReplayResult{}, err
	}
	if err := evidence.CheckComparison(c); err != nil {
		return ReplayResult{}, err
	}
	b, a, err := parsePair(before, after)
	if err != nil {
		return ReplayResult{}, err
	}
	if input(before, b) != c.Before || input(after, a) != c.After {
		return ReplayResult{}, errors.New("original comparison input bytes or identity mismatch")
	}
	n := 0
	for _, w := range c.Report.Witnesses {
		if err := recheck(b, a, w); err != nil {
			return ReplayResult{}, err
		}
		if w.Before.Action == w.After.Action {
			return ReplayResult{}, errors.New("difference witness has equal decisions")
		}
		n++
	}
	for i, r := range c.Report.Obligations {
		if r.Witness == nil {
			continue
		}
		w := *r.Witness
		if err := recheck(b, a, w); err != nil {
			return ReplayResult{}, err
		}
		ob := c.Conditions.Obligations[i]
		if !matches(ob.Match, w.Packet) {
			return ReplayResult{}, errors.New("obligation witness is outside its predicate")
		}
		valid := ob.Kind == "must_permit" && w.After.Action == "deny" || ob.Kind == "must_deny" && w.After.Action == "permit" || ob.Kind == "must_remain" && w.Before.Action != w.After.Action
		if !valid {
			return ReplayResult{}, errors.New("packet does not violate obligation")
		}
		n++
	}
	r, err := verify.CompareEngine(ctx, b, a, c.Conditions, c.Report.Options, c.EngineVersion)
	if err != nil {
		return ReplayResult{}, err
	}
	if c.Report.Budget.TimingLimited || r.Budget.TimingLimited {
		return ReplayResult{Status: "TIMING_SENSITIVE", SHA256: c.SHA256, RecheckedWitnesses: n}, nil
	}
	x, err := json.Marshal(r)
	if err != nil {
		return ReplayResult{}, err
	}
	y, err := json.Marshal(c.Report)
	if err != nil {
		return ReplayResult{}, err
	}
	if string(x) != string(y) {
		return ReplayResult{}, errors.New("replayed comparison report does not match")
	}
	return ReplayResult{Status: "COMPARISON_REPLAYED", SHA256: c.SHA256, RecheckedWitnesses: n}, nil
}

func parsePair(before, after []byte) (ciscoacl.ACL, ciscoacl.ACL, error) {
	b, err := ciscoacl.Parse(before)
	if err != nil {
		return ciscoacl.ACL{}, ciscoacl.ACL{}, parseError("before", err)
	}
	a, err := ciscoacl.Parse(after)
	if err != nil {
		return ciscoacl.ACL{}, ciscoacl.ACL{}, parseError("after", err)
	}
	if b.Name != a.Name || b.Kind != a.Kind {
		return b, a, &Error{Code: "IDENTITY_MISMATCH", Message: "ACL identities must match"}
	}
	return b, a, nil
}
func parseError(side string, err error) error {
	var e *ciscoacl.Error
	if errors.As(err, &e) {
		return &Error{Code: e.Code, Message: e.Message, Side: side, Line: e.Line}
	}
	return &Error{Code: "PARSE_FAILED", Message: "ACL parse failed", Side: side}
}
func input(data []byte, acl ciscoacl.ACL) evidence.Input {
	return evidence.Input{SHA256: evidence.ConfigDigest(data), Name: acl.Name, Kind: acl.Kind}
}
func recheck(b, a ciscoacl.ACL, w verify.Witness) error {
	x, err := b.Evaluate(w.Packet)
	if err != nil {
		return err
	}
	y, err := a.Evaluate(w.Packet)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
		return errors.New("witness source/decision mismatch")
	}
	return nil
}
func matches(r intent.Region, p ciscoacl.Packet) bool {
	protocol := r.Protocol
	if protocol != "ip" {
		expect := protocol
		switch protocol {
		case "tcp":
			expect = "6"
		case "udp":
			expect = "17"
		case "icmp":
			expect = "1"
		}
		// Compare normalized numeric protocol without any model interpretation.
		var n uint16
		for _, c := range expect {
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + uint16(c-'0')
		}
		if n > 255 || uint8(n) != p.Protocol {
			return false
		}
	}
	if !inPrefixes(r.SrcCIDR, r.ExcludeSrc, p.SrcIP) || !inPrefixes(r.DstCIDR, r.ExcludeDst, p.DstIP) {
		return false
	}
	return inPorts(r.SrcPorts, p.SrcPort) && inPorts(r.DstPorts, p.DstPort)
}
func inPrefixes(include string, excludes []string, addr netip.Addr) bool {
	p, err := netip.ParsePrefix(include)
	if err != nil || !p.Contains(addr) {
		return false
	}
	for _, s := range excludes {
		x, err := netip.ParsePrefix(s)
		if err != nil || x.Contains(addr) {
			return false
		}
	}
	return true
}
func inPorts(intervals []intent.Interval, p uint16) bool {
	if len(intervals) == 0 {
		return true
	}
	for _, i := range intervals {
		if p >= i.Lo && p <= i.Hi {
			return true
		}
	}
	return false
}

package repair

import (
	"context"
	"errors"
	"net/netip"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/verify"
	"plenilune/internal/workflow"
	"reflect"
	"strconv"
)

func Replay(ctx context.Context, before, after []byte, r Result) (workflow.ReplayResult, error) {
	if err := ctx.Err(); err != nil {
		return workflow.ReplayResult{}, err
	}
	if err := Check(r); err != nil {
		return workflow.ReplayResult{}, err
	}
	if evidence.ConfigDigest(before) != r.BeforeSHA256 || evidence.ConfigDigest(after) != r.BaseAfterSHA256 {
		return workflow.ReplayResult{}, errors.New("repair original input mismatch")
	}
	rechecked := 0
	timing := r.Collateral.Stats.TimingLimited
	if r.Comparison != nil {
		checked, err := workflow.Replay(ctx, before, []byte(r.Candidate), *r.Comparison)
		if err != nil {
			return workflow.ReplayResult{}, err
		}
		rechecked += checked.RecheckedWitnesses
		timing = timing || checked.Status == "TIMING_SENSITIVE"
	}
	if r.Collateral.Witness != nil {
		if r.Policy == nil || r.Policy.AllowedChanges == nil {
			return workflow.ReplayResult{}, errors.New("collateral witness lacks policy")
		}
		if err := checkCollateralWitness(before, []byte(r.Candidate), r.Policy.AllowedChanges, *r.Collateral.Witness); err != nil {
			return workflow.ReplayResult{}, err
		}
		rechecked++
	}
	// Verify rerenders from the original after, not from the stored candidate.
	// The embedded comparison is recomputed in its own recorded evidence
	// schema, so receipts from an earlier evidence era stay byte-identical.
	evidenceSchema := evidence.ComparisonSchema
	if r.Comparison != nil {
		evidenceSchema = r.Comparison.Schema
	}
	fresh, err := verifyVersion(ctx, before, after, r.Policy, r.Request, r.Options, r.Version, evidenceSchema)
	if err != nil {
		return workflow.ReplayResult{}, err
	}
	if fresh.Candidate != r.Candidate || fresh.CandidateSHA256 != r.CandidateSHA256 || fresh.PolicySHA256 != r.PolicySHA256 {
		return workflow.ReplayResult{}, errors.New("rerendered candidate or frozen policy differs")
	}
	if fresh.Collateral.Stats.TimingLimited || fresh.Comparison != nil && fresh.Comparison.Report.Budget.TimingLimited {
		timing = true
	}
	if timing {
		return workflow.ReplayResult{Status: "TIMING_SENSITIVE", SHA256: r.SHA256, RecheckedWitnesses: rechecked}, nil
	}
	if fresh.SHA256 != r.SHA256 {
		return workflow.ReplayResult{}, errors.New("replayed repair proof or candidate differs")
	}
	return workflow.ReplayResult{Status: "REPAIR_REPLAYED", SHA256: r.SHA256, RecheckedWitnesses: rechecked}, nil
}

func checkCollateralWitness(before, candidate []byte, allowed []intent.Region, w verify.Witness) error {
	b, err := ciscoacl.Parse(before)
	if err != nil {
		return err
	}
	c, err := ciscoacl.Parse(candidate)
	if err != nil {
		return err
	}
	x, err := b.Evaluate(w.Packet)
	if err != nil {
		return err
	}
	y, err := c.Evaluate(w.Packet)
	if err != nil {
		return err
	}
	if x.Action == y.Action || !reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
		return errors.New("collateral witness decision/source mismatch")
	}
	for _, r := range allowed {
		if contains(r, w.Packet) {
			return errors.New("collateral witness lies inside allowed region")
		}
	}
	return nil
}

func contains(r intent.Region, p ciscoacl.Packet) bool {
	if r.Protocol != "ip" {
		text := r.Protocol
		switch text {
		case "tcp":
			text = "6"
		case "udp":
			text = "17"
		case "icmp":
			text = "1"
		}
		n, err := strconv.ParseUint(text, 10, 8)
		if err != nil || uint8(n) != p.Protocol {
			return false
		}
	}
	if !inside(r.SrcCIDR, r.ExcludeSrc, p.SrcIP) || !inside(r.DstCIDR, r.ExcludeDst, p.DstIP) {
		return false
	}
	return port(r.SrcPorts, p.SrcPort) && port(r.DstPorts, p.DstPort)
}
func inside(text string, excludes []string, addr netip.Addr) bool {
	p, err := netip.ParsePrefix(text)
	if err != nil || !p.Contains(addr) {
		return false
	}
	for _, text := range excludes {
		x, err := netip.ParsePrefix(text)
		if err != nil || x.Contains(addr) {
			return false
		}
	}
	return true
}
func port(xs []intent.Interval, p uint16) bool {
	if len(xs) == 0 {
		return true
	}
	for _, x := range xs {
		if p >= x.Lo && p <= x.Hi {
			return true
		}
	}
	return false
}

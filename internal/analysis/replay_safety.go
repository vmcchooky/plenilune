package analysis

import (
	"context"
	"encoding/json"
	"net/netip"
	"reflect"
	"strconv"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/prefixlist"
	"plenilune/internal/verify"
)

func supportedVersions(v Versions) bool {
	current := currentVersions()
	if v.Intake != current.Intake || v.ACLParser != current.ACLParser {
		return false
	}
	// Artifacts sealed under a previous ACL engine replay with that
	// engine's accounting (reviewObject dispatches on Versions.ACLEngine).
	switch v.ACLEngine {
	case verify.EngineVersion, verify.ResidualEngineVersion, verify.IndexedEngineVersion:
	default:
		return false
	}
	prefixCurrent := v.PrefixListParser == current.PrefixListParser && v.PrefixListEngine == current.PrefixListEngine
	prefixLegacy := v.PrefixListParser == prefixlist.LegacyParserVersion && v.PrefixListEngine == prefixlist.LegacyEngineVersion
	return prefixCurrent || prefixLegacy
}

func mismatch() error {
	return invalid("EVIDENCE_MISMATCH", "replayed analysis or recorded evidence differs")
}

func aclTiming(a *ACLAnalysis) bool {
	return a != nil && (a.Budget.TimingLimited || a.Changes.Budget.TimingLimited)
}

// A timing stop excuses only the affected ACL computation, never unrelated
// prefix results, input metadata, coverage, policy or inventory. It is not a
// successful full replay. Stored point evidence is still checked independently.
// deadlineMarked reports whether an object was recorded as cut off by
// the run's context (P1-5 partial artifacts). Which objects a deadline
// cut is timing-dependent by construction, so such artifacts replay as
// TIMING_SENSITIVE, with the completed objects' point evidence still
// checked exactly.
func deadlineMarked(o Object) bool {
	return o.Reason == "deadline_exceeded" || o.Reason == "canceled"
}

func replaySafety(ctx context.Context, before, after []byte, recorded, fresh Result) (int, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	timing, n := false, 0
	for _, o := range fresh.Objects {
		timing = timing || aclTiming(o.ACL)
		if o.ACL != nil {
			n += len(o.ACL.Witnesses) + len(o.ACL.Changes.Classes)
		}
		if o.PrefixList != nil {
			n += len(o.PrefixList.Report.Classes)
		}
	}
	deadlineCut := false
	for _, o := range recorded.Objects {
		timing = timing || aclTiming(o.ACL)
		deadlineCut = deadlineCut || deadlineMarked(o)
	}
	freshCut := false
	for _, o := range fresh.Objects {
		freshCut = freshCut || deadlineMarked(o)
	}
	// A run cut by its deadline — the recorded one, or this rerun — makes
	// the comparison timing-dependent in either direction.
	timing = timing || deadlineCut || freshCut
	if !timing {
		return n, false, nil
	}
	if len(recorded.Objects) != len(fresh.Objects) {
		return 0, false, mismatch()
	}
	// Validate derived aggregate fields before excluding timing-dependent counts.
	derived := recorded
	derived.Summary = Summary{UnanalyzedChanges: recorded.Summary.UnanalyzedChanges}
	derived.Limitations = []string{}
	summarize(&derived)
	if !reflect.DeepEqual(derived.Summary, recorded.Summary) || !reflect.DeepEqual(derived.Limitations, recorded.Limitations) {
		return 0, false, mismatch()
	}
	x, y := recorded, fresh
	x.SHA256, y.SHA256 = "", ""
	x.Summary = Summary{UnanalyzedChanges: recorded.Summary.UnanalyzedChanges}
	y.Summary = Summary{UnanalyzedChanges: fresh.Summary.UnanalyzedChanges}
	x.Objects = append([]Object(nil), recorded.Objects...)
	y.Objects = append([]Object(nil), fresh.Objects...)
	n = 0
	for i, o := range recorded.Objects {
		f := fresh.Objects[i]
		if deadlineMarked(o) {
			// The recorded object was cut by the run's deadline; the
			// fresh run may have completed it. Identity must agree;
			// status, reason and engine payload are timing-dependent
			// and leave the projection on both sides.
			if o.Domain != f.Domain || o.Name != f.Name || o.Kind != f.Kind {
				return 0, false, mismatch()
			}
			x.Objects[i].Status, y.Objects[i].Status = "", ""
			x.Objects[i].Reason, y.Objects[i].Reason = "", ""
			x.Objects[i].ACL, y.Objects[i].ACL = nil, nil
			x.Objects[i].PrefixList, y.Objects[i].PrefixList = nil, nil
			continue
		}
		if deadlineMarked(f) {
			// The fresh run was cut where the recorded run completed.
			// The recorded point evidence is still rechecked exactly;
			// the fresh object's missing payload leaves the projection.
			if o.ACL != nil {
				if o.Before == nil || o.After == nil {
					return 0, false, mismatch()
				}
				b, err := sourceACL(before, o.Before)
				if err != nil {
					return 0, false, err
				}
				a, err := sourceACL(after, o.After)
				if err != nil {
					return 0, false, err
				}
				if err := validateACLPointEvidence(o, recorded, b, a); err != nil {
					return 0, false, err
				}
				n += len(o.ACL.Witnesses) + len(o.ACL.Changes.Classes)
			}
			x.Objects[i].Status, y.Objects[i].Status = "", ""
			x.Objects[i].Reason, y.Objects[i].Reason = "", ""
			x.Objects[i].ACL, y.Objects[i].ACL = nil, nil
			x.Objects[i].PrefixList, y.Objects[i].PrefixList = nil, nil
			continue
		}
		if o.ACL == nil || f.ACL == nil {
			continue
		} // Exact projection below checks both.
		if o.Before == nil || o.After == nil {
			return 0, false, mismatch()
		}
		b, err := sourceACL(before, f.Before)
		if err != nil {
			return 0, false, err
		}
		a, err := sourceACL(after, f.After)
		if err != nil {
			return 0, false, err
		}
		if err := validateACLPointEvidence(o, recorded, b, a); err != nil {
			return 0, false, err
		}
		n += len(o.ACL.Witnesses) + len(o.ACL.Changes.Classes)
		left, right := *o.ACL, *f.ACL
		if left.Budget.TimingLimited || right.Budget.TimingLimited {
			clearComparisonTiming(&left)
			clearComparisonTiming(&right)
			x.Objects[i].Status, y.Objects[i].Status = "", ""
		}
		if left.Changes.Budget.TimingLimited || right.Changes.Budget.TimingLimited {
			clearChangesTiming(&left.Changes)
			clearChangesTiming(&right.Changes)
		}
		x.Objects[i].ACL, y.Objects[i].ACL = &left, &right
	}
	for _, o := range recorded.Objects {
		if o.PrefixList != nil {
			n += len(o.PrefixList.Report.Classes)
		}
	}
	if deadlineCut || freshCut {
		// Whether a run was cut decides its interruption limitations,
		// the coverage lines of the cut objects (unanalyzed there,
		// analyzed here) and whether the global ledger record exists;
		// all three are timing-dependent. (The recorded limitations
		// were already validated against its own objects above.)
		x.Limitations, y.Limitations = nil, nil
		x.Coverage, y.Coverage = nil, nil
		x.GlobalBudget, y.GlobalBudget = nil, nil
	}
	// JSON equality treats nil/empty exactly as the sealed wire does, and covers
	// all current fields by default instead of an allowlist that can forget one.
	xb, ex := json.Marshal(x)
	yb, ey := json.Marshal(y)
	if ex != nil || ey != nil || string(xb) != string(yb) {
		return 0, false, mismatch()
	}
	return n, true, nil
}

func clearComparisonTiming(a *ACLAnalysis) {
	a.Verdict, a.Reason = "", ""
	a.SearchComplete, a.Truncated = false, false
	a.Witnesses = nil
	a.Budget = packetset.Stats{}
	// How much a timing-stopped search had decided is timing-dependent.
	a.Partial = nil
}
func clearChangesTiming(r *verify.ChangeReport) {
	r.Complete, r.ClassesTruncated = false, false
	r.Reason = ""
	r.Classes = nil
	r.TotalClasses = 0
	r.Budget = packetset.Stats{}
	r.Partial = nil
}

func sourceACL(source []byte, e *Extent) (ciscoacl.ACL, error) {
	if e == nil || e.ByteStart == nil || e.ByteEnd == nil || *e.ByteStart < 0 || *e.ByteEnd < *e.ByteStart || *e.ByteEnd > len(source) {
		return ciscoacl.ACL{}, mismatch()
	}
	a, err := ciscoacl.Parse(source[*e.ByteStart:*e.ByteEnd])
	if err != nil {
		return ciscoacl.ACL{}, mismatch()
	}
	return a, nil
}

func validateACLPointEvidence(o Object, r Result, b, a ciscoacl.ACL) error {
	x := o.ACL
	opts := r.Request.Options.ACL
	if o.Status != statusFor(x.Verdict) || x.Engine != r.Versions.ACLEngine || x.Reason != x.Budget.StopReason {
		return mismatch()
	}
	if x.Budget.TimingLimited && x.SearchComplete {
		return mismatch()
	}
	_, err := evidence.SealComparison(evidence.Comparison{
		Tool: r.Tool, EngineVersion: x.Engine,
		Before: evidence.Input{SHA256: o.Before.SHA256, Name: b.Name, Kind: b.Kind},
		After:  evidence.Input{SHA256: o.After.SHA256, Name: a.Name, Kind: a.Kind},
		Report: verify.Report{Verdict: x.Verdict, SearchComplete: x.SearchComplete, Reason: x.Reason, Witnesses: x.Witnesses, WitnessesTruncated: x.Truncated, Budget: x.Budget, Options: opts},
	})
	if err != nil {
		return mismatch()
	}
	for _, w := range x.Witnesses {
		if !validWitness(b, a, w, "diff") {
			return mismatch()
		}
	}
	c := x.Changes
	var allowed []intent.Region
	wantHash := ""
	for _, p := range r.Request.Policies {
		if p.Domain == DomainACL && p.Name == o.Name {
			allowed = p.Policy.AllowedChanges
			wantHash = p.PolicySHA256
		}
	}
	if x.PolicySHA256 != wantHash || c.ScopeDeclared != (allowed != nil) || c.Schema != verify.ChangeSchema || c.Engine != x.Engine || c.Options.Limits != opts.Limits || c.Options.MaxClasses != r.Request.Options.MaxClasses {
		return mismatch()
	}
	if !validStats(c.Budget, c.Options.Limits) || c.Reason != c.Budget.StopReason {
		return mismatch()
	}
	if c.Complete {
		if c.Budget.StopReason != "" || c.TotalClasses < len(c.Classes) || len(c.Classes) > c.Options.MaxClasses || c.ClassesTruncated != (c.TotalClasses > len(c.Classes)) {
			return mismatch()
		}
		if len(c.Classes) != min(c.TotalClasses, c.Options.MaxClasses) {
			return mismatch()
		}
		if c.Partial != nil {
			return mismatch()
		}
	} else if c.Partial == nil {
		// Engines before v0.4 withhold everything on a budget stop.
		if c.Reason == "" || len(c.Classes) != 0 || c.TotalClasses != 0 || c.ClassesTruncated {
			return mismatch()
		}
	} else {
		// A v0.4 partial explanation: every emitted class must be one of
		// the partial's decided pairs with the same fragment count, so a
		// class the search never decided cannot be smuggled in. (The
		// class witnesses themselves are re-evaluated below.)
		if c.Reason == "" || c.ClassesTruncated || len(c.Classes) > c.Options.MaxClasses {
			return mismatch()
		}
		decided := map[[2]int]int{}
		for _, pair := range c.Partial.DecidedPairs {
			decided[[2]int{pair.Before, pair.After}] = pair.Pieces
		}
		for _, cl := range c.Classes {
			bo, ao := -1, -1
			if cl.Before.Ordinal != nil {
				bo = *cl.Before.Ordinal
			}
			if cl.After.Ordinal != nil {
				ao = *cl.After.Ordinal
			}
			if pieces, ok := decided[[2]int{bo, ao}]; !ok || pieces != cl.Pieces {
				return mismatch()
			}
		}
	}
	for _, cl := range c.Classes {
		if cl.Pieces < 1 || len(cl.Examples) != min(cl.Pieces, 3) || cl.ExamplesTruncated != (cl.Pieces > len(cl.Examples)) || !validWitness(b, a, cl.Witness, "change") || !matchesRef(cl.Before, cl.Witness.Before) || !matchesRef(cl.After, cl.Witness.After) {
			return mismatch()
		}
		want := "newly_denied"
		if cl.Witness.After.Action == "permit" {
			want = "newly_permitted"
		}
		if cl.Effect != want {
			return mismatch()
		}
		if allowed == nil {
			if cl.Scope != "undeclared" || cl.OutsideWitness != nil {
				return mismatch()
			}
		} else {
			pointInside := false
			for _, region := range allowed {
				pointInside = pointInside || containsPacket(region, cl.Witness.Packet)
			}
			if cl.Scope == "inside_allowed" && !pointInside || cl.Scope == "outside_allowed" && pointInside {
				return mismatch()
			}
			if cl.Scope != "inside_allowed" && cl.Scope != "outside_allowed" && cl.Scope != "mixed" {
				return mismatch()
			}
			if cl.Scope == "inside_allowed" {
				if cl.OutsideWitness != nil {
					return mismatch()
				}
			} else {
				if cl.OutsideWitness == nil || !validWitness(b, a, *cl.OutsideWitness, "change-outside") || !matchesRef(cl.Before, cl.OutsideWitness.Before) || !matchesRef(cl.After, cl.OutsideWitness.After) {
					return mismatch()
				}
				for _, region := range allowed {
					if containsPacket(region, cl.OutsideWitness.Packet) {
						return mismatch()
					}
				}
			}
		}
	}
	return nil
}

func validStats(s packetset.Stats, l packetset.Limits) bool {
	if s.Operations < 0 || s.Operations > l.MaxOperations || s.PeakBoxes < 0 || s.PeakBoxes > l.MaxBoxes {
		return false
	}
	switch s.StopReason {
	case "", "operations", "boxes":
		return !s.TimingLimited
	case "wall_clock", "context_deadline":
		return s.TimingLimited
	}
	return false
}

func validWitness(b, a ciscoacl.ACL, w verify.Witness, label string) bool {
	x, err := b.Evaluate(w.Packet)
	if err != nil {
		return false
	}
	y, err := a.Evaluate(w.Packet)
	if err != nil || x.Action == y.Action || !reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
		return false
	}
	id := w.ID
	w.ID = ""
	encoded, err := json.Marshal(struct {
		Label   string
		Witness verify.Witness
	}{label, w})
	return err == nil && id == "wit-"+digest(encoded)
}
func matchesRef(r verify.RuleRef, d ciscoacl.Decision) bool {
	return r.Action == d.Action && r.Implicit == d.Implicit && reflect.DeepEqual(r.Ordinal, d.RuleOrdinal) && reflect.DeepEqual(r.Line, d.Line)
}
func containsPacket(r intent.Region, p ciscoacl.Packet) bool {
	protocol := r.Protocol
	switch protocol {
	case "tcp":
		protocol = "6"
	case "udp":
		protocol = "17"
	case "icmp":
		protocol = "1"
	}
	if protocol != "ip" {
		v, err := strconv.ParseUint(protocol, 10, 8)
		if err != nil || uint8(v) != p.Protocol {
			return false
		}
	}
	inside := func(text string, excluded []string, ip netip.Addr) bool {
		n, err := netip.ParsePrefix(text)
		if err != nil || !n.Contains(ip) {
			return false
		}
		for _, s := range excluded {
			e, err := netip.ParsePrefix(s)
			if err != nil || e.Contains(ip) {
				return false
			}
		}
		return true
	}
	port := func(intervals []intent.Interval, v uint16) bool {
		if len(intervals) == 0 {
			return true
		}
		for _, i := range intervals {
			if v >= i.Lo && v <= i.Hi {
				return true
			}
		}
		return false
	}
	return inside(r.SrcCIDR, r.ExcludeSrc, p.SrcIP) && inside(r.DstCIDR, r.ExcludeDst, p.DstIP) && port(r.SrcPorts, p.SrcPort) && port(r.DstPorts, p.DstPort)
}

package repair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/patch"
	"plenilune/internal/verify"
	"plenilune/internal/workflow"
)

// Deterministic repair enumeration (P1-3). From the change classes and
// obligation violations of an (before, after, policy) input, Enumerate
// builds a finite candidate set with bounded transforms, verifies every
// candidate with the unchanged Verify gate, and returns the candidates
// in one fixed order. The enumeration is a pure function of its input:
// the same input produces a byte-identical record across runs, machines
// and process restarts, because generation order, deduplication and the
// sort key are all derived from the input alone — never from map
// iteration, wall time, or the order proposals happened to arrive in.
//
// The record is an annotation layer over ordinary repair receipts: the
// receipt format (plenilune.repair/v1, acl-repair/v0.4) is unchanged, so
// no repair version bump was needed and receipts from enumeration replay
// through the existing Replay path. The enumeration has its own schema
// and scheme version below.
const EnumerationSchema = "plenilune.repair-enumeration/v1"
const EnumerationVersion = "repair-enumeration/v1"

// MaxEnumerationBytes bounds the sealed enumeration record. A record
// embeds one full receipt per candidate, and each receipt embeds the
// rendered candidate source, so the bound is generous but finite.
const MaxEnumerationBytes = 64 << 20

// Enumeration bounds from the plan item: targets and generated
// candidates are capped, per-target fan-out is capped, and the whole
// enumeration answers to the caller's context deadline.
const (
	MaxEnumerateTargets    = 32
	MaxEnumerateCandidates = 64
	maxTargetBoxes         = 8
	maxDecidingRules       = 8
)

// Transform names recorded on candidates. The first five are the bounded
// transforms of the plan item; TransformExternal marks a caller-supplied
// proposal (for example an AI proposal) folded into the same frame.
const (
	TransformRemoveRule      = "remove_deciding_rule"
	TransformFlipRule        = "flip_deciding_rule"
	TransformInsertException = "insert_exception"
	TransformReorderSwap     = "reorder_swapped_rules"
	TransformNarrowRule      = "narrow_deciding_rule"
	TransformExternal        = "external"
)

// Candidate is one enumerated repair proposal with its real Verify
// receipt. Index is the candidate's position in the deterministic
// order; Resolved counts the enumeration targets the candidate's own
// receipt proves fixed (see Enumerate for the exact definition). A
// candidate is a repair only when Result.Status is VERIFIED; anything
// else is a rejected or unverified proposal with its reason, never a
// success.
type Candidate struct {
	Index     int           `json:"index"`
	Transform string        `json:"transform"`
	Target    string        `json:"target"`
	Resolved  int           `json:"resolved"`
	Request   patch.Request `json:"request"`
	Result    Result        `json:"result"`
}

// Enumeration is the sealed deterministic candidate record for one
// (before, after, policy, options) input. Complete is false when the
// underlying change or obligation analysis stopped on budget: the
// record then carries the stop reason and no candidates, because
// enumeration targets cannot be established exactly — incompleteness is
// never dressed up as "nothing to repair".
type Enumeration struct {
	Schema              string      `json:"schema"`
	Version             string      `json:"version"`
	Complete            bool        `json:"complete"`
	Reason              string      `json:"reason,omitempty"`
	BeforeSHA256        string      `json:"before_sha256"`
	BaseAfterSHA256     string      `json:"base_after_sha256"`
	PolicySHA256        string      `json:"policy_sha256,omitempty"`
	RepairVersion       string      `json:"repair_version"`
	Targets             int         `json:"targets"`
	TargetsTruncated    bool        `json:"targets_truncated,omitempty"`
	CandidatesTruncated bool        `json:"candidates_truncated,omitempty"`
	Candidates          []Candidate `json:"candidates"`
	// Sensitive follows the receipt convention: the record embeds
	// rendered candidate configuration text. It is a handling label,
	// excluded from the digest, not evidence content.
	Sensitive bool   `json:"sensitive,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

type enumTarget struct {
	label string
	class *verify.ClassRegion
	ob    *verify.ObligationRegion
}

type generated struct {
	transform string
	target    string
	request   patch.Request
}

// Enumerate builds and verifies the deterministic candidate set for one
// input. external carries caller-supplied proposals (for example AI
// proposals captured from a session): they enter the same enumeration
// space — deduplicated against engine-enumerated candidates by request
// digest, verified by the same gate, and sorted by the same key — so a
// proposal can never gain rank from arriving early or from its source.
//
// Targets are the change classes outside (or straddling) the accepted
// allowed-change scope — every class when no scope is declared — plus
// one target per violated obligation. Candidate order is the plan's:
// (number of edits, number of targets resolved, candidate hash). The
// order is a deterministic display order, not a quality claim: no
// candidate is called "best" or "safest".
//
// Resolved is computed from the candidate's own sealed receipt only: one
// point per violated obligation the candidate's comparison proves HOLDS,
// plus every class target when the receipt's collateral is VERIFIED
// (collateral is a single exact proof that behavior outside the allowed
// scope matches the original before, so class resolution is
// all-or-nothing — no partial class credit is invented).
func Enumerate(ctx context.Context, before, after []byte, policy *Policy, opts verify.Options, external ...patch.Request) (Enumeration, error) {
	if len(before) > ciscoacl.MaxInputBytes || len(after) > ciscoacl.MaxInputBytes {
		return Enumeration{}, errors.New("ACL input exceeds limit")
	}
	beforeSHA := evidence.ConfigDigest(before)
	afterSHA := evidence.ConfigDigest(after)
	e := Enumeration{Schema: EnumerationSchema, Version: EnumerationVersion, Complete: true, BeforeSHA256: beforeSHA, BaseAfterSHA256: afterSHA, RepairVersion: Version, Candidates: []Candidate{}}
	var np *Policy
	obligationsDigest := strings.Repeat("0", 64)
	if policy != nil {
		n, err := NormalizePolicy(*policy)
		if err != nil {
			return Enumeration{}, err
		}
		np = &n
		e.PolicySHA256, err = PolicyDigest(n)
		if err != nil {
			return Enumeration{}, err
		}
		_, obligationsDigest, err = intent.Digest(n.Conditions)
		if err != nil {
			return Enumeration{}, err
		}
	}
	bACL, err := ciscoacl.Parse(before)
	if err != nil {
		return Enumeration{}, err
	}
	aACL, err := ciscoacl.Parse(after)
	if err != nil {
		return Enumeration{}, err
	}
	var allowed []intent.Region
	if np != nil {
		allowed = np.AllowedChanges
	}
	classRegions, changeRep, classesComplete, err := verify.ChangeClassRegions(ctx, bACL, aACL, allowed, verify.ChangeOptions{Limits: opts.Limits, MaxClasses: verify.MaxClasses})
	if err != nil {
		return Enumeration{}, err
	}
	if !classesComplete {
		e.Complete, e.Reason = false, changeRep.Reason
		return sealEnumeration(e)
	}
	targets := []enumTarget{}
	for i := range classRegions {
		cr := &classRegions[i]
		if allowed != nil && cr.Class.Scope != "outside_allowed" && cr.Class.Scope != "mixed" {
			continue
		}
		targets = append(targets, enumTarget{label: fmt.Sprintf("class:%d:%d", ordRef(cr.Class.Before), ordRef(cr.Class.After)), class: cr})
	}
	violated := map[string]bool{}
	if np != nil {
		obRegions, compRep, obsComplete, err := verify.ObligationViolationRegions(ctx, bACL, aACL, &np.Conditions, opts)
		if err != nil {
			return Enumeration{}, err
		}
		if !obsComplete {
			e.Complete, e.Reason = false, compRep.Reason
			return sealEnumeration(e)
		}
		for i := range obRegions {
			or := &obRegions[i]
			targets = append(targets, enumTarget{label: "obligation:" + or.Obligation.ID, ob: or})
			violated[or.Obligation.ID] = true
		}
	}
	if len(targets) > MaxEnumerateTargets {
		targets = targets[:MaxEnumerateTargets]
		e.TargetsTruncated = true
	}
	e.Targets = len(targets)
	classTargets := 0
	for _, t := range targets {
		if t.class != nil {
			classTargets++
		}
	}

	gen := &generator{ctx: ctx, before: before, after: after, bACL: bACL, aACL: aACL, beforeSHA: beforeSHA, afterSHA: afterSHA, obligationsDigest: obligationsDigest, opts: opts}
	for _, t := range targets {
		if gen.full() {
			e.CandidatesTruncated = true
			break
		}
		if t.class != nil {
			gen.classTarget(t)
		} else {
			gen.obligationTarget(t)
		}
	}
	if gen.err != nil {
		return Enumeration{}, gen.err
	}
	if gen.truncated {
		e.CandidatesTruncated = true
	}
	list := gen.list
	for _, req := range external {
		list = append(list, generated{transform: TransformExternal, target: "external", request: req})
	}
	seen := map[string]bool{}
	uniq := list[:0]
	for _, g := range list {
		d, err := requestDigest(g.request)
		if err != nil {
			return Enumeration{}, err
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		uniq = append(uniq, g)
	}
	type sortable struct {
		c   Candidate
		key string
	}
	rows := []sortable{}
	for _, g := range uniq {
		res, err := Verify(ctx, before, after, policy, g.request, opts)
		if err != nil {
			return Enumeration{}, err
		}
		resolved := 0
		if res.Comparison != nil {
			for _, ob := range res.Comparison.Report.Obligations {
				if violated[ob.ID] && ob.Status == "HOLDS" {
					resolved++
				}
			}
		}
		if res.Collateral.Status == "VERIFIED" {
			resolved += classTargets
		}
		key := res.CandidateSHA256
		if key == "" {
			key, err = requestDigest(g.request)
			if err != nil {
				return Enumeration{}, err
			}
		}
		rows = append(rows, sortable{c: Candidate{Transform: g.transform, Target: g.target, Resolved: resolved, Request: g.request, Result: res}, key: key})
	}
	slices.SortStableFunc(rows, func(x, y sortable) int {
		if n := len(x.c.Request.Edits) - len(y.c.Request.Edits); n != 0 {
			return n
		}
		if x.c.Resolved != y.c.Resolved {
			return y.c.Resolved - x.c.Resolved
		}
		return strings.Compare(x.key, y.key)
	})
	for i := range rows {
		rows[i].c.Index = i
		e.Candidates = append(e.Candidates, rows[i].c)
		if rows[i].c.Result.Candidate != "" {
			e.Sensitive = true
		}
	}
	return sealEnumeration(e)
}

func ordRef(r verify.RuleRef) int {
	if r.Ordinal == nil {
		return -1
	}
	return *r.Ordinal
}

func requestDigest(r patch.Request) (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return evidence.ConfigDigest(b), nil
}

type generator struct {
	ctx               context.Context
	before, after     []byte
	bACL, aACL        ciscoacl.ACL
	beforeSHA         string
	afterSHA          string
	obligationsDigest string
	opts              verify.Options
	list              []generated
	truncated         bool
	err               error
}

func (g *generator) full() bool { return len(g.list) >= MaxEnumerateCandidates }

func (g *generator) add(transform, target string, edits []patch.Edit) {
	if g.err != nil {
		return
	}
	if len(edits) < 1 || len(edits) > patch.MaxEdits {
		return
	}
	if g.full() {
		g.truncated = true
		return
	}
	g.list = append(g.list, generated{transform: transform, target: target, request: patch.Request{Schema: patch.Schema, BaseBeforeSHA256: g.beforeSHA, BaseAfterSHA256: g.afterSHA, ObligationsDigest: g.obligationsDigest, Scope: intent.Scope, Edits: edits}})
}

func (g *generator) ref(rule ciscoacl.Rule) *patch.RuleRef {
	return &patch.RuleRef{BaseAfterSHA256: g.afterSHA, Ordinal: rule.Ordinal, Line: rule.Line}
}

// classTarget generates the bounded transforms for one change class: the
// deciding after rule is the class's own after rule by construction.
func (g *generator) classTarget(t enumTarget) {
	cr := t.class
	afterOrd := ordRef(cr.Class.After)
	if afterOrd >= 0 {
		rule := g.aACL.Rules[afterOrd]
		g.add(TransformRemoveRule, t.label, []patch.Edit{{Op: "remove", Ref: g.ref(rule)}})
		if body, ok := flipBody(ruleBody(g.after, rule)); ok {
			g.add(TransformFlipRule, t.label, []patch.Edit{{Op: "replace", Ref: g.ref(rule), Content: body}})
		}
	}
	for i, box := range cr.Boxes {
		if i >= maxTargetBoxes {
			break
		}
		text, ok := verify.RenderBoxRule(cr.Class.Before.Action, box)
		if !ok {
			continue
		}
		if afterOrd >= 0 {
			g.add(TransformInsertException, t.label, []patch.Edit{{Op: "add_before", Ref: g.ref(g.aACL.Rules[afterOrd]), Content: text}})
		} else {
			g.add(TransformInsertException, t.label, []patch.Edit{{Op: "append", Content: text}})
		}
	}
	g.reorderCandidate(t)
	if afterOrd >= 0 {
		g.narrow(t, g.aACL.Rules[afterOrd], cr.Boxes)
	}
}

// obligationTarget generates the bounded transforms for one violated
// obligation. The deciding after rules are found by evaluating a sample
// packet from each violation box with the linear evaluator — the same
// independent check witnesses go through.
func (g *generator) obligationTarget(t enumTarget) {
	or := t.ob
	deciders := decidingRules(g.aACL, or.Boxes, maxDecidingRules)
	for _, ord := range deciders {
		if ord < 0 {
			continue
		}
		rule := g.aACL.Rules[ord]
		g.add(TransformRemoveRule, t.label, []patch.Edit{{Op: "remove", Ref: g.ref(rule)}})
		if body, ok := flipBody(ruleBody(g.after, rule)); ok {
			g.add(TransformFlipRule, t.label, []patch.Edit{{Op: "replace", Ref: g.ref(rule), Content: body}})
		}
	}
	for i, box := range or.Boxes {
		if i >= maxTargetBoxes {
			break
		}
		action := ""
		switch or.Obligation.Kind {
		case "must_permit":
			action = "permit"
		case "must_deny":
			action = "deny"
		case "must_remain":
			p, ok := samplePacket(box)
			if !ok {
				continue
			}
			dec, err := g.bACL.Evaluate(p)
			if err != nil {
				g.err = err
				return
			}
			action = dec.Action
		}
		text, ok := verify.RenderBoxRule(action, box)
		if !ok {
			continue
		}
		dec := decidingRule(g.aACL, box)
		if dec >= 0 {
			g.add(TransformInsertException, t.label, []patch.Edit{{Op: "add_before", Ref: g.ref(g.aACL.Rules[dec]), Content: text}})
		} else {
			g.add(TransformInsertException, t.label, []patch.Edit{{Op: "append", Content: text}})
		}
	}
	for _, ord := range deciders {
		if ord < 0 {
			continue
		}
		g.narrow(t, g.aACL.Rules[ord], or.Boxes)
	}
}

// narrow rewrites the deciding rule's predicate to rule \ violation
// region, but only when the difference is exactly one box and that box
// renders as one valid rule — the plan's representability condition.
func (g *generator) narrow(t enumTarget, rule ciscoacl.Rule, violation []packetset.Box) {
	if g.err != nil {
		return
	}
	raw, err := verify.RuleRegion(rule, g.opts.Limits)
	if err != nil {
		g.err = err
		return
	}
	b, err := packetset.NewBudget(g.ctx, g.opts.Limits)
	if err != nil {
		g.err = err
		return
	}
	defer b.Close()
	diff, err := packetset.Difference(raw, violation, b)
	if err != nil {
		// A budget stop while shaping a candidate is not an input
		// error: the transform is simply not offered.
		if errors.Is(err, packetset.ErrLimit) {
			return
		}
		g.err = err
		return
	}
	if len(diff) != 1 {
		return
	}
	text, ok := verify.RenderBoxRule(rule.Action, diff[0])
	if !ok {
		return
	}
	g.add(TransformNarrowRule, t.label, []patch.Edit{{Op: "replace", Ref: g.ref(rule), Content: text}})
}

// reorderCandidate detects the one reorder shape the plan covers: the
// class's deciding pair are two rules whose contents were swapped
// between before and after. The candidate removes the later swapped
// rule and re-adds it before the earlier one (patch v1 has no move
// primitive; reorder is remove + add, re-checked like any candidate).
func (g *generator) reorderCandidate(t enumTarget) {
	cr := t.class
	i, j := ordRef(cr.Class.Before), ordRef(cr.Class.After)
	if i < 0 || j < 0 {
		return
	}
	j2 := indexOfRuleKey(g.aACL.Rules, ruleKey(g.bACL.Rules[i]))
	i2 := indexOfRuleKey(g.bACL.Rules, ruleKey(g.aACL.Rules[j]))
	if j2 < 0 || i2 < 0 || j2 == j || i2 == i {
		return
	}
	lo, hi := j, j2
	if lo > hi {
		lo, hi = hi, lo
	}
	body := ruleBody(g.after, g.aACL.Rules[hi])
	if body == "" {
		return
	}
	g.add(TransformReorderSwap, t.label, []patch.Edit{
		{Op: "remove", Ref: g.ref(g.aACL.Rules[hi])},
		{Op: "add_before", Ref: g.ref(g.aACL.Rules[lo]), Content: body},
	})
}

// decidingRules returns the distinct after rules deciding the boxes'
// sample packets, in ascending ordinal order with the implicit deny (-1)
// last, capped at max rules.
func decidingRules(acl ciscoacl.ACL, boxes []packetset.Box, max int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, box := range boxes {
		ord := decidingRule(acl, box)
		if !seen[ord] {
			seen[ord] = true
			out = append(out, ord)
		}
	}
	slices.SortFunc(out, func(a, b int) int {
		if a < 0 {
			return 1
		}
		if b < 0 {
			return -1
		}
		return a - b
	})
	if len(out) > max {
		out = out[:max]
	}
	return out
}

func decidingRule(acl ciscoacl.ACL, box packetset.Box) int {
	p, ok := samplePacket(box)
	if !ok {
		return -1
	}
	dec, err := acl.Evaluate(p)
	if err != nil || dec.Implicit || dec.RuleOrdinal == nil {
		return -1
	}
	return *dec.RuleOrdinal
}

func samplePacket(box packetset.Box) (ciscoacl.Packet, bool) {
	proto, src, dst, sport, dport, ok := box.Witness()
	if !ok {
		return ciscoacl.Packet{}, false
	}
	addr := func(n uint32) netip.Addr {
		return netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	}
	return ciscoacl.Packet{Protocol: proto, SrcIP: addr(src), DstIP: addr(dst), SrcPort: sport, DstPort: dport}, true
}

// ruleBody extracts a rule's unsequenced body text from the base source:
// the parser's line for that rule, minus its sequence number when the
// source carries one.
func ruleBody(source []byte, rule ciscoacl.Rule) string {
	lines := strings.Split(string(source), "\n")
	if rule.Line < 1 || rule.Line > len(lines) {
		return ""
	}
	fields := strings.Fields(strings.TrimSuffix(lines[rule.Line-1], "\r"))
	if rule.Sequence != nil && len(fields) > 0 && fields[0] == strconv.FormatUint(uint64(*rule.Sequence), 10) {
		fields = fields[1:]
	}
	return strings.Join(fields, " ")
}

func flipBody(body string) (string, bool) {
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return "", false
	}
	switch strings.ToLower(fields[0]) {
	case "permit":
		fields[0] = "deny"
	case "deny":
		fields[0] = "permit"
	default:
		return "", false
	}
	return strings.Join(fields, " "), true
}

// ruleKey identifies a rule by semantics only — action, protocol,
// addresses and ports — ignoring ordinal, line, sequence and logging.
func ruleKey(r ciscoacl.Rule) string {
	b, err := json.Marshal(struct {
		Action   string              `json:"action"`
		Protocol *uint8              `json:"protocol"`
		Src      ciscoacl.Address    `json:"src"`
		Dst      ciscoacl.Address    `json:"dst"`
		SrcPorts []ciscoacl.Interval `json:"src_ports"`
		DstPorts []ciscoacl.Interval `json:"dst_ports"`
	}{r.Action, r.Protocol, r.Src, r.Dst, r.SrcPorts, r.DstPorts})
	if err != nil {
		return ""
	}
	return string(b)
}

func indexOfRuleKey(rules []ciscoacl.Rule, key string) int {
	if key == "" {
		return -1
	}
	for i, r := range rules {
		if ruleKey(r) == key {
			return i
		}
	}
	return -1
}

func sealEnumeration(e Enumeration) (Enumeration, error) {
	if err := validateEnumeration(e); err != nil {
		return Enumeration{}, err
	}
	h, err := enumerationDigest(e)
	if err != nil {
		return Enumeration{}, err
	}
	e.SHA256 = h
	return e, nil
}

func enumerationDigest(e Enumeration) (string, error) {
	e.SHA256 = ""
	e.Sensitive = false // handling label only; never part of the sealed projection
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	if len(b) > MaxEnumerationBytes {
		return "", errors.New("repair enumeration exceeds limit")
	}
	return evidence.ConfigDigest(b), nil
}

// CheckEnumeration verifies the record's seal and internal consistency:
// every embedded receipt passes Check, indexes are sequential, and the
// candidate order is the deterministic order of the scheme.
func CheckEnumeration(e Enumeration) error {
	if err := validateEnumeration(e); err != nil {
		return err
	}
	h, err := enumerationDigest(e)
	if err != nil {
		return err
	}
	if h != e.SHA256 {
		return errors.New("repair enumeration digest mismatch")
	}
	return nil
}

func validateEnumeration(e Enumeration) error {
	if e.Schema != EnumerationSchema || e.Version != EnumerationVersion {
		return errors.New("unsupported repair enumeration schema/version")
	}
	if e.RepairVersion != Version {
		return errors.New("unsupported enumeration repair version")
	}
	if !e.Complete {
		if e.Reason == "" || len(e.Candidates) != 0 {
			return errors.New("incomplete enumeration must carry a reason and no candidates")
		}
		return nil
	}
	prev := ""
	for i, c := range e.Candidates {
		if c.Index != i {
			return errors.New("enumeration candidate index out of order")
		}
		if err := Check(c.Result); err != nil {
			return err
		}
		if !reflect.DeepEqual(c.Request, c.Result.Request) {
			return errors.New("enumeration candidate request differs from receipt")
		}
		if c.Resolved < 0 || c.Resolved > e.Targets {
			return errors.New("enumeration candidate resolved count out of range")
		}
		key := c.Result.CandidateSHA256
		if key == "" {
			d, err := requestDigest(c.Request)
			if err != nil {
				return err
			}
			key = d
		}
		cur := fmt.Sprintf("%03d/%03d/%s", len(c.Request.Edits), 999-c.Resolved, key)
		if i > 0 && cur < prev {
			return errors.New("enumeration candidates out of deterministic order")
		}
		prev = cur
	}
	return nil
}

// ReplayEnumeration re-runs the enumeration from the original inputs and
// the recorded external proposals and requires the same sealed record,
// mirroring repair Replay: byte-level agreement first, and a
// TIMING_SENSITIVE disposition — never a pass — when only timing-limited
// accounting differs.
func ReplayEnumeration(ctx context.Context, before, after []byte, policy *Policy, e Enumeration) (workflow.ReplayResult, error) {
	if err := ctx.Err(); err != nil {
		return workflow.ReplayResult{}, err
	}
	if err := CheckEnumeration(e); err != nil {
		return workflow.ReplayResult{}, err
	}
	if evidence.ConfigDigest(before) != e.BeforeSHA256 || evidence.ConfigDigest(after) != e.BaseAfterSHA256 {
		return workflow.ReplayResult{}, errors.New("enumeration original input mismatch")
	}
	if e.PolicySHA256 == "" {
		if policy != nil {
			return workflow.ReplayResult{}, errors.New("enumeration policy mismatch")
		}
	} else {
		if policy == nil {
			return workflow.ReplayResult{}, errors.New("enumeration policy mismatch")
		}
		h, err := PolicyDigest(*policy)
		if err != nil {
			return workflow.ReplayResult{}, err
		}
		if h != e.PolicySHA256 {
			return workflow.ReplayResult{}, errors.New("enumeration policy mismatch")
		}
	}
	opts := verify.DefaultOptions()
	external := []patch.Request{}
	for _, c := range e.Candidates {
		if c.Transform == TransformExternal {
			external = append(external, c.Request)
		}
	}
	if len(e.Candidates) > 0 {
		opts = e.Candidates[0].Result.Options
	}
	fresh, err := Enumerate(ctx, before, after, policy, opts, external...)
	if err != nil {
		return workflow.ReplayResult{}, err
	}
	if fresh.SHA256 == e.SHA256 {
		return workflow.ReplayResult{Status: "ENUMERATION_REPLAYED", SHA256: e.SHA256}, nil
	}
	timing := false
	sameShape := len(fresh.Candidates) == len(e.Candidates)
	if sameShape {
		for i := range fresh.Candidates {
			f, o := fresh.Candidates[i], e.Candidates[i]
			if f.Result.Status != o.Result.Status || f.Result.CandidateSHA256 != o.Result.CandidateSHA256 {
				sameShape = false
				break
			}
			if f.Result.Collateral.Stats.TimingLimited || o.Result.Collateral.Stats.TimingLimited {
				timing = true
			}
			if f.Result.Comparison != nil && f.Result.Comparison.Report.Budget.TimingLimited ||
				o.Result.Comparison != nil && o.Result.Comparison.Report.Budget.TimingLimited {
				timing = true
			}
		}
	}
	if sameShape && timing {
		return workflow.ReplayResult{Status: "TIMING_SENSITIVE", SHA256: e.SHA256}, nil
	}
	return workflow.ReplayResult{}, errors.New("replayed enumeration differs")
}

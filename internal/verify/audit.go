package verify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"slices"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
)

// Exact single-ACL rule audit (P1-4). The audit examines one ACL on its
// own — no comparison — and reports two layers that are never mixed:
//
//   - Findings are exact statements about the modeled semantics, proved
//     with the same box algebra and linear evaluator the comparison
//     engine uses: a rule is unreachable when its effective region is
//     empty, and redundant when removing it provably changes no packet's
//     decision. No finding is heuristic and none is ever upgraded from
//     an incomplete search.
//   - Relations are exact match-set relations between rule pairs in the
//     Al-Shaer & Hamed taxonomy (generalization, correlation). The
//     relations themselves are computed exactly, but any warning read
//     into them is heuristic: a generalization pair is the normal
//     exception-before-general-rule idiom as often as it is a mistake.
//     They are therefore reported separately from findings.
//
// The audit is report-only: it never edits the ACL and offers no
// removal path. Display order puts rules with an empty effective
// region first, because real ACLs carry intentional deny-before-permit
// idioms and the reviewer should see provably dead rules before
// redundancy notes and relation warnings.
//
// Nothing in the audit materializes a whole effective region: the
// effective region of a broad late rule is the complement of every
// earlier match and fragments far beyond the box budget even for
// small ACLs (the same phenomenon the comparison engine avoids by
// never building deny-side permit sets). All three passes therefore
// work on streamed carve pieces (streamDifference): a depth-first
// carve whose live stack stays along one carve path, with early exit
// as soon as the question at hand is decided.
//
// Budget discipline matches the comparison engine: one budget covers
// the whole audit, and exhaustion ends the audit INCONCLUSIVE with
// the stage and rule cursor of the stop recorded — never as a clean
// bill of health for the unexamined rules.

// Audit finding and relation kinds.
const (
	AuditUnreachable    = "unreachable"
	AuditRedundant      = "redundant"
	AuditGeneralization = "generalization"
	AuditCorrelation    = "correlation"
)

// Audit stage values: the pass a budget stop interrupted.
const (
	AuditStageEffective  = "effective"
	AuditStageRedundancy = "redundancy"
	AuditStageRelations  = "relations"
)

// AuditWitness is one evaluator-checked packet attached to an audit
// finding or relation: the packet itself and the decision the linear
// evaluator gives it in the audited ACL. It demonstrates the reported
// statement where a single packet can (an unreachable rule's packet
// decided earlier; a relation's packet inside both rules' matches).
// Universal statements (redundancy) are proved by region algebra and
// carry region evidence instead.
type AuditWitness struct {
	ID       string            `json:"id"`
	Packet   ciscoacl.Packet   `json:"packet"`
	Decision ciscoacl.Decision `json:"decision"`
}

// AuditFinding is one exact statement about one rule.
//
// Unreachable: EffectiveBoxes is 0 — every packet the rule matches is
// decided by an earlier rule. Witness, when the match region is
// nonempty, is a packet from the rule's own match that the evaluator
// decides at an earlier rule. MatchBoxes counts the boxes of the rule's
// raw match region (0 when the rule matches nothing at all).
//
// Redundant: the rule's effective region is nonempty, but replaying
// first-match over that region with only the later rules — the exact
// post-removal semantics — decides every packet with the rule's own
// action. CoveredBy lists, in ascending order, the later rules that
// decide part of the region with the same action; ImplicitDeny records
// that the remainder falls through to the implicit final deny (only
// possible for a deny rule). This implies the coverage form
// E_i ⊆ union(later same-action rules) for the covered part, and
// additionally recognizes the implicit-deny fallthrough.
// EffectiveBoxes counts the effective region's boxes in the audit's
// streaming carve decomposition (a valid disjoint decomposition of
// the region; box counts are representation-dependent, as elsewhere
// in this engine).
type AuditFinding struct {
	Kind           string        `json:"kind"`
	Rule           int           `json:"rule"`
	Line           int           `json:"line"`
	Action         string        `json:"action"`
	MatchBoxes     int           `json:"match_boxes"`
	EffectiveBoxes int           `json:"effective_boxes"`
	CoveredBy      []int         `json:"covered_by,omitempty"`
	ImplicitDeny   bool          `json:"implicit_deny,omitempty"`
	Witness        *AuditWitness `json:"witness,omitempty"`
}

// AuditRelation is one exact match-set relation between an earlier and
// a later rule whose actions differ, in the Al-Shaer & Hamed taxonomy:
//
//   - generalization: the earlier rule's match is a strict subset of
//     the later rule's match — the earlier rule is an exception placed
//     before a broader rule of the other action.
//   - correlation: the two matches partially overlap — each matches
//     packets the other does not, and they share packets.
//
// Pairs whose matches are equal, nested the other way, or same-action
// are not relations: equal matches with different actions are already
// covered by the later rule's unreachable finding, and same-action
// overlap is the mechanism of redundancy, not a relation. The witness
// packet lies in both matches — inside the earlier rule's effective
// region when any of it reaches the later rule — and its recorded
// decision is whatever the evaluator actually decides.
type AuditRelation struct {
	Kind          string        `json:"kind"`
	Earlier       int           `json:"earlier"`
	Later         int           `json:"later"`
	EarlierAction string        `json:"earlier_action"`
	LaterAction   string        `json:"later_action"`
	Witness       *AuditWitness `json:"witness,omitempty"`
}

// AuditReport is the result of auditing one ACL. Complete is true only
// when every pass finished; on a budget stop Complete is false, Reason
// carries the budget stop reason, and Stage/StageRule (with
// StageRule2 for the pair passes) name where the search stopped.
// Findings decided before the stop stay — each is exact on its own —
// and everything from the stop onward is unexamined, never implied
// clean. RelationsComplete is false whenever the relation pass did not
// finish (including when an earlier pass stopped).
type AuditReport struct {
	Complete          bool            `json:"complete"`
	Reason            string          `json:"reason,omitempty"`
	Stage             string          `json:"stage,omitempty"`
	StageRule         *int            `json:"stage_rule,omitempty"`
	StageRule2        *int            `json:"stage_rule2,omitempty"`
	Rules             int             `json:"rules"`
	Findings          []AuditFinding  `json:"findings"`
	Relations         []AuditRelation `json:"relations"`
	RelationsComplete bool            `json:"relations_complete"`
	Budget            packetset.Stats `json:"budget"`
	Options           Options         `json:"options"`
}

// AuditACL audits one ACL exactly under the given options. MaxWitnesses
// in opts is not used: audit witnesses are one demonstrating packet per
// finding or relation, not a capped sample.
func AuditACL(ctx context.Context, acl ciscoacl.ACL, opts Options) (AuditReport, error) {
	probe := ciscoacl.Packet{SrcIP: netip.IPv4Unspecified(), DstIP: netip.IPv4Unspecified()}
	if _, err := acl.Evaluate(probe); err != nil {
		return AuditReport{}, err
	}
	b, err := packetset.NewBudget(ctx, opts.Limits)
	if err != nil {
		return AuditReport{}, err
	}
	defer b.Close()
	n := len(acl.Rules)
	r := AuditReport{Rules: n, Findings: []AuditFinding{}, Relations: []AuditRelation{}, Options: opts}
	// incomplete records a budget stop: the findings gathered so far
	// stay, the stop is named by stage and rule cursor, and nothing
	// past the stop is characterized. Non-budget errors (including
	// cancellation) are returned as errors, mirroring Compare.
	incomplete := func(stage string, rule, rule2 int, cause error) (AuditReport, error) {
		if !errors.Is(cause, packetset.ErrLimit) {
			return AuditReport{}, cause
		}
		r.Budget = b.Stats()
		r.Reason = r.Budget.StopReason
		r.Stage = stage
		if rule >= 0 {
			r.StageRule = &rule
		}
		if rule2 >= 0 {
			r.StageRule2 = &rule2
		}
		return r, nil
	}

	// Pass 1 — effective-region emptiness. E_i = raw_i minus the union
	// of the raw regions of all earlier rules; the streaming carve
	// decides emptiness without materializing E_i, exiting at the
	// first surviving piece. Raw regions are retained for later
	// passes; the covered index accumulates them in order.
	raws := make([]packetset.Region, n)
	unreachable := make([]bool, n)
	covered := packetset.NewIndex()
	stored := 0
	for i, rule := range acl.Rules {
		if err := b.SetReserved(stored + covered.Len()); err != nil {
			return incomplete(AuditStageEffective, i, -1, err)
		}
		raw, err := ruleRegion(rule, b)
		if err != nil {
			return incomplete(AuditStageEffective, i, -1, err)
		}
		raws[i] = raw
		survives := false
		err = streamDifference(raw, covered, b, func(packetset.Box) error {
			survives = true
			return errStreamStop
		})
		if err != nil && !errors.Is(err, errStreamStop) {
			return incomplete(AuditStageEffective, i, -1, err)
		}
		if !survives {
			unreachable[i] = true
			f := AuditFinding{Kind: AuditUnreachable, Rule: rule.Ordinal, Line: rule.Line, Action: rule.Action, MatchBoxes: len(raw), EffectiveBoxes: 0}
			if len(raw) > 0 {
				if err := b.Tick(len(acl.Rules) + 1); err != nil {
					return incomplete(AuditStageEffective, i, -1, err)
				}
				w, err := auditWitness(raw[0], acl, "unreachable")
				if err != nil {
					return AuditReport{}, err
				}
				if w.Decision.Implicit || w.Decision.RuleOrdinal == nil || *w.Decision.RuleOrdinal >= i {
					return AuditReport{}, errors.New("unreachable witness failed independent evaluation")
				}
				f.Witness = &w
			}
			r.Findings = append(r.Findings, f)
		}
		if err := covered.Add(raw, b); err != nil {
			return incomplete(AuditStageEffective, i, -1, err)
		}
		stored += len(raw)
	}

	// Pass 2 — redundancy. For each rule pass 1 found reachable,
	// stream its effective region piece by piece and decide each piece
	// against the later rules in first-match order — the exact
	// post-removal semantics. One piece decided by a different action
	// (or, for a permit rule, left to the implicit deny) settles the
	// rule as not redundant; the prefix index of earlier rules is
	// rebuilt per rule from the retained raw regions.
	if err := b.SetReserved(stored); err != nil {
		return incomplete(AuditStageRedundancy, 0, -1, err)
	}
	for i, rule := range acl.Rules {
		if unreachable[i] {
			continue
		}
		earlier := packetset.NewIndex()
		for k := 0; k < i; k++ {
			if err := earlier.Add(raws[k], b); err != nil {
				return incomplete(AuditStageRedundancy, i, -1, err)
			}
		}
		probe := &redundancyProbe{acl: acl, raws: raws, rule: i, b: b, coveredBy: map[int]bool{}, stageJ: -1}
		err := streamDifference(raws[i], earlier, b, func(piece packetset.Box) error {
			probe.pieces++
			return probe.decide(piece)
		})
		if err != nil && !errors.Is(err, errStreamStop) {
			return incomplete(AuditStageRedundancy, i, probe.stageJ, err)
		}
		if probe.failed {
			continue
		}
		coveredBy := make([]int, 0, len(probe.coveredBy))
		for j := range probe.coveredBy {
			coveredBy = append(coveredBy, j)
		}
		slices.Sort(coveredBy)
		r.Findings = append(r.Findings, AuditFinding{Kind: AuditRedundant, Rule: rule.Ordinal, Line: rule.Line, Action: rule.Action, MatchBoxes: len(raws[i]), EffectiveBoxes: probe.pieces, CoveredBy: coveredBy, ImplicitDeny: probe.implicit})
	}
	sortFindings(r.Findings)

	// Pass 3 — pairwise relations (heuristic layer). Candidate pairs
	// come from a rules index over all raw boxes; each candidate is
	// then decided with exact budgeted region operations.
	idx := packetset.NewIndex()
	owner := []int{}
	for i, raw := range raws {
		if err := idx.Add(raw, b); err != nil {
			return incomplete(AuditStageRelations, i, -1, err)
		}
		for range raw {
			owner = append(owner, i)
		}
	}
	type pair struct{ i, j int }
	candidates := map[pair]bool{}
	for i := 0; i < n; i++ {
		for _, box := range raws[i] {
			ids, err := idx.Candidates(box, b)
			if err != nil {
				return incomplete(AuditStageRelations, i, -1, err)
			}
			for _, id := range ids {
				if owner[id] > i {
					candidates[pair{i, owner[id]}] = true
				}
			}
		}
	}
	pairs := make([]pair, 0, len(candidates))
	for p := range candidates {
		pairs = append(pairs, p)
	}
	slices.SortFunc(pairs, func(x, y pair) int {
		if x.i != y.i {
			return x.i - y.i
		}
		return x.j - y.j
	})
	var witnessIndex *packetset.Index
	witnessIndexFor := -1
	for _, p := range pairs {
		ei, ej := acl.Rules[p.i], acl.Rules[p.j]
		if ei.Action == ej.Action {
			continue
		}
		inter, err := packetset.Intersection(raws[p.i], raws[p.j], b)
		if err != nil {
			return incomplete(AuditStageRelations, p.i, p.j, err)
		}
		if len(inter) == 0 {
			continue
		}
		di, err := packetset.Difference(raws[p.i], raws[p.j], b)
		if err != nil {
			return incomplete(AuditStageRelations, p.i, p.j, err)
		}
		dj, err := packetset.Difference(raws[p.j], raws[p.i], b)
		if err != nil {
			return incomplete(AuditStageRelations, p.i, p.j, err)
		}
		var kind string
		switch {
		case len(di) == 0 && len(dj) == 0:
			// Equal match sets with different actions: the later rule
			// is already reported unreachable; no relation is claimed.
			continue
		case len(di) == 0:
			kind = AuditGeneralization
		case len(dj) == 0:
			// The earlier rule's match covers the later rule's entire
			// match: shadowing territory, covered by findings.
			continue
		default:
			kind = AuditCorrelation
		}
		if err := b.Tick(len(acl.Rules) + 1); err != nil {
			return incomplete(AuditStageRelations, p.i, p.j, err)
		}
		// Prefer a witness the earlier rule itself decides: the first
		// effective piece of the earlier rule that reaches the later
		// rule's match. Fall back to the plain match intersection.
		if witnessIndexFor != p.i {
			witnessIndex = packetset.NewIndex()
			for k := 0; k < p.i; k++ {
				if err := witnessIndex.Add(raws[k], b); err != nil {
					return incomplete(AuditStageRelations, p.i, p.j, err)
				}
			}
			witnessIndexFor = p.i
		}
		box := inter[0]
		err = streamDifference(raws[p.i], witnessIndex, b, func(piece packetset.Box) error {
			hit, err := packetset.Intersection(packetset.Region{piece}, raws[p.j], b)
			if err != nil {
				return err
			}
			if len(hit) > 0 {
				box = hit[0]
				return errStreamStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStreamStop) {
			return incomplete(AuditStageRelations, p.i, p.j, err)
		}
		w, err := auditWitness(box, acl, kind)
		if err != nil {
			return AuditReport{}, err
		}
		if !regionHasPacket(raws[p.i], w.Packet) || !regionHasPacket(raws[p.j], w.Packet) {
			return AuditReport{}, errors.New("relation witness outside rule matches")
		}
		r.Relations = append(r.Relations, AuditRelation{Kind: kind, Earlier: p.i, Later: p.j, EarlierAction: ei.Action, LaterAction: ej.Action, Witness: &w})
	}
	r.RelationsComplete = true
	r.Complete = true
	r.Budget = b.Stats()
	return r, nil
}

// errStreamStop aborts a streamDifference walk early: the caller has
// the answer it streamed for. It is never a budget stop and never
// leaves the package.
var errStreamStop = errors.New("audit stream stopped by caller")

// streamDifference emits, one at a time and in a fixed order, the
// pieces of raw that survive subtracting every cut in the index —
// exactly raw minus the union of the indexed regions, as a disjoint
// decomposition — without materializing the whole difference. The
// walk is depth-first over Subtract splits: the stack holds only the
// pieces along the current carve path, and each piece is decided
// against the index's candidate cuts for that piece (the same
// candidate mechanism decideAgainst uses: cuts outside the candidate
// set are disjoint from the piece). Budget accounting rides on the
// primitives: Candidates and Subtract tick, and queued pieces are
// box-checked as they are stacked.
func streamDifference(raw packetset.Region, cuts *packetset.Index, b *packetset.Budget, emit func(packetset.Box) error) error {
	for _, box := range raw {
		stack := []packetset.Box{box}
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			ids, err := cuts.Candidates(p, b)
			if err != nil {
				return err
			}
			var hit *packetset.Box
			for _, id := range ids {
				c := cuts.Box(id)
				if _, ok := packetset.Intersect(p, c); ok {
					hit = &c
					break
				}
			}
			if hit == nil {
				if err := emit(p); err != nil {
					return err
				}
				continue
			}
			if err := packetset.Subtract(p, *hit, b, func(rest packetset.Box) error {
				stack = append(stack, rest)
				return b.CheckBoxes(len(stack) + 1)
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// redundancyProbe decides the effective pieces of one rule against
// the later rules, streaming. failed records the first piece whose
// post-removal decision differs from the rule's action (or falls to
// the implicit deny for a permit rule); coveredBy collects the later
// rules that decide at least one piece with the rule's action;
// implicit records that at least one piece falls through to the
// implicit deny (possible only for a deny rule); pieces counts the
// streamed effective pieces.
type redundancyProbe struct {
	acl       ciscoacl.ACL
	raws      []packetset.Region
	rule      int
	b         *packetset.Budget
	pieces    int
	failed    bool
	implicit  bool
	coveredBy map[int]bool
	stageJ    int
}

func (p *redundancyProbe) decide(piece packetset.Box) error {
	action := p.acl.Rules[p.rule].Action
	work := []packetset.Box{piece}
	for j := p.rule + 1; j < len(p.acl.Rules) && len(work) > 0; j++ {
		p.stageJ = j
		next := []packetset.Box{}
		for _, w := range work {
			hits := false
			for _, rb := range p.raws[j] {
				if _, ok := packetset.Intersect(w, rb); ok {
					hits = true
					break
				}
			}
			if !hits {
				next = append(next, w)
				continue
			}
			if p.acl.Rules[j].Action != action {
				p.failed = true
				return errStreamStop
			}
			p.coveredBy[j] = true
			pieces := []packetset.Box{w}
			for _, rb := range p.raws[j] {
				carved := []packetset.Box{}
				for _, pc := range pieces {
					if err := packetset.Subtract(pc, rb, p.b, func(rest packetset.Box) error {
						carved = append(carved, rest)
						return p.b.CheckBoxes(len(carved) + len(pieces) + 1)
					}); err != nil {
						return err
					}
				}
				pieces = carved
			}
			next = append(next, pieces...)
		}
		work = next
	}
	if len(work) > 0 {
		if action != "deny" {
			p.failed = true
			return errStreamStop
		}
		p.implicit = true
	}
	return nil
}

// auditWitness builds the demonstrating packet for one box and records
// the decision the linear evaluator actually gives it.
func auditWitness(box packetset.Box, acl ciscoacl.ACL, label string) (AuditWitness, error) {
	proto, src, dst, sport, dport, ok := box.Witness()
	if !ok {
		return AuditWitness{}, errors.New("no valid packet in audit region")
	}
	p := ciscoacl.Packet{Protocol: proto, SrcIP: address(src), DstIP: address(dst), SrcPort: sport, DstPort: dport}
	if !packetset.Contains(box, proto, src, dst, sport, dport) {
		return AuditWitness{}, errors.New("audit witness outside its region")
	}
	d, err := acl.Evaluate(p)
	if err != nil {
		return AuditWitness{}, err
	}
	w := AuditWitness{Packet: p, Decision: d}
	encoded, err := json.Marshal(struct {
		Label   string
		Witness AuditWitness
	}{label, w})
	if err != nil {
		return AuditWitness{}, err
	}
	sum := sha256.Sum256(encoded)
	w.ID = "wit-" + hex.EncodeToString(sum[:])
	return w, nil
}

// regionHasPacket reports whether the packet lies in any box of the
// region, using the same per-box containment the witness checks use.
func regionHasPacket(r packetset.Region, p ciscoacl.Packet) bool {
	src := p.SrcIP.As4()
	dst := p.DstIP.As4()
	s := uint32(src[0])<<24 | uint32(src[1])<<16 | uint32(src[2])<<8 | uint32(src[3])
	d := uint32(dst[0])<<24 | uint32(dst[1])<<16 | uint32(dst[2])<<8 | uint32(dst[3])
	for _, box := range r {
		if packetset.Contains(box, p.Protocol, s, d, p.SrcPort, p.DstPort) {
			return true
		}
	}
	return false
}

// sortFindings orders findings for display: rules with an empty
// effective region (unreachable) first, then redundant rules, each in
// ascending rule order.
func sortFindings(fs []AuditFinding) {
	rank := func(k string) int {
		if k == AuditUnreachable {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(fs, func(x, y AuditFinding) int {
		if rank(x.Kind) != rank(y.Kind) {
			return rank(x.Kind) - rank(y.Kind)
		}
		return x.Rule - y.Rule
	})
}

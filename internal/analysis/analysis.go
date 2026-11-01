package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
	"plenilune/internal/prefixlist"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
)

const (
	RequestSchema = "plenilune.analysis-request/v1"
	Schema        = "plenilune.analysis/v2"
	// SchemaV1 is the frozen pre-redaction artifact schema. New analysis is
	// never emitted as v1; v1 artifacts remain replayable from their original
	// sources (see Replay).
	SchemaV1            = "plenilune.analysis/v1"
	ReplayRequestSchema = "plenilune.analysis-replay-request/v1"
	// MaxAnalyzed is the default cap on behavior-analyzed objects per
	// request; Options.MaxAnalyzed overrides it within 1..MaxAnalyzedLimit.
	MaxAnalyzed = 16
	// MaxAnalyzedLimit bounds the max_analyzed option.
	MaxAnalyzedLimit = 64
	// MaxWorkers bounds the worker pool of one analysis run.
	MaxWorkers      = 64
	MaxSelect       = 64
	MaxPolicies     = 16
	DefaultDeadline = 30 * time.Second
	MaxDeadline     = 120 * time.Second
	maxUnanalyzed   = 100
	MaxResultBytes  = 16 << 20
)

var refPattern = regexp.MustCompile(`^[\x20-\x7e]{0,256}$`)

func digest(b []byte) string { return evidence.ConfigDigest(b) }

// Error is a typed request or input failure. No artifact is produced with it.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func invalid(code, msg string) error { return &Error{Code: code, Message: msg} }

// Source is one configuration version. Ref is an opaque caller label echoed
// back for correlation; it is not authenticated and not part of identity.
type Source struct {
	Text string `json:"text"`
	Ref  string `json:"ref,omitempty"`
}

type ObjectRef struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
}

// ObjectPolicy supplies an operator-accepted ACL policy whose allowed-change
// regions classify each change class as inside or outside the declared scope.
type ObjectPolicy struct {
	Domain string        `json:"domain"`
	Name   string        `json:"name"`
	Policy repair.Policy `json:"policy"`
}

type Options struct {
	ACL        verify.Options `json:"acl"`
	MaxClasses int            `json:"max_classes"`
	DeadlineMS int            `json:"deadline_ms"`
	// MaxAnalyzed caps behavior-analyzed objects per request (1..64).
	// Zero means the default (16) and is how pre-P1-5 artifacts record it:
	// the field is absent from their options echo.
	MaxAnalyzed int `json:"max_analyzed,omitempty"`
}

func DefaultOptions() Options {
	return Options{ACL: verify.DefaultOptions(), MaxClasses: verify.DefaultMaxClasses, DeadlineMS: int(DefaultDeadline / time.Millisecond), MaxAnalyzed: MaxAnalyzed}
}

// maxAnalyzed resolves the option: 0 is unset (the default), never a cap
// of zero objects.
func (o Options) maxAnalyzed() int {
	if o.MaxAnalyzed <= 0 {
		return MaxAnalyzed
	}
	return o.MaxAnalyzed
}

// ExecOptions controls how an analysis executes, never what it produces.
// The sealed artifact is byte-identical for any worker count, so the
// worker count is deliberately not part of the request or the artifact.
// Workers <= 0 selects DefaultExecOptions.
type ExecOptions struct {
	Workers int
}

// DefaultExecOptions is the plan's default pool: min(4, GOMAXPROCS).
func DefaultExecOptions() ExecOptions {
	w := runtime.GOMAXPROCS(0)
	if w > 4 {
		w = 4
	}
	if w < 1 {
		w = 1
	}
	return ExecOptions{Workers: w}
}

func (e ExecOptions) workers() int {
	if e.Workers <= 0 {
		return DefaultExecOptions().Workers
	}
	if e.Workers > MaxWorkers {
		return MaxWorkers
	}
	return e.Workers
}

type Request struct {
	Schema   string         `json:"schema"`
	Before   Source         `json:"before"`
	After    Source         `json:"after"`
	Select   []ObjectRef    `json:"select,omitempty"`
	Policies []ObjectPolicy `json:"policies,omitempty"`
	Options  *Options       `json:"options,omitempty"`
}

type SourceInfo struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Lines  int    `json:"lines"`
	Ref    string `json:"ref,omitempty"`
}

type Coverage struct {
	Lines            int      `json:"lines"`
	NoiseLines       int      `json:"noise_lines"`
	AnalyzedLines    int      `json:"analyzed_lines"`
	StructuralLines  int      `json:"structural_lines"`
	UnsupportedLines int      `json:"unsupported_lines"`
	UnanalyzedLines  int      `json:"unanalyzed_lines"`
	Sections         []Count  `json:"unanalyzed_sections"`
	Warnings         []string `json:"warnings"`
}

type Count struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// SectionChange is one structural change in an unanalyzed section. The
// current (v2) shape never carries raw configuration text: HeaderKind keeps
// at most the first two header tokens, Lines counts the section's lines and
// SectionSHA256 binds the exact section text so a holder of the sources can
// verify which section changed. For a modified section Lines counts the
// after section and SectionSHA256 binds before-text, a NUL separator and
// after-text. Header is the frozen v1 field, populated only when reproducing
// a v1 artifact for replay.
type SectionChange struct {
	Kind          string `json:"kind"`
	Header        string `json:"header,omitempty"`
	HeaderKind    string `json:"header_kind,omitempty"`
	Lines         int    `json:"lines,omitempty"`
	SectionSHA256 string `json:"section_sha256,omitempty"`
	Change        string `json:"change"`
	BeforeLine    int    `json:"before_line,omitempty"`
	AfterLine     int    `json:"after_line,omitempty"`
}

// headerKind keeps at most the first two tokens of a section header, e.g.
// "interface GigabitEthernet0/1" or "snmp-server community". Anything after
// the second token (community strings, secret values, hashes) is dropped.
func headerKind(text string) string {
	f := fields(text)
	if len(f) > 2 {
		f = f[:2]
	}
	return strings.Join(f, " ")
}

type ACLAnalysis struct {
	Engine         string              `json:"engine"`
	Verdict        string              `json:"verdict"`
	SearchComplete bool                `json:"search_complete"`
	Reason         string              `json:"reason,omitempty"`
	Witnesses      []verify.Witness    `json:"witnesses"`
	Truncated      bool                `json:"witnesses_truncated"`
	Budget         packetset.Stats     `json:"budget"`
	Changes        verify.ChangeReport `json:"changes"`
	PolicySHA256   string              `json:"policy_sha256,omitempty"`
	// Partial is present only when the comparison's budget stopped the
	// search (engine v0.4+): it separates the rule pairs already decided
	// from the undecided remainder. The changes report carries its own
	// partial for the explanation stage.
	Partial *verify.PartialResult `json:"partial,omitempty"`
}

type PrefixListAnalysis struct {
	Parser string            `json:"parser"`
	Engine string            `json:"engine"`
	Report prefixlist.Report `json:"report"`
}

type Object struct {
	Domain     string                 `json:"domain"`
	Name       string                 `json:"name"`
	Kind       string                 `json:"kind"`
	Status     string                 `json:"status"`
	Reason     string                 `json:"reason,omitempty"`
	Before     *Extent                `json:"before,omitempty"`
	After      *Extent                `json:"after,omitempty"`
	References map[string][]Reference `json:"references"`
	Errors     []ObjectError          `json:"errors,omitempty"`
	ACL        *ACLAnalysis           `json:"acl,omitempty"`
	PrefixList *PrefixListAnalysis    `json:"prefix_list,omitempty"`
}

// GlobalBudget records the analysis-wide budget ledger when it
// constrained the run: the caps are the request's ACL limits applied to
// the whole analysis instead of per object, and Operations/Boxes are the
// totals charged by the recorded objects (per object: compare + changes
// operations; the larger of the two stage peak boxes — an object's stages
// run in sequence, so that is its live-box footprint). The field is
// absent whenever the ledger never bound, so unconstrained artifacts are
// byte-identical to pre-P1-5 artifacts.
type GlobalBudget struct {
	MaxOperations int `json:"max_operations"`
	MaxBoxes      int `json:"max_boxes"`
	Operations    int `json:"operations"`
	Boxes         int `json:"boxes"`
}

type Summary struct {
	Objects           int `json:"objects"`
	Changed           int `json:"changed"`
	Equivalent        int `json:"equivalent"`
	Unchanged         int `json:"unchanged"`
	Added             int `json:"added"`
	Removed           int `json:"removed"`
	Unsupported       int `json:"unsupported"`
	Inconclusive      int `json:"inconclusive"`
	NotAnalyzed       int `json:"not_analyzed"`
	UnanalyzedChanges int `json:"unanalyzed_changes"`
}

type PolicyEcho struct {
	Domain       string        `json:"domain"`
	Name         string        `json:"name"`
	PolicySHA256 string        `json:"policy_sha256"`
	Policy       repair.Policy `json:"policy"`
}

type RequestEcho struct {
	Select   []ObjectRef  `json:"select"`
	Policies []PolicyEcho `json:"policies"`
	Options  Options      `json:"options"`
}

type Versions struct {
	Intake           string `json:"intake"`
	ACLParser        string `json:"acl_parser"`
	ACLEngine        string `json:"acl_engine"`
	PrefixListParser string `json:"prefix_list_parser"`
	PrefixListEngine string `json:"prefix_list_engine"`
}

// Result is the sealed review artifact. It describes behavior only for
// analyzed objects; everything else is listed as structural or unanalyzed.
type Result struct {
	Schema              string                `json:"schema"`
	Tool                evidence.Tool         `json:"tool"`
	Versions            Versions              `json:"versions"`
	Request             RequestEcho           `json:"request"`
	Sources             map[string]SourceInfo `json:"sources"`
	Objects             []Object              `json:"objects"`
	Coverage            map[string]Coverage   `json:"coverage"`
	UnanalyzedChanges   []SectionChange       `json:"unanalyzed_changes"`
	UnanalyzedTruncated bool                  `json:"unanalyzed_changes_truncated"`
	Summary             Summary               `json:"summary"`
	GlobalBudget        *GlobalBudget         `json:"global_budget,omitempty"`
	Limitations         []string              `json:"limitations"`
	SHA256              string                `json:"sha256,omitempty"`
}

func currentVersions() Versions {
	return Versions{Intake: IntakeVersion, ACLParser: ciscoacl.ParserVersion, ACLEngine: verify.EngineVersion, PrefixListParser: prefixlist.ParserVersion, PrefixListEngine: prefixlist.EngineVersion}
}

func validateRequest(req Request) (Options, []PolicyEcho, error) {
	if req.Schema != RequestSchema {
		return Options{}, nil, invalid("REQUEST_SCHEMA", "expected "+RequestSchema)
	}
	for _, s := range []Source{req.Before, req.After} {
		if len(s.Text) > MaxSourceBytes {
			return Options{}, nil, invalid("INPUT_TOO_LARGE", "configuration exceeds 4 MiB")
		}
		if !validText([]byte(s.Text)) {
			return Options{}, nil, invalid("INPUT_ENCODING", "configuration must be UTF-8")
		}
		if !refPattern.MatchString(s.Ref) {
			return Options{}, nil, invalid("SOURCE_REF_INVALID", "ref must be at most 256 printable ASCII characters")
		}
		if countLines([]byte(s.Text)) > MaxSourceLines {
			return Options{}, nil, invalid("INPUT_TOO_LARGE", "configuration exceeds 200000 lines")
		}
	}
	if len(req.Select) > MaxSelect || len(req.Policies) > MaxPolicies {
		return Options{}, nil, invalid("REQUEST_LIMIT", "too many selected objects or policies")
	}
	for _, s := range req.Select {
		if s.Domain != DomainACL && s.Domain != DomainPrefixList || s.Name == "" || len(s.Name) > 128 {
			return Options{}, nil, invalid("SELECT_INVALID", "select needs a supported domain and a name")
		}
	}
	opts := DefaultOptions()
	if req.Options != nil {
		opts = *req.Options
	}
	d := packetset.Defaults()
	l := opts.ACL.Limits
	if opts.MaxClasses < 1 || opts.MaxClasses > verify.MaxClasses || opts.ACL.MaxWitnesses < 1 || opts.ACL.MaxWitnesses > 64 || opts.MaxAnalyzed < 0 || opts.MaxAnalyzed > MaxAnalyzedLimit || opts.DeadlineMS < 1 || opts.DeadlineMS > int(MaxDeadline/time.Millisecond) ||
		l.MaxOperations < 1 || l.MaxOperations > d.MaxOperations || l.MaxBoxes < 1 || l.MaxBoxes > d.MaxBoxes || l.Timeout <= 0 || l.Timeout > d.Timeout {
		return Options{}, nil, invalid("OPTIONS_INVALID", "options are outside supported bounds")
	}
	policies := []PolicyEcho{}
	seen := map[string]bool{}
	for _, p := range req.Policies {
		if p.Domain != DomainACL {
			return Options{}, nil, invalid("POLICY_INVALID", "policies are supported for the acl domain only")
		}
		if seen[p.Name] {
			return Options{}, nil, invalid("POLICY_INVALID", "duplicate policy for one object")
		}
		seen[p.Name] = true
		n, err := repair.NormalizePolicy(p.Policy)
		if err != nil {
			return Options{}, nil, invalid("POLICY_INVALID", "invalid policy definitions or scope")
		}
		if !n.OperatorAccepted {
			return Options{}, nil, invalid("POLICY_NOT_ACCEPTED", "scope classification requires an operator-accepted policy")
		}
		h, err := repair.PolicyDigest(n)
		if err != nil {
			return Options{}, nil, invalid("POLICY_INVALID", "invalid policy")
		}
		policies = append(policies, PolicyEcho{Domain: p.Domain, Name: p.Name, PolicySHA256: h, Policy: n})
	}
	return opts, policies, nil
}

// Analyze reviews two configuration versions with the default worker
// pool. A context cancelled before the run starts is an error; a deadline
// or cancellation that lands while objects are being evaluated yields a
// partial artifact with the unfinished objects marked, not an error.
func Analyze(ctx context.Context, req Request) (Result, error) {
	return analyzeVersion(ctx, req, currentVersions())
}

// AnalyzeExec is Analyze with an explicit worker pool size. The pool is
// an execution detail only: the artifact is identical for any count.
func AnalyzeExec(ctx context.Context, req Request, exec ExecOptions) (Result, error) {
	return analyzeSchema(ctx, req, currentVersions(), Schema, exec)
}

func analyzeVersion(ctx context.Context, req Request, versions Versions) (Result, error) {
	return analyzeSchema(ctx, req, versions, Schema, DefaultExecOptions())
}

// analyzeSchema emits the artifact in the given schema shape. The v1 shape
// is produced only to recompute a recorded v1 artifact during replay; new
// analysis always uses Schema.
func analyzeSchema(ctx context.Context, req Request, versions Versions, schema string, exec ExecOptions) (Result, error) {
	legacy := schema == SchemaV1
	opts, policies, err := validateRequest(req)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(opts.DeadlineMS)*time.Millisecond)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Result{}, deadlineError(err)
	}
	selected := req.Select
	if selected == nil {
		selected = []ObjectRef{}
	}
	res := Result{
		Schema: schema, Tool: evidence.Tool{Version: evidence.ToolVersion, Commit: "unversioned"}, Versions: versions,
		Request: RequestEcho{Select: selected, Policies: policies, Options: opts},
		Sources: map[string]SourceInfo{}, Objects: []Object{}, Coverage: map[string]Coverage{},
		UnanalyzedChanges: []SectionChange{}, Limitations: []string{},
	}
	invs := map[string]inventory{}
	for side, s := range map[string]Source{"before": req.Before, "after": req.After} {
		data := []byte(s.Text)
		invs[side] = intakeFormat(data, versions.PrefixListParser, legacy)
		res.Sources[side] = SourceInfo{SHA256: digest(data), Bytes: len(data), Lines: invs[side].lines, Ref: s.Ref}
	}
	before, after := invs["before"], invs["after"]
	order := []*found{}
	pair := map[string][2]*found{}
	for _, inv := range []inventory{before, after} {
		for _, f := range inv.objects {
			k := key(f.domain, f.kind, f.name)
			if _, ok := pair[k]; !ok {
				order = append(order, f)
				pair[k] = [2]*found{before.byKey[k], after.byKey[k]}
			}
		}
	}
	wanted := func(domain, name string) bool {
		if len(req.Select) == 0 {
			return true
		}
		for _, s := range req.Select {
			if s.Domain == domain && s.Name == name {
				return true
			}
		}
		return false
	}
	for _, sel := range req.Select {
		found := false
		for _, f := range order {
			if f.domain == sel.Domain && f.name == sel.Name {
				found = true
				break
			}
		}
		if !found {
			return Result{}, invalid("SELECT_INVALID", "selected object is not present in both configuration inventories")
		}
	}
	policyFor := map[string]PolicyEcho{}
	for _, p := range policies {
		policyFor[p.Name] = p
	}

	// Phase 1 — sequential classification in canonical order. Structural
	// outcomes (errors, added, removed, unchanged, not selected, object
	// limit) are final here; engine-bound objects take a slot. The
	// classification is pure, so the slot assignment is deterministic.
	slots := []*analysisSlot{}
	analyzed := 0
	for _, f := range order {
		sides := pair[key(f.domain, f.kind, f.name)]
		obj, needsEngine := classifyObject(sides[0], sides[1], wanted(f.domain, f.name), analyzed < opts.maxAnalyzed())
		if needsEngine {
			analyzed++
			s := &analysisSlot{index: len(res.Objects), b: sides[0], a: sides[1], base: obj}
			if p, ok := policyFor[f.name]; ok {
				s.policy, s.scoped = p, true
			}
			slots = append(slots, s)
		}
		res.Objects = append(res.Objects, obj)
	}

	// Phase 2 — engine work on a bounded worker pool. Each worker writes
	// only its own slot's outcome; nothing shared is mutated, so the
	// completion order cannot leak into the artifact.
	if len(slots) > 0 {
		jobs := make(chan *analysisSlot)
		var wg sync.WaitGroup
		n := exec.workers()
		if n > len(slots) {
			n = len(slots)
		}
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for s := range jobs {
					s.outcome = runSlot(ctx, s, opts, versions)
				}
			}()
		}
		for _, s := range slots {
			jobs <- s
		}
		close(jobs)
		wg.Wait()
	}

	// Phase 3 — canonical-order assembly under the global budget ledger.
	// The ledger is the request's ACL limits applied to the whole run:
	// each recorded object is charged its operations (compare + changes)
	// and its box footprint (the larger stage peak; an object's stages
	// run in sequence, so their peaks never coexist). An object whose
	// charge fits the remainder is recorded verbatim — the clamping a
	// sequential run would apply never binds for it, so the speculative
	// parallel result is the sequential result. The first object that
	// does not fit is recomputed with the remainder as its stage limits,
	// which is exactly what the sequential run would produce, partial
	// records included. An object whose stages cannot even start under
	// the remainder is not recorded, and neither is any later object:
	// remainders never grow.
	remOps, remBoxes := opts.ACL.Limits.MaxOperations, opts.ACL.Limits.MaxBoxes
	bound, halted := false, false
	for _, s := range slots {
		dst := &res.Objects[s.index]
		switch s.outcome.kind {
		case outcomeFailed:
			return Result{}, s.outcome.err
		case outcomeUnfinished:
			markUnfinished(dst, ctx)
			continue
		}
		obj := s.outcome.obj
		if halted {
			markGlobalBudget(dst)
			continue
		}
		ops, boxes := objectCharge(obj)
		if ops <= remOps && boxes <= remBoxes {
			*dst = obj
			remOps -= ops
			remBoxes -= boxes
			continue
		}
		bound = true
		if obj.Domain != DomainACL || remOps < 1 || remBoxes < 1 {
			// Prefix-list comparisons expose no limits to clamp, and a
			// remainder below one unit starts no stage at all.
			markGlobalBudget(dst)
			halted = true
			continue
		}
		report, err := compareStage(ctx, s, stageLimits(opts.ACL.Limits, remOps, remBoxes), opts, versions)
		if err != nil {
			if ctx.Err() != nil {
				markUnfinished(dst, ctx)
				continue
			}
			return Result{}, invalid("ANALYSIS_FAILED", "ACL comparison failed")
		}
		if remOps-report.Budget.Operations < 1 {
			// The comparison consumed the remainder; the explanation
			// stage cannot start, so the object is not recorded
			// (admission is all-or-nothing per object).
			markGlobalBudget(dst)
			halted = true
			continue
		}
		changes, err := changesStage(ctx, s, stageLimits(opts.ACL.Limits, remOps-report.Budget.Operations, remBoxes), opts, versions)
		if err != nil {
			if ctx.Err() != nil {
				markUnfinished(dst, ctx)
				continue
			}
			return Result{}, invalid("ANALYSIS_FAILED", "ACL change explanation failed")
		}
		obj = aclObject(s, report, changes, versions)
		*dst = obj
		ops, boxes = objectCharge(obj)
		remOps -= ops
		remBoxes -= boxes
	}
	if bound {
		res.GlobalBudget = &GlobalBudget{
			MaxOperations: opts.ACL.Limits.MaxOperations, MaxBoxes: opts.ACL.Limits.MaxBoxes,
			Operations: opts.ACL.Limits.MaxOperations - remOps, Boxes: opts.ACL.Limits.MaxBoxes - remBoxes,
		}
	}

	for _, p := range policies {
		found := false
		for _, o := range res.Objects {
			found = found || o.Domain == p.Domain && o.Name == p.Name && o.Kind == "named-extended"
		}
		if !found {
			return Result{}, invalid("POLICY_INVALID", "policy names an ACL that is not present")
		}
	}
	res.Coverage["before"], res.Coverage["after"] = coverage(before, res.Objects), coverage(after, res.Objects)
	res.UnanalyzedChanges, res.Summary.UnanalyzedChanges = sectionChanges(before.unanalyzed, after.unanalyzed, legacy)
	if len(res.UnanalyzedChanges) > maxUnanalyzed {
		res.UnanalyzedChanges, res.UnanalyzedTruncated = res.UnanalyzedChanges[:maxUnanalyzed], true
	}
	summarize(&res)
	return Seal(res)
}

// analysisSlot is one engine-bound object: its classified base (extents,
// references) plus the inputs and outcome of its engine evaluation.
type analysisSlot struct {
	index   int
	b, a    *found
	base    Object
	policy  PolicyEcho
	scoped  bool
	outcome slotOutcome
}

type outcomeKind int

const (
	outcomeNone outcomeKind = iota
	outcomeDone
	outcomeUnfinished // the run's context ended before or during evaluation
	outcomeFailed
)

type slotOutcome struct {
	kind outcomeKind
	obj  Object
	err  error
}

// runSlot evaluates one slot with the per-object limits. A context that
// ends before or during evaluation makes the outcome unfinished, never a
// failure; genuine engine failures keep their typed error.
func runSlot(ctx context.Context, s *analysisSlot, opts Options, versions Versions) slotOutcome {
	if ctx.Err() != nil {
		return slotOutcome{kind: outcomeUnfinished}
	}
	obj, err := runObjectComparison(ctx, s, opts, versions)
	if err != nil {
		if ctx.Err() != nil {
			return slotOutcome{kind: outcomeUnfinished}
		}
		return slotOutcome{kind: outcomeFailed, err: err}
	}
	return slotOutcome{kind: outcomeDone, obj: obj}
}

// stageLimits clamps the per-object limits to a ledger remainder. The
// per-stage timeout is not part of the ledger and is kept as configured.
func stageLimits(l packetset.Limits, remOps, remBoxes int) packetset.Limits {
	return packetset.Limits{
		MaxOperations: min(l.MaxOperations, remOps),
		MaxBoxes:      min(l.MaxBoxes, remBoxes),
		Timeout:       l.Timeout,
	}
}

// objectCharge is the global-ledger charge of a recorded object (see the
// assembly comment in analyzeSchema). Prefix-list comparisons use their
// own operations counter and no box algebra, so they charge operations
// only.
func objectCharge(o Object) (ops, boxes int) {
	if o.ACL != nil {
		return o.ACL.Budget.Operations + o.ACL.Changes.Budget.Operations,
			max(o.ACL.Budget.PeakBoxes, o.ACL.Changes.Budget.PeakBoxes)
	}
	if o.PrefixList != nil {
		return o.PrefixList.Report.Operations, 0
	}
	return 0, 0
}

// markUnfinished records an object the run's context cut off. The object
// carries no engine payload; the reason separates a deadline from a
// caller cancellation.
func markUnfinished(o *Object, ctx context.Context) {
	o.Status = "not_analyzed"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		o.Reason = "deadline_exceeded"
	} else {
		o.Reason = "canceled"
	}
}

// markGlobalBudget records an object the global budget ledger did not
// admit: it was never begun under the ledger's sequential accounting.
func markGlobalBudget(o *Object) {
	o.Status, o.Reason = "not_analyzed", "global_budget"
}

func deadlineError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return invalid("DEADLINE_EXCEEDED", "analysis did not finish before the request deadline")
	}
	return invalid("CANCELED", "analysis canceled")
}

func extentOf(f *found) *Extent {
	if f == nil {
		return nil
	}
	e := f.extent
	return &e
}

// classifyObject builds an object's base record (extents, references,
// intake errors) and settles every outcome that needs no engine run, in
// the historical precedence: unsupported, added, removed, unchanged, not
// selected, object limit. It reports whether the object is engine-bound;
// slotAvailable says the max_analyzed cap has room for it.
func classifyObject(b, a *found, wanted, slotAvailable bool) (Object, bool) {
	any := b
	if any == nil {
		any = a
	}
	obj := Object{Domain: any.domain, Name: any.name, Kind: any.kind, Before: extentOf(b), After: extentOf(a), References: map[string][]Reference{"before": {}, "after": {}}}
	if b != nil {
		obj.References["before"] = append(obj.References["before"], b.refs...)
	}
	if a != nil {
		obj.References["after"] = append(obj.References["after"], a.refs...)
	}
	for side, f := range map[string]*found{"before": b, "after": a} {
		if f != nil && f.err != nil {
			e := *f.err
			e.Side = side
			obj.Errors = append(obj.Errors, e)
		}
	}
	if len(obj.Errors) == 2 && obj.Errors[0].Side == "after" {
		obj.Errors[0], obj.Errors[1] = obj.Errors[1], obj.Errors[0]
	}
	switch {
	case len(obj.Errors) > 0:
		obj.Status = "unsupported"
	case b == nil:
		obj.Status = "added"
	case a == nil:
		obj.Status = "removed"
	case b.canonical == a.canonical:
		obj.Status = "unchanged"
	case !wanted:
		obj.Status, obj.Reason = "not_analyzed", "not_selected"
	case !slotAvailable:
		obj.Status, obj.Reason = "not_analyzed", "object_limit"
	default:
		return obj, true
	}
	return obj, false
}

// runObjectComparison evaluates one slot with the per-object limits (the
// speculative parallel path). Errors from a live context are typed
// failures; errors racing a dead context are returned raw for the caller
// to classify as unfinished.
func runObjectComparison(ctx context.Context, s *analysisSlot, opts Options, versions Versions) (Object, error) {
	switch s.base.Domain {
	case DomainACL:
		report, err := compareStage(ctx, s, opts.ACL.Limits, opts, versions)
		if err != nil {
			if ctx.Err() != nil {
				return Object{}, err
			}
			return Object{}, invalid("ANALYSIS_FAILED", "ACL comparison failed")
		}
		changes, err := changesStage(ctx, s, opts.ACL.Limits, opts, versions)
		if err != nil {
			if ctx.Err() != nil {
				return Object{}, err
			}
			return Object{}, invalid("ANALYSIS_FAILED", "ACL change explanation failed")
		}
		return aclObject(s, report, changes, versions), nil
	case DomainPrefixList:
		report, err := prefixlist.CompareVersion(ctx, *s.b.prefix, *s.a.prefix, opts.MaxClasses, versions.PrefixListEngine)
		if err != nil {
			if ctx.Err() != nil {
				return Object{}, err
			}
			return Object{}, invalid("ANALYSIS_FAILED", "prefix-list comparison failed")
		}
		obj := s.base
		obj.PrefixList = &PrefixListAnalysis{Parser: versions.PrefixListParser, Engine: versions.PrefixListEngine, Report: report}
		obj.Status = statusFor(report.Verdict)
		return obj, nil
	}
	return Object{}, invalid("ANALYSIS_FAILED", "unsupported object domain")
}

func compareStage(ctx context.Context, s *analysisSlot, limits packetset.Limits, opts Options, versions Versions) (verify.Report, error) {
	return verify.CompareEngine(ctx, *s.b.acl, *s.a.acl, nil, verify.Options{Limits: limits, MaxWitnesses: opts.ACL.MaxWitnesses}, versions.ACLEngine)
}

func changesStage(ctx context.Context, s *analysisSlot, limits packetset.Limits, opts Options, versions Versions) (verify.ChangeReport, error) {
	var allowed []intent.Region
	if s.scoped {
		allowed = s.policy.Policy.AllowedChanges
	}
	return verify.ChangesEngine(ctx, *s.b.acl, *s.a.acl, allowed, verify.ChangeOptions{Limits: limits, MaxClasses: opts.MaxClasses}, versions.ACLEngine)
}

// aclObject assembles a completed ACL object from its two stage results.
func aclObject(s *analysisSlot, report verify.Report, changes verify.ChangeReport, versions Versions) Object {
	obj := s.base
	obj.ACL = &ACLAnalysis{Engine: versions.ACLEngine, Verdict: report.Verdict, SearchComplete: report.SearchComplete, Reason: report.Reason, Witnesses: report.Witnesses, Truncated: report.WitnessesTruncated, Budget: report.Budget, Changes: changes, Partial: report.Partial}
	if s.scoped {
		obj.ACL.PolicySHA256 = s.policy.PolicySHA256
	}
	obj.Status = statusFor(report.Verdict)
	return obj
}

func statusFor(verdict string) string {
	switch verdict {
	case "DIFFERENT":
		return "changed"
	case "EQUIVALENT":
		return "equivalent"
	}
	return "inconclusive"
}

func coverage(inv inventory, reviewed []Object) Coverage {
	c := Coverage{Lines: inv.lines, NoiseLines: inv.noise, Sections: []Count{}, Warnings: []string{}}
	status := map[string]string{}
	for _, o := range reviewed {
		status[key(o.Domain, o.Kind, o.Name)] = o.Status
	}
	for _, o := range inv.objects {
		switch {
		case o.err != nil:
			c.UnsupportedLines += o.lineCount
		case status[key(o.domain, o.kind, o.name)] == "not_analyzed":
			c.UnanalyzedLines += o.lineCount
		case status[key(o.domain, o.kind, o.name)] == "added" || status[key(o.domain, o.kind, o.name)] == "removed":
			c.StructuralLines += o.lineCount
		default:
			c.AnalyzedLines += o.lineCount
		}
	}
	kinds := map[string]int{}
	order := []string{}
	for _, s := range inv.unanalyzed {
		n := len(s.lines())
		c.UnanalyzedLines += n
		k := sectionKind(fields(s.header.text))
		if kinds[k] == 0 {
			order = append(order, k)
		}
		kinds[k]++
	}
	c.UnanalyzedLines += len(inv.orphans)
	for _, k := range order {
		c.Sections = append(c.Sections, Count{Kind: k, Count: kinds[k]})
	}
	if inv.bom {
		c.Warnings = append(c.Warnings, "byte_order_mark_present")
	}
	if len(inv.orphans) > 0 {
		c.Warnings = append(c.Warnings, "indented_lines_without_section")
	}
	return c
}

// sectionChanges compares unanalyzed sections by header text. This is a
// structural text comparison and makes no behavioral claim. The current
// shape redacts the header (see SectionChange); legacy reproduces the v1
// shape with the raw header text for replay of v1 artifacts.
func sectionChanges(before, after []section, legacy bool) ([]SectionChange, int) {
	index := func(ss []section) (map[string]section, []string) {
		m, order := map[string]section{}, []string{}
		seen := map[string]int{}
		for _, s := range ss {
			h := s.header.text
			seen[h]++
			k := h
			if seen[h] > 1 {
				k = h + "\x00" + string(rune('0'+seen[h]))
			}
			m[k] = s
			order = append(order, k)
		}
		return m, order
	}
	shape := func(sc SectionChange, b, a *section) SectionChange {
		if legacy {
			sc.Header = b.header.text
			return sc
		}
		sc.HeaderKind = headerKind(b.header.text)
		switch sc.Change {
		case "removed":
			sc.Lines = len(b.lines())
			sc.SectionSHA256 = digest([]byte(b.text()))
		case "added":
			sc.Lines = len(a.lines())
			sc.SectionSHA256 = digest([]byte(a.text()))
		default: // modified: bind both sides, count the after section
			sc.Lines = len(a.lines())
			sc.SectionSHA256 = digest([]byte(b.text() + "\x00" + a.text()))
		}
		return sc
	}
	bm, border := index(before)
	am, aorder := index(after)
	out := []SectionChange{}
	for _, k := range border {
		b := bm[k]
		kind := sectionKind(fields(b.header.text))
		a, ok := am[k]
		switch {
		case !ok:
			out = append(out, shape(SectionChange{Kind: kind, Change: "removed", BeforeLine: b.header.number}, &b, nil))
		case a.text() != b.text():
			out = append(out, shape(SectionChange{Kind: kind, Change: "modified", BeforeLine: b.header.number, AfterLine: a.header.number}, &b, &a))
		}
	}
	for _, k := range aorder {
		if _, ok := bm[k]; !ok {
			a := am[k]
			out = append(out, shape(SectionChange{Kind: sectionKind(fields(a.header.text)), Change: "added", AfterLine: a.header.number}, &a, &a))
		}
	}
	return out, len(out)
}

func summarize(r *Result) {
	s := &r.Summary
	s.Objects = len(r.Objects)
	for _, o := range r.Objects {
		switch o.Status {
		case "changed":
			s.Changed++
		case "equivalent":
			s.Equivalent++
		case "unchanged":
			s.Unchanged++
		case "added":
			s.Added++
		case "removed":
			s.Removed++
		case "unsupported":
			s.Unsupported++
		case "inconclusive":
			s.Inconclusive++
		case "not_analyzed":
			s.NotAnalyzed++
		}
	}
	// Limitations are stated, not inferred away.
	r.Limitations = append(r.Limitations, "behavior_evaluated_per_object_without_binding", "acl_scope_ipv4_unfragmented_or_initial_with_complete_l4", "prefix_list_scope_ipv4_route_prefix_only")
	if r.Coverage["before"].UnanalyzedLines+r.Coverage["after"].UnanalyzedLines > 0 {
		r.Limitations = append(r.Limitations, "unanalyzed_configuration_present")
	}
	if s.Unsupported > 0 {
		r.Limitations = append(r.Limitations, "unsupported_objects_present")
	}
	if s.Added+s.Removed > 0 {
		r.Limitations = append(r.Limitations, "added_or_removed_objects_not_behavior_compared")
	}
	// Interruption and ledger marks are derived from the recorded objects
	// (and the ledger record), never inferred away, so replay recomputes
	// the same list from the same artifact content.
	deadline, canceled := false, false
	for _, o := range r.Objects {
		switch o.Reason {
		case "deadline_exceeded":
			deadline = true
		case "canceled":
			canceled = true
		}
	}
	if deadline {
		r.Limitations = append(r.Limitations, "analysis_deadline_exceeded")
	}
	if canceled {
		r.Limitations = append(r.Limitations, "analysis_canceled")
	}
	if r.GlobalBudget != nil {
		r.Limitations = append(r.Limitations, "global_budget_exhausted")
	}
}

// Seal stamps the digest over the full artifact except the digest field.
func Seal(r Result) (Result, error) {
	r.SHA256 = ""
	b, err := json.Marshal(r)
	if err != nil {
		return Result{}, err
	}
	if len(b) > MaxResultBytes {
		return Result{}, invalid("RESULT_TOO_LARGE", "analysis exceeds the artifact size limit")
	}
	r.SHA256 = digest(b)
	return r, nil
}

type ReplayResult struct {
	Status             string `json:"status"`
	SHA256             string `json:"sha256"`
	RecheckedWitnesses int    `json:"rechecked_witnesses"`
}

// Replay recomputes the artifact from the original configuration bytes and
// the recorded request echo. Any byte, version or result difference fails.
// Artifacts recorded under the frozen v1 schema are recomputed in the v1
// shape, so they stay replayable; new analysis is never emitted as v1.
func Replay(ctx context.Context, before, after []byte, r Result) (ReplayResult, error) {
	claimed := r.SHA256
	sealed, err := Seal(r)
	if err != nil || sealed.SHA256 != claimed || claimed == "" {
		return ReplayResult{}, invalid("EVIDENCE_MISMATCH", "analysis digest does not match its content")
	}
	if r.Schema != Schema && r.Schema != SchemaV1 || !supportedVersions(r.Versions) || r.Tool.Version != evidence.ToolVersion {
		return ReplayResult{}, invalid("EVIDENCE_MISMATCH", "unsupported analysis schema or engine version")
	}
	if r.Sources["before"].SHA256 != digest(before) || r.Sources["after"].SHA256 != digest(after) {
		return ReplayResult{}, invalid("EVIDENCE_MISMATCH", "original configuration bytes do not match")
	}
	req := Request{Schema: RequestSchema, Before: Source{Text: string(before), Ref: r.Sources["before"].Ref}, After: Source{Text: string(after), Ref: r.Sources["after"].Ref}, Select: r.Request.Select, Options: &r.Request.Options}
	if len(req.Select) == 0 {
		req.Select = nil
	}
	for _, p := range r.Request.Policies {
		req.Policies = append(req.Policies, ObjectPolicy{Domain: p.Domain, Name: p.Name, Policy: p.Policy})
	}
	fresh, err := analyzeSchema(ctx, req, r.Versions, r.Schema, DefaultExecOptions())
	if err != nil {
		return ReplayResult{}, err
	}
	rechecked, timing, err := replaySafety(ctx, before, after, r, fresh)
	if err != nil {
		return ReplayResult{}, err
	}
	if timing {
		return ReplayResult{Status: "TIMING_SENSITIVE", SHA256: claimed, RecheckedWitnesses: rechecked}, nil
	}
	if fresh.SHA256 != claimed {
		return ReplayResult{}, invalid("EVIDENCE_MISMATCH", "replayed analysis differs from the receipt")
	}
	return ReplayResult{Status: "ANALYSIS_REPLAYED", SHA256: claimed, RecheckedWitnesses: rechecked}, nil
}

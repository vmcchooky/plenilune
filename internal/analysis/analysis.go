package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
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
	MaxAnalyzed         = 16
	MaxSelect           = 64
	MaxPolicies         = 16
	DefaultDeadline     = 30 * time.Second
	MaxDeadline         = 120 * time.Second
	maxUnanalyzed       = 100
	MaxResultBytes      = 16 << 20
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
}

func DefaultOptions() Options {
	return Options{ACL: verify.DefaultOptions(), MaxClasses: verify.DefaultMaxClasses, DeadlineMS: int(DefaultDeadline / time.Millisecond)}
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
	if opts.MaxClasses < 1 || opts.MaxClasses > verify.MaxClasses || opts.ACL.MaxWitnesses < 1 || opts.ACL.MaxWitnesses > 64 || opts.DeadlineMS < 1 || opts.DeadlineMS > int(MaxDeadline/time.Millisecond) ||
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

// Analyze reviews two configuration versions. Context cancellation or the
// request deadline aborts without a partial artifact.
func Analyze(ctx context.Context, req Request) (Result, error) {
	return analyzeVersion(ctx, req, currentVersions())
}
func analyzeVersion(ctx context.Context, req Request, versions Versions) (Result, error) {
	return analyzeSchema(ctx, req, versions, Schema)
}

// analyzeSchema emits the artifact in the given schema shape. The v1 shape
// is produced only to recompute a recorded v1 artifact during replay; new
// analysis always uses Schema.
func analyzeSchema(ctx context.Context, req Request, versions Versions, schema string) (Result, error) {
	legacy := schema == SchemaV1
	opts, policies, err := validateRequest(req)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(opts.DeadlineMS)*time.Millisecond)
	defer cancel()
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
	analyzed := 0
	for _, f := range order {
		if err := ctx.Err(); err != nil {
			return Result{}, deadlineError(err)
		}
		sides := pair[key(f.domain, f.kind, f.name)]
		obj, err := reviewObject(ctx, sides[0], sides[1], wanted(f.domain, f.name), &analyzed, opts, policyFor, versions)
		if err != nil {
			return Result{}, err
		}
		res.Objects = append(res.Objects, obj)
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
	if err := ctx.Err(); err != nil {
		return Result{}, deadlineError(err)
	}
	return Seal(res)
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

func reviewObject(ctx context.Context, b, a *found, wanted bool, analyzed *int, opts Options, policies map[string]PolicyEcho, versions Versions) (Object, error) {
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
		return obj, nil
	case b == nil:
		obj.Status = "added"
		return obj, nil
	case a == nil:
		obj.Status = "removed"
		return obj, nil
	case b.canonical == a.canonical:
		obj.Status = "unchanged"
		return obj, nil
	case !wanted:
		obj.Status, obj.Reason = "not_analyzed", "not_selected"
		return obj, nil
	case *analyzed >= MaxAnalyzed:
		obj.Status, obj.Reason = "not_analyzed", "object_limit"
		return obj, nil
	}
	*analyzed++
	switch obj.Domain {
	case DomainACL:
		report, err := verify.CompareEngine(ctx, *b.acl, *a.acl, nil, opts.ACL, versions.ACLEngine)
		if err != nil {
			if ctx.Err() != nil {
				return Object{}, deadlineError(ctx.Err())
			}
			return Object{}, invalid("ANALYSIS_FAILED", "ACL comparison failed")
		}
		var allowed []intent.Region
		p, scoped := policies[obj.Name]
		if scoped {
			allowed = p.Policy.AllowedChanges
		}
		changes, err := verify.ChangesEngine(ctx, *b.acl, *a.acl, allowed, verify.ChangeOptions{Limits: opts.ACL.Limits, MaxClasses: opts.MaxClasses}, versions.ACLEngine)
		if err != nil {
			if ctx.Err() != nil {
				return Object{}, deadlineError(ctx.Err())
			}
			return Object{}, invalid("ANALYSIS_FAILED", "ACL change explanation failed")
		}
		obj.ACL = &ACLAnalysis{Engine: versions.ACLEngine, Verdict: report.Verdict, SearchComplete: report.SearchComplete, Reason: report.Reason, Witnesses: report.Witnesses, Truncated: report.WitnessesTruncated, Budget: report.Budget, Changes: changes, Partial: report.Partial}
		if scoped {
			obj.ACL.PolicySHA256 = p.PolicySHA256
		}
		obj.Status = statusFor(report.Verdict)
	case DomainPrefixList:
		report, err := prefixlist.CompareVersion(ctx, *b.prefix, *a.prefix, opts.MaxClasses, versions.PrefixListEngine)
		if err != nil {
			if ctx.Err() != nil {
				return Object{}, deadlineError(ctx.Err())
			}
			return Object{}, invalid("ANALYSIS_FAILED", "prefix-list comparison failed")
		}
		obj.PrefixList = &PrefixListAnalysis{Parser: versions.PrefixListParser, Engine: versions.PrefixListEngine, Report: report}
		obj.Status = statusFor(report.Verdict)
	}
	return obj, nil
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
	fresh, err := analyzeSchema(ctx, req, r.Versions, r.Schema)
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

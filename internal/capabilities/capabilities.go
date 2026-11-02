// Package capabilities builds the single capabilities document
// (plenilune.capabilities/v1) that the CLI `capabilities` command and
// GET /api/capabilities both serve, byte-for-byte the same value from
// this one builder. Callers (e.g. a MoonStorm adapter) read it BEFORE
// sending configuration, to check schema versions, features and limits
// and refuse a job early with a clear reason instead of failing deep in
// an analysis.
//
// Every value is derived from the owning package's own constants and
// defaults at build time. The only curated lists are Features and
// ErrorCodes, and the package's contract tests cross-check those
// against the code (features have implementing packages exercised by
// the build; every published error code must appear as a literal in
// non-test source outside this package). Within v1 the document only
// grows additively: fields, list entries and supported versions may be
// added, never removed or renamed. See CONTRACTS §4.
package capabilities

import (
	"runtime/debug"
	"time"

	"plenilune/internal/agent"
	"plenilune/internal/analysis"
	"plenilune/internal/audit"
	"plenilune/internal/chat"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/evidence"
	"plenilune/internal/packetset"
	"plenilune/internal/prefixlist"
	"plenilune/internal/repair"
	"plenilune/internal/verify"
)

// Schema is the capabilities document's own schema id.
const Schema = "plenilune.capabilities/v1"

// Feature names a caller may require before submitting work.
const (
	FeatureRepairEnumeration = "repair_enumeration"
	FeatureAudit             = "audit"
	FeatureInterrogationChat = "interrogation_chat"
	FeatureAIProposals       = "ai_proposals"
	FeatureCompareCache      = "compare_cache"
	FeaturePartialResults    = "partial_results"
	FeatureDualDigest        = "dual_digest"
)

// Features is the curated feature list for this build. Every entry
// names a capability that exists in the binary today; the contract
// test pins the list so it cannot silently drift from the document.
var Features = []string{
	FeatureAIProposals,
	FeatureAudit,
	FeatureCompareCache,
	FeatureDualDigest,
	FeatureInterrogationChat,
	FeaturePartialResults,
	FeatureRepairEnumeration,
}

// ErrorCodes is the curated list of typed error codes a caller can
// receive from the CLI, the analysis contract or the local HTTP API.
// The contract test verifies each code is emitted as a literal
// somewhere in non-test source outside this package, so the list
// cannot advertise codes nothing produces. Status/outcome strings
// (EQUIVALENT, VERIFIED, ANALYSIS_REPLAYED, …) are not error codes
// and are deliberately absent. Since P2-3 the list is exactly the
// errtax registry set (a test there enforces both directions): every
// published code has a stage/retryable classification, including the
// ACL parser's detail codes, which the parse/trace surfaces emit raw.
var ErrorCodes = []string{
	"ACL_INVALID",
	"AI_CONFIG_INVALID",
	"AI_MODEL_UNSUPPORTED",
	"ANALYSIS_FAILED",
	"AUDIT_INPUT_INVALID",
	"CANCELED",
	"CHAT_ARTIFACT_INVALID",
	"CHAT_INVALID",
	"CHAT_LOCALE_UNSUPPORTED",
	"CHAT_NO_ARTIFACT",
	"CHAT_QUESTION_EMPTY",
	"CHAT_QUESTION_TOO_LONG",
	"COMPARISON_FAILED",
	"COMPARISON_INVALID",
	"CONTENT_TYPE",
	"DEADLINE_EXCEEDED",
	"DUPLICATE_DEFINITION",
	"EVIDENCE_FAILED",
	"EVIDENCE_MISMATCH",
	"INPUT_ENCODING",
	"INPUT_FAILED",
	"INPUT_INVALID",
	"INPUT_READ",
	"INPUT_TOO_LARGE",
	"INVALID_JSON",
	"INVALID_OBLIGATION",
	"INVALID_PACKET",
	"INVALID_PATCH",
	"INVALID_POLICY",
	"INVALID_PROPOSALS",
	"JSON_INVALID",
	"LOCAL_ORIGIN_REQUIRED",
	"OBLIGATIONS_INVALID",
	"OPTIONS_INVALID",
	"OUTPUT_FAILED",
	"PACKET_INVALID",
	"PARSE_DOCUMENT",
	"PARSE_EMPTY_ACL",
	"PARSE_ENCODING",
	"PARSE_FAILED",
	"PARSE_HEADER",
	"PARSE_INVALID_IP",
	"PARSE_INVALID_PORT",
	"PARSE_INVALID_TOKEN",
	"PARSE_LIMIT",
	"PARSE_MULTIPLE_ACLS",
	"PARSE_NOT_FOUND",
	"PARSE_SEQUENCE_ORDER",
	"POLICY_INPUT_INVALID",
	"POLICY_INVALID",
	"POLICY_NOT_ACCEPTED",
	"REPAIR_INPUT_INVALID",
	"REQUEST_LIMIT",
	"REQUEST_SCHEMA",
	"RESULT_TOO_LARGE",
	"ROUTE_INVALID",
	"SELECT_INVALID",
	"SERVER_BUSY",
	"SESSION_INPUT_INVALID",
	"SOURCE_REF_INVALID",
	"TIMING_SENSITIVE",
	"TRACE_INVALID",
	"UI_NOT_BUILT",
	"UNSUPPORTED_KIND",
	"USAGE",
}

type ToolInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

type SchemaSupport struct {
	ID        string   `json:"id"`
	Current   string   `json:"current"`
	Supported []string `json:"supported"`
}

type Domain struct {
	ID            string `json:"id"`
	Description   string `json:"description"`
	ParserVersion string `json:"parser_version"`
	EngineVersion string `json:"engine_version"`
}

// Bound is a numeric option/limit with its default and ceiling.
type Bound struct {
	Default int64 `json:"default"`
	Max     int64 `json:"max"`
}

type ACLLimits struct {
	MaxRules      int   `json:"max_rules"`
	MaxLines      int   `json:"max_lines"`
	MaxInputBytes int   `json:"max_input_bytes"`
	MaxOperations Bound `json:"max_operations"`
	MaxBoxes      Bound `json:"max_boxes"`
	TimeoutNS     Bound `json:"timeout_ns"`
	MaxWitnesses  Bound `json:"max_witnesses"`
}

type PrefixListLimits struct {
	MaxEntries int `json:"max_entries"`
}

type AnalysisLimits struct {
	MaxAnalyzed Bound `json:"max_analyzed"`
	MaxClasses  Bound `json:"max_classes"`
	DeadlineMS  Bound `json:"deadline_ms"`
}

type AISessionLimits struct {
	MaxModelRequests   Bound `json:"max_model_requests"`
	MaxToolCalls       Bound `json:"max_tool_calls"`
	MaxRepairProposals Bound `json:"max_repair_proposals"`
	// MaxOutputCorrections is an output-shape budget, not volume.
	MaxOutputCorrections Bound `json:"max_output_corrections"`
	// SessionDeadlineDefaultMS has no separate ceiling in the agent
	// config validation (any positive value is accepted), so only
	// the default is published — no ceiling is invented here.
	SessionDeadlineDefaultMS int64 `json:"session_deadline_default_ms"`
}

type Limits struct {
	ACL        ACLLimits        `json:"acl"`
	PrefixList PrefixListLimits `json:"prefix_list"`
	Analysis   AnalysisLimits   `json:"analysis"`
	AISession  AISessionLimits  `json:"ai_session"`
}

// Option is one entry of the analysis/compare options surface: the
// request JSON name and the default a caller gets when it is omitted.
type Option struct {
	Name    string `json:"name"`
	Default int64  `json:"default"`
}

type Document struct {
	Schema     string          `json:"schema"`
	Tool       ToolInfo        `json:"tool"`
	Schemas    []SchemaSupport `json:"schemas"`
	Domains    []Domain        `json:"domains"`
	Limits     Limits          `json:"limits"`
	Options    []Option        `json:"options"`
	Features   []string        `json:"features"`
	ErrorCodes []string        `json:"error_codes"`
}

// Build returns the capabilities document for this binary, resolving
// the tool commit from the build info exactly as the CLI version
// command does ("unversioned" when no VCS stamp is present).
func Build() Document {
	return BuildWithTool(evidence.ToolVersion, buildCommit())
}

// BuildWithTool is Build with an explicit tool identity, so tests can
// pin a deterministic document (the snapshot contract test).
func BuildWithTool(version, commit string) Document {
	packetDefaults := packetset.Defaults()
	analysisDefaults := analysis.DefaultOptions()
	return Document{
		Schema: Schema,
		Tool:   ToolInfo{Name: "plenilune", Version: version, Commit: commit},
		Schemas: []SchemaSupport{
			{ID: "plenilune.evidence", Current: evidence.ComparisonSchema, Supported: []string{evidence.ComparisonSchemaV1, evidence.ComparisonSchema}},
			{ID: "plenilune.analysis", Current: analysis.Schema, Supported: []string{analysis.SchemaV1, analysis.SchemaV2, analysis.Schema}},
			{ID: "plenilune.analysis-request", Current: analysis.RequestSchema, Supported: []string{analysis.RequestSchema}},
			{ID: "plenilune.analysis-replay-request", Current: analysis.ReplayRequestSchema, Supported: []string{analysis.ReplayRequestSchema}},
			{ID: "plenilune.repair", Current: repair.Schema, Supported: []string{repair.Schema}},
			{ID: "plenilune.repair-enumeration", Current: repair.EnumerationSchema, Supported: []string{repair.EnumerationSchema}},
			{ID: "plenilune.policy", Current: repair.PolicySchema, Supported: []string{repair.PolicySchema}},
			{ID: "plenilune.audit", Current: audit.Schema, Supported: []string{audit.Schema}},
			{ID: "plenilune.chat", Current: chat.Schema, Supported: []string{chat.Schema}},
			{ID: "plenilune.ai", Current: agent.ResultSchema, Supported: []string{agent.ResultSchema}},
			{ID: "plenilune.trace", Current: evidence.TraceSchema, Supported: []string{evidence.TraceSchema}},
		},
		Domains: []Domain{
			{ID: "acl", Description: "Cisco IOS named extended IPv4 ACL", ParserVersion: ciscoacl.ParserVersion, EngineVersion: verify.EngineVersion},
			{ID: "prefix-list", Description: "Cisco IOS IPv4 prefix-list", ParserVersion: prefixlist.ParserVersion, EngineVersion: prefixlist.EngineVersion},
		},
		Limits: Limits{
			ACL: ACLLimits{
				MaxRules:      ciscoacl.MaxRules,
				MaxLines:      ciscoacl.MaxLines,
				MaxInputBytes: ciscoacl.MaxInputBytes,
				MaxOperations: Bound{Default: int64(packetDefaults.MaxOperations), Max: int64(packetDefaults.MaxOperations)},
				MaxBoxes:      Bound{Default: int64(packetDefaults.MaxBoxes), Max: int64(packetDefaults.MaxBoxes)},
				TimeoutNS:     Bound{Default: int64(packetDefaults.Timeout), Max: int64(packetDefaults.Timeout)},
				MaxWitnesses:  Bound{Default: int64(analysisDefaults.ACL.MaxWitnesses), Max: 64},
			},
			PrefixList: PrefixListLimits{MaxEntries: prefixlist.MaxEntries},
			Analysis: AnalysisLimits{
				MaxAnalyzed: Bound{Default: int64(analysis.MaxAnalyzed), Max: int64(analysis.MaxAnalyzedLimit)},
				MaxClasses:  Bound{Default: int64(verify.DefaultMaxClasses), Max: int64(verify.MaxClasses)},
				DeadlineMS:  Bound{Default: int64(analysis.DefaultDeadline / time.Millisecond), Max: int64(analysis.MaxDeadline / time.Millisecond)},
			},
			AISession: AISessionLimits{
				MaxModelRequests:         Bound{Default: int64(agent.DefaultMaxModelRequests), Max: 32},
				MaxToolCalls:             Bound{Default: int64(agent.DefaultMaxToolCalls), Max: 64},
				MaxRepairProposals:       Bound{Default: int64(agent.DefaultMaxRepairProposals), Max: 16},
				MaxOutputCorrections:     Bound{Default: int64(agent.DefaultMaxOutputCorrections), Max: 8},
				SessionDeadlineDefaultMS: int64(agent.DefaultSessionDeadline / time.Millisecond),
			},
		},
		Options: []Option{
			{Name: "acl.limits.max_operations", Default: int64(analysisDefaults.ACL.Limits.MaxOperations)},
			{Name: "acl.limits.max_boxes", Default: int64(analysisDefaults.ACL.Limits.MaxBoxes)},
			{Name: "acl.limits.timeout_ns", Default: int64(analysisDefaults.ACL.Limits.Timeout)},
			{Name: "acl.max_witnesses", Default: int64(analysisDefaults.ACL.MaxWitnesses)},
			{Name: "max_classes", Default: int64(analysisDefaults.MaxClasses)},
			{Name: "deadline_ms", Default: int64(analysisDefaults.DeadlineMS)},
			{Name: "max_analyzed", Default: int64(analysisDefaults.MaxAnalyzed)},
		},
		Features:   append([]string(nil), Features...),
		ErrorCodes: append([]string(nil), ErrorCodes...),
	}
}

// buildCommit mirrors the CLI version command's VCS resolution.
func buildCommit() string {
	revision := "unversioned"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
		}
	}
	return revision
}

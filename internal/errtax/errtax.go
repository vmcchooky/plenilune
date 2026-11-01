// Package errtax is the unified error taxonomy (P2-3): the single
// registry that classifies every error code a caller can receive from
// the CLI, the analysis contract or the local HTTP API into a stage
// and a retryable flag, and the shared envelope shape the surfaces
// (CLI stderr JSON, HTTP error responses) emit for those errors.
//
// The registry is exactly the published code set: it equals
// capabilities.ErrorCodes (a contract test enforces both directions),
// so the CONTRACTS error table, the capabilities document and this
// package can never drift apart. Codes are never renamed — renaming a
// published code requires a contract bump; new codes are added here
// and to capabilities in the same change.
//
// Detail codes of inner packages that a surface wraps into a published
// code (prefix-list parser codes → PARSE_FAILED, patch codes →
// INVALID_PATCH / REPAIR_INPUT_INVALID, agent input codes →
// SESSION_INPUT_INVALID) are deliberately not in the registry: they
// never cross a boundary as an error code. The ACL parser's detail
// codes ARE published, because the parse/trace surfaces emit the
// parser error itself.
package errtax

import (
	"errors"

	"plenilune/internal/analysis"
	"plenilune/internal/chat"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/patch"
	"plenilune/internal/prefixlist"
	"plenilune/internal/workflow"
)

// Stage values: the pipeline stage an error belongs to. Callers map
// by code/stage and never parse message text.
const (
	StageRequest  = "request"
	StageParse    = "parse"
	StageValidate = "validate"
	StageEngine   = "engine"
	StageBudget   = "budget"
	StageReplay   = "replay"
	StageProvider = "provider"
)

// Envelope is the one error shape every boundary emits. The CLI writes
// it flat on stderr; the HTTP API wraps it as {"error": ...}. Field is
// present when the failing request field is known; Line/Side carry the
// source position for parser and comparison errors. Retryable means
// the same request, unchanged, may succeed on a later attempt — only
// transient conditions (concurrency, cancellation, wall-clock
// deadline, I/O write/read hiccups, timing-sensitive replay) qualify.
type Envelope struct {
	Code      string `json:"code"`
	Stage     string `json:"stage"`
	Message   string `json:"message"`
	Field     string `json:"field,omitempty"`
	Retryable bool   `json:"retryable"`
	Line      int    `json:"line,omitempty"`
	Side      string `json:"side,omitempty"`
}

type classification struct {
	stage     string
	retryable bool
	field     string
}

// registry is the published taxonomy: code → stage/retryable/field.
// Keep it in the same alphabetical order as capabilities.ErrorCodes.
var registry = map[string]classification{
	"ACL_INVALID":               {StageParse, false, ""},
	"AI_CONFIG_INVALID":         {StageProvider, false, ""},
	"AI_MODEL_UNSUPPORTED":      {StageProvider, false, ""},
	"ANALYSIS_FAILED":           {StageEngine, false, ""},
	"AUDIT_INPUT_INVALID":       {StageValidate, false, ""},
	"BUNDLE_DIGEST_MISMATCH":    {StageReplay, false, ""},
	"BUNDLE_FILE_MISSING":       {StageValidate, false, ""},
	"BUNDLE_INVALID":            {StageValidate, false, ""},
	"CANCELED":                  {StageRequest, true, ""},
	"CHAT_ARTIFACT_INVALID":     {StageReplay, false, ""},
	"CHAT_INVALID":              {StageValidate, false, ""},
	"CHAT_LOCALE_UNSUPPORTED":   {StageValidate, false, "locale"},
	"CHAT_NO_ARTIFACT":          {StageValidate, false, ""},
	"CHAT_QUESTION_EMPTY":       {StageValidate, false, "question"},
	"CHAT_QUESTION_TOO_LONG":    {StageValidate, false, "question"},
	"COMPARISON_FAILED":         {StageEngine, false, ""},
	"COMPARISON_INVALID":        {StageValidate, false, ""},
	"CONTENT_TYPE":              {StageRequest, false, ""},
	"DEADLINE_EXCEEDED":         {StageBudget, true, ""},
	"DUPLICATE_DEFINITION":      {StageParse, false, ""},
	"EVIDENCE_FAILED":           {StageEngine, false, ""},
	"EVIDENCE_MISMATCH":         {StageReplay, false, ""},
	"INBOX_INVALID":             {StageValidate, false, "id"},
	"INPUT_ENCODING":            {StageValidate, false, ""},
	"INPUT_FAILED":              {StageRequest, false, ""},
	"INPUT_INVALID":             {StageParse, false, ""},
	"INPUT_READ":                {StageRequest, true, ""},
	"INPUT_TOO_LARGE":           {StageRequest, false, ""},
	"INVALID_JSON":              {StageRequest, false, ""},
	"INVALID_OBLIGATION":        {StageValidate, false, ""},
	"INVALID_PACKET":            {StageValidate, false, ""},
	"INVALID_PATCH":             {StageValidate, false, ""},
	"INVALID_POLICY":            {StageValidate, false, ""},
	"INVALID_PROPOSALS":         {StageValidate, false, ""},
	"JSON_INVALID":              {StageRequest, false, ""},
	"LOCAL_ORIGIN_REQUIRED":     {StageRequest, false, ""},
	"OBLIGATIONS_INVALID":       {StageValidate, false, ""},
	"OPTIONS_INVALID":           {StageValidate, false, "options"},
	"OUTPUT_FAILED":             {StageRequest, true, ""},
	"PACKET_INVALID":            {StageValidate, false, ""},
	"PARSE_DOCUMENT":            {StageParse, false, ""},
	"PARSE_EMPTY_ACL":           {StageParse, false, ""},
	"PARSE_ENCODING":            {StageParse, false, ""},
	"PARSE_FAILED":              {StageParse, false, ""},
	"PARSE_HEADER":              {StageParse, false, ""},
	"PARSE_INVALID_IP":          {StageParse, false, ""},
	"PARSE_INVALID_PORT":        {StageParse, false, ""},
	"PARSE_INVALID_TOKEN":       {StageParse, false, ""},
	"PARSE_LIMIT":               {StageParse, false, ""},
	"PARSE_MULTIPLE_ACLS":       {StageParse, false, ""},
	"PARSE_NOT_FOUND":           {StageParse, false, ""},
	"PARSE_SEQUENCE_ORDER":      {StageParse, false, ""},
	"POLICY_INPUT_INVALID":      {StageValidate, false, ""},
	"POLICY_INVALID":            {StageValidate, false, "policies"},
	"POLICY_NOT_ACCEPTED":       {StageValidate, false, "policies"},
	"REPAIR_INPUT_INVALID":      {StageValidate, false, ""},
	"REQUEST_LIMIT":             {StageValidate, false, ""},
	"REQUEST_SCHEMA":            {StageValidate, false, "schema"},
	"RESULT_TOO_LARGE":          {StageValidate, false, ""},
	"ROUTE_INVALID":             {StageParse, false, ""},
	"SELECT_INVALID":            {StageValidate, false, "select"},
	"SERVER_BUSY":               {StageRequest, true, ""},
	"SESSION_INPUT_INVALID":     {StageValidate, false, ""},
	"SIGNATURE_DIGEST_MISMATCH": {StageReplay, false, ""},
	"SIGNATURE_INVALID":         {StageValidate, false, ""},
	"SIGNATURE_MISMATCH":        {StageReplay, false, ""},
	"SIGN_ARTIFACT_UNSUPPORTED": {StageValidate, false, ""},
	"SIGN_KEY_INVALID":          {StageValidate, false, ""},
	"SOURCE_REF_INVALID":        {StageValidate, false, "ref"},
	// TIMING_SENSITIVE is a replay status, not an error: it is
	// classified so the published list stays one registry, and it is
	// retryable because a replay under different load can complete.
	"TIMING_SENSITIVE": {StageReplay, true, ""},
	"TRACE_INVALID":    {StageEngine, false, ""},
	"UI_NOT_BUILT":     {StageRequest, false, ""},
	"UNSUPPORTED_KIND": {StageValidate, false, ""},
	"USAGE":            {StageRequest, false, ""},
	"WATCHES_INVALID":  {StageValidate, false, "watches"},
}

// Lookup classifies a published code. ok is false for codes outside
// the published set.
func Lookup(code string) (stage string, retryable bool, field string, ok bool) {
	c, ok := registry[code]
	if !ok {
		return "", false, "", false
	}
	return c.stage, c.retryable, c.field, true
}

// Codes returns the published code set (the registry keys).
func Codes() []string {
	out := make([]string, 0, len(registry))
	for code := range registry {
		out = append(out, code)
	}
	return out
}

// For builds the envelope for a code/message pair. An unregistered
// code classifies as validate/non-retryable: emitting one is a bug
// the registry process exists to catch, never a silent new contract.
func For(code, message string) Envelope {
	stage, retryable, field, ok := Lookup(code)
	if !ok {
		stage = StageValidate
	}
	return Envelope{Code: code, Stage: stage, Message: message, Field: field, Retryable: retryable}
}

// FromError adapts a typed domain error to the envelope when it
// carries a published code, extracting the source line/side the
// domain type knows. ok is false when the error carries no published
// code (including wrapped detail codes a surface must map itself), so
// the surface keeps its existing fallback mapping.
func FromError(err error) (Envelope, bool) {
	var code, message, side string
	var line int
	var aclErr *ciscoacl.Error
	var plErr *prefixlist.Error
	var wfErr *workflow.Error
	var anErr *analysis.Error
	var chatErr *chat.Error
	var patchErr *patch.Error
	switch {
	case errors.As(err, &aclErr):
		code, message, line = aclErr.Code, aclErr.Message, aclErr.Line
	case errors.As(err, &plErr):
		code, message, line = plErr.Code, plErr.Message, plErr.Line
	case errors.As(err, &wfErr):
		code, message, side, line = wfErr.Code, wfErr.Message, wfErr.Side, wfErr.Line
	case errors.As(err, &anErr):
		code, message = anErr.Code, anErr.Message
	case errors.As(err, &chatErr):
		code, message = chatErr.Code, chatErr.Message
	case errors.As(err, &patchErr):
		code, message = patchErr.Code, patchErr.Message
	default:
		return Envelope{}, false
	}
	if _, _, _, ok := Lookup(code); !ok {
		return Envelope{}, false
	}
	env := For(code, message)
	env.Line, env.Side = line, side
	return env, true
}

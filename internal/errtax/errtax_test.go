package errtax

import (
	"errors"
	"sort"
	"testing"

	"plenilune/internal/analysis"
	"plenilune/internal/capabilities"
	"plenilune/internal/chat"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/patch"
	"plenilune/internal/prefixlist"
	"plenilune/internal/workflow"
)

// The taxonomy registry and the capabilities error_codes list are
// the same published set, enforced in both directions: a code cannot
// be classified here without being advertised to callers, and a code
// cannot be advertised without a stage/retryable classification.
func TestRegistryMatchesCapabilities(t *testing.T) {
	got := Codes()
	sort.Strings(got)
	want := append([]string(nil), capabilities.ErrorCodes...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("registry has %d codes, capabilities lists %d\nregistry: %v\ncapabilities: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("code set drift at %d: registry %q vs capabilities %q\nregistry: %v\ncapabilities: %v", i, got[i], want[i], got, want)
		}
	}
}

func TestLookupSpotChecks(t *testing.T) {
	cases := []struct {
		code      string
		stage     string
		retryable bool
		field     string
	}{
		{"USAGE", StageRequest, false, ""},
		{"CANCELED", StageRequest, true, ""},
		{"SERVER_BUSY", StageRequest, true, ""},
		{"JSON_INVALID", StageRequest, false, ""},
		{"PARSE_FAILED", StageParse, false, ""},
		{"PARSE_INVALID_TOKEN", StageParse, false, ""},
		{"REQUEST_SCHEMA", StageValidate, false, "schema"},
		{"OPTIONS_INVALID", StageValidate, false, "options"},
		{"CHAT_QUESTION_TOO_LONG", StageValidate, false, "question"},
		{"COMPARISON_FAILED", StageEngine, false, ""},
		{"DEADLINE_EXCEEDED", StageBudget, true, ""},
		{"EVIDENCE_MISMATCH", StageReplay, false, ""},
		{"TIMING_SENSITIVE", StageReplay, true, ""},
		{"AI_CONFIG_INVALID", StageProvider, false, ""},
		{"AI_MODEL_UNSUPPORTED", StageProvider, false, ""},
	}
	for _, tc := range cases {
		stage, retryable, field, ok := Lookup(tc.code)
		if !ok || stage != tc.stage || retryable != tc.retryable || field != tc.field {
			t.Errorf("Lookup(%q) = %q, %v, %q, %v; want %q, %v, %q, true", tc.code, stage, retryable, field, ok, tc.stage, tc.retryable, tc.field)
		}
	}
	if _, _, _, ok := Lookup("NOT_A_CODE"); ok {
		t.Fatal("unregistered code classified")
	}
}

func TestForUnknownCodeDefaultsToValidate(t *testing.T) {
	env := For("SOME_FUTURE_CODE", "boom")
	if env.Stage != StageValidate || env.Retryable {
		t.Fatalf("unknown code envelope %+v, want validate/non-retryable", env)
	}
	if env.Code != "SOME_FUTURE_CODE" || env.Message != "boom" {
		t.Fatalf("envelope lost code/message: %+v", env)
	}
}

func TestFromErrorExtractsTypedErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want Envelope
	}{
		{
			name: "acl parser detail with line",
			err:  &ciscoacl.Error{Code: "PARSE_INVALID_TOKEN", Line: 7, Message: "bad token"},
			want: Envelope{Code: "PARSE_INVALID_TOKEN", Stage: StageParse, Message: "bad token", Line: 7},
		},
		{
			name: "workflow comparison error with side and line",
			err:  &workflow.Error{Code: "PARSE_FAILED", Message: "ACL parse failed", Side: "before", Line: 3},
			want: Envelope{Code: "PARSE_FAILED", Stage: StageParse, Message: "ACL parse failed", Line: 3, Side: "before"},
		},
		{
			name: "analysis error gains its field",
			err:  &analysis.Error{Code: "OPTIONS_INVALID", Message: "options are outside supported bounds"},
			want: Envelope{Code: "OPTIONS_INVALID", Stage: StageValidate, Message: "options are outside supported bounds", Field: "options"},
		},
		{
			name: "chat error",
			err:  &chat.Error{Code: "CHAT_NO_ARTIFACT", Message: "no completed artifact to interrogate"},
			want: Envelope{Code: "CHAT_NO_ARTIFACT", Stage: StageValidate, Message: "no completed artifact to interrogate"},
		},
		{
			name: "wrapped typed error still adapts",
			err:  errors.Join(errors.New("outer"), &ciscoacl.Error{Code: "PARSE_HEADER", Line: 1, Message: "bad header"}),
			want: Envelope{Code: "PARSE_HEADER", Stage: StageParse, Message: "bad header", Line: 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, ok := FromError(tc.err)
			if !ok {
				t.Fatalf("FromError(%v) not adapted", tc.err)
			}
			if env != tc.want {
				t.Fatalf("FromError = %+v, want %+v", env, tc.want)
			}
		})
	}
}

// Detail codes that surfaces wrap into published codes must NOT
// adapt: the surface's own fallback mapping (PARSE_FAILED,
// INVALID_PATCH, …) stays in charge of them.
func TestFromErrorRejectsUnpublishedDetailCodes(t *testing.T) {
	for _, err := range []error{
		&prefixlist.Error{Code: "PREFIX_LIST_SYNTAX", Line: 2, Message: "bad entry"},
		&patch.Error{Code: "PATCH_LIMIT", Message: "too many edits"},
		errors.New("plain error"),
	} {
		if env, ok := FromError(err); ok {
			t.Fatalf("FromError(%v) = %+v, want not adapted", err, env)
		}
	}
}

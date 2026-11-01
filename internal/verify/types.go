// Package verify performs exact first-match ACL comparison within a bounded
// complete-header IPv4 model, independently of providers and presentation.
package verify

import (
	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
)

type Options struct {
	Limits       packetset.Limits `json:"limits"`
	MaxWitnesses int              `json:"max_witnesses"`
}

func DefaultOptions() Options { return Options{Limits: packetset.Defaults(), MaxWitnesses: 8} }

type Report struct {
	Verdict            string             `json:"verdict"`
	SearchComplete     bool               `json:"search_complete"`
	Reason             string             `json:"reason,omitempty"`
	Witnesses          []Witness          `json:"witnesses"`
	WitnessesTruncated bool               `json:"witnesses_truncated"`
	Obligations        []ObligationResult `json:"obligations"`
	Budget             packetset.Stats    `json:"budget"`
	Options            Options            `json:"options"`
	// Partial is present only when the search stopped on its budget and
	// the engine recorded what had already been decided (P1-2). It never
	// upgrades the verdict: SearchComplete stays false, and the undecided
	// part is bounded but not characterized.
	Partial *PartialResult `json:"partial,omitempty"`
}

// RulePair names one deciding rule pair: the ordinal of the rule that
// decides on the before side and on the after side. Ordinal -1 is the
// implicit final deny. Pieces counts the proven difference fragments
// decided by the pair; it is 0 in an undecided pair, where the pair is
// named only as part of the not-yet-decided remainder.
type RulePair struct {
	Before int `json:"before"`
	After  int `json:"after"`
	Pieces int `json:"pieces"`
}

// UndecidedPart bounds one part of a stopped search that was not decided.
// The rule ranges are inclusive ordinal ranges on each side; a side with
// no rules in the undecided part omits its range. WorkBoxes counts the
// undecided work boxes when the stop left a direction partially decided.
// Nothing inside the bounds is characterized: the packets there may
// differ or not.
type UndecidedPart struct {
	Direction   string  `json:"direction"`
	BeforeRules *[2]int `json:"before_rules,omitempty"`
	AfterRules  *[2]int `json:"after_rules,omitempty"`
	WorkBoxes   int     `json:"work_boxes,omitempty"`
}

// Partial stage values: the pipeline stage a budget stop interrupted.
const (
	PartialStagePrefix       = "prefix"
	PartialStagePermitBefore = "permit_before"
	PartialStagePermitAfter  = "permit_after"
	PartialStageDecideLost   = "decide_lost"
	PartialStageDecideGained = "decide_gained"
	PartialStageWitnesses    = "witnesses"
	PartialStageScopeIndex   = "scope_index"
	PartialStageClasses      = "classes"
)

// Caps on the pair listings inside a partial result. The listings are an
// index into the decided/undecided split, not the proof itself; the
// truncation flags record when a listing was cut.
const (
	MaxPartialDecidedPairs   = 256
	MaxPartialUndecidedPairs = 256
)

// PartialResult records what a budget-stopped search had already decided
// when it stopped, separated from what it left undecided (P1-2). It is
// produced only by engines that record progress (acl-exact/v0.4+); older
// engines keep the all-or-nothing shape and never emit it.
//
// Decided content is exact: PrefixPairs counts the leading rule pairs the
// prefix alignment decided identically (pair (i, i) for i < PrefixPairs),
// and DecidedPairs lists the rule pairs proven to decide at least one
// difference fragment, each fragment evaluator-checked through the
// witnesses/classes of the enclosing report. Undecided content is a
// conservative bound: every not-yet-decided fragment lies inside the
// listed parts, and nothing inside them is claimed decided, equivalent
// or different.
type PartialResult struct {
	Stage                   string          `json:"stage"`
	BeforeRule              *int            `json:"before_rule,omitempty"`
	AfterRule               *int            `json:"after_rule,omitempty"`
	PrefixPairs             int             `json:"prefix_pairs"`
	DecidedPairs            []RulePair      `json:"decided_pairs"`
	DecidedPairsTruncated   bool            `json:"decided_pairs_truncated,omitempty"`
	UndecidedPairs          []RulePair      `json:"undecided_pairs,omitempty"`
	UndecidedPairsTruncated bool            `json:"undecided_pairs_truncated,omitempty"`
	Undecided               []UndecidedPart `json:"undecided"`
}
type Witness struct {
	ID     string            `json:"id"`
	Packet ciscoacl.Packet   `json:"packet"`
	Before ciscoacl.Decision `json:"before"`
	After  ciscoacl.Decision `json:"after"`
}
type ObligationResult struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Reason  string   `json:"reason,omitempty"`
	Witness *Witness `json:"witness,omitempty"`
}

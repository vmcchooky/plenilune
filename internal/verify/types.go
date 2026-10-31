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

package ciscoacl

import (
	"errors"
	"net/netip"
	"plenilune/internal/jsonio"
)

// UnmarshalJSON applies the same required-field rules to standalone packets and
// packets embedded in evidence. Zero is valid; missing and null are not zero.
func (p *Packet) UnmarshalJSON(data []byte) error {
	var in struct {
		Protocol *uint8     `json:"protocol"`
		SrcIP    netip.Addr `json:"src_ip"`
		DstIP    netip.Addr `json:"dst_ip"`
		SrcPort  *uint16    `json:"src_port"`
		DstPort  *uint16    `json:"dst_port"`
	}
	if err := jsonio.Decode(data, 4096, &in); err != nil {
		return err
	}
	if in.Protocol == nil || in.SrcPort == nil || in.DstPort == nil {
		return jsonio.Invalid("packet requires all five fields")
	}
	n := Packet{Protocol: *in.Protocol, SrcIP: in.SrcIP, DstIP: in.DstIP, SrcPort: *in.SrcPort, DstPort: *in.DstPort}
	if err := n.Validate(); err != nil {
		var domain *Error
		if errors.As(err, &domain) {
			return jsonio.Invalid(domain.Message)
		}
		return jsonio.Invalid("packet is outside supported scope")
	}
	*p = n
	return nil
}

// Decision requires an explicit action/implicit flag in evidence; null or
// missing flags must not silently acquire Go's zero-value false.
func (d *Decision) UnmarshalJSON(data []byte) error {
	var in struct {
		Action      *string `json:"action"`
		RuleOrdinal *int    `json:"rule_ordinal,omitempty"`
		Line        *int    `json:"line,omitempty"`
		Implicit    *bool   `json:"implicit"`
	}
	if err := jsonio.Decode(data, 4096, &in); err != nil {
		return err
	}
	if in.Action == nil || in.Implicit == nil {
		return jsonio.Invalid("decision requires action and implicit flag")
	}
	*d = Decision{Action: *in.Action, RuleOrdinal: in.RuleOrdinal, Line: in.Line, Implicit: *in.Implicit}
	return nil
}

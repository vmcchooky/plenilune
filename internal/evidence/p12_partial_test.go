package evidence

// P1-2: a partial explanation is part of the sealed comparison. The
// validator must accept exactly the shapes the v0.4 engine can emit and
// reject anything that would let a partial read as a complete verdict or
// smuggle in pairs the search never decided.

import (
	"strings"
	"testing"

	"plenilune/internal/packetset"
	"plenilune/internal/verify"
)

func p12Partial() *verify.PartialResult {
	return &verify.PartialResult{
		Stage:        verify.PartialStagePermitBefore,
		PrefixPairs:  256,
		DecidedPairs: []verify.RulePair{},
		Undecided: []verify.UndecidedPart{
			{Direction: "delta", BeforeRules: &[2]int{256, 511}, AfterRules: &[2]int{256, 511}},
		},
	}
}

func p12Comparison(partial *verify.PartialResult) Comparison {
	sha := strings.Repeat("a", 64)
	return Comparison{
		Tool:          Tool{Version: ToolVersion, Commit: "unversioned"},
		EngineVersion: verify.EngineVersion,
		Before:        Input{SHA256: sha, Name: "DEMO", Kind: "named-extended"},
		After:         Input{SHA256: sha, Name: "DEMO", Kind: "named-extended"},
		Report: verify.Report{
			Verdict:        "INCONCLUSIVE",
			SearchComplete: false,
			Reason:         "operations",
			Partial:        partial,
			Options:        verify.DefaultOptions(),
			Budget:         packetset.Stats{Operations: 10, PeakBoxes: 2, StopReason: "operations"},
		},
	}
}

func TestP12PartialComparisonSealsAndChecks(t *testing.T) {
	c, err := SealComparison(p12Comparison(p12Partial()))
	if err != nil {
		t.Fatalf("valid partial rejected: %v", err)
	}
	if err := CheckComparison(c); err != nil {
		t.Fatalf("sealed partial failed check: %v", err)
	}
	// Tampering with the proven part breaks the seal like any other field.
	c.Report.Partial.PrefixPairs = 999
	if err := CheckComparison(c); err == nil {
		t.Fatal("tampered partial accepted")
	}
}

func TestP12PartialValidationRejects(t *testing.T) {
	cases := map[string]func(*verify.PartialResult){
		"unknown stage": func(p *verify.PartialResult) { p.Stage = "somewhere" },
		"unsorted decided pairs": func(p *verify.PartialResult) {
			p.DecidedPairs = []verify.RulePair{{Before: 5, After: 5, Pieces: 1}, {Before: 3, After: 3, Pieces: 1}}
		},
		"pair without pieces": func(p *verify.PartialResult) {
			p.DecidedPairs = []verify.RulePair{{Before: 3, After: 3, Pieces: 0}}
		},
		"empty undecided": func(p *verify.PartialResult) { p.Undecided = nil },
		"bogus direction": func(p *verify.PartialResult) { p.Undecided[0].Direction = "sideways" },
		"inverted range": func(p *verify.PartialResult) {
			p.Undecided[0].BeforeRules = &[2]int{511, 256}
		},
		"truncated flag without pairs": func(p *verify.PartialResult) {
			p.DecidedPairsTruncated = true
		},
	}
	for name, mutate := range cases {
		p := p12Partial()
		mutate(p)
		if _, err := SealComparison(p12Comparison(p)); err == nil {
			t.Fatalf("%s: invalid partial sealed", name)
		}
	}

	// The witnesses stage is the one shape with an empty remainder: the
	// delta was fully decided and only witness materialization stopped.
	p := p12Partial()
	p.Stage = verify.PartialStageWitnesses
	p.Undecided = nil
	if _, err := SealComparison(p12Comparison(p)); err != nil {
		t.Fatalf("witnesses-stage partial rejected: %v", err)
	}

	// The stop reason recorded by the partial must be the report's reason.
	c := p12Comparison(p12Partial())
	c.Report.Reason = "boxes"
	if _, err := SealComparison(c); err == nil {
		t.Fatal("partial with mismatched reason sealed")
	}

	// A partial on a completed search is a contradiction in terms.
	c = p12Comparison(p12Partial())
	c.Report.SearchComplete = true
	if _, err := SealComparison(c); err == nil {
		t.Fatal("partial on a complete search sealed")
	}
}

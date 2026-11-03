package repair

import (
	"context"
	"encoding/json"
	"plenilune/internal/ciscoacl"
	"plenilune/internal/verify"
	"reflect"
	"strings"
	"testing"
)

func cloneEnumeration(t *testing.T, e Enumeration) Enumeration {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var out Enumeration
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEnumerationLegacyVersionFrozenSealAndReplay(t *testing.T) {
	b, a, p, _ := fixture(t)
	e, err := enumerateVersion(context.Background(), b, a, &p, verify.DefaultOptions(), LegacyEnumerationVersion)
	if err != nil {
		t.Fatal(err)
	}
	// Captured before enumeration version plumbing, from the original algorithm
	// with historical v0.4 receipts explicitly restored after Verify's bump.
	if e.SHA256 != "38075576a8892a24f46e3e5cb4ac16ae3b6e2846b0ccd47b1f7259b9dbd2c62e" {
		t.Fatalf("frozen legacy enumeration changed: %s", e.SHA256)
	}
	if e.Version != "repair-enumeration/v1" || e.RepairVersion != PartialVersion || e.ParserVersion != "" || e.Options != nil {
		t.Fatal("legacy envelope changed")
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), `"parser_version":"cisco-acl/v0.3"`) || strings.Contains(string(raw), `"repair_version":"acl-repair/v0.5"`) {
		t.Fatal("legacy acquired current pins")
	}
	for _, c := range e.Candidates {
		if c.Result.Version != PartialVersion {
			t.Fatal("legacy candidate version changed")
		}
		if c.Result.Comparison != nil && c.Result.Comparison.ParserVersion != ciscoacl.LegacyParserVersion {
			t.Fatal("legacy candidate comparison parser changed")
		}
		if got, err := Replay(context.Background(), b, a, c.Result); err != nil || got.Status != "REPAIR_REPLAYED" {
			t.Fatalf("legacy candidate replay: %+v %v", got, err)
		}
	}
	got, err := ReplayEnumeration(context.Background(), b, a, &p, e)
	if err != nil || got.Status != "ENUMERATION_REPLAYED" || got.SHA256 != e.SHA256 {
		t.Fatalf("legacy replay: %+v %v", got, err)
	}
	current, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Candidates) != len(e.Candidates) {
		t.Fatal("version bump changed candidate generation")
	}
	for i, c := range e.Candidates {
		n := current.Candidates[i]
		if c.Transform != n.Transform || c.Target != n.Target || c.Resolved != n.Resolved || !reflect.DeepEqual(c.Request, n.Request) || c.Result.Status != n.Result.Status || c.Result.Candidate != n.Result.Candidate {
			t.Fatal("version bump changed legacy extended algorithm")
		}
	}
}

func TestEnumerationCurrentPinsWithoutCandidates(t *testing.T) {
	source := []byte("ip access-list standard X\n deny any\n permit host 192.0.2.7\n")
	for _, starved := range []bool{false, true} {
		opts := verify.DefaultOptions()
		opts.MaxWitnesses = 3
		if starved {
			opts.Limits.MaxOperations = 1
		}
		e, err := Enumerate(context.Background(), source, source, nil, opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(e.Candidates) != 0 || e.Complete == starved {
			t.Fatalf("unexpected no-candidate shape: %+v", e)
		}
		if e.Version != EnumerationVersion || e.RepairVersion != Version || e.ParserVersion != ciscoacl.ParserVersion || e.Options == nil || !reflect.DeepEqual(*e.Options, opts) {
			t.Fatal("current source/options pins missing")
		}
		got, err := ReplayEnumeration(context.Background(), source, source, nil, e)
		if err != nil || got.Status != "ENUMERATION_REPLAYED" || got.SHA256 != e.SHA256 {
			t.Fatalf("no-candidate replay: %+v %v", got, err)
		}
	}
	if _, err := enumerateVersion(context.Background(), source, source, nil, verify.DefaultOptions(), LegacyEnumerationVersion); err == nil {
		t.Fatal("legacy enumeration accepted standard source")
	}
}

func TestEnumerationRejectsRehashedVersionAndOptionsTampering(t *testing.T) {
	b, a, p, _ := fixture(t)
	old, err := enumerateVersion(context.Background(), b, a, &p, verify.DefaultOptions(), LegacyEnumerationVersion)
	if err != nil {
		t.Fatal(err)
	}
	current, err := Enumerate(context.Background(), b, a, &p, verify.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		base   Enumeration
		change func(*Enumeration)
	}{
		{old, func(e *Enumeration) { e.ParserVersion = ciscoacl.LegacyParserVersion }},
		{old, func(e *Enumeration) { o := verify.DefaultOptions(); e.Options = &o }},
		{old, func(e *Enumeration) { e.RepairVersion = Version }},
		{current, func(e *Enumeration) { e.ParserVersion = "" }},
		{current, func(e *Enumeration) { e.ParserVersion = ciscoacl.LegacyParserVersion }},
		{current, func(e *Enumeration) { e.RepairVersion = PartialVersion }},
		{current, func(e *Enumeration) { e.Options = nil }},
		{current, func(e *Enumeration) { e.Options.Limits.MaxOperations = 0 }},
		{current, func(e *Enumeration) { e.Options.MaxWitnesses = 65 }},
		{current, func(e *Enumeration) { e.Options.MaxWitnesses = 1 }},
		{current, func(e *Enumeration) { e.Candidates[0].Result = old.Candidates[0].Result }},
	} {
		e := cloneEnumeration(t, tt.base)
		tt.change(&e)
		if _, err := sealEnumeration(e); err == nil {
			t.Fatal("inconsistent version/options envelope sealed")
		}
		if _, err := ReplayEnumeration(context.Background(), b, a, &p, e); err == nil {
			t.Fatal("tampered enumeration replayed")
		}
	}
}

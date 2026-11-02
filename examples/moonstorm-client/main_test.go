package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"plenilune/internal/cli"
)

// realDocument produces the tool's actual capabilities document
// in-process, exactly as the shipped binary would print it, by
// invoking the CLI the way internal/cli's own tests do — no binary
// is executed, so the gate tests cannot flake on build state.
func realDocument(t *testing.T) document {
	t.Helper()
	var out, errb bytes.Buffer
	if code := cli.Run(context.Background(), []string{"capabilities", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("capabilities exit %d: %s", code, errb.String())
	}
	var doc document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode capabilities document: %v", err)
	}
	return doc
}

func TestGateAcceptsTheRealDocument(t *testing.T) {
	if reasons := gate(realDocument(t)); len(reasons) > 0 {
		t.Fatalf("real document refused: %v", reasons)
	}
}

func TestGateRefusesWrongSchemaVersions(t *testing.T) {
	for _, id := range []string{"plenilune.evidence", "plenilune.analysis"} {
		doc := realDocument(t)
		for i := range doc.Schemas {
			if doc.Schemas[i].ID == id {
				doc.Schemas[i].Current = id + "/v0"
			}
		}
		reasons := gate(doc)
		if len(reasons) != 1 || !strings.Contains(reasons[0], id) {
			t.Fatalf("%s downgraded: reasons = %v, want one naming the schema", id, reasons)
		}
	}
}

func TestGateRefusesMissingFeature(t *testing.T) {
	doc := realDocument(t)
	kept := doc.Features[:0]
	for _, f := range doc.Features {
		if f != "audit" {
			kept = append(kept, f)
		}
	}
	doc.Features = kept
	reasons := gate(doc)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "audit") {
		t.Fatalf("reasons = %v, want one naming the missing feature", reasons)
	}
}

func TestGateRefusesUnknownCapabilitiesSchema(t *testing.T) {
	doc := realDocument(t)
	doc.Schema = "plenilune.capabilities/v99"
	if reasons := gate(doc); len(reasons) == 0 {
		t.Fatal("unknown capabilities schema accepted")
	}
}

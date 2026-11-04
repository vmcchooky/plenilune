package verify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/intent"
	"plenilune/internal/packetset"
)

// The CIS obligation library (examples/cis, P3-8) is data, and data rots.
// Every shipped set must normalize under the obligations validator, and
// the bogon set must actually bite: an ACL that permits bogon sources
// violates it; one that denies them holds.

func loadCISSet(t *testing.T, name string) intent.Set {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "cis", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var set intent.Set
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	norm, _, err := intent.Digest(set)
	if err != nil {
		t.Fatalf("library set %s does not normalize: %v", name, err)
	}
	return norm
}

func TestCISLibrarySetsNormalize(t *testing.T) {
	for _, name := range []string{
		"ingress-bogons-core.json",
		"ingress-bogons-special.json",
		"cleartext-services.json",
		"egress-template.json",
	} {
		loadCISSet(t, name)
	}
}

func TestCISBogonSetBites(t *testing.T) {
	set := loadCISSet(t, "ingress-bogons-core.json")
	opts := Options{Limits: packetset.Defaults(), MaxWitnesses: 4}
	parse := func(text string) ciscoacl.ACL {
		a, err := ciscoacl.Parse([]byte(text))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return a
	}
	// Permissive edge ACL: every must_deny bogon obligation is violated.
	open := parse("ip access-list extended EDGE\n permit ip any any\n")
	rep, err := CompareEngine(context.Background(), open, open, &set, opts, EngineVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range rep.Obligations {
		if o.Status != "VIOLATED" {
			t.Fatalf("open ACL: obligation %s = %s, want VIOLATED", o.ID, o.Status)
		}
	}
	// Filtered edge ACL: bogons denied first, the rest permitted.
	filtered := parse(`ip access-list extended EDGE
 deny ip 0.0.0.0 0.255.255.255 any
 deny ip 10.0.0.0 0.255.255.255 any
 deny ip 127.0.0.0 0.255.255.255 any
 deny ip 169.254.0.0 0.0.255.255 any
 deny ip 172.16.0.0 0.15.255.255 any
 deny ip 192.168.0.0 0.0.255.255 any
 permit ip any any
`)
	rep, err = CompareEngine(context.Background(), filtered, filtered, &set, opts, EngineVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range rep.Obligations {
		if o.Status != "HOLDS" {
			t.Fatalf("filtered ACL: obligation %s = %s, want HOLDS", o.ID, o.Status)
		}
	}
}

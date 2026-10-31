package intent

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPortIntervalRequiresBothBounds(t *testing.T) {
	for _, data := range []string{`{"hi":22}`, `{"lo":0}`, `null`, `{"lo":0,"hi":null}`, `{"lo":0,"hi":22,"hi":23}`} {
		var p Interval
		if json.Unmarshal([]byte(data), &p) == nil {
			t.Errorf("accepted interval %s", data)
		}
	}
	var p Interval
	if err := json.Unmarshal([]byte(`{"lo":0,"hi":65535}`), &p); err != nil || p.Hi != 65535 {
		t.Fatalf("rejected full interval: %v", err)
	}
}

func fixture() Set {
	return Set{Schema: Schema, Scope: Scope, Obligations: []Obligation{{ID: "OB-1", Kind: "must_deny", Match: Region{Protocol: "tcp", SrcCIDR: "10.0.0.0/24", DstCIDR: "any", DstPorts: []Interval{{Lo: 22, Hi: 22}}}}}}
}

func TestPredicateIsBoundByDigest(t *testing.T) {
	s := fixture()
	_, before, err := Digest(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Obligations[0].Match.DstPorts[0].Hi = 23
	_, after, err := Digest(s)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("changing predicate left digest unchanged")
	}
}

func TestCanonicalNormalizationOwnsData(t *testing.T) {
	s := fixture()
	s.Obligations[0].Match.ExcludeSrc = []string{"10.0.0.128/26", "10.0.0.0/25", "10.0.0.0/26"}
	s.Obligations[0].Match.DstPorts = []Interval{{Lo: 65535, Hi: 65535}, {Lo: 22, Hi: 22}, {Lo: 23, Hi: 24}}
	n, h, err := Digest(s)
	if err != nil {
		t.Fatal(err)
	}
	n2, h2, err := Digest(n)
	if err != nil || h != h2 || !reflect.DeepEqual(n, n2) {
		t.Fatalf("not idempotent: %v", err)
	}
	if !reflect.DeepEqual(n.Obligations[0].Match.DstPorts, []Interval{{Lo: 22, Hi: 24}, {Lo: 65535, Hi: 65535}}) {
		t.Fatal("interval normalization wrong")
	}
	n.Obligations[0].Match.DstPorts[0].Hi = 99
	if s.Obligations[0].Match.DstPorts[1].Hi != 22 {
		t.Fatal("normalization mutated input")
	}
}

func TestRejectEmptyOrUnsupportedPredicates(t *testing.T) {
	cases := []func(*Set){
		func(s *Set) { s.Scope = "all-network" },
		func(s *Set) { s.Obligations = nil },
		func(s *Set) { s.Obligations = append(s.Obligations, s.Obligations[0]) },
		func(s *Set) { s.Obligations[0].Match.ExcludeSrc = []string{"10.0.0.0/25", "10.0.0.128/25"} },
		func(s *Set) { s.Obligations[0].Match.ExcludeSrc = []string{"10.1.0.0/24"} },
		func(s *Set) { s.Obligations[0].Match.Protocol = "ip" },
		func(s *Set) { s.Obligations[0].Match.DstPorts = []Interval{{Lo: 23, Hi: 22}} },
		func(s *Set) { s.Obligations[0].Match.SrcCIDR = "::/0" },
	}
	for i, mutate := range cases {
		s := fixture()
		mutate(&s)
		if _, err := Normalize(s); err == nil {
			t.Errorf("accepted invalid case %d", i)
		}
	}
}

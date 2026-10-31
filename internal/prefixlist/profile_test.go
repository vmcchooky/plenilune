package prefixlist

import (
	"context"
	"math"
	"testing"
)

func TestSequenceBoundsAndOmittedSequenceOrder(t *testing.T) {
	for _, text := range []string{
		"ip prefix-list P seq 4294967295 permit 10.0.0.0/8",
		"ip prefix-list P seq 4294967290 permit 10.0.0.0/8\nip prefix-list P deny 0.0.0.0/0 le 32",
	} {
		if _, err := ParseLines("P", lines(text)); err == nil {
			t.Fatal("out-of-profile sequence accepted")
		}
	}
	l := mustList(t, "P", "ip prefix-list P seq 4294967289 permit 10.0.0.0/8\nip prefix-list P deny 0.0.0.0/0 le 32")
	if l.Entries[1].Seq != MaxSequence {
		t.Fatal("maximum valid implicit sequence lost")
	}
	l = mustList(t, "P", "ip prefix-list P deny 10.0.0.0/8\nip prefix-list P permit 0.0.0.0/0 le 32")
	l.Entries[0], l.Entries[1] = l.Entries[1], l.Entries[0]
	l.Entries[0].Seq, l.Entries[1].Seq = 5, 10
	if err := l.Validate(); err == nil {
		t.Fatal("forged implicit sequence order accepted")
	}
	if _, err := l.Evaluate(route(t, "10.0.0.0/8")); err == nil {
		t.Fatal("invalid IR evaluated")
	}
}

func TestPublicBoundariesRejectInvalidIR(t *testing.T) {
	base := mustList(t, "P", "ip prefix-list P seq 10 permit 10.0.0.0/8 le 24\nip prefix-list P seq 20 deny 0.0.0.0/0 le 32")
	cases := []struct {
		name string
		edit func(*List)
	}{
		{"empty", func(l *List) { l.Entries = nil }},
		{"reserved name", func(l *List) { l.Name = "detail" }},
		{"action", func(l *List) { l.Entries[0].Action = "allow" }},
		{"length", func(l *List) { l.Entries[0].Len = 33 }},
		{"host bits", func(l *List) { l.Entries[0].Network++ }},
		{"derived interval", func(l *List) { l.Entries[0].Hi = 32 }},
		{"sequence bound", func(l *List) { l.Entries[1].Seq = math.MaxUint32 }},
		{"sequence order", func(l *List) { l.Entries[0], l.Entries[1] = l.Entries[1], l.Entries[0] }},
		{"duplicate line", func(l *List) { l.Entries[1].Line = l.Entries[0].Line }},
		{"missing source", func(l *List) { l.Entries[0].Text = "" }},
		{"false source", func(l *List) { l.Entries[0].Text = "ip prefix-list P seq 10 deny 10.0.0.0/8 le 24" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := base
			bad.Entries = append([]Entry(nil), base.Entries...)
			tc.edit(&bad)
			if _, err := Compare(context.Background(), bad, base, DefaultMaxClasses); err == nil {
				t.Fatal("invalid IR produced comparison")
			}
			if _, err := bad.Evaluate(route(t, "10.0.0.0/8")); err == nil {
				t.Fatal("invalid IR produced decision")
			}
		})
	}
}

func TestInclusiveProfileCountsCompleteRouteUniverse(t *testing.T) {
	deny := mustList(t, "P", "ip prefix-list P deny 0.0.0.0/0 le 32")
	for _, tc := range []struct {
		rule  string
		count uint64
		first string
	}{
		{"permit 0.0.0.0/0 le 32", 8589934591, "0.0.0.0/0"},
		{"permit 10.0.0.0/8 ge 24 le 24", 65536, "10.0.0.0/24"},
		{"permit 10.0.0.0/8 le 24", 131071, "10.0.0.0/8"},
		{"permit 192.0.2.1/32", 1, "192.0.2.1/32"},
	} {
		t.Run(tc.rule, func(t *testing.T) {
			allow := mustList(t, "P", "ip prefix-list P "+tc.rule)
			r, err := Compare(context.Background(), deny, allow, DefaultMaxClasses)
			if err != nil || !r.Complete || r.Verdict != "DIFFERENT" || len(r.Classes) != 1 || r.Classes[0].Routes != tc.count || r.Classes[0].Witness.Route.String() != tc.first {
				t.Fatalf("incorrect exact count: %+v %v", r, err)
			}
		})
	}
}

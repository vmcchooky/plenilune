package prefixlist

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Fuzz targets for the prefix-list parsers (plan P1-6). Seeds are mined from
// the project's synthetic testdata and unit tests (documentation address
// space only). Invariants: never panic, deterministic results, and the
// current and legacy parsers both stay total on arbitrary text.

// definitionLines extracts the source lines of the first prefix-list named
// in the input, mirroring how analysis intake groups definition lines.
func definitionLines(data []byte) (string, []SourceLine) {
	var name string
	var out []SourceLine
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		n, ok := IsDefinition(line)
		if !ok {
			continue
		}
		if name == "" {
			name = n
		}
		if n != name {
			continue
		}
		out = append(out, SourceLine{Number: i + 1, Text: line})
	}
	return name, out
}

func FuzzParsePrefixList(f *testing.F) {
	f.Add([]byte("ip prefix-list CUSTOMER seq 5 permit 203.0.113.0/24\nip prefix-list CUSTOMER seq 10 deny 0.0.0.0/0 le 32\n"))
	f.Add([]byte("ip prefix-list PL permit 192.0.2.0/24 ge 25 le 28\nip prefix-list PL deny 198.51.100.0/25\n"))
	f.Add([]byte("ip prefix-list PL permit 10.0.0.0/8\nip prefix-list PL permit 172.16.0.0/12\n"))
	f.Add([]byte("ip prefix-list PL description synthetic routes\nip prefix-list PL seq 5 permit 203.0.113.0/24 ge 32\n"))
	f.Add([]byte("ip prefix-list PL permit 203.0.113.1/24\n"))
	f.Add([]byte("ip prefix-list PL seq 4294967294 permit 0.0.0.0/0\nip prefix-list PL permit 10.0.0.0/8\n"))
	f.Add([]byte(""))
	f.Add([]byte("hostname router\nrouter bgp 64500\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		name, lines := definitionLines(data)
		if name == "" || len(lines) == 0 {
			return
		}
		for _, version := range []string{ParserVersion, LegacyParserVersion} {
			l, err := ParseLinesVersion(name, lines, version)
			if err != nil {
				continue
			}
			again, err := ParseLinesVersion(name, lines, version)
			if err != nil || !reflect.DeepEqual(l, again) {
				t.Fatalf("parser %s is not deterministic: %v", version, err)
			}
			if err := l.Validate(); err != nil {
				t.Fatalf("parser %s returned a list that fails Validate: %v", version, err)
			}
		}
	})
}

// FuzzPrefixListRoundTrip is parse -> print -> re-parse over the canonical
// text form: every parsed entry is printed as one definition line and the
// printed lines must re-parse to the same entries (sequence numbers are
// printed explicitly, so SeqGiven is excluded from the comparison).
func FuzzPrefixListRoundTrip(f *testing.F) {
	f.Add([]byte("ip prefix-list CUSTOMER seq 5 permit 203.0.113.0/24\nip prefix-list CUSTOMER seq 10 deny 0.0.0.0/0 le 32\n"))
	f.Add([]byte("ip prefix-list PL permit 192.0.2.0/24 ge 25 le 28\nip prefix-list PL deny 198.51.100.0/25\n"))
	f.Add([]byte("ip prefix-list PL permit 10.0.0.0/8\nip prefix-list PL permit 172.16.0.0/12\n"))
	f.Add([]byte("ip prefix-list PL seq 5 permit 203.0.113.0/24 ge 32\nip prefix-list PL seq 10 deny 203.0.113.128/25\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		name, lines := definitionLines(data)
		if name == "" || len(lines) == 0 {
			return
		}
		l, err := ParseLines(name, lines)
		if err != nil {
			return
		}
		printed := make([]SourceLine, 0, len(l.Entries))
		for i, e := range l.Entries {
			text := fmt.Sprintf("ip prefix-list %s seq %d %s %s", name, e.Seq, e.Action, Route{Network: e.Network, Len: e.Len})
			if e.Ge != 0 {
				text += fmt.Sprintf(" ge %d", e.Ge)
			}
			if e.Le != 0 {
				text += fmt.Sprintf(" le %d", e.Le)
			}
			printed = append(printed, SourceLine{Number: i + 1, Text: text})
		}
		back, err := ParseLines(name, printed)
		if err != nil {
			t.Fatalf("printed form does not re-parse: %v\n%v", err, printed)
		}
		if len(back.Entries) != len(l.Entries) {
			t.Fatalf("round trip changed entry count: %d -> %d", len(l.Entries), len(back.Entries))
		}
		for i := range l.Entries {
			a, b := l.Entries[i], back.Entries[i]
			if a.Seq != b.Seq || a.Action != b.Action || a.Network != b.Network || a.Len != b.Len ||
				a.Ge != b.Ge || a.Le != b.Le || a.Lo != b.Lo || a.Hi != b.Hi {
				t.Fatalf("round trip changed entry %d:\n%+v\n%+v", i, a, b)
			}
		}
	})
}

package prefixlist

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

func validName(name string) bool {
	return listName.MatchString(name) && !strings.EqualFold(name, "detail") && !strings.EqualFold(name, "summary") && !strings.EqualFold(name, "sequence-number")
}

// Validate rejects malformed exported IR before arithmetic or proof generation.
// Source lines remain unique but need not follow sequence order.
func (l List) Validate() error {
	if !validName(l.Name) {
		return fail("PREFIX_LIST_NAME", 0, "unsupported prefix-list name")
	}
	if len(l.Entries) == 0 {
		return fail("PREFIX_LIST_EMPTY", 0, "prefix-list has no permit/deny entries")
	}
	if len(l.Entries) > MaxEntries {
		return fail("PREFIX_LIST_LIMIT", 0, "prefix-list exceeds entry limit")
	}
	seen := make(map[int]bool, len(l.Entries))
	var prev uint32
	source := make([]SourceLine, 0, len(l.Entries))
	for _, e := range l.Entries {
		if e.Seq == 0 || e.Seq > MaxSequence || e.Seq <= prev {
			return fail("PREFIX_LIST_SEQUENCE", e.Line, "entries must have unique increasing valid sequences")
		}
		prev = e.Seq
		if e.Line < 1 || seen[e.Line] || e.Text == "" || len(e.Text) > 8192 || strings.ContainsAny(e.Text, "\r\n") {
			return fail("PREFIX_LIST_SOURCE", e.Line, "entry source references are invalid")
		}
		seen[e.Line] = true
		if e.Action != "permit" && e.Action != "deny" {
			return fail("PREFIX_LIST_SYNTAX", e.Line, "action must be permit or deny")
		}
		if e.Len > 32 || e.Network&^mask(e.Len) != 0 {
			return fail("PREFIX_LIST_PREFIX", e.Line, "expected canonical IPv4 network")
		}
		lo, hi := e.Len, e.Len
		if e.Ge > 0 {
			if e.Ge <= e.Len || e.Ge > 32 {
				return fail("PREFIX_LIST_RANGE", e.Line, "ge must exceed network length and be at most 32")
			}
			lo, hi = e.Ge, 32
		}
		if e.Le > 0 {
			if e.Le <= e.Len || e.Le > 32 || e.Ge > e.Le {
				return fail("PREFIX_LIST_RANGE", e.Line, "invalid inclusive length range")
			}
			hi = e.Le
		}
		if e.Lo != lo || e.Hi != hi {
			return fail("PREFIX_LIST_RANGE", e.Line, "derived length interval differs from ge/le")
		}
		source = append(source, SourceLine{Number: e.Line, Text: e.Text})
	}
	// Resolve omitted sequences from original source order, not entry order.
	// A per-entry parse would let forged Seq values reverse first-match behavior.
	slices.SortFunc(source, func(a, b SourceLine) int {
		if a.Number < b.Number {
			return -1
		}
		if a.Number > b.Number {
			return 1
		}
		return 0
	})
	parsed, err := parseLinesLegacy(l.Name, source)
	if err != nil || !reflect.DeepEqual(l, parsed) {
		return fail("PREFIX_LIST_SOURCE", 0, "entry fields differ from source text or implicit sequence order")
	}
	return nil
}

// Versioned entry points exist for frozen receipt replay, never as a fallback
// after current parsing/verification fails.
func ParseLinesVersion(name string, lines []SourceLine, version string) (List, error) {
	switch version {
	case ParserVersion:
		return ParseLines(name, lines)
	case LegacyParserVersion:
		return parseLinesLegacy(name, lines)
	default:
		return List{}, fmt.Errorf("unsupported prefix-list parser version")
	}
}

func CompareVersion(ctx context.Context, before, after List, maxClasses int, version string) (Report, error) {
	switch version {
	case EngineVersion:
		return Compare(ctx, before, after, maxClasses)
	case LegacyEngineVersion:
		return compareLegacy(ctx, before, after, maxClasses)
	default:
		return Report{}, fmt.Errorf("unsupported prefix-list engine version")
	}
}

// Package prefixlist compares a declared canonical IPv4 IOS-style subset.
// Exact results concern this model, not device acceptance or routing behavior.
// It has no network, device or file I/O.
package prefixlist

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	ParserVersion              = "cisco-prefix-list/v0.2"
	EngineVersion              = "prefix-list-exact/v0.2"
	LegacyParserVersion        = "cisco-prefix-list/v0.1"
	LegacyEngineVersion        = "prefix-list-exact/v0.1"
	Profile                    = "ios-style-canonical-ipv4-prefix-list/v1"
	MaxSequence         uint32 = 4294967294
	MaxEntries                 = 10000
)

var listName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,63}$`)

// Entry matches routes whose first Len bits equal Network and whose length is
// in [Lo, Hi]. Inclusive endpoints belong to the declared model; this is not
// a claim that all IOS releases accept every syntax in the subset.
type Entry struct {
	Seq      uint32 `json:"seq"`
	SeqGiven bool   `json:"seq_given"`
	Action   string `json:"action"`
	Network  uint32 `json:"network"`
	Len      uint8  `json:"len"`
	Ge       uint8  `json:"ge,omitempty"`
	Le       uint8  `json:"le,omitempty"`
	Lo       uint8  `json:"lo"`
	Hi       uint8  `json:"hi"`
	Line     int    `json:"line"`
	Text     string `json:"text"`
}

type List struct {
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
}

type Error struct {
	Code    string `json:"code"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s line %d: %s", e.Code, e.Line, e.Message) }

func fail(code string, line int, msg string) error {
	return &Error{Code: code, Line: line, Message: msg}
}

// SourceLine is one configuration line with its 1-based line number.
type SourceLine struct {
	Number int
	Text   string
}

// IsDefinition reports whether a top-level line defines an IPv4 prefix-list
// entry or description, and returns the list name.
func IsDefinition(text string) (string, bool) {
	f := strings.Fields(text)
	if len(f) < 3 || !strings.EqualFold(f[0], "ip") || !strings.EqualFold(f[1], "prefix-list") {
		return "", false
	}
	if strings.EqualFold(f[2], "sequence-number") {
		return "", false
	}
	return f[2], true
}

// ParseLines builds one list from its definition lines in text order.
// Omitted sequence numbers follow Cisco's documented rule: 5 for the first
// entry, otherwise the highest sequence so far plus 5. Duplicate sequence
// numbers are rejected because the effective entry would depend on history.
func ParseLines(name string, lines []SourceLine) (List, error) {
	if !validName(name) {
		return List{}, fail("PREFIX_LIST_NAME", 0, "unsupported prefix-list name")
	}
	l := List{Name: name, Entries: []Entry{}}
	seen := map[uint32]int{}
	var highest uint32
	for _, line := range lines {
		f := strings.Fields(line.Text)
		if len(f) < 4 || !strings.EqualFold(f[0], "ip") || !strings.EqualFold(f[1], "prefix-list") || f[2] != name {
			return List{}, fail("PREFIX_LIST_SYNTAX", line.Number, "expected ip prefix-list "+name)
		}
		rest := f[3:]
		if strings.EqualFold(rest[0], "description") {
			if len(rest) < 2 {
				return List{}, fail("PREFIX_LIST_SYNTAX", line.Number, "description text is missing")
			}
			continue
		}
		e := Entry{Line: line.Number, Text: strings.TrimSpace(line.Text)}
		if strings.EqualFold(rest[0], "seq") {
			if len(rest) < 2 {
				return List{}, fail("PREFIX_LIST_SYNTAX", line.Number, "sequence number is missing")
			}
			v, err := strconv.ParseUint(rest[1], 10, 32)
			if err != nil || v == 0 || v > uint64(MaxSequence) {
				return List{}, fail("PREFIX_LIST_SEQUENCE", line.Number, "invalid sequence number")
			}
			e.Seq, e.SeqGiven = uint32(v), true
			rest = rest[2:]
		} else {
			if uint64(highest)+5 > uint64(MaxSequence) {
				return List{}, fail("PREFIX_LIST_SEQUENCE", line.Number, "sequence number overflow")
			}
			e.Seq = highest + 5
		}
		if len(rest) < 2 {
			return List{}, fail("PREFIX_LIST_SYNTAX", line.Number, "action and prefix are required")
		}
		e.Action = strings.ToLower(rest[0])
		if e.Action != "permit" && e.Action != "deny" {
			return List{}, fail("PREFIX_LIST_SYNTAX", line.Number, "action must be permit or deny")
		}
		p, err := netip.ParsePrefix(rest[1])
		if err != nil || !p.Addr().Is4() {
			return List{}, fail("PREFIX_LIST_PREFIX", line.Number, "expected IPv4 network/length")
		}
		// This canonical subset rejects host bits; it does not guess how a
		// particular device normalizes them.
		if p.Masked() != p {
			return List{}, fail("PREFIX_LIST_HOST_BITS", line.Number, "network has host bits set beyond its length")
		}
		a := p.Addr().As4()
		e.Network = uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])
		e.Len = uint8(p.Bits())
		rest = rest[2:]
		for len(rest) > 0 {
			if len(rest) < 2 {
				return List{}, fail("PREFIX_LIST_SYNTAX", line.Number, "ge/le requires a value")
			}
			v, err := strconv.ParseUint(rest[1], 10, 8)
			if err != nil || v < 1 || v > 32 {
				return List{}, fail("PREFIX_LIST_RANGE", line.Number, "ge/le must be 1 through 32")
			}
			switch strings.ToLower(rest[0]) {
			case "ge":
				if e.Ge != 0 || e.Le != 0 {
					return List{}, fail("PREFIX_LIST_SYNTAX", line.Number, "ge must appear once, before le")
				}
				e.Ge = uint8(v)
			case "le":
				if e.Le != 0 {
					return List{}, fail("PREFIX_LIST_SYNTAX", line.Number, "le must appear once")
				}
				e.Le = uint8(v)
			default:
				return List{}, fail("PREFIX_LIST_UNSUPPORTED", line.Number, "unsupported prefix-list option")
			}
			rest = rest[2:]
		}
		// Profile rule: len < ge <= le <= 32. Equal ge/le represents one length.
		switch {
		case e.Ge == 0 && e.Le == 0:
			e.Lo, e.Hi = e.Len, e.Len
		case e.Ge != 0 && e.Le == 0:
			if e.Ge <= e.Len {
				return List{}, fail("PREFIX_LIST_RANGE", line.Number, "ge must exceed the network length")
			}
			e.Lo, e.Hi = e.Ge, 32
		case e.Ge == 0:
			if e.Le <= e.Len {
				return List{}, fail("PREFIX_LIST_RANGE", line.Number, "le must exceed the network length")
			}
			e.Lo, e.Hi = e.Len, e.Le
		default:
			if e.Ge <= e.Len || e.Le < e.Ge {
				return List{}, fail("PREFIX_LIST_RANGE", line.Number, "expected length < ge <= le <= 32")
			}
			e.Lo, e.Hi = e.Ge, e.Le
		}
		if first, dup := seen[e.Seq]; dup {
			return List{}, fail("PREFIX_LIST_SEQUENCE", line.Number, fmt.Sprintf("duplicate sequence number (first at line %d)", first))
		}
		seen[e.Seq] = line.Number
		highest = max(highest, e.Seq)
		if len(l.Entries) >= MaxEntries {
			return List{}, fail("PREFIX_LIST_LIMIT", line.Number, "prefix-list exceeds entry limit")
		}
		l.Entries = append(l.Entries, e)
	}
	if len(l.Entries) == 0 {
		// Empty/description-only definitions are outside this nonempty subset.
		return List{}, fail("PREFIX_LIST_EMPTY", 0, "prefix-list has no permit/deny entries")
	}
	slices.SortStableFunc(l.Entries, func(a, b Entry) int {
		if a.Seq < b.Seq {
			return -1
		}
		if a.Seq > b.Seq {
			return 1
		}
		return 0
	})
	if err := l.Validate(); err != nil {
		return List{}, err
	}
	return l, nil
}

// Route is a destination prefix. Host bits beyond Len must be zero.
type Route struct {
	Network uint32 `json:"-"`
	Len     uint8  `json:"-"`
}

func (r Route) String() string {
	return fmt.Sprintf("%d.%d.%d.%d/%d", byte(r.Network>>24), byte(r.Network>>16), byte(r.Network>>8), byte(r.Network), r.Len)
}

func (r Route) MarshalText() ([]byte, error) { return []byte(r.String()), nil }

func (r *Route) UnmarshalText(b []byte) error {
	p, err := netip.ParsePrefix(string(b))
	if err != nil || !p.Addr().Is4() || p.Masked() != p {
		return fail("ROUTE_INVALID", 0, "route must be a canonical IPv4 prefix")
	}
	a := p.Addr().As4()
	r.Network, r.Len = uint32(a[0])<<24|uint32(a[1])<<16|uint32(a[2])<<8|uint32(a[3]), uint8(p.Bits())
	return nil
}

type Decision struct {
	Action   string  `json:"action"`
	Seq      *uint32 `json:"seq,omitempty"`
	Line     *int    `json:"line,omitempty"`
	Implicit bool    `json:"implicit"`
}

func mask(n uint8) uint32 {
	if n == 0 {
		return 0
	}
	return ^uint32(0) << (32 - n)
}

// Matches is the direct, bit-mask definition used by Evaluate.
func (e Entry) Matches(r Route) bool {
	return r.Len >= e.Lo && r.Len <= e.Hi && r.Network&mask(e.Len) == e.Network
}

// Evaluate checks entries in sequence order; first match wins; no match is
// the implicit deny. It does not use the interval algebra.
func (l List) Evaluate(r Route) (Decision, error) {
	if err := l.Validate(); err != nil {
		return Decision{}, err
	}
	return l.evaluateUnchecked(r)
}

func (l List) evaluateUnchecked(r Route) (Decision, error) {
	if r.Len > 32 || r.Network&^mask(r.Len) != 0 {
		return Decision{}, fail("ROUTE_INVALID", 0, "route must be a canonical IPv4 prefix")
	}
	for _, e := range l.Entries {
		if e.Matches(r) {
			seq, line := e.Seq, e.Line
			return Decision{Action: e.Action, Seq: &seq, Line: &line}, nil
		}
	}
	return Decision{Action: "deny", Implicit: true}, nil
}

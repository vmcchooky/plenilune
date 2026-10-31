package prefixlist

// Frozen v0.1 parser: replay only. New analysis never selects it.
import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

func parseLinesLegacy(name string, lines []SourceLine) (List, error) {
	if !listName.MatchString(name) {
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
			if err != nil || v == 0 {
				return List{}, fail("PREFIX_LIST_SEQUENCE", line.Number, "invalid sequence number")
			}
			e.Seq, e.SeqGiven = uint32(v), true
			rest = rest[2:]
		} else {
			e.Seq = highest + 5
			if e.Seq < highest {
				return List{}, fail("PREFIX_LIST_SEQUENCE", line.Number, "sequence number overflow")
			}
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
		// Host bits are not normalized: the documentation does not define them.
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
		// Documented: len < ge <= le <= 32. ge == le is accepted (see design).
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
		// Empty-list behavior is not documented for IOS; never guess it.
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
	return l, nil
}

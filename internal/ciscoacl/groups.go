package ciscoacl

import (
	"sort"
	"strings"
)

// Object-groups (parser v0.4). Definitions live only in the source text;
// the parser resolves every reference into the rule IR at parse time —
// network groups become canonical Address sets (Rule.SrcSet / Rule.DstSet),
// service groups become merged port Interval sets. No group structure is
// retained in the DTO, and no rule is ever textually expanded: a rule that
// references groups stays ONE rule whose fields are unions.
// See docs/semantics/object-groups-v04.md.

// GroupParserVersion is the first parser version whose extended grammar
// includes object-groups. Intake upgrades ACL objects to group-aware
// parsing only for artifacts recorded at this version or later.
const GroupParserVersion = "cisco-acl/v0.4"

const (
	// MaxGroups bounds object-group definitions in one document.
	MaxGroups = 64
	// MaxGroupMembers bounds the resolved membership of one group
	// (after nested group-object references are flattened).
	MaxGroupMembers = 1024
	// MaxGroupDepth bounds group-object nesting.
	MaxGroupDepth = 8
)

type netMember struct {
	addr  Address
	group string // nested group-object name; empty when addr is set
	line  int
}

type networkGroup struct {
	name    string
	members []netMember
	line    int
}

type svcMember struct {
	intervals []Interval // empty when group is set
	group     string     // nested group-object name; empty when intervals is set
	line      int
}

type serviceGroup struct {
	name    string
	tcp     bool
	udp     bool
	members []svcMember
	line    int
}

type groupTable struct {
	network map[string]*networkGroup
	service map[string]*serviceGroup
}

func newGroupTable() *groupTable {
	return &groupTable{network: map[string]*networkGroup{}, service: map[string]*serviceGroup{}}
}

func (g *groupTable) empty() bool { return len(g.network) == 0 && len(g.service) == 0 }

// resolveNetwork flattens a network group (including nested references)
// into its canonical member set: normalized, sorted by (Value, Wildcard),
// exact duplicates removed.
func (g *groupTable) resolveNetwork(name string, refLine int) ([]Address, error) {
	out, err := g.networkMembers(name, refLine, 0, map[string]bool{})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value < out[j].Value
		}
		return out[i].Wildcard < out[j].Wildcard
	})
	dedup := out[:0]
	for i, a := range out {
		if i > 0 && a == out[i-1] {
			continue
		}
		dedup = append(dedup, a)
	}
	return dedup, nil
}

func (g *groupTable) networkMembers(name string, refLine, depth int, visiting map[string]bool) ([]Address, error) {
	grp, ok := g.network[name]
	if !ok {
		return nil, fail("PARSE_GROUP_UNDEFINED", refLine, "undefined network object-group")
	}
	if visiting[name] {
		return nil, fail("PARSE_GROUP_CYCLE", grp.line, "network object-group reference cycle")
	}
	if depth > MaxGroupDepth {
		return nil, fail("PARSE_GROUP_LIMIT", grp.line, "network object-group nesting too deep")
	}
	visiting[name] = true
	defer delete(visiting, name)
	out := []Address{}
	for _, m := range grp.members {
		if m.group == "" {
			out = append(out, m.addr)
			continue
		}
		sub, err := g.networkMembers(m.group, m.line, depth+1, visiting)
		if err != nil {
			return nil, err
		}
		out = append(out, sub...)
		if len(out) > MaxGroupMembers {
			return nil, fail("PARSE_GROUP_LIMIT", grp.line, "network object-group membership too large")
		}
	}
	return out, nil
}

// resolveService flattens a service group for one protocol (6 or 17) into
// its canonical port set: intervals sorted by Lo with overlapping or
// adjacent intervals merged.
func (g *groupTable) resolveService(name string, proto uint8, refLine int) ([]Interval, error) {
	grp, ok := g.service[name]
	if !ok {
		return nil, fail("PARSE_GROUP_UNDEFINED", refLine, "undefined service object-group")
	}
	if proto == 6 && !grp.tcp || proto == 17 && !grp.udp {
		return nil, fail("PARSE_GROUP_PROTOCOL", refLine, "service object-group does not cover the rule protocol")
	}
	raw, err := g.serviceIntervals(name, refLine, 0, map[string]bool{})
	if err != nil {
		return nil, err
	}
	sort.Slice(raw, func(i, j int) bool {
		if raw[i].Lo != raw[j].Lo {
			return raw[i].Lo < raw[j].Lo
		}
		return raw[i].Hi < raw[j].Hi
	})
	out := raw[:0]
	for _, iv := range raw {
		if len(out) > 0 && uint32(iv.Lo) <= uint32(out[len(out)-1].Hi)+1 {
			if iv.Hi > out[len(out)-1].Hi {
				out[len(out)-1].Hi = iv.Hi
			}
			continue
		}
		out = append(out, iv)
	}
	if len(out) > MaxPortIntervals {
		return nil, fail("PARSE_GROUP_LIMIT", refLine, "service object-group resolves to too many port intervals")
	}
	return out, nil
}

func (g *groupTable) serviceIntervals(name string, refLine, depth int, visiting map[string]bool) ([]Interval, error) {
	grp, ok := g.service[name]
	if !ok {
		return nil, fail("PARSE_GROUP_UNDEFINED", refLine, "undefined service object-group")
	}
	if visiting[name] {
		return nil, fail("PARSE_GROUP_CYCLE", grp.line, "service object-group reference cycle")
	}
	if depth > MaxGroupDepth {
		return nil, fail("PARSE_GROUP_LIMIT", grp.line, "service object-group nesting too deep")
	}
	visiting[name] = true
	defer delete(visiting, name)
	out := []Interval{}
	for _, m := range grp.members {
		if m.group == "" {
			out = append(out, m.intervals...)
			continue
		}
		sub, err := g.serviceIntervals(m.group, m.line, depth+1, visiting)
		if err != nil {
			return nil, err
		}
		out = append(out, sub...)
		if len(out) > MaxGroupMembers {
			return nil, fail("PARSE_GROUP_LIMIT", grp.line, "service object-group membership too large")
		}
	}
	return out, nil
}

// groupNameOK applies the ACL name grammar to group names.
func groupNameOK(name string) bool {
	return aclName.MatchString(name) && !strings.EqualFold(name, "any")
}

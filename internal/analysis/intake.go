// Package analysis is the domain-neutral review layer: it reads configuration
// versions, inventories analyzable objects, dispatches each object to its
// domain engine and seals one versioned artifact. It never infers that an
// unanalyzed part of a configuration is unchanged or safe.
package analysis

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/prefixlist"
)

const (
	IntakeVersion  = "plenilune-intake/v0.1"
	MaxSourceBytes = 4 << 20
	MaxSourceLines = 200_000
	maxReferences  = 20
)

// Domain identifiers. A domain owns its parser, semantics and verdicts.
const (
	DomainACL        = "acl"
	DomainPrefixList = "prefix-list"
)

type line struct {
	number     int
	text       string
	start, end int
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

func splitLines(data []byte) []line {
	out := []line{}
	start := 0
	for n := 1; start < len(data); n++ {
		end := bytes.IndexByte(data[start:], '\n')
		next := len(data)
		if end >= 0 {
			next = start + end + 1
		}
		text := strings.TrimRight(string(data[start:next]), "\r\n")
		out = append(out, line{number: n, text: text, start: start, end: next})
		start = next
	}
	return out
}

func indented(s string) bool { return s != "" && (s[0] == ' ' || s[0] == '\t') }

func noise(s string) bool {
	t := strings.TrimSpace(s)
	return t == "" || strings.HasPrefix(t, "!") || strings.EqualFold(t, "end")
}

// section is one top-level line with the indented lines that follow it.
type section struct {
	header line
	body   []line
}

func (s section) lines() []line { return append([]line{s.header}, s.body...) }

func (s section) text() string {
	parts := []string{}
	for _, l := range s.lines() {
		parts = append(parts, strings.TrimRight(l.text, " \t"))
	}
	return strings.Join(parts, "\n")
}

func sections(lines []line) ([]section, []line) {
	out, orphans := []section{}, []line{}
	for _, l := range lines {
		if noise(l.text) {
			continue
		}
		if indented(l.text) {
			if len(out) == 0 {
				orphans = append(orphans, l)
				continue
			}
			out[len(out)-1].body = append(out[len(out)-1].body, l)
			continue
		}
		out = append(out, section{header: l})
	}
	return out, orphans
}

// Extent locates one object in its source. ByteStart/ByteEnd are present only
// when the object is one contiguous block whose bytes are passed to its engine.
type Extent struct {
	Lines     [][2]int `json:"lines"`
	ByteStart *int     `json:"byte_start,omitempty"`
	ByteEnd   *int     `json:"byte_end,omitempty"`
	SHA256    string   `json:"sha256"`
	Units     int      `json:"units"`
}

type Reference struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type ObjectError struct {
	Side    string `json:"side"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}

// found is one side's view of an object before pairing.
type found struct {
	domain, name, kind string
	extent             Extent
	refs               []Reference
	err                *ObjectError
	acl                *ciscoacl.ACL
	aclBytes           []byte
	raw                []byte
	prefix             *prefixlist.List
	canonical          string
	lineCount          int
}

type inventory struct {
	objects    []*found
	byKey      map[string]*found
	unanalyzed []section
	orphans    []line
	lines      int
	noise      int
	bom        bool
}

// key separates IPv4 and IPv6 namespaces, which IOS keeps distinct.
func key(domain, kind, name string) string {
	family := "ipv4"
	if kind == "ipv6" {
		family = "ipv6"
	}
	return domain + "\x00" + family + "\x00" + name
}

// grouped kinds are defined by many top-level lines sharing one name.
func grouped(domain, kind string) bool {
	return domain == DomainPrefixList || kind == "numbered"
}

func rangesOf(ls []line) [][2]int {
	out := [][2]int{}
	for _, l := range ls {
		if n := len(out); n > 0 && out[n-1][1]+1 == l.number {
			out[n-1][1] = l.number
			continue
		}
		out = append(out, [2]int{l.number, l.number})
	}
	return out
}

func fields(s string) []string { return strings.Fields(s) }

func lower(f []string, i int) string {
	if i < len(f) {
		return strings.ToLower(f[i])
	}
	return ""
}

// classify names the object a top-level header defines, if any.
func classify(f []string) (domain, name, kind string) {
	switch {
	case lower(f, 0) == "ip" && lower(f, 1) == "access-list" && lower(f, 2) == "extended" && len(f) == 4:
		return DomainACL, f[3], "named-extended"
	case lower(f, 0) == "ip" && lower(f, 1) == "access-list" && lower(f, 2) == "standard" && len(f) == 4:
		return DomainACL, f[3], "named-standard"
	case lower(f, 0) == "ipv6" && lower(f, 1) == "access-list" && len(f) >= 3:
		return DomainACL, f[2], "ipv6"
	case lower(f, 0) == "access-list" && len(f) >= 2:
		return DomainACL, f[1], "numbered"
	case lower(f, 0) == "ipv6" && lower(f, 1) == "prefix-list" && len(f) >= 3 && lower(f, 2) != "sequence-number":
		return DomainPrefixList, f[2], "ipv6"
	}
	if name, ok := prefixlist.IsDefinition(strings.Join(f, " ")); ok {
		return DomainPrefixList, name, "ipv4"
	}
	return "", "", ""
}

func sectionKind(f []string) string {
	switch lower(f, 0) {
	case "ip", "ipv6", "no", "snmp-server", "logging", "spanning-tree":
		if len(f) > 1 {
			return lower(f, 0) + " " + lower(f, 1)
		}
	}
	return lower(f, 0)
}

func intake(data []byte) inventory { return intakeWithParser(data, prefixlist.ParserVersion) }
func intakeWithParser(data []byte, parser string) inventory {
	inv := inventory{byKey: map[string]*found{}}
	lines := splitLines(data)
	inv.lines = len(lines)
	inv.bom = bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	for _, l := range lines {
		if noise(l.text) {
			inv.noise++
		}
	}
	add := func(f *found) *found {
		k := key(f.domain, f.kind, f.name)
		if prev, ok := inv.byKey[k]; ok {
			return prev
		}
		inv.byKey[k] = f
		inv.objects = append(inv.objects, f)
		return f
	}
	// A document that is exactly one supported ACL keeps its full bytes, so
	// digests and replay match single-ACL files used before configuration intake.
	if acl, err := ciscoacl.Parse(data); err == nil {
		start, end := 0, len(data)
		f := add(&found{domain: DomainACL, name: acl.Name, kind: acl.Kind, acl: &acl, aclBytes: data, raw: data, extent: Extent{Lines: [][2]int{{1, len(lines)}}, ByteStart: &start, ByteEnd: &end, Units: len(acl.Rules)}, canonical: string(data), lineCount: len(lines) - inv.noise})
		f.extent.SHA256 = digest(data)
		return inv
	}
	secs, orphans := sections(lines)
	inv.orphans = orphans
	groupLines := map[string][]line{}
	for _, s := range secs {
		f := fields(s.header.text)
		domain, name, kind := classify(f)
		if domain == "" {
			inv.unanalyzed = append(inv.unanalyzed, s)
			continue
		}
		k := key(domain, kind, name)
		if grouped(domain, kind) {
			add(&found{domain: domain, name: name, kind: kind})
			groupLines[k] = append(groupLines[k], s.lines()...)
			continue
		}
		if existing, dup := inv.byKey[k]; dup {
			if existing.err == nil {
				existing.err = &ObjectError{Code: "DUPLICATE_DEFINITION", Message: "object is defined more than once; device merge behavior is not modeled", Line: s.header.number}
			}
			last := s.lines()[len(s.lines())-1]
			block := data[s.header.start:last.end]
			existing.raw = append(existing.raw, block...)
			existing.extent.Lines = append(existing.extent.Lines, rangesOf(s.lines())...)
			existing.extent.ByteStart, existing.extent.ByteEnd = nil, nil
			existing.extent.SHA256, existing.extent.Units = digest(existing.raw), 0
			existing.lineCount += len(s.lines())
			continue
		}
		obj := add(&found{domain: domain, name: name, kind: kind, extent: Extent{Lines: rangesOf(s.lines())}, lineCount: len(s.lines())})
		last := s.lines()[len(s.lines())-1]
		start, end := s.header.start, last.end
		block := data[start:end]
		obj.raw = block
		obj.extent.ByteStart, obj.extent.ByteEnd, obj.extent.SHA256 = &start, &end, digest(block)
		if kind != "named-extended" {
			obj.err = &ObjectError{Code: "UNSUPPORTED_KIND", Message: "this object kind is recognized but not analyzed", Line: s.header.number}
			continue
		}
		obj.aclBytes, obj.canonical = block, string(block)
		acl, err := ciscoacl.Parse(block)
		if err != nil {
			obj.err = aclError(err, s.header.number)
			continue
		}
		obj.acl, obj.extent.Units = &acl, len(acl.Rules)
	}
	for _, obj := range inv.objects {
		if !grouped(obj.domain, obj.kind) {
			continue
		}
		ls := groupLines[key(obj.domain, obj.kind, obj.name)]
		src := []prefixlist.SourceLine{}
		canonical := []string{}
		var raw []byte
		for _, l := range ls {
			src = append(src, prefixlist.SourceLine{Number: l.number, Text: l.text})
			canonical = append(canonical, l.text)
			raw = append(raw, data[l.start:l.end]...)
		}
		obj.extent.Lines, obj.lineCount = rangesOf(ls), len(ls)
		obj.canonical = strings.Join(canonical, "\n")
		obj.extent.SHA256 = digest(raw)
		if obj.kind != "ipv4" {
			obj.err = &ObjectError{Code: "UNSUPPORTED_KIND", Message: "this object kind is recognized but not analyzed", Line: ls[0].number}
			continue
		}
		list, err := prefixlist.ParseLinesVersion(obj.name, src, parser)
		if err != nil {
			obj.err = prefixError(err)
			continue
		}
		obj.prefix, obj.extent.Units = &list, len(list.Entries)
	}
	collectReferences(lines, inv)
	return inv
}

func aclError(err error, headerLine int) *ObjectError {
	if e, ok := err.(*ciscoacl.Error); ok {
		out := &ObjectError{Code: e.Code, Message: e.Message}
		if e.Line > 0 {
			out.Line = headerLine + e.Line - 1
		}
		return out
	}
	return &ObjectError{Code: "PARSE_FAILED", Message: "ACL parse failed", Line: headerLine}
}

func prefixError(err error) *ObjectError {
	if e, ok := err.(*prefixlist.Error); ok {
		return &ObjectError{Code: e.Code, Message: e.Message, Line: e.Line}
	}
	return &ObjectError{Code: "PARSE_FAILED", Message: "prefix-list parse failed"}
}

// collectReferences records where objects are used. References are structural
// context; Plenilune does not model interface or routing binding semantics.
func collectReferences(lines []line, inv inventory) {
	note := func(domain, kind, name string, l line) {
		if obj, ok := inv.byKey[key(domain, kind, name)]; ok && len(obj.refs) < maxReferences && (len(obj.refs) == 0 || obj.refs[len(obj.refs)-1].Line != l.number) {
			obj.refs = append(obj.refs, Reference{Line: l.number, Text: strings.TrimSpace(l.text)})
		}
	}
	for _, l := range lines {
		f := fields(l.text)
		if d, _, _ := classify(f); d != "" && !indented(l.text) {
			continue
		}
		for i, tok := range f {
			t := strings.ToLower(tok)
			if (t == "access-group" || t == "access-class") && i+1 < len(f) {
				kind := "named-extended"
				if lower(f, 0) == "ipv6" {
					kind = "ipv6"
				}
				note(DomainACL, kind, f[i+1], l)
				note(DomainACL, "numbered", f[i+1], l)
			}
			if t == "prefix-list" && i+1 < len(f) {
				if i >= 2 && lower(f, i-2) == "match" {
					for _, name := range f[i+1:] {
						note(DomainPrefixList, "ipv4", name, l)
					}
				} else {
					note(DomainPrefixList, "ipv4", f[i+1], l)
				}
			}
			if t == "prefix" && i >= 1 && lower(f, i-1) == "distribute-list" && i+1 < len(f) {
				note(DomainPrefixList, "ipv4", f[i+1], l)
			}
		}
	}
}

func validText(data []byte) bool { return utf8.Valid(data) }

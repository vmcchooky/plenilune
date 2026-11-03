package analysis

import (
	"plenilune/internal/ciscoacl"
	"plenilune/internal/ciscoacl6"
	"strconv"
	"strings"
)

func intakeFormat(data []byte, prefixParser string, legacyText bool) inventory {
	return intakeVersions(data, ciscoacl.ParserVersion, prefixParser, ciscoacl6.ParserVersion, legacyText)
}

// Legacy intake remains frozen. Parser3 upgrades only standard definitions;
// other domains and their historical source/coverage behavior stay with it.
func intakeVersions(data []byte, aclParser, prefixParser, acl6Parser string, legacyText bool) inventory {
	inv := intakeLegacy(data, prefixParser, legacyText)
	if aclParser == ciscoacl.LegacyParserVersion {
		return inv
	}
	if acl, err := ciscoacl.ParseVersion(data, aclParser); err == nil && acl.Kind == "standard" {
		start, end := 0, len(data)
		obj := &found{domain: DomainACL, name: acl.Name, kind: acl.Kind, acl: &acl,
			aclBytes: data, raw: data, canonical: string(data), lineCount: inv.lines - inv.noise,
			extent: Extent{Lines: [][2]int{{1, inv.lines}}, ByteStart: &start, ByteEnd: &end, SHA256: digest(data), Units: len(acl.Rules)}}
		inv.objects, inv.byKey = []*found{obj}, map[string]*found{key(DomainACL, acl.Kind, acl.Name): obj}
		inv.unanalyzed, inv.orphans = nil, nil
		return inv
	}
	type definitions struct {
		rows               []line
		named, extended    int
		numbered, standard bool
	}
	groups := map[string]*definitions{}
	secs, _ := sections(splitLines(data))
	for _, s := range secs {
		domain, name, kind := classify(fields(s.header.text))
		if domain != DomainACL || kind == "ipv6" {
			continue
		}
		g := groups[name]
		if g == nil {
			g = &definitions{}
			groups[name] = g
		}
		g.rows = append(g.rows, s.lines()...)
		switch kind {
		case "named-standard":
			g.named++
			g.standard = true
		case "named-extended":
			g.extended++
		case "numbered":
			if standardNumber(name) {
				g.numbered, g.standard = true, true
			}
		}
	}
	for name, g := range groups {
		if !g.standard {
			continue
		}
		obj := inv.byKey[key(DomainACL, "standard", name)]
		if obj == nil {
			continue
		} // all recognized definitions are inventoried by legacy intake
		var raw []byte
		for _, row := range g.rows {
			raw = append(raw, data[row.start:row.end]...)
		}
		obj.kind, obj.acl, obj.aclBytes, obj.err = "standard", nil, nil, nil
		obj.raw, obj.canonical, obj.lineCount = raw, string(raw), len(g.rows)
		obj.extent = Extent{Lines: rangesOf(g.rows), SHA256: digest(raw)}
		if g.named > 1 || g.extended > 0 || g.named > 0 && g.numbered {
			obj.err = &ObjectError{Code: "DUPLICATE_DEFINITION", Message: "mixed or repeated ACL definitions are not modeled", Line: g.rows[0].number}
			continue
		}
		// A named block retains intervening comment/blank bytes. Numbered
		// definitions concatenate exactly their selected source lines.
		if g.named == 1 || len(obj.extent.Lines) == 1 {
			start, end := g.rows[0].start, g.rows[len(g.rows)-1].end
			raw = data[start:end]
			obj.extent.ByteStart, obj.extent.ByteEnd = &start, &end
			obj.extent.SHA256 = digest(raw)
			obj.raw, obj.canonical = raw, string(raw)
		}
		acl, err := ciscoacl.ParseVersion(raw, aclParser)
		if err != nil {
			obj.err = aclError(err, g.rows[0].number)
			if obj.extent.ByteStart == nil {
				if e, ok := err.(*ciscoacl.Error); ok && e.Line > 0 && e.Line <= len(g.rows) {
					obj.err.Line = g.rows[e.Line-1].number
				}
			}
			continue
		}
		obj.acl, obj.aclBytes, obj.extent.Units = &acl, raw, len(acl.Rules)
	}
	upgradeGroupACLs(data, inv, aclParser)
	upgradeIPv6ACLs(inv, acl6Parser)
	return inv
}

// upgradeGroupACLs re-parses named-extended ACL objects with the v0.4
// group-aware parser when the document carries object-group definitions.
// The legacy pass parses an ACL block in isolation, so group references
// fail there; the upgrade splices the document's group sections together
// with the ACL's own section (all other lines blanked, so line numbers
// stay absolute) and parses that. Group-free documents and pre-v0.4
// artifact replays are untouched. An object's effective source — and
// therefore its digest — becomes the spliced text: a group-only edit is a
// real change to the object and must not digest-identical to the old one.
func upgradeGroupACLs(data []byte, inv inventory, aclParser string) {
	if aclParser != ciscoacl.GroupParserVersion {
		return
	}
	lines := splitLines(data)
	secs, _ := sections(lines)
	var groupRanges [][2]int
	for _, s := range secs {
		if lower(fields(s.header.text), 0) == "object-group" {
			groupRanges = append(groupRanges, rangesOf(s.lines())...)
		}
	}
	if len(groupRanges) == 0 {
		return
	}
	for _, obj := range inv.objects {
		if obj.domain != DomainACL || obj.kind != "named-extended" {
			continue
		}
		if obj.err != nil && obj.err.Code == "DUPLICATE_DEFINITION" {
			continue
		}
		include := map[int]bool{}
		var objRanges [][2]int
		for _, r := range append(append([][2]int{}, groupRanges...), obj.extent.Lines...) {
			for n := r[0]; n <= r[1]; n++ {
				include[n] = true
			}
			objRanges = append(objRanges, r)
		}
		last := 0
		for n := range include {
			if n > last {
				last = n
			}
		}
		parts := make([]string, 0, last)
		byNumber := map[int]string{}
		for _, l := range lines {
			byNumber[l.number] = l.text
		}
		for n := 1; n <= last; n++ {
			if include[n] {
				parts = append(parts, byNumber[n])
			} else {
				parts = append(parts, "")
			}
		}
		spliced := []byte(strings.Join(parts, "\n"))
		acl, err := ciscoacl.ParseVersion(spliced, aclParser)
		if err != nil {
			obj.err = aclError(err, 1)
			if e, ok := err.(*ciscoacl.Error); ok && e.Line > 0 {
				obj.err.Line = e.Line // spliced text is document-aligned
			}
			obj.acl = nil
			continue
		}
		obj.err = nil
		obj.acl, obj.aclBytes = &acl, spliced
		obj.raw, obj.canonical = spliced, string(spliced)
		obj.lineCount = len(include)
		obj.extent = Extent{Lines: objRanges, SHA256: digest(spliced), Units: len(acl.Rules)}
	}
}

// upgradeIPv6ACLs parses IPv6 ACL objects with the ciscoacl6 parser when
// the run's recorded versions carry a v6 parser identity (P3-7). Runs
// without one — every artifact sealed before P3-7 — keep the legacy
// outcome (ipv6 sections recognized, marked unsupported) byte-for-byte.
func upgradeIPv6ACLs(inv inventory, acl6Parser string) {
	if acl6Parser == "" {
		return
	}
	for _, obj := range inv.objects {
		if obj.domain != DomainACL || obj.kind != "ipv6" {
			continue
		}
		if obj.err != nil && obj.err.Code == "DUPLICATE_DEFINITION" {
			continue
		}
		headerLine := 0
		if len(obj.extent.Lines) > 0 {
			headerLine = obj.extent.Lines[0][0]
		}
		acl, err := ciscoacl6.Parse(obj.raw)
		if err != nil {
			obj.err = acl6Error(err, headerLine)
			obj.acl6 = nil
			continue
		}
		obj.err = nil
		obj.acl6 = &acl
		obj.extent.Units = len(acl.Rules)
		// The legacy pass leaves canonical empty for kinds it does not
		// analyze, which would make every pair "unchanged"; the block text
		// is the conservative identity (any textual difference goes to the
		// engine, which decides semantic equivalence).
		obj.canonical = string(obj.raw)
	}
}

func standardNumber(name string) bool {
	n, err := strconv.Atoi(name)
	return err == nil && strconv.Itoa(n) == name && (n >= 1 && n <= 99 || n >= 1300 && n <= 1999)
}

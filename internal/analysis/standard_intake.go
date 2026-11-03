package analysis

import (
	"plenilune/internal/ciscoacl"
	"strconv"
)

func intakeFormat(data []byte, prefixParser string, legacyText bool) inventory {
	return intakeVersions(data, ciscoacl.ParserVersion, prefixParser, legacyText)
}

// Legacy intake remains frozen. Parser3 upgrades only standard definitions;
// other domains and their historical source/coverage behavior stay with it.
func intakeVersions(data []byte, aclParser, prefixParser string, legacyText bool) inventory {
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
	return inv
}

func standardNumber(name string) bool {
	n, err := strconv.Atoi(name)
	return err == nil && strconv.Itoa(n) == name && (n >= 1 && n <= 99 || n >= 1300 && n <= 1999)
}

package patch

import (
	"strconv"
	"strings"
)

type sourceLine struct{ text, eol string }

func splitLines(source string) ([]sourceLine, string) {
	parts := strings.SplitAfter(source, "\n")
	out := make([]sourceLine, 0, len(parts))
	preferred := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		line := sourceLine{text: part}
		if strings.HasSuffix(part, "\r\n") {
			line.text = part[:len(part)-2]
			line.eol = "\r\n"
		} else if strings.HasSuffix(part, "\n") {
			line.text = part[:len(part)-1]
			line.eol = "\n"
		}
		if preferred == "" && line.eol != "" {
			preferred = line.eol
		}
		out = append(out, line)
	}
	if preferred == "" {
		preferred = "\n"
	}
	return out, preferred
}

func indent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// Source sequence numbers are presentation metadata. Reassign them in final
// entry order while leaving unsequenced remarks and other source lines intact.
func renumber(lines []sourceLine) {
	next := 10
	for i := range lines {
		prefix := indent(lines[i].text)
		if prefix == "" {
			continue
		}
		body := strings.TrimLeft(lines[i].text, " \t")
		first, rest, found := splitToken(body)
		sequenced := digits(first)
		if sequenced {
			if !found {
				continue
			}
			body = strings.TrimLeft(rest, " \t")
			first, _, _ = splitToken(body)
		}
		if strings.EqualFold(first, "permit") || strings.EqualFold(first, "deny") || sequenced && strings.EqualFold(first, "remark") {
			lines[i].text = prefix + strconv.Itoa(next) + " " + body
			next += 10
		}
	}
}

func splitToken(s string) (string, string, bool) {
	for i, c := range s {
		if c == ' ' || c == '\t' {
			return s[:i], s[i:], true
		}
	}
	return s, "", false
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

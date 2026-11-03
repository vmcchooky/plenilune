package patch

import (
	"sort"
	"strconv"
	"strings"

	"plenilune/internal/ciscoacl"
)

// The source was already validated. Track sequence rank without moving any
// original line: physical-order resequencing would change standard networks.
func standardLineOrders(lines []sourceLine, acl ciscoacl.ACL) (bool, int64) {
	f := strings.Fields(lines[acl.Rules[0].Line-1].text)
	if len(f) > 0 && strings.EqualFold(f[0], "access-list") {
		return true, 0
	}
	var highest uint64
	for i := range lines {
		if indent(lines[i].text) == "" {
			continue
		}
		f := strings.Fields(lines[i].text)
		if len(f) == 0 {
			continue
		}
		sequence := highest + 10
		if digits(f[0]) {
			sequence, _ = strconv.ParseUint(f[0], 10, 32)
			f = f[1:]
		}
		if len(f) == 0 || !strings.EqualFold(f[0], "permit") && !strings.EqualFold(f[0], "deny") && !strings.EqualFold(f[0], "remark") {
			continue
		}
		highest = max(highest, sequence)
		lines[i].order, lines[i].entry = int64(sequence)*8, true
	}
	return false, int64(highest) * 8
}

// Eight rank slots leave space for each bounded edit; only one edit can target
// a base rule. Assign explicit final sequences to entries including remarks so
// implicit append numbering cannot collide after insertion/removal.
func renumberStandard(lines []sourceLine) {
	var entries []int
	for i, line := range lines {
		if line.entry {
			entries = append(entries, i)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return lines[entries[i]].order < lines[entries[j]].order })
	for rank, index := range entries {
		prefix := indent(lines[index].text)
		body := strings.TrimLeft(lines[index].text, " \t")
		first, rest, found := splitToken(body)
		if digits(first) && found {
			body = strings.TrimLeft(rest, " \t")
		}
		lines[index].text = prefix + strconv.Itoa(10*(rank+1)) + " " + body
	}
}

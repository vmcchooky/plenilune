package prefixlist

// Frozen v0.1 execution/accounting: replay only. New analysis uses v0.2.
import (
	"context"
	"errors"
	"slices"
)

func partitionLegacy(ctx context.Context, l List, ops *int) ([33][]segment, error) {
	var out [33][]segment
	for length := 0; length <= 32; length++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if *ops > maxOperations {
			return out, errBudget
		}
		size := uint64(1) << uint(length)
		covered := []segment{}
		for i, e := range l.Entries {
			*ops++
			if *ops > maxOperations {
				return out, errBudget
			}
			if uint8(length) < e.Lo || uint8(length) > e.Hi {
				continue
			}
			shift := uint(32 - length)
			lo := uint64(e.Network) >> shift
			hi := uint64(e.Network|^mask(e.Len)) >> shift
			if length == 0 {
				lo, hi = 0, 0
			}
			var err error
			covered, err = insertUncoveredLegacy(ctx, covered, lo, hi, i, ops)
			if err != nil {
				return out, err
			}
		}
		// Fill gaps with the implicit deny so each length is a full partitionLegacy.
		full := []segment{}
		next := uint64(0)
		for _, s := range covered {
			if s.Lo > next {
				full = append(full, segment{Len: uint8(length), Lo: next, Hi: s.Lo - 1, Owner: -1})
			}
			s.Len = uint8(length)
			full = append(full, s)
			next = s.Hi + 1
		}
		if next < size {
			full = append(full, segment{Len: uint8(length), Lo: next, Hi: size - 1, Owner: -1})
		}
		out[length] = full
	}
	return out, nil
}

// insertUncoveredLegacy adds the parts of [lo,hi] not already covered, owned by
// entry i, keeping covered sorted and disjoint (earlier entries win). Both
// inputs are sorted; the merge is linear and accounted.

func insertUncoveredLegacy(ctx context.Context, covered []segment, lo, hi uint64, owner int, ops *int) ([]segment, error) {
	add := []segment{}
	cursor := lo
	at, _ := slices.BinarySearchFunc(covered, lo, func(s segment, v uint64) int {
		if s.Hi < v {
			return -1
		}
		return 1
	})
	for j := at; j < len(covered) && cursor <= hi; j++ {
		*ops++
		if *ops > maxOperations {
			return nil, errBudget
		}
		s := covered[j]
		if s.Lo > hi {
			break
		}
		if s.Lo > cursor {
			add = append(add, segment{Lo: cursor, Hi: s.Lo - 1, Owner: owner})
		}
		if s.Hi >= hi {
			cursor = hi + 1
			break
		}
		cursor = s.Hi + 1
	}
	if cursor <= hi {
		add = append(add, segment{Lo: cursor, Hi: hi, Owner: owner})
	}
	if len(add) == 0 {
		return covered, nil
	}
	out := make([]segment, 0, len(covered)+len(add))
	i, j := 0, 0
	for i < len(covered) || j < len(add) {
		*ops++
		if *ops > maxOperations {
			return nil, errBudget
		}
		if j >= len(add) || (i < len(covered) && covered[i].Lo <= add[j].Lo) {
			out = append(out, covered[i])
			i++
		} else {
			out = append(out, add[j])
			j++
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Range is an exact run of same-length routes from First to Last inclusive.

func refLegacy(l List, owner int) EntryRef {
	if owner < 0 {
		return EntryRef{Action: "deny", Implicit: true}
	}
	e := l.Entries[owner]
	seq, line := e.Seq, e.Line
	return EntryRef{Action: e.Action, Seq: &seq, Line: &line, Text: e.Text}
}

func actionOfLegacy(l List, owner int) string {
	if owner < 0 {
		return "deny"
	}
	return l.Entries[owner].Action
}

func routeAtLegacy(length uint8, n uint64) Route {
	if length == 0 {
		return Route{}
	}
	return Route{Network: uint32(n << uint(32-length)), Len: length}
}

// CompareLegacy decides the full route space exactly: for every prefix length the
// two first-match partitions are swept together. Each class witness is
// re-evaluated with the linear evaluator and must name the class's entries.

func compareLegacy(ctx context.Context, before, after List, maxClasses int) (Report, error) {
	if maxClasses < 1 || maxClasses > MaxClasses {
		return Report{}, errors.New("max classes must be 1 to 128")
	}
	if before.Name != after.Name {
		return Report{}, errors.New("prefix-list identity mismatch")
	}
	if len(before.Entries) > MaxEntries || len(after.Entries) > MaxEntries {
		return Report{}, errors.New("prefix-list exceeds entry limit")
	}
	r := Report{Verdict: "INCONCLUSIVE", Classes: []Class{}, MaxClasses: maxClasses}
	ops := 0
	bp, err := partitionLegacy(ctx, before, &ops)
	if errors.Is(err, errBudget) {
		r.Reason, r.Operations = "operations", ops
		return r, nil
	}
	if err != nil {
		return Report{}, err
	}
	ap, err := partitionLegacy(ctx, after, &ops)
	if errors.Is(err, errBudget) {
		r.Reason, r.Operations = "operations", ops
		return r, nil
	}
	if err != nil {
		return Report{}, err
	}
	groups, keys, stop, err := sweepLegacy(ctx, before, after, bp, ap, &ops)
	if err != nil {
		return Report{}, err
	}
	if stop != "" {
		r.Reason, r.Operations = stop, ops
		return r, nil
	}
	order := func(n int) int {
		if n < 0 {
			return 1 << 30
		}
		return n
	}
	slices.SortFunc(keys, func(a, b key) int {
		if order(a.before) != order(b.before) {
			return order(a.before) - order(b.before)
		}
		return order(a.after) - order(b.after)
	})
	r.TotalClasses = len(keys)
	for _, k := range keys {
		if len(r.Classes) >= maxClasses {
			r.ClassesTruncated = true
			break
		}
		segs := mergeAdjacentLegacy(groups[k])
		route := routeAtLegacy(segs[0].Len, segs[0].Lo)
		x, err := before.evaluateUnchecked(route)
		if err != nil {
			return Report{}, err
		}
		y, err := after.evaluateUnchecked(route)
		if err != nil {
			return Report{}, err
		}
		if x.Action == y.Action || !decidedByLegacy(x, before, k.before) || !decidedByLegacy(y, after, k.after) {
			return Report{}, errors.New("prefix-list class failed independent evaluation")
		}
		c := Class{Before: refLegacy(before, k.before), After: refLegacy(after, k.after), Witness: Witness{Route: route, Before: x, After: y}, Lengths: []int{}, Ranges: []Range{}, TotalRanges: len(segs)}
		c.Effect = "newly_denied"
		if c.After.Action == "permit" {
			c.Effect = "newly_permitted"
		}
		for _, s := range segs {
			n := s.Hi - s.Lo + 1
			c.Routes += n
			if len(c.Lengths) == 0 || c.Lengths[len(c.Lengths)-1] != int(s.Len) {
				c.Lengths = append(c.Lengths, int(s.Len))
			}
			if len(c.Ranges) < maxExamples {
				c.Ranges = append(c.Ranges, Range{Length: s.Len, First: routeAtLegacy(s.Len, s.Lo), Last: routeAtLegacy(s.Len, s.Hi), Routes: n})
			} else {
				c.RangesTruncated = true
			}
		}
		r.Classes = append(r.Classes, c)
	}
	r.Complete, r.Operations = true, ops
	r.Verdict = "EQUIVALENT"
	if len(keys) > 0 {
		r.Verdict = "DIFFERENT"
	}
	return r, nil
}

func sweepLegacy(ctx context.Context, before, after List, bp, ap [33][]segment, ops *int) (map[key][]segment, []key, string, error) {
	groups := map[key][]segment{}
	keys := []key{}
	segments := 0
	for length := 0; length <= 32; length++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, "", err
		}
		x, y := bp[length], ap[length]
		i, j := 0, 0
		for i < len(x) && j < len(y) {
			*ops++
			if *ops > maxOperations {
				return nil, nil, "operations", nil
			}
			lo, hi := max(x[i].Lo, y[j].Lo), min(x[i].Hi, y[j].Hi)
			if actionOfLegacy(before, x[i].Owner) != actionOfLegacy(after, y[j].Owner) {
				k := key{x[i].Owner, y[j].Owner}
				if _, ok := groups[k]; !ok {
					keys = append(keys, k)
				}
				groups[k] = append(groups[k], segment{Len: uint8(length), Lo: lo, Hi: hi})
				segments++
				if segments > maxSegments {
					return nil, nil, "segments", nil
				}
			}
			if x[i].Hi == hi {
				i++
			}
			if y[j].Hi == hi {
				j++
			}
		}
	}
	return groups, keys, "", nil
}

func decidedByLegacy(d Decision, l List, owner int) bool {
	if owner < 0 {
		return d.Implicit
	}
	return !d.Implicit && d.Seq != nil && *d.Seq == l.Entries[owner].Seq
}

func mergeAdjacentLegacy(segs []segment) []segment {
	out := []segment{}
	for _, s := range segs {
		if n := len(out); n > 0 && out[n-1].Len == s.Len && out[n-1].Hi+1 == s.Lo {
			out[n-1].Hi = s.Hi
			continue
		}
		out = append(out, s)
	}
	return out
}

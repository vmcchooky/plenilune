package verify

// P1-1 baseline measurement harness (measurement only; no engine changes).
//
// Reuses the fixed v0.2 corpus generator (corpusACL/corpusCases in
// measure_test.go) so numbers are directly comparable with
// docs/research/engine-v02.json, and adds the two cases the P1-1 plan item
// registers for the LCP gate but the v0.2 corpus lacks:
//
//   - append_rule:  one new permit rule inserted immediately before the
//     final "deny ip any any" (operator-style append). At base size 512 the
//     after side would hold 513 rules and exceeds the parser cap
//     (ciscoacl.MaxRules = 512), so the largest append measurement uses a
//     511-rule base whose after side is exactly 512 rules; that row is
//     recorded under rules=511 with case "append_rule_to_512".
//   - change_last:  the action (permit<->deny) of the last generated rule
//     (index n-2, immediately before the final deny) is flipped. Rule count
//     is unchanged, so this case is measurable at every corpus size.
//
// Cases whose after side cannot be parsed (insert_top_deny and shadow_all
// at 512 rules -> 513 rules) are recorded as explicit NOT_MEASURED rows with
// the parse error, instead of being silently skipped as TestMeasureCorpus
// does.
//
// Runs only on explicit request:
//
//	PLENILUNE_P11_OUT=<path.json> go test ./internal/verify/ -run '^TestP11Baseline$' -count=1 -v
//
// Metrics per case: the engine's own budget counters from the Compare
// report (Budget.Operations = engine operation count charged against the
// 2M cap; Budget.PeakBoxes = peak live boxes charged against the 50k cap),
// the verdict + SearchComplete + Budget.StopReason, wall time of each of 5
// runs (median/min/max recorded), and allocated MiB per run measured the
// same way as the v0.2 harness (runtime MemStats TotalAlloc delta / runs).

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"plenilune/internal/ciscoacl"
)

type p11Row struct {
	Rules        int       `json:"rules"`
	Case         string    `json:"case"`
	Verdict      string    `json:"verdict"`
	Complete     bool      `json:"search_complete"`
	StopReason   string    `json:"stop_reason"`
	Operations   int       `json:"operations"`
	OpsAllRuns   []int     `json:"ops_all_runs"`
	PeakBoxes    int       `json:"peak_boxes"`
	Witnesses    int       `json:"witnesses"`
	RunMillis    []float64 `json:"run_ms"`
	MedianMillis float64   `json:"median_ms"`
	MinMillis    float64   `json:"min_ms"`
	MaxMillis    float64   `json:"max_ms"`
	AllocatedMiB float64   `json:"allocated_mib_per_run"`
	Note         string    `json:"note,omitempty"`
}

func p11Cases(n int) []corpusCase {
	cases := corpusCases(n)
	base := corpusACL(n, uint64(n))

	// append_rule: new permit inserted immediately before the final deny.
	appended := append([]string(nil), base[:len(base)-1]...)
	appended = append(appended,
		"permit tcp host 192.0.2.200 host 203.0.113.200 eq 8443",
		base[len(base)-1])
	cases = append(cases, corpusCase{name: "append_rule", before: base, after: appended})

	// change_last: flip the action of the last generated rule (index n-2).
	changed := append([]string(nil), base...)
	last := changed[len(changed)-2]
	if strings.HasPrefix(last, "permit ") {
		changed[len(changed)-2] = "deny " + strings.TrimPrefix(last, "permit ")
	} else {
		changed[len(changed)-2] = "permit " + strings.TrimPrefix(last, "deny ")
	}
	cases = append(cases, corpusCase{name: "change_last", before: base, after: changed})
	return cases
}

func millis(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func TestP11Baseline(t *testing.T) {
	out := os.Getenv("PLENILUNE_P11_OUT")
	if out == "" {
		t.Skip("set PLENILUNE_P11_OUT to record P1-1 baseline measurements")
	}
	var rows []p11Row
	measure := func(n int, c corpusCase, note string) {
		row := p11Row{Rules: n, Case: c.name, Note: note}
		before, err := ciscoacl.Parse(render("CORPUS", c.before))
		if err != nil {
			row.Verdict = "NOT_MEASURED"
			row.Note = strings.TrimSpace(note + " before parse: " + err.Error())
			rows = append(rows, row)
			return
		}
		after, err := ciscoacl.Parse(render("CORPUS", c.after))
		if err != nil {
			row.Verdict = "NOT_MEASURED"
			row.Note = strings.TrimSpace(note + " after parse: " + err.Error())
			rows = append(rows, row)
			return
		}
		var times []time.Duration
		var report Report
		var ms runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms)
		startAlloc := ms.TotalAlloc
		for i := 0; i < 5; i++ {
			start := time.Now()
			report, err = Compare(context.Background(), before, after, nil, DefaultOptions())
			times = append(times, time.Since(start))
			if err != nil {
				t.Fatalf("%d/%s: %v", n, c.name, err)
			}
			row.OpsAllRuns = append(row.OpsAllRuns, report.Budget.Operations)
		}
		runtime.ReadMemStats(&ms)
		row.Verdict, row.Complete, row.StopReason = report.Verdict, report.SearchComplete, report.Budget.StopReason
		row.Operations, row.PeakBoxes, row.Witnesses = report.Budget.Operations, report.Budget.PeakBoxes, len(report.Witnesses)
		sorted := append([]time.Duration(nil), times...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		for _, d := range times {
			row.RunMillis = append(row.RunMillis, millis(d))
		}
		row.MedianMillis, row.MinMillis, row.MaxMillis = millis(sorted[2]), millis(sorted[0]), millis(sorted[4])
		row.AllocatedMiB = float64(ms.TotalAlloc-startAlloc) / 5 / (1 << 20)
		rows = append(rows, row)
	}
	for _, n := range []int{100, 250, 512} {
		for _, c := range p11Cases(n) {
			measure(n, c, "")
		}
	}
	// Largest constructible append: 511-rule base -> 512-rule after.
	base511 := corpusACL(511, 511)
	appended := append([]string(nil), base511[:len(base511)-1]...)
	appended = append(appended,
		"permit tcp host 192.0.2.200 host 203.0.113.200 eq 8443",
		base511[len(base511)-1])
	measure(511, corpusCase{name: "append_rule_to_512", before: base511, after: appended},
		"base 511 rules, after exactly 512 rules (parser cap); the 512-base append is not constructible")

	data, err := json.MarshalIndent(map[string]any{
		"go": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH,
		"cpus": runtime.NumCPU(), "gomaxprocs": runtime.GOMAXPROCS(0),
		"engine": EngineVersion, "runs_per_case": 5,
		"limits": DefaultOptions().Limits, "rows": rows,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"plenilune/internal/ciscoacl"
)

// corpusACL builds a deterministic synthetic ACL of n rules with shapes seen
// in management/server ACLs: host and subnet sources, service ports, ranges,
// neq, mixed protocols and a final deny. Documentation address space only.
func corpusACL(n int, seed uint64) []string {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	services := []string{"eq 22", "eq 443", "eq 80", "range 8000 8099", "eq 53", "neq 23", "gt 1023", "eq 3389"}
	lines := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		action := "permit"
		if r.IntN(5) == 0 {
			action = "deny"
		}
		var src string
		switch r.IntN(4) {
		case 0:
			src = fmt.Sprintf("host 192.0.2.%d", r.IntN(254)+1)
		case 1:
			src = fmt.Sprintf("192.0.2.%d 0.0.0.15", r.IntN(16)*16)
		case 2:
			src = fmt.Sprintf("198.51.%d.0 0.0.0.255", r.IntN(64))
		default:
			src = "any"
		}
		dst := fmt.Sprintf("host 203.0.113.%d", r.IntN(254)+1)
		if r.IntN(3) == 0 {
			dst = fmt.Sprintf("203.0.113.%d 0.0.0.31", r.IntN(8)*32)
		}
		switch r.IntN(6) {
		case 0:
			lines = append(lines, fmt.Sprintf("%s icmp %s %s", action, src, dst))
		case 1:
			lines = append(lines, fmt.Sprintf("%s udp %s %s %s", action, src, dst, services[r.IntN(len(services))]))
		default:
			lines = append(lines, fmt.Sprintf("%s tcp %s %s %s", action, src, dst, services[r.IntN(len(services))]))
		}
	}
	return append(lines, "deny ip any any")
}

func render(name string, rules []string) []byte {
	var b strings.Builder
	b.WriteString("ip access-list extended " + name + "\n")
	for i, rule := range rules {
		fmt.Fprintf(&b, " %d %s\n", (i+1)*10, rule)
	}
	b.WriteString("!\n")
	return []byte(b.String())
}

type corpusCase struct {
	name   string
	before []string
	after  []string
}

func corpusCases(n int) []corpusCase {
	base := corpusACL(n, uint64(n))
	cases := []corpusCase{{name: "identical", before: base, after: base}}
	insertTop := append([]string{"deny tcp host 192.0.2.10 any eq 22"}, base...)
	cases = append(cases, corpusCase{name: "insert_top_deny", before: base, after: insertTop})
	swapped := append([]string(nil), base...)
	mid := len(swapped) / 2
	swapped[mid], swapped[mid+1] = swapped[mid+1], swapped[mid]
	cases = append(cases, corpusCase{name: "swap_adjacent", before: base, after: swapped})
	port := append([]string(nil), base...)
	for i, rule := range port {
		if strings.Contains(rule, " eq 443") {
			port[i] = strings.Replace(rule, " eq 443", " eq 8443", 1)
			break
		}
	}
	cases = append(cases, corpusCase{name: "change_port", before: base, after: port})
	rewrite := append([]string{"deny ip any any"}, base...)
	cases = append(cases, corpusCase{name: "shadow_all", before: base, after: rewrite})
	return cases
}

type measurement struct {
	Rules         int     `json:"rules"`
	Case          string  `json:"case"`
	Verdict       string  `json:"verdict"`
	Complete      bool    `json:"search_complete"`
	StopReason    string  `json:"stop_reason"`
	Operations    int     `json:"operations"`
	PeakBoxes     int     `json:"peak_boxes"`
	Witnesses     int     `json:"witnesses"`
	MedianMillis  float64 `json:"median_ms"`
	AllocatedMiB  float64 `json:"allocated_mib"`
	ChangeClasses int     `json:"change_classes,omitempty"`
	ClassOps      int     `json:"change_class_operations,omitempty"`
	ClassComplete bool    `json:"change_classes_complete,omitempty"`
	ClassMillis   float64 `json:"change_class_median_ms,omitempty"`
}

// TestMeasureCorpus records engine cost on the fixed corpus. It is a
// measurement, not a correctness oracle, and only runs on explicit request.
func TestMeasureCorpus(t *testing.T) {
	out := os.Getenv("PLENILUNE_MEASURE_OUT")
	if out == "" {
		t.Skip("set PLENILUNE_MEASURE_OUT to record corpus measurements")
	}
	var rows []measurement
	for _, n := range []int{25, 100, 250, 512} {
		for _, c := range corpusCases(n) {
			before, err := ciscoacl.Parse(render("CORPUS", c.before))
			if err != nil {
				t.Fatalf("%d/%s before: %v", n, c.name, err)
			}
			after, err := ciscoacl.Parse(render("CORPUS", c.after))
			if err != nil {
				if strings.Contains(err.Error(), "rule limit") {
					continue
				}
				t.Fatalf("%d/%s after: %v", n, c.name, err)
			}
			row := measurement{Rules: n, Case: c.name}
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
			}
			runtime.ReadMemStats(&ms)
			row.Verdict, row.Complete, row.StopReason = report.Verdict, report.SearchComplete, report.Budget.StopReason
			row.Operations, row.PeakBoxes, row.Witnesses = report.Budget.Operations, report.Budget.PeakBoxes, len(report.Witnesses)
			row.MedianMillis = median(times)
			row.AllocatedMiB = float64(ms.TotalAlloc-startAlloc) / 5 / (1 << 20)
			if measureClasses != nil {
				measureClasses(&row, before, after)
			}
			rows = append(rows, row)
		}
	}
	data, err := json.MarshalIndent(map[string]any{
		"go": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "cpus": runtime.NumCPU(),
		"engine": EngineVersion, "runs_per_case": 5, "limits": DefaultOptions().Limits, "rows": rows,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// measureClasses records the separate change-class budget and latency.
var measureClasses func(*measurement, ciscoacl.ACL, ciscoacl.ACL)

func init() {
	measureClasses = func(row *measurement, before, after ciscoacl.ACL) {
		var times []time.Duration
		var report ChangeReport
		for i := 0; i < 3; i++ {
			start := time.Now()
			got, err := Changes(context.Background(), before, after, nil, DefaultChangeOptions())
			if err != nil {
				return
			}
			report, times = got, append(times, time.Since(start))
		}
		row.ChangeClasses, row.ClassOps, row.ClassComplete = len(report.Classes), report.Budget.Operations, report.Complete
		row.ClassMillis = median(times)
	}
}

func median(ds []time.Duration) float64 {
	sorted := append([]time.Duration(nil), ds...)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return float64(sorted[len(sorted)/2].Microseconds()) / 1000
}

// v0.2 must reach the same verdict and obligation outcomes as v0.1 whenever
// v0.1 completes. Witness boxes may be represented differently, so every v0.2
// witness is re-evaluated independently instead of compared to v0.1's.
func TestIndexedEngineMatchesLegacyOnCorpus(t *testing.T) {
	for _, n := range []int{8, 25, 60, 100} {
		for _, c := range corpusCases(n) {
			before, err := ciscoacl.Parse(render("CORPUS", c.before))
			if err != nil {
				t.Fatal(err)
			}
			after, err := ciscoacl.Parse(render("CORPUS", c.after))
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := CompareEngine(context.Background(), before, after, nil, DefaultOptions(), LegacyEngineVersion)
			if err != nil {
				t.Fatal(err)
			}
			current, err := CompareEngine(context.Background(), before, after, nil, DefaultOptions(), EngineVersion)
			if err != nil {
				t.Fatal(err)
			}
			if !legacy.SearchComplete {
				continue
			}
			if !current.SearchComplete || current.Verdict != legacy.Verdict || len(current.Witnesses) == 0 != (len(legacy.Witnesses) == 0) {
				t.Fatalf("%d/%s: v0.2 %s/%v vs v0.1 %s/%v", n, c.name, current.Verdict, current.SearchComplete, legacy.Verdict, legacy.SearchComplete)
			}
			for _, w := range current.Witnesses {
				x, _ := before.Evaluate(w.Packet)
				y, _ := after.Evaluate(w.Packet)
				if x.Action == y.Action || !reflect.DeepEqual(x, w.Before) || !reflect.DeepEqual(y, w.After) {
					t.Fatalf("%d/%s: witness failed independent evaluation", n, c.name)
				}
			}
		}
	}
}

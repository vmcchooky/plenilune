package intent

import (
	"reflect"
	"testing"

	"plenilune/internal/jsonio"
)

// Fuzz target for the conditions decoder + normalizer (plan P1-6).
// Invariants: never panic; a set that decodes and normalizes is a fixed
// point of Normalize, and its digest is stable across repeated calls.
func FuzzNormalizeJSON(f *testing.F) {
	f.Add([]byte(`{"schema":"plenilune.obligations/v1","scope":"ipv4-unfragmented-or-initial-with-complete-l4/v1","obligations":[{"id":"OB-1","kind":"must_permit","match":{"protocol":"tcp","src_cidr":"192.0.2.0/24","dst_cidr":"198.51.100.10/32","exclude_src":[],"exclude_dst":[],"src_ports":[],"dst_ports":[{"lo":22,"hi":22}]}}]}`))
	f.Add([]byte(`{"schema":"plenilune.obligations/v1","scope":"ipv4-unfragmented-or-initial-with-complete-l4/v1","obligations":[{"id":"OB-2","kind":"must_remain","match":{"protocol":"17","src_cidr":"10.0.0.0/8","dst_cidr":"0.0.0.0/0","exclude_src":["10.1.0.0/16"],"exclude_dst":[],"src_ports":[{"lo":1024,"hi":65535}],"dst_ports":[{"lo":53,"hi":53}]}},{"id":"OB-1","kind":"must_deny","match":{"protocol":"ip","src_cidr":"0.0.0.0/0","dst_cidr":"0.0.0.0/0","exclude_src":[],"exclude_dst":[],"src_ports":[],"dst_ports":[]}}]}`))
	f.Add([]byte(`{"schema":"plenilune.obligations/v1","scope":"ipv4-unfragmented-or-initial-with-complete-l4/v1","obligations":[]}`))
	f.Add([]byte(`{"schema":"wrong","scope":"ipv4-unfragmented-or-initial-with-complete-l4/v1","obligations":[{"id":"OB-1","kind":"must_permit","match":{"protocol":"tcp","src_cidr":"192.0.2.0/24","dst_cidr":"198.51.100.10/32","exclude_src":[],"exclude_dst":[],"src_ports":[],"dst_ports":[]}}]}`))
	f.Add([]byte(`{"schema":"plenilune.obligations/v1","obligations":[{"id":"OB-1","kind":"must_permit","match":{"protocol":"tcp","src_cidr":"999.0.2.0/24","dst_cidr":"198.51.100.10/32","dst_ports":[{"lo":90,"hi":22}]}}]}`))
	f.Add([]byte(``))
	f.Add([]byte(`{"schema":"plenilune.obligations/v1","scope":"ipv4-unfragmented-or-initial-with-complete-l4/v1","obligations":[{"id":"OB-1","kind":"must_permit","match":{"protocol":"tcp","src_cidr":"192.0.2.0/24","dst_cidr":"198.51.100.10/32","exclude_src":[],"exclude_dst":[],"src_ports":[],"dst_ports":[{"lo":22,"hi":22}]},"extra":1}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var in Set
		if err := jsonio.Decode(data, MaxJSONBytes, &in); err != nil {
			return
		}
		n, err := Normalize(in)
		if err != nil {
			return
		}
		again, err := Normalize(n)
		if err != nil {
			t.Fatalf("Normalize rejected its own output: %v", err)
		}
		if !reflect.DeepEqual(n, again) {
			t.Fatalf("Normalize is not idempotent:\n%+v\n%+v", n, again)
		}
		_, d1, err := Digest(n)
		if err != nil {
			t.Fatalf("Digest rejected a normalized set: %v", err)
		}
		_, d2, err := Digest(in)
		if err != nil || d1 != d2 {
			t.Fatalf("Digest is not stable across normalization: %q vs %q (%v)", d1, d2, err)
		}
	})
}

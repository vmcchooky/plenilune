package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

// The shipped capability examples (examples/, N3 sweep) are runnable
// before/after pairs, and runnable data rots like the CIS library does.
// Each pair must analyze to the object status its README advertises:
// the standard-ACL host deny and the added object-group member change
// behavior; the IPv6 /48 split into two /49s does not.

func readExamplePair(t *testing.T, dir string) (string, string) {
	t.Helper()
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join("..", "..", "examples", dir, name))
		if err != nil {
			t.Fatalf("read examples/%s/%s: %v", dir, name, err)
		}
		return string(raw)
	}
	return read("before.cfg"), read("after.cfg")
}

func TestN3ShippedExamplesAnalyzeAsAdvertised(t *testing.T) {
	cases := []struct {
		dir, name, want string
	}{
		{"standard-acl", "FILTER-IN", "changed"},
		{"object-groups", "MGMT", "changed"},
		{"ipv6-acl", "V6-EDGE", "equivalent"},
	}
	for _, tc := range cases {
		before, after := readExamplePair(t, tc.dir)
		r := run(t, before, after, nil)
		o := object(t, r, DomainACL, tc.name)
		if o.Status != tc.want {
			t.Errorf("examples/%s: object %s status = %q, want %q", tc.dir, tc.name, o.Status, tc.want)
		}
		if len(o.Errors) != 0 {
			t.Errorf("examples/%s: unexpected object errors: %+v", tc.dir, o.Errors)
		}
	}
}

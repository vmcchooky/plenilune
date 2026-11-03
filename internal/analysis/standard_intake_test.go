package analysis

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestStandardFullConfigNamedAndNumberedReplay(t *testing.T) {
	before := "hostname SYNTH\naccess-list 7 deny 192.0.2.0 0.0.0.255\ninterface Loopback0\n description synthetic\naccess-list 7 permit host 192.0.2.10\nip access-list standard NAMED\n 10 deny 192.0.2.0 0.0.0.255\n 20 permit host 192.0.2.10\n!\n"
	after := strings.ReplaceAll(before, "deny 192.0.2.0", "permit 192.0.2.0")
	r := run(t, before, after, nil)
	for _, name := range []string{"7", "NAMED"} {
		o := object(t, r, DomainACL, name)
		if o.Kind != "standard" || o.Status != "changed" || o.ACL == nil || o.ACL.Verdict != "DIFFERENT" {
			t.Fatalf("standard %s not reviewed: %+v", name, o)
		}
	}
	numbered := object(t, r, DomainACL, "7")
	if !reflect.DeepEqual(numbered.Before.Lines, [][2]int{{2, 2}, {5, 5}}) || numbered.Before.ByteStart != nil {
		t.Fatalf("numbered source mapping: %+v", numbered.Before)
	}
	if numbered.Before.SHA256 != digest([]byte("access-list 7 deny 192.0.2.0 0.0.0.255\naccess-list 7 permit host 192.0.2.10\n")) {
		t.Fatal("numbered hash is not bound to exact selected source bytes")
	}
	replayed, err := Replay(context.Background(), []byte(before), []byte(after), r)
	if err != nil || replayed.Status != "ANALYSIS_REPLAYED" || replayed.SHA256 != r.SHA256 {
		t.Fatalf("standard replay: %+v %v", replayed, err)
	}
}

func TestStandardMixedDefinitionFailsClosed(t *testing.T) {
	for _, before := range []string{
		"ip access-list standard 7\n permit any\n!\naccess-list 7 deny any\n",
		"access-list 7 deny any\nip access-list standard 7\n permit any\n",
		"ip access-list extended 7\n permit ip any any\n!\naccess-list 7 deny any\n",
	} {
		r := run(t, before, before, nil)
		o := object(t, r, DomainACL, "7")
		if o.Status != "unsupported" || len(o.Errors) == 0 || o.Errors[0].Code != "DUPLICATE_DEFINITION" || o.ACL != nil {
			t.Fatalf("mixed definition gained a verdict: %+v", o)
		}
	}
}

func TestStandardAndIPv6SameNameKeepSeparateSourceNamespaces(t *testing.T) {
	v4 := "ip access-list standard SAME\n permit any\n!\n"
	v6 := "ipv6 access-list SAME\n permit ipv6 any any\n!\n"
	for _, config := range []string{v4 + v6, v6 + v4} {
		r := run(t, config, config, nil)
		standard, ipv6 := false, false
		for _, object := range r.Objects {
			if object.Name != "SAME" {
				continue
			}
			if object.Kind == "standard" {
				standard = true
				if object.Status != "unchanged" || len(object.Errors) != 0 || object.Before.SHA256 != digest([]byte("ip access-list standard SAME\n permit any\n")) {
					t.Fatalf("IPv6 contaminated IPv4: %+v", object)
				}
			}
			if object.Kind == "ipv6" {
				ipv6 = true
				// Since P3-7 the IPv6 namespace is analyzed in its own right;
				// identical sources classify unchanged without touching the
				// IPv4 object that shares the name.
				if object.Status != "unchanged" || len(object.Errors) != 0 {
					t.Fatalf("IPv6 namespace mishandled: %+v", object)
				}
			}
		}
		if !standard || !ipv6 {
			t.Fatal("source namespaces collapsed")
		}
	}
}

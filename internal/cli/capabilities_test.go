package cli

import (
	"reflect"
	"testing"

	"plenilune/internal/capabilities"
	"plenilune/internal/jsonio"
)

// TestCapabilitiesCommandServesTheSharedDocument: the CLI surface
// must serve exactly the document the shared builder produces in
// this process (same tool version, same build-info commit), with and
// without the explicit --json flag.
func TestCapabilitiesCommandServesTheSharedDocument(t *testing.T) {
	want := capabilities.Build()
	for _, args := range [][]string{{"capabilities"}, {"capabilities", "--json"}} {
		code, body, errout := invoke(args...)
		if code != 0 || errout != "" {
			t.Fatalf("%v: exit %d, stderr %s", args, code, errout)
		}
		var got capabilities.Document
		if err := jsonio.Decode([]byte(body), 1<<20, &got); err != nil {
			t.Fatalf("%v: decode: %v", args, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%v: CLI document differs from capabilities.Build()\ngot:  %+v\nwant: %+v", args, got, want)
		}
		if got.Schema != capabilities.Schema || got.Tool.Version != Version {
			t.Fatalf("%v: schema/tool = %s/%s", args, got.Schema, got.Tool.Version)
		}
	}
}

func TestCapabilitiesCommandRejectsJunk(t *testing.T) {
	if code, _, _ := invoke("capabilities", "extra"); code != 2 {
		t.Fatalf("positional argument: exit %d, want 2", code)
	}
	if code, _, _ := invoke("capabilities", "--bogus"); code != 2 {
		t.Fatalf("unknown flag: exit %d, want 2", code)
	}
}

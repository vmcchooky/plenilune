package demohttp

import (
	"net/http"
	"reflect"
	"testing"

	"plenilune/internal/capabilities"
)

// TestCapabilitiesRouteServesTheSharedDocument pins the P2-2 rule
// that the HTTP surface and the CLI surface are one builder: the
// decoded document must equal capabilities.Build() exactly.
func TestCapabilitiesRouteServesTheSharedDocument(t *testing.T) {
	rec := do(t, NewHandler(""), http.MethodGet, "/api/capabilities", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var got capabilities.Document
	decodeJSON(t, rec, &got)
	if want := capabilities.Build(); !reflect.DeepEqual(got, want) {
		t.Fatalf("HTTP document differs from capabilities.Build()\ngot:  %+v\nwant: %+v", got, want)
	}
}

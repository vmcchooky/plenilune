package demohttp

import (
	"errors"
	"log"
	"net/http"

	"plenilune/internal/bundle"
)

// maxBundleBody bounds a bundle export request: the comparison or
// analysis artifact dominates (16 MiB), plus both sources, the
// repair receipt and the two view files.
const maxBundleBody = 24 << 20

// bundleExportRequest is the web review-export payload: the raw
// artifact texts of one review case. Optional parts are pointers so
// an absent file stays absent — an empty repair string must not
// become an empty repair.json in the bundle.
type bundleExportRequest struct {
	Before       string  `json:"before"`
	After        string  `json:"after"`
	Comparison   string  `json:"comparison"`
	Repair       *string `json:"repair,omitempty"`
	Report       *string `json:"report,omitempty"`
	Instructions *string `json:"instructions,omitempty"`
}

// handleBundleExport builds the review bundle manifest for a case
// with the same shared builder the CLI exporter uses and returns the
// canonical manifest bytes, so a manifest exported from the UI is
// byte-identical to one exported by plenilune bundle-export for the
// same case (acceptance P2-4). The manifest is an integrity index
// over files the caller downloads alongside it; the receipts inside
// those files remain the proof.
func handleBundleExport(w http.ResponseWriter, r *http.Request) {
	var in bundleExportRequest
	if !decodeRequest(w, r, maxBundleBody, &in) {
		return
	}
	// Empty strings are absent parts, not empty files: skipping them
	// lets the shared builder's required-role check reject a case
	// that is missing its comparison instead of indexing zero bytes.
	files := map[string][]byte{}
	if in.Before != "" {
		files[bundle.RoleBefore] = []byte(in.Before)
	}
	if in.After != "" {
		files[bundle.RoleAfter] = []byte(in.After)
	}
	if in.Comparison != "" {
		files[bundle.RoleComparison] = []byte(in.Comparison)
	}
	if in.Repair != nil {
		files[bundle.RoleRepair] = []byte(*in.Repair)
	}
	if in.Report != nil {
		files[bundle.RoleReport] = []byte(*in.Report)
	}
	if in.Instructions != nil {
		files[bundle.RoleInstructions] = []byte(*in.Instructions)
	}
	manifest, err := bundle.Build(files)
	if err != nil {
		var be *bundle.Error
		if errors.As(err, &be) {
			writeError(w, http.StatusBadRequest, apiError{Code: be.Code, Message: be.Message, Field: be.Role})
			return
		}
		writeError(w, http.StatusBadRequest, apiError{Code: "BUNDLE_INVALID", Message: "bundle could not be built"})
		return
	}
	data, err := bundle.Marshal(manifest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiError{Code: "BUNDLE_INVALID", Message: "bundle manifest could not be encoded"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		log.Printf("demo response write failed: %v", err)
	}
}

// Package bundle builds and verifies review bundle manifests
// (plenilune.bundle/v1, P2-4). A bundle is one review case as a plain
// directory of files plus a manifest that indexes them: every entry
// names a role, a relative path inside the bundle directory and the
// SHA-256 of that file's bytes. The manifest is an integrity index
// only — it carries no verdict and proves nothing by itself. The
// receipts inside the files remain the proof (their own digests,
// including the P2-1 content_sha256/artifact pair, are what anchor a
// replay), the report file remains a human view, and this manifest
// only guarantees that the bytes a reviewer loads are the bytes the
// exporter indexed. There is no archive format: a bundle is the
// directory, readable with any tool.
//
// The manifest is deliberately not self-hashed: a hash of the index
// would anchor nothing (whoever rewrites a file can rewrite the
// index), while the artifact digests sealed inside the files are
// verified by the replay machinery itself.
//
// This package is pure: it works on byte maps keyed by role. File
// system access belongs to the surfaces (CLI, demo server) exactly
// as everywhere else in the repository.
package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"plenilune/internal/jsonio"
)

// Schema is the bundle manifest schema id.
const Schema = "plenilune.bundle/v1"

// ManifestName is the canonical file name of the manifest inside a
// bundle directory.
const ManifestName = "bundle-manifest.json"

// MaxManifestBytes bounds a manifest: six short entries, so 64 KiB
// is generous and still refuses absurd inputs before allocation.
const MaxManifestBytes = 64 << 10

// Roles of the files in one review case. The first three are
// required — a case without both sources and its artifact is not a
// bundle; the rest are optional at export, but once a file is listed
// in the manifest it is required to be present and to match.
const (
	RoleBefore       = "before"
	RoleAfter        = "after"
	RoleComparison   = "comparison"
	RoleRepair       = "repair"
	RoleReport       = "report"
	RoleInstructions = "instructions"
)

// roleOrder is the canonical manifest entry order. Manifests are
// written and accepted only in this order, so the same case always
// produces the same manifest bytes.
var roleOrder = []string{
	RoleBefore,
	RoleAfter,
	RoleComparison,
	RoleRepair,
	RoleReport,
	RoleInstructions,
}

var requiredRoles = []string{RoleBefore, RoleAfter, RoleComparison}

// fileNames is the canonical in-bundle file name per role, used by
// exporters. A loader accepts any safe relative path (see Parse);
// the names only fix what export produces.
var fileNames = map[string]string{
	RoleBefore:       "before.acl",
	RoleAfter:        "after.acl",
	RoleComparison:   "comparison.json",
	RoleRepair:       "repair.json",
	RoleReport:       "report.md",
	RoleInstructions: "instructions.md",
}

// File is one manifest entry: the bytes at Path inside the bundle
// directory play Role in the case and hash to SHA256.
type File struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Manifest is the bundle index document.
type Manifest struct {
	Schema string `json:"schema"`
	Files  []File `json:"files"`
}

// Error is the bundle package's typed error. Code is a published
// taxonomy code (registered in errtax and capabilities); Role names
// the case file the failure belongs to when there is one, so callers
// never have to parse the message to learn which file failed.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Role    string `json:"role,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func invalid(role, format string, args ...any) *Error {
	return &Error{Code: "BUNDLE_INVALID", Message: fmt.Sprintf(format, args...), Role: role}
}

// FileName returns the canonical in-bundle file name for a role.
func FileName(role string) (string, bool) {
	name, ok := fileNames[role]
	return name, ok
}

// Digest is the SHA-256 of raw file bytes, hex-encoded — the only
// hash a bundle ever computes.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func knownRole(role string) bool {
	_, ok := fileNames[role]
	return ok
}

// Build assembles the manifest for one case from its file bytes,
// keyed by role. The result depends only on the bytes: no timestamps,
// no tool identity, so the CLI exporter and the demo server produce
// byte-identical manifests for the same case. Unknown roles and
// missing required roles are rejected; optional roles join when
// present.
func Build(files map[string][]byte) (Manifest, error) {
	for role := range files {
		if !knownRole(role) {
			return Manifest{}, invalid(role, "unknown bundle role %q", role)
		}
	}
	m := Manifest{Schema: Schema}
	for _, role := range roleOrder {
		data, ok := files[role]
		if !ok {
			continue
		}
		m.Files = append(m.Files, File{Role: role, Path: fileNames[role], SHA256: Digest(data)})
	}
	for _, role := range requiredRoles {
		if _, ok := files[role]; !ok {
			return Manifest{}, invalid(role, "review case is missing the required %s file", role)
		}
	}
	if err := validate(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Marshal is the canonical manifest encoding: indented JSON with a
// trailing newline, the exact bytes exporters write to
// bundle-manifest.json and the server returns.
func Marshal(m Manifest) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Parse strictly decodes a manifest and validates it fully: schema,
// known roles only, no duplicate role or path, required roles
// present, entries in canonical role order, digests well-formed and
// paths safe relative paths that cannot escape the bundle directory.
func Parse(data []byte) (Manifest, error) {
	var m Manifest
	if err := jsonio.Decode(data, MaxManifestBytes, &m); err != nil {
		return Manifest{}, invalid("", "bundle manifest is not valid %s JSON: %v", Schema, err)
	}
	if err := validate(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func validate(m Manifest) error {
	if m.Schema != Schema {
		return invalid("", "bundle manifest schema is %q, want %s", m.Schema, Schema)
	}
	if len(m.Files) == 0 {
		return invalid("", "bundle manifest lists no files")
	}
	seenRoles := map[string]bool{}
	seenPaths := map[string]bool{}
	lastOrder := -1
	for _, f := range m.Files {
		if !knownRole(f.Role) {
			return invalid(f.Role, "unknown bundle role %q", f.Role)
		}
		if seenRoles[f.Role] {
			return invalid(f.Role, "bundle manifest lists role %q twice", f.Role)
		}
		seenRoles[f.Role] = true
		order := roleIndex(f.Role)
		if order < lastOrder {
			return invalid(f.Role, "bundle manifest entries are not in canonical role order")
		}
		lastOrder = order
		if err := validatePath(f); err != nil {
			return err
		}
		if seenPaths[f.Path] {
			return invalid(f.Role, "bundle manifest lists path %q twice", f.Path)
		}
		seenPaths[f.Path] = true
		if !isDigest(f.SHA256) {
			return invalid(f.Role, "bundle entry for role %q has a malformed sha256", f.Role)
		}
	}
	for _, role := range requiredRoles {
		if !seenRoles[role] {
			return invalid(role, "bundle manifest is missing the required %s entry", role)
		}
	}
	return nil
}

// validatePath accepts only a clean relative path that stays inside
// the bundle directory on any OS: no absolute paths, no drive or
// backslash forms, no "." or ".." elements, and never the manifest
// itself.
func validatePath(f File) error {
	p := f.Path
	switch {
	case p == "":
		return invalid(f.Role, "bundle entry for role %q has an empty path", f.Role)
	case p == ManifestName:
		return invalid(f.Role, "bundle entry for role %q names the manifest itself", f.Role)
	case strings.Contains(p, "\\"):
		return invalid(f.Role, "bundle path %q must use forward slashes", p)
	case path.IsAbs(p):
		return invalid(f.Role, "bundle path %q is not relative", p)
	case p == ".." || strings.HasPrefix(p, "../"):
		return invalid(f.Role, "bundle path %q escapes the bundle directory", p)
	case len(p) >= 2 && p[1] == ':':
		return invalid(f.Role, "bundle path %q is not relative", p)
	case path.Clean(p) != p:
		return invalid(f.Role, "bundle path %q is not a clean relative path inside the bundle", p)
	}
	return nil
}

func isDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func roleIndex(role string) int {
	for i, r := range roleOrder {
		if r == role {
			return i
		}
	}
	return -1
}

// Entry returns the manifest entry for a role.
func (m Manifest) Entry(role string) (File, bool) {
	for _, f := range m.Files {
		if f.Role == role {
			return f, true
		}
	}
	return File{}, false
}

// Verify checks a loaded case against the manifest: every listed
// file must be present in files (keyed by role) and hash to the
// listed digest. A listed file that is absent — required or
// optional — is BUNDLE_FILE_MISSING; a byte difference is
// BUNDLE_DIGEST_MISMATCH. Both name the role.
func (m Manifest) Verify(files map[string][]byte) error {
	for _, f := range m.Files {
		data, ok := files[f.Role]
		if !ok {
			return &Error{Code: "BUNDLE_FILE_MISSING", Message: fmt.Sprintf("bundle file for role %q (%s) is missing", f.Role, f.Path), Role: f.Role}
		}
		if Digest(data) != f.SHA256 {
			return &Error{Code: "BUNDLE_DIGEST_MISMATCH", Message: fmt.Sprintf("bundle file for role %q (%s) does not match the manifest digest", f.Role, f.Path), Role: f.Role}
		}
	}
	return nil
}

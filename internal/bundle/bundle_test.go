package bundle

import (
	"errors"
	"strings"
	"testing"
)

func caseFiles() map[string][]byte {
	return map[string][]byte{
		RoleBefore:       []byte("ip access-list extended DEMO\n permit tcp any any eq 22\n"),
		RoleAfter:        []byte("ip access-list extended DEMO\n deny ip any any\n"),
		RoleComparison:   []byte("{\"schema\":\"plenilune.evidence/v2\"}\n"),
		RoleRepair:       []byte("{\"schema\":\"plenilune.repair/v1\"}\n"),
		RoleReport:       []byte("# Report\n"),
		RoleInstructions: []byte("# Replay\n"),
	}
}

func codeOf(t *testing.T, err error) *Error {
	t.Helper()
	if err == nil {
		t.Fatal("expected a bundle error, got nil")
	}
	var be *Error
	if !errors.As(err, &be) {
		t.Fatalf("error %v is not a bundle.Error", err)
	}
	return be
}

func TestBuildCanonicalManifest(t *testing.T) {
	files := caseFiles()
	m, err := Build(files)
	if err != nil {
		t.Fatal(err)
	}
	if m.Schema != Schema {
		t.Fatalf("schema = %q", m.Schema)
	}
	if len(m.Files) != len(files) {
		t.Fatalf("entries = %d, want %d", len(m.Files), len(files))
	}
	for i, f := range m.Files {
		if f.Role != roleOrder[i] {
			t.Fatalf("entry %d role = %q, want canonical order %q", i, f.Role, roleOrder[i])
		}
		name, _ := FileName(f.Role)
		if f.Path != name {
			t.Fatalf("entry %s path = %q, want canonical %q", f.Role, f.Path, name)
		}
		if f.SHA256 != Digest(files[f.Role]) {
			t.Fatalf("entry %s digest does not match its bytes", f.Role)
		}
	}
	// The manifest is a pure function of the bytes: a second build is
	// byte-identical (this is what makes CLI and server exports equal).
	again, err := Build(files)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Marshal(again)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("manifest bytes differ across builds:\n%s\n---\n%s", a, b)
	}
	// Minimal case: optional roles are absent, not empty.
	minimal := map[string][]byte{RoleBefore: files[RoleBefore], RoleAfter: files[RoleAfter], RoleComparison: files[RoleComparison]}
	mm, err := Build(minimal)
	if err != nil {
		t.Fatal(err)
	}
	if len(mm.Files) != 3 {
		t.Fatalf("minimal entries = %d, want 3", len(mm.Files))
	}
	if _, ok := mm.Entry(RoleRepair); ok {
		t.Fatal("minimal manifest lists an absent optional role")
	}
}

func TestBuildRejectsUnknownAndMissingRoles(t *testing.T) {
	files := caseFiles()
	files["candidate"] = []byte("x")
	if be := codeOf(t, errOf(Build(files))); be.Code != "BUNDLE_INVALID" || be.Role != "candidate" {
		t.Fatalf("unknown role error = %+v", be)
	}
	delete(files, "candidate")
	delete(files, RoleComparison)
	if be := codeOf(t, errOf(Build(files))); be.Code != "BUNDLE_INVALID" || be.Role != RoleComparison {
		t.Fatalf("missing required error = %+v", be)
	}
}

func errOf(_ Manifest, err error) error { return err }

func TestParseVerifyRoundTrip(t *testing.T) {
	files := caseFiles()
	m, err := Build(files)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := parsed.Verify(files); err != nil {
		t.Fatalf("verify of the exported case failed: %v", err)
	}
}

func TestVerifyDetectsFlippedByteAndWrongManifestDigest(t *testing.T) {
	files := caseFiles()
	m, err := Build(files)
	if err != nil {
		t.Fatal(err)
	}
	// Flip one byte in the comparison file: the failure must name
	// the role whose bytes changed.
	tampered := caseFiles()
	c := append([]byte(nil), tampered[RoleComparison]...)
	c[3] ^= 0x01
	tampered[RoleComparison] = c
	if be := codeOf(t, m.Verify(tampered)); be.Code != "BUNDLE_DIGEST_MISMATCH" || be.Role != RoleComparison {
		t.Fatalf("tampered file error = %+v", be)
	}
	// A manifest whose recorded digest is wrong fails the same way,
	// even though the files themselves are untouched.
	forged := m
	forged.Files = append([]File(nil), m.Files...)
	for i := range forged.Files {
		if forged.Files[i].Role == RoleReport {
			forged.Files[i].SHA256 = Digest([]byte("some other report"))
		}
	}
	raw, err := Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if be := codeOf(t, parsed.Verify(files)); be.Code != "BUNDLE_DIGEST_MISMATCH" || be.Role != RoleReport {
		t.Fatalf("wrong-digest error = %+v", be)
	}
}

func TestVerifyMissingFiles(t *testing.T) {
	files := caseFiles()
	m, err := Build(files)
	if err != nil {
		t.Fatal(err)
	}
	// A required file removed from the directory.
	missing := caseFiles()
	delete(missing, RoleAfter)
	if be := codeOf(t, m.Verify(missing)); be.Code != "BUNDLE_FILE_MISSING" || be.Role != RoleAfter {
		t.Fatalf("missing required error = %+v", be)
	}
	// An OPTIONAL file that the manifest lists is just as required
	// to be present and matching: listed means indexed.
	delete(missing, RoleAfter)
	missing[RoleAfter] = files[RoleAfter]
	delete(missing, RoleInstructions)
	if be := codeOf(t, m.Verify(missing)); be.Code != "BUNDLE_FILE_MISSING" || be.Role != RoleInstructions {
		t.Fatalf("missing listed optional error = %+v", be)
	}
}

func manifestJSON(entries string) []byte {
	return []byte("{\"schema\":\"" + Schema + "\",\"files\":[" + entries + "]}\n")
}

func entry(role, path, digest string) string {
	return "{\"role\":\"" + role + "\",\"path\":\"" + path + "\",\"sha256\":\"" + digest + "\"}"
}

func TestParseRejectsUnsafeAndMalformedManifests(t *testing.T) {
	d := Digest([]byte("x"))
	valid := entry(RoleBefore, "before.acl", d) + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d)
	tests := map[string]string{
		"parent escape":      entry(RoleBefore, "../before.acl", d) + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"nested escape":      entry(RoleBefore, "sub/../../before.acl", d) + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"bare parent":        entry(RoleBefore, "..", d) + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"absolute path":      entry(RoleBefore, "/etc/before.acl", d) + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"backslash path":     entry(RoleBefore, `..\before.acl`, d) + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"drive path":         entry(RoleBefore, "C:/before.acl", d) + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"manifest itself":    entry(RoleBefore, ManifestName, d) + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"duplicate role":     valid + "," + entry(RoleBefore, "other.acl", d),
		"duplicate path":     entry(RoleBefore, "same.acl", d) + "," + entry(RoleAfter, "same.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"unknown role":       valid + "," + entry("candidate", "candidate.acl", d),
		"wrong order":        entry(RoleAfter, "after.acl", d) + "," + entry(RoleBefore, "before.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"malformed digest":   entry(RoleBefore, "before.acl", "xyz") + "," + entry(RoleAfter, "after.acl", d) + "," + entry(RoleComparison, "comparison.json", d),
		"missing required":   entry(RoleBefore, "before.acl", d) + "," + entry(RoleAfter, "after.acl", d),
		"unknown json field": strings.TrimSuffix(string(manifestJSON(valid)), "}\n") + ",\"extra\":1}\n",
	}
	for name, body := range tests {
		if _, err := Parse([]byte(body)); err == nil {
			t.Fatalf("%s: manifest accepted", name)
		} else if be := codeOf(t, err); be.Code != "BUNDLE_INVALID" {
			t.Fatalf("%s: code = %q, want BUNDLE_INVALID", name, be.Code)
		}
	}
	if _, err := Parse([]byte("{\"schema\":\"plenilune.bundle/v99\",\"files\":[" + valid + "]}")); err == nil {
		t.Fatal("wrong schema accepted")
	}
	if _, err := Parse(manifestJSON(valid)); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

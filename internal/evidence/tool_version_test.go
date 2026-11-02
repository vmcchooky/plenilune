package evidence

import "testing"

// TestSupportedToolVersionSet pins the accepted tool-version set from
// both sides: the current release and the pre-release development line
// are accepted (their artifacts must keep replaying after the 1.0.0
// version bump — the frozen replay fixtures are the end-to-end proof),
// and anything else is rejected.
func TestSupportedToolVersionSet(t *testing.T) {
	for _, v := range []string{ToolVersion, LegacyToolVersion} {
		if !SupportedToolVersion(v) {
			t.Errorf("SupportedToolVersion(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "0.1.0", "0.2.0", "1.0.1", "2.0.0", "9.9.9", "1.0.0-dev", "v1.0.0"} {
		if SupportedToolVersion(v) {
			t.Errorf("SupportedToolVersion(%q) = true, want false", v)
		}
	}
	if ToolVersion != "1.0.0" || LegacyToolVersion != "0.2.0-dev" {
		t.Errorf("version constants drifted: ToolVersion=%q LegacyToolVersion=%q", ToolVersion, LegacyToolVersion)
	}
}

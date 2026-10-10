package updates

import "testing"

// Versions are ordered as Semantic Versioning orders them, a pre-release
// before its version; what is not a version is never newer, nor older.
func TestVersionsAreComparedInSemanticOrder(t *testing.T) {
	for _, tc := range []struct {
		version, current string
		newer            bool
	}{
		{"1.5.0", "1.4.0", true},
		{"v1.5.0", "1.4.0", true},
		{"1.4.1", "1.4.0", true},
		{"2.0.0", "1.99.99", true},
		{"1.10.0", "1.9.0", true},
		{"1.4.0", "1.4.0", false},
		{"v1.4.0", "1.4.0", false},
		{"1.4.0+build.7", "1.4.0", false},
		{"1.3.9", "1.4.0", false},
		{"1.4.0", "1.4.0-rc.1", true},
		{"1.4.0-rc.2", "1.4.0-rc.1", true},
		{"1.4.0-rc.10", "1.4.0-rc.9", true},
		{"1.4.0-rc.1.1", "1.4.0-rc.1", true},
		{"1.4.0-beta", "1.4.0-alpha", true},
		{"1.4.0-alpha", "1.4.0-1", true},
		{"1.4.0-rc.1", "1.4.0", false},
		{"1.5.0", "dev", false},
		{"1.5.0", "", false},
		{"dev", "1.4.0", false},
		{"1.5", "1.4.0", false},
		{"01.5.0", "1.4.0", false},
		{"1.5.0-", "1.4.0", false},
		{"1.5.0-rc..1", "1.4.0", false},
		{"1.5.0-rc.01", "1.4.0", false},
	} {
		if got := newer(tc.version, tc.current); got != tc.newer {
			t.Errorf("newer(%q, %q) = %v, want %v", tc.version, tc.current, got, tc.newer)
		}
	}
}

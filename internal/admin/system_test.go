package admin

import (
	"runtime"
	"testing"
)

// The status tells the admin app where Polyfin runs: in a container, else
// on the system it was built for.
func TestStatusTellsTheSystem(t *testing.T) {
	t.Parallel()
	want := map[string]string{"linux": "linux", "darwin": "macos", "windows": "windows"}[runtime.GOOS]
	if want == "" {
		want = "linux"
	}
	if got := system(false); got != want {
		t.Errorf("outside a container: %q, want %q", got, want)
	}
	if got := system(true); got != "container" {
		t.Errorf("in a container: %q", got)
	}
}

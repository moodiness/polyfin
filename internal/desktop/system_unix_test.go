//go:build !windows

package desktop

import (
	"strings"
	"testing"
)

func TestOpener(t *testing.T) {
	for goos, want := range map[string]string{
		"darwin": "open http://localhost:8096/admin/",
		"linux":  "xdg-open http://localhost:8096/admin/",
	} {
		name, args := opener(goos, "http://localhost:8096/admin/")
		if got := name + " " + strings.Join(args, " "); got != want {
			t.Errorf("opener(%s) = %q", goos, got)
		}
	}
}

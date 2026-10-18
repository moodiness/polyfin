//go:build !windows

package desktop

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func hideWindow(*exec.Cmd) {}

// systemLanguage is, on macOS, the first of the user's preferred
// languages; elsewhere the environment told it already.
func systemLanguage() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	// The answer is a list: ( "fr-FR", "en-US" ).
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.Trim(strings.TrimSpace(line), `",()`)
		if line != "" {
			return line
		}
	}
	return ""
}

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

// Open opens an address in the default browser, or a folder in the
// system's file manager.
func Open(target string) error {
	name, args := opener(runtime.GOOS, target)
	cmd := WindowCommand(context.Background(), name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// The app does not wait for it, and reaps it when it ends.
	go func() { _ = cmd.Wait() }()
	return nil
}

// opener is the program that opens target on goos, and its arguments.
func opener(goos, target string) (string, []string) {
	if goos == "darwin" {
		return "open", []string{target}
	}
	return "xdg-open", []string{target}
}

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

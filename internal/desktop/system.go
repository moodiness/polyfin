package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/i18n"
)

// Command is exec.CommandContext for the console programs the app runs,
// such as the server and PostgreSQL's: on Windows they open no window,
// as the app has no console of its own.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	return cmd
}

// WindowCommand is exec.CommandContext for a program with windows of its
// own, such as an installer: they show as when the program is opened
// from the file manager.
func WindowCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// Copy puts text on the clipboard.
func Copy(text string) error {
	for _, candidate := range clipboard(runtime.GOOS) {
		path, err := exec.LookPath(candidate[0])
		if err != nil {
			continue
		}
		cmd := Command(context.Background(), path, candidate[1:]...)
		cmd.Stdin = strings.NewReader(text)
		// xclip and wl-copy stay on to serve the clipboard.
		cmd.WaitDelay = 5 * time.Second
		return cmd.Run()
	}
	return errors.New("no clipboard program found")
}

// clipboard is the programs, tried in turn, that write their input to
// the clipboard on goos.
func clipboard(goos string) [][]string {
	switch goos {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "windows":
		return [][]string{{"clip"}}
	default:
		return [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
}

// Language is the system's language when the app speaks it (one of
// i18n.Languages), else English.
func Language() string {
	for _, variable := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(variable); value != "" && value != "C" && value != "POSIX" {
			return languageOf(value)
		}
	}
	return languageOf(systemLanguage())
}

// languageOf reads a locale such as fr_FR.UTF-8, de-CH or pt-BR (see
// i18n.Match).
func languageOf(locale string) string {
	if language := i18n.Match(locale); language != "" {
		return language
	}
	return "en"
}

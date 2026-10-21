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

// Command is exec.CommandContext for the programs the app runs, which on
// Windows open no console window.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	return cmd
}

// Open opens an address in the default browser, or a folder in the
// system's file manager.
func Open(target string) error {
	name, args := opener(runtime.GOOS, target, isDir(target))
	return startDetached(name, args...)
}

// opener is the program that opens target on goos, and its arguments.
func opener(goos, target string, dir bool) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{target}
	case "windows":
		if dir {
			return "explorer", []string{target}
		}
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		return "xdg-open", []string{target}
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// startDetached starts a program the app does not wait for, and reaps it
// when it ends.
func startDetached(name string, args ...string) error {
	cmd := Command(context.Background(), name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
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

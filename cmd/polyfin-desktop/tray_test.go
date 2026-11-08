//go:build !darwin || cgo

package main

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// An icon that never shows is an error in the log, and the app runs on;
// one that shows, or a quit before the time is up, is not.
func TestAwaitTray(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, nil))
	expired := make(chan time.Time, 1)
	expired <- time.Now()
	if awaitTray(make(chan struct{}), make(chan struct{}), nil, expired, logger) {
		t.Error("an icon that never showed counts as shown")
	}
	if !strings.Contains(out.String(), "level=ERROR") || !strings.Contains(out.String(), "polyfin-desktop -quit") {
		t.Errorf("log = %q", out.String())
	}
	out.Reset()
	shown := make(chan struct{})
	close(shown)
	if !awaitTray(shown, make(chan struct{}), nil, nil, logger) {
		t.Error("the icon that showed does not count")
	}
	quitting := make(chan struct{})
	close(quitting)
	if awaitTray(make(chan struct{}), quitting, nil, nil, logger) {
		t.Error("a quit counts as shown")
	}
	if out.Len() != 0 {
		t.Errorf("log = %q", out.String())
	}
}

// When the tray's library writes that it gave up on the icon, as in a
// Windows session whose notification area refuses icons, the app says at
// once that it runs without it, not after trayWait. The library's other
// complaints end no wait.
//
//nolint:paralleltest // swaps the default slog logger and the log package's output
func TestAnIconTheLibraryGaveUpOnIsToldAtOnce(t *testing.T) {
	previous, writer, flags := slog.Default(), log.Writer(), log.Flags()
	defer func() {
		slog.SetDefault(previous)
		slog.SetLogLoggerLevel(slog.LevelInfo)
		log.SetOutput(writer)
		log.SetFlags(flags)
	}()
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, nil))
	refused := logLibraries(logger)
	// The library writes synchronously: the line has gone through before
	// Printf returns.
	log.Printf("systray error: unable to set tooltip: %s\n", "Unspecified error")
	select {
	case <-refused:
		t.Fatal("a tooltip the library could not set counts as an icon it gave up on")
	default:
	}
	log.Printf("systray error: unable to init instance: %s\n", "Unspecified error")
	told := make(chan bool, 1)
	go func() { told <- awaitTray(make(chan struct{}), make(chan struct{}), refused, nil, logger) }()
	select {
	case shown := <-told:
		if shown {
			t.Error("an icon the library gave up on counts as shown")
		}
	case <-time.After(time.Minute):
		t.Fatal("the app still waits for an icon the library gave up on")
	}
	if got := out.String(); !strings.Contains(got, `level=ERROR msg="The tray icon could not be shown`) {
		t.Errorf("log = %q", got)
	}
}

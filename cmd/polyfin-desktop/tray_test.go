//go:build !darwin || cgo

package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// An icon that never shows is an error in the log, and the app runs on;
// one that shows, or a quit before the time is up, is not.
func TestAwaitTray(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, nil))
	expired := make(chan time.Time, 1)
	expired <- time.Now()
	if awaitTray(make(chan struct{}), make(chan struct{}), expired, logger) {
		t.Error("an icon that never showed counts as shown")
	}
	if !strings.Contains(out.String(), "level=ERROR") || !strings.Contains(out.String(), "polyfin-desktop -quit") {
		t.Errorf("log = %q", out.String())
	}
	out.Reset()
	shown := make(chan struct{})
	close(shown)
	if !awaitTray(shown, make(chan struct{}), nil, logger) {
		t.Error("the icon that showed does not count")
	}
	quitting := make(chan struct{})
	close(quitting)
	if awaitTray(make(chan struct{}), quitting, nil, logger) {
		t.Error("a quit counts as shown")
	}
	if out.Len() != 0 {
		t.Errorf("log = %q", out.String())
	}
}

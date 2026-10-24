//go:build darwin && !cgo

package main

import (
	"log/slog"
	"os/exec"

	"github.com/moodiness/polyfin/internal/desktop/app"
)

// runTray, in a build without cgo, which macOS's menu bar needs, runs
// without the icon, as -headless does.
func runTray(options app.Options, logger *slog.Logger, asked requests) *exec.Cmd {
	logger.Warn("This build has no menu bar icon: it was built without cgo")
	return runHeadless(options, logger, asked)
}

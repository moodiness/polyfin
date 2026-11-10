package cluster

import (
	"context"
	"os/exec"
	"runtime"
)

// executable is name with the system's executable suffix.
func executable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// command is exec.CommandContext for PostgreSQL's programs: on Windows
// they open no window, as Polyfin Desktop has no console of its own.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	return cmd
}

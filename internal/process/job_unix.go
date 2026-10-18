//go:build !windows

package process

// KillChildrenOnExit does nothing on Linux and macOS, as before: Polyfin
// stops its programs as it stops on SIGTERM or SIGINT, and a container
// stopping ends them all.
func KillChildrenOnExit() error { return nil }

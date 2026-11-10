//go:build !windows

package cluster

import "os/exec"

func hideWindow(*exec.Cmd) {}

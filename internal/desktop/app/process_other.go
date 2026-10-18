//go:build !windows && !darwin && !linux

package app

import "syscall"

func parentDeath(*syscall.SysProcAttr) {}

func startTime(int) (int64, bool) { return 0, false }

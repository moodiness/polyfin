package app

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// parentDeath stops the server when the app dies.
func parentDeath(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGTERM
}

// startTime is when pid started, in clock ticks since boot: the 22nd
// field of its stat, after the name in parentheses.
func startTime(pid int) (int64, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	text := string(data)
	end := strings.LastIndexByte(text, ')')
	if end < 0 {
		return 0, false
	}
	fields := strings.Fields(text[end+1:])
	// The fields after the name start at the third.
	if len(fields) < 20 {
		return 0, false
	}
	start, err := strconv.ParseInt(fields[19], 10, 64)
	return start, err == nil
}

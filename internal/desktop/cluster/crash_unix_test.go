//go:build !windows

package cluster

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The lock file of a cluster that crashed is cleared, and the cluster
// recovers as it starts again.
func TestClusterRecoversFromACrashedPostmaster(t *testing.T) {
	bin := postgresBin(t)
	data := t.TempDir()
	ctx := context.Background()
	next := newCluster(t, bin, data)
	if _, err := next.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := next.Start(ctx); err != nil {
		t.Fatal(err)
	}
	stopAtEnd(t, next)
	// The cluster crashed, its lock file left behind.
	children := childProcesses(t, next.PID())
	process, err := os.FindProcess(next.PID())
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = process.Wait()
	waitGone(t, children)
	if _, err := os.Stat(filepath.Join(next.Dir, lockFile)); err != nil {
		t.Fatalf("the crash left no lock file: %v", err)
	}
	if err := next.Start(ctx); err != nil {
		t.Fatalf("Start after a crash: %v", err)
	}
	query(t, next.URL(), "SELECT 'ok'")
}

// childProcesses is the processes of the postmaster pid.
func childProcesses(t *testing.T, pid int) []int {
	t.Helper()
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatalf("pgrep: %v", err)
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		if n, err := strconv.Atoi(field); err == nil {
			pids = append(pids, n)
		}
	}
	return pids
}

// waitGone waits until the killed postmaster's children have ended, which
// they do once they see it gone, so that its shared memory is released.
func waitGone(t *testing.T, pids []int) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for _, pid := range pids {
		for syscall.Kill(pid, 0) == nil {
			if time.Now().After(deadline) {
				t.Fatalf("process %d of the killed cluster did not end", pid)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

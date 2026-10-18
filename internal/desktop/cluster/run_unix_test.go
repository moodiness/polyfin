//go:build !windows

package cluster

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A pg_ctl that leaves a process behind holding its output, as the
// postmaster does on Windows, does not hang the app: start gets the log
// file, pg_ctl.log, apart from the cluster's log, which pg_ctl -l opens
// itself; the other commands give up waiting for their output.
func TestRunDoesNotWaitForWhatAProgramLeaves(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"pg_ctl $1\"\nsleep 60 &\n"
	if err := os.WriteFile(filepath.Join(bin, "pg_ctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	c := &Cluster{Bin: bin, Dir: dir, Log: filepath.Join(dir, "log", "postgresql.log")}
	started := time.Now()
	if err := c.run(context.Background(), "pg_ctl", "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Errorf("start waited %s for the process it left", elapsed)
	}
	if log, _ := os.ReadFile(filepath.Join(dir, "log", "pg_ctl.log")); !strings.Contains(string(log), "pg_ctl start") {
		t.Errorf("start's output is not in pg_ctl.log: %q", log)
	}
	if _, err := os.Stat(c.Log); !os.IsNotExist(err) {
		t.Errorf("the app opened the cluster's log itself: %v", err)
	}
	started = time.Now()
	_ = c.run(context.Background(), "pg_ctl", "status")
	if elapsed := time.Since(started); elapsed > 50*time.Second {
		t.Errorf("status waited %s for the process it left", elapsed)
	}
}

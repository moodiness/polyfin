//go:build !windows

package cluster

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A pg_ctl that leaves a process behind holding its output, as the
// postmaster does on Windows, does not hang the app: start gets the log
// file, pg_ctl.log, apart from the cluster's log, which pg_ctl -l opens
// itself; the other commands give up waiting for their output.
func TestRunDoesNotWaitForWhatAProgramLeaves(t *testing.T) {
	t.Parallel()
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

// A port another program takes between its choice and PostgreSQL's start,
// given it as a free port meanwhile, gives way to another one: PostgreSQL
// ends at once on a port taken, and the start goes on on a port chosen
// again, which is kept.
func TestAPortTakenAsTheClusterStartsGivesWayToAnother(t *testing.T) {
	t.Parallel()
	bin := t.TempDir()
	pgCtl := filepath.Join(bin, "pg_ctl")
	// pg_ctl start records its port. The first start ends, as PostgreSQL
	// does on a port taken, once the test took that port; the next one
	// starts.
	script := `#!/bin/sh
for arg; do
	if [ "$prev" = -o ]; then echo "${arg#-p }" >> "$0.ports"; fi
	prev=$arg
done
if [ -e "$0.taken" ]; then exit 0; fi
while [ ! -e "$0.taken" ]; do sleep 0.01; done
echo "pg_ctl: could not start server" >&2
exit 1
`
	if err := os.WriteFile(pgCtl, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	c := &Cluster{Bin: bin, Dir: dir, Log: filepath.Join(dir, "log", "postgresql.log")}
	// A test that fails before taking the port does not leave pg_ctl
	// waiting.
	t.Cleanup(func() { _ = os.WriteFile(pgCtl+".taken", nil, 0o600) })
	started := make(chan error, 1)
	go func() { started <- c.startPostgres(context.Background()) }()
	var first string
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if ports, _ := os.ReadFile(pgCtl + ".ports"); strings.HasSuffix(string(ports), "\n") {
			first = strings.TrimSpace(string(ports))
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pg_ctl was never given a port")
		}
	}
	// Another program may have taken it already, which does as well.
	if listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", first)); err == nil {
		defer listener.Close()
	}
	if err := os.WriteFile(pgCtl+".taken", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-started; err != nil {
		t.Fatalf("start: %v", err)
	}
	ports, _ := os.ReadFile(pgCtl + ".ports")
	tried := strings.Fields(string(ports))
	if len(tried) != 2 || tried[1] == first || tried[1] != strconv.Itoa(c.port) {
		t.Fatalf("ports tried %v, the cluster's %d; the first one taken: %s", tried, c.port, first)
	}
	if kept, _ := os.ReadFile(filepath.Join(dir, portFile)); strings.TrimSpace(string(kept)) != tried[1] {
		t.Errorf("port kept %q, want %s", kept, tried[1])
	}
}

// A start on the port set in Port is not tried on another one, though
// another program holds it.
func TestTheSetPortIsTheOnlyOneTried(t *testing.T) {
	t.Parallel()
	bin := t.TempDir()
	pgCtl := filepath.Join(bin, "pg_ctl")
	script := "#!/bin/sh\necho start >> \"$0.starts\"\necho 'pg_ctl: could not start server' >&2\nexit 1\n"
	if err := os.WriteFile(pgCtl, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	dir := t.TempDir()
	c := &Cluster{Bin: bin, Dir: dir, Log: filepath.Join(dir, "log", "postgresql.log"), Port: listener.Addr().(*net.TCPAddr).Port}
	if err := c.startPostgres(context.Background()); err == nil || !strings.Contains(err.Error(), "could not start server") {
		t.Fatalf("start = %v, want pg_ctl's failure", err)
	}
	if starts, _ := os.ReadFile(pgCtl + ".starts"); strings.Count(string(starts), "start") != 1 {
		t.Errorf("starts: %q, want one", starts)
	}
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// embeddedBin is the folder of PostgreSQL 18's server programs the tests
// of the embedded database run, which POLYFIN_TEST_POSTGRES_BIN names.
func embeddedBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("POLYFIN_TEST_POSTGRES_BIN")
	if bin == "" {
		t.Skip("POLYFIN_TEST_POSTGRES_BIN is not set")
	}
	return bin
}

// syncBuffer is a log the database's watch and the test share.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func ping(ctx context.Context, url string) error {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return conn.Ping(ctx)
}

// The cluster is made in the folder postgresql of the data folder, with
// its log in log, started, started again by the watch when its postmaster
// dies, and stopped by stop.
func TestEmbeddedDatabaseIsWatchedAndStopped(t *testing.T) {
	t.Parallel()
	bin := embeddedBin(t)
	data := t.TempDir()
	logs := &syncBuffer{}
	ctx := context.Background()
	db := newEmbedded(bin, data, slog.New(slog.NewTextHandler(logs, nil)))
	db.interval = 200 * time.Millisecond
	if err := db.start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			db.stop()
		}
	})
	if db.cluster.Dir != filepath.Join(data, "postgresql") || db.cluster.Log != filepath.Join(data, "log", "postgresql.log") {
		t.Errorf("folders: %s, %s", db.cluster.Dir, db.cluster.Log)
	}
	if err := ping(ctx, db.url()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	pid := db.cluster.PID()
	process, err := os.FindProcess(pid)
	if err != nil || pid == 0 {
		t.Fatalf("postmaster %d: %v", pid, err)
	}
	if err := process.Kill(); err != nil {
		t.Fatalf("kill the postmaster: %v", err)
	}
	deadline := time.Now().Add(time.Minute)
	for !strings.Contains(logs.String(), "starting it again") || ping(ctx, db.url()) != nil {
		if time.Now().After(deadline) {
			t.Fatalf("the database did not start again:\n%s", logs.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	db.stop()
	stopped = true
	if db.cluster.Running(ctx) {
		t.Error("the database still runs after stop")
	}
	if _, err := os.Stat(filepath.Join(db.cluster.Dir, "postmaster.pid")); !os.IsNotExist(err) {
		t.Errorf("lock file after stop: %v", err)
	}
	if !strings.Contains(logs.String(), "The database stopped") {
		t.Errorf("no stop logged:\n%s", logs.String())
	}
}

// polyfin serve with POLYFIN_DATABASE=embedded starts its database before
// it serves, and stops it once it has stopped serving.
func TestServeRunsItsOwnDatabase(t *testing.T) {
	t.Parallel()
	bin := embeddedBin(t)
	data := t.TempDir()
	// serve sends crashes to data's crash.log through the runtime, which
	// keeps its own copy of the file open for the life of the process:
	// Windows deletes no open file, so it is given back before data goes.
	t.Cleanup(func() { _ = debug.SetCrashOutput(nil, debug.CrashOptions{}) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	env := map[string]string{
		"POLYFIN_DATABASE": "embedded",
		"POLYFIN_DATA_DIR": data,
		"POLYFIN_LISTEN":   address,
		"PATH":             bin,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		code  int
		again bool
	}
	done := make(chan result, 1)
	go func() {
		code, again := runServe(ctx, func(name string) string { return env[name] })
		done <- result{code, again}
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		resp, err := client.Get("http://" + address + "/healthz")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case r := <-done:
			t.Fatalf("serve ended before it served: %+v", r)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("serve did not answer /healthz")
		}
		time.Sleep(200 * time.Millisecond)
	}
	lock := filepath.Join(data, "postgresql", "postmaster.pid")
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("the database does not run while serving: %v", err)
	}
	cancel()
	select {
	case r := <-done:
		if r.code != 0 || r.again {
			t.Errorf("serve ended with %+v, want 0 and no start again", r)
		}
	case <-time.After(2 * time.Minute):
		t.Fatal("serve did not stop")
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the database still runs after serve: %v", err)
	}
}

// A configuration that sets both POLYFIN_DATABASE and the URL stops
// polyfin serve before anything starts.
func TestServeRefusesTwoDatabases(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"POLYFIN_DATABASE":     "embedded",
		"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin",
		"POLYFIN_DATA_DIR":     t.TempDir(),
	}
	code, again := runServe(context.Background(), func(name string) string { return env[name] })
	if code != 1 || again {
		t.Errorf("runServe = %d, %v; want 1, false", code, again)
	}
}

func TestServerProgramsAreFoundInPath(t *testing.T) {
	t.Parallel()
	empty, bin := t.TempDir(), t.TempDir()
	name := "pg_ctl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	path := strings.Join([]string{"", empty, bin}, string(filepath.ListSeparator))
	if got, err := serverPrograms(path); err != nil || got != bin {
		t.Errorf("serverPrograms = %q, %v; want %q", got, err, bin)
	}
	if _, err := serverPrograms(empty); err == nil || !strings.Contains(err.Error(), "pg_ctl") {
		t.Errorf("serverPrograms without pg_ctl: %v", err)
	}
	if usage := fmt.Sprint(usage); !strings.Contains(usage, "POLYFIN_DATABASE ") {
		t.Error("the usage text does not tell POLYFIN_DATABASE")
	}
}

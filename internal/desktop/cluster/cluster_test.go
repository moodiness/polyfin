package cluster

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/desktop"
)

// The tests that run PostgreSQL need its server programs, version 18:
// POLYFIN_TEST_POSTGRES_BIN names their folder. POLYFIN_TEST_POSTGRES_PORT
// gives them a port, else they take a free one: internal/desktop/app reads
// it too, so a fixed port needs the two packages tested apart.
func postgresBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("POLYFIN_TEST_POSTGRES_BIN")
	if bin == "" {
		t.Skip("POLYFIN_TEST_POSTGRES_BIN is not set")
	}
	return bin
}

func testPort(t *testing.T) int {
	t.Helper()
	if text := os.Getenv("POLYFIN_TEST_POSTGRES_PORT"); text != "" {
		port, err := strconv.Atoi(text)
		if err != nil {
			t.Fatalf("POLYFIN_TEST_POSTGRES_PORT: %v", err)
		}
		return port
	}
	return 0
}

func newCluster(t *testing.T, bin, data string) *Cluster {
	t.Helper()
	return &Cluster{Bin: bin, Dir: filepath.Join(data, "postgresql"), Log: filepath.Join(data, "log", "postgresql.log"), Port: testPort(t)}
}

func stopAtEnd(t *testing.T, c *Cluster) {
	t.Cleanup(func() {
		if err := c.Stop(context.Background()); err != nil {
			t.Errorf("stop: %v", err)
		}
	})
}

func query(t *testing.T, url, sql string) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var answer string
	if err := conn.QueryRow(ctx, sql).Scan(&answer); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return answer
}

// The first start makes the cluster, with a password only the user reads,
// on the loopback alone; the second reuses it and its password.
func TestClusterIsMadeOnceAndReused(t *testing.T) {
	bin := postgresBin(t)
	data := t.TempDir()
	ctx := context.Background()
	c := newCluster(t, bin, data)
	created, err := c.Prepare(ctx)
	if err != nil || !created {
		t.Fatalf("Prepare: created %v, %v", created, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(c.Dir, passwordFile))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("password file: %v, %v", info, err)
		}
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := query(t, c.URL(), "SELECT current_setting('listen_addresses') || ' ' || current_database() || ' ' || current_setting('server_version_num')::int / 10000"); got != "127.0.0.1 polyfin 18" {
		t.Errorf("settings = %q", got)
	}
	if got := query(t, c.URL(), "SELECT current_setting('unix_socket_directories')"); got != "" {
		t.Errorf("unix_socket_directories = %q", got)
	}
	// The ICU root collation sorts letters regardless of case.
	if got := query(t, c.URL(), "SELECT string_agg(x, ',' ORDER BY x) FROM unnest(ARRAY['b', 'B', 'a', 'Z']) x"); got != "a,b,B,Z" {
		t.Errorf("collation order = %q", got)
	}
	// A wrong password is refused.
	wrong := strings.Replace(c.URL(), c.password, "wrong", 1)
	if conn, err := pgx.Connect(ctx, wrong); err == nil {
		conn.Close(ctx)
		t.Error("a wrong password was accepted")
	}
	firstURL, pid := c.URL(), c.PID()
	if pid == 0 {
		t.Fatal("no postmaster process recorded")
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if c.Running(ctx) {
		t.Fatal("the cluster still runs after Stop")
	}

	again := newCluster(t, bin, data)
	created, err = again.Prepare(ctx)
	if err != nil || created {
		t.Fatalf("second Prepare: created %v, %v", created, err)
	}
	if err := again.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	stopAtEnd(t, again)
	if again.URL() != firstURL && again.Port != 0 {
		t.Errorf("second start URL changed with a fixed port")
	}
	if got := query(t, again.URL(), "SELECT current_database()"); got != Database {
		t.Errorf("database = %q", got)
	}
}

// A cluster left running by a crash of the app is stopped, then started
// again.
func TestClusterLeftRunningIsRestarted(t *testing.T) {
	bin := postgresBin(t)
	data := t.TempDir()
	ctx := context.Background()
	c := newCluster(t, bin, data)
	if _, err := c.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	stopAtEnd(t, c)
	leftPID := c.PID()

	// The app crashed: a new one finds the cluster running.
	next := newCluster(t, bin, data)
	if _, err := next.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := next.Start(ctx); err != nil {
		t.Fatalf("Start over a running cluster: %v", err)
	}
	if next.PID() == leftPID {
		t.Error("the cluster left running was not restarted")
	}
	query(t, next.URL(), "SELECT 'ok'")

}

// A lock file whose process number another process holds, the test's
// own, is removed: nothing answers on its port.
func TestRecoverRemovesAStaleLock(t *testing.T) {
	dir := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	lock := filepath.Join(dir, lockFile)
	content := strconv.Itoa(os.Getpid()) + "\n" + dir + "\n1700000000\n" + strconv.Itoa(port) + "\n\n127.0.0.1\n"
	if err := os.WriteFile(lock, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Cluster{Bin: t.TempDir(), Dir: dir, password: "unused"}
	if pid := c.PID(); pid != os.Getpid() {
		t.Errorf("PID = %d", pid)
	}
	if err := c.recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the stale lock file is still there: %v", err)
	}
}

// Something that is not PostgreSQL on the lock's port is not the cluster.
func TestProbeTellsNoClusterFromAnotherService(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			conn.Close()
		}
	}()
	c := &Cluster{password: "unused"}
	if got := c.probe(context.Background(), listener.Addr().(*net.TCPAddr).Port); got != none {
		t.Errorf("probe = %v", got)
	}
}

func TestInitdbArgumentsAndURL(t *testing.T) {
	args := strings.Join(initdbArgs("/data/postgresql.new", "/data/.password-1"), " ")
	for _, want := range []string{"-U polyfin", "--pwfile=/data/.password-1", "--auth=scram-sha-256", "--encoding=UTF8", "--icu-locale=und"} {
		if !strings.Contains(args, want) {
			t.Errorf("initdb arguments %q lack %q", args, want)
		}
	}
	c := &Cluster{password: "abc123", port: 54321}
	if got := c.URL(); got != "postgresql://polyfin:abc123@127.0.0.1:54321/polyfin?sslmode=disable" {
		t.Errorf("URL = %q", got)
	}
	env := environment([]string{"PATH=/bin", "PGDATA=/elsewhere", "PGPASSWORD=x", "HOME=/home/a"})
	if strings.Join(env, " ") != "PATH=/bin HOME=/home/a" {
		t.Errorf("environment = %v", env)
	}
	if desktop.Executable("pg_ctl") == "" {
		t.Fatal("no executable name")
	}
}

// The port chosen at first start is kept while it is free.
func TestChosenPortIsKept(t *testing.T) {
	c := &Cluster{Dir: t.TempDir()}
	first, err := c.choosePort()
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.choosePort()
	if err != nil || second != first {
		t.Fatalf("second port %d, %v; first %d", second, err, first)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first)))
	if err != nil {
		t.Skipf("the port was taken meanwhile: %v", err)
	}
	defer listener.Close()
	third, err := c.choosePort()
	if err != nil || third == first {
		t.Fatalf("a taken port was kept: %d, %v", third, err)
	}
}

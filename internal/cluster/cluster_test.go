package cluster

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The tests that run PostgreSQL need its server programs, version 18:
// POLYFIN_TEST_POSTGRES_BIN names their folder. POLYFIN_TEST_POSTGRES_PORT
// gives them a port, else they take a free one: internal/desktop/app reads
// it too, so a fixed port needs the two packages tested apart, and the
// tests that run PostgreSQL here run one at a time.
func postgresBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("POLYFIN_TEST_POSTGRES_BIN")
	if bin == "" {
		t.Skip("POLYFIN_TEST_POSTGRES_BIN is not set")
	}
	if os.Getenv("POLYFIN_TEST_POSTGRES_PORT") != "" {
		fixedPort.Lock()
		t.Cleanup(fixedPort.Unlock)
	}
	return bin
}

// fixedPort is held by the test whose cluster has the fixed port.
var fixedPort sync.Mutex

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
	t.Parallel()
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
	t.Parallel()
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

// lightValues are the settings light gives, as current_setting shows them.
var lightValues = map[string]string{
	"shared_buffers": "32MB", "max_connections": "20", "max_locks_per_transaction": "512", "max_worker_processes": "2",
	"max_parallel_workers": "0", "autovacuum_max_workers": "1", "max_wal_size": "256MB", "jit": "off",
}

func checkLight(t *testing.T, c *Cluster) {
	t.Helper()
	for name, want := range lightValues {
		if got := query(t, c.URL(), "SELECT current_setting('"+name+"')"); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	conf, err := os.ReadFile(filepath.Join(c.Dir, "postgresql.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(conf), lightMark); n != 1 {
		t.Errorf("postgresql.conf holds the light settings %d times", n)
	}
}

// A new cluster is light. A cluster made before the light settings, left
// running by a crash of the app, is stopped, gets them, and starts again
// with its data.
func TestClusterIsLightNewOrOlder(t *testing.T) {
	t.Parallel()
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
	checkLight(t, c)
	execute(t, c.URL(), "CREATE TABLE kept AS SELECT 'there' AS word")
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	// An older version made it: its postgresql.conf lacks the light
	// settings, and the crash of the app left it running.
	path := filepath.Join(c.Dir, "postgresql.conf")
	conf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(conf), light, "", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	older := newCluster(t, bin, data)
	if _, err := older.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	olderURL := startedByHand(t, older)
	stopAtEnd(t, older)
	if got := query(t, olderURL, "SELECT current_setting('max_connections')"); got != "100" {
		t.Fatalf("the older cluster runs with max_connections %q", got)
	}

	next := newCluster(t, bin, data)
	if _, err := next.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := next.Start(ctx); err != nil {
		t.Fatalf("Start over an older running cluster: %v", err)
	}
	stopAtEnd(t, next)
	checkLight(t, next)
	if got := query(t, next.URL(), "SELECT word FROM kept"); got != "there" {
		t.Errorf("kept = %q", got)
	}
}

// startedByHand starts c with pg_ctl alone, on the port it keeps, as an
// older version would, and returns its URL.
func startedByHand(t *testing.T, c *Cluster) string {
	t.Helper()
	port, err := c.choosePort()
	if err != nil {
		t.Fatal(err)
	}
	c.port = port
	if err := c.run(context.Background(), "pg_ctl", "start", "-D", c.Dir, "-l", c.Log, "-w", "-o", "-p "+strconv.Itoa(port)); err != nil {
		t.Fatal(err)
	}
	return c.URL()
}

// lighten appends the light settings once, keeps the file's mode, and
// leaves no file beside it.
func TestLightenAppendsOnce(t *testing.T) {
	t.Parallel()
	c := &Cluster{Dir: t.TempDir()}
	path := filepath.Join(c.Dir, "postgresql.conf")
	older := "max_connections = 100\n" + settings
	if err := os.WriteFile(path, []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := c.lighten(); err != nil {
			t.Fatal(err)
		}
	}
	conf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(conf) != older+light {
		t.Errorf("postgresql.conf =\n%s", conf)
	}
	if info, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Errorf("mode: %v, %v", info, err)
	}
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Errorf("a file is left beside: %v", err)
	}
	// The settings come last: they win over the older lines.
	if !strings.HasSuffix(string(conf), "jit = off\n") {
		t.Error("the light settings are not last")
	}
}

// A lock file whose process number another process holds, the test's
// own, is removed: nothing answers on its port.
func TestRecoverRemovesAStaleLock(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	if executable("pg_ctl") == "" {
		t.Fatal("no executable name")
	}
}

// The port chosen at first start is kept while it is free, and another is
// chosen once it is taken. Between two choices, another test, of this
// package or of another one running at the same time, may be given the
// chosen port, if only to try it as choosePort does: the choice is then
// made again, in a folder of its own. A cluster that never keeps its
// port fails every try.
func TestChosenPortIsKept(t *testing.T) {
	t.Parallel()
	for attempt := 1; !chosenPortIsKept(t); attempt++ {
		if attempt == 5 {
			t.Fatal("the chosen port was not kept at any of 5 tries")
		}
	}
}

// chosenPortIsKept checks that a cluster keeps the port it chose, then
// leaves it once taken. It reports false when the port changed between
// two choices, or was taken before the test could take it, which another
// program using the port meanwhile explains.
func chosenPortIsKept(t *testing.T) bool {
	t.Helper()
	c := &Cluster{Dir: t.TempDir()}
	first, err := c.choosePort()
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.choosePort()
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Logf("second port %d; first %d", second, first)
		return false
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first)))
	if err != nil {
		t.Logf("port %d taken meanwhile: %v", first, err)
		return false
	}
	defer listener.Close()
	third, err := c.choosePort()
	if err != nil || third == first {
		t.Fatalf("a taken port was kept: %d, %v", third, err)
	}
	return true
}

// A cluster another major version of PostgreSQL made is refused before
// anything runs, with a message that says how to move it.
func TestPrepareRefusesAnotherMajorVersion(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "postgresql")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, versionFile), []byte("17\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, passwordFile), []byte("abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Cluster{Bin: filepath.Join(t.TempDir(), "none"), Dir: dir, Log: filepath.Join(dir, "log")}
	_, err := c.Prepare(context.Background())
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL 17") || !strings.Contains(err.Error(), "PostgreSQL "+Major) {
		t.Fatalf("Prepare = %v, want a major version error", err)
	}
	if err := os.WriteFile(filepath.Join(dir, versionFile), []byte(Major+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare of a %s cluster: %v", Major, err)
	}
}

// Package cluster runs the PostgreSQL 18 cluster of Polyfin Desktop: made
// at first start in the data folder, with a random password kept in a file
// only the user can read, listening on 127.0.0.1 alone, started before the
// server and stopped after it. A cluster a crash of the app left running is
// stopped, and the lock file of one that crashed is cleared, before it
// starts again.
package cluster

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moodiness/polyfin/internal/desktop"
)

const (
	// User is the cluster's only role, its superuser, which the server
	// signs in as; Database is the server's database.
	User     = "polyfin"
	Database = "polyfin"

	// passwordFile and portFile are kept in the cluster's folder, which
	// PostgreSQL makes readable by the user only.
	passwordFile = "polyfin.password"
	portFile     = "polyfin.port"
	lockFile     = "postmaster.pid"

	// startWait bounds pg_ctl's wait for the cluster to start, crash
	// recovery included, and to stop.
	startWait = 300 * time.Second
	stopWait  = 120 * time.Second
	// probeWait bounds the attempt to reach a cluster a crash left
	// running.
	probeWait = 5 * time.Second
)

// settings are appended to the postgresql.conf initdb writes: no Unix
// socket, which would go in a shared folder, and TCP on the loopback
// only. The port is given at each start.
const settings = `
# Polyfin Desktop: the server reaches the cluster over TCP on the
# loopback, with the password of polyfin.password.
listen_addresses = '127.0.0.1'
unix_socket_directories = ''
`

// Cluster is a PostgreSQL cluster in Dir, run with the programs of Bin.
type Cluster struct {
	// Bin is the folder of initdb, pg_ctl and postgres.
	Bin string
	// Dir is the cluster's data folder.
	Dir string
	// Log is the file PostgreSQL logs to.
	Log string
	// Port is the port to listen on; 0 keeps the one chosen at first
	// start while it is free, else chooses another free one.
	Port   int
	Logger *slog.Logger

	password string
	port     int
}

// Prepare makes the cluster when Dir has none, and reads its password. It
// reports whether it made it. A cluster whose making was interrupted is
// made again: it is made beside Dir and renamed into place once complete.
func (c *Cluster) Prepare(ctx context.Context) (bool, error) {
	created := false
	if _, err := os.Stat(filepath.Join(c.Dir, "PG_VERSION")); errors.Is(err, os.ErrNotExist) {
		if err := c.initialize(ctx); err != nil {
			return false, err
		}
		created = true
	} else if err != nil {
		return false, err
	}
	password, err := os.ReadFile(filepath.Join(c.Dir, passwordFile))
	if err != nil {
		return created, fmt.Errorf("read the database password: %w", err)
	}
	c.password = strings.TrimSpace(string(password))
	if c.password == "" {
		return created, fmt.Errorf("the database password file %s is empty", filepath.Join(c.Dir, passwordFile))
	}
	return created, nil
}

// initialize runs initdb into a folder beside Dir, then renames it.
func (c *Cluster) initialize(ctx context.Context) error {
	parent := filepath.Dir(c.Dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	building := c.Dir + ".new"
	if err := os.RemoveAll(building); err != nil {
		return err
	}
	password, err := newPassword()
	if err != nil {
		return err
	}
	// initdb reads the password from a file, never from its command line.
	pwfile, err := os.CreateTemp(parent, ".password-*")
	if err != nil {
		return err
	}
	defer os.Remove(pwfile.Name())
	if err := pwfile.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		pwfile.Close()
		return err
	}
	_, err = pwfile.WriteString(password + "\n")
	if closeErr := pwfile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	c.logger().Info("Creating the database cluster", "folder", c.Dir)
	if err := c.run(ctx, "initdb", initdbArgs(building, pwfile.Name())...); err != nil {
		return fmt.Errorf("create the database cluster: %w", err)
	}
	conf, err := os.OpenFile(filepath.Join(building, "postgresql.conf"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, err = conf.WriteString(settings)
	if closeErr := conf.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(building, passwordFile), []byte(password+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(building, c.Dir)
}

// initdbArgs makes a cluster whose superuser is User with the password of
// pwfile, signing in with SCRAM, in UTF-8. The C locale is the same on
// every system, Windows' locale names included; ICU's root collation sorts
// text as Docker's en_US.utf8 cluster does, on every system alike.
func initdbArgs(dir, pwfile string) []string {
	return []string{"-D", dir, "-U", User, "--pwfile=" + pwfile, "--auth=scram-sha-256",
		"--encoding=UTF8", "--locale=C", "--locale-provider=icu", "--icu-locale=und", "--no-instructions"}
}

// newPassword is 24 random bytes in hexadecimal, which a URL carries
// unescaped.
func newPassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Start starts the cluster, after stopping one a crash of the app left
// running or clearing the lock file of one that crashed, and creates the
// server's database when it has none. Prepare must have run.
func (c *Cluster) Start(ctx context.Context) error {
	if err := c.recover(ctx); err != nil {
		return err
	}
	port, err := c.choosePort()
	if err != nil {
		return err
	}
	c.port = port
	if err := os.MkdirAll(filepath.Dir(c.Log), 0o700); err != nil {
		return err
	}
	c.logger().Info("Starting the database", "port", port)
	err = c.run(ctx, "pg_ctl", "start", "-D", c.Dir, "-l", c.Log, "-w", "-t", seconds(startWait),
		"-o", "-p "+strconv.Itoa(port))
	if err != nil {
		return fmt.Errorf("start the database (see %s): %w", c.Log, err)
	}
	if err := c.createDatabase(ctx); err != nil {
		return err
	}
	return nil
}

// Stop stops the cluster, ending its connections, and waits until it has
// stopped: at once when fast shutdown does not finish in time.
func (c *Cluster) Stop(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(c.Dir, lockFile)); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	err := c.run(ctx, "pg_ctl", "stop", "-D", c.Dir, "-m", "fast", "-w", "-t", seconds(stopWait))
	if err == nil {
		c.logger().Info("The database stopped")
		return nil
	}
	if !c.Running(ctx) {
		return nil
	}
	c.logger().Warn("The database did not stop in time: stopping it at once", "error", err)
	return c.run(ctx, "pg_ctl", "stop", "-D", c.Dir, "-m", "immediate", "-w", "-t", seconds(stopWait))
}

// Running tells whether pg_ctl finds the cluster running.
func (c *Cluster) Running(ctx context.Context) bool {
	return c.run(ctx, "pg_ctl", "status", "-D", c.Dir) == nil
}

// URL is the server's database URL, with the password: never logged.
func (c *Cluster) URL() string {
	return c.url(Database)
}

func (c *Cluster) url(database string) string {
	u := url.URL{Scheme: "postgresql", User: url.UserPassword(User, c.password),
		Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(c.port)), Path: "/" + database, RawQuery: "sslmode=disable"}
	return u.String()
}

// PID is the process of the running cluster's postmaster, 0 when none is
// recorded.
func (c *Cluster) PID() int {
	pid, _, _ := readLock(filepath.Join(c.Dir, lockFile))
	return pid
}

// ListeningPort is the port the cluster listens on once started.
func (c *Cluster) ListeningPort() int { return c.port }

// choosePort is Port when set; else the port chosen at an earlier start
// while it is free, else a free port, kept for the next starts.
func (c *Cluster) choosePort() (int, error) {
	if c.Port != 0 {
		return c.Port, nil
	}
	path := filepath.Join(c.Dir, portFile)
	if data, err := os.ReadFile(path); err == nil {
		if port, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && port > 0 && free(port) {
			return port, nil
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, os.WriteFile(path, []byte(strconv.Itoa(port)+"\n"), 0o600)
}

func free(port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	return listener.Close() == nil
}

// state is what answers on the port a lock file names.
type state int

const (
	// none: nothing answers as PostgreSQL.
	none state = iota
	// ours: the cluster, which takes its password.
	ours
	// other: another PostgreSQL, which refuses the password.
	other
)

// recover deals with the lock file a cluster left: one still running,
// left by a crash of the app, is stopped; the lock of one no longer
// running, whose process number another process may have taken since, is
// removed, and PostgreSQL recovers its data as it starts.
func (c *Cluster) recover(ctx context.Context) error {
	path := filepath.Join(c.Dir, lockFile)
	pid, port, err := readLock(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || port == 0 {
		c.logger().Warn("The database's lock file cannot be read: removing it", "error", err)
		return os.Remove(path)
	}
	switch c.probe(ctx, port) {
	case ours:
		c.logger().Warn("The database was left running: stopping it", "pid", pid)
		if err := c.Stop(ctx); err != nil {
			return fmt.Errorf("stop the database left running: %w", err)
		}
		return nil
	default:
		c.logger().Warn("The database stopped without removing its lock file: removing it", "pid", pid)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
}

// probe tells what answers on port. A PostgreSQL that takes the password,
// or that is starting, recovering or stopping, is the cluster; one that
// refuses the password is another.
func (c *Cluster) probe(ctx context.Context, port int) state {
	saved := c.port
	c.port = port
	address := c.url("postgres")
	c.port = saved
	attempt, cancel := context.WithTimeout(ctx, probeWait)
	defer cancel()
	conn, err := pgx.Connect(attempt, address)
	if err == nil {
		_ = conn.Close(ctx)
		return ours
	}
	var pgErr *pgconn.PgError
	switch {
	case !errors.As(err, &pgErr):
		return none
	case pgErr.Code == "57P03":
		// The database is starting up, recovering or shutting down.
		return ours
	default:
		return other
	}
}

// readLock reads the process number and the port of a lock file: its
// first and fourth lines.
func readLock(path string) (pid, port int, err error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for line := 1; line <= 4 && scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		switch line {
		case 1:
			pid, _ = strconv.Atoi(text)
		case 4:
			port, _ = strconv.Atoi(text)
		}
	}
	return pid, port, scanner.Err()
}

// createDatabase creates the server's database when the cluster has none:
// at first start, or after a first start interrupted before it.
func (c *Cluster) createDatabase(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, c.url("postgres"))
	if err != nil {
		return fmt.Errorf("connect to the database: %w", err)
	}
	defer conn.Close(ctx)
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = $1)", Database).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	c.logger().Info("Creating the database", "database", Database)
	_, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{Database}.Sanitize())
	return err
}

// waitDelay bounds the wait for a program's output once it has ended:
// a program that leaves a process behind holding its output must not hang
// the app.
const waitDelay = 10 * time.Second

// run runs one of PostgreSQL's programs, its output kept for the error.
// pg_ctl start leaves the postmaster running, which inherits pg_ctl's
// output on Windows: it gets a file, never a pipe the app waits on.
func (c *Cluster) run(ctx context.Context, program string, args ...string) error {
	cmd := desktop.Command(ctx, filepath.Join(c.Bin, desktop.Executable(program)), args...)
	cmd.Env = environment(os.Environ())
	cmd.WaitDelay = waitDelay
	var out []byte
	var err error
	if len(args) > 0 && args[0] == "start" {
		out, err = c.runLogged(cmd)
	} else {
		out, err = cmd.CombinedOutput()
	}
	if err != nil {
		var exit *exec.ExitError
		if message := strings.TrimSpace(string(out)); message != "" && errors.As(err, &exit) {
			return fmt.Errorf("%s: %w: %s", program, err, lastLines(message, 5))
		}
		return fmt.Errorf("%s: %w", program, err)
	}
	return nil
}

// runLogged runs cmd with its own output in a file of its own, pg_ctl.log
// beside the cluster's log, and returns that output. The cluster's log is
// left to pg_ctl -l alone: Windows refuses to open a file a second time
// while the app holds it.
func (c *Cluster) runLogged(cmd *exec.Cmd) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(c.Log), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(c.programLog(), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	cmd.Stdout, cmd.Stderr = file, file
	runErr := cmd.Run()
	if runErr == nil {
		return nil, nil
	}
	out := make([]byte, 64<<10)
	n, _ := file.ReadAt(out, 0)
	return out[:n], runErr
}

// programLog is the file of pg_ctl start's own output.
func (c *Cluster) programLog() string {
	return filepath.Join(filepath.Dir(c.Log), "pg_ctl.log")
}

// environment is the app's, without the PG variables, which would point
// PostgreSQL's programs at another cluster or server.
func environment(env []string) []string {
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(strings.ToUpper(name), "PG") {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

func lastLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " ")
}

func seconds(d time.Duration) string { return strconv.Itoa(int(d / time.Second)) }

func (c *Cluster) logger() *slog.Logger {
	if c.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return c.Logger
}

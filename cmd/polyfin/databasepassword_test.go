package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/backup"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/process"
	"github.com/moodiness/polyfin/internal/testdb"
)

// hardPassword holds what a connection string must escape: a quote, a
// backslash, a space, and the characters that part a URL.
const hardPassword = `s3cret pa'ss@w:o/r%d#\?&=+`

// setPassword sets PGPASSWORD to password until the test ends, without a
// password file nor a service of whoever runs the tests, and returns the
// environment a Polyfin started with it alone has.
func setPassword(t *testing.T, password string) []string {
	t.Helper()
	t.Setenv("PGPASSFILE", filepath.Join(t.TempDir(), "none"))
	t.Setenv("PGSERVICE", "")
	t.Setenv(passwordVariable, password)
	return []string{passwordVariable + "=" + password}
}

// differences names the settings of where and how Polyfin connects that
// two configurations of the driver do not share.
func differences(a, b *pgconn.Config) []string {
	var names []string
	differ := func(name string, different bool) {
		if different {
			names = append(names, name)
		}
	}
	sameTLS := func(a, b *tls.Config) bool {
		if a == nil || b == nil {
			return a == b
		}
		return a.ServerName == b.ServerName && a.InsecureSkipVerify == b.InsecureSkipVerify
	}
	differ("Host", a.Host != b.Host)
	differ("Port", a.Port != b.Port)
	differ("Database", a.Database != b.Database)
	differ("User", a.User != b.User)
	differ("Password", a.Password != b.Password)
	differ("ConnectTimeout", a.ConnectTimeout != b.ConnectTimeout)
	differ("RuntimeParams", !maps.Equal(a.RuntimeParams, b.RuntimeParams))
	differ("KerberosSrvName", a.KerberosSrvName != b.KerberosSrvName)
	differ("KerberosSpn", a.KerberosSpn != b.KerberosSpn)
	differ("SSLNegotiation", a.SSLNegotiation != b.SSLNegotiation)
	differ("ValidateConnect", (a.ValidateConnect == nil) != (b.ValidateConnect == nil))
	differ("TLSConfig", !sameTLS(a.TLSConfig, b.TLSConfig))
	differ("Fallbacks", len(a.Fallbacks) != len(b.Fallbacks))
	for i := range min(len(a.Fallbacks), len(b.Fallbacks)) {
		differ("Fallbacks.Host", a.Fallbacks[i].Host != b.Fallbacks[i].Host)
		differ("Fallbacks.Port", a.Fallbacks[i].Port != b.Fallbacks[i].Port)
		differ("Fallbacks.TLSConfig", !sameTLS(a.Fallbacks[i].TLSConfig, b.Fallbacks[i].TLSConfig))
	}
	return names
}

// With PGPASSWORD set and a connection string without a password, the
// password is written into the string and the variable leaves the
// environment: the driver then reads the string, without the variable,
// into the configuration it read it into with the variable. That holds
// for URLs and for keyword=value pairs, for several hosts, a socket's
// folder and an IPv6 address, and for passwords a connection string must
// escape.
//
//nolint:paralleltest // sets PGPASSWORD in the process's environment, which carryPassword takes out of it
func TestThePasswordOfPGPASSWORDIsCarriedInTheConnectionString(t *testing.T) {
	connections := []string{
		"postgresql://polyfin@db.example:5432/polyfin",
		"postgresql://polyfin@db.example:5432/polyfin?sslmode=disable&pool_max_conns=4",
		"postgres://polyfin@db.example/polyfin?sslmode=require&application_name=polyfin",
		"postgresql://polyfin:@db.example/polyfin",
		"postgresql://db.example/polyfin",
		"postgresql://db.example",
		"postgresql://db.example?connect_timeout=5",
		"postgresql://po%20ly%40fin@db.example/poly%20fin",
		"postgresql://polyfin@db1.example:5432,db2.example:5433/polyfin?target_session_attrs=read-write",
		"postgresql://polyfin@db1.example,db2.example,db3.example/polyfin?sslmode=disable",
		"postgresql://polyfin@[2001:db8::1]:5432/polyfin",
		"postgresql:///polyfin?host=/var/run/postgresql",
		"postgresql://polyfin@%2Fvar%2Frun%2Fpostgresql/polyfin",
		"postgresql://polyfin@db.example/polyfin?user=other",
		"host=db.example port=5432 user=polyfin dbname=polyfin sslmode=disable pool_max_conns=4",
		"host=db.example dbname=polyfin",
		"host=/var/run/postgresql dbname=polyfin user=polyfin",
		"host=db1.example,db2.example port=5432,5433 user=polyfin dbname=polyfin",
		"host = db.example user = 'po ly\\'fin' dbname=poly\\ fin",
		"dbname=polyfin host=db.example application_name=polyfin\\",
		"  host=db.example dbname=polyfin  ",
	}
	passwords := []string{
		"plain", "it's", `back\slash`, `ends with a backslash\`, "two words", " spaces around ", "p@ss", "a:b", "a/b", "100%", "%41", "a#b",
		"?&=+", "'", `\'`, "mot de passe éè✓", hardPassword,
	}
	for _, connection := range connections {
		for _, password := range passwords {
			environ := setPassword(t, password)
			with, err := pgconn.ParseConfig(connection)
			if err != nil {
				t.Fatalf("%q: %v", connection, err)
			}
			if with.Password != password {
				t.Fatalf("%q: the driver does not take the password of PGPASSWORD", connection)
			}
			carried, err := carryPassword(connection, environ)
			if err != nil {
				t.Errorf("%q with %q: %v", connection, password, err)
				continue
			}
			if _, set := os.LookupEnv(passwordVariable); set {
				t.Errorf("%q with %q: PGPASSWORD is still in the environment", connection, password)
			}
			if carried == connection {
				t.Errorf("%q with %q: the connection string is as it was", connection, password)
			}
			read, err := pgconn.ParseConfig(carried)
			if err != nil {
				t.Errorf("%q with %q: the driver cannot read %q: %v", connection, password, carried, err)
				continue
			}
			if names := differences(with, read); len(names) > 0 || !sameConnection(with, read) {
				t.Errorf("%q with %q: the driver reads %q otherwise: %q", connection, password, carried, names)
			}
		}
	}
}

// A connection string that gives the password itself, or names a service
// whose file gives it, is left as it is, and so is the address of the
// database Polyfin runs itself: PGPASSWORD, which the driver did not use,
// leaves the environment all the same.
//
//nolint:paralleltest // sets PGPASSWORD in the process's environment, which carryPassword takes out of it
func TestAConnectionStringThatNeedsNoPGPASSWORDIsLeftAsItIs(t *testing.T) {
	services := filepath.Join(t.TempDir(), "pg_service.conf")
	if err := os.WriteFile(services, []byte("[polyfin]\nhost=db.example\nuser=polyfin\ndbname=polyfin\npassword=its own\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGSERVICEFILE", services)
	for _, connection := range []string{
		"postgresql://polyfin:its%20own@db.example:5432/polyfin",
		"postgresql://polyfin@db.example/polyfin?password=its%20own",
		"postgresql://polyfin:another@db.example/polyfin?password=its%20own",
		"host=db.example user=polyfin password='its own' dbname=polyfin",
		"password=its\\ own host=db.example",
		"service=polyfin",
		"postgresql://?service=polyfin",
		"",
	} {
		for _, password := range []string{"of the variable", "its own", ""} {
			environ := setPassword(t, password)
			var with *pgconn.Config
			if connection != "" {
				var err error
				if with, err = pgconn.ParseConfig(connection); err != nil || with.Password != "its own" {
					t.Fatalf("%q: %v", connection, err)
				}
			}
			carried, err := carryPassword(connection, environ)
			if err != nil || carried != connection {
				t.Errorf("%q with PGPASSWORD %q: %q, %v; want it as it was", connection, password, carried, err)
			}
			if _, set := os.LookupEnv(passwordVariable); set {
				t.Errorf("%q with PGPASSWORD %q: PGPASSWORD is still in the environment", connection, password)
			}
			if with == nil {
				continue
			}
			if read, err := pgconn.ParseConfig(carried); err != nil || len(differences(with, read)) > 0 || !sameConnection(with, read) {
				t.Errorf("%q with PGPASSWORD %q: the driver reads it otherwise without the variable: %v", connection, password, err)
			}
		}
	}
}

// PGPASSWORD stays in the environment, and the connection string as it
// was, when Polyfin was not started with the variable, when the driver
// cannot read the string, which the start then refuses as before, and
// when the driver does not read the string written with the password as
// it read the first with the variable: the error tells, without the
// password.
//
//nolint:paralleltest // sets PGPASSWORD in the process's environment, which carryPassword takes out of it
func TestPGPASSWORDStaysWhenTheConnectionStringCannotCarryIt(t *testing.T) {
	const connection, password = "postgresql://polyfin@db.example/polyfin", "of the variable"
	stays := func(t *testing.T, what, carried string) {
		t.Helper()
		if carried != connection {
			t.Errorf("%s: the connection string is %q", what, carried)
		}
		if got := os.Getenv(passwordVariable); got != password {
			t.Errorf("%s: PGPASSWORD is %q", what, got)
		}
	}
	environ := setPassword(t, password)
	if carried, err := carryPassword(connection, nil); err != nil {
		t.Error(err)
	} else {
		stays(t, "not started with the variable", carried)
	}
	if carried, err := carryPassword("postgresql://polyfin@[::1/polyfin", environ); err != nil || carried != "postgresql://polyfin@[::1/polyfin" || os.Getenv(passwordVariable) != password {
		t.Errorf("a string the driver cannot read: %q, %v, PGPASSWORD %q", carried, err, os.Getenv(passwordVariable))
	}
	for what, write := range map[string]func(connection, password string) string{
		"the string as it was": func(connection, _ string) string { return connection },
		"another password":     func(connection, password string) string { return withPassword(connection, password+" ") },
		"another host": func(_, password string) string {
			return withPassword("postgresql://polyfin@db.example.net/polyfin", password)
		},
		"another user": func(_, password string) string {
			return withPassword("postgresql://other@db.example/polyfin", password)
		},
		"one more setting": func(connection, password string) string {
			return withPassword(connection, password) + "?sslmode=disable"
		},
		"one more host": func(_, password string) string {
			return withPassword("postgresql://polyfin@db.example,db2.example/polyfin", password)
		},
		"a string not readable": func(string, string) string { return "postgresql://polyfin@[::1/polyfin" },
	} {
		carried, err := carryPasswordWith(connection, environ, write)
		if !errors.Is(err, errPasswordStays) {
			t.Errorf("%s: %v, want errPasswordStays", what, err)
		} else if strings.Contains(err.Error(), password) {
			t.Errorf("%s: the error holds the password: %v", what, err)
		}
		stays(t, what, carried)
	}
}

// sameConnection tells apart two configurations that differ by any one
// setting: the password, where Polyfin connects, how the connection is
// encrypted, the hosts tried next, what is asked of the server.
func TestSameConnectionTellsConfigurationsApart(t *testing.T) {
	t.Parallel()
	read := func(connection string) *pgconn.Config {
		t.Helper()
		config, err := pgconn.ParseConfig(connection)
		if err != nil {
			t.Fatal(err)
		}
		return config
	}
	const first = "postgresql://polyfin:password@db.example:5432/polyfin?sslmode=verify-full"
	if !sameConnection(read(first), read(first)) {
		t.Error("the same connection string read twice differs")
	}
	if !sameConnection(read(first), read("host=db.example port=5432 user=polyfin password=password dbname=polyfin sslmode=verify-full")) {
		t.Error("the same settings as a URL and as pairs differ")
	}
	for _, other := range []string{
		"postgresql://polyfin:Password@db.example:5432/polyfin?sslmode=verify-full",
		"postgresql://polyfin@db.example:5432/polyfin?sslmode=verify-full&passfile=/nowhere",
		"postgresql://other:password@db.example:5432/polyfin?sslmode=verify-full",
		"postgresql://polyfin:password@db.example.net:5432/polyfin?sslmode=verify-full",
		"postgresql://polyfin:password@db.example:5433/polyfin?sslmode=verify-full",
		"postgresql://polyfin:password@db.example:5432/other?sslmode=verify-full",
		"postgresql://polyfin:password@db.example:5432/polyfin?sslmode=disable",
		"postgresql://polyfin:password@db.example:5432/polyfin?sslmode=require",
		"postgresql://polyfin:password@db.example:5432/polyfin?sslmode=prefer",
		"postgresql://polyfin:password@db.example:5432,db2.example:5432/polyfin?sslmode=verify-full",
		"postgresql://polyfin:password@db.example:5432/polyfin?sslmode=verify-full&connect_timeout=5",
		"postgresql://polyfin:password@db.example:5432/polyfin?sslmode=verify-full&application_name=polyfin",
		"postgresql://polyfin:password@db.example:5432/polyfin?sslmode=verify-full&target_session_attrs=read-write",
	} {
		if sameConnection(read(first), read(other)) {
			t.Errorf("%q reads as %q", other, first)
		}
	}
	prefer, allow := read("postgresql://polyfin:password@db.example/polyfin?sslmode=prefer"), read("postgresql://polyfin:password@db.example/polyfin?sslmode=allow")
	if sameConnection(prefer, allow) {
		t.Error("sslmode prefer and allow, which try TLS in another order, read the same")
	}
}

// fakePgDump is the body of a pg_dump that records its arguments, one a
// line, and its environment in the folder $record names, then writes a
// dump to its --file.
const fakePgDump = `printf '%s\n' "$@" > "$record/args"
env > "$record/env"
for arg; do
	case $arg in --file=*) printf 'PGDMP' > "${arg#--file=}" ;; esac
done
`

// A password carried in the connection string is where backups and
// restores read one: pg_dump gets it in PGPASSWORD alone, never on its
// command line nor in the log, and the driver reads it from the string at
// the two places a restore connects from.
//
//nolint:paralleltest // sets PGPASSWORD in the process's environment, which carryPassword takes out of it
func TestBackupsAndRestoresReadThePasswordCarriedInTheConnectionString(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for pg_dump is a shell script")
	}
	for _, connection := range []string{
		"postgresql://polyfin@db.example:5432/polyfin?sslmode=disable",
		"postgresql://db.example:5432/polyfin?sslmode=disable",
		"host=db.example port=5432 user=polyfin dbname=polyfin sslmode=disable",
	} {
		carried, err := carryPassword(connection, setPassword(t, hardPassword))
		if err != nil || carried == connection {
			t.Fatalf("%q: %q, %v", connection, carried, err)
		}
		dir, record := t.TempDir(), t.TempDir()
		pgDump := filepath.Join(t.TempDir(), "pg_dump")
		if err := process.WriteExecutable(pgDump, strings.NewReader("#!/bin/sh\nrecord='"+record+"'\n"+fakePgDump), 0o700); err != nil {
			t.Fatal(err)
		}
		logs := &bytes.Buffer{}
		backups := backup.New(backup.Config{
			Folder:      func() string { return dir },
			DatabaseURL: carried,
			PgDump:      pgDump,
			Settings:    func() accounts.Settings { return accounts.Settings{BackupsKept: 3} },
			Logger:      slog.New(slog.NewTextHandler(logs, nil)),
		})
		if err := backups.Run(t.Context()); err != nil {
			t.Fatalf("%q: %v", connection, err)
		}
		args, _ := os.ReadFile(filepath.Join(record, "args"))
		env, _ := os.ReadFile(filepath.Join(record, "env"))
		if !strings.Contains(string(args), "--dbname=") || strings.Contains(string(args), "s3cret") || strings.Contains(string(args), "password=") {
			t.Errorf("%q: pg_dump's arguments:\n%s", connection, args)
		}
		var passwords []string
		for line := range strings.Lines(string(env)) {
			if line = strings.TrimSuffix(line, "\n"); strings.Contains(line, "s3cret") {
				passwords = append(passwords, line)
			}
		}
		if !slices.Equal(passwords, []string{passwordVariable + "=" + hardPassword}) {
			t.Errorf("%q: pg_dump's environment holds the password in %q", connection, passwords)
		}
		if strings.Contains(logs.String(), "s3cret") {
			t.Errorf("%q: the log holds the password: %s", connection, logs)
		}
		// A restore reads the connection string twice more: for the
		// connection it restores through, and for a pool once restored.
		config, err := pgx.ParseConfig(carried)
		if err != nil || config.Password != hardPassword || config.Host != "db.example" {
			t.Errorf("%q: the restore's connection: %v", connection, err)
		}
		pool, err := pgxpool.New(t.Context(), carried)
		if err != nil {
			t.Fatalf("%q: the pool after a restore: %v", connection, err)
		}
		if got := pool.Config().ConnConfig; got.Password != hardPassword || got.Host != "db.example" {
			t.Errorf("%q: the pool after a restore does not read the password", connection)
		}
		pool.Close()
	}
}

// The tests' database, reached by a URL without its password and
// PGPASSWORD, still opens once the password is carried in the URL and the
// variable is out of the environment: at start, and as a restore does.
//
//nolint:paralleltest // sets PGPASSWORD in the process's environment, which carryPassword takes out of it
func TestTheDatabaseOpensWithThePasswordCarriedFromPGPASSWORD(t *testing.T) {
	address, err := url.Parse(testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	password, has := address.User.Password()
	if !has {
		t.Skip("the tests' database URL has no password")
	}
	address.User = url.User(address.User.Username())
	carried, err := carryPassword(address.String(), setPassword(t, password))
	if err != nil || carried == address.String() {
		t.Fatalf("the URL was not written with the password: %v", err)
	}
	if _, set := os.LookupEnv(passwordVariable); set {
		t.Fatal("PGPASSWORD is still in the environment")
	}
	pool, err := database.Connect(t.Context(), carried, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("at start: %v", err)
	}
	pool.Close()
	conn, err := pgx.Connect(t.Context(), carried)
	if err != nil {
		t.Fatalf("the restore's connection: %v", err)
	}
	_ = conn.Close(t.Context())
	restored, err := pgxpool.New(t.Context(), carried)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.Ping(t.Context()); err != nil {
		t.Fatalf("the pool after a restore: %v", err)
	}
}

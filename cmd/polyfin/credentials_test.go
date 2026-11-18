package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/ocr"
	"github.com/moodiness/polyfin/internal/process"
)

// The credentials of the tests' environment: the key in base64, as
// POLYFIN_SECRET_KEY takes it, and a database URL with its password.
const (
	environmentKey      = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	environmentDatabase = "postgresql://polyfin:database-password@db.example:5432/polyfin"
)

// setEnvironment sets variables in the process's environment until the
// test ends, and returns them in os.Environ's form: what the Polyfin of a
// test is started with, so that a test takes nothing out of the
// environment of whoever runs it.
func setEnvironment(t *testing.T, variables map[string]string) []string {
	t.Helper()
	environ := make([]string, 0, len(variables))
	for name, value := range variables {
		t.Setenv(name, value)
		environ = append(environ, name+"="+value)
	}
	return environ
}

// standIn is a program named name in a folder of its own, a shell script
// that writes its environment to the file seen names, then output, and
// exits with code.
func standIn(t *testing.T, name, output string, code int) (program, seen string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in programs are shell scripts")
	}
	dir := t.TempDir()
	program, seen = filepath.Join(dir, name), filepath.Join(dir, "environment")
	script := "#!/bin/sh\nenv > '" + seen + "'\ncat <<'END'\n" + output + "\nEND\nexit " + strconv.Itoa(code) + "\n"
	if err := process.WriteExecutable(program, strings.NewReader(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return program, seen
}

// environmentOf is the environment the stand-in program wrote to seen,
// one variable an entry.
func environmentOf(t *testing.T, seen string) []string {
	t.Helper()
	data, err := os.ReadFile(seen)
	if err != nil {
		t.Fatalf("the program was not run: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// Once polyfin serve has read its configuration, no program it runs gets
// the variables that hold its credentials: neither its own database's
// programs, the first it runs, nor ffprobe, FFmpeg and Tesseract, which
// read what sources send. They still get the others: PATH, the proxy
// variables FFmpeg reads, those of the hardware, and Polyfin's own that
// hold no credential.
//
//nolint:paralleltest // sets variables of the process's environment, which polyfin serve takes out of it
func TestTheProgramsPolyfinRunsGetNoneOfItsCredentials(t *testing.T) {
	kept := map[string]string{
		"HTTP_PROXY":        "http://proxy.example:3128",
		"no_proxy":          "localhost,127.0.0.1",
		"LIBVA_DRIVER_NAME": "iHD",
		"POLYFIN_FFMPEG":    "/usr/local/bin/ffmpeg",
	}
	environ := setEnvironment(t, kept)
	credentials := map[string]string{
		"POLYFIN_SECRET_KEY":   environmentKey,
		"POLYFIN_DATABASE_URL": environmentDatabase,
		// Not read by Polyfin, and named as the values the admin app hides.
		"POLYFIN_UPDATE_TOKEN": "a-token-of-the-administrator",
	}
	environ = append(environ, setEnvironment(t, credentials)...)
	// Polyfin runs its own database here, whose first program, initdb,
	// fails: polyfin serve stops there, the configuration read, the key
	// from the environment.
	initdb, seenByInitdb := standIn(t, "initdb", "initdb: error: no cluster is made here", 1)
	bin := filepath.Dir(initdb)
	if err := os.WriteFile(filepath.Join(bin, "pg_ctl"), nil, 0o700); err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	getenv := func(name string) string {
		switch name {
		case "POLYFIN_DATABASE":
			return config.EmbeddedDatabase
		case "POLYFIN_DATA_DIR":
			return data
		case "PATH":
			return bin
		case "POLYFIN_SECRET_KEY":
			return os.Getenv(name)
		}
		return ""
	}
	if code, again := runServe(context.Background(), getenv, environ); code != 1 || again {
		t.Fatalf("runServe = %d, %v; want 1, false: initdb fails", code, again)
	}
	seen := map[string]string{"initdb": seenByInitdb}

	ffprobe, seenByFFprobe := standIn(t, "ffprobe", `{"streams": [{"index": 0, "codec_type": "video", "codec_name": "h264"}], "format": {}}`, 0)
	if _, err := (media.Prober{Path: ffprobe, Timeout: time.Minute}).Probe(t.Context(), "http://127.0.0.1:1/movie.mkv"); err != nil {
		t.Fatal(err)
	}
	seen["ffprobe"] = seenByFFprobe

	ffmpeg, seenByFFmpeg := standIn(t, "ffmpeg", " V....D libx264 H.264", 0)
	manager, err := hls.NewManager(ffmpeg, filepath.Join(t.TempDir(), "hls"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	encoders := manager.Encoders()
	manager.Close()
	if !slices.Contains(encoders, "libx264") {
		t.Fatalf("FFmpeg was not asked its encoders: %q", encoders)
	}
	seen["FFmpeg"] = seenByFFmpeg

	tesseract, seenByTesseract := standIn(t, "tesseract", "List of available languages in \"/usr/share/tessdata/\" (1):\neng", 0)
	if languages, err := (&ocr.Tesseract{Path: tesseract}).Languages(t.Context()); err != nil || !slices.Equal(languages, []string{"eng"}) {
		t.Fatalf("Tesseract's languages: %q, %v", languages, err)
	}
	seen["Tesseract"] = seenByTesseract

	for program, file := range seen {
		environment := environmentOf(t, file)
		for _, entry := range environment {
			for name, value := range credentials {
				if strings.HasPrefix(entry, name+"=") || strings.Contains(entry, value) {
					t.Errorf("%s is given %s", program, name)
				}
			}
		}
		for name, value := range kept {
			if !slices.Contains(environment, name+"="+value) {
				t.Errorf("%s is not given %s", program, name)
			}
		}
		if !slices.ContainsFunc(environment, func(entry string) bool { return strings.HasPrefix(entry, "PATH=") }) {
			t.Errorf("%s is not given PATH", program)
		}
	}
}

// polyfin serve takes PGPASSWORD out of its environment too, once it has
// read its configuration (see carryPassword): the programs it runs then
// are not given the database's password.
//
//nolint:paralleltest // sets PGPASSWORD in the process's environment, which polyfin serve takes out of it
func TestTheProgramsPolyfinRunsAreNotGivenPGPASSWORD(t *testing.T) {
	environ := setPassword(t, "database-password")
	// Polyfin is to run its own database, and finds no program for it:
	// polyfin serve stops there, the configuration read.
	settings := map[string]string{"POLYFIN_DATABASE": config.EmbeddedDatabase, "POLYFIN_DATA_DIR": t.TempDir(), "PATH": t.TempDir()}
	if code, again := runServe(t.Context(), func(name string) string { return settings[name] }, environ); code != 1 || again {
		t.Fatalf("runServe = %d, %v; want 1, false: no folder has pg_ctl", code, again)
	}
	ffprobe, seen := standIn(t, "ffprobe", `{"streams": [{"index": 0, "codec_type": "video", "codec_name": "h264"}], "format": {}}`, 0)
	if _, err := (media.Prober{Path: ffprobe, Timeout: time.Minute}).Probe(t.Context(), "http://127.0.0.1:1/movie.mkv"); err != nil {
		t.Fatal(err)
	}
	for _, entry := range environmentOf(t, seen) {
		if strings.HasPrefix(entry, passwordVariable+"=") || strings.Contains(entry, "database-password") {
			t.Error("ffprobe is given PGPASSWORD")
		}
	}
}

// The configuration read before the credentials left the environment
// stays as it was read, and the admin app still tells that the variables
// are set, without their values.
//
//nolint:paralleltest // sets variables of the process's environment, and takes them out of it
func TestPolyfinKeepsTheConfigurationItReadOnceItsCredentialsLeftItsEnvironment(t *testing.T) {
	started := map[string]string{
		"POLYFIN_SECRET_KEY":   environmentKey,
		"POLYFIN_DATABASE_URL": environmentDatabase,
		"POLYFIN_LISTEN":       "127.0.0.1:8097",
	}
	environ := setEnvironment(t, started)
	cfg, err := config.Load(func(name string) string {
		if _, set := started[name]; !set {
			return ""
		}
		return os.Getenv(name)
	})
	if err != nil {
		t.Fatal(err)
	}
	forgetCredentials(environ)
	for _, name := range []string{"POLYFIN_SECRET_KEY", "POLYFIN_DATABASE_URL"} {
		if value, set := os.LookupEnv(name); set {
			t.Errorf("%s is still in the environment (%d bytes)", name, len(value))
		}
	}
	if got := os.Getenv("POLYFIN_LISTEN"); got != "127.0.0.1:8097" {
		t.Errorf("POLYFIN_LISTEN, which holds no credential, is %q", got)
	}
	if cfg.DatabaseURL != environmentDatabase || !bytes.Equal(cfg.SecretKey, []byte("0123456789abcdef0123456789abcdef")) || cfg.Listen != "127.0.0.1:8097" {
		t.Errorf("the configuration changed: database %t, key %t, listen %q",
			cfg.DatabaseURL == environmentDatabase, bytes.Equal(cfg.SecretKey, []byte("0123456789abcdef0123456789abcdef")), cfg.Listen)
	}
	shown := map[string]config.Variable{}
	for _, variable := range config.Variables(environ, cfg) {
		shown[variable.Name] = variable
	}
	for name, want := range map[string]config.Variable{
		"POLYFIN_SECRET_KEY":   {Set: true, Hidden: true, Known: true},
		"POLYFIN_DATABASE_URL": {Value: "db.example:5432/polyfin", Set: true, Known: true},
		"POLYFIN_LISTEN":       {Value: "127.0.0.1:8097", Set: true, Known: true},
	} {
		want.Name = name
		if got := shown[name]; got != want {
			t.Errorf("the admin app is told %+v, want %+v", got, want)
		}
	}
}

// startAgainVariable names, for the processes of
// TestAStartAgainGivesTheNewPolyfinTheEnvironmentItWasStartedWith, the file
// the Polyfin started again writes its environment to.
const startAgainVariable = "POLYFIN_TEST_START_AGAIN"

// After a restore, Polyfin starts again in place: the new Polyfin gets the
// environment the first was started with, the credentials the first took
// out of its own included, and so reads the same configuration.
func TestAStartAgainGivesTheNewPolyfinTheEnvironmentItWasStartedWith(t *testing.T) {
	t.Parallel()
	seen := filepath.Join(t.TempDir(), "environment")
	started := []string{
		"POLYFIN_SECRET_KEY=" + environmentKey,
		"POLYFIN_DATABASE_URL=" + environmentDatabase,
		"POLYFIN_LISTEN=127.0.0.1:8097",
	}
	command := exec.Command(os.Args[0], "-test.run=^TestPolyfinStartsAgain$", "-test.count=1")
	command.Env = append(append(os.Environ(), startAgainVariable+"="+seen), started...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("the Polyfin that starts again: %v\n%s", err, output)
	}
	data, err := os.ReadFile(seen)
	if err != nil {
		t.Fatalf("no Polyfin was started again: %v", err)
	}
	environment := strings.Split(string(data), "\x00")
	for _, entry := range started {
		if !slices.Contains(environment, entry) {
			name, _, _ := strings.Cut(entry, "=")
			t.Errorf("the Polyfin started again is not given %s as the first was", name)
		}
	}
}

// TestPolyfinStartsAgain is the Polyfin of
// TestAStartAgainGivesTheNewPolyfinTheEnvironmentItWasStartedWith: it takes
// its credentials out of its environment as polyfin serve does, then
// starts again as after a restore; started again, it writes its
// environment to the file startAgainVariable names.
func TestPolyfinStartsAgain(t *testing.T) {
	t.Parallel()
	seen := os.Getenv(startAgainVariable)
	if seen == "" {
		t.Skip("run by TestAStartAgainGivesTheNewPolyfinTheEnvironmentItWasStartedWith")
	}
	again := seen + ".again"
	if _, err := os.Stat(again); err == nil {
		if err := os.WriteFile(seen, []byte(strings.Join(os.Environ(), "\x00")), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(again, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	environ := os.Environ()
	forgetCredentials(environ)
	for _, name := range []string{"POLYFIN_SECRET_KEY", "POLYFIN_DATABASE_URL"} {
		if _, set := os.LookupEnv(name); set {
			t.Fatalf("%s is still in the environment of the Polyfin that starts again", name)
		}
	}
	t.Fatal(startAgain(environ))
}

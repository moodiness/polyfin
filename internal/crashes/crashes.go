// Package crashes keeps the trace of a crash of the server for its next
// start. Go prints an unrecovered panic or a fatal error to standard
// error, which a restart under Docker or a service manager loses from the
// admin app: Watch sends it to a file of the data folder too, which the
// next start reads, so that the administrators are told of the crash and
// System › Health shows it.
package crashes

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/logs"
)

const (
	// fileName is the file the runtime writes a crash to, and
	// previousName the one a start moves it to, until its crash is kept.
	fileName     = "crash.log"
	previousName = "crash.previous.log"
	// excerptLines and excerptBytes bound the start of a trace a report
	// keeps.
	excerptLines = 20
	excerptBytes = 4 << 10
)

// Report is a crash a start found: when its trace was written, and the
// first lines of it, redacted.
type Report struct {
	At    time.Time
	Trace string
}

// Watch reads the crash the last run of the server left in dir, if any
// (see Found), and sends the crashes of this run to a new file there.
func Watch(dir string) (*Report, error) {
	report, err := Found(dir)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, fileName), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return report, err
	}
	// The runtime keeps its own copy of the descriptor, which it writes to
	// as the process dies.
	defer file.Close()
	return report, debug.SetCrashOutput(file, debug.CrashOptions{})
}

// Found is the crash the last run of the server left in dir, nil when it
// left none. The crash found stays in dir until Forget, so that a start
// that cannot keep it finds it again; a newer crash replaces it.
func Found(dir string) (*Report, error) {
	current, previous := filepath.Join(dir, fileName), filepath.Join(dir, previousName)
	if info, err := os.Stat(current); err == nil && info.Size() > 0 {
		if err := os.Rename(current, previous); err != nil {
			return nil, err
		}
	}
	report, err := read(previous)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return report, err
}

// Forget deletes the crash Watch found in dir, once it is kept.
func Forget(dir string) error {
	err := os.Remove(filepath.Join(dir, previousName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// read reads the crash of the file at path, nil when it is empty.
func read(path string) (*Report, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	trace := Excerpt(bufio.NewScanner(file))
	if trace == "" {
		return nil, nil
	}
	return &Report{At: info.ModTime(), Trace: trace}, nil
}

// Excerpt is the start of the trace lines reads: the first lines that are
// not blank, at most excerptLines of them and excerptBytes, each redacted
// as the log is, since a panic's message may hold anything.
func Excerpt(lines *bufio.Scanner) string {
	var kept []string
	size := 0
	for len(kept) < excerptLines && lines.Scan() {
		line := strings.TrimRight(lines.Text(), " \t\r")
		if line == "" && len(kept) == 0 {
			continue
		}
		line = logs.Redact(line)
		if size+len(line)+1 > excerptBytes {
			break
		}
		kept = append(kept, line)
		size += len(line) + 1
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

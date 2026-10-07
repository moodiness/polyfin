// Package backup backs Polyfin's database up into a folder with pg_dump,
// while the settings turn backups on: every day at the hour they choose and
// whenever an administrator asks. It keeps the newest backups there.
//
// A backup is pg_dump's custom format, which pg_restore restores into an
// empty database. It is written under a hidden temporary name and renamed
// once complete: a file named like a backup is always a whole one.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moodiness/polyfin/internal/accounts"
)

// StaleAfter is the age past which the last backup made is a problem while
// backups are on: two daily runs missed.
const StaleAfter = 48 * time.Hour

// nameLayout is the time in a backup's name, polyfin-YYYYMMDD-HHMMSS.dump.
const nameLayout = "20060102-150405"

// backupName matches the backups Polyfin makes, the only files it deletes
// to keep the newest; partialName matches a backup being written, which a
// crash may leave.
var (
	backupName  = regexp.MustCompile(`^polyfin-\d{8}-\d{6}\.dump$`)
	partialName = regexp.MustCompile(`^\.polyfin-\d{8}-\d{6}\.dump\.partial$`)
)

// maxErrorBytes bounds what is kept of pg_dump's error output: its last
// lines tell what failed.
const maxErrorBytes = 2000

// DB is where how the last backup went is recorded.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Config is what the service needs.
type Config struct {
	// Folder gives the folder backups are written to while backups are
	// on, empty while they are off, read whenever it is used.
	Folder func() string
	// DatabaseURL is the database backed up, POLYFIN_DATABASE_URL. It
	// holds the password, which is never logged nor written on pg_dump's
	// command line: pg_dump gets it in PGPASSWORD.
	DatabaseURL string
	// PgDump is pg_dump's executable, a path or a name looked up in PATH;
	// empty is pg_dump.
	PgDump string
	// DB records how the last backup went; nil records nothing.
	DB DB
	// Settings gives BackupHour and BackupsKept, read when they are used.
	Settings func() accounts.Settings
	Logger   *slog.Logger
}

// Service makes the backups.
type Service struct {
	cfg Config
	now func() time.Time
}

// New returns the backup service of cfg.
func New(cfg Config) *Service {
	if cfg.PgDump == "" {
		cfg.PgDump = "pg_dump"
	}
	return &Service{cfg: cfg, now: time.Now}
}

// Dir is the folder backups are written to, empty while they are off.
func (s *Service) Dir() string {
	if s == nil || s.cfg.Folder == nil {
		return ""
	}
	return s.cfg.Folder()
}

// Available reports whether backups are on.
func (s *Service) Available() bool {
	return s.Dir() != ""
}

// PgDump is pg_dump's executable.
func (s *Service) PgDump() string {
	return s.cfg.PgDump
}

// Hour is the hour of the server's time zone the database is backed up at
// every day, the settings' BackupHour; -1 while backups are off, which
// leaves the daily task to be run by hand.
func (s *Service) Hour() int {
	if !s.Available() {
		return -1
	}
	return s.cfg.Settings().BackupHour
}

// Status is how backups went.
type Status struct {
	// RanAt is when the last run started, nil before the first; Error is
	// why it failed, empty after a success.
	RanAt *time.Time
	Error string
	// MadeAt is when the last backup made started, nil before the first;
	// File is its name in the folder, Size its bytes.
	MadeAt *time.Time
	File   string
	Size   int64
}

// Problem reports whether an administrator should look at backups, at
// now: the last run failed, or the last backup made is older than
// StaleAfter. Before the first run there is none.
func (st Status) Problem(now time.Time) bool {
	if st.RanAt == nil {
		return false
	}
	return st.Error != "" || st.MadeAt == nil || now.Sub(*st.MadeAt) > StaleAfter
}

// Status reads how the last backup went.
func (s *Service) Status(ctx context.Context) (Status, error) {
	var st Status
	if s == nil || s.cfg.DB == nil {
		return st, nil
	}
	err := s.cfg.DB.QueryRow(ctx, "SELECT ran_at, error, made_at, file, size FROM backup_status").
		Scan(&st.RanAt, &st.Error, &st.MadeAt, &st.File, &st.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, nil
	}
	return st, err
}

// ErrOff fails a backup asked for while backups are off.
var ErrOff = errors.New("backups are off: turn them on under Settings › Backups")

// Run backs the database up into the backup folder, records how it went,
// then deletes the oldest backups there past the settings' BackupsKept. It
// is the scheduled task; while backups are off, it fails with ErrOff.
func (s *Service) Run(ctx context.Context) error {
	// The folder of the run is the one when it starts.
	dir := s.Dir()
	if dir == "" {
		return ErrOff
	}
	started := s.now()
	name, size, err := s.dump(ctx, dir, started)
	if ctx.Err() != nil {
		// A cancelled run leaves what the last one recorded.
		return ctx.Err()
	}
	s.record(ctx, started, name, size, err)
	if err != nil {
		return err
	}
	s.cfg.Logger.Info("The database was backed up", "file", name, "size", size)
	deleted, err := prune(dir, s.cfg.Settings().BackupsKept)
	if len(deleted) > 0 {
		s.cfg.Logger.Info("Deleted old database backups", "files", deleted)
	}
	if err != nil {
		return fmt.Errorf("the backup was made, but older ones could not be deleted: %w", err)
	}
	return nil
}

// dump writes a backup started at started into dir, and returns its name
// and size.
func (s *Service) dump(ctx context.Context, dir string, started time.Time) (string, int64, error) {
	name := "polyfin-" + started.Format(nameLayout) + ".dump"
	final := filepath.Join(dir, name)
	if _, err := os.Lstat(final); !errors.Is(err, fs.ErrNotExist) {
		return "", 0, fmt.Errorf("%s already exists", name)
	}
	removePartials(dir, s.cfg.Logger)
	conninfo, password, err := splitPassword(s.cfg.DatabaseURL)
	if err != nil {
		return "", 0, err
	}
	partial := filepath.Join(dir, "."+name+".partial")
	cmd := exec.CommandContext(ctx, s.cfg.PgDump, "--format=custom", "--no-password", "--file="+partial, "--dbname="+conninfo)
	cmd.Env = environment(os.Environ(), password)
	output := &tail{}
	cmd.Stderr = output
	cmd.WaitDelay = 10 * time.Second
	if err := cmd.Run(); err != nil {
		_ = os.Remove(partial)
		message := strings.TrimSpace(string(output.data))
		if password != "" {
			message = strings.ReplaceAll(message, password, "********")
		}
		if message != "" {
			return "", 0, fmt.Errorf("pg_dump failed (%w): %s", err, message)
		}
		return "", 0, fmt.Errorf("pg_dump failed: %w", err)
	}
	// A backup holds password hashes and the keys of addons and services:
	// only its owner reads it.
	err = os.Chmod(partial, 0o600)
	var info fs.FileInfo
	if err == nil {
		info, err = os.Stat(partial)
	}
	if err == nil {
		err = os.Rename(partial, final)
	}
	if err != nil {
		_ = os.Remove(partial)
		return "", 0, err
	}
	syncDir(dir)
	return name, info.Size(), nil
}

// record saves how the run started at started went: a failure keeps the
// last backup made.
func (s *Service) record(ctx context.Context, started time.Time, name string, size int64, failure error) {
	if s.cfg.DB == nil {
		return
	}
	var err error
	if failure != nil {
		_, err = s.cfg.DB.Exec(ctx, `INSERT INTO backup_status (ran_at, error) VALUES ($1, $2)
			ON CONFLICT (singleton) DO UPDATE SET ran_at = excluded.ran_at, error = excluded.error`, started, failure.Error())
	} else {
		_, err = s.cfg.DB.Exec(ctx, `INSERT INTO backup_status (ran_at, error, made_at, file, size) VALUES ($1, '', $1, $2, $3)
			ON CONFLICT (singleton) DO UPDATE SET ran_at = excluded.ran_at, error = '', made_at = excluded.made_at,
				file = excluded.file, size = excluded.size`, started, name, size)
	}
	if err != nil {
		s.cfg.Logger.Warn("Could not record how the database backup went", "error", err)
	}
}

// environment is pg_dump's environment: the server's, without Polyfin's
// own variables, which pg_dump does not read and which hold the database
// URL, with the password in PGPASSWORD when the URL gives one.
func environment(environ []string, password string) []string {
	result := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if strings.HasPrefix(entry, "POLYFIN_") || (password != "" && strings.HasPrefix(entry, "PGPASSWORD=")) {
			continue
		}
		result = append(result, entry)
	}
	if password != "" {
		result = append(result, "PGPASSWORD="+password)
	}
	return result
}

// prune deletes the oldest of Polyfin's backups in dir past the keep
// newest, and returns the names of those deleted. Other files are left
// alone, even those named almost like a backup.
func prune(dir string, keep int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var backups []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && backupName.MatchString(entry.Name()) {
			backups = append(backups, entry.Name())
		}
	}
	// The names sort as the times they hold.
	slices.Sort(backups)
	var deleted []string
	var errs []error
	for _, name := range backups[:max(len(backups)-keep, 0)] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			errs = append(errs, err)
			continue
		}
		deleted = append(deleted, name)
	}
	return deleted, errors.Join(errs...)
}

// removePartials deletes the backups a crash left half written: a run
// never overlaps another.
func removePartials(dir string, logger *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && partialName.MatchString(entry.Name()) {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				logger.Warn("Could not delete an unfinished database backup", "file", entry.Name(), "error", err)
			}
		}
	}
}

// syncDir makes a rename in dir durable, where the system allows it.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// tail keeps the last maxErrorBytes written to it.
type tail struct{ data []byte }

func (t *tail) Write(p []byte) (int, error) {
	t.data = append(t.data, p...)
	if extra := len(t.data) - maxErrorBytes; extra > 0 {
		t.data = t.data[extra:]
	}
	return len(p), nil
}

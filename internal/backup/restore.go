package backup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/secrets"
)

// A restore puts back the database as a backup holds it, from the folder,
// a destination or a file an administrator sends. It goes in two steps.
// Prepare reads the backup, without changing anything: it must be one of
// Polyfin's, of a schema this Polyfin knows, and the stored secrets it
// holds are counted that the current key cannot decrypt. Start then backs
// the current database up, as a backup named before-restore, and asks
// Polyfin to stop: once it has, with every playback, conversion and
// recording stopped, Restore replaces the database's content with the
// backup's in one transaction, which a failure rolls back, migrates it,
// and Polyfin starts again on it.
//
// The restore runs in the database Polyfin uses, rather than in a new
// database swapped in for it: that needs no other privilege than owning
// Polyfin's tables, which every setup grants, where creating and renaming
// databases needs CREATEDB and owning the database, which a database
// created for Polyfin by an administrator of PostgreSQL may not grant.

// restorableName matches the backups that can be restored: those Polyfin
// makes every day, with the time they were made, and the before-restore
// ones, plain or encrypted. restoreTemporary matches a backup downloaded,
// sent or decrypted to be restored.
var (
	restorableName   = regexp.MustCompile(`^polyfin-(\d{8}-\d{6})(-before-restore)?\.dump(\.enc)?$`)
	restoreTemporary = regexp.MustCompile(`^\.polyfin-restore-[0-9a-f]{16}\.dump$`)
)

// beforeRestoreSuffix ends the name of the backup made before a restore.
const beforeRestoreSuffix = "-before-restore"

// The errors of a restore.
var (
	// ErrNoPgRestore reports a server without pg_restore.
	ErrNoPgRestore = errors.New("pg_restore was not found: install PostgreSQL 18's client tools")
	// ErrBackupNotFound reports a backup the folder or the destination
	// does not have.
	ErrBackupNotFound = errors.New("no such backup")
	// ErrNotABackup reports a file that is not a backup of Polyfin's
	// database.
	ErrNotABackup = errors.New("the file is not a backup of Polyfin's database")
	// ErrNewerBackup reports a backup of a newer Polyfin, and
	// ErrChangedBackup one whose migrations differ from this Polyfin's.
	ErrNewerBackup   = errors.New("the backup was made by a newer Polyfin: update Polyfin first")
	ErrChangedBackup = errors.New("the backup was made by a Polyfin whose database migrations differ from this one's")
	// ErrNoPlan reports a restore started without a backup prepared, or
	// with another than the last one prepared.
	ErrNoPlan = errors.New("the backup to restore is no longer prepared: choose it again")
	// ErrUnreadableSecrets reports a restore of a backup holding secrets
	// the current key cannot decrypt, not confirmed.
	ErrUnreadableSecrets = errors.New("the backup holds keys that POLYFIN_SECRET_KEY cannot decrypt")
	// ErrRestoring reports a restore asked for while one runs.
	ErrRestoring = errors.New("a restore is under way")
	// ErrPassphraseRequired reports a restore of an encrypted backup not
	// opened with its passphrase yet (see Unlock).
	ErrPassphraseRequired = errors.New("the backup is encrypted: its passphrase is needed to restore it")
)

// File is a backup that can be restored: its name, size, and when it was
// made, from its name.
type File struct {
	Name   string
	Size   int64
	MadeAt time.Time
	// BeforeRestore tells a backup made before a restore, Encrypted one
	// encrypted with a passphrase, from its name.
	BeforeRestore bool
	Encrypted     bool
}

// Listing lists the backups of the folder and of each destination, newest
// first. A destination that cannot be read has its error instead.
type Listing struct {
	Folder       []File
	Destinations []DestinationFiles
}

// DestinationFiles are the backups of a destination.
type DestinationFiles struct {
	Destination Destination
	Files       []File
	Error       error
}

// Backups lists the backups that can be restored.
func (s *Service) Backups(ctx context.Context) (Listing, error) {
	dir := s.Dir()
	if dir == "" {
		return Listing{}, ErrOff
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Listing{}, err
	}
	listing := Listing{Folder: []File{}, Destinations: []DestinationFiles{}}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if file, ok := restorable(entry.Name(), info.Size()); ok {
			listing.Folder = append(listing.Folder, file)
		}
	}
	sortFiles(listing.Folder)
	destinations, err := s.loadDestinations(ctx)
	if err != nil {
		return Listing{}, err
	}
	for _, d := range destinations {
		files, err := s.remoteBackups(ctx, d)
		listing.Destinations = append(listing.Destinations, DestinationFiles{Destination: s.describe(d), Files: files, Error: err})
	}
	return listing, nil
}

func (s *Service) remoteBackups(ctx context.Context, d stored) ([]File, error) {
	r, err := s.open(ctx, d)
	if err != nil {
		return []File{}, err
	}
	defer r.close()
	listed, err := r.list(ctx)
	if err != nil {
		return []File{}, err
	}
	files := []File{}
	for _, f := range listed {
		if file, ok := restorable(f.name, f.size); ok {
			files = append(files, file)
		}
	}
	sortFiles(files)
	return files, nil
}

func restorable(name string, size int64) (File, bool) {
	made, ok := madeAt(name)
	if !ok {
		return File{}, false
	}
	return File{Name: name, Size: size, MadeAt: made, BeforeRestore: strings.Contains(name, beforeRestoreSuffix+".dump"),
		Encrypted: encryptedName(name)}, true
}

// sortFiles puts the newest first.
func sortFiles(files []File) {
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(b.Name, a.Name) })
}

// Plan is a backup prepared to be restored.
type Plan struct {
	// ID tells it from the ones prepared before.
	ID string
	// File is the backup's name, or the name of the file sent.
	File string
	// Destination is the destination it comes from, nil for the folder or
	// a file sent; Uploaded tells a file sent.
	Destination *Destination
	Uploaded    bool
	// MadeAt is when it was made, nil for a file sent, whose name may say
	// anything.
	MadeAt *time.Time
	Size   int64
	// Unreadable counts, by kind (see the secrets package's Kind
	// constants), the stored secrets the current key cannot decrypt.
	Unreadable map[string]int
	// Encrypted tells an encrypted backup. Locked tells one not opened
	// with its passphrase yet: it is neither checked nor restored before
	// Unlock opens it, and Unreadable is empty until then.
	Encrypted bool
	Locked    bool
	// path is the file restored, temporary when it was downloaded, sent or
	// decrypted, in dir, the folder of the restore.
	path      string
	temporary bool
	dir       string
	// setup tells a restore from the setup page, of a database without an
	// administrator, which has nothing to back up first.
	setup bool
	// expires is when it is dropped unless its restore was started.
	expires time.Time
	// By is the administrator who started the restore, empty from the
	// setup page; BeforeRestore the backup made of the current database
	// first.
	By            string
	BeforeRestore string
}

// UnreadableCount is how many stored secrets the current key cannot
// decrypt.
func (p Plan) UnreadableCount() int {
	total := 0
	for _, count := range p.Unreadable {
		total += count
	}
	return total
}

// PgRestore is pg_restore's executable.
func (s *Service) PgRestore() string {
	return s.cfg.PgRestore
}

// CanRestore tells why a backup cannot be restored: backups are off, or
// pg_restore is missing; nil when it can.
func (s *Service) CanRestore() error {
	if !s.Available() {
		return ErrOff
	}
	if _, err := exec.LookPath(s.cfg.PgRestore); err != nil {
		return ErrNoPgRestore
	}
	return nil
}

// Prepare reads the backup name of the folder, or of the destination id
// names when not nil, to restore it (see Plan). A backup of a
// destination is downloaded into the folder first.
func (s *Service) Prepare(ctx context.Context, destination *accounts.ID, name string) (Plan, error) {
	dir := s.Dir()
	if dir == "" {
		return Plan{}, ErrOff
	}
	if !restorableName.MatchString(name) {
		return Plan{}, ErrBackupNotFound
	}
	plan := Plan{File: name, dir: dir}
	if made, ok := madeAt(name); ok {
		plan.MadeAt = &made
	}
	if destination == nil {
		plan.path = filepath.Join(dir, name)
		info, err := os.Lstat(plan.path)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
			return Plan{}, ErrBackupNotFound
		}
		if err != nil {
			return Plan{}, err
		}
		if err := s.CanRestore(); err != nil {
			return Plan{}, err
		}
		plan.Size = info.Size()
		return s.prepare(ctx, plan)
	}
	if err := s.CanRestore(); err != nil {
		return Plan{}, err
	}
	if err := s.checkNotRestoring(); err != nil {
		return Plan{}, err
	}
	d, err := s.loadDestination(ctx, *destination)
	if err != nil {
		return Plan{}, err
	}
	described := s.describe(d)
	plan.Destination = &described
	r, err := s.open(ctx, d)
	if err != nil {
		return Plan{}, err
	}
	defer r.close()
	// The download is the size the destination lists, never more.
	files, err := r.list(ctx)
	if err != nil {
		return Plan{}, err
	}
	index := slices.IndexFunc(files, func(f remoteFile) bool { return f.name == name })
	if index < 0 {
		return Plan{}, ErrBackupNotFound
	}
	// A size the destination does not tell is bounded as a file sent is.
	listed, limit := files[index].size, files[index].size
	if listed == unknownSize {
		limit = maxDownload
	}
	path := temporaryPath(dir)
	size, err := copyTo(path, func(w io.Writer) error { return r.get(ctx, name, &cappedWriter{w: w, max: limit}) })
	if errors.Is(err, errNotFound) {
		return Plan{}, ErrBackupNotFound
	}
	if err == nil && listed != unknownSize && size != listed {
		_ = os.Remove(path)
		err = fmt.Errorf("the destination sent %d bytes of the %d it lists", size, listed)
	}
	if err != nil {
		return Plan{}, fmt.Errorf("download %s: %w", name, err)
	}
	plan.path, plan.temporary, plan.Size = path, true, size
	return s.prepare(ctx, plan)
}

// PrepareUpload reads a backup an administrator sends, named name, from
// body, to restore it (see Plan). It is written into the folder first.
func (s *Service) PrepareUpload(ctx context.Context, name string, body io.Reader) (Plan, error) {
	if err := s.CanRestore(); err != nil {
		return Plan{}, err
	}
	return s.prepareUpload(ctx, s.Dir(), name, body, false)
}

// PrepareSetupUpload reads a backup sent from the setup page, before any
// administrator exists, named name, from body, to restore it (see Plan).
// Backups are off then, as a new server has them: it is written into
// SetupFolder, unless they are on. Its restore backs nothing up first:
// a database without an administrator holds nothing set up yet.
func (s *Service) PrepareSetupUpload(ctx context.Context, name string, body io.Reader) (Plan, error) {
	if _, err := exec.LookPath(s.cfg.PgRestore); err != nil {
		return Plan{}, ErrNoPgRestore
	}
	dir := s.Dir()
	if dir == "" {
		dir = s.cfg.SetupFolder
		if dir == "" {
			return Plan{}, ErrOff
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Plan{}, err
		}
	}
	return s.prepareUpload(ctx, dir, name, body, true)
}

func (s *Service) prepareUpload(ctx context.Context, dir, name string, body io.Reader, setup bool) (Plan, error) {
	if err := s.checkNotRestoring(); err != nil {
		return Plan{}, err
	}
	path := temporaryPath(dir)
	size, err := copyTo(path, func(w io.Writer) error {
		_, err := io.Copy(w, body)
		return err
	})
	if err != nil {
		return Plan{}, err
	}
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" || len(name) > 255 || strings.ContainsFunc(name, func(r rune) bool { return r < ' ' }) {
		name = "upload.dump"
	}
	return s.prepare(ctx, Plan{File: name, Uploaded: true, path: path, temporary: true, dir: dir, setup: setup, Size: size})
}

// temporaryPath is the path of a new backup to restore in dir, a name of
// its own: the files of other plans, or of a restore under way, are never
// touched.
func temporaryPath(dir string) string {
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	return filepath.Join(dir, ".polyfin-restore-"+hex.EncodeToString(random)+".dump")
}

// prepare inspects plan's file and keeps it as the backup to restore. An
// encrypted backup is kept locked, to be opened by Unlock.
func (s *Service) prepare(ctx context.Context, plan Plan) (Plan, error) {
	encrypted, err := isEncrypted(plan.path)
	if err == nil && encrypted {
		plan.Encrypted, plan.Locked, plan.Unreadable = true, true, map[string]int{}
	} else if err == nil {
		plan.Unreadable, err = s.inspect(ctx, plan.path)
	}
	if err != nil {
		if plan.temporary {
			_ = os.Remove(plan.path)
		}
		return Plan{}, err
	}
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	plan.ID = hex.EncodeToString(random)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restoring {
		if plan.temporary {
			_ = os.Remove(plan.path)
		}
		return Plan{}, ErrRestoring
	}
	if s.plan != nil && s.plan.temporary && s.plan.path != plan.path {
		_ = os.Remove(s.plan.path)
	}
	plan.expires = time.Now().Add(s.planLifetime)
	s.plan = &plan
	// A plan never started is dropped, with its file, after a while; one
	// whose start fails past that time, by Start.
	time.AfterFunc(s.planLifetime, func() { s.Discard(plan.ID) })
	return plan, nil
}

// Unlock opens the encrypted backup prepared as id with passphrase: it is
// decrypted into a new file of the restore's folder, readable only by its
// owner, every chunk checked, then checked as Prepare checks a backup, and
// the plan, unlocked, is restored from that file. A wrong passphrase
// fails with ErrWrongPassphrase and leaves the backup prepared, to try
// another; a damaged backup, or one this Polyfin cannot restore, is
// dropped with its error.
func (s *Service) Unlock(ctx context.Context, id, passphrase string) (Plan, error) {
	// One backup is opened at a time: scrypt takes 128 MiB.
	s.unlocks.Lock()
	defer s.unlocks.Unlock()
	s.mu.Lock()
	switch {
	case s.restoring:
		s.mu.Unlock()
		return Plan{}, ErrRestoring
	case s.plan == nil || s.plan.ID != id:
		s.mu.Unlock()
		return Plan{}, ErrNoPlan
	case !s.plan.Locked:
		plan := *s.plan
		s.mu.Unlock()
		return plan, nil
	}
	plan := *s.plan
	s.mu.Unlock()
	path := temporaryPath(plan.dir)
	err := decryptFile(ctx, plan.path, path, passphrase)
	var unreadable map[string]int
	if err == nil {
		unreadable, err = s.inspect(ctx, path)
		if err != nil {
			_ = os.Remove(path)
		}
	}
	if err != nil {
		if !errors.Is(err, ErrWrongPassphrase) && ctx.Err() == nil {
			s.Discard(id)
		}
		return Plan{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restoring || s.plan == nil || s.plan.ID != id {
		_ = os.Remove(path)
		return Plan{}, ErrNoPlan
	}
	if s.plan.temporary {
		_ = os.Remove(s.plan.path)
	}
	s.plan.path, s.plan.temporary, s.plan.Locked, s.plan.Unreadable = path, true, false, unreadable
	return *s.plan, nil
}

// decryptFile writes the dump of the encrypted backup at path, opened with
// passphrase, to a new file at to, readable only by its owner; a failure
// leaves no file.
func decryptFile(ctx context.Context, path, to, passphrase string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dump, err := newDecrypter(f, passphrase)
	if err != nil {
		return err
	}
	_, err = copyTo(to, func(w io.Writer) error {
		_, err := io.Copy(w, contextReader{ctx: ctx, r: dump})
		return err
	})
	return err
}

// contextReader reads r until ctx ends.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// PlanLifetime is how long a backup prepared to be restored stays so: its
// file, downloaded or sent, is deleted after it.
const PlanLifetime = time.Hour

// Discard drops the backup prepared as id, and deletes its file when it
// was downloaded or sent, unless its restore was started. It reports
// whether it dropped it.
func (s *Service) Discard(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restoring || s.plan == nil || s.plan.ID != id {
		return false
	}
	if s.plan.temporary {
		_ = os.Remove(s.plan.path)
	}
	s.plan = nil
	return true
}

// checkNotRestoring fails with ErrRestoring while a restore is under way:
// nothing is downloaded nor written then.
func (s *Service) checkNotRestoring() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restoring {
		return ErrRestoring
	}
	return nil
}

// RemoveRestoreLeftovers deletes from the folder, and from SetupFolder,
// the backups downloaded, sent or decrypted to be restored that a stop
// left behind. Polyfin calls it at startup, before any is prepared.
func (s *Service) RemoveRestoreLeftovers() {
	dirs := []string{s.Dir()}
	if s.cfg.SetupFolder != dirs[0] {
		dirs = append(dirs, s.cfg.SetupFolder)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if !entry.Type().IsRegular() || !restoreTemporary.MatchString(entry.Name()) || (s.plan != nil && s.plan.path == path) {
				continue
			}
			if err := os.Remove(path); err != nil {
				s.cfg.Logger.Warn("Could not delete a backup left from a restore", "file", entry.Name(), "error", err)
			} else {
				s.cfg.Logger.Info("Deleted a backup left from a restore", "file", entry.Name())
			}
		}
	}
}

// inspect checks that the file at path is a backup of Polyfin's database
// that this Polyfin can migrate, and counts by kind the stored secrets in
// it that the current key cannot decrypt.
func (s *Service) inspect(ctx context.Context, path string) (map[string]int, error) {
	contents, err := s.pgRestore(ctx, "--list", path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotABackup, err)
	}
	var migrations, settings bool
	for line := range strings.Lines(string(contents)) {
		fields := strings.Fields(line)
		// An entry: its number, then two other numbers, its type, schema,
		// name and owner.
		if len(fields) < 6 || !strings.HasSuffix(fields[0], ";") {
			continue
		}
		switch strings.Join(fields[3:6], " ") {
		case "TABLE DATA public":
			if len(fields) > 6 && fields[6] == "schema_migrations" {
				migrations = true
			}
		case "TABLE public settings":
			settings = true
		}
	}
	if !migrations || !settings {
		return nil, ErrNotABackup
	}
	args := []string{"--data-only", "--file=-", "--table=schema_migrations"}
	for _, table := range secrets.SealedTables() {
		args = append(args, "--table="+table)
	}
	data, err := s.pgRestore(ctx, append(args, path)...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotABackup, err)
	}
	applied := map[int]string{}
	unreadable := map[string]int{}
	err = newScript(bytes.NewReader(data)).eachRow(func(table string, row map[string]string) error {
		if table == "schema_migrations" {
			version, err := strconv.Atoi(row["version"])
			if err != nil {
				return ErrNotABackup
			}
			applied[version] = row["checksum"]
			return nil
		}
		if kind, count := s.cfg.Secrets.UnreadableIn(table, row); count > 0 {
			unreadable[kind] += count
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotABackup, err)
	}
	if len(applied) == 0 {
		return nil, ErrNotABackup
	}
	switch err := database.CheckApplied(applied); {
	case errors.Is(err, database.ErrNewerSchema):
		return nil, ErrNewerBackup
	case errors.Is(err, database.ErrChangedMigration):
		return nil, fmt.Errorf("%w: %w", ErrChangedBackup, err)
	case err != nil:
		return nil, err
	}
	return unreadable, nil
}

// pgRestore runs pg_restore with args, which reads a file and connects to
// nothing, and returns what it writes.
func (s *Service) pgRestore(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, s.cfg.PgRestore, args...)
	cmd.Env = environment(os.Environ(), "")
	output := &tail{}
	cmd.Stderr = output
	cmd.WaitDelay = 10 * time.Second
	data, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(string(output.data)); message != "" {
			return nil, fmt.Errorf("pg_restore failed (%w): %s", err, message)
		}
		return nil, fmt.Errorf("pg_restore failed: %w", err)
	}
	return data, nil
}

// Start restores the backup prepared as id, by administrator: anyway goes
// ahead when it holds secrets the current key cannot decrypt. It backs
// the current database up first, then asks Polyfin to stop, which it does
// once Requested gives the plan. An encrypted backup must have been
// unlocked.
func (s *Service) Start(ctx context.Context, id string, anyway bool, administrator string) error {
	return s.start(ctx, id, anyway, administrator, false)
}

// StartSetup restores the backup prepared from the setup page as id (see
// PrepareSetupUpload), as Start does, without a backup first.
func (s *Service) StartSetup(ctx context.Context, id string, anyway bool) error {
	return s.start(ctx, id, anyway, "", true)
}

func (s *Service) start(ctx context.Context, id string, anyway bool, administrator string, setup bool) error {
	s.mu.Lock()
	plan := s.plan
	switch {
	case s.restoring:
		s.mu.Unlock()
		return ErrRestoring
	case plan == nil || plan.ID != id || plan.setup != setup:
		s.mu.Unlock()
		return ErrNoPlan
	case plan.Locked:
		s.mu.Unlock()
		return ErrPassphraseRequired
	case plan.UnreadableCount() > 0 && !anyway:
		s.mu.Unlock()
		return ErrUnreadableSecrets
	}
	s.restoring = true
	s.mu.Unlock()
	name := ""
	if !plan.setup {
		var err error
		if name, err = s.backUpBeforeRestore(ctx); err != nil {
			s.mu.Lock()
			s.restoring = false
			// Its time passed while the backup was made: the timer could
			// not drop it then.
			if s.plan != nil && s.plan.ID == id && !time.Now().Before(s.plan.expires) {
				if s.plan.temporary {
					_ = os.Remove(s.plan.path)
				}
				s.plan = nil
			}
			s.mu.Unlock()
			return fmt.Errorf("the current database could not be backed up first, so nothing was restored: %w", err)
		}
	}
	restore := *plan
	restore.By, restore.BeforeRestore = administrator, name
	s.cfg.Logger.Info("Restoring a database backup: Polyfin stops, then starts again on it", "file", restore.File,
		"administrator", administrator, "before_restore", name, "setup", restore.setup)
	s.requests <- restore
	return nil
}

// Requested gives the restore Start asks for: Polyfin then stops, and
// calls Restore.
func (s *Service) Requested() <-chan Plan {
	return s.requests
}

// backUpBeforeRestore backs the current database up into the folder, as
// a backup named before-restore that retention never deletes, and returns
// its name.
func (s *Service) backUpBeforeRestore(ctx context.Context) (string, error) {
	s.dumps.Lock()
	defer s.dumps.Unlock()
	dir := s.Dir()
	if dir == "" {
		return "", ErrOff
	}
	name, _, err := s.dump(ctx, dir, s.now(), beforeRestoreSuffix)
	return name, err
}

// Restore replaces the content of the database with plan's backup, in one
// transaction, then migrates it, and records the restore there; a failure
// leaves the database as it was, and is recorded in it. Polyfin must have
// stopped: nothing else may use the database meanwhile.
func (s *Service) Restore(ctx context.Context, plan Plan) error {
	if plan.temporary {
		defer os.Remove(plan.path)
	}
	// Another backup may be prepared and restored afterwards.
	defer func() {
		s.mu.Lock()
		s.restoring, s.plan = false, nil
		s.mu.Unlock()
	}()
	started := s.now()
	err := s.restore(ctx, plan)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		s.cfg.Logger.Error("The backup could not be restored: the database is as it was", "file", plan.File, "error", err)
	} else {
		s.cfg.Logger.Info("The backup was restored", "file", plan.File, "administrator", plan.By)
	}
	pool, poolErr := pgxpool.New(ctx, s.cfg.DatabaseURL)
	if poolErr != nil {
		return errors.Join(err, poolErr)
	}
	defer pool.Close()
	if err == nil {
		// The backup's schema may be older: it is brought to this
		// Polyfin's before anything is written to it.
		if migrateErr := database.Migrate(ctx, pool); migrateErr != nil {
			return fmt.Errorf("the backup was restored, but could not be migrated: %w", migrateErr)
		}
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	if _, recordErr := pool.Exec(ctx, `INSERT INTO backup_restore (restored_at, file, administrator, error) VALUES ($1, $2, $3, $4)
		ON CONFLICT (singleton) DO UPDATE SET restored_at = excluded.restored_at, file = excluded.file,
			administrator = excluded.administrator, error = excluded.error`, started, plan.File, plan.By, message); recordErr != nil {
		s.cfg.Logger.Warn("Could not record how the restore went", "error", recordErr)
	}
	if err == nil {
		var language string
		if pool.QueryRow(ctx, "SELECT language FROM settings").Scan(&language) != nil {
			language = "en"
		}
		log := activity.New(pool, func() accounts.Settings { return accounts.Settings{Language: language} }, s.cfg.Logger)
		if plan.setup {
			log.BackupRestoredAtSetup(ctx, plan.File)
		} else {
			log.BackupRestored(ctx, plan.By, plan.File)
		}
		s.recordBeforeRestore(ctx, pool, plan.BeforeRestore)
	}
	return err
}

// recordBeforeRestore records, in the database restored, the backup made
// before the restore as the last backup made: the backup's own record
// predates it, and would tell of none, or of an older one.
func (s *Service) recordBeforeRestore(ctx context.Context, db DB, name string) {
	made, ok := madeAt(name)
	if !ok {
		return
	}
	info, err := os.Stat(filepath.Join(s.Dir(), name))
	if err != nil {
		return
	}
	if _, err := db.Exec(ctx, `INSERT INTO backup_status (ran_at, error, made_at, file, size) VALUES ($1, '', $1, $2, $3)
		ON CONFLICT (singleton) DO UPDATE SET ran_at = excluded.ran_at, error = '', made_at = excluded.made_at,
			file = excluded.file, size = excluded.size`, made, name, info.Size()); err != nil {
		s.cfg.Logger.Warn("Could not record the backup made before the restore", "error", err)
	}
}

// restore runs pg_restore's script of plan's backup in one transaction,
// after dropping what Polyfin's database holds.
func (s *Service) restore(ctx context.Context, plan Plan) error {
	conn, err := pgx.Connect(ctx, s.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	cmd := exec.CommandContext(ctx, s.cfg.PgRestore, "--no-owner", "--no-privileges", "--no-publications", "--no-subscriptions",
		"--no-security-labels", "--no-tablespaces", "--file=-", plan.path)
	cmd.Env = environment(os.Environ(), "")
	output := &tail{}
	cmd.Stderr = output
	cmd.WaitDelay = 10 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	failed := func(err error) error {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if message := strings.TrimSpace(string(output.data)); message != "" {
			return fmt.Errorf("%w (pg_restore: %s)", err, message)
		}
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return failed(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if err := dropAll(ctx, tx); err != nil {
		return failed(fmt.Errorf("empty the database: %w", err))
	}
	if err := newScript(bufio.NewReader(stdout)).run(ctx, conn.PgConn()); err != nil {
		return failed(err)
	}
	if err := cmd.Wait(); err != nil {
		if message := strings.TrimSpace(string(output.data)); message != "" {
			return fmt.Errorf("pg_restore failed (%w): %s", err, message)
		}
		return fmt.Errorf("pg_restore failed: %w", err)
	}
	return tx.Commit(ctx)
}

// dropAll drops what the database user owns in the database: its schemas
// but public, and in public, its tables, views, sequences, functions and
// types. What extensions or other users own is left alone.
func dropAll(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT format('DROP SCHEMA %I CASCADE', n.nspname)
		FROM pg_namespace n
		WHERE n.nspowner = (SELECT oid FROM pg_roles WHERE rolname = current_user) AND n.nspname <> 'public'
			AND n.nspname NOT LIKE 'pg\_%' AND n.nspname <> 'information_schema'
			AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid = 'pg_namespace'::regclass AND d.objid = n.oid AND d.deptype = 'e')
		UNION ALL
		SELECT format('DROP %s IF EXISTS public.%I CASCADE', CASE c.relkind WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW'
			WHEN 'S' THEN 'SEQUENCE' WHEN 'f' THEN 'FOREIGN TABLE' ELSE 'TABLE' END, c.relname)
		FROM pg_class c
		WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
			AND c.relowner = (SELECT oid FROM pg_roles WHERE rolname = current_user)
			AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype = 'e')
			-- A column's sequence goes with its table.
			AND NOT EXISTS (SELECT FROM pg_depend d WHERE c.relkind = 'S' AND d.classid = 'pg_class'::regclass AND d.objid = c.oid
				AND d.refclassid = 'pg_class'::regclass AND d.deptype IN ('a', 'i'))
		UNION ALL
		SELECT format('DROP %s IF EXISTS %s CASCADE', CASE p.prokind WHEN 'a' THEN 'AGGREGATE' WHEN 'p' THEN 'PROCEDURE' ELSE 'FUNCTION' END,
			p.oid::regprocedure)
		FROM pg_proc p
		WHERE p.pronamespace = 'public'::regnamespace AND p.proowner = (SELECT oid FROM pg_roles WHERE rolname = current_user)
			AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e')
		UNION ALL
		SELECT format('DROP TYPE IF EXISTS %s CASCADE', t.oid::regtype)
		FROM pg_type t
		WHERE t.typnamespace = 'public'::regnamespace AND t.typowner = (SELECT oid FROM pg_roles WHERE rolname = current_user)
			AND (t.typtype IN ('e', 'd', 'r') OR (t.typtype = 'c' AND (SELECT relkind FROM pg_class WHERE oid = t.typrelid) = 'c'))
			AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid = 'pg_type'::regclass AND d.objid = t.oid AND d.deptype = 'e')`)
	if err != nil {
		return err
	}
	statements, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// Restore is how the last restore went.
type Restore struct {
	At            time.Time
	File          string
	Administrator string
	Error         string
}

// LastRestore reads how the last restore went, nil before the first.
func (s *Service) LastRestore(ctx context.Context) (*Restore, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, nil
	}
	var r Restore
	err := s.cfg.DB.QueryRow(ctx, "SELECT restored_at, file, administrator, error FROM backup_restore").
		Scan(&r.At, &r.File, &r.Administrator, &r.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

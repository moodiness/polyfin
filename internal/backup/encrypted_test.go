package backup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// fakeDump is what fakePgDump writes.
const fakeDump = "PGDMP partial complete"

// While a passphrase is set, every backup is encrypted as it is written:
// in the folder and at each destination, under its own name, with nothing
// of the dump in the clear. Retention counts the backups made before,
// unencrypted, with the others, in the folder and at each destination.
// One downloaded from any destination, or of the folder, is prepared
// locked: it restores only once its passphrase opened it, a wrong one
// leaving it prepared; a damaged one is dropped with its files.
func TestEncryptedBackupsAreSentAndOpened(t *testing.T) {
	h := withDestinations(t)
	h.kept = 3
	h.service.cfg.PgRestore = fakePgRestore(t)
	store := newTestS3(t, "backups", "AKIDEXAMPLE")
	sftpServer := newTestSFTP(t, "polyfin", "sftp password", newHostKey(t), t.TempDir())
	dav := newTestWebDAV(t, "polyfin", "dav password")
	two := 2
	destinations := []Destination{
		h.create(t, Draft{Kind: KindS3, Address: new(store.url), Bucket: new("backups"), Username: new("AKIDEXAMPLE"), Secret: new("s3 secret"),
			Kept: &two}),
		h.create(t, Draft{Kind: KindSFTP, Address: new(sftpServer.host), Port: new(sftpServer.port), Folder: new(sftpServer.dir),
			Username: new("polyfin"), Secret: new("sftp password"), Kept: &two}),
		h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"), Secret: new("dav password"), Kept: &two}),
	}
	dirs := []string{store.dir, sftpServer.dir, dav.dir}

	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local)
	h.runAt(t, at)
	h.passphrase = testPassphrase
	for day := 1; day < 4; day++ {
		h.runAt(t, at.AddDate(0, 0, day))
	}
	if args, _ := os.ReadFile(filepath.Join(h.record, "args")); strings.Contains(string(args), "--file") {
		t.Errorf("pg_dump wrote an encrypted backup's dump to a file:\n%s", args)
	}
	newest := "polyfin-20261004-040000.dump.enc"
	if got, want := fileNames(t, h.dir), []string{"polyfin-20261002-040000.dump.enc", "polyfin-20261003-040000.dump.enc", newest}; !slices.Equal(got, want) {
		t.Errorf("the folder:\n%q\nwant\n%q", got, want)
	}
	for i, dir := range dirs {
		if got, want := fileNames(t, dir), []string{"polyfin-20261003-040000.dump.enc", newest}; !slices.Equal(got, want) {
			t.Errorf("%s:\n%q\nwant\n%q", destinations[i].Kind, got, want)
		}
		sealed, err := os.ReadFile(filepath.Join(dir, newest))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(sealed, []byte("PGDMP")) {
			t.Errorf("%s: the dump is in the clear", destinations[i].Kind)
		}
		if dump, err := decryptBytes(sealed, testPassphrase); err != nil || string(dump) != fakeDump {
			t.Errorf("%s: %q %v", destinations[i].Kind, dump, err)
		}
	}
	listing, err := h.service.Backups(t.Context())
	if err != nil || len(listing.Folder) != 3 || !listing.Folder[0].Encrypted || listing.Folder[0].Name != newest {
		t.Errorf("listing: %+v %v", listing.Folder, err)
	}

	for _, d := range destinations {
		plan, err := h.service.Prepare(t.Context(), &d.ID, newest)
		if err != nil || !plan.Locked || !plan.Encrypted {
			t.Fatalf("%s: prepared %+v %v", d.Kind, plan, err)
		}
		if err := h.service.Start(t.Context(), plan.ID, true, "root"); !errors.Is(err, ErrPassphraseRequired) {
			t.Errorf("%s: started locked: %v", d.Kind, err)
		}
		if _, err := h.service.Unlock(t.Context(), plan.ID, "not the passphrase"); !errors.Is(err, ErrWrongPassphrase) {
			t.Errorf("%s: a wrong passphrase: %v", d.Kind, err)
		}
		unlocked, err := h.service.Unlock(t.Context(), plan.ID, testPassphrase)
		if err != nil || unlocked.Locked || unlocked.ID != plan.ID || unlocked.Unreadable == nil {
			t.Fatalf("%s: unlocked %+v %v", d.Kind, unlocked, err)
		}
		// The download is replaced with the dump it holds.
		files := restoreFiles(t, h.dir)
		if len(files) != 1 {
			t.Fatalf("%s: restore files %q", d.Kind, files)
		}
		if dump, _ := os.ReadFile(filepath.Join(h.dir, files[0])); string(dump) != fakeDump {
			t.Errorf("%s: the file restored is %q", d.Kind, dump)
		}
	}

	// A backup of the folder stays where it is once opened.
	plan, err := h.service.Prepare(t.Context(), nil, newest)
	if err != nil || !plan.Locked {
		t.Fatalf("the folder's: %+v %v", plan, err)
	}
	if _, err := h.service.Unlock(t.Context(), plan.ID, testPassphrase); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h.dir, newest)); err != nil {
		t.Errorf("the folder's backup after it was opened: %v", err)
	}

	// A chunk changed at a destination: nothing is restored, and no file
	// of it stays.
	damaged := filepath.Join(dav.dir, newest)
	sealed, err := os.ReadFile(damaged)
	if err != nil {
		t.Fatal(err)
	}
	sealed[headerSize+3] ^= 1
	if err := os.WriteFile(damaged, sealed, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err = h.service.Prepare(t.Context(), &destinations[2].ID, newest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Unlock(t.Context(), plan.ID, testPassphrase); !errors.Is(err, ErrDamagedBackup) {
		t.Errorf("a damaged backup: %v", err)
	}
	if err := h.service.Start(t.Context(), plan.ID, true, "root"); !errors.Is(err, ErrNoPlan) {
		t.Errorf("a damaged backup started: %v", err)
	}
	if files := restoreFiles(t, h.dir); len(files) != 0 {
		t.Errorf("the files of a damaged backup stay: %q", files)
	}
}

// A passphrase the current key cannot decrypt reads as none: backups then
// fail rather than be written unencrypted.
func TestAnUnreadablePassphraseFailsTheBackup(t *testing.T) {
	h := newHarness(t, nil)
	h.service.cfg.PassphraseUnreadable = func() bool { return true }
	if err := h.service.Run(t.Context()); !errors.Is(err, ErrUnreadablePassphrase) {
		t.Errorf("run: %v", err)
	}
	if files := h.files(t); len(files) != 0 {
		t.Errorf("files: %q", files)
	}
}

// encryptedRestoring is a restoring service whose backups are encrypted
// with testPassphrase.
func encryptedRestoring(t *testing.T) *restoring {
	t.Helper()
	r := newRestoring(t)
	r.service.cfg.Settings = func() accounts.Settings {
		return accounts.Settings{BackupHour: 4, BackupsKept: 7, BackupPassphrase: testPassphrase}
	}
	return r
}

// An encrypted backup sent to S3, SFTP and WebDAV storage is downloaded
// from each, opened with its passphrase and restored: what was added
// since is gone.
func TestAnEncryptedBackupIsRestoredFromEachDestination(t *testing.T) {
	r := encryptedRestoring(t)
	ctx := t.Context()
	h := &harness{service: r.service}
	store := newTestS3(t, "backups", "AKIDEXAMPLE")
	sftpServer := newTestSFTP(t, "polyfin", "sftp password", newHostKey(t), t.TempDir())
	dav := newTestWebDAV(t, "polyfin", "dav password")
	destinations := []Destination{
		h.create(t, Draft{Kind: KindS3, Address: new(store.url), Bucket: new("backups"), Username: new("AKIDEXAMPLE"), Secret: new("s3 secret")}),
		h.create(t, Draft{Kind: KindSFTP, Address: new(sftpServer.host), Port: new(sftpServer.port), Folder: new(sftpServer.dir),
			Username: new("polyfin"), Secret: new("sftp password")}),
		h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"), Secret: new("dav password")}),
	}
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local)
	name := r.backUp(t, at)
	if !strings.HasSuffix(name, ".dump.enc") {
		t.Fatalf("the backup made: %q", name)
	}
	store2, err := accounts.Open(ctx, r.pool)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range destinations {
		added := "added-" + d.Kind
		if _, err := store2.CreateUser(ctx, accounts.NewUser{Name: added, Password: "a password"}); err != nil {
			t.Fatal(err)
		}
		plan, err := r.service.Prepare(ctx, &d.ID, name)
		if err != nil || !plan.Locked {
			t.Fatalf("%s: prepared %+v %v", d.Kind, plan, err)
		}
		plan, err = r.service.Unlock(ctx, plan.ID, testPassphrase)
		if err != nil || plan.UnreadableCount() != 0 {
			t.Fatalf("%s: unlocked %+v %v", d.Kind, plan, err)
		}
		r.service.now = func() time.Time { return at.Add(time.Duration(i+1) * time.Hour) }
		r.restore(t, plan, false)
		if r.count(t, "SELECT count(*) FROM users WHERE name = $1", added) != 0 {
			t.Errorf("%s: a user added after the backup is still there", d.Kind)
		}
	}
	// The backups made before each restore are encrypted too.
	for _, file := range fileNames(t, r.dir) {
		if strings.Contains(file, "before-restore") && !strings.HasSuffix(file, ".dump.enc") {
			t.Errorf("a before-restore backup is not encrypted: %s", file)
		}
	}
}

// A new server, without an administrator nor backups turned on, restores
// an encrypted backup sent from the setup page: it is written to the
// setup folder, opened with its passphrase, and restored without a backup
// first; its administrators are back.
func TestAFreshInstallRestoresFromTheSetupPage(t *testing.T) {
	r := encryptedRestoring(t)
	ctx := t.Context()
	source, err := accounts.Open(ctx, r.pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.CreateFirstAdministrator(ctx, "root", "root's password", "en"); err != nil {
		t.Fatal(err)
	}
	name := r.backUp(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	sealed, err := os.ReadFile(filepath.Join(r.dir, name))
	if err != nil {
		t.Fatal(err)
	}

	fresh := migrated.New(t)
	setupFolder := filepath.Join(t.TempDir(), "backups")
	service := New(Config{Folder: func() string { return "" }, SetupFolder: setupFolder, DatabaseURL: testdb.CopyURL(t, fresh),
		PgDump: r.service.cfg.PgDump, PgRestore: r.service.cfg.PgRestore, DB: fresh, Secrets: r.service.cfg.Secrets,
		Settings: func() accounts.Settings { return accounts.Settings{} }, Logger: r.service.cfg.Logger})
	if _, err := service.PrepareUpload(ctx, name, bytes.NewReader(sealed)); !errors.Is(err, ErrOff) {
		t.Errorf("an administrator's upload while backups are off: %v", err)
	}
	plan, err := service.PrepareSetupUpload(ctx, name, bytes.NewReader(sealed))
	if err != nil || !plan.Locked || !plan.Uploaded {
		t.Fatalf("prepared: %+v %v", plan, err)
	}
	if err := service.Start(ctx, plan.ID, false, "root"); !errors.Is(err, ErrNoPlan) {
		t.Errorf("a backup sent from the setup page started as an administrator's: %v", err)
	}
	if err := service.StartSetup(ctx, plan.ID, false); !errors.Is(err, ErrPassphraseRequired) {
		t.Errorf("started locked: %v", err)
	}
	if plan, err = service.Unlock(ctx, plan.ID, testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.StartSetup(ctx, plan.ID, false); err != nil {
		t.Fatal(err)
	}
	requested := <-service.Requested()
	if err := service.Restore(ctx, requested); err != nil {
		t.Fatal(err)
	}
	restored, err := accounts.Open(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if required, err := restored.SetupRequired(ctx); err != nil || required {
		t.Errorf("setup still required: %v", err)
	}
	if _, err := restored.Authenticate(ctx, "root", "root's password"); err != nil {
		t.Errorf("the administrator restored signs in: %v", err)
	}
	// Nothing was backed up first, and the files of the restore are gone.
	if files := fileNames(t, setupFolder); len(files) != 0 {
		t.Errorf("the setup folder holds %q", files)
	}
	var entries int
	if err := fresh.QueryRow(ctx, "SELECT count(*) FROM activity_log WHERE type = 'BackupRestored' AND name = $1",
		"The database backup "+name+" was restored from the setup page").Scan(&entries); err != nil || entries != 1 {
		t.Errorf("activity: %d %v", entries, err)
	}
}

package backup

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// testKey is the POLYFIN_SECRET_KEY of the tests.
var testKey = []byte("0123456789abcdef0123456789abcdef")

func newBox(t *testing.T, key []byte) *secrets.Box {
	t.Helper()
	box, err := secrets.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return box
}

// withDestinations is a harness whose service keeps destinations in a
// migrated database, sealing their secrets with testKey.
func withDestinations(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, migrated.New(t))
	h.service.cfg.Secrets = newBox(t, testKey)
	return h
}

func (h *harness) runAt(t *testing.T, at time.Time) {
	t.Helper()
	h.service.now = func() time.Time { return at }
	if err := h.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) create(t *testing.T, draft Draft) Destination {
	t.Helper()
	d, err := h.service.CreateDestination(t.Context(), draft)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Each backup made is sent to every destination; there, the oldest past
// the destination's own number kept are deleted, and nothing else: files
// named almost like a backup, files outside the S3 prefix and other
// folders stay. What an interrupted upload left is deleted. Each
// destination records its last upload and its newest backup.
func TestBackupsAreSentToEachDestination(t *testing.T) {
	t.Parallel()
	h := withDestinations(t)
	h.kept = 3
	store := newTestS3(t, "backups", "AKIDEXAMPLE")
	sftpServer := newTestSFTP(t, "polyfin", "sftp password", newHostKey(t), t.TempDir())
	dav := newTestWebDAV(t, "polyfin", "dav password")

	others := []string{"notes.txt", "polyfin-latest.dump", "polyfin-20200101-000000.dump.bak"}
	for _, dir := range []string{filepath.Join(store.dir, "polyfin"), sftpServer.dir, dav.dir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range others {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Outside the prefix, a backup is not the destination's.
	outside := filepath.Join(store.dir, "elsewhere", "polyfin-20200101-000000.dump")
	if err := os.MkdirAll(filepath.Dir(outside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("PGDMP"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sftpServer.dir, ".polyfin-20200101-000000.dump.partial"), []byte("PGD"), 0o600); err != nil {
		t.Fatal(err)
	}

	two := 2
	s3Destination := h.create(t, Draft{Kind: KindS3, Address: new(store.url), Bucket: new("backups"), Folder: new("/polyfin/"),
		Username: new("AKIDEXAMPLE"), Secret: new("s3 secret"), Kept: &two})
	sftpDestination := h.create(t, Draft{Kind: KindSFTP, Address: new(sftpServer.host), Port: new(sftpServer.port),
		Folder: new(sftpServer.dir), Username: new("polyfin"), Secret: new("sftp password"), Kept: &two})
	davDestination := h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"), Secret: new("dav password"),
		Kept: &two})
	if s3Destination.Folder != "polyfin" || s3Destination.Port != 0 || sftpDestination.HostKey != "" || davDestination.Kept != 2 {
		t.Errorf("saved: %+v %+v %+v", s3Destination, sftpDestination, davDestination)
	}

	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local)
	for day := range 3 {
		h.runAt(t, at.AddDate(0, 0, day))
	}
	newest := []string{"polyfin-20261002-040000.dump", "polyfin-20261003-040000.dump"}
	want := slices.Concat(others, newest)
	slices.Sort(want)
	for name, dir := range map[string]string{"S3": filepath.Join(store.dir, "polyfin"), "SFTP": sftpServer.dir, "WebDAV": dav.dir} {
		if got := fileNames(t, dir); !slices.Equal(got, want) {
			t.Errorf("%s:\n%q\nwant\n%q", name, got, want)
		}
		if data, _ := os.ReadFile(filepath.Join(dir, newest[1])); string(data) != "PGDMP partial complete" {
			t.Errorf("%s: the backup sent is %q", name, data)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("the backup outside the prefix: %v", err)
	}
	if got := fileNames(t, h.dir); len(got) != 3 {
		t.Errorf("the folder keeps its own number: %q", got)
	}

	destinations, err := h.service.Destinations(t.Context())
	if err != nil || len(destinations) != 3 {
		t.Fatalf("destinations: %+v %v", destinations, err)
	}
	newestAt := at.AddDate(0, 0, 2)
	for _, d := range destinations {
		if d.RanAt == nil || !d.RanAt.Equal(newestAt) || d.Error != "" || d.NewestFile != newest[1] || d.NewestAt == nil ||
			!d.NewestAt.Equal(newestAt) || d.Problem(newestAt.Add(time.Hour), MinStaleAfter) != "" || !d.SecretSet || d.Unreadable {
			t.Errorf("%s: %+v", d.Name(), d)
		}
	}
	if fingerprint := destinations[1].HostKeyFingerprint(); fingerprint == "" {
		t.Error("the SFTP server's host key was not saved at the first connection")
	}

	// The passwords and keys are stored encrypted.
	rows, err := h.service.cfg.DB.Query(t.Context(), "SELECT secret FROM backup_destinations")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var secret string
		if err := rows.Scan(&secret); err != nil {
			t.Fatal(err)
		}
		if !secrets.Sealed(secret) {
			t.Errorf("a secret is stored as it is: %q", secret)
		}
	}
	rows.Close()
}

// A destination whose upload fails records why, which is a problem to look
// at, without failing the backup; the next backup sends what it missed.
// A destination whose newest backup is older than two days is a problem
// too.
func TestAFailedUploadIsTriedAgainAtTheNextBackup(t *testing.T) {
	t.Parallel()
	h := withDestinations(t)
	dav := newTestWebDAV(t, "polyfin", "dav password")
	d := h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"), Secret: new("dav password")})
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local)
	h.runAt(t, at)

	dav.setDown(true)
	h.runAt(t, at.AddDate(0, 0, 1))
	failed, err := h.service.Destination(t.Context(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(failed.Error, "503") || failed.NewestFile != "polyfin-20261001-040000.dump" ||
		failed.Problem(at.AddDate(0, 0, 1), MinStaleAfter) != ProblemFailed {
		t.Errorf("after a failure: %+v", failed)
	}
	if status, _ := h.service.Status(t.Context()); status.Error != "" || status.File != "polyfin-20261002-040000.dump" {
		t.Errorf("the backup itself: %+v", status)
	}

	dav.setDown(false)
	h.runAt(t, at.AddDate(0, 0, 2))
	want := []string{"polyfin-20261001-040000.dump", "polyfin-20261002-040000.dump", "polyfin-20261003-040000.dump"}
	if got := fileNames(t, dav.dir); !slices.Equal(got, want) {
		t.Errorf("after the next backup: %q", got)
	}
	recovered, _ := h.service.Destination(t.Context(), d.ID)
	if recovered.Error != "" || recovered.Problem(at.AddDate(0, 0, 2), MinStaleAfter) != "" {
		t.Errorf("after the next backup: %+v", recovered)
	}
	if problem := recovered.Problem(at.AddDate(0, 0, 2).Add(MinStaleAfter+time.Minute), MinStaleAfter); problem != ProblemStale {
		t.Errorf("two days later: %q", problem)
	}
	if problem := (Destination{}).Problem(at, MinStaleAfter); problem != "" {
		t.Errorf("before the first upload: %q", problem)
	}
}

// Test writes a file, reads it back and deletes it: nothing is left. The
// SFTP server's key is saved at the first connection; a server offering
// another key afterwards is refused before any password is sent, and its
// key is shown until it is accepted by its fingerprint.
func TestDestinationTestAndHostKey(t *testing.T) {
	t.Parallel()
	h := withDestinations(t)
	store := newTestS3(t, "backups", "AKIDEXAMPLE")
	dav := newTestWebDAV(t, "polyfin", "dav password")
	first := newTestSFTP(t, "polyfin", "sftp password", newHostKey(t), t.TempDir())
	s3Destination := h.create(t, Draft{Kind: KindS3, Address: new(store.url), Bucket: new("backups"), Username: new("AKIDEXAMPLE"),
		Secret: new("s3 secret")})
	davDestination := h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"), Secret: new("dav password")})
	sftpDestination := h.create(t, Draft{Kind: KindSFTP, Address: new(first.host), Port: new(first.port), Folder: new(first.dir),
		Username: new("polyfin"), Secret: new("sftp password")})
	for _, d := range []Destination{s3Destination, davDestination, sftpDestination} {
		if _, err := h.service.TestDestination(t.Context(), d.ID); err != nil {
			t.Errorf("%s: %v", d.Name(), err)
		}
	}
	for name, dir := range map[string]string{"S3": store.dir, "WebDAV": dav.dir, "SFTP": first.dir} {
		if got := fileNames(t, dir); len(got) != 0 {
			t.Errorf("%s keeps %q", name, got)
		}
	}
	saved, _ := h.service.Destination(t.Context(), sftpDestination.ID)
	if saved.HostKey == "" {
		t.Fatal("no host key saved")
	}

	// Another server answers at the address: the database is changed
	// directly, as the admin API forgets the key with the address.
	otherKey := newHostKey(t)
	other := newTestSFTP(t, "polyfin", "sftp password", otherKey, first.dir)
	if _, err := h.service.cfg.DB.Exec(t.Context(), "UPDATE backup_destinations SET port = $2 WHERE id = $1", sftpDestination.ID,
		other.port); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.TestDestination(t.Context(), sftpDestination.ID); !errors.Is(err, ErrHostKeyChanged) {
		t.Errorf("another host key: %v", err)
	}
	after, _ := h.service.Destination(t.Context(), sftpDestination.ID)
	if after.HostKey != saved.HostKey {
		t.Error("the saved key was replaced")
	}
	if want := ssh.FingerprintSHA256(otherKey.PublicKey()); after.OfferedHostKeyFingerprint() != want {
		t.Errorf("offered key shown: %q, want %q", after.OfferedHostKeyFingerprint(), want)
	}
	if n := other.passwords.Load(); n != 0 {
		t.Errorf("%d passwords sent to a key not accepted", n)
	}
	accepted, err := h.service.UpdateDestination(t.Context(), sftpDestination.ID, Draft{AcceptHostKey: after.OfferedHostKeyFingerprint()})
	if err != nil || accepted.HostKeyFingerprint() != after.OfferedHostKeyFingerprint() || accepted.OfferedHostKey != "" {
		t.Fatalf("accepting the key shown: %+v %v", accepted, err)
	}
	if _, err := h.service.TestDestination(t.Context(), sftpDestination.ID); err != nil {
		t.Errorf("after accepting the key: %v", err)
	}

	// A wrong password fails the test, saying why.
	if _, err := h.service.UpdateDestination(t.Context(), davDestination.ID, Draft{Secret: new("wrong")}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.TestDestination(t.Context(), davDestination.ID); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("a wrong password: %v", err)
	}
}

// Once an SFTP destination's key is forgotten, or its port changed, the
// key the server offers is refused before any password is sent, and shown;
// only the key of the fingerprint accepted is saved.
func TestAForgottenHostKeyIsAcceptedByTheFingerprintShown(t *testing.T) {
	t.Parallel()
	h := withDestinations(t)
	firstKey, otherKey := newHostKey(t), newHostKey(t)
	first := newTestSFTP(t, "polyfin", "sftp password", firstKey, t.TempDir())
	other := newTestSFTP(t, "polyfin", "sftp password", otherKey, first.dir)
	d := h.create(t, Draft{Kind: KindSFTP, Address: new(first.host), Port: new(first.port), Folder: new(first.dir),
		Username: new("polyfin"), Secret: new("sftp password")})
	if _, err := h.service.TestDestination(t.Context(), d.ID); err != nil {
		t.Fatal(err)
	}

	// Forget: the same server's key is shown, not saved.
	forgotten, err := h.service.UpdateDestination(t.Context(), d.ID, Draft{HostKey: new("")})
	if err != nil || forgotten.HostKey != "" || !forgotten.HostKeyForgotten {
		t.Fatalf("forgetting the key: %+v %v", forgotten, err)
	}
	sent := first.passwords.Load()
	if _, err := h.service.TestDestination(t.Context(), d.ID); !errors.Is(err, ErrHostKeyNotAccepted) {
		t.Errorf("after forgetting the key: %v", err)
	}
	shown, _ := h.service.Destination(t.Context(), d.ID)
	if shown.HostKey != "" || shown.OfferedHostKeyFingerprint() != ssh.FingerprintSHA256(firstKey.PublicKey()) {
		t.Errorf("after forgetting the key: saved %q, shown %q", shown.HostKey, shown.OfferedHostKeyFingerprint())
	}
	if n := first.passwords.Load(); n != sent {
		t.Errorf("%d passwords sent to a key not accepted", n-sent)
	}

	// Another port: the key shown is the new server's.
	if _, err := h.service.UpdateDestination(t.Context(), d.ID, Draft{Port: new(other.port)}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.TestDestination(t.Context(), d.ID); !errors.Is(err, ErrHostKeyNotAccepted) {
		t.Errorf("another port: %v", err)
	}
	if n := other.passwords.Load(); n != 0 {
		t.Errorf("%d passwords sent to a key not accepted", n)
	}
	moved, _ := h.service.Destination(t.Context(), d.ID)
	want := ssh.FingerprintSHA256(otherKey.PublicKey())
	if moved.OfferedHostKeyFingerprint() != want {
		t.Fatalf("another port: shown %q, want %q", moved.OfferedHostKeyFingerprint(), want)
	}

	// The key accepted is the one shown, no other.
	for _, wrong := range []string{ssh.FingerprintSHA256(firstKey.PublicKey()), "SHA256:x"} {
		if _, err := h.service.UpdateDestination(t.Context(), d.ID, Draft{AcceptHostKey: wrong}); !errors.Is(err, ErrOfferedKeyChanged) {
			t.Errorf("accepting %s: %v", wrong, err)
		}
	}
	if still, _ := h.service.Destination(t.Context(), d.ID); still.HostKey != "" || still.OfferedHostKey != moved.OfferedHostKey {
		t.Errorf("a key not shown changed the destination: %+v", still)
	}
	accepted, err := h.service.UpdateDestination(t.Context(), d.ID, Draft{AcceptHostKey: want})
	if err != nil || accepted.HostKeyFingerprint() != want || accepted.OfferedHostKey != "" || accepted.HostKeyForgotten {
		t.Fatalf("accepting the key shown: %+v %v", accepted, err)
	}
	if _, err := h.service.TestDestination(t.Context(), d.ID); err != nil || other.passwords.Load() == 0 {
		t.Errorf("after accepting the key: %v", err)
	}
}

// A destination is refused when it cannot work; its password is kept when
// a change leaves it out; with another POLYFIN_SECRET_KEY, it is
// unreadable and sends nothing, saying why.
func TestDestinationsAreChecked(t *testing.T) {
	t.Parallel()
	h := withDestinations(t)
	private := newPrivateKey(t)
	for _, test := range []struct {
		name  string
		draft Draft
		err   error
	}{
		{"an unknown kind", Draft{Kind: "ftp", Address: new("ftp.example")}, ErrInvalidDestinationKind},
		{"an S3 endpoint with a path", Draft{Kind: KindS3, Address: new("https://s3.example/bucket"), Bucket: new("backups"),
			Username: new("a"), Secret: new("b")}, ErrInvalidDestinationAddress},
		{"an S3 endpoint with a user", Draft{Kind: KindS3, Address: new("https://a:b@s3.example"), Bucket: new("backups"),
			Username: new("a"), Secret: new("b")}, ErrInvalidDestinationAddress},
		{"no bucket", Draft{Kind: KindS3, Address: new("https://s3.example"), Username: new("a"), Secret: new("b")}, ErrInvalidBucket},
		{"a prefix going up", Draft{Kind: KindS3, Address: new("https://s3.example"), Bucket: new("backups"), Folder: new("a/../b"),
			Username: new("a"), Secret: new("b")}, ErrInvalidDestinationFolder},
		{"no secret key", Draft{Kind: KindS3, Address: new("https://s3.example"), Bucket: new("backups"), Username: new("a")},
			ErrInvalidSecret},
		{"an SFTP address", Draft{Kind: KindSFTP, Address: new("sftp://nas.lan"), Username: new("a"), Secret: new("b")},
			ErrInvalidDestinationAddress},
		{"an SFTP port", Draft{Kind: KindSFTP, Address: new("nas.lan"), Port: new(70000), Username: new("a"), Secret: new("b")},
			ErrInvalidPort},
		{"no SFTP password nor key", Draft{Kind: KindSFTP, Address: new("nas.lan"), Username: new("a")}, ErrInvalidSecret},
		{"a broken private key", Draft{Kind: KindSFTP, Address: new("nas.lan"), Username: new("a"), PrivateKey: new("not a key")},
			ErrInvalidPrivateKey},
		{"a WebDAV address", Draft{Kind: KindWebDAV, Address: new("nas.lan/dav")}, ErrInvalidDestinationAddress},
		{"too many kept", Draft{Kind: KindWebDAV, Address: new("https://nas.lan/dav/"), Kept: new(accounts.MaxBackupsKept + 1)},
			ErrInvalidDestinationKept},
	} {
		if _, err := h.service.CreateDestination(t.Context(), test.draft); !errors.Is(err, test.err) {
			t.Errorf("%s: %v, want %v", test.name, err, test.err)
		}
	}
	keyed, err := h.service.CreateDestination(t.Context(), Draft{Kind: KindSFTP, Address: new("[::1]"), Username: new("a"),
		PrivateKey: new(private)})
	if err != nil || keyed.Address != "::1" || keyed.Port != 22 || !keyed.PrivateKeySet || keyed.SecretSet || keyed.Kept != 7 {
		t.Errorf("an SFTP server with a key: %+v %v", keyed, err)
	}
	if keyed.Name() != "a@::1" {
		t.Errorf("name: %q", keyed.Name())
	}

	dav := newTestWebDAV(t, "polyfin", "dav password")
	d := h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"), Secret: new("dav password")})
	if _, err := h.service.UpdateDestination(t.Context(), d.ID, Draft{Kept: new(3)}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.TestDestination(t.Context(), d.ID); err != nil {
		t.Errorf("the password left out is kept: %v", err)
	}
	h.service.cfg.Secrets = newBox(t, []byte("fedcba9876543210fedcba9876543210"))
	unreadable, _ := h.service.Destination(t.Context(), d.ID)
	if !unreadable.Unreadable {
		t.Error("a password sealed with another key is readable")
	}
	h.runAt(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	if after, _ := h.service.Destination(t.Context(), d.ID); !strings.Contains(after.Error, "POLYFIN_SECRET_KEY") {
		t.Errorf("an upload with an unreadable password: %+v", after)
	}
	if got := fileNames(t, dav.dir); len(got) != 0 {
		t.Errorf("sent with an unreadable password: %q", got)
	}
	if err := h.service.DeleteDestination(t.Context(), d.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.service.DeleteDestination(t.Context(), d.ID); !errors.Is(err, ErrDestinationNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}

// newPrivateKey is an SSH private key in OpenSSH's format.
func newPrivateKey(t *testing.T) string {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

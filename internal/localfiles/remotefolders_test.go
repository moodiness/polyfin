package localfiles

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/source"
)

// remoteCase is a folder of a storage a test serves dir from: the folder
// to add, with a wrong credential and with the right one, what reads its
// files, the ranges they were read with when the fake tells them, and
// what stops the storage.
type remoteCase struct {
	name   string
	kind   string
	wrong  NewFolder
	right  Changes
	ranges func() []string
	stop   func()
	// secrets are the columns that must hold what the folder was added
	// with, sealed.
	secrets []string
}

// remoteCases serve dir, holding a folder movies, as an S3 bucket, another
// one answering ranges with 200 OK, as rclone serve s3 does, an SFTP
// folder signed in with a private key and its passphrase, and an rclone
// remote.
func remoteCases(t *testing.T, dir string) []remoteCase {
	t.Helper()
	bucket, s3Server := serveS3(t, dir, "media", "AKIDTEST", "right secret")
	okBucket, okServer := serveS3(t, dir, "served", "AKIDTEST", "right secret")
	okBucket.rangesOK = true
	sftpKey, private := newSigner(t)
	sftpServer := serveSFTP(t, "reader", "", sftpKey.PublicKey())
	_, otherPrivate := newSigner(t)
	rclone, rcloneServer := serveRclone(t, "media", dir, "rc", "right password")
	rcloneHost, _ := url.Parse(rcloneServer.URL)
	return []remoteCase{
		{
			name: "s3", kind: ShareS3,
			wrong: NewFolder{Name: "Bucket", Path: "s3://media/movies/", Kind: KindMovies, Endpoint: s3Server.URL, Region: "eu-test-1",
				User: "AKIDTEST", Password: "wrong secret"},
			right: Changes{Password: new("right secret")}, ranges: bucket.readRanges, stop: s3Server.Close, secrets: []string{"share_password"},
		},
		{
			name: "s3 answering ranges with 200", kind: ShareS3,
			wrong: NewFolder{Name: "Bucket of rclone", Path: "s3://served/movies/", Kind: KindMovies, Endpoint: okServer.URL, Region: "eu-test-1",
				User: "AKIDTEST", Password: "wrong secret"},
			right: Changes{Password: new("right secret")}, ranges: okBucket.readRanges, stop: okServer.Close, secrets: []string{"share_password"},
		},
		{
			name: "sftp", kind: ShareSFTP,
			wrong: NewFolder{Name: "Server", Path: "sftp://" + sftpServer.addr + filepath.ToSlash(filepath.Join(dir, "movies")),
				Kind: KindMovies, User: "reader", PrivateKey: openSSHKey(t, otherPrivate, "pass phrase"), Passphrase: "pass phrase"},
			right: Changes{PrivateKey: new(openSSHKey(t, private, "other phrase")), Passphrase: new("other phrase")}, stop: sftpServer.stop,
			secrets: []string{"share_private_key", "share_passphrase"},
		},
		{
			name: "rclone", kind: ShareRclone,
			wrong: NewFolder{Name: "Remote", Path: "rclone://" + rcloneHost.Host + "/movies", Kind: KindMovies, Remote: "media", User: "rc",
				Password: "wrong password"},
			right: Changes{Password: new("right password")}, ranges: rclone.readRanges, stop: rcloneServer.Close,
			secrets: []string{"share_password"},
		},
	}
}

// A bucket, an SFTP folder and an rclone remote are scanned folder by
// folder and matched as a local folder is, their secrets stored sealed. A
// wrong secret key, private key or password is refused until the right
// one is entered. Their videos are read through the source cache straight
// from the storage: a seek reads only the blocks missing, and those read
// before need the storage no more.
func TestRemoteFoldersAreScannedAndRead(t *testing.T) {
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922) - 1080p.mkv"), 10)
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Extras", "The General (1926).mkv"), 10)
	write(t, filepath.Join(dir, "movies", "An Unknown Picture (1950).mkv"), 10)
	write(t, filepath.Join(dir, "elsewhere", "The General (1926).mkv"), 10)
	data := content(3<<20 + 12345)
	if err := os.WriteFile(filepath.Join(dir, "movies", "Night of the Living Dead (1968).mkv"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cache, err := source.New(t.TempDir(), func() int64 { return 1 << 30 }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	want := map[string]string{
		"Night of the Living Dead (1968).mkv":           "tt0063350",
		"Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv": "tt0013442",
		"An Unknown Picture (1950).mkv":                 "unmatched: not_found",
	}
	for _, c := range remoteCases(t, dir) {
		t.Run(c.name, func(t *testing.T) {
			addon, err := e.service.Add(t.Context(), c.wrong)
			if err != nil {
				t.Fatal(err)
			}
			e.idle(addon.ID)
			if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Error != errRefused || folder.Files != 0 || folder.Share != c.kind {
				t.Errorf("a wrong secret: %+v", folder)
			}
			if _, err := e.service.Update(t.Context(), addon.ID, c.right); err != nil {
				t.Fatal(err)
			}
			e.idle(addon.ID)
			if got := e.files(addon.ID); !maps.Equal(got, want) {
				t.Errorf("files: %v", got)
			}
			folder, err := e.service.Folder(t.Context(), addon.ID)
			if err != nil || folder.Error != "" || folder.PollMinutes == nil || *folder.PollMinutes != *defaultPollMinutes(c.kind) {
				t.Errorf("folder: %+v %v", folder, err)
			}
			for _, column := range c.secrets {
				var stored string
				if err := e.pool.QueryRow(t.Context(), "SELECT "+column+" FROM local_folders WHERE addon_id = $1", addon.ID).Scan(&stored); err != nil ||
					!secrets.Sealed(stored) {
					t.Errorf("%s is not stored sealed: %v", column, err)
				}
			}

			key := e.fileKey(addon.ID, "Night of the Living Dead (1968).mkv")
			location, ok := e.service.Source(t.Context(), key)
			if !ok || location.Opener == nil {
				t.Fatalf("no location in the source cache: %+v", location)
			}
			src := cache.Open(addon.ID, location, nil)
			defer src.Release()
			read := func(off int64) error {
				t.Helper()
				got := make([]byte, 1000)
				if _, err := src.ReadAt(t.Context(), got, off); err != nil {
					return err
				}
				if !bytes.Equal(got, data[off:off+1000]) {
					t.Errorf("the bytes at %d differ", off)
				}
				return nil
			}
			before := 0
			if c.ranges != nil {
				before = len(c.ranges())
			}
			for _, off := range []int64{2<<20 + 77, 5} {
				if err := read(off); err != nil {
					t.Fatalf("at %d: %v", off, err)
				}
			}
			if c.ranges != nil {
				if got := c.ranges()[before:]; !slices.Equal(got, []string{"bytes=2097152-", "bytes=0-"}) {
					t.Errorf("ranges read: %v", got)
				}
			}
			// The file opens as one, for the scans and the loopback too.
			file, _, err := e.service.Open(t.Context(), key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Seek(123_456, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, 10)
			if _, err := io.ReadFull(file, got); err != nil || !bytes.Equal(got, data[123_456:123_466]) {
				t.Errorf("bytes after a seek: %v", err)
			}
			file.Close()
			c.stop()
			for _, off := range []int64{2<<20 + 500, 300} {
				if err := read(off); err != nil {
					t.Errorf("a block read before, at %d: %v", off, err)
				}
			}
		})
	}
}

// A bucket's folders are listed page by page: a folder holding more than
// a page lists whole.
func TestBucketListingsArePaged(t *testing.T) {
	dir := t.TempDir()
	for i := range 7 {
		write(t, filepath.Join(dir, "movies", "Movie "+string(rune('A'+i))+" (1950).mkv"), 10)
	}
	write(t, filepath.Join(dir, "movies", "Shows", "Old", "The General (1926).mkv"), 10)
	_, server := serveS3(t, dir, "media", "AKIDTEST", "secret")
	r, err := newRemote(shareAccess{address: "s3://media/movies", endpoint: server.URL, region: "eu-test-1", user: "AKIDTEST",
		password: "secret"}, hostKeys{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.close)
	entries, err := r.list(t.Context(), "")
	if err != nil || len(entries) != 8 {
		t.Fatalf("%d entries, %v", len(entries), err)
	}
	if entries, err := r.list(t.Context(), "Shows"); err != nil || len(entries) != 1 || !entries[0].folder || entries[0].name != "Old" {
		t.Errorf("a folder: %+v %v", entries, err)
	}
	other, err := newRemote(shareAccess{address: "s3://media/movies/Shows/Old/The General (1926).mkv", endpoint: server.URL,
		region: "eu-test-1", user: "AKIDTEST", password: "secret"}, hostKeys{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(other.close)
	if _, err := other.list(t.Context(), ""); shareFailure(err) != errNotFolder {
		t.Errorf("an object as the folder: %v", err)
	}
	missing, err := newRemote(shareAccess{address: "s3://other/movies", endpoint: server.URL, region: "eu-test-1", user: "AKIDTEST",
		password: "secret"}, hostKeys{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(missing.close)
	if _, err := missing.list(t.Context(), ""); shareFailure(err) != errMissing {
		t.Errorf("a bucket missing: %v", err)
	}
}

// An rclone instance that does not know the remote answers that it is
// missing; a path that is a file is no folder.
func TestRcloneRemoteMissing(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922).mkv"), 10)
	_, server := serveRclone(t, "media", dir, "rc", "secret")
	host, _ := url.Parse(server.URL)
	for address, failure := range map[string]string{
		"rclone://" + host.Host + "/movies":                      "",
		"rclone://" + host.Host + "/elsewhere":                   errMissing,
		"rclone://" + host.Host + "/movies/Nosferatu (1922).mkv": errNotFolder,
	} {
		r := newRclone(must(url.Parse(address)), shareAccess{user: "rc", password: "secret", remote: "media"})
		if _, err := r.list(t.Context(), ""); err != nil && shareFailure(err) != failure || err == nil && failure != "" {
			t.Errorf("%s: %v", address, err)
		}
	}
	r := newRclone(must(url.Parse("rclone://"+host.Host+"/movies")), shareAccess{user: "rc", password: "secret", remote: "other"})
	if _, err := r.list(t.Context(), ""); shareFailure(err) != errMissing {
		t.Errorf("a remote rclone does not know: %v", err)
	}
	e := newEnv(t)
	remotes, failure, err := e.service.RcloneRemotes(t.Context(), server.URL, "rc", new("secret"), nil)
	if err != nil || failure != "" || !slices.Equal(remotes, []string{"media"}) {
		t.Errorf("remotes: %v %q %v", remotes, failure, err)
	}
	if _, failure, err := e.service.RcloneRemotes(t.Context(), server.URL, "rc", new("wrong"), nil); err != nil || failure != errRefused {
		t.Errorf("a wrong password: %q %v", failure, err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// An SFTP server's host key is saved at the first connection; another one
// later is refused, the folder failing with host_key_changed and keeping
// its files, until an administrator accepts it. A wrong password is
// refused.
func TestSFTPHostKeys(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922).mkv"), 10)
	server := serveSFTP(t, "reader", "pass phrase", nil)
	address := "sftp://" + server.addr + filepath.ToSlash(filepath.Join(dir, "movies"))
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Server", Path: address, Kind: KindMovies, User: "reader", Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	server.mu.Lock()
	first := ssh.FingerprintSHA256(server.hostKey.PublicKey())
	server.mu.Unlock()
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Error != "" || folder.Files != 1 || folder.HostKey != first || folder.OfferedHostKey != "" {
		t.Fatalf("first connection: %+v %v", folder, err)
	}

	other := server.rekey(t)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	folder, err = e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Error != errHostKey || folder.Files != 1 || folder.HostKey != first ||
		folder.OfferedHostKey != ssh.FingerprintSHA256(other) {
		t.Fatalf("another host key: %+v %v", folder, err)
	}
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{AcceptHostKey: true}); err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	folder, err = e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Error != "" || folder.Files != 1 || folder.HostKey != ssh.FingerprintSHA256(other) || folder.OfferedHostKey != "" {
		t.Errorf("the host key accepted: %+v %v", folder, err)
	}

	if _, err := e.service.Update(t.Context(), addon.ID, Changes{Password: new("wrong")}); err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Error != errRefused || folder.Files != 1 {
		t.Errorf("a wrong password: %+v", folder)
	}
	// A connection the server closed is made again.
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{Password: new("pass phrase")}); err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	server.cut()
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Error != "" || folder.Files != 1 {
		t.Errorf("after the connection closed: %+v", folder)
	}
}

// A file added to a bucket, an SFTP folder or an rclone remote is found
// by the next listing round, the share polled as such; one listed only
// at its scans is not watched.
func TestRemoteFoldersArePolled(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	_, s3Server := serveS3(t, dir, "media", "AKIDTEST", "secret")
	sftpServer := serveSFTP(t, "reader", "pass phrase", nil)
	_, rcloneServer := serveRclone(t, "media", dir, "rc", "secret")
	rcloneHost, _ := url.Parse(rcloneServer.URL)
	for i, access := range []shareAccess{
		{address: "s3://media/movies", endpoint: s3Server.URL, region: "eu-test-1", user: "AKIDTEST", password: "secret"},
		{address: "sftp://" + sftpServer.addr + filepath.ToSlash(filepath.Join(dir, "movies")), user: "reader", password: "pass phrase"},
		{address: "rclone://" + rcloneHost.Host + "/movies", remote: "media", user: "rc", password: "secret"},
	} {
		kind := shareKind(access.address)
		t.Run(kind, func(t *testing.T) {
			r, err := newRemote(access, hostKeys{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(r.close)
			id := testID(byte(i + 1))
			wt, _ := newPollTest(t, r, func(context.Context) ([]watchFolder, error) {
				return []watchFolder{{id: id, root: access.address, every: testEvery}}, nil
			})
			wt.start()
			name := "Movie " + kind + " (1950).mkv"
			write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", name), 10)
			wt.scanned(id)
			wt.watchedAs(id, SharePolled, kind)
			if err := os.Remove(filepath.Join(dir, "movies", "Nosferatu (1922)", name)); err != nil {
				t.Fatal(err)
			}
			wt.scanned(id)
		})
	}

	// How often a folder is listed comes from its setting; 0 lists it
	// only at its scans.
	settings := accounts.DefaultSettings()
	settings.WatchLocalFolders = true
	installed := []addons.Addon{{ID: testID(1), Kind: addons.KindLocal, ManifestURL: "s3://media/movies", Enabled: true},
		{ID: testID(2), Kind: addons.KindLocal, ManifestURL: "rclone://rclone:5572/movies", Enabled: true}}
	got := watchable(settings, installed, map[accounts.ID]int{testID(1): 30, testID(2): 0})
	if want := []watchFolder{{id: testID(1), root: "s3://media/movies", every: 30 * time.Minute}}; !slices.Equal(got, want) {
		t.Errorf("watched: %+v", got)
	}
}

// A disc image on a bucket is matched and plays as on any share: its
// main title as one MPEG-TS file, read from the image by ranges.
func TestBucketDiscImages(t *testing.T) {
	defer func(old time.Duration) { minMovie = old }(minMovie)
	minMovie = 20 * time.Second
	e := newEnv(t)
	dir := t.TempDir()
	unpackImage(t, "bluray", filepath.Join(dir, "movies", "Night of the Living Dead (1968).iso"))
	_, server := serveS3(t, dir, "media", "AKIDTEST", "secret")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Bucket", Path: "s3://media/movies", Kind: KindMovies, Endpoint: server.URL,
		Region: "eu-test-1", User: "AKIDTEST", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if got := e.files(addon.ID); got["Night of the Living Dead (1968).iso"] != "tt0063350" {
		t.Fatalf("files: %v", got)
	}
	key := strings.TrimSuffix(e.fileKey(addon.ID, "Night of the Living Dead (1968).iso"), ".iso")
	in, err := e.service.Input(t.Context(), key)
	if err != nil || in.Format != "mpegts" || in.Path != "http://files.example/"+key+".m2ts" {
		t.Errorf("input %+v %v", in, err)
	}
	if got, err := e.service.Keyframes(t.Context(), key); err != nil || len(got) == 0 {
		t.Errorf("%d keyframes, %v", len(got), err)
	}
	file, _, err := e.service.Open(t.Context(), key+".m2ts")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil || size == 0 {
		t.Fatalf("%d bytes, %v", size, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	whole, err := io.ReadAll(file)
	if err != nil || int64(len(whole)) != size {
		t.Fatalf("%d bytes read, %v", len(whole), err)
	}
	if _, err := file.Seek(size/2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	part := make([]byte, 4096)
	if _, err := io.ReadFull(file, part); err != nil || !bytes.Equal(part, whole[size/2:size/2+4096]) {
		t.Errorf("the bytes after a seek differ: %v", err)
	}
	// A title of a disc is read through the loopback, not the cache.
	if _, ok := e.service.Source(t.Context(), key+".m2ts"); ok {
		t.Error("a disc's title has a location in the source cache")
	}
}

// newPollTest polls the share r reads, scanned first, as the folders
// folders lists; each scan of the watch is a scan of the fake, and told.
func newPollTest(t *testing.T, r remote, folders func(context.Context) ([]watchFolder, error)) (*watchTest, *fakeShares) {
	t.Helper()
	shares := &fakeShares{t: t, kind: KindMovies, r: r}
	shares.rescan()
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	scans := make(chan accounts.ID, 64)
	w := newWatcher(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), folders, func(_ context.Context, id accounts.ID) {
		shares.rescan()
		scans <- id
	}, openNotifyWatcher, shares, wg.Go)
	w.quiet, w.check, w.every, w.gap = testQuiet, time.Hour, time.Hour, 0
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
	return &watchTest{t: t, w: w, wg: wg, scans: scans}, shares
}

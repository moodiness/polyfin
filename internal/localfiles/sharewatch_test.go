package localfiles

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/secrets"
)

// testEvery is how often the shares of the tests are polled.
const testEvery = 20 * time.Millisecond

// fakeShares is the shareSource of a share a test serves: its reader, and
// the prints of its last scan, which rescan makes as a scan does.
type fakeShares struct {
	t     *testing.T
	kind  string
	reads atomic.Int32

	mu      sync.Mutex
	r       remote
	prints  map[string]uint64
	kept    int
	scanned bool
}

func (f *fakeShares) shareReader(context.Context, accounts.ID, string) (remote, string, error) {
	f.reads.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.r, f.kind, nil
}

func (f *fakeShares) shareSeen(accounts.ID) seenShare {
	f.mu.Lock()
	defer f.mu.Unlock()
	return seenShare{prints: f.prints, kept: f.kept, scanned: f.scanned}
}

func (f *fakeShares) use(r remote) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.r = r
}

// rescan lists the share as a scan does, and keeps the prints it made.
func (f *fakeShares) rescan() {
	f.mu.Lock()
	defer f.mu.Unlock()
	root, err := f.r.list(context.Background(), "")
	if err != nil {
		f.t.Errorf("the share cannot be listed: %v", err)
		return
	}
	list, err := walkShare(context.Background(), f.r, root, f.kind)
	if err != nil {
		f.t.Errorf("the share cannot be walked: %v", err)
		return
	}
	f.prints, f.scanned = list.prints, true
	f.kept++
}

// davRemote reads a share served by serveWebDAV.
func davRemote(t *testing.T, address, user, password string) remote {
	t.Helper()
	r, err := newRemote(address, user, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.close)
	return r
}

// newShareWatchTest polls a WebDAV share of movies serving dir, scanned
// first; each scan of the watch is a scan of the fake, and told.
func newShareWatchTest(t *testing.T, dir string, folders func(address string) func(context.Context) ([]watchFolder, error)) (*watchTest,
	*fakeShares, string) {
	t.Helper()
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	address := server.URL + "/movies"
	shares := &fakeShares{t: t, kind: KindMovies, r: davRemote(t, address, "reader", "pass phrase")}
	shares.rescan()
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	scans := make(chan accounts.ID, 64)
	w := newWatcher(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), folders(address), func(_ context.Context, id accounts.ID) {
		shares.rescan()
		scans <- id
	}, openNotifyWatcher, shares, wg.Go)
	w.quiet, w.check, w.every, w.gap = testQuiet, time.Hour, testEvery, 0
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
	return &watchTest{t: t, w: w, wg: wg, scans: scans}, shares, address
}

// oneShare lists the share at an address as the folder id.
func oneShare(id accounts.ID) func(address string) func(context.Context) ([]watchFolder, error) {
	return func(address string) func(context.Context) ([]watchFolder, error) { return one(id, address) }
}

// A file added to a share's folder, deep or at its root, has the share
// scanned within a few polls, once.
func TestShareWatchScansAfterFilesAdded(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(dir, "movies", "Shows", "Old", "The General (1926).mkv"), 10)
	id := testID(1)
	wt, _, _ := newShareWatchTest(t, dir, oneShare(id))
	wt.start()

	write(t, filepath.Join(dir, "movies", "Shows", "Old", "The Gold Rush (1925).mkv"), 10)
	wt.scanned(id)
	wt.still()

	write(t, filepath.Join(dir, "movies", "Night of the Living Dead (1968)", "Night of the Living Dead (1968).mkv"), 10)
	wt.scanned(id)
	wt.still()

	// A subtitle file is read by a scan too, as is an image, and so is a
	// removal.
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).fr.srt"), 10)
	wt.scanned(id)
	wt.still()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "poster.jpg"), 10)
	wt.scanned(id)
	wt.still()
	if err := os.Remove(filepath.Join(dir, "movies", "Shows", "Old", "The General (1926).mkv")); err != nil {
		t.Fatal(err)
	}
	wt.scanned(id)
	wt.still()
}

// A share whose listings are those of its last scan is not scanned, nor
// one where only files a scan does not read change.
func TestShareWatchQuietWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	wt, shares, _ := newShareWatchTest(t, dir, oneShare(id))
	wt.start()
	wt.still()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "notes.txt"), 10)
	write(t, filepath.Join(dir, "movies", ".hidden", "The General (1926).mkv"), 10)
	wt.still()
	if shares.reads.Load() < 3 {
		t.Errorf("polled %d times, want several", shares.reads.Load())
	}
}

// With the setting off, shares are not polled.
func TestShareWatchOff(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922).mkv"), 10)
	settings := accounts.DefaultSettings()
	settings.WatchLocalFolders = false
	id := testID(1)
	wt, shares, _ := newShareWatchTest(t, dir, func(address string) func(context.Context) ([]watchFolder, error) {
		installed := []addons.Addon{{ID: id, Kind: addons.KindLocal, ManifestURL: address, Enabled: true}}
		return func(context.Context) ([]watchFolder, error) { return watchable(settings, installed), nil }
	})
	wt.start()
	write(t, filepath.Join(dir, "movies", "The General (1926).mkv"), 10)
	wt.still()
	if shares.reads.Load() != 0 || len(wt.w.polls) != 0 {
		t.Errorf("polled %d times with the setting off", shares.reads.Load())
	}
}

// A share that refuses the user, or cannot be reached, is unwatched with
// why; once it answers again, it is scanned and watched.
func TestShareWatchFailure(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	wt, shares, address := newShareWatchTest(t, dir, oneShare(id))
	good := shares.r
	wt.start()

	reason := func(want string) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(testEvery) {
			if wt.w.reason(id) == want {
				return
			}
		}
		t.Fatalf("unwatched %q, want %q", wt.w.reason(id), want)
	}
	shares.use(davRemote(t, address, "reader", "wrong"))
	reason(errRefused)
	wt.still()

	shares.use(davRemote(t, "http://127.0.0.1:1/movies", "reader", "pass phrase"))
	reason(errUnreachable)

	shares.use(good)
	wt.scanned(id)
	reason("")
	wt.still()
}

// countedRemote counts the listings of a remote.
type countedRemote struct {
	remote
	lists atomic.Int32
}

func (c *countedRemote) list(ctx context.Context, rel string) ([]remoteEntry, error) {
	c.lists.Add(1)
	return c.remote.list(ctx, rel)
}

// A round lists at most lists folders, its root first; the next ones go
// on where it stopped, so that every folder is listed over a few rounds.
func TestShareWatchRoundsAreBounded(t *testing.T) {
	dir := t.TempDir()
	for i := range 7 {
		write(t, filepath.Join(dir, "movies", fmt.Sprintf("Movie %d (19%d0)", i, 2+i), "movie.mkv"), 10)
	}
	id := testID(1)
	wt, shares, _ := newShareWatchTest(t, dir, oneShare(id))
	counted := &countedRemote{remote: shares.r}
	shares.use(counted)
	wt.w.lists = 3
	wt.w.polled = make(chan sharePolled, 8)
	p := &sharePoll{id: id}

	// The last of the seven folders changes: each round lists the root
	// and two folders, so the fourth one lists it.
	write(t, filepath.Join(dir, "movies", "Movie 6 (1980)", "movie.fr.srt"), 10)
	for round := 1; round <= 5; round++ {
		counted.lists.Store(0)
		wt.w.round(t.Context(), p)
		if got := counted.lists.Load(); got > 3 {
			t.Fatalf("round %d listed %d folders, want 3 at most", round, got)
		}
		select {
		case got := <-wt.w.polled:
			if got.id != id || round != 4 {
				t.Fatalf("round %d found %v changed, want round 4", round, got)
			}
			return
		default:
		}
	}
	t.Fatal("the change was never found")
}

// A poll that compared the prints of a scan before the last one is
// stale: the scan that ended since listed the share again, and the next
// round compares with what that scan kept. A poll that compared the last
// prints has the share scanned.
func TestShareWatchDropsStalePolls(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	wt, shares, address := newShareWatchTest(t, dir, oneShare(id))
	wt.w.polls[id] = &polling{address: address, cancel: func() {}}
	wt.w.polled = make(chan sharePolled, 1)
	p := &sharePoll{id: id, address: address}
	poll := func() sharePolled {
		t.Helper()
		wt.w.round(t.Context(), p)
		select {
		case got := <-wt.w.polled:
			return got
		default:
			t.Fatal("the change was not found")
			return sharePolled{}
		}
	}

	write(t, filepath.Join(dir, "movies", "The General (1926)", "The General (1926).mkv"), 10)
	stale := poll()
	// A scan ends before the watch reads the poll: it kept the share as
	// it is now.
	shares.rescan()
	wt.w.polledShare(stale)
	if _, waiting := wt.w.changed[id]; waiting {
		t.Fatal("a poll that compared the prints of an earlier scan has the share scanned again")
	}
	write(t, filepath.Join(dir, "movies", "Metropolis (1927)", "Metropolis (1927).mkv"), 10)
	wt.w.polledShare(poll())
	if _, waiting := wt.w.changed[id]; !waiting {
		t.Fatal("a poll that compared the last prints does not have the share scanned")
	}
}

// A movie copied to a WebDAV share is scanned and matched without a scan
// asked for; with nothing changed, the share is not scanned.
func TestShareWatchFindsNewMovie(t *testing.T) {
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/movies/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	var scans atomic.Int32
	w := newWatcher(e.service.ctx, e.service.logger, e.service.watchable, func(ctx context.Context, id accounts.ID) {
		scans.Add(1)
		e.service.watchScan(ctx, id)
	}, openNotifyWatcher, e.service, e.service.wg.Go)
	w.quiet, w.every, w.gap = testQuiet, testEvery, 0
	e.service.watch = w
	w.reconcile()
	e.service.wg.Go(w.loop)

	time.Sleep(10 * testEvery)
	if got := scans.Load(); got != 0 {
		t.Fatalf("%d scans with nothing changed", got)
	}
	write(t, filepath.Join(dir, "movies", "The General (1926)", "The General (1926).mkv"), 10)
	for deadline := time.Now().Add(10 * time.Second); e.files(addon.ID)["The General (1926)/The General (1926).mkv"] != "tt0017925"; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the new movie was not found: %v", e.files(addon.ID))
		}
	}
	// The scan's listings are those of the share now: no scan follows.
	time.Sleep(10 * testEvery)
	if got := scans.Load(); got != 1 {
		t.Errorf("%d scans, want 1", got)
	}

	// A share that stops answering is unwatched, with why, on its folder.
	server.Close()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(testEvery) {
		folder, err := e.service.Folder(t.Context(), addon.ID)
		if err != nil {
			t.Fatal(err)
		}
		if folder.Unwatched == errUnreachable {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Unwatched %q, want %q", folder.Unwatched, errUnreachable)
		}
	}
}

// The files of discs a scan walks count in a folder's print, so that a
// poll sees them change: a disc image, a DVD's video files and a Blu-ray
// disc's playlists. A file a scan does not walk does not.
func TestFolderPrintCountsDiscFiles(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, c := range []struct{ rel, name string }{
		{"", "Box (2010).iso"},
		{"Nosferatu (1922)/VIDEO_TS", "VTS_01_1.VOB"},
		{"The General (1926)/BDMV/PLAYLIST", "00000.mpls"},
	} {
		file := remoteEntry{name: c.name, size: 10, modified: at}
		grown := file
		grown.size = 11
		print := folderPrint(KindMovies, c.rel, []remoteEntry{file})
		if print == folderPrint(KindMovies, c.rel, nil) || print == folderPrint(KindMovies, c.rel, []remoteEntry{grown}) {
			t.Errorf("%s/%s does not count in its folder's print", c.rel, c.name)
		}
	}
	if folderPrint(KindMovies, "", []remoteEntry{{name: "notes.txt", size: 10, modified: at}}) != folderPrint(KindMovies, "", nil) {
		t.Error("a file a scan does not read counts in its folder's print")
	}
}

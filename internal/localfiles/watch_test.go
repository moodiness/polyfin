package localfiles

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// testQuiet is how long the folders of the tests stay unchanged before
// they are scanned.
const testQuiet = 100 * time.Millisecond

// watchTest runs a watcher whose scans are only told.
type watchTest struct {
	t     *testing.T
	w     *watcher
	wg    *sync.WaitGroup
	scans chan accounts.ID
}

func newWatchTest(t *testing.T, folders func(context.Context) ([]watchFolder, error), open func() (fileWatcher, error)) *watchTest {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	scans := make(chan accounts.ID, 64)
	w := newWatcher(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), folders,
		func(_ context.Context, id accounts.ID) { scans <- id }, open, wg.Go)
	w.quiet, w.check = testQuiet, time.Hour
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
	return &watchTest{t: t, w: w, wg: wg, scans: scans}
}

// start watches the folders, then handles the events in the background.
func (wt *watchTest) start() {
	wt.w.reconcile()
	wt.wg.Go(wt.w.loop)
}

// scanned waits for the scan of a folder.
func (wt *watchTest) scanned(id accounts.ID) {
	wt.t.Helper()
	select {
	case got := <-wt.scans:
		if got != id {
			wt.t.Fatalf("scanned %v, want %v", got, id)
		}
	case <-time.After(5 * time.Second):
		wt.t.Fatal("the folder was not scanned")
	}
}

// still checks that no scan follows for a few debounces.
func (wt *watchTest) still() {
	wt.t.Helper()
	select {
	case id := <-wt.scans:
		wt.t.Fatalf("%v was scanned again", id)
	case <-time.After(3 * testQuiet):
	}
}

// one lists a folder.
func one(id accounts.ID, root string) func(context.Context) ([]watchFolder, error) {
	return func(context.Context) ([]watchFolder, error) { return []watchFolder{{id: id, root: root}}, nil }
}

func testID(b byte) accounts.ID { return accounts.ID{15: b} }

func TestWatchScansAfterFilesAdded(t *testing.T) {
	root := t.TempDir()
	id := testID(1)
	wt := newWatchTest(t, one(id, root), openNotifyWatcher)
	wt.start()
	// A file at the root and a new folder holding another: one scan.
	write(t, filepath.Join(root, "Night of the Living Dead (1968).mkv"), 10)
	write(t, filepath.Join(root, "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	wt.scanned(id)
	wt.still()
	// The new folder is watched.
	write(t, filepath.Join(root, "Nosferatu (1922)", "Nosferatu (1922) - 720p.mkv"), 10)
	wt.scanned(id)
	wt.still()
}

func TestWatchScansOnceAfterBurst(t *testing.T) {
	root := t.TempDir()
	id := testID(1)
	fake := newFakeWatcher()
	wt := newWatchTest(t, one(id, root), func() (fileWatcher, error) { return fake, nil })
	wt.start()
	real, _ := filepath.EvalSymlinks(root)
	for i := range 50 {
		op := fsnotify.Write
		if i == 0 {
			op = fsnotify.Create
		}
		fake.events <- fsnotify.Event{Name: filepath.Join(real, "The General (1926).mkv"), Op: op}
	}
	wt.scanned(id)
	wt.still()
}

func TestWatchScansAfterRemoveAndRename(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "The General (1926).mkv"), 10)
	write(t, filepath.Join(root, "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(root, "The Gold Rush (1925)", "The Gold Rush (1925).mkv"), 10)
	id := testID(1)
	wt := newWatchTest(t, one(id, root), openNotifyWatcher)
	wt.start()

	if err := os.Remove(filepath.Join(root, "The General (1926).mkv")); err != nil {
		t.Fatal(err)
	}
	wt.scanned(id)
	wt.still()

	if err := os.Rename(filepath.Join(root, "Nosferatu (1922)", "Nosferatu (1922).mkv"),
		filepath.Join(root, "Nosferatu (1922)", "Nosferatu (1922) - 1080p.mkv")); err != nil {
		t.Fatal(err)
	}
	wt.scanned(id)
	wt.still()

	// A folder renamed is watched under its new name.
	if err := os.Rename(filepath.Join(root, "The Gold Rush (1925)"), filepath.Join(root, "The Gold Rush (1925) {imdb-tt0015864}")); err != nil {
		t.Fatal(err)
	}
	wt.scanned(id)
	wt.still()
	write(t, filepath.Join(root, "The Gold Rush (1925) {imdb-tt0015864}", "The Gold Rush (1925) - 720p.mkv"), 10)
	wt.scanned(id)

	if err := os.RemoveAll(filepath.Join(root, "The Gold Rush (1925) {imdb-tt0015864}")); err != nil {
		t.Fatal(err)
	}
	wt.scanned(id)
	wt.still()
}

func TestWatchIgnoresOtherFiles(t *testing.T) {
	root := t.TempDir()
	id := testID(1)
	wt := newWatchTest(t, one(id, root), openNotifyWatcher)
	wt.start()
	write(t, filepath.Join(root, "notes.txt"), 10)
	write(t, filepath.Join(root, "Extras", "Making of.mkv"), 10)
	write(t, filepath.Join(root, ".hidden", "The General (1926).mkv"), 10)
	wt.still()
}

func TestWatchFolderBack(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "movies")
	write(t, filepath.Join(root, "The General (1926).mkv"), 10)
	id := testID(1)
	wt := newWatchTest(t, one(id, root), openNotifyWatcher)
	wt.start()

	// Gone, it is scanned, which reports it missing.
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	wt.scanned(id)
	wt.still()

	// Back, it is scanned and watched again.
	write(t, filepath.Join(root, "Nosferatu (1922).mkv"), 10)
	wt.w.kick()
	wt.scanned(id)
	wt.still()
	write(t, filepath.Join(root, "The Gold Rush (1925).mkv"), 10)
	wt.scanned(id)
}

func TestWatchable(t *testing.T) {
	on := accounts.DefaultSettings()
	if !on.WatchLocalFolders {
		t.Fatal("folders are not watched by default")
	}
	local := addons.Addon{ID: testID(1), Kind: addons.KindLocal, ManifestURL: "/media/movies", Enabled: true}
	installed := []addons.Addon{
		local,
		{ID: testID(2), Kind: addons.KindLocal, ManifestURL: "smb://nas.local/media/movies", Enabled: true},
		{ID: testID(3), Kind: addons.KindLocal, ManifestURL: "https://nas.local/dav/shows", Enabled: true},
		{ID: testID(4), Kind: addons.KindLocal, ManifestURL: "/media/off", Enabled: false},
		{ID: testID(5), Kind: addons.KindStremio, ManifestURL: "https://addon.example/manifest.json", Enabled: true},
	}
	if got := watchable(on, installed); !slices.Equal(got, []watchFolder{{id: local.ID, root: local.ManifestURL}}) {
		t.Errorf("watched %v, want the local folder only", got)
	}
	off := on
	off.WatchLocalFolders = false
	if got := watchable(off, installed); len(got) != 0 {
		t.Errorf("watched %v with the setting off", got)
	}
}

func TestWatchNothingWhenOff(t *testing.T) {
	root := t.TempDir()
	settings := accounts.DefaultSettings()
	settings.WatchLocalFolders = false
	installed := []addons.Addon{
		{ID: testID(1), Kind: addons.KindLocal, ManifestURL: root, Enabled: true},
		{ID: testID(2), Kind: addons.KindLocal, ManifestURL: "smb://nas.local/media", Enabled: true},
	}
	var opened atomic.Int32
	wt := newWatchTest(t, func(context.Context) ([]watchFolder, error) { return watchable(settings, installed), nil },
		func() (fileWatcher, error) {
			opened.Add(1)
			return openNotifyWatcher()
		})
	wt.start()
	write(t, filepath.Join(root, "The General (1926).mkv"), 10)
	wt.still()
	if opened.Load() != 0 {
		t.Error("a watcher runs with the setting off")
	}
}

func TestWatchSharesNever(t *testing.T) {
	settings := accounts.DefaultSettings()
	installed := []addons.Addon{
		{ID: testID(1), Kind: addons.KindLocal, ManifestURL: "smb://nas.local/media", Enabled: true},
		{ID: testID(2), Kind: addons.KindLocal, ManifestURL: "http://nas.local/dav", Enabled: true},
	}
	var opened atomic.Int32
	wt := newWatchTest(t, func(context.Context) ([]watchFolder, error) { return watchable(settings, installed), nil },
		func() (fileWatcher, error) {
			opened.Add(1)
			return newFakeWatcher(), nil
		})
	wt.w.reconcile()
	if opened.Load() != 0 || len(wt.w.dirs) != 0 {
		t.Error("a network share is watched")
	}
}

func TestWatchLimit(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	fake := newFakeWatcher()
	fake.limit = 1
	wt := newWatchTest(t, one(id, root), func() (fileWatcher, error) { return fake, nil })
	wt.w.retry = time.Hour

	wt.w.reconcile()
	if got := wt.w.reason(id); got != unwatchedWatchLimit {
		t.Fatalf("unwatched %q, want %q", got, unwatchedWatchLimit)
	}
	if len(fake.watched()) != 0 || len(wt.w.dirs) != 0 {
		t.Errorf("watches %v kept for a folder not watched", fake.watched())
	}

	// Tried again only after the retry delay.
	fake.limit = 0
	wt.w.reconcile()
	if wt.w.reason(id) == "" {
		t.Fatal("tried again before the retry delay")
	}
	wt.w.retry = 0
	wt.w.reconcile()
	if got := wt.w.reason(id); got != "" {
		t.Errorf("unwatched %q once the limit was raised", got)
	}
	if len(fake.watched()) != 2 {
		t.Errorf("watches %v, want the folder and its subfolder", fake.watched())
	}
}

func TestWatchFailureReasons(t *testing.T) {
	root := t.TempDir()
	id := testID(1)
	for _, c := range []struct {
		err  error
		want string
	}{
		{syscall.ENOSPC, unwatchedWatchLimit},
		{syscall.EMFILE, unwatchedInstanceLimit},
		{syscall.EACCES, unwatchedFailed},
	} {
		wt := newWatchTest(t, one(id, root), func() (fileWatcher, error) { return nil, c.err })
		wt.w.reconcile()
		if got := wt.w.reason(id); got != c.want {
			t.Errorf("%v: unwatched %q, want %q", c.err, got, c.want)
		}
	}
}

func TestWatchLimitKeepsSchedule(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, map[string]int{"Nosferatu (1922)/Nosferatu (1922).mkv": 10})
	fake := newFakeWatcher()
	fake.limit = 1
	w := newWatcher(e.service.ctx, e.service.logger, e.service.watchable, e.service.watchScan,
		func() (fileWatcher, error) { return fake, nil }, e.service.wg.Go)
	e.service.watch = w
	w.reconcile()

	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Unwatched != unwatchedWatchLimit {
		t.Fatalf("Unwatched %q, want %q", folder.Unwatched, unwatchedWatchLimit)
	}

	// The schedule still scans the folder.
	write(t, filepath.Join(dir, "The General (1926).mkv"), 10)
	if _, err := e.pool.Exec(t.Context(), "UPDATE local_folders SET checked_at = now() - interval '7 hours' WHERE addon_id = $1", addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.service.ScanDue(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["The General (1926).mkv"]; got != "tt0017925" {
		t.Errorf("The General: %q after the scheduled scan", got)
	}
}

func TestWatchFindsNewFile(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, map[string]int{"Nosferatu (1922).mkv": 10})
	w := newWatcher(e.service.ctx, e.service.logger, e.service.watchable, e.service.watchScan, openNotifyWatcher, e.service.wg.Go)
	w.quiet = testQuiet
	e.service.watch = w
	w.reconcile()
	e.service.wg.Go(w.loop)

	write(t, filepath.Join(dir, "The General (1926)", "The General (1926).mkv"), 10)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if e.files(addon.ID)["The General (1926)/The General (1926).mkv"] == "tt0017925" {
			return
		}
	}
	t.Fatalf("the new file was not found: %v", e.files(addon.ID))
}

// fakeWatcher is a fileWatcher whose events a test sends; past limit
// watches, it fails as inotify does.
type fakeWatcher struct {
	events chan fsnotify.Event
	errors chan error
	limit  int

	mu    sync.Mutex
	added map[string]bool
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{events: make(chan fsnotify.Event), errors: make(chan error), added: map[string]bool{}}
}

func (f *fakeWatcher) Add(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.limit > 0 && len(f.added) >= f.limit {
		return syscall.ENOSPC
	}
	f.added[path] = true
	return nil
}

func (f *fakeWatcher) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.added, path)
	return nil
}

func (f *fakeWatcher) watched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var paths []string
	for path := range f.added {
		paths = append(paths, path)
	}
	return paths
}

func (f *fakeWatcher) Events() <-chan fsnotify.Event { return f.events }
func (f *fakeWatcher) Errors() <-chan error          { return f.errors }
func (f *fakeWatcher) Close() error                  { return nil }

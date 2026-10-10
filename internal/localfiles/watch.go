package localfiles

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// How folders are watched: a folder is scanned again watchQuiet after its
// files stop changing; the folders to watch are looked for every
// watchCheck, and a folder that could not be watched is tried again
// watchRetry later.
const (
	watchQuiet = 5 * time.Second
	watchCheck = 30 * time.Second
	watchRetry = ScanCheck
)

// Why a folder to watch is not, its Unwatched: the system's limit of
// watches is reached (ENOSPC on Linux, fs.inotify.max_user_watches), its
// limit of watchers or of open files (EMFILE, fs.inotify.max_user_instances
// on Linux), or another failure.
const (
	unwatchedWatchLimit    = "watch_limit"
	unwatchedInstanceLimit = "instance_limit"
	unwatchedFailed        = "failed"
)

// fileWatcher watches folders for changes: fsnotify's watcher, inotify on
// Linux, which tests replace.
type fileWatcher interface {
	Add(path string) error
	Remove(path string) error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Close() error
}

// notifyWatcher is a fileWatcher of fsnotify.
type notifyWatcher struct{ w *fsnotify.Watcher }

func openNotifyWatcher() (fileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return notifyWatcher{w}, nil
}

func (n notifyWatcher) Add(path string) error         { return n.w.Add(path) }
func (n notifyWatcher) Remove(path string) error      { return n.w.Remove(path) }
func (n notifyWatcher) Events() <-chan fsnotify.Event { return n.w.Events }
func (n notifyWatcher) Errors() <-chan error          { return n.w.Errors }
func (n notifyWatcher) Close() error                  { return n.w.Close() }

// watchFolder is a folder to watch: its addon and its path in the
// container.
type watchFolder struct {
	id   accounts.ID
	root string
}

// watchable are the folders to watch among the server's addons: the
// enabled local folders in the container, none when the setting is off.
// Network shares are never watched: they keep their schedule.
func watchable(settings accounts.Settings, installed []addons.Addon) []watchFolder {
	if !settings.WatchLocalFolders {
		return nil
	}
	var folders []watchFolder
	for _, addon := range installed {
		if addon.Local() && addon.Enabled && shareKind(addon.ManifestURL) == "" {
			folders = append(folders, watchFolder{id: addon.ID, root: addon.ManifestURL})
		}
	}
	return folders
}

// watching is how the watch of a folder goes: its path, and the same with
// its symbolic links resolved, as events name it; whether it is watched;
// whether it went missing, so that it is scanned when it is back; and when
// it last could not be watched.
type watching struct {
	root     string
	real     string
	on       bool
	lost     bool
	failedAt time.Time
}

// watcher watches the folders folders lists, each folder of their trees
// but those a scan leaves out, and scans a folder quiet after its video
// files or folders stop changing. It looks for the folders to watch every
// check, and when kicked; a folder it could not watch is tried again after
// retry, and keeps its schedule meanwhile.
type watcher struct {
	// ctx ends the watch.
	ctx     context.Context
	logger  *slog.Logger
	folders func(ctx context.Context) ([]watchFolder, error)
	scan    func(ctx context.Context, id accounts.ID)
	open    func() (fileWatcher, error)
	// spawn runs a scan in the background: Service.wg.Go.
	spawn func(func())

	quiet, check, retry time.Duration

	kicks chan struct{}
	due   chan accounts.ID
	done  chan accounts.ID

	mu sync.Mutex
	// unwatched are why the folders to watch that could not be are not,
	// by folder.
	unwatched map[accounts.ID]string

	// What follows belongs to the loop. files is nil while nothing is
	// watched; dirs are the folders watched, by path, with their folder;
	// changed the last change of the folders waiting for their scan;
	// scanning the folders a scan of the watch is under way for, again
	// those that changed meanwhile.
	files    fileWatcher
	watched  map[accounts.ID]*watching
	dirs     map[string]accounts.ID
	changed  map[accounts.ID]time.Time
	scanning map[accounts.ID]bool
	again    map[accounts.ID]bool
}

func newWatcher(ctx context.Context, logger *slog.Logger, folders func(context.Context) ([]watchFolder, error), scan func(context.Context, accounts.ID),
	open func() (fileWatcher, error), spawn func(func())) *watcher {
	return &watcher{ctx: ctx, logger: logger, folders: folders, scan: scan, open: open, spawn: spawn,
		quiet: watchQuiet, check: watchCheck, retry: watchRetry,
		kicks: make(chan struct{}, 1), due: make(chan accounts.ID), done: make(chan accounts.ID),
		unwatched: map[accounts.ID]string{}, watched: map[accounts.ID]*watching{}, dirs: map[string]accounts.ID{},
		changed: map[accounts.ID]time.Time{}, scanning: map[accounts.ID]bool{}, again: map[accounts.ID]bool{}}
}

// run watches the folders until the watch ends.
func (w *watcher) run() {
	w.reconcile()
	w.loop()
}

// loop handles the events, the scans due and the checks until the watch
// ends, then stops watching.
func (w *watcher) loop() {
	defer w.close()
	ticker := time.NewTicker(w.check)
	defer ticker.Stop()
	for {
		var events <-chan fsnotify.Event
		var failures <-chan error
		if w.files != nil {
			events, failures = w.files.Events(), w.files.Errors()
		}
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.reconcile()
		case <-w.kicks:
			w.reconcile()
		case event, ok := <-events:
			if !ok {
				w.close()
				continue
			}
			w.event(event)
		case err, ok := <-failures:
			if !ok {
				w.close()
				continue
			}
			w.failure(err)
		case id := <-w.due:
			w.settle(id)
		case id := <-w.done:
			delete(w.scanning, id)
			if w.again[id] {
				delete(w.again, id)
				w.startScan(id)
			}
		}
	}
}

// kick looks for the folders to watch at once, such as after one was
// added or moved.
func (w *watcher) kick() {
	if w == nil {
		return
	}
	select {
	case w.kicks <- struct{}{}:
	default:
	}
}

// reason tells why a folder to watch is not watched, empty when it is or
// need not be.
func (w *watcher) reason(id accounts.ID) string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.unwatched[id]
}

func (w *watcher) setReason(id accounts.ID, reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if reason == "" {
		delete(w.unwatched, id)
	} else {
		w.unwatched[id] = reason
	}
}

// reconcile watches the folders to watch not watched yet, those back after
// they went missing and those to try again, and stops watching the others;
// with none to watch, no watcher runs.
func (w *watcher) reconcile() {
	list, err := w.folders(w.ctx)
	if err != nil {
		if w.ctx.Err() == nil {
			w.logger.Warn("The local folders to watch could not be read", "error", err)
		}
		return
	}
	want := map[accounts.ID]string{}
	for _, folder := range list {
		want[folder.id] = folder.root
	}
	for id, state := range w.watched {
		if root, ok := want[id]; !ok || root != state.root {
			w.unwatch(id)
			delete(w.watched, id)
			delete(w.changed, id)
			w.setReason(id, "")
		}
	}
	if len(want) == 0 {
		w.close()
		return
	}
	for id, root := range want {
		if w.watched[id] == nil {
			w.watched[id] = &watching{root: root}
		}
	}
	now := time.Now()
	for id, root := range want {
		state := w.watched[id]
		if state.on || !state.failedAt.IsZero() && now.Sub(state.failedAt) < w.retry {
			continue
		}
		// A folder missing is reported by its scan; it is watched when it
		// is back.
		if readable(root) != "" {
			state.lost = true
			w.setReason(id, "")
			continue
		}
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			state.lost = true
			continue
		}
		if w.files == nil {
			if w.files, err = w.open(); err != nil {
				w.files = nil
				for id := range want {
					w.fail(id, err)
				}
				return
			}
		}
		state.real = real
		if !w.addTree(id, real) {
			continue
		}
		state.on, state.failedAt = true, time.Time{}
		w.setReason(id, "")
		if state.lost {
			state.lost = false
			w.change(id)
		}
	}
}

// addTree watches a folder and the folders under it a scan reads; a
// failure leaves the whole folder unwatched, and reports false.
func (w *watcher) addTree(id accounts.ID, top string) bool {
	err := filepath.WalkDir(top, func(path string, d fs.DirEntry, err error) error {
		// A folder gone meanwhile, or that cannot be listed, is skipped,
		// as a scan skips it.
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != top && skippedFolder(d.Name()) {
			return fs.SkipDir
		}
		if err := w.files.Add(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipDir
			}
			return err
		}
		w.dirs[path] = id
		return nil
	})
	if err != nil {
		w.fail(id, err)
		return false
	}
	return true
}

// fail leaves a folder unwatched after err, until it is tried again: it
// keeps its schedule.
func (w *watcher) fail(id accounts.ID, err error) {
	w.unwatch(id)
	state := w.watched[id]
	state.on, state.failedAt = false, time.Now()
	reason := unwatchedFailed
	switch {
	case errors.Is(err, syscall.ENOSPC):
		reason = unwatchedWatchLimit
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		reason = unwatchedInstanceLimit
	}
	w.setReason(id, reason)
	w.logger.Warn("A local folder cannot be watched for changes; it is scanned on its schedule", "path", state.root, "reason", reason,
		"error", err)
}

// unwatch stops watching the folders of a folder's tree.
func (w *watcher) unwatch(id accounts.ID) {
	for path, owner := range w.dirs {
		if owner == id {
			w.forget(path)
		}
	}
}

// drop stops watching a folder gone and those under it.
func (w *watcher) drop(dir string) {
	prefix := dir + string(filepath.Separator)
	for path := range w.dirs {
		if path == dir || strings.HasPrefix(path, prefix) {
			w.forget(path)
		}
	}
}

func (w *watcher) forget(path string) {
	delete(w.dirs, path)
	if w.files != nil {
		// A folder gone is no longer watched already.
		_ = w.files.Remove(path)
	}
}

// close stops watching every folder; they are watched again at the next
// check when there are some to watch.
func (w *watcher) close() {
	if w.files != nil {
		_ = w.files.Close()
		w.files = nil
	}
	clear(w.dirs)
	for _, state := range w.watched {
		state.on = false
	}
}

// event handles a change: a folder added is watched, a folder watched gone
// is no longer, and a change of a folder or of a video file schedules the
// scan of its folder.
func (w *watcher) event(event fsnotify.Event) {
	name := filepath.Clean(event.Name)
	if id, ok := w.dirs[name]; ok {
		if !event.Has(fsnotify.Remove) && !event.Has(fsnotify.Rename) {
			return
		}
		w.drop(name)
		if state := w.watched[id]; state != nil && name == state.real {
			state.on, state.lost = false, true
		}
		w.change(id)
		return
	}
	id, ok := w.dirs[filepath.Dir(name)]
	if !ok {
		return
	}
	base := filepath.Base(name)
	if event.Has(fsnotify.Create) {
		if info, err := os.Lstat(name); err == nil && info.IsDir() {
			if !skippedFolder(base) && w.addTree(id, name) {
				w.change(id)
			}
			return
		}
	}
	if videoFile(base) && event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
		w.change(id)
	}
}

// failure handles a watcher's error: events lost, the folders watched are
// all scanned again.
func (w *watcher) failure(err error) {
	if !errors.Is(err, fsnotify.ErrEventOverflow) {
		w.logger.Warn("Watching the local folders failed", "error", err)
		return
	}
	for id, state := range w.watched {
		if state.on {
			w.change(id)
		}
	}
}

// change schedules the scan of a folder quiet after its last change.
func (w *watcher) change(id accounts.ID) {
	if _, waiting := w.changed[id]; !waiting {
		w.arm(id, w.quiet)
	}
	w.changed[id] = time.Now()
}

func (w *watcher) arm(id accounts.ID, after time.Duration) {
	time.AfterFunc(after, func() {
		select {
		case w.due <- id:
		case <-w.ctx.Done():
		}
	})
}

// settle scans a folder whose files stopped changing, or waits for them
// to.
func (w *watcher) settle(id accounts.ID) {
	last, ok := w.changed[id]
	if !ok {
		return
	}
	if wait := w.quiet - time.Since(last); wait > 0 {
		w.arm(id, wait)
		return
	}
	delete(w.changed, id)
	w.startScan(id)
}

// startScan scans a folder in the background, or once the scan under way
// ends.
func (w *watcher) startScan(id accounts.ID) {
	if w.scanning[id] {
		w.again[id] = true
		return
	}
	w.scanning[id] = true
	w.spawn(func() {
		w.scan(w.ctx, id)
		select {
		case w.done <- id:
		case <-w.ctx.Done():
		}
	})
}

package localfiles

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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

// Why a folder to watch is not, its Unwatched: on Linux, the system's
// limit of watches is reached (ENOSPC, fs.inotify.max_user_watches), or its
// limit of watchers or of open files (EMFILE, fs.inotify.max_user_instances);
// on macOS, which watches each folder and file through a file of its own,
// its limit of open files (EMFILE, kern.maxfilesperproc); or another
// failure, the only one on Windows, which has no such limits (see
// unwatchedReason).
const (
	unwatchedWatchLimit     = "watch_limit"
	unwatchedInstanceLimit  = "instance_limit"
	unwatchedOpenFilesLimit = "open_files_limit"
	unwatchedFailed         = "failed"
)

// fileWatcher watches folders for changes: fsnotify's watcher, which tests
// replace. fsnotify watches through inotify on Linux, kqueue on macOS and
// ReadDirectoryChangesW on Windows.
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

// watchFolder is a folder to watch: its addon, its path in the container
// or a share's address, and its kind, KindMusic for a music folder, else
// empty: a watch reads the folders of movies and shows alike. every is
// how often a share is listed again, 0 for shareEvery (see sharewatch.go).
// follow tells whether a folder in the container follows the symbolic
// links leading out of it: the folders they lead to are watched too.
type watchFolder struct {
	id     accounts.ID
	root   string
	kind   string
	every  time.Duration
	follow bool
}

// watchable are the folders to watch among the server's addons: the
// enabled local folders, in the container or network shares, none when
// the setting is off. polls are how often, in minutes, the shares whose
// kind has a setting of it are listed again, by folder: those listed only
// at their scans, 0, are not watched.
func watchable(settings accounts.Settings, installed []addons.Addon, polls map[accounts.ID]int) []watchFolder {
	if !settings.WatchLocalFolders {
		return nil
	}
	var folders []watchFolder
	for _, addon := range installed {
		if addon.Local() && addon.Enabled {
			folder := watchFolder{id: addon.ID, root: addon.ManifestURL}
			if addon.HasMusic() {
				folder.kind = KindMusic
			}
			if minutes, ok := polls[addon.ID]; ok {
				if minutes == 0 {
					continue
				}
				folder.every = time.Duration(minutes) * time.Minute
			}
			folders = append(folders, folder)
		}
	}
	return folders
}

// watching is how the watch of a folder goes: its path, and the same with
// its symbolic links resolved, as events name it; its kind and whether it
// follows the links leading out of it (see watchFolder); whether it is
// watched; whether it went missing, so that it is scanned when it is back;
// and when it last could not be watched.
type watching struct {
	root     string
	real     string
	kind     string
	follow   bool
	on       bool
	lost     bool
	failedAt time.Time
}

// watcher watches the folders folders lists, each folder of their trees
// but those a scan leaves out, and scans a folder quiet after its video
// files, side files or folders stop changing. It looks for the folders to
// watch every check, and when kicked; a folder it could not watch is tried
// again after retry, and keeps its schedule meanwhile. It polls the
// network shares among them instead, read through shares (see poll).
type watcher struct {
	// ctx ends the watch.
	ctx     context.Context
	logger  *slog.Logger
	folders func(ctx context.Context) ([]watchFolder, error)
	scan    func(ctx context.Context, id accounts.ID)
	open    func() (fileWatcher, error)
	shares  shareSource
	// spawn runs a scan in the background: Service.wg.Go.
	spawn func(func())

	quiet, check, retry time.Duration
	// rewatch has the folders created in a tree watched again as its scan
	// begins: rewatchCreated, which tests set.
	rewatch bool
	// every, notifiedEvery, rearm, gap and lists pace the polls and the
	// watches of the shares (see shareEvery).
	every, notifiedEvery, rearm, gap time.Duration
	lists                            int

	kicks chan struct{}
	due   chan accounts.ID
	done  chan accounts.ID
	// polled are the shares a poll found changed, notified those whose
	// server told of a change.
	polled   chan sharePolled
	notified chan accounts.ID

	mu sync.Mutex
	// unwatched are why the folders to watch that could not be are not,
	// by folder; shareWatches how the shares are watched.
	unwatched    map[accounts.ID]string
	shareWatches map[accounts.ID]shareWatchState

	// What follows belongs to the loop. files is nil while nothing is
	// watched; dirs are the folders watched, by path, with the folders
	// whose trees hold them, or whose links lead to them; created the
	// folders created in a folder's tree since its last scan began, when
	// they are watched again (see rewatch); changed the last change of the
	// folders waiting for their scan; scanning the folders a scan of the
	// watch is under way for, again those that changed meanwhile; polls
	// the shares polled.
	files    fileWatcher
	watched  map[accounts.ID]*watching
	dirs     map[string][]accounts.ID
	created  map[accounts.ID][]string
	changed  map[accounts.ID]time.Time
	scanning map[accounts.ID]bool
	again    map[accounts.ID]bool
	polls    map[accounts.ID]*polling
}

func newWatcher(ctx context.Context, logger *slog.Logger, folders func(context.Context) ([]watchFolder, error), scan func(context.Context, accounts.ID),
	open func() (fileWatcher, error), shares shareSource, spawn func(func())) *watcher {
	return &watcher{ctx: ctx, logger: logger, folders: folders, scan: scan, open: open, shares: shares, spawn: spawn,
		quiet: watchQuiet, check: watchCheck, retry: watchRetry, rewatch: rewatchCreated, every: shareEvery, notifiedEvery: shareNotifiedEvery,
		rearm: shareRearm, gap: shareGap, lists: shareLists,
		kicks: make(chan struct{}, 1), due: make(chan accounts.ID), done: make(chan accounts.ID), polled: make(chan sharePolled),
		notified: make(chan accounts.ID), unwatched: map[accounts.ID]string{}, shareWatches: map[accounts.ID]shareWatchState{},
		watched: map[accounts.ID]*watching{}, dirs: map[string][]accounts.ID{}, created: map[accounts.ID][]string{},
		changed: map[accounts.ID]time.Time{}, scanning: map[accounts.ID]bool{}, again: map[accounts.ID]bool{}, polls: map[accounts.ID]*polling{}}
}

// run watches the folders until the watch ends.
func (w *watcher) run() {
	w.guard(w.reconcile)
	w.loop()
}

// guard runs a step of the watch, of what reads the folders and the
// shares: a panic of it is logged with its stack, and the watch goes on
// with its next event, check, round or scan.
func (w *watcher) guard(step func()) {
	defer logPanic(w.logger, "A step of the folders' watch ended on a panic")
	step()
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
			w.guard(w.reconcile)
		case <-w.kicks:
			w.guard(w.reconcile)
		case event, ok := <-events:
			if !ok {
				w.close()
				continue
			}
			w.guard(func() { w.event(event) })
		case id := <-w.notified:
			if w.polls[id] != nil {
				w.guard(func() { w.change(id) })
			}
		case err, ok := <-failures:
			if !ok {
				w.close()
				continue
			}
			w.guard(func() { w.failure(err) })
		case id := <-w.due:
			w.guard(func() { w.settle(id) })
		case poll := <-w.polled:
			w.guard(func() { w.polledShare(poll) })
		case id := <-w.done:
			delete(w.scanning, id)
			if w.again[id] {
				delete(w.again, id)
				w.guard(func() { w.startScan(id) })
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
// with none in the container to watch, no watcher runs. The shares to
// watch are polled, and those no longer to watch are not.
func (w *watcher) reconcile() {
	list, err := w.folders(w.ctx)
	if err != nil {
		if w.ctx.Err() == nil {
			w.logger.Warn("The local folders to watch could not be read", "error", err)
		}
		return
	}
	want := map[accounts.ID]watchFolder{}
	shares := map[accounts.ID]watchFolder{}
	for _, folder := range list {
		if shareKind(folder.root) != "" {
			shares[folder.id] = folder
		} else {
			want[folder.id] = folder
		}
	}
	w.pollShares(shares)
	for id, state := range w.watched {
		if folder, ok := want[id]; !ok || folder.root != state.root || folder.kind != state.kind || folder.follow != state.follow {
			w.unwatch(id)
			delete(w.watched, id)
			delete(w.changed, id)
			delete(w.created, id)
			w.setReason(id, "")
		}
	}
	if len(want) == 0 {
		w.close()
		return
	}
	for id, folder := range want {
		if w.watched[id] == nil {
			w.watched[id] = &watching{root: folder.root, kind: folder.kind, follow: folder.follow}
		}
	}
	now := time.Now()
	for id, folder := range want {
		root := folder.root
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

// addTree watches a folder and the folders under it a scan reads, and
// when its folder follows the links leading out of it, the folders they
// lead to (see linkedDir), each once; a failure leaves the whole folder
// unwatched, and reports false.
func (w *watcher) addTree(id accounts.ID, top string) bool {
	state := w.watched[id]
	var seen map[string]bool
	if state.follow {
		seen = map[string]bool{}
	}
	if err := w.addDirs(id, state, top, seen); err != nil {
		w.fail(id, err)
		return false
	}
	return true
}

// addDirs watches top and the folders under it a scan of the folder
// state watches reads, for the folder id; with seen, the folders watched
// so far, it follows the links leading out of the folder.
func (w *watcher) addDirs(id accounts.ID, state *watching, top string, seen map[string]bool) error {
	return filepath.WalkDir(top, func(path string, d fs.DirEntry, err error) error {
		// A folder gone meanwhile, or that cannot be listed, is skipped,
		// as a scan skips it.
		if err != nil {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if seen == nil || skippedIn(state.kind, d.Name()) {
				return nil
			}
			if target, ok := linkedDir(state.real, path); ok && !seen[target] {
				return w.addDirs(id, state, target, seen)
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != top && skippedIn(state.kind, d.Name()) {
			return fs.SkipDir
		}
		if seen != nil {
			if seen[path] {
				return fs.SkipDir
			}
			seen[path] = true
		}
		if err := w.files.Add(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipDir
			}
			return err
		}
		if !slices.Contains(w.dirs[path], id) {
			w.dirs[path] = append(w.dirs[path], id)
		}
		return nil
	})
}

// linkedDir is the real path of the folder the link at leads to, when a
// watch of the folder whose real path is real follows it: a folder out of
// it, and not one that holds it, which would lead back into it.
func linkedDir(real, at string) (string, bool) {
	target, err := filepath.EvalSymlinks(at)
	if err != nil || within(real, target) || within(target, real) {
		return "", false
	}
	info, err := os.Stat(target)
	return target, err == nil && info.IsDir()
}

// fail leaves a folder unwatched after err, until it is tried again: it
// keeps its schedule.
func (w *watcher) fail(id accounts.ID, err error) {
	w.unwatch(id)
	state := w.watched[id]
	state.on, state.failedAt = false, time.Now()
	reason := unwatchedReason(err)
	w.setReason(id, reason)
	w.logger.Warn("A local folder cannot be watched for changes; it is scanned on its schedule", "path", state.root, "reason", reason,
		"error", err)
}

// unwatch stops watching the folders of a folder's tree, and those its
// links lead to, but those another folder's watch has too.
func (w *watcher) unwatch(id accounts.ID) {
	for path, owners := range w.dirs {
		if !slices.Contains(owners, id) {
			continue
		}
		if len(owners) == 1 {
			w.forget(path)
		} else {
			w.dirs[path] = slices.DeleteFunc(slices.Clone(owners), func(owner accounts.ID) bool { return owner == id })
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
// is no longer, and a change of a folder, or of a file its folder's scan
// lists (see walked), schedules the scan of its folder, of each folder
// whose watch has it.
func (w *watcher) event(event fsnotify.Event) {
	name := filepath.Clean(event.Name)
	if owners, ok := w.dirs[name]; ok {
		if !event.Has(fsnotify.Remove) && !event.Has(fsnotify.Rename) {
			return
		}
		w.drop(name)
		for _, id := range owners {
			if state := w.watched[id]; state != nil && name == state.real {
				state.on, state.lost = false, true
			}
			w.change(id)
		}
		return
	}
	// Watching a folder created changes w.dirs.
	for _, id := range slices.Clone(w.dirs[filepath.Dir(name)]) {
		w.entryEvent(id, name, event)
	}
}

// entryEvent handles a change of the entry name, of a folder the watch of
// the folder id has. A folder that follows the links leading out of it
// watches the folder a link created leads to, and is scanned again when an
// entry that may have been such a link goes.
func (w *watcher) entryEvent(id accounts.ID, name string, event fsnotify.Event) {
	base := filepath.Base(name)
	state := w.watched[id]
	if event.Has(fsnotify.Create) {
		info, err := os.Lstat(name)
		if err == nil && info.IsDir() {
			if !skippedIn(state.kind, base) && w.addTree(id, name) {
				if w.rewatch {
					w.created[id] = append(w.created[id], name)
				}
				w.change(id)
			}
			return
		}
		if err == nil && info.Mode()&fs.ModeSymlink != 0 && state.follow && !skippedIn(state.kind, base) {
			if _, ok := linkedDir(state.real, name); ok {
				if w.addTree(id, name) {
					w.change(id)
				}
				return
			}
		}
	}
	file, side := walked(state.kind, filepath.ToSlash(name))
	switch {
	case (file || side) && event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0:
		w.change(id)
	case state.follow && event.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
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

// polledShare has a share a poll found changed scanned again quiet later.
// A poll finds a share changed until a scan lists it again: a share
// waiting for its scan, or being scanned, is not made to wait longer; nor
// is one a scan kept other prints of since the poll read those it
// compared, as the next round compares with them. One no longer watched
// waits for none.
func (w *watcher) polledShare(poll sharePolled) {
	if _, waiting := w.changed[poll.id]; w.polls[poll.id] == nil || waiting || w.scanning[poll.id] ||
		w.shares.shareSeen(poll.id).kept != poll.kept {
		return
	}
	w.change(poll.id)
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
// ends. The folders created in its tree are watched again first (see
// rewatchCreated): the scan then lists what was written before.
func (w *watcher) startScan(id accounts.ID) {
	if w.scanning[id] {
		w.again[id] = true
		return
	}
	w.rewatchCreatedIn(id)
	w.scanning[id] = true
	w.spawn(func() {
		// A panic of the scan is logged; its end is told all the same, and
		// the folder is scanned again on its next change.
		w.guard(func() { w.scan(w.ctx, id) })
		select {
		case w.done <- id:
		case <-w.ctx.Done():
		}
	})
}

// rewatchCreatedIn watches again the folders created in a folder's tree
// since its last scan began, those still watched.
func (w *watcher) rewatchCreatedIn(id accounts.ID) {
	created := w.created[id]
	delete(w.created, id)
	for _, dir := range created {
		if slices.Contains(w.dirs[dir], id) && !w.addTree(id, dir) {
			return
		}
	}
}

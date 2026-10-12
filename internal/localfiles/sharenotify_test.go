package localfiles

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// fakeNotifier is an SMB share a test serves: a WebDAV share's reader
// whose watches the test drives. Each watch set up is sent on watches,
// once armed; fail, when set, ends each watch before it is armed.
type fakeNotifier struct {
	remote
	root    bool
	fail    error
	watches chan *fakeWatch
}

// fakeWatch is a watch of a fakeNotifier: how it was asked for; the
// changes to tell, none for STATUS_NOTIFY_ENUM_DIR; the error to end it
// with; and its end.
type fakeWatch struct {
	how   notifyHow
	tell  chan []string
	fail  chan error
	ended chan struct{}
}

func newFakeNotifier(r remote) *fakeNotifier {
	return &fakeNotifier{remote: r, watches: make(chan *fakeWatch, 8)}
}

func (f *fakeNotifier) splittable() bool { return f.root }

func (f *fakeNotifier) watch(ctx context.Context, how notifyHow) error {
	if f.fail != nil {
		return f.fail
	}
	fw := &fakeWatch{how: how, tell: make(chan []string), fail: make(chan error), ended: make(chan struct{})}
	defer close(fw.ended)
	how.armed()
	f.watches <- fw
	for {
		select {
		case <-ctx.Done():
			return nil
		case paths := <-fw.tell:
			how.changed(paths)
		case err := <-fw.fail:
			return err
		}
	}
}

// armed waits for the next watch of the share.
func (f *fakeNotifier) armed(t *testing.T) *fakeWatch {
	t.Helper()
	select {
	case fw := <-f.watches:
		return fw
	case <-time.After(5 * time.Second):
		t.Fatal("the share was not watched")
		return nil
	}
}

// testSMB is the address of the SMB shares of the tests, read by fakes.
const testSMB = "smb://nas.local/movies"

// newShareNotifyTest watches the shares folders lists, read through a
// fakeNotifier of a share of movies in dir, polled every hour while
// notified, and every testEvery otherwise.
func newShareNotifyTest(t *testing.T, dir string, folders func(context.Context) ([]watchFolder, error)) (*watchTest, *fakeShares,
	*fakeNotifier) {
	t.Helper()
	wt, shares, _ := newShareWatchTest(t, dir, func(string) func(context.Context) ([]watchFolder, error) { return folders })
	notifier := newFakeNotifier(shares.r)
	shares.use(notifier)
	wt.w.notifiedEvery, wt.w.rearm = time.Hour, time.Hour
	return wt, shares, notifier
}

// watchedAs waits for a share to be watched as how, polled for why.
func (wt *watchTest) watchedAs(id accounts.ID, how, why string) {
	wt.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(testEvery) {
		gotHow, gotWhy := wt.w.shareWatch(id)
		if gotHow == how && gotWhy == why {
			return
		}
		if time.Now().After(deadline) {
			wt.t.Fatalf("watched %q (%q), want %q (%q)", gotHow, gotWhy, how, why)
		}
	}
}

// A change the server tells of has the share scanned once, quiet after
// the last one; so does a change it could not tell (ENUM_DIR). A change
// in a folder a scan skips does not.
func TestShareNotifyScansAfterQuiet(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	wt, _, notifier := newShareNotifyTest(t, dir, one(id, testSMB))
	wt.start()
	fw := notifier.armed(t)
	if fw.how.split {
		t.Error("the share's tree is not watched whole")
	}
	wt.watchedAs(id, ShareNotified, "")

	start := time.Now()
	fw.tell <- []string{"Metropolis (1927)"}
	fw.tell <- []string{"Metropolis (1927)/Metropolis (1927).mkv"}
	wt.scanned(id)
	if waited := time.Since(start); waited < testQuiet {
		t.Errorf("scanned %v after the change, before the quiet period", waited)
	}
	wt.still()

	fw.tell <- nil
	wt.scanned(id)
	wt.still()

	fw.tell <- []string{".hidden/The General (1926).mkv", "@eaDir/Nosferatu (1922).mkv"}
	wt.still()
}

// A watch that ends after it was armed, as when the server ended the
// session, is set up again at once, and the share scanned: what changed
// meanwhile was not told. One that cannot be set up leaves the share
// polled, with why, and found changed by its polls.
func TestShareNotifyRearms(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	wt, _, notifier := newShareNotifyTest(t, dir, one(id, testSMB))
	wt.start()
	fw := notifier.armed(t)

	// A round found the share unreachable meanwhile: once watched again,
	// it answers.
	wt.w.setReason(id, errUnreachable)
	fw.fail <- errShareUnreachable
	again := notifier.armed(t)
	if reason := wt.w.reason(id); reason != "" {
		t.Errorf("unwatched %q once watched again", reason)
	}
	wt.scanned(id)
	wt.watchedAs(id, ShareNotified, "")
	again.tell <- []string{"Metropolis (1927)"}
	wt.scanned(id)
	wt.still()

	// The next failure, with the server down, waits rearm: the share is
	// polled meanwhile.
	again.fail <- errShareUnreachable
	write(t, filepath.Join(dir, "movies", "Metropolis (1927)", "Metropolis (1927).mkv"), 10)
	wt.scanned(id)
	wt.watchedAs(id, SharePolled, polledFailed)
}

// A server that refuses change notifications leaves the share polled.
func TestShareNotifyUnsupported(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	wt, _, notifier := newShareNotifyTest(t, dir, one(id, testSMB))
	notifier.fail = errNotifyUnsupported
	wt.start()
	write(t, filepath.Join(dir, "movies", "The General (1926).mkv"), 10)
	wt.scanned(id)
	wt.watchedAs(id, SharePolled, polledUnsupported)
}

// A watch whose server does not tell of a change of the share's root a
// poll found is let go: the share is scanned, and polled again every few
// minutes. Watching a share's root, the watch is split first.
func TestShareNotifySilentFallsBackToPolling(t *testing.T) {
	for _, root := range []bool{false, true} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
		id := testID(1)
		wt, shares, notifier := newShareNotifyTest(t, dir, one(id, testSMB))
		notifier.root = root
		wt.w.notifiedEvery = testEvery
		shares.read = make(chan struct{}, 1)
		wt.start()
		fw := notifier.armed(t)

		// A round looks at the watch, then lists the root: the second
		// read of the share after the watch was armed is by a round that
		// found the watch on.
		select {
		case <-shares.read:
		default:
		}
		for range 2 {
			select {
			case <-shares.read:
			case <-time.After(5 * time.Second):
				t.Fatal("the share was not polled")
			}
		}
		write(t, filepath.Join(dir, "movies", "Metropolis (1927)", "Metropolis (1927).mkv"), 10)
		wt.scanned(id)
		select {
		case <-fw.ended:
		case <-time.After(5 * time.Second):
			t.Fatal("the silent watch was kept")
		}
		if !root {
			wt.watchedAs(id, SharePolled, polledSilent)
			continue
		}
		split := notifier.armed(t)
		if !split.how.split {
			t.Error("the watch of the share's root was not split")
		}
		// The split watch's changes are told.
		split.tell <- []string{"Metropolis (1927)/Metropolis (1927).fr.srt"}
		wt.scanned(id)
	}
}

// A watch whose server does not tell of a change in a folder of the share
// a poll found, as Samba misses those made on its own disk, is kept, and
// the share listed again every few minutes too.
func TestShareNotifyMissedDeepChange(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	wt, shares, notifier := newShareNotifyTest(t, dir, one(id, testSMB))
	wt.w.notifiedEvery = testEvery
	shares.read = make(chan struct{}, 1)
	wt.start()
	fw := notifier.armed(t)
	select {
	case <-shares.read:
	default:
	}
	for range 2 {
		select {
		case <-shares.read:
		case <-time.After(5 * time.Second):
			t.Fatal("the share was not polled")
		}
	}
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).fr.srt"), 10)
	wt.scanned(id)
	wt.watchedAs(id, ShareNotified, notifiedPartial)
	if wt.w.interval(&sharePoll{notify: &shareNotify{on: true, partial: true}}) != wt.w.every {
		t.Error("a share whose server misses changes is not listed every few minutes")
	}
	select {
	case <-fw.ended:
		t.Fatal("the watch was let go")
	default:
	}
	// Its changes are still told.
	fw.tell <- []string{"Metropolis (1927)"}
	wt.scanned(id)
}

// A WebDAV share is polled, with why; turned off, the watches end, and
// no share is watched.
func TestShareNotifyWebDAVAndOff(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922).mkv"), 10)
	id := testID(1)
	wt, _, _ := newShareWatchTest(t, dir, oneShare(id))
	wt.start()
	write(t, filepath.Join(dir, "movies", "The General (1926).mkv"), 10)
	wt.scanned(id)
	wt.watchedAs(id, SharePolled, polledWebDAV)

	var off atomic.Bool
	installed := []addons.Addon{{ID: id, Kind: addons.KindLocal, ManifestURL: testSMB, Enabled: true}}
	wt, _, notifier := newShareNotifyTest(t, dir, func(context.Context) ([]watchFolder, error) {
		settings := accounts.DefaultSettings()
		settings.WatchLocalFolders = !off.Load()
		return watchable(settings, installed), nil
	})
	wt.start()
	fw := notifier.armed(t)
	off.Store(true)
	wt.w.kick()
	select {
	case <-fw.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the watch went on with the setting off")
	}
	wt.watchedAs(id, "", "")
	wt.still()
	if len(notifier.watches) != 0 {
		t.Error("the share was watched again with the setting off")
	}
}

// A change in a folder a scan skips is no change.
func TestRelevantChange(t *testing.T) {
	for _, c := range []struct {
		paths []string
		want  bool
	}{
		{nil, true},
		{[]string{"Metropolis (1927)"}, true},
		{[]string{".hidden/x.mkv"}, false},
		{[]string{"#recycle/Metropolis (1927)/Metropolis (1927).mkv", "Metropolis (1927)/Metropolis (1927).mkv"}, true},
		{[]string{"@eaDir"}, false},
	} {
		if got := relevantChange(KindMovies, c.paths); got != c.want {
			t.Errorf("%q: %v, want %v", c.paths, got, c.want)
		}
	}
}

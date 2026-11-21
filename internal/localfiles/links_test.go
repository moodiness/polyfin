package localfiles

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// symlink makes a symbolic link at link leading to target.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// realDir is a new temporary folder by its real path, as walks and
// watches name what they find.
func realDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// keptLogs keeps the messages logged, with their attributes.
type keptLogs struct {
	mu      sync.Mutex
	records []slog.Record
}

func (k *keptLogs) Enabled(context.Context, slog.Level) bool { return true }
func (k *keptLogs) WithAttrs([]slog.Attr) slog.Handler       { return k }
func (k *keptLogs) WithGroup(string) slog.Handler            { return k }

func (k *keptLogs) Handle(_ context.Context, r slog.Record) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.records = append(k.records, r.Clone())
	return nil
}

// with lists the records logged as msg, each by its attributes.
func (k *keptLogs) with(msg string) []map[string]string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var found []map[string]string
	for _, r := range k.records {
		if r.Message != msg {
			continue
		}
		attrs := map[string]string{}
		r.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.String()
			return true
		})
		found = append(found, attrs)
	}
	return found
}

const skippedLinksMessage = "A local folder's scan skipped symbolic links"

// By default, the links leading out of a folder, to a file or to a
// folder, are skipped; its page counts them, and the log tells them once
// per scan.
func TestLinksLeadingOutAreSkippedAndCounted(t *testing.T) {
	t.Parallel()
	logs := &keptLogs{}
	e := newEnvLogged(t, slog.New(logs), newMetaAddon(t))
	outside := t.TempDir()
	write(t, filepath.Join(outside, "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(outside, "Shelf", "The General (1926).mkv"), 10)
	addon, dir := e.folder(KindMovies, map[string]int{"Night of the Living Dead (1968).mkv": 10})
	if got := logs.with(skippedLinksMessage); len(got) != 0 {
		t.Fatalf("logged before any link: %v", got)
	}
	symlink(t, filepath.Join(outside, "Nosferatu (1922).mkv"), filepath.Join(dir, "Nosferatu (1922).mkv"))
	symlink(t, filepath.Join(outside, "Shelf"), filepath.Join(dir, "Shelf"))
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := e.files(addon.ID), map[string]string{"Night of the Living Dead (1968).mkv": "tt0063350"}; !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if folder.FollowLinks || folder.SkippedLinks != (LinkCounts{Out: 2}) {
		t.Errorf("follow %v, skipped %+v", folder.FollowLinks, folder.SkippedLinks)
	}
	logged := logs.with(skippedLinksMessage)
	if len(logged) != 1 || logged[0]["leading_out"] != "2" || logged[0]["follow_links"] != "false" {
		t.Errorf("logged: %v", logged)
	}
}

// With the option on, the links leading out of a folder are followed, to
// files and through folders; the files are served while the option stays
// on, which is read again at each request.
func TestLinksLeadingOutAreFollowedWhenAsked(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(outside, "Shelf", "The General (1926)", "The General (1926).mkv"), 20)
	addon, dir := e.folder(KindMovies, map[string]int{"Night of the Living Dead (1968).mkv": 10})
	symlink(t, filepath.Join(outside, "Nosferatu (1922).mkv"), filepath.Join(dir, "Nosferatu (1922).mkv"))
	symlink(t, filepath.Join(outside, "Shelf"), filepath.Join(dir, "Shelf"))
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{FollowLinks: new(true)}); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Night of the Living Dead (1968).mkv":             "tt0063350",
		"Nosferatu (1922).mkv":                            "tt0013442",
		"Shelf/The General (1926)/The General (1926).mkv": "tt0017925",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !folder.FollowLinks || folder.SkippedLinks != (LinkCounts{}) {
		t.Errorf("follow %v, skipped %+v", folder.FollowLinks, folder.SkippedLinks)
	}
	key := e.fileKey(addon.ID, "Shelf/The General (1926)/The General (1926).mkv")
	file, _, err := e.service.Open(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.(*os.File).Stat()
	file.Close()
	if err != nil || info.Size() != 20 {
		t.Errorf("served %v, %v", info, err)
	}
	// The option off, the file is served no more, before any scan.
	if _, err := e.pool.Exec(t.Context(), "UPDATE local_folders SET follow_links = false WHERE addon_id = $1", addon.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.service.Open(t.Context(), key); err != ErrGone {
		t.Errorf("served with the option off: %v", err)
	}
}

// A link is never followed into another folder of the server that a user
// of the folder does not use: the scan skips and counts it, and a file
// found through it before is served no more once that folder's users
// change.
func TestLinksIntoFoldersOfOtherUsersAreFenced(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	other, otherDir := e.folder(KindMovies, map[string]int{"The Gold Rush (1925).mkv": 10})
	addon, dir := e.folder(KindMovies, map[string]int{"Night of the Living Dead (1968).mkv": 10})
	symlink(t, filepath.Join(otherDir, "The Gold Rush (1925).mkv"), filepath.Join(dir, "The Gold Rush (1925).mkv"))
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{FollowLinks: new(true)}); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	// Used by everyone, the other folder is reached by every user of this
	// one.
	if got := e.files(addon.ID)["The Gold Rush (1925).mkv"]; got != "tt0015864" {
		t.Fatalf("followed into a folder used by everyone: %q", got)
	}
	key := e.fileKey(addon.ID, "The Gold Rush (1925).mkv")
	file, _, err := e.service.Open(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := e.addons.SetUsedBy(t.Context(), other.ID, addons.UsedBy{Listed: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.service.Open(t.Context(), key); err != ErrGone {
		t.Errorf("served from a folder its users do not use: %v", err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := e.files(addon.ID), map[string]string{"Night of the Living Dead (1968).mkv": "tt0063350"}; !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if folder.SkippedLinks != (LinkCounts{Fenced: 1}) {
		t.Errorf("skipped %+v", folder.SkippedLinks)
	}
}

// reaches tells whether every user of a folder uses another.
func TestReaches(t *testing.T) {
	t.Parallel()
	g1, g2, u1 := testID(1), testID(2), testID(3)
	everyone := addons.UsedBy{}
	for _, c := range []struct {
		from, to addons.UsedBy
		want     bool
	}{
		{everyone, everyone, true},
		{addons.UsedBy{Listed: true, Groups: []accounts.ID{g1}}, everyone, true},
		{everyone, addons.UsedBy{Listed: true, Groups: []accounts.ID{g1, g2}}, false},
		{addons.UsedBy{Listed: true, Groups: []accounts.ID{g1}}, addons.UsedBy{Listed: true, Groups: []accounts.ID{g1, g2}}, true},
		{addons.UsedBy{Listed: true, Groups: []accounts.ID{g1, g2}}, addons.UsedBy{Listed: true, Groups: []accounts.ID{g1}}, false},
		{addons.UsedBy{Listed: true, Users: []accounts.ID{u1}}, addons.UsedBy{Listed: true, Groups: []accounts.ID{g1}}, false},
		{addons.UsedBy{Listed: true, Users: []accounts.ID{u1}}, addons.UsedBy{Listed: true, Users: []accounts.ID{u1}}, true},
	} {
		if got := reaches(c.from, c.to); got != c.want {
			t.Errorf("reaches(%+v, %+v) = %v", c.from, c.to, got)
		}
	}
}

// A walk that follows links reads a folder reached through several links
// once, and stops at the links that lead back to a folder read already, to
// a folder holding the folder walked, or round in a loop, counting them.
func TestLinkLoopsAreStopped(t *testing.T) {
	t.Parallel()
	base := realDir(t)
	root, outside := filepath.Join(base, "root"), filepath.Join(base, "outside")
	write(t, filepath.Join(outside, "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	symlink(t, outside, filepath.Join(root, "Linked"))
	symlink(t, outside, filepath.Join(outside, "Again"))
	symlink(t, base, filepath.Join(outside, "Up"))
	symlink(t, filepath.Join(outside, "Ring 2.mkv"), filepath.Join(outside, "Ring 1.mkv"))
	symlink(t, filepath.Join(outside, "Ring 1.mkv"), filepath.Join(outside, "Ring 2.mkv"))
	list, err := walk(t.Context(), root, KindMovies, linkRule{follow: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(maps.Keys(list.files)); !slices.Equal(got, []string{"Linked/Nosferatu (1922)/Nosferatu (1922).mkv"}) {
		t.Errorf("files: %v", got)
	}
	if list.links != (LinkCounts{Broken: 4}) {
		t.Errorf("skipped %+v", list.links)
	}
	// Not following them, the walk counts the one link leading out.
	list, err = walk(t.Context(), root, KindMovies, linkRule{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.files) != 0 || list.links != (LinkCounts{Out: 1}) {
		t.Errorf("files %v, skipped %+v", list.files, list.links)
	}
}

// Folders reached through links are read within the bounds of a share's
// walk.
func TestLinkedFoldersAreBounded(t *testing.T) {
	t.Parallel()
	base := realDir(t)
	root, outside := filepath.Join(base, "root"), filepath.Join(base, "outside")
	deep := outside
	for range shareWalk.depth + 1 {
		deep = filepath.Join(deep, "d")
	}
	write(t, filepath.Join(deep, "Nosferatu (1922).mkv"), 10)
	symlink(t, outside, filepath.Join(root, "Linked"))
	_, err := walk(t.Context(), root, KindMovies, linkRule{follow: true})
	var bound *shareBound
	if !errors.As(err, &bound) || bound.failure != errBoundDepth {
		t.Errorf("walk: %v", err)
	}
}

// A link to a file Polyfin cannot read is counted, and what was found
// through it is kept, as for a folder it cannot list.
func TestLinksToUnreadableFilesAreReported(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions do not stop this user from reading a file")
	}
	base := realDir(t)
	root, outside := filepath.Join(base, "root"), filepath.Join(base, "outside")
	write(t, filepath.Join(root, "Nosferatu (1922).mkv"), 10)
	secret := filepath.Join(outside, "The General (1926).mkv")
	write(t, secret, 10)
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o644) })
	symlink(t, secret, filepath.Join(root, "The General (1926).mkv"))
	// One inside the folder too.
	if err := os.Chmod(filepath.Join(root, "Nosferatu (1922).mkv"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "Nosferatu (1922).mkv"), 0o644) })
	symlink(t, filepath.Join(root, "Nosferatu (1922).mkv"), filepath.Join(root, "Nosferatu (1922) - copy.mkv"))
	list, err := walk(t.Context(), root, KindMovies, linkRule{follow: true})
	if err != nil {
		t.Fatal(err)
	}
	if list.links != (LinkCounts{Unreadable: 2}) {
		t.Errorf("skipped %+v", list.links)
	}
	slices.Sort(list.skipped)
	if !slices.Equal(list.skipped, []string{"Nosferatu (1922) - copy.mkv", "The General (1926).mkv"}) {
		t.Errorf("kept: %v", list.skipped)
	}
	if _, ok := list.files["The General (1926).mkv"]; ok {
		t.Error("an unreadable file is listed")
	}
}

// A watch of a folder that follows the links leading out of it watches
// the folders they lead to, those of links added too, and scans the
// folder on their changes; one that does not, does not watch them.
func TestWatchFollowsLinksLeadingOut(t *testing.T) {
	t.Parallel()
	base := realDir(t)
	root, plain, outside := filepath.Join(base, "root"), filepath.Join(base, "plain"), filepath.Join(base, "outside")
	write(t, filepath.Join(outside, "Show", "Season 1", "Show - S01E01.mkv"), 10)
	symlink(t, outside, filepath.Join(root, "Linked"))
	symlink(t, outside, filepath.Join(plain, "Linked"))
	id, other := testID(1), testID(2)
	fake := newFakeWatcher()
	wt := newWatchTest(t, func(context.Context) ([]watchFolder, error) {
		return []watchFolder{{id: id, root: root, follow: true}, {id: other, root: plain}}, nil
	}, func() (fileWatcher, error) { return fake, nil })
	wt.start()
	season := filepath.Join(outside, "Show", "Season 1")
	if watched := fake.watched(); !slices.Contains(watched, outside) || !slices.Contains(watched, season) {
		t.Fatalf("watched %v, want %s among them", watched, season)
	}
	if owners := wt.dirs(season); !slices.Equal(owners, []accounts.ID{id}) {
		t.Errorf("%s watched for %v", season, owners)
	}
	fake.events <- fsnotify.Event{Name: filepath.Join(season, "Show - S01E02.mkv"), Op: fsnotify.Create}
	wt.scanned(id)
	// A link added leads to a folder watched from then on.
	more := filepath.Join(base, "more")
	write(t, filepath.Join(more, "Other", "Other - S01E01.mkv"), 10)
	symlink(t, more, filepath.Join(root, "More"))
	fake.events <- fsnotify.Event{Name: filepath.Join(root, "More"), Op: fsnotify.Create}
	wt.scanned(id)
	if watched := fake.watched(); !slices.Contains(watched, filepath.Join(more, "Other")) {
		t.Errorf("watched %v, want %s among them", watched, filepath.Join(more, "Other"))
	}
}

// A folder its links lead to, watched for two folders, stays watched for
// one when the other is no longer watched.
func TestWatchKeepsFoldersLinkedFromOthers(t *testing.T) {
	t.Parallel()
	base := realDir(t)
	first, second, outside := filepath.Join(base, "first"), filepath.Join(base, "second"), filepath.Join(base, "outside")
	write(t, filepath.Join(outside, "Nosferatu (1922).mkv"), 10)
	symlink(t, outside, filepath.Join(first, "Linked"))
	symlink(t, outside, filepath.Join(second, "Linked"))
	id, other := testID(1), testID(2)
	var mu sync.Mutex
	folders := []watchFolder{{id: id, root: first, follow: true}, {id: other, root: second, follow: true}}
	fake := newFakeWatcher()
	wt := newWatchTest(t, func(context.Context) ([]watchFolder, error) {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(folders), nil
	}, func() (fileWatcher, error) { return fake, nil })
	wt.w.reconcile()
	if owners := wt.dirs(outside); len(owners) != 2 {
		t.Fatalf("%s watched for %v", outside, owners)
	}
	mu.Lock()
	folders = folders[:1]
	mu.Unlock()
	wt.w.reconcile()
	if !slices.Contains(fake.watched(), outside) {
		t.Fatalf("watched %v, want %s among them", fake.watched(), outside)
	}
	wt.wg.Go(wt.w.loop)
	fake.events <- fsnotify.Event{Name: filepath.Join(outside, "Nosferatu (1922).mkv"), Op: fsnotify.Write}
	wt.scanned(id)
}

// A file written in a folder reached through a link leading out is seen
// by the system's watcher, and scans the folder.
func TestWatchScansThroughLinks(t *testing.T) {
	t.Parallel()
	base := realDir(t)
	root, outside := filepath.Join(base, "root"), filepath.Join(base, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "Nosferatu (1922)"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, outside, filepath.Join(root, "Linked"))
	id := testID(1)
	wt := newNotifyTest(t, func(context.Context) ([]watchFolder, error) {
		return []watchFolder{{id: id, root: root, follow: true}}, nil
	})
	wt.start()
	wt.change(func() { write(t, filepath.Join(outside, "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10) })
	wt.scanned(id)
}

// dirs tells the folders whose watch has dir, as the loop sees them; it
// is read before the loop runs, or while it waits for an event.
func (wt *watchTest) dirs(dir string) []accounts.ID {
	owners := slices.Clone(wt.w.dirs[dir])
	slices.SortFunc(owners, func(a, b accounts.ID) int { return strings.Compare(a.String(), b.String()) })
	return owners
}

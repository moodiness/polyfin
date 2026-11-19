package localfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// grownShare is a share whose folders hold what holds tells, by their
// path and how deep they are under the share's folder; lists counts its
// listings, and a walk that lists more than grownLists, as one no bound
// ends would, is failed.
type grownShare struct {
	holds func(rel string, depth int) []remoteEntry
	lists atomic.Int32
}

const grownLists = 2000

func (s *grownShare) list(_ context.Context, rel string) ([]remoteEntry, error) {
	if s.lists.Add(1) > grownLists {
		return nil, fmt.Errorf("the walk goes on after %d listings", grownLists)
	}
	depth := 0
	if rel != "" {
		depth = strings.Count(rel, "/") + 1
	}
	return s.holds(rel, depth), nil
}

func (s *grownShare) open(context.Context, string) (io.ReadSeekCloser, time.Time, error) {
	return nil, time.Time{}, fs.ErrNotExist
}

func (s *grownShare) close() {}

// walked walks the share as a scan of a movies folder does, within b.
func (s *grownShare) walked(t *testing.T, b walkBounds) (listing, error) {
	t.Helper()
	root, err := s.list(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	return b.walk(t.Context(), s, root, KindMovies)
}

// failure is the bound a walk met, empty when it met none.
func failure(err error) string {
	var bound *shareBound
	if !errors.As(err, &bound) {
		return ""
	}
	return bound.failure
}

// A share whose every folder holds a folder is walked as deep as a walk
// follows folders, then fails it: the walk ends by itself, with the bound
// it met and where. The bounds of a walk are those the folder's page and
// the documentation tell.
func TestAShareNestedWithoutEndFailsItsWalk(t *testing.T) {
	t.Parallel()
	share := &grownShare{holds: func(string, int) []remoteEntry {
		return []remoteEntry{{name: "a", folder: true}, {name: "Movie (2020).mkv", size: 1}}
	}}
	if want := (walkBounds{depth: 32, folders: 1_000_000, files: 4_000_000, paths: 512 << 20}); shareWalk != want {
		t.Fatalf("a walk is bounded by %+v, want %+v", shareWalk, want)
	}
	_, err := share.walked(t, shareWalk)
	// The share's folder, then one folder a level down to the last.
	if lists := share.lists.Load(); failure(err) != errBoundDepth || lists != 33 {
		t.Fatalf("%d folders listed, %v; want 33, then the walk failed for its depth", lists, err)
	}
	if !strings.Contains(err.Error(), "more than 32 deep") {
		t.Errorf("the failure does not tell its bound: %v", err)
	}
}

// A share that holds more folders, more files, or longer paths than one
// walk reads fails it once the bound is met.
func TestAShareOfMoreThanAWalkReadsFailsIt(t *testing.T) {
	t.Parallel()
	small := walkBounds{depth: 32, folders: 1000, files: 1000, paths: 1 << 20}
	// wide holds fifty folders in each folder, three deep, and a movie
	// beside them: 127,550 folders.
	wide := func(rel string, depth int) []remoteEntry {
		entries := []remoteEntry{{name: "Movie (2020).mkv", size: 1}}
		for i := 0; depth < 3 && i < 50; i++ {
			entries = append(entries, remoteEntry{name: fmt.Sprintf("Folder %02d", i), folder: true})
		}
		return entries
	}
	// full holds 99 movies in each folder, and a folder: without end.
	full := func(string, int) []remoteEntry {
		entries := []remoteEntry{{name: "More", folder: true}}
		for i := 1; i < 100; i++ {
			entries = append(entries, remoteEntry{name: fmt.Sprintf("Movie %02d (2020).mkv", i), size: 1})
		}
		return entries
	}
	with := func(change func(b *walkBounds)) walkBounds {
		b := small
		change(&b)
		return b
	}
	for _, c := range []struct {
		name   string
		bounds walkBounds
		holds  func(rel string, depth int) []remoteEntry
		want   string
		// lists are the folders listed at most.
		lists int32
	}{
		{name: "folders", bounds: small, holds: wide, want: errBoundFolders, lists: 1000},
		{name: "files", bounds: small, holds: full, want: errBoundFiles, lists: 12},
		{name: "paths", bounds: with(func(b *walkBounds) { b.paths = 4096 }), holds: full, want: errBoundFiles, lists: 12},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			share := &grownShare{holds: c.holds}
			list, err := share.walked(t, c.bounds)
			if lists := share.lists.Load(); failure(err) != c.want || lists > c.lists || len(list.files) != 0 {
				t.Fatalf("%d folders listed, %d files kept, %v; want the walk failed as %s within %d listings", lists, len(list.files), err, c.want, c.lists)
			}
		})
	}
}

// A share just under the bounds of a walk is walked whole: folders as
// deep as a walk follows them, and as many folders, files and bytes of
// paths as it reads.
func TestAShareAtTheBoundsOfAWalkIsWalkedWhole(t *testing.T) {
	t.Parallel()
	t.Run("depth", func(t *testing.T) {
		t.Parallel()
		share := &grownShare{holds: func(_ string, depth int) []remoteEntry {
			entries := []remoteEntry{{name: "Movie (2020).mkv", size: 1}}
			if depth < shareWalk.depth {
				entries = append(entries, remoteEntry{name: "a", folder: true})
			}
			return entries
		}}
		list, err := share.walked(t, shareWalk)
		deepest := strings.Repeat("a/", shareWalk.depth) + "Movie (2020).mkv"
		if _, found := list.files[deepest]; err != nil || len(list.files) != shareWalk.depth+1 || !found {
			t.Fatalf("%d files found, the deepest %v, %v; want %d", len(list.files), found, err, shareWalk.depth+1)
		}
	})
	t.Run("folders, files and paths", func(t *testing.T) {
		t.Parallel()
		// Ten folders of ten folders, each of the 111 holding nine
		// movies: 110 folders to list, 999 files.
		holds := func(_ string, depth int) []remoteEntry {
			var entries []remoteEntry
			for i := range 9 {
				entries = append(entries, remoteEntry{name: fmt.Sprintf("Movie %d (2020).mkv", i), size: 1})
			}
			for i := 0; depth < 2 && i < 10; i++ {
				entries = append(entries, remoteEntry{name: fmt.Sprintf("Folder %d", i), folder: true})
			}
			return entries
		}
		paths := 0
		for _, depth := range []int{0, 1, 2} {
			folders := []int{1, 10, 100}[depth]
			under := depth * len("Folder 0/")
			paths += folders * 9 * (under + len("Movie 0 (2020).mkv"))
			if depth < 2 {
				paths += folders * 10 * (under + len("Folder 0"))
			}
		}
		exact := walkBounds{depth: 2, folders: 110, files: 999, paths: paths}
		share := &grownShare{holds: holds}
		list, err := share.walked(t, exact)
		if err != nil || len(list.files) != 999 || len(list.prints) != 111 {
			t.Fatalf("%d files found in %d folders, %v; want 999 in 111", len(list.files), len(list.prints), err)
		}
		for name, c := range map[string]struct {
			change func(b *walkBounds)
			want   string
		}{
			"one level more":          {func(b *walkBounds) { b.depth-- }, errBoundDepth},
			"one folder more":         {func(b *walkBounds) { b.folders-- }, errBoundFolders},
			"one file more":           {func(b *walkBounds) { b.files-- }, errBoundFiles},
			"one byte more of paths":  {func(b *walkBounds) { b.paths-- }, errBoundFiles},
			"the bounds a scan walks": {func(b *walkBounds) { *b = shareWalk }, ""},
		} {
			bounds := exact
			c.change(&bounds)
			share := &grownShare{holds: holds}
			if _, err := share.walked(t, bounds); failure(err) != c.want || c.want == "" && err != nil {
				t.Errorf("%s: %v, want the walk failed as %q", name, err, c.want)
			}
		}
	})
}

// A share that grew past a bound fails its scan with the bound it met,
// keeps the files found before, and is not scanned again at once: the
// attempt is dated at its end, so that the next scan on schedule comes a
// whole interval later, and the polls, which no longer have the prints
// of a scan to compare the share with, ask for one more scan, not for one
// a round.
func TestAShareThatMeetsABoundFailsItsScanAndIsNotScannedAtOnce(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922).mkv"), 10)
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	address := server.URL + "/movies"
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: address, Kind: KindMovies, User: "reader", Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if folder, err := e.service.Folder(t.Context(), addon.ID); err != nil || folder.Error != "" || folder.Files != 1 {
		t.Fatalf("the share as it was: %+v %v", folder, err)
	}
	if e.service.shareSeen(addon.ID).prints == nil {
		t.Fatal("the scan kept no prints")
	}

	// Folders one level deeper than a walk follows them.
	write(t, filepath.Join(dir, "movies", filepath.FromSlash(strings.Repeat("deeper/", shareWalk.depth+1)), "The General (1926).mkv"), 10)
	// Each time the scan asks the time, an hour has passed.
	var mu sync.Mutex
	started := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	clock := started
	e.service.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		at := clock
		clock = clock.Add(time.Hour)
		return at
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	ended := clock.Add(-time.Hour)
	mu.Unlock()
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Error != errBoundDepth || folder.Files != 1 || folder.Matched != 1 {
		t.Fatalf("the share grown too deep: %+v %v", folder, err)
	}
	if folder.CheckedAt == nil || !folder.CheckedAt.Equal(ended) || !ended.After(started) {
		t.Errorf("the attempt is dated %v, want the end of its scan, %v, begun at %v", folder.CheckedAt, ended, started)
	}

	ctx, cancel := context.WithCancel(t.Context())
	wg := &sync.WaitGroup{}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
	w := newWatcher(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), one(addon.ID, address), func(context.Context, accounts.ID) {},
		openNotifyWatcher, e.service, wg.Go)
	w.polled = make(chan sharePolled, 1)
	p := newSharePoll(addon.ID, address, testEvery)
	asked := 0
	for range 3 {
		w.round(ctx, p)
		select {
		case <-w.polled:
			asked++
		default:
		}
	}
	if asked != 1 {
		t.Errorf("three rounds of the share's poll asked for %d scans, want one", asked)
	}
}

// countedList counts the listings of a remote, and ends the walk at the
// stop-th: one that would not end by itself.
type countedList struct {
	remote
	lists atomic.Int32
	stop  int32
	end   context.CancelFunc
}

func (c *countedList) list(ctx context.Context, rel string) ([]remoteEntry, error) {
	if c.lists.Add(1) == c.stop {
		c.end()
	}
	return c.remote.list(ctx, rel)
}

// A link of an SFTP folder to a folder elsewhere is entered, as the
// server follows it; a link that leads to its own folder, or to a folder
// above it, is not: the folders it holds would be listed again under it,
// link after link.
func TestAnSFTPLinkLeadingBackIsNotEntered(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	movies := filepath.Join(dir, "movies")
	write(t, filepath.Join(movies, "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(movies, "Silent", "The General (1926).mkv"), 10)
	write(t, filepath.Join(dir, "other", "Metropolis (1927).mkv"), 10)
	links := map[string]string{
		filepath.Join(movies, "Elsewhere"):    filepath.Join("..", "other"),
		filepath.Join(movies, "Again"):        ".",
		filepath.Join(movies, "Silent", "Up"): "..",
	}
	if runtime.GOOS != "windows" {
		// Back to its own folder through a path, and by its full path:
		// on Windows the fake server tells a link's target as Windows
		// writes it, with \ where the walk reads /.
		links[filepath.Join(movies, "Silent", "Same")] = filepath.Join("..", "Silent")
		links[filepath.Join(movies, "Silent", "Whole")] = movies
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	server := serveSFTP(t, "reader", "pass phrase", nil)
	r, err := newRemote(shareAccess{address: "sftp://" + server.addr + sftpPath(movies), user: "reader", password: "pass phrase"}, hostKeys{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.close)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	counted := &countedList{remote: r, stop: 200, end: cancel}
	root, err := counted.list(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	list, err := walkShare(ctx, counted, root, KindMovies)
	lists := counted.lists.Load()
	if err != nil {
		t.Fatalf("%d folders listed: %v", lists, err)
	}
	want := []string{"Elsewhere/Metropolis (1927).mkv", "Nosferatu (1922).mkv", "Silent/The General (1926).mkv"}
	for _, rel := range want {
		if _, found := list.files[rel]; !found {
			t.Errorf("%s was not found", rel)
		}
	}
	// The folder, the one beside it and the one a link leads to.
	if len(list.files) != len(want) || lists != 3 {
		t.Errorf("%d files found in %d folders listed, want %d in 3", len(list.files), lists, len(want))
	}
}

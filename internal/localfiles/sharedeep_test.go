package localfiles

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/webdav"
)

// How a test's WebDAV server answers a listing at infinite depth: as
// asked, refused as Apache's does by default, or as a listing of one
// folder, as a server ignoring the depth asked would.
const (
	deepAnswered = iota
	deepRefused
	deepIgnored
)

// deepServer serves a folder over WebDAV, counting its listings by the
// depth asked, and a delay before each answer.
type deepServer struct {
	*httptest.Server
	mu    sync.Mutex
	lists map[string]int
}

func serveDeep(tb testing.TB, dir string, how int, delay time.Duration) *deepServer {
	tb.Helper()
	handler := &webdav.Handler{FileSystem: webdav.Dir(dir), LockSystem: webdav.NewMemLS()}
	s := &deepServer{lists: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			s.mu.Lock()
			s.lists[r.Header.Get("Depth")]++
			s.mu.Unlock()
			if r.Header.Get("Depth") == "infinity" {
				switch how {
				case deepRefused:
					w.Header().Set("Content-Type", "application/xml; charset=utf-8")
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><D:error xmlns:D="DAV:"><D:propfind-finite-depth/></D:error>`))
					return
				case deepIgnored:
					r.Header.Set("Depth", "1")
				}
			}
		}
		time.Sleep(delay)
		handler.ServeHTTP(w, r)
	}))
	tb.Cleanup(s.Close)
	return s
}

// counted returns the listings the server answered since the last call,
// by depth, and forgets them.
func (s *deepServer) counted() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	lists := s.lists
	s.lists = map[string]int{}
	return lists
}

// walkWebDAV walks the share at address as a scan of a movies folder
// does, with r, within b.
func walkWebDAV(tb testing.TB, r *webdavShare, b walkBounds) (listing, error) {
	tb.Helper()
	root, err := r.list(context.Background(), "")
	if err != nil {
		tb.Fatal(err)
	}
	return b.walk(context.Background(), r, root, KindMovies)
}

func newTestWebDAV(tb testing.TB, address string) *webdavShare {
	tb.Helper()
	u, err := url.Parse(address)
	if err != nil {
		tb.Fatal(err)
	}
	r := newWebDAV(u, "", "")
	tb.Cleanup(r.close)
	return r
}

// deepTree writes a share's folder: movies one and three folders deep,
// subtitles beside one, a folder a scan does not enter holding a movie, a
// file a scan does not read, and an empty folder.
func deepTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(dir, "Nosferatu (1922)", "Nosferatu (1922).en.srt"), 3)
	write(t, filepath.Join(dir, "Nosferatu (1922)", "@eaDir", "Deeper", "The General (1926).mkv"), 10)
	write(t, filepath.Join(dir, "Collections", "Silent", "Keaton", "The General (1926).mkv"), 20)
	write(t, filepath.Join(dir, "Collections", "notes.txt"), 5)
	if err := os.Mkdir(filepath.Join(dir, "Empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// sameListing reports how two walks of one share differ, empty when they
// found the same files, side files and folders' prints.
func sameListing(got, want listing) string {
	switch {
	case !maps.Equal(got.files, want.files):
		return "files"
	case !maps.Equal(got.sides, want.sides):
		return "side files"
	case !maps.Equal(got.prints, want.prints):
		return "prints"
	case len(got.skipped) != len(want.skipped):
		return "folders skipped"
	}
	return ""
}

// A WebDAV share that lists its folder at infinite depth is walked with
// that one listing, and an empty folder listed alone, rather than with
// one listing per folder; and the walk finds what one folder by folder
// does.
func TestAWebDAVShareListedDeepIsWalkedInOneListing(t *testing.T) {
	t.Parallel()
	dir := deepTree(t)
	deep := serveDeep(t, dir, deepAnswered, 0)
	got, err := walkWebDAV(t, newTestWebDAV(t, deep.URL+"/"), shareWalk)
	if err != nil {
		t.Fatal(err)
	}
	// The root, listed before the walk, then the deep listing, then the
	// empty folder alone.
	if lists := deep.counted(); !maps.Equal(lists, map[string]int{"1": 2, "infinity": 1}) {
		t.Errorf("listings by depth: %v, want the root, one deep listing and the empty folder", lists)
	}
	flat := serveDeep(t, dir, deepRefused, 0)
	want, err := walkWebDAV(t, newTestWebDAV(t, flat.URL+"/"), shareWalk)
	if err != nil {
		t.Fatal(err)
	}
	if lists := flat.counted(); lists["1"] != 6 {
		t.Errorf("listings by depth of the share walked folder by folder: %v, want 6 of one folder", lists)
	}
	if differ := sameListing(got, want); differ != "" {
		t.Errorf("the deep walk and the walk folder by folder found other %s: %+v against %+v", differ, got, want)
	}
	if _, ok := got.files["Collections/Silent/Keaton/The General (1926).mkv"]; !ok {
		t.Errorf("the movie three folders deep is not found: %v", got.files)
	}
	if _, ok := got.files["Nosferatu (1922)/@eaDir/Deeper/The General (1926).mkv"]; ok {
		t.Errorf("a movie in a folder a scan does not enter is found: %v", got.files)
	}
}

// A change three folders deep, which leaves the modification times and
// listings of the folders above it as they were, is found by the next
// deep walk, in the files and in the print of its own folder.
func TestADeepWalkFindsANestedChange(t *testing.T) {
	t.Parallel()
	dir := deepTree(t)
	server := serveDeep(t, dir, deepAnswered, 0)
	r := newTestWebDAV(t, server.URL+"/")
	before, err := walkWebDAV(t, r, shareWalk)
	if err != nil {
		t.Fatal(err)
	}
	keaton := filepath.Join(dir, "Collections", "Silent", "Keaton")
	stamps := map[string]time.Time{}
	for _, folder := range []string{dir, filepath.Join(dir, "Collections"), filepath.Join(dir, "Collections", "Silent"), keaton} {
		info, err := os.Stat(folder)
		if err != nil {
			t.Fatal(err)
		}
		stamps[folder] = info.ModTime()
	}
	write(t, filepath.Join(keaton, "Sherlock Jr. (1924).mkv"), 30)
	write(t, filepath.Join(keaton, "The General (1926).mkv"), 25)
	for folder, stamp := range stamps {
		// Each folder keeps the time it had, the deepest one included: no
		// time above the change tells it.
		if err := os.Chtimes(folder, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	server.counted()
	after, err := walkWebDAV(t, r, shareWalk)
	if err != nil {
		t.Fatal(err)
	}
	if lists := server.counted(); lists["infinity"] != 1 || lists["1"] != 2 {
		t.Errorf("listings by depth: %v, want one deep listing", lists)
	}
	if _, ok := after.files["Collections/Silent/Keaton/Sherlock Jr. (1924).mkv"]; !ok {
		t.Errorf("the movie added three folders deep is not found: %v", after.files)
	}
	if got := after.files["Collections/Silent/Keaton/The General (1926).mkv"].size; got != 25 {
		t.Errorf("the movie changed three folders deep is %d bytes, want 25", got)
	}
	for _, rel := range []string{"", "Collections", "Collections/Silent"} {
		if after.prints[rel] != before.prints[rel] {
			t.Errorf("the print of %q changed, which holds no change", rel)
		}
	}
	if after.prints["Collections/Silent/Keaton"] == before.prints["Collections/Silent/Keaton"] {
		t.Error("the print of the folder changed is the same")
	}
}

// A share that refuses a listing at infinite depth is walked folder by
// folder, whole, and not asked for a deep listing again for a while.
func TestAWebDAVShareRefusingDeepListingsIsWalkedFolderByFolder(t *testing.T) {
	t.Parallel()
	dir := deepTree(t)
	server := serveDeep(t, dir, deepRefused, 0)
	r := newTestWebDAV(t, server.URL+"/")
	for round := range 2 {
		list, err := walkWebDAV(t, r, shareWalk)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := list.files["Collections/Silent/Keaton/The General (1926).mkv"]; !ok {
			t.Errorf("walk %d: the movie three folders deep is not found: %v", round, list.files)
		}
		want := map[string]int{"1": 6, "infinity": 1}
		if round > 0 {
			want = map[string]int{"1": 6}
		}
		if lists := server.counted(); !maps.Equal(lists, want) {
			t.Errorf("walk %d: listings by depth: %v, want %v", round, lists, want)
		}
	}
	if now := time.Now(); r.deep.worth(now) || !r.deep.worth(now.Add(deepRetry)) {
		t.Error("a deep listing is asked again before deepRetry, or never")
	}
}

// A server answering a listing at infinite depth as one of one folder,
// which shows each folder under the share's empty, has the share walked
// folder by folder: nothing deeper is missed.
func TestAWebDAVShareIgnoringTheDepthIsWalkedFolderByFolder(t *testing.T) {
	t.Parallel()
	dir := deepTree(t)
	server := serveDeep(t, dir, deepIgnored, 0)
	got, err := walkWebDAV(t, newTestWebDAV(t, server.URL+"/"), shareWalk)
	if err != nil {
		t.Fatal(err)
	}
	if lists := server.counted(); lists["infinity"] != 1 {
		t.Errorf("listings by depth: %v, want one deep listing asked", lists)
	}
	flat := serveDeep(t, dir, deepRefused, 0)
	want, err := walkWebDAV(t, newTestWebDAV(t, flat.URL+"/"), shareWalk)
	if err != nil {
		t.Fatal(err)
	}
	if differ := sameListing(got, want); differ != "" {
		t.Errorf("the walk found other %s than one folder by folder: %+v against %+v", differ, got, want)
	}
}

// A deep listing holding more folders than a walk lists fails the walk
// as the walk folder by folder fails, rather than having the share walked
// so.
func TestADeepListingMeetsTheBoundsOfAWalk(t *testing.T) {
	t.Parallel()
	server := serveDeep(t, deepTree(t), deepAnswered, 0)
	_, err := walkWebDAV(t, newTestWebDAV(t, server.URL+"/"), walkBounds{depth: 32, folders: 3, files: 100, paths: 1 << 20})
	if got := failure(err); got != errBoundFolders {
		t.Errorf("the walk failed with %v, want %q", err, errBoundFolders)
	}
	if lists := server.counted(); !maps.Equal(lists, map[string]int{"1": 1, "infinity": 1}) {
		t.Errorf("listings by depth: %v, want the root and one deep listing", lists)
	}
}

// countedPages returns the pages of listings the bucket answered since
// the last call, by delimiter, and forgets them.
func (f *fakeS3) countedPages() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	pages := f.pages
	f.pages = map[string]int{}
	return pages
}

// An S3 bucket is walked with pages listing every object under its
// prefix, without a delimiter, rather than with a listing per folder: the
// walk finds what one folder by folder does, and a change three folders
// deep.
func TestAnS3BucketIsWalkedWithoutADelimiter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).en.srt"), 3)
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "@eaDir", "Deeper", "The General (1926).mkv"), 10)
	write(t, filepath.Join(dir, "movies", "Collections", "Silent", "Keaton", "The General (1926).mkv"), 20)
	write(t, filepath.Join(dir, "movies", "Collections", "notes.txt"), 5)
	write(t, filepath.Join(dir, "elsewhere", "Sherlock Jr. (1924).mkv"), 5)
	bucket, server := serveS3(t, dir, "media", "AKIDTEST", "secret")
	share := func(deep bool) *s3Share {
		r, err := newRemote(shareAccess{address: "s3://media/movies", endpoint: server.URL, region: "eu-test-1", user: "AKIDTEST",
			password: "secret"}, hostKeys{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(r.close)
		s := r.(*s3Share)
		if !deep {
			s.deep.failed(time.Now())
		}
		return s
	}
	walked := func(s *s3Share) listing {
		root, err := s.list(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		list, err := shareWalk.walk(t.Context(), s, root, KindMovies)
		if err != nil {
			t.Fatal(err)
		}
		return list
	}
	deep := share(true)
	got := walked(deep)
	// The root, two entries, in one page with the delimiter; the five
	// objects under the prefix, two a page, in three without.
	if pages := bucket.countedPages(); !maps.Equal(pages, map[string]int{"/": 1, "": 3}) {
		t.Errorf("pages by delimiter: %v, want the root's and three without a delimiter", pages)
	}
	want := walked(share(false))
	if pages := bucket.countedPages(); pages[""] != 0 || pages["/"] != 6 {
		t.Errorf("pages by delimiter of the bucket walked folder by folder: %v, want 6 with the delimiter", pages)
	}
	if differ := sameListing(got, want); differ != "" {
		t.Errorf("the deep walk and the walk folder by folder found other %s: %+v against %+v", differ, got, want)
	}
	if _, ok := got.files["Collections/Silent/Keaton/The General (1926).mkv"]; !ok {
		t.Errorf("the movie three folders deep is not found: %v", got.files)
	}
	if len(got.files) != 2 {
		t.Errorf("files: %v, want the two movies the scan reads", got.files)
	}

	write(t, filepath.Join(dir, "movies", "Collections", "Silent", "Keaton", "Sherlock Jr. (1924).mkv"), 30)
	after := walked(deep)
	if _, ok := after.files["Collections/Silent/Keaton/Sherlock Jr. (1924).mkv"]; !ok {
		t.Errorf("the movie added three folders deep is not found: %v", after.files)
	}
	if after.prints["Collections/Silent/Keaton"] == got.prints["Collections/Silent/Keaton"] {
		t.Error("the print of the folder changed is the same")
	}
}

// BenchmarkWebDAVWalk walks a share of 1,000 movie folders, each holding
// a movie, its subtitles and a folder of extras with one file, served on
// the loopback and through a server answering each request 2 ms late, as
// a share over a network does: folder by folder, as before deep listings,
// and with one deep listing.
func BenchmarkWebDAVWalk(b *testing.B) {
	dir := b.TempDir()
	for i := range 1000 {
		folder := filepath.Join(dir, "Movie "+strconv.Itoa(i)+" (2000)")
		if err := os.MkdirAll(filepath.Join(folder, "Featurettes"), 0o755); err != nil {
			b.Fatal(err)
		}
		for _, name := range []string{"Movie " + strconv.Itoa(i) + " (2000).mkv", "Movie " + strconv.Itoa(i) + " (2000).en.srt", "Featurettes/Trailer.mkv"} {
			if err := os.WriteFile(filepath.Join(folder, name), []byte("x"), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	for _, delay := range []time.Duration{0, 2 * time.Millisecond} {
		for _, deep := range []bool{false, true} {
			name := "folders"
			if deep {
				name = "deep"
			}
			b.Run(fmt.Sprintf("%s/delay=%v", name, delay), func(b *testing.B) {
				server := serveDeep(b, dir, deepAnswered, delay)
				r := newTestWebDAV(b, server.URL+"/")
				server.counted()
				for b.Loop() {
					r.deep.failedAt.Store(0)
					if !deep {
						r.deep.failed(time.Now())
					}
					if _, err := walkWebDAV(b, r, shareWalk); err != nil {
						b.Fatal(err)
					}
				}
				lists := server.counted()
				b.ReportMetric(float64(lists["1"]+lists["infinity"])/float64(b.N), "propfinds/op")
			})
		}
	}
}

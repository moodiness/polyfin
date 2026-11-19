package localfiles

import (
	"bytes"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
)

// heldShare is a WebDAV share a test serves through a front that, once
// holding, keeps each listing waiting until release lets it through, or
// until the scan asking for it stops.
type heldShare struct {
	*httptest.Server
	holding atomic.Bool
}

// serveHeld serves dir over WebDAV behind a front holding listings while
// holding is set: asked is closed at the first listing held, and each
// waits for release.
func serveHeld(t *testing.T, dir string, asked chan struct{}, release <-chan struct{}) *heldShare {
	t.Helper()
	share := serveWebDAV(t, dir, "reader", "pass phrase")
	h := &heldShare{}
	var once sync.Once
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" && h.holding.Load() {
			// The body read whole, the server notices the client leaving.
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			once.Do(func() { close(asked) })
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		share.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(h.Close)
	return h
}

// sealedEnv is an environment whose shares' passwords can be sealed.
func sealedEnv(t *testing.T) env {
	t.Helper()
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	return e
}

// addHeld adds a movies folder of the share h serves, and waits for its
// first scan.
func (e env) addHeld(h *heldShare) accounts.ID {
	e.t.Helper()
	addon, err := e.service.Add(e.t.Context(), NewFolder{Name: "NAS", Path: h.URL + "/movies/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		e.t.Fatal(err)
	}
	e.idle(addon.ID)
	return addon.ID
}

// A share's scan that lasts longer than one scan may is stopped, and the
// folder fails with too_slow: the files found before stay, and the next
// scan on schedule counts from the end of the one stopped.
func TestAShareScanStopsAtItsTime(t *testing.T) {
	t.Parallel()
	e := sealedEnv(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922).mkv"), 10)
	// Never released: a listing held waits for the scan to stop.
	share := serveHeld(t, dir, make(chan struct{}), nil)
	id := e.addHeld(share)
	want := map[string]string{"Nosferatu (1922).mkv": "tt0013442"}
	if got := e.files(id); !maps.Equal(got, want) {
		t.Fatalf("files of the first scan: %v", got)
	}
	share.holding.Store(true)
	e.service.shareScanTime = 100 * time.Millisecond
	before := time.Now()
	if err := e.service.Scan(t.Context(), id); err != nil {
		t.Fatalf("a scan out of time: %v, want it told on the folder", err)
	}
	folder, err := e.service.Folder(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Error != errBoundTime {
		t.Errorf("the folder's failure is %q, want %q", folder.Error, errBoundTime)
	}
	if folder.CheckedAt == nil || folder.CheckedAt.Before(before) {
		t.Errorf("the scan stopped is dated %v, want after its start %v", folder.CheckedAt, before)
	}
	if got := e.files(id); !maps.Equal(got, want) {
		t.Errorf("files after the scan stopped: %v, want those found before", got)
	}
}

// The due scans of shares on different servers run side by side: each
// share here holds its listings until the other share was asked for one,
// which scans one after the other would never do within a scan's time.
func TestOneServersScanHoldsNoOtherServer(t *testing.T) {
	t.Parallel()
	e := sealedEnv(t)
	first, second := t.TempDir(), t.TempDir()
	write(t, filepath.Join(first, "movies", "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(second, "movies", "Night of the Living Dead (1968).mkv"), 10)
	// Each share's listings wait for the other share to be asked.
	firstAsked, secondAsked := make(chan struct{}), make(chan struct{})
	a := serveHeld(t, first, firstAsked, secondAsked)
	b := serveHeld(t, second, secondAsked, firstAsked)
	ids := []accounts.ID{e.addHeld(a), e.addHeld(b)}
	if scanGroup(a.URL+"/movies/") == scanGroup(b.URL+"/movies/") {
		t.Fatal("the two shares are on one server")
	}
	a.holding.Store(true)
	b.holding.Store(true)
	// Long enough for the other share's scan to start, however busy the
	// machine; one after the other, the first share's scan reaches it.
	e.service.shareScanTime = 10 * time.Second
	if err := e.service.ScanDue(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		folder, err := e.service.Folder(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if folder.Error != "" {
			t.Errorf("a share waiting for the other's scan failed with %q: their scans ran one after the other", folder.Error)
		}
	}
	select {
	case <-firstAsked:
	default:
		t.Error("the first share was not listed")
	}
	select {
	case <-secondAsked:
	default:
		t.Error("the second share was not listed")
	}
}

// The folders of one server are scanned in one group, those in the
// container in another.
func TestScanGroups(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"/media/movies":                          "",
		"smb://NAS.local/media/movies":           "smb://nas.local",
		"smb://nas.local/media/shows":            "smb://nas.local",
		"smb://nas.local:4450/media":             "smb://nas.local:4450",
		"sftp://nas.local/srv/media":             "sftp://nas.local",
		"https://dav.example/movies":             "https://dav.example",
		"s3://media/movies":                      "s3://media",
		"rclone://rclone:5572/movies":            "rclone://rclone:5572",
		"http://127.0.0.1:8080/dav/movies/shows": "http://127.0.0.1:8080",
	}
	for location, want := range cases {
		if got := scanGroup(location); got != want {
			t.Errorf("scanGroup(%q) = %q, want %q", location, got, want)
		}
	}
}

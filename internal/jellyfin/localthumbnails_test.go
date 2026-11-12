package jellyfin

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// localThumbnailsSetup is the movie with a local folder's file beside the
// addon's two versions, thumbnails turned on, one every 5 s, and an
// administrator signed in.
type localThumbnailsSetup struct {
	playbackSetup
	admin      accounts.User
	adminToken string
	movieID    accounts.ID
	local      library.Version
}

// localThumbnails links forced.mkv, 15 s of 64x64 video, in the folder
// Shelf to the movie, and stores its analysis.
func localThumbnails(t *testing.T) localThumbnailsSetup {
	t.Helper()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := playingOn(t, s)
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Movie (2008) 1080p.mkv"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}
	p.setting(t, func(s *accounts.Settings) { s.Trickplay, s.TrickplayInterval = true, 5 })
	l := localThumbnailsSetup{playbackSetup: p, admin: s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })}
	l.adminToken = s.signIn("admin", "phone")
	l.movieID, _ = accounts.ParseID(p.movie)
	versions, err := p.library.Versions(t.Context(), l.admin, l.movieID)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions {
		if version.LocalFolder {
			l.local = version
		}
	}
	if !l.local.LocalFolder {
		t.Fatalf("no local version: %+v", versions)
	}
	analysis := media.Analysis{Format: "matroska,webm", Duration: 15008 * time.Millisecond, Size: int64(len(file)),
		Streams: []media.Stream{{Index: 0, Type: "audio", Codec: "opus"}, {Index: 1, Type: "video", Codec: "h264", Width: 64, Height: 64}}}
	stored, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", l.local.ID, stored); err != nil {
		t.Fatal(err)
	}
	return l
}

// stale gives the local file a set of one thumbnail, as made before.
func (l localThumbnailsSetup) stale(t *testing.T) {
	t.Helper()
	info := thumbnails.Info{Width: 320, Height: 320, TileWidth: 10, TileHeight: 10, ThumbnailCount: 1, Interval: 5000, Bandwidth: 100}
	if err := l.handler.Thumbnails.SaveTrickplay(t.Context(), l.local.ID, l.movieID, info, [][]byte{[]byte("\xff\xd8 stale")}); err != nil {
		t.Fatal(err)
	}
}

// thumbnailCount is how many thumbnails the local file has at 320 pixels.
func (l localThumbnailsSetup) thumbnailCount(t *testing.T) int {
	t.Helper()
	manifest, err := l.handler.Thumbnails.Manifest(t.Context(), l.movieID)
	if err != nil {
		t.Fatal(err)
	}
	return manifest[l.local.ID][320].ThumbnailCount
}

// remadeWithin reports whether the local file's thumbnails were made
// again from its file within d: 4 for its 15 s.
func (l localThumbnailsSetup) remadeWithin(t *testing.T, d time.Duration) bool {
	t.Helper()
	for deadline := time.Now().Add(d); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if l.thumbnailCount(t) == 4 {
			return true
		}
	}
	return false
}

// Make again replaces a local file's thumbnails with those of its file,
// which the Versions page then tells; an addon's link is refused, and so
// is any while the settings turn the images off.
func TestMakeThumbnailsAgainRemakesALocalFile(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	l := localThumbnails(t)
	l.stale(t)
	result, err := l.handler.MakeThumbnailsAgain(t.Context(), l.admin, l.movieID, l.local.ID)
	if err != nil || result.State != thumbnails.StateWaiting && result.State != thumbnails.StateWorking {
		t.Fatalf("make again: %+v %v", result, err)
	}
	if !l.remadeWithin(t, 8*time.Second) {
		t.Fatalf("the thumbnails were not made again: %d", l.thumbnailCount(t))
	}
	var local *thumbnails.Result
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		explained, err := l.handler.ExplainVersions(t.Context(), l.admin, l.movieID)
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range explained.Sources {
			for _, stream := range source.Streams {
				switch stream.Version {
				case l.local.ID:
					local = stream.Thumbnails
				case l.versions[0].ID, l.versions[1].ID:
					if stream.Thumbnails != nil {
						t.Fatalf("an addon's link: %+v", stream.Thumbnails)
					}
				}
			}
		}
		if local != nil && local.State == thumbnails.StateMade {
			break
		}
	}
	if local == nil || local.State != thumbnails.StateMade || local.Count != 4 || local.At == nil {
		t.Errorf("the local file on the Versions page: %+v", local)
	}

	if _, err := l.handler.MakeThumbnailsAgain(t.Context(), l.admin, l.movieID, l.versions[0].ID); !errors.Is(err, thumbnails.ErrNotLocal) {
		t.Errorf("an addon's link: %v", err)
	}
	l.setting(t, func(s *accounts.Settings) { s.Trickplay, s.ChapterImages = false, false })
	if _, err := l.handler.MakeThumbnailsAgain(t.Context(), l.admin, l.movieID, l.local.ID); !errors.Is(err, thumbnails.ErrTurnedOff) {
		t.Errorf("turned off: %v", err)
	}
}

// Refresh metadata with Replace existing trickplay images makes a local
// file's thumbnails again, with a full refresh only, as in Jellyfin.
func TestRefreshRegeneratesLocalThumbnails(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	l := localThumbnails(t)
	l.stale(t)
	refresh := func(mode string) {
		t.Helper()
		if status, body := l.call(http.MethodPost, "/Items/"+l.movie+"/Refresh?metadataRefreshMode="+mode+"&imageRefreshMode="+mode+
			"&replaceAllMetadata=false&replaceAllImages=false&regenerateTrickplay=true", app("phone", l.adminToken), nil); status != http.StatusNoContent {
			t.Fatalf("refresh %s: %d %s", mode, status, body)
		}
	}
	refresh("Default")
	time.Sleep(300 * time.Millisecond)
	if n := l.thumbnailCount(t); n != 1 {
		t.Fatalf("a default refresh made the thumbnails again: %d", n)
	}
	refresh("FullRefresh")
	if !l.remadeWithin(t, 8*time.Second) {
		t.Errorf("a full refresh did not make the thumbnails again: %d", l.thumbnailCount(t))
	}
}

// A local folder's library tells Jellyfin apps that it extracts trickplay
// images while the settings turn thumbnails on; an addon's library never.
func TestLocalLibrariesTellTrickplayExtraction(t *testing.T) {
	t.Parallel()
	l := localThumbnails(t)
	extraction := func() map[string]bool {
		t.Helper()
		status, body := l.call(http.MethodGet, "/Library/VirtualFolders", app("phone", l.adminToken), nil)
		var folders []VirtualFolderInfo
		if err := json.Unmarshal(body, &folders); status != http.StatusOK || err != nil {
			t.Fatalf("virtual folders: %d %s", status, body)
		}
		got := map[string]bool{}
		for _, folder := range folders {
			got[folder.Name] = folder.LibraryOptions.EnableTrickplayImageExtraction
		}
		return got
	}
	got := extraction()
	if !got["Shelf"] || len(got) < 2 {
		t.Errorf("thumbnails on: %v", got)
	}
	for name, on := range got {
		if name != "Shelf" && on {
			t.Errorf("the addon's library %s extracts trickplay images", name)
		}
	}
	l.setting(t, func(s *accounts.Settings) { s.Trickplay = false })
	if got := extraction(); got["Shelf"] {
		t.Errorf("thumbnails off: %v", got)
	}
}

// The local folders' versions are listed, as the task Make thumbnails
// walks them, from every local folder's library, from the folder's own
// library or from their title, and none from an addon's library.
func TestLocalVersionsListTheLocalFiles(t *testing.T) {
	t.Parallel()
	l := localThumbnails(t)
	var views QueryResult
	l.get(t, "/UserViews", l.adminToken, &views)
	ids := map[string]accounts.ID{}
	for _, view := range views.Items {
		ids[view.Name], _ = accounts.ParseID(view.Id)
	}
	walk := func(parent accounts.ID) []string {
		t.Helper()
		var listed []string
		err := l.library.LocalVersions(t.Context(), parent, func(item library.Item, version library.Version) error {
			listed = append(listed, item.ID.String()+" "+version.ID.String())
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return listed
	}
	want := l.movie + " " + l.local.ID.String()
	for name, parent := range map[string]accounts.ID{"every local folder": {}, "the folder's library": ids["Shelf"], "the title": l.movieID} {
		if got := walk(parent); len(got) != 1 || got[0] != want {
			t.Errorf("from %s: %v, want %s", name, got, want)
		}
	}
	for name, id := range ids {
		if name != "Shelf" {
			if got := walk(id); len(got) != 0 {
				t.Errorf("from the library %s: %v", name, got)
			}
		}
	}
}

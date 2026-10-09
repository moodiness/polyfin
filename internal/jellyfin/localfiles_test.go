package jellyfin

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A file of a local folder linked to a title is one more of its versions,
// beside the addons' streams, named after its folder, and Polyfin serves
// it from the disk with byte ranges.
func TestLocalFilesAreVersions(t *testing.T) {
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open))
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
	// The streaming addon offers no search: the file is linked by hand.
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}

	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	local := (*movie.MediaSources)[2]
	if local.Name != "Shelf · 1080p · 52 KB" || local.Path == "" {
		t.Errorf("local version: %+v", local)
	}
	response, body := fetchURL(t, p.url+"/Items/"+local.Id+"/Download?ApiKey="+p.token, http.Header{"Range": {"bytes=100-199"}})
	if response.StatusCode != http.StatusPartialContent || body != string(file[100:200]) ||
		response.Header.Get("Content-Range") != "bytes 100-199/52341" {
		t.Errorf("a range of the file: %d %v", response.StatusCode, response.Header)
	}
	// The folder's library lists the title, and the file once gone is
	// not offered any more.
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	shelf := ""
	for _, view := range views.Items {
		if view.Name == "Shelf" {
			shelf = view.Id
		}
	}
	var page QueryResult
	if p.get(t, "/Items?ParentId="+shelf, p.token, &page); len(page.Items) != 1 || page.Items[0].Id != p.movie {
		t.Errorf("the folder's library: %+v", page.Items)
	}
	if err := os.Remove(filepath.Join(dir, "Movie (2008) 1080p.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	for _, source := range *movie.MediaSources {
		if strings.HasPrefix(source.Name, "Shelf") {
			t.Errorf("a version of a file gone: %+v", source)
		}
	}
}

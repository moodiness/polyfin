package jellyfin

import (
	"image/png"
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

// The artwork beside a local folder's movie replaces the addon's of the
// same type in its details, by tags of its own that change with the file,
// and is served resized at the size apps ask for, without any path of the
// server's showing; off, the addon's is back.
func TestLocalArtworkInJellyfinApps(t *testing.T) {
	t.Parallel()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := playingOn(t, s)
	details := func() BaseItemDto {
		t.Helper()
		var movie BaseItemDto
		if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie); status != http.StatusOK {
			t.Fatalf("details: %d", status)
		}
		return movie
	}
	addonPoster := details().ImageTags["Primary"]
	if addonPoster == "" {
		t.Fatal("the addon gives no poster")
	}

	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, "Movie (2008)"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Movie (2008)", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Movie (2008) 1080p.mkv", []byte("\x1a\x45\xdf\xa3 media bytes"))
	write("poster.png", picture(t, 600, 900, 255))
	backdrop := picture(t, 320, 180, 255)
	write("Movie (2008) 1080p-fanart.png", backdrop)
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008)/Movie (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}

	movie := details()
	poster := movie.ImageTags["Primary"]
	if poster == "" || poster == addonPoster || len(movie.BackdropImageTags) != 1 {
		t.Fatalf("local artwork: %v %v", movie.ImageTags, movie.BackdropImageTags)
	}
	// The test server itself answers on the loopback interface: what must
	// not show is the folder's path, and the URLs standing for its images.
	status, body := s.call(http.MethodGet, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, app("tv", p.token), nil)
	status2, listed := s.call(http.MethodGet, "/Items/"+p.movie+"/Images", app("tv", p.token), nil)
	for _, answer := range []string{string(body), string(listed)} {
		if status != http.StatusOK || status2 != http.StatusOK || strings.Contains(answer, dir) || strings.Contains(answer, "polyfin-local") ||
			strings.Contains(answer, "poster.png") {
			t.Errorf("a path of the server shows: %d %d %s", status, status2, answer)
		}
	}

	// The poster, resized to the width asked for, rounded up to a step.
	response, served := fetchURL(t, p.url+"/Items/"+p.movie+"/Images/Primary?maxWidth=200&tag="+poster, nil)
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/png" ||
		response.Header.Get("ETag") != `"`+poster+`"` {
		t.Fatalf("poster: %d %v", response.StatusCode, response.Header)
	}
	if config, err := png.DecodeConfig(strings.NewReader(served)); err != nil || config.Width != 240 || config.Height != 360 {
		t.Errorf("resized poster: %+v %v", config, err)
	}
	if response, served := fetchURL(t, p.url+"/Items/"+p.movie+"/Images/Backdrop/0", nil); response.StatusCode != http.StatusOK ||
		served != string(backdrop) {
		t.Errorf("backdrop: %d %d bytes", response.StatusCode, len(served))
	}

	// Replaced, the poster has another tag, and is served as it is now.
	write("poster.png", picture(t, 300, 450, 255))
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	replaced := details().ImageTags["Primary"]
	if replaced == poster || replaced == addonPoster {
		t.Fatalf("replaced poster: %q, was %q", replaced, poster)
	}
	response, served = fetchURL(t, p.url+"/Items/"+p.movie+"/Images/Primary?tag="+replaced, nil)
	if config, err := png.DecodeConfig(strings.NewReader(served)); response.StatusCode != http.StatusOK || err != nil || config.Width != 300 {
		t.Errorf("replaced poster: %d %+v %v", response.StatusCode, config, err)
	}

	// Off, the addon's artwork is back.
	if _, err := folders.Update(t.Context(), addon.ID, localfiles.Changes{UseArtwork: new(false)}); err != nil {
		t.Fatal(err)
	}
	if movie := details(); movie.ImageTags["Primary"] != addonPoster || len(movie.BackdropImageTags) != 0 {
		t.Errorf("with the option off: %v %v", movie.ImageTags, movie.BackdropImageTags)
	}
}

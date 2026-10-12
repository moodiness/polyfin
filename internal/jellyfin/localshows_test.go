package jellyfin

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/webdav"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// localShow is a test server, its ffprobe at ffprobe, with a shows folder
// holding the test series Blu-ray disc of package localfiles as Season
// 01's Disc 1 of "Show", linked to tt3000, served as a WebDAV share when
// share is set: its playlist 00000 plays all, 00001 (185 seconds) and
// 00002 (200 seconds) are episodes 1 and 2, given by hand, and 00003 (30
// seconds) is an extra. It returns the show's episodes, by number.
func localShow(t *testing.T, ffprobe string, share bool) (playbackSetup, map[int]BaseItemDto) {
	t.Helper()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := playingOn(t, s)
	dir := t.TempDir()
	unpackDisc(t, "series-bluray.tar.gz", filepath.Join(dir, "Show", "Season 01", "Disc 1"))
	location := dir
	if share {
		server := httptest.NewServer(&webdav.Handler{FileSystem: webdav.Dir(dir), LockSystem: webdav.NewMemLS()})
		t.Cleanup(server.Close)
		location = server.URL + "/"
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shows", Path: location, Kind: localfiles.KindShows})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Show", "tt3000"); err != nil {
		t.Fatal(err)
	}
	playlist := "Show/Season 01/Disc 1/BDMV/PLAYLIST/"
	if err := folders.SetEpisodes(t.Context(), addon.ID, []localfiles.EpisodeChange{{Path: playlist + "00001.mpls", Episode: "S01E01"},
		{Path: playlist + "00002.mpls", Episode: "S01E02"}}); err != nil {
		t.Fatal(err)
	}
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	var page QueryResult
	for _, view := range views.Items {
		if view.Name == "Shows" {
			p.get(t, "/Items?ParentId="+view.Id, p.token, &page)
		}
	}
	if len(page.Items) != 1 {
		t.Fatalf("the shows: %+v", page.Items)
	}
	var episodes QueryResult
	p.get(t, "/Shows/"+page.Items[0].Id+"/Episodes?UserId="+p.user.ID.String(), p.token, &episodes)
	byNumber := map[int]BaseItemDto{}
	for _, episode := range episodes.Items {
		var details BaseItemDto
		p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+episode.Id, p.token, &details)
		if details.IndexNumber == nil || details.ParentIndexNumber == nil || *details.ParentIndexNumber != 1 {
			t.Fatalf("an episode: %+v", details)
		}
		byNumber[*details.IndexNumber] = details
	}
	if len(byNumber) != 2 {
		t.Fatalf("episodes: %+v", episodes.Items)
	}
	return p, byNumber
}

// The titles of a show's Blu-ray disc in a shows folder, once given their
// episodes, are those episodes' versions through the Jellyfin API, named
// after the disc and their playlist, which play over HLS only; the play
// all title and the extra are none.
func TestLocalShowDiscsAreEpisodes(t *testing.T) {
	t.Parallel()
	p, episodes := localShow(t, "ffprobe-not-installed", false)
	for number, path := range map[int]string{1: "Season 01 - Disc 1 - title 1.m2ts", 2: "Season 01 - Disc 1 - title 2.m2ts"} {
		sources := episodes[number].MediaSources
		if sources == nil || len(*sources) != 1 {
			t.Fatalf("episode %d: %+v", number, sources)
		}
		source := (*sources)[0]
		var version BaseItemDto
		if p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+source.Id, p.token, &version); version.Path != path || source.Name != "Shows · 6 MB" ||
			source.SupportsDirectPlay || source.SupportsDirectStream {
			t.Errorf("episode %d's version: %q %+v", number, version.Path, source)
		}
	}
}

// Each episode of a show's Blu-ray disc plays its own playlist through
// FFmpeg's bluray protocol, its video copied: episode 1 its 185 seconds,
// episode 2 its 200, whole and from a seek (see playsCopiedFor).
func TestLocalShowDiscEpisodesPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := discFFmpeg(t, "-protocols", "bluray")
	playsEpisodes(t, ffmpeg, ffprobe, false)
}

// On a WebDAV share, each episode of a show's Blu-ray disc plays its
// playlist as one MPEG-TS file Polyfin serves FFmpeg from the share, its
// video copied, whole and from a seek.
func TestShareShowDiscEpisodesPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := discFFmpeg(t, "-demuxers", "mpegts")
	playsEpisodes(t, ffmpeg, ffprobe, true)
}

// playsEpisodes plays both episodes of localShow (see playsCopiedFor).
func playsEpisodes(t *testing.T, ffmpeg, ffprobe string, share bool) {
	t.Helper()
	p, episodes := localShow(t, ffprobe, share)
	for number, seconds := range map[int]int{1: 185, 2: 200} {
		episode := episodes[number]
		if sources := episode.MediaSources; sources == nil || len(*sources) != 1 {
			t.Fatalf("episode %d: %+v", number, sources)
		}
		playsCopiedFor(t, p, episode.Id, (*episode.MediaSources)[0], seconds, []string{"Video h264 ", "Audio ac3 "}, ffmpeg, ffprobe)
	}
}

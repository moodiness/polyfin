package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// extrasSetup is a server with a local folder of kind holding the media
// files made under dir, scanned, its unit linked to imdb.
type extrasSetup struct {
	playbackSetup
	folders *localfiles.Service
	folder  addons.Addon
	dir     string
}

// newExtrasSetup makes the media of files with the FFmpeg of
// POLYFIN_TEST_FFMPEG, skipping the test without it: a video for each
// video name, a song for each audio name, distinct by their lengths in
// seconds. It adds them as a local folder of kind, scans it and links unit
// to imdb.
func newExtrasSetup(t *testing.T, kind string, files map[string]int, unit, imdb string) extrasSetup {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	dir := t.TempDir()
	for rel, seconds := range files {
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		length := strconv.Itoa(seconds)
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=" + length}
		if strings.HasSuffix(rel, ".mp3") {
			args = append(args, "-c:a", "libmp3lame", "-b:a", "64k", target)
		} else {
			args = append(args, "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=1:duration="+length, "-map", "1:v", "-map", "0:a",
				"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", target)
		}
		if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg %s: %v %s", rel, err, out)
		}
	}
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := playingOn(t, s)
	folder, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: kind})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), folder.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), folder.ID, unit, imdb); err != nil {
		t.Fatal(err)
	}
	return extrasSetup{playbackSetup: p, folders: folders, folder: folder, dir: dir}
}

// extraNames lists the names and extra types of items, sorted.
func extraNames(items []BaseItemDto) []string {
	var names []string
	for _, item := range items {
		names = append(names, item.Name+" "+item.Type+" "+item.ExtraType)
	}
	slices.Sort(names)
	return names
}

// A movie's extras, beside it in its folder, are its special features,
// local trailer and theme video, none of them a title nor a version; each
// plays as a local file, with a resume point of its own: the making-of
// lasts long enough to keep one (see userdata.MinResumeDuration). With the
// folder's setting off, a scan reads the folder as it did before extras
// were read.
func TestLocalMovieExtras(t *testing.T) {
	t.Parallel()
	x := newExtrasSetup(t, localfiles.KindMovies, map[string]int{
		"Movie (2008)/Movie (2008).mkv":            12,
		"Movie (2008)/Extras/Making of.mkv":        400,
		"Movie (2008)/Movie (2008)-featurette.mkv": 9,
		"Movie (2008)/trailer.mp4":                 8,
		"Movie (2008)/backdrops/intro.mkv":         7,
	}, "Movie (2008)/Movie (2008).mkv", "tt1000")
	p := x.playbackSetup
	user := p.user.ID.String()

	var movie BaseItemDto
	p.get(t, "/Users/"+user+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("the movie's versions: %+v", movie.MediaSources)
	}
	if movie.LocalTrailerCount == nil || *movie.LocalTrailerCount != 1 || movie.SpecialFeatureCount == nil || *movie.SpecialFeatureCount != 2 {
		t.Errorf("counts: %v %v", movie.LocalTrailerCount, movie.SpecialFeatureCount)
	}
	folder, err := x.folders.Folder(t.Context(), x.folder.ID)
	if err != nil || folder.Files != 1 || folder.Unmatched != 0 || folder.Extras != 4 || !folder.ReadExtras {
		t.Fatalf("the folder: %+v %v", folder, err)
	}

	var features []BaseItemDto
	p.get(t, "/Items/"+p.movie+"/SpecialFeatures?userId="+user, p.token, &features)
	if got := extraNames(features); !slices.Equal(got, []string{"Making of Video Unknown", "Movie (2008) Video Featurette"}) {
		t.Errorf("special features: %v", got)
	}
	for _, feature := range features {
		if feature.ParentId != p.movie || feature.MediaType != "Video" {
			t.Errorf("a special feature: %+v", feature)
		}
	}
	var trailers []BaseItemDto
	p.get(t, "/Users/"+user+"/Items/"+p.movie+"/LocalTrailers", p.token, &trailers)
	if got := extraNames(trailers); !slices.Equal(got, []string{"trailer Video Trailer"}) || trailers[0].ParentId != p.movie {
		t.Errorf("local trailers: %v", got)
	}
	var videos themeMediaResult
	p.get(t, "/Items/"+p.movie+"/ThemeVideos", p.token, &videos)
	if got := extraNames(videos.Items); !slices.Equal(got, []string{"intro Video ThemeVideo"}) || videos.OwnerId != p.movie {
		t.Errorf("theme videos: %+v", videos)
	}
	// The movie's library lists the movie alone.
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	for _, view := range views.Items {
		if view.Name == "Shelf" {
			var page QueryResult
			if p.get(t, "/Items?ParentId="+view.Id+"&Recursive=true", p.token, &page); len(page.Items) != 1 || page.Items[0].Id != p.movie {
				t.Errorf("the folder's library: %+v", page.Items)
			}
		}
	}

	// A user whose parental control hides the movie reaches none of its
	// extras.
	child := p.testServer.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(0)} })
	childToken := p.signIn("child", "tv")
	for _, path := range []string{"/Items/" + p.movie + "/SpecialFeatures", "/Users/" + child.ID.String() + "/Items/" + features[0].Id,
		"/Users/" + child.ID.String() + "/Items/" + p.movie + "/LocalTrailers"} {
		if status, data := p.call(http.MethodGet, path, app("tv", childToken), nil); status != http.StatusNotFound {
			t.Errorf("%s for a child: %d %s", path, status, data)
		}
	}

	// An extra plays its file, and keeps its own resume point.
	making := features[0]
	var details BaseItemDto
	p.get(t, "/Users/"+user+"/Items/"+making.Id, p.token, &details)
	if details.Type != "Video" || details.ExtraType != "Unknown" || details.ParentId != p.movie {
		t.Errorf("the extra's details: %+v", details)
	}
	status, data := p.call(http.MethodPost, "/Items/"+making.Id+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": user, "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	source := info.MediaSources[0]
	if source.RunTimeTicks == nil || *source.RunTimeTicks < 3_990_000_000 || *source.RunTimeTicks > 4_010_000_000 || !source.SupportsDirectPlay && source.TranscodingUrl == "" {
		t.Errorf("the extra's source: %+v", source)
	}
	file, err := os.ReadFile(filepath.Join(x.dir, "Movie (2008)", "Extras", "Making of.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	response, body := fetchURL(t, p.url+"/Items/"+making.Id+"/Download?ApiKey="+p.token, nil)
	if response.StatusCode != http.StatusOK || body != string(file) {
		t.Errorf("the extra's file: %d, %d bytes", response.StatusCode, len(body))
	}
	if status, data := p.call(http.MethodPost, "/Sessions/Playing/Stopped", app("tv", p.token), map[string]any{
		"ItemId": making.Id, "MediaSourceId": source.Id, "PositionTicks": 1_200_000_000}); status != http.StatusNoContent {
		t.Fatalf("stopped: %d %s", status, data)
	}
	p.get(t, "/Users/"+user+"/Items/"+making.Id, p.token, &details)
	if details.UserData.PlaybackPositionTicks != 1_200_000_000 || details.UserData.Played {
		t.Errorf("the extra's resume point: %+v", details.UserData)
	}
	p.get(t, "/Users/"+user+"/Items/"+p.movie, p.token, &movie)
	if movie.UserData.PlaybackPositionTicks != 0 {
		t.Errorf("the movie's resume point: %+v", movie.UserData)
	}

	// Off, the next scan reads the folder as before extras were read: the
	// extras folder is left out, the others are files of their own.
	if _, err := x.folders.Update(t.Context(), x.folder.ID, localfiles.Changes{ReadExtras: new(false)}); err != nil {
		t.Fatal(err)
	}
	if err := x.folders.Scan(t.Context(), x.folder.ID); err != nil {
		t.Fatal(err)
	}
	folder, err = x.folders.Folder(t.Context(), x.folder.ID)
	if err != nil || folder.Files != 4 || folder.Extras != 0 || folder.ReadExtras {
		t.Fatalf("the folder without extras: %+v %v", folder, err)
	}
	p.get(t, "/Items/"+p.movie+"/SpecialFeatures", p.token, &features)
	p.get(t, "/Items/"+p.movie+"/LocalTrailers", p.token, &trailers)
	if len(features) != 0 || len(trailers) != 0 {
		t.Errorf("extras without them: %v %v", features, trailers)
	}
	p.get(t, "/Users/"+user+"/Items/"+p.movie, p.token, &movie)
	if *movie.LocalTrailerCount != 0 || *movie.SpecialFeatureCount != 0 {
		t.Errorf("counts without extras: %d %d", *movie.LocalTrailerCount, *movie.SpecialFeatureCount)
	}
	if status, _ := p.call(http.MethodGet, "/Users/"+user+"/Items/"+making.Id, app("tv", p.token), nil); status != http.StatusNotFound {
		t.Errorf("an extra gone: %d", status)
	}
}

// A series' theme song, in its folder, is its theme media, which its
// seasons inherit; it plays as audio.
func TestLocalShowThemeSongs(t *testing.T) {
	t.Parallel()
	x := newExtrasSetup(t, localfiles.KindShows, map[string]int{
		"Show/Season 01/Show S01E01.mkv": 6,
		"Show/theme.mp3":                 5,
	}, "Show", "tt3000")
	p := x.playbackSetup
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	var shows QueryResult
	for _, view := range views.Items {
		if view.Name == "Shelf" {
			p.get(t, "/Items?ParentId="+view.Id, p.token, &shows)
		}
	}
	if len(shows.Items) != 1 {
		t.Fatalf("the shows: %+v", shows.Items)
	}
	series := shows.Items[0].Id
	var songs themeMediaResult
	p.get(t, "/Items/"+series+"/ThemeSongs", p.token, &songs)
	if got := extraNames(songs.Items); !slices.Equal(got, []string{"theme Audio ThemeSong"}) || songs.OwnerId != series ||
		songs.Items[0].MediaType != "Audio" || songs.Items[0].ParentId != series {
		t.Fatalf("theme songs: %+v", songs)
	}
	var seasons QueryResult
	p.get(t, "/Shows/"+series+"/Seasons?userId="+p.user.ID.String(), p.token, &seasons)
	if len(seasons.Items) == 0 {
		t.Fatal("no season")
	}
	season := seasons.Items[0].Id
	var media struct {
		ThemeVideosResult, ThemeSongsResult themeMediaResult
	}
	p.get(t, "/Items/"+season+"/ThemeMedia?inheritFromParent=true&sortBy=Random", p.token, &media)
	if len(media.ThemeSongsResult.Items) != 1 || media.ThemeSongsResult.OwnerId != series || len(media.ThemeVideosResult.Items) != 0 {
		t.Errorf("the season's inherited theme media: %+v", media)
	}
	p.get(t, "/Items/"+season+"/ThemeSongs", p.token, &songs)
	if len(songs.Items) != 0 || songs.OwnerId != season {
		t.Errorf("the season's own theme songs: %+v", songs)
	}
	var page QueryResult
	if p.get(t, "/Shows/"+series+"/Episodes?userId="+p.user.ID.String(), p.token, &page); len(page.Items) == 0 {
		t.Fatal("no episode")
	}

	p.get(t, "/Items/"+series+"/ThemeSongs", p.token, &songs)
	status, data := p.call(http.MethodPost, "/Items/"+songs.Items[0].Id+"/PlaybackInfo", app("tv", p.token), map[string]any{"UserId": p.user.ID.String()})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 || info.MediaSources[0].Path == "" {
		t.Fatalf("the theme song's PlaybackInfo: %d %s", status, data)
	}
	file, err := os.ReadFile(filepath.Join(x.dir, "Show", "theme.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	if response, body := fetchURL(t, info.MediaSources[0].Path, nil); response.StatusCode != http.StatusOK || body != string(file) {
		t.Errorf("the theme song's file: %d, %d bytes", response.StatusCode, len(body))
	}
}

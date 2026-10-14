package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/loudness"
	"github.com/moodiness/polyfin/internal/stremio"
)

// countedFFmpeg is an FFmpeg that runs ffmpeg and counts its runs.
func countedFFmpeg(t *testing.T, ffmpeg string) (string, func() int) {
	t.Helper()
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs")
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho run >> '"+runs+"'\nexec '"+ffmpeg+"' \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, func() int {
		data, err := os.ReadFile(runs)
		if err != nil {
			return 0
		}
		return strings.Count(string(data), "run")
	}
}

// gainsOf lists the gains of a listing's items by name, none for those
// without one.
func gainsOf(items []BaseItemDto) map[string]float64 {
	gains := map[string]float64{}
	for _, item := range items {
		if item.NormalizationGain != nil {
			gains[item.Name] = *item.NormalizationGain
		}
	}
	return gains
}

// lufsEnabled reports, by library name, whether the library options of the
// music libraries enable LUFS scans.
func lufsEnabled(t *testing.T, s testServer, token string) map[string]bool {
	t.Helper()
	status, body := s.call(http.MethodGet, "/Library/VirtualFolders", app("web", token), nil)
	var folders []VirtualFolderInfo
	if status != http.StatusOK || json.Unmarshal(body, &folders) != nil {
		t.Fatalf("virtual folders: %d %s", status, body)
	}
	enabled := map[string]bool{}
	for _, folder := range folders {
		enabled[folder.Name] = folder.LibraryOptions.EnableLUFSScan
	}
	return enabled
}

// The songs and albums of a local music folder carry the gains of their
// ReplayGain tags in listings and details, and none while Even the volume
// between songs is off. Music libraries report LUFS scans as measuring is
// set.
func TestSongsAndAlbumsCarryTheirGains(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
		folders.UseTools(ffprobe, ffmpeg)
	})
	dir := t.TempDir()
	for _, song := range []struct{ file, title, gain string }{{"01 First.flac", "First Light", "-3.50 dB"}, {"02 Second.flac", "Second Wind", "-1.25 dB"}} {
		path := filepath.Join(dir, "Tone Quartet", "Sine Studies", song.file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "sine=duration=1", "-metadata", "title="+song.title,
			"-metadata", "artist=Tone Quartet", "-metadata", "album=Sine Studies", "-metadata", "REPLAYGAIN_TRACK_GAIN="+song.gain,
			"-metadata", "REPLAYGAIN_ALBUM_GAIN=-2.00 dB", path).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Records", Path: dir, Kind: localfiles.KindMusic})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	s.user("listener", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token := s.signIn("listener", "web")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	if len(views.Items) != 1 {
		t.Fatalf("libraries: %+v", views.Items)
	}
	records := views.Items[0].Id

	describe := func() (albums, songs map[string]float64, details float64, ok bool) {
		t.Helper()
		var list QueryResult
		s.get(t, "/Items?ParentId="+records+"&IncludeItemTypes=MusicAlbum&Recursive=true", token, &list)
		if len(list.Items) != 1 {
			t.Fatalf("albums: %+v", list.Items)
		}
		albums = gainsOf(list.Items)
		album := list.Items[0].Id
		s.get(t, "/Items?ParentId="+album, token, &list)
		songs = gainsOf(list.Items)
		var item BaseItemDto
		s.get(t, "/Items/"+album, token, &item)
		if item.NormalizationGain != nil {
			details, ok = *item.NormalizationGain, true
		}
		return albums, songs, details, ok
	}
	albums, songs, details, ok := describe()
	if albums["Sine Studies"] != -2 || len(songs) != 2 || songs["First Light"] != -3.5 || songs["Second Wind"] != -1.25 || !ok || details != -2 {
		t.Errorf("gains: albums %v, songs %v, details %v %v", albums, songs, details, ok)
	}
	if lufsEnabled(t, s, token)["Records"] {
		t.Error("LUFS scans with measuring off")
	}
	s.setting(t, func(settings *accounts.Settings) { settings.MeasureLoudness = "local" })
	if !lufsEnabled(t, s, token)["Records"] {
		t.Error("no LUFS scans while local songs are measured")
	}

	s.setting(t, func(settings *accounts.Settings) { settings.EvenVolume = false })
	if albums, songs, _, ok := describe(); len(albums) != 0 || len(songs) != 0 || ok {
		t.Errorf("gains with the switch off: albums %v, songs %v, details %v", albums, songs, ok)
	}
	if lufsEnabled(t, s, token)["Records"] {
		t.Error("LUFS scans with the switch off")
	}
}

// An addon's song is measured once after its first play, through playback,
// and its gain served after; its album's, once both its songs are. An
// audiobook is never measured.
func TestAddonSongsAreMeasuredAfterTheirFirstPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, flac := tone(t)
	meter, runs := countedFFmpeg(t, ffmpeg)
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), func(o *Options, pool *pgxpool.Pool) {
		o.Loudness = loudness.New(pool, loudness.NewMeter(meter), slog.New(slog.NewTextHandler(io.Discard, nil)), o.Accounts.Settings)
		t.Cleanup(o.Loudness.Close)
	})
	addon := newFakeEclipse(t, flac)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addon.url, false); err != nil {
		t.Fatal(err)
	}
	books := newFakeEclipse(t, flac)
	books.content = "audiobook"
	if _, err := s.addons.Install(t.Context(), addons.Shared(), books.url, false); err != nil {
		t.Fatal(err)
	}
	s.user("listener", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token := s.signIn("listener", "web")
	s.setting(t, func(settings *accounts.Settings) { settings.MeasureLoudness = "all" })

	// The music addon's row of albums; the audiobook addon's rows are books
	// libraries.
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var library, name string
	for _, view := range views.Items {
		if view.CollectionType == "music" && strings.HasPrefix(view.Name, "New Releases") {
			library, name = view.Id, view.Name
		}
	}
	enabled := lufsEnabled(t, s, token)
	if !enabled[name] {
		t.Errorf("an addon's music library without LUFS scans: %v", enabled)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.MeasureLoudness = "local" })
	if enabled := lufsEnabled(t, s, token); enabled[name] {
		t.Errorf("an addon's music library with LUFS scans while local songs alone are measured: %v", enabled)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.MeasureLoudness = "all" })
	albumsOf := "/Items?ParentId=" + library + "&IncludeItemTypes=MusicAlbum&Recursive=true&SortBy=SortName"
	var albums QueryResult
	s.get(t, albumsOf, token, &albums)
	if len(albums.Items) != 2 || albums.Items[1].Name != "Sine Studies" {
		t.Fatalf("albums: %+v", albums.Items)
	}
	sine := albums.Items[1].Id
	var songs QueryResult
	s.get(t, "/Items?ParentId="+sine, token, &songs)
	if len(songs.Items) != 2 || len(gainsOf(songs.Items)) != 0 {
		t.Fatalf("songs before a play: %+v", songs.Items)
	}
	play := func(id string) {
		t.Helper()
		if status, body := s.call(http.MethodPost, "/Sessions/Playing", app("web", token), map[string]any{"ItemId": id}); status != http.StatusNoContent {
			t.Fatalf("playing: %d %s", status, body)
		}
		s.handler.Loudness.Wait()
	}
	play(songs.Items[0].Id)
	play(songs.Items[0].Id)
	if n := runs(); n != 1 {
		t.Errorf("FFmpeg ran %d times for one song played twice", n)
	}
	// The gain of the tone as the meter reads its file.
	measured, err := loudness.NewMeter(ffmpeg).Measure(t.Context(), flac)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := loudness.Gain(measured)
	s.get(t, "/Items?ParentId="+sine, token, &songs)
	gains := gainsOf(songs.Items)
	if len(gains) != 1 || math.Abs(gains[songs.Items[0].Name]-want) > 0.3 {
		t.Errorf("gains after a play: %v, want %.2f for %s", gains, want, songs.Items[0].Name)
	}
	s.get(t, albumsOf, token, &albums)
	if gains := gainsOf(albums.Items); len(gains) != 0 {
		t.Errorf("an album with a song unknown has a gain: %v", gains)
	}
	play(songs.Items[1].Id)
	s.get(t, albumsOf, token, &albums)
	if gains := gainsOf(albums.Items); len(gains) != 1 || math.Abs(gains["Sine Studies"]-want) > 0.3 {
		t.Errorf("the album's gain once its songs are known: %v", gains)
	}
	var details BaseItemDto
	s.get(t, "/Items/"+songs.Items[1].Id, token, &details)
	if details.NormalizationGain == nil || math.Abs(*details.NormalizationGain-want) > 0.3 {
		t.Errorf("the song's details: %v", details.NormalizationGain)
	}

	// An audiobook is never measured.
	before := runs()
	var chapters QueryResult
	s.get(t, "/Items?IncludeItemTypes=AudioBook&Recursive=true", token, &chapters)
	if len(chapters.Items) == 0 {
		t.Fatal("no audiobooks")
	}
	play(chapters.Items[0].Id)
	if n := runs(); n != before {
		t.Errorf("FFmpeg ran %d times for an audiobook", n-before)
	}
}

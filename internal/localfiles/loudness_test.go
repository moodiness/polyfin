package localfiles

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/loudness"
)

// sine makes a mono sine of seconds at path, at amplitude, with tags, in
// the format its extension tells: EBU R128 reads it at
// 20·log10(amplitude) - 3.01 LUFS.
func sine(t *testing.T, ffmpeg, path string, amplitude float64, seconds int, tags map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=1000:duration=" + strconv.Itoa(seconds),
		"-af", "volume=" + strconv.FormatFloat(amplitude*8, 'f', -1, 64)}
	for key, value := range tags {
		args = append(args, "-metadata", key+"="+value)
	}
	switch filepath.Ext(path) {
	case ".m4a":
		args = append(args, "-c:a", "aac", "-movflags", "use_metadata_tags")
	case ".mp3":
		args = append(args, "-c:a", "libmp3lame", "-id3v2_version", "3")
	case ".opus":
		args = append(args, "-c:a", "libopus")
	}
	if out, err := exec.Command(ffmpeg, append(args, path)...).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// lufs is the loudness EBU R128 reads of a mono sine at amplitude.
func lufs(amplitude float64) float64 { return 20*math.Log10(amplitude) - 3.01 }

// albumTracks returns a music folder's album and its tracks by title.
func (e env) albumTracks(id accounts.ID, album string) (eclipse.Album, map[string]eclipse.Track) {
	e.t.Helper()
	found, ok := e.albumTitles(id)[album]
	if !ok {
		e.t.Fatalf("no album %s", album)
	}
	page, err := e.service.MusicAlbum(e.t.Context(), id, found.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	tracks := map[string]eclipse.Track{}
	for _, track := range page.Tracks {
		tracks[track.Title] = track
	}
	return found, tracks
}

// near reports whether a gain is known and within a tenth of a dB of want.
func near(gain *float64, want float64) bool {
	return gain != nil && math.Abs(*gain-want) < 0.1
}

// A scan reads the ReplayGain tags of ID3, Vorbis comments and MP4 alike,
// and moves Opus's R128 gains to the same reference. An album's gain is
// the album tag its tracks agree on. The tracks read before gains were are
// read again by the next scan, once.
func TestReplayGainTagsAreRead(t *testing.T) {
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	dir := t.TempDir()
	album := func(title string) map[string]string {
		return map[string]string{"artist": "Band", "album": title}
	}
	with := func(tags map[string]string, more ...string) map[string]string {
		for i := 0; i < len(more); i += 2 {
			tags[more[i]] = more[i+1]
		}
		return tags
	}
	sine(t, ffmpeg, filepath.Join(dir, "Band", "Tagged", "01 One.flac"), 0.125, 1, with(album("Tagged"), "title", "One",
		"REPLAYGAIN_TRACK_GAIN", "-3.50 dB", "REPLAYGAIN_ALBUM_GAIN", "-4.00 dB"))
	sine(t, ffmpeg, filepath.Join(dir, "Band", "Tagged", "02 Two.mp3"), 0.125, 1, with(album("Tagged"), "title", "Two",
		"REPLAYGAIN_TRACK_GAIN", "-2.00 dB", "REPLAYGAIN_ALBUM_GAIN", "-4.00 dB"))
	sine(t, ffmpeg, filepath.Join(dir, "Band", "Tagged", "03 Three.m4a"), 0.125, 1, with(album("Tagged"), "title", "Three",
		"REPLAYGAIN_TRACK_GAIN", "+1.25 dB", "REPLAYGAIN_ALBUM_GAIN", "-4.00 dB"))
	sine(t, ffmpeg, filepath.Join(dir, "Band", "Opus", "01 Four.opus"), 0.125, 1, with(album("Opus"), "title", "Four",
		"R128_TRACK_GAIN", "-1280", "R128_ALBUM_GAIN", "256"))
	sine(t, ffmpeg, filepath.Join(dir, "Band", "Mixed", "01 Five.flac"), 0.125, 1, with(album("Mixed"), "title", "Five",
		"REPLAYGAIN_ALBUM_GAIN", "-1.00 dB"))
	sine(t, ffmpeg, filepath.Join(dir, "Band", "Mixed", "02 Six.flac"), 0.125, 1, with(album("Mixed"), "title", "Six",
		"REPLAYGAIN_ALBUM_GAIN", "-2.00 dB"))
	id := e.musicFolder(dir)

	tagged, tracks := e.albumTracks(id, "Tagged by Band")
	if !near(tracks["One"].Gain, -3.5) || !near(tracks["Two"].Gain, -2) || !near(tracks["Three"].Gain, 1.25) || !near(tagged.Gain, -4) {
		t.Errorf("ReplayGain tags: album %v, tracks %v %v %v", tagged.Gain, tracks["One"].Gain, tracks["Two"].Gain, tracks["Three"].Gain)
	}
	opus, tracks := e.albumTracks(id, "Opus by Band")
	if !near(tracks["Four"].Gain, 0) || !near(opus.Gain, 6) {
		t.Errorf("R128 tags: album %v, track %v", opus.Gain, tracks["Four"].Gain)
	}
	// Tracks that disagree on their album gain give none, nor do tracks
	// without a gain of their own while unmeasured.
	mixed, tracks := e.albumTracks(id, "Mixed by Band")
	if mixed.Gain != nil || tracks["Five"].Gain != nil {
		t.Errorf("an album whose tracks disagree: %v %v", mixed.Gain, tracks["Five"].Gain)
	}

	// A track read before gains were is read again once.
	if _, err := e.pool.Exec(t.Context(), `UPDATE local_tracks SET track_gain = NULL, album_gain = NULL, gains_read = false
		WHERE addon_id = $1 AND path = 'Band/Tagged/01 One.flac'`, id); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, tracks := e.albumTracks(id, "Tagged by Band"); !near(tracks["One"].Gain, -3.5) {
		t.Errorf("the track read again: %v", tracks["One"].Gain)
	}
	if _, err := e.pool.Exec(t.Context(), `UPDATE local_tracks SET track_gain = 9 WHERE addon_id = $1 AND path = 'Band/Tagged/01 One.flac'`,
		id); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, tracks := e.albumTracks(id, "Tagged by Band"); !near(tracks["One"].Gain, 9) {
		t.Errorf("an unchanged track was read again: %v", tracks["One"].Gain)
	}
}

// countedFFmpeg is an FFmpeg that runs ffmpeg and counts its runs.
func countedFFmpeg(t *testing.T, ffmpeg string) (string, func() int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the counting FFmpeg is a shell script")
	}
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

// serveFiles serves the folders' files to FFmpeg, as playback's loopback
// interface does.
func (e env) serveFiles() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, modified, err := e.service.Open(r.Context(), strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		http.ServeContent(w, r, "", modified, file)
	}))
	e.t.Cleanup(server.Close)
	e.service.UseFiles(server.URL+"/", "")
}

// measure sets which songs are measured.
func (e env) measure(which string) {
	e.t.Helper()
	settings := e.users.Settings()
	settings.MeasureLoudness = which
	if _, err := e.users.UpdateSettings(e.t.Context(), settings); err != nil {
		e.t.Fatal(err)
	}
}

// rescan scans a music folder and waits for the measures that follow.
func (e env) rescan(id accounts.ID) {
	e.t.Helper()
	if err := e.service.Scan(e.t.Context(), id); err != nil {
		e.t.Fatal(err)
	}
	e.service.measures.Wait()
}

// With measuring on, the tracks without a track gain are measured after a
// scan, once, and again once their file changes; those longer than 15
// minutes never are. An album's gain is its tracks' loudness together
// once each is known. With measuring off, FFmpeg never runs.
func TestLocalTracksAreMeasured(t *testing.T) {
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	meter, runs := countedFFmpeg(t, ffmpeg)
	e.service.UseMeter(loudness.NewMeter(meter))
	e.serveFiles()
	dir := t.TempDir()
	tags := func(title, album string, more ...string) map[string]string {
		tags := map[string]string{"title": title, "artist": "Band", "album": album}
		for i := 0; i < len(more); i += 2 {
			tags[more[i]] = more[i+1]
		}
		return tags
	}
	sine(t, ffmpeg, filepath.Join(dir, "Band", "Mixed", "01 Tagged.flac"), 0.125, 3, tags("Tagged", "Mixed", "REPLAYGAIN_TRACK_GAIN", "-3.00 dB"))
	plain := filepath.Join(dir, "Band", "Mixed", "02 Plain.flac")
	sine(t, ffmpeg, plain, 0.5, 3, tags("Plain", "Mixed"))
	long := filepath.Join(dir, "Band", "Long", "01 Long.flac")
	if err := os.MkdirAll(filepath.Dir(long), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono", "-t", "901",
		"-metadata", "title=Long", "-metadata", "artist=Band", "-metadata", "album=Long", long).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	id := e.musicFolder(dir)
	e.service.measures.Wait()
	if n := runs(); n != 0 {
		t.Errorf("FFmpeg ran %d times with measuring off", n)
	}
	album, tracks := e.albumTracks(id, "Mixed by Band")
	if album.Gain != nil || tracks["Plain"].Gain != nil || !near(tracks["Tagged"].Gain, -3) {
		t.Errorf("before measuring: album %v, tracks %v %v", album.Gain, tracks["Tagged"].Gain, tracks["Plain"].Gain)
	}

	e.measure("local")
	e.rescan(id)
	if n := runs(); n != 1 {
		t.Errorf("FFmpeg ran %d times, once expected: the long and the tagged tracks are not measured", n)
	}
	album, tracks = e.albumTracks(id, "Mixed by Band")
	together := 10 * math.Log10((math.Pow(10, (loudness.Reference+3)/10)+math.Pow(10, lufs(0.5)/10))/2)
	if !near(tracks["Plain"].Gain, loudness.Reference-lufs(0.5)) || !near(tracks["Tagged"].Gain, -3) ||
		!near(album.Gain, loudness.Reference-together) {
		t.Errorf("measured: album %v (want %.2f), tracks %v %v", album.Gain, loudness.Reference-together, tracks["Tagged"].Gain,
			tracks["Plain"].Gain)
	}
	if _, tracks := e.albumTracks(id, "Long by Band"); tracks["Long"].Gain != nil {
		t.Errorf("the long track has a gain: %v", *tracks["Long"].Gain)
	}
	folder, err := e.service.Folder(t.Context(), id)
	if err != nil || folder.Gained != 2 || folder.Measured != 1 {
		t.Errorf("figures: %d with a gain, %d measured: %v", folder.Gained, folder.Measured, err)
	}

	// Measured once.
	e.rescan(id)
	if n := runs(); n != 1 {
		t.Errorf("FFmpeg ran %d times: an unchanged track was measured again", n)
	}
	// A changed file is measured again.
	sine(t, ffmpeg, plain, 0.125, 3, tags("Plain", "Mixed"))
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(plain, later, later); err != nil {
		t.Fatal(err)
	}
	e.rescan(id)
	if _, tracks := e.albumTracks(id, "Mixed by Band"); runs() != 2 || !near(tracks["Plain"].Gain, loudness.Reference-lufs(0.125)) {
		t.Errorf("the changed track: %d runs, gain %v", runs(), tracks["Plain"].Gain)
	}
}

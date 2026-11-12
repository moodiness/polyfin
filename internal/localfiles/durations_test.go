package localfiles

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// movie makes a video of seconds at path with ffmpeg, with sound.
func movie(t *testing.T, ffmpeg, path string, seconds int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	length := strconv.Itoa(seconds)
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=duration="+length+":size=64x64:rate=10",
		"-f", "lavfi", "-i", "sine=duration="+length, "-c:v", "mpeg4", "-c:a", "aac", "-shortest", path).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// about reports whether d is seconds, give or take the half second a
// container's length may differ by.
func about(d time.Duration, seconds int) bool {
	want := time.Duration(seconds) * time.Second
	return d > want-time.Second/2 && d < want+time.Second/2
}

// A scan probes the videos of movies and shows folders for how long they
// play, matched or plain: once, kept while the file is the same, probed
// again once it changes. A video that cannot be probed is not tried again;
// one scanned while ffprobe cannot be run is probed at a later scan.
func TestScansProbeLengths(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	movies, shows := t.TempDir(), t.TempDir()
	movie(t, ffmpeg, filepath.Join(movies, "Nosferatu (1922)/Nosferatu (1922).mkv"), 3)
	movie(t, ffmpeg, filepath.Join(movies, "Some Movie (2019)/Some Movie (2019).mkv"), 3)
	write(t, filepath.Join(movies, "Broken (2020)/Broken (2020).mkv"), 1000)
	movie(t, ffmpeg, filepath.Join(shows, "The Cisco Kid/Season 01/The Cisco Kid S01E02.mkv"), 3)
	movie(t, ffmpeg, filepath.Join(shows, "Paper Lanterns/Season 01/Paper Lanterns S01E01.mkv"), 3)
	add := func(kind, dir string) accounts.ID {
		t.Helper()
		addon, err := e.service.Add(t.Context(), NewFolder{Name: kind, Path: dir, Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		e.idle(addon.ID)
		return addon.ID
	}
	moviesID, showsID := add(KindMovies, movies), add(KindShows, shows)
	if got := e.files(moviesID)["Nosferatu (1922)/Nosferatu (1922).mkv"]; got != "tt0013442" {
		t.Fatalf("the movie: %s", got)
	}
	if got := e.files(showsID)["The Cisco Kid/Season 01/The Cisco Kid S01E02.mkv"]; got != "tt0042084" {
		t.Fatalf("the show: %s", got)
	}
	plainOf := func(id accounts.ID, rel string) string {
		t.Helper()
		var plain string
		if err := e.pool.QueryRow(t.Context(), "SELECT plain_id FROM local_files WHERE addon_id = $1 AND path = $2", id, rel).Scan(&plain); err != nil {
			t.Fatal(err)
		}
		return plain
	}
	someMovie := plainOf(moviesID, "Some Movie (2019)/Some Movie (2019).mkv")
	broken := plainOf(moviesID, "Broken (2020)/Broken (2020).mkv")
	lanterns := plainOf(showsID, "Paper Lanterns/Season 01/Paper Lanterns S01E01.mkv")
	check := func(when string, someLength int) {
		t.Helper()
		titles, err := e.service.TitleDurations(t.Context(), []string{"tt0013442", someMovie, broken, "tt0042084", lanterns, "tt0063350"})
		if err != nil || len(titles) != 2 || !about(titles["tt0013442"], 3) || !about(titles[someMovie], someLength) {
			t.Fatalf("%s: the movies' lengths: %v %v", when, titles, err)
		}
		for show, episode := range map[string][2]int{"tt0042084": {1, 2}, lanterns: {1, 1}} {
			episodes, err := e.service.EpisodeDurations(t.Context(), show)
			if err != nil || len(episodes) != 1 || !about(episodes[episode], 3) {
				t.Fatalf("%s: the episodes' lengths of %s: %v %v", when, show, episodes, err)
			}
		}
		// Probed, and not to be again.
		var seconds *float64
		if err := e.pool.QueryRow(t.Context(), "SELECT duration FROM local_files WHERE addon_id = $1 AND path = $2", moviesID,
			"Broken (2020)/Broken (2020).mkv").Scan(&seconds); err != nil || seconds == nil || *seconds != 0 {
			t.Fatalf("%s: the broken video's duration: %v %v", when, seconds, err)
		}
	}
	check("first scan", 3)

	// No video is probed again while it is the same: the scans would store
	// 0 for each, ffprobe failing on every video (FFmpeg takes none of its
	// options).
	e.service.UseTools(ffmpeg, ffmpeg)
	for _, id := range []accounts.ID{moviesID, showsID} {
		if err := e.service.Scan(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	check("unchanged", 3)

	// A video that changed while ffprobe cannot be run is left without a
	// length, rather than with none for good: the next scan that can run
	// it probes it.
	e.service.UseTools(filepath.Join(t.TempDir(), "ffprobe"), ffmpeg)
	movie(t, ffmpeg, filepath.Join(movies, "Some Movie (2019)/Some Movie (2019).mkv"), 5)
	if err := e.service.Scan(t.Context(), moviesID); err != nil {
		t.Fatal(err)
	}
	if titles, err := e.service.TitleDurations(t.Context(), []string{someMovie}); err != nil || len(titles) != 0 {
		t.Fatalf("changed without ffprobe: %v %v", titles, err)
	}
	e.service.UseTools(ffprobe, ffmpeg)
	if err := e.service.Scan(t.Context(), moviesID); err != nil {
		t.Fatal(err)
	}
	check("changed", 5)
}

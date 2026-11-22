package localfiles

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// toldAdded collects what a service tells of the titles its scans add.
func toldAdded(s *Service) func() [][]accounts.ID {
	var mu sync.Mutex
	var told [][]accounts.ID
	s.UseAdded(func(items []accounts.ID) {
		mu.Lock()
		defer mu.Unlock()
		told = append(told, slices.Clone(items))
	})
	return func() [][]accounts.ID {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(told)
	}
}

// A folder's first scan tells nothing, however much it holds. A later scan
// tells each movie it found new once, a .strm file's included, and not
// another version of a movie the folder had; a scan finding nothing new
// tells nothing.
func TestNewMoviesAreToldAfterTheFirstScan(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	told := toldAdded(e.service)
	ctx := t.Context()
	addon, dir := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 10})
	if got := told(); len(got) != 0 {
		t.Fatalf("the first scan told %v", got)
	}

	write(t, filepath.Join(dir, "The Gold Rush (1925).mkv"), 10)
	write(t, filepath.Join(dir, "The General (1926)", "The General (1926) - 1080p.mkv"), 20)
	writeStrm(t, dir, "Nosferatu (1922).strm", "https://cdn.example/v/nosferatu.mp4", time.Now())
	if err := e.service.Scan(ctx, addon.ID); err != nil {
		t.Fatal(err)
	}
	want := [][]accounts.ID{{library.TitleItemID(library.KindMovie, "tt0013442"), library.TitleItemID(library.KindMovie, "tt0015864")}}
	if got := told(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("the second scan told %v, want Nosferatu and The Gold Rush %v", got, want)
	}

	if err := e.service.Scan(ctx, addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := told(); len(got) != 1 {
		t.Errorf("a scan finding nothing new told %v", got[1:])
	}
}

// A show's episode found after the folder's first scan is told as the
// episode's item.
func TestNewEpisodesOfLocalShowsAreTold(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	told := toldAdded(e.service)
	addon, dir := e.folder(KindShows, map[string]int{"The Cisco Kid/Season 1/The Cisco Kid S01E01.mkv": 10})
	write(t, filepath.Join(dir, "The Cisco Kid/Season 1/The Cisco Kid S01E02.mkv"), 10)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	want := [][]accounts.ID{{library.EpisodeItemID("tt0042084:1:2")}}
	if got := told(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("told %v, want the second episode %v", got, want)
	}
}

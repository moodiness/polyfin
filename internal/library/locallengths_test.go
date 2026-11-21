package library

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// lengthsLocal answers the lengths of local files, counting how often it
// is asked; the rest of Local is not used.
type lengthsLocal struct {
	Local
	asked atomic.Int32
}

func (l *lengthsLocal) TitleDurations(context.Context, []string) (map[string]time.Duration, error) {
	l.asked.Add(1)
	return map[string]time.Duration{"tt50": 90 * time.Minute}, nil
}

func (l *lengthsLocal) EpisodeDurations(context.Context, string) (map[[2]int]time.Duration, error) {
	l.asked.Add(1)
	return map[[2]int]time.Duration{{1, 1}: 45 * time.Minute}, nil
}

// The lengths of movies and episodes without one are looked for among
// local files only while the server has a local folder: listings no
// longer ask the database for them on a server without one.
func TestLocalLengthsAreLookedForOnlyWithALocalFolder(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	local := &lengthsLocal{}
	e.service.local = local
	ctx := t.Context()
	lengths := func() (time.Duration, time.Duration) {
		t.Helper()
		episodes := []Item{{Kind: KindEpisode, ParentIndexNumber: 1, IndexNumber: 1}}
		e.service.episodeLengthsOf(ctx, "tt60", episodes)
		movie := &Item{Kind: KindMovie, StremioID: "tt50"}
		e.service.localLengths(ctx, []*Item{movie})
		return movie.Runtime, episodes[0].Runtime
	}
	if movie, episode := lengths(); movie != 0 || episode != 0 || local.asked.Load() != 0 {
		t.Errorf("without a local folder: lengths %v and %v, local files asked %d times", movie, episode, local.asked.Load())
	}
	if _, err := e.addons.Create(ctx, addons.Shared(), addons.KindLocal, "/media/movies",
		func(accounts.ID) stremio.Manifest { return stremio.Manifest{} }, func(pgx.Tx, addons.Addon) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if movie, episode := lengths(); movie != 90*time.Minute || episode != 45*time.Minute || local.asked.Load() != 2 {
		t.Errorf("with a local folder: lengths %v and %v, local files asked %d times", movie, episode, local.asked.Load())
	}
}

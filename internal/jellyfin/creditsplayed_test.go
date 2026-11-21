package jellyfin

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// An episode stopped in its end credits, which a chapter of its version
// names, is played even under the played threshold, and Next Up moves on
// to the next one; a stop before the credits keeps its resume point, and
// so does any stop with the skip buttons turned off. A movie stopped in
// its credits keeps its resume point: scenes may follow them.
func TestStoppingInTheEndCreditsPlaysAnEpisode(t *testing.T) {
	t.Parallel()
	p := skipping(t)
	// The episode's first version lasts 45 minutes, its credits starting
	// at 38, at 84%.
	p.storeAnalysis(t, p.versions(t, p.episode)[0], lasting(2_700_000,
		chapter("Chapter 1", 0, 2_280_000), chapter("Credits", 2_280_000, 2_700_000)))
	// The movie's lasts two hours, its credits starting at 100 minutes.
	p.storeAnalysis(t, p.versions(t, p.movie)[0], lasting(7_200_000,
		chapter("Chapter 1", 0, 6_000_000), chapter("Credits", 6_000_000, 7_200_000)))
	stop := func(item string, at time.Duration) UserItemData {
		t.Helper()
		// The title's own identifier names its first version.
		body := map[string]any{"ItemId": item, "MediaSourceId": item, "PositionTicks": int64(at / 100)}
		if status, data := p.call(http.MethodPost, "/Sessions/Playing/Stopped", app("tv", p.token), body); status != http.StatusNoContent {
			t.Fatalf("stop: %d %s", status, data)
		}
		var detail BaseItemDto
		if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+item, p.token, &detail); status != http.StatusOK {
			t.Fatalf("item %s: %d", item, status)
		}
		return detail.UserData
	}
	list := func(path string) []string {
		t.Helper()
		var page QueryResult
		if status := p.get(t, path, p.token, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		return itemIDs(page.Items)
	}
	resumed := func(data UserItemData, at time.Duration) bool {
		return !data.Played && data.PlaybackPositionTicks == int64(at/100)
	}

	if data := stop(p.episode, 37*time.Minute); !resumed(data, 37*time.Minute) {
		t.Errorf("stopped before the credits: %+v", data)
	}
	if data := stop(p.movie, 105*time.Minute); !resumed(data, 105*time.Minute) {
		t.Errorf("a movie stopped in its credits: %+v", data)
	}
	p.setting(t, func(s *accounts.Settings) { s.SkipButtons = false })
	if data := stop(p.episode, 39*time.Minute); !resumed(data, 39*time.Minute) {
		t.Errorf("stopped in the credits with the skip buttons off: %+v", data)
	}
	if resume := list("/UserItems/Resume"); !slices.Contains(resume, p.episode) {
		t.Errorf("continue watching without the episode under way: %v", resume)
	}
	p.setting(t, func(s *accounts.Settings) { s.SkipButtons = true })
	if data := stop(p.episode, 39*time.Minute); !data.Played || data.PlaybackPositionTicks != 0 || data.LastPlayedDate == nil {
		t.Errorf("stopped in the credits: %+v", data)
	}
	if resume := list("/UserItems/Resume"); slices.Contains(resume, p.episode) {
		t.Errorf("continue watching keeps the episode played: %v", resume)
	}
	if next := list("/Shows/NextUp?SeriesId=" + p.series); !slices.Equal(next, []string{p.third}) {
		t.Errorf("next up: got %v, want the next episode %s", next, p.third)
	}
}

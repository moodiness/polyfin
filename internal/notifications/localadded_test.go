package notifications

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The titles a local folder's scan found new reach the server's Jellyfin
// targets as one ItemAdded each, with their provider IDs and no user; no
// user's own target, nor another kind of target, receives them.
func TestLocalTitlesAddedReachTheServersJellyfinTargets(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	movie, series, episode := jellyfinTitles()
	h.library.describe(movie, series, episode)
	h.add(t, nil, h.jellyfinTarget("/added", JellyfinItemAdded))
	h.add(t, &h.admin, h.jellyfinTarget("/mine", JellyfinItemAdded))
	h.add(t, nil, h.webhook("/polyfin", EventsFor(nil)...))

	h.LocalTitlesAdded([]accounts.ID{movie.ID, episode.ID})
	got := h.targets.wait(t, "/added", 2)
	h.idle(t)
	if len(got) != 2 {
		t.Fatalf("%d messages, want 2", len(got))
	}
	fields(t, "the movie", got[0].body, map[string]any{"NotificationType": "ItemAdded", "ItemId": movie.ID.String(), "ItemType": "Movie",
		"Name": "Night Train", "Year": float64(2008), "Provider_imdb": "tt0000031", "Provider_tmdb": "31", "UserId": nil,
		"NotificationUsername": nil})
	fields(t, "the episode", got[1].body, map[string]any{"NotificationType": "ItemAdded", "ItemId": episode.ID.String(),
		"ItemType": "Episode", "SeriesName": "Harbour", "SeasonNumber": float64(1), "EpisodeNumber": float64(2), "Year": float64(2019),
		"Provider_tvdb": "4102", "UserId": nil})
	if n := len(h.targets.at("/mine")) + len(h.targets.at("/polyfin")); n != 0 {
		t.Errorf("%d messages reached a user's own target or a webhook", n)
	}
}

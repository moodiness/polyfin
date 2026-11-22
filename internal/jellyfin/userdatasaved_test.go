package jellyfin

import (
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Each save of a user's data tells UserDataSaved the item, its data
// afterwards and why, by Jellyfin's save reasons: a playback's start,
// positions and stop, a played mark, a favorite, a rating, and data an
// app sends; and each playback report tells its app's device.
func TestSavedUserDataIsTold(t *testing.T) {
	t.Parallel()
	p := playing(t)
	type saved struct {
		user   accounts.ID
		item   string
		data   userdata.Data
		reason string
	}
	var mu sync.Mutex
	var told []saved
	p.handler.UserDataSaved = func(user accounts.User, item library.Item, data userdata.Data, reason string) {
		mu.Lock()
		defer mu.Unlock()
		told = append(told, saved{user: user.ID, item: item.ID.String(), data: data, reason: reason})
	}
	tv := app("tv", p.token)
	version := p.versions[1].ID.String()
	for _, step := range []struct {
		method, path string
		body         map[string]any
	}{
		{http.MethodPost, "/Sessions/Playing", map[string]any{"ItemId": p.movie, "MediaSourceId": version, "PositionTicks": 0, "PlayMethod": "DirectPlay"}},
		{http.MethodPost, "/Sessions/Playing/Progress", map[string]any{"ItemId": p.movie, "MediaSourceId": version, "PositionTicks": 600_000_000}},
		{http.MethodPost, "/Sessions/Playing/Stopped", map[string]any{"ItemId": p.movie, "MediaSourceId": version, "PositionTicks": 1_200_000_000}},
		{http.MethodPost, "/UserPlayedItems/" + p.movie, nil},
		{http.MethodPost, "/UserFavoriteItems/" + p.movie, nil},
		{http.MethodPost, "/UserItems/" + p.movie + "/Rating?likes=true", nil},
		{http.MethodPost, "/UserItems/" + p.movie + "/UserData", map[string]any{"PlayCount": 5}},
		{http.MethodDelete, "/UserPlayedItems/" + p.movie, nil},
	} {
		if status, data := p.call(step.method, step.path, tv, step.body); status >= 300 {
			t.Fatalf("%s %s: %d %s", step.method, step.path, status, data)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	var reasons []string
	for _, s := range told {
		if s.user != p.user.ID || s.item != p.movie {
			t.Errorf("told %+v, want the member's movie", s)
		}
		reasons = append(reasons, s.reason)
	}
	want := []string{userdata.SavedPlaybackStart, userdata.SavedPlaybackProgress, userdata.SavedPlaybackFinished, userdata.SavedTogglePlayed,
		userdata.SavedUpdateUserRating, userdata.SavedUpdateUserRating, userdata.SavedUpdateUserData, userdata.SavedTogglePlayed}
	if !slices.Equal(reasons, want) {
		t.Fatalf("reasons %v, want %v", reasons, want)
	}
	if d := told[3].data; !d.Played {
		t.Errorf("after the played mark: %+v", d)
	}
	if d := told[4].data; !d.Favorite {
		t.Errorf("after the favorite: %+v", d)
	}
	if d := told[5].data; d.Likes() == nil || !*d.Likes() {
		t.Errorf("after the rating: %+v", d)
	}
	if d := told[6].data; d.PlayCount != 5 {
		t.Errorf("after the app's data: %+v", d)
	}
	if d := told[7].data; d.Played || !d.Favorite {
		t.Errorf("after the unplayed mark: %+v", d)
	}
}

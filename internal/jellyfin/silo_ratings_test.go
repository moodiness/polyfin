package jellyfin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// rating reads the user's rating of a title from its page, as the Apple
// app reads it (see silotest.CheckUserRating), and from the Jellyfin API.
func (r siloRated) rating(t *testing.T, id string) (int, *float64) {
	t.Helper()
	status, data := r.do(t, http.MethodGet, "/api/v2/catalog/items/"+id, r.token, r.profile, nil)
	if status != http.StatusOK {
		t.Fatalf("the page: %d %s", status, data)
	}
	shown, err := silotest.CheckUserRating(data)
	if err != nil {
		t.Errorf("%v: %s", err, data)
	}
	var item BaseItemDto
	r.testServer.get(t, "/Items/"+id, r.jellyfin, &item)
	return shown, item.UserData.Rating
}

// The Apple app rates a title from its page, in half stars: the rating
// is the user's Jellyfin rating, which reads back on the page, and
// removing it, twice, leaves the title unrated. A rating outside 1 to 10
// is refused; a like or a dislike from a Jellyfin app shows as five stars
// or half a star.
func TestSiloAppsRateTitles(t *testing.T) {
	t.Parallel()
	r := newSiloRated(t)
	allowed := r.ids["Allowed"]
	if shown, stored := r.rating(t, allowed); shown != 0 || stored != nil {
		t.Fatalf("rated before any rating: %d, %v", shown, stored)
	}
	for _, score := range []int64{7, 10, 1} {
		if status, data := r.do(t, http.MethodPut, "/api/v2/ratings/"+allowed, r.token, r.profile, map[string]int64{"rating": score}); status != http.StatusNoContent {
			t.Fatalf("rating %d: %d %s", score, status, data)
		}
		if shown, stored := r.rating(t, allowed); shown != int(score) || stored == nil || *stored != float64(score) {
			t.Errorf("rated %d: the page shows %d, Jellyfin keeps %v", score, shown, stored)
		}
	}
	for _, body := range []any{map[string]int64{"rating": 0}, map[string]int64{"rating": 11}, map[string]any{}} {
		if status, data := r.do(t, http.MethodPut, "/api/v2/ratings/"+allowed, r.token, r.profile, body); status != http.StatusUnprocessableEntity {
			t.Errorf("%v: %d %s", body, status, data)
		}
	}
	for range 2 {
		if status, data := r.do(t, http.MethodDelete, "/api/v2/ratings/"+allowed, r.token, r.profile, nil); status != http.StatusNoContent {
			t.Fatalf("removing the rating: %d %s", status, data)
		}
		if shown, stored := r.rating(t, allowed); shown != 0 || stored != nil {
			t.Errorf("rated after its removal: %d, %v", shown, stored)
		}
	}
	for likes, want := range map[string]int{"true": 10, "false": 1} {
		if status, data := r.testServer.call(http.MethodPost, "/UserItems/"+allowed+"/Rating?likes="+likes, app("tv", r.jellyfin), nil); status != http.StatusOK {
			t.Fatalf("likes=%s: %d %s", likes, status, data)
		}
		if shown, _ := r.rating(t, allowed); shown != want {
			t.Errorf("likes=%s shows %d", likes, shown)
		}
	}
	if status, _ := r.do(t, http.MethodPut, "/api/v2/ratings/nothing", r.token, r.profile, map[string]int64{"rating": 5}); status != http.StatusNotFound {
		t.Errorf("an unknown title: %d", status)
	}
}

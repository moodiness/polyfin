package admin

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/library"
)

// reachedItems reaches the items of its IDs only, and keeps who asked.
type reachedItems struct {
	ids   []accounts.ID
	mu    sync.Mutex
	asked []string
}

func (r *reachedItems) Items(_ context.Context, user accounts.User, ids []accounts.ID) ([]library.Item, error) {
	r.mu.Lock()
	r.asked = append(r.asked, user.Name)
	r.mu.Unlock()
	var items []library.Item
	for _, id := range ids {
		if slices.Contains(r.ids, id) {
			items = append(items, library.Item{ID: id})
		}
	}
	return items, nil
}

// Recent activity tells where an entry's name holds a title the signed-in
// administrator still reaches, in UTF-16 code units; a title removed or out
// of their view, and an entry written before titles were kept, tell none.
func TestActivityTellsTheTitlesTheAdministratorReaches(t *testing.T) {
	t.Parallel()
	shown, gone := accounts.ID{1}, accounts.ID{2}
	reached := &reachedItems{ids: []accounts.ID{shown}}
	var log *activity.Store
	var deps testDeps
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		o.ActivityTitles, o.WebClient = reached, true
		log, deps = o.Activity, d
	})
	admin := api.signedIn("root", true)
	ctx := t.Context()
	if _, err := deps.pool.Exec(ctx, `INSERT INTO activity_log (name, type, item_id) VALUES ('sam is playing Amélie on TV', 'VideoPlayback', $1)`,
		shown.String()); err != nil {
		t.Fatal(err)
	}
	user := accounts.User{ID: accounts.ID{9}, Name: "Zoé 🎬"}
	log.PlaybackStarted(ctx, user, "Amélie", gone.String(), "TV")
	log.PlaybackStarted(ctx, user, "Amélie", shown.String(), "TV")

	var page struct {
		Items []activityEntryJSON `json:"items"`
	}
	if status := admin.raw(http.MethodGet, "/activity?type=VideoPlayback", "", &page); status != http.StatusOK {
		t.Fatalf("activity: %d", status)
	}
	if len(page.Items) != 3 {
		t.Fatalf("activity: %+v", page.Items)
	}
	// "Zoé 🎬 is playing ": the clapper board is 2 code units.
	if title := page.Items[0].Title; title == nil || *title != (activityTitleJSON{ID: shown.String(), Start: 18, End: 24}) {
		t.Errorf("a title reached: %+v", title)
	}
	if title := page.Items[1].Title; title != nil {
		t.Errorf("a title removed: %+v", title)
	}
	if title := page.Items[2].Title; title != nil {
		t.Errorf("an entry from before: %+v", title)
	}
	if !slices.Equal(reached.asked, []string{"root"}) {
		t.Errorf("titles found for %v, want the administrator once", reached.asked)
	}
}

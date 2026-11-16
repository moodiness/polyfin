package library

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// The episodes PickedEpisodes lists but its caller does not keep are
// recorded after the request, with the seasons, as the series' listing
// records them; a list recorded already is not recorded again, and one
// that changed is.
func TestUnpickedEpisodesAreRecordedOncePerList(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), listingAddon())
	e.listed(e.member, "Shows")
	show := itemID(titleKey(KindSeries, "tt100"))
	var episodes []accounts.ID
	for season := 1; season <= 2; season++ {
		for episode := 1; episode <= 3; episode++ {
			episodes = append(episodes, itemID(episodeKey(fmt.Sprintf("tt100:%d:%d", season, episode))))
		}
	}
	recorded := func() {
		t.Helper()
		select {
		case <-e.service.EpisodesRecorded():
		case <-time.After(30 * time.Second):
			t.Fatal("the episodes are still being recorded")
		}
	}
	// pickFirst lists as Next Up does, the request ending once answered.
	pickFirst := func() {
		t.Helper()
		ctx, cancel := context.WithCancel(t.Context())
		listed, err := e.service.PickedEpisodes(ctx, e.member, []accounts.ID{show}, func(_ accounts.ID, all []Item) []Item { return all[:1] })
		cancel()
		if err != nil || len(listed[show]) != 1 || listed[show][0].ID != episodes[0] {
			t.Fatalf("picked: %+v %v", listed, err)
		}
		recorded()
	}
	stored := func(id accounts.ID) (record, bool) {
		t.Helper()
		r, err := e.service.load(t.Context(), id)
		return r, err == nil
	}

	pickFirst()
	for _, id := range append(slices.Clone(episodes), itemID(seasonKey("tt100", 1)), itemID(seasonKey("tt100", 2))) {
		if _, ok := stored(id); !ok {
			t.Errorf("%v not recorded", id)
		}
	}

	// The same list again: nothing is recorded, as the row deleted behind
	// its back shows.
	if _, err := e.service.db.Exec(t.Context(), "DELETE FROM items WHERE id = $1", episodes[5]); err != nil {
		t.Fatal(err)
	}
	pickFirst()
	if _, ok := stored(episodes[5]); ok {
		t.Error("the same list was recorded again")
	}

	// A list that changed is recorded again.
	v, err := e.service.view(t.Context(), e.member)
	if err != nil {
		t.Fatal(err)
	}
	_, records, err := e.service.orderedEpisodes(t.Context(), v, show, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range records {
		if r.ID == episodes[4] {
			video := *r.Video
			video.Title = "Renamed"
			records[i].Video = &video
		}
	}
	e.service.recordEpisodesLater(context.Background(), map[accounts.ID][]record{show: records})
	recorded()
	if r, ok := stored(episodes[4]); !ok || r.Video == nil || r.Video.Title != "Renamed" {
		t.Errorf("the changed episode: %+v %v", r.Video, ok)
	}
	if _, ok := stored(episodes[5]); !ok {
		t.Error("the changed list's other episodes were not recorded")
	}
}

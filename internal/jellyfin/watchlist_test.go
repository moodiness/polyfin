package jellyfin

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/watchlist"
)

func TestTheWatchlistViewListsTheTitlesNotStartedLatestFirst(t *testing.T) {
	t.Parallel()
	s, _, titles := grouping(t)
	member := s.user("member", nil)
	child := s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(10)} })
	token := s.signIn("member", "tv")
	if _, ok := s.viewIDs(t, token)["Watchlist"]; ok {
		t.Fatal("an empty watchlist shows")
	}
	lists := watchlist.New(s.pool)
	type listed struct {
		id  string
		day int
	}
	save := func(user accounts.ID, list string, entries ...listed) {
		t.Helper()
		var saved []watchlist.Title
		for _, title := range entries {
			id, err := accounts.ParseID(title.id)
			if err != nil {
				t.Fatal(err)
			}
			saved = append(saved, watchlist.Title{ID: id, AddedAt: new(time.Date(2026, 9, title.day, 0, 0, 0, 0, time.UTC))})
		}
		if _, err := lists.Save(t.Context(), user, list, saved, true); err != nil {
			t.Fatal(err)
		}
	}
	// The latest added first, a title on two lists by its latest date.
	save(member.ID, watchlist.Tracking("trakt"), listed{titles.show, 10}, listed{titles.kids, 3})
	save(member.ID, watchlist.Connection("stremio"), listed{titles.adults, 5}, listed{titles.show, 1})
	save(child.ID, watchlist.Import, listed{titles.adults, 5}, listed{titles.kids, 3})

	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	view := views.Items[len(views.Items)-1]
	if view.Name != "Watchlist" || view.Type != "CollectionFolder" || view.CollectionType != "" {
		t.Fatalf("views: %+v", views.Items)
	}
	listing := func(token, path string) []string {
		t.Helper()
		var page QueryResult
		if status := s.get(t, path, token, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		return itemNames(page.Items)
	}
	if got := listing(token, "/Items?ParentId="+view.Id); !slices.Equal(got, []string{"Show", "Adults", "Kids"}) {
		t.Errorf("listing: %v", got)
	}
	// As jellyfin-web lists a folder: its own order holds.
	if got := listing(token, "/Items?ParentId="+view.Id+"&Recursive=true&IncludeItemTypes=Movie,Series&SortBy=SortName&SortOrder=Ascending"); !slices.Equal(got, []string{"Show", "Adults", "Kids"}) {
		t.Errorf("listing sorted by name: %v", got)
	}
	// Its home row, as apps ask for a view's latest items.
	var latest []BaseItemDto
	s.get(t, "/Items/Latest?ParentId="+view.Id+"&Limit=2", token, &latest)
	if got := itemNames(latest); !slices.Equal(got, []string{"Show", "Adults"}) {
		t.Errorf("home row: %v", got)
	}
	// Parental control hides titles there as everywhere.
	childToken := s.signIn("child", "tablet")
	if got := listing(childToken, "/Items?ParentId="+view.Id); !slices.Equal(got, []string{"Kids"}) {
		t.Errorf("child's listing: %v", got)
	}

	// An episode played starts its series; a movie played or resumed is
	// started: each leaves the view.
	var episodes QueryResult
	s.get(t, "/Shows/"+titles.show+"/Episodes", token, &episodes)
	if len(episodes.Items) == 0 {
		t.Fatal("no episodes")
	}
	s.call(http.MethodPost, "/UserPlayedItems/"+episodes.Items[0].Id, app("tv", token), nil)
	if got := listing(token, "/Items?ParentId="+view.Id); !slices.Equal(got, []string{"Adults", "Kids"}) {
		t.Errorf("after an episode played: %v", got)
	}
	s.call(http.MethodPost, "/UserPlayedItems/"+titles.adults, app("tv", token), nil)
	s.call(http.MethodPost, "/UserPlayedItems/"+titles.kids, app("tv", token), nil)
	if _, ok := s.viewIDs(t, token)["Watchlist"]; ok {
		t.Error("the view shows once every title is started")
	}
	// An app that still shows it lists it empty.
	if got := listing(token, "/Items?ParentId="+view.Id); len(got) != 0 {
		t.Errorf("emptied view: %v", got)
	}

	// Its name follows the server language.
	s.setting(t, func(settings *accounts.Settings) { settings.Language = "fr" })
	if _, ok := s.viewIDs(t, childToken)["À regarder"]; !ok {
		t.Error("no French name")
	}
}

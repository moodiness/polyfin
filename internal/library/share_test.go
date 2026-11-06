package library

import (
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Titles to share are found in the catalogs that declare no search extra
// too, from what Polyfin recorded of them, asking no addon: those of the
// addons the user uses only, that their parental control allows and that
// they did not hide.
func TestShareTitlesFindsKnownTitlesOfCatalogsWithoutSearch(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rated := func(id, kind, name, rating string) stremio.Meta {
		return stremio.Meta{ID: id, Type: kind, Name: name, Extras: &stremio.Extras{Certification: rating}}
	}
	everyone := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "series", ID: "shows", Name: "Shows"}}},
		catalogs: map[string][]stremio.Meta{
			"movie/top": {{ID: "tt1", Type: "movie", Name: "Night Train"}, {ID: "tt2", Type: "movie", Name: "Family Night"},
				{ID: "tt3", Type: "movie", Name: "Morning"}},
			"series/shows": {{ID: "tt4", Type: "series", Name: "Night Shift"}},
		},
		metas: map[string]stremio.Meta{
			"movie/tt1": rated("tt1", "movie", "Night Train", "R"), "movie/tt2": rated("tt2", "movie", "Family Night", "PG"),
			"movie/tt3":  rated("tt3", "movie", "Morning", "PG"),
			"series/tt4": rated("tt4", "series", "Night Shift", "TV-PG"),
		},
	}
	e.install(addons.Shared(), everyone)
	// The administrator's own server addon, which the member does not use.
	server := httptest.NewServer(&fakeAddon{
		manifest: stremio.Manifest{ID: "b", Name: "B", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "picks", Name: "Picks"}}},
		catalogs: map[string][]stremio.Meta{"movie/picks": {{ID: "tt9", Type: "movie", Name: "Night Owl"}}},
	})
	t.Cleanup(server.Close)
	picks, err := e.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.addons.SetUsedBy(t.Context(), picks.ID, addons.UsedBy{Listed: true, Users: []accounts.ID{e.admin.ID}}); err != nil {
		t.Fatal(err)
	}
	// The administrator's libraries list every title once.
	for _, name := range []string{"Top", "Shows", "Picks"} {
		e.children(e.admin, name, 0, 100)
	}
	requests := len(everyone.catalogRequests())

	shared := func(user accounts.User, term string) []string {
		t.Helper()
		found, err := e.service.ShareTitles(t.Context(), user, term, 20)
		if err != nil {
			t.Fatal(err)
		}
		got := names(found)
		slices.Sort(got)
		return got
	}
	if got := shared(e.admin, "night"); !slices.Equal(got, []string{"Family Night", "Night Owl", "Night Shift", "Night Train"}) {
		t.Errorf("the administrator's titles: %v", got)
	}
	// The member does not use the administrator's addon.
	if got := shared(e.member, " NIGHT "); !slices.Equal(got, []string{"Family Night", "Night Shift", "Night Train"}) {
		t.Errorf("the member's titles: %v", got)
	}
	if got := shared(e.member, "Owl"); len(got) != 0 {
		t.Errorf("a title of an addon the member does not use: %v", got)
	}
	// Parental control leaves out the R movie.
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	if got := shared(child, "night"); !slices.Equal(got, []string{"Family Night", "Night Shift"}) {
		t.Errorf("the child's titles: %v", got)
	}
	// A title the member hid is left out.
	train, err := e.service.ShareTitles(t.Context(), e.member, "Night Train", 20)
	if err != nil || len(train) != 1 {
		t.Fatalf("Night Train: %v %v", names(train), err)
	}
	if err := e.service.userData.Hide(t.Context(), e.member.ID, train[0].ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := shared(e.member, "night"); !slices.Equal(got, []string{"Family Night", "Night Shift"}) {
		t.Errorf("the member's titles once Night Train is hidden: %v", got)
	}
	// The limit holds, and no catalog was asked again.
	if found, err := e.service.ShareTitles(t.Context(), e.admin, "night", 2); err != nil || len(found) != 2 {
		t.Errorf("two titles: %v %v", names(found), err)
	}
	if got := len(everyone.catalogRequests()); got != requests {
		t.Errorf("%d catalog requests while sharing", got-requests)
	}
}

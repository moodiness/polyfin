package library

import (
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// genreAddon is ratedAddon whose complete descriptions give genres. Its
// catalog rows give none, but Homemade's, which names a genre of its
// description, and Crime's, which names one of its two.
func genreAddon() *fakeAddon {
	addon := ratedAddon()
	for id, genres := range map[string][]string{
		"movie/tt1": {"Family", "Animation"}, "movie/tt2": {"Crime", "Horror"}, "movie/tt3": {"Horror"},
		"movie/tt4": {"Comedy"}, "movie/tt5": {"Drama"}, "series/tt7": {"Animation"}, "series/tt8": {"Drama", "Horror"},
	} {
		meta := addon.metas[id]
		meta.Genres = genres
		addon.metas[id] = meta
	}
	rows := addon.catalogs["movie/top"]
	rows[1].Genres = []string{"Crime"}
	rows[2].Genres = []string{"horror"}
	return addon
}

// blocking creates a user who blocks genres.
func (e env) blocking(name string, genres ...string) accounts.User {
	e.t.Helper()
	user, err := e.users.CreateUser(e.t.Context(), accounts.NewUser{Name: name, Password: "correct horse"})
	if err != nil {
		e.t.Fatal(err)
	}
	user, err = e.users.UpdateUser(e.t.Context(), user.ID, accounts.UserChanges{BlockedGenres: &genres}, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return user
}

func TestBlockedGenresHideTitles(t *testing.T) {
	e := newEnv(t)
	addon := genreAddon()
	e.install(addons.Shared(), addon)
	kid := e.blocking("kid", "HORROR")

	all := e.children(e.member, "Top", 0, 100)
	if got := names(all.Items); len(got) != 6 || addon.metaRequests() != 0 {
		t.Fatalf("listing without blocked genres: %v, %d meta requests", got, addon.metaRequests())
	}
	byName := map[string]accounts.ID{}
	for _, item := range all.Items {
		byName[item.Name] = item.ID
	}

	// Crime's description and Homemade's row name Horror, in any case.
	page := e.children(kid, "Top", 0, 100)
	if got := names(page.Items); !slices.Equal(got, []string{"Family", "Teen", "Adult", "Local"}) || page.Total != 4 || page.More {
		t.Fatalf("listing blocking Horror: %v total=%d more=%v", got, page.Total, page.More)
	}
	// Homemade's row was enough: its description was not asked for.
	if got := addon.metaRequests(); got != 5 {
		t.Errorf("%d meta requests, want one per title but Homemade", got)
	}
	if results, _ := e.service.Search(t.Context(), kid, "r", []Kind{KindMovie}, 10); slices.Contains(names(results), "Crime") {
		t.Errorf("search shows a blocked title: %v", names(results))
	}
	for _, name := range []string{"Crime", "Homemade"} {
		if _, err := e.service.Item(t.Context(), kid, byName[name]); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s opened: %v", name, err)
		}
		if _, err := e.service.Versions(t.Context(), kid, byName[name]); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s versions: %v", name, err)
		}
	}
	if item, err := e.service.Item(t.Context(), kid, byName["Adult"]); err != nil || item.OfficialRating != "NC-17" {
		t.Errorf("a title of other genres: %+v %v", item, err)
	}
	if items, _ := e.service.Items(t.Context(), kid, []accounts.ID{byName["Crime"], byName["Family"]}); !slices.Equal(names(items), []string{"Family"}) {
		t.Errorf("items by identifier: %v", names(items))
	}
	// A version listed for another user is no way around the genres.
	versions, err := e.service.Versions(t.Context(), e.member, byName["Crime"])
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	if _, err := e.service.Version(t.Context(), kid, byName["Crime"], versions[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("remembered version played: %v", err)
	}

	// Seasons and episodes follow their series.
	if got := names(e.children(kid, "Shows", 0, 10).Items); !slices.Equal(got, []string{"Cartoon"}) {
		t.Errorf("shows blocking Horror: %v", got)
	}
	drama := itemID(titleKey(KindSeries, "tt8"))
	if _, err := e.service.Seasons(t.Context(), kid, drama); !errors.Is(err, ErrNotFound) {
		t.Errorf("seasons of a blocked series: %v", err)
	}
	episodes, err := e.service.Episodes(t.Context(), e.member, drama, nil)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("episodes: %v %v", episodes, err)
	}
	if _, err := e.service.Item(t.Context(), kid, episodes[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("episode of a blocked series: %v", err)
	}

	// Genres and ratings both hide: a limit of PG-13 also hides Adult.
	both := e.blocking("both", "Horror")
	both, err = e.users.UpdateUser(t.Context(), both.ID, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(13)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(e.children(both, "Top", 0, 100).Items); !slices.Equal(got, []string{"Family", "Teen", "Local"}) {
		t.Errorf("listing blocking Horror up to PG-13: %v", got)
	}

	// The genres learned are kept: a server started afresh does not ask
	// again.
	requests := addon.metaRequests()
	e.service = New(e.service.db, e.addons, stremio.NewClient("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	if got := names(e.children(kid, "Top", 0, 100).Items); !slices.Equal(got, []string{"Family", "Teen", "Adult", "Local"}) || addon.metaRequests() != requests {
		t.Errorf("listing after a restart: %v, %d meta requests, want %d", got, addon.metaRequests(), requests)
	}
}

func TestUnknownGenresAreHiddenUntilKnown(t *testing.T) {
	e := newEnv(t)
	addon := genreAddon()
	addon.metaGate = make(chan struct{})
	e.install(addons.Shared(), addon)
	e.service.ratingWait = 50 * time.Millisecond
	kid := e.blocking("kid", "Horror")

	started := time.Now()
	page := e.children(kid, "Top", 0, 100)
	if len(page.Items) != 0 || !page.More || time.Since(started) > 5*time.Second {
		t.Fatalf("listing while descriptions are slow: %v more=%v after %v", names(page.Items), page.More, time.Since(started))
	}
	close(addon.metaGate)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := names(e.children(kid, "Top", 0, 100).Items)
		if slices.Equal(got, []string{"Family", "Teen", "Adult", "Local"}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("listing once descriptions answered: %v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestOnlyTheServersAddonsGiveGenres(t *testing.T) {
	e := newEnv(t)
	e.install(addons.Shared(), genreAddon())
	kid := e.blocking("kid", "Horror")
	crime := itemID(titleKey(KindMovie, "tt2"))

	// Another user's own addon describes Crime without Horror first: its
	// genres are not kept as Crime's.
	e.install(addons.Personal(e.admin.ID), fakeRatings())
	if page := e.children(e.admin, "Mine", 0, 10); !slices.Equal(names(page.Items), []string{"Crime"}) {
		t.Fatalf("own library: %v", names(page.Items))
	}
	if _, err := e.service.Item(t.Context(), e.admin, crime); err != nil {
		t.Fatal(err)
	}
	if r, _ := e.service.load(t.Context(), crime); r.Genres != nil {
		t.Errorf("a user's own addon set the genres: %v", *r.Genres)
	}
	if _, err := e.service.Item(t.Context(), kid, crime); !errors.Is(err, ErrNotFound) {
		t.Errorf("Crime opened by the kid after another user's addon described it: %v", err)
	}
	if r, _ := e.service.load(t.Context(), crime); r.Genres == nil || !slices.Equal(*r.Genres, []string{"Crime", "Horror"}) {
		t.Errorf("genres from the server's addon: %v", r.Genres)
	}

	// The kid's own addon is left out of their libraries and of the
	// descriptions they get.
	e.install(addons.Personal(kid.ID), fakeRatings())
	updated, err := e.users.UpdateUser(t.Context(), kid.ID, accounts.UserChanges{UseSharedAddons: new(false)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	kid = updated
	libraries, _ := e.service.Libraries(t.Context(), kid)
	if got := names(libraries); slices.Contains(got, "Mine") || !slices.Contains(got, "Top") {
		t.Errorf("kid's libraries: %v", got)
	}
	if _, err := e.service.Item(t.Context(), kid, crime); !errors.Is(err, ErrNotFound) {
		t.Errorf("Crime opened through the kid's own addon: %v", err)
	}
}

func TestHiddenLibrariesLeaveTheViewButNotTheirTitles(t *testing.T) {
	e := newEnv(t)
	addon := genreAddon()
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	servers, err := e.service.ServerLibraries(t.Context())
	if err != nil || len(servers) != 2 || servers[0].ID != top.ID || servers[0].Name != "Top" {
		t.Fatalf("server libraries: %+v %v", servers, err)
	}
	hidden := []accounts.ID{top.ID}
	member, err := e.users.UpdateUser(t.Context(), e.member.ID, accounts.UserChanges{HiddenLibraries: &hidden}, nil)
	if err != nil {
		t.Fatal(err)
	}
	libraries, _ := e.service.Libraries(t.Context(), member)
	if got := names(libraries); !slices.Equal(got, []string{"Shows"}) {
		t.Errorf("libraries with Top hidden: %v", got)
	}
	if _, err := e.service.Children(t.Context(), member, top.ID, 0, 10, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("listing a hidden library: %v", err)
	}
	if _, err := e.service.Item(t.Context(), member, top.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("opening a hidden library: %v", err)
	}
	// Its titles stay reachable, by search or by identifier.
	crime := itemID(titleKey(KindMovie, "tt2"))
	if results, _ := e.service.Search(t.Context(), member, "Crime", []Kind{KindMovie}, 10); !slices.Contains(names(results), "Crime") {
		t.Errorf("search with Top hidden: %v", names(results))
	}
	if item, err := e.service.Item(t.Context(), member, crime); err != nil || item.Name != "Crime" {
		t.Errorf("a title of the hidden library: %+v %v", item, err)
	}
	if versions, err := e.service.Versions(t.Context(), member, crime); err != nil || len(versions) != 1 {
		t.Errorf("playing a title of the hidden library: %v %v", versions, err)
	}
	// Libraries added later show; other users keep theirs.
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "b", Name: "B", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "new", Name: "New"}}},
		catalogs: map[string][]stremio.Meta{},
	})
	libraries, _ = e.service.Libraries(t.Context(), member)
	if got := names(libraries); !slices.Equal(got, []string{"Shows", "New"}) {
		t.Errorf("libraries after one was added: %v", got)
	}
	if libraries, _ := e.service.Libraries(t.Context(), e.admin); len(libraries) != 3 {
		t.Errorf("another user's libraries: %v", names(libraries))
	}
}

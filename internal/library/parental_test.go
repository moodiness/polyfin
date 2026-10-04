package library

import (
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// ratedAddon serves a movie catalog whose rows carry no rating, as
// AIOMetadata's, and complete descriptions that do, plus two series.
func ratedAddon() *fakeAddon {
	movie := func(id, name, rating string) (stremio.Meta, stremio.Meta) {
		row := stremio.Meta{ID: id, Type: "movie", Name: name}
		full := row
		if rating != "" {
			full.Extras = &stremio.Extras{Certification: rating}
		}
		return row, full
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "skip"}, {Name: "search"}}},
				{Type: "series", ID: "shows", Name: "Shows"},
			}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
		streams:  map[string][]stremio.Stream{},
		pageSize: 2,
	}
	for _, m := range []struct{ id, name, rating string }{
		{"tt1", "Family", "PG"}, {"tt2", "Crime", "R"}, {"tt3", "Homemade", ""},
		{"tt4", "Teen", "PG-13"}, {"tt5", "Adult", "NC-17"}, {"tt6", "Local", ""},
	} {
		row, full := movie(m.id, m.name, m.rating)
		if m.id == "tt6" {
			// Only the local certification is known.
			full.Extras = &stremio.Extras{CertificationLocal: "FSK 12"}
		}
		addon.catalogs["movie/top"] = append(addon.catalogs["movie/top"], row)
		addon.metas["movie/"+m.id] = full
		addon.streams["movie/"+m.id] = []stremio.Stream{{Name: "1080p", URL: "https://cdn.example/" + m.id}}
	}
	for _, s := range []struct{ id, name, rating string }{{"tt7", "Cartoon", "TV-Y7"}, {"tt8", "Drama", "TV-MA"}} {
		addon.catalogs["series/shows"] = append(addon.catalogs["series/shows"], stremio.Meta{ID: s.id, Type: "series", Name: s.name})
		addon.metas["series/"+s.id] = stremio.Meta{ID: s.id, Type: "series", Name: s.name, Extras: &stremio.Extras{Certification: s.rating},
			Videos: []stremio.Video{{ID: s.id + ":1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z"}}}
		addon.streams["series/"+s.id+":1:1"] = []stremio.Stream{{Name: "720p", URL: "https://cdn.example/" + s.id}}
	}
	return addon
}

// restrict sets a user's parental control.
func (e env) restrict(name string, control accounts.ParentalControl) accounts.User {
	e.t.Helper()
	user, err := e.users.CreateUser(e.t.Context(), accounts.NewUser{Name: name, Password: "correct horse"})
	if err != nil {
		e.t.Fatal(err)
	}
	user, err = e.users.UpdateUser(e.t.Context(), user.ID, accounts.UserChanges{Parental: &control}, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return user
}

func (a *fakeAddon) metaRequests() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	count := 0
	for _, request := range a.requests {
		if strings.HasPrefix(request, "meta/") {
			count++
		}
	}
	return count
}

func (e env) children(user accounts.User, library string, start, count int) Page {
	e.t.Helper()
	page, err := e.service.Children(e.t.Context(), user, e.library(user, library).ID, start, count, "")
	if err != nil {
		e.t.Fatal(err)
	}
	return page
}

func TestParentalControlHidesTitlesAboveTheLimit(t *testing.T) {
	e := newEnv(t)
	addon := ratedAddon()
	e.install(addons.Shared(), addon)
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13), BlockUnrated: []string{"Movie"}})

	// An unrestricted user lists everything, without a description being
	// asked for.
	all := e.children(e.member, "Top", 0, 100)
	if got := names(all.Items); len(got) != 6 || addon.metaRequests() != 0 {
		t.Fatalf("unrestricted listing: %v, %d meta requests", got, addon.metaRequests())
	}
	byName := map[string]accounts.ID{}
	for _, item := range all.Items {
		byName[item.Name] = item.ID
	}

	// R, NC-17 and the unrated movie are hidden; the local rating counts.
	page := e.children(child, "Top", 0, 100)
	if got := names(page.Items); !slices.Equal(got, []string{"Family", "Teen", "Local"}) || page.Total != 3 || page.More {
		t.Fatalf("restricted listing: %v total=%d more=%v", got, page.Total, page.More)
	}
	// Positions count only the titles the user sees.
	if got := names(e.children(child, "Top", 1, 1).Items); !slices.Equal(got, []string{"Teen"}) {
		t.Errorf("second title: %v", got)
	}
	if results, _ := e.service.Search(t.Context(), child, "r", []Kind{KindMovie}, 10); slices.Contains(names(results), "Crime") {
		t.Errorf("search shows a hidden title: %v", names(results))
	}

	for _, name := range []string{"Crime", "Homemade", "Adult"} {
		if _, err := e.service.Item(t.Context(), child, byName[name]); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s opened: %v", name, err)
		}
		if _, err := e.service.Versions(t.Context(), child, byName[name]); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s versions: %v", name, err)
		}
	}
	if items, _ := e.service.Items(t.Context(), child, []accounts.ID{byName["Crime"], byName["Family"]}); !slices.Equal(names(items), []string{"Family"}) {
		t.Errorf("items by identifier: %v", names(items))
	}
	if item, err := e.service.Item(t.Context(), e.member, byName["Crime"]); err != nil || item.OfficialRating != "R" {
		t.Errorf("unrestricted user opening Crime: %+v %v", item, err)
	}
	// A version listed for another user is not a way around the limit.
	versions, err := e.service.Versions(t.Context(), e.member, byName["Crime"])
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	if _, err := e.service.Version(t.Context(), child, byName["Crime"], versions[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("remembered version played: %v", err)
	}
	if _, err := e.service.Version(t.Context(), e.member, byName["Crime"], versions[0].ID); err != nil {
		t.Errorf("unrestricted user playing Crime: %v", err)
	}

	// Seasons and episodes follow their series.
	shows := e.children(child, "Shows", 0, 10)
	if got := names(shows.Items); !slices.Equal(got, []string{"Cartoon"}) {
		t.Fatalf("restricted shows: %v", got)
	}
	drama := itemID(titleKey(KindSeries, "tt8"))
	if _, err := e.service.Seasons(t.Context(), child, drama); !errors.Is(err, ErrNotFound) {
		t.Errorf("seasons of a hidden series: %v", err)
	}
	episodes, err := e.service.Episodes(t.Context(), e.member, drama, nil)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("episodes: %v %v", episodes, err)
	}
	if _, err := e.service.Item(t.Context(), child, episodes[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("episode of a hidden series: %v", err)
	}
	if _, err := e.service.Versions(t.Context(), child, episodes[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("episode versions of a hidden series: %v", err)
	}
	if cartoon, err := e.service.Episodes(t.Context(), child, itemID(titleKey(KindSeries, "tt7")), nil); err != nil || len(cartoon) != 1 {
		t.Errorf("episodes of an allowed series: %v %v", cartoon, err)
	}

	// Unrated movies are only hidden from users who block them.
	older := e.restrict("older", accounts.ParentalControl{MaxRating: new(13)})
	if got := names(e.children(older, "Top", 0, 100).Items); !slices.Equal(got, []string{"Family", "Homemade", "Teen", "Local"}) {
		t.Errorf("listing with unrated movies allowed: %v", got)
	}
	// At the limit's score, the subscore decides: NC-17 is R with more.
	strict := e.restrict("strict", accounts.ParentalControl{MaxRating: new(17), MaxSubRating: new(0)})
	if got := names(e.children(strict, "Top", 0, 100).Items); slices.Contains(got, "Adult") || !slices.Contains(got, "Crime") {
		t.Errorf("listing up to R: %v", got)
	}
}

func TestRatingsAreLookedUpOnce(t *testing.T) {
	e := newEnv(t)
	addon := ratedAddon()
	e.install(addons.Shared(), addon)
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13), BlockUnrated: []string{"Movie"}})

	e.children(child, "Top", 0, 100)
	if got := addon.metaRequests(); got != 6 {
		t.Fatalf("first listing: %d meta requests, want one per title", got)
	}
	// Ratings, and the absence of one, are kept: a server started afresh,
	// whose description cache is empty, does not ask again.
	e.service = New(e.service.db, e.addons, stremio.NewClient("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	if got := names(e.children(child, "Top", 0, 100).Items); !slices.Equal(got, []string{"Family", "Teen", "Local"}) || addon.metaRequests() != 6 {
		t.Errorf("listing after a restart: %v, %d meta requests", got, addon.metaRequests())
	}
	// Listing the catalog again does not forget them.
	e.children(e.member, "Top", 0, 100)
	e.service = New(e.service.db, e.addons, stremio.NewClient("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	if e.children(child, "Top", 0, 100); addon.metaRequests() != 6 {
		t.Errorf("ratings lost when the catalog was listed again: %d meta requests", addon.metaRequests())
	}
}

func TestSlowRatingsAreHiddenUntilKnown(t *testing.T) {
	e := newEnv(t)
	addon := ratedAddon()
	addon.metaGate = make(chan struct{})
	e.install(addons.Shared(), addon)
	e.service.ratingWait = 50 * time.Millisecond
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})

	started := time.Now()
	page := e.children(child, "Top", 0, 100)
	if len(page.Items) != 0 || time.Since(started) > 5*time.Second {
		t.Fatalf("listing while descriptions are slow: %v after %v", names(page.Items), time.Since(started))
	}
	// The lookups went on: the next listing knows the ratings.
	close(addon.metaGate)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := names(e.children(child, "Top", 0, 100).Items)
		if slices.Equal(got, []string{"Family", "Homemade", "Teen", "Local"}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("listing once descriptions answered: %v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// fakeRatings is an addon a user could install to describe a title without
// its certification.
func fakeRatings() *fakeAddon {
	crime := stremio.Meta{ID: "tt2", Type: "movie", Name: "Crime"}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "p", Name: "Mine", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine"}}},
		catalogs: map[string][]stremio.Meta{"movie/mine": {crime}},
		metas:    map[string]stremio.Meta{"movie/tt2": crime},
	}
}

func TestOnlyTheServersAddonsRateTitles(t *testing.T) {
	e := newEnv(t)
	e.install(addons.Shared(), ratedAddon())
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	crime := itemID(titleKey(KindMovie, "tt2"))

	// Another user's own addon describes Crime without its R first: it is
	// not kept as Crime's rating. (Users' addons on the local network, as
	// the test's, are an administrator's.)
	e.install(addons.Personal(e.admin.ID), fakeRatings())
	if page := e.children(e.admin, "Mine", 0, 10); !slices.Equal(names(page.Items), []string{"Crime"}) {
		t.Fatalf("own library: %v", names(page.Items))
	}
	if _, err := e.service.Item(t.Context(), e.admin, crime); err != nil {
		t.Fatal(err)
	}
	if r, _ := e.service.load(t.Context(), crime); r.Rating != nil {
		t.Errorf("a user's own addon set the rating: %q", *r.Rating)
	}
	if _, err := e.service.Item(t.Context(), child, crime); !errors.Is(err, ErrNotFound) {
		t.Errorf("Crime opened by the child after another user's addon described it: %v", err)
	}
	if r, _ := e.service.load(t.Context(), crime); r.Rating == nil || *r.Rating != "R" {
		t.Errorf("rating from the server's addon: %v", r.Rating)
	}

	// The child's own addon is left out of their libraries and of the
	// descriptions they get, and they keep the server's addons.
	e.install(addons.Personal(child.ID), fakeRatings())
	if err := e.addons.SetUsesSharedAddons(t.Context(), child.ID, false); err != nil {
		t.Fatal(err)
	}
	libraries, _ := e.service.Libraries(t.Context(), child)
	if got := names(libraries); slices.Contains(got, "Mine") || !slices.Contains(got, "Top") {
		t.Errorf("child's libraries: %v", got)
	}
	if _, err := e.service.Item(t.Context(), child, crime); !errors.Is(err, ErrNotFound) {
		t.Errorf("Crime opened through the child's own addon: %v", err)
	}
	if _, err := e.service.Versions(t.Context(), child, crime); !errors.Is(err, ErrNotFound) {
		t.Errorf("Crime played through the child's own addon: %v", err)
	}
	if r, _ := e.service.load(t.Context(), crime); r.Rating == nil || *r.Rating != "R" {
		t.Errorf("rating after the child's addon: %v", r.Rating)
	}
}

func TestRestrictedListingsDoNotCrawlCatalogs(t *testing.T) {
	e := newEnv(t)
	unrated, rated := titles("movie", 200), titles("movie", 200)
	for i := range rated {
		rated[i].ID = "r" + rated[i].ID
		rated[i].Extras = &stremio.Extras{Certification: "R"}
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "new", Name: "New", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "movie", ID: "adult", Name: "Adult", Extra: []stremio.Extra{{Name: "skip"}}},
			}},
		catalogs: map[string][]stremio.Meta{"movie/new": unrated, "movie/adult": rated},
		metas:    map[string]stremio.Meta{},
		pageSize: 20,
		metaGate: make(chan struct{}),
	}
	for _, meta := range unrated {
		addon.metas["movie/"+meta.ID] = meta
	}
	e.install(addons.Shared(), addon)
	t.Cleanup(func() { close(addon.metaGate) })
	e.service.ratingWait = 50 * time.Millisecond
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})

	// Ratings that do not come in time stop the listing at the first page,
	// which says more may follow.
	page := e.children(child, "New", 0, 20)
	if len(page.Items) != 0 || !page.More || len(addon.catalogRequests()) != 1 {
		t.Errorf("listing of unknown ratings: %v more=%v, catalog requests %v", names(page.Items), page.More, addon.catalogRequests())
	}
	// A catalog whose titles are all hidden is read a few pages deep, not
	// whole.
	before := len(addon.catalogRequests())
	page = e.children(child, "Adult", 0, 20)
	if requests := len(addon.catalogRequests()) - before; len(page.Items) != 0 || !page.More || requests > hiddenReach {
		t.Errorf("listing of hidden titles: %v more=%v, %d catalog requests", names(page.Items), page.More, requests)
	}
}

func TestRatingsAreOnlyLookedUpWhenTheyMatter(t *testing.T) {
	e := newEnv(t)
	addon := ratedAddon()
	e.install(addons.Shared(), addon)
	// Books are hidden unrated, which no title of Polyfin is.
	books := e.restrict("books", accounts.ParentalControl{BlockUnrated: []string{"Book"}})
	if got := names(e.children(books, "Top", 0, 100).Items); len(got) != 6 || addon.metaRequests() != 0 {
		t.Errorf("listing for a user hiding unrated books: %v, %d meta requests", got, addon.metaRequests())
	}
	movies := e.restrict("movies", accounts.ParentalControl{BlockUnrated: []string{"Movie"}})
	if got := names(e.children(movies, "Shows", 0, 100).Items); len(got) != 2 || addon.metaRequests() != 0 {
		t.Errorf("shows for a user hiding unrated movies: %v, %d meta requests", got, addon.metaRequests())
	}
}

func TestOldRatingsAreAskedAgain(t *testing.T) {
	e := newEnv(t)
	addon := ratedAddon()
	e.install(addons.Shared(), addon)
	child := e.restrict("child", accounts.ParentalControl{BlockUnrated: []string{"Movie"}})
	if got := names(e.children(child, "Top", 0, 100).Items); slices.Contains(got, "Homemade") {
		t.Fatalf("unrated movie listed: %v", got)
	}
	// The movie gets a rating; a day later, the listing asks again.
	addon.mu.Lock()
	addon.metas["movie/tt3"] = stremio.Meta{ID: "tt3", Type: "movie", Name: "Homemade", Extras: &stremio.Extras{Certification: "PG"}}
	addon.mu.Unlock()
	e.service = New(e.service.db, e.addons, stremio.NewClient("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	e.service.now = func() time.Time { return time.Now().Add(24 * time.Hour) }
	deadline := time.Now().Add(5 * time.Second)
	for !slices.Contains(names(e.children(child, "Top", 0, 100).Items), "Homemade") {
		if time.Now().After(deadline) {
			t.Fatal("the new rating never reached the listing")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

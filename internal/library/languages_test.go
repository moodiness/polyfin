package library

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// describedAddon lists a movie and a series, and describes them, a third
// title too, which it lists in no catalog.
func describedAddon() *fakeAddon {
	movie := stremio.Meta{ID: "tt1", Type: "movie", Name: "The Heist", Poster: "https://images.example/heist.jpg",
		Description: "A crew plans a heist.", Genres: stremio.Names{"Drama"}, Cast: stremio.Names{"Ana Actor"},
		Extras: &stremio.Extras{Certification: "PG"}}
	plain := stremio.Meta{ID: "tt3", Type: "movie", Name: "Plain", Genres: stremio.Names{"Comedy"}, Extras: &stremio.Extras{Certification: "PG"}}
	show := stremio.Meta{ID: "tt2", Type: "series", Name: "The Show", Poster: "https://images.example/show.jpg", Genres: stremio.Names{"Comedy"},
		Extras: &stremio.Extras{Certification: "TV-PG"},
		Videos: []stremio.Video{
			{ID: "tt2:1:1", Title: "Pilot", Season: 1, Episode: 1, Thumbnail: "https://images.example/pilot.jpg", Released: "2020-01-01T00:00:00Z"},
			{ID: "tt2:1:2", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00Z"},
		}}
	row := func(m stremio.Meta) stremio.Meta {
		m.Videos, m.Extras, m.Cast, m.Description = nil, nil, nil, ""
		return m
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Drama", "Comedy"}}}},
				{Type: "series", ID: "top", Name: "Top"},
				{Type: "movie", ID: "search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}},
			}},
		catalogs: map[string][]stremio.Meta{"movie/top": {row(movie), row(plain)}, "series/top": {row(show)},
			"movie/search": {row(movie), row(plain)}},
		metas: map[string]stremio.Meta{"movie/tt1": movie, "movie/tt3": plain, "series/tt2": show},
	}
}

// translatedAddon describes the titles of describedAddon in a language,
// but the third, with other genres, ratings and people, which must not
// count, and searches by its names.
func translatedAddon(language, movieName, showName, pilotName string) *fakeAddon {
	movie := stremio.Meta{ID: "tt1", Type: "movie", Name: movieName, Poster: "https://images.example/heist-" + language + ".jpg",
		Background: "https://images.example/heist-backdrop-" + language + ".jpg", Description: "Overview " + language,
		Genres: stremio.Names{"Genre " + language}, Cast: stremio.Names{"Someone " + language}, Extras: &stremio.Extras{Certification: "NC-17"}}
	show := stremio.Meta{ID: "tt2", Type: "series", Name: showName, Poster: "https://images.example/show-" + language + ".jpg",
		Genres: stremio.Names{"Genre " + language}, Extras: &stremio.Extras{Certification: "TV-MA"},
		Videos: []stremio.Video{
			{ID: "tt2:1:1", Title: pilotName, Season: 1, Episode: 1, Thumbnail: "https://images.example/pilot-" + language + ".jpg", Overview: "Pilot " + language},
		}}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "a." + language, Name: "A " + language, Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}},
			}},
		catalogs: map[string][]stremio.Meta{"movie/search": {{ID: "tt1", Type: "movie", Name: movieName, Poster: movie.Poster}}},
		metas:    map[string]stremio.Meta{"movie/tt1": movie, "series/tt2": show},
	}
}

// languageEnv installs translatedAddon in French and in German as the
// server's addons in those languages, first, so that they would describe
// titles before describedAddon if they could, then describedAddon, then a
// list addon that lists the movie without describing it. Its member reads
// French, and german German; the administrator keeps the server's
// descriptions.
type languageEnv struct {
	env
	base, french, german *fakeAddon
	germanUser           accounts.User
}

func newLanguageEnv(t *testing.T) languageEnv {
	t.Helper()
	e := languageEnv{env: newEnv(t), base: describedAddon(), french: translatedAddon("fr", "Le Casse", "La Série", "Le pilote"),
		german: translatedAddon("de", "Der Coup", "Die Serie", "Der Pilot")}
	e.installIn(e.french, "fr")
	e.installIn(e.german, "de")
	e.install(addons.Shared(), e.base)
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "picks", Name: "Picks", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "picks", Name: "Picks"}}},
		catalogs: map[string][]stremio.Meta{"movie/picks": {{ID: "tt1", Type: "movie", Name: "The Heist", Genres: stremio.Names{"Drama"}}}},
	})
	e.member = e.speaking(e.member, "fr")
	german, err := e.users.CreateUser(t.Context(), accounts.NewUser{Name: "german", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	e.germanUser = e.speaking(german, "de")
	return e
}

// installIn installs addon for the server in a language.
func (e languageEnv) installIn(addon *fakeAddon, language string) accounts.ID {
	e.t.Helper()
	e.install(addons.Shared(), addon)
	list, err := e.addons.Addons(e.t.Context(), addons.Shared())
	if err != nil {
		e.t.Fatal(err)
	}
	for _, installed := range list {
		if installed.Manifest.ID == addon.manifest.ID {
			if _, err := e.addons.SetLanguage(e.t.Context(), addons.Shared(), installed.ID, language); err != nil {
				e.t.Fatal(err)
			}
			return installed.ID
		}
	}
	e.t.Fatalf("addon %s not installed", addon.manifest.ID)
	return accounts.ID{}
}

// speaking sets a user's metadata language.
func (e languageEnv) speaking(user accounts.User, language string) accounts.User {
	e.t.Helper()
	user, err := e.users.UpdateUser(e.t.Context(), user.ID, accounts.UserChanges{MetadataLanguage: &language}, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return user
}

// titleIDs lists the titles of the library named name for the
// administrator, by name.
func (e languageEnv) titleIDs(name string) map[string]accounts.ID {
	e.t.Helper()
	ids := map[string]accounts.ID{}
	for _, item := range e.children(e.admin, name, 0, 10).Items {
		ids[item.Name] = item.ID
	}
	return ids
}

// translated lists a library for user until the title at index shows want,
// as descriptions read in the background come.
func (e languageEnv) translated(user accounts.User, library string, index int, want string) Page {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		page := e.children(user, library, 0, 10)
		if len(page.Items) > index && page.Items[index].Name == want {
			return page
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("%s for %s: %v, want %q at %d", library, user.Name, names(page.Items), want, index)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestUsersSeeATitleInTheirLanguage(t *testing.T) {
	t.Parallel()
	e := newLanguageEnv(t)
	movies := e.titleIDs("Top (Movies)")
	shows := e.titleIDs("Top (Shows)")

	// Each user sees the movie's page in their language, its facts those
	// of the addon that listed it.
	for _, c := range []struct {
		user                 accounts.User
		name, overview, art  string
		backdrop, seasonName string
	}{
		{e.admin, "The Heist", "A crew plans a heist.", "https://images.example/heist.jpg", "", "Season 1"},
		{e.member, "Le Casse", "Overview fr", "https://images.example/heist-fr.jpg", "https://images.example/heist-backdrop-fr.jpg", "Saison 1"},
		{e.germanUser, "Der Coup", "Overview de", "https://images.example/heist-de.jpg", "https://images.example/heist-backdrop-de.jpg", "Season 1"},
	} {
		item, err := e.service.Item(t.Context(), c.user, movies["The Heist"])
		if err != nil {
			t.Fatalf("%s: %v", c.user.Name, err)
		}
		if item.Name != c.name || item.Overview != c.overview || item.Images.Primary != c.art || item.Images.Backdrop != c.backdrop {
			t.Errorf("%s sees %q %q %q %q", c.user.Name, item.Name, item.Overview, item.Images.Primary, item.Images.Backdrop)
		}
		if !slices.Equal(item.Genres, []string{"Drama"}) || item.OfficialRating != "PG" || len(item.People) != 1 || item.People[0].Name != "Ana Actor" {
			t.Errorf("%s: facts %v %q %+v", c.user.Name, item.Genres, item.OfficialRating, item.People)
		}

		// The series' episodes too; an episode the translation leaves out
		// keeps the description's, and seasons are named in English or
		// French, else in the server's language.
		seasons, err := e.service.Seasons(t.Context(), c.user, shows["The Show"])
		if err != nil || len(seasons) != 1 || seasons[0].Name != c.seasonName {
			t.Errorf("%s: seasons %v %v", c.user.Name, names(seasons), err)
			continue
		}
		episodes, err := e.service.Episodes(t.Context(), c.user, shows["The Show"], &seasons[0].ID)
		if err != nil || len(episodes) != 2 {
			t.Fatalf("%s: episodes %v %v", c.user.Name, names(episodes), err)
		}
		pilot := map[string]string{"admin": "Pilot", "member": "Le pilote", "german": "Der Pilot"}[c.user.Name]
		if episodes[0].Name != pilot || episodes[0].SeriesName == "The Show" && c.user.ID != e.admin.ID {
			t.Errorf("%s: episodes %v of %q", c.user.Name, names(episodes), episodes[0].SeriesName)
		}
		if c.user.ID != e.admin.ID && (episodes[0].Images.Primary != "https://images.example/pilot-"+c.user.MetadataLanguage+".jpg" ||
			episodes[1].Name != map[string]string{"member": "Épisode 2", "german": "Episode 2"}[c.user.Name]) {
			t.Errorf("%s: episodes %q %q", c.user.Name, episodes[0].Images.Primary, episodes[1].Name)
		}
		if episode, err := e.service.Item(t.Context(), c.user, episodes[0].ID); err != nil || episode.Name != pilot {
			t.Errorf("%s: the episode by identifier: %q %v", c.user.Name, episode.Name, err)
		}
	}

	// Generated library names follow the user's language too.
	if got := names(mustLibraries(t, e.env, e.member)); !slices.Equal(got, []string{"Top (Films)", "Top (Séries)", "Picks"}) {
		t.Errorf("libraries in French: %v", got)
	}

	// Images come without a user: the address of the description kept stays
	// the title's own, and a tag finds the one a user's language gave.
	if art, err := e.service.Artwork(t.Context(), movies["The Heist"], "Primary"); err != nil || art.URL != "https://images.example/heist.jpg" {
		t.Errorf("artwork: %q %v", art.URL, err)
	}
	for _, url := range []string{"https://images.example/heist-fr.jpg", "https://images.example/heist-de.jpg", "https://images.example/heist.jpg"} {
		if art, ok := e.service.TaggedArtwork(t.Context(), movies["The Heist"], ImageTag(url)); !ok || art.URL != url {
			t.Errorf("artwork tagged for %s: %q %v", url, art.URL, ok)
		}
	}
	episodes, _ := e.service.Episodes(t.Context(), e.member, shows["The Show"], nil)
	if art, ok := e.service.TaggedArtwork(t.Context(), episodes[0].ID, ImageTag("https://images.example/pilot-fr.jpg")); !ok ||
		art.URL != "https://images.example/pilot-fr.jpg" {
		t.Errorf("episode artwork tagged: %q %v", art.URL, ok)
	}
	if _, ok := e.service.TaggedArtwork(t.Context(), movies["The Heist"], ImageTag("https://images.example/elsewhere.jpg")); ok {
		t.Error("a tag of no description of the title found an image")
	}

	// The administrator's page stays in the server's language once the
	// translations are kept: they are kept per language.
	if item, _ := e.service.Item(t.Context(), e.admin, movies["The Heist"]); item.Name != "The Heist" {
		t.Errorf("the administrator after the translations: %q", item.Name)
	}
}

func mustLibraries(t *testing.T, e env, user accounts.User) []Item {
	t.Helper()
	libraries, err := e.service.Libraries(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	return libraries
}

func TestAListingShowsTheTranslationOnceKept(t *testing.T) {
	t.Parallel()
	e := newLanguageEnv(t)
	gate := make(chan struct{})
	e.french.mu.Lock()
	e.french.metaGate = gate
	e.french.mu.Unlock()

	// While the French description waits, the listing shows the catalog's.
	page := e.children(e.member, "Top (Films)", 0, 10)
	if got := names(page.Items); !slices.Equal(got, []string{"The Heist", "Plain"}) || page.Items[0].Images.Primary != "https://images.example/heist.jpg" {
		t.Fatalf("before the translation: %v %q", got, page.Items[0].Images.Primary)
	}
	close(gate)
	page = e.translated(e.member, "Top (Films)", 0, "Le Casse")
	if page.Items[0].Images.Primary != "https://images.example/heist-fr.jpg" || page.Items[1].Name != "Plain" {
		t.Errorf("once kept: %v %q", names(page.Items), page.Items[0].Images.Primary)
	}
	// Home rows show it too, and the other users their own.
	latest, err := e.service.Latest(t.Context(), e.member, e.library(e.member, "Top (Films)").ID, 10)
	if err != nil || len(latest.Items) == 0 || latest.Items[0].Name != "Le Casse" {
		t.Errorf("home row: %v %v", names(latest.Items), err)
	}
	if got := names(e.children(e.admin, "Top (Movies)", 0, 10).Items); !slices.Equal(got, []string{"The Heist", "Plain"}) {
		t.Errorf("the administrator's listing: %v", got)
	}
	e.translated(e.germanUser, "Top (Movies)", 0, "Der Coup")
}

// Genres, blocked genres, hidden titles, parental control and smart lists
// judge the facts of the addon that listed a title, whatever the user's
// language, and a user under parental control sees their language too.
func TestATranslationChangesNoFacts(t *testing.T) {
	t.Parallel()
	e := newLanguageEnv(t)
	movies := e.titleIDs("Top (Movies)")
	// The French description is kept first, so that it could count.
	e.translated(e.member, "Top (Films)", 0, "Le Casse")

	// A genre page lists the title by its genre, not the translation's.
	library := e.library(e.member, "Top (Films)")
	drama, err := e.service.Children(t.Context(), e.member, library.ID, 0, 10, "Drama")
	if err != nil || !slices.Equal(names(drama.Items), []string{"Le Casse"}) {
		t.Errorf("drama page: %v %v", names(drama.Items), err)
	}

	// A blocked genre hides by the listing's genres; the translation's
	// genres hide nothing.
	blocking := e.speaking(e.restrict("blocking", accounts.ParentalControl{}), "fr")
	blocking, err = e.users.UpdateUser(t.Context(), blocking.ID, accounts.UserChanges{BlockedGenres: &[]string{"Drama", "Genre fr"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(e.children(blocking, "Top (Films)", 0, 10).Items); !slices.Equal(got, []string{"Plain"}) {
		t.Errorf("blocking drama: %v", got)
	}
	shows := e.children(blocking, "Top (Séries)", 0, 10)
	if len(shows.Items) != 1 {
		t.Errorf("the comedy series, whose translation is of a blocked genre: %v", names(shows.Items))
	}

	// Parental control judges the listing addon's rating, PG, not the
	// translation's NC-17, and the child sees French.
	child := e.speaking(e.restrict("child", accounts.ParentalControl{MaxRating: new(13)}), "fr")
	item, err := e.service.Item(t.Context(), child, movies["The Heist"])
	if err != nil || item.Name != "Le Casse" || item.OfficialRating != "PG" {
		t.Errorf("the child's page: %q %q %v", item.Name, item.OfficialRating, err)
	}
	if got := names(e.translated(child, "Top (Films)", 0, "Le Casse").Items); !slices.Equal(got, []string{"Le Casse", "Plain"}) {
		t.Errorf("the child's listing: %v", got)
	}

	// A title listed by an addon that does not describe it is described by
	// the next addon without a language, never by a translator that comes
	// first: its facts stay, for every user.
	picked := e.children(e.admin, "Picks", 0, 10)
	if len(picked.Items) != 1 || picked.Items[0].ID != movies["The Heist"] {
		t.Fatalf("picks: %v", names(picked.Items))
	}
	if item, err := e.service.Item(t.Context(), e.admin, movies["The Heist"]); err != nil || item.Name != "The Heist" ||
		!slices.Equal(item.Genres, []string{"Drama"}) || item.OfficialRating != "PG" {
		t.Errorf("the picked title for the administrator: %q %v %q %v", item.Name, item.Genres, item.OfficialRating, err)
	}
	if item, err := e.service.Item(t.Context(), child, movies["The Heist"]); err != nil || item.Name != "Le Casse" || item.OfficialRating != "PG" {
		t.Errorf("the picked title for the child: %q %q %v", item.Name, item.OfficialRating, err)
	}
	if got := names(e.children(blocking, "Picks", 0, 10).Items); len(got) != 0 {
		t.Errorf("picks blocking drama: %v", got)
	}

	// A hidden title is left out by its identifier, which stays.
	if err := e.service.userData.Hide(t.Context(), e.member.ID, movies["The Heist"], time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := names(e.children(e.member, "Top (Films)", 0, 10).Items); !slices.Equal(got, []string{"Plain"}) {
		t.Errorf("after hiding: %v", got)
	}
	if _, err := e.service.userData.Show(t.Context(), e.member.ID, movies["The Heist"]); err != nil {
		t.Fatal(err)
	}

	// A smart list matches the listing addon's genres, and the French reader
	// sees its titles in French.
	var baseID accounts.ID
	installed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	for _, addon := range installed {
		if addon.Manifest.ID == "a" {
			baseID = addon.ID
		}
	}
	list := e.makeList(nil, "Dramas", baseID, []string{"movie/top"},
		group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldGenre, Value: "drama"}), smartlists.SortName, 0)
	if got := e.settled(e.member, list); !slices.Equal(got, []string{"Le Casse"}) {
		t.Errorf("smart list of dramas: %v", got)
	}
	french := e.makeList(nil, "French genre", baseID, []string{"movie/top"},
		group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldGenre, Value: "Genre fr"}), smartlists.SortName, 0)
	if got := e.settled(e.member, french); len(got) != 0 {
		t.Errorf("smart list of the translation's genre: %v", got)
	}
}

func TestATitleWithoutATranslationShowsTheServersDescription(t *testing.T) {
	t.Parallel()
	e := newLanguageEnv(t)
	movies := e.titleIDs("Top (Movies)")
	// Neither translator describes Plain.
	item, err := e.service.Item(t.Context(), e.member, movies["Plain"])
	if err != nil || item.Name != "Plain" || !slices.Equal(item.Genres, []string{"Comedy"}) {
		t.Errorf("an untranslated title: %q %v %v", item.Name, item.Genres, err)
	}
	// A language no addon has is the server's.
	spanish := e.speaking(e.admin, "es")
	if item, err := e.service.Item(t.Context(), spanish, movies["The Heist"]); err != nil || item.Name != "The Heist" {
		t.Errorf("a language no addon has: %q %v", item.Name, err)
	}
	if got := names(mustLibraries(t, e.env, spanish)); !slices.Equal(got, []string{"Top (Movies)", "Top (Shows)", "Picks"}) {
		t.Errorf("libraries in a language no addon has: %v", got)
	}
}

func TestSearchAsksTheTranslatorsOfTheUsersLanguage(t *testing.T) {
	t.Parallel()
	e := newLanguageEnv(t)
	movies := e.titleIDs("Top (Movies)")

	// The administrator's search asks no translator.
	if found, err := e.service.Search(t.Context(), e.admin, "casse", []Kind{KindMovie}, 10); err != nil || len(found) != 0 {
		t.Errorf("the administrator's search: %v %v", names(found), err)
	}
	if requests := e.french.catalogRequests(); len(requests) != 0 {
		t.Errorf("the administrator's search asked the French addon: %v", requests)
	}

	// The French reader finds the title by its French name; the German
	// addon is not asked.
	found, err := e.service.Search(t.Context(), e.member, "casse", []Kind{KindMovie}, 10)
	if err != nil || !slices.Equal(names(found), []string{"Le Casse"}) || found[0].ID != movies["The Heist"] {
		t.Errorf("the French search: %v %v", names(found), err)
	}
	for _, request := range e.german.catalogRequests() {
		if strings.Contains(request, "search") {
			t.Errorf("the French search asked the German addon: %s", request)
		}
	}
	// What the French search found changes nothing for the others.
	if item, err := e.service.Item(t.Context(), e.admin, movies["The Heist"]); err != nil || item.Name != "The Heist" ||
		item.Images.Primary != "https://images.example/heist.jpg" {
		t.Errorf("the administrator after the French search: %q %q %v", item.Name, item.Images.Primary, err)
	}
	if art, err := e.service.Artwork(t.Context(), movies["The Heist"], "Primary"); err != nil || art.URL != "https://images.example/heist.jpg" {
		t.Errorf("artwork after the French search: %q %v", art.URL, err)
	}
}

package library

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// knownCredit is a title of a person's combined credits on the fake TMDB.
func knownCredit(kind string, id int, name string, votes int, popularity float64, date string) map[string]any {
	credit := map[string]any{"id": id, "media_type": kind, "vote_count": votes, "popularity": popularity,
		"poster_path": fmt.Sprintf("/poster%d.jpg", id), "character": "Someone"}
	if kind == "tv" {
		credit["name"], credit["first_air_date"] = name, date
	} else {
		credit["title"], credit["release_date"] = name, date
	}
	return credit
}

// orderedTitles lists the names of the titles of kinds a person's page
// lists, in order, none when they list nothing.
func orderedTitles(t *testing.T, s *Service, user accounts.User, person accounts.ID, kinds ...Kind) []string {
	t.Helper()
	page, err := s.PersonTitles(t.Context(), user, person, kinds, 40)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		t.Fatalf("the titles of %s: %v", person, err)
	}
	return names(page.Items)
}

// While a key is saved, a person's page lists, after the titles of the
// libraries, the titles TMDB knows them best for: the most voted first,
// the most popular among equal votes, without talk shows, appearances as
// themselves, titles for adults and titles not out, 15 at most, the
// library titles among them listed once, first.
func TestAPersonListsTheTitlesTheyAreBestKnownFor(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	both := knownCredit("movie", 1605, "Both Ways", 9000, 1, "2015-05-05")
	both["character"] = "Self"
	cast := []any{
		knownCredit("movie", 1001, "Harbor Lights", 5000, 1, "2011-01-01"),
		knownCredit("movie", 1011, "Salt Road", 10, 1, "2011-01-01"),
		knownCredit("tv", 2501, "Long Watch", 3000, 1, "2016-01-01"),
		both,
	}
	for i := range 10 {
		cast = append(cast, knownCredit("movie", 1501+i, "Known "+strconv.Itoa(1501+i), 2000-10*i, 1, "2012-01-01"))
	}
	cast = append(cast,
		knownCredit("movie", 1513, "Less Popular", 1500, 1, "2013-01-01"),
		knownCredit("movie", 1514, "More Popular", 1500, 50, "2013-01-01"),
		knownCredit("movie", 1515, "Sixteenth", 1400, 1, "2013-01-01"))
	talk := knownCredit("tv", 2901, "Late Talk", 9999, 99, "2001-01-01")
	talk["genre_ids"] = []int{35, 10767}
	herself := knownCredit("movie", 1601, "Herself Story", 9998, 99, "2001-01-01")
	herself["character"] = "Herself (archive footage)"
	adult := knownCredit("movie", 1602, "Adult Title", 9997, 99, "2001-01-01")
	adult["adult"] = true
	cast = append(cast, talk, herself, adult,
		knownCredit("movie", 1603, "Coming Soon", 9996, 99, "2099-01-01"),
		knownCredit("movie", 1604, "No Date", 9995, 99, ""))
	e.tmdb.answers["/person/6000/combined_credits"] = map[string]any{"id": 6000, "cast": cast,
		"crew": []any{map[string]any{"id": 1605, "media_type": "movie", "title": "Both Ways", "job": "Director",
			"release_date": "2015-05-05", "vote_count": 9000, "popularity": 1}}}
	e.open(e.admin, "Harbor Lights")
	ana := itemID(personTMDBKey("6000"))

	want := []string{"Harbor Lights", "Salt Road", "Both Ways", "Long Watch"}
	for i := range 10 {
		want = append(want, "Known "+strconv.Itoa(1501+i))
	}
	want = append(want, "More Popular", "Less Popular")
	// A user who hid two of them gets the next one in their place; that
	// one is no title of the others'.
	for _, hidden := range []accounts.ID{itemID(titleKey(KindMovie, "tmdb:1605")), itemID(titleKey(KindSeries, "tmdb:2501"))} {
		if err := e.service.userData.Hide(t.Context(), e.member.ID, hidden, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if got := orderedTitles(t, e.service, e.member, ana, KindMovie, KindSeries); !slices.Equal(got, append(slices.Concat(want[:2], want[4:]), "Sixteenth")) {
		t.Errorf("Ana Actor's titles for a user who hid two:\n got %v", got)
	}
	// They open Sixteenth, whose description credits Ana Actor: Polyfin
	// knows her in it, which lists it only among the 15.
	sixteenth := stremio.Meta{ID: "tmdb:1515", Type: "movie", Name: "Sixteenth", Cast: addonCast}
	e.addon.mu.Lock()
	e.addon.metas["movie/tmdb:1515"] = sixteenth
	e.addon.mu.Unlock()
	if _, err := e.service.Item(t.Context(), e.member, itemID(titleKey(KindMovie, "tmdb:1515"))); err != nil {
		t.Fatal(err)
	}
	if got := orderedTitles(t, e.service, e.admin, ana, KindMovie, KindSeries); !slices.Equal(got, want) {
		t.Errorf("Ana Actor's titles:\n got %v\nwant %v", got, want)
	}
	if got := orderedTitles(t, e.service, e.admin, ana, KindSeries); !slices.Equal(got, []string{"Long Watch"}) {
		t.Errorf("Ana Actor's series: %v", got)
	}
	// They open through the addon, by their TMDB identifier, which it
	// describes: no external identifiers are asked.
	page, err := e.service.PersonTitles(t.Context(), e.admin, ana, []Kind{KindMovie}, 40)
	if err != nil || len(page.Items) < 3 {
		t.Fatalf("Ana Actor's movies: %v, %v", names(page.Items), err)
	}
	opened, err := e.service.Item(t.Context(), e.admin, page.Items[2].ID)
	if err != nil || opened.Name != "Both Ways" || opened.StremioID != "tmdb:1605" || opened.ProviderIDs["Tmdb"] != "1605" {
		t.Errorf("opening Both Ways: %+v, %v", opened, err)
	}
	if asked := e.tmdb.requests("/3/movie/1501/external_ids"); len(asked) != 0 {
		t.Errorf("external identifiers asked: %v", asked)
	}
	if asked := e.tmdb.requests("/3/person/6000/combined_credits"); len(asked) != 1 {
		t.Errorf("Ana Actor's credits were asked %d times", len(asked))
	}
	// Without a key, she lists the titles she is known in only, and TMDB
	// is asked nothing.
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	if got := orderedTitles(t, e.service, e.admin, ana, KindMovie, KindSeries); !slices.Equal(got, []string{"Harbor Lights", "Sixteenth"}) {
		t.Errorf("Ana Actor's titles without a key: %v", got)
	}
	if asked := e.tmdb.requests("/3/person/6000/combined_credits"); len(asked) != 1 {
		t.Errorf("Ana Actor's credits were asked %d times", len(asked))
	}
}

// knownForEnv is a person, kept by the TMDB identifier 7000 and credited
// in no title, on a fake TMDB, and the server's addon describing titles
// of manifest.
type knownForEnv struct {
	env
	tmdb   *fakeCreditsTMDB
	addon  *fakeAddon
	person accounts.ID
}

func newKnownForEnv(t *testing.T, resources []stremio.Resource, credits []any) knownForEnv {
	t.Helper()
	e := knownForEnv{env: newEnv(t), tmdb: newFakeCreditsTMDB(t)}
	client := tmdb.New(e.tmdb.URL+"/3", e.tmdb.URL+"/img/", "test", e.users.Settings)
	e.service.UseTMDB(client)
	queue := tmdb.NewQueue(client, tmdb.QueueOptions{Interval: time.Millisecond, PassCheck: time.Hour})
	t.Cleanup(queue.Close)
	e.service.UseTitleCredits(queue)
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	e.addon = &fakeAddon{
		manifest: stremio.Manifest{ID: "described", Name: "Described", Version: "1", Types: []string{"movie", "series"}, Resources: resources},
		metas:    map[string]stremio.Meta{},
	}
	e.install(addons.Shared(), e.addon)
	e.tmdb.answers["/person/7000/combined_credits"] = map[string]any{"id": 7000, "cast": credits}
	key := personTMDBKey("7000")
	e.person = itemID(key)
	if err := e.service.save(t.Context(), []record{{ID: e.person, Key: key, Kind: KindPerson, Person: &Person{Name: "Zoe Known"}, TMDBID: "7000"}}); err != nil {
		t.Fatal(err)
	}
	return e
}

// A title is listed by its IMDb identifier, from TMDB's external
// identifiers, asked once, when the user's addons describe titles by IMDb
// identifier only; titles none of them describes are left out.
func TestTitlesNoAddonDescribesAreLeftOut(t *testing.T) {
	t.Parallel()
	e := newKnownForEnv(t, []stremio.Resource{{Name: "meta", Types: []string{"movie"}, IDPrefixes: []string{"tt"}}}, []any{
		knownCredit("movie", 1701, "By IMDb", 300, 1, "2010-01-01"),
		knownCredit("movie", 1702, "No IMDb", 200, 1, "2010-01-01"),
		knownCredit("tv", 2701, "A Series", 100, 1, "2010-01-01"),
	})
	e.tmdb.answers["/movie/1701/external_ids"] = map[string]any{"imdb_id": "tt0170100"}
	e.tmdb.answers["/movie/1702/external_ids"] = map[string]any{"imdb_id": nil}
	e.addon.metas["movie/tt0170100"] = stremio.Meta{ID: "tt0170100", Type: "movie", Name: "By IMDb, described"}
	for range 2 {
		if got := orderedTitles(t, e.service, e.admin, e.person, KindMovie, KindSeries); !slices.Equal(got, []string{"By IMDb"}) {
			t.Errorf("Zoe Known's titles: %v", got)
		}
	}
	for _, path := range []string{"/3/movie/1701/external_ids", "/3/movie/1702/external_ids"} {
		if asked := e.tmdb.requests(path); len(asked) != 1 {
			t.Errorf("%s asked %d times", path, len(asked))
		}
	}
	if asked := e.tmdb.requests("/3/tv/"); len(asked) != 0 {
		t.Errorf("a series no addon describes was looked up: %v", asked)
	}
	page, err := e.service.PersonTitles(t.Context(), e.admin, e.person, []Kind{KindMovie}, 40)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := e.service.Item(t.Context(), e.admin, page.Items[0].ID)
	if err != nil || opened.Name != "By IMDb, described" || opened.StremioID != "tt0170100" {
		t.Errorf("opening By IMDb: %+v, %v", opened, err)
	}
}

// Parental control and hidden titles apply to the titles a person is known
// for, and a restricted user sees a title only once its rating is known.
func TestParentalControlAppliesToTheTitlesAPersonIsKnownFor(t *testing.T) {
	t.Parallel()
	e := newKnownForEnv(t, []stremio.Resource{{Name: "meta", IDPrefixes: []string{"tmdb:"}}}, []any{
		knownCredit("movie", 1801, "Family", 400, 1, "2010-01-01"),
		knownCredit("movie", 1802, "Crime", 300, 1, "2010-01-01"),
		knownCredit("movie", 1803, "Cartoon", 200, 1, "2010-01-01"),
		knownCredit("movie", 1804, "Shown", 100, 1, "2010-01-01"),
	})
	for _, m := range []struct{ id, name, rating string }{{"1801", "Family", "PG"}, {"1802", "Crime", "R"}, {"1803", "Cartoon", "G"}, {"1804", "Shown", "PG"}} {
		e.addon.metas["movie/tmdb:"+m.id] = stremio.Meta{ID: "tmdb:" + m.id, Type: "movie", Name: m.name, Extras: &stremio.Extras{Certification: m.rating}}
	}
	e.addon.metaGate = make(chan struct{})
	e.service.ratingWait = 50 * time.Millisecond
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	everyone := []string{"Family", "Crime", "Cartoon", "Shown"}
	if got := orderedTitles(t, e.service, e.admin, e.person, KindMovie); !slices.Equal(got, everyone) {
		t.Errorf("Zoe Known's titles: %v", got)
	}
	if err := e.service.userData.Hide(t.Context(), e.admin.ID, e.titleOf(t, "Shown"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := orderedTitles(t, e.service, e.admin, e.person, KindMovie); !slices.Equal(got, everyone[:3]) {
		t.Errorf("Zoe Known's titles once Shown is hidden: %v", got)
	}
	// Their ratings are not known yet.
	if got := orderedTitles(t, e.service, child, e.person, KindMovie); len(got) != 0 {
		t.Errorf("Zoe Known's titles for a child before their ratings are known: %v", got)
	}
	close(e.addon.metaGate)
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := orderedTitles(t, e.service, child, e.person, KindMovie)
		if slices.Equal(got, []string{"Family", "Cartoon", "Shown"}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Zoe Known's titles for a child once their ratings are known: %v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// titleOf is the item of the title named name the person's page lists to
// the administrator.
func (e knownForEnv) titleOf(t *testing.T, name string) accounts.ID {
	t.Helper()
	page, err := e.service.PersonTitles(t.Context(), e.admin, e.person, []Kind{KindMovie, KindSeries}, 40)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Name == name {
			return item.ID
		}
	}
	t.Fatalf("no %q among %v", name, names(page.Items))
	return accounts.ID{}
}

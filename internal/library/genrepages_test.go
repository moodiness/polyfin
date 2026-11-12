package library

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// genreNames lists the names of the titles of kinds of a genre's page.
func genreNames(t *testing.T, s *Service, user accounts.User, name string, kinds ...Kind) []string {
	t.Helper()
	page, err := s.GenreTitles(t.Context(), user, []string{name}, func(option string) bool { return option == name }, kinds, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.More || page.Total != len(page.Items) {
		t.Errorf("%s's page: total %d of %d, more %v", name, page.Total, len(page.Items), page.More)
	}
	return names(page.Items)
}

// A genre no catalog filters by lists the movies and series Polyfin keeps
// of it, whatever the letter case of their genres, after its catalogs'
// titles, each once, by name; an administrator's genres win over the
// addons'. Parental control, blocked genres, hidden titles and the wait
// for a movie's digital release apply.
func TestAGenrePageListsTheTitlesKeptOfIt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.service.ratingWait = 30 * time.Second
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	e.service.SetClock(func() time.Time { return now })
	// Echo Lane is in cinemas only, since 1 October.
	f := newFakeReleasesTMDB(t, map[string]releasesOf{"tt955": {955, map[int]string{3: "2026-10-01"}}})
	e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	title := func(id, kind, name, rating, released string, genres ...string) stremio.Meta {
		return stremio.Meta{ID: id, Type: kind, Name: name, ReleaseInfo: stremio.Text(released[:4]), Released: released + "T00:00:00.000Z",
			Genres: genres, Extras: &stremio.Extras{Certification: rating}}
	}
	movies := []stremio.Meta{
		title("tt951", "movie", "Ash Road", "PG", "2001-05-05", "Drama"),
		title("tt952", "movie", "Blue Hour", "PG", "2002-05-05", "Drama", "Crime"),
		title("tt953", "movie", "Cold Night", "R", "2003-05-05", "drama"),
		title("tt954", "movie", "Dawn Light", "PG", "2004-05-05", "Comedy"),
		title("tt955", "movie", "Echo Lane", "PG", "2026-10-01", "Drama"),
		title("tt956", "movie", "Fern Hill", "PG", "2005-05-05", "Comedy"),
		title("tt957", "movie", "Gray Dock", "PG", "2006-05-05", "Drama"),
	}
	shows := []stremio.Meta{title("tt961", "series", "Harbor Tales", "TV-PG", "2010-05-05", "DRAMA")}
	shelf := &fakeAddon{
		manifest: stremio.Manifest{ID: "shelf", Name: "Shelf", Version: "1", Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "shelf", Name: "Shelf"}, {Type: "series", ID: "shows", Name: "Shows"}}},
		catalogs: map[string][]stremio.Meta{"movie/shelf": movies, "series/shows": shows},
		metas:    map[string]stremio.Meta{},
	}
	for _, m := range slices.Concat(movies, shows) {
		shelf.metas[m.Type+"/"+m.ID] = m
	}
	e.install(addons.Shared(), shelf)
	ids := map[string]accounts.ID{}
	for _, library := range []string{"Shelf", "Shows"} {
		for _, item := range e.children(e.admin, library, 0, 20).Items {
			ids[item.Name] = item.ID
		}
	}
	// Before the edits and the catalog filtering by genre: the kept titles
	// alone.
	if got, want := genreNames(t, e.service, e.admin, "Drama", KindMovie, KindSeries),
		[]string{"Ash Road", "Blue Hour", "Cold Night", "Echo Lane", "Gray Dock", "Harbor Tales"}; !slices.Equal(got, want) {
		t.Errorf("Drama's titles kept:\n got %v\nwant %v", got, want)
	}
	// An administrator makes Fern Hill a drama, and Gray Dock a comedy.
	for name, genres := range map[string][]string{"Fern Hill": {"Drama"}, "Gray Dock": {"Comedy"}} {
		base, err := e.service.Original(t.Context(), e.admin, ids[name])
		if err != nil {
			t.Fatal(err)
		}
		if err := e.service.SaveOverrides(t.Context(), base, Overrides{Genres: genres}); err != nil {
			t.Fatal(err)
		}
	}
	picks := &fakeAddon{
		manifest: stremio.Manifest{ID: "picks", Name: "Picks", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "picks", Name: "Picks", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Drama"}}}}}},
		catalogs: map[string][]stremio.Meta{"movie/picks": {
			{ID: "pick:1", Type: "movie", Name: "Catalog Drama", Genres: stremio.Names{"Drama"}},
			movies[0],
		}},
	}
	e.install(addons.Shared(), picks)
	want := []string{"Catalog Drama", "Ash Road", "Blue Hour", "Cold Night", "Echo Lane", "Fern Hill", "Harbor Tales"}
	if got := genreNames(t, e.service, e.admin, "Drama", KindMovie, KindSeries); !slices.Equal(got, want) {
		t.Fatalf("Drama's page:\n got %v\nwant %v", got, want)
	}
	if got := genreNames(t, e.service, e.admin, "Drama", KindSeries); !slices.Equal(got, []string{"Harbor Tales"}) {
		t.Errorf("its series: %v", got)
	}
	if got := genreNames(t, e.service, e.admin, "Comedy", KindMovie); !slices.Equal(got, []string{"Dawn Light", "Gray Dock"}) {
		t.Errorf("Comedy's movies: %v", got)
	}
	without := func(names ...string) []string {
		return slices.DeleteFunc(slices.Clone(want), func(name string) bool { return slices.Contains(names, name) })
	}

	// Parental control leaves out Cold Night (R), blocked crimes Blue Hour,
	// and a user's hidden titles theirs.
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	if got := genreNames(t, e.service, child, "Drama", KindMovie, KindSeries); !slices.Equal(got, without("Cold Night")) {
		t.Errorf("for a child: %v", got)
	}
	calm, err := e.users.CreateUser(t.Context(), accounts.NewUser{Name: "calm", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if calm, err = e.users.UpdateUser(t.Context(), calm.ID, accounts.UserChanges{BlockedGenres: &[]string{"crime"}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := genreNames(t, e.service, calm, "Drama", KindMovie, KindSeries); !slices.Equal(got, without("Blue Hour")) {
		t.Errorf("for a user blocking crimes: %v", got)
	}
	if err := e.service.userData.Hide(t.Context(), e.member.ID, ids["Fern Hill"], time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := genreNames(t, e.service, e.member, "Drama", KindMovie, KindSeries); !slices.Equal(got, without("Fern Hill")) {
		t.Errorf("for a user who hid Fern Hill: %v", got)
	}

	// Echo Lane waits for its digital release.
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y"; s.WaitDigitalRelease = true })
	if got := genreNames(t, e.service, e.admin, "Drama", KindMovie, KindSeries); !slices.Equal(got, without("Echo Lane")) {
		t.Errorf("with the wait for digital releases: %v", got)
	}

	// The genres kept, each once as most titles spell it, the most titles
	// first, and those an administrator set.
	genres, err := e.service.GenreNames(t.Context())
	if err != nil || !slices.Equal(genres, []string{"Drama", "Comedy", "Crime"}) {
		t.Errorf("the genres kept: %q %v", genres, err)
	}
}

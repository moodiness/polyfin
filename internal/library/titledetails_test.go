package library

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// details gives the TMDB answer of path, a movie's or series' details
// with its credits, networks, companies and tagline.
func (f *fakeCreditsTMDB) details(path string, networks, companies []string, tagline string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	named := func(names []string) []any {
		list := []any{}
		for _, name := range names {
			list = append(list, map[string]any{"name": name})
		}
		return list
	}
	answer := f.answers[path].(map[string]any)
	answer["networks"], answer["production_companies"], answer["tagline"] = named(networks), named(companies), tagline
}

// A movie's studios are its companies, a series' its networks then its
// companies, each once, and both show TMDB's tagline, with their credits:
// on their page, and in a listing of several.
func TestTitlesTakeTheirStudiosAndTaglineFromTMDB(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.tmdb.details("/movie/1001", []string{"Ignored TV"}, []string{"North Pictures", "South Films"}, "Une lumière.")
	e.tmdb.details("/tv/2031", []string{"Coast TV", "Coast Streams"}, []string{"Coast TV", "Harbor Works"}, "Chaque marée.")

	harbor := e.open(e.admin, "Harbor Lights")
	if !slices.Equal(harbor.Studios, []string{"North Pictures", "South Films"}) || !slices.Equal(harbor.Taglines, []string{"Une lumière."}) {
		t.Errorf("Harbor Lights: %q %q", harbor.Studios, harbor.Taglines)
	}
	tidewater := e.open(e.admin, "Tidewater")
	if !slices.Equal(tidewater.Studios, []string{"Coast TV", "Coast Streams", "Harbor Works"}) ||
		!slices.Equal(tidewater.Taglines, []string{"Chaque marée."}) {
		t.Errorf("Tidewater: %q %q", tidewater.Studios, tidewater.Taglines)
	}
	items, err := e.service.Items(t.Context(), e.admin, []accounts.ID{e.titles["Tidewater"], e.titles["Harbor Lights"], e.titles["Quiet Pier"]})
	if err != nil || len(items) != 3 {
		t.Fatalf("%d items: %v", len(items), err)
	}
	if !slices.Equal(items[0].Studios, tidewater.Studios) || !slices.Equal(items[1].Taglines, harbor.Taglines) ||
		items[2].Studios != nil || items[2].Taglines != nil {
		t.Errorf("listed: %q %q %q %q", items[0].Studios, items[1].Taglines, items[2].Studios, items[2].Taglines)
	}
	// Without a key, they are gone.
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	if item, err := e.service.Item(t.Context(), e.admin, e.titles["Harbor Lights"]); err != nil || item.Studios != nil || item.Taglines != nil {
		t.Errorf("without a key: %q %q %v", item.Studios, item.Taglines, err)
	}
}

// An administrator's studios and tagline win over TMDB's; the editor's
// save of TMDB's unchanged, made against the original, saves no edit, so
// that the title follows TMDB when it changes.
func TestEditsWinOverTMDBsStudiosAndTagline(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.tmdb.details("/movie/1002", nil, []string{"North Pictures"}, "Une jetée.")
	id := e.titles["Quiet Pier"]
	e.open(e.admin, "Quiet Pier")
	base, err := e.service.Original(t.Context(), e.admin, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(base.Studios, []string{"North Pictures"}) || !slices.Equal(base.Taglines, []string{"Une jetée."}) {
		t.Fatalf("the original: %q %q", base.Studios, base.Taglines)
	}
	if err := e.service.SaveOverrides(t.Context(), base, Overrides{Studios: []string{"North Pictures"}, Taglines: []string{"Une jetée."}}); err != nil {
		t.Fatal(err)
	}
	if saved := e.service.overridesNow().fields[id]; saved.Studios != nil || saved.Taglines != nil {
		t.Errorf("an unchanged save kept %q %q", saved.Studios, saved.Taglines)
	}
	if err := e.service.SaveOverrides(t.Context(), base, Overrides{Studios: []string{"Own Studio"}, Taglines: []string{"Mine."}}); err != nil {
		t.Fatal(err)
	}
	item, err := e.service.Item(t.Context(), e.admin, id)
	if err != nil || !slices.Equal(item.Studios, []string{"Own Studio"}) || !slices.Equal(item.Taglines, []string{"Mine."}) {
		t.Errorf("edited: %q %q %v", item.Studios, item.Taglines, err)
	}
}

// A title TMDB credits no one in still shows its studios and tagline.
func TestDetailsShowWithoutCredits(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.tmdb.movie("", 1011, nil, nil)
	e.tmdb.details("/movie/1011", nil, []string{"Salt Company"}, "Le sel.")
	item := e.open(e.admin, "Salt Road")
	if !slices.Equal(item.Studios, []string{"Salt Company"}) || !slices.Equal(item.Taglines, []string{"Le sel."}) ||
		!slices.Equal(cast(item.People, "Actor"), addonCast) {
		t.Errorf("Salt Road: %q %q %v", item.Studios, item.Taglines, cast(item.People, "Actor"))
	}
}

// A tagline kept in another language than the server's is hidden until
// the title is read again in the new one; its studios still show.
func TestATaglineOfAnotherLanguageIsHidden(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.tmdb.details("/movie/1002", nil, []string{"North Pictures"}, "Une jetée.")
	e.open(e.admin, "Quiet Pier")
	e.settle()
	e.setting(func(s *accounts.Settings) { s.Language = "en" })
	item, err := e.service.Item(t.Context(), e.admin, e.titles["Quiet Pier"])
	if err != nil || item.Taglines != nil || !slices.Equal(item.Studios, []string{"North Pictures"}) {
		t.Errorf("in English, before it is read again: %q %q %v", item.Taglines, item.Studios, err)
	}
	e.tmdb.details("/movie/1002", nil, []string{"North Pictures"}, "A pier.")
	if item := e.open(e.admin, "Quiet Pier"); !slices.Equal(item.Taglines, []string{"A pier."}) {
		t.Errorf("read again in English: %q", item.Taglines)
	}
}

// Credits kept before their details were are due: a page or a pass reads
// them again, and the title gets its studios. (The client answers the
// second read from the answer it keeps a day.)
func TestCreditsKeptWithoutDetailsAreDue(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.tmdb.details("/movie/1002", nil, []string{"North Pictures"}, "Une jetée.")
	e.open(e.admin, "Quiet Pier")
	e.settle()
	if _, err := e.service.db.Exec(t.Context(), `UPDATE title_credits SET networks = NULL, companies = NULL, tagline = NULL`); err != nil {
		t.Fatal(err)
	}
	e.service.forgetCredits(creditsKey{kind: "movie", provider: "imdb", id: "tt0882002"})
	kept, err := e.service.readTitleCredits(t.Context(), []creditsKey{{kind: "movie", provider: "imdb", id: "tt0882002"}})
	k := kept[creditsKey{kind: "movie", provider: "imdb", id: "tt0882002"}]
	if err != nil || k.details || !k.due(time.Now(), "fr", e.titles["Quiet Pier"]) {
		t.Fatalf("an old row: %+v %v", k, err)
	}
	if item := e.open(e.admin, "Quiet Pier"); !slices.Equal(item.Studios, []string{"North Pictures"}) {
		t.Errorf("read again: %q", item.Studios)
	}
}

// A page waits for its details as for its credits: when TMDB is late, it
// shows none, and the next open does.
func TestAPageWithoutDetailsInTimeShowsThemNextTime(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.creditsWait = 50 * time.Millisecond
	e.service.UseTitleCredits(e.queue)
	e.tmdb.details("/movie/1002", nil, []string{"North Pictures"}, "Une jetée.")
	e.tmdb.mu.Lock()
	e.tmdb.limited = 1
	e.tmdb.mu.Unlock()
	if item := e.open(e.admin, "Quiet Pier"); item.Studios != nil || item.Taglines != nil {
		t.Errorf("in time: %q %q", item.Studios, item.Taglines)
	}
	e.settle()
	if item := e.open(e.admin, "Quiet Pier"); !slices.Equal(item.Studios, []string{"North Pictures"}) {
		t.Errorf("the next open: %q", item.Studios)
	}
}

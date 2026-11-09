package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// describedSeries is a series as a metadata addon describes it, with its
// TMDB identifier as moviedb_id: of an IMDb identifier when its Stremio
// identifier is one, of none else.
func describedSeries(id, name, released string, moviedb int) string {
	imdb := ""
	if strings.HasPrefix(id, "tt") {
		imdb = fmt.Sprintf(`"imdb_id":%q,`, id)
	}
	return fmt.Sprintf(`{"id":%q,%s"type":"series","name":%q,"releaseInfo":%q,"moviedb_id":%d}`, id, imdb, name, released, moviedb)
}

// "IT" (1990), which a metadata addon describes with another show's TMDB
// identifier (113981), and "The Last of Us", which it describes with one
// TMDB does not know (502119); TMDB's are 19614 and 100088.
var (
	itMoviedb        = describedSeries("tt0099864", "IT", "1990", 113981)
	lastOfUsUnknown  = describedSeries("tt3581920", "The Last of Us", "2023-", 502119)
	lastOfUsRight    = describedSeries("tt3581920", "The Last of Us", "2023-", 100088)
	itFoundOnTMDB    = map[string]any{"tv_results": []any{map[string]any{"id": 19614}}}
	lastOfUsOnTMDB   = map[string]any{"tv_results": []any{map[string]any{"id": 100088}}}
	moviedbCheckWait = time.Minute
)

// moviedbEnv is an env whose library lists the series of a metadata addon
// that gives their TMDB identifiers as moviedb_id, in "Shows", with a fake
// TMDB that checks them in its queue of background lookups.
type moviedbEnv struct {
	env
	tmdb  *fakeCreditsTMDB
	queue *tmdb.Queue
	addon *fakeAddon
	metas []string
}

// newMoviedbEnv is a moviedbEnv of the server's TMDB key (none when
// empty), whose addon describes the series of metas, raw descriptions,
// alike in its catalog and its descriptions. TMDB answers what the test
// gives its fake before the library is first listed.
func newMoviedbEnv(t *testing.T, key string, metas ...string) moviedbEnv {
	t.Helper()
	e := moviedbEnv{env: newEnv(t), tmdb: newFakeCreditsTMDB(t)}
	client := tmdb.New(e.tmdb.URL+"/3", e.tmdb.URL+"/img/", "test", e.users.Settings)
	e.service.UseTMDB(client)
	e.service.creditsWait = moviedbCheckWait
	e.queue = tmdb.NewQueue(client, tmdb.QueueOptions{Interval: time.Millisecond, PassCheck: time.Hour})
	t.Cleanup(e.queue.Close)
	e.service.UseTitleCredits(e.queue)
	e.setting(func(s *accounts.Settings) { s.TMDBKey = key })
	e.addon = &fakeAddon{
		manifest: stremio.Manifest{ID: "described", Name: "Described", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt", "kitsu:"}}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}}},
	}
	e.describe(metas...)
	e.install(addons.Shared(), e.addon)
	return e
}

// describe has the addon describe metas, raw descriptions, from now on,
// in place of those of the same identifiers.
func (e *moviedbEnv) describe(metas ...string) {
	e.t.Helper()
	for _, meta := range metas {
		id := describedID(e.t, meta)
		e.metas = slices.DeleteFunc(e.metas, func(m string) bool { return describedID(e.t, m) == id })
		e.metas = append(e.metas, meta)
	}
	raw := map[string]string{}
	for _, meta := range e.metas {
		raw["series/"+describedID(e.t, meta)] = meta
	}
	e.addon.mu.Lock()
	defer e.addon.mu.Unlock()
	e.addon.rawCatalogs = map[string]string{"series/shows": "[" + strings.Join(e.metas, ",") + "]"}
	e.addon.rawMetas = raw
}

// describedID is the Stremio identifier of a raw description.
func describedID(t *testing.T, meta string) string {
	t.Helper()
	var described struct{ ID string }
	if err := json.Unmarshal([]byte(meta), &described); err != nil {
		t.Fatal(err)
	}
	return described.ID
}

// listed lists the library's series by name, as a user browsing it sees
// them.
func (e moviedbEnv) listed() map[string]Item {
	e.t.Helper()
	items := map[string]Item{}
	for _, item := range e.children(e.admin, "Shows", 0, 20).Items {
		items[item.Name] = item
	}
	return items
}

// item is the series of a Stremio identifier as its page shows it.
func (e moviedbEnv) item(stremioID string) Item {
	e.t.Helper()
	item, err := e.service.Item(e.t.Context(), e.admin, itemID(titleKey(KindSeries, stremioID)))
	if err != nil {
		e.t.Fatal(err)
	}
	return item
}

// settle waits until the queue's lookups ended.
func (e moviedbEnv) settle() {
	e.t.Helper()
	if err := e.queue.Settled(e.t.Context()); err != nil {
		e.t.Fatal(err)
	}
}

// stored returns the TMDB identifier, and its mark of moviedb_id, of the
// preview the items row of a series stores, empty for none.
func (e moviedbEnv) stored(stremioID string) (tmdbID, fromMoviedb string) {
	e.t.Helper()
	if err := e.service.db.QueryRow(e.t.Context(), `SELECT COALESCE(data->'meta'->>'_tmdbId', ''),
		COALESCE(data->'meta'->>'_tmdbIdFromMoviedb', '') FROM items WHERE id = $1`,
		itemID(titleKey(KindSeries, stremioID))).Scan(&tmdbID, &fromMoviedb); err != nil {
		e.t.Fatal(err)
	}
	return tmdbID, fromMoviedb
}

// verdict returns the verdict kept on the TMDB identifier of a series of
// an IMDb identifier, else of a year, written as "wrong", "wrong, replaced
// by <id>", "right", or "none" when it was not checked.
func (e moviedbEnv) verdict(tmdbID int, imdb string, year int) string {
	e.t.Helper()
	var wrong bool
	var replacedBy int
	err := e.service.db.QueryRow(e.t.Context(), `SELECT wrong, COALESCE(replaced_by, 0) FROM moviedb_id_checks
		WHERE kind = 'tv' AND tmdb_id = $1 AND imdb_id = $2 AND year = $3`, tmdbID, imdb, year).Scan(&wrong, &replacedBy)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "none"
	case err != nil:
		e.t.Fatal(err)
	case wrong && replacedBy > 0:
		return fmt.Sprintf("wrong, replaced by %d", replacedBy)
	case wrong:
		return "wrong"
	}
	return "right"
}

// checks counts the verdicts kept.
func (e moviedbEnv) checks() int {
	e.t.Helper()
	var n int
	if err := e.service.db.QueryRow(e.t.Context(), `SELECT count(*) FROM moviedb_id_checks`).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// shows checks the TMDB identifier a series shows, empty for none, on its
// page and in a new listing of the library.
func (e moviedbEnv) shows(stremioID, name, want string) {
	e.t.Helper()
	if got := e.item(stremioID).ProviderIDs["Tmdb"]; got != want {
		e.t.Errorf("%s's page shows TMDB %q, want %q", name, got, want)
	}
	if got := e.listed()[name].ProviderIDs["Tmdb"]; got != want {
		e.t.Errorf("%s listed shows TMDB %q, want %q", name, got, want)
	}
}

// A moviedb_id that names another title on TMDB, as IT's names another
// show, or none TMDB knows, as The Last of Us', is replaced by the one
// TMDB's find gives for the title's IMDb identifier, once the queue made
// the check: on the title's page, in listings and in its stored preview,
// which keeps its mark of moviedb_id. The verdict is kept, and the
// replacement known right; each check costs one request.
func TestWrongMoviedbIdentifiersAreReplacedByTMDBs(t *testing.T) {
	t.Parallel()
	e := newMoviedbEnv(t, "k3y", itMoviedb, lastOfUsUnknown)
	e.tmdb.finds["tt0099864"] = itFoundOnTMDB
	e.tmdb.finds["tt3581920"] = lastOfUsOnTMDB
	e.listed()
	e.settle()
	// Each title is found once, and no external identifiers are asked; a
	// corrected title's credits are asked by its new identifier, which
	// this test does not count.
	for _, imdb := range []string{"tt0099864", "tt3581920"} {
		if got := e.tmdb.requests("/3/find/" + imdb); len(got) != 1 {
			t.Errorf("TMDB was asked %v, want one find of %s", got, imdb)
		}
	}
	for _, asked := range e.tmdb.requests("/3/tv/") {
		if strings.Contains(asked, "/external_ids") || strings.HasPrefix(asked, "/3/tv/113981") || strings.HasPrefix(asked, "/3/tv/502119") {
			t.Errorf("TMDB was asked %s, want nothing of the wrong identifiers", asked)
		}
	}
	for _, title := range []struct {
		id, name    string
		addons, now int
	}{{"tt0099864", "IT", 113981, 19614}, {"tt3581920", "The Last of Us", 502119, 100088}} {
		now := fmt.Sprint(title.now)
		e.shows(title.id, title.name, now)
		if id, mark := e.stored(title.id); id != now || mark != "true" {
			t.Errorf("%s's preview stores TMDB %q (from moviedb_id %q), want %s (true)", title.name, id, mark, now)
		}
		if got, want := e.verdict(title.addons, title.id, 0), "wrong, replaced by "+now; got != want {
			t.Errorf("%s's moviedb_id %d: %s, want %s", title.name, title.addons, got, want)
		}
		if got := e.verdict(title.now, title.id, 0); got != "right" {
			t.Errorf("%s's TMDB identifier %d: %s, want right", title.name, title.now, got)
		}
	}
}

// A moviedb_id TMDB's find gives too is kept, after that one request, and
// known right.
func TestARightMoviedbIdentifierIsKeptAfterOneRequest(t *testing.T) {
	t.Parallel()
	e := newMoviedbEnv(t, "k3y", lastOfUsRight)
	e.tmdb.finds["tt3581920"] = lastOfUsOnTMDB
	e.listed()
	e.settle()
	if got := e.tmdb.requests("/3/"); len(got) != 1 || !strings.HasPrefix(got[0], "/3/find/tt3581920?") {
		t.Errorf("TMDB was asked %v, want one find", got)
	}
	if got := e.verdict(100088, "tt3581920", 0); got != "right" {
		t.Errorf("verdict: %s, want right", got)
	}
	e.shows("tt3581920", "The Last of Us", "100088")
	if id, mark := e.stored("tt3581920"); id != "100088" || mark != "true" {
		t.Errorf("the preview stores TMDB %q (from moviedb_id %q), want 100088 (true)", id, mark)
	}
}

// When TMDB's find has no series of the title's IMDb identifier, the
// moviedb_id is judged by its external identifiers: TMDB not knowing it,
// or giving it another IMDb identifier, drops it, from the page, listings
// and the stored preview with its mark; giving it none keeps it.
func TestMoviedbIdentifiersFindHasNoneOfAreJudgedByTheirIMDbIdentifier(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		external any // TMDB's external identifiers of 502119, nil for a 404
		verdict  string
		shown    string
	}{
		{"unknown to TMDB", nil, "wrong", ""},
		{"without an IMDb identifier", map[string]any{"id": 502119, "imdb_id": nil}, "right", "502119"},
		{"of another IMDb identifier", map[string]any{"id": 502119, "imdb_id": "tt0000001"}, "wrong", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			e := newMoviedbEnv(t, "k3y", lastOfUsUnknown)
			if test.external != nil {
				e.tmdb.answers["/tv/502119/external_ids"] = test.external
			}
			e.listed()
			e.settle()
			if got := e.verdict(502119, "tt3581920", 0); got != test.verdict {
				t.Errorf("verdict: %s, want %s", got, test.verdict)
			}
			e.shows("tt3581920", "The Last of Us", test.shown)
			wantMark := "true"
			if test.shown == "" {
				wantMark = ""
			}
			if id, mark := e.stored("tt3581920"); id != test.shown || mark != wantMark {
				t.Errorf("the preview stores TMDB %q (from moviedb_id %q), want %q (%q)", id, mark, test.shown, wantMark)
			}
			if find, external := e.tmdb.requests("/3/find/"), e.tmdb.requests("/3/tv/502119/external_ids"); len(find) != 1 || len(external) != 1 {
				t.Errorf("TMDB was asked find %v and external identifiers %v, want each once", find, external)
			}
		})
	}
}

// The moviedb_id of a title without an IMDb identifier is judged by the
// year TMDB gives its title, from one request of its details: a year
// apart is kept; three apart, or TMDB not knowing it, drops it.
func TestMoviedbIdentifiersOfTitlesWithoutIMDbAreJudgedByTheirYear(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		aired   string // TMDB's first air date of 4100, empty for a 404
		verdict string
		shown   string
	}{
		{"a year apart", "1991-04-01", "right", "4100"},
		{"three years apart", "1993-04-01", "wrong", ""},
		{"unknown to TMDB", "", "wrong", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			e := newMoviedbEnv(t, "k3y", describedSeries("kitsu:4000", "Night Tide", "1990", 4100))
			if test.aired != "" {
				e.tmdb.answers["/tv/4100"] = map[string]any{"id": 4100, "first_air_date": test.aired}
			}
			e.listed()
			e.settle()
			if got := e.verdict(4100, "", 1990); got != test.verdict {
				t.Errorf("verdict: %s, want %s", got, test.verdict)
			}
			e.shows("kitsu:4000", "Night Tide", test.shown)
			if got := e.tmdb.requests("/3/"); len(got) != 1 || !strings.HasPrefix(got[0], "/3/tv/4100?") {
				t.Errorf("TMDB was asked %v, want the details of 4100 once", got)
			}
		})
	}
}

// Another moviedb_id an addon gives later for a title is checked on its
// own, as the former's verdict does not judge it; one equal to the
// identifier TMDB replaced the former by is known right, and costs no
// request. A verdict that finds one wrong forgets the title's
// description, which the next page described again shows.
func TestAnotherMoviedbIdentifierIsCheckedAnew(t *testing.T) {
	t.Parallel()
	e := newMoviedbEnv(t, "k3y", itMoviedb)
	e.tmdb.finds["tt0099864"] = itFoundOnTMDB
	e.listed()
	e.settle()
	if got := e.verdict(113981, "tt0099864", 0); got != "wrong, replaced by 19614" {
		t.Errorf("the first moviedb_id 113981: %s, want wrong, replaced by 19614", got)
	}

	e.describe(describedSeries("tt0099864", "IT", "1990", 1000))
	if got := e.item("tt0099864").ProviderIDs["Tmdb"]; got != "1000" {
		t.Errorf("IT's page shows TMDB %q before 1000 is checked, want 1000", got)
	}
	e.settle()
	if got := e.verdict(1000, "tt0099864", 0); got != "wrong, replaced by 19614" {
		t.Errorf("the later moviedb_id 1000: %s, want wrong, replaced by 19614", got)
	}

	asked := len(e.tmdb.requests("/3/"))
	e.describe(describedSeries("tt0099864", "IT", "1990", 19614))
	e.shows("tt0099864", "IT", "19614")
	e.settle()
	if got := e.tmdb.requests("/3/"); len(got) != asked {
		t.Errorf("TMDB was asked %v, want nothing more after the first %d", got[asked:], asked)
	}
	if got := e.checks(); got != 3 {
		t.Errorf("%d verdicts kept, want 3 (113981, 1000 and 19614)", got)
	}
	described := 0
	e.addon.mu.Lock()
	defer e.addon.mu.Unlock()
	for _, asked := range e.addon.requests {
		if asked == "meta/series/tt0099864" {
			described++
		}
	}
	if described != 2 {
		t.Errorf("the addon described IT %d times, want twice (with 1000, then 19614)", described)
	}
}

// Without a key, no moviedb_id is checked: nothing is asked of TMDB, even
// when a title's page is opened, no verdict is kept, and titles show the
// addon's identifier.
func TestMoviedbIdentifiersAreNotCheckedWithoutAKey(t *testing.T) {
	t.Parallel()
	e := newMoviedbEnv(t, "", itMoviedb)
	e.tmdb.finds["tt0099864"] = itFoundOnTMDB
	e.listed()
	e.service.AwaitTitleCredits(e.t.Context(), e.item("tt0099864"))
	e.settle()
	if got := e.tmdb.requests("/"); len(got) != 0 {
		t.Errorf("TMDB was asked %v, want nothing", got)
	}
	if got := e.checks(); got != 0 {
		t.Errorf("%d verdicts kept, want none", got)
	}
	e.shows("tt0099864", "IT", "113981")
	if id, mark := e.stored("tt0099864"); id != "113981" || mark != "true" {
		t.Errorf("the preview stores TMDB %q (from moviedb_id %q), want 113981 (true)", id, mark)
	}
}

// A title whose TMDB identifier an administrator set is not checked, and
// shows the identifier set, while the others are checked.
func TestAnEditedTMDBIdentifierIsNotChecked(t *testing.T) {
	t.Parallel()
	e := newMoviedbEnv(t, "k3y", itMoviedb, lastOfUsUnknown)
	e.tmdb.finds["tt0099864"] = itFoundOnTMDB
	e.tmdb.finds["tt3581920"] = lastOfUsOnTMDB
	editIDs(t, e.service, KindSeries, described(t, itMoviedb)[0], new("1234"), nil)
	e.listed()
	e.service.AwaitTitleCredits(e.t.Context(), e.item("tt0099864"))
	e.settle()
	for _, asked := range e.tmdb.requests("/") {
		if strings.Contains(asked, "tt0099864") || strings.Contains(asked, "113981") {
			t.Errorf("TMDB was asked %s about the edited title", asked)
		}
	}
	if got := e.verdict(113981, "tt0099864", 0); got != "none" {
		t.Errorf("IT's moviedb_id: %s, want none", got)
	}
	e.shows("tt0099864", "IT", "1234")
	e.shows("tt3581920", "The Last of Us", "100088")
}

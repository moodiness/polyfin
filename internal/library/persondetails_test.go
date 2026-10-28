package library

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// fakePeopleTMDB answers TMDB's person search, details and credits from
// what it is given, counting the requests by path and query, and fails
// the searches of the names in failing while they are there.
type fakePeopleTMDB struct {
	*httptest.Server
	mu       sync.Mutex
	asked    map[string]int
	searches map[string][]map[string]any
	people   map[string]map[string]any // "id" or "id/language" → details
	credits  map[string][]map[string]any
	failing  map[string]bool
}

func newFakePeopleTMDB(t *testing.T) *fakePeopleTMDB {
	t.Helper()
	f := &fakePeopleTMDB{asked: map[string]int{}, searches: map[string][]map[string]any{}, people: map[string]map[string]any{},
		credits: map[string][]map[string]any{}, failing: map[string]bool{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		query := r.URL.Query()
		f.asked[r.URL.Path+"?"+query.Get("query")+query.Get("language")]++
		if query.Get("api_key") != "k3y" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		rest, _ := strings.CutPrefix(r.URL.Path, "/3/")
		switch {
		case rest == "search/person":
			if f.failing[query.Get("query")] {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searches[query.Get("query")]})
		case strings.HasSuffix(rest, "/combined_credits"):
			id := strings.TrimSuffix(strings.TrimPrefix(rest, "person/"), "/combined_credits")
			_ = json.NewEncoder(w).Encode(map[string]any{"cast": f.credits[id]})
		case strings.HasPrefix(rest, "person/"):
			id := strings.TrimPrefix(rest, "person/")
			details, ok := f.people[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			answer := map[string]any{"id": details["id"], "name": details["name"], "birthday": details["birthday"],
				"deathday": details["deathday"], "place_of_birth": details["place_of_birth"], "profile_path": details["profile_path"],
				"imdb_id": details["imdb_id"], "biography": f.people[id+"/"+query.Get("language")]["biography"]}
			_ = json.NewEncoder(w).Encode(answer)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// requests counts the requests whose path and query start with prefix.
func (f *fakePeopleTMDB) requests(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for key, count := range f.asked {
		if strings.HasPrefix(key, prefix) {
			n += count
		}
	}
	return n
}

// peopleEnv is an env whose library looks people up on a fake TMDB, with
// the movie "The Heist" (2020) crediting the people of cast and directors.
type peopleEnv struct {
	languageEnv
	tmdb  *fakePeopleTMDB
	heist accounts.ID
	clock *testClock
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func newPeopleEnv(t *testing.T, key string, cast []stremio.CastMember, directors ...string) peopleEnv {
	t.Helper()
	e := peopleEnv{languageEnv: languageEnv{env: newEnv(t)}, tmdb: newFakePeopleTMDB(t), clock: &testClock{now: time.Now()}}
	e.service.SetClock(e.clock.Now)
	e.service.UseTMDB(tmdb.New(e.tmdb.URL+"/3", e.tmdb.URL+"/img/", "test", e.users.Settings))
	e.setting(func(s *accounts.Settings) { s.TMDBKey = key })
	movie := stremio.Meta{ID: "tt1", Type: "movie", Name: "The Heist", Released: "2020-03-04T00:00:00.000Z", Genres: stremio.Names{"Drama"},
		Director: directors, Extras: &stremio.Extras{Certification: "PG", Cast: cast}}
	row := movie
	row.Extras, row.Director = nil, nil
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": {row}},
		metas:    map[string]stremio.Meta{"movie/tt1": movie},
	})
	e.heist = e.titleIDs("Top")["The Heist"]
	return e
}

// cast opens The Heist's page and returns its people, with the photos its
// cast shows, once the lookups it started ended.
func (e peopleEnv) cast(user accounts.User) map[string]Person {
	e.t.Helper()
	item, err := e.service.Item(e.t.Context(), user, e.heist)
	if err != nil {
		e.t.Fatal(err)
	}
	e.service.CastPhotos(e.t.Context(), user, item.People)
	e.service.personLookups.wg.Wait()
	people := map[string]Person{}
	for _, p := range e.service.CastPhotos(e.t.Context(), user, item.People) {
		people[p.Name] = p
	}
	return people
}

// person opens a person's page, as the Jellyfin API does: looking them up,
// then describing them.
func (e peopleEnv) person(user accounts.User, name string) Item {
	e.t.Helper()
	e.service.LookUpPerson(e.t.Context(), user, PersonID(name))
	e.service.personLookups.wg.Wait()
	item, err := e.service.Item(e.t.Context(), user, PersonID(name))
	if err != nil {
		e.t.Fatal(err)
	}
	return item
}

func (f *fakePeopleTMDB) addPerson(id, name, profile, biography string, extra map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	details := map[string]any{"id": json.Number(id), "name": name, "profile_path": profile}
	for k, v := range extra {
		details[k] = v
	}
	f.people[id] = details
	f.people[id+"/en"] = map[string]any{"biography": biography}
}

// A person TMDB finds by name gets TMDB's photo in the cast and on their
// page, their biography in the user's metadata language, else TMDB's in
// English, their birth and death dates and their place of birth, as
// Jellyfin's TmdbPersonProvider gives them; a person the addon names by
// TMDB identifier is that person, without a search.
func TestPeopleAreFilledFromTMDB(t *testing.T) {
	t.Parallel()
	e := newPeopleEnv(t, "k3y", []stremio.CastMember{{Name: "Ana Actor", Character: "Ana"}, {Name: "Ben Actor", TmdbID: "21"}}, "Eve Director")
	e.tmdb.searches["Ana Actor"] = []map[string]any{{"id": 11, "name": "Ana Actor", "popularity": 5, "profile_path": "/ana.jpg"},
		{"id": 12, "name": "Ana Actorson", "popularity": 50}}
	e.tmdb.addPerson("11", "Ana Actor", "/ana.jpg", "Ana was born.", map[string]any{"birthday": "1970-05-06",
		"deathday": "2020-01-02", "place_of_birth": "Lyon, France", "imdb_id": "nm11"})
	e.tmdb.people["11/fr"] = map[string]any{"biography": "Ana est née."}
	e.tmdb.searches["Ben Actor"] = []map[string]any{{"id": 99, "name": "Ben Actor", "popularity": 80, "profile_path": "/other.jpg"}}
	e.tmdb.addPerson("21", "Ben Actor", "/ben.jpg", "Ben, by his identifier.", nil)
	e.tmdb.searches["Eve Director"] = []map[string]any{{"id": 31, "name": "Eve Director", "popularity": 1, "profile_path": "/eve.jpg"}}
	e.tmdb.addPerson("31", "Eve Director", "/eve.jpg", "", nil)

	cast := e.cast(e.admin)
	for name, photo := range map[string]string{"Ana Actor": "/img/original/ana.jpg", "Ben Actor": "/img/original/ben.jpg",
		"Eve Director": "/img/original/eve.jpg"} {
		if got := cast[name].Image; got != e.tmdb.URL+photo {
			t.Errorf("%s's photo in the cast: %q", name, got)
		}
	}
	if e.tmdb.requests("/3/search/person?Ben Actor") != 0 {
		t.Error("Ben Actor, named by his TMDB identifier, was searched by name")
	}

	ana := e.person(e.admin, "Ana Actor")
	if ana.Overview != "Ana was born." || ana.PremiereDate == nil || ana.PremiereDate.Format(time.DateOnly) != "1970-05-06" ||
		ana.EndDate == nil || ana.EndDate.Format(time.DateOnly) != "2020-01-02" || len(ana.ProductionLocations) != 1 ||
		ana.ProductionLocations[0] != "Lyon, France" || ana.ProviderIDs["Tmdb"] != "11" || ana.ProviderIDs["Imdb"] != "nm11" ||
		ana.Images.Primary != e.tmdb.URL+"/img/original/ana.jpg" {
		t.Errorf("Ana Actor's page: %+v", ana)
	}
	if ben := e.person(e.admin, "Ben Actor"); ben.Overview != "Ben, by his identifier." || ben.ProviderIDs["Tmdb"] != "21" {
		t.Errorf("Ben Actor's page: %+v", ben)
	}
	// A user of another metadata language gets the biography in it, or
	// TMDB's English one when it has none; each is asked once.
	fr := "fr"
	member, err := e.users.UpdateUser(t.Context(), e.member.ID, accounts.UserChanges{MetadataLanguage: &fr}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.person(member, "Ana Actor").Overview; got != "Ana est née." {
		t.Errorf("Ana Actor's biography in French: %q", got)
	}
	if got := e.person(member, "Ben Actor").Overview; got != "Ben, by his identifier." {
		t.Errorf("Ben Actor's biography without a French one: %q", got)
	}
	e.person(member, "Ana Actor")
	if got := e.tmdb.requests("/3/person/11?fr"); got != 1 {
		t.Errorf("Ana Actor's French biography asked %d times", got)
	}
	if got := e.tmdb.requests("/3/search/person?Ana Actor"); got != 1 {
		t.Errorf("Ana Actor searched %d times", got)
	}
}

// Of several people of the same name, the one credited in the title the
// person was met in is theirs, from the titles each is known for, else
// from their credits; when no title tells them apart, the person gets no
// one's photo or biography. A photo of TMDB's the addon gives names its
// person.
func TestACommonNameIsNotMisattributed(t *testing.T) {
	t.Parallel()
	e := newPeopleEnv(t, "k3y", []stremio.CastMember{{Name: "Ana Actor"}, {Name: "John Smith"}, {Name: "Kim Lee"},
		{Name: "Dan Actor", Photo: "https://image.tmdb.org/t/p/w185/dan2.jpg"}})
	heist := map[string]any{"id": 700, "media_type": "movie", "title": "The Heist", "release_date": "2020-03-01"}
	other := map[string]any{"id": 800, "media_type": "movie", "title": "Another Movie", "release_date": "2001-01-01"}
	oldHeist := map[string]any{"id": 900, "media_type": "movie", "title": "The Heist", "release_date": "1965-01-01"}
	e.tmdb.searches["Ana Actor"] = []map[string]any{
		{"id": 11, "name": "Ana Actor", "popularity": 90, "profile_path": "/famous.jpg", "known_for": []any{other}},
		{"id": 12, "name": "Ana Actor", "popularity": 2, "profile_path": "/ana.jpg", "known_for": []any{heist}}}
	e.tmdb.addPerson("11", "Ana Actor", "/famous.jpg", "The famous one.", nil)
	e.tmdb.addPerson("12", "Ana Actor", "/ana.jpg", "The one in The Heist.", nil)
	e.tmdb.searches["Kim Lee"] = []map[string]any{
		{"id": 21, "name": "Kim Lee", "popularity": 90, "profile_path": "/kim1.jpg", "known_for": []any{other}},
		{"id": 22, "name": "Kim Lee", "popularity": 5, "profile_path": "/kim2.jpg", "known_for": []any{other}}}
	e.tmdb.credits["21"] = []map[string]any{other, oldHeist}
	e.tmdb.credits["22"] = []map[string]any{other, heist}
	e.tmdb.addPerson("21", "Kim Lee", "/kim1.jpg", "The famous Kim.", nil)
	e.tmdb.addPerson("22", "Kim Lee", "/kim2.jpg", "Kim of The Heist.", nil)
	e.tmdb.searches["John Smith"] = []map[string]any{
		{"id": 31, "name": "John Smith", "popularity": 90, "profile_path": "/john1.jpg", "known_for": []any{other}},
		{"id": 32, "name": "John Smith", "popularity": 5, "profile_path": "/john2.jpg"}}
	e.tmdb.addPerson("31", "John Smith", "/john1.jpg", "The famous John.", nil)
	e.tmdb.addPerson("32", "John Smith", "/john2.jpg", "Another John.", nil)
	e.tmdb.searches["Dan Actor"] = []map[string]any{
		{"id": 41, "name": "Dan Actor", "popularity": 90, "profile_path": "/dan1.jpg"},
		{"id": 42, "name": "Dan Actor", "popularity": 5, "profile_path": "/dan2.jpg"}}
	e.tmdb.addPerson("41", "Dan Actor", "/dan1.jpg", "The famous Dan.", nil)
	e.tmdb.addPerson("42", "Dan Actor", "/dan2.jpg", "Dan of The Heist.", nil)

	cast := e.cast(e.admin)
	for name, want := range map[string]string{"Ana Actor": e.tmdb.URL + "/img/original/ana.jpg",
		"Kim Lee": e.tmdb.URL + "/img/original/kim2.jpg", "John Smith": ""} {
		if got := cast[name].Image; got != want {
			t.Errorf("%s's photo in the cast: %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{"Ana Actor": "The one in The Heist.", "Kim Lee": "Kim of The Heist.",
		"John Smith": "", "Dan Actor": "Dan of The Heist."} {
		if got := e.person(e.admin, name); got.Overview != want {
			t.Errorf("%s's biography: %q, want %q", name, got.Overview, want)
		}
	}
	if got := e.person(e.admin, "Dan Actor").Images.Primary; got != "https://image.tmdb.org/t/p/w185/dan2.jpg" {
		t.Errorf("Dan Actor's photo: %q, want the addon's", got)
	}
	if got := e.tmdb.requests("/3/person/41") + e.tmdb.requests("/3/person/42/combined_credits"); got != 0 {
		t.Errorf("Dan Actor, named by his photo, cost %d more requests", got)
	}
	// Someone TMDB could not tell is not asked about again at once.
	before := e.tmdb.requests("/3/search/person?John Smith")
	e.cast(e.admin)
	e.person(e.admin, "John Smith")
	if got := e.tmdb.requests("/3/search/person?John Smith"); got != before {
		t.Errorf("John Smith searched again: %d, then %d", before, got)
	}
}

// The addon's photo wins over TMDB's, which still gives the biography;
// without the server's TMDB key, TMDB is never asked; a lookup that
// failed is tried again later, not at once.
func TestTheAddonsPhotoAndTheKeyComeFirst(t *testing.T) {
	t.Parallel()
	e := newPeopleEnv(t, "", []stremio.CastMember{{Name: "Cleo Actor", Photo: "https://images.example/cleo.jpg"}, {Name: "Finn Actor"}})
	e.tmdb.searches["Cleo Actor"] = []map[string]any{{"id": 51, "name": "Cleo Actor", "popularity": 3, "profile_path": "/cleo.jpg"}}
	e.tmdb.addPerson("51", "Cleo Actor", "/cleo.jpg", "Cleo's life.", nil)
	e.tmdb.searches["Finn Actor"] = []map[string]any{{"id": 61, "name": "Finn Actor", "popularity": 3, "profile_path": "/finn.jpg"}}
	e.tmdb.addPerson("61", "Finn Actor", "/finn.jpg", "Finn's life.", nil)
	e.tmdb.failing["Finn Actor"] = true

	if cast := e.cast(e.admin); cast["Finn Actor"].Image != "" {
		t.Errorf("Finn Actor's photo without a key: %q", cast["Finn Actor"].Image)
	}
	if got := e.person(e.admin, "Cleo Actor"); got.Overview != "" || got.Images.Primary != "https://images.example/cleo.jpg" {
		t.Errorf("Cleo Actor's page without a key: %+v", got)
	}
	if got := e.tmdb.requests("/"); got != 0 {
		t.Fatalf("TMDB asked %d times without a key", got)
	}

	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	if got := e.person(e.admin, "Cleo Actor"); got.Overview != "Cleo's life." || got.Images.Primary != "https://images.example/cleo.jpg" {
		t.Errorf("Cleo Actor's page: %q, photo %q", got.Overview, got.Images.Primary)
	}
	if cast := e.cast(e.admin); cast["Finn Actor"].Image != "" {
		t.Errorf("Finn Actor's photo while TMDB fails: %q", cast["Finn Actor"].Image)
	}
	failed := e.tmdb.requests("/3/search/person?Finn Actor")
	e.mu(func() { e.tmdb.failing["Finn Actor"] = false })
	e.cast(e.admin)
	if got := e.tmdb.requests("/3/search/person?Finn Actor"); got != failed {
		t.Errorf("Finn Actor asked again at once after a failure: %d, then %d", failed, got)
	}
	e.clock.advance(personRetry + time.Second)
	if cast := e.cast(e.admin); cast["Finn Actor"].Image != e.tmdb.URL+"/img/original/finn.jpg" {
		t.Errorf("Finn Actor's photo once tried again: %q", cast["Finn Actor"].Image)
	}
}

// mu runs change under the fake TMDB's lock.
func (e peopleEnv) mu(change func()) {
	e.tmdb.mu.Lock()
	defer e.tmdb.mu.Unlock()
	change()
}

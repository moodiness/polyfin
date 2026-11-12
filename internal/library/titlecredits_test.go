package library

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// fakeCreditsTMDB answers TMDB's find, the credits of movies and series
// and people's combined credits from what it is given, records each
// request's path and query, refuses the first limited requests for their
// rate (Retry-After: 1) and fails the paths of failing.
type fakeCreditsTMDB struct {
	*httptest.Server
	mu       sync.Mutex
	asked    []string
	finds    map[string]map[string]any
	answers  map[string]any
	limited  int
	failing  map[string]bool
	refusals []time.Time
}

func newFakeCreditsTMDB(t *testing.T) *fakeCreditsTMDB {
	t.Helper()
	f := &fakeCreditsTMDB{finds: map[string]map[string]any{}, answers: map[string]any{}, failing: map[string]bool{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		query := r.URL.Query()
		query.Del("api_key")
		f.asked = append(f.asked, r.URL.Path+"?"+query.Encode())
		path, _ := strings.CutPrefix(r.URL.Path, "/3")
		switch {
		case f.limited > 0:
			f.limited--
			f.refusals = append(f.refusals, time.Now())
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		case f.failing[path]:
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasPrefix(path, "/find/"):
			found := map[string]any{"movie_results": []any{}, "tv_results": []any{}}
			for k, v := range f.finds[strings.TrimPrefix(path, "/find/")] {
				found[k] = v
			}
			_ = json.NewEncoder(w).Encode(found)
		default:
			// A page of a list may have an answer of its own.
			answer, ok := f.answers[path+"?page="+query.Get("page")]
			if !ok {
				answer, ok = f.answers[path]
			}
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(answer)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// requests returns the requests whose path starts with prefix.
func (f *fakeCreditsTMDB) requests(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(f.asked), func(asked string) bool { return !strings.HasPrefix(asked, prefix) })
}

// movie makes TMDB know a movie of an IMDb identifier, with cast, in
// billing order, and crew.
func (f *fakeCreditsTMDB) movie(imdb string, id int, cast, crew []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if imdb != "" {
		f.finds[imdb] = map[string]any{"movie_results": []any{map[string]any{"id": id}}}
	}
	// TMDB lists the cast in no particular order: the billing is order.
	shuffled := slices.Clone(cast)
	slices.Reverse(shuffled)
	f.answers["/movie/"+strconv.Itoa(id)] = map[string]any{"id": id, "credits": map[string]any{"cast": shuffled, "crew": crew}}
}

// billed is a cast of actors of TMDB identifiers ids, each named after
// names[id], else "Billed Extra <id>", with a photo but those of bare.
func billed(ids []int, names map[int]string, bare ...int) []map[string]any {
	var cast []map[string]any
	for order, id := range ids {
		name := names[id]
		if name == "" {
			name = fmt.Sprintf("Billed Extra %d", id)
		}
		member := map[string]any{"id": id, "name": name, "character": fmt.Sprintf("Character %d", order), "order": order}
		if !slices.Contains(bare, id) {
			member["profile_path"] = fmt.Sprintf("/p%d.jpg", id)
		}
		cast = append(cast, member)
	}
	return cast
}

// creditsEnv is an env whose library reads titles' credits on a fake TMDB,
// with a library of movies, "Harbor Lights" (tt0882001), "Quiet Pier"
// (tt0882002), "Salt Road" (tmdb:1011), "Kelp Bay" (tt0882012), "Night
// Shift" (tt0882013, rated R) and "Hidden Cove" (tmdb:1014), and one of
// series, "Tidewater" (tt0882031), each described with the people of an
// addon that names its cast and crew.
type creditsEnv struct {
	env
	tmdb    *fakeCreditsTMDB
	queue   *tmdb.Queue
	titles  map[string]accounts.ID
	addon   *fakeAddon
	persons map[string]int
}

// The addon's people of every title.
var addonCast = []string{"Ana Actor", "Ben Actor", "Cleo Actor", "Dan Actor"}

func newCreditsEnv(t *testing.T, key string, passCheck time.Duration) creditsEnv {
	t.Helper()
	e := creditsEnv{env: newEnv(t), tmdb: newFakeCreditsTMDB(t)}
	client := tmdb.New(e.tmdb.URL+"/3", e.tmdb.URL+"/img/", "test", e.users.Settings)
	e.service.UseTMDB(client)
	// A title's page waits for its credits as long as the lookups take on
	// a loaded machine: no test here reads them late.
	e.service.creditsWait = time.Minute
	e.queue = tmdb.NewQueue(client, tmdb.QueueOptions{Interval: time.Millisecond, PassCheck: passCheck})
	t.Cleanup(e.queue.Close)
	e.setting(func(s *accounts.Settings) { s.TMDBKey, s.Language = key, "fr" })
	type title struct{ id, kind, name, rating string }
	titles := []title{{"tt0882001", "movie", "Harbor Lights", "PG"}, {"tt0882002", "movie", "Quiet Pier", "PG"},
		{"tmdb:1011", "movie", "Salt Road", "PG"}, {"tt0882012", "movie", "Kelp Bay", "PG"}, {"tt0882013", "movie", "Night Shift", "R"},
		{"tmdb:1014", "movie", "Hidden Cove", "PG"}, {"tt0882031", "series", "Tidewater", "PG"}}
	e.addon = &fakeAddon{
		manifest: stremio.Manifest{ID: "names", Name: "Names", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt", "tmdb:"}}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "series", ID: "shows", Name: "Shows"}}},
		catalogs: map[string][]stremio.Meta{}, metas: map[string]stremio.Meta{},
	}
	for _, t := range titles {
		row := stremio.Meta{ID: t.id, Type: t.kind, Name: t.name, ReleaseInfo: "2011", Genres: stremio.Names{"Drama"},
			Extras: &stremio.Extras{Certification: t.rating}}
		full := row
		full.Cast, full.Director, full.Writer = addonCast, []string{"Eve Director"}, []string{"Will Writer"}
		if t.kind == "series" {
			full.Videos = []stremio.Video{{ID: t.id + ":1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2018-01-01T00:00:00Z"}}
		}
		catalog := "movie/top"
		if t.kind == "series" {
			catalog = "series/shows"
		}
		e.addon.catalogs[catalog] = append(e.addon.catalogs[catalog], row)
		e.addon.metas[t.kind+"/"+t.id] = full
	}
	e.install(addons.Shared(), e.addon)
	e.titles = map[string]accounts.ID{}
	for _, library := range []string{"Top", "Shows"} {
		for _, item := range e.children(e.admin, library, 0, 20).Items {
			e.titles[item.Name] = item.ID
		}
	}
	// TMDB's people: the addon's four actors, a second Ben Actor, the
	// director and writer, and extras the addon does not name.
	e.persons = map[string]int{"Ana Actor": 6000, "Ben Actor": 6001, "Cleo Actor": 6002, "Dan Actor": 6003, "Eve Director": 6100, "Will Writer": 6101}
	names := map[int]string{}
	for name, id := range e.persons {
		names[id] = name
	}
	names[6900] = "Ben Actor"
	harbor := []int{6000, 6001, 6002, 6003}
	for i := 1; i <= 21; i++ {
		harbor = append(harbor, 8000+i)
	}
	crew := []map[string]any{{"id": 6100, "name": "Eve Director", "job": "Director", "department": "Directing", "profile_path": "/p6100.jpg"},
		{"id": 6101, "name": "Will Writer", "job": "Screenplay", "department": "Writing"},
		{"id": 6102, "name": "Ed Editor", "job": "Editor", "department": "Editing"}}
	e.tmdb.movie("tt0882001", 1001, billed(harbor, names, 6003), crew)
	e.tmdb.movie("tt0882002", 1002, billed([]int{6000, 6900}, names), nil)
	e.tmdb.movie("", 1011, billed([]int{6000}, names), nil)
	e.tmdb.movie("tt0882012", 1012, billed([]int{6000}, names), nil)
	e.tmdb.movie("tt0882013", 1013, billed([]int{6000}, names), nil)
	e.tmdb.movie("", 1014, billed([]int{6000}, names), nil)
	aggregate := billed([]int{6002, 6000}, names)
	for _, member := range aggregate {
		member["roles"] = []any{map[string]any{"character": member["character"], "episode_count": 2}}
		delete(member, "character")
	}
	aggregate[0]["roles"] = append(aggregate[0]["roles"].([]any), map[string]any{"character": "Old Cleo", "episode_count": 9})
	e.tmdb.finds["tt0882031"] = map[string]any{"tv_results": []any{map[string]any{"id": 2031}}}
	e.tmdb.answers["/tv/2031"] = map[string]any{"id": 2031, "aggregate_credits": map[string]any{"cast": aggregate},
		"credits": map[string]any{"crew": crew[:1]}}
	return e
}

// open opens a title's page as the Jellyfin API does: waiting a moment
// for its credits, then describing it again when they came.
func (e creditsEnv) open(user accounts.User, name string) Item {
	e.t.Helper()
	item, err := e.service.Item(e.t.Context(), user, e.titles[name])
	if err != nil {
		e.t.Fatal(err)
	}
	if e.service.AwaitTitleCredits(e.t.Context(), item) {
		if item, err = e.service.Item(e.t.Context(), user, item.ID); err != nil {
			e.t.Fatal(err)
		}
	}
	return item
}

// settle waits until the credits' lookups ended.
func (e creditsEnv) settle() {
	e.t.Helper()
	if err := e.queue.Settled(e.t.Context()); err != nil {
		e.t.Fatal(err)
	}
}

func cast(people []Person, kind string) []string {
	var named []string
	for _, p := range people {
		if p.Type == kind {
			named = append(named, p.Name)
		}
	}
	return named
}

// titlesOf lists the names of the titles of kind a person's page lists.
func (e creditsEnv) titlesOf(user accounts.User, person accounts.ID, kind Kind) []string {
	e.t.Helper()
	page, err := e.service.PersonTitles(e.t.Context(), user, person, []Kind{kind}, 20)
	if err != nil {
		e.t.Fatalf("the titles of %s: %v", person, err)
	}
	var named []string
	for _, item := range page.Items {
		named = append(named, item.Name)
	}
	slices.Sort(named)
	return named
}

// While a key is saved, a movie's people are TMDB's credits: its first 15
// actors in billing order, each with TMDB's photo, else the addon's, and
// its directors and writers, the rest of its crew left out; a series',
// its aggregate cast, one credit per role. Each person is kept by their
// TMDB identifier and lists the title. Roles are asked in the server's
// metadata language.
func TestTitlesTakeTheirPeopleFromTMDB(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.addon.metas["movie/tt0882001"] = func() stremio.Meta {
		m := e.addon.metas["movie/tt0882001"]
		m.Cast = nil
		m.Extras = &stremio.Extras{Certification: "PG", Cast: []stremio.CastMember{{Name: "Ana Actor"}, {Name: "Ben Actor"},
			{Name: "Cleo Actor"}, {Name: "Dan Actor", Photo: "https://addon.example/dan.jpg"}}}
		return m
	}()

	harbor := e.open(e.admin, "Harbor Lights")
	want := []string{"Ana Actor", "Ben Actor", "Cleo Actor", "Dan Actor"}
	for i := 8001; i <= 8011; i++ {
		want = append(want, fmt.Sprintf("Billed Extra %d", i))
	}
	if got := cast(harbor.People, "Actor"); !slices.Equal(got, want) {
		t.Errorf("Harbor Lights' actors: %v", got)
	}
	if got := append(cast(harbor.People, "Director"), cast(harbor.People, "Writer")...); !slices.Equal(got, []string{"Eve Director", "Will Writer"}) ||
		len(harbor.People) != 17 {
		t.Errorf("Harbor Lights' crew: %v of %d people", got, len(harbor.People))
	}
	for _, p := range harbor.People {
		photo := e.tmdb.URL + "/img/original/p" + p.TMDB + ".jpg"
		switch p.Name {
		case "Dan Actor":
			photo = "https://addon.example/dan.jpg"
		case "Will Writer":
			photo = ""
		}
		if p.Image != photo || p.TMDB == "" || p.ID != itemID(personTMDBKey(p.TMDB)) {
			t.Errorf("%s: %+v, want the photo %q", p.Name, p, photo)
		}
	}
	if got := harbor.People[0].Role; got != "Character 0" {
		t.Errorf("Ana Actor's role: %q", got)
	}
	if asked := e.tmdb.requests("/3/movie/1001?"); len(asked) != 1 || !strings.Contains(asked[0], "language=fr") {
		t.Errorf("TMDB was asked %v", asked)
	}
	extra := itemID(personTMDBKey("8011"))
	if got := e.titlesOf(e.admin, extra, KindMovie); !slices.Equal(got, []string{"Harbor Lights"}) {
		t.Errorf("Billed Extra 8011's movies: %v", got)
	}

	tidewater := e.open(e.admin, "Tidewater")
	var roles []string
	for _, p := range tidewater.People {
		roles = append(roles, p.Name+"/"+p.Role)
	}
	if !slices.Equal(roles, []string{"Cleo Actor/Old Cleo", "Cleo Actor/Character 0", "Ana Actor/Character 1", "Eve Director/Director"}) {
		t.Errorf("Tidewater's people: %v", roles)
	}
	// Opened again, the title asks TMDB nothing.
	e.open(e.admin, "Harbor Lights")
	e.settle()
	if asked := e.tmdb.requests("/3/movie/1001?"); len(asked) != 1 {
		t.Errorf("TMDB was asked again: %v", asked)
	}
}

// The addon's people stay without a key, for a title TMDB does not know,
// and when TMDB fails, and its photos fill those TMDB has none of.
func TestTheAddonsPeopleStayWithoutTMDB(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "", time.Hour)
	e.service.UseTitleCredits(e.queue)
	addonPeople := append(slices.Clone(addonCast), "Eve Director", "Will Writer")
	people := func(item Item) []string {
		var named []string
		for _, p := range item.People {
			named = append(named, p.Name)
		}
		return named
	}
	if got := people(e.open(e.admin, "Harbor Lights")); !slices.Equal(got, addonPeople) {
		t.Errorf("without a key: %v", got)
	}
	if asked := e.tmdb.requests("/"); len(asked) != 0 {
		t.Errorf("TMDB was asked %v without a key", asked)
	}
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	e.tmdb.mu.Lock()
	e.tmdb.failing["/movie/1002"] = true
	delete(e.tmdb.finds, "tt0882012")
	e.tmdb.mu.Unlock()
	if got := people(e.open(e.admin, "Quiet Pier")); !slices.Equal(got, addonPeople) {
		t.Errorf("when TMDB fails: %v", got)
	}
	if got := people(e.open(e.admin, "Kelp Bay")); !slices.Equal(got, addonPeople) {
		t.Errorf("for a title TMDB does not know: %v", got)
	}
	e.settle()
	if got := people(e.open(e.admin, "Quiet Pier")); !slices.Equal(got, addonPeople) {
		t.Errorf("when TMDB failed again: %v", got)
	}
	// Once TMDB answers, its credits are the title's, and taking the key
	// off brings the addon's back.
	e.tmdb.mu.Lock()
	delete(e.tmdb.failing, "/movie/1002")
	e.tmdb.mu.Unlock()
	if got := people(e.open(e.admin, "Quiet Pier")); !slices.Equal(got, []string{"Ana Actor", "Ben Actor"}) {
		t.Errorf("once TMDB answered: %v", got)
	}
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	if got := people(e.open(e.admin, "Quiet Pier")); !slices.Equal(got, addonPeople) {
		t.Errorf("once the key was taken off: %v", got)
	}
}

// Two people of the same name TMDB tells apart are two people, each
// listing their own titles. The person the addon named, kept by name,
// becomes the TMDB person credited in their title, with the users' marks,
// and the name still reaches them.
func TestNamesakesAreKeptApart(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "", time.Hour)
	e.service.UseTitleCredits(e.queue)
	// Without a key, the addon's Ben Actor is one person, kept by name,
	// whom a user marks as a favorite.
	e.open(e.admin, "Harbor Lights")
	byName := PersonID("Ben Actor")
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO user_data (user_id, item_id, favorite) VALUES ($1, $2, true)`, e.member.ID, byName); err != nil {
		t.Fatal(err)
	}
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	harbor, pier := e.open(e.admin, "Harbor Lights"), e.open(e.admin, "Quiet Pier")
	ben := func(item Item) Person {
		for _, p := range item.People {
			if p.Name == "Ben Actor" {
				return p
			}
		}
		t.Fatalf("no Ben Actor in %s", item.Name)
		return Person{}
	}
	first, second := ben(harbor), ben(pier)
	if first.ID == second.ID || first.TMDB != "6001" || second.TMDB != "6900" {
		t.Fatalf("the two Ben Actor: %+v and %+v", first, second)
	}
	if got := e.titlesOf(e.admin, first.ID, KindMovie); !slices.Equal(got, []string{"Harbor Lights"}) {
		t.Errorf("the first Ben Actor's movies: %v", got)
	}
	if got := e.titlesOf(e.admin, second.ID, KindMovie); !slices.Equal(got, []string{"Quiet Pier"}) {
		t.Errorf("the second Ben Actor's movies: %v", got)
	}
	item, err := e.service.Item(t.Context(), e.admin, byName)
	if err != nil || item.ID != first.ID {
		t.Errorf("Ben Actor by name: %+v %v", item, err)
	}
	var favorite bool
	if err := e.service.db.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM user_data WHERE user_id = $1 AND item_id = $2 AND favorite)`,
		e.member.ID, first.ID).Scan(&favorite); err != nil || !favorite {
		t.Errorf("the favorite did not follow Ben Actor: %v", err)
	}
	var derived accounts.ID
	if err := e.service.db.QueryRow(t.Context(), `SELECT polyfin_person_tmdb_id('6001')`).Scan(&derived); err != nil || derived != first.ID {
		t.Errorf("the database derives %s for 6001, the library %s: %v", derived, first.ID, err)
	}
}

// A pass reads the credits of the libraries' titles nobody opened, at
// TMDB's pace: the lookup TMDB refused for its rate waits as long as
// Retry-After asks, then is made again. The people then list those
// titles.
func TestAPassReadsTheLibrariesCreditsAtTMDBsPace(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", 5*time.Millisecond)
	e.tmdb.mu.Lock()
	e.tmdb.limited = 1
	e.tmdb.mu.Unlock()
	started := time.Now()
	e.service.UseTitleCredits(e.queue)
	for {
		e.settle()
		var kept int
		if err := e.service.db.QueryRow(t.Context(), `SELECT count(*) FROM title_credits WHERE cardinality(titles) > 0`).Scan(&kept); err != nil {
			t.Fatal(err)
		}
		if kept == len(e.titles) {
			break
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("the pass read the credits of %d titles", kept)
		case <-time.After(10 * time.Millisecond):
		}
	}
	e.tmdb.mu.Lock()
	refusals := slices.Clone(e.tmdb.refusals)
	e.tmdb.mu.Unlock()
	if len(refusals) != 1 {
		t.Fatalf("refused %d requests", len(refusals))
	}
	if took := time.Since(started); took < time.Second {
		t.Errorf("the pass took %v, though TMDB asked to wait a second", took)
	}
	// The refused request was made again, after the wait.
	asked := e.tmdb.requests("/")
	first := asked[0]
	if asked[1] != first {
		t.Errorf("after the refusal of %s, TMDB was asked %v", first, asked[1:])
	}
	ana := itemID(personTMDBKey("6000"))
	if got := e.titlesOf(e.admin, ana, KindMovie); !slices.Equal(got, []string{"Harbor Lights", "Hidden Cove", "Kelp Bay", "Night Shift", "Quiet Pier", "Salt Road"}) {
		t.Errorf("Ana Actor's movies: %v", got)
	}
	if got := e.titlesOf(e.admin, ana, KindSeries); !slices.Equal(got, []string{"Tidewater"}) {
		t.Errorf("Ana Actor's series: %v", got)
	}
}

// A person's page lists the library titles TMDB credits them in, though
// no one opened them, matched by their TMDB identifier, or by the one box
// sets or credits learnt for their IMDb one; titles beyond the libraries,
// those the user's parental control forbids, and those they hid are not.
func TestAPersonListsTheLibraryTitlesTMDBCreditsThemIn(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.tmdb.answers["/person/6000/combined_credits"] = map[string]any{"id": 6000,
		"cast": []any{
			map[string]any{"id": 1001, "media_type": "movie", "title": "Harbor Lights"},
			map[string]any{"id": 1011, "media_type": "movie", "title": "Salt Road"},
			map[string]any{"id": 1012, "media_type": "movie", "title": "Kelp Bay"},
			map[string]any{"id": 1013, "media_type": "movie", "title": "Night Shift"},
			map[string]any{"id": 1501, "media_type": "movie", "title": "Far Lighthouse"},
			map[string]any{"id": 2031, "media_type": "tv", "name": "Tidewater"},
		},
		"crew": []any{map[string]any{"id": 1014, "media_type": "movie", "title": "Hidden Cove", "job": "Producer"}}}
	// Box sets learnt Kelp Bay's TMDB identifier, and Night Shift's;
	// Tidewater's credits were read.
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO movie_collections (provider, id, found, asked_at, tmdb_id)
		VALUES ('imdb', 'tt0882012', true, now(), 1012), ('imdb', 'tt0882013', true, now(), 1013)`); err != nil {
		t.Fatal(err)
	}
	e.open(e.admin, "Tidewater")
	e.open(e.admin, "Harbor Lights")
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	ana := itemID(personTMDBKey("6000"))
	if err := e.service.userData.Hide(t.Context(), child.ID, e.titles["Hidden Cove"], time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := e.titlesOf(e.admin, ana, KindMovie); !slices.Equal(got, []string{"Harbor Lights", "Hidden Cove", "Kelp Bay", "Night Shift", "Salt Road"}) {
		t.Errorf("Ana Actor's movies: %v", got)
	}
	if got := e.titlesOf(child, ana, KindMovie); !slices.Equal(got, []string{"Harbor Lights", "Kelp Bay", "Salt Road"}) {
		t.Errorf("Ana Actor's movies for a child who hid Hidden Cove: %v", got)
	}
	if got := e.titlesOf(child, ana, KindSeries); !slices.Equal(got, []string{"Tidewater"}) {
		t.Errorf("Ana Actor's series: %v", got)
	}
	if asked := e.tmdb.requests("/3/person/6000/combined_credits"); len(asked) != 1 {
		t.Errorf("Ana Actor's credits were asked %d times", len(asked))
	}
	// Without a key, she lists the titles she is known in.
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	if got := e.titlesOf(e.admin, ana, KindMovie); !slices.Equal(got, []string{"Harbor Lights"}) {
		t.Errorf("Ana Actor's movies without a key: %v", got)
	}
}

// Refresh metadata on a person by their name's identifier, once they are
// kept by their TMDB identifier, refreshes that person: they are looked up
// on TMDB again.
func TestRefreshByNameReachesThePersonOfTheIdentifier(t *testing.T) {
	t.Parallel()
	e := newPeopleEnv(t, "k3y", []stremio.CastMember{{Name: "Ben Actor", TmdbID: "21"}})
	e.tmdb.addPerson("21", "Ben Actor", "/ben.jpg", "Ben's life.", nil)
	e.cast(e.admin)
	byID := itemID(personTMDBKey("21"))
	if got := e.person(e.admin, "Ben Actor"); got.ID != byID || got.Overview != "Ben's life." {
		t.Fatalf("Ben Actor's page: %s %q", got.ID, got.Overview)
	}
	e.mu(func() { e.tmdb.people["21/en"] = map[string]any{"biography": "Ben's new life."} })
	if err := e.service.Refresh(t.Context(), e.admin, PersonID("Ben Actor")); err != nil {
		t.Fatal(err)
	}
	e.service.personLookups.wg.Wait()
	item, err := e.service.Item(t.Context(), e.admin, byID)
	if err != nil || item.Overview != "Ben's new life." {
		t.Errorf("after Refresh metadata by name: %q %v", item.Overview, err)
	}
}

// Refresh metadata on a title reads its TMDB credits again, ahead of the
// passes, and its people then are TMDB's current ones; until then, the
// kept ones show.
func TestRefreshReadsATitlesCreditsAgain(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	if got := cast(e.open(e.admin, "Quiet Pier").People, "Actor"); !slices.Equal(got, []string{"Ana Actor", "Ben Actor"}) {
		t.Fatalf("Quiet Pier's actors: %v", got)
	}
	e.tmdb.movie("tt0882002", 1002, billed([]int{6002, 6000, 6900}, map[int]string{6002: "Cleo Actor", 6000: "Ana Actor", 6900: "Ben Actor"}), nil)
	if err := e.service.Refresh(t.Context(), e.admin, e.titles["Quiet Pier"]); err != nil {
		t.Fatal(err)
	}
	e.settle()
	item, err := e.service.Item(t.Context(), e.admin, e.titles["Quiet Pier"])
	if err != nil {
		t.Fatal(err)
	}
	if got := cast(item.People, "Actor"); !slices.Equal(got, []string{"Cleo Actor", "Ana Actor", "Ben Actor"}) {
		t.Errorf("Quiet Pier's actors after Refresh metadata: %v", got)
	}
	if got := e.titlesOf(e.admin, itemID(personTMDBKey("6002")), KindMovie); !slices.Equal(got, []string{"Quiet Pier"}) {
		t.Errorf("Cleo Actor's movies: %v", got)
	}
	if asked := e.tmdb.requests("/3/movie/1002?"); len(asked) != 2 {
		t.Errorf("Quiet Pier's credits were asked %d times", len(asked))
	}
}

// A title's kept credits are read from the database once, then from
// memory, which every write of them follows: a refresh asks TMDB again,
// and the title shows its new credits.
func TestKeptCreditsAreRemembered(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	if got := cast(e.open(e.admin, "Quiet Pier").People, "Actor"); !slices.Equal(got, []string{"Ana Actor", "Ben Actor"}) {
		t.Fatalf("Quiet Pier's actors: %v", got)
	}
	e.settle()
	// Changed behind Polyfin's back, the credits kept are not read again.
	if _, err := e.service.db.Exec(t.Context(), `UPDATE title_credits SET credits = '[]'`); err != nil {
		t.Fatal(err)
	}
	item, err := e.service.Item(t.Context(), e.admin, e.titles["Quiet Pier"])
	if err != nil {
		t.Fatal(err)
	}
	if got := cast(item.People, "Actor"); !slices.Equal(got, []string{"Ana Actor", "Ben Actor"}) {
		t.Errorf("Quiet Pier's actors as remembered: %v", got)
	}
	e.tmdb.movie("tt0882002", 1002, billed([]int{6002, 6000, 6900}, map[int]string{6002: "Cleo Actor", 6000: "Ana Actor", 6900: "Ben Actor"}), nil)
	if err := e.service.Refresh(t.Context(), e.admin, e.titles["Quiet Pier"]); err != nil {
		t.Fatal(err)
	}
	e.settle()
	items, err := e.service.Items(t.Context(), e.admin, []accounts.ID{e.titles["Quiet Pier"], e.titles["Harbor Lights"]})
	if err != nil || len(items) != 2 {
		t.Fatalf("%d items: %v", len(items), err)
	}
	if got := cast(items[0].People, "Actor"); !slices.Equal(got, []string{"Cleo Actor", "Ana Actor", "Ben Actor"}) {
		t.Errorf("Quiet Pier's actors after Refresh metadata: %v", got)
	}
	if got := cast(items[1].People, "Actor"); !slices.Equal(got, addonCast) {
		t.Errorf("Harbor Lights' actors, its credits not read yet: %v", got)
	}
}

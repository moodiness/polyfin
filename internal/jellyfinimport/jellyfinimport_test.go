package jellyfinimport

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/userdata"
)

const goodKey = "0123456789abcdef0123456789abcdef"

// fakeJellyfin answers as a Jellyfin server does the requests of an
// import, with the API key goodKey: its users, and their items by the
// filter asked, a page at a time.
type fakeJellyfin struct {
	url string

	mu       sync.Mutex
	requests []*http.Request
	users    []map[string]any
	// items are each user's items by filter (IsPlayed, IsResumable,
	// IsFavorite); series, the identifiers of the series by their id.
	items  map[string]map[string][]map[string]any
	series map[string]map[string]string
	// refused are the users whose items answer 401, as after the key was
	// revoked; hold, when set, holds every items request until closed.
	refused map[string]bool
	hold    chan struct{}
}

func newFakeJellyfin(t *testing.T) *fakeJellyfin {
	t.Helper()
	f := &fakeJellyfin{items: map[string]map[string][]map[string]any{}, series: map[string]map[string]string{}, refused: map[string]bool{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

func (f *fakeJellyfin) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Clone(r.Context()))
	hold := f.hold
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/System/Info/Public" {
		_, _ = io.WriteString(w, `{"Id":"f1e2d3","ServerName":"Home","Version":"10.10.7","ProductName":"Jellyfin Server"}`)
		return
	}
	if r.Header.Get("Authorization") != `MediaBrowser Token="`+goodKey+`"` {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	query := r.URL.Query()
	f.mu.Lock()
	defer f.mu.Unlock()
	switch user, found := strings.CutPrefix(r.URL.Path, "/Users/"); {
	case r.URL.Path == "/Users":
		_ = json.NewEncoder(w).Encode(f.users)
	case found && strings.HasSuffix(user, "/Items"):
		user = strings.TrimSuffix(user, "/Items")
		if hold != nil {
			f.mu.Unlock()
			<-hold
			f.mu.Lock()
		}
		if f.refused[user] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var all []map[string]any
		if ids := query.Get("Ids"); ids != "" {
			for _, id := range strings.Split(ids, ",") {
				if providers, ok := f.series[id]; ok {
					all = append(all, map[string]any{"Id": id, "Type": "Series", "ProviderIds": providers})
				}
			}
		} else {
			all = f.items[user][query.Get("Filters")]
		}
		start, _ := strconv.Atoi(query.Get("StartIndex"))
		limit, err := strconv.Atoi(query.Get("Limit"))
		if err != nil {
			limit = len(all)
		}
		page := all[min(start, len(all)):min(start+limit, len(all))]
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": page, "TotalRecordCount": len(all), "StartIndex": start})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// movie is a movie item with providers and the user's data.
func movie(id, name string, providers map[string]string, userData map[string]any) map[string]any {
	return map[string]any{"Id": id, "Name": name, "Type": "Movie", "ProductionYear": 2019, "ProviderIds": providers,
		"RunTimeTicks": int64(100 * time.Minute / tick), "UserData": userData}
}

// episode is an episode of series numbered so (nil leaves the number
// out), with the user's data.
func episode(id, series string, season int, number *int, userData map[string]any) map[string]any {
	item := map[string]any{"Id": id, "Name": "Episode " + id, "Type": "Episode", "SeriesId": series, "SeriesName": "Series " + series,
		"ParentIndexNumber": season, "ProviderIds": map[string]string{"Tvdb": "777"}, "RunTimeTicks": int64(50 * time.Minute / tick),
		"UserData": userData}
	if number != nil {
		item["IndexNumber"] = *number
	}
	return item
}

// sent lists the requests the fake received.
func (f *fakeJellyfin) sent() []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*http.Request(nil), f.requests...)
}

type harness struct {
	*Service
	pool  *pgxpool.Pool
	store *accounts.Store
	lib   *library.Service
	data  *userdata.Store
	f     *fakeJellyfin
}

// newHarness is a service on a fresh database reading a fake Jellyfin
// server two items a page, without waiting between requests.
func newHarness(t *testing.T) harness {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := stremio.NewClient("test")
	lib := library.New(pool, addons.New(pool, client), client, logger, store.Settings)
	data := userdata.New(pool)
	s := New(Options{Settings: store.Settings, Titles: lib, UserData: data, Version: "1.2.3", Logger: logger})
	s.timing.gap, s.timing.retryFirst, s.timing.pageSize = 0, time.Millisecond, 2
	t.Cleanup(s.Close)
	return harness{Service: s, pool: pool, store: store, lib: lib, data: data, f: newFakeJellyfin(t)}
}

func (h harness) user(t *testing.T, name string) accounts.User {
	t.Helper()
	user, err := h.store.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

// listed records a title or an episode as a catalog listed it.
func (h harness) listed(t *testing.T, kind, key, data string) {
	t.Helper()
	if _, err := h.pool.Exec(t.Context(), "INSERT INTO items (id, key, kind, data) VALUES (gen_random_uuid(), $1, $2, $3::jsonb)",
		key, kind, data); err != nil {
		t.Fatal(err)
	}
}

// idOf is the item ref designates.
func (h harness) idOf(t *testing.T, ref library.TitleRef) userdata.Item {
	t.Helper()
	found, err := h.lib.Resolve(t.Context(), []library.TitleRef{ref})
	if err != nil || len(found[0]) != 1 {
		t.Fatalf("%+v: %v %v", ref, found, err)
	}
	return userdata.Item{ID: found[0][0].ID, Series: found[0][0].Series, Season: found[0][0].Season}
}

// dataOf is the data user keeps of ref's item.
func (h harness) dataOf(t *testing.T, user accounts.ID, ref library.TitleRef) userdata.Data {
	t.Helper()
	id := h.idOf(t, ref).ID
	got, err := h.data.Get(t.Context(), user, []accounts.ID{id})
	if err != nil {
		t.Fatal(err)
	}
	return got[id]
}

// imported starts an import of targets and waits for it to end.
func (h harness) imported(t *testing.T, targets ...Target) Status {
	t.Helper()
	if _, err := h.Start(t.Context(), h.f.url, goodKey, func(Server, []User) ([]Target, error) { return targets, nil }); err != nil {
		t.Fatal(err)
	}
	return h.ended(t)
}

// ended waits for the import running to end.
func (h harness) ended(t *testing.T) Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if status := h.Current(); status != nil && status.State != StateRunning {
			return *status
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %+v", h.Current())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func at(text string) time.Time {
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		panic(err)
	}
	return t
}

func TestUsersAreListedWithTheKeyAndFailuresTellWhy(t *testing.T) {
	h := newHarness(t)
	h.f.users = []map[string]any{
		{"Id": "a1", "Name": "Alice", "LastActivityDate": "2026-10-01T20:00:00.0000000Z", "Policy": map[string]any{"IsAdministrator": true}},
		{"Id": "b2", "Name": "Bob", "LastActivityDate": "0001-01-01T00:00:00.0000000Z",
			"Policy": map[string]any{"IsDisabled": true, "IsHidden": true}},
	}
	server, users, err := h.Users(t.Context(), h.f.url+"/", goodKey)
	if err != nil {
		t.Fatal(err)
	}
	if server.Name != "Home" || server.Version != "10.10.7" || server.Address != h.f.url {
		t.Errorf("server: %+v", server)
	}
	if len(users) != 2 || users[0].Name != "Alice" || !users[0].Administrator || users[0].Disabled || users[0].LastActivity == nil ||
		!users[0].LastActivity.Equal(at("2026-10-01T20:00:00Z")) {
		t.Errorf("alice: %+v", users)
	}
	if users[1].Administrator || !users[1].Disabled || !users[1].Hidden || users[1].LastActivity != nil {
		t.Errorf("bob: %+v", users[1])
	}
	for _, r := range h.f.sent() {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.Header.Get("User-Agent"), "Polyfin/") || strings.Contains(r.URL.String(), goodKey) {
			t.Errorf("%s %s, User-Agent %q", r.Method, r.URL, r.Header.Get("User-Agent"))
		}
	}

	if _, _, err := h.Users(t.Context(), h.f.url, "wrong-key"); !errors.Is(err, ErrKeyRefused) {
		t.Errorf("wrong key: %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	if _, _, err := h.Users(t.Context(), closed.URL, goodKey); !errors.Is(err, ErrUnreachable) {
		t.Errorf("nothing at the address: %v", err)
	}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>A web page</html>")
	}))
	defer web.Close()
	if _, _, err := h.Users(t.Context(), web.URL, goodKey); !errors.Is(err, ErrNotJellyfin) {
		t.Errorf("a web page: %v", err)
	}
	for _, address := range []string{"", "ftp://192.168.1.10", "http://user:secret@192.168.1.10:8096", "http://:8096"} {
		if _, _, err := h.Users(t.Context(), address, goodKey); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("%q: %v", address, err)
		}
	}
	for typed, want := range map[string]string{
		"192.168.1.10:8096":                 "http://192.168.1.10:8096",
		" https://media.example/jellyfin/ ": "https://media.example/jellyfin",
	} {
		if got, err := ParseAddress(typed); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", typed, got, err, want)
		}
	}
}

func TestWatchDataIsMatchedAndOnlyAddedTo(t *testing.T) {
	h := newHarness(t)
	alice := h.user(t, "alice")
	// A catalog listed a movie by its TMDB identifier, and a series by its
	// TVDB one with its first episode.
	h.listed(t, "movie", "movie|tmdb:550", `{"meta":{"id":"tmdb:550","type":"movie","name":"Listed by TMDB"}}`)
	h.listed(t, "series", "series|tvdb:81189", `{"meta":{"id":"tvdb:81189","type":"series","name":"Listed by TVDB"}}`)
	h.listed(t, "episode", "episode|tvdb:81189:1:1",
		`{"seriesId":"tvdb:81189","video":{"id":"tvdb:81189:1:1","season":1,"episode":1}}`)

	byIMDb, byTMDB := library.TitleRef{IMDb: "tt0000001"}, library.TitleRef{TMDB: 550}
	resumed, newerHere := library.TitleRef{IMDb: "tt0000002"}, library.TitleRef{IMDb: "tt0000003"}
	imdbEpisode := library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 2}
	resumedEpisode := library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 3}
	tvdbEpisode := library.TitleRef{Episode: true, TVDB: 81189, Season: 1, Number: 1}
	second, third := library.TitleRef{Episode: true, TVDB: 81189, Season: 1, Number: 2}, library.TitleRef{Episode: true, TVDB: 81189, Season: 1, Number: 3}
	series := library.TitleRef{Series: true, TVDB: 81189}

	// Polyfin already counts five plays of the first movie, the latest
	// after Jellyfin's, and keeps a newer resume point of another.
	if _, err := h.data.Change(t.Context(), alice.ID, []userdata.Item{h.idOf(t, byIMDb)}, func(d *userdata.Data) {
		d.Played, d.PlayCount, d.LastPlayed = true, 5, new(at("2026-09-10T20:00:00Z"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.data.Change(t.Context(), alice.ID, []userdata.Item{h.idOf(t, newerHere)}, func(d *userdata.Data) {
		d.Position, d.Runtime, d.LastPlayed = 50*time.Minute, 100*time.Minute, new(at("2026-10-05T20:00:00Z"))
	}); err != nil {
		t.Fatal(err)
	}

	two, three := 2, 3
	h.f.series = map[string]map[string]string{"s-imdb": {"Imdb": "tt0100", "Tvdb": "1"}, "s-tvdb": {"Tvdb": "81189"}}
	played := func(date string, count int) map[string]any {
		return map[string]any{"Played": true, "PlayCount": count, "LastPlayedDate": date}
	}
	multi := episode("e3", "s-tvdb", 1, &two, played("2026-09-06T20:00:00.0000000Z", 1))
	multi["IndexNumberEnd"] = 3
	h.f.items["jf-alice"] = map[string][]map[string]any{
		"IsPlayed": {
			movie("m1", "By IMDb", map[string]string{"Imdb": "tt0000001", "Tmdb": "10"}, played("2026-09-01T20:00:00.0000000Z", 3)),
			movie("m2", "By TMDB", map[string]string{"Tmdb": "550"}, played("2026-09-02T20:00:00.0000000Z", 2)),
			movie("m3", "Holiday", map[string]string{}, played("2026-09-03T20:00:00.0000000Z", 1)),
			movie("m4", "Unknown", map[string]string{"Tmdb": "999999"}, played("2026-09-03T20:00:00.0000000Z", 1)),
			episode("e1", "s-imdb", 1, &two, played("2026-09-04T20:00:00.0000000Z", 1)),
			episode("e2", "s-tvdb", 1, new(1), played("2026-09-05T20:00:00.0000000Z", 1)),
			multi,
			episode("e4", "s-imdb", 2, nil, played("2026-09-07T20:00:00.0000000Z", 1)),
		},
		"IsResumable": {
			movie("r1", "Resumed", map[string]string{"Imdb": "tt0000002"},
				map[string]any{"PlaybackPositionTicks": int64(30 * time.Minute / tick), "LastPlayedDate": "2026-10-01T20:00:00.0000000Z"}),
			movie("r2", "Newer here", map[string]string{"Imdb": "tt0000003"},
				map[string]any{"PlaybackPositionTicks": int64(30 * time.Minute / tick), "LastPlayedDate": "2026-10-01T20:00:00.0000000Z"}),
			episode("r3", "s-imdb", 1, &three,
				map[string]any{"PlaybackPositionTicks": int64(20 * time.Minute / tick), "LastPlayedDate": "2026-10-02T20:00:00.0000000Z"}),
		},
		"IsFavorite": {
			movie("m1", "By IMDb", map[string]string{"Imdb": "tt0000001", "Tmdb": "10"}, map[string]any{"IsFavorite": true}),
			{"Id": "s-tvdb", "Name": "Listed by TVDB", "Type": "Series", "ProviderIds": map[string]string{"Tvdb": "81189"},
				"UserData": map[string]any{"IsFavorite": true}},
			episode("e1", "s-imdb", 1, &two, map[string]any{"IsFavorite": true}),
		},
	}

	status := h.imported(t, Target{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"})
	if status.State != StateDone || len(status.Users) != 1 {
		t.Fatalf("%+v", status)
	}
	got := status.Users[0]
	// Newly played: by TMDB, the three episodes of the two series (one
	// file holding two); the movie by IMDb was played already.
	if got.State != UserDone || got.Read != 14 || got.Played != 5 || got.Resumed != 2 || got.Favorites != 3 || got.UnmatchedCount != 3 {
		t.Errorf("counts: %+v", got)
	}
	reasons := map[string]string{}
	for _, u := range got.Unmatched {
		reasons[u.Name] = u.Reason
	}
	if len(reasons) != 3 || reasons["Holiday"] != ReasonNoIdentifier || reasons["Unknown"] != ReasonNotFound ||
		reasons["Episode e4"] != ReasonNoIdentifier {
		t.Errorf("unmatched: %+v", got.Unmatched)
	}

	check := func(name string, ref library.TitleRef, want func(userdata.Data) bool) {
		t.Helper()
		if d := h.dataOf(t, alice.ID, ref); !want(d) {
			t.Errorf("%s: %+v", name, d)
		}
	}
	check("by IMDb", byIMDb, func(d userdata.Data) bool {
		return d.Played && d.PlayCount == 5 && d.LastPlayed.Equal(at("2026-09-10T20:00:00Z")) && d.Favorite
	})
	check("by TMDB", byTMDB, func(d userdata.Data) bool {
		return d.Played && d.PlayCount == 2 && d.LastPlayed.Equal(at("2026-09-02T20:00:00Z"))
	})
	check("episode by its series' IMDb", imdbEpisode, func(d userdata.Data) bool {
		return d.Played && d.LastPlayed.Equal(at("2026-09-04T20:00:00Z")) && d.Favorite
	})
	check("episode by its series' TVDB", tvdbEpisode, func(d userdata.Data) bool { return d.Played })
	check("second of a file", second, func(d userdata.Data) bool { return d.Played && d.LastPlayed.Equal(at("2026-09-06T20:00:00Z")) })
	check("third of a file", third, func(d userdata.Data) bool { return d.Played })
	check("resumed", resumed, func(d userdata.Data) bool {
		return !d.Played && d.Position == 30*time.Minute && d.Runtime == 100*time.Minute && d.LastPlayed.Equal(at("2026-10-01T20:00:00Z"))
	})
	check("newer here", newerHere, func(d userdata.Data) bool { return d.Position == 50*time.Minute })
	check("resumed episode", resumedEpisode, func(d userdata.Data) bool { return d.Position == 20*time.Minute })
	check("favorite series", series, func(d userdata.Data) bool { return d.Favorite })

	// Watched on in Polyfin since, then imported again: nothing is added
	// twice, and Polyfin's newer resume point stays.
	if _, err := h.data.Change(t.Context(), alice.ID, []userdata.Item{h.idOf(t, resumed)}, func(d *userdata.Data) {
		d.Position, d.LastPlayed = 60*time.Minute, new(at("2026-10-08T20:00:00Z"))
	}); err != nil {
		t.Fatal(err)
	}
	again := h.imported(t, Target{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}).Users[0]
	if again.Played != 0 || again.Resumed != 0 || again.Favorites != 0 || again.UnmatchedCount != 3 {
		t.Errorf("again: %+v", again)
	}
	check("resumed, after", resumed, func(d userdata.Data) bool { return d.Position == 60*time.Minute })
	check("by TMDB, after", byTMDB, func(d userdata.Data) bool { return d.PlayCount == 2 })

	for _, r := range h.f.sent() {
		if r.Method != http.MethodGet {
			t.Errorf("an import sent %s %s", r.Method, r.URL.Path)
		}
	}
}

func TestOneImportRunsAtATimeAndStops(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.user(t, "alice"), h.user(t, "bob")
	h.f.items["jf-alice"] = map[string][]map[string]any{"IsPlayed": {movie("m1", "One", map[string]string{"Imdb": "tt0000001"},
		map[string]any{"Played": true})}}
	hold := make(chan struct{})
	h.f.hold = hold
	targets := []Target{{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}, {JellyfinID: "jf-bob", User: bob.ID, UserName: "bob"}}
	if _, err := h.Start(t.Context(), h.f.url, goodKey, func(Server, []User) ([]Target, error) { return targets, nil }); err != nil {
		t.Fatal(err)
	}
	chose := false
	if _, err := h.Start(t.Context(), h.f.url, goodKey, func(Server, []User) ([]Target, error) {
		chose = true
		return targets, nil
	}); !errors.Is(err, ErrRunning) || chose {
		t.Errorf("a second import: %v, chose %v", err, chose)
	}
	h.Stop()
	close(hold)
	status := h.ended(t)
	if status.State != StateStopped || status.Users[0].State != UserWaiting || status.Users[1].State != UserWaiting {
		t.Errorf("stopped: %+v", status)
	}
	if d, _ := h.data.Get(t.Context(), alice.ID, []accounts.ID{h.idOf(t, library.TitleRef{IMDb: "tt0000001"}).ID}); len(d) != 0 {
		t.Errorf("a stopped import saved %+v", d)
	}

	// The key revoked midway: the first user's data is imported, the
	// import fails at the second.
	h.f.mu.Lock()
	h.f.hold = nil
	h.f.refused["jf-bob"] = true
	h.f.mu.Unlock()
	status = h.imported(t, targets...)
	if status.State != StateFailed || status.Problem != ProblemKeyRefused || status.Users[0].State != UserDone || status.Users[0].Played != 1 ||
		status.Users[1].State != UserFailed {
		t.Errorf("refused: %+v", status)
	}

	// A choice that fails starts nothing, and leaves the next free.
	if _, err := h.Start(t.Context(), h.f.url, goodKey, func(Server, []User) ([]Target, error) {
		return nil, errors.New("name taken")
	}); err == nil || errors.Is(err, ErrRunning) {
		t.Errorf("failed choice: %v", err)
	}
	if _, err := h.Start(t.Context(), h.f.url, goodKey, func(Server, []User) ([]Target, error) { return nil, nil }); err != nil {
		t.Errorf("after a failed choice: %v", err)
	}
}

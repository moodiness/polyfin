package jellyfinimport

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
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
// import, with the API key goodKey, the server's, a user's own key, or a
// session signed in: its users, and their items by the filter asked, a
// page at a time.
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
	// revoked; forbidden, those whose items answer 403, which the key may
	// not read; hold, when set, holds every items request until closed.
	refused, forbidden map[string]bool
	hold               chan struct{}
	// userKeys are users' own keys, by key, which /Users/Me answers with
	// their owner. Another user's items answer them 403, as Jellyfin's do,
	// or, with ignoresUser, the owner's items, as some servers answer.
	userKeys    map[string]string
	ignoresUser bool
	// passwords are the users' passwords by name, which a sign-in names
	// them by, as Jellyfin's apps do; barred users may not sign in, and
	// signsInAs signs a user named in as another, by id. sessions are the
	// users signed in by their token, until they sign out: a session reads
	// as its user's own key does. noMe leaves /Users/Me out, and
	// adminsOnly lists the users to administrators' sessions only, as
	// Jellyfin does.
	passwords  map[string]string
	barred     map[string]bool
	signsInAs  map[string]string
	sessions   map[string]string
	noMe       bool
	adminsOnly bool
	// emby answers as Emby behind a proxy passing on only /emby: the key
	// in X-Emby-Token, a sign-in's app and device in X-Emby-Authorization,
	// no /Users/Me, and the server's API keys under /Auth/Keys.
	emby bool
}

func newFakeJellyfin(t *testing.T) *fakeJellyfin {
	t.Helper()
	f := &fakeJellyfin{items: map[string]map[string][]map[string]any{}, series: map[string]map[string]string{}, refused: map[string]bool{},
		forbidden: map[string]bool{}, userKeys: map[string]string{}, passwords: map[string]string{}, barred: map[string]bool{},
		signsInAs: map[string]string{}, sessions: map[string]string{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// signIn answers a sign-in as Jellyfin does: it needs the app and device
// signing in, and a user's name and password.
func (f *fakeJellyfin) signIn(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if f.emby {
		auth = r.Header.Get("X-Emby-Authorization")
	}
	for _, field := range []string{`Client="`, `Device="`, `DeviceId="`, `Version="`} {
		if !strings.Contains(auth, field) || strings.Contains(auth, field+`"`) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	var body struct{ Username, Pw string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if password, known := f.passwords[body.Username]; !known || password != body.Pw {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if f.barred[body.Username] {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	id := f.signsInAs[body.Username]
	for _, user := range f.users {
		if id == "" && user["Name"] == body.Username {
			id = user["Id"].(string)
		}
	}
	token := "session-" + strconv.Itoa(len(f.requests))
	f.sessions[token] = id
	_ = json.NewEncoder(w).Encode(map[string]any{"User": map[string]any{"Id": id, "Name": body.Username}, "AccessToken": token})
}

// open counts the sessions signed in and not out.
func (f *fakeJellyfin) open() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions)
}

func (f *fakeJellyfin) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Clone(r.Context()))
	hold := f.hold
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	if f.emby {
		trimmed, under := strings.CutPrefix(path, "/emby")
		if !under {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		path = trimmed
	}
	if path == "/System/Info/Public" {
		_, _ = io.WriteString(w, `{"Id":"f1e2d3","ServerName":"Home","Version":"10.10.7","ProductName":"Jellyfin Server"}`)
		return
	}
	query := r.URL.Query()
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method == http.MethodPost && path == "/Users/AuthenticateByName" {
		f.signIn(w, r)
		return
	}
	key := strings.TrimSuffix(strings.TrimPrefix(r.Header.Get("Authorization"), `MediaBrowser Token="`), `"`)
	if f.emby {
		key = r.Header.Get("X-Emby-Token")
	}
	owner, userKey := f.userKeys[key]
	if id, session := f.sessions[key]; session {
		owner, userKey = id, true
	}
	if key != goodKey && !userKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch user, found := strings.CutPrefix(path, "/Users/"); {
	case r.Method == http.MethodPost && path == "/Sessions/Logout":
		delete(f.sessions, key)
		w.WriteHeader(http.StatusNoContent)
	case path == "/Users":
		if f.adminsOnly && userKey && !f.administrator(owner) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(f.users)
	case f.emby && path == "/Auth/Keys":
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{{"Id": 2, "AccessToken": goodKey, "AppName": "Polyfin"}}})
	case f.emby && path == "/Users/Me":
		// Emby reads "Me" as a user's identifier.
		w.WriteHeader(http.StatusInternalServerError)
	case path == "/Users/Me":
		if !userKey || f.noMe {
			// Jellyfin's answer to the server's key, which is no user's;
			// without /Users/Me, every key gets it.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": owner, "Name": "Owner"})
	case found && strings.HasSuffix(user, "/Items"):
		user = strings.TrimSuffix(user, "/Items")
		if hold != nil {
			f.mu.Unlock()
			<-hold
			f.mu.Lock()
		}
		switch {
		case userKey && user != owner && f.ignoresUser:
			user = owner
		case userKey && user != owner, f.forbidden[user]:
			w.WriteHeader(http.StatusForbidden)
			return
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
		if fields := query.Get("Fields"); f.emby && !strings.Contains(fields, "UserDataPlayCount,UserDataLastPlayedDate") {
			// Emby lists user data without these unless asked.
			var bare []map[string]any
			for _, item := range page {
				item = maps.Clone(item)
				if data, ok := item["UserData"].(map[string]any); ok {
					data = maps.Clone(data)
					delete(data, "PlayCount")
					delete(data, "LastPlayedDate")
					item["UserData"] = data
				}
				bare = append(bare, item)
			}
			page = bare
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": page, "TotalRecordCount": len(all), "StartIndex": start})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// administrator tells whether the user id names is an administrator.
func (f *fakeJellyfin) administrator(id string) bool {
	for _, user := range f.users {
		if policy, _ := user["Policy"].(map[string]any); user["Id"] == id && policy["IsAdministrator"] == true {
			return true
		}
	}
	return false
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
	if _, err := h.Start(t.Context(), to(h.f.url, Credentials{Key: goodKey}), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) { return targets, nil }); err != nil {
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

// to is a connection to the Jellyfin server at address with credentials.
func to(address string, credentials Credentials) Connection {
	return Connection{Kind: Jellyfin, Address: address, Credentials: credentials}
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
	server, users, err := h.Users(t.Context(), to(h.f.url+"/", Credentials{Key: goodKey}))
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

	if _, _, err := h.Users(t.Context(), to(h.f.url, Credentials{Key: "wrong-key"})); !errors.Is(err, ErrKeyRefused) {
		t.Errorf("wrong key: %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	if _, _, err := h.Users(t.Context(), to(closed.URL, Credentials{Key: goodKey})); !errors.Is(err, ErrUnreachable) {
		t.Errorf("nothing at the address: %v", err)
	}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>A web page</html>")
	}))
	defer web.Close()
	if _, _, err := h.Users(t.Context(), to(web.URL, Credentials{Key: goodKey})); !errors.Is(err, ErrNotJellyfin) {
		t.Errorf("a web page: %v", err)
	}
	for _, address := range []string{"", "ftp://192.168.1.10", "http://user:secret@192.168.1.10:8096", "http://:8096"} {
		if _, _, err := h.Users(t.Context(), to(address, Credentials{Key: goodKey})); !errors.Is(err, ErrInvalidAddress) {
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
	if _, err := h.Start(t.Context(), to(h.f.url, Credentials{Key: goodKey}), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) { return targets, nil }); err != nil {
		t.Fatal(err)
	}
	chose := false
	if _, err := h.Start(t.Context(), to(h.f.url, Credentials{Key: goodKey}), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) {
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
	if _, err := h.Start(t.Context(), to(h.f.url, Credentials{Key: goodKey}), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) {
		return nil, errors.New("name taken")
	}); err == nil || errors.Is(err, ErrRunning) {
		t.Errorf("failed choice: %v", err)
	}
	if _, err := h.Start(t.Context(), to(h.f.url, Credentials{Key: goodKey}), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) { return nil, nil }); err != nil {
		t.Errorf("after a failed choice: %v", err)
	}
}

// A user's own key reads only that user's watch data. Some servers answer
// it with its owner's data whatever user is asked, which an import would
// put in another user's account: only its owner is imported with it.
func TestAUserKeyImportsOnlyItsOwnersWatchData(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.user(t, "alice"), h.user(t, "bob")
	h.f.users = []map[string]any{{"Id": "jf-alice", "Name": "Alice"}, {"Id": "jf-bob", "Name": "Bob"}}
	h.f.items["jf-alice"] = map[string][]map[string]any{"IsPlayed": {movie("m1", "Alice's", map[string]string{"Imdb": "tt0000001"},
		map[string]any{"Played": true})}}
	const aliceKey = "alices-own-key"
	h.f.userKeys[aliceKey] = "jf-alice"
	h.f.ignoresUser = true

	if server, _, err := h.Users(t.Context(), to(h.f.url, Credentials{Key: goodKey})); err != nil || server.KeyOwner != "" {
		t.Errorf("the server's key: %+v %v", server, err)
	}
	if server, _, err := h.Users(t.Context(), to(h.f.url, Credentials{Key: aliceKey})); err != nil || server.KeyOwner != "jf-alice" {
		t.Fatalf("Alice's key: %+v %v", server, err)
	}

	// Bob's watch data with Alice's key: refused, before anything is read.
	_, err := h.Start(t.Context(), to(h.f.url, Credentials{Key: aliceKey}), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) {
		return []Target{{JellyfinID: "jf-bob", User: bob.ID, UserName: "bob"}}, nil
	})
	if !errors.Is(err, ErrNotKeyOwner) {
		if err == nil {
			h.ended(t)
		}
		t.Errorf("Bob with Alice's key: %v", err)
	}
	aliceMovie := library.TitleRef{IMDb: "tt0000001"}
	if d := h.dataOf(t, bob.ID, aliceMovie); d.Played {
		t.Errorf("Bob was given Alice's movie: %+v", d)
	}
	for _, r := range h.f.sent() {
		if strings.HasPrefix(r.URL.Path, "/Users/jf-bob") {
			t.Errorf("read %s", r.URL.Path)
		}
	}

	// Alice's, with her key: imported.
	if _, err := h.Start(t.Context(), to(h.f.url, Credentials{Key: aliceKey}), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) {
		return []Target{{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if status := h.ended(t); status.State != StateDone || status.Users[0].Played != 1 {
		t.Errorf("Alice: %+v", status)
	}
	if d := h.dataOf(t, alice.ID, aliceMovie); !d.Played {
		t.Errorf("Alice's movie: %+v", d)
	}
}

// A user whose data the server does not let the key read fails alone, and
// says so: the import goes on with the next users.
func TestAUserTheKeyMayNotReadFailsAlone(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.user(t, "alice"), h.user(t, "bob")
	h.f.items["jf-alice"] = map[string][]map[string]any{"IsPlayed": {movie("m1", "Alice's", map[string]string{"Imdb": "tt0000001"},
		map[string]any{"Played": true})}}
	h.f.forbidden["jf-bob"] = true
	status := h.imported(t, Target{JellyfinID: "jf-bob", User: bob.ID, UserName: "bob"}, Target{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"})
	if status.State != StateDone || status.Problem != "" {
		t.Errorf("import: %+v", status)
	}
	if bobs := status.Users[0]; bobs.State != UserFailed || bobs.Problem != ProblemForbidden {
		t.Errorf("Bob: %+v", bobs)
	}
	if alices := status.Users[1]; alices.State != UserDone || alices.Played != 1 {
		t.Errorf("Alice: %+v", alices)
	}
}

// Signed in with a name and password, an import reads as that user, and
// reads each other user signed in as them, with their password: even a
// server that answers a session with its own user's data whatever user is
// asked gives each user theirs. Every session ends.
func TestSigningInReadsEachUserAsThemselves(t *testing.T) {
	h := newHarness(t)
	alice, bob, carol := h.user(t, "alice"), h.user(t, "bob"), h.user(t, "carol")
	h.f.users = []map[string]any{{"Id": "jf-alice", "Name": "Alice"}, {"Id": "jf-bob", "Name": "Bob"}, {"Id": "jf-carol", "Name": "Carol"}}
	// Carol's account has no password.
	h.f.passwords = map[string]string{"Alice": "alice's password", "Bob": "bob's password", "Carol": ""}
	movies := map[string]string{"jf-alice": "tt0000001", "jf-bob": "tt0000002", "jf-carol": "tt0000003"}
	for user, imdb := range movies {
		h.f.items[user] = map[string][]map[string]any{"IsPlayed": {movie(user, user, map[string]string{"Imdb": imdb}, map[string]any{"Played": true})}}
	}
	h.f.ignoresUser = true
	// A session is its user's even where /Users/Me does not tell.
	h.f.noMe = true
	asAlice := Credentials{Name: "Alice", Password: "alice's password"}

	server, _, err := h.Users(t.Context(), to(h.f.url, asAlice))
	if err != nil || server.KeyOwner != "jf-alice" {
		t.Fatalf("signed in as Alice: %+v %v", server, err)
	}
	if open := h.f.open(); open != 0 {
		t.Errorf("sessions left after listing the users: %d", open)
	}

	// Bob's watch data, Bob not signed in: refused.
	_, err = h.Start(t.Context(), to(h.f.url, asAlice), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) {
		return []Target{{JellyfinID: "jf-bob", User: bob.ID, UserName: "bob"}}, nil
	})
	if !errors.Is(err, ErrNotKeyOwner) {
		if err == nil {
			h.ended(t)
		}
		t.Errorf("Bob not signed in: %v", err)
	}

	_, err = h.Start(t.Context(), to(h.f.url, asAlice), accounts.User{}, func(_ Server, _ []User, signIn SignIn) ([]Target, error) {
		if err := signIn("jf-bob", "bob's password"); err != nil {
			return nil, err
		}
		if err := signIn("jf-carol", ""); err != nil {
			return nil, err
		}
		return []Target{{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}, {JellyfinID: "jf-bob", User: bob.ID, UserName: "bob"},
			{JellyfinID: "jf-carol", User: carol.ID, UserName: "carol"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if status := h.ended(t); status.State != StateDone {
		t.Errorf("import: %+v", status)
	}
	for jellyfinID, user := range map[string]accounts.User{"jf-alice": alice, "jf-bob": bob, "jf-carol": carol} {
		for other, imdb := range movies {
			if played := h.dataOf(t, user.ID, library.TitleRef{IMDb: imdb}).Played; played != (other == jellyfinID) {
				t.Errorf("%s has %s's movie: %v", user.Name, other, played)
			}
		}
	}
	if open := h.f.open(); open != 0 {
		t.Errorf("sessions left after the import: %d", open)
	}
}

// A password the server refuses, an account it does not let sign in, or a
// sign-in that opens another user's session starts nothing, and says why;
// the sessions opened meanwhile end.
func TestASignInThatFailsStartsNothing(t *testing.T) {
	h := newHarness(t)
	h.f.users = []map[string]any{{"Id": "jf-alice", "Name": "Alice"}, {"Id": "jf-bob", "Name": "Bob"}, {"Id": "jf-carol", "Name": "Carol"},
		{"Id": "jf-dan", "Name": "Dan"}}
	h.f.passwords = map[string]string{"Alice": "alice's password", "Bob": "bob's password", "Carol": "carol's password", "Dan": "dan's password"}
	// Bob's account is disabled; signing Dan in opens Alice's session.
	h.f.barred["Bob"] = true
	h.f.signsInAs["Dan"] = "jf-alice"
	asAlice := Credentials{Name: "Alice", Password: "alice's password"}

	if _, _, err := h.Users(t.Context(), to(h.f.url, Credentials{Name: "Alice", Password: "wrong"})); !errors.Is(err, ErrSignInRefused) {
		t.Errorf("a wrong password: %v", err)
	}
	if _, _, err := h.Users(t.Context(), to(h.f.url, Credentials{Name: "Bob", Password: "bob's password"})); !errors.Is(err, ErrSignInForbidden) {
		t.Errorf("a disabled account: %v", err)
	}
	for _, c := range []struct {
		jellyfinID, password string
		want                 error
	}{
		{"jf-bob", "bob's password", ErrSignInForbidden},
		{"jf-carol", "wrong", ErrSignInRefused},
		{"jf-dan", "dan's password", ErrOtherUser},
	} {
		_, err := h.Start(t.Context(), to(h.f.url, asAlice), accounts.User{}, func(_ Server, _ []User, signIn SignIn) ([]Target, error) {
			if c.jellyfinID != "jf-carol" {
				if err := signIn("jf-carol", "carol's password"); err != nil {
					return nil, err
				}
			}
			return nil, signIn(c.jellyfinID, c.password)
		})
		if !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.jellyfinID, err, c.want)
		}
	}
	if open := h.f.open(); open != 0 {
		t.Errorf("sessions left: %d", open)
	}
	if current := h.Current(); current != nil {
		t.Errorf("an import started: %+v", current)
	}
}

// A user imports their own watch data: Polyfin signs in as them, never
// lists the server's users, which their session may not, and reads only
// theirs, into their own account, even from a server that answers every
// user with the session's data. A refused password starts nothing, and one
// import runs at a time, the administrator's included.
func TestAUserImportsOnlyTheirOwnWatchData(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.user(t, "alice"), h.user(t, "bob")
	h.f.users = []map[string]any{{"Id": "jf-alice", "Name": "Alice", "Policy": map[string]any{"IsAdministrator": true}}, {"Id": "jf-bob", "Name": "Bob"}}
	h.f.passwords = map[string]string{"Alice": "alice's password", "Bob": "bob's password"}
	h.f.items["jf-alice"] = map[string][]map[string]any{"IsPlayed": {movie("m1", "Alice's", map[string]string{"Imdb": "tt0000001"},
		map[string]any{"Played": true})}}
	h.f.items["jf-bob"] = map[string][]map[string]any{
		"IsPlayed":   {movie("m2", "Bob's", map[string]string{"Imdb": "tt0000002"}, map[string]any{"Played": true})},
		"IsFavorite": {movie("m3", "Bob's favorite", map[string]string{"Imdb": "tt0000003"}, map[string]any{"IsFavorite": true})},
	}
	h.f.ignoresUser, h.f.noMe, h.f.adminsOnly = true, true, true
	asBob := Connection{Kind: Jellyfin, Address: h.f.url, Credentials: Credentials{Name: "Bob", Password: "wrong"}}

	if _, err := h.ImportOwn(t.Context(), asBob, bob); !errors.Is(err, ErrSignInRefused) {
		t.Errorf("a wrong password: %v", err)
	}
	if current := h.Current(); current != nil {
		t.Errorf("a wrong password started an import: %+v", current)
	}
	asBob.Password = "bob's password"
	started, err := h.ImportOwn(t.Context(), asBob, bob)
	if err != nil {
		t.Fatal(err)
	}
	status := h.ended(t)
	if status.State != StateDone || !status.Own || status.StartedBy != bob.ID || len(status.Users) != 1 || status.Users[0].User != bob.ID ||
		status.Users[0].Played != 1 || status.Users[0].Favorites != 1 {
		t.Errorf("Bob's import: %+v", status)
	}
	aliceMovie, bobMovie, bobFavorite := library.TitleRef{IMDb: "tt0000001"}, library.TitleRef{IMDb: "tt0000002"}, library.TitleRef{IMDb: "tt0000003"}
	if !h.dataOf(t, bob.ID, bobMovie).Played || !h.dataOf(t, bob.ID, bobFavorite).Favorite || h.dataOf(t, bob.ID, aliceMovie).Played {
		t.Error("Bob's watch data")
	}
	for _, ref := range []library.TitleRef{aliceMovie, bobMovie, bobFavorite} {
		if d := h.dataOf(t, alice.ID, ref); d.Played || d.Favorite {
			t.Errorf("Alice was given %+v: %+v", ref, d)
		}
	}
	if own := h.Own(bob.ID); own == nil || own.ID != started.ID {
		t.Errorf("Bob's own import: %+v", own)
	}
	if own := h.Own(alice.ID); own != nil {
		t.Errorf("Alice sees %+v", own)
	}
	if open := h.f.open(); open != 0 {
		t.Errorf("sessions left: %d", open)
	}
	if _, err := h.ImportOwn(t.Context(), Connection{Kind: Plex, Address: h.f.url, Credentials: Credentials{Key: goodKey}}, bob); !errors.Is(err, ErrNotSignedIn) {
		t.Errorf("from Plex: %v", err)
	}

	// While the administrator's import runs, Bob waits.
	hold := make(chan struct{})
	h.f.mu.Lock()
	h.f.hold = hold
	h.f.mu.Unlock()
	if _, err := h.Start(t.Context(), to(h.f.url, Credentials{Key: goodKey}), accounts.User{}, func(Server, []User, SignIn) ([]Target, error) {
		return []Target{{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ImportOwn(t.Context(), asBob, bob); !errors.Is(err, ErrRunning) {
		t.Errorf("while another import runs: %v", err)
	}
	h.Stop()
	close(hold)
	h.ended(t)
	if own := h.Own(bob.ID); own == nil || own.ID != started.ID {
		t.Errorf("Bob's own import after the administrator's: %+v", own)
	}
}

// Emby takes its key in X-Emby-Token, and serves its API under /emby. It
// has no /Users/Me: one of its API keys is taken, but not a user's key,
// whose owner Polyfin could not tell, as some servers answer it with its
// owner's data whatever user is asked. Signed in, each user is read signed
// in as themselves.
func TestEmbyIsReadWithItsHeaderAndUnderItsPath(t *testing.T) {
	h := newHarness(t)
	h.f.emby = true
	alice, bob := h.user(t, "alice"), h.user(t, "bob")
	h.f.users = []map[string]any{{"Id": "jf-alice", "Name": "Alice", "Policy": map[string]any{"IsAdministrator": true}}, {"Id": "jf-bob", "Name": "Bob"}}
	h.f.passwords = map[string]string{"Alice": "alice's password", "Bob": "bob's password"}
	movies := map[string]string{"jf-alice": "tt0000001", "jf-bob": "tt0000002"}
	for user, imdb := range movies {
		h.f.items[user] = map[string][]map[string]any{"IsPlayed": {movie(user, user, map[string]string{"Imdb": imdb},
			map[string]any{"Played": true, "PlayCount": 3, "LastPlayedDate": "2026-09-01T20:00:00.0000000Z"})}}
	}
	h.f.ignoresUser = true
	h.f.userKeys["alices-own-key"] = "jf-alice"
	emby := func(credentials Credentials) Connection {
		return Connection{Kind: Emby, Address: h.f.url, Credentials: credentials}
	}

	server, users, err := h.Users(t.Context(), emby(Credentials{Key: goodKey}))
	if err != nil || server.Kind != Emby || server.Address != h.f.url+"/emby" || server.KeyOwner != "" || len(users) != 2 {
		t.Fatalf("with the server's key: %+v %v %v", server, users, err)
	}
	if _, _, err := h.Users(t.Context(), emby(Credentials{Key: "alices-own-key"})); !errors.Is(err, ErrUserKey) {
		t.Errorf("with Alice's own key: %v", err)
	}

	_, err = h.Start(t.Context(), emby(Credentials{Name: "Alice", Password: "alice's password"}), accounts.User{},
		func(server Server, _ []User, signIn SignIn) ([]Target, error) {
			if server.KeyOwner != "jf-alice" {
				t.Errorf("signed in as Alice: %+v", server)
			}
			if err := signIn("jf-bob", "bob's password"); err != nil {
				return nil, err
			}
			return []Target{{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}, {JellyfinID: "jf-bob", User: bob.ID, UserName: "bob"}}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if status := h.ended(t); status.State != StateDone || status.Kind != Emby {
		t.Errorf("import: %+v", status)
	}
	for jellyfinID, user := range map[string]accounts.User{"jf-alice": alice, "jf-bob": bob} {
		for other, imdb := range movies {
			d := h.dataOf(t, user.ID, library.TitleRef{IMDb: imdb})
			if d.Played != (other == jellyfinID) {
				t.Errorf("%s has %s's movie: %v", user.Name, other, d.Played)
			}
			// Emby tells the play count and date only when asked.
			if other == jellyfinID && (d.PlayCount != 3 || d.LastPlayed == nil || !d.LastPlayed.Equal(at("2026-09-01T20:00:00Z"))) {
				t.Errorf("%s's plays: %+v", user.Name, d)
			}
		}
	}
	if open := h.f.open(); open != 0 {
		t.Errorf("sessions left: %d", open)
	}
	for _, r := range h.f.sent() {
		if strings.Contains(r.URL.String(), goodKey) || strings.Contains(r.Header.Get("Authorization"), "Token") {
			t.Errorf("%s %s sent the key elsewhere than in X-Emby-Token", r.Method, r.URL.Path)
		}
	}
}

const plexToken = "plex-owners-token"

// fakePlex answers as a Plex Media Server does, in JSON, to its owner's
// token: its accounts, its library sections with their items and the
// owner's data of them, and each account's playback history, a page at a
// time.
type fakePlex struct {
	url string

	mu       sync.Mutex
	requests []*http.Request
	accounts []map[string]any
	sections []map[string]any
	// items are each section's items by type number ("1" movies, "2"
	// shows, "4" episodes), and history each account's plays, by id.
	items   map[string]map[string][]map[string]any
	history map[string][]map[string]any
}

func newFakePlex(t *testing.T) *fakePlex {
	t.Helper()
	f := &fakePlex{items: map[string]map[string][]map[string]any{}, history: map[string][]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

func (f *fakePlex) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Clone(r.Context()))
	// Plex answers XML unless asked for JSON.
	if r.Header.Get("Accept") != "application/json" {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<MediaContainer size="0"/>`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	answer := func(container map[string]any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": container})
	}
	if r.URL.Path == "/identity" {
		answer(map[string]any{"machineIdentifier": "0a1b2c", "version": "1.42.2.10156"})
		return
	}
	if r.Header.Get("X-Plex-Token") != plexToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	query := r.URL.Query()
	// page answers the part of all the paging asks for.
	page := func(all []map[string]any) {
		start, _ := strconv.Atoi(query.Get("X-Plex-Container-Start"))
		size, err := strconv.Atoi(query.Get("X-Plex-Container-Size"))
		if err != nil {
			size = len(all)
		}
		part := all[min(start, len(all)):min(start+size, len(all))]
		answer(map[string]any{"size": len(part), "totalSize": len(all), "offset": start, "Metadata": part})
	}
	section, inSection := strings.CutPrefix(r.URL.Path, "/library/sections/")
	switch {
	case r.URL.Path == "/":
		answer(map[string]any{"friendlyName": "Living room", "version": "1.42.2.10156"})
	case r.URL.Path == "/accounts":
		answer(map[string]any{"size": len(f.accounts), "Account": f.accounts})
	case r.URL.Path == "/library/sections":
		answer(map[string]any{"size": len(f.sections), "Directory": f.sections})
	case inSection && strings.HasSuffix(section, "/all"):
		items := f.items[strings.TrimSuffix(section, "/all")][query.Get("type")]
		if query.Get("includeGuids") != "1" {
			// Plex leaves the identifiers out unless asked.
			var bare []map[string]any
			for _, item := range items {
				copied := maps.Clone(item)
				delete(copied, "Guid")
				bare = append(bare, copied)
			}
			items = bare
		}
		page(items)
	case r.URL.Path == "/status/sessions/history/all":
		page(f.history[query.Get("accountID")])
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// guids are the Guid list of a Plex item.
func guids(ids ...string) []map[string]any {
	list := []map[string]any{}
	for _, id := range ids {
		list = append(list, map[string]any{"id": id})
	}
	return list
}

// The owner's watch data comes from the library sections: played movies
// and episodes with their play counts and dates, and resume points.
// Another account's comes from its playback history: its plays, counted
// and dated. Episodes are found by their show's identifiers and their
// numbers, and Plex is only read.
func TestPlexImportsTheOwnersAndEachAccountsWatchData(t *testing.T) {
	h := newHarness(t)
	p := newFakePlex(t)
	owner, carol := h.user(t, "owner"), h.user(t, "carol")
	// A catalog listed a movie by TMDB, and a series by TVDB with its
	// first episode.
	h.listed(t, "movie", "movie|tmdb:550", `{"meta":{"id":"tmdb:550","type":"movie","name":"Listed by TMDB"}}`)
	h.listed(t, "series", "series|tvdb:81189", `{"meta":{"id":"tvdb:81189","type":"series","name":"Listed by TVDB"}}`)
	h.listed(t, "episode", "episode|tvdb:81189:1:1",
		`{"seriesId":"tvdb:81189","video":{"id":"tvdb:81189:1:1","season":1,"episode":1}}`)
	unix := func(text string) int64 { return at(text).Unix() }
	p.accounts = []map[string]any{{"id": 0, "key": "/accounts/0", "name": ""}, {"id": 5, "key": "/accounts/5", "name": "Carol"},
		{"id": 1, "key": "/accounts/1", "name": "Owner"}}
	p.sections = []map[string]any{{"key": "1", "type": "movie", "title": "Movies"}, {"key": "2", "type": "show", "title": "Shows"},
		{"key": "3", "type": "artist", "title": "Music"}}
	p.items["1"] = map[string][]map[string]any{"1": {
		{"ratingKey": "11", "type": "movie", "title": "Twice seen", "year": 2019, "Guid": guids("imdb://tt0000001", "tmdb://10"),
			"viewCount": 2, "lastViewedAt": unix("2026-09-01T20:00:00Z"), "duration": 6000000},
		{"ratingKey": "12", "type": "movie", "title": "Half seen", "year": 2020, "Guid": guids("tmdb://550"),
			"viewOffset": 1800000, "lastViewedAt": unix("2026-09-02T20:00:00Z"), "duration": 6000000},
		{"ratingKey": "13", "type": "movie", "title": "Holiday", "viewCount": 1, "lastViewedAt": unix("2026-09-03T20:00:00Z")},
		{"ratingKey": "14", "type": "movie", "title": "Carol's", "year": 2021, "Guid": guids("imdb://tt0000004"), "duration": 6000000},
	}}
	// The show's identifiers, never the episodes' own, find its episodes.
	p.items["2"] = map[string][]map[string]any{
		"2": {{"ratingKey": "20", "type": "show", "title": "A show", "Guid": guids("tvdb://81189", "imdb://tt0100")}},
		"4": {
			{"ratingKey": "21", "type": "episode", "title": "Pilot", "grandparentRatingKey": "20", "grandparentTitle": "A show",
				"parentIndex": 1, "index": 1, "Guid": guids("imdb://tt0900001", "tvdb://900001"), "viewCount": 1, "lastViewedAt": unix("2026-09-04T20:00:00Z")},
			{"ratingKey": "22", "type": "episode", "title": "Second", "grandparentRatingKey": "20", "grandparentTitle": "A show",
				"parentIndex": 1, "index": 2, "Guid": guids("imdb://tt0900002"), "viewOffset": 600000, "duration": 3000000,
				"lastViewedAt": unix("2026-09-05T20:00:00Z")},
		},
	}
	p.history["5"] = []map[string]any{
		{"historyKey": "/status/sessions/history/3", "ratingKey": "14", "key": "/library/metadata/14", "type": "movie", "title": "Carol's",
			"viewedAt": unix("2026-09-08T20:00:00Z"), "accountID": 5},
		{"historyKey": "/status/sessions/history/2", "ratingKey": "21", "key": "/library/metadata/21", "type": "episode", "title": "Pilot",
			"grandparentKey": "/library/metadata/20", "grandparentTitle": "A show", "parentIndex": 1, "index": 1,
			"viewedAt": unix("2026-09-07T20:00:00Z"), "accountID": 5},
		{"historyKey": "/status/sessions/history/1", "ratingKey": "14", "key": "/library/metadata/14", "type": "movie", "title": "Carol's",
			"viewedAt": unix("2026-09-06T20:00:00Z"), "accountID": 5},
		{"historyKey": "/status/sessions/history/0", "ratingKey": "31", "key": "/library/metadata/31", "type": "track", "title": "A song",
			"viewedAt": unix("2026-09-05T20:00:00Z"), "accountID": 5},
	}
	plex := Connection{Kind: Plex, Address: p.url, Credentials: Credentials{Key: plexToken}}

	server, users, err := h.Users(t.Context(), plex)
	if err != nil || server.Name != "Living room" || server.KeyOwner != "" || len(users) != 2 || users[0].ID != "1" || !users[0].Administrator ||
		users[1].ID != "5" || users[1].Name != "Carol" || users[1].Administrator {
		t.Fatalf("users: %+v %+v %v", server, users, err)
	}
	if _, _, err := h.Users(t.Context(), Connection{Kind: Plex, Address: p.url, Credentials: Credentials{Key: "wrong"}}); !errors.Is(err, ErrKeyRefused) {
		t.Errorf("a wrong token: %v", err)
	}
	if _, err := h.Start(t.Context(), plex, accounts.User{}, func(Server, []User, SignIn) ([]Target, error) {
		return []Target{{JellyfinID: "1", User: owner.ID, UserName: "owner"}, {JellyfinID: "5", User: carol.ID, UserName: "carol"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	status := h.ended(t)
	if status.State != StateDone || status.Kind != Plex {
		t.Fatalf("import: %+v", status)
	}
	// The owner: two movies and an episode played (the home video is not
	// found), a movie and an episode resumed.
	if got := status.Users[0]; got.Played != 2 || got.Resumed != 2 || got.UnmatchedCount != 1 || got.Unmatched[0].Name != "Holiday" ||
		got.Unmatched[0].Reason != ReasonNoIdentifier {
		t.Errorf("owner: %+v", got)
	}
	if got := status.Users[1]; got.Played != 2 || got.Resumed != 0 || got.UnmatchedCount != 0 {
		t.Errorf("Carol: %+v", got)
	}
	check := func(name string, user accounts.ID, ref library.TitleRef, want func(userdata.Data) bool) {
		t.Helper()
		if d := h.dataOf(t, user, ref); !want(d) {
			t.Errorf("%s: %+v", name, d)
		}
	}
	twice, half, carols := library.TitleRef{IMDb: "tt0000001"}, library.TitleRef{TMDB: 550}, library.TitleRef{IMDb: "tt0000004"}
	pilot, second := library.TitleRef{Episode: true, TVDB: 81189, Season: 1, Number: 1}, library.TitleRef{Episode: true, TVDB: 81189, Season: 1, Number: 2}
	check("owner's movie seen twice", owner.ID, twice, func(d userdata.Data) bool {
		return d.Played && d.PlayCount == 2 && d.LastPlayed.Equal(at("2026-09-01T20:00:00Z"))
	})
	check("owner's movie half seen", owner.ID, half, func(d userdata.Data) bool {
		return !d.Played && d.Position == 30*time.Minute && d.Runtime == 100*time.Minute && d.LastPlayed.Equal(at("2026-09-02T20:00:00Z"))
	})
	check("owner's pilot", owner.ID, pilot, func(d userdata.Data) bool { return d.Played && d.LastPlayed.Equal(at("2026-09-04T20:00:00Z")) })
	check("owner's second episode", owner.ID, second, func(d userdata.Data) bool { return d.Position == 10*time.Minute })
	check("Carol's movie, for the owner", owner.ID, carols, func(d userdata.Data) bool { return !d.Played })
	check("Carol's movie", carol.ID, carols, func(d userdata.Data) bool {
		return d.Played && d.PlayCount == 2 && d.LastPlayed.Equal(at("2026-09-08T20:00:00Z"))
	})
	check("Carol's pilot", carol.ID, pilot, func(d userdata.Data) bool { return d.Played && d.LastPlayed.Equal(at("2026-09-07T20:00:00Z")) })
	check("the owner's movies, for Carol", carol.ID, twice, func(d userdata.Data) bool { return !d.Played })
	check("the owner's resume point, for Carol", carol.ID, half, func(d userdata.Data) bool { return d.Position == 0 })

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.requests {
		if r.Method != http.MethodGet || strings.Contains(r.URL.String(), plexToken) {
			t.Errorf("Plex was sent %s %s", r.Method, r.URL)
		}
	}
}

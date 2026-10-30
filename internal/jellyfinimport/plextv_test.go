package jellyfinimport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// fakePlexTV answers as plex.tv and Plex's Discover service do the
// requests of a sign-in to Plex: a PIN, claimed once claimed is set, or
// forgotten once expired; who a token is, their Plex Home and the switch
// to its profiles; the servers each token reaches; and each token's
// Watchlist, whose listing leaves the identifiers out, as metadata gives
// them item by item.
type fakePlexTV struct {
	url string

	mu        sync.Mutex
	claimed   string
	expired   bool
	expiresIn int
	clients   []string
	users     map[string]map[string]any
	home      []map[string]any
	pins      map[string]string
	switched  map[string]string
	resources map[string][]map[string]any
	watchlist map[string][]map[string]any
	metadata  map[string]map[string]any
	signedOut []string
}

func newFakePlexTV(t *testing.T) *fakePlexTV {
	t.Helper()
	f := &fakePlexTV{expiresIn: 900, users: map[string]map[string]any{}, pins: map[string]string{}, switched: map[string]string{},
		resources: map[string][]map[string]any{}, watchlist: map[string][]map[string]any{}, metadata: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

func (f *fakePlexTV) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clients = append(f.clients, r.Header.Get("X-Plex-Client-Identifier"))
	w.Header().Set("Content-Type", "application/json")
	answer := func(status int, body any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
	token := r.Header.Get("X-Plex-Token")
	known := token != "" && (f.users[token] != nil || f.resources[token] != nil)
	query := r.URL.Query()
	switch path := r.URL.Path; {
	case r.Method == http.MethodPost && path == "/api/v2/pins":
		if query.Get("strong") != "true" {
			answer(http.StatusBadRequest, nil)
			return
		}
		answer(http.StatusCreated, map[string]any{"id": 4242, "code": "c0de", "expiresIn": f.expiresIn, "authToken": nil})
	case path == "/api/v2/pins/4242":
		if f.expired {
			answer(http.StatusNotFound, nil)
			return
		}
		var claimed any
		if f.claimed != "" {
			claimed = f.claimed
		}
		answer(http.StatusOK, map[string]any{"id": 4242, "code": "c0de", "authToken": claimed})
	case !known:
		answer(http.StatusUnauthorized, nil)
	case path == "/api/v2/user":
		answer(http.StatusOK, f.users[token])
	case path == "/api/v2/home/users":
		if f.users[token]["home"] != true {
			answer(http.StatusForbidden, nil)
			return
		}
		answer(http.StatusOK, map[string]any{"users": f.home})
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/api/v2/home/users/") && strings.HasSuffix(path, "/switch"):
		profile := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v2/home/users/"), "/switch")
		if pin := f.pins[profile]; pin != "" && query.Get("pin") != pin {
			answer(http.StatusUnauthorized, nil)
			return
		}
		answer(http.StatusCreated, map[string]any{"authToken": f.switched[profile]})
	case path == "/api/v2/resources":
		if query.Get("includeHttps") != "1" || query.Get("includeRelay") != "1" {
			answer(http.StatusBadRequest, nil)
			return
		}
		answer(http.StatusOK, f.resources[token])
	case r.Method == http.MethodDelete && path == "/api/v2/users/signout":
		f.signedOut = append(f.signedOut, token)
		w.WriteHeader(http.StatusNoContent)
	case path == "/library/sections/watchlist/all":
		all := f.watchlist[token]
		start, _ := strconv.Atoi(query.Get("X-Plex-Container-Start"))
		size, _ := strconv.Atoi(query.Get("X-Plex-Container-Size"))
		if size > 100 {
			// Discover refuses larger pages.
			answer(http.StatusBadRequest, nil)
			return
		}
		part := all[min(start, len(all)):min(start+size, len(all))]
		answer(http.StatusOK, map[string]any{"MediaContainer": map[string]any{"size": len(part), "totalSize": len(all), "offset": start,
			"Metadata": part}})
	case strings.HasPrefix(path, "/library/metadata/"):
		item, ok := f.metadata[strings.TrimPrefix(path, "/library/metadata/")]
		if !ok {
			answer(http.StatusNotFound, nil)
			return
		}
		answer(http.StatusOK, map[string]any{"MediaContainer": map[string]any{"size": 1, "Metadata": []map[string]any{item}}})
	default:
		answer(http.StatusNotFound, nil)
	}
}

// claim has Plex sign token in once the PIN is checked.
func (f *fakePlexTV) claim(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimed = token
}

// out lists the tokens signed out of Plex.
func (f *fakePlexTV) out() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.signedOut)
}

// fakeClock is a clock tests move on.
type fakeClock struct{ at atomic.Int64 }

func (c *fakeClock) now() time.Time      { return time.Unix(0, c.at.Load()) }
func (c *fakeClock) add(d time.Duration) { c.at.Add(int64(d)) }
func newClock(at time.Time) *fakeClock   { c := &fakeClock{}; c.at.Store(at.UnixNano()); return c }
func plexServer(token string, connections ...map[string]any) map[string]any {
	return map[string]any{"name": "Living room", "clientIdentifier": "0a1b2c", "provides": "server", "owned": false,
		"accessToken": token, "connections": connections}
}

// plexSigningIn is a harness whose import signs in to Plex at tv, on a
// clock tests move on, with a watchlist, and its log.
func plexSigningIn(t *testing.T, tv *fakePlexTV) (harness, *fakeClock, *lockedBuffer) {
	t.Helper()
	h := newHarness(t)
	logged := &lockedBuffer{}
	h.logger = slog.New(slog.NewTextHandler(logged, nil))
	clock := newClock(at("2026-10-30T12:00:00Z"))
	h.now = clock.now
	h.plexTV = PlexTV{API: tv.url, SignIn: tv.url + "/auth", Discover: tv.url}
	h.Service.watchlist = watchlist.New(h.pool)
	return h, clock, logged
}

// signedIn starts a sign-in to Plex for user, has Plex sign them in with
// the account's token, and answers the account.
func (h harness) signedIn(t *testing.T, tv *fakePlexTV, clock *fakeClock, user accounts.User) PlexAccount {
	t.Helper()
	link, err := h.PlexSignIn(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link.URL, tv.url+"/auth#?") || !strings.Contains(link.URL, "clientID="+h.device) ||
		!strings.Contains(link.URL, "code=c0de") || !strings.Contains(link.URL, "context%5Bdevice%5D%5Bproduct%5D=Polyfin") {
		t.Errorf("link: %s", link.URL)
	}
	if _, err := h.PlexAccount(t.Context(), user.ID); !errors.Is(err, ErrPlexSignInPending) {
		t.Fatalf("before Plex signs in: %v", err)
	}
	tv.claim("account-token")
	// plex.tv is asked once a second at most.
	if _, err := h.PlexAccount(t.Context(), user.ID); !errors.Is(err, ErrPlexSignInPending) {
		t.Fatalf("within the second: %v", err)
	}
	clock.add(time.Second)
	account, err := h.PlexAccount(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

// A user who signs in to Plex imports their own watch data from a server
// a friend shares: what they played, marked or synced as played, read with
// their own token for the server, never the owner's, their resume points,
// their ratings, and their Watchlist, whose items' identifiers are asked
// one by one. The server answers at the first of its addresses that names
// it; every token is signed out of Plex once the import ends, and none is
// logged or answered.
func TestAUserSignedInToPlexImportsTheirOwnWatchData(t *testing.T) {
	tv := newFakePlexTV(t)
	h, clock, logged := plexSigningIn(t, tv)
	p := smallPlex(t)
	impostor := newFakePlex(t)
	impostor.machine = "ffffff"
	friend := h.user(t, "friend")
	viewed := at("2026-09-01T20:00:00Z").Unix()
	// The owner played the second movie three times; the friend did not.
	p.items["1"]["1"][1]["viewCount"] = 3
	p.states["friend-server-token"] = map[string]map[string]any{
		"11": {"viewCount": 1, "lastViewedAt": viewed},
		"12": {"viewOffset": 1800000, "lastViewedAt": viewed},
		"20": {"userRating": 8.0},
		"21": {"viewCount": 1, "lastViewedAt": viewed},
	}
	tv.users["account-token"] = map[string]any{"uuid": "u-friend", "username": "friend", "title": "Friend"}
	tv.resources["account-token"] = []map[string]any{
		{"name": "Phone", "clientIdentifier": "phone", "provides": "player", "accessToken": "phone-token"},
		plexServer("friend-server-token", map[string]any{"uri": p.url, "local": false}, map[string]any{"uri": "ftp://nowhere", "local": true},
			map[string]any{"uri": impostor.url, "local": true}),
	}
	// A title started is left out of the Watchlist view: these are not.
	tv.watchlist["account-token"] = []map[string]any{
		{"ratingKey": "w1", "type": "movie", "title": "Wanted", "guid": "plex://movie/w1"},
		{"ratingKey": "w2", "type": "show", "title": "Another show", "guid": "plex://show/w2", "Guid": guids("imdb://tt0200")},
		{"ratingKey": "w3", "type": "movie", "title": "Unknown", "guid": "plex://movie/w3"},
	}
	tv.metadata["w1"] = map[string]any{"ratingKey": "w1", "type": "movie", "title": "Wanted", "year": 2024, "Guid": guids("imdb://tt0000013")}

	account := h.signedIn(t, tv, clock, friend)
	if account.Name != "Friend" || len(account.Profiles) != 0 || len(account.Servers) != 1 || account.Servers[0].ID != "0a1b2c" ||
		account.Servers[0].Owned {
		t.Fatalf("account: %+v", account)
	}
	if _, err := h.ImportOwnPlex(t.Context(), friend, "elsewhere", true); !errors.Is(err, ErrPlexUnknownServer) {
		t.Errorf("another server: %v", err)
	}
	snapshot, err := h.ImportOwnPlex(t.Context(), friend, "0a1b2c", true)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Own || snapshot.Address != p.url || snapshot.ServerName != "Living room" {
		t.Errorf("import: %+v", snapshot)
	}
	status := h.ended(t)
	got := status.Users[0]
	if status.State != StateDone || got.JellyfinName != "Friend" || got.Played != 2 || got.Resumed != 1 || got.Ratings != 1 ||
		got.Watchlist != 2 || got.UnmatchedCount != 1 || got.Unmatched[0].Name != "Unknown" || got.Unmatched[0].Reason != ReasonNoIdentifier ||
		got.NothingAdded != "" {
		t.Fatalf("import: %+v", status)
	}
	check := func(name string, ref library.TitleRef, want func(d userDataView) bool) {
		t.Helper()
		d := h.dataOf(t, friend.ID, ref)
		if !want(userDataView{played: d.Played, position: d.Position, rating: d.Rating}) {
			t.Errorf("%s: %+v", name, d)
		}
	}
	check("marked played", library.TitleRef{IMDb: "tt0000011"}, func(d userDataView) bool { return d.played })
	check("the owner's play", library.TitleRef{IMDb: "tt0000012"}, func(d userDataView) bool { return !d.played && d.position == 30*time.Minute })
	check("synced episode", library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 1}, func(d userDataView) bool { return d.played })
	check("rated show", library.TitleRef{Series: true, IMDb: "tt0100"}, func(d userDataView) bool { return d.rating != nil && *d.rating == 8 })
	titles, err := h.Service.watchlist.Titles(t.Context(), friend.ID)
	if err != nil || len(titles) != 2 || !slices.Contains(titles, h.idOf(t, library.TitleRef{IMDb: "tt0000013"}).ID) {
		t.Errorf("watchlist: %v %v", titles, err)
	}

	if out := tv.out(); !slices.Equal(out, []string{"account-token"}) {
		t.Errorf("signed out: %v", out)
	}
	if _, err := h.PlexAccount(t.Context(), friend.ID); !errors.Is(err, ErrPlexSignInEnded) {
		t.Errorf("after the import: %v", err)
	}
	for _, client := range tv.clients {
		if client != h.device {
			t.Errorf("plex.tv was told client %q", client)
		}
	}
	answered, _ := json.Marshal(struct {
		Account PlexAccount
		Status  Status
	}{account, status})
	for _, token := range []string{"account-token", "friend-server-token", "phone-token"} {
		if strings.Contains(logged.String(), token) || strings.Contains(string(answered), token) {
			t.Errorf("%s logged or answered: %s %s", token, logged.String(), answered)
		}
	}
	if !strings.Contains(logged.String(), "signedIn=true") {
		t.Errorf("logged: %s", logged.String())
	}
}

// userDataView is what a test checks of a title's data.
type userDataView struct {
	played   bool
	position time.Duration
	rating   *float64
}

// A PIN Plex forgot, or one past its expiry, ends the sign-in: nothing can
// be imported with it.
func TestAnExpiredPlexSignInIsRefused(t *testing.T) {
	tv := newFakePlexTV(t)
	h, clock, _ := plexSigningIn(t, tv)
	user := h.user(t, "friend")
	if _, err := h.PlexSignIn(t.Context(), user.ID); err != nil {
		t.Fatal(err)
	}
	tv.mu.Lock()
	tv.expired = true
	tv.mu.Unlock()
	if _, err := h.PlexAccount(t.Context(), user.ID); !errors.Is(err, ErrPlexSignInEnded) {
		t.Errorf("forgotten: %v", err)
	}
	if _, err := h.ImportOwnPlex(t.Context(), user, "0a1b2c", true); !errors.Is(err, ErrPlexSignInEnded) {
		t.Errorf("import: %v", err)
	}

	tv.mu.Lock()
	tv.expired, tv.expiresIn = false, 60
	tv.mu.Unlock()
	if _, err := h.PlexSignIn(t.Context(), user.ID); err != nil {
		t.Fatal(err)
	}
	clock.add(61 * time.Second)
	if _, err := h.PlexAccount(t.Context(), user.ID); !errors.Is(err, ErrPlexSignInEnded) {
		t.Errorf("past its expiry: %v", err)
	}
}

// A server is reached only at an address that answers with its machine
// identifier: one that answers as another server is not it. The sign-in
// is kept, and the account read from it with nothing played says so.
func TestAPlexServerMustAnswerAsItself(t *testing.T) {
	tv := newFakePlexTV(t)
	h, clock, _ := plexSigningIn(t, tv)
	p := smallPlex(t)
	impostor := newFakePlex(t)
	impostor.machine = "ffffff"
	impostor.states["friend-server-token"] = map[string]map[string]any{}
	p.states["friend-server-token"] = map[string]map[string]any{}
	user := h.user(t, "friend")
	tv.users["account-token"] = map[string]any{"uuid": "u-friend", "title": "Friend"}
	tv.resources["account-token"] = []map[string]any{plexServer("friend-server-token", map[string]any{"uri": impostor.url, "local": true})}
	h.signedIn(t, tv, clock, user)
	if _, err := h.ImportOwnPlex(t.Context(), user, "0a1b2c", false); !errors.Is(err, ErrPlexServerUnreachable) {
		t.Fatalf("an impostor: %v", err)
	}
	if len(impostor.requests) != 1 || impostor.requests[0].URL.Path != "/identity" || impostor.requests[0].Header.Get("X-Plex-Token") != "" {
		t.Errorf("the impostor was asked: %v", impostor.requests)
	}
	if account, err := h.PlexAccount(t.Context(), user.ID); err != nil || len(account.Servers) != 1 {
		t.Errorf("the sign-in is kept: %+v %v", account, err)
	}
	if out := tv.out(); len(out) != 0 {
		t.Errorf("signed out: %v", out)
	}
}

// An account signed in to Plex that has nothing played, in progress or
// rated on the server says so; a relayed address is used when it is the
// only one.
func TestAPlexAccountWithNothingWatchedSaysSo(t *testing.T) {
	tv := newFakePlexTV(t)
	h, clock, _ := plexSigningIn(t, tv)
	p := smallPlex(t)
	p.states["friend-server-token"] = map[string]map[string]any{}
	user := h.user(t, "friend")
	tv.users["account-token"] = map[string]any{"uuid": "u-friend", "title": "Friend"}
	tv.resources["account-token"] = []map[string]any{plexServer("friend-server-token", map[string]any{"uri": p.url, "relay": true})}
	h.signedIn(t, tv, clock, user)
	if _, err := h.ImportOwnPlex(t.Context(), user, "0a1b2c", false); err != nil {
		t.Fatal(err)
	}
	if got := h.ended(t).Users[0]; got.NothingAdded != NothingPlexOwn || got.Sections != 2 || got.Listed != 4 {
		t.Errorf("friend: %+v", got)
	}
}

// A managed profile of a Plex Home has no sign-in of its own: the Home's
// member signs in, then switches to it, with its PIN, as Plex's apps do.
// The import then reads the profile's own data, with its own token.
func TestAPlexHomeProfileIsImportedWithItsOwnToken(t *testing.T) {
	tv := newFakePlexTV(t)
	h, clock, _ := plexSigningIn(t, tv)
	p := smallPlex(t)
	kid := h.user(t, "kid")
	p.states["kid-server-token"] = map[string]map[string]any{"11": {"viewCount": 1, "lastViewedAt": at("2026-09-01T20:00:00Z").Unix()}}
	tv.users["account-token"] = map[string]any{"uuid": "u-parent", "title": "Parent", "home": true}
	tv.home = []map[string]any{
		{"uuid": "u-kid", "title": "Kid", "restricted": true, "protected": true},
		{"uuid": "u-parent", "title": "Parent", "restricted": false},
	}
	tv.pins["u-kid"] = "1234"
	tv.switched["u-kid"] = "kid-token"
	tv.resources["account-token"] = []map[string]any{}
	tv.resources["kid-token"] = []map[string]any{plexServer("kid-server-token", map[string]any{"uri": p.url, "local": true})}

	account := h.signedIn(t, tv, clock, kid)
	if len(account.Profiles) != 2 || account.Profiles[0].ID != "u-parent" || account.Profile != "u-parent" ||
		!account.Profiles[1].Managed || !account.Profiles[1].Protected || len(account.Servers) != 0 {
		t.Fatalf("account: %+v", account)
	}
	if _, err := h.PlexProfile(t.Context(), kid.ID, "u-nobody", ""); !errors.Is(err, ErrPlexUnknownProfile) {
		t.Errorf("no such profile: %v", err)
	}
	if _, err := h.PlexProfile(t.Context(), kid.ID, "u-kid", "0000"); !errors.Is(err, ErrPlexProfilePIN) {
		t.Errorf("a wrong PIN: %v", err)
	}
	account, err := h.PlexProfile(t.Context(), kid.ID, "u-kid", "1234")
	if err != nil || account.Profile != "u-kid" || len(account.Servers) != 1 {
		t.Fatalf("Kid: %+v %v", account, err)
	}
	if _, err := h.ImportOwnPlex(t.Context(), kid, "0a1b2c", true); err != nil {
		t.Fatal(err)
	}
	status := h.ended(t)
	if got := status.Users[0]; got.JellyfinName != "Kid" || got.Played != 1 {
		t.Errorf("Kid: %+v", status)
	}
	if out := tv.out(); !slices.Contains(out, "kid-token") || !slices.Contains(out, "account-token") {
		t.Errorf("signed out: %v", out)
	}
}

// The owner's ratings come with the administrator's import, read with the
// owner's token: a movie's, a show's, which is the series', and an
// episode's. A rating Polyfin has stays.
func TestTheOwnersPlexRatingsComeWithTheAdministratorsImport(t *testing.T) {
	h := newHarness(t)
	p := smallPlex(t)
	owner := h.user(t, "owner")
	p.items["1"]["1"][0]["userRating"] = 8.0
	p.items["2"]["2"][0]["userRating"] = 6.0
	p.items["2"]["4"][0]["userRating"] = 10.0
	movie := library.TitleRef{IMDb: "tt0000011"}
	if _, err := h.data.Change(t.Context(), owner.ID, []userdata.Item{h.idOf(t, movie)}, func(d *userdata.Data) { d.Rating = new(4.0) }); err != nil {
		t.Fatal(err)
	}
	got := h.plexImported(t, p, "1", owner)
	if got.Ratings != 2 || got.UpToDate != 1 || got.NothingAdded != "" {
		t.Errorf("owner: %+v", got)
	}
	for name, want := range map[string]struct {
		ref    library.TitleRef
		rating float64
	}{"movie": {movie, 4}, "series": {library.TitleRef{Series: true, IMDb: "tt0100"}, 6},
		"episode": {library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 1}, 10}} {
		if d := h.dataOf(t, owner.ID, want.ref); d.Rating == nil || *d.Rating != want.rating {
			t.Errorf("%s: %+v", name, d)
		}
	}
}

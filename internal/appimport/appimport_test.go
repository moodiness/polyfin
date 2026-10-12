package appimport

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/userdata"
)

type harness struct {
	*Service
	pool   *pgxpool.Pool
	store  *accounts.Store
	lib    *library.Service
	data   *userdata.Store
	addons *addons.Store
	// personalAddons is the server's switch of users' own addons.
	personalAddons *atomic.Bool
}

// newHarness is a service on a fresh database; configure, if any,
// completes its options.
func newHarness(t *testing.T, configure ...func(*Options)) harness {
	t.Helper()
	pool := migrated.New(t)
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := stremio.NewClient("test")
	addonStore := addons.New(pool, client)
	lib := library.New(pool, addonStore, client, logger, store.Settings)
	data := userdata.New(pool)
	personalAddons := &atomic.Bool{}
	personalAddons.Store(true)
	settings := func() accounts.Settings {
		s := store.Settings()
		s.PersonalAddons = personalAddons.Load()
		return s
	}
	options := Options{Settings: settings, Titles: lib, UserData: data, Addons: addonStore, Logger: logger, DB: pool}
	for _, c := range configure {
		c(&options)
	}
	s := New(options)
	t.Cleanup(s.Close)
	return harness{Service: s, pool: pool, store: store, lib: lib, data: data, addons: addonStore, personalAddons: personalAddons}
}

func (h harness) user(t *testing.T, name string, administrator bool) accounts.User {
	t.Helper()
	user, err := h.store.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: "correct horse", IsAdministrator: administrator})
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

// idOf is the item ref designates, the first when several.
func (h harness) idOf(t *testing.T, ref library.TitleRef) userdata.Item {
	t.Helper()
	found, err := h.lib.Resolve(t.Context(), []library.TitleRef{ref})
	if err != nil || len(found[0]) == 0 {
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

// imported holds account for by, imports choice of it into user's
// account, and waits for the import to end.
func (h harness) imported(t *testing.T, by, user accounts.User, account Account, choice Choice) Status {
	t.Helper()
	if _, err := h.Start(by, user, h.Hold(by.ID, account), choice); err != nil {
		t.Fatal(err)
	}
	return h.ended(t, user.ID)
}

// ended waits for the import into user's account to end.
func (h harness) ended(t *testing.T, user accounts.ID) Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if status := h.Current(user); status != nil && status.State != StateRunning {
			return *status
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %+v", h.Current(user))
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

// ref is the title an app names by id, failing the test when it is not
// one Polyfin finds titles by.
func ref(t *testing.T, id string, movie bool, season, episode int) library.TitleRef {
	t.Helper()
	r, ok := TitleRef(nil, id, movie, season, episode)
	if !ok {
		t.Fatalf("%s is not matched", id)
	}
	return r
}

func TestEntriesAreMatchedAndOnlyAddedTo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.user(t, "alice", false)
	// A catalog listed a movie by its TMDB identifier, and an anime
	// catalog an entry by its Kitsu identifier with its third episode.
	h.listed(t, "movie", "movie|tmdb:550", `{"meta":{"id":"tmdb:550","type":"movie","name":"Listed by TMDB"}}`)
	h.listed(t, "series", "series|kitsu:12", `{"meta":{"id":"kitsu:12","type":"series","name":"Anime"}}`)
	h.listed(t, "episode", "episode|kitsu:12:3", `{"seriesId":"kitsu:12","season":1,"video":{"id":"kitsu:12:3","season":1,"episode":3}}`)

	byIMDb, byTMDB := ref(t, "tt0000001", true, 0, 0), ref(t, "tmdb:550", true, 0, 0)
	resumed, newerHere, nearlyDone := ref(t, "tt0000002", true, 0, 0), ref(t, "tt0000003", true, 0, 0), ref(t, "tt0000004", true, 0, 0)
	episode, series := ref(t, "tt0100", false, 1, 2), ref(t, "tt0100", false, 0, 0)
	animeEpisode := ref(t, "kitsu:12", false, 1, 3)

	// Polyfin already counts five plays of the first movie, the latest
	// after the app's, and keeps a newer resume point of another.
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

	resume := func(position, runtime time.Duration, when string) *Resume {
		return &Resume{Position: position, Runtime: runtime, At: at(when)}
	}
	entries := []Entry{
		{Title: byIMDb, Played: true, PlayedAt: new(at("2026-09-01T20:00:00Z")), Plays: 3},
		{Title: byTMDB, Played: true, PlayedAt: new(at("2026-09-02T20:00:00Z")), Plays: 2, Favorite: true},
		{Title: episode, Played: true, PlayedAt: new(at("2026-09-04T20:00:00Z"))},
		{Title: animeEpisode, Played: true},
		{Title: resumed, Resume: resume(30*time.Minute, 100*time.Minute, "2026-10-01T20:00:00Z")},
		{Title: newerHere, Resume: resume(30*time.Minute, 100*time.Minute, "2026-10-01T20:00:00Z")},
		// Past the played threshold: played.
		{Title: nearlyDone, Resume: resume(95*time.Minute, 100*time.Minute, "2026-10-02T20:00:00Z")},
		// A series whose episodes played the source could not read: it
		// is reported, and a favorite still.
		{Title: series, Label: Label{ID: "tt0100", Name: "Series", Kind: "series"}, Favorite: true, Problem: ReasonEpisodeList},
		{Title: library.TitleRef{}, Label: Label{ID: "local:home-video", Name: "Holiday", Kind: "movie"}, Played: true},
		{Title: ref(t, "tmdb:999999", true, 0, 0), Label: Label{ID: "tmdb:999999", Name: "Unknown", Kind: "movie"}, Favorite: true},
	}
	account := Account{App: "test", Profiles: []Profile{{ID: "1", Name: "Main", Entries: entries}}}

	status := h.imported(t, alice, alice, account, Choice{Profile: "1"})
	if status.State != StateDone || status.App != "test" || status.Profile != "Main" || status.User != alice.ID || status.StartedBy != alice.ID {
		t.Fatalf("%+v", status)
	}
	// Newly played: by TMDB, the episode, the anime's episode, and the
	// movie past the threshold; the movie by IMDb was played already.
	if status.Read != 10 || status.Played != 4 || status.Resumed != 1 || status.Favorites != 2 || status.UnmatchedCount != 3 {
		t.Errorf("counts: %+v", status)
	}
	reasons := map[string]string{}
	for _, u := range status.Unmatched {
		reasons[u.Name] = u.Reason
	}
	if len(reasons) != 3 || reasons["Holiday"] != ReasonOtherIdentifier || reasons["Unknown"] != ReasonNotFound ||
		reasons["Series"] != ReasonEpisodeList {
		t.Errorf("unmatched: %+v", status.Unmatched)
	}

	check := func(name string, ref library.TitleRef, want func(userdata.Data) bool) {
		t.Helper()
		if d := h.dataOf(t, alice.ID, ref); !want(d) {
			t.Errorf("%s: %+v", name, d)
		}
	}
	check("by IMDb", byIMDb, func(d userdata.Data) bool {
		return d.Played && d.PlayCount == 5 && d.LastPlayed.Equal(at("2026-09-10T20:00:00Z"))
	})
	check("by TMDB", byTMDB, func(d userdata.Data) bool {
		return d.Played && d.PlayCount == 2 && d.LastPlayed.Equal(at("2026-09-02T20:00:00Z")) && d.Favorite
	})
	check("episode", episode, func(d userdata.Data) bool { return d.Played && d.LastPlayed.Equal(at("2026-09-04T20:00:00Z")) })
	check("anime episode", animeEpisode, func(d userdata.Data) bool { return d.Played && d.LastPlayed == nil })
	check("resumed", resumed, func(d userdata.Data) bool {
		return !d.Played && d.Position == 30*time.Minute && d.Runtime == 100*time.Minute && d.LastPlayed.Equal(at("2026-10-01T20:00:00Z"))
	})
	check("newer here", newerHere, func(d userdata.Data) bool { return d.Position == 50*time.Minute })
	check("nearly done", nearlyDone, func(d userdata.Data) bool { return d.Played && d.Position == 0 })
	check("favorite series", series, func(d userdata.Data) bool { return d.Favorite })

	// Watched on in Polyfin since, then imported again: nothing is added
	// twice, and Polyfin's newer resume point stays.
	if _, err := h.data.Change(t.Context(), alice.ID, []userdata.Item{h.idOf(t, resumed)}, func(d *userdata.Data) {
		d.Position, d.LastPlayed = 60*time.Minute, new(at("2026-10-08T20:00:00Z"))
	}); err != nil {
		t.Fatal(err)
	}
	again := h.imported(t, alice, alice, account, Choice{Profile: "1"})
	if again.Played != 0 || again.Resumed != 0 || again.Favorites != 0 || again.UnmatchedCount != 3 {
		t.Errorf("again: %+v", again)
	}
	check("resumed, after", resumed, func(d userdata.Data) bool { return d.Position == 60*time.Minute })
}

// fakeAddon serves an addon's manifest at any address.
func fakeAddon(t *testing.T, name string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"`+strings.ToLower(name)+`","name":"`+name+`","version":"1.0.0","resources":["catalog"],
			"catalogs":[{"type":"movie","id":"top","name":"Top"}]}`)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/u/secret-token/manifest.json"
}

func TestAddonsAreInstalledOnlyWhenAllowedAndChosen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	root, bob := h.user(t, "root", true), h.user(t, "bob", false)
	first, second := fakeAddon(t, "First"), fakeAddon(t, "Second")
	account := Account{App: "test", Profiles: []Profile{{ID: "1", Name: "Main",
		Addons: []Addon{{Name: "First", ManifestURL: first}, {Name: "Second", ManifestURL: second}}}}}

	// The server's switch off, or the user's: chosen addons are refused,
	// and the account stays held.
	h.personalAddons.Store(false)
	key := h.Hold(root.ID, account)
	if _, err := h.Start(root, bob, key, Choice{Profile: "1", Addons: []int{0}}); !errors.Is(err, ErrAddonsNotAllowed) {
		t.Errorf("server's switch off: %v", err)
	}
	h.personalAddons.Store(true)
	noOwn := bob
	noOwn.PersonalAddons = false
	if _, err := h.Start(root, noOwn, key, Choice{Profile: "1", Addons: []int{0}}); !errors.Is(err, ErrAddonsNotAllowed) {
		t.Errorf("user's permission off: %v", err)
	}
	for _, chosen := range [][]int{{2}, {-1}, {0, 0}} {
		if _, err := h.Start(root, bob, key, Choice{Profile: "1", Addons: chosen}); !errors.Is(err, ErrUnknownAddon) {
			t.Errorf("addons %v: %v", chosen, err)
		}
	}

	// An administrator imports into Bob's account the second addon only,
	// on the local network, which only they reach.
	started, err := h.Start(root, bob, key, Choice{Profile: "1", Addons: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(started.Addons) != 1 || started.Addons[0].Name != "Second" || strings.Contains(started.Addons[0].Address, "secret-token") {
		t.Errorf("started: %+v", started.Addons)
	}
	status := h.ended(t, bob.ID)
	if status.StartedBy != root.ID || len(status.Addons) != 1 || !status.Addons[0].Installed || status.Addons[0].Problem != "" {
		t.Errorf("an administrator's import: %+v", status)
	}
	installed, err := h.addons.Addons(t.Context(), addons.Personal(bob.ID))
	if err != nil || len(installed) != 1 || installed[0].ManifestURL != second {
		t.Errorf("Bob's addons: %+v %v", installed, err)
	}

	// The administrator again: the second is installed already. Bob
	// himself: his own requests stay off local networks.
	status = h.imported(t, root, bob, account, Choice{Profile: "1", Addons: []int{1}})
	if len(status.Addons) != 1 || status.Addons[0].Installed || status.Addons[0].Problem != AddonExists {
		t.Errorf("again: %+v", status.Addons)
	}
	status = h.imported(t, bob, bob, account, Choice{Profile: "1", Addons: []int{0}})
	if len(status.Addons) != 1 || status.Addons[0].Problem != AddonPrivateNetwork || status.StartedBy != bob.ID {
		t.Errorf("Bob's import: %+v", status.Addons)
	}
	// No addon chosen, none installed, whatever the switch.
	h.personalAddons.Store(false)
	if status := h.imported(t, bob, bob, account, Choice{Profile: "1"}); len(status.Addons) != 0 || status.State != StateDone {
		t.Errorf("no addon chosen: %+v", status)
	}
}

func TestAnAccountReadIsHeldForItsReaderForAFewMinutes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice, bob := h.user(t, "alice", false), h.user(t, "bob", false)
	account := Account{App: "test", Profiles: []Profile{{ID: "1", Name: "Main"}}}

	key := h.Hold(alice.ID, account)
	if _, err := h.Start(bob, bob, key, Choice{Profile: "1"}); !errors.Is(err, ErrExpired) {
		t.Errorf("another user's key: %v", err)
	}
	if _, err := h.Start(alice, alice, key, Choice{Profile: "2"}); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("an unknown profile: %v", err)
	}
	// A new read replaces the last.
	newer := h.Hold(alice.ID, account)
	if _, err := h.Start(alice, alice, key, Choice{Profile: "1"}); !errors.Is(err, ErrExpired) {
		t.Errorf("the replaced read: %v", err)
	}
	if _, err := h.Start(alice, alice, newer, Choice{Profile: "1"}); err != nil {
		t.Fatal(err)
	}
	h.ended(t, alice.ID)
	// Imported, it is held no more.
	if _, err := h.Start(alice, alice, newer, Choice{Profile: "1"}); !errors.Is(err, ErrExpired) {
		t.Errorf("imported already: %v", err)
	}
	// Left alone, it is dropped.
	h.mu.Lock()
	h.holdFor = 10 * time.Millisecond
	h.mu.Unlock()
	key = h.Hold(alice.ID, account)
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.mu.Lock()
		held := h.held[alice.ID] != nil
		h.mu.Unlock()
		if !held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the account read is still held")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := h.Start(alice, alice, key, Choice{Profile: "1"}); !errors.Is(err, ErrExpired) {
		t.Errorf("dropped: %v", err)
	}
}

func TestTitlesAreNamedByTheIdentifiersAppsUse(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		id              string
		movie           bool
		season, episode int
		want            library.TitleRef
		ok              bool
	}{
		{"tt0000001", true, 0, 0, library.TitleRef{IMDb: "tt0000001"}, true},
		{"tt0100", false, 0, 0, library.TitleRef{Series: true, IMDb: "tt0100"}, true},
		{"tt0100", false, 0, 1, library.TitleRef{Episode: true, IMDb: "tt0100", Season: 0, Number: 1}, true},
		{"tmdb:550", true, 0, 0, library.TitleRef{TMDB: 550}, true},
		{"tmdb:30", false, 2, 5, library.TitleRef{Episode: true, TMDB: 30, Season: 2, Number: 5}, true},
		// Without the mapping, as anime catalogs list the entry.
		{"kitsu:12", false, 1, 3, library.TitleRef{Episode: true, Anime: library.AnimeRef{Kitsu: 12, Episode: 3}}, true},
		{"kitsu:7", true, 0, 0, library.TitleRef{Anime: library.AnimeRef{Kitsu: 7}}, true},
		{"tmdb:x", true, 0, 0, library.TitleRef{}, false},
		{"mal:5", true, 0, 0, library.TitleRef{}, false},
		{"srv1:abc", true, 0, 0, library.TitleRef{}, false},
		{"custom-prefix:1", false, 1, 1, library.TitleRef{}, false},
	} {
		got, ok := TitleRef(nil, c.id, c.movie, c.season, c.episode)
		if got != c.want || ok != c.ok {
			t.Errorf("%s %d %d: %+v %v", c.id, c.season, c.episode, got, ok)
		}
	}
}

// A title no catalog listed is recorded with the name the app gives it, a
// series' for its episodes, so that lists name it before an addon
// describes it.
func TestUnlistedTitlesKeepTheAppsName(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.user(t, "alice", false)
	entries := []Entry{
		{Title: ref(t, "tt0000901", true, 0, 0), Label: Label{ID: "tt0000901", Name: "The Unlisted Movie", Kind: "movie"}, Played: true},
		{Title: ref(t, "tt0000902", false, 1, 2), Label: Label{ID: "tt0000902", Name: "The Unlisted Show", Kind: "episode", Season: 1, Episode: 2},
			Played: true},
	}
	h.imported(t, alice, alice, Account{App: "test", Profiles: []Profile{{ID: "1", Entries: entries}}}, Choice{Profile: "1"})
	for key, want := range map[string]string{"movie|tt0000901": "The Unlisted Movie", "series|tt0000902": "The Unlisted Show"} {
		var name string
		if err := h.pool.QueryRow(t.Context(), "SELECT coalesce(data->'meta'->>'name', '') FROM items WHERE key = $1", key).Scan(&name); err != nil ||
			name != want {
			t.Errorf("%s: named %q, %v; want %q", key, name, err, want)
		}
	}
}

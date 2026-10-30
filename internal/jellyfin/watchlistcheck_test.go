package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// waitingAddon is an addon of movies and a series whose streams the test
// gives and takes away (see setStreams).
type waitingAddon struct {
	url     string
	mu      sync.Mutex
	streams map[string][]stremio.Stream
}

func (a *waitingAddon) setStreams(id string, streams ...stremio.Stream) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.streams[id] = streams
}

func newWaitingAddon(t *testing.T) *waitingAddon {
	t.Helper()
	a := &waitingAddon{streams: map[string][]stremio.Stream{}}
	metas := map[string]stremio.Meta{
		"tt7001": {Type: "movie", Name: "Open Door", Released: "2001-06-01T00:00:00.000Z", ReleaseInfo: "2001", Extras: &stremio.Extras{Certification: "G"}},
		"tt7002": {Type: "movie", Name: "Grim Night", Released: "2005-09-01T00:00:00.000Z", ReleaseInfo: "2005", Extras: &stremio.Extras{Certification: "R"}},
		"tt7003": {Type: "movie", Name: "Tall Tale", Released: "2010-03-01T00:00:00.000Z", ReleaseInfo: "2010", Extras: &stremio.Extras{Certification: "G"}},
		"tt7004": {Type: "movie", Name: "Far Off", Released: "2099-01-01T00:00:00.000Z", ReleaseInfo: "2099", Extras: &stremio.Extras{Certification: "G"}},
		"tt7005": {Type: "series", Name: "Slow Show", Released: "2012-01-01T00:00:00.000Z", ReleaseInfo: "2012", Extras: &stremio.Extras{Certification: "TV-G"},
			Videos: []stremio.Video{{ID: "tt7005:1:1", Title: "Start", Season: 1, Episode: 1, Released: "2012-01-01T00:00:00.000Z"}}},
	}
	row := func(id string) stremio.Meta {
		m := metas[id]
		m.ID, m.Extras, m.Videos = id, nil, nil
		return m
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, _ := url.PathUnescape(strings.TrimSuffix(r.URL.EscapedPath(), ".json"))
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "waiting", Name: "Waiting", Version: "1",
				Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{row("tt7001"), row("tt7002"), row("tt7003"), row("tt7004")}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{row("tt7005")}})
		case len(parts) == 3 && parts[0] == "meta":
			m := metas[parts[2]]
			m.ID = parts[2]
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": m})
		case len(parts) == 3 && parts[0] == "stream":
			a.mu.Lock()
			streams := append([]stremio.Stream{}, a.streams[parts[2]]...)
			a.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	a.url = server.URL + "/manifest.json"
	return a
}

// told keeps who the checks told of which title.
type told struct {
	mu    sync.Mutex
	names map[accounts.ID]string
	list  []string
}

func (t *told) tell(_ context.Context, user accounts.User, title accounts.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.list = append(t.list, user.Name+" "+t.names[title])
}

func (t *told) all() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.list)
}

// A title of a watchlist that gains a version moves from Still waiting to
// Now available, and its user is told once, after a restart too; one that
// loses its versions moves back, and is not told again when it gains some
// anew. A movie to come, and a version taller than the user's quality
// group, leave a title waiting; a title parental control hides is never
// checked for, listed to or told to the user it hides from.
func TestWatchlistTitlesMoveToNowAvailableWhenVersionsAreFound(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	addon := newWaitingAddon(t)
	installed, err := s.addons.Install(t.Context(), addons.Shared(), addon.url, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: installed.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	member := s.user("member", nil)
	child := s.user("child", func(c *accounts.UserChanges) {
		c.Parental = &accounts.ParentalControl{MaxRating: new(10)}
		c.QualityGroup = new(1080)
	})
	token, childToken := s.signIn("member", "tv"), s.signIn("child", "tablet")
	views := s.viewIDs(t, token)
	ids, names := map[string]accounts.ID{}, map[accounts.ID]string{}
	for _, view := range []string{views["Top"], views["Shows"]} {
		var page QueryResult
		s.get(t, "/Items?ParentId="+view, token, &page)
		for _, item := range page.Items {
			id, _ := accounts.ParseID(item.Id)
			ids[item.Name], names[id] = id, item.Name
		}
	}
	if len(ids) != 5 {
		t.Fatalf("titles: %v", ids)
	}
	lists := watchlist.New(s.pool)
	save := func(user accounts.ID, titles ...string) {
		var saved []watchlist.Title
		for i, name := range titles {
			saved = append(saved, watchlist.Title{ID: ids[name], AddedAt: new(time.Date(2026, 9, 10-i, 0, 0, 0, 0, time.UTC))})
		}
		if _, err := lists.Save(t.Context(), user, watchlist.Import, saved, false); err != nil {
			t.Fatal(err)
		}
	}
	save(member.ID, "Open Door", "Grim Night", "Tall Tale", "Far Off", "Slow Show")
	save(child.ID, "Open Door", "Grim Night", "Tall Tale")
	s.addRow(t, homerows.Row{Name: "Now available", Kind: homerows.KindAvailable, Place: homerows.PlaceTop, Everyone: true, Library: true})
	s.addRow(t, homerows.Row{Name: "Still waiting", Kind: homerows.KindWaiting, Place: homerows.PlaceBottom, Everyone: true})

	now := time.Now()
	var clock sync.Mutex
	at := func() time.Time {
		clock.Lock()
		defer clock.Unlock()
		return now
	}
	later := func() {
		clock.Lock()
		defer clock.Unlock()
		now = now.Add(7 * time.Hour)
	}
	s.library.SetClock(at)
	tells := &told{names: names}
	checker := func(find func(context.Context, accounts.User, accounts.ID) (watchlist.Found, error)) *watchlist.Checker {
		return watchlist.NewChecker(watchlist.CheckerOptions{Store: lists, Users: s.store.Users, Hours: func() int { return 6 }, Find: find,
			Tell: tells.tell, Logger: s.handler.Logger, Now: at})
	}
	check := func(c *watchlist.Checker) {
		t.Helper()
		if err := c.Check(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	// The rows' titles; those found at the same check, by name.
	rows := func(s testServer, token string) []string {
		t.Helper()
		var result []string
		for _, row := range s.customRowsOf(t, token) {
			names := itemNames(row.Items)
			if row.Kind == "NowAvailable" {
				slices.Sort(names)
			}
			result = append(result, row.Name+": "+strings.Join(names, ", "))
		}
		return result
	}
	expect := func(what string, got []string, want ...string) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", what, got, want)
		}
	}

	// No version yet: every title released waits, the movie to come too.
	first := checker(s.handler.FindWatchlistVersions)
	check(first)
	expect("member's rows before any version", rows(s, token), "Still waiting: Open Door, Grim Night, Tall Tale, Far Off, Slow Show")
	expect("child's rows before any version", rows(s, childToken), "Still waiting: Open Door, Tall Tale")

	// Versions come; the movie to come lists one too, Tall Tale only one
	// taller than the child's quality group, and Grim Night only one the
	// version rules hide.
	s.setting(t, func(settings *accounts.Settings) { settings.HiddenVersionWords = []string{"sample"} })
	stream := func(name string) stremio.Stream {
		return stremio.Stream{Name: "Waiting\n" + name, URL: "http://127.0.0.1:9/" + url.PathEscape(name) + ".mkv"}
	}
	addon.setStreams("tt7001", stream("Open.Door.1080p"))
	addon.setStreams("tt7002", stream("Grim.Night.Sample.1080p"))
	addon.setStreams("tt7003", stream("Tall.Tale.2160p"))
	addon.setStreams("tt7004", stream("Far.Off.1080p"))
	addon.setStreams("tt7005:1:1", stream("Slow.Show.S01E01.720p"))
	// Within the hours set, nothing is checked again.
	check(first)
	expect("told within the hours set", tells.all())
	later()
	check(first)
	toldOnce := tells.all()
	slices.Sort(toldOnce)
	expect("told", toldOnce, "child Open Door", "member Open Door", "member Slow Show", "member Tall Tale")
	expect("member's rows once versions are found", rows(s, token), "Now available: Open Door, Slow Show, Tall Tale",
		"Still waiting: Grim Night, Far Off")
	expect("child's rows once versions are found", rows(s, childToken), "Now available: Open Door", "Still waiting: Tall Tale")
	// Other apps list the row as a library, parental control applying.
	if listed := s.viewIDs(t, childToken); listed["Now available"] == "" {
		t.Error("the child's Now available is not a library")
	} else {
		var page QueryResult
		s.get(t, "/Items?ParentId="+listed["Now available"], childToken, &page)
		expect("the child's Now available library", itemNames(page.Items), "Open Door")
	}

	// A restart: what was found stays, and only Grim Night, which finds a
	// version the rules leave, is told of.
	options := s.handler.Options
	options.Library = library.New(s.pool, s.addons, options.Stremio, options.Logger, s.store.Settings)
	options.Library.SetFollowUps()
	options.Library.SetClock(at)
	restarted := New(options)
	server := httptest.NewServer(restarted)
	t.Cleanup(server.Close)
	again := s
	again.url, again.handler, again.library = server.URL, restarted, options.Library
	expect("member's rows after a restart", rows(again, token), "Now available: Open Door, Slow Show, Tall Tale",
		"Still waiting: Grim Night, Far Off")
	addon.setStreams("tt7002", stream("Grim.Night.Sample.1080p"), stream("Grim.Night.720p"))
	later()
	check(checker(restarted.FindWatchlistVersions))
	if got := tells.all(); len(got) != 5 || got[4] != "member Grim Night" {
		t.Errorf("told after a restart: %q", got)
	}
	expect("member's rows after Grim Night found a version", rows(again, token), "Now available: Grim Night, Open Door, Slow Show, Tall Tale",
		"Still waiting: Far Off")

	// Open Door loses its versions: it waits again, and its next ones
	// are not told of.
	addon.setStreams("tt7001")
	later()
	check(checker(restarted.FindWatchlistVersions))
	expect("child's rows once Open Door lost its versions", rows(again, childToken), "Still waiting: Open Door, Tall Tale")
	expect("member's rows once Open Door lost its versions", rows(again, token), "Now available: Grim Night, Slow Show, Tall Tale",
		"Still waiting: Open Door, Far Off")
	addon.setStreams("tt7001", stream("Open.Door.720p"))
	later()
	check(checker(restarted.FindWatchlistVersions))
	expect("child's rows once Open Door found versions again", rows(again, childToken), "Now available: Open Door", "Still waiting: Tall Tale")
	if got := tells.all(); len(got) != 5 {
		t.Errorf("told in all: %q", got)
	}
}

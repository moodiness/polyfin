package stremio

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/appimport"
	"github.com/moodiness/polyfin/internal/appimport/stremio/stremiotest"
	addon "github.com/moodiness/polyfin/internal/stremio"
)

// fake is a fake Stremio with the sample account (see stremiotest.Sample),
// and a reader of it.
func fake(t *testing.T) (*stremiotest.Fake, *Reader) {
	t.Helper()
	f := stremiotest.New(t)
	f.Set(func(f *stremiotest.Fake) {
		f.Email, f.Password = "bob@example.com", "bob's password"
		f.Library = stremiotest.Sample()
		f.Addons = stremiotest.SampleAddons("https://first.example/u/secret/manifest.json", "https://second.example/manifest.json")
	})
	return f, New(Options{Metas: stremiotest.SampleMetas(), Version: "1.2.3", API: f.API, Link: f.Link})
}

func at(text string) time.Time {
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		panic(err)
	}
	return t
}

// entries are the entries of the account's one profile, by label.
func entries(t *testing.T, account appimport.Account) map[appimport.Label]appimport.Entry {
	t.Helper()
	if account.App != App || len(account.Profiles) != 1 || account.Profiles[0].ID != "1" {
		t.Fatalf("account: %+v", account)
	}
	byLabel := map[appimport.Label]appimport.Entry{}
	for _, e := range account.Profiles[0].Entries {
		byLabel[appimport.Label{ID: e.Label.ID, Kind: e.Label.Kind, Season: e.Label.Season, Episode: e.Label.Episode}] = e
	}
	return byLabel
}

func TestTheLinkCodeIsApprovedThenTheAccountReadAndSignedOut(t *testing.T) {
	t.Parallel()
	f, r := fake(t)
	now := time.Now()
	var mu sync.Mutex
	r.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	bob, carol := accounts.ID{1}, accounts.ID{2}

	if _, err := r.PollLink(t.Context(), bob, false); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("before any code: %v", err)
	}
	link, err := r.StartLink(t.Context(), bob)
	if err != nil || link.Code == "" || link.URL != f.Link+"/"+link.Code {
		t.Fatalf("link: %+v %v", link, err)
	}
	if _, err := r.PollLink(t.Context(), bob, false); !errors.Is(err, ErrLinkPending) {
		t.Errorf("before the approval: %v", err)
	}
	// The code is Bob's.
	if _, err := r.PollLink(t.Context(), carol, false); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("Carol polling Bob's code: %v", err)
	}
	f.Approve()
	account, err := r.PollLink(t.Context(), bob, false)
	if err != nil || len(entries(t, account)) != 11 {
		t.Fatalf("after the approval: %+v %v", account, err)
	}
	if f.Open() != 0 {
		t.Error("the session the code opened is still open")
	}
	// The code served once.
	if _, err := r.PollLink(t.Context(), bob, false); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("polled again: %v", err)
	}

	// A code is waited for a few minutes only.
	if _, err := r.StartLink(t.Context(), bob); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	now = now.Add(defaultTiming.linkFor)
	mu.Unlock()
	f.Approve()
	if _, err := r.PollLink(t.Context(), bob, false); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("past a few minutes: %v", err)
	}
	for _, request := range f.Requests() {
		if request.Link && request.Path == "/api/v2/read" && !strings.Contains(request.Query, "type=Read&code=") {
			t.Errorf("read: %+v", request)
		}
	}
}

func TestThePasswordSignsInThenOut(t *testing.T) {
	t.Parallel()
	f, r := fake(t)
	if _, err := r.SignIn(t.Context(), "bob@example.com", "wrong", false); !errors.Is(err, ErrSignInRefused) {
		t.Errorf("a wrong password: %v", err)
	}
	account, err := r.SignIn(t.Context(), " bob@example.com ", "bob's password", false)
	if err != nil || len(entries(t, account)) != 11 {
		t.Fatalf("%+v %v", account, err)
	}
	if f.Open() != 0 {
		t.Error("the session is still open")
	}
	var paths []string
	for _, request := range f.Requests() {
		paths = append(paths, request.Path)
	}
	if want := []string{"/api/login", "/api/login", "/api/datastoreGet", "/api/addonCollectionGet", "/api/logout"}; !slices.Equal(paths, want) {
		t.Errorf("requests: %v", paths)
	}
}

func TestAPastedKeyIsNeverSignedOut(t *testing.T) {
	t.Parallel()
	f, r := fake(t)
	var pasted string
	f.Set(func(f *stremiotest.Fake) { pasted = f.Pasted })
	if _, err := r.ReadKey(t.Context(), "someone else's", false); !errors.Is(err, ErrKeyRefused) {
		t.Errorf("a wrong key: %v", err)
	}
	if _, err := r.ReadKey(t.Context(), "  ", false); !errors.Is(err, ErrKeyRefused) {
		t.Errorf("no key: %v", err)
	}
	// Copied from the browser's console, quoted.
	account, err := r.ReadKey(t.Context(), ` "`+pasted+`" `, false)
	if err != nil || len(entries(t, account)) != 11 {
		t.Fatalf("%+v %v", account, err)
	}
	for _, request := range f.Requests() {
		if request.Path == "/api/logout" {
			t.Errorf("signed out: %+v", request)
		}
	}
	f.Set(func(f *stremiotest.Fake) {
		if f.Pasted != pasted {
			t.Error("the pasted key no longer signs in")
		}
	})
}

func TestTheLibraryIsReadAsStremiosAppsReadIt(t *testing.T) {
	t.Parallel()
	_, r := fake(t)
	account, err := r.ReadKey(t.Context(), "pasted-key-0123456789abcdef", false)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := entries(t, account)
	entry := func(id, kind string, season, episode int) appimport.Entry {
		t.Helper()
		e, ok := byLabel[appimport.Label{ID: id, Kind: kind, Season: season, Episode: episode}]
		if !ok {
			t.Errorf("no entry %s %s S%dE%d", id, kind, season, episode)
		}
		return e
	}
	playedAt := func(e appimport.Entry, when string) bool { return e.PlayedAt != nil && e.PlayedAt.Equal(at(when)) }

	// In the library, watched twice: played, on the watchlist from when
	// Stremio first kept it; the older copy of it is left out.
	if e := entry("tt0000001", "movie", 0, 0); !e.Played || e.Plays != 2 || !playedAt(e, "2026-09-01T20:00:00Z") || !e.Watchlist ||
		e.AddedAt == nil || !e.AddedAt.Equal(at("2026-08-01T20:00:00Z")) || e.Resume != nil || e.Title.IMDb != "tt0000001" ||
		e.Label.Name != "First movie" {
		t.Errorf("first movie: %+v", e)
	}
	// Played without being added: in Continue Watching, not on the
	// watchlist.
	if e := entry("tt0000002", "movie", 0, 0); e.Played || e.Watchlist || e.Resume == nil || e.Resume.Position != 30*time.Minute ||
		e.Resume.Runtime != 100*time.Minute || !e.Resume.At.Equal(at("2026-10-01T20:00:00Z")) {
		t.Errorf("second movie: %+v", e)
	}
	// Removed from the library: watched most of the way, played at its
	// last change; its resume point no longer shows.
	if e := entry("tt0000003", "movie", 0, 0); !e.Played || e.Plays != 0 || !playedAt(e, "2026-09-02T20:00:00Z") || e.Watchlist || e.Resume != nil {
		t.Errorf("removed movie: %+v", e)
	}
	if e := entry("tt0100", "series", 0, 0); !e.Watchlist || e.AddedAt != nil || e.Played || e.Problem != "" || !e.Title.Series ||
		e.Title.IMDb != "tt0100" {
		t.Errorf("series: %+v", e)
	}
	for episode := 1; episode <= 3; episode++ {
		if e := entry("tt0100", "episode", 1, episode); !e.Played || !playedAt(e, "2026-09-04T20:00:00Z") || e.Watchlist || e.Resume != nil ||
			!e.Title.Episode || e.Title.Season != 1 || e.Title.Number != episode {
			t.Errorf("S1E%d: %+v", episode, e)
		}
	}
	if e := entry("tt0100", "episode", 2, 1); e.Played || e.Resume == nil || e.Resume.Position != 5*time.Minute ||
		e.Resume.Runtime != 50*time.Minute {
		t.Errorf("S2E1: %+v", e)
	}
	// Watched against an episode Polyfin's addons do not list: reported.
	if e := entry("tt0200", "series", 0, 0); e.Problem != appimport.ReasonEpisodeList || e.Played || e.Watchlist {
		t.Errorf("unlisted series: %+v", e)
	}
	if e := entry("custom:1", "series", 0, 0); appimport.Identified(e.Title) || !e.Watchlist {
		t.Errorf("home video: %+v", e)
	}
	if e := entry("tmdb:999999", "movie", 0, 0); !e.Played || e.Title.TMDB != 999999 {
		t.Errorf("unknown: %+v", e)
	}
	for label := range byLabel {
		if strings.HasPrefix(label.ID, "channel:") || strings.HasPrefix(label.ID, "tv:") || strings.HasPrefix(label.ID, "local:") {
			t.Errorf("%+v is not a title", label)
		}
	}

	// Stremio's own addon is left out, and each other one listed once.
	want := []appimport.Addon{{Name: "First", ManifestURL: "https://first.example/u/secret/manifest.json"},
		{Name: "Second", ManifestURL: "https://second.example/manifest.json"}}
	if got := account.Profiles[0].Addons; !slices.Equal(got, want) {
		t.Errorf("addons: %+v", got)
	}
}

// Each combination of removed and temp, of a movie watched once and
// stopped again: on the watchlist in the library only, a resume point in
// Continue Watching only, played in all.
func TestRemovedAndTempTellTheLibraryAndContinueWatching(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		removed, temp       bool
		watchlist, resuming bool
	}{
		{false, false, true, true},
		{true, true, false, true},
		{true, false, false, false},
		{false, true, false, true},
	} {
		f := stremiotest.New(t)
		f.Set(func(f *stremiotest.Fake) {
			f.Library = []any{map[string]any{"_id": "tt0000001", "type": "movie", "name": "Movie", "removed": c.removed, "temp": c.temp,
				"_mtime": "2026-09-01T20:00:00.000Z",
				"state": map[string]any{"lastWatched": "2026-09-01T20:00:00.000Z", "timesWatched": 1, "timeOffset": 60_000,
					"duration": 600_000}}}
			f.Addons = []any{}
		})
		r := New(Options{API: f.API, Link: f.Link})
		var pasted string
		f.Set(func(f *stremiotest.Fake) { pasted = f.Pasted })
		account, err := r.ReadKey(t.Context(), pasted, false)
		if err != nil {
			t.Fatal(err)
		}
		got := account.Profiles[0].Entries
		if len(got) != 1 || !got[0].Played || got[0].Watchlist != c.watchlist || (got[0].Resume != nil) != c.resuming {
			t.Errorf("removed %v, temp %v: %+v", c.removed, c.temp, got)
		}
	}
}

func TestStremioFailingOrAnsweringOtherwiseIsTold(t *testing.T) {
	t.Parallel()
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>Not Stremio</html>"))
	}))
	defer web.Close()
	r := New(Options{API: web.URL, Link: web.URL})
	if _, err := r.SignIn(t.Context(), "bob@example.com", "password", false); !errors.Is(err, ErrUnexpected) {
		t.Errorf("not Stremio: %v", err)
	}
	if _, err := r.StartLink(t.Context(), accounts.ID{1}); !errors.Is(err, ErrUnexpected) {
		t.Errorf("a link code from not Stremio: %v", err)
	}
	web.Close()
	if _, err := r.ReadKey(t.Context(), "key", false); !errors.Is(err, ErrUnreachable) {
		t.Errorf("not answering: %v", err)
	}
	if _, err := r.StartLink(t.Context(), accounts.ID{1}); !errors.Is(err, ErrUnreachable) {
		t.Errorf("a link code while not answering: %v", err)
	}
}

// The documented example of stremio-core's watched field: the episodes 1
// to 5 of the first season, the fifth watched last.
const documented = "tt2934286:1:5:5:eJyTZwAAAEAAIA=="

// season lists the videos of a series' episodes 1 to count of season.
func season(series string, number, count int) []addon.Video {
	var videos []addon.Video
	for episode := 1; episode <= count; episode++ {
		videos = append(videos, addon.Video{ID: series + ":" + strconv.Itoa(number) + ":" + strconv.Itoa(episode), Season: addon.Number(number),
			Episode: addon.Number(episode)})
	}
	return videos
}

func TestTheWatchedFieldIsReadAgainstTheEpisodesListedNow(t *testing.T) {
	t.Parallel()
	field, ok := parseWatched(documented)
	if !ok || field.anchor != "tt2934286:1:5" || field.length != 5 || !slices.Equal(field.bits, []byte{0x1f, 0x00}) || !field.any() {
		t.Fatalf("%+v %v", field, ok)
	}
	if empty, ok := parseWatched("undefined:1:eJwDAAAAAAE="); !ok || empty.any() {
		t.Errorf("an empty field: %+v %v", empty, ok)
	}
	for _, broken := range []string{"", "tt1:1", "tt1:x:eJwDAAAAAAE=", "tt1:1:not base64", "tt1:1:AAAA"} {
		if _, ok := parseWatched(broken); ok {
			t.Errorf("%q read", broken)
		}
	}

	read := func(videos []addon.Video) map[string]bool {
		t.Helper()
		list := ordered(videos)
		places, ok := field.places(videoIDs(list))
		if !ok {
			return nil
		}
		watched := map[string]bool{}
		for _, i := range places {
			watched[list[i].ID] = true
		}
		return watched
	}
	first := season("tt2934286", 1, 8)
	want := map[string]bool{"tt2934286:1:1": true, "tt2934286:1:2": true, "tt2934286:1:3": true, "tt2934286:1:4": true, "tt2934286:1:5": true}
	if got := read(first); len(got) != 5 || !mapsEqual(got, want) {
		t.Errorf("as written: %v", got)
	}
	// A special listed since, before the anchor: the bits move with it.
	special := append([]addon.Video{{ID: "tt2934286:0:1", Season: 0, Episode: 1}}, first...)
	if got := read(special); !mapsEqual(got, want) {
		t.Errorf("with a special: %v", got)
	}
	// An episode listed since after the anchor changes nothing before.
	if got := read(append(slices.Clone(first), addon.Video{ID: "tt2934286:2:1", Season: 2, Episode: 1})); !mapsEqual(got, want) {
		t.Errorf("with a new season: %v", got)
	}
	// Without the anchor, nothing can be read.
	if got := read(slices.Delete(slices.Clone(first), 4, 5)); got != nil {
		t.Errorf("without the anchor: %v", got)
	}
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// The series' episodes are read in place: an episode added before the
// anchor since, none when the anchor is gone, and the report says so.
func TestAccountEpisodesFollowTheirAnchor(t *testing.T) {
	t.Parallel()
	f := stremiotest.New(t)
	f.Set(func(f *stremiotest.Fake) {
		f.Library = []any{
			map[string]any{"_id": "tt2934286", "type": "series", "name": "Shifted", "removed": true, "temp": true,
				"state": map[string]any{"lastWatched": "2026-09-01T20:00:00.000Z", "watched": documented}},
			map[string]any{"_id": "tt0300", "type": "series", "name": "Gone", "removed": false, "temp": false,
				"state": map[string]any{"lastWatched": "2026-09-01T20:00:00.000Z", "watched": stremiotest.Watched("tt0300:1:2", 2, 0, 1)}},
		}
		f.Addons = []any{}
	})
	videos := append([]addon.Video{{ID: "tt2934286:1:0", Season: 1, Episode: 0, Released: "2025-01-01T00:00:00Z"}},
		season("tt2934286", 1, 6)...)
	metas := stremiotest.Metas{
		"tt2934286": {ID: "tt2934286", Type: "series", Videos: append(videos, videos[3])},
		"tt0300":    {ID: "tt0300", Type: "series", Videos: season("tt0300", 1, 1)},
	}
	r := New(Options{Metas: metas, API: f.API, Link: f.Link})
	account, err := r.ReadKey(t.Context(), "pasted-key-0123456789abcdef", false)
	if err != nil {
		t.Fatal(err)
	}
	var played []int
	byLabel := entries(t, account)
	for label, e := range byLabel {
		if label.ID == "tt2934286" && label.Kind == "episode" && e.Played {
			played = append(played, label.Episode)
		}
	}
	slices.Sort(played)
	if !slices.Equal(played, []int{1, 2, 3, 4, 5}) || len(byLabel) != 6 {
		t.Errorf("shifted: %v %+v", played, byLabel)
	}
	if e := byLabel[appimport.Label{ID: "tt0300", Kind: "series"}]; e.Problem != appimport.ReasonEpisodeList || !e.Watchlist {
		t.Errorf("gone: %+v", e)
	}
}

func TestVideosAreOrderedAsStremiosAppsOrderThem(t *testing.T) {
	t.Parallel()
	got := videoIDs(ordered([]addon.Video{
		{ID: "b", Season: 1, Episode: 2},
		{ID: "a", Season: 1, Episode: 1, Released: "2026-01-02T00:00:00Z"},
		{ID: "z", Season: 1, Episode: 1, Released: "2026-01-01T00:00:00Z"},
		{ID: "b", Season: 9, Episode: 9},
		{ID: "special", Season: 0, Episode: 1},
		{ID: "unnumbered"},
		{ID: "unnumbered-dated", Released: "2020-01-01T00:00:00Z"},
		{ID: "bonus"},
	}))
	if want := []string{"unnumbered", "bonus", "unnumbered-dated", "special", "z", "a", "b"}; !slices.Equal(got, want) {
		t.Errorf("%v", got)
	}
}

func TestStayingConnectedKeepsTheKeyForLaterReads(t *testing.T) {
	t.Parallel()
	f, r := fake(t)

	// Signed in by password to stay connected, the session stays open, and
	// the account holds its key, with the account's email.
	account, err := r.SignIn(t.Context(), "bob@example.com", "bob's password", true)
	if err != nil {
		t.Fatal(err)
	}
	if account.SignIn == nil || !account.SignIn.Owned || account.SignIn.Secret == "" || account.Name != "bob@example.com" || f.Open() != 1 {
		t.Fatalf("account: %q %+v, %d open", account.Name, account.SignIn, f.Open())
	}
	kept := *account.SignIn
	for _, request := range f.Requests() {
		if request.Path == "/api/logout" {
			t.Error("a session kept was signed out")
		}
	}

	// A later read uses the key kept, which Stremio does not renew.
	again, renewed, err := r.Reread(t.Context(), kept)
	if err != nil || renewed != kept || again.Name != "bob@example.com" || len(entries(t, again)) == 0 {
		t.Fatalf("read again: %+v %v", renewed, err)
	}

	// Signing out ends the session Polyfin opened; Stremio refuses the key
	// from then on.
	if err := r.SignOut(t.Context(), kept); err != nil {
		t.Errorf("signing out: %v", err)
	}
	if f.Open() != 0 {
		t.Error("the session is still open")
	}
	if _, _, err := r.Reread(t.Context(), kept); !errors.Is(err, appimport.ErrSignInRefused) {
		t.Errorf("a key signed out: %v", err)
	}

	// A link code approved to stay connected keeps its session too.
	if _, err := r.StartLink(t.Context(), accounts.ID{1}); err != nil {
		t.Fatal(err)
	}
	f.Approve()
	linked, err := r.PollLink(t.Context(), accounts.ID{1}, true)
	if err != nil || linked.SignIn == nil || !linked.SignIn.Owned || linked.Name != "bob@example.com" || f.Open() != 1 {
		t.Fatalf("linked: %+v %v, %d open", linked.SignIn, err, f.Open())
	}
	// Stremio ending every session refuses it.
	f.Revoke()
	if _, _, err := r.Reread(t.Context(), *linked.SignIn); !errors.Is(err, appimport.ErrSignInRefused) {
		t.Errorf("a key revoked: %v", err)
	}
}

func TestAPastedKeyKeptIsNeverSignedOut(t *testing.T) {
	t.Parallel()
	f, r := fake(t)
	var pasted string
	f.Set(func(f *stremiotest.Fake) { pasted = f.Pasted })
	account, err := r.ReadKey(t.Context(), pasted, true)
	if err != nil || account.SignIn == nil || account.SignIn.Owned || account.SignIn.Secret != pasted {
		t.Fatalf("account: %+v %v", account.SignIn, err)
	}
	if err := r.SignOut(t.Context(), *account.SignIn); err != nil {
		t.Errorf("signing out: %v", err)
	}
	for _, request := range f.Requests() {
		if request.Path == "/api/logout" {
			t.Error("a pasted key was signed out")
		}
	}
	if _, _, err := r.Reread(t.Context(), *account.SignIn); err != nil {
		t.Errorf("read again: %v", err)
	}
}

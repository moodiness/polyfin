package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/importlinks"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// importTitlesTitles describes the movie tt0111161 and the series
// tt0903747, and finds the items of neither: a link is kept, and applies
// nothing.
type importTitlesTitles struct{}

func (importTitlesTitles) Resolve(_ context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error) {
	return make([][]library.TitleTarget, len(refs)), nil
}

func (importTitlesTitles) RemoteLookup(_ context.Context, kind library.Kind, id string) (library.RemoteResult, bool, error) {
	known := map[string]library.RemoteResult{
		"tt0111161": {Item: library.Item{Name: "The Shawshank Redemption", ProductionYear: 1994,
			ProviderIDs: map[string]string{"Tmdb": "278"}}},
		"tt0903747": {Item: library.Item{Name: "Breaking Bad", ProductionYear: 2008, ProviderIDs: map[string]string{"Tmdb": "1396"}}},
	}
	kinds := map[string]library.Kind{"tt0111161": library.KindMovie, "tt0903747": library.KindSeries}
	if kinds[id] != kind {
		return library.RemoteResult{}, false, nil
	}
	return known[id], true, nil
}

func (importTitlesTitles) LookUpIDs(context.Context, library.Kind, map[string]string) (*library.RemoteResult, error) {
	return nil, nil
}

// importTitlesAPI serves the admin API with the kept titles of imports,
// and returns their store.
func importTitlesAPI(t *testing.T) (testAPI, *importlinks.Store) {
	t.Helper()
	var store *importlinks.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		store = importlinks.New(importlinks.Options{DB: deps.pool, Settings: o.Accounts.Settings, Titles: importTitlesTitles{},
			UserData: userdata.New(deps.pool), Watchlist: watchlist.New(deps.pool),
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		o.ImportTitles = store
	})
	return api, store
}

// userNamed is the account of name.
func userNamed(t *testing.T, api testAPI, name string) accounts.User {
	t.Helper()
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Name == name {
			return u
		}
	}
	t.Fatalf("no user %s", name)
	return accounts.User{}
}

// keepUnfound keeps for user, as an import of nuvio would, a movie and two
// episodes of a series it did not find.
func keepUnfound(t *testing.T, store *importlinks.Store, user accounts.ID) {
	t.Helper()
	entries := []importlinks.Entry{
		{Key: "tmdb:9", Label: importlinks.Label{Kind: importlinks.KindMovie, Name: "A movie", IDs: "tmdb:9", Year: 1994},
			Reason: "not_found", Played: true},
		{Key: "tmdb:123", Label: importlinks.Label{Kind: importlinks.KindEpisode, Name: "Show", IDs: "tmdb:123", Season: 1,
			Episode: 3}, Reason: "not_found", Played: true},
		{Key: "tmdb:123", Label: importlinks.Label{Kind: importlinks.KindEpisode, Name: "Show", IDs: "tmdb:123", Season: 1,
			Episode: 4}, Reason: "not_found", Played: true},
	}
	if err := store.Save(t.Context(), user, "nuvio", entries, true); err != nil {
		t.Fatal(err)
	}
}

// importTitleNamed is the title of key in a list answered, the first of
// its episodes.
func importTitleNamed(t *testing.T, body map[string]any, key string) map[string]any {
	t.Helper()
	titles, _ := body["titles"].([]any)
	for _, title := range titles {
		if m, _ := title.(map[string]any); m["key"] == key {
			return m
		}
	}
	t.Fatalf("no title %s in %v", key, body)
	return nil
}

// checkImportTitles lists, links and unlinks the titles of the account
// prefix serves, as b.
func checkImportTitles(t *testing.T, b browser, prefix string, by accounts.User) {
	t.Helper()
	// Counts are of rows: each episode kept of a series is one.
	status, body, _ := b.call(http.MethodGet, prefix+"/import-titles", nil)
	sources, _ := body["sources"].([]any)
	if status != http.StatusOK || len(sources) != 1 {
		t.Fatalf("sources: %d %v", status, body)
	}
	if s := sources[0].(map[string]any); s["source"] != "nuvio" || s["notFound"] != 3.0 || s["linked"] != 0.0 {
		t.Errorf("source: %v", s)
	}
	status, body, _ = b.call(http.MethodGet, prefix+"/import-titles/nuvio", nil)
	if titles, _ := body["titles"].([]any); status != http.StatusOK || len(titles) != 3 {
		t.Fatalf("titles: %d %v", status, body)
	}
	if movie := importTitleNamed(t, body, "tmdb:9"); movie["kind"] != "movie" || movie["year"] != 1994.0 ||
		movie["season"] != 0.0 || movie["reason"] != "not_found" || movie["link"] != nil {
		t.Errorf("movie: %v", movie)
	}
	if episode := importTitleNamed(t, body, "tmdb:123"); episode["kind"] != "episode" || episode["name"] != "Show" ||
		episode["year"] != nil || episode["ids"] != "tmdb:123" || episode["season"] != 1.0 || episode["episode"] != 3.0 {
		t.Errorf("episode: %v", episode)
	}

	status, body, _ = b.call(http.MethodPut, prefix+"/import-titles/nuvio/links",
		map[string]string{"key": "tmdb:123", "target": "https://www.imdb.com/title/tt0903747/"})
	link, _ := body["link"].(map[string]any)
	if status != http.StatusOK || link["kind"] != "series" || link["imdb"] != "tt0903747" || link["tmdb"] != 1396.0 ||
		link["name"] != "Breaking Bad" || link["year"] != 2008.0 || link["linkedAt"] == nil {
		t.Fatalf("link: %d %v", status, body)
	}
	if linkedBy, _ := link["linkedBy"].(map[string]any); linkedBy["id"] != by.ID.String() || linkedBy["name"] != by.Name {
		t.Errorf("linked by: %v", link["linkedBy"])
	}
	_, body, _ = b.call(http.MethodGet, prefix+"/import-titles", nil)
	if s := body["sources"].([]any)[0].(map[string]any); s["notFound"] != 1.0 || s["linked"] != 2.0 {
		t.Errorf("source once linked: %v", s)
	}
	_, body, _ = b.call(http.MethodGet, prefix+"/import-titles/nuvio", nil)
	if l, _ := importTitleNamed(t, body, "tmdb:123")["link"].(map[string]any); l["imdb"] != "tt0903747" {
		t.Errorf("listed link: %v", l)
	}

	for _, c := range []struct {
		key, target string
		status      int
		code        string
	}{
		{"tmdb:9", "not an identifier", http.StatusBadRequest, "invalid_target"},
		{"tmdb:9", "tt0903747", http.StatusBadRequest, "kind_mismatch"},
		{"tmdb:9", "https://www.themoviedb.org/tv/1396", http.StatusBadRequest, "kind_mismatch"},
		{"tmdb:9", "https://www.themoviedb.org/movie/424242", http.StatusNotFound, "target_not_found"},
		{"tmdb:404", "tt0111161", http.StatusNotFound, "not_found"},
	} {
		status, body, _ := b.call(http.MethodPut, prefix+"/import-titles/nuvio/links", map[string]string{"key": c.key, "target": c.target})
		if status != c.status || body["error"] != c.code {
			t.Errorf("%s to %s: %d %v, want %d %s", c.key, c.target, status, body, c.status, c.code)
		}
	}

	if status, body, _ := b.call(http.MethodDelete, prefix+"/import-titles/nuvio/links", map[string]string{"key": "tmdb:123"}); status != http.StatusNoContent {
		t.Fatalf("unlink: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodDelete, prefix+"/import-titles/nuvio/links", map[string]string{"key": "tmdb:123"}); status != http.StatusNotFound || body["error"] != "not_found" {
		t.Errorf("unlink again: %d %v", status, body)
	}
	_, body, _ = b.call(http.MethodGet, prefix+"/import-titles/nuvio", nil)
	if l := importTitleNamed(t, body, "tmdb:123")["link"]; l != nil {
		t.Errorf("link once unlinked: %v", l)
	}

	for _, source := range []string{"Nuvio", "nuvio1"} {
		if status, body, _ := b.call(http.MethodGet, prefix+"/import-titles/"+source, nil); status != http.StatusNotFound || body["error"] != "not_found" {
			t.Errorf("source %s: %d %v", source, status, body)
		}
	}
}

func TestImportTitlesOfTheOwnAccount(t *testing.T) {
	t.Parallel()
	api, store := importTitlesAPI(t)
	member := api.signedIn("member", false)
	other := api.signedIn("other", false)
	user := userNamed(t, api, "member")
	keepUnfound(t, store, user.ID)
	keepUnfound(t, store, userNamed(t, api, "other").ID)
	checkImportTitles(t, member, "/account", user)

	// The other account's list is untouched by the member's links, and
	// out of their reach.
	_, body, _ := other.call(http.MethodGet, "/account/import-titles/nuvio", nil)
	if l := importTitleNamed(t, body, "tmdb:123")["link"]; l != nil {
		t.Errorf("the other account's title: %v", l)
	}
	otherID := userNamed(t, api, "other").ID.String()
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/users/" + otherID + "/import-titles", nil},
		{http.MethodGet, "/users/" + otherID + "/import-titles/nuvio", nil},
		{http.MethodPut, "/users/" + otherID + "/import-titles/nuvio/links", map[string]string{"key": "tmdb:9", "target": "tt0111161"}},
		{http.MethodDelete, "/users/" + otherID + "/import-titles/nuvio/links", map[string]string{"key": "tmdb:9"}},
	} {
		if status, body, _ := member.call(c.method, c.path, c.body); status != http.StatusForbidden {
			t.Errorf("%s %s by a member: %d %v", c.method, c.path, status, body)
		}
	}
	linked, err := store.Linked(t.Context(), userNamed(t, api, "other").ID, "nuvio")
	if err != nil || len(linked) != 0 {
		t.Errorf("the other account's links: %v %v", linked, err)
	}
}

func TestImportTitlesOfAUserByAnAdministrator(t *testing.T) {
	t.Parallel()
	api, store := importTitlesAPI(t)
	admin := api.signedIn("admin", true)
	api.signedIn("member", false)
	user := userNamed(t, api, "member")
	keepUnfound(t, store, user.ID)
	checkImportTitles(t, admin, "/users/"+user.ID.String(), userNamed(t, api, "admin"))

	// The administrator's own list is empty.
	if _, body, _ := admin.call(http.MethodGet, "/account/import-titles", nil); len(body["sources"].([]any)) != 0 {
		t.Errorf("the administrator's sources: %v", body)
	}
	unknown := accounts.ID{1}.String()
	for _, path := range []string{"/users/" + unknown + "/import-titles", "/users/" + unknown + "/import-titles/nuvio",
		"/users/nobody/import-titles"} {
		if status, body, _ := admin.call(http.MethodGet, path, nil); status != http.StatusNotFound || body["error"] != "not_found" {
			t.Errorf("%s: %d %v", path, status, body)
		}
	}
}

func TestImportTitlesWithoutTheStore(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	if status, body, _ := member.call(http.MethodGet, "/account/import-titles", nil); status != http.StatusNotFound || body["error"] != "not_found" {
		t.Errorf("without the store: %d %v", status, body)
	}
}

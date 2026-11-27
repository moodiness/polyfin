package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// entriesVersions offers the Versions page, and records the items its
// routes act on.
type entriesVersions struct {
	Versions
	mu    sync.Mutex
	items []accounts.ID
}

func (v *entriesVersions) record(item accounts.ID) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.items = append(v.items, item)
}

// acted returns the items recorded since the last call.
func (v *entriesVersions) acted() []accounts.ID {
	v.mu.Lock()
	defer v.mu.Unlock()
	items := v.items
	v.items = nil
	return items
}

func (v *entriesVersions) ExplainVersions(_ context.Context, _ accounts.User, id accounts.ID) (jellyfin.VersionsExplanation, error) {
	v.record(id)
	return jellyfin.VersionsExplanation{}, nil
}

func (v *entriesVersions) PinVersion(_ context.Context, _, _ accounts.User, item, _ accounts.ID) error {
	v.record(item)
	return nil
}

func (v *entriesVersions) UnpinVersion(_ context.Context, item accounts.ID) error {
	v.record(item)
	return nil
}

func (v *entriesVersions) BlockVersion(_ context.Context, _, _ accounts.User, item, _ accounts.ID, _ bool, _ string) (library.VersionBlock, error) {
	v.record(item)
	return library.VersionBlock{Item: item}, nil
}

func (v *entriesVersions) AnalyzeVersion(_ context.Context, _ accounts.User, item, _ accounts.ID) (string, error) {
	v.record(item)
	return "", nil
}

func (v *entriesVersions) MakeThumbnailsAgain(_ context.Context, _ accounts.User, item, _ accounts.ID) (thumbnails.Result, error) {
	v.record(item)
	return thumbnails.Result{State: thumbnails.StateWaiting}, nil
}

func (v *entriesVersions) AskVersionsAgain(_ context.Context, _, _ accounts.User, item accounts.ID) (time.Duration, error) {
	v.record(item)
	return 0, nil
}

// newEntriesProvider is an Xtream account (user / secret) that lists the
// same series twice, 81 in 4K and 82, sharing a TMDB id, with episodes
// S1E1 and S1E2, and S1E3 for 82.
func newEntriesProvider(t *testing.T) string {
	t.Helper()
	episode := func(id string, number int) map[string]any {
		return map[string]any{"id": id, "episode_num": number, "season": 1, "title": "Severance - Episode", "container_extension": "mkv"}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/player_api.php" || query.Get("username") != "user" || query.Get("password") != "secret" {
			http.NotFound(w, r)
			return
		}
		var answer any
		switch query.Get("action") {
		case "":
			answer = map[string]any{"user_info": map[string]any{"auth": 1}}
		case "get_series_categories":
			answer = []any{map[string]any{"category_id": "3", "category_name": "4K Series"}, map[string]any{"category_id": "4", "category_name": "Series"}}
		case "get_series":
			answer = []any{
				map[string]any{"name": "Severance 4K", "series_id": "81", "category_id": "3", "tmdb": "95396", "last_modified": "1"},
				map[string]any{"name": "Severance", "series_id": "82", "category_id": "4", "tmdb": "95396", "last_modified": "1"}}
		case "get_series_info":
			episodes := []any{episode("8101", 1), episode("8102", 2)}
			if query.Get("series_id") == "82" {
				episodes = []any{episode("8201", 1), episode("8202", 2), episode("8203", 3)}
			}
			answer = map[string]any{"info": map[string]any{"plot": "A show."}, "episodes": map[string]any{"1": episodes}}
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// entriesSetup is the admin API with the provider's account as a source
// of series that groups them, as an administrator and a member see it:
// the group's series, 81, and its S1E2, and the other entry's series.
type entriesSetup struct {
	api                   testAPI
	administrator, member browser
	user                  accounts.User
	lib                   *library.Service
	versions              *entriesVersions
	shows                 accounts.ID
	show4K, show, episode accounts.ID
}

func newEntriesSetup(t *testing.T) entriesSetup {
	t.Helper()
	s := entriesSetup{versions: &entriesVersions{}}
	var channels *iptv.Service
	s.api = newTestAPI(t, 10, func(o *Options, _ testDeps) {
		s.lib, channels = o.ShownTitles.(*library.Service), o.IPTV
		o.Titles, o.Versions = s.lib, s.versions
	})
	s.administrator = s.api.signedIn("administrator", true)
	s.member = s.api.signedIn("member", false)
	users, err := s.api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Name == "administrator" {
			s.user = u
		}
	}
	on, off := true, false
	source, err := channels.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: newEntriesProvider(t), Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &off, Series: &on, LiveTv: &off,
		Enrichment: &off}}, false)
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := s.lib.Libraries(t.Context(), s.user)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range libraries {
		if l.CollectionType == "tvshows" {
			s.shows = l.ID
		}
	}
	if len(s.series(t)) != 2 {
		t.Fatalf("series before: %v", s.series(t))
	}
	if err := channels.Update(t.Context(), addons.Shared(), source.ID, iptv.Changes{Options: &iptv.OptionsPatch{TitleVersions: &on}}, false); err != nil {
		t.Fatal(err)
	}
	s.show4K = library.TitleItemID(library.KindSeries, "polyfin-iptv:"+source.ID.String()+":series:81")
	s.show = library.TitleItemID(library.KindSeries, "polyfin-iptv:"+source.ID.String()+":series:82")
	if series := s.series(t); !slices.Equal(series, []accounts.ID{s.show4K}) {
		t.Fatalf("series grouped: %v", series)
	}
	s.episode = s.episodes(t, s.show4K)["1x2"]
	return s
}

// series lists the series of the library.
func (s entriesSetup) series(t *testing.T) []accounts.ID {
	t.Helper()
	page, err := s.lib.Children(t.Context(), s.user, s.shows, 0, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	var ids []accounts.ID
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

// episodes lists a series' episodes by season and number, once opened.
func (s entriesSetup) episodes(t *testing.T, series accounts.ID) map[string]accounts.ID {
	t.Helper()
	if _, err := s.lib.Item(t.Context(), s.user, series); err != nil {
		t.Fatal(err)
	}
	episodes, err := s.lib.Episodes(t.Context(), s.user, series, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]accounts.ID{}
	for _, e := range episodes {
		result[strconv.Itoa(e.ParentIndexNumber)+"x"+strconv.Itoa(e.IndexNumber)] = e.ID
	}
	return result
}

// entryFlags reads an entries answer as each entry's key and flags; none
// without entries.
func entryFlags(body map[string]any) []string {
	entries, _ := body["entries"].([]any)
	var result []string
	for _, e := range entries {
		entry := e.(map[string]any)
		flags := entry["key"].(string)
		for _, flag := range []string{"leader", "member", "apart", "listed"} {
			if entry[flag] == true {
				flags += " " + flag
			}
		}
		result = append(result, flags)
	}
	return result
}

// The Versions page lists the entries of an IPTV source that share an
// episode's series' TMDB id, for administrators alone, and keeps one apart
// or groups it again from the episode's page, which answers the series'
// entries though the group, and its episode, ended; a key that is not one
// of them is refused.
func TestVersionsPageKeepsIPTVEntriesApart(t *testing.T) {
	t.Parallel()
	s := newEntriesSetup(t)
	administrator, member := s.administrator, s.member
	entries := "/titles/" + s.episode.String() + "/entries"
	grouped := []string{"81 leader member listed", "82 member listed"}

	if status, body, _ := member.call(http.MethodGet, entries, nil); status != http.StatusForbidden || body["error"] != "forbidden" {
		t.Errorf("a member's entries: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, entries+"/apart", map[string]any{"key": "82", "apart": true}); status != http.StatusForbidden ||
		body["error"] != "forbidden" {
		t.Errorf("a member keeping apart: %d %v", status, body)
	}
	status, body, _ := administrator.call(http.MethodGet, entries, nil)
	if got := entryFlags(body); status != http.StatusOK || !slices.Equal(got, grouped) {
		t.Fatalf("the episode's entries: %d %q %v", status, got, body)
	}
	for _, key := range []string{"83", ""} {
		if status, body, _ := administrator.call(http.MethodPut, entries+"/apart", map[string]any{"key": key, "apart": true}); status != http.StatusBadRequest ||
			body["error"] != "invalid_entry" {
			t.Errorf("keeping %q apart: %d %v", key, status, body)
		}
	}

	// Keeping the first entry apart ends the group, whose episode the page
	// shows: the answer is the series' entries all the same.
	status, body, _ = administrator.call(http.MethodPut, entries+"/apart", map[string]any{"key": "81", "apart": true})
	if got := entryFlags(body); status != http.StatusOK || !slices.Equal(got, []string{"81 apart listed", "82 listed"}) {
		t.Fatalf("the first entry kept apart: %d %q %v", status, got, body)
	}
	if series := s.series(t); len(series) != 2 {
		t.Errorf("series apart: %v", series)
	}
	// The page, still on the group's episode, which is the first entry's
	// own episode of its numbers now, lists the entries again and groups
	// the first again.
	status, body, _ = administrator.call(http.MethodGet, entries, nil)
	if got := entryFlags(body); status != http.StatusOK || !slices.Equal(got, []string{"81 apart listed", "82 listed"}) {
		t.Fatalf("the former episode's entries: %d %q %v", status, got, body)
	}
	status, body, _ = administrator.call(http.MethodPut, entries+"/apart", map[string]any{"key": "81", "apart": false})
	if got := entryFlags(body); status != http.StatusOK || !slices.Equal(got, grouped) {
		t.Fatalf("the first entry grouped again: %d %q %v", status, got, body)
	}
	if series := s.series(t); !slices.Equal(series, []accounts.ID{s.show4K}) {
		t.Errorf("series grouped again: %v", series)
	}
}

// Every route of the Versions page answers, for an item gone as titles
// were grouped or a group ended, the item it became: a grouped entry's
// series is the group's, and the group's episode is the first entry's own
// once the group ended.
func TestVersionsPageFollowsMovedIPTVItems(t *testing.T) {
	t.Parallel()
	s := newEntriesSetup(t)
	administrator := s.administrator
	var title liveItemJSON
	if status := administrator.raw(http.MethodGet, "/titles/"+s.show.String(), "", &title); status != http.StatusOK || title.ID != s.show4K.String() {
		t.Errorf("the grouped entry's series: %d %+v", status, title)
	}
	var episodes []liveItemJSON
	if status := administrator.raw(http.MethodGet, "/titles/"+s.show.String()+"/episodes", "", &episodes); status != http.StatusOK || len(episodes) != 3 ||
		episodes[1].ID != s.episode.String() {
		t.Errorf("the grouped entry's episodes: %d %+v", status, episodes)
	}

	if status, body, _ := administrator.call(http.MethodPut, "/titles/"+s.episode.String()+"/entries/apart", map[string]any{"key": "82", "apart": true}); status != http.StatusOK {
		t.Fatalf("keeping the other entry apart: %d %v", status, body)
	}
	moved := s.episodes(t, s.show4K)["1x2"]
	if moved == s.episode || moved == (accounts.ID{}) {
		t.Fatalf("the first entry's own S1E2: %s", moved)
	}
	if status := administrator.raw(http.MethodGet, "/titles/"+s.episode.String(), "", &title); status != http.StatusOK || title.ID != moved.String() ||
		title.Kind != "episode" || title.Episode != 2 {
		t.Errorf("the former episode: %d %+v", status, title)
	}
	page := "/titles/" + s.episode.String()
	version := accounts.ID{9}.String()
	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/versions", ""},
		{http.MethodPost, "/versions/search", ""},
		{http.MethodPut, "/versions/pin", `{"version":"` + version + `"}`},
		{http.MethodDelete, "/versions/pin", ""},
		{http.MethodPost, "/versions/blocks", `{"version":"` + version + `"}`},
		{http.MethodPost, "/versions/" + version + "/analyze", ""},
		{http.MethodPost, "/versions/" + version + "/thumbnails", ""},
	} {
		status := administrator.raw(route.method, page+route.path, route.body, nil)
		if acted := s.versions.acted(); status >= http.StatusBadRequest || !slices.Equal(acted, []accounts.ID{moved}) {
			t.Errorf("%s %s: %d, acted on %v, not %s", route.method, route.path, status, acted, moved)
		}
	}
	if status, body, _ := administrator.call(http.MethodGet, page+"/entries", nil); status != http.StatusOK ||
		!slices.Equal(entryFlags(body), []string{"81 listed", "82 apart listed"}) {
		t.Errorf("the former episode's entries: %d %v", status, body)
	}
}

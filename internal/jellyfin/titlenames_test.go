package jellyfin

import (
	"cmp"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// namesProvider is an Xtream account (user / secret) whose list names
// its titles in its own way: a movie under a translated title, another
// under the name its metadata gives, three copies of one movie, one that
// goes, and a series with numbered episodes; dropped leaves entries out
// of its lists, and tmdbs gives entries other TMDB ids.
type namesProvider struct {
	url     string
	mu      sync.Mutex
	dropped map[string]bool
	tmdbs   map[string]string
}

func (p *namesProvider) drop(ids ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range ids {
		p.dropped[id] = true
	}
}

// identify gives the entry of id another TMDB id.
func (p *namesProvider) identify(id, tmdb string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tmdbs[id] = tmdb
}

func newNamesProvider(t *testing.T) *namesProvider {
	t.Helper()
	p := &namesProvider{dropped: map[string]bool{}, tmdbs: map[string]string{}}
	listed := func(entries ...map[string]any) []any {
		p.mu.Lock()
		defer p.mu.Unlock()
		var result []any
		for _, e := range entries {
			if id, _ := e["stream_id"].(string); !p.dropped[id] {
				result = append(result, e)
			}
		}
		return result
	}
	movie := func(id, name, category, tmdb, added string) map[string]any {
		p.mu.Lock()
		defer p.mu.Unlock()
		return map[string]any{"name": name, "stream_id": id, "category_id": category, "container_extension": "mkv", "added": added,
			"tmdb": cmp.Or(p.tmdbs[id], tmdb)}
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
		case "get_vod_categories":
			answer = []any{map[string]any{"category_id": "1", "category_name": "4K Movies"}, map[string]any{"category_id": "2", "category_name": "Movies"}}
		case "get_series_categories":
			answer = []any{map[string]any{"category_id": "3", "category_name": "Series"}}
		case "get_vod_streams":
			answer = listed(
				movie("61", "D+ - Le Voyage de Zeb (FR)", "2", "1000", "1700000600"),
				movie("62", "Mango Tale", "2", "2000", "1700000500"),
				movie("63", "D+ | Orb Saga 4K", "1", "3000", "1700000400"),
				movie("64", "D+ | Orb Saga FHD", "2", "3000", "1700000300"),
				movie("66", "D+ | Orb Saga HD", "2", "3000", "1700000250"),
				movie("65", "D+ - Bobine Perdue", "2", "4000", "1700000200"))
		case "get_series":
			answer = []any{map[string]any{"name": "D+ - Les Jours de Quill (FR)", "series_id": "71", "category_id": "3", "tmdb": "5000", "last_modified": "1"}}
		case "get_series_info":
			episode := func(id string, number int) map[string]any {
				return map[string]any{"id": id, "episode_num": number, "season": 1, "title": "Quill - S01E0" + id[len(id)-1:] + " - Episode " + id[len(id)-1:],
					"container_extension": "mkv"}
			}
			answer = map[string]any{"info": map[string]any{"plot": "A show."}, "episodes": map[string]any{"1": []any{episode("7101", 1), episode("7102", 2)}}}
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	p.url = server.URL
	return p
}

// metadataNamesAddon is a Stremio metadata addon describing the provider's TMDB
// movies and series under their own names, and the series' episodes
// under their titles.
func metadataNamesAddon(t *testing.T) string {
	t.Helper()
	metas := map[string]map[string]any{
		"movie/tmdb:1000": {"name": "Zeb Quest"},
		"movie/tmdb:2000": {"name": "Mango Tale"},
		"movie/tmdb:3000": {"name": "Orb Saga"},
		"movie/tmdb:4000": {"name": "Lost Reel"},
		"series/tmdb:5000": {"name": "Quill Days", "videos": []any{
			map[string]any{"id": "tmdb:5000:1:1", "season": 1, "episode": 1, "title": "The Quill Begins"},
			map[string]any{"id": "tmdb:5000:1:2", "season": 1, "episode": 2, "name": "Ink and Iron"},
		}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "test.names", "version": "1.0.0", "name": "Metadata", "types": []string{"movie", "series"},
				"resources": []any{map[string]any{"name": "meta", "types": []string{"movie", "series"}, "idPrefixes": []string{"tmdb:"}}},
				"catalogs":  []any{}})
			return
		}
		key, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/meta/"), ".json")
		meta, found := metas[key]
		if !ok || !found {
			http.NotFound(w, r)
			return
		}
		typ, id, _ := strings.Cut(key, "/")
		described := map[string]any{"id": id, "type": typ, "description": "The metadata addon's overview."}
		for k, v := range meta {
			described[k] = v
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": described})
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// namesSetup adds the provider's account as a source of movies, and of
// series when asked, whose titles of a TMDB id are versions of one title
// and take their metadata names, for a member, on a clock the test moves.
type namesSetup struct {
	s        testServer
	provider *namesProvider
	source   accounts.ID
	member   accounts.User
	token    string
	views    map[string]string
	clock    *atomic.Pointer[time.Time]
}

func newNamesSetup(t *testing.T, series bool) namesSetup {
	t.Helper()
	provider := newNamesProvider(t)
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	clock := &atomic.Pointer[time.Time]{}
	started := time.Now()
	clock.Store(&started)
	s.handler.now = func() time.Time { return *clock.Load() }
	if _, err := s.addons.Install(t.Context(), addons.Shared(), metadataNamesAddon(t), false); err != nil {
		t.Fatal(err)
	}
	on, off := true, false
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: provider.url, Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, Series: &series, LiveTv: &off,
		MetadataNames: &on, TitleVersions: &on}}, false)
	if err != nil {
		t.Fatal(err)
	}
	return namesSetup{s: s, provider: provider, source: addon.ID, member: member, token: token, views: s.viewsByType(t, token), clock: clock}
}

// item is the item of the source's title of identifier id ("vod:61").
func (n namesSetup) item(kind library.Kind, id string) string {
	return library.TitleItemID(kind, "polyfin-iptv:"+n.source.String()+":"+id).String()
}

// options changes the source's options.
func (n namesSetup) options(t *testing.T, patch iptv.OptionsPatch) {
	t.Helper()
	if err := n.s.iptv.Update(t.Context(), addons.Shared(), n.source, iptv.Changes{Options: &patch}, false); err != nil {
		t.Fatal(err)
	}
}

// names lists a view's titles' names by identifier.
func (n namesSetup) names(t *testing.T, view string) map[string]string {
	t.Helper()
	var page QueryResult
	n.s.get(t, "/Items?ParentId="+n.views[view]+"&userId="+n.member.ID.String(), n.token, &page)
	result := map[string]string{}
	for _, item := range page.Items {
		result[item.Id] = item.Name
	}
	return result
}

// search lists the names of the movies a search finds.
func (n namesSetup) search(t *testing.T, term string) []string {
	t.Helper()
	var found QueryResult
	n.s.get(t, "/Items?Recursive=true&IncludeItemTypes=Movie&searchTerm="+url.QueryEscape(term)+"&userId="+n.member.ID.String(), n.token, &found)
	return itemNames(found.Items)
}

// open opens an item as an app does.
func (n namesSetup) open(t *testing.T, id string) BaseItemDto {
	t.Helper()
	var item BaseItemDto
	if status := n.s.get(t, "/Users/"+n.member.ID.String()+"/Items/"+id, n.token, &item); status != http.StatusOK {
		t.Fatalf("opening %s: %d", id, status)
	}
	return item
}

// found counts the movies a search finds.
func (n namesSetup) found(t *testing.T, term string) int {
	t.Helper()
	var found QueryResult
	n.s.get(t, "/Items?Recursive=true&IncludeItemTypes=Movie&searchTerm="+url.QueryEscape(term)+"&userId="+n.member.ID.String(), n.token, &found)
	return found.TotalRecordCount
}

// A source whose option is on names a title after the metadata addon once
// it is opened: before, the provider's name; after, the addon's in its
// listings, latest, searches by either name, its stored record and,
// within minutes, the listings by name; a series' episodes under the
// addon's titles. Turned off, or without enrichment, the provider's names
// show again, and turned back on the names learned; an administrator's
// name wins.
func TestIPTVTitlesTakeTheirMetadataNames(t *testing.T) {
	t.Parallel()
	n := newNamesSetup(t, true)
	zeb, orb, quill := n.item(library.KindMovie, "vod:61"), n.item(library.KindMovie, "vod:63"), n.item(library.KindSeries, "series:71")
	byName := "/Items?" + url.Values{"ParentId": {n.views["movies"]}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"},
		"SortBy": {"SortName"}}.Encode()

	// Before it is opened: the provider's name.
	movies := n.names(t, "movies")
	if len(movies) != 4 || !strings.Contains(movies[zeb], "Voyage") {
		t.Fatalf("movies before: %q", movies)
	}
	if found := n.search(t, "quest"); len(found) != 0 {
		t.Errorf("found by the addon's name before it was learned: %q", found)
	}
	if sorted, _ := n.s.pageNames(t, byName, n.token); len(sorted) != 4 || sorted[3] != "Mango Tale" {
		t.Errorf("by name before: %q", sorted)
	}

	// Opened: the addon's name, everywhere the title shows.
	if opened := n.open(t, zeb); opened.Name != "Zeb Quest" {
		t.Errorf("opened: %q", opened.Name)
	}
	if movies = n.names(t, "movies"); movies[zeb] != "Zeb Quest" || !strings.Contains(movies[orb], "D+") {
		t.Errorf("listed after the open: %q", movies)
	}
	for _, term := range []string{"quest", "voyage"} {
		if found := n.search(t, term); !slices.Equal(found, []string{"Zeb Quest"}) {
			t.Errorf("%s found %q", term, found)
		}
	}
	var latest []BaseItemDto
	n.s.get(t, "/Users/"+n.member.ID.String()+"/Items/Latest?ParentId="+n.views["movies"]+"&Limit=10", n.token, &latest)
	if !slices.Contains(itemNames(latest), "Zeb Quest") {
		t.Errorf("latest: %q", itemNames(latest))
	}
	if !until(10*time.Second, func() bool {
		var name string
		err := n.s.pool.QueryRow(t.Context(), "SELECT coalesce(data->'meta'->>'name', '') FROM items WHERE id = $1", mustID(t, zeb)).Scan(&name)
		return err == nil && name == "Zeb Quest"
	}) {
		t.Error("the stored record keeps the provider's name")
	}
	later := n.clock.Load().Add(orderFresh + time.Minute)
	n.clock.Store(&later)
	if !until(10*time.Second, func() bool {
		sorted, _ := n.s.pageNames(t, byName, n.token)
		return len(sorted) == 4 && sorted[3] == "Zeb Quest"
	}) {
		sorted, _ := n.s.pageNames(t, byName, n.token)
		t.Errorf("by name after the open: %q", sorted)
	}

	// A series' episodes take the addon's titles.
	if shows := n.names(t, "tvshows"); !strings.Contains(shows[quill], "Quill") || shows[quill] == "Quill Days" {
		t.Errorf("the series before: %q", shows)
	}
	if opened := n.open(t, quill); opened.Name != "Quill Days" {
		t.Errorf("the series opened: %q", opened.Name)
	}
	var episodes QueryResult
	n.s.get(t, "/Shows/"+quill+"/Episodes?userId="+n.member.ID.String(), n.token, &episodes)
	if got := itemNames(episodes.Items); !slices.Equal(got, []string{"The Quill Begins", "Ink and Iron"}) {
		t.Errorf("the episodes: %q", got)
	}

	// Off, or without enrichment, the provider's names; on again, those
	// learned, without opening the titles again.
	on, off := true, false
	for _, patch := range []iptv.OptionsPatch{{MetadataNames: &off}, {MetadataNames: &on, Enrichment: &off}} {
		n.options(t, patch)
		if movies = n.names(t, "movies"); !strings.Contains(movies[zeb], "Voyage") {
			t.Errorf("%+v: %q", patch, movies)
		}
		if found := n.search(t, "quest"); len(found) != 0 {
			t.Errorf("%+v: quest found %q", patch, found)
		}
	}
	n.options(t, iptv.OptionsPatch{Enrichment: &on})
	if movies = n.names(t, "movies"); movies[zeb] != "Zeb Quest" {
		t.Errorf("on again: %q", movies)
	}

	// An administrator's name wins.
	item, err := n.s.library.Item(t.Context(), n.member, mustID(t, zeb))
	if err != nil {
		t.Fatal(err)
	}
	renamed := "Zeb, Extended"
	if err := n.s.library.SaveOverrides(t.Context(), item, library.Overrides{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	if movies = n.names(t, "movies"); movies[zeb] != renamed {
		t.Errorf("renamed by an administrator: %q", movies[zeb])
	}
	if opened := n.open(t, zeb); opened.Name != renamed {
		t.Errorf("opened, renamed by an administrator: %q", opened.Name)
	}
}

// A group of titles takes the name its leader learned, its entries
// keeping the provider's names; with the leader no longer listed, its
// face shows the leader's name. A title the provider no longer lists, or
// lists under another TMDB id, loses its name, and so does a leader no
// longer listed once it is kept apart.
func TestIPTVTitleGroupsTakeTheirLeadersMetadataName(t *testing.T) {
	t.Parallel()
	n := newNamesSetup(t, false)
	zeb, orb, lost := n.item(library.KindMovie, "vod:61"), n.item(library.KindMovie, "vod:63"), n.item(library.KindMovie, "vod:65")
	learned := func(key string) bool {
		t.Helper()
		var count int
		if err := n.s.pool.QueryRow(t.Context(), "SELECT count(*) FROM iptv_title_names WHERE addon_id = $1 AND key = $2", n.source, key).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count > 0
	}
	if movies := n.names(t, "movies"); len(movies) != 4 || !strings.Contains(movies[orb], "D+") {
		t.Fatalf("the group before: %q", movies)
	}
	for id, want := range map[string]string{orb: "Orb Saga", lost: "Lost Reel", zeb: "Zeb Quest"} {
		if opened := n.open(t, id); opened.Name != want {
			t.Errorf("opened %q, want %q", opened.Name, want)
		}
	}
	if movies := n.names(t, "movies"); len(movies) != 4 || movies[orb] != "Orb Saga" {
		t.Errorf("the group listed: %q", movies)
	}
	if found := n.search(t, "orb saga"); !slices.Equal(found, []string{"Orb Saga"}) || n.found(t, "orb saga") != 1 {
		t.Errorf("the group searched: %q, counted %d", found, n.found(t, "orb saga"))
	}
	if _, _, entries, err := n.s.iptv.TitleEntries(t.Context(), "polyfin-iptv:"+n.source.String()+":vod:63"); err != nil || len(entries) != 3 ||
		slices.ContainsFunc(entries, func(e iptv.TitleEntry) bool { return !strings.HasPrefix(e.Name, "D+") }) {
		t.Errorf("the group's entries: %+v %v", entries, err)
	}

	n.provider.drop("63", "65")
	n.provider.identify("61", "1001")
	if err := n.s.iptv.Refresh(t.Context(), addons.Shared(), n.source, false); err != nil {
		t.Fatal(err)
	}
	movies := n.names(t, "movies")
	if len(movies) != 3 || movies[orb] != "Orb Saga" || !strings.Contains(movies[zeb], "Voyage") {
		t.Errorf("the group without its leader, a movie of another TMDB id: %q", movies)
	}
	if opened := n.open(t, orb); opened.Name != "Orb Saga" {
		t.Errorf("the group without its leader opened: %q", opened.Name)
	}
	if learned("65") || learned("61") || !learned("63") {
		t.Errorf("names kept: the title gone's %v, the title of another TMDB id's %v, the group leader's %v", learned("65"), learned("61"),
			learned("63"))
	}
	if err := n.s.iptv.KeepApart(t.Context(), n.source, "movie", "63", true); err != nil {
		t.Fatal(err)
	}
	if learned("63") {
		t.Error("the former leader, kept apart, keeps its name")
	}
}

// A group whose leader the provider no longer lists keeps its name until
// the option that groups the titles is turned off.
func TestIPTVLearnedNamesGoWithTheirGroups(t *testing.T) {
	t.Parallel()
	n := newNamesSetup(t, false)
	n.names(t, "movies")
	if opened := n.open(t, n.item(library.KindMovie, "vod:63")); opened.Name != "Orb Saga" {
		t.Fatalf("the group opened: %q", opened.Name)
	}
	n.provider.drop("63")
	if err := n.s.iptv.Refresh(t.Context(), addons.Shared(), n.source, false); err != nil {
		t.Fatal(err)
	}
	learned := func() bool {
		t.Helper()
		var count int
		if err := n.s.pool.QueryRow(t.Context(), "SELECT count(*) FROM iptv_title_names WHERE addon_id = $1 AND key = '63'", n.source).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count > 0
	}
	if !learned() {
		t.Fatal("the leader no longer listed lost its name")
	}
	off := false
	n.options(t, iptv.OptionsPatch{TitleVersions: &off})
	if learned() {
		t.Error("the former leader keeps its name once grouping is off")
	}
}

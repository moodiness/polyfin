package jellyfin

import (
	"encoding/json"
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
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// versionsProvider is an Xtream account (user / secret) that lists the
// same movie twice, in 4K and FHD, beside another, and the same series
// twice, whose second entry has a third episode and one without a
// number; dropped leaves entries out of its lists, at first the third
// copies of the movie, in HD (54), and of the series (83), which restore
// lists.
type versionsProvider struct {
	url     string
	mu      sync.Mutex
	dropped map[string]bool
}

func (p *versionsProvider) drop(ids ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range ids {
		p.dropped[id] = true
	}
}

func (p *versionsProvider) restore(ids ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range ids {
		delete(p.dropped, id)
	}
}

func newVersionsProvider(t *testing.T) *versionsProvider {
	t.Helper()
	p := &versionsProvider{dropped: map[string]bool{"54": true, "83": true}}
	listed := func(entries ...map[string]any) []any {
		p.mu.Lock()
		defer p.mu.Unlock()
		var result []any
		for _, e := range entries {
			id, _ := e["stream_id"].(string)
			if id == "" {
				id, _ = e["series_id"].(string)
			}
			if !p.dropped[id] {
				result = append(result, e)
			}
		}
		return result
	}
	episode := func(id string, number any, title string) map[string]any {
		return map[string]any{"id": id, "episode_num": number, "season": 1, "title": "Severance - " + title, "container_extension": "mkv"}
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
			answer = []any{map[string]any{"category_id": "3", "category_name": "4K Series"}, map[string]any{"category_id": "4", "category_name": "Series"}}
		case "get_vod_streams":
			answer = listed(
				map[string]any{"name": "EN - Dune (2021) 4K", "stream_id": "51", "category_id": "1", "container_extension": "mkv", "added": "1700000300",
					"tmdb": "438631"},
				map[string]any{"name": "EN - Dune (2021) FHD", "stream_id": "52", "category_id": "2", "container_extension": "mp4", "added": "1700000200",
					"tmdb": "438631"},
				map[string]any{"name": "EN - Arrival (2016)", "stream_id": "53", "category_id": "2", "container_extension": "mkv", "added": "1700000100",
					"tmdb": "329865"},
				map[string]any{"name": "EN - Dune (2021) HD", "stream_id": "54", "category_id": "2", "container_extension": "mkv", "added": "1700000050",
					"tmdb": "438631"})
		case "get_series":
			answer = listed(
				map[string]any{"name": "Severance 4K", "series_id": "81", "category_id": "3", "tmdb": "95396", "last_modified": "1"},
				map[string]any{"name": "Severance", "series_id": "82", "category_id": "4", "tmdb": "95396", "last_modified": "1"},
				map[string]any{"name": "Severance HD", "series_id": "83", "category_id": "4", "tmdb": "95396", "last_modified": "1"})
		case "get_series_info":
			episodes := []any{episode("8101", 1, "S01E01 - Good News"), episode("8102", 2, "S01E02 - Half Loop")}
			switch query.Get("series_id") {
			case "82":
				episodes = []any{episode("8201", 1, "S01E01 - Good News"), episode("8202", 2, "S01E02 - Half Loop"),
					episode("8203", 3, "S01E03 - In Perpetuity"), episode("8299", "", "Behind the Scenes")}
			case "83":
				episodes = []any{episode("8301", 1, "S01E01 - Good News")}
			}
			answer = map[string]any{"info": map[string]any{"plot": "A show."}, "episodes": map[string]any{"1": episodes}}
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	p.url = server.URL
	return p
}

// versionsSetup adds the provider's account as a source of movies and
// series, without versions grouped, for a member.
type versionsSetup struct {
	s        testServer
	provider *versionsProvider
	source   accounts.ID
	member   accounts.User
	token    string
	views    map[string]string
}

func newVersionsSetup(t *testing.T) versionsSetup {
	t.Helper()
	provider := newVersionsProvider(t)
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	on, off := true, false
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: provider.url, Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, Series: &on, LiveTv: &off,
		Enrichment: &off}}, false)
	if err != nil {
		t.Fatal(err)
	}
	return versionsSetup{s: s, provider: provider, source: addon.ID, member: member, token: token, views: s.viewsByType(t, token)}
}

// group turns the source's option on or off.
func (v versionsSetup) group(t *testing.T, on bool) {
	t.Helper()
	if err := v.s.iptv.Update(t.Context(), addons.Shared(), v.source, iptv.Changes{Options: &iptv.OptionsPatch{TitleVersions: &on}}, false); err != nil {
		t.Fatal(err)
	}
}

// list lists a view's titles by identifier.
func (v versionsSetup) list(t *testing.T, view string) (map[string]BaseItemDto, int) {
	t.Helper()
	var page QueryResult
	v.s.get(t, "/Items?ParentId="+v.views[view]+"&userId="+v.member.ID.String(), v.token, &page)
	byID := map[string]BaseItemDto{}
	for _, item := range page.Items {
		byID[item.Id] = item
	}
	return byID, page.TotalRecordCount
}

// open opens an item as an app does, which answers the item it is now.
func (v versionsSetup) open(t *testing.T, id string) BaseItemDto {
	t.Helper()
	var item BaseItemDto
	if status := v.s.get(t, "/Users/"+v.member.ID.String()+"/Items/"+id, v.token, &item); status != http.StatusOK {
		t.Fatalf("opening %s: %d", id, status)
	}
	return item
}

// episodes lists a series' episodes by season and number, "1x0" for one
// without a number, once its view and the series are opened, as an app
// does.
func (v versionsSetup) episodes(t *testing.T, series string) map[string]BaseItemDto {
	t.Helper()
	v.list(t, "tvshows")
	v.open(t, series)
	var page QueryResult
	v.s.get(t, "/Shows/"+series+"/Episodes?userId="+v.member.ID.String(), v.token, &page)
	result := map[string]BaseItemDto{}
	for _, e := range page.Items {
		season, number := 0, 0
		if e.ParentIndexNumber != nil {
			season = *e.ParentIndexNumber
		}
		if e.IndexNumber != nil {
			number = *e.IndexNumber
		}
		result[strconv.Itoa(season)+"x"+strconv.Itoa(number)] = e
	}
	return result
}

// versions lists the addresses and labels of an item's versions.
func (v versionsSetup) versions(t *testing.T, id string) ([]string, []string) {
	t.Helper()
	versions, err := v.s.library.Versions(t.Context(), v.member, mustID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	var addresses, names []string
	for _, version := range versions {
		addresses = append(addresses, strings.TrimPrefix(version.URL, v.provider.url))
		names = append(names, version.Name)
	}
	return addresses, names
}

// change sets what the member did with an item.
func (v versionsSetup) change(t *testing.T, id string, change func(*userdata.Data)) {
	t.Helper()
	if _, err := userdata.New(v.s.pool).Change(t.Context(), v.member.ID, []userdata.Item{{ID: mustID(t, id)}}, change); err != nil {
		t.Fatal(err)
	}
}

// The entries of one source that share a TMDB id are one title once the
// option is on: listed, counted and searched once, under the identifier
// of the first in the provider's order, with each entry's file as a
// version labelled by its quality and category; a series' episodes are
// matched by season and number, an episode one entry alone has, or
// without a number, played from that entry. What the member did with
// every entry moves to the title's items, and the former entries open as
// the title.
func TestIPTVTitlesOfOneTMDBIDAreVersionsOfOneTitle(t *testing.T) {
	t.Parallel()
	v := newVersionsSetup(t)
	movies, total := v.list(t, "movies")
	if total != 3 || len(movies) != 3 {
		t.Fatalf("movies before: %d %+v", total, movies)
	}
	var shows QueryResult
	v.s.get(t, "/Items?ParentId="+v.views["tvshows"], v.token, &shows)
	if len(shows.Items) != 2 {
		t.Fatalf("series before: %+v", shows.Items)
	}
	// The movies and series by their entries' identifiers.
	stremioIDs := map[string]string{}
	for _, id := range []string{"vod:51", "vod:52", "series:81", "series:82"} {
		stremioIDs[id] = "polyfin-iptv:" + v.source.String() + ":" + id
	}
	dune4K := library.TitleItemID(library.KindMovie, stremioIDs["vod:51"]).String()
	duneFHD := library.TitleItemID(library.KindMovie, stremioIDs["vod:52"]).String()
	show4K := library.TitleItemID(library.KindSeries, stremioIDs["series:81"]).String()
	show := library.TitleItemID(library.KindSeries, stremioIDs["series:82"]).String()
	firstEpisodes, secondEpisodes := v.episodes(t, show4K), v.episodes(t, show)
	if len(firstEpisodes) != 2 || len(secondEpisodes) != 4 {
		t.Fatalf("episodes before: %d and %d", len(firstEpisodes), len(secondEpisodes))
	}
	at := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	v.change(t, duneFHD, func(d *userdata.Data) { d.MarkPlayed(&at, at); d.Favorite = true; d.Rating = new(8.0) })
	v.change(t, show, func(d *userdata.Data) { d.Favorite = true })
	v.change(t, firstEpisodes["1x1"].Id, func(d *userdata.Data) { d.MarkPlayed(&at, at) })
	v.change(t, secondEpisodes["1x2"].Id, func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 10*time.Minute, time.Hour, &at })
	v.change(t, secondEpisodes["1x3"].Id, func(d *userdata.Data) { d.MarkPlayed(&at, at) })

	v.group(t, true)
	movies, total = v.list(t, "movies")
	if _, ok := movies[dune4K]; total != 2 || len(movies) != 2 || !ok || movies[dune4K].Name != "Dune" {
		t.Fatalf("movies grouped: %d %+v", total, movies)
	}
	var found QueryResult
	v.s.get(t, "/Items?Recursive=true&IncludeItemTypes=Movie&searchTerm=dune", v.token, &found)
	if len(found.Items) != 1 || found.Items[0].Id != dune4K {
		t.Errorf("search: %+v", found.Items)
	}
	if d := movies[dune4K].UserData; !d.Played || !d.IsFavorite || d.Likes == nil || !*d.Likes {
		t.Errorf("the movie's data: %+v", d)
	}
	addresses, names := v.versions(t, dune4K)
	if !slices.Equal(addresses, []string{"/movie/user/secret/51.mkv", "/movie/user/secret/52.mp4"}) || len(names) != 2 ||
		!strings.Contains(names[0], "4K · 4K Movies") || !strings.Contains(names[1], "FHD · Movies") {
		t.Errorf("the movie's versions: %q %q", addresses, names)
	}
	if opened := v.open(t, duneFHD); opened.Id != dune4K {
		t.Errorf("the former entry opens as %s %q, not the title", opened.Id, opened.Name)
	}

	v.s.get(t, "/Items?ParentId="+v.views["tvshows"]+"&userId="+v.member.ID.String(), v.token, &shows)
	if len(shows.Items) != 1 || shows.Items[0].Id != show4K || !shows.Items[0].UserData.IsFavorite {
		t.Fatalf("series grouped: %+v", shows.Items)
	}
	episodes := v.episodes(t, show4K)
	if len(episodes) != 4 {
		t.Fatalf("the title's episodes: %+v", episodes)
	}
	if !episodes["1x1"].UserData.Played || episodes["1x2"].UserData.PlaybackPositionTicks != int64(10*time.Minute/100) ||
		!episodes["1x3"].UserData.Played || episodes["1x0"].UserData.Played {
		t.Errorf("the episodes' data: %+v %+v %+v", episodes["1x1"].UserData, episodes["1x2"].UserData, episodes["1x3"].UserData)
	}
	for numbers, want := range map[string][]string{
		"1x1": {"/series/user/secret/8101.mkv", "/series/user/secret/8201.mkv"},
		"1x2": {"/series/user/secret/8102.mkv", "/series/user/secret/8202.mkv"},
		"1x3": {"/series/user/secret/8203.mkv"},
		"1x0": {"/series/user/secret/8299.mkv"},
	} {
		if addresses, _ := v.versions(t, episodes[numbers].Id); !slices.Equal(addresses, want) {
			t.Errorf("episode %s: %q, want %q", numbers, addresses, want)
		}
	}
	if opened := v.open(t, secondEpisodes["1x2"].Id); opened.Id != episodes["1x2"].Id {
		t.Errorf("the former episode opens as %s %q", opened.Id, opened.Name)
	}
	if opened := v.open(t, show); opened.Id != show4K {
		t.Errorf("the former series opens as %s %q", opened.Id, opened.Name)
	}
}

// A group of three keeps its identifiers once the provider no longer
// lists the entry they came from; keeping apart one of the two entries
// left ends it, and the first of them still listed has the group's data;
// grouped again, the two form a group under its identifiers. Turning the
// option off gives the group's data to the first entry's own items, or to
// the entry that has the episode.
func TestIPTVTitleVersionsOutliveTheirFirstEntry(t *testing.T) {
	t.Parallel()
	v := newVersionsSetup(t)
	v.provider.restore("54", "83")
	stremio := func(id string) string { return "polyfin-iptv:" + v.source.String() + ":" + id }
	dune4K := library.TitleItemID(library.KindMovie, stremio("vod:51")).String()
	duneFHD := library.TitleItemID(library.KindMovie, stremio("vod:52")).String()
	duneHD := library.TitleItemID(library.KindMovie, stremio("vod:54")).String()
	show4K := library.TitleItemID(library.KindSeries, stremio("series:81")).String()
	show := library.TitleItemID(library.KindSeries, stremio("series:82")).String()
	if err := v.s.iptv.Refresh(t.Context(), addons.Shared(), v.source, false); err != nil {
		t.Fatal(err)
	}
	v.list(t, "movies")
	v.list(t, "tvshows")
	v.group(t, true)
	episodes := v.episodes(t, show4K)
	at := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	v.change(t, dune4K, func(d *userdata.Data) { d.MarkPlayed(&at, at) })
	v.change(t, episodes["1x2"].Id, func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 20*time.Minute, time.Hour, &at })
	v.change(t, episodes["1x3"].Id, func(d *userdata.Data) { d.Favorite = true })

	// Turned off: the first entry's own items have the group's data, and
	// an episode only the other has is that entry's.
	v.group(t, false)
	first, second := v.episodes(t, show4K), v.episodes(t, show)
	if first["1x2"].UserData.PlaybackPositionTicks != int64(20*time.Minute/100) || !second["1x3"].UserData.IsFavorite ||
		second["1x2"].UserData.PlaybackPositionTicks != 0 {
		t.Errorf("split: %+v %+v %+v", first["1x2"].UserData, second["1x3"].UserData, second["1x2"].UserData)
	}
	movies, total := v.list(t, "movies")
	if _, ok := movies[duneFHD]; total != 4 || !ok || !movies[dune4K].UserData.Played {
		t.Errorf("movies split: %d %+v", total, movies)
	}

	// On again, then the provider drops the first entries: the title keeps
	// its identifiers, with the other entries' files.
	v.group(t, true)
	v.provider.drop("51", "81")
	if err := v.s.iptv.Refresh(t.Context(), addons.Shared(), v.source, false); err != nil {
		t.Fatal(err)
	}
	movies, total = v.list(t, "movies")
	if total != 2 || !movies[dune4K].UserData.Played {
		t.Fatalf("movies without the first entry: %d %+v", total, movies)
	}
	if addresses, _ := v.versions(t, dune4K); !slices.Equal(addresses, []string{"/movie/user/secret/52.mp4", "/movie/user/secret/54.mkv"}) {
		t.Errorf("the movie's versions without the first entry: %q", addresses)
	}
	episodes = v.episodes(t, show4K)
	if addresses, _ := v.versions(t, episodes["1x1"].Id); len(episodes) != 4 ||
		!slices.Equal(addresses, []string{"/series/user/secret/8201.mkv", "/series/user/secret/8301.mkv"}) {
		t.Errorf("the series without the first entry: %d episodes, %q", len(episodes), addresses)
	}

	// Keeping one of the two entries left apart ends the group: the first
	// of them, kept apart or not, has the group's data.
	_, _, entries, err := v.s.iptv.TitleEntries(t.Context(), stremio("vod:51"))
	if err != nil || len(entries) != 3 || !entries[0].Leader || entries[0].Listed || !entries[1].Member || entries[1].Key != "52" ||
		!entries[2].Member || entries[2].Key != "54" {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	if err := v.s.iptv.KeepApart(t.Context(), v.source, "movie", "52", true); err != nil {
		t.Fatal(err)
	}
	movies, total = v.list(t, "movies")
	if _, grouped := movies[dune4K]; grouped || total != 3 || !movies[duneFHD].UserData.Played || movies[duneHD].UserData.Played {
		t.Errorf("the entry kept apart: %d %+v", total, movies)
	}
	_, _, entries, err = v.s.iptv.TitleEntries(t.Context(), stremio("vod:52"))
	if err != nil || len(entries) != 2 || !entries[0].Apart || entries[0].Key != "52" || entries[1].Member || entries[1].Apart {
		t.Errorf("entries kept apart: %+v %v", entries, err)
	}
	if err := v.s.iptv.KeepApart(t.Context(), v.source, "movie", "52", false); err != nil {
		t.Fatal(err)
	}
	if movies, _ = v.list(t, "movies"); len(movies) != 2 || movies[duneFHD].Name != "Dune" || !movies[duneFHD].UserData.Played {
		t.Errorf("the two entries left grouped again: %+v", movies)
	}
	if opened := v.open(t, show); opened.Id != show4K {
		t.Errorf("the other series opens as %s", opened.Id)
	}
}

// Keeping the first entry of a series apart ends its group of two: what
// the member did with the group's episodes goes to the first entry's own
// episodes of the same numbers, or to the other entry's when the first
// lacks one, which the group's episodes open as; grouped again, the
// entries' data is the group's episodes' again, under the same
// identifiers.
func TestIPTVSeriesKeptApartTakesItsEpisodesData(t *testing.T) {
	t.Parallel()
	v := newVersionsSetup(t)
	stremio := func(id string) string { return "polyfin-iptv:" + v.source.String() + ":" + id }
	show4K := library.TitleItemID(library.KindSeries, stremio("series:81")).String()
	show := library.TitleItemID(library.KindSeries, stremio("series:82")).String()
	v.list(t, "tvshows")
	v.group(t, true)
	grouped := v.episodes(t, show4K)
	if len(grouped) != 4 {
		t.Fatalf("the group's episodes: %+v", grouped)
	}
	at := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	v.change(t, show4K, func(d *userdata.Data) { d.Favorite = true })
	v.change(t, grouped["1x2"].Id, func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 20*time.Minute, time.Hour, &at })
	v.change(t, grouped["1x3"].Id, func(d *userdata.Data) { d.MarkPlayed(&at, at); d.Favorite = true })
	resume := int64(20 * time.Minute / 100)

	if err := v.s.iptv.KeepApart(t.Context(), v.source, "series", "81", true); err != nil {
		t.Fatal(err)
	}
	shows, _ := v.list(t, "tvshows")
	if len(shows) != 2 || !shows[show4K].UserData.IsFavorite || shows[show].UserData.IsFavorite {
		t.Fatalf("series apart: %+v", shows)
	}
	first, second := v.episodes(t, show4K), v.episodes(t, show)
	if len(first) != 2 || len(second) != 4 {
		t.Fatalf("episodes apart: %d and %d", len(first), len(second))
	}
	for _, e := range []BaseItemDto{first["1x2"], second["1x3"]} {
		if e.Id == grouped["1x2"].Id || e.Id == grouped["1x3"].Id {
			t.Errorf("episode %s kept the group's identifier", e.Name)
		}
	}
	if d := first["1x2"].UserData; d.PlaybackPositionTicks != resume || d.Played {
		t.Errorf("the first entry's S1E2: %+v", d)
	}
	if d := second["1x3"].UserData; !d.Played || !d.IsFavorite {
		t.Errorf("the other entry's S1E3: %+v", d)
	}
	if d := second["1x2"].UserData; d.PlaybackPositionTicks != 0 || d.Played {
		t.Errorf("the other entry's S1E2: %+v", d)
	}
	if d := first["1x1"].UserData; d.PlaybackPositionTicks != 0 || d.Played || d.IsFavorite {
		t.Errorf("the first entry's S1E1: %+v", d)
	}
	_, _, entries, err := v.s.iptv.TitleEntries(t.Context(), stremio("series:81"))
	if err != nil || len(entries) != 2 || !entries[0].Apart || entries[0].Key != "81" || entries[1].Member || entries[1].Apart {
		t.Errorf("entries apart: %+v %v", entries, err)
	}
	// An app holding the group's episodes opens the episodes that have
	// their data now.
	if opened := v.open(t, grouped["1x2"].Id); opened.Id != first["1x2"].Id || opened.UserData.PlaybackPositionTicks != resume {
		t.Errorf("the group's former S1E2 opens as %s %+v", opened.Id, opened.UserData)
	}
	if opened := v.open(t, grouped["1x3"].Id); opened.Id != second["1x3"].Id || !opened.UserData.Played {
		t.Errorf("the group's former S1E3 opens as %s %+v", opened.Id, opened.UserData)
	}

	if err := v.s.iptv.KeepApart(t.Context(), v.source, "series", "81", false); err != nil {
		t.Fatal(err)
	}
	shows, _ = v.list(t, "tvshows")
	if len(shows) != 1 || !shows[show4K].UserData.IsFavorite {
		t.Fatalf("series grouped again: %+v", shows)
	}
	again := v.episodes(t, show4K)
	if len(again) != 4 {
		t.Fatalf("the group's episodes again: %+v", again)
	}
	for numbers, e := range grouped {
		if again[numbers].Id != e.Id {
			t.Errorf("episode %s is %s again, not %s", numbers, again[numbers].Id, e.Id)
		}
	}
	if d := again["1x2"].UserData; d.PlaybackPositionTicks != resume || d.Played {
		t.Errorf("the group's S1E2 again: %+v", d)
	}
	if d := again["1x3"].UserData; !d.Played || !d.IsFavorite {
		t.Errorf("the group's S1E3 again: %+v", d)
	}
	if d := again["1x1"].UserData; d.PlaybackPositionTicks != 0 || d.Played {
		t.Errorf("the group's S1E1 again: %+v", d)
	}
	if opened := v.open(t, first["1x2"].Id); opened.Id != grouped["1x2"].Id {
		t.Errorf("the first entry's S1E2 opens as %s %q", opened.Id, opened.Name)
	}
	// The group's former identifiers open its episodes again.
	if opened := v.open(t, grouped["1x2"].Id); opened.Id != grouped["1x2"].Id || opened.UserData.PlaybackPositionTicks != resume {
		t.Errorf("the group's S1E2 opens as %s %+v", opened.Id, opened.UserData)
	}
}

// Keeping the second of two entries apart ends their group: the first
// entry's own episodes have the group's data, and an episode only the
// second has gives its data to the second's, which the group's former
// episode opens as.
func TestIPTVSeriesKeptApartLeavesOneEntry(t *testing.T) {
	t.Parallel()
	v := newVersionsSetup(t)
	stremio := func(id string) string { return "polyfin-iptv:" + v.source.String() + ":" + id }
	show4K := library.TitleItemID(library.KindSeries, stremio("series:81")).String()
	show := library.TitleItemID(library.KindSeries, stremio("series:82")).String()
	v.list(t, "tvshows")
	v.group(t, true)
	grouped := v.episodes(t, show4K)
	at := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	v.change(t, grouped["1x2"].Id, func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 20*time.Minute, time.Hour, &at })
	v.change(t, grouped["1x3"].Id, func(d *userdata.Data) { d.MarkPlayed(&at, at); d.Favorite = true })

	if err := v.s.iptv.KeepApart(t.Context(), v.source, "series", "82", true); err != nil {
		t.Fatal(err)
	}
	if shows, _ := v.list(t, "tvshows"); len(shows) != 2 {
		t.Fatalf("series apart: %+v", shows)
	}
	first, second := v.episodes(t, show4K), v.episodes(t, show)
	if len(first) != 2 || len(second) != 4 || first["1x2"].Id == grouped["1x2"].Id {
		t.Fatalf("episodes apart: %+v %+v", first, second)
	}
	if d := first["1x2"].UserData; d.PlaybackPositionTicks != int64(20*time.Minute/100) {
		t.Errorf("the first entry's S1E2: %+v", d)
	}
	if d := second["1x3"].UserData; !d.Played || !d.IsFavorite {
		t.Errorf("the second entry's S1E3: %+v", d)
	}
	// The group's former S1E3, which no list shows any more, opens as the
	// second entry's, with the data.
	if opened := v.open(t, grouped["1x3"].Id); opened.Id != second["1x3"].Id || !opened.UserData.Played || !opened.UserData.IsFavorite {
		t.Errorf("the group's former S1E3 opens as %s %+v", opened.Id, opened.UserData)
	}
	_, _, entries, err := v.s.iptv.TitleEntries(t.Context(), stremio("series:81"))
	if err != nil || len(entries) != 2 || entries[0].Member || entries[0].Apart || !entries[1].Apart {
		t.Errorf("entries: %+v %v", entries, err)
	}
}

// An entry the provider leaves out of one refresh ends a group of two,
// whose first entry's own episodes have the group's data; listed again at
// the next, it forms the group again, with the data on the group's
// episodes, under the same identifiers.
func TestIPTVSeriesGroupOfTwoEndsWhileAnEntryIsUnlisted(t *testing.T) {
	t.Parallel()
	v := newVersionsSetup(t)
	stremio := func(id string) string { return "polyfin-iptv:" + v.source.String() + ":" + id }
	show4K := library.TitleItemID(library.KindSeries, stremio("series:81")).String()
	v.list(t, "tvshows")
	v.group(t, true)
	grouped := v.episodes(t, show4K)
	at := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	resume := int64(20 * time.Minute / 100)
	v.change(t, show4K, func(d *userdata.Data) { d.Favorite = true })
	v.change(t, grouped["1x2"].Id, func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 20*time.Minute, time.Hour, &at })
	v.change(t, grouped["1x3"].Id, func(d *userdata.Data) { d.MarkPlayed(&at, at) })
	if d := v.episodes(t, show4K)["1x2"].UserData; d.PlaybackPositionTicks != resume {
		t.Fatalf("the group's S1E2 before: %+v", d)
	}
	refresh := func() {
		t.Helper()
		if err := v.s.iptv.Refresh(t.Context(), addons.Shared(), v.source, false); err != nil {
			t.Fatal(err)
		}
	}

	v.provider.drop("82")
	refresh()
	shows, _ := v.list(t, "tvshows")
	if len(shows) != 1 || !shows[show4K].UserData.IsFavorite {
		t.Fatalf("series while the second entry is unlisted: %+v", shows)
	}
	alone := v.episodes(t, show4K)
	if len(alone) != 2 || alone["1x2"].Id == grouped["1x2"].Id || alone["1x2"].UserData.PlaybackPositionTicks != resume || alone["1x1"].UserData.Played {
		t.Errorf("the first entry's own episodes: %+v", alone)
	}
	if _, _, entries, err := v.s.iptv.TitleEntries(t.Context(), stremio("series:81")); err != nil || len(entries) != 1 || entries[0].Member {
		t.Errorf("entries while the second entry is unlisted: %+v %v", entries, err)
	}

	v.provider.restore("82")
	refresh()
	shows, _ = v.list(t, "tvshows")
	if len(shows) != 1 || !shows[show4K].UserData.IsFavorite {
		t.Fatalf("series grouped again: %+v", shows)
	}
	again := v.episodes(t, show4K)
	if len(again) != 4 {
		t.Fatalf("the group's episodes again: %+v", again)
	}
	for numbers, e := range grouped {
		if again[numbers].Id != e.Id {
			t.Errorf("episode %s is %s again, not %s", numbers, again[numbers].Id, e.Id)
		}
	}
	if d := again["1x2"].UserData; d.PlaybackPositionTicks != resume {
		t.Errorf("the group's S1E2 again: %+v", d)
	}
	if d := again["1x3"].UserData; !d.Played {
		t.Errorf("the group's S1E3 again: %+v", d)
	}
	if d := again["1x1"].UserData; d.Played || d.PlaybackPositionTicks != 0 {
		t.Errorf("the group's S1E1 again: %+v", d)
	}
}

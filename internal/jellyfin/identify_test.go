package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// remoteSearch is what jellyfin-web's Identify posts: the name and year,
// empty unless typed, and every identifier field, empty unless typed.
func remoteSearch(t *testing.T, s testServer, token, itemType string, name string, ids map[string]string) []RemoteSearchResult {
	t.Helper()
	fields := map[string]string{"Imdb": "", "Tmdb": "", "Tvdb": ""}
	for key, value := range ids {
		fields[key] = value
	}
	status, body := s.call(http.MethodPost, "/Items/RemoteSearch/"+itemType, app("dashboard", token),
		map[string]any{"SearchInfo": map[string]any{"Name": name, "Year": "", "ProviderIds": fields}, "IncludeDisabledProviders": true})
	var results []RemoteSearchResult
	if status != http.StatusOK || json.Unmarshal(body, &results) != nil {
		t.Fatalf("search %s %v: %d %s", itemType, ids, status, body)
	}
	return results
}

// Identify finds a title by an IMDb identifier alone, through the
// server's addons, of the item's kind only; by a TMDB or TVDB identifier
// through TMDB, with the server's key: without one, those find nothing
// and TMDB is never asked. Jellyfin 12.4's TMDB providers look the
// identifiers up before the name.
func TestIdentifyFindsTitlesByTheirIdentifiers(t *testing.T) {
	t.Parallel()
	s := newImageSearch(t)
	only := func(results []RemoteSearchResult, name, imdb string) bool {
		return len(results) == 1 && results[0].Name == name && results[0].ProviderIds["Imdb"] == imdb && results[0].SearchProviderName == "Alpha"
	}
	if results := remoteSearch(t, s.testServer, s.admin, "Series", "", map[string]string{"Imdb": "tt20"}); !only(results, "Show", "tt20") {
		t.Errorf("a series by its IMDb identifier: %+v", results)
	}
	if results := remoteSearch(t, s.testServer, s.admin, "Movie", "", map[string]string{"Imdb": "TT10"}); !only(results, "Searched", "tt10") {
		t.Errorf("a movie by its IMDb identifier: %+v", results)
	}
	if results := remoteSearch(t, s.testServer, s.admin, "Series", "", map[string]string{"Imdb": "tt10"}); len(results) != 0 {
		t.Errorf("a movie's identifier searched for a series: %+v", results)
	}

	// Without a key, TMDB and TVDB identifiers find nothing, asking TMDB
	// nothing.
	for _, ids := range []map[string]string{{"Tmdb": "77"}, {"Tvdb": "9077"}} {
		if results := remoteSearch(t, s.testServer, s.admin, "Series", "", ids); len(results) != 0 {
			t.Errorf("%v without a key: %+v", ids, results)
		}
	}
	if hits := s.hits(s.tmdbHits, "/3/tv/77/external_ids") + s.hits(s.tmdbHits, "/3/find/9077"); hits != 0 {
		t.Errorf("TMDB asked without a key: %d", hits)
	}

	s.setting(t, func(settings *accounts.Settings) { settings.TMDBKey = "k3y" })
	for _, ids := range []map[string]string{{"Tmdb": "77"}, {"Tvdb": "9077"}} {
		if results := remoteSearch(t, s.testServer, s.admin, "Series", "", ids); !only(results, "Show", "tt20") {
			t.Errorf("%v with a key: %+v", ids, results)
		}
	}
	if s.hits(s.tmdbHits, "/3/tv/77/external_ids") == 0 || s.hits(s.tmdbHits, "/3/find/9077") == 0 {
		t.Errorf("TMDB not asked: %v", s.tmdbHits)
	}
	// An identifier TMDB does not know finds nothing.
	if results := remoteSearch(t, s.testServer, s.admin, "Series", "", map[string]string{"Tvdb": "1"}); len(results) != 0 {
		t.Errorf("an unknown TVDB identifier: %+v", results)
	}
}

// finderAddon is a metadata addon whose series search finds Gull Point
// (2015), the wrong show of that name (tt0301), and Harbor Show (2011),
// tt0303. It describes them, and the right Gull Point, tt0302, which its
// search does not find, rated TV-MA, with a poster, a backdrop and a logo.
func finderAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	episodes := func(id string) []stremio.Video {
		return []stremio.Video{
			{ID: id + ":1:1", Title: "One", Season: 1, Episode: 1, Released: "2015-03-01T00:00:00.000Z"},
			{ID: id + ":1:2", Title: "Two", Season: 1, Episode: 2, Released: "2015-03-08T00:00:00.000Z"},
		}
	}
	metas := func() map[string]stremio.Meta {
		return map[string]stremio.Meta{
			"tt0301": {ID: "tt0301", Type: "series", Name: "Gull Point", ReleaseInfo: "2015", ImdbID: "tt0301", Videos: episodes("tt0301")},
			"tt0302": {ID: "tt0302", Type: "series", Name: "Gull Point", ReleaseInfo: "2015", ImdbID: "tt0302",
				Poster: server.URL + "/poster.jpg", Background: server.URL + "/backdrop.jpg", Logo: server.URL + "/logo.png",
				Extras: &stremio.Extras{Certification: "TV-MA"}, Videos: episodes("tt0302")},
			"tt0303": {ID: "tt0303", Type: "series", Name: "Harbor Show", ReleaseInfo: "2011", ImdbID: "tt0303", Videos: episodes("tt0303")},
		}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "finder", Name: "Finder", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "find", Name: "Find", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}}})
		case strings.HasPrefix(path, "/catalog/series/find/search="):
			query, _ := url.PathUnescape(strings.TrimPrefix(path, "/catalog/series/find/search="))
			var found []stremio.Meta
			for _, id := range []string{"tt0301", "tt0303"} {
				if meta := metas()[id]; strings.EqualFold(meta.Name, query) {
					meta.Videos = nil
					found = append(found, meta)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": found})
		case strings.HasPrefix(path, "/meta/series/"):
			if meta, ok := metas()[strings.TrimPrefix(path, "/meta/series/")]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
				return
			}
			http.NotFound(w, r)
		case path == "/poster.jpg" || path == "/backdrop.jpg" || path == "/logo.png":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// withFinder installs finderAddon on a plainSetup and signs an
// administrator in.
func withFinder(t *testing.T, p plainSetup) string {
	t.Helper()
	if _, err := p.addons.Install(t.Context(), addons.Shared(), finderAddon(t), false); err != nil {
		t.Fatal(err)
	}
	p.testServer.user("boss", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	return p.signIn("boss", "dashboard")
}

// seriesOf finds the series of a library a user sees by its IMDb
// identifier.
func (p plainSetup) seriesOf(t *testing.T, token, imdb string) (BaseItemDto, bool) {
	t.Helper()
	for _, series := range p.listed(t, token, p.showsLibrary, "Series") {
		var details BaseItemDto
		p.get(t, "/Users/"+p.user+"/Items/"+series.Id, p.token, &details)
		if details.ProviderIds != nil && (*details.ProviderIds)["Imdb"] == imdb {
			return details, true
		}
	}
	return BaseItemDto{}, false
}

// Identify applies to a show a search matched to the wrong title, as in
// Jellyfin, where it sets any item's provider identifiers: the whole show
// folder is linked to the title chosen, found by its IMDb identifier, its
// episodes and images with it. A title the addons alone list still cannot
// be bound to another. The title the show became is judged by parental
// control and hidden as any other.
func TestIdentifyRelinksAMatchedLocalShow(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	admin := withFinder(t, p)
	p.write(t, "Shows/Gull Point (2015)/Season 01/Gull Point S01E01.mp4", p.video)
	p.write(t, "Shows/Gull Point (2015)/Season 01/Gull Point S01E02.mp4", p.video)
	p.scan(t)
	wrong, ok := p.seriesOf(t, p.token, "tt0301")
	if !ok {
		t.Fatalf("Gull Point not matched to tt0301: %v", itemNames(p.listed(t, p.token, p.showsLibrary, "Series")))
	}
	// The member watched the first episode under the wrong title.
	var before QueryResult
	p.get(t, "/Shows/"+wrong.Id+"/Episodes?UserId="+p.user, p.token, &before)
	if len(before.Items) == 0 || *before.Items[0].IndexNumber != 1 {
		t.Fatalf("the wrong show's episodes: %v", itemNames(before.Items))
	}
	p.mark(t, http.MethodPost, "/UserPlayedItems/"+before.Items[0].Id)

	// The right show, which the search does not find, by its identifier.
	results := remoteSearch(t, p.testServer, admin, "Series", "", map[string]string{"Imdb": "tt0302"})
	if len(results) != 1 || results[0].ProviderIds["Imdb"] != "tt0302" {
		t.Errorf("the right show by its identifier: %+v", results)
	}
	chosen := map[string]any{"Name": "Gull Point", "ProviderIds": map[string]string{"Imdb": "tt0302"}, "SearchProviderName": "Finder"}
	if status, body := p.call(http.MethodPost, "/Items/RemoteSearch/Apply/"+p.movie+"?ReplaceAllImages=true", app("dashboard", admin),
		chosen); status != http.StatusInternalServerError {
		t.Errorf("applying to a title no local file holds: %d %s", status, body)
	}
	if status, body := p.call(http.MethodPost, "/Items/RemoteSearch/Apply/"+wrong.Id+"?ReplaceAllImages=true", app("dashboard", admin),
		chosen); status != http.StatusNoContent {
		t.Fatalf("applying to the matched show: %d %s", status, body)
	}
	if _, ok := p.seriesOf(t, p.token, "tt0301"); ok {
		t.Error("the wrong show is still listed")
	}
	right, ok := p.seriesOf(t, p.token, "tt0302")
	if !ok || right.ImageTags["Primary"] == "" || right.ImageTags["Logo"] == "" || len(right.BackdropImageTags) == 0 {
		t.Fatalf("the right show: %v %+v %v", ok, right.ImageTags, right.BackdropImageTags)
	}
	// jellyfin-web reopens the item it identified by its identifier: the
	// wrong show's answers the right one, its episodes too.
	var reopened BaseItemDto
	if status := p.get(t, "/Users/"+p.user+"/Items/"+wrong.Id, p.token, &reopened); status != http.StatusOK || reopened.Id != right.Id {
		t.Errorf("the wrong show reopened: %d %s, want %s", status, reopened.Id, right.Id)
	}
	var wrongEpisodes QueryResult
	p.get(t, "/Shows/"+wrong.Id+"/Episodes?UserId="+p.user, p.token, &wrongEpisodes)
	if len(wrongEpisodes.Items) > 0 {
		var episode BaseItemDto
		p.get(t, "/Users/"+p.user+"/Items/"+wrongEpisodes.Items[0].Id, p.token, &episode)
		if episode.SeriesId != right.Id {
			t.Errorf("the wrong show's episode reopened: %s %s, want %s", episode.Id, episode.SeriesId, right.Id)
		}
	}
	var episodes QueryResult
	p.get(t, "/Shows/"+right.Id+"/Episodes?UserId="+p.user, p.token, &episodes)
	if len(episodes.Items) != 2 {
		t.Fatalf("the right show's episodes: %v", itemNames(episodes.Items))
	}
	var episode BaseItemDto
	p.get(t, "/Users/"+p.user+"/Items/"+episodes.Items[0].Id, p.token, &episode)
	if episode.MediaSources == nil || !slices.ContainsFunc(*episode.MediaSources, func(s MediaSourceInfo) bool {
		return strings.HasPrefix(s.Name, "Local shows")
	}) {
		t.Errorf("the right show's episode plays no file: %+v", episode.MediaSources)
	}
	if data := p.userData(t, episodes.Items[0].Id); !data.Played {
		t.Errorf("the played episode's data did not follow its file: %+v", data)
	}
	// The link holds through a scan and a refresh.
	p.scan(t)
	if status, body := p.call(http.MethodPost, "/Items/"+right.Id+"/Refresh?MetadataRefreshMode=FullRefresh&ImageRefreshMode=FullRefresh",
		app("dashboard", admin), nil); status != http.StatusNoContent {
		t.Fatalf("refresh: %d %s", status, body)
	}
	if _, ok := p.seriesOf(t, p.token, "tt0302"); !ok {
		t.Error("the right show lost after a scan and a refresh")
	}

	// TV-MA is kept from a child allowed up to TV-PG.
	child := p.testServer.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(10)} })
	childToken := p.signIn("child", "tv")
	var item BaseItemDto
	if status := p.get(t, "/Users/"+child.ID.String()+"/Items/"+right.Id, childToken, &item); status != http.StatusNotFound {
		t.Errorf("the child opens the TV-MA show: %d", status)
	}
	if slices.ContainsFunc(p.listed(t, childToken, p.showsLibrary, "Series"), func(i BaseItemDto) bool { return i.Id == right.Id }) {
		t.Error("the child lists the TV-MA show")
	}
	if status, data := p.call(http.MethodPost, "/Polyfin/UserItems/"+right.Id+"/Hidden", app("tv", p.token), nil); status/100 != 2 {
		t.Fatalf("hiding: %d %s", status, data)
	}
	if slices.ContainsFunc(p.listed(t, p.token, p.showsLibrary, "Series"), func(i BaseItemDto) bool { return i.Id == right.Id }) {
		t.Error("a hidden show is listed")
	}
}

// An IMDb identifier typed in Edit metadata for a plain series links its
// show folder to that title, as Identify does: Jellyfin keeps the
// identifiers typed.
func TestEditMetadataIMDbLinksALocalTitle(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	admin := withFinder(t, p)
	_, plainSeries, _ := p.titles(t)
	if status, body := p.call(http.MethodPost, "/Items/"+plainSeries, app("dashboard", admin),
		editorBody(plainSeries, map[string]any{"Name": "Harbor Show", "ProviderIds": map[string]string{"Imdb": "tt0302", "Tvdb": ""}})); status != http.StatusNoContent {
		t.Fatalf("edit: %d %s", status, body)
	}
	right, ok := p.seriesOf(t, p.token, "tt0302")
	if !ok {
		t.Fatalf("the show is not linked: %v", itemNames(p.listed(t, p.token, p.showsLibrary, "Series")))
	}
	var item BaseItemDto
	if status := p.get(t, "/Users/"+p.user+"/Items/"+plainSeries, p.token, &item); status != http.StatusOK || item.Id != right.Id {
		t.Errorf("the plain series reopened: %d %s, want %s", status, item.Id, right.Id)
	}
}

// Refresh metadata matches a plain video's files again, as Jellyfin reads
// an item's files again: a show the metadata addons find now is matched.
func TestRefreshMatchesAPlainVideoAgain(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	_, plainSeries, _ := p.titles(t)
	// The addon's search finds Harbor Show (2011) from now on.
	admin := withFinder(t, p)
	if status, body := p.call(http.MethodPost, "/Items/"+plainSeries+"/Refresh?Recursive=true&MetadataRefreshMode=FullRefresh&ImageRefreshMode=FullRefresh",
		app("dashboard", admin), nil); status != http.StatusNoContent {
		t.Fatalf("refresh: %d %s", status, body)
	}
	matched, ok := p.seriesOf(t, p.token, "tt0303")
	if !ok {
		t.Fatalf("the show is not matched: %v", itemNames(p.listed(t, p.token, p.showsLibrary, "Series")))
	}
	var item BaseItemDto
	if status := p.get(t, "/Users/"+p.user+"/Items/"+plainSeries, p.token, &item); status != http.StatusOK || item.Id != matched.Id {
		t.Errorf("the plain series reopened: %d %s, want %s", status, item.Id, matched.Id)
	}
}

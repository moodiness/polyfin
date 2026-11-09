package jellyfin

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/trackers"
)

// describedIDsAddon serves, for "tt" identifiers only, descriptions
// trimmed from a metadata addon's, which give TMDB's and TVDB's
// identifiers as moviedb_id and tvdb_id: a movie, a series, "IT" (1990),
// whose moviedb_id is its TVDB identifier copied, and a series another
// addon describes with _tmdbId.
func describedIDsAddon(t *testing.T) string {
	t.Helper()
	videos := func(id string) string {
		return fmt.Sprintf(`[{"id":"%[1]s:1:1","title":"%[1]s one","season":1,"episode":1,"released":"2001-01-01T00:00:00.000Z"},
			{"id":"%[1]s:1:2","title":"%[1]s two","season":1,"episode":2,"released":"2001-01-08T00:00:00.000Z"}]`, id)
	}
	metas := map[string]string{
		"movie/tt0111161": `{"id":"tt0111161","imdb_id":"tt0111161","type":"movie","name":"The Shawshank Redemption","moviedb_id":278}`,
		"series/tt0239195": `{"id":"tt0239195","imdb_id":"tt0239195","type":"series","name":"Survivor","moviedb_id":14658,"tvdb_id":76733,
			"videos":` + videos("tt0239195") + `}`,
		"series/tt0099864": `{"id":"tt0099864","imdb_id":"tt0099864","type":"series","name":"IT","moviedb_id":113981,"tvdb_id":"113981",
			"videos":` + videos("tt0099864") + `}`,
		"series/tt0386676": `{"id":"tt0386676","imdb_id":"tt0386676","type":"series","name":"Described","_tmdbId":"2316","moviedb_id":9,
			"videos":` + videos("tt0386676") + `}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch path {
		case "/manifest.json":
			_, _ = io.WriteString(w, `{"id":"ids","name":"Ids","version":"1","types":["movie","series"],"idPrefixes":["tt"],
				"resources":["catalog","meta"],"catalogs":[{"type":"movie","id":"top","name":"Top"},{"type":"series","id":"shows","name":"Shows"}]}`)
			return
		case "/catalog/movie/top.json":
			_, _ = io.WriteString(w, `{"metas":[`+metas["movie/tt0111161"]+`]}`)
			return
		case "/catalog/series/shows.json":
			_, _ = io.WriteString(w, `{"metas":[`+metas["series/tt0239195"]+`,`+metas["series/tt0099864"]+`,`+metas["series/tt0386676"]+`]}`)
			return
		}
		if meta, ok := metas[strings.TrimSuffix(strings.TrimPrefix(path, "/meta/"), ".json")]; ok {
			_, _ = io.WriteString(w, `{"meta":`+meta+`}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

type describedIDs struct {
	testServer
	token string
	user  accounts.User
	// titles are the movie's and series' identifiers by name, and
	// episodes, those of each series' second episode.
	titles, episodes map[string]string
}

// describingIDs lists the addon's catalogs as libraries on s, with
// PublicMetaDB the segment database asked, and signs a member in.
func describingIDs(t *testing.T, s testServer) describedIDs {
	t.Helper()
	s.handler.Segments = mediasegments.New(s.pool, []mediasegments.Source{{Name: mediasegments.PublicMetaDB, URL: "http://127.0.0.1:9"}}, "test",
		s.handler.Logger, s.store.Settings)
	user := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), describedIDsAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	d := describedIDs{testServer: s, token: s.signIn("member", "tv"), user: user, titles: map[string]string{}, episodes: map[string]string{}}
	var views QueryResult
	s.get(t, "/UserViews", d.token, &views)
	for _, view := range views.Items {
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, d.token, &page)
		for _, item := range page.Items {
			if item.Type != "Movie" && item.Type != "Series" {
				continue
			}
			d.titles[item.Name] = item.Id
			if item.Type != "Series" {
				continue
			}
			var episodes QueryResult
			s.get(t, "/Shows/"+item.Id+"/Episodes", d.token, &episodes)
			for _, episode := range episodes.Items {
				if episode.IndexNumber != nil && *episode.IndexNumber == 2 {
					d.episodes[item.Name] = episode.Id
				}
			}
		}
	}
	if len(d.titles) != 4 || len(d.episodes) != 3 {
		t.Fatalf("titles %v, episodes %v", d.titles, d.episodes)
	}
	return d
}

// Apps see the TMDB and TVDB identifiers a description gives as
// moviedb_id and tvdb_id, and their links; "IT" (1990), whose moviedb_id
// is its TVDB identifier copied, shows its TVDB identifier only.
func TestAppsSeeDescribedTMDBAndTVDBIdentifiers(t *testing.T) {
	t.Parallel()
	d := describingIDs(t, newTestServer(t, 10))
	for name, want := range map[string]struct {
		ids  map[string]string
		urls []string
	}{
		"The Shawshank Redemption": {map[string]string{"Imdb": "tt0111161", "Tmdb": "278"},
			[]string{"https://www.imdb.com/title/tt0111161", "https://www.themoviedb.org/movie/278"}},
		"Survivor": {map[string]string{"Imdb": "tt0239195", "Tmdb": "14658", "Tvdb": "76733"},
			[]string{"https://www.imdb.com/title/tt0239195", "https://www.themoviedb.org/tv/14658", "https://thetvdb.com/?tab=series&id=76733"}},
		"IT": {map[string]string{"Imdb": "tt0099864", "Tvdb": "113981"},
			[]string{"https://www.imdb.com/title/tt0099864", "https://thetvdb.com/?tab=series&id=113981"}},
		"Described": {map[string]string{"Imdb": "tt0386676", "Tmdb": "2316"},
			[]string{"https://www.imdb.com/title/tt0386676", "https://www.themoviedb.org/tv/2316"}},
	} {
		var item BaseItemDto
		if status := d.get(t, "/Items/"+d.titles[name], d.token, &item); status != http.StatusOK || item.ProviderIds == nil ||
			item.ExternalUrls == nil {
			t.Fatalf("%s: %d %+v", name, status, item)
		}
		if fmt.Sprint(*item.ProviderIds) != fmt.Sprint(want.ids) {
			t.Errorf("%s: provider identifiers %v, want %v", name, *item.ProviderIds, want.ids)
		}
		var urls []string
		for _, link := range *item.ExternalUrls {
			urls = append(urls, link.Url)
		}
		if !slices.Equal(urls, want.urls) {
			t.Errorf("%s: links %v, want %v", name, urls, want.urls)
		}
	}
}

// The segment databases are asked about a title by the TMDB identifier its
// description gives as moviedb_id. PublicMetaDB, which knows episodes by
// TMDB's numbers, is not asked about an episode of a series whose TMDB
// identifier is its moviedb_id while nothing tells TMDB's numbering of
// it; it is once something does, and for a series another addon gives
// _tmdbId, as before.
func TestPublicMetaDBIsNotAskedForEpisodesNumberedByAGuess(t *testing.T) {
	t.Parallel()
	d := describingIDs(t, newTestServer(t, 10))
	d.setting(t, func(s *accounts.Settings) {
		s.PublicMetaDBKey = "pm-Good4mN2pQrS7tUvWx3yZaB4cD5eF6gH7iJ8kL9mN0oP1qR2sT3uV4wXyZ5aB"
	})
	title := func(id string) mediasegments.Title {
		t.Helper()
		item, err := d.library.Item(t.Context(), d.user, mustID(t, id))
		if err != nil {
			t.Fatal(err)
		}
		title, err := d.handler.segmentTitle(t.Context(), d.user, item)
		if err != nil {
			t.Fatal(err)
		}
		return title
	}
	if movie := title(d.titles["The Shawshank Redemption"]); movie.TMDB != "278" || movie.IMDb != "tt0111161" {
		t.Errorf("the movie: %+v", movie)
	}
	numbers := func(name string) string {
		t.Helper()
		episode := title(d.episodes[name])
		return fmt.Sprintf("%s S%dE%d [%d %d]", episode.TMDB, episode.Season, episode.Episode, episode.TMDBSeason, episode.TMDBEpisode)
	}
	if got := numbers("Survivor"); got != "14658 S1E2 [0 -1]" {
		t.Errorf("by moviedb_id, unnumbered: %s", got)
	}
	if got := numbers("Described"); got != "2316 S1E2 [0 0]" {
		t.Errorf("by _tmdbId: %s", got)
	}
	// TMDB's numbering kept for the series: the addon's S1E2 is TMDB's
	// S1E3.
	if _, err := d.pool.Exec(t.Context(), `INSERT INTO episode_alignments (series, tmdb_id, episodes, places, asked_at)
		VALUES ('tt0239195', 14658, 2, '[[1,1,1,1],[1,3,1,2]]', now())`); err != nil {
		t.Fatal(err)
	}
	if got := numbers("Survivor"); got != "14658 S1E2 [1 3]" {
		t.Errorf("by moviedb_id, numbered: %s", got)
	}
}

// The tracking services hear of a title by the TMDB and TVDB identifiers
// its description gives as moviedb_id and tvdb_id: PublicMetaDB by the
// TMDB one, without looking it up by IMDb's. "IT" (1990), whose
// moviedb_id is left out, is looked up as before.
func TestTrackingServicesHearOfDescribedIdentifiers(t *testing.T) {
	t.Parallel()
	fakes := newTrackerFakes(t)
	var tracker *trackers.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.MDBList: fakes.url + "/mdblist", trackers.PublicMetaDB: fakes.url + "/publicmetadb"}})
		options.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	d := describingIDs(t, s)
	for service, key := range map[string]string{trackers.MDBList: "mdblist-member", trackers.PublicMetaDB: "pm-member"} {
		if _, err := tracker.ConnectKey(t.Context(), d.user.ID, service, key); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{d.titles["The Shawshank Redemption"], d.episodes["Survivor"], d.episodes["IT"]} {
		if status, body := d.call(http.MethodPost, "/UserPlayedItems/"+id, app("tv", d.token), nil); status != http.StatusOK {
			t.Fatalf("marking %s played: %d %s", id, status, body)
		}
	}
	var got []string
	for _, call := range fakes.wait(t, 6) {
		got = append(got, call.summary())
	}
	slices.Sort(got)
	want := []string{
		`POST /mdblist/sync/watched {"movies":[{"ids":{"imdb":"tt0111161","tmdb":278}}]}`,
		`POST /mdblist/sync/watched {"shows":[{"ids":{"imdb":"tt0099864","tvdb":113981},"seasons":[{"episodes":[{"number":2}],"number":1}]}]}`,
		`POST /mdblist/sync/watched {"shows":[{"ids":{"imdb":"tt0239195","tmdb":14658,"tvdb":76733},"seasons":[{"episodes":[{"number":2}],"number":1}]}]}`,
		// The fake looks every title up as TMDB's series 30.
		`POST /publicmetadb/api/external/watched {"episode":2,"media_type":"tv","season":1,"tmdb_id":14658}`,
		`POST /publicmetadb/api/external/watched {"episode":2,"media_type":"tv","season":1,"tmdb_id":30}`,
		`POST /publicmetadb/api/external/watched {"media_type":"movie","tmdb_id":278}`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("sent:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

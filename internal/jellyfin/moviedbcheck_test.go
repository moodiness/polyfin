package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/tmdb"
	"github.com/moodiness/polyfin/internal/trackers"
)

// checkedTMDB is a fake TMDB whose find gives the series of two IMDb
// identifiers: "The Last of Us" (tt3581920) is its series 100088, "IT"
// (1990, tt0099864) its series 19614. It answers 404 to anything else,
// and records the paths asked.
type checkedTMDB struct {
	*httptest.Server
	mu    sync.Mutex
	asked []string
}

func newCheckedTMDB(t *testing.T) *checkedTMDB {
	t.Helper()
	series := map[string]int{"tt3581920": 100088, "tt0099864": 19614}
	f := &checkedTMDB{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.asked = append(f.asked, r.URL.Path)
		f.mu.Unlock()
		imdb, ok := strings.CutPrefix(r.URL.Path, "/3/find/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		found := []any{}
		if id, ok := series[imdb]; ok {
			found = append(found, map[string]any{"id": id})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": []any{}, "tv_results": found})
	}))
	t.Cleanup(f.Close)
	return f
}

// requests returns the paths the fake was asked.
func (f *checkedTMDB) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// checkedAddon serves, for "tt" identifiers only, descriptions trimmed
// from a metadata addon's, whose moviedb_id is wrong: "The Last of Us"
// gives 502119, which TMDB does not know as its series, and "IT" (1990)
// gives 113981, another show's.
func checkedAddon(t *testing.T) string {
	t.Helper()
	videos := func(id string) string {
		return fmt.Sprintf(`[{"id":"%[1]s:1:1","title":"%[1]s one","season":1,"episode":1,"released":"2001-01-01T00:00:00.000Z"},
			{"id":"%[1]s:1:2","title":"%[1]s two","season":1,"episode":2,"released":"2001-01-08T00:00:00.000Z"}]`, id)
	}
	metas := map[string]string{
		"series/tt3581920": `{"id":"tt3581920","imdb_id":"tt3581920","type":"series","name":"The Last of Us","releaseInfo":"2023-",
			"moviedb_id":502119,"videos":` + videos("tt3581920") + `}`,
		"series/tt0099864": `{"id":"tt0099864","imdb_id":"tt0099864","type":"series","name":"IT","releaseInfo":"1990",
			"moviedb_id":113981,"videos":` + videos("tt0099864") + `}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch path {
		case "/manifest.json":
			_, _ = io.WriteString(w, `{"id":"checked","name":"Checked","version":"1","types":["series"],"idPrefixes":["tt"],
				"resources":["catalog","meta"],"catalogs":[{"type":"series","id":"shows","name":"Shows"}]}`)
			return
		case "/catalog/series/shows.json":
			_, _ = io.WriteString(w, `{"metas":[`+metas["series/tt3581920"]+`,`+metas["series/tt0099864"]+`]}`)
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

// checkedEnv is a server whose library lists checkedAddon's series, with
// a TMDB client on checkedTMDB and its queue of background lookups.
type checkedEnv struct {
	testServer
	tmdb  *checkedTMDB
	queue *tmdb.Queue
	token string
	user  accounts.User
	// titles are the series' identifiers by name, and episodes, those
	// of each series' second episode.
	titles, episodes map[string]string
}

// newCheckedEnv starts the server with key as the TMDB key, configure
// completing its options, lists the addon's catalog as a library, and
// signs a member in, who lists the series and their episodes.
func newCheckedEnv(t *testing.T, key string, configure ...func(*Options, *pgxpool.Pool)) checkedEnv {
	t.Helper()
	fake := newCheckedTMDB(t)
	var queue *tmdb.Queue
	configure = append(configure, func(o *Options, _ *pgxpool.Pool) {
		client := tmdb.New(fake.URL+"/3", fake.URL+"/t/p/", "test", o.Accounts.Settings)
		o.Library.UseTMDB(client)
		// The passes wait an hour: no test runs one.
		queue = tmdb.NewQueue(client, tmdb.QueueOptions{Interval: time.Millisecond, PassCheck: time.Hour})
		t.Cleanup(queue.Close)
		o.Library.UseTitleCredits(queue)
	})
	s := newProbingServer(t, 10, "ffprobe-not-installed", configure...)
	s.setting(t, func(settings *accounts.Settings) { settings.TMDBKey = key })
	user := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), checkedAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	e := checkedEnv{testServer: s, tmdb: fake, queue: queue, token: s.signIn("member", "tv"), user: user,
		titles: map[string]string{}, episodes: map[string]string{}}
	var views QueryResult
	s.get(t, "/UserViews", e.token, &views)
	for _, view := range views.Items {
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, e.token, &page)
		for _, item := range page.Items {
			if item.Type != "Series" {
				continue
			}
			e.titles[item.Name] = item.Id
			var episodes QueryResult
			s.get(t, "/Shows/"+item.Id+"/Episodes", e.token, &episodes)
			for _, episode := range episodes.Items {
				if episode.IndexNumber != nil && *episode.IndexNumber == 2 {
					e.episodes[item.Name] = episode.Id
				}
			}
		}
	}
	if len(e.titles) != 2 || len(e.episodes) != 2 {
		t.Fatalf("titles %v, episodes %v", e.titles, e.episodes)
	}
	return e
}

// settle waits until the queue has made every lookup asked.
func (e checkedEnv) settle(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := e.queue.Settled(ctx); err != nil {
		t.Fatal(err)
	}
}

// page opens a series' page, and returns its provider identifiers and
// links.
func (e checkedEnv) page(t *testing.T, name string) (map[string]string, []string) {
	t.Helper()
	var item BaseItemDto
	if status := e.get(t, "/Items/"+e.titles[name], e.token, &item); status != http.StatusOK || item.ProviderIds == nil ||
		item.ExternalUrls == nil {
		t.Fatalf("%s: %d %+v", name, status, item)
	}
	var urls []string
	for _, link := range *item.ExternalUrls {
		urls = append(urls, link.Url)
	}
	return *item.ProviderIds, urls
}

// While a TMDB key is saved, a series whose description gives a wrong
// TMDB identifier as moviedb_id shows TMDB's once checked, on its page
// and in its link: "The Last of Us", whose 502119 TMDB does not know as
// its series, shows 100088, and "IT" (1990), whose 113981 is another
// show's, shows 19614. Each was checked by one request to TMDB's find.
func TestAppsSeeCheckedMoviedbIdentifiers(t *testing.T) {
	t.Parallel()
	e := newCheckedEnv(t, "k3y")
	e.settle(t)
	for name, want := range map[string]struct {
		ids  map[string]string
		urls []string
	}{
		"The Last of Us": {map[string]string{"Imdb": "tt3581920", "Tmdb": "100088"},
			[]string{"https://www.imdb.com/title/tt3581920", "https://www.themoviedb.org/tv/100088"}},
		"IT": {map[string]string{"Imdb": "tt0099864", "Tmdb": "19614"},
			[]string{"https://www.imdb.com/title/tt0099864", "https://www.themoviedb.org/tv/19614"}},
	} {
		ids, urls := e.page(t, name)
		if fmt.Sprint(ids) != fmt.Sprint(want.ids) {
			t.Errorf("%s: provider identifiers %v, want %v", name, ids, want.ids)
		}
		if !slices.Equal(urls, want.urls) {
			t.Errorf("%s: links %v, want %v", name, urls, want.urls)
		}
	}
	finds := slices.DeleteFunc(e.tmdb.requests(), func(path string) bool { return !strings.HasPrefix(path, "/3/find/") })
	slices.Sort(finds)
	if !slices.Equal(finds, []string{"/3/find/tt0099864", "/3/find/tt3581920"}) {
		t.Errorf("TMDB's find was asked %v", finds)
	}
}

// The tracking services hear of a series whose moviedb_id was wrong by
// TMDB's identifier once checked: PublicMetaDB is told of an episode of
// "The Last of Us" as one of TMDB's series 100088, not 502119.
func TestTrackingServicesHearOfCheckedMoviedbIdentifiers(t *testing.T) {
	t.Parallel()
	fakes := newTrackerFakes(t)
	var tracker *trackers.Service
	e := newCheckedEnv(t, "k3y", func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.MDBList: fakes.url + "/mdblist", trackers.PublicMetaDB: fakes.url + "/publicmetadb"}})
		options.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	e.settle(t)
	for service, key := range map[string]string{trackers.MDBList: "mdblist-member", trackers.PublicMetaDB: "pm-member"} {
		if _, err := tracker.ConnectKey(t.Context(), e.user.ID, service, key); err != nil {
			t.Fatal(err)
		}
	}
	id := e.episodes["The Last of Us"]
	if status, body := e.call(http.MethodPost, "/UserPlayedItems/"+id, app("tv", e.token), nil); status != http.StatusOK {
		t.Fatalf("marking %s played: %d %s", id, status, body)
	}
	var got []string
	for _, call := range fakes.wait(t, 2) {
		got = append(got, call.summary())
	}
	slices.Sort(got)
	want := []string{
		`POST /mdblist/sync/watched {"shows":[{"ids":{"imdb":"tt3581920","tmdb":100088},"seasons":[{"episodes":[{"number":2}],"number":1}]}]}`,
		`POST /publicmetadb/api/external/watched {"episode":2,"media_type":"tv","season":1,"tmdb_id":100088}`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("sent:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Without a TMDB key, nothing is checked: the series show the TMDB
// identifiers their descriptions give as moviedb_id, and TMDB is never
// asked.
func TestMoviedbIdentifiersWithoutAKeyAreNotChecked(t *testing.T) {
	t.Parallel()
	e := newCheckedEnv(t, "")
	for name, want := range map[string]string{"The Last of Us": "502119", "IT": "113981"} {
		ids, urls := e.page(t, name)
		if ids["Tmdb"] != want || !slices.Contains(urls, "https://www.themoviedb.org/tv/"+want) {
			t.Errorf("%s: provider identifiers %v, links %v, want the TMDB identifier %s", name, ids, urls, want)
		}
	}
	e.settle(t)
	if asked := e.tmdb.requests(); len(asked) != 0 {
		t.Errorf("TMDB was asked %v without a key", asked)
	}
}

package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
	"github.com/moodiness/polyfin/internal/userdata"
)

// listedAddon serves a catalog of movies, each with three actors and a
// director, and one of series of episodes episodes each, with two actors.
func listedAddon(t *testing.T, movies, series, episodes int) string {
	t.Helper()
	cast := func(title string, count int) *stremio.Extras {
		extras := &stremio.Extras{Certification: "PG"}
		for i := range count {
			extras.Cast = append(extras.Cast, stremio.CastMember{Name: fmt.Sprintf("%s actor %d", title, i), Character: "Role"})
		}
		return extras
	}
	movie := func(i int) stremio.Meta {
		id := fmt.Sprintf("tt71%03d", i)
		return stremio.Meta{ID: id, Type: "movie", Name: fmt.Sprintf("Movie %d", i), ReleaseInfo: "2020", Runtime: "1h 30min",
			Released: fmt.Sprintf("2020-01-%02dT00:00:00Z", i%28+1), TmdbID: stremio.Text(fmt.Sprint(7100 + i)), Genres: []string{"Drama"},
			Director: []string{"Director " + id}, Extras: cast(id, 3)}
	}
	show := func(i int, videos bool) stremio.Meta {
		id := fmt.Sprintf("tt72%03d", i)
		meta := stremio.Meta{ID: id, Type: "series", Name: fmt.Sprintf("Show %d", i), ReleaseInfo: "2020-", Runtime: "45min",
			Genres: []string{"Drama"}, Extras: cast(id, 2)}
		if videos {
			for e := range episodes {
				meta.Videos = append(meta.Videos, stremio.Video{ID: fmt.Sprintf("%s:1:%d", id, e+1), Title: fmt.Sprintf("Episode %d", e+1),
					Season: 1, Episode: stremio.Number(e + 1), Released: fmt.Sprintf("2020-01-%02dT00:00:00Z", e+1), Runtime: "45min"})
			}
		}
		return meta
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "listed", Name: "Listed", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "movies", Name: "Movies"}, {Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/movie/movies"):
			metas := make([]stremio.Meta, movies)
			for i := range metas {
				metas[i] = movie(i)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			metas := make([]stremio.Meta, series)
			for i := range metas {
				metas[i] = show(i, false)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/movie/tt71"):
			var i int
			_, _ = fmt.Sscanf(strings.TrimPrefix(path, "/meta/movie/tt71"), "%03d", &i)
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie(i)})
		case strings.HasPrefix(path, "/meta/series/tt72"):
			var i int
			_, _ = fmt.Sscanf(strings.TrimPrefix(path, "/meta/series/tt72"), "%03d", &i)
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show(i, true)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// Continue Watching, Next Up, a library's latest titles and a page of a
// library describe each title from its record, its series and its people:
// once they are kept, each sends a fixed number of statements, the series,
// credits and people of all its items read at once, never one per item.
func TestListingStatementsStayWithinABudget(t *testing.T) {
	t.Parallel()
	const movies, series, episodes = 16, 16, 4
	// TMDB answers nothing: the credits kept below are all it knows.
	api := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(api.Close)
	s := newProbingServer(t, 10, "ffprobe-not-installed")
	s.setting(t, func(settings *accounts.Settings) { settings.TMDBKey = "k3y" })
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), listedAddon(t, movies, series, episodes), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "movies"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	views := s.viewIDs(t, token)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views["Movies"], token, &page)
	if len(page.Items) != movies {
		t.Fatalf("%d movies", len(page.Items))
	}
	report := func(item string, position int64) {
		t.Helper()
		if status, body := s.call(http.MethodPost, "/Sessions/Playing/Stopped", app("tv", token),
			map[string]any{"ItemId": item, "PositionTicks": position}); status != http.StatusNoContent {
			t.Fatalf("stopped: %d %s", status, body)
		}
	}
	// Every movie half watched, TMDB crediting every other one.
	for i, movie := range page.Items {
		report(movie.Id, 27_000_000_000)
		if i%2 == 0 {
			if _, err := s.pool.Exec(t.Context(), `INSERT INTO title_credits (kind, provider, id, tmdb_id, language, credits, titles, asked_at)
				VALUES ('movie', 'tmdb', $1, $2, $3, $4, ARRAY[$5::uuid], now())`, fmt.Sprint(7100+i), 7100+i, s.store.Settings().Language,
				fmt.Sprintf(`[{"id": %d, "name": "Credited %d", "kind": "Actor", "role": "Lead"}]`, 9100+i, i), movie.Id); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Every series' first episode watched, its second half watched.
	s.get(t, "/Items?ParentId="+views["Shows"], token, &page)
	if len(page.Items) != series {
		t.Fatalf("%d series", len(page.Items))
	}
	for _, show := range page.Items {
		var shown QueryResult
		s.get(t, "/Shows/"+show.Id+"/Episodes", token, &shown)
		if status, body := s.call(http.MethodPost, "/UserPlayedItems/"+shown.Items[0].Id, app("tv", token), nil); status != http.StatusOK {
			t.Fatalf("played: %d %s", status, body)
		}
		report(shown.Items[1].Id, 13_500_000_000)
	}

	// A restart, whose library and users' data count their statements.
	counter := &statementCounter{}
	config := s.pool.Config()
	config.ConnConfig.Tracer = counter
	counted, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(counted.Close)
	options := s.handler.Options
	options.Library = library.New(counted, s.addons, options.Stremio, options.Logger, s.store.Settings)
	options.Library.SetFollowUps()
	client := tmdb.New(api.URL+"/3", api.URL+"/t/p/", "test", s.store.Settings)
	options.Library.UseTMDB(client)
	// The passes of titles' credits wait an hour: none runs here.
	queue := tmdb.NewQueue(client, tmdb.QueueOptions{Interval: time.Millisecond, PassCheck: time.Hour})
	t.Cleanup(queue.Close)
	options.Library.UseTitleCredits(queue)
	options.UserData = userdata.New(counted)
	server := httptest.NewServer(New(options))
	t.Cleanup(server.Close)
	restarted := s
	restarted.url = server.URL
	// Each budget is the statements measured, and three more: describing
	// each item with one more statement adds 16.
	for _, request := range []struct {
		name, path string
		items      int
		budget     int64
	}{
		{"Continue Watching", "/UserItems/Resume?Limit=100", movies + series, 10},
		{"Next Up", "/Shows/NextUp?Limit=100", series, 10},
		{"Latest movies", "/Items/Latest?Limit=100&ParentId=" + views["Movies"], movies, 7},
		{"Latest shows", "/Items/Latest?Limit=100&ParentId=" + views["Shows"], series, 10},
		{"A page of movies", "/Items?ParentId=" + views["Movies"], movies, 7},
		{"A page of shows", "/Items?ParentId=" + views["Shows"], series, 10},
	} {
		// The first request after the restart reads the addons'
		// descriptions and stores what it lists; the second only reads.
		var listed []BaseItemDto
		for range 2 {
			counter.reset()
			listed = nil
			var status int
			if strings.HasPrefix(request.path, "/Items/Latest") {
				status = restarted.get(t, request.path, token, &listed)
			} else {
				var page QueryResult
				status = restarted.get(t, request.path, token, &page)
				listed = page.Items
			}
			if status != http.StatusOK {
				t.Fatalf("%s: %d", request.name, status)
			}
		}
		if len(listed) != request.items {
			t.Fatalf("%s: %d items", request.name, len(listed))
		}
		n := counter.statements.Load()
		t.Logf("%s: %d statements for %d items", request.name, n, len(listed))
		if n > request.budget {
			t.Errorf("%s: %d statements for %d items, beyond %d; the most sent:\n%s", request.name, n, len(listed), request.budget,
				counter.mostSent(8))
		}
	}
}

package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// seriesProvider is an Xtream account (user / secret) of count series,
// each of seasons seasons of episodes episodes, and specials (season 0)
// of one episode.
func seriesProvider(t *testing.T, count, seasons, episodes int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/player_api.php" || query.Get("username") != "user" || query.Get("password") != "secret" {
			http.NotFound(w, r)
			return
		}
		var answer any
		switch query.Get("action") {
		case "":
			answer = map[string]any{"user_info": map[string]any{"auth": 1, "allowed_output_formats": []string{"ts"}}}
		case "get_series_categories":
			answer = []any{map[string]any{"category_id": "2", "category_name": "Drama"}}
		case "get_series":
			list := make([]any, count)
			for i := range list {
				list[i] = map[string]any{"name": fmt.Sprintf("Show %02d", i), "series_id": 100 + i, "category_id": "2", "last_modified": "1"}
			}
			answer = list
		case "get_series_info":
			id, _ := strconv.Atoi(query.Get("series_id"))
			all := map[string]any{"0": []any{map[string]any{"id": fmt.Sprintf("%d00", id), "episode_num": 1, "season": 0,
				"title": "Special", "container_extension": "mkv"}}}
			for season := 1; season <= seasons; season++ {
				list := make([]any, episodes)
				for e := range list {
					list[e] = map[string]any{"id": fmt.Sprintf("%d%d%02d", id, season, e+1), "episode_num": e + 1, "season": season,
						"title": fmt.Sprintf("S%02dE%02d", season, e+1), "container_extension": "mkv",
						"info": map[string]any{"releasedate": "2020-01-01"}}
				}
				all[strconv.Itoa(season)] = list
			}
			answer = map[string]any{"info": map[string]any{"plot": "A show."}, "episodes": all}
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// shows is a test server with series named "Show N", from an Xtream
// source or an addon, and a member signed in.
type shows struct {
	testServer
	member accounts.User
	token  string
	// library is the series' library.
	library string
	// series are the series' identifiers by name.
	series map[string]string
}

// newIPTVShows serves count series "Show NN" from an Xtream source, of
// seasons seasons of episodes episodes, and a special each (see
// seriesProvider).
func newIPTVShows(t *testing.T, count, seasons, episodes int) shows {
	t.Helper()
	s := newTestServer(t, 10)
	on := true
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: seriesProvider(t, count, seasons, episodes), Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Series: &on}}, false); err != nil {
		t.Fatal(err)
	}
	return listedShows(t, s, count, func(token string) string { return s.viewsByType(t, token)["tvshows"] })
}

// newAddonShows serves count series "Show N" from an addon, of one
// season of episodes episodes (see watchedSeriesAddon). Unlike an Xtream
// source's, their descriptions are not paced.
func newAddonShows(t *testing.T, count, episodes int) shows {
	t.Helper()
	s := newTestServer(t, 10)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), watchedSeriesAddon(t, count, episodes), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	return listedShows(t, s, count, func(token string) string { return s.viewIDs(t, token)["Shows"] })
}

// listedShows signs the member in and lists the count series of the
// library view gives.
func listedShows(t *testing.T, s testServer, count int, view func(token string) string) shows {
	t.Helper()
	x := shows{testServer: s, member: s.user("member", nil), series: map[string]string{}}
	x.token = s.signIn("member", "tv")
	x.library = view(x.token)
	var listed QueryResult
	s.get(t, "/Items?ParentId="+x.library+"&Limit=200", x.token, &listed)
	if len(listed.Items) != count {
		t.Fatalf("%d series", len(listed.Items))
	}
	for _, show := range listed.Items {
		x.series[show.Name] = show.Id
	}
	return x
}

// episode returns a show's episode by its place, as "S1E2".
func (x shows) episode(t *testing.T, show, place string) BaseItemDto {
	t.Helper()
	var episodes QueryResult
	x.get(t, "/Shows/"+x.series[show]+"/Episodes", x.token, &episodes)
	for _, episode := range episodes.Items {
		if episode.ParentIndexNumber != nil && episode.IndexNumber != nil && fmt.Sprintf("S%dE%d", *episode.ParentIndexNumber, *episode.IndexNumber) == place {
			return episode
		}
	}
	t.Fatalf("no %s %s", show, place)
	return BaseItemDto{}
}

// set changes the member's data of a show's episode.
func (x shows) set(t *testing.T, show, place string, change func(*userdata.Data)) {
	t.Helper()
	episode := x.episode(t, show, place)
	item := userdata.Item{ID: mustID(t, episode.Id), Series: mustID(t, episode.SeriesId), Season: mustID(t, episode.SeasonId)}
	if _, err := x.handler.Options.UserData.Change(t.Context(), x.member.ID, []userdata.Item{item}, change); err != nil {
		t.Fatal(err)
	}
}

// played marks a show's episode played at.
func (x shows) played(t *testing.T, show, place string, at time.Time) {
	t.Helper()
	x.set(t, show, place, func(d *userdata.Data) { d.Played, d.PlayCount, d.Position, d.LastPlayed = true, d.PlayCount+1, 0, &at })
}

// started gives a show's episode a resume point, played at.
func (x shows) started(t *testing.T, show, place string, at time.Time) {
	t.Helper()
	x.set(t, show, place, func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 10*time.Minute, 45*time.Minute, &at })
}

// list lists the episodes path answers, as "Show 01 S1E2", with the total
// it counts.
func (x shows) list(t *testing.T, path string) ([]string, int) {
	t.Helper()
	var page QueryResult
	if status := x.get(t, path, x.token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	names := make([]string, len(page.Items))
	for i, item := range page.Items {
		names[i] = item.Name
		if item.Type == "Episode" {
			names[i] = fmt.Sprintf("%s S%dE%d", item.SeriesName, *item.ParentIndexNumber, *item.IndexNumber)
		}
	}
	return names, page.TotalRecordCount
}

// Next Up and Continue Watching of IPTV series, through each rule of
// docs/watch-state.md: the latest played first and series only started
// last, specials left out, an episode under way offered when resumable,
// a rewatch's next episode, Max days in Next Up, a series taken off Next
// Up until a newer play, one dropped on a tracking service, a series
// asked for alone, a parent, and paging.
func TestIPTVNextUpAndContinueWatchingAnswers(t *testing.T) {
	t.Parallel()
	x := newIPTVShows(t, 6, 2, 4)
	base := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	at := func(hours float64) time.Time { return base.Add(time.Duration(hours * float64(time.Hour))) }
	x.played(t, "Show 00", "S1E1", at(1))
	x.played(t, "Show 00", "S1E2", at(2))
	x.started(t, "Show 00", "S1E4", at(2.5))
	for i, place := range []string{"S1E1", "S1E2", "S1E3", "S1E4"} {
		x.played(t, "Show 01", place, at(3+float64(i)/10))
	}
	x.played(t, "Show 01", "S2E1", at(4))
	x.played(t, "Show 01", "S1E1", at(20))
	x.played(t, "Show 02", "S1E1", at(5))
	x.started(t, "Show 02", "S1E2", at(6))
	x.played(t, "Show 03", "S1E1", at(7))
	if err := x.handler.Options.UserData.RemoveFromNextUp(t.Context(), x.member.ID, mustID(t, x.series["Show 03"]), at(8)); err != nil {
		t.Fatal(err)
	}
	x.played(t, "Show 04", "S1E1", at(9))
	x.started(t, "Show 04", "S1E2", at(9.5))
	x.drop(t, x.member.ID.String(), "trakt", x.series["Show 04"], at(10))
	x.played(t, "Show 05", "S0E1", at(11))

	check := func(path string, want []string, total int) {
		t.Helper()
		got, counted := x.list(t, path)
		if !slices.Equal(got, want) || counted != total {
			t.Errorf("%s: %q, total %d; want %q, total %d", path, got, counted, want, total)
		}
	}
	check("/Shows/NextUp", []string{"Show 02 S1E2", "Show 01 S2E2", "Show 00 S1E3", "Show 05 S1E1"}, 4)
	check("/Shows/NextUp?enableRewatching=true", []string{"Show 01 S1E2", "Show 02 S1E2", "Show 01 S2E2", "Show 00 S1E3", "Show 05 S1E1"}, 5)
	check("/Shows/NextUp?enableResumable=false", []string{"Show 01 S2E2", "Show 00 S1E3", "Show 05 S1E1"}, 3)
	check("/Shows/NextUp?nextUpDateCutoff="+at(4.5).Format(time.RFC3339), []string{"Show 02 S1E2", "Show 01 S2E2", "Show 05 S1E1"}, 3)
	check("/Shows/NextUp?limit=2&enableTotalRecordCount=false", []string{"Show 02 S1E2", "Show 01 S2E2"}, 0)
	check("/Shows/NextUp?startIndex=1&limit=2", []string{"Show 01 S2E2", "Show 00 S1E3"}, 4)
	check("/Shows/NextUp?parentId="+x.library, []string{"Show 02 S1E2", "Show 01 S2E2", "Show 00 S1E3", "Show 05 S1E1"}, 4)
	check("/Shows/NextUp?seriesId="+x.series["Show 03"], []string{"Show 03 S1E2"}, 1)
	check("/Shows/NextUp?seriesId="+x.series["Show 04"], []string{"Show 04 S1E2"}, 1)
	check("/Shows/NextUp?seriesId="+x.series["Show 04"]+"&enableResumable=false", nil, 0)
	check("/UserItems/Resume", []string{"Show 02 S1E2", "Show 00 S1E4"}, 2)
	check("/UserItems/Resume?limit=1&enableTotalRecordCount=false", []string{"Show 02 S1E2"}, 1)
	check("/UserItems/Resume?startIndex=1&limit=1&enableTotalRecordCount=false", []string{"Show 00 S1E4"}, 1)
	check("/UserItems/Resume?limit=1", []string{"Show 02 S1E2"}, 2)

	// A play after the removal lifts it.
	x.played(t, "Show 03", "S1E2", at(12))
	check("/Shows/NextUp", []string{"Show 03 S1E3", "Show 02 S1E2", "Show 01 S2E2", "Show 00 S1E3", "Show 05 S1E1"}, 5)
	// A hidden series leaves both.
	if status, body := x.call(http.MethodPost, "/Polyfin/UserItems/"+x.series["Show 02"]+"/Hidden", app("tv", x.token), nil); status/100 != 2 {
		t.Fatalf("hide: %d %s", status, body)
	}
	check("/Shows/NextUp", []string{"Show 03 S1E3", "Show 01 S2E2", "Show 00 S1E3", "Show 05 S1E1"}, 4)
	check("/UserItems/Resume?limit=1&enableTotalRecordCount=false", []string{"Show 00 S1E4"}, 1)
}

// Next Up looks at the 50 series watched last, those dropped on a
// tracking service not counting.
func TestNextUpLooksAtTheFiftySeriesWatchedLast(t *testing.T) {
	t.Parallel()
	const count = 53
	x := newAddonShows(t, count, 2)
	base := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	for i := range count {
		x.played(t, fmt.Sprintf("Show %d", i), "S1E1", base.Add(time.Duration(i)*time.Hour))
	}
	x.drop(t, x.member.ID.String(), "trakt", x.series[fmt.Sprintf("Show %d", count-1)], base.Add(100*time.Hour))
	var want []string
	for i := count - 2; i >= count-51; i-- {
		want = append(want, fmt.Sprintf("Show %d S1E2", i))
	}
	if got, _ := x.list(t, "/Shows/NextUp"); !slices.Equal(got, want) {
		t.Errorf("next up: %q\nwant %q", got, want)
	}
}

// Next Up of IPTV series, right after a restart and again: it describes
// each series once after the restart, in one statement each, then from the
// descriptions kept, its source checked once; it writes and locks no
// item, the episodes stored before the restart being known, and its
// statements stay within a few per series after the restart, what it
// records after its answer included (two look-ups each), then none per
// series while the lists stay the same. Reading every episode the user
// has data of, describing each series twice and merging every episode of
// every series after each restart took several times as many.
//
// With the fields the home page asks for, Path among them, each episode
// shown reads what is known of its versions too: its streams come from
// its series' description kept, so the series' details are still read
// once after the restart and never again, in a few statements per episode
// shown. Each episode read its series' title and details, and decoded
// them, for its streams.
func TestIPTVNextUpStatementsAfterARestart(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, query string
		// perEpisode are the statements each episode shown adds: its
		// record and its series' (see library.Service.CachedVersions),
		// and its series' title for its streams.
		perEpisode int
	}{
		{"plain", "", 0},
		{"home page", "?limit=24&fields=PrimaryImageAspectRatio,DateCreated,Path,MediaSourceCount", 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			const series = 8
			x := newIPTVShows(t, series, 3, 10)
			base := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
			for i := range series {
				x.played(t, fmt.Sprintf("Show %02d", i), "S2E4", base.Add(time.Duration(i)*time.Hour))
			}
			path := "/Shows/NextUp" + c.query
			want, _ := x.list(t, path)
			if len(want) != series {
				t.Fatalf("Next Up before the restart: %q", want)
			}

			// A restart, whose library, IPTV sources and users' data count
			// their statements.
			counter := &statementCounter{}
			config := x.pool.Config()
			config.ConnConfig.Tracer = counter
			counted, err := pgxpool.NewWithConfig(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(counted.Close)
			options := x.handler.Options
			options.Library = library.New(counted, x.addons, options.Stremio, options.Logger, x.store.Settings)
			options.Library.SetFollowUps()
			options.IPTV = iptv.New(counted, x.addons, options.Stremio, options.Logger, x.store.Settings)
			options.Library.UseIPTV(options.IPTV)
			options.UserData = userdata.New(counted)
			server := httptest.NewServer(New(options))
			t.Cleanup(server.Close)
			restarted := x
			restarted.url = server.URL
			details := regexp.MustCompile(`\biptv_details\b`)
			checks := regexp.MustCompile(`\bFROM iptv_sources WHERE addon_id\b`)
			for _, when := range []string{"after the restart", "again"} {
				counter.reset()
				if got, _ := restarted.list(t, path); !slices.Equal(got, want) {
					t.Errorf("Next Up %s: %q, want %q", when, got, want)
				}
				// What it records after its answer counts too.
				episodesRecorded(t, options.Library)
				described, checked := 0, 0
				counter.mu.Lock()
				for sql, sent := range counter.sent {
					if details.MatchString(sql) {
						described += sent
					}
					if checks.MatchString(sql) {
						checked += sent
					}
				}
				counter.mu.Unlock()
				n := counter.statements.Load()
				t.Logf("Next Up %s: %d statements, %d writing or locking items\n%s", when, n, counter.itemWrites.Load(), counter.mostSent(8))
				budget, read := 12, 0
				if when == "after the restart" {
					budget, read = 3*series+12, series
					if c.perEpisode > 0 {
						// The version blocks and pins, which each of the
						// episodes described at once may read before they
						// are kept.
						budget += 2 * listingAtOnce
					}
				}
				if budget += c.perEpisode * series; n > int64(budget) {
					t.Errorf("Next Up %s: %d statements for %d series", when, n, series)
				}
				if writes := counter.itemWrites.Load(); writes != 0 {
					t.Errorf("Next Up %s: %d statements wrote or locked items", when, writes)
				}
				if described != read || checked != 1 {
					t.Errorf("Next Up %s: the series' details read %d times for %d series, their source checked %d times", when, described, series, checked)
				}
			}
		})
	}
}

// After its answer, Next Up records the episodes of its series it does
// not show, as their listing would: a search by name finds them.
func TestNextUpRecordsTheEpisodesItDoesNotShow(t *testing.T) {
	t.Parallel()
	x := newAddonShows(t, 1, 8)
	x.played(t, "Show 0", "S1E1", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	// As if no listing had recorded the series' episodes.
	if _, err := x.pool.Exec(t.Context(), "DELETE FROM items WHERE kind = 'episode'"); err != nil {
		t.Fatal(err)
	}
	if got, _ := x.list(t, "/Shows/NextUp"); !slices.Equal(got, []string{"Show 0 S1E2"}) {
		t.Fatalf("Next Up: %q", got)
	}
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if got := hintNames(hints(t, x.testServer, x.token, "searchTerm=Episode%207&includeItemTypes=Episode")); slices.Equal(got, []string{"Episode Episode 7"}) {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("search by an episode's name: %q", got)
		}
	}
}

// episodesRecorded waits for lib to record the episodes Next Up listed
// but did not show.
func episodesRecorded(t *testing.T, lib *library.Service) {
	t.Helper()
	select {
	case <-lib.EpisodesRecorded():
	case <-time.After(30 * time.Second):
		t.Fatal("the episodes Next Up did not show are still being recorded")
	}
}

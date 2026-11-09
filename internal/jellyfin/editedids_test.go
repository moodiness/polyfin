package jellyfin

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/trackers"
)

// editingIDs is describingIDs with an administrator signed in, whose
// token it returns.
func editingIDs(t *testing.T, s testServer) (describedIDs, string) {
	t.Helper()
	d := describingIDs(t, s)
	d.testServer.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	return d, d.signIn("admin", "dashboard")
}

// editIDs saves, as jellyfin-web's metadata editor does, fields and the
// provider identifiers ids of the item id.
func (d describedIDs) editIDs(t *testing.T, admin, id string, ids map[string]string, fields map[string]any) {
	t.Helper()
	body := editorBody(id, fields)
	body["ProviderIds"] = ids
	if status, answer := d.call(http.MethodPost, "/Items/"+id, app("dashboard", admin), body); status != http.StatusNoContent {
		t.Fatalf("editing %s: %d %s", id, status, answer)
	}
}

// shownIDs returns the name, provider identifiers and links that the
// user of token sees of the item id.
func (d describedIDs) shownIDs(t *testing.T, token, id string) (string, string, []string) {
	t.Helper()
	var item BaseItemDto
	if status := d.get(t, "/Items/"+id, token, &item); status != http.StatusOK || item.ProviderIds == nil || item.ExternalUrls == nil {
		t.Fatalf("%s: %d %+v", id, status, item)
	}
	var urls []string
	for _, link := range *item.ExternalUrls {
		urls = append(urls, link.Url)
	}
	return item.Name, fmt.Sprint(*item.ProviderIds), urls
}

// An administrator corrects a movie's TMDB identifier and a series' TMDB
// and TVDB ones in the metadata editor: every user sees them and their
// links. A value that is no identifier, and a movie's TVDB identifier,
// change nothing while the other fields are kept; a save leaving out the
// identifiers keeps them; the source's values, or none, follow the source
// again. An episode's identifiers are not edited.
func TestEditedTMDBAndTVDBIdentifiersShowForEveryUser(t *testing.T) {
	t.Parallel()
	d, admin := editingIDs(t, newTestServer(t, 10))
	movie, series := d.titles["The Shawshank Redemption"], d.titles["Survivor"]
	d.editIDs(t, admin, movie, map[string]string{"Imdb": "tt0111161", "Tmdb": "680", "Tvdb": ""}, nil)
	d.editIDs(t, admin, series, map[string]string{"Imdb": "tt0239195", "Tmdb": "1400", "Tvdb": "900"}, nil)
	edited := map[string]struct {
		ids  string
		urls []string
	}{
		movie: {fmt.Sprint(map[string]string{"Imdb": "tt0111161", "Tmdb": "680"}),
			[]string{"https://www.imdb.com/title/tt0111161", "https://www.themoviedb.org/movie/680"}},
		series: {fmt.Sprint(map[string]string{"Imdb": "tt0239195", "Tmdb": "1400", "Tvdb": "900"}),
			[]string{"https://www.imdb.com/title/tt0239195", "https://www.themoviedb.org/tv/1400", "https://thetvdb.com/?tab=series&id=900"}},
	}
	for _, token := range []string{admin, d.token} {
		for id, want := range edited {
			if _, ids, urls := d.shownIDs(t, token, id); ids != want.ids || !slices.Equal(urls, want.urls) {
				t.Errorf("%s: %s %v, want %s %v", id, ids, urls, want.ids, want.urls)
			}
		}
	}

	// Values that are no identifier, and a movie's TVDB identifier, are
	// left out; the name is kept.
	for _, bad := range []map[string]string{{"Tmdb": "abc"}, {"Tmdb": "0"}, {"Tmdb": "-680"}, {"Tmdb": "2147483648"}, {"Tmdb": "680", "Tvdb": "12"}} {
		bad["Imdb"] = "tt0111161"
		d.editIDs(t, admin, movie, bad, map[string]any{"Name": "Renamed"})
		if o := d.library.Overrides(mustID(t, movie)); o.TmdbID == nil || *o.TmdbID != "680" || o.TvdbID != nil || o.Name == nil || *o.Name != "Renamed" {
			t.Errorf("after %v: %+v", bad, o)
		}
	}
	if name, ids, _ := d.shownIDs(t, d.token, movie); name != "Renamed" || ids != edited[movie].ids {
		t.Errorf("after values that are no identifier: %s %s", name, ids)
	}
	// A save without the identifiers keeps them.
	d.editIDs(t, admin, series, map[string]string{"Imdb": "tt0239195"}, nil)
	if _, ids, _ := d.shownIDs(t, d.token, series); ids != edited[series].ids {
		t.Errorf("after a save without them: %s", ids)
	}

	// An episode's identifiers stay its source's.
	episode := d.episodes["Survivor"]
	d.editIDs(t, admin, episode, map[string]string{"Tmdb": "5", "Tvdb": "6"}, nil)
	if o := d.library.Overrides(mustID(t, episode)); o.TmdbID != nil || o.TvdbID != nil {
		t.Errorf("episode: %+v", o)
	}

	// The source's values, or none, follow the source again.
	d.editIDs(t, admin, movie, map[string]string{"Imdb": "tt0111161", "Tmdb": "278"}, nil)
	d.editIDs(t, admin, series, map[string]string{"Imdb": "tt0239195", "Tmdb": "", "Tvdb": "76733"}, nil)
	for id, want := range map[string]string{
		movie:  fmt.Sprint(map[string]string{"Imdb": "tt0111161", "Tmdb": "278"}),
		series: fmt.Sprint(map[string]string{"Imdb": "tt0239195", "Tmdb": "14658", "Tvdb": "76733"}),
	} {
		if o := d.library.Overrides(mustID(t, id)); o.TmdbID != nil || o.TvdbID != nil {
			t.Errorf("%s kept: %+v", id, o)
		}
		if _, ids, _ := d.shownIDs(t, d.token, id); ids != want {
			t.Errorf("%s: %s, want %s", id, ids, want)
		}
	}
}

// The segment databases are asked about a movie or series by the TMDB
// identifier an administrator set, which is no moviedb_id: PublicMetaDB
// is asked about the series' episodes in TMDB's numbering. What was
// learnt by the former identifier, the segments of the movie and of the
// series' episodes and the series' alignment on TMDB's numbering, is
// dropped; that of other titles is kept.
func TestEditedTMDBIdentifiersAskTheSegmentDatabasesAgain(t *testing.T) {
	t.Parallel()
	d, admin := editingIDs(t, newTestServer(t, 10))
	d.setting(t, func(s *accounts.Settings) {
		s.PublicMetaDBKey = "pm-Good4mN2pQrS7tUvWx3yZaB4cD5eF6gH7iJ8kL9mN0oP1qR2sT3uV4wXyZ5aB"
	})
	title := func(id string) string {
		t.Helper()
		item, err := d.library.Item(t.Context(), d.user, mustID(t, id))
		if err != nil {
			t.Fatal(err)
		}
		title, err := d.handler.segmentTitle(t.Context(), d.user, item)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%s S%dE%d [%d %d]", title.TMDB, title.Season, title.Episode, title.TMDBSeason, title.TMDBEpisode)
	}
	movie, series := d.titles["The Shawshank Redemption"], d.titles["Survivor"]
	if got := title(d.episodes["Survivor"]); got != "14658 S1E2 [0 -1]" {
		t.Fatalf("by moviedb_id: %s", got)
	}
	for _, id := range []string{movie, d.episodes["Survivor"], d.episodes["IT"]} {
		if _, err := d.pool.Exec(t.Context(), "INSERT INTO media_segments (item_id, answers) VALUES ($1, '{}')", mustID(t, id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.pool.Exec(t.Context(), `INSERT INTO episode_alignments (series, tmdb_id, episodes, places, asked_at)
		VALUES ('tt0239195', 14658, 2, '[[1,1,1,1],[1,3,1,2]]', now()), ('tt0099864', 113981, 2, '[[1,1,1,1],[1,2,1,2]]', now())`); err != nil {
		t.Fatal(err)
	}

	d.editIDs(t, admin, movie, map[string]string{"Imdb": "tt0111161", "Tmdb": "680"}, nil)
	d.editIDs(t, admin, series, map[string]string{"Imdb": "tt0239195", "Tmdb": "1400", "Tvdb": "76733"}, nil)
	if got := title(movie); got != "680 S0E0 [0 0]" {
		t.Errorf("the movie: %s", got)
	}
	if got := title(d.episodes["Survivor"]); got != "1400 S1E2 [0 0]" {
		t.Errorf("by the identifier set: %s", got)
	}
	var segments []string
	rows, err := d.pool.Query(t.Context(), "SELECT item_id::text FROM media_segments")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		segments = append(segments, strings.ReplaceAll(id, "-", ""))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{strings.ReplaceAll(mustID(t, d.episodes["IT"]).String(), "-", "")}; !slices.Equal(segments, want) {
		t.Errorf("segments kept: %v, want %v", segments, want)
	}
	var aligned []string
	rows, err = d.pool.Query(t.Context(), "SELECT series FROM episode_alignments ORDER BY series")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var series string
		if err := rows.Scan(&series); err != nil {
			t.Fatal(err)
		}
		aligned = append(aligned, series)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(aligned, []string{"tt0099864"}) {
		t.Errorf("alignments kept: %v", aligned)
	}
}

// The tracking services hear of a movie and of a series' episode by the
// TMDB and TVDB identifiers an administrator set.
func TestTrackingServicesHearOfEditedIdentifiers(t *testing.T) {
	t.Parallel()
	fakes := newTrackerFakes(t)
	var tracker *trackers.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.MDBList: fakes.url + "/mdblist", trackers.PublicMetaDB: fakes.url + "/publicmetadb"}})
		options.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	d, admin := editingIDs(t, s)
	for service, key := range map[string]string{trackers.MDBList: "mdblist-member", trackers.PublicMetaDB: "pm-member"} {
		if _, err := tracker.ConnectKey(t.Context(), d.user.ID, service, key); err != nil {
			t.Fatal(err)
		}
	}
	d.editIDs(t, admin, d.titles["The Shawshank Redemption"], map[string]string{"Imdb": "tt0111161", "Tmdb": "680"}, nil)
	d.editIDs(t, admin, d.titles["Survivor"], map[string]string{"Imdb": "tt0239195", "Tmdb": "1400", "Tvdb": "900"}, nil)
	for _, id := range []string{d.titles["The Shawshank Redemption"], d.episodes["Survivor"]} {
		if status, body := d.call(http.MethodPost, "/UserPlayedItems/"+id, app("tv", d.token), nil); status != http.StatusOK {
			t.Fatalf("marking %s played: %d %s", id, status, body)
		}
	}
	var got []string
	for _, call := range fakes.wait(t, 4) {
		got = append(got, call.summary())
	}
	slices.Sort(got)
	want := []string{
		`POST /mdblist/sync/watched {"movies":[{"ids":{"imdb":"tt0111161","tmdb":680}}]}`,
		`POST /mdblist/sync/watched {"shows":[{"ids":{"imdb":"tt0239195","tmdb":1400,"tvdb":900},"seasons":[{"episodes":[{"number":2}],"number":1}]}]}`,
		`POST /publicmetadb/api/external/watched {"episode":2,"media_type":"tv","season":1,"tmdb_id":1400}`,
		`POST /publicmetadb/api/external/watched {"media_type":"movie","tmdb_id":680}`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("sent:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// editedAnimeMapping is an anime mapping that knows a Kitsu movie, by IMDb
// and TMDB identifiers, and an entry of a series that is TVDB's second
// season.
func editedAnimeMapping(t *testing.T) *anime.Service {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ids.json" {
			_, _ = w.Write([]byte(`[
				{"type":"TV","anidb_id":2,"kitsu_id":12,"mal_id":22,"anilist_id":32,"imdb_id":["tt0000100"],"tvdb_id":500},
				{"type":"MOVIE","anidb_id":4,"kitsu_id":14,"mal_id":24,"anilist_id":34,"imdb_id":["tt0000400"],"themoviedb_id":{"movie":[4000]}}
			]`))
			return
		}
		_, _ = w.Write([]byte(`<anime-list>
			<anime anidbid="2" tvdbid="500" defaulttvdbseason="2"><name>Second</name></anime>
			<anime anidbid="4" tvdbid="movie" defaulttvdbseason="1"><name>The movie</name></anime>
		</anime-list>`))
	}))
	t.Cleanup(server.Close)
	m := anime.New(anime.Options{Dir: t.TempDir(), Version: "test", IDsURL: server.URL + "/ids.json", EpisodesURL: server.URL + "/episodes.xml"})
	t.Cleanup(m.Close)
	eventually(t, "the anime mapping being read", m.Ready)
	return m
}

// libraryItemID is the identifier the library derives for the item it
// keeps under key.
func libraryItemID(key string) accounts.ID {
	sum := sha256.Sum256([]byte("polyfin:item:" + key))
	var id accounts.ID
	copy(id[:], sum[:16])
	return id
}

// Anime the mapping identifies are sent with the TMDB and TVDB identifiers
// an administrator set of the movie, or of the episode's series, instead
// of the mapping's, numbered as the mapping numbers them and keeping
// their entry.
func TestEditedIdentifiersWinOverTheAnimeMapping(t *testing.T) {
	t.Parallel()
	mapping := editedAnimeMapping(t)
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		tracker := trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger, Anime: mapping})
		t.Cleanup(tracker.Close)
		options.Trackers = tracker
	})
	user := s.user("member", nil)
	movie := library.Item{ID: libraryItemID("movie|kitsu:14"), Kind: library.KindMovie, StremioID: "kitsu:14"}
	series := library.Item{ID: libraryItemID("series|kitsu:12"), Kind: library.KindSeries, StremioID: "kitsu:12"}
	episode := library.Item{ID: libraryItemID("episode|kitsu:12:3"), Kind: library.KindEpisode, StremioID: "kitsu:12:3",
		SeriesID: series.ID, ParentIndexNumber: 1, IndexNumber: 3}
	names := map[accounts.ID]string{movie.ID: "movie", episode.ID: "episode"}
	sent := func() string {
		t.Helper()
		var lines []string
		for _, title := range s.handler.trackedTitles(user, []library.Item{movie, episode})(t.Context()) {
			lines = append(lines, fmt.Sprintf("%s %s tmdb %d tvdb %d episode %t S%dE%d %+v", names[title.Item], title.IMDb, title.TMDB, title.TVDB,
				title.Episode, title.Season, title.Number, title.Anime))
		}
		return strings.Join(lines, "\n")
	}
	mapped := "movie tt0000400 tmdb 4000 tvdb 0 episode false S0E0 &{Source:kitsu ID:14 Episode:0}\n" +
		"episode tt0000100 tmdb 0 tvdb 500 episode true S2E3 &{Source:kitsu ID:12 Episode:3}"
	if got := sent(); got != mapped {
		t.Fatalf("by the mapping:\n%s\nwant\n%s", got, mapped)
	}
	if err := s.library.SaveOverrides(t.Context(), movie, library.Overrides{TmdbID: new("777")}); err != nil {
		t.Fatal(err)
	}
	if err := s.library.SaveOverrides(t.Context(), series, library.Overrides{TmdbID: new("888"), TvdbID: new("999")}); err != nil {
		t.Fatal(err)
	}
	want := "movie tt0000400 tmdb 777 tvdb 0 episode false S0E0 &{Source:kitsu ID:14 Episode:0}\n" +
		"episode tt0000100 tmdb 888 tvdb 999 episode true S2E3 &{Source:kitsu ID:12 Episode:3}"
	if got := sent(); got != want {
		t.Errorf("edited:\n%s\nwant\n%s", got, want)
	}
}

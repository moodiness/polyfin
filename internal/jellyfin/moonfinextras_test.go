package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/ratings"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// newExtrasTMDB serves TMDB's API as it answers the extras, for the key
// k3y: the movie 550 (also found by tt0137523), with its reviews in
// English only, its images and videos; the person 287; the series 1399,
// its season 2 and next episode; the series 1400 with nothing scheduled;
// and the IMDb identifiers of the movie 10 (tt0000) and the series 20
// (tt0100), which the fake MDBList knows. Anything else is unknown.
func newExtrasTMDB(t *testing.T) string {
	t.Helper()
	image := func(path string, language any, rating float64) map[string]any {
		return map[string]any{"file_path": path, "iso_639_1": language, "vote_average": rating}
	}
	answers := map[string]any{
		"/3/find/tt0137523":        map[string]any{"movie_results": []any{map[string]any{"id": 550}}, "tv_results": []any{}},
		"/3/movie/10/external_ids": map[string]any{"imdb_id": "tt0000"},
		"/3/tv/20/external_ids":    map[string]any{"imdb_id": "tt0100"},
		"/3/movie/550": map[string]any{"overview": "A story.", "vote_average": 8.4, "genres": []any{map[string]any{"id": 18, "name": "Drama"}},
			"runtime": 139, "backdrop_path": "/main.jpg",
			"images": map[string]any{"backdrops": []any{image("/titled.jpg", "en", 5), image("/plain.jpg", nil, 4)},
				"logos": []any{image("/logo.svg", "en", 5)}},
			"videos": map[string]any{"results": []any{map[string]any{"key": "key1", "site": "YouTube", "type": "Trailer", "official": true, "iso_639_1": "en"}}}},
		"/3/person/287": map[string]any{"homepage": "https://person.example", "external_ids": map[string]any{"imdb_id": "nm0000093",
			"instagram_id": "insta", "twitter_id": "", "facebook_id": nil, "tiktok_id": "tok", "youtube_id": "tube"}},
		"/3/tv/1399/season/2": map[string]any{"episodes": []any{map[string]any{"episode_number": 1, "vote_average": 8.2},
			map[string]any{"episode_number": 2, "vote_average": 7.5}}},
		"/3/tv/1399": map[string]any{"next_episode_to_air": map[string]any{"air_date": "2026-12-01", "season_number": 3, "episode_number": 4, "name": "The Return"}},
		"/3/tv/1400": map[string]any{"next_episode_to_air": nil},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "k3y" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/3/movie/550/reviews" {
			results := []any{}
			if r.URL.Query().Get("language") == "en-US" {
				results = append(results, map[string]any{"author": "ghost", "author_details": map[string]any{"name": "Ghost", "avatar_path": "/a.jpg", "rating": 8},
					"content":    `<p>A <b>great</b> story.</p><p>See <a href="https://example.com">this</a>.<br>Yes</p>`,
					"created_at": "2024-01-02T03:04:05.000Z", "url": "https://tmdb.example/r/1"},
					map[string]any{"author": "far", "author_details": map[string]any{"avatar_path": "/https://avatars.example/far.png", "rating": nil},
						"content": "Plain text."})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
			return
		}
		answer, ok := answers[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/3"
}

// newExtrasServer is a server asking newExtrasTMDB with the server's TMDB
// key k3y, or without a key when keyed is false, with a member signed in.
func newExtrasServer(t *testing.T, keyed bool) (testServer, string) {
	t.Helper()
	api := newExtrasTMDB(t)
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.TMDB = tmdb.New(api, "https://image.example/t/p/", "test", o.Accounts.Settings)
	})
	if keyed {
		s.setting(t, func(settings *accounts.Settings) { settings.TMDBKey = "k3y" })
	}
	s.user("member", nil)
	return s, s.signIn("member", "tv")
}

// extraGet asks a Moonfin extra as the member, and returns its status and
// its JSON answer, nil for none.
func (s testServer) extraGet(t *testing.T, path, token string) (int, map[string]any) {
	t.Helper()
	status, body := s.call(http.MethodGet, path, app("tv", token), nil)
	var answer map[string]any
	if json.Unmarshal(body, &answer) != nil {
		answer = nil
	}
	return status, answer
}

// extraThumb asks /Moonfin/Tmdb/Thumb without following its redirect, and
// returns its status and Location.
func (s testServer) extraThumb(t *testing.T, query, token string) (int, string) {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url+"/Moonfin/Tmdb/Thumb?"+query, nil)
	request.Header.Set("Authorization", app("tv", token))
	response, err := noRedirects.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	return response.StatusCode, response.Header.Get("Location")
}

// The TMDB extras answer as Moonfin's apps read them, each with success:
// true: reviews in English when none are in the language asked, their
// HTML as light Markdown; a person's links, those TMDB has none of left
// out; a title's details by its IMDb identifier, with a textless backdrop,
// a logo as a PNG and a YouTube trailer; a redirect to its titled
// backdrop, over http when asked; the ratings of a season and of one of
// its episodes; and a series' next episode.
func TestMoonfinTMDBExtrasAnswerAsTheAppsReadThem(t *testing.T) {
	t.Parallel()
	s, token := newExtrasServer(t, true)
	check := func(path string, want map[string]any) {
		t.Helper()
		status, answer := s.extraGet(t, path, token)
		got, _ := json.Marshal(answer)
		expected, _ := json.Marshal(want)
		if status != http.StatusOK || string(got) != string(expected) {
			t.Errorf("%s: %d %s, want %s", path, status, got, expected)
		}
	}
	check("/Moonfin/Tmdb/Reviews?tmdbId=550&type=movie&language=fr-FR", map[string]any{"success": true, "reviews": []any{
		map[string]any{"author": "Ghost", "rating": 8, "content": "A **great** story.\n\nSee [this](https://example.com).\nYes",
			"createdAt": "2024-01-02T03:04:05.000Z", "avatarUrl": "https://image.example/t/p/w185/a.jpg", "url": "https://tmdb.example/r/1"},
		map[string]any{"author": "far", "content": "Plain text.", "avatarUrl": "https://avatars.example/far.png"},
	}})
	check("/Moonfin/Tmdb/PersonLinks?tmdbId=287", map[string]any{"success": true, "imdbId": "nm0000093", "homepage": "https://person.example",
		"instagram": "insta", "tiktok": "tok", "youtube": "tube"})
	check("/Moonfin/Tmdb/Details?type=movie&imdbId=tt0137523", map[string]any{"success": true, "overview": "A story.", "rating": 8.4,
		"genres": []string{"Drama"}, "runTimeTicks": int64(139) * 60 * 10_000_000, "backdropPath": "/plain.jpg", "logoPath": "/logo.png",
		"trailerUrl": "https://www.youtube.com/watch?v=key1"})
	check("/Moonfin/Tmdb/SeasonRatings?tmdbId=1399&season=2", map[string]any{"success": true, "episodes": []any{
		map[string]any{"episodeNumber": 1, "voteAverage": 8.2}, map[string]any{"episodeNumber": 2, "voteAverage": 7.5}}})
	check("/Moonfin/Tmdb/EpisodeRating?tmdbId=1399&season=2&episode=2", map[string]any{"success": true, "voteAverage": 7.5})
	check("/Moonfin/Tmdb/NextEpisode?tmdbId=1399", map[string]any{"success": true, "airDate": "2026-12-01", "seasonNumber": 3,
		"episodeNumber": 4, "name": "The Return"})
	if status, location := s.extraThumb(t, "type=movie&tmdbId=550", token); status != http.StatusFound || location != "https://image.example/t/p/w780/titled.jpg" {
		t.Errorf("thumb: %d %q", status, location)
	}
	if status, location := s.extraThumb(t, "type=movie&imdbId=tt0137523&scheme=http", token); status != http.StatusFound ||
		location != "http://image.example/t/p/w780/titled.jpg" {
		t.Errorf("thumb over http: %d %q", status, location)
	}
}

// Without the server's TMDB key, each TMDB extra answers success: false
// with an error, and the thumb 404.
func TestMoonfinTMDBExtrasWithoutAKey(t *testing.T) {
	t.Parallel()
	s, token := newExtrasServer(t, false)
	for _, path := range []string{
		"/Moonfin/Tmdb/Reviews?tmdbId=550&type=movie", "/Moonfin/Tmdb/PersonLinks?tmdbId=287", "/Moonfin/Tmdb/Details?type=movie&tmdbId=550",
		"/Moonfin/Tmdb/EpisodeRating?tmdbId=1399&season=2&episode=1", "/Moonfin/Tmdb/SeasonRatings?tmdbId=1399&season=2",
		"/Moonfin/Tmdb/NextEpisode?tmdbId=1399",
	} {
		if status, answer := s.extraGet(t, path, token); status != http.StatusOK || answer["success"] != false || answer["error"] != noTMDBKey {
			t.Errorf("%s: %d %v", path, status, answer)
		}
	}
	if status, _ := s.extraThumb(t, "type=movie&tmdbId=550", token); status != http.StatusNotFound {
		t.Errorf("thumb: %d", status)
	}
}

// A title, person, season or episode TMDB does not know, and a series with
// nothing scheduled, answer success: false with an error; the thumb of an
// unknown title 404.
func TestMoonfinTMDBExtrasOfUnknownTitles(t *testing.T) {
	t.Parallel()
	s, token := newExtrasServer(t, true)
	for _, path := range []string{
		"/Moonfin/Tmdb/Reviews?tmdbId=404&type=tv", "/Moonfin/Tmdb/PersonLinks?tmdbId=404", "/Moonfin/Tmdb/Details?type=movie&tmdbId=404",
		"/Moonfin/Tmdb/Details?type=tv&imdbId=tt0137523", "/Moonfin/Tmdb/EpisodeRating?tmdbId=1399&season=2&episode=9",
		"/Moonfin/Tmdb/SeasonRatings?tmdbId=1399&season=9", "/Moonfin/Tmdb/NextEpisode?tmdbId=1400", "/Moonfin/Tmdb/NextEpisode?tmdbId=404",
	} {
		if status, answer := s.extraGet(t, path, token); status != http.StatusOK || answer["success"] != false || answer["error"] == nil {
			t.Errorf("%s: %d %v", path, status, answer)
		}
	}
	for _, query := range []string{"type=movie&tmdbId=404", "type=movie&imdbId=tt9999999"} {
		if status, _ := s.extraThumb(t, query, token); status != http.StatusNotFound {
			t.Errorf("thumb of %s: %d", query, status)
		}
	}
}

// Parameters out of shape answer 400.
func TestMoonfinExtrasRefuseBadParameters(t *testing.T) {
	t.Parallel()
	s, token := newExtrasServer(t, true)
	for _, path := range []string{
		"/Moonfin/Tmdb/Reviews?tmdbId=550&type=show", "/Moonfin/Tmdb/Reviews?tmdbId=0&type=movie", "/Moonfin/Tmdb/PersonLinks?tmdbId=x",
		"/Moonfin/Tmdb/Details?type=movie", "/Moonfin/Tmdb/Details?type=movie&imdbId=nm0000093", "/Moonfin/Tmdb/Details?tmdbId=550",
		"/Moonfin/Tmdb/EpisodeRating?tmdbId=1399&season=2", "/Moonfin/Tmdb/SeasonRatings?tmdbId=1399&season=-1",
		"/Moonfin/Tmdb/NextEpisode", "/Moonfin/MdbList/Ratings?type=tv&tmdbId=10", "/Moonfin/MdbList/Ratings?type=movie&tmdbId=tt0000",
	} {
		if status, _ := s.extraGet(t, path, token); status != http.StatusBadRequest {
			t.Errorf("%s: %d", path, status)
		}
	}
	if status, _ := s.extraThumb(t, "tmdbId=550", token); status != http.StatusBadRequest {
		t.Errorf("thumb without a type: %d", status)
	}
}

// /Moonfin/MdbList/Ratings answers the MDBList ratings Polyfin keeps of a
// title, asked about by the IMDb identifier TMDB gives, as the library's
// titles are: success: false while it is asked about in the background,
// then its IMDb rating, critics', audience and Metacritic scores. Without
// the server's MDBList key, it answers success: false with an error.
func TestMoonfinMDBListRatingsAreTheKeptScores(t *testing.T) {
	t.Parallel()
	api := newExtrasTMDB(t)
	s, token, _ := browsingOn(t, newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.TMDB = tmdb.New(api, "https://image.example/t/p/", "test", o.Accounts.Settings)
	}))
	f := newFakeMDBList(t)
	lookups := ratings.New(s.pool, f.url, "test", slog.New(slog.NewTextHandler(io.Discard, nil)), s.store.Settings)
	t.Cleanup(lookups.Close)
	s.library.UseRatings(lookups)
	s.setting(t, func(settings *accounts.Settings) { settings.TMDBKey, settings.MDBListRatingsKey = "k3y", "server-key" })
	answer := f.hold(t)
	movie := "/Moonfin/MdbList/Ratings?type=movie&tmdbId=10&profile=tv"
	if status, got := s.extraGet(t, movie, token); status != http.StatusOK || got["success"] != false {
		t.Fatalf("before MDBList answered: %d %v", status, got)
	}
	answer()
	described := func(path string) string {
		_, got := s.extraGet(t, path, token)
		if got["success"] != true {
			return ""
		}
		text, _ := json.Marshal(got["ratings"])
		return string(text)
	}
	want := `[{"score":81,"source":"imdb","value":8.1},{"score":97,"source":"tomatoes","value":97},` +
		`{"score":60,"source":"tomatoes_audience","value":60},{"score":12,"source":"metacritic","value":12}]`
	eventually(t, "the movie rated", func() bool { return described(movie) == want })
	show := "/Moonfin/MdbList/Ratings?type=show&tmdbId=20"
	want = `[{"score":89,"source":"imdb","value":8.9},{"score":88,"source":"tomatoes","value":88},` +
		`{"score":91,"source":"tomatoes_audience","value":91},{"score":12,"source":"metacritic","value":12}]`
	eventually(t, "the show rated", func() bool { return described(show) == want })
	if asked := f.requests(); !slices.Equal(asked, []string{"POST /imdb/movie tt0000 server-key", "POST /imdb/show tt0100 server-key"}) {
		t.Errorf("MDBList was asked %q", asked)
	}

	s.setting(t, func(settings *accounts.Settings) { settings.MDBListRatingsKey = "" })
	if status, got := s.extraGet(t, movie, token); status != http.StatusOK || got["success"] != false || got["error"] == nil {
		t.Errorf("without a key: %d %v", status, got)
	}
}

// A review's HTML becomes light Markdown: bold and italics, links over
// http or https only, headings, quotes, list items and line breaks, its
// entities decoded, scripts dropped and blank lines kept to one.
func TestLightMarkdownKeepsTheShapeOfAReview(t *testing.T) {
	t.Parallel()
	for _, c := range [][2]string{
		{"Line one\r\nLine two", "Line one\nLine two"},
		{"<p>A <strong>bold</strong> and <em>light</em> one</p>", "A **bold** and *light* one"},
		{"<ul><li>One</li><li>Two</li></ul>", "- One\n- Two"},
		{"<h2>Title</h2>Text", "## Title\n\nText"},
		{"<script>x()</script>Kept &amp; done", "Kept & done"},
		{`<a href="javascript:x()">plain</a> <a href="http://a.example/">link</a>`, "plain [link](http://a.example/)"},
		{"<blockquote>Quoted</blockquote>Said", "> Quoted\n\nSaid"},
		{"One<br/><br><br>Two<hr>Three", "One\n\nTwo\n\n---\n\nThree"},
	} {
		if got := lightMarkdown(c[0]); got != c[1] {
			t.Errorf("%q: %q, want %q", c[0], got, c[1])
		}
	}
}

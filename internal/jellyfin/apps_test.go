package jellyfin

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// hints searches as an app does while the user types.
func hints(t *testing.T, s testServer, token, query string) SearchHintResult {
	t.Helper()
	var result SearchHintResult
	if status := s.get(t, "/Search/Hints?"+query, token, &result); status != http.StatusOK {
		t.Fatalf("hints %s: %d", query, status)
	}
	return result
}

func hintNames(result SearchHintResult) []string {
	names := make([]string, 0, len(result.SearchHints))
	for _, hint := range result.SearchHints {
		names = append(names, hint.Type+" "+hint.Name)
	}
	return names
}

func TestSearchHintsFindTitlesEpisodesAndPeopleTheUserReaches(t *testing.T) {
	s, token, views := browsing(t)
	// The series' episodes and the people of a movie are known once opened.
	var shows, episodes, movies QueryResult
	s.get(t, "/Items?ParentId="+views["Shows"], token, &shows)
	s.get(t, "/Shows/"+shows.Items[0].Id+"/Episodes", token, &episodes)
	s.get(t, "/Items?ParentId="+views["Top"], token, &movies)
	s.get(t, "/Items/"+movies.Items[0].Id, token, nil)

	if got := hintNames(hints(t, s, token, "searchTerm=Movie")); !slices.Equal(got, []string{"Movie Movie 0", "Movie Movie 1", "Movie Movie 2"}) {
		t.Errorf("movies: %v", got)
	}
	pilot := hints(t, s, token, "searchTerm=pilot")
	if len(pilot.SearchHints) != 1 || pilot.TotalRecordCount != 1 {
		t.Fatalf("episodes: %+v", pilot)
	}
	episode := pilot.SearchHints[0]
	if episode.Type != "Episode" || episode.Name != "Pilot" || episode.Series != "Show" || episode.Id != episodes.Items[0].Id ||
		episode.ItemId != episode.Id || *episode.IndexNumber != 1 || *episode.ParentIndexNumber != 1 || episode.MediaType != "Video" ||
		episode.PrimaryImageTag == "" || episode.BackdropImageItemId != shows.Items[0].Id {
		t.Errorf("episode hint: %+v", episode)
	}
	if got := hintNames(hints(t, s, token, "searchTerm=actor")); !slices.Equal(got, []string{"Person Actor"}) {
		t.Errorf("people: %v", got)
	}
	// Types narrow the hints; paging keeps the total.
	if got := hintNames(hints(t, s, token, "searchTerm=e&includeItemTypes=Episode")); !slices.Equal(got, []string{"Episode Second", "Episode Return"}) &&
		!slices.Equal(got, []string{"Episode Return", "Episode Second"}) {
		t.Errorf("episodes containing e: %v", got)
	}
	if got := hintNames(hints(t, s, token, "searchTerm=pilot&includeMedia=false")); len(got) != 0 {
		t.Errorf("without media: %v", got)
	}
	page := hints(t, s, token, "searchTerm=Movie&startIndex=1&limit=1")
	if got := hintNames(page); !slices.Equal(got, []string{"Movie Movie 1"}) || page.TotalRecordCount != 3 {
		t.Errorf("second page: %v of %d", got, page.TotalRecordCount)
	}
	// Jellyfin's shape: the older ItemId, no artist, and no channel.
	_, body := s.call(http.MethodGet, "/Search/Hints?searchTerm=Movie%200", app("tv", token), nil)
	var raw struct{ SearchHints []map[string]json.RawMessage }
	if err := json.Unmarshal(body, &raw); err != nil || len(raw.SearchHints) != 1 {
		t.Fatalf("raw hints: %s", body)
	}
	for key, want := range map[string]string{"Type": `"Movie"`, "Artists": `[]`, "ChannelId": `null`, "ProductionYear": `2020`, "MediaType": `"Video"`} {
		if got := string(raw.SearchHints[0][key]); got != want {
			t.Errorf("%s: %s, want %s", key, got, want)
		}
	}
	if _, ok := raw.SearchHints[0]["IndexNumber"]; ok {
		t.Error("a movie's hint has an IndexNumber")
	}

	// A user who blocks dramas finds none, episodes of dramas included.
	s.user("viewer", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"drama"} })
	viewer := s.signIn("viewer", "tv")
	if got := hintNames(hints(t, s, viewer, "searchTerm=pilot")); len(got) != 0 {
		t.Errorf("blocked episode: %v", got)
	}
	if got := hintNames(hints(t, s, viewer, "searchTerm=Movie")); !slices.Equal(got, []string{"Movie Movie 0", "Movie Movie 2"}) {
		t.Errorf("blocked movie: %v", got)
	}
	if status, body := s.call(http.MethodGet, "/Search/Hints", app("tv", token), nil); status != http.StatusBadRequest || !strings.Contains(string(body), "The searchTerm field is required.") {
		t.Errorf("without a term: %d %s", status, body)
	}
}

func TestMovieRecommendationsFollowSimilarTitles(t *testing.T) {
	s, token, views := browsing(t)
	var movies QueryResult
	s.get(t, "/Items?ParentId="+views["Top"], token, &movies)
	recommendations := func() []RecommendationDto {
		t.Helper()
		var result []RecommendationDto
		if status := s.get(t, "/Movies/Recommendations?categoryLimit=3&itemLimit=4", token, &result); status != http.StatusOK || result == nil {
			t.Fatalf("recommendations: %d %v", status, result)
		}
		return result
	}
	if got := recommendations(); len(got) != 0 {
		t.Errorf("before anything is played: %+v", got)
	}
	// Movie 0 is played: Movie 2 shares its genre.
	if status, _ := s.call(http.MethodPost, "/UserPlayedItems/"+movies.Items[0].Id, app("tv", token), nil); status != http.StatusOK {
		t.Fatalf("played: %d", status)
	}
	got := recommendations()
	if len(got) != 1 || got[0].RecommendationType != "SimilarToRecentlyPlayed" || got[0].BaselineItemName != "Movie 0" ||
		got[0].CategoryId != movies.Items[0].Id || !slices.Equal(itemNames(got[0].Items), []string{"Movie 2"}) {
		t.Fatalf("recommendations: %+v", got)
	}
	// Played titles are not recommended, so the category goes.
	s.call(http.MethodPost, "/UserPlayedItems/"+movies.Items[2].Id, app("tv", token), nil)
	for _, category := range recommendations() {
		if slices.Contains(itemNames(category.Items), "Movie 2") || slices.Contains(itemNames(category.Items), "Movie 0") {
			t.Errorf("a played title is recommended: %+v", category)
		}
	}
	// Unplayed again, it is recommended again; without similar titles, no
	// category, as no addon is asked.
	s.call(http.MethodDelete, "/UserPlayedItems/"+movies.Items[2].Id, app("tv", token), nil)
	if len(recommendations()) != 1 {
		t.Fatal("the category did not come back")
	}
	s.setting(t, func(settings *accounts.Settings) { settings.SimilarTitles = false })
	if got := recommendations(); len(got) != 0 {
		t.Errorf("with similar titles off: %+v", got)
	}
}

func TestSmallRoutesAnswerAsJellyfin(t *testing.T) {
	s, token, views := browsing(t)
	var movies QueryResult
	s.get(t, "/Items?ParentId="+views["Top"], token, &movies)
	movie := movies.Items[0]
	for path, want := range map[string]string{
		"/Trailers?startIndex=2":              `{"Items":[],"TotalRecordCount":0,"StartIndex":2}`,
		"/Channels":                           `{"Items":[],"TotalRecordCount":0,"StartIndex":0}`,
		"/Channels/Features":                  `[]`,
		"/Channels/Items/Latest":              `{"Items":[],"TotalRecordCount":0,"StartIndex":0}`,
		"/Items/" + movie.Id + "/ThemeSongs":  `{"OwnerId":"` + movie.Id + `","Items":[],"TotalRecordCount":0,"StartIndex":0}`,
		"/Items/" + movie.Id + "/ThemeVideos": `{"OwnerId":"` + movie.Id + `","Items":[],"TotalRecordCount":0,"StartIndex":0}`,
		"/UserViews/GroupingOptions":          `[{"Name":"Shows","Id":"` + views["Shows"] + `"},{"Name":"Top","Id":"` + views["Top"] + `"}]`,
		"/Items/" + movie.Id + "/Images":      `[{"ImageType":"Primary","ImageTag":"` + movie.ImageTags["Primary"] + `","Size":0},{"ImageType":"Backdrop","ImageIndex":0,"ImageTag":"` + movie.BackdropImageTags[0] + `","Size":0}]`,
	} {
		status, body := s.call(http.MethodGet, path, app("tv", token), nil)
		if status != http.StatusOK || strings.TrimSpace(string(body)) != want {
			t.Errorf("%s: %d %s\nwant %s", path, status, body, want)
		}
	}
	for path, want := range map[string]int{
		// Jellyfin finds no plugin for a channel it does not know.
		"/Channels/" + strings.Repeat("ab", 16) + "/Features": http.StatusBadRequest,
		"/Channels/" + strings.Repeat("ab", 16) + "/Items":    http.StatusBadRequest,
		"/Items/" + strings.Repeat("ab", 16) + "/ThemeSongs":  http.StatusNotFound,
		"/Items/" + strings.Repeat("ab", 16) + "/Images":      http.StatusNotFound,
	} {
		if status, body := s.call(http.MethodGet, path, app("tv", token), nil); status != want {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
	// Only administrators read another user's grouping options.
	admin := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	if status, _ := s.call(http.MethodGet, "/Users/"+admin.ID.String()+"/GroupingOptions", app("tv", token), nil); status != http.StatusForbidden {
		t.Errorf("another user's grouping options: %d", status)
	}

	// Polyfin's sources open without a live stream; closing one succeeds.
	if status, body := s.call(http.MethodPost, "/LiveStreams/Open?itemId="+movie.Id, app("tv", token), nil); status != http.StatusBadRequest || string(body) != "Error processing request." {
		t.Errorf("open: %d %s", status, body)
	}
	if status, _ := s.call(http.MethodPost, "/LiveStreams/Close?liveStreamId=anything", app("tv", token), nil); status != http.StatusNoContent {
		t.Errorf("close: %d", status)
	}
	if status, body := s.call(http.MethodPost, "/LiveStreams/Close", app("tv", token), nil); status != http.StatusBadRequest || !strings.Contains(string(body), "liveStreamId") {
		t.Errorf("close without a stream: %d %s", status, body)
	}

	// A session reports what it shows, which session lists give.
	if status, body := s.call(http.MethodPost, "/Sessions/Viewing?itemId="+movie.Id, app("tv", token), nil); status != http.StatusNoContent {
		t.Fatalf("viewing: %d %s", status, body)
	}
	sessions := s.sessions(t, token, "")
	if len(sessions) != 1 || sessions[0].NowViewingItem == nil || sessions[0].NowViewingItem.Id != movie.Id {
		t.Errorf("sessions: %+v", sessions)
	}
	if status, _ := s.call(http.MethodPost, "/Sessions/Viewing", app("tv", token), nil); status != http.StatusBadRequest {
		t.Errorf("viewing nothing: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Sessions/Viewing?itemId="+strings.Repeat("ab", 16), app("tv", token), nil); status != http.StatusNotFound {
		t.Errorf("viewing an unknown item: %d", status)
	}
	// Another user's session is the administrator's to set.
	adminToken := s.signIn("admin", "phone")
	other := s.sessions(t, adminToken, "deviceId=phone-id")[0].Id
	if status, _ := s.call(http.MethodPost, "/Sessions/Viewing?itemId="+movie.Id+"&sessionId="+other, app("tv", token), nil); status != http.StatusForbidden {
		t.Errorf("viewing on another user's session: %d", status)
	}
}

func TestYearsAreThoseTheYearPagesOffer(t *testing.T) {
	s, token, _, views := pagesServer(t)
	years := func(query string) []string {
		t.Helper()
		var result QueryResult
		if status := s.get(t, "/Years?"+query, token, &result); status != http.StatusOK || result.TotalRecordCount != len(result.Items) && !strings.Contains(query, "limit") {
			t.Fatalf("years %s: %d %+v", query, status, result)
		}
		names := itemNames(result.Items)
		for _, item := range result.Items {
			if item.Type != "Year" || item.Id != nameID("year", item.Name) {
				t.Errorf("year item: %+v", item)
			}
		}
		return names
	}
	for query, want := range map[string][]string{
		"":                                     {"2020", "2021"},
		"sortOrder=Descending":                 {"2021", "2020"},
		"includeItemTypes=Movie":               {"2020", "2021"},
		"includeItemTypes=Series":              {},
		"parentId=" + views["Yearly"]:          {"2020", "2021"},
		"parentId=" + views["Popular"]:         {},
		"startIndex=1&limit=1":                 {"2021"},
		"parentId=" + strings.Repeat("ab", 16): {},
	} {
		if got := years(query); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", query, got, want)
		}
	}
}

func TestFallbackFontsComeFromTheFontsFolder(t *testing.T) {
	s, token, _ := browsing(t)
	dir := t.TempDir()
	for name, content := range map[string]string{
		"Big.ttf": "big font bytes", "family/Small.OTF": "small", "family/notes.txt": "not a font",
		"other/Big.ttf": "the same name again", "Empty.woff": "",
	} {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.handler.FontsDir = dir
	var fonts []FontFile
	if status := s.get(t, "/FallbackFont/Fonts", token, &fonts); status != http.StatusOK {
		t.Fatalf("fonts: %d", status)
	}
	var names []string
	for _, font := range fonts {
		names = append(names, font.Name)
	}
	// Smallest first, empty files left out.
	if !slices.Equal(names, []string{"Small.OTF", "Big.ttf"}) || fonts[0].Size != 5 {
		t.Errorf("fonts: %+v", fonts)
	}
	status, header, body := s.send(http.MethodGet, "/FallbackFont/Fonts/small.otf", app("tv", token), "", "")
	if status != http.StatusOK || string(body) != "small" || header.Get("Content-Type") != "font/otf" {
		t.Errorf("font: %d %v %q", status, header, body)
	}
	// Like Jellyfin, an unknown font answers nothing, successfully.
	if status, _, body := s.send(http.MethodGet, "/FallbackFont/Fonts/notes.txt", app("tv", token), "", ""); status != http.StatusOK || len(body) != 0 {
		t.Errorf("not a font: %d %q", status, body)
	}
	if status, _, _ := s.send(http.MethodGet, "/FallbackFont/Fonts", "", "", ""); status != http.StatusUnauthorized {
		t.Errorf("without credentials: %d", status)
	}
	// Without the folder, none, as Jellyfin answers without one.
	s.handler.FontsDir = filepath.Join(dir, "missing")
	recorded := recordedAnswers(t)["Fallback fonts as viewer"]
	status, _, body = s.send(http.MethodGet, "/FallbackFont/Fonts", app("tv", token), "", "")
	if status != recorded.Status || strings.TrimSpace(string(body)) != string(recorded.Body) {
		t.Errorf("without fonts: %d %s, want %d %s", status, body, recorded.Status, recorded.Body)
	}
	// jellyfin-web loads fallback fonts only when the encoding options
	// enable them: when there are some.
	var encoding EncodingOptions
	if s.get(t, "/System/Configuration/encoding", token, &encoding); encoding.EnableFallbackFont {
		t.Error("fallback fonts enabled without fonts")
	}
	s.handler.FontsDir = dir
	if s.get(t, "/System/Configuration/encoding", token, &encoding); !encoding.EnableFallbackFont {
		t.Error("fallback fonts not enabled with fonts")
	}
}

// API keys have no user or session of their own: the routes that act for
// one answer as Jellyfin does without one, and change nothing.
func TestAPIKeysWithoutAUserChangeNothing(t *testing.T) {
	s, token, views := browsing(t)
	_, key, err := s.store.CreateAPIKey(t.Context(), "Script")
	if err != nil {
		t.Fatal(err)
	}
	var movies QueryResult
	s.get(t, "/Items?ParentId="+views["Top"], token, &movies)
	photo := base64.StdEncoding.EncodeToString(picture(t, 10, 10, 255))
	for _, test := range []struct {
		method, path, contentType, body string
		want                            int
	}{
		{http.MethodGet, "/UserImage", "", "", http.StatusBadRequest},
		{http.MethodPost, "/UserImage", "image/png", photo, http.StatusNotFound},
		{http.MethodDelete, "/UserImage", "", "", http.StatusNotFound},
		{http.MethodPost, "/Sessions/Viewing?itemId=" + movies.Items[0].Id, "", "", http.StatusBadRequest},
		{http.MethodGet, "/Movies/Recommendations", "", "", http.StatusOK},
		{http.MethodGet, "/Search/Hints?searchTerm=Movie", "", "", http.StatusOK},
	} {
		if status, _, body := s.send(test.method, test.path, app("script", key), test.contentType, test.body); status != test.want {
			t.Errorf("%s %s: %d %s", test.method, test.path, status, body)
		}
	}
	var count int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM users WHERE image IS NOT NULL").Scan(&count); err != nil || count != 0 {
		t.Errorf("pictures stored: %d %v", count, err)
	}
	if sessions := s.sessions(t, token, ""); len(sessions) != 1 || sessions[0].NowViewingItem != nil {
		t.Errorf("sessions: %+v", sessions)
	}
}

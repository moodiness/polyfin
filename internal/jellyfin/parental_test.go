package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// ratingsAddon serves movies rated as the fixture titles were (Restricted R,
// Allowed PG, Unrated none) and a TV-MA series, with streams. Its catalog
// rows carry no rating, as AIOMetadata's do. Ann Lee plays in Allowed and
// Restricted, both dramas, Rex Only in Restricted alone; a people search
// finds Ann Lee's.
func ratingsAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	ratings := map[string]string{"tt1": "PG", "tt2": "R", "tt3": "", "tt4": "TV-MA"}
	meta := func(id string) stremio.Meta {
		names := map[string]string{"tt1": "Allowed", "tt2": "Restricted", "tt3": "Unrated", "tt4": "Show"}
		m := stremio.Meta{ID: id, Type: "movie", Name: names[id], Extras: &stremio.Extras{Certification: ratings[id]}}
		switch id {
		case "tt1":
			m.Genres, m.Extras.Cast = []string{"Drama"}, []stremio.CastMember{{Name: "Ann Lee"}}
		case "tt2":
			m.Genres, m.Extras.Cast = []string{"Drama"}, []stremio.CastMember{{Name: "Ann Lee"}, {Name: "Rex Only"}}
		case "tt4":
			m.Type = "series"
			m.Videos = []stremio.Video{{ID: "tt4:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z"}}
		}
		return m
	}
	row := func(id string) stremio.Meta {
		m := meta(id)
		m.Extras, m.Videos = nil, nil
		return m
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "rated", Name: "Rated", Version: "1",
				Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}, {Name: "subtitles"}},
				Catalogs: []stremio.Catalog{
					{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search"}, {Name: "genre", Options: []string{"Drama"}}}},
					{Type: "series", ID: "shows", Name: "Shows"},
					{Type: "movie", ID: "people_search.people_search_movie", Name: "People Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}},
				}})
		case strings.HasPrefix(path, "/catalog/movie/people_search.people_search_movie/"):
			var found []stremio.Meta
			if strings.Contains(path, "search=Ann%20Lee") {
				found = []stremio.Meta{row("tt2"), row("tt1")}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": found})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			metas := []stremio.Meta{row("tt1"), row("tt2"), row("tt3")}
			if strings.Contains(path, "search=") {
				metas = []stremio.Meta{row("tt2")}
			} else if strings.Contains(path, "genre=Drama") {
				metas = metas[:2]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{row("tt4")}})
		case len(parts) == 3 && parts[0] == "meta":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta(parts[2])})
		case len(parts) == 3 && parts[0] == "stream":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "1080p", URL: server.URL + "/files/" + parts[2] + ".mkv",
				BehaviorHints: stremio.StreamBehavior{Filename: parts[2] + ".mkv", VideoSize: 4_000_000_000}}}})
		case len(parts) == 3 && parts[0] == "subtitles":
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{{ID: "en", URL: "https://subs.example/en.srt", Lang: "eng"}}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// postRaw posts a body as is, as JSON.
func (s testServer) postRaw(path, authorization, body string) (int, []byte) {
	s.t.Helper()
	request, _ := http.NewRequestWithContext(s.t.Context(), http.MethodPost, s.url+path, strings.NewReader(body))
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		s.t.Fatal(err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

// parentalAnswer is how Jellyfin 12.1 answered a restricted user's request.
type parentalAnswer struct {
	Status int
	Body   json.RawMessage
	Names  []string
}

func recordedParental(t *testing.T) map[string]parentalAnswer {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "parental", "answers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var answers map[string]parentalAnswer
	if err := json.Unmarshal(raw, &answers); err != nil {
		t.Fatal(err)
	}
	return answers
}

// sameAnswer reports whether an answer is Jellyfin's: its status, a string
// body, the fields and errors of a problem (not its trace), or the names of
// a listing.
func sameAnswer(want parentalAnswer, status int, body []byte) bool {
	if status != want.Status {
		return false
	}
	if want.Names != nil {
		var page QueryResult
		var items []BaseItemDto
		if json.Unmarshal(body, &page) == nil && page.Items != nil {
			items = page.Items
		} else {
			_ = json.Unmarshal(body, &items)
		}
		return slices.Equal(itemNames(items), want.Names)
	}
	if len(want.Body) == 0 {
		return true
	}
	var wanted, got any
	if json.Unmarshal(want.Body, &wanted) != nil || json.Unmarshal(body, &got) != nil {
		return false
	}
	if problem, ok := wanted.(map[string]any); ok {
		answer, ok := got.(map[string]any)
		if !ok {
			return false
		}
		delete(problem, "traceId")
		delete(answer, "traceId")
		// Jellyfin's conversion messages also give where in the request
		// the value was.
		for _, errs := range []any{problem["errors"], answer["errors"]} {
			if fields, ok := errs.(map[string]any); ok {
				for name, messages := range fields {
					list := messages.([]any)
					for i, message := range list {
						text, _ := message.(string)
						text, _, _ = strings.Cut(text, " | LineNumber")
						list[i] = strings.TrimSuffix(text, ".")
					}
					fields[name] = list
				}
			}
		}
	}
	return reflect.DeepEqual(wanted, got)
}

func TestParentalControlMatchesJellyfin(t *testing.T) {
	s := newTestServer(t, 10)
	admin := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	child := s.user("child", nil)
	s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), ratingsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	adminToken, childToken, memberToken := s.signIn("admin", "tv"), s.signIn("child", "tablet"), s.signIn("member", "phone")
	answers := recordedParental(t)
	check := func(key string, status int, body []byte) {
		t.Helper()
		want, ok := answers[key]
		if !ok {
			t.Fatalf("no recorded answer %s", key)
		}
		if !sameAnswer(want, status, body) {
			t.Errorf("%s: got %d %s, Jellyfin answered %d %s %v", key, status, body, want.Status, want.Body, want.Names)
		}
	}

	// The administrator's app posts the whole policy it read, changed, with
	// the score of the rating picked from the list.
	var ratings []ParentalRating
	s.get(t, "/Localization/ParentalRatings", adminToken, &ratings)
	pg13 := ratings[slices.IndexFunc(ratings, func(r ParentalRating) bool { return r.Name == "PG-13" })].RatingScore
	var user map[string]any
	s.get(t, "/Users/"+child.ID.String(), adminToken, &user)
	policy := user["Policy"].(map[string]any)
	policy["MaxParentalRating"], policy["MaxParentalSubRating"], policy["BlockUnratedItems"] = pg13.Score, pg13.SubScore, []string{"Movie"}
	encoded, _ := json.Marshal(policy)
	path := "/Users/" + child.ID.String() + "/Policy"

	status, body := s.postRaw(path, app("phone", memberToken), string(encoded))
	check("PolicyAsMember", status, body)
	status, body = s.postRaw(path, app("tv", adminToken), "")
	check("PolicyEmptyBody", status, body)
	unknownKind, _ := json.Marshal(map[string]any{"BlockUnratedItems": []string{"Film"}})
	status, body = s.postRaw(path, app("tv", adminToken), string(unknownKind))
	check("PolicyUnknownKind", status, body)
	status, body = s.postRaw("/Users/0123456789abcdef0123456789abcdef/Policy", app("tv", adminToken), string(encoded))
	check("PolicyUnknownUser", status, body)
	status, body = s.postRaw("/Users/"+admin.ID.String()+"/Policy", app("tv", adminToken), `{"IsAdministrator":true,"IsDisabled":true}`)
	check("PolicyDisableAdministrator", status, body)
	status, body = s.postRaw("/Users/"+admin.ID.String()+"/Policy", app("tv", adminToken), `{"IsAdministrator":false}`)
	check("PolicyLastAdministrator", status, body)
	status, body = s.postRaw(path, app("tv", adminToken), string(encoded))
	check("Policy", status, body)

	// The user's policy shows the limit as Jellyfin's does.
	status, body = s.call(http.MethodGet, "/Users/"+child.ID.String(), app("tv", adminToken), nil)
	check("ChildUser", status, body)
	var dto struct{ Policy map[string]any }
	_ = json.Unmarshal(body, &dto)
	raw, _ := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "parental", "policy.json"))
	var wantPolicy map[string]any
	_ = json.Unmarshal(raw, &wantPolicy)
	for key, value := range wantPolicy {
		if !reflect.DeepEqual(dto.Policy[key], value) {
			t.Errorf("policy %s: %v, Jellyfin has %v", key, dto.Policy[key], value)
		}
	}
	shape, _ := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "user-restricted.json"))
	var want, got any
	_ = json.Unmarshal(shape, &want)
	_ = json.Unmarshal(body, &got)
	for _, difference := range compareShapes("user-restricted", want, got, shapeRules{}) {
		t.Error(difference)
	}

	// The titles, as the unrestricted administrator lists them.
	var views QueryResult
	s.get(t, "/UserViews", adminToken, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, adminToken, &page)
		for _, item := range page.Items {
			ids[item.Name] = item.Id
		}
		ids[view.Name] = view.Id
	}
	var seasons, episodes QueryResult
	s.get(t, "/Shows/"+ids["Show"]+"/Seasons", adminToken, &seasons)
	s.get(t, "/Shows/"+ids["Show"]+"/Episodes", adminToken, &episodes)
	if len(seasons.Items) != 1 || len(episodes.Items) != 1 {
		t.Fatalf("show: %d seasons, %d episodes", len(seasons.Items), len(episodes.Items))
	}
	childApp := app("tablet", childToken)
	childID := child.ID.String()
	for key, request := range map[string]string{
		"MovieRestricted":    "/Users/" + childID + "/Items/" + ids["Restricted"],
		"MovieAllowed":       "/Users/" + childID + "/Items/" + ids["Allowed"],
		"MovieUnrated":       "/Users/" + childID + "/Items/" + ids["Unrated"],
		"SeriesRestricted":   "/Users/" + childID + "/Items/" + ids["Show"],
		"SeasonRestricted":   "/Users/" + childID + "/Items/" + seasons.Items[0].Id,
		"EpisodeRestricted":  "/Users/" + childID + "/Items/" + episodes.Items[0].Id,
		"SeasonsRestricted":  "/Shows/" + ids["Show"] + "/Seasons?userId=" + childID,
		"EpisodesRestricted": "/Shows/" + ids["Show"] + "/Episodes?userId=" + childID,
		"Movies":             "/Items?userId=" + childID + "&parentId=" + ids["Top"] + "&includeItemTypes=Movie&recursive=true&sortBy=SortName",
		"Shows":              "/Items?userId=" + childID + "&parentId=" + ids["Shows"] + "&includeItemTypes=Series&recursive=true",
		"Search":             "/Items?userId=" + childID + "&recursive=true&searchTerm=restricted&includeItemTypes=Movie",
		"Latest":             "/Items/Latest?userId=" + childID + "&parentId=" + ids["Top"],
	} {
		status, body := s.call(http.MethodGet, request, childApp, nil)
		// Jellyfin's titles were named after its test movies.
		if names := answers[key].Names; len(names) > 0 {
			answers[key] = parentalAnswer{Status: answers[key].Status, Names: []string{"Allowed"}}
		}
		check(key, status, body)
	}
	status, body = s.call(http.MethodPost, "/Items/"+ids["Restricted"]+"/PlaybackInfo?userId="+childID, childApp, map[string]any{})
	check("PlaybackInfoRestricted", status, body)
	status, body = s.call(http.MethodPost, "/UserFavoriteItems/"+ids["Restricted"]+"?userId="+childID, childApp, nil)
	check("FavoriteRestricted", status, body)
	status, body = s.call(http.MethodGet, "/Users/"+admin.ID.String()+"/Items/"+ids["Restricted"], app("tv", adminToken), nil)
	check("Unrestricted", status, body)

	// The subtitle search and playlists keep to the limit too.
	if status, _ := s.call(http.MethodGet, "/Items/"+ids["Restricted"]+"/RemoteSearch/Subtitles/eng", childApp, nil); status != http.StatusNotFound {
		t.Errorf("subtitle search of a hidden title: %d", status)
	}
	playlist := s.createPlaylist(t, adminToken, map[string]any{"Name": "Mix", "Ids": []string{ids["Allowed"], ids["Restricted"]}, "IsPublic": true})
	if entries := s.playlistEntries(t, childToken, playlist); !slices.Equal(itemNames(entries), []string{"Allowed"}) {
		t.Errorf("playlist as the child sees it: %v", itemNames(entries))
	}
	if entries := s.playlistEntries(t, adminToken, playlist); len(entries) != 2 {
		t.Errorf("playlist as the administrator sees it: %v", itemNames(entries))
	}
}

func TestParentalControlCoversPeopleSimilarTitlesAndDownloads(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), ratingsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	adminToken, childToken := s.signIn("admin", "tv"), s.signIn("child", "tablet")
	var views, page QueryResult
	s.get(t, "/UserViews", adminToken, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, adminToken, &page)
	ids := map[string]string{}
	for _, item := range page.Items {
		ids[item.Name] = item.Id
	}
	// The administrator opens both dramas, which records their credits.
	people := map[string]string{}
	for _, title := range []string{"Allowed", "Restricted"} {
		var item BaseItemDto
		if status := s.get(t, "/Items/"+ids[title], adminToken, &item); status != http.StatusOK || item.People == nil {
			t.Fatalf("%s: %d", title, status)
		}
		for _, person := range *item.People {
			people[person.Name] = person.Id
		}
	}
	names := func(token, path string) []string {
		t.Helper()
		var page QueryResult
		if status := s.get(t, path, token, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		return itemNames(page.Items)
	}

	// The person page counts, and lists, only the titles the child may see.
	var ann namedItemDto
	if status := s.get(t, "/Items/"+people["Ann Lee"], childToken, &ann); status != http.StatusOK || ann.MovieCount != 1 {
		t.Errorf("Ann Lee for the child: %d, %d movies", status, ann.MovieCount)
	}
	if s.get(t, "/Items/"+people["Ann Lee"], adminToken, &ann); ann.MovieCount != 2 {
		t.Errorf("Ann Lee for the administrator: %d movies", ann.MovieCount)
	}
	titles := "/Items?personIds=" + people["Ann Lee"] + "&recursive=true&includeItemTypes=Movie"
	if got := names(childToken, titles); !slices.Equal(got, []string{"Allowed"}) {
		t.Errorf("Ann Lee's titles for the child: %v", got)
	}
	if got := names(adminToken, titles); !slices.Contains(got, "Restricted") {
		t.Errorf("Ann Lee's titles for the administrator: %v", got)
	}
	// Someone credited only in a hidden title is not found, nor counted.
	if status := s.get(t, "/Items/"+people["Rex Only"], childToken, nil); status != http.StatusNotFound {
		t.Errorf("Rex Only for the child: %d", status)
	}
	var persons QueryResult
	s.get(t, "/Persons", childToken, &persons)
	if got := itemNames(persons.Items); !slices.Equal(got, []string{"Ann Lee"}) || persons.TotalRecordCount != 1 {
		t.Errorf("people for the child: %v (%d)", got, persons.TotalRecordCount)
	}
	if got := names(adminToken, "/Persons"); !slices.Equal(got, []string{"Ann Lee", "Rex Only"}) {
		t.Errorf("people for the administrator: %v", got)
	}

	// Similar titles leave out what the child may not see.
	if got := names(childToken, "/Items/"+ids["Allowed"]+"/Similar"); slices.Contains(got, "Restricted") {
		t.Errorf("similar titles for the child: %v", got)
	}
	if got := names(adminToken, "/Items/"+ids["Allowed"]+"/Similar"); !slices.Contains(got, "Restricted") {
		t.Errorf("similar titles for the administrator: %v", got)
	}

	// A hidden title is refused for download, as for streaming.
	download := func(token, title string) int {
		response, _ := fetchURL(t, s.url+"/Items/"+ids[title]+"/Download?ApiKey="+token, nil)
		return response.StatusCode
	}
	if status := download(childToken, "Restricted"); status != http.StatusNotFound {
		t.Errorf("child downloading Restricted: %d", status)
	}
	if status := download(childToken, "Allowed"); status != http.StatusOK {
		t.Errorf("child downloading Allowed: %d", status)
	}
	if status := download(adminToken, "Restricted"); status != http.StatusOK {
		t.Errorf("administrator downloading Restricted: %d", status)
	}
}

// A user an administrator disables through their policy is signed out of
// every device, and leaves the SyncPlay groups their apps were in.
func TestDisablingAUsersPolicySignsTheirAppsOut(t *testing.T) {
	p := newSyncPlayers(t)
	p.testServer.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	admin := p.signIn("admin", "admin-tv")
	bob, _ := p.store.Authenticate(t.Context(), "bob", "correct horse")
	if status, body := p.postRaw("/Users/"+bob.ID.String()+"/Policy", app("admin-tv", admin), `{"IsDisabled":true}`); status != http.StatusNoContent {
		t.Fatalf("disabling bob: %d %s", status, body)
	}
	p.bob.ended(t)
	p.alice.expect(t, "UserLeft")
	if status, _ := p.call(http.MethodGet, "/Users/Me", app("bob-tv", p.bob.token), nil); status != http.StatusUnauthorized {
		t.Errorf("bob's app still signed in: %d", status)
	}
	if status, _ := p.call(http.MethodGet, "/Users/Me", app("admin-tv", admin), nil); status != http.StatusOK {
		t.Errorf("the administrator's app was signed out: %d", status)
	}
}

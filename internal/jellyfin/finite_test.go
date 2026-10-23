package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// unboundedAddon serves movies whose IMDb ratings are text that reads as
// a number that is not finite, and one rated 7.5.
func unboundedAddon(t *testing.T) string {
	t.Helper()
	ratings := map[string]string{"tt1": "Infinity", "tt2": "NaN", "tt3": "1e999", "tt4": "-inf", "tt5": "7.5"}
	names := map[string]string{"tt1": "Endless", "tt2": "Undefined", "tt3": "Huge", "tt4": "Below", "tt5": "Rated"}
	meta := func(id string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "movie", Name: names[id], ReleaseInfo: "2020", ImdbRating: stremio.Text(ratings[id])}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "unbounded", Name: "Unbounded", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{meta("tt1"), meta("tt2"), meta("tt3"), meta("tt4"), meta("tt5")}})
		case strings.HasPrefix(path, "/meta/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta(strings.TrimPrefix(path, "/meta/movie/"))})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A rating that is not a finite number is no rating, as in Jellyfin 12.3,
// which keeps none (#18267): JSON has no such numbers, and an answer that
// held one could not be written. An addon's counts as none; one sent to the
// metadata editor ("NaN", "Infinity", "-Infinity", or a number too large
// for a float) is taken as no value, which Jellyfin answers 204 to: the
// field follows the addon again.
func TestRatingsThatAreNotFiniteNumbersAreNone(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), unboundedAddon(t), false); err != nil {
		t.Fatal(err)
	}
	admin := s.signIn("admin", "dashboard")
	var views QueryResult
	s.get(t, "/UserViews", admin, &views)
	if len(views.Items) != 1 {
		t.Fatalf("views: %v", itemNames(views.Items))
	}
	status, body := s.call(http.MethodGet, "/Items?ParentId="+views.Items[0].Id, app("dashboard", admin), nil)
	if status != http.StatusOK {
		t.Fatalf("listing: %d %s", status, body)
	}
	var listing QueryResult
	if err := json.Unmarshal(body, &listing); err != nil {
		t.Fatalf("listing: %v: %s", err, body)
	}
	ids := map[string]string{}
	for _, item := range listing.Items {
		ids[item.Name] = item.Id
		if rated := item.CommunityRating != nil; rated != (item.Name == "Rated") {
			t.Errorf("%s listed with CommunityRating %v", item.Name, item.CommunityRating)
		}
	}
	if len(ids) != 5 {
		t.Fatalf("listed: %v", ids)
	}
	details := func(name string) BaseItemDto {
		t.Helper()
		status, body := s.call(http.MethodGet, "/Items/"+ids[name], app("dashboard", admin), nil)
		var dto BaseItemDto
		if status != http.StatusOK || json.Unmarshal(body, &dto) != nil {
			t.Fatalf("%s: %d %s", name, status, body)
		}
		return dto
	}
	for _, name := range []string{"Endless", "Undefined", "Huge", "Below"} {
		if dto := details(name); dto.CommunityRating != nil {
			t.Errorf("%s: CommunityRating %v", name, *dto.CommunityRating)
		}
	}

	rated := ids["Rated"]
	edit := func(fields map[string]any) (int, []byte) {
		t.Helper()
		return s.call(http.MethodPost, "/Items/"+rated, app("dashboard", admin), editorBody(rated, fields))
	}
	if status, body := edit(map[string]any{"Name": "Rated", "CommunityRating": "9", "CriticRating": "88"}); status != http.StatusNoContent {
		t.Fatalf("edit: %d %s", status, body)
	}
	for _, value := range []string{"NaN", "Infinity", "-Infinity", "1e999"} {
		var status int
		var body []byte
		if value == "1e999" {
			// Sent as a JSON number, too large for a float.
			raw, _ := json.Marshal(editorBody(rated, map[string]any{"Name": "Rated", "CommunityRating": "@", "CriticRating": "@"}))
			status, body = s.postRaw("/Items/"+rated, app("dashboard", admin), strings.ReplaceAll(string(raw), `"@"`, value))
		} else {
			status, body = edit(map[string]any{"Name": "Rated", "CommunityRating": value, "CriticRating": value})
		}
		if status != http.StatusNoContent {
			t.Errorf("%s: %d %s", value, status, body)
			continue
		}
		dto := details("Rated")
		if dto.CommunityRating == nil || *dto.CommunityRating != 7.5 || dto.CriticRating != nil {
			t.Errorf("after %s: CommunityRating %v, CriticRating %v, want the addon's 7.5 and none", value, dto.CommunityRating, dto.CriticRating)
		}
		if status, body := edit(map[string]any{"Name": "Rated", "CommunityRating": "9", "CriticRating": "88"}); status != http.StatusNoContent {
			t.Fatalf("edit again: %d %s", status, body)
		}
	}
}

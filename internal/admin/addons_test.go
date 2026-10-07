package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func localAddon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "local", "name": "Local", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}]}`))
	}))
	t.Cleanup(server.Close)
	return server.URL + "/u/secret-token/manifest.json"
}

func TestAddonScopes(t *testing.T) {
	api := newTestAPI(t, 10)
	manifestURL := localAddon(t)
	member := api.signedIn("member", false)
	administrator := api.signedIn("administrator", true)

	if status, body, _ := member.call(http.MethodGet, "/scopes/shared/addons", nil); status != http.StatusForbidden || body["error"] != "forbidden" {
		t.Errorf("member reading shared addons: %d %v", status, body)
	}
	// Only administrators may make the server reach local networks.
	if status, body, _ := member.call(http.MethodPost, "/scopes/me/addons", map[string]string{"manifestUrl": manifestURL}); status != http.StatusForbidden || body["error"] != "private_network" {
		t.Errorf("member installing a local addon: %d %v", status, body)
	}
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": manifestURL})
	if status != http.StatusCreated {
		t.Fatalf("administrator installing a local addon: %d %v", status, body)
	}
	if url, _ := body["manifestUrl"].(string); strings.Contains(url, "secret-token") || !strings.HasSuffix(url, "/…/manifest.json") {
		t.Errorf("manifest URL not redacted: %q", url)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": "https://example.com/catalog.json"}); status != http.StatusBadRequest || body["error"] != "invalid_manifest_url" {
		t.Errorf("invalid URL: %d %v", status, body)
	}
}

func TestAddonPreferences(t *testing.T) {
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	if _, body, _ := member.call(http.MethodGet, "/account/addon-preferences", nil); body["useSharedAddons"] != true {
		t.Errorf("default preference: %v", body)
	}
	member.call(http.MethodPut, "/account/addon-preferences", map[string]bool{"useSharedAddons": false})
	if _, body, _ := member.call(http.MethodGet, "/account/addon-preferences", nil); body["useSharedAddons"] != false {
		t.Errorf("saved preference: %v", body)
	}
}

// appNames lists the names apps show for a scope's libraries, "" for the
// libraries they do not show.
func (b browser) appNames(scope string) []string {
	b.api.t.Helper()
	response, err := b.client.Get(b.api.url + "/admin/api/scopes/" + scope + "/libraries")
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	var libraries []struct {
		AppName *string `json:"appName"`
	}
	if err := json.NewDecoder(response.Body).Decode(&libraries); err != nil {
		b.api.t.Fatal(err)
	}
	names := make([]string, len(libraries))
	for i, library := range libraries {
		if library.AppName != nil {
			names[i] = *library.AppName
		}
	}
	return names
}

func TestLibrariesShowTheNameAppsShow(t *testing.T) {
	api := newTestAPI(t, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "local", "name": "Local", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}, {"type": "series", "id": "top", "name": "Top"},
				{"type": "movie", "id": "search", "name": "Search", "extra": [{"name": "search", "isRequired": true}]}]}`))
	}))
	defer server.Close()
	administrator := api.signedIn("administrator", true)
	for _, scope := range []string{"shared", "me"} {
		if status, body, _ := administrator.call(http.MethodPost, "/scopes/"+scope+"/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"}); status != http.StatusCreated {
			t.Fatalf("installing in %s: %d %v", scope, status, body)
		}
	}
	if got := administrator.appNames("shared"); !slices.Equal(got, []string{"Top (Movies)", "Top (Shows)", ""}) {
		t.Errorf("shared libraries: %q", got)
	}
	// A user's own libraries are named after the server's they follow.
	want := []string{"Top (Movies, Local) (2)", "Top (Shows, Local) (2)", ""}
	if got := administrator.appNames("me"); !slices.Equal(got, want) {
		t.Errorf("own libraries: %q, want %q", got, want)
	}
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "fr"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("saving French: %d %v", status, body)
	}
	if got := administrator.appNames("shared"); !slices.Equal(got, []string{"Top (Films)", "Top (Séries)", ""}) {
		t.Errorf("shared libraries in French: %q", got)
	}
}

// narrowedLibrary is what the admin app reads of a library's genre and
// maximum.
type narrowedLibrary struct {
	AddonID     string   `json:"addonId"`
	CatalogType string   `json:"catalogType"`
	CatalogID   string   `json:"catalogId"`
	AppName     *string  `json:"appName"`
	Genre       *string  `json:"genre"`
	Genres      []string `json:"genres"`
	MaxItems    *int     `json:"maxItems"`
	Filterable  bool     `json:"filterable"`
}

// saveLibraries puts the server's libraries, and answers the status, the
// libraries answered and the error code.
func (b browser) saveLibraries(libraries ...map[string]any) (int, []narrowedLibrary, string) {
	b.api.t.Helper()
	encoded, _ := json.Marshal(map[string]any{"libraries": libraries})
	request, _ := http.NewRequestWithContext(b.api.t.Context(), http.MethodPut, b.api.url+"/admin/api/scopes/shared/libraries", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &failure)
		return response.StatusCode, nil, failure.Error
	}
	var saved []narrowedLibrary
	if err := json.Unmarshal(raw, &saved); err != nil {
		b.api.t.Fatal(err)
	}
	return response.StatusCode, saved, ""
}

// An administrator narrows a library to one of its catalog's genres and
// sets the most titles it lists; apps name it after the genre. A genre the
// catalog does not offer, a maximum that is not a whole number from 1 to
// 20,000, and either on a live TV catalog are refused. A collection catalog
// that requires its only genre takes a maximum, but offers no genre.
func TestLibrariesTakeAGenreAndAMaximum(t *testing.T) {
	api := newTestAPI(t, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "local", "name": "Local", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top", "extra": [{"name": "genre", "options": ["Comedy", "Drama"]}]},
				{"type": "tv", "id": "channels", "name": "Channels"},
				{"type": "collection", "id": "sets", "name": "Sets", "extra": [{"name": "genre", "isRequired": true, "options": ["None"]}]}]}`))
	}))
	defer server.Close()
	administrator := api.signedIn("administrator", true)
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, body)
	}
	addonID, _ := body["id"].(string)
	top := func(extra map[string]any) map[string]any {
		library := map[string]any{"addonId": addonID, "catalogType": "movie", "catalogId": "top", "name": nil}
		maps.Copy(library, extra)
		return library
	}
	status, saved, code := administrator.saveLibraries(top(map[string]any{"genre": "Drama", "maxItems": 40}))
	if status != http.StatusOK {
		t.Fatalf("saving: %d %s", status, code)
	}
	byID := map[string]narrowedLibrary{}
	for _, l := range saved {
		byID[l.CatalogType+"/"+l.CatalogID] = l
	}
	if l := byID["movie/top"]; l.Genre == nil || *l.Genre != "Drama" || l.MaxItems == nil || *l.MaxItems != 40 || !l.Filterable ||
		!slices.Equal(l.Genres, []string{"Comedy", "Drama"}) || l.AppName == nil || *l.AppName != "Top · Drama" {
		t.Errorf("the narrowed library: %+v", l)
	}
	if l := byID["tv/channels"]; l.Filterable || l.Genres == nil || len(l.Genres) != 0 || l.Genre != nil || l.MaxItems != nil {
		t.Errorf("a live TV catalog: %+v", l)
	}
	if l := byID["collection/sets"]; !l.Filterable || l.Genres == nil || len(l.Genres) != 0 {
		t.Errorf("a collection catalog that requires its only genre: %+v", l)
	}

	channels := func(extra map[string]any) map[string]any {
		library := map[string]any{"addonId": addonID, "catalogType": "tv", "catalogId": "channels"}
		maps.Copy(library, extra)
		return library
	}
	for name, tc := range map[string]struct {
		library map[string]any
		code    string
	}{
		"a genre the catalog does not offer": {top(map[string]any{"genre": "Horror"}), "invalid_library_genre"},
		"a genre for a live TV catalog":      {channels(map[string]any{"genre": "Drama"}), "invalid_library_genre"},
		"a maximum that is no whole number":  {top(map[string]any{"maxItems": 12.5}), "invalid_library_max_items"},
		"no title at most":                   {top(map[string]any{"maxItems": 0}), "invalid_library_max_items"},
		"more than the most":                 {top(map[string]any{"maxItems": 20001}), "invalid_library_max_items"},
		"a maximum for a live TV catalog":    {channels(map[string]any{"maxItems": 10}), "invalid_library_max_items"},
	} {
		if status, _, code := administrator.saveLibraries(tc.library); status != http.StatusBadRequest || code != tc.code {
			t.Errorf("%s: %d %s, want %s", name, status, code, tc.code)
		}
	}

	// Saved without them, the library lists its whole catalog again.
	if status, saved, code := administrator.saveLibraries(top(nil)); status != http.StatusOK || saved[0].Genre != nil || saved[0].MaxItems != nil ||
		saved[0].AppName == nil || *saved[0].AppName != "Top" {
		t.Errorf("cleared: %d %s %+v", status, code, saved)
	}
}

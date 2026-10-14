package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// listAddon serves Top, a movie catalog of two titles, Comedy (1998) and
// Drama (2003), which offers a genre filter.
func listAddon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/manifest.json"):
			_, _ = w.Write([]byte(`{"id": "lists", "name": "Lists", "version": "1.0.0", "resources": ["catalog", "meta"], "types": ["movie"],
				"catalogs": [{"type": "movie", "id": "top", "name": "Top", "extra": [{"name": "genre", "options": ["Comedy", "Drama"]}]},
					{"type": "movie", "id": "search", "name": "Search", "extra": [{"name": "search", "isRequired": true}]}]}`))
		case strings.HasPrefix(r.URL.Path, "/catalog/movie/top"):
			_, _ = w.Write([]byte(`{"metas": [{"id": "tt1", "type": "movie", "name": "Comedy", "genres": ["Comedy"], "releaseInfo": "1998", "imdbRating": "7.1"},
				{"id": "tt2", "type": "movie", "name": "Drama", "genres": ["Drama"], "releaseInfo": "2003"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// read gets path from the admin API, whatever its answer.
func (b browser) read(path string) (int, any) {
	b.api.t.Helper()
	request, _ := http.NewRequestWithContext(b.api.t.Context(), http.MethodGet, b.api.url+"/admin/api"+path, nil)
	answer, err := b.client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer answer.Body.Close()
	var decoded any
	_ = json.NewDecoder(answer.Body).Decode(&decoded)
	return answer.StatusCode, decoded
}

func TestSmartListsAreMadeAndPreviewedInTheirScope(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	status, addon, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": listAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, addon)
	}

	// The catalogs a list may draw from, searches left out, and the
	// languages its rules may name.
	_, choices := administrator.read("/scopes/shared/smart-lists/catalogs")
	catalogs, _ := choices.(map[string]any)["catalogs"].([]any)
	languages, _ := choices.(map[string]any)["languages"].([]any)
	if len(catalogs) != 1 || catalogs[0].(map[string]any)["catalogId"] != "top" || !slices.Contains(languages, any("fr")) {
		t.Fatalf("choices: %v", choices)
	}
	if genres := catalogs[0].(map[string]any)["genres"]; len(genres.([]any)) != 2 {
		t.Errorf("genres: %v", genres)
	}

	list := map[string]any{
		"name": "Before 2000", "shownAs": "library", "sort": "name", "maxItems": 10,
		"catalogs": []map[string]any{{"addonId": addon["id"], "type": "movie", "id": "top"}},
		"rules":    map[string]any{"field": "group", "match": "all", "rules": []map[string]any{{"field": "year", "max": 1999}}},
	}
	for _, c := range []struct {
		change func(map[string]any)
		code   string
	}{
		{func(l map[string]any) { l["name"] = " " }, "invalid_smart_list_name"},
		{func(l map[string]any) { l["maxItems"] = 1.5 }, "invalid_smart_list_max_items"},
		{func(l map[string]any) { l["sort"] = "size" }, "invalid_smart_list_sort"},
		{func(l map[string]any) { l["catalogs"] = []any{} }, "invalid_smart_list_catalogs"},
		{func(l map[string]any) {
			l["rules"] = map[string]any{"field": "group", "match": "all", "rules": []map[string]any{{"field": "language", "value": "elvish"}}}
		}, "invalid_smart_list_rules"},
	} {
		invalid := map[string]any{}
		for key, value := range list {
			invalid[key] = value
		}
		c.change(invalid)
		if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/smart-lists", invalid); status != http.StatusBadRequest || body["error"] != c.code {
			t.Errorf("%s: %d %v", c.code, status, body)
		}
	}

	// A preview lists the titles as they stand, before the list is saved.
	status, preview, _ := administrator.call(http.MethodPost, "/scopes/shared/smart-lists/preview", list)
	titles, _ := preview["titles"].([]any)
	if status != http.StatusOK || len(titles) != 1 || titles[0].(map[string]any)["name"] != "Comedy" || preview["listed"] != 1.0 ||
		preview["unknown"] != 0.0 || preview["complete"] != true {
		t.Fatalf("preview: %d %v", status, preview)
	}
	if title := titles[0].(map[string]any); title["year"] != 1998.0 || title["rating"] != 7.1 || title["type"] != "movie" {
		t.Errorf("previewed title: %v", title)
	}

	status, saved, _ := administrator.call(http.MethodPost, "/scopes/shared/smart-lists", list)
	if status != http.StatusCreated || saved["name"] != "Before 2000" || saved["maxItems"] != 10.0 || saved["itemId"] == "" {
		t.Fatalf("created: %d %v", status, saved)
	}
	id := saved["id"].(string)
	list["name"] = "Nineties"
	if status, body, _ := administrator.call(http.MethodPut, "/scopes/shared/smart-lists/"+id, list); status != http.StatusOK || body["name"] != "Nineties" {
		t.Errorf("updated: %d %v", status, body)
	}
	// The server's lists shown as libraries are among the libraries a
	// user may be shown.
	if _, body, _ := administrator.call(http.MethodGet, "/user-content-choices", nil); !strings.Contains(stringOf(body["libraries"]), saved["itemId"].(string)) {
		t.Errorf("content choices: %v", body["libraries"])
	}

	// A member reaches only their own lists.
	if status, _ := member.read("/scopes/shared/smart-lists"); status != http.StatusForbidden {
		t.Errorf("a member reading the server's lists: %d", status)
	}
	if status, own := member.read("/scopes/me/smart-lists"); status != http.StatusOK || len(own.([]any)) != 0 {
		t.Errorf("the member's own lists: %d %v", status, own)
	}
	if status, body, _ := member.call(http.MethodGet, "/scopes/me/smart-lists/"+id, nil); status != http.StatusNotFound || body["error"] != "not_found" {
		t.Errorf("a member reading the server's list as their own: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodDelete, "/scopes/me/smart-lists/"+id, nil); status != http.StatusNotFound {
		t.Errorf("a member deleting the server's list: %d %v", status, body)
	}

	if status, _, _ := administrator.call(http.MethodDelete, "/scopes/shared/smart-lists/"+id, nil); status != http.StatusNoContent {
		t.Errorf("deleting: %d", status)
	}
	if status, left := administrator.read("/scopes/shared/smart-lists"); status != http.StatusOK || len(left.([]any)) != 0 {
		t.Errorf("listing after deleting: %d %v", status, left)
	}
}

func stringOf(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

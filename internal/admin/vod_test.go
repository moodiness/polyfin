package admin

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

// The VOD options are read and checked; the source counts its movies and
// series, previews their categories, and its catalogs are libraries.
func TestIPTVVODOptions(t *testing.T) {
	api := newTestAPI(t, 10)
	provider := newProvider(t)
	administrator := api.signedIn("administrator", true)
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Account", "kind": "xtream",
		"server": provider, "username": "user", "password": "secret-pw", "options": map[string]any{"movies": true, "vodExcluded": []string{"movie:Kids"}}})
	if status != http.StatusCreated {
		t.Fatalf("adding with movies: %d %v", status, body)
	}
	source := body["source"].(map[string]any)
	options, vod := source["options"].(map[string]any), source["vod"].(map[string]any)
	if options["liveTv"] != true || options["movies"] != true || options["series"] != false || options["vodLibraries"] != "type" ||
		options["enrichment"] != true || fmt.Sprint(options["vodExcluded"]) != "[movie:Kids]" {
		t.Errorf("options: %v", options)
	}
	if vod["movies"] != 2.0 || vod["shownMovies"] != 1.0 || vod["series"] != 0.0 || vod["movieCategories"] != 2.0 {
		t.Errorf("vod: %v", vod)
	}
	id := body["id"].(string)
	var rows []string
	for _, l := range administrator.libraries("shared") {
		if l["addonId"] == id && l["enabled"] == true {
			rows = append(rows, fmt.Sprint(l["catalogType"], "/", l["catalogId"], " ", l["guides"] != nil))
		}
	}
	if !slices.Equal(rows, []string{"tv/channels true", "movie/movies false"}) {
		t.Errorf("libraries: %q", rows)
	}
	status, preview, _ := administrator.call(http.MethodGet, "/scopes/shared/iptv/"+id+"/preview?by=movie", nil)
	categories, _ := preview["categories"].([]any)
	if status != http.StatusOK || preview["total"] != 2.0 || len(categories) != 2 || categories[1].(map[string]any)["excluded"] != true {
		t.Errorf("movie preview: %d %v", status, preview)
	}
	status, preview, _ = administrator.call(http.MethodPost, "/scopes/shared/iptv/preview", map[string]any{"kind": "xtream", "server": provider,
		"username": "user", "password": "secret-pw", "by": "series"})
	if status != http.StatusOK || preview["total"] != 1.0 {
		t.Errorf("series preview of an account: %d %v", status, preview)
	}
	for _, bad := range []map[string]any{
		{"liveTv": false, "movies": false, "series": false},
		{"vodLibraries": "shelf"},
		{"vodExcluded": []string{"g:News"}},
	} {
		if status, body, _ := administrator.call(http.MethodPatch, "/scopes/shared/iptv/"+id, map[string]any{"options": bad}); status != http.StatusBadRequest ||
			body["error"] != "invalid_options" {
			t.Errorf("%v: %d %v", bad, status, body)
		}
	}
	status, body, _ = administrator.call(http.MethodPatch, "/scopes/shared/iptv/"+id, map[string]any{"options": map[string]any{"series": true,
		"vodLibraries": "category"}})
	if status != http.StatusOK || body["source"].(map[string]any)["vod"].(map[string]any)["series"] != 1.0 {
		t.Errorf("series by category: %d %v", status, body)
	}
	rows = nil
	for _, l := range administrator.libraries("shared") {
		if l["addonId"] == id {
			rows = append(rows, fmt.Sprint(l["catalogType"], " ", l["catalogName"], " ", l["enabled"], " ", l["browsable"]))
		}
	}
	if !slices.Equal(rows, []string{"tv Account true true", "movie Action true true", "series Drama true true", "movie Account false false",
		"series Account false false"}) {
		t.Errorf("libraries by category: %q", rows)
	}
	_, sources, _ := administrator.call(http.MethodGet, "/sources", nil)
	listed := fmt.Sprint(sources["addons"])
	if !slices.ContainsFunc(sources["addons"].([]any), func(a any) bool {
		s, _ := a.(map[string]any)["source"].(map[string]any)
		return s != nil && s["vod"] != nil && s["options"].(map[string]any)["series"] == true
	}) {
		t.Errorf("sources: %s", listed)
	}
}

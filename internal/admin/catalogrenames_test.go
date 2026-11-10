package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
)

// Refresh, Refresh everything and a new address tell which libraries
// followed their catalog's new identifier and which were removed with
// their catalog; other changes of an addon tell neither.
func TestRefreshTellsTheLibrariesThatFollowedAndThoseRemoved(t *testing.T) {
	t.Parallel()
	var manifest atomic.Pointer[string]
	set := func(catalogs string) {
		body := `{"id": "renames", "name": "Renames", "version": "1.0.0", "resources": ["catalog"], "types": ["movie"], "catalogs": [` + catalogs + `]}`
		manifest.Store(&body)
	}
	set(`{"type": "movie", "id": "top", "name": "Top"}, {"type": "movie", "id": "pop", "name": "Popular"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(*manifest.Load())) }))
	t.Cleanup(server.Close)
	var store *addons.Store
	api := newTestAPI(t, 10, func(_ *Options, deps testDeps) { store = deps.addons })
	administrator := api.signedIn("administrator", true)
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, body)
	}
	if _, told := body["libraries"]; told {
		t.Errorf("installing tells libraries: %v", body)
	}
	id := body["id"].(string)
	libraries := func(body map[string]any) string {
		t.Helper()
		encoded, err := json.Marshal(body["libraries"])
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}

	set(`{"type": "movie", "id": "top2", "name": "Top"}`)
	status, body, _ = administrator.call(http.MethodPost, "/scopes/shared/addons/"+id+"/refresh", nil)
	want := `{"followed":[{"catalogId":"top","catalogType":"movie","name":"Top","newId":"top2"}],` +
		`"removed":[{"catalogId":"pop","catalogType":"movie","name":"Popular"}]}`
	if status != http.StatusOK || libraries(body) != want {
		t.Errorf("refresh: %d %s", status, libraries(body))
	}

	set(`{"type": "movie", "id": "top3", "name": "Top"}`)
	status, body, _ = administrator.call(http.MethodPost, "/scopes/shared/addons/"+id+"/refresh-all", nil)
	if want := `{"followed":[{"catalogId":"top2","catalogType":"movie","name":"Top","newId":"top3"}],"removed":[]}`; status != http.StatusOK ||
		libraries(body) != want {
		t.Errorf("refresh everything: %d %s", status, libraries(body))
	}

	status, body, _ = administrator.call(http.MethodPatch, "/scopes/shared/addons/"+id, map[string]any{"manifestUrl": server.URL + "/other/manifest.json"})
	if want := `{"followed":[],"removed":[]}`; status != http.StatusOK || libraries(body) != want {
		t.Errorf("a new address: %d %s", status, libraries(body))
	}
	status, body, _ = administrator.call(http.MethodPatch, "/scopes/shared/addons/"+id, map[string]any{"enabled": true})
	if _, told := body["libraries"]; status != http.StatusOK || told {
		t.Errorf("turned on: %d %v", status, body)
	}

	// The library is the only one left, under its new identifier.
	list, err := store.Libraries(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	var enabled []string
	for _, l := range list {
		if l.Enabled {
			enabled = append(enabled, l.Catalog.ID)
		}
	}
	if !slices.Equal(enabled, []string{"top3"}) {
		t.Errorf("enabled libraries: %v", enabled)
	}
}

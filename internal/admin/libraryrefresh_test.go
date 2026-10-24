package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// An administrator has a movie library's descriptions read again from its
// row under Content › Libraries: the libraries tell which can be, and how
// the refresh goes. A live TV catalog's cannot, a member cannot ask, and a
// library without a refresh has none to stop.
func TestAdministratorsRefreshALibrarysDescriptions(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	overview := "Before."
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch path := r.URL.Path; {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "reels", Name: "Reels", Version: "1", Types: []string{"movie", "tv"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "tv", ID: "channels", Name: "Channels"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "tt0001", Type: "movie", Name: "First"}}})
		case path == "/meta/movie/tt0001.json":
			asked = append(asked, path)
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: "tt0001", Type: "movie", Name: "First", Description: overview}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	var lib *library.Service
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) { lib = o.LibraryRefreshes.(*library.Service) })
	admin := api.signedIn("root", true)
	member := api.signedIn("member", false)
	status, installed, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"})
	if status != http.StatusCreated {
		t.Fatalf("install: %d %v", status, installed)
	}
	addonID := installed["id"].(string)
	find := func(catalogType string) map[string]any {
		t.Helper()
		libraries := admin.libraries("shared")
		i := slices.IndexFunc(libraries, func(l map[string]any) bool { return l["catalogType"] == catalogType })
		if i < 0 {
			t.Fatalf("no %s library in %v", catalogType, libraries)
		}
		return libraries[i]
	}
	movies := find("movie")
	if movies["refreshable"] != true || movies["refresh"] != nil || find("tv")["refreshable"] != false {
		t.Fatalf("before: movies %v %v, live TV %v", movies["refreshable"], movies["refresh"], find("tv")["refreshable"])
	}
	// An app read the library's first page.
	root, err := api.store.Authenticate(t.Context(), "root", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := accounts.ParseID(movies["itemId"].(string))
	if page, err := lib.Children(t.Context(), root, id, 0, 10, ""); err != nil || len(page.Items) != 1 {
		t.Fatalf("the first page: %v %v", page.Items, err)
	}
	mu.Lock()
	overview, asked = "After.", nil
	mu.Unlock()

	body := map[string]string{"addonId": addonID, "catalogType": "movie", "catalogId": "top"}
	if status, _, _ := member.call(http.MethodPost, "/scopes/shared/libraries/refresh", body); status != http.StatusForbidden {
		t.Errorf("a member refreshing: %d", status)
	}
	if status, _, _ := admin.call(http.MethodPost, "/scopes/shared/libraries/refresh",
		map[string]string{"addonId": addonID, "catalogType": "tv", "catalogId": "channels"}); status != http.StatusBadRequest {
		t.Errorf("refreshing live TV: %d", status)
	}
	if status, state, _ := admin.call(http.MethodPost, "/scopes/shared/libraries/refresh", body); status != http.StatusAccepted ||
		state["state"] != library.RefreshQueued && state["state"] != library.RefreshRunning && state["state"] != library.RefreshDone {
		t.Fatalf("refreshing: %d %v", status, state)
	}
	var refresh map[string]any
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		refresh, _ = find("movie")["refresh"].(map[string]any)
		if refresh != nil && refresh["state"] == library.RefreshDone || time.Now().After(deadline) {
			break
		}
	}
	if refresh["state"] != library.RefreshDone || refresh["titles"] != 1.0 || refresh["done"] != 1.0 || refresh["failed"] != 0.0 ||
		refresh["startedAt"] == nil || refresh["endedAt"] == nil {
		t.Errorf("the refresh: %v", refresh)
	}
	mu.Lock()
	if len(asked) != 1 {
		t.Errorf("descriptions asked: %v", asked)
	}
	mu.Unlock()
	if item, err := lib.Item(t.Context(), root, firstTitle(t, lib, root, id)); err != nil || item.Overview != "After." {
		t.Errorf("the title after: %q %v", item.Overview, err)
	}
	if status, _, _ := admin.call(http.MethodPost, "/scopes/shared/libraries/refresh/stop", body); status != http.StatusNotFound {
		t.Errorf("stopping an ended refresh: %d", status)
	}
}

func firstTitle(t *testing.T, lib *library.Service, user accounts.User, library accounts.ID) accounts.ID {
	t.Helper()
	page, err := lib.Children(t.Context(), user, library, 0, 1, "")
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("the library: %v %v", page.Items, err)
	}
	return page.Items[0].ID
}

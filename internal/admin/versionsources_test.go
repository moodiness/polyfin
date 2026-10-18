package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Order of sources lists the server's sources that list versions of
// movies and episodes, folders included, by ID; the settings answer them
// with their names, in the order of Content › Sources. A PUT saves the
// order, one leaving it out keeps it, and one holding a source the server
// does not have, or one twice, is refused. A source installed later is
// offered at the end without being placed, and a removed one leaves the
// order.
func TestSettingsVersionSourceOrder(t *testing.T) {
	t.Parallel()
	// Each addon serves its manifest under its own path: the relays list
	// streams of movies and series, the catalog lists none.
	names := map[string]string{"one": "Relay One", "two": "Relay Two", "three": "Relay Three", "catalog": "Catalog", "own": "Own Relay"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/manifest.json")
		if !ok || names[key] == "" {
			http.NotFound(w, r)
			return
		}
		resources := []stremio.Resource{{Name: "stream"}}
		if key == "catalog" {
			resources = []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}
		}
		_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "relay." + key, Name: names[key], Version: "1", Types: []string{"movie", "series"},
			IDPrefixes: []string{"tt"}, Resources: resources, Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
	}))
	t.Cleanup(server.Close)
	var folders *localfiles.Service
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		folders = localfiles.New(deps.pool, deps.addons, library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings), logger,
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Folders, store = folders, deps.addons
	})
	administrator := api.signedIn("administrator", true)
	install := func(scope addons.Scope, key string) addons.Addon {
		t.Helper()
		addon, err := store.Install(t.Context(), scope, server.URL+"/"+key+"/manifest.json", false)
		if err != nil {
			t.Fatal(err)
		}
		return addon
	}
	one, two, catalog := install(addons.Shared(), "one"), install(addons.Shared(), "two"), install(addons.Shared(), "catalog")
	shelf, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: t.TempDir(), Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	records, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Records", Path: t.TempDir(), Kind: localfiles.KindMusic})
	if err != nil {
		t.Fatal(err)
	}
	users, err := api.store.Users(t.Context())
	if err != nil || len(users) != 1 {
		t.Fatalf("users: %v %v", users, err)
	}
	own := install(addons.Personal(users[0].ID), "own")

	order := func(body map[string]any) string {
		var sources []string
		for _, entry := range body["versionSources"].([]any) {
			source := entry.(map[string]any)
			sources = append(sources, source["name"].(string)+"="+source["id"].(string))
		}
		return fmt.Sprint(body["versionSourceOrder"], sources)
	}
	sources := func(placed []addons.Addon, offered ...addons.Addon) string {
		var ids, entries []string
		for _, addon := range placed {
			ids = append(ids, addon.ID.String())
		}
		for _, addon := range offered {
			entries = append(entries, addon.Manifest.Name+"="+addon.ID.String())
		}
		return fmt.Sprint(ids, entries)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); order(body) != sources(nil, one, two, shelf) {
		t.Errorf("default settings: %s", order(body))
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	settings := maps.Clone(base)
	maps.Copy(settings, map[string]any{"versionOrder": []string{"source", "language"},
		"versionSourceOrder": []string{shelf.ID.String(), two.ID.String()}})
	want := sources([]addons.Addon{shelf, two}, one, two, shelf)
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || order(body) != want ||
		fmt.Sprint(body["versionOrder"]) != "[source language]" {
		t.Fatalf("saving the order: %d %s %v", status, order(body), body["versionOrder"])
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || order(body) != want {
		t.Errorf("saving without it: %d %s", status, order(body))
	}
	for name, value := range map[string]any{
		"an unknown source":             []string{accounts.ID{0x77}.String()},
		"a source listing no streams":   []string{catalog.ID.String()},
		"a music folder":                []string{records.ID.String()},
		"a user's own addon":            []string{own.ID.String()},
		"something else than a source":  []string{"relay"},
		"a source of another JSON type": []int{1},
	} {
		refused := maps.Clone(base)
		refused["versionSourceOrder"] = value
		status, body, _ := administrator.call(http.MethodPut, "/settings", refused)
		if name == "a source of another JSON type" {
			if status != http.StatusBadRequest {
				t.Errorf("%s: %d %v", name, status, body)
			}
			continue
		}
		if status != http.StatusBadRequest || body["error"] != "invalid_version_source_order" {
			t.Errorf("%s: %d %v", name, status, body)
		}
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); order(body) != want {
		t.Errorf("a refused order changed the settings: %s", order(body))
	}

	// A source installed later is offered at the end, not placed.
	three := install(addons.Shared(), "three")
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); order(body) != sources([]addons.Addon{shelf, two}, one, two, shelf, three) {
		t.Errorf("a source installed later: %s", order(body))
	}
	// A source removed leaves the order, when read and when saved.
	if err := store.Remove(t.Context(), addons.Shared(), two.ID); err != nil {
		t.Fatal(err)
	}
	want = sources([]addons.Addon{shelf}, one, shelf, three)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); order(body) != want {
		t.Errorf("a source removed: %s", order(body))
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || order(body) != want {
		t.Errorf("saving after the removal: %d %s", status, order(body))
	}
	if got := api.store.Settings().VersionSourceOrder; len(got) != 1 || got[0] != shelf.ID {
		t.Errorf("stored after the removal: %v", got)
	}
}

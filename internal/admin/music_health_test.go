package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/stremio"
)

// TestMusicAddonsHaveTheirHealth checks that the requests to an Eclipse
// addon count in its health, listed and checked like a Stremio addon's.
func TestMusicAddonsHaveTheirHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "com.example.tones", "name": "Tones", "version": "1.0.0", "resources": []string{"search", "stream"}})
		case "/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"tracks": []any{}})
		default:
			http.Error(w, "down", http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(server.Close)
	var client *stremio.Client
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		client = deps.client
		o.Health = HealthSources{Addons: deps.client, DatabaseSize: func(ctx context.Context) (int64, error) { return database.Size(ctx, deps.pool) }}
	})
	admin := api.signedIn("root", true)
	status, installed, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"})
	if status != http.StatusCreated || installed["kind"] != "eclipse" {
		t.Fatalf("install: %d %v", status, installed)
	}
	addon := eclipse.Addon{ManifestURL: server.URL + "/manifest.json", Manifest: eclipse.Manifest{Resources: []string{"search"}}}
	if _, err := eclipse.NewClient(client).Search(t.Context(), addon, "tone"); err != nil {
		t.Fatal(err)
	}
	if _, err := eclipse.NewClient(client).Stream(t.Context(), addon, "t1"); err == nil {
		t.Fatal("a failing stream answered")
	}
	var health healthJSON
	if status := admin.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK {
		t.Fatalf("health: %d", status)
	}
	if len(health.Addons) != 1 || health.Addons[0].ID != installed["id"] || health.Addons[0].Requests != 3 || health.Addons[0].Failures != 1 ||
		health.Addons[0].Failure != "http_5xx" && health.Addons[0].Failure == "" {
		t.Fatalf("addons: %+v", health.Addons)
	}
	var checked addonHealthJSON
	if status := admin.raw(http.MethodPost, "/health/addons/"+health.Addons[0].ID+"/check", "", &checked); status != http.StatusOK ||
		checked.Requests != 4 || checked.Failure != "" {
		t.Errorf("check: %d %+v", status, checked)
	}
}

package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Administrators see on the dashboard the addons, IPTV sources and guides
// users keep for themselves, with their owner, after the server's.
func TestDashboardShowsUsersOwnAddonsAndGuides(t *testing.T) {
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": "own-tv", "name": "Own TV", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "tv", "id": "channels", "name": "Channels"}]}`)
	}))
	t.Cleanup(addon.Close)
	var store *addons.Store
	var client *stremio.Client
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		store, client = deps.addons, deps.client
		o.Health = HealthSources{Addons: deps.client}
	})
	admin := api.signedIn("root", true)
	member := api.signedIn("sam", false)
	if status, body, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)}); status != http.StatusCreated {
		t.Fatalf("install: %d %v", status, body)
	}
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var samID accounts.ID
	for _, u := range users {
		if u.Name == "sam" {
			samID = u.ID
		}
	}
	sam := samID.String()
	scope := addons.Personal(samID)
	own, err := store.Install(t.Context(), scope, addon.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	key := addons.LibraryKey{AddonID: own.ID, CatalogType: "tv", CatalogID: "channels"}
	if _, err := store.SetLibraries(t.Context(), scope, []addons.LibraryChoice{{AddonID: own.ID, CatalogType: "tv", CatalogID: "channels"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetGuide(t.Context(), scope, key, "https://guides.example/sam/secret.xml"); err != nil {
		t.Fatal(err)
	}
	// Installing it read its manifest; a request made for Sam's apps is
	// recorded like any other.
	_, _ = client.Catalog(t.Context(), addon.URL+"/manifest.json", "tv", "channels", nil, false)

	var sources struct {
		Addons []struct {
			ID    string     `json:"id"`
			Name  string     `json:"name"`
			Owner *ownerJSON `json:"owner"`
		} `json:"addons"`
		Guides []struct {
			AddonID string     `json:"addonId"`
			Owner   *ownerJSON `json:"owner"`
			Guide   *guideJSON `json:"guide"`
		} `json:"guides"`
	}
	if status := admin.raw(http.MethodGet, "/sources", "", &sources); status != http.StatusOK {
		t.Fatalf("sources: %d", status)
	}
	if len(sources.Addons) != 2 || sources.Addons[0].Owner != nil || sources.Addons[1].ID != own.ID.String() ||
		sources.Addons[1].Owner == nil || sources.Addons[1].Owner.Name != "sam" || sources.Addons[1].Owner.ID != sam {
		t.Errorf("addons: %+v", sources.Addons)
	}
	if len(sources.Guides) != 1 || sources.Guides[0].AddonID != own.ID.String() || sources.Guides[0].Owner == nil ||
		sources.Guides[0].Owner.Name != "sam" || sources.Guides[0].Guide == nil || sources.Guides[0].Guide.URL != "https://guides.example/…" {
		t.Errorf("guides: %+v", sources.Guides)
	}

	var health healthJSON
	admin.raw(http.MethodGet, "/health", "", &health)
	if len(health.Addons) != 2 || health.Addons[0].Owner != nil || health.Addons[1].Owner == nil ||
		health.Addons[1].Owner.Name != "sam" || health.Addons[1].Requests != 2 || health.Addons[1].LastSuccessAt == nil {
		t.Fatalf("health: %+v", health.Addons)
	}
	// Checking Sam's addon follows Sam's rules: a member's addon may not
	// reach the local network, where this one is.
	var checked addonHealthJSON
	if status := admin.raw(http.MethodPost, "/health/addons/"+own.ID.String()+"/check", "", &checked); status != http.StatusOK ||
		checked.Owner == nil || checked.Failure != stremio.FailurePrivateNetwork || checked.Requests != 3 {
		t.Errorf("check: %d %+v", status, checked)
	}
	if status := admin.raw(http.MethodPost, "/health/addons/"+own.ID.String()+"/check", "", nil); status != http.StatusTooManyRequests {
		t.Errorf("second check: %d", status)
	}

	for _, path := range []string{"/sources", "/health"} {
		if status := member.raw(http.MethodGet, path, "", nil); status != http.StatusForbidden {
			t.Errorf("member %s: %d", path, status)
		}
	}
}

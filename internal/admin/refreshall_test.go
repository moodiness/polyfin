package admin

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// Refresh everything downloads an addon's manifest again and makes its
// kept pages old, within the scope checks of Refresh: the server's
// addons for administrators, a user's own while they may have some.
func TestRefreshEverythingKeepsToTheScopesOfRefresh(t *testing.T) {
	t.Parallel()
	var manifests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		manifests.Add(1)
		_, _ = w.Write([]byte(`{"id": "top", "name": "Top", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}]}`))
	}))
	t.Cleanup(server.Close)
	var store *addons.Store
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(_ *Options, deps testDeps) { store, pool = deps.addons, deps.pool })
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var memberUser accounts.User
	for _, u := range users {
		if u.Name == "member" {
			memberUser = u
		}
	}
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, body)
	}
	shared := body["id"].(string)
	// The member's own addon, installed as an administrator would reach it.
	own, err := store.Install(t.Context(), addons.Personal(memberUser.ID), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	mine := own.ID.String()
	if _, err := pool.Exec(t.Context(), `INSERT INTO catalog_pages (addon_id, catalog_type, catalog_id, genre, skip, config, metas, fetched_at)
		VALUES ($1, 'movie', 'top', '', 0, '', '[]', now())`, shared); err != nil {
		t.Fatal(err)
	}
	refreshAll := func(b browser, scope, id string) (int, map[string]any) {
		t.Helper()
		status, body, _ := b.call(http.MethodPost, "/scopes/"+scope+"/addons/"+id+"/refresh-all", nil)
		return status, body
	}

	// The server's addon: a member may not, an administrator may.
	before := manifests.Load()
	if status, body := refreshAll(member, "shared", shared); status != http.StatusForbidden || body["error"] != "forbidden" {
		t.Errorf("a member on the server's addon: %d %v", status, body)
	}
	if got := manifests.Load(); got != before {
		t.Errorf("a refused refresh downloaded %d manifests", got-before)
	}
	if status, body := refreshAll(administrator, "shared", shared); status != http.StatusOK || body["id"] != shared {
		t.Fatalf("an administrator on the server's addon: %d %v", status, body)
	}
	if got := manifests.Load(); got != before+1 {
		t.Errorf("Refresh everything downloaded %d manifests", got-before)
	}
	// Its page kept is past the catalog refresh age.
	var seconds int
	if err := pool.QueryRow(t.Context(), "SELECT EXTRACT(EPOCH FROM now() - fetched_at)::int FROM catalog_pages WHERE addon_id = $1", shared).
		Scan(&seconds); err != nil {
		t.Fatal(err)
	}
	if age := time.Duration(seconds) * time.Second; age < time.Hour {
		t.Errorf("the addon's page kept is %v old", age)
	}
	// Another scope's addon is not found there.
	if status, body := refreshAll(administrator, "me", shared); status != http.StatusNotFound {
		t.Errorf("the server's addon among the administrator's own: %d %v", status, body)
	}
	if status, body := refreshAll(administrator, "me", mine); status != http.StatusNotFound {
		t.Errorf("another user's addon: %d %v", status, body)
	}

	// The member's own addon, while they may have some, is downloaded
	// again as the member reaches it: not on the local network, which this
	// test's address is on.
	if status, body := refreshAll(member, "me", mine); status != http.StatusForbidden || body["error"] != "private_network" {
		t.Errorf("the member's own addon: %d %v", status, body)
	}
	if _, err := api.store.UpdateUser(t.Context(), memberUser.ID, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(13)}}, nil); err != nil {
		t.Fatal(err)
	}
	if status, body := refreshAll(member, "me", mine); status != http.StatusForbidden || body["error"] != "parental_control_own_sources" {
		t.Errorf("under parental control: %d %v", status, body)
	}
	if _, err := api.store.UpdateUser(t.Context(), memberUser.ID, accounts.UserChanges{Parental: &accounts.ParentalControl{}, PersonalAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, body := refreshAll(member, "me", mine); status != http.StatusForbidden || body["error"] != "personal_addons_disabled" {
		t.Errorf("without their own addons: %d %v", status, body)
	}
}

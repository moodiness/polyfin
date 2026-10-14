package admin

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/jellyfin"
)

// fixedChoices offers one collection for home rows.
type fixedChoices struct{}

func (fixedChoices) HomeRowCollections(context.Context, accounts.User) ([]jellyfin.HomeRowCollection, error) {
	id, _ := accounts.ParseID("0123456789abcdef0123456789abcdef")
	return []jellyfin.HomeRowCollection{{ID: id, Name: "Picks", Source: "users"}}, nil
}

// rowNames lists the names of the rows of an answer.
func rowNames(answer map[string]any) []string {
	var result []string
	rows, _ := answer["rows"].([]any)
	for _, row := range rows {
		result = append(result, row.(map[string]any)["name"].(string))
	}
	return result
}

// Administrators add, change, order and delete the web player's home rows,
// choosing among the catalogs of the server's addons and the collections;
// rows that cannot work are refused. Members may not.
func TestHomeRowRoutes(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		o.HomeRows = homerows.New(d.pool)
		o.HomeRowChoices = fixedChoices{}
	})
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	if status, _, _ := member.call(http.MethodGet, "/home-rows", nil); status != http.StatusForbidden {
		t.Errorf("a member's list: %d", status)
	}
	status, addon, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": listAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, addon)
	}
	status, choices, _ := administrator.call(http.MethodGet, "/home-rows/choices", nil)
	catalogs, _ := choices["catalogs"].([]any)
	collections, _ := choices["collections"].([]any)
	if status != http.StatusOK || len(catalogs) != 1 || catalogs[0].(map[string]any)["catalogId"] != "top" || len(collections) != 1 ||
		collections[0].(map[string]any)["name"] != "Picks" {
		t.Fatalf("choices: %d %v", status, choices)
	}

	catalog := map[string]any{"name": "Top movies", "kind": "catalog", "place": "top", "addonId": addon["id"], "catalogType": "movie",
		"catalogId": "top", "everyone": true}
	for _, refused := range []map[string]any{
		{"name": "", "kind": "new", "place": "top", "everyone": true},
		{"name": "Rows", "kind": "trending", "place": "top", "everyone": true},
		{"name": "Rows", "kind": "new", "place": "middle", "everyone": true},
		{"name": "Rows", "kind": "catalog", "place": "top", "everyone": true},
		{"name": "Rows", "kind": "new", "place": "top", "addonId": addon["id"], "catalogType": "movie", "catalogId": "top", "everyone": true},
		{"name": "Rows", "kind": "collection", "place": "top", "collectionId": "not an identifier", "everyone": true},
		{"name": "Rows", "kind": "because", "place": "top", "count": 11, "everyone": true},
		// Catalogs the choices do not offer, which no row could draw: one
		// that cannot be browsed, one the addon does not have, and one of
		// no addon installed.
		{"name": "Rows", "kind": "catalog", "place": "top", "addonId": addon["id"], "catalogType": "movie", "catalogId": "search", "everyone": true},
		{"name": "Rows", "kind": "catalog", "place": "top", "addonId": addon["id"], "catalogType": "collection", "catalogId": "sets", "everyone": true},
		{"name": "Rows", "kind": "catalog", "place": "top", "addonId": "0123456789abcdef0123456789abcdef", "catalogType": "movie", "catalogId": "top",
			"everyone": true},
	} {
		if status, body, _ := administrator.call(http.MethodPost, "/home-rows", refused); status != http.StatusBadRequest || body["error"] != "invalid_home_row" {
			t.Errorf("%v: %d %v", refused, status, body)
		}
	}
	status, created, _ := administrator.call(http.MethodPost, "/home-rows", catalog)
	if status != http.StatusCreated || created["kind"] != "catalog" || created["addonId"] != addon["id"] || created["library"] != false || created["count"] != 3.0 {
		t.Fatalf("created: %d %v", status, created)
	}
	_, because, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "Because you watched", "kind": "because",
		"place": "resume", "count": 2, "everyone": true})
	var me map[string]any
	_, me, _ = member.call(http.MethodGet, "/session", nil)
	memberID := me["user"].(map[string]any)["id"].(string)
	_, chosen, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "For the member", "kind": "collection",
		"place": "nextup", "collectionId": "0123456789abcdef0123456789abcdef", "everyone": false, "users": []string{memberID}, "library": true})
	if users, _ := chosen["users"].([]any); len(users) != 1 || users[0] != memberID || chosen["library"] != true {
		t.Fatalf("a row for the member: %v", chosen)
	}
	_, list, _ := administrator.call(http.MethodGet, "/home-rows", nil)
	if got := rowNames(list); !slices.Equal(got, []string{"Top movies", "Because you watched", "For the member"}) {
		t.Errorf("rows: %v", got)
	}
	status, list, _ = administrator.call(http.MethodPut, "/home-rows/order", map[string]any{"ids": []any{chosen["id"], created["id"]}})
	if got := rowNames(list); status != http.StatusOK || !slices.Equal(got, []string{"For the member", "Top movies", "Because you watched"}) {
		t.Errorf("ordered: %d %v", status, got)
	}
	catalog["place"], catalog["name"] = "bottom", "Top of the week"
	if status, body, _ := administrator.call(http.MethodPatch, "/home-rows/"+created["id"].(string), catalog); status != http.StatusOK ||
		body["place"] != "bottom" || body["name"] != "Top of the week" {
		t.Errorf("changed: %d %v", status, body)
	}
	unbrowsable := map[string]any{"name": "Search", "kind": "catalog", "place": "top", "addonId": addon["id"], "catalogType": "movie",
		"catalogId": "search", "everyone": true}
	if status, body, _ := administrator.call(http.MethodPatch, "/home-rows/"+created["id"].(string), unbrowsable); status != http.StatusBadRequest ||
		body["error"] != "invalid_home_row" {
		t.Errorf("changed to a catalog not offered: %d %v", status, body)
	}

	// The member sees the rows shown to them, and turns one off.
	status, own, _ := member.call(http.MethodGet, "/account/home-rows", nil)
	if got := rowNames(own); status != http.StatusOK || !slices.Equal(got, []string{"For the member", "Top of the week", "Because you watched"}) {
		t.Fatalf("the member's rows: %d %v", status, own)
	}
	if status, _, _ := member.call(http.MethodPut, "/account/home-rows/"+because["id"].(string), map[string]bool{"off": true}); status != http.StatusNoContent {
		t.Errorf("turning off: %d", status)
	}
	_, own, _ = member.call(http.MethodGet, "/account/home-rows", nil)
	rows, _ := own["rows"].([]any)
	if len(rows) != 3 || rows[2].(map[string]any)["off"] != true || rows[0].(map[string]any)["off"] != false {
		t.Errorf("after turning off: %v", own)
	}
	// The administrator does not see the row for the member, and cannot
	// turn it off.
	_, own, _ = administrator.call(http.MethodGet, "/account/home-rows", nil)
	if got := rowNames(own); !slices.Equal(got, []string{"Top of the week", "Because you watched"}) {
		t.Errorf("the administrator's rows: %v", got)
	}
	if status, _, _ := administrator.call(http.MethodPut, "/account/home-rows/"+chosen["id"].(string), map[string]bool{"off": true}); status != http.StatusNotFound {
		t.Errorf("turning off a row not shown: %d", status)
	}

	if status, _, _ := administrator.call(http.MethodDelete, "/home-rows/"+because["id"].(string), nil); status != http.StatusNoContent {
		t.Errorf("deleting: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodDelete, "/home-rows/"+because["id"].(string), nil); status != http.StatusNotFound {
		t.Errorf("deleting again: %d", status)
	}
	for range homerows.MaxRows - 2 {
		if status, body, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "New", "kind": "new", "place": "top",
			"everyone": true}); status != http.StatusCreated {
			t.Fatalf("adding: %d %v", status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "New", "kind": "new", "place": "top",
		"everyone": true}); status != http.StatusConflict || body["error"] != "too_many_home_rows" {
		t.Errorf("one row too many: %d %v", status, body)
	}
}

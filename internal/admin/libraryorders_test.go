package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/preferences"
)

// storedLibraryOrders counts and clears the orders of libraries in the
// preferences store, as the Jellyfin handler does, which also forgets what
// it remembers of them.
type storedLibraryOrders struct{ *preferences.Store }

func (s storedLibraryOrders) ClearLibraryOrders(ctx context.Context) (int, error) {
	users, err := s.Store.ClearLibraryOrders(ctx)
	return len(users), err
}

// Apply this order to every user, under Content › Libraries, says first how
// many users keep an order of libraries of their own, then clears those
// orders and only them, recording who did it. Members may do neither.
func TestApplyingTheLibraryOrderToEveryUser(t *testing.T) {
	t.Parallel()
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		pool = d.pool
		o.LibraryOrders = storedLibraryOrders{preferences.New(d.pool)}
	})
	ctx := t.Context()
	prefs := preferences.New(pool)
	admin := api.signedIn("admin", true)
	member := api.signedIn("sam", false)
	saved := map[accounts.ID]string{
		api.userID("sam"):             `{"SubtitleMode":"Always","OrderedViews":["0123456789abcdef0123456789abcdef"]}`,
		mustCreateUser(t, api, "kim"): `{"SubtitleMode":"Smart","OrderedViews":[]}`,
		mustCreateUser(t, api, "lee"): `{"SubtitleMode":"None","OrderedViews":["fedcba9876543210fedcba9876543210"]}`,
	}
	for user, value := range saved {
		if err := prefs.PutConfiguration(ctx, user, json.RawMessage(value)); err != nil {
			t.Fatal(err)
		}
	}
	const path = "/scopes/shared/libraries/user-orders"
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		if status, _, _ := member.call(method, path, nil); status != http.StatusForbidden {
			t.Errorf("member %s: %d", method, status)
		}
	}
	if status, body, _ := admin.call(http.MethodGet, path, nil); status != http.StatusOK || body["users"] != 2.0 {
		t.Fatalf("users with an order of their own: %d %v", status, body)
	}

	if status, body, _ := admin.call(http.MethodDelete, path, nil); status != http.StatusOK || body["users"] != 2.0 {
		t.Fatalf("applying the order: %d %v", status, body)
	}
	for user, value := range saved {
		raw, _, err := prefs.Configuration(ctx, user)
		var before, after map[string]any
		_ = json.Unmarshal([]byte(value), &before)
		_ = json.Unmarshal(raw, &after)
		if err != nil || after["OrderedViews"] == nil || len(after["OrderedViews"].([]any)) != 0 || after["SubtitleMode"] != before["SubtitleMode"] {
			t.Errorf("configuration after: %s %v, from %s", raw, err, value)
		}
	}
	if _, body, _ := admin.call(http.MethodGet, path, nil); body["users"] != 0.0 {
		t.Errorf("users with an order of their own after: %v", body)
	}
	if names := activityNames(t, admin, "ServerConfigurationUpdated"); !slices.Contains(names,
		"admin applied the order of the libraries to every user (2 with an order of their own)") {
		t.Errorf("activity: %q", names)
	}
}

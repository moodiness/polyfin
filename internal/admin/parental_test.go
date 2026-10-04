package admin

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestAdministratorsSetParentalControl(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "child", "password": "correct horse"})
	if control := created["parentalControl"]; !reflect.DeepEqual(control, map[string]any{"maxRating": nil, "maxSubRating": nil, "blockUnrated": []any{}}) {
		t.Fatalf("new user's parental control: %v", control)
	}
	path := "/users/" + created["id"].(string)

	// The limit offered for PG-13, as the app reads it from the list.
	response, err := administrator.client.Get(api.url + "/admin/api/parental-ratings")
	if err != nil {
		t.Fatal(err)
	}
	var ratings []struct {
		Name     string
		Score    int
		SubScore *int
	}
	_ = json.NewDecoder(response.Body).Decode(&ratings)
	response.Body.Close()
	var limit map[string]any
	for _, rating := range ratings {
		if rating.Name == "PG-13" {
			limit = map[string]any{"maxRating": float64(rating.Score), "maxSubRating": float64(*rating.SubScore)}
		}
		if rating.Name == "Unrated" {
			t.Error("the ratings offered as a limit list unrated titles")
		}
	}
	if limit == nil {
		t.Fatalf("PG-13 is not offered: %+v", ratings)
	}
	control := map[string]any{"maxRating": limit["maxRating"], "maxSubRating": limit["maxSubRating"], "blockUnrated": []any{"series", "Movie"}}
	status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"parentalControl": control})
	// Kinds are named and ordered as Jellyfin's policy has them.
	want := map[string]any{"maxRating": float64(13), "maxSubRating": float64(0), "blockUnrated": []any{"Movie", "Series"}}
	if status != http.StatusOK || !reflect.DeepEqual(updated["parentalControl"], want) {
		t.Fatalf("setting the limit: %d %v", status, updated)
	}
	// Other changes keep it.
	if _, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"isHidden": true}); !reflect.DeepEqual(updated["parentalControl"], want) {
		t.Errorf("limit after another change: %v", updated["parentalControl"])
	}
	id, _ := accounts.ParseID(created["id"].(string))
	stored, err := api.store.User(t.Context(), id)
	if err != nil || stored.Parental.MaxRating == nil || *stored.Parental.MaxRating != 13 {
		t.Errorf("stored limit: %+v %v", stored.Parental, err)
	}

	if status, body, _ := administrator.call(http.MethodPatch, path, map[string]any{"parentalControl": map[string]any{"blockUnrated": []string{"Film"}}}); status != http.StatusBadRequest || body["error"] != "invalid_parental_control" {
		t.Errorf("unknown kind: %d %v", status, body)
	}
	// Lifting the limit.
	status, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"parentalControl": map[string]any{"maxRating": nil, "blockUnrated": []string{}}})
	if status != http.StatusOK || !reflect.DeepEqual(updated["parentalControl"], map[string]any{"maxRating": nil, "maxSubRating": nil, "blockUnrated": []any{}}) {
		t.Errorf("lifting the limit: %d %v", status, updated)
	}

	if status, _, _ := member.call(http.MethodPatch, path, map[string]any{"parentalControl": control}); status != http.StatusForbidden {
		t.Errorf("member setting a limit: %d", status)
	}
	if status, _, _ := member.call(http.MethodGet, "/parental-ratings", nil); status != http.StatusForbidden {
		t.Errorf("member reading the ratings: %d", status)
	}
}

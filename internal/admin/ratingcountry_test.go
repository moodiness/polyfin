package admin

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

// The rating country is a setting: the US by default, one of its choices,
// kept when a PUT leaves it out, and the ratings offered as a limit are
// those of the country.
func TestSettingsRatingCountry(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	ratingNames := func() []string {
		t.Helper()
		response, err := administrator.client.Get(api.url + "/admin/api/parental-ratings")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var ratings []struct{ Name string }
		if err := json.NewDecoder(response.Body).Decode(&ratings); err != nil {
			t.Fatal(err)
		}
		names := make([]string, len(ratings))
		for i, rating := range ratings {
			names[i] = rating.Name
		}
		return names
	}

	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	bounds, _ := body["bounds"].(map[string]any)
	country, _ := bounds["ratingCountry"].(map[string]any)
	choices, _ := country["choices"].([]any)
	if body["ratingCountry"] != "US" || country["default"] != "US" || !slices.Contains(choices, any("IT")) {
		t.Errorf("default rating country: %v, bounds %v", body["ratingCountry"], country)
	}
	if names := ratingNames(); !slices.Contains(names, "PG-13") || slices.Contains(names, "VM12") {
		t.Errorf("US ratings offered: %q", names)
	}

	settings := map[string]any{"serverName": "Maison", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en", "ratingCountry": "IT"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || body["ratingCountry"] != "IT" {
		t.Fatalf("saving Italy: %d %v", status, body)
	}
	if got := api.store.RatingCountry(); got != "IT" {
		t.Errorf("stored rating country: %q", got)
	}
	if names := ratingNames(); !slices.Contains(names, "VM12") || !slices.Contains(names, "VM14 / 14+") || slices.Contains(names, "PG-13") {
		t.Errorf("Italian ratings offered: %q", names)
	}

	settings["ratingCountry"] = "XX"
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusBadRequest || body["error"] != "invalid_rating_country" {
		t.Errorf("an unknown rating country: %d %v", status, body)
	}
	delete(settings, "ratingCountry")
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || body["ratingCountry"] != "IT" {
		t.Errorf("saving without a rating country: %d %v", status, body)
	}
	if got := api.store.RatingCountry(); got != "IT" {
		t.Errorf("rating country after the refused and the partial saves: %q", got)
	}
}

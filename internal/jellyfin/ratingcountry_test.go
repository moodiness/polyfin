package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The server configuration's MetadataCountryCode is the rating country:
// apps read it, the dashboard sets it in any case, and a code Polyfin has
// no ratings of is ignored. The parental ratings offered follow it.
func TestMetadataCountryCodeIsTheRatingCountry(t *testing.T) {
	t.Parallel()
	s, admin, _ := administrated(t)
	country := func(path string) string {
		t.Helper()
		var configuration struct{ MetadataCountryCode string }
		_, body := s.call(http.MethodGet, path, admin, nil)
		if err := json.Unmarshal(body, &configuration); err != nil {
			t.Fatalf("%s: %v %s", path, err, body)
		}
		return configuration.MetadataCountryCode
	}
	ratingNames := func() []string {
		t.Helper()
		var ratings []ParentalRating
		_, body := s.call(http.MethodGet, "/Localization/ParentalRatings", admin, nil)
		_ = json.Unmarshal(body, &ratings)
		names := make([]string, len(ratings))
		for i, rating := range ratings {
			names[i] = rating.Name
		}
		return names
	}
	if got, startup := country("/System/Configuration"), country("/Startup/Configuration"); got != "US" || startup != "US" {
		t.Errorf("default country: %q, at startup %q", got, startup)
	}
	if names := ratingNames(); !slices.Contains(names, "PG-13") || slices.Contains(names, "VM12") {
		t.Errorf("US ratings: %q", names)
	}
	if status, body := s.call(http.MethodPost, "/System/Configuration", admin, map[string]string{"MetadataCountryCode": "it"}); status != http.StatusNoContent {
		t.Fatalf("setting Italy: %d %s", status, body)
	}
	if got, startup := country("/System/Configuration"), country("/Startup/Configuration"); got != "IT" || startup != "IT" || s.store.RatingCountry() != "IT" {
		t.Errorf("country after setting Italy: %q, at startup %q, stored %q", got, startup, s.store.RatingCountry())
	}
	if names := ratingNames(); !slices.Contains(names, "VM12") || !slices.Contains(names, "VM14 / 14+") || slices.Contains(names, "PG-13") {
		t.Errorf("Italian ratings: %q", names)
	}
	if status, body := s.call(http.MethodPost, "/System/Configuration", admin, map[string]string{"MetadataCountryCode": "XX"}); status != http.StatusNoContent {
		t.Fatalf("an unknown country: %d %s", status, body)
	}
	if got := country("/System/Configuration"); got != "IT" {
		t.Errorf("an unknown country changed it to %q", got)
	}
}

// A collection's rating is its least restrictive one, scored under the
// rating country: under Italy, T (for all) before VM14.
func TestCollectionRatingsAreScoredUnderTheRatingCountry(t *testing.T) {
	t.Parallel()
	compare := func(a, b string) int { return compareRatings(a, b, "IT") }
	if got := slices.MinFunc([]string{"VM14", "T"}, compare); got != "T" {
		t.Errorf("least restrictive of VM14 and T under Italy: %q", got)
	}
}

// italianAddon serves two movies whose descriptions rate them for the US
// and for Italy: Night is PG in the US, VM18 in Italy, and Day is PG and T.
func italianAddon(t *testing.T) string {
	t.Helper()
	metas := map[string]stremio.Meta{
		"tt1": {ID: "tt1", Type: "movie", Name: "Night", Extras: &stremio.Extras{Certification: "PG", Certifications: stremio.Certifications{"IT": "VM18"}}},
		"tt2": {ID: "tt2", Type: "movie", Name: "Day", Extras: &stremio.Extras{Certification: "PG", Certifications: stremio.Certifications{"IT": "T"}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "italian", Name: "Italian", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			// Rows carry no rating, so titles are judged by their
			// descriptions.
			rows := []stremio.Meta{{ID: "tt1", Type: "movie", Name: "Night"}, {ID: "tt2", Type: "movie", Name: "Day"}}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": rows})
		case len(parts) == 3 && parts[0] == "meta":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": metas[parts[2]]})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A title is judged by its rating for the rating country: Night, PG in the
// US, is seen by a child limited to 13 there, and hidden from them under
// Italy, where it is VM18; Day stays.
func TestParentalControlFollowsTheRatingCountry(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), italianAddon(t), false); err != nil {
		t.Fatal(err)
	}
	adminToken, childToken := s.signIn("admin", "tv"), s.signIn("child", "tablet")
	var views QueryResult
	s.get(t, "/UserViews", adminToken, &views)
	if len(views.Items) == 0 {
		t.Fatal("no views")
	}
	titles := func() []string {
		t.Helper()
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+views.Items[0].Id, childToken, &page); status != http.StatusOK {
			t.Fatalf("listing: %d", status)
		}
		names := itemNames(page.Items)
		slices.Sort(names)
		return names
	}
	if got := titles(); !slices.Equal(got, []string{"Day", "Night"}) {
		t.Errorf("the child's titles under the US: %v", got)
	}
	settings := s.store.Settings()
	settings.RatingCountry = "IT"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if got := titles(); !slices.Equal(got, []string{"Day"}) {
		t.Errorf("the child's titles under Italy: %v", got)
	}
	// The title's details show its rating for Italy.
	var page QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, adminToken, &page)
	var day BaseItemDto
	for _, item := range page.Items {
		if item.Name == "Day" {
			s.get(t, "/Items/"+item.Id, adminToken, &day)
		}
	}
	if day.OfficialRating != "T" {
		t.Errorf("Day's rating under Italy: %q", day.OfficialRating)
	}
}

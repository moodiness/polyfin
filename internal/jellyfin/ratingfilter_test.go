package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// filtered lists what a listing of /Items answers user with, its titles'
// names sorted and its count.
func (e edited) filtered(t *testing.T, token, path string) ([]string, int) {
	t.Helper()
	var page QueryResult
	if status := e.get(t, path, token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	names := itemNames(page.Items)
	slices.Sort(names)
	return names, page.TotalRecordCount
}

// /Items narrows its titles by their ratings as Jellyfin's does: a lowest
// or highest rating keeps the titles within it and those without a
// rating, hasParentalRating=false keeps only the latter unless a limit
// applies, and true changes nothing. officialRatings and hasOfficialRating
// read the official rating. The ratings are those of the titles'
// descriptions: Allowed PG, Restricted R, Unrated none.
func TestItemsFilterByRating(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	top := "/Items?ParentId=" + e.ids["Top"] + "&"
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"maxOfficialRating=PG-13", []string{"Allowed", "Unrated"}},
		{"MaxOfficialRating=PG-13&hasParentalRating=true", []string{"Allowed", "Unrated"}},
		{"maxOfficialRating=G", []string{"Unrated"}},
		{"minOfficialRating=PG-13", []string{"Restricted", "Unrated"}},
		{"minOfficialRating=PG&maxOfficialRating=PG", []string{"Allowed", "Unrated"}},
		{"maxOfficialRating=Nonsense", []string{"Allowed", "Restricted", "Unrated"}},
		{"hasParentalRating=true", []string{"Allowed", "Restricted", "Unrated"}},
		{"hasParentalRating=false", []string{"Unrated"}},
		{"hasParentalRating=false&maxOfficialRating=R", []string{"Allowed", "Restricted", "Unrated"}},
		{"officialRatings=" + url.QueryEscape("PG|NC-17"), []string{"Allowed"}},
		{"hasOfficialRating=true", []string{"Allowed", "Restricted"}},
		{"hasOfficialRating=false", []string{"Unrated"}},
	} {
		if got, total := e.filtered(t, e.member, top+c.query); !slices.Equal(got, c.want) || total != len(c.want) {
			t.Errorf("%s: %v, counted %d", c.query, got, total)
		}
	}
	if status, _ := e.call(http.MethodGet, top+"hasParentalRating=maybe", app("tv", e.member), nil); status != http.StatusBadRequest {
		t.Errorf("a flag that is no boolean: %d", status)
	}
	// Titles asked by identifier, and found by a search, are narrowed too.
	ids := "/Items?ids=" + e.ids["Allowed"] + "," + e.ids["Restricted"] + "&maxOfficialRating=PG-13"
	if got, _ := e.filtered(t, e.member, ids); !slices.Equal(got, []string{"Allowed"}) {
		t.Errorf("by identifier: %v", got)
	}
	if got, _ := e.filtered(t, e.member, "/Items?searchTerm=Restricted&includeItemTypes=Movie&maxOfficialRating=PG-13"); len(got) != 0 {
		t.Errorf("a search: %v", got)
	}
	// A limit asked never lifts the user's parental control, and the
	// user's limit keeps hasParentalRating=false from narrowing, as in
	// Jellyfin.
	for _, query := range []string{"maxOfficialRating=R", "hasParentalRating=false"} {
		if got, total := e.filtered(t, e.child, top+query); !slices.Equal(got, []string{"Allowed", "Unrated"}) || total != 2 {
			t.Errorf("the child's titles with %s: %v, counted %d", query, got, total)
		}
	}
}

// Seasons and episodes are judged by their series' rating, as Jellyfin's
// inherit it, and have no official rating of their own.
func TestItemsFilterEpisodesByTheirSeriesRating(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	if got, _ := e.filtered(t, e.member, "/Items?ParentId="+e.ids["Shows"]+"&maxOfficialRating=PG-13"); len(got) != 0 {
		t.Errorf("the TV-MA series under PG-13: %v", got)
	}
	var seasons QueryResult
	e.get(t, "/Shows/"+e.ids["Show"]+"/Seasons", e.member, &seasons)
	if len(seasons.Items) != 1 {
		t.Fatalf("seasons: %v", itemNames(seasons.Items))
	}
	season := seasons.Items[0].Id
	for _, c := range []struct {
		path string
		want int
	}{
		{"/Items?ParentId=" + e.ids["Show"] + "&maxOfficialRating=PG-13", 0},
		{"/Items?ParentId=" + e.ids["Show"] + "&maxOfficialRating=TV-MA", 1},
		{"/Items?ParentId=" + season + "&maxOfficialRating=PG-13", 0},
		{"/Items?ParentId=" + season + "&minOfficialRating=TV-MA", 1},
		{"/Items?ParentId=" + season + "&hasParentalRating=false", 0},
		{"/Items?ParentId=" + season + "&hasOfficialRating=true", 0},
		{"/Items?ParentId=" + season + "&hasOfficialRating=false", 1},
	} {
		if got, total := e.filtered(t, e.member, c.path); len(got) != c.want || total != c.want {
			t.Errorf("%s: %v, counted %d", c.path, got, total)
		}
	}
}

// A title an administrator gave a custom rating is judged by it, as
// parental control judges it; its official rating stays its own.
func TestItemsFilterByCustomRating(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	allowed := e.ids["Allowed"]
	edit := editorBody(allowed, map[string]any{"Name": "Allowed", "OfficialRating": "PG", "CustomRating": "R"})
	if status, body := e.call(http.MethodPost, "/Items/"+allowed, app("dashboard", e.admin), edit); status != http.StatusNoContent {
		t.Fatalf("edit: %d %s", status, body)
	}
	top := "/Items?ParentId=" + e.ids["Top"] + "&"
	if got, total := e.filtered(t, e.member, top+"maxOfficialRating=PG-13"); !slices.Equal(got, []string{"Unrated"}) || total != 1 {
		t.Errorf("under PG-13: %v, counted %d", got, total)
	}
	if got, _ := e.filtered(t, e.member, top+"officialRatings=PG"); !slices.Equal(got, []string{"Allowed"}) {
		t.Errorf("officially PG: %v", got)
	}
}

// italianRatings serves two movies rated for the US and for Italy: Teen
// is PG-13 and VM14, Adult R and VM18.
func italianRatings(t *testing.T) string {
	t.Helper()
	metas := map[string]stremio.Meta{
		"tt1": {ID: "tt1", Type: "movie", Name: "Teen", Extras: &stremio.Extras{Certification: "PG-13", Certifications: stremio.Certifications{"IT": "VM14"}}},
		"tt2": {ID: "tt2", Type: "movie", Name: "Adult", Extras: &stremio.Extras{Certification: "R", Certifications: stremio.Certifications{"IT": "VM18"}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "italianrated", Name: "Italian", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			rows := []stremio.Meta{{ID: "tt1", Type: "movie", Name: "Teen"}, {ID: "tt2", Type: "movie", Name: "Adult"}}
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

// A rating asked as a limit is read under the rating country: under Italy,
// VM14 keeps the VM14 title and hides the VM18 one.
func TestItemsFilterByRatingUnderTheRatingCountry(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), italianRatings(t), false); err != nil {
		t.Fatal(err)
	}
	settings := s.store.Settings()
	settings.RatingCountry = "IT"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	e := edited{testServer: s, admin: s.signIn("admin", "tv")}
	var views QueryResult
	s.get(t, "/UserViews", e.admin, &views)
	if len(views.Items) == 0 {
		t.Fatal("no views")
	}
	if got, total := e.filtered(t, e.admin, "/Items?ParentId="+views.Items[0].Id+"&maxOfficialRating=VM14"); !slices.Equal(got, []string{"Teen"}) || total != 1 {
		t.Errorf("under VM14: %v, counted %d", got, total)
	}
}

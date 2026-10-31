package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// moreShowsAddon lists, in its "More" library, the show catalogAddon lists
// in "Shows".
func moreShowsAddon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "more", Name: "More", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}, IDPrefixes: []string{"tt"},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "more", Name: "More"}}})
		case strings.HasPrefix(path, "/catalog/series/more"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "tt0100", Type: "series", Name: "Show", ImdbID: "tt0100"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// As in Jellyfin 12.4, the libraries a user leaves out of their home
// screen sections (LatestItemsExcludes) are left out of Next Up and
// Continue Watching, unless another library lists the title too.
func TestHomeScreenSectionsLeaveOutTheExcludedLibraries(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0])
	for _, id := range []string{tr.movie, tr.episodes[2]} {
		tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": id, "PositionTicks": 0})
		tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": id, "PositionTicks": 12_000_000_000})
	}
	tr.testServer.user("other", nil)
	other := tr.signIn("other", "phone")
	if status, _ := tr.call(http.MethodPost, "/UserPlayedItems/"+tr.episodes[0], app("phone", other), nil); status != http.StatusOK {
		t.Fatalf("other's played: %d", status)
	}
	if got := tr.list(t, "/Shows/NextUp"); !slices.Equal(got, tr.episodes[1:2]) {
		t.Fatalf("next up before: %v", got)
	}
	if got := tr.list(t, "/UserItems/Resume"); !slices.Equal(got, []string{tr.episodes[2], tr.movie}) {
		t.Fatalf("resume before: %v", got)
	}

	if status := tr.configure(t, "/Users/Configuration", tr.token, map[string]any{
		"LatestItemsExcludes": []string{tr.views["Shows"]},
	}); status != http.StatusNoContent {
		t.Fatalf("configuring: %d", status)
	}
	if got := tr.list(t, "/Shows/NextUp"); len(got) != 0 {
		t.Errorf("next up while excluded: %v", got)
	}
	// The movie's library is not excluded.
	if got := tr.list(t, "/UserItems/Resume"); !slices.Equal(got, []string{tr.movie}) {
		t.Errorf("resume while excluded: %v", got)
	}
	// Asked within the library, or for the series, Jellyfin lists them.
	if got := tr.list(t, "/Shows/NextUp?parentId="+tr.views["Shows"]); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("next up within the library: %v", got)
	}
	if got := tr.list(t, "/Shows/NextUp?seriesId="+tr.series); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("next up of the series: %v", got)
	}
	if got := tr.list(t, "/UserItems/Resume?parentId="+tr.views["Shows"]); !slices.Equal(got, tr.episodes[2:3]) {
		t.Errorf("resume within the library: %v", got)
	}
	// The library itself still opens, and another user is unaffected.
	if got := tr.list(t, "/Items?ParentId="+tr.views["Shows"]); !slices.Equal(got, []string{tr.series}) {
		t.Errorf("the library: %v", got)
	}
	var page QueryResult
	if tr.get(t, "/Shows/NextUp", other, &page); !slices.Equal(itemIDs(page.Items), tr.episodes[1:2]) {
		t.Errorf("another user's next up: %v", itemIDs(page.Items))
	}

	// A library not excluded lists the show too: it stays.
	more, err := tr.addons.Install(t.Context(), addons.Shared(), moreShowsAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := tr.addons.Libraries(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	var choices []addons.LibraryChoice
	for _, l := range libraries {
		if l.Enabled && l.AddonID != more.ID {
			choices = append(choices, addons.LibraryChoice{AddonID: l.AddonID, CatalogType: l.Catalog.Type, CatalogID: l.Catalog.ID})
		}
	}
	choices = append(choices, addons.LibraryChoice{AddonID: more.ID, CatalogType: "series", CatalogID: "more"})
	if _, err := tr.addons.SetLibraries(t.Context(), addons.Shared(), choices); err != nil {
		t.Fatal(err)
	}
	moreView := tr.viewIDs(t, tr.token)["More"]
	if got := tr.list(t, "/Items?ParentId="+moreView); !slices.Equal(got, []string{tr.series}) {
		t.Fatalf("the other library: %v", got)
	}
	// Listed in the excluded library last, it is found in the other one.
	tr.list(t, "/Items?ParentId="+tr.views["Shows"])
	if got := tr.list(t, "/Shows/NextUp"); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("next up of a show another library lists: %v", got)
	}
	if got := tr.list(t, "/UserItems/Resume"); !slices.Equal(got, []string{tr.episodes[2], tr.movie}) {
		t.Errorf("resume of a show another library lists: %v", got)
	}
	// Both excluded, it is left out again.
	if status := tr.configure(t, "/Users/Configuration", tr.token, map[string]any{
		"LatestItemsExcludes": []string{tr.views["Shows"], moreView},
	}); status != http.StatusNoContent {
		t.Fatalf("configuring both: %d", status)
	}
	if got := tr.list(t, "/Shows/NextUp"); len(got) != 0 {
		t.Errorf("next up with both excluded: %v", got)
	}
}

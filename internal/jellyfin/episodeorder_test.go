package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// airingSeriesAddon serves a series catalog, Airing, whose listing gives
// no episode, of four series whose descriptions date their episodes
// around now: Late Show's latest regular episode aired 2 days ago, Early
// Show's 40 days ago but for a special 1 day ago, Middle Show's 10 days
// ago, with one more to air next week, and Quiet Show has none.
func airingSeriesAddon(t *testing.T) string {
	t.Helper()
	day := func(days int) string {
		return time.Now().UTC().AddDate(0, 0, days).Format(time.DateOnly) + "T00:00:00.000Z"
	}
	episode := func(id string, season, number, days int) stremio.Video {
		return stremio.Video{ID: fmt.Sprintf("%s:%d:%d", id, season, number), Season: stremio.Number(season), Episode: stremio.Number(number),
			Released: day(days)}
	}
	series := []stremio.Meta{
		{ID: "tt31", Type: "series", Name: "Early Show", Videos: []stremio.Video{episode("tt31", 1, 1, -50), episode("tt31", 1, 2, -40),
			episode("tt31", 0, 1, -1)}},
		{ID: "tt32", Type: "series", Name: "Late Show", Videos: []stremio.Video{episode("tt32", 1, 1, -9), episode("tt32", 1, 2, -2)}},
		{ID: "tt33", Type: "series", Name: "Middle Show", Videos: []stremio.Video{episode("tt33", 2, 1, -10), episode("tt33", 2, 2, 7)}},
		{ID: "tt34", Type: "series", Name: "Quiet Show"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "airing", Name: "Airing", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "airing", Name: "Airing"}}})
		case strings.HasPrefix(path, "/catalog/series/airing"):
			var previews []stremio.Meta
			for _, meta := range series {
				meta.Videos = nil
				previews = append(previews, meta)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": previews})
		case strings.HasPrefix(path, "/meta/series/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/series/"), ".json")
			for _, meta := range series {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// jellyfin-web's "Date Episode Added" sort of a series library asks for
// DateLastContentAdded: the series come by their latest regular episode
// aired, the latest first, specials and episodes to come left out, a
// series without one last, the same across pages. Its "Date Played" sort,
// SeriesDatePlayed, orders them by the episode the user last played.
func TestASeriesLibrarySortsByDateEpisodeAdded(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), airingSeriesAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "airing"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	if len(views.Items) != 1 {
		t.Fatalf("views: %v", itemNames(views.Items))
	}
	// As jellyfin-web's TV shows tab asks.
	query := func(sort, order string, start, limit int) string {
		return fmt.Sprintf("/Users/%s/Items?SortBy=%s,SortName&SortOrder=%s&IncludeItemTypes=Series&Recursive=true"+
			"&Fields=PrimaryImageAspectRatio&ImageTypeLimit=1&StartIndex=%d&Limit=%d&ParentId=%s", member.ID, sort, order, start, limit,
			views.Items[0].Id)
	}
	want := []string{"Late Show", "Middle Show", "Early Show", "Quiet Show"}
	// The descriptions are read in the background at the first listing.
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := s.listingNames(t, token, query("DateLastContentAdded", "Descending", 0, 100))
		if slices.Equal(got, want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Date Episode Added: %v, want %v", got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
	paged := append(s.listingNames(t, token, query("DateLastContentAdded", "Descending", 0, 2)),
		s.listingNames(t, token, query("DateLastContentAdded", "Descending", 2, 2))...)
	if !slices.Equal(paged, want) {
		t.Errorf("in pages of 2: %v", paged)
	}
	if got := s.listingNames(t, token, query("DateLastContentAdded", "Ascending", 0, 100)); !slices.Equal(got,
		[]string{"Quiet Show", "Early Show", "Middle Show", "Late Show"}) {
		t.Errorf("ascending: %v", got)
	}

	// The member played an episode of Quiet Show yesterday, and one of
	// Early Show an hour ago.
	ids := map[string]accounts.ID{}
	var page QueryResult
	s.get(t, query("SortName", "Ascending", 0, 100), token, &page)
	for _, item := range page.Items {
		id, _ := parseGUID(item.Id)
		ids[item.Name] = id
	}
	played := userdata.New(s.pool)
	for name, ago := range map[string]time.Duration{"Quiet Show": 24 * time.Hour, "Early Show": time.Hour} {
		if _, err := played.Change(t.Context(), member.ID, []userdata.Item{{ID: accounts.ID{byte(ago / time.Hour)}, Series: ids[name]}},
			func(d *userdata.Data) { d.MarkPlayed(new(time.Now().Add(-ago)), time.Now()) }); err != nil {
			t.Fatal(err)
		}
	}
	// The series never played come by name, descending too: a single
	// order applies to every key.
	if got := s.listingNames(t, token, query("SeriesDatePlayed", "Descending", 0, 100)); !slices.Equal(got,
		[]string{"Early Show", "Quiet Show", "Middle Show", "Late Show"}) {
		t.Errorf("Date Played: %v", got)
	}
}

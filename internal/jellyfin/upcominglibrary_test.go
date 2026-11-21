package jellyfin

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
)

// A library's Upcoming tab lists the coming episodes of its first series,
// followed or not, as Jellyfin lists every coming episode of a library. It
// answers at once with the series already described, and has the others
// described in the background; the hidden and dropped ones stay out, and
// the home row is unchanged.
func TestUpcomingInALibraryListsTheComingEpisodesOfItsSeries(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), airingAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "movies"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "tonight"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	tr := tracking{testServer: s, token: token, user: member.ID.String(), views: map[string]string{}}
	var page QueryResult
	s.get(t, "/UserViews", token, &page)
	for _, view := range page.Items {
		tr.views[view.Name] = view.Id
	}
	upcoming := func(query string) []string { t.Helper(); return tr.list(t, "/Shows/Upcoming?"+query) }
	library := "parentId=" + tr.views["Tonight"]
	series := map[string]string{}
	s.get(t, "/Items?ParentId="+tr.views["Tonight"], token, &page)
	for _, show := range page.Items {
		series[show.Name] = show.Id
	}
	// No series is followed, and none is described yet: the tab answers
	// at once with what is known, nothing, and has the library's series
	// described in the background.
	if got := upcoming(library); len(got) != 0 {
		t.Errorf("in the library, before its series are described: %v", got)
	}
	var got []string
	eventually(t, "the library's series described", func() bool {
		got = upcoming(library)
		return len(got) == 6
	})
	episodes := map[string]string{}
	for _, id := range series {
		var listed QueryResult
		s.get(t, "/Shows/"+id+"/Episodes", token, &listed)
		for _, episode := range listed.Items {
			episodes[episode.Name] = episode.Id
		}
	}
	named := func(names ...string) []string {
		ids := make([]string, 0, len(names))
		for _, name := range names {
			ids = append(ids, episodes[name])
		}
		return ids
	}
	// Then it lists the episodes of the library's series from yesterday
	// on, soonest first, as Jellyfin does.
	want := named("Bravo hours ago", "Bravo special", "Bravo tomorrow", "Charlie tomorrow", "Alpha next week", "Bravo next week")
	if !slices.Equal(got, want) {
		t.Errorf("in the library: %v, want %v", got, want)
	}
	// The home row still lists the followed series only.
	if got := upcoming(""); len(got) != 0 {
		t.Errorf("without a parent: %v", got)
	}
	// A movie library has no series.
	if got := upcoming("parentId=" + tr.views["Movies"]); len(got) != 0 {
		t.Errorf("in the movie library: %v", got)
	}

	// A hidden series and a dropped one stay out.
	if status := tr.send(t, http.MethodPost, "/Polyfin/UserItems/"+series["Charlie"]+"/Hidden", nil); status != http.StatusNoContent {
		t.Fatalf("hiding Charlie: %d", status)
	}
	s.drop(t, tr.user, "simkl", series["Alpha"], time.Now().AddDate(0, 0, -1))
	want = named("Bravo hours ago", "Bravo special", "Bravo tomorrow", "Bravo next week")
	if got := upcoming(library); !slices.Equal(got, want) {
		t.Errorf("with Charlie hidden and Alpha dropped: %v, want %v", got, want)
	}
}

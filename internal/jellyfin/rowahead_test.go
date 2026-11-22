package jellyfin

import (
	"maps"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The latest titles of a library have the first rowAheadTitles listed
// ahead, with playback prepared ahead: a movie's own versions, a series'
// next episode's. A row asked again asks nothing more, and a title opened
// from it lists its version at once.
func TestTheLatestRowsListTheirFirstTitlesAhead(t *testing.T) {
	t.Parallel()
	rs := rowsOn(t, false)
	var views QueryResult
	rs.get(t, "/UserViews", rs.token, &views)
	parents := map[string]string{}
	for _, view := range views.Items {
		parents[view.Name] = view.Id
	}
	latest := func(view string) (names, ids []string) {
		t.Helper()
		var items []BaseItemDto
		if status := rs.get(t, "/Users/"+rs.member.ID.String()+"/Items/Latest?Limit=16&ParentId="+parents[view], rs.token, &items); status != http.StatusOK ||
			len(items) != rowTitles {
			t.Fatalf("the latest %s: %d, %d titles", view, status, len(items))
		}
		for _, item := range items {
			names, ids = append(names, item.Name), append(ids, item.Id)
		}
		return names, ids
	}

	// Switched off, the rows ask the addons nothing.
	latest("Movies")
	rs.listedAhead(t)
	if asked := rs.addon.requests(); len(asked) != 0 {
		t.Fatalf("switched off: %v", asked)
	}

	rs.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	movies, movieIDs := latest("Movies")
	rs.listedAhead(t)
	want := onceEach(movies[:rowAheadTitles])
	if asked := rs.addon.requests(); !maps.Equal(asked, want) {
		t.Fatalf("the latest movies: %v, want %v", asked, want)
	}
	// The first episode of each series was played: the second is next.
	series, _ := latest("Shows")
	rs.listedAhead(t)
	for _, name := range series[:rowAheadTitles] {
		want[name+":1:2"]++
	}
	if asked := rs.addon.requests(); !maps.Equal(asked, want) {
		t.Fatalf("the latest series: %v, want %v", asked, want)
	}

	latest("Movies")
	latest("Shows")
	rs.listedAhead(t)
	if asked := rs.addon.requests(); !maps.Equal(asked, want) {
		t.Errorf("asked again: %v, want %v", asked, want)
	}
	var progress VersionProgress
	if rs.get(t, "/Polyfin/Items/"+movieIDs[0]+"/Versions", rs.token, &progress); progress != (VersionProgress{Pending: 0, Count: 1, Known: 1}) {
		t.Errorf("the first movie's versions: %+v", progress)
	}
	if asked := rs.addon.requests(); !maps.Equal(asked, want) {
		t.Errorf("after opening: %v, want %v", asked, want)
	}
}

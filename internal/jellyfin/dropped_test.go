package jellyfin

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
)

// drop records that user dropped series on service at at, as package
// trackers does.
func (s testServer) drop(t *testing.T, user, service, series string, at time.Time) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO tracker_dropped (user_id, service, series_id, dropped_at, seen_at)
		VALUES ($1, $2, $3, $4, $4)`, user, service, series, at); err != nil {
		t.Fatal(err)
	}
}

// A series dropped on a tracking service leaves Next Up and the episodes
// of Continue Watching, movies staying, while asked for alone it still
// answers; another user's drop changes nothing, and a play after the
// drop brings the series back.
func TestADroppedSeriesLeavesNextUpAndContinueWatching(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0]+"?datePlayed=2025-01-01T00:00:00Z")
	for _, id := range []string{tr.episodes[2], tr.movie} {
		tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": id, "PositionTicks": 0})
		tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": id, "PositionTicks": int64(12_000_000_000)})
	}
	nextUp := func(query string) []string { t.Helper(); return tr.list(t, "/Shows/NextUp?"+query) }
	resume := func() []string { t.Helper(); return tr.list(t, "/UserItems/Resume") }
	if got := nextUp(""); !slices.Equal(got, tr.episodes[1:2]) {
		t.Fatalf("next up: %v", got)
	}
	if got := resume(); !slices.Equal(got, []string{tr.movie, tr.episodes[2]}) {
		t.Fatalf("resume: %v", got)
	}

	// Another user's drop is theirs.
	other := tr.testServer.user("other", nil)
	tr.drop(t, other.ID.String(), "simkl", tr.series, time.Now())
	if got := nextUp(""); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("next up after another user's drop: %v", got)
	}

	// Dropped after the plays, on two services, the series leaves both
	// rows.
	tr.drop(t, tr.user, "simkl", tr.series, time.Now().AddDate(0, 0, -1))
	tr.drop(t, tr.user, "trakt", tr.series, time.Now().Add(time.Hour))
	if got := nextUp(""); len(got) != 0 {
		t.Errorf("next up while dropped: %v", got)
	}
	if got := resume(); !slices.Equal(got, []string{tr.movie}) {
		t.Errorf("resume while dropped: %v", got)
	}
	if got := nextUp("seriesId=" + tr.series); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("the series asked for: %v", got)
	}
	// What was played stays played.
	if data := tr.userData(t, tr.episodes[0]); !data.Played {
		t.Errorf("the played episode: %+v", data)
	}
	member, err := tr.store.User(t.Context(), mustID(t, tr.user))
	if err != nil {
		t.Fatal(err)
	}
	// Listed once, its services in the Tracking page's order, dated by
	// the later drop.
	dropped, err := tr.handler.NextUpDropped(t.Context(), member)
	if err != nil || len(dropped) != 1 || dropped[0].Item.ID != mustID(t, tr.series) ||
		!slices.Equal(dropped[0].Services, []string{"trakt", "simkl"}) || !dropped[0].At.After(time.Now()) {
		t.Errorf("dropped series: %+v %v", dropped, err)
	}

	// A play after the drop brings it back in both.
	if _, err := tr.pool.Exec(t.Context(), "UPDATE tracker_dropped SET dropped_at = $1, seen_at = $1", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.episodes[2], "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.episodes[2], "PositionTicks": int64(12_000_000_000)})
	if got := nextUp(""); len(got) == 0 {
		t.Errorf("next up after a later play: %v", got)
	}
	if got := resume(); !slices.Contains(got, tr.episodes[2]) {
		t.Errorf("resume after a later play: %v", got)
	}
	if dropped, err := tr.handler.NextUpDropped(t.Context(), member); err != nil || len(dropped) != 0 {
		t.Errorf("dropped series after a later play: %+v %v", dropped, err)
	}
}

// A series dropped on a tracking service is no candidate of Upcoming,
// played or favorite, nor of Next Up, until a play after the drop.
func TestADroppedSeriesLeavesUpcoming(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), freshAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "fresh"},
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
	series := map[string]string{}
	episodes := map[string]string{}
	s.get(t, "/Items?ParentId="+tr.views["Fresh"], token, &page)
	for _, show := range page.Items {
		series[show.Name] = show.Id
		var listed QueryResult
		s.get(t, "/Shows/"+show.Id+"/Episodes", token, &listed)
		for _, episode := range listed.Items {
			episodes[episode.Name] = episode.Id
		}
	}
	played := time.Now().UTC().AddDate(0, 0, -300).Format(time.RFC3339)
	for _, name := range []string{"Fresh 1", "Dateless 1"} {
		tr.mark(t, http.MethodPost, "/UserPlayedItems/"+episodes[name]+"?datePlayed="+played)
	}
	tr.mark(t, http.MethodPost, "/UserFavoriteItems/"+series["Fresh"])
	upcoming := func() []string { t.Helper(); return tr.list(t, "/Shows/Upcoming") }
	nextUp := func() []string { t.Helper(); return slices.Sorted(slices.Values(tr.list(t, "/Shows/NextUp"))) }
	fresh := []string{episodes["Fresh 2"], episodes["Fresh 3"]}
	if got := upcoming(); !slices.Equal(got, fresh) {
		t.Fatalf("upcoming: %v, want %v", got, fresh)
	}

	s.drop(t, tr.user, "simkl", series["Fresh"], time.Now().AddDate(0, 0, -10))
	if got := upcoming(); len(got) != 0 {
		t.Errorf("upcoming while dropped: %v", got)
	}
	if got := nextUp(); !slices.Equal(got, []string{episodes["Dateless 2"]}) {
		t.Errorf("next up while dropped: %v", got)
	}

	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+episodes["Fresh 1"]+"?datePlayed="+time.Now().UTC().Format(time.RFC3339))
	if got := upcoming(); !slices.Equal(got, fresh) {
		t.Errorf("upcoming after a later play: %v", got)
	}
	if got, want := nextUp(), slices.Sorted(slices.Values([]string{episodes["Fresh 2"], episodes["Dateless 2"]})); !slices.Equal(got, want) {
		t.Errorf("next up after a later play: %v, want %v", got, want)
	}
}

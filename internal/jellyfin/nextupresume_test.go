package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Remove from Continue Watching leaves Next Up as it is, the episode then
// showing there, unless the user chose that it takes the series off Next
// Up too, as Remove from Next Up does.
func TestRemovingFromContinueWatchingTakesOffNextUpWhenChosen(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	member, err := accounts.ParseID(tr.user)
	if err != nil {
		t.Fatal(err)
	}
	web := func() []string { t.Helper(); return tr.list(t, "/Shows/NextUp?enableResumable=false") }
	underWay := func() {
		t.Helper()
		tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.episodes[1], "PositionTicks": 0})
		tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.episodes[1], "PositionTicks": 6_000_000_000})
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0]+"?datePlayed=2025-01-01T00:00:00Z")
	underWay()
	if got := web(); len(got) != 0 {
		t.Fatalf("with the episode under way: %v", got)
	}
	tr.mark(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.episodes[1]+"/Resume")
	if got := web(); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("off Continue Watching, without the choice: %v", got)
	}

	if _, err := tr.store.UpdateUser(t.Context(), member, accounts.UserChanges{NextUpWithResume: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	underWay()
	tr.mark(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.episodes[1]+"/Resume")
	if got := tr.list(t, "/UserItems/Resume"); len(got) != 0 {
		t.Errorf("Continue Watching with the choice: %v", got)
	}
	if got := web(); len(got) != 0 {
		t.Errorf("off Continue Watching, with the choice: %v", got)
	}
	if got := tr.list(t, "/Shows/NextUp?seriesId="+tr.series); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("the series asked for: %v", got)
	}
	// A play brings it back, as after Remove from Next Up.
	underWay()
	if got := tr.list(t, "/Shows/NextUp"); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("after a play: %v", got)
	}
}

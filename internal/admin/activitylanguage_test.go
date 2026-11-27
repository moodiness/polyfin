package admin

import (
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
)

// Recent activity names its entries in the language the admin app asks
// for, with their short overviews and the titles they link where each
// language puts them, in UTF-16 code units; without a language Polyfin
// speaks, in the server's.
func TestActivityIsNamedInTheLanguageAsked(t *testing.T) {
	t.Parallel()
	shown := accounts.ID{1}
	var log *activity.Store
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.ActivityTitles, o.WebClient = &reachedItems{ids: []accounts.ID{shown}}, true
		log = o.Activity
	})
	admin := api.signedIn("root", true)
	log.PlaybackStarted(t.Context(), accounts.User{ID: accounts.ID{9}, Name: "Zoé 🎬"}, "Amélie", shown.String(), "TV")

	type entry struct {
		name, address string
		start, end    int
	}
	english := []entry{{"Zoé 🎬 is playing Amélie on TV", "", 18, 24}, {"root signed in", "Address: ", 0, 0}}
	for language, want := range map[string][]entry{
		"en":    english,
		"fr":    {{"Zoé 🎬 regarde Amélie sur TV", "", 15, 21}, {"Connexion de root", "Adresse\u202f: ", 0, 0}},
		"de":    {{"Zoé 🎬 schaut Amélie auf TV", "", 14, 20}, {"root hat sich angemeldet", "Adresse: ", 0, 0}},
		"de-AT": {{"Zoé 🎬 schaut Amélie auf TV", "", 14, 20}, {"root hat sich angemeldet", "Adresse: ", 0, 0}},
		"":      english,
		"nl":    english,
	} {
		var page struct {
			Items []activityEntryJSON `json:"items"`
		}
		if status := admin.raw(http.MethodGet, "/activity?type=VideoPlayback,AuthenticationSucceeded&language="+language, "", &page); status != http.StatusOK {
			t.Fatalf("%q: %d", language, status)
		}
		if len(page.Items) != len(want) {
			t.Fatalf("%q: %+v", language, page.Items)
		}
		for i, e := range page.Items {
			got := entry{name: e.Name}
			if e.ShortOverview != nil {
				got.address, _, _ = strings.Cut(*e.ShortOverview, "127.0.0.1")
			}
			if e.Title != nil {
				got.start, got.end = e.Title.Start, e.Title.End
			}
			if got != want[i] {
				t.Errorf("%q: %+v, want %+v", language, got, want[i])
			}
		}
	}
}

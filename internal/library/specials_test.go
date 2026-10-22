package library

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// showSpecials turns Settings.HideSpecials off, for tests about season 0.
func (e env) showSpecials(t *testing.T) {
	t.Helper()
	settings := e.users.Settings()
	settings.HideSpecials = false
	if _, err := e.users.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
}

// Specials (season 0) are left out of a series' seasons and episodes by
// default, and listed again when the setting is off.
func TestSpecialsAreHiddenByDefault(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	show := stremio.Meta{ID: "tt300", Type: "series", Name: "Show", Videos: []stremio.Video{
		{ID: "tt300:0:1", Title: "Special", Season: 0, Episode: 1, Released: "2019-12-01T00:00:00Z"},
		{ID: "tt300:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z"},
	}}
	preview := show
	preview.Videos = nil
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"series"}, IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"series/top": {preview}},
		metas:    map[string]stremio.Meta{"series/tt300": show},
	})
	libraries, err := e.service.Libraries(t.Context(), e.member)
	if err != nil || len(libraries) != 1 {
		t.Fatalf("libraries: %v, %v", libraries, err)
	}
	page, err := e.service.Children(t.Context(), e.member, libraries[0].ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("titles: %v, %v", page.Items, err)
	}
	series := page.Items[0].ID
	seasons, err := e.service.Seasons(t.Context(), e.member, series)
	if err != nil || slices.ContainsFunc(seasons, func(s Item) bool { return s.IndexNumber == 0 }) || len(seasons) != 1 {
		t.Fatalf("seasons with specials hidden: %v, %v", seasons, err)
	}
	episodes, err := e.service.Episodes(t.Context(), e.member, series, nil)
	if err != nil || len(episodes) != 1 || episodes[0].ParentIndexNumber != 1 {
		t.Fatalf("episodes with specials hidden: %v, %v", episodes, err)
	}
	e.showSpecials(t)
	if episodes, err = e.service.Episodes(t.Context(), e.member, series, nil); err != nil || len(episodes) != 2 {
		t.Fatalf("episodes with specials shown: %v, %v", episodes, err)
	}
}

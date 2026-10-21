package library

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The search catalogs a local folder's titles are looked up in: those of
// the addons describing titles, whose required properties are the search
// alone, as Stremio apps search; neither people searches, which look up
// who is credited, nor Polyfin's own sources, which neither search the
// metadata of titles nor keep the addons given a language from standing
// in for the addons without one.
func TestRemoteSources(t *testing.T) {
	t.Parallel()
	metadata := func(name, language string, catalogs ...stremio.Catalog) installed {
		return installed{addon: addons.Addon{Kind: addons.KindStremio, Language: language, Manifest: stremio.Manifest{ID: name, Name: name,
			Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}, Catalogs: catalogs}}}
	}
	search := func(id string, extra ...stremio.Extra) stremio.Catalog {
		return stremio.Catalog{Type: "movie", ID: id, Name: id, Extra: append([]stremio.Extra{{Name: "search", IsRequired: true}}, extra...)}
	}
	iptv := installed{addon: addons.Addon{Kind: addons.KindM3U, Manifest: stremio.Manifest{ID: "tv", Name: "TV", Types: []string{"movie"},
		Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}, Catalogs: []stremio.Catalog{search("tv.movie.search")}}}}
	names := func(sources []source) []string {
		var ids []string
		for _, src := range sources {
			ids = append(ids, src.addon.addon.Manifest.Name+"/"+src.catalog.ID)
		}
		return ids
	}
	for _, c := range []struct {
		name   string
		addons []installed
		want   []string
	}{
		{"an IPTV source beside a translator", []installed{iptv, metadata("French", "fr", search("search"))}, []string{"French/search"}},
		{"a people search", []installed{metadata("Meta", "", search("search"), search("people_search_movie"))}, []string{"Meta/search"}},
		{"other required properties", []installed{metadata("Meta", "", search("search"),
			search("by_genre", stremio.Extra{Name: "genre", IsRequired: true, Options: []string{"Drama"}}),
			search("paged", stremio.Extra{Name: "skip"}, stremio.Extra{Name: "genre", Options: []string{"Drama"}}))},
			[]string{"Meta/search", "Meta/paged"}},
	} {
		if got := names(remoteSources(view{addons: c.addons}, KindMovie, "Heist")); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

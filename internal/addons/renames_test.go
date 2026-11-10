package addons

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// A catalog gone and a catalog new are a rename when they share their
// type, their name, trimmed and case ignored, and their extra properties
// by name and requirement, and no other catalog gone or new does.
func TestMatchRenames(t *testing.T) {
	t.Parallel()
	genre := []stremio.Extra{{Name: "genre", Options: []string{"Drama"}}, {Name: "skip"}}
	manifest := func(catalogs ...stremio.Catalog) stremio.Manifest { return stremio.Manifest{Catalogs: catalogs} }
	for _, c := range []struct {
		name     string
		old, new stremio.Manifest
		want     []Rename
	}{
		{"an identifier changed",
			manifest(stremio.Catalog{Type: "movie", ID: "top", Name: "Top", Extra: genre}, stremio.Catalog{Type: "series", ID: "top", Name: "Top"}),
			manifest(stremio.Catalog{Type: "movie", ID: "top2", Name: " top ", Extra: []stremio.Extra{{Name: "skip"}, {Name: "genre", Options: []string{"Comedy"}}}},
				stremio.Catalog{Type: "series", ID: "top", Name: "Top"}),
			[]Rename{{CatalogType: "movie", From: "top", To: "top2"}}},
		{"several, each of its type",
			manifest(stremio.Catalog{Type: "movie", ID: "a", Name: "Top"}, stremio.Catalog{Type: "series", ID: "a", Name: "Top"}),
			manifest(stremio.Catalog{Type: "series", ID: "b", Name: "Top"}, stremio.Catalog{Type: "movie", ID: "b", Name: "Top"}),
			[]Rename{{CatalogType: "movie", From: "a", To: "b"}, {CatalogType: "series", From: "a", To: "b"}}},
		{"two new catalogs of that name",
			manifest(stremio.Catalog{Type: "movie", ID: "top", Name: "Trending"}),
			manifest(stremio.Catalog{Type: "movie", ID: "top2", Name: "Trending"}, stremio.Catalog{Type: "movie", ID: "top3", Name: "trending"}),
			nil},
		{"two catalogs gone of that name",
			manifest(stremio.Catalog{Type: "movie", ID: "a", Name: "Trending"}, stremio.Catalog{Type: "movie", ID: "b", Name: "Trending"}),
			manifest(stremio.Catalog{Type: "movie", ID: "c", Name: "Trending"}),
			nil},
		{"an extra property changed",
			manifest(stremio.Catalog{Type: "movie", ID: "top", Name: "Top", Extra: genre}),
			manifest(stremio.Catalog{Type: "movie", ID: "top2", Name: "Top", Extra: []stremio.Extra{{Name: "year"}, {Name: "skip"}}}),
			nil},
		{"an extra property now required",
			manifest(stremio.Catalog{Type: "movie", ID: "top", Name: "Top", Extra: genre}),
			manifest(stremio.Catalog{Type: "movie", ID: "top2", Name: "Top", Extra: []stremio.Extra{{Name: "genre", IsRequired: true, Options: []string{"Drama"}},
				{Name: "skip"}}}),
			nil},
		{"the type changed",
			manifest(stremio.Catalog{Type: "movie", ID: "top", Name: "Top"}),
			manifest(stremio.Catalog{Type: "series", ID: "top2", Name: "Top"}),
			nil},
		{"the name changed",
			manifest(stremio.Catalog{Type: "movie", ID: "top", Name: "Top"}),
			manifest(stremio.Catalog{Type: "movie", ID: "top2", Name: "Best"}),
			nil},
		{"nothing gone",
			manifest(stremio.Catalog{Type: "movie", ID: "top", Name: "Top"}),
			manifest(stremio.Catalog{Type: "movie", ID: "top", Name: "Top"}, stremio.Catalog{Type: "movie", ID: "top2", Name: "Top"}),
			nil},
	} {
		if got := matchRenames(c.old, c.new); !slices.Equal(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A catalog keeps the key it had, a catalog renamed takes the key of the
// catalog it was, back to its own when it takes its first identifier
// again, and a catalog that brings back an identifier a library followed
// away from takes a key of its own.
func TestSettleKeys(t *testing.T) {
	t.Parallel()
	catalogs := func(ids ...string) stremio.Manifest {
		var m stremio.Manifest
		for _, id := range ids {
			m.Catalogs = append(m.Catalogs, stremio.Catalog{Type: "movie", ID: id})
		}
		return m
	}
	key := func(id string) CatalogKey { return CatalogKey{Type: "movie", ID: id} }
	kept := func(catalog, id string) KeptKey { return KeptKey{Catalog: key(catalog), Key: key(id)} }
	for _, c := range []struct {
		name     string
		keys     []KeptKey
		manifest stremio.Manifest
		moved    map[CatalogKey]CatalogKey
		want     []KeptKey
	}{
		{"none kept", nil, catalogs("a", "b"), nil, []KeptKey{}},
		{"a rename", nil, catalogs("b"), map[CatalogKey]CatalogKey{key("b"): key("a")}, []KeptKey{kept("b", "a")}},
		{"a rename again keeps the first key", []KeptKey{kept("b", "a")}, catalogs("c"), map[CatalogKey]CatalogKey{key("c"): key("b")},
			[]KeptKey{kept("c", "a")}},
		{"renamed back", []KeptKey{kept("b", "a")}, catalogs("a"), map[CatalogKey]CatalogKey{key("a"): key("b")}, []KeptKey{}},
		{"kept", []KeptKey{kept("b", "a")}, catalogs("b", "x"), nil, []KeptKey{kept("b", "a")}},
		{"gone", []KeptKey{kept("b", "a")}, catalogs("x"), nil, []KeptKey{}},
		{"the old identifier back", []KeptKey{kept("b", "a")}, catalogs("a", "b"), nil, []KeptKey{kept("b", "a"), kept("a", "a|2")}},
		{"the old identifier back, its key taken too", []KeptKey{kept("b", "a")}, catalogs("a", "a|2", "b"), nil,
			[]KeptKey{kept("b", "a"), kept("a", "a|3")}},
		{"the key of its own kept", []KeptKey{kept("b", "a"), kept("a", "a|2")}, catalogs("a", "b"), nil,
			[]KeptKey{kept("a", "a|2"), kept("b", "a")}},
	} {
		if got := settleKeys(c.keys, c.manifest, c.moved); !slices.Equal(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

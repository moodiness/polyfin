package jellyfin

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// shapedAddon serves a movie catalog, Top, of four movies whose posters
// declare each shape: Wide (landscape), Square, Tall (poster) and Plain
// (none); and a collection catalog, Sets, of one wide collection, Wide set,
// which groups Top.
func shapedAddon(t *testing.T) string {
	t.Helper()
	// The titles are made once the server, which their posters name, runs:
	// no request comes before.
	var top []stremio.Meta
	metas := map[string]stremio.Meta{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		set := stremio.Meta{ID: "col:wide", Type: "collection", Name: "Wide set", Poster: server.URL + "/poster/set.jpg", PosterShape: "landscape"}
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "shaped", Name: "Shaped", Version: "1",
				Types: []string{"movie", "collection"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "collection", ID: "sets", Name: "Sets"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": top})
		case strings.HasPrefix(path, "/catalog/collection/sets"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{set}})
		case path == "/meta/collection/col:wide" || path == "/meta/collection/col%3Awide":
			set.Collection = &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "top"}}}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": set})
		case strings.HasPrefix(path, "/meta/"):
			meta, ok := metas[strings.TrimPrefix(path, "/meta/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		case strings.HasPrefix(path, "/poster/"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("poster"))
		default:
			http.NotFound(w, r)
		}
	}))
	for _, m := range []struct{ id, name, shape string }{
		{"tt1", "Wide", "landscape"}, {"tt2", "Square", "square"}, {"tt3", "Tall", "poster"}, {"tt4", "Plain", ""},
	} {
		meta := stremio.Meta{ID: m.id, Type: "movie", Name: m.name, Poster: server.URL + "/poster/" + m.id + ".jpg", PosterShape: m.shape}
		top, metas["movie/"+m.id] = append(top, meta), meta
	}
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// aspectRatio is an item's PrimaryImageAspectRatio, 0 without one.
func aspectRatio(item BaseItemDto) float64 {
	if item.PrimaryImageAspectRatio == nil {
		return 0
	}
	return *item.PrimaryImageAspectRatio
}

// A title or collection whose addon declares a landscape poster gets a
// PrimaryImageAspectRatio of 16:9, a square one 1:1, and a poster, or none,
// 2:3, in listings and details, within collections too. A poster uploaded
// in its place drops the shape.
func TestAddonPosterShapesGiveTheirAspectRatio(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	if _, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	addon, err := s.addons.Install(t.Context(), addons.Shared(), shapedAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "sets"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("admin", "web")
	views := s.viewIDs(t, token)
	const wide, square, tall = 16.0 / 9.0, 1.0, 2.0 / 3.0
	want := map[string]float64{"Wide": wide, "Square": square, "Tall": tall, "Plain": tall}
	ids := map[string]string{}
	check := func(what, path string, want map[string]float64) {
		t.Helper()
		var page QueryResult
		if status := s.get(t, path, token, &page); status != http.StatusOK || len(page.Items) != len(want) {
			t.Fatalf("%s: %d %v", what, status, itemNames(page.Items))
		}
		for _, item := range page.Items {
			ids[item.Name] = item.Id
			if got := aspectRatio(item); got != want[item.Name] {
				t.Errorf("%s: %s's ratio is %v, want %v", what, item.Name, got, want[item.Name])
			}
		}
	}
	details := func(what, name string, want float64) {
		t.Helper()
		var item BaseItemDto
		if status := s.get(t, "/Items/"+ids[name], token, &item); status != http.StatusOK {
			t.Fatalf("%s: %d", what, status)
		}
		if got := aspectRatio(item); got != want {
			t.Errorf("%s: %s's ratio is %v, want %v", what, name, got, want)
		}
	}

	check("the library", "/Items?ParentId="+views["Top"]+"&Fields=PrimaryImageAspectRatio", want)
	for name, ratio := range want {
		details("details", name, ratio)
	}
	check("the collection library", "/Items?ParentId="+views["Sets"]+"&Fields=PrimaryImageAspectRatio", map[string]float64{"Wide set": wide})
	details("the collection's details", "Wide set", wide)
	check("the collection's titles", "/Items?ParentId="+ids["Wide set"]+"&Fields=PrimaryImageAspectRatio", want)
	boxSet := s.createCollection(t, token, "name=Picks&ids="+ids["Wide"]+","+ids["Square"]+","+ids["Tall"])
	check("a collection of the user's", "/Items?ParentId="+boxSet+"&Fields=PrimaryImageAspectRatio",
		map[string]float64{"Wide": wide, "Square": square, "Tall": tall})

	// A poster uploaded in place of the addon's has no shape of the addon's.
	for _, name := range []string{"Wide", "Square", "Wide set"} {
		status, _, body := s.send(http.MethodPost, "/Items/"+ids[name]+"/Images/Primary", app("dashboard", token), "image/png",
			base64.StdEncoding.EncodeToString(picture(t, 20, 30, 255)))
		if status != http.StatusNoContent {
			t.Fatalf("upload to %s: %d %s", name, status, body)
		}
	}
	uploaded := map[string]float64{"Wide": tall, "Square": tall, "Tall": tall, "Plain": tall}
	check("the library, posters uploaded", "/Items?ParentId="+views["Top"]+"&Fields=PrimaryImageAspectRatio", uploaded)
	check("the collection library, poster uploaded", "/Items?ParentId="+views["Sets"]+"&Fields=PrimaryImageAspectRatio", map[string]float64{"Wide set": tall})
	for name, ratio := range uploaded {
		details("details, posters uploaded", name, ratio)
	}
	details("the collection's details, poster uploaded", "Wide set", tall)
}

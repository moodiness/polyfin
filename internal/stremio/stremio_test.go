package stremio

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestParseManifest(t *testing.T) {
	manifest, err := ParseManifest([]byte(`{
		"id": "org.example", "version": "1.0.0", "name": "Example",
		"logo": "http://insecure.example/logo.png",
		"resources": ["catalog", {"name": "stream", "types": ["movie"], "idPrefixes": ["tt"]}],
		"catalogs": [
			{"type": "movie", "id": "top", "name": "Top", "extra": [{"name": "genre", "isRequired": true, "options": ["All", "Action"]}, {"name": "skip"}]},
			{"type": "movie", "id": "search", "name": "Search", "extra": [{"name": "search", "isRequired": true}]},
			{"type": "series", "id": "legacy", "extraSupported": ["search", "genre"], "extraRequired": ["search"]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.ResourceNames(); strings.Join(got, ",") != "catalog,stream" {
		t.Errorf("resources: %v", got)
	}
	if manifest.Logo != "" {
		t.Errorf("an insecure logo URL was kept: %q", manifest.Logo)
	}
	for id, want := range map[string]bool{"top": true, "search": false, "legacy": false} {
		catalog := manifest.Catalogs[map[string]int{"top": 0, "search": 1, "legacy": 2}[id]]
		if catalog.Browsable() != want {
			t.Errorf("%s browsable = %v, want %v", id, catalog.Browsable(), want)
		}
	}
	if legacy := manifest.Catalogs[2]; legacy.Name != "legacy" || len(legacy.Extra) != 2 {
		t.Errorf("legacy extras not converted: %+v", legacy)
	}

	for _, invalid := range []string{`[]`, `{"id": "x", "name": "x", "version": "1"}`, `<html>`} {
		if _, err := ParseManifest([]byte(invalid)); !errors.Is(err, ErrInvalidManifest) {
			t.Errorf("%s: got %v", invalid, err)
		}
	}
}

func TestManifestURLs(t *testing.T) {
	for raw, want := range map[string]string{
		" stremio://addon.example/abc/manifest.json ": "https://addon.example/abc/manifest.json",
		"HTTPS://addon.example/manifest.json#x":       "https://addon.example/manifest.json",
		"http://192.168.1.2:3000/manifest.json":       "http://192.168.1.2:3000/manifest.json",
	} {
		if got, err := NormalizeManifestURL(raw); err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"ftp://addon.example/manifest.json", "https://addon.example/catalog.json", "https://user:pw@addon.example/manifest.json", "manifest.json"} {
		if _, err := NormalizeManifestURL(raw); !errors.Is(err, ErrInvalidManifestURL) {
			t.Errorf("%q accepted", raw)
		}
	}
	if got := RedactManifestURL("https://addon.example/u/secret-token/manifest.json"); got != "https://addon.example/…/manifest.json" {
		t.Errorf("redacted: %q", got)
	}
}

func TestConfinedRequestsStayOffLocalNetworks(t *testing.T) {
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "x", "name": "X", "version": "1", "resources": ["catalog"]}`))
	}))
	defer addon.Close()
	client := NewClient("test")
	manifestURL := addon.URL + "/secret-token/manifest.json"

	if _, err := client.Manifest(t.Context(), manifestURL, false); err != nil {
		t.Fatalf("trusted request to a local addon: %v", err)
	}
	_, err := client.Manifest(t.Context(), manifestURL, true)
	if !errors.Is(err, ErrPrivateNetwork) {
		t.Fatalf("confined request to a local addon: got %v", err)
	}

	addon.Close()
	_, err = client.Manifest(t.Context(), manifestURL, false)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("closed addon: got %v", err)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("the error exposes the manifest URL: %v", err)
	}
}

// Addons type some fields loosely, AIOMetadata sending a title's directors
// as one string for one: such a field must not cost the title, or the
// catalog page it is on.
func TestLooselyTypedTitles(t *testing.T) {
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/movie/top.json":
			_, _ = w.Write([]byte(`{"metas": [
				{"id": "tt1", "type": "movie", "director": "Joe Russo, Anthony Russo", "genres": "Action", "writer": ["Writer", 7, {"name": "Object"}], "name": "One"},
				{"id": "tt2", "type": "movie", "videos": "none", "name": "Two"},
				{"id": "tt3", "type": "movie", "director": ["Director"], "name": "Three"}]}`))
		case "/catalog/movie/broken.json":
			_, _ = w.Write([]byte(`{"metas": "none"}`))
		case "/meta/movie/tt1.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt1", "type": "movie", "cast": "Actor", "trailers": {"source": "x"}, "name": "One"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer addon.Close()
	client := NewClient("test")
	manifestURL := addon.URL + "/manifest.json"

	metas, err := client.Catalog(t.Context(), manifestURL, "movie", "top", nil, false)
	if err != nil || len(metas) != 3 {
		t.Fatalf("catalog: %v %+v", err, metas)
	}
	if one := metas[0]; one.Name != "One" || !slices.Equal(one.Director, Names{"Joe Russo", "Anthony Russo"}) ||
		!slices.Equal(one.Genres, Names{"Action"}) || !slices.Equal(one.Writer, Names{"Writer"}) {
		t.Errorf("names: %+v", one)
	}
	// The field Polyfin cannot read is left empty, and the fields after it
	// read.
	if two := metas[1]; two.ID != "tt2" || two.Videos != nil || two.Name != "Two" {
		t.Errorf("mistyped field: %+v", two)
	}
	if three := metas[2]; !slices.Equal(three.Director, Names{"Director"}) {
		t.Errorf("list: %+v", three)
	}
	if _, err := client.Catalog(t.Context(), manifestURL, "movie", "broken", nil, false); !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("a page that is not a list: %v", err)
	}

	meta, err := client.Meta(t.Context(), manifestURL, "movie", "tt1", false)
	if err != nil || meta.Name != "One" || !slices.Equal(meta.Cast, Names{"Actor"}) || meta.Trailers != nil {
		t.Errorf("meta: %v %+v", err, meta)
	}
}

// Season posters come in the order of the seasons, a missing one as null:
// an element that is not an image keeps its place, so that the others stay
// with their seasons. Keyed by season number instead, they keep their
// numbers, and a poster the addon also gives by number keeps that one.
func TestSeasonPostersKeepTheirPlaces(t *testing.T) {
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/meta/series/tt1.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt1", "type": "series", "name": "Show",
				"app_extras": {"seasonPosters": ["https://images.example/1.jpg", null, 7, "https://images.example/4.jpg"]}}}`))
		case "/meta/series/tt2.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt2", "type": "series", "name": "Other",
				"app_extras": {"seasonPosters": "https://images.example/1.jpg", "certification": "TV-14"}}}`))
		case "/meta/series/tt3.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt3", "type": "series", "name": "Keyed",
				"app_extras": {"seasonPosters": {"0": "https://images.example/s0.jpg", "1": "https://images.example/s1.jpg",
					"2": null, "3": 7, "x": "https://images.example/x.jpg"},
					"seasonPosterByNumber": {"1": "https://images.example/one.jpg"}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer addon.Close()
	client := NewClient("test")
	meta, err := client.Meta(t.Context(), addon.URL+"/manifest.json", "series", "tt1", false)
	if err != nil || meta.Extras == nil ||
		!slices.Equal(meta.Extras.SeasonPosters, Posters{"https://images.example/1.jpg", "", "", "https://images.example/4.jpg"}) {
		t.Errorf("posters: %v %+v", err, meta.Extras)
	}
	// Anything but a list is left out, and the rest read.
	other, err := client.Meta(t.Context(), addon.URL+"/manifest.json", "series", "tt2", false)
	if err != nil || other.Name != "Other" || other.Extras == nil || other.Extras.SeasonPosters != nil || other.Extras.Certification != "TV-14" {
		t.Errorf("not a list: %v %+v", err, other.Extras)
	}
	keyed, err := client.Meta(t.Context(), addon.URL+"/manifest.json", "series", "tt3", false)
	want := map[string]string{"0": "https://images.example/s0.jpg", "1": "https://images.example/one.jpg"}
	if err != nil || keyed.Extras == nil || keyed.Extras.SeasonPosters != nil || !maps.Equal(keyed.Extras.SeasonPosterByNumber, want) {
		t.Errorf("keyed by season number: %v %+v", err, keyed.Extras)
	}
}

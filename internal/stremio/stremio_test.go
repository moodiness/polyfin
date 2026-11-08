package stremio

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestParseManifest(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// Addons type some fields loosely, some sending a title's directors
// as one string for one: such a field must not cost the title, or the
// catalog page it is on.
func TestLooselyTypedTitles(t *testing.T) {
	t.Parallel()
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

// The cast, directors and writers an addon gives as links, in place of the
// lists the protocol deprecates, are read into those lists, in their order,
// their categories in any letter case, as Stremio apps show them. A list
// the addon gives wins over its links; links of other categories, and
// elements that are no links, are left out.
func TestLinksCreditPeople(t *testing.T) {
	t.Parallel()
	links := `"links": [
		{"name": "Drama", "category": "Genres", "url": "stremio:///discover"},
		{"name": "Ana Actor", "category": "Cast", "url": "stremio:///search?search=Ana"},
		{"name": "Eve Director", "category": "Directors", "url": "stremio:///search?search=Eve"},
		{"name": "Ben Actor", "category": "actor", "url": "stremio:///search?search=Ben"},
		"Not a link", {"name": 7, "category": "Cast"}, {"name": " ", "category": "Cast"},
		{"name": "Finn Writer", "category": "WRITERS", "url": "stremio:///search?search=Finn"},
		{"name": "Gus Writer", "category": "writer", "url": "stremio:///search?search=Gus"},
		{"name": "tt1", "category": "imdb", "url": "https://example.org/title/tt1"}]`
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/movie/top.json":
			_, _ = w.Write([]byte(`{"metas": [{"id": "tt1", "type": "movie", "name": "Linked", ` + links + `}]}`))
		case "/meta/movie/tt1.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt1", "type": "movie", "name": "Linked", ` + links + `}}`))
		case "/meta/movie/tt2.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt2", "type": "movie", "name": "Listed", "cast": ["Cleo Actor"], ` + links + `}}`))
		case "/meta/movie/tt3.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt3", "type": "movie", "name": "Odd", "links": {"name": "Ana Actor"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer addon.Close()
	client := NewClient("test")
	manifestURL := addon.URL + "/manifest.json"
	credited := func(what string, meta Meta) {
		t.Helper()
		if !slices.Equal(meta.Cast, Names{"Ana Actor", "Ben Actor"}) || !slices.Equal(meta.Director, Names{"Eve Director"}) ||
			!slices.Equal(meta.Writer, Names{"Finn Writer", "Gus Writer"}) || meta.Genres != nil {
			t.Errorf("%s: cast %v, directors %v, writers %v, genres %v", what, meta.Cast, meta.Director, meta.Writer, meta.Genres)
		}
	}
	meta, err := client.Meta(t.Context(), manifestURL, "movie", "tt1", false)
	if err != nil || meta.Name != "Linked" {
		t.Fatalf("meta: %v %+v", err, meta)
	}
	credited("meta", meta)
	metas, err := client.Catalog(t.Context(), manifestURL, "movie", "top", nil, false)
	if err != nil || len(metas) != 1 {
		t.Fatalf("catalog: %v %+v", err, metas)
	}
	credited("catalog", metas[0])

	listed, err := client.Meta(t.Context(), manifestURL, "movie", "tt2", false)
	if err != nil || !slices.Equal(listed.Cast, Names{"Cleo Actor"}) || !slices.Equal(listed.Director, Names{"Eve Director"}) {
		t.Errorf("a list and links: %v cast %v, directors %v", err, listed.Cast, listed.Director)
	}
	odd, err := client.Meta(t.Context(), manifestURL, "movie", "tt3", false)
	if err != nil || odd.Name != "Odd" || odd.Cast != nil {
		t.Errorf("links that are no list: %v %+v", err, odd)
	}
}

// Season posters come in the order of the seasons, a missing one as null:
// an element that is not an image keeps its place, so that the others stay
// with their seasons. Keyed by season number instead, they keep their
// numbers, and a poster the addon also gives by number keeps that one.
func TestSeasonPostersKeepTheirPlaces(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/meta/series/tt1.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt1", "type": "series", "name": "Show",
				"app_extras": {"seasonPosters": ["https://images.example/1.jpg", null, 7, "https://images.example/4.jpg"]}}}`))
		case "/meta/series/tt2.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt2", "type": "series", "name": "Other",
				"app_extras": {"seasonPosters": "https://images.example/1.jpg", "certification": "TV-14",
					"certifications": {"US": "TV-14", "it": " VM14 ", "FR": "", "DE": 16, "GB": null}}}}`))
		case "/meta/series/tt4.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt4", "type": "series", "name": "Listed",
				"app_extras": {"certifications": ["VM14"], "certification": "TV-MA"}}}`))
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
	// Ratings by country are kept by upper-case code, without those that
	// are not texts; anything but an object is left out.
	if other.Extras != nil && !maps.Equal(other.Extras.Certifications, Certifications{"US": "TV-14", "IT": "VM14"}) {
		t.Errorf("certifications: %+v", other.Extras.Certifications)
	}
	listed, err := client.Meta(t.Context(), addon.URL+"/manifest.json", "series", "tt4", false)
	if err != nil || listed.Extras == nil || listed.Extras.Certifications != nil || listed.Extras.Certification != "TV-MA" {
		t.Errorf("certifications as a list: %v %+v", err, listed.Extras)
	}
	keyed, err := client.Meta(t.Context(), addon.URL+"/manifest.json", "series", "tt3", false)
	want := map[string]string{"0": "https://images.example/s0.jpg", "1": "https://images.example/one.jpg"}
	if err != nil || keyed.Extras == nil || keyed.Extras.SeasonPosters != nil || !maps.Equal(keyed.Extras.SeasonPosterByNumber, want) {
		t.Errorf("keyed by season number: %v %+v", err, keyed.Extras)
	}
}

// A cast member's TMDB identifier is the one an addon gives as tmdb_id,
// else a numeric id, as a number or a string; any other id names no one on
// TMDB.
func TestCastMembersTellTheirTMDBIdentifier(t *testing.T) {
	t.Parallel()
	var extras Extras
	if err := unmarshal([]byte(`{"cast": [{"name": "A", "tmdb_id": 11, "id": 99}, {"name": "B", "id": "12"},
		{"name": "C", "id": "nm0000001"}, {"name": "D"}]}`), &extras); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, member := range extras.Cast {
		got = append(got, member.PersonTMDB())
	}
	if want := []string{"11", "12", "", ""}; !slices.Equal(got, want) {
		t.Errorf("identifiers %q, want %q", got, want)
	}
}

// An addon that refuses a description answers with one of its own notices,
// of another identifier and another type: that is no description of the
// title. A description by another identifier, or without a type, is.
func TestANoticeDescribedAsATitleIsNoDescription(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/meta/other/library.item.1.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "addonerror.%7B%22errorDescription%22%3A%22Rate%20Limit%20Exceeded%22%7D",
				"name": "[x] Addon - Error", "description": "Rate Limit Exceeded", "type": "movie"}}`))
		case "/meta/other/library.item.2.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "library.item.2", "name": "Item", "type": "movie"}}`))
		case "/meta/movie/tmdb:1.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt1", "name": "By IMDb", "type": "movie"}}`))
		case "/meta/series/kitsu:1.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "kitsu:1:1", "name": "Untyped"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer addon.Close()
	client := NewClient("test")
	manifestURL := addon.URL + "/manifest.json"
	if meta, err := client.Meta(t.Context(), manifestURL, "other", "library.item.1", false); !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("a notice: %v %+v", err, meta)
	}
	for _, asked := range [][2]string{{"other", "library.item.2"}, {"movie", "tmdb:1"}, {"series", "kitsu:1"}} {
		if meta, err := client.Meta(t.Context(), manifestURL, asked[0], asked[1], false); err != nil || meta.Name == "" {
			t.Errorf("%s %s: %v %+v", asked[0], asked[1], err, meta)
		}
	}
}

// An addon refusing a description for its rate may describe its notice
// with the type asked for: it is no description either, and tells the
// refusal.
func TestARateLimitNoticeOfTheTypeAskedIsNoDescription(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"meta": {"id": "addonerror.%7B%22errorDescription%22%3A%22Rate%20Limit%20Exceeded%22%7D",
			"name": "[x] Addon - Error", "description": "Rate Limit Exceeded", "type": "movie"}}`))
	}))
	defer addon.Close()
	meta, err := NewClient("test").Meta(t.Context(), addon.URL+"/manifest.json", "movie", "tt1", false)
	if !errors.Is(err, ErrRateLimited) || !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("a rate-limit notice of a movie: %v %+v", err, meta)
	}
}

// A manifest URL with a query asks for each resource in place of
// /manifest.json, the query following, as Stremio does: a token kept
// there reaches every request.
func TestAManifestURLsQueryFollowsEachResource(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var asked []string
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Query().Get("token") != "abc" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"metas":[],"meta":{"id":"tt1","type":"movie","name":"A"},"streams":[],"subtitles":[]}`))
	}))
	defer addon.Close()
	manifest := addon.URL + "/cfg/manifest.json?token=abc"
	client := NewClient("test")
	if _, err := client.Catalog(t.Context(), manifest, "movie", "top", []ExtraValue{{Name: "skip", Value: "100"}}, false); err != nil {
		t.Errorf("catalog: %v", err)
	}
	if _, err := client.Meta(t.Context(), manifest, "movie", "tt1", false); err != nil {
		t.Errorf("meta: %v", err)
	}
	if _, err := client.Streams(t.Context(), manifest, "movie", "tt1", false); err != nil {
		t.Errorf("streams: %v", err)
	}
	if _, err := client.Subtitles(t.Context(), manifest, "movie", "tt1", nil, false); err != nil {
		t.Errorf("subtitles: %v", err)
	}
	want := []string{
		"/cfg/catalog/movie/top/skip=100.json?token=abc",
		"/cfg/meta/movie/tt1.json?token=abc",
		"/cfg/stream/movie/tt1.json?token=abc",
		"/cfg/subtitles/movie/tt1.json?token=abc",
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(asked, want) {
		t.Errorf("asked %q", asked)
	}
	if health, _ := client.Health(manifest); health.Requests != 4 || health.Failures != 0 {
		t.Errorf("health: %+v", health)
	}
}

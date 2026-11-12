package tmdb

import (
	"net/url"
	"slices"
	"testing"
)

// A studio's titles come from TMDB's discover, the most popular first,
// out by today and none for adults: movies by any of its companies,
// series by any of its networks, else by any of its companies. Each page
// is one request.
func TestDiscoverAsksByNetworksOrCompanies(t *testing.T) {
	t.Parallel()
	client, asked := creditsServer(t, map[string]any{
		"/3/discover/movie": map[string]any{"results": []any{
			map[string]any{"id": 501, "title": "North Light", "release_date": "2020-01-02", "poster_path": "/n.jpg", "vote_count": 30,
				"popularity": 9.5},
			map[string]any{"id": 502, "title": "Grown Only", "release_date": "2021-01-01", "adult": true},
			map[string]any{"id": 0, "title": "No Identifier"},
		}},
		"/3/discover/tv": map[string]any{"results": []any{map[string]any{"id": 601, "name": "Harbor Watch", "first_air_date": "2019-05-05"}}},
	})
	movies, err := client.Discover(t.Context(), Movie, []int{21}, []int{31, 32}, 2, "2026-11-12")
	if err != nil {
		t.Fatal(err)
	}
	want := []Discovered{{ID: 501, Name: "North Light", Date: "2020-01-02", PosterPath: "/n.jpg", Votes: 30, Popularity: 9.5},
		{ID: 502, Name: "Grown Only", Date: "2021-01-01", Adult: true}}
	if !slices.Equal(movies, want) {
		t.Errorf("movies %+v", movies)
	}
	series, err := client.Discover(t.Context(), TV, []int{21, 22}, []int{31}, 1, "2026-11-12")
	if err != nil || len(series) != 1 || series[0].Name != "Harbor Watch" || series[0].Date != "2019-05-05" {
		t.Errorf("series %+v %v", series, err)
	}
	if _, err := client.Discover(t.Context(), TV, nil, []int{31}, 1, "2026-11-12"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Discover(t.Context(), Movie, []int{21}, nil, 1, "2026-11-12"); err == nil {
		t.Error("movies were discovered without a company")
	}
	query := func(values url.Values) string { return values.Encode() }
	if got := asked(); !slices.Equal(got, []string{
		"/3/discover/movie?" + query(url.Values{"include_adult": {"false"}, "page": {"2"}, "primary_release_date.lte": {"2026-11-12"},
			"sort_by": {"popularity.desc"}, "with_companies": {"31|32"}}),
		"/3/discover/tv?" + query(url.Values{"first_air_date.lte": {"2026-11-12"}, "include_adult": {"false"}, "page": {"1"},
			"sort_by": {"popularity.desc"}, "with_networks": {"21|22"}}),
		"/3/discover/tv?" + query(url.Values{"first_air_date.lte": {"2026-11-12"}, "include_adult": {"false"}, "page": {"1"},
			"sort_by": {"popularity.desc"}, "with_companies": {"31"}}),
	}) {
		t.Errorf("TMDB was asked %q", got)
	}
}

package tmdb

import (
	"net/url"
	"slices"
	"testing"
)

// A title's streaming services come with its details, by country: those
// offering it by subscription, free, or free with ads, in that order, each
// once, the countries of none and the services of no name or identifier
// left out; those to rent or buy are not services.
func TestTitleDetailsKeepTheServicesOfSubscriptionsAndFreeOffers(t *testing.T) {
	t.Parallel()
	service := func(id int, name string) map[string]any {
		return map[string]any{"provider_id": id, "provider_name": name, "logo_path": "/l.png"}
	}
	client, _ := creditsServer(t, map[string]any{"/3/movie/1001": map[string]any{"watch/providers": map[string]any{"results": map[string]any{
		"fr": map[string]any{"flatrate": []any{service(8, " Stream One ")}, "free": []any{service(9, "Free Two"), service(0, "No Identifier")},
			"ads":  []any{service(10, "Ads Three"), service(8, "Stream One"), service(14, " ")},
			"rent": []any{service(11, "Rent Four")}, "buy": []any{service(12, "Buy Five")}},
		"US": map[string]any{"rent": []any{service(11, "Rent Four")}},
	}}}})
	details, err := client.TitleDetails(t.Context(), Movie, "1001", "fr")
	if err != nil {
		t.Fatal(err)
	}
	want := []StreamingService{{8, "Stream One", "/l.png"}, {9, "Free Two", "/l.png"}, {10, "Ads Three", "/l.png"}}
	if len(details.Services) != 1 || !slices.Equal(details.Services["FR"], want) {
		t.Errorf("services %+v", details.Services)
	}
}

// A service's titles come from TMDB's discover, in the country, offered by
// subscription, free or free with ads; without a service or a country,
// TMDB is asked nothing.
func TestDiscoverServicesAsksByServiceAndCountry(t *testing.T) {
	t.Parallel()
	client, asked := creditsServer(t, map[string]any{
		"/3/discover/tv": map[string]any{"results": []any{map[string]any{"id": 601, "name": "Harbor Watch", "first_air_date": "2019-05-05"}}},
	})
	series, err := client.DiscoverServices(t.Context(), TV, []int{8, 9}, "FR", 2, "2026-11-12")
	if err != nil || len(series) != 1 || series[0].Name != "Harbor Watch" {
		t.Errorf("series %+v %v", series, err)
	}
	if _, err := client.DiscoverServices(t.Context(), TV, nil, "FR", 1, "2026-11-12"); err == nil {
		t.Error("titles were discovered without a service")
	}
	if _, err := client.DiscoverServices(t.Context(), Movie, []int{8}, "", 1, "2026-11-12"); err == nil {
		t.Error("titles were discovered without a country")
	}
	if got := asked(); !slices.Equal(got, []string{
		"/3/discover/tv?" + url.Values{"first_air_date.lte": {"2026-11-12"}, "include_adult": {"false"}, "page": {"2"},
			"sort_by": {"popularity.desc"}, "watch_region": {"FR"}, "with_watch_monetization_types": {"flatrate|free|ads"},
			"with_watch_providers": {"8|9"}}.Encode(),
	}) {
		t.Errorf("TMDB was asked %q", got)
	}
}

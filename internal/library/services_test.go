package library

import (
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// offer is a streaming service of TMDB's watch providers.
func offer(id int, name string) map[string]any {
	return map[string]any{"provider_id": id, "provider_name": name, "logo_path": "/s.png", "display_priority": id}
}

// offers gives the TMDB answer of path, a movie's or series' details,
// these watch providers, by country then kind of offer.
func (f *fakeCreditsTMDB) offers(path string, byCountry map[string]map[string][]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	answer := f.answers[path].(map[string]any)
	results := map[string]any{}
	for country, kinds := range byCountry {
		results[country] = kinds
	}
	answer["watch/providers"] = map[string]any{"results": results}
}

// keptAsked returns when a title's credits were last read, by its key.
func (e creditsEnv) keptAsked(key creditsKey) time.Time {
	e.t.Helper()
	kept, err := e.service.readTitleCredits(e.t.Context(), []creditsKey{key})
	if err != nil {
		e.t.Fatal(err)
	}
	return kept[key].asked
}

// The pass reads, in the background, with a title's credits, the
// streaming services that offer it in the rating country by subscription,
// free or free with ads, those to rent or buy left out, each once: the
// title carries them as its tags. Kept 30 days, they are not read again
// sooner; another rating country hides them until they are read for it.
func TestThePassKeepsTheStreamingServicesOfTheRatingCountry(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	var elapsed atomic.Int64
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	e.setting(func(s *accounts.Settings) { s.RatingCountry = "FR" })
	e.tmdb.offers("/movie/1001", map[string]map[string][]any{
		"FR": {"flatrate": {offer(8, "Stream One")}, "free": {offer(9, " Free Two ")}, "ads": {offer(10, "Ads Three"), offer(8, "Stream One")},
			"rent": {offer(11, "Rent Four")}, "buy": {offer(12, "Buy Five")}},
		"US": {"flatrate": {offer(13, "Other Six")}},
	})
	e.service.UseTitleCredits(e.queue)
	if err := e.service.creditsPass(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.settle()
	harbor := e.titles["Harbor Lights"]
	item, err := e.service.Item(t.Context(), e.admin, harbor)
	if err != nil || !slices.Equal(item.Tags, []string{"Stream One", "Free Two", "Ads Three"}) {
		t.Fatalf("Harbor Lights in France: %q %v", item.Tags, err)
	}
	asked := e.tmdb.requests("/3/movie/1001?")
	if len(asked) == 0 || !strings.Contains(asked[0], "append_to_response="+url.QueryEscape("credits,watch/providers")) {
		t.Errorf("TMDB was asked %v", asked)
	}
	var logo string
	if err := e.service.db.QueryRow(t.Context(), `SELECT logo_path FROM tmdb_services WHERE tmdb_id = 9`).Scan(&logo); err != nil ||
		logo != "/s.png" {
		t.Errorf("the service kept: %q %v", logo, err)
	}
	// A title TMDB names no service of has none.
	if item, err := e.service.Item(t.Context(), e.admin, e.titles["Quiet Pier"]); err != nil || item.Tags != nil {
		t.Errorf("Quiet Pier: %q %v", item.Tags, err)
	}

	// Within 30 days, a pass reads nothing again.
	key := creditsKey{kind: "movie", provider: "imdb", id: "tt0882001"}
	first := e.keptAsked(key)
	elapsed.Store(int64(29 * 24 * time.Hour))
	if err := e.service.creditsPass(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if again := e.keptAsked(key); !again.Equal(first) {
		t.Errorf("read again within 30 days: %v, then %v", first, again)
	}
	elapsed.Store(int64(31 * 24 * time.Hour))
	if err := e.service.creditsPass(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if again := e.keptAsked(key); !again.After(first) {
		t.Errorf("not read again after 30 days: %v, then %v", first, again)
	}

	// In the US, they are hidden until read again, then the US's show.
	e.setting(func(s *accounts.Settings) { s.RatingCountry = "US" })
	if item, err := e.service.Item(t.Context(), e.admin, harbor); err != nil || item.Tags != nil {
		t.Errorf("in the US, before they are read again: %q %v", item.Tags, err)
	}
	if err := e.service.creditsPass(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if item, err := e.service.Item(t.Context(), e.admin, harbor); err != nil || !slices.Equal(item.Tags, []string{"Other Six"}) {
		t.Errorf("in the US: %q %v", item.Tags, err)
	}
}

// An administrator's tags win over TMDB's services; the editor's save of
// TMDB's unchanged saves no edit.
func TestEditedTagsWinOverStreamingServices(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.tmdb.offers("/movie/1002", map[string]map[string][]any{"US": {"flatrate": {offer(8, "Stream One")}}})
	id := e.titles["Quiet Pier"]
	if item := e.open(e.admin, "Quiet Pier"); !slices.Equal(item.Tags, []string{"Stream One"}) {
		t.Fatalf("Quiet Pier: %q", item.Tags)
	}
	base, err := e.service.Original(t.Context(), e.admin, id)
	if err != nil || !slices.Equal(base.Tags, []string{"Stream One"}) {
		t.Fatalf("the original: %q %v", base.Tags, err)
	}
	if err := e.service.SaveOverrides(t.Context(), base, Overrides{Tags: []string{"Stream One"}}); err != nil {
		t.Fatal(err)
	}
	if saved := e.service.overridesNow().fields[id]; saved.Tags != nil {
		t.Errorf("an unchanged save kept %q", saved.Tags)
	}
	if err := e.service.SaveOverrides(t.Context(), base, Overrides{Tags: []string{"Mine"}}); err != nil {
		t.Fatal(err)
	}
	if item, err := e.service.Item(t.Context(), e.admin, id); err != nil || !slices.Equal(item.Tags, []string{"Mine"}) {
		t.Errorf("edited: %q %v", item.Tags, err)
	}
	if shown := e.service.TagsShown(t.Context(), []Item{{ID: id, Kind: KindMovie, ProviderIDs: map[string]string{"Imdb": "tt0882002"}}}); !slices.Equal(shown[0], []string{"Mine"}) {
		t.Errorf("the tags shown: %q", shown)
	}
}

// A service's page lists the titles its catalogs list for its name, then
// those whose kept services name it, then TMDB's most popular titles it
// offers in the rating country.
func TestAServicePageListsCatalogsThenKeptThenDiscover(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.setting(func(s *accounts.Settings) { s.RatingCountry = "DE" })
	on := map[string]map[string][]any{"DE": {"flatrate": {offer(8, "Stream One")}}}
	e.tmdb.offers("/movie/1001", on)
	e.tmdb.offers("/movie/1012", on)
	e.tmdb.offers("/tv/2031", on)
	e.tmdb.offers("/movie/1002", map[string]map[string][]any{"US": {"flatrate": {offer(8, "Stream One")}}})
	for _, name := range []string{"Harbor Lights", "Kelp Bay", "Quiet Pier", "Tidewater"} {
		e.open(e.admin, name)
	}
	e.settle()
	picks := &fakeAddon{
		manifest: stremio.Manifest{ID: "picks", Name: "Picks", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "picks", Name: "Picks", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Stream One"}}}}}},
		catalogs: map[string][]stremio.Meta{"movie/picks": {{ID: "pick:1", Type: "movie", Name: "Catalog Pick", Genres: stremio.Names{"Stream One"}}}},
	}
	e.install(addons.Shared(), picks)
	e.tmdb.mu.Lock()
	e.tmdb.answers["/discover/movie?page=1"] = map[string]any{"results": []any{
		discovered("movie", 1001, "Harbor Lights", "2011-01-01", 90),
		discovered("movie", 1301, "Discovered One", "2015-01-01", 50),
	}}
	e.tmdb.answers["/discover/movie?page=2"] = map[string]any{"results": []any{}}
	e.tmdb.answers["/discover/tv?page=1"] = map[string]any{"results": []any{discovered("tv", 2301, "Discovered Show", "2017-01-01", 40)}}
	e.tmdb.answers["/discover/tv?page=2"] = map[string]any{"results": []any{}}
	e.tmdb.mu.Unlock()

	page, err := e.service.ServiceTitles(t.Context(), e.admin, []string{"stream one"}, func(option string) bool { return strings.EqualFold(option, "stream one") },
		[]Kind{KindMovie, KindSeries}, 0, 100)
	want := []string{"Catalog Pick", "Harbor Lights", "Kelp Bay", "Tidewater", "Discovered One", "Discovered Show"}
	if err != nil || !slices.Equal(names(page.Items), want) {
		t.Fatalf("Stream One's page:\n got %v %v\nwant %v", names(page.Items), err, want)
	}
	asked := e.tmdb.requests("/3/discover/movie?")
	if len(asked) != 2 || !strings.Contains(asked[0], "watch_region=DE&with_watch_monetization_types="+url.QueryEscape("flatrate|free|ads")+
		"&with_watch_providers=8") {
		t.Errorf("movies asked %v", asked)
	}

	// Without a key, it lists its catalogs' and kept titles only.
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	page, err = e.service.ServiceTitles(t.Context(), e.admin, []string{"Stream One"}, func(string) bool { return false }, []Kind{KindMovie}, 0, 100)
	if err != nil || !slices.Equal(names(page.Items), []string{"Harbor Lights", "Kelp Bay"}) {
		t.Errorf("without a key: %v %v", names(page.Items), err)
	}
}

// A smart list's service rule knows a title's services once they are kept
// for the rating country, and has those not kept read in the background;
// without a key, or for a title TMDB does not know, a title has none.
func TestServiceRulesKnowTheServicesOnceRead(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.tmdb.offers("/movie/1001", map[string]map[string][]any{"US": {"free": {offer(9, "Free Two")}}})
	e.service.UseTitleCredits(e.queue)
	items, err := e.service.Items(t.Context(), e.admin, []accounts.ID{e.titles["Harbor Lights"], e.titles["Quiet Pier"]})
	if err != nil || len(items) != 2 {
		t.Fatalf("%d items: %v", len(items), err)
	}
	items = append(items, Item{ID: e.titles["Harbor Lights"], Kind: KindMovie, ProviderIDs: map[string]string{"Imdb": "tt0999999"}})
	services, known, err := e.service.keptServices(t.Context(), items)
	if err != nil || known[0] || known[1] || known[2] {
		t.Fatalf("before they are read: %q %v %v", services, known, err)
	}
	e.settle()
	services, known, err = e.service.keptServices(t.Context(), items)
	if err != nil || !known[0] || !slices.Equal(services[0], []string{"Free Two"}) || !known[1] || len(services[1]) != 0 ||
		!known[2] || services[2] != nil {
		t.Errorf("once read: %q %v %v", services, known, err)
	}
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	if services, known, err := e.service.keptServices(t.Context(), items); err != nil || !known[0] || services[0] != nil {
		t.Errorf("without a key: %q %v %v", services, known, err)
	}
}

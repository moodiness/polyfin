package library

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// cinemaAddon serves Popular, a movie catalog that offers a genre filter,
// and Shows, a series catalog that offers none, in pages of 2. Their
// listings give genres, years and IMDb ratings; the language comes only
// with a title's description.
func cinemaAddon() *fakeAddon {
	type title struct {
		id, kind, name, year, rating, language string
		genres                                 []string
	}
	all := []title{
		{"tt01", "movie", "Le Dîner", "1998", "7.7", "fr", []string{"Comedy"}},
		{"tt02", "movie", "La Haine", "1995", "8.1", "French", []string{"Drama"}},
		{"tt03", "movie", "Amélie", "2001", "8.3", "fr", []string{"Comedy", "Romance"}},
		{"tt04", "movie", "Heat", "1995", "8.3", "en", []string{"Action", "Drama"}},
		{"tt05", "movie", "Les Visiteurs", "1993", "7.0", "fr", []string{"Comedy"}},
		{"tt06", "series", "Kaamelott", "2005", "8.6", "fr", []string{"Comedy"}},
		{"tt07", "series", "Friends", "1994", "8.9", "en", []string{"Comedy"}},
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "cinema", Name: "Cinema", Version: "1", Types: []string{"movie", "series"},
			IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "popular", Name: "Popular", Extra: []stremio.Extra{{Name: "skip"},
					{Name: "genre", Options: []string{"Action", "Comedy", "Drama", "Romance"}}}},
				{Type: "series", ID: "shows", Name: "Shows", Extra: []stremio.Extra{{Name: "skip"}}},
			}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
		pageSize: 2,
	}
	for _, t := range all {
		row := stremio.Meta{ID: t.id, Type: t.kind, Name: t.name, ReleaseInfo: stremio.Text(t.year), ImdbRating: stremio.Text(t.rating),
			Genres: t.genres}
		catalog := "movie/popular"
		if t.kind == "series" {
			catalog = "series/shows"
		}
		addon.catalogs[catalog] = append(addon.catalogs[catalog], row)
		full := row
		full.Language = t.language
		addon.metas[t.kind+"/"+t.id] = full
	}
	return addon
}

// smartEnv installs cinemaAddon for the server and returns it with the
// identifier of the addon.
func smartEnv(t *testing.T) (env, *fakeAddon, accounts.ID) {
	t.Helper()
	e := newEnv(t)
	addon := cinemaAddon()
	e.install(addons.Shared(), addon)
	list, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(list) != 1 {
		t.Fatalf("addons: %v %v", list, err)
	}
	return e, addon, list[0].ID
}

// group makes a group of rules.
func group(match string, rules ...smartlists.Rule) smartlists.Rule {
	return smartlists.Rule{Field: smartlists.FieldGroup, Match: match, Rules: rules}
}

// makeList saves a list of the server's, or of owner's own, drawing from
// the catalogs of addon given as "type/id".
func (e env) makeList(owner *accounts.ID, name string, addon accounts.ID, catalogs []string, rules smartlists.Rule, sort string, most int) smartlists.List {
	e.t.Helper()
	list := smartlists.List{Owner: owner, Name: name, ShownAs: smartlists.ShownAsLibrary, Rules: rules, Sort: sort, MaxItems: most}
	for _, c := range catalogs {
		kind, id, _ := strings.Cut(c, "/")
		list.Catalogs = append(list.Catalogs, smartlists.Catalog{AddonID: addon, Type: kind, ID: id})
	}
	saved, err := e.service.smart.Create(e.t.Context(), list)
	if err != nil {
		e.t.Fatal(err)
	}
	return saved
}

// listing lists a smart list for user.
func (e env) listing(user accounts.User, list smartlists.List) Page {
	e.t.Helper()
	page, err := e.service.Children(e.t.Context(), user, SmartListItemID(list.ID), 0, 100, "")
	if err != nil {
		e.t.Fatal(err)
	}
	return page
}

// settled lists a smart list for user until no title waits for its
// description, as the descriptions read in the background come.
func (e env) settled(user accounts.User, list smartlists.List) []string {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		preview, err := e.service.PreviewSmartList(e.t.Context(), user, list)
		if err != nil {
			e.t.Fatal(err)
		}
		if preview.Unknown == 0 {
			return names(e.listing(user, list).Items)
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("%d titles still wait for their description", preview.Unknown)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestASmartListOfFrenchMoviesOfThe1990sNotPlayedLosesAMovieOncePlayed(t *testing.T) {
	t.Parallel()
	e, _, addon := smartEnv(t)
	from, to := 1990.0, 1999.0
	list := e.makeList(nil, "French nineties", addon, []string{"movie/popular", "series/shows"}, group(smartlists.MatchAll,
		smartlists.Rule{Field: smartlists.FieldType, Value: "movie"},
		smartlists.Rule{Field: smartlists.FieldLanguage, Value: "fr"},
		smartlists.Rule{Field: smartlists.FieldYear, Min: &from, Max: &to},
		smartlists.Rule{Field: smartlists.FieldState, Value: smartlists.StateUnplayed},
	), smartlists.SortName, 0)

	if got := e.settled(e.member, list); !slices.Equal(got, []string{"La Haine", "Le Dîner", "Les Visiteurs"}) {
		t.Fatalf("listing: %v", got)
	}
	page := e.listing(e.member, list)
	if _, err := e.service.userData.Change(t.Context(), e.member.ID, []userdata.Item{{ID: page.Items[0].ID}},
		func(d *userdata.Data) { d.Played = true }); err != nil {
		t.Fatal(err)
	}
	if got := names(e.listing(e.member, list).Items); !slices.Equal(got, []string{"Le Dîner", "Les Visiteurs"}) {
		t.Errorf("once La Haine is played: %v", got)
	}
	// Another user did not play it.
	if got := names(e.listing(e.admin, list).Items); len(got) != 3 {
		t.Errorf("another user's listing: %v", got)
	}
}

func TestASmartListLeavesOutATitleUntilItsLanguageIsKnown(t *testing.T) {
	t.Parallel()
	e, addon, id := smartEnv(t)
	gate := make(chan struct{})
	addon.mu.Lock()
	addon.metaGate = gate
	addon.mu.Unlock()
	list := e.makeList(nil, "French", id, []string{"movie/popular"},
		group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldLanguage, Value: "fre"}), smartlists.SortCatalog, 0)

	// No description is known yet: nothing matches, every movie waits.
	preview, err := e.service.PreviewSmartList(t.Context(), e.member, list)
	if err != nil || len(preview.Titles) != 0 || preview.Unknown != 5 {
		t.Fatalf("before the descriptions: %v titles, %d unknown, %v", names(preview.Titles), preview.Unknown, err)
	}
	if page := e.listing(e.member, list); len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("listing before the descriptions: %v of %d", names(page.Items), page.Total)
	}
	// Descriptions read in the background let the list grow.
	close(gate)
	if got := e.settled(e.member, list); !slices.Equal(got, []string{"Le Dîner", "La Haine", "Amélie", "Les Visiteurs"}) {
		t.Errorf("once described: %v", got)
	}
}

func TestASmartListMatchesAnyRuleOfAGroup(t *testing.T) {
	t.Parallel()
	e, _, addon := smartEnv(t)
	list := e.makeList(nil, "Action or romance", addon, []string{"movie/popular", "series/shows"}, group(smartlists.MatchAll,
		group(smartlists.MatchAny,
			smartlists.Rule{Field: smartlists.FieldGenre, Value: "action"},
			smartlists.Rule{Field: smartlists.FieldGenre, Value: "Romance"}),
		smartlists.Rule{Field: smartlists.FieldYear, Not: true, Max: new(1994.0)},
	), smartlists.SortName, 0)
	if got := names(e.listing(e.member, list).Items); !slices.Equal(got, []string{"Amélie", "Heat"}) {
		t.Errorf("listing: %v", got)
	}
}

// A list sorts its titles and keeps its maximum; a genre the rules
// require is asked of a catalog that offers it, while a catalog that does
// not is read whole.
func TestASmartListSortsKeepsItsMaximumAndAsksForItsGenre(t *testing.T) {
	t.Parallel()
	e, addon, id := smartEnv(t)
	list := e.makeList(nil, "Best comedies", id, []string{"movie/popular", "series/shows"},
		group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldGenre, Value: "comedy"}), smartlists.SortRating, 3)
	page := e.listing(e.member, list)
	if got := names(page.Items); !slices.Equal(got, []string{"Friends", "Kaamelott", "Amélie"}) || page.Total != 3 {
		t.Errorf("listing: %v of %d", got, page.Total)
	}
	for _, request := range addon.catalogRequests() {
		if strings.HasPrefix(request, "catalog/movie/popular") && !strings.Contains(request, "genre=Comedy") ||
			strings.HasPrefix(request, "catalog/series/shows") && strings.Contains(request, "genre") {
			t.Errorf("catalog request: %s", request)
		}
	}
	// A library without type, as it holds movies and series.
	item, err := e.service.Item(t.Context(), e.member, SmartListItemID(list.ID))
	if err != nil || item.Kind != KindLibrary || item.CollectionType != "" || item.Name != "Best comedies" {
		t.Errorf("item: %+v %v", item, err)
	}

	// By premiere date, the latest first; among the first two of each
	// catalog.
	ranked := e.makeList(nil, "Latest first", id, []string{"movie/popular", "series/shows"},
		group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldRank, Max: new(2.0)}), smartlists.SortPremiere, 0)
	if got := names(e.listing(e.member, ranked).Items); !slices.Equal(got, []string{"Kaamelott", "Le Dîner", "La Haine", "Friends"}) {
		t.Errorf("by premiere date: %v", got)
	}
}

func TestAUsersOwnSmartListIsTheirsAlone(t *testing.T) {
	t.Parallel()
	e, _, addon := smartEnv(t)
	own := e.makeList(&e.member.ID, "Mine", addon, []string{"movie/popular"}, group(smartlists.MatchAll), smartlists.SortCatalog, 0)
	shared := e.makeList(nil, "Everyone's", addon, []string{"series/shows"}, group(smartlists.MatchAll), smartlists.SortCatalog, 0)
	seen := func(user accounts.User) []string {
		t.Helper()
		libraries, err := e.service.Libraries(t.Context(), user)
		if err != nil {
			t.Fatal(err)
		}
		return names(libraries)
	}
	if got := seen(e.member); !slices.Contains(got, "Mine") || !slices.Contains(got, "Everyone's") {
		t.Errorf("the member's libraries: %v", got)
	}
	if got := seen(e.admin); slices.Contains(got, "Mine") || !slices.Contains(got, "Everyone's") {
		t.Errorf("another user's libraries: %v", got)
	}
	if _, err := e.service.Children(t.Context(), e.admin, SmartListItemID(own.ID), 0, 10, ""); err == nil {
		t.Error("another user lists the member's own list")
	}
	// A series library, as it holds series only.
	if item, err := e.service.Item(t.Context(), e.admin, SmartListItemID(shared.ID)); err != nil || item.CollectionType != "tvshows" {
		t.Errorf("the server's list: %+v %v", item, err)
	}
}

package jellyfin

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// siloRated serves Silo's API on a server with the rated addon: Allowed
// (PG) and Restricted (R) dramas crediting Ann Lee, and Show, a series.
type siloRated struct {
	siloApps
	member         accounts.User
	token, profile string
	jellyfin       string
	ids            map[string]string
}

func newSiloRated(t *testing.T) siloRated {
	t.Helper()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), ratingsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	allowSilo(t, s)
	r := siloRated{siloApps: servingSilo(t, s), member: member, jellyfin: s.signIn("member", "tv"), ids: map[string]string{}}
	r.token, r.profile = r.signIn(t, "member")
	var views QueryResult
	s.get(t, "/UserViews", r.jellyfin, &views)
	for _, view := range views.Items {
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, r.jellyfin, &page)
		for _, item := range page.Items {
			r.ids[item.Name] = item.Id
		}
	}
	if r.ids["Allowed"] == "" || r.ids["Restricted"] == "" {
		t.Fatalf("titles: %v", r.ids)
	}
	return r
}

func (r siloRated) status(t *testing.T, method, path string) int {
	t.Helper()
	status, _ := r.do(t, method, path, r.token, r.profile, nil)
	return status
}

func (r siloRated) titles(t *testing.T, path string) []string {
	t.Helper()
	var page siloPage
	r.get(t, path, r.token, r.profile, &page)
	return cardTitles(page.Items)
}

// Favorites and the watchlist go both ways between Silo's apps and the
// Jellyfin API: what the app adds shows in Jellyfin apps, its state reads
// back, and taking it off takes it off.
func TestSiloAppsFavoritesAndWatchlistRoundTrip(t *testing.T) {
	t.Parallel()
	r := newSiloRated(t)
	allowed := r.ids["Allowed"]

	if status := r.status(t, http.MethodPut, "/api/v2/favorites/"+allowed); status != http.StatusNoContent {
		t.Fatalf("favorite: %d", status)
	}
	var entry struct {
		ItemID string `json:"item_id"`
	}
	r.get(t, "/api/v2/favorites/"+allowed, r.token, r.profile, &entry)
	var item BaseItemDto
	r.testServer.get(t, "/Items/"+allowed, r.jellyfin, &item)
	if !item.UserData.IsFavorite {
		t.Errorf("not a favorite for Jellyfin apps: %+v", item.UserData)
	}
	if got := r.titles(t, "/api/v2/favorites"); !slices.Equal(got, []string{"Allowed"}) {
		t.Errorf("favorites: %v", got)
	}
	if got := r.titles(t, "/api/v2/catalog?source=favorites"); !slices.Equal(got, []string{"Allowed"}) {
		t.Errorf("favorites in the catalog: %v", got)
	}
	if status := r.status(t, http.MethodDelete, "/api/v2/favorites/"+allowed); status != http.StatusNoContent {
		t.Fatalf("unfavorite: %d", status)
	}
	if status := r.status(t, http.MethodGet, "/api/v2/favorites/"+allowed); status != http.StatusNotFound {
		t.Errorf("a favorite no longer: %d", status)
	}

	if status := r.status(t, http.MethodPut, "/api/v2/watchlist/"+allowed); status != http.StatusNoContent {
		t.Fatalf("watchlist: %d", status)
	}
	// Twice changes nothing.
	if status := r.status(t, http.MethodPut, "/api/v2/watchlist/"+allowed); status != http.StatusNoContent {
		t.Fatalf("watchlist again: %d", status)
	}
	r.get(t, "/api/v2/watchlist/"+allowed, r.token, r.profile, &entry)
	if got := r.titles(t, "/api/v2/watchlist"); !slices.Equal(got, []string{"Allowed"}) {
		t.Errorf("watchlist: %v", got)
	}
	if got := r.titles(t, "/api/v2/catalog?source=watchlist"); !slices.Equal(got, []string{"Allowed"}) {
		t.Errorf("watchlist in the catalog: %v", got)
	}
	var detail struct {
		UserState struct {
			InWatchlist bool `json:"in_watchlist"`
		} `json:"user_state"`
	}
	r.get(t, "/api/v2/catalog/items/"+allowed, r.token, r.profile, &detail)
	if !detail.UserState.InWatchlist {
		t.Errorf("the page does not tell the title is on the watchlist")
	}
	// Jellyfin apps see it in the user's Watchlist playlist.
	if names := watchlistNames(t, r.testServer, r.jellyfin); !slices.Equal(names, []string{"Allowed"}) {
		t.Errorf("the Watchlist playlist: %v", names)
	}
	if status := r.status(t, http.MethodDelete, "/api/v2/watchlist/"+allowed); status != http.StatusNoContent {
		t.Fatalf("off the watchlist: %d", status)
	}
	if status := r.status(t, http.MethodGet, "/api/v2/watchlist/"+allowed); status != http.StatusNotFound {
		t.Errorf("on the watchlist still: %d", status)
	}
}

// A person's page opens from the cast of a title, by the number the cast
// gives, with their titles.
func TestSiloAppsOpenAPersonFromACast(t *testing.T) {
	t.Parallel()
	r := newSiloRated(t)
	var detail struct {
		Cast []struct {
			Name     string `json:"name"`
			PersonID string `json:"person_id"`
		} `json:"cast"`
	}
	r.get(t, "/api/v2/catalog/items/"+r.ids["Restricted"], r.token, r.profile, &detail)
	person := ""
	for _, credit := range detail.Cast {
		if credit.Name == "Ann Lee" {
			person = credit.PersonID
		}
	}
	if person == "" {
		t.Fatalf("cast: %+v", detail.Cast)
	}
	var page struct {
		Name string `json:"name"`
	}
	r.get(t, "/api/v2/catalog/people/"+person, r.token, r.profile, &page)
	if page.Name != "Ann Lee" {
		t.Errorf("person: %+v", page)
	}
	credits := "/api/v2/catalog?source=person&person_id=" + person + "&sort=-year"
	if got := r.titles(t, credits); !slices.Contains(got, "Allowed") || !slices.Contains(got, "Restricted") {
		t.Errorf("Ann Lee's titles: %v", got)
	}
	var people struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	r.get(t, "/api/v2/catalog/people?q=Ann&media_scope=video", r.token, r.profile, &people)
	if len(people.Items) != 1 || people.Items[0].ID != person {
		t.Errorf("people search: %+v", people.Items)
	}
	// The Apple app searches people only when the server says so.
	var search struct {
		PeopleMediaScope bool `json:"people_media_scope"`
	}
	r.get(t, "/api/v2/catalog/search/capabilities", r.token, r.profile, &search)
	if !search.PeopleMediaScope {
		t.Errorf("people search not offered")
	}
	if status := r.status(t, http.MethodGet, "/api/v2/catalog/people/999999"); status != http.StatusNotFound {
		t.Errorf("unknown person: %d", status)
	}

	// Under parental control, Restricted leaves their titles.
	r.limit(t, r.member, func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	if got := r.titles(t, credits); slices.Contains(got, "Restricted") {
		t.Errorf("Ann Lee's titles under parental control: %v", got)
	}
}

// The lists Silo's apps gained keep to the user's limits: a title they
// hid, a genre they may not see and parental control keep titles out of
// favorites, the watchlist, a person's titles, similar titles, For You
// and the calendar.
func TestSiloAppsNewListsUnderTheUsersLimits(t *testing.T) {
	t.Parallel()
	r := newSiloRated(t)
	allowed, restricted := r.ids["Allowed"], r.ids["Restricted"]
	for _, id := range []string{allowed, restricted} {
		for _, list := range []string{"favorites", "watchlist"} {
			if status := r.status(t, http.MethodPut, "/api/v2/"+list+"/"+id); status != http.StatusNoContent {
				t.Fatalf("%s %s: %d", list, id, status)
			}
		}
	}
	var detail struct {
		Cast []struct {
			Name     string `json:"name"`
			PersonID string `json:"person_id"`
		} `json:"cast"`
	}
	r.get(t, "/api/v2/catalog/items/"+restricted, r.token, r.profile, &detail)
	person := detail.Cast[0].PersonID
	// Played, Allowed makes For You, with the titles similar to it.
	if status, _ := r.call(http.MethodPost, "/UserPlayedItems/"+allowed, app("tv", r.jellyfin), nil); status != http.StatusOK {
		t.Fatalf("played: %d", status)
	}
	lists := func() map[string][]string {
		t.Helper()
		got := map[string][]string{
			"favorites": r.titles(t, "/api/v2/favorites"),
			"watchlist": r.titles(t, "/api/v2/watchlist"),
			"person":    r.titles(t, "/api/v2/catalog?source=person&person_id="+person),
		}
		var similar siloPage
		r.get(t, "/api/v2/recommendations/similar/"+r.ids["Unrated"]+"?limit=10", r.token, r.profile, &similar)
		got["similar"] = cardTitles(similar.Items)
		var discover struct {
			Items []struct {
				Items []siloCard `json:"items"`
			} `json:"items"`
		}
		r.get(t, "/api/v2/recommendations/discover", r.token, r.profile, &discover)
		for _, row := range discover.Items {
			got["for you"] = append(got["for you"], cardTitles(row.Items)...)
		}
		return got
	}
	before := lists()
	for _, list := range []string{"favorites", "person"} {
		if !slices.Contains(before[list], "Restricted") {
			t.Fatalf("%s before any limit: %v", list, before[list])
		}
	}
	// Started titles stay on the watchlist, the latest added first.
	if !slices.Equal(before["watchlist"], []string{"Restricted", "Allowed"}) {
		t.Fatalf("watchlist before any limit: %v", before["watchlist"])
	}
	check := func(label string) {
		t.Helper()
		for list, titles := range lists() {
			if slices.Contains(titles, "Restricted") {
				t.Errorf("%s: %s lists Restricted: %v", label, list, titles)
			}
		}
	}
	if status, _ := r.call(http.MethodPost, "/Polyfin/UserItems/"+restricted+"/Hidden", app("tv", r.jellyfin), nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	check("hidden")
	if status, _ := r.call(http.MethodDelete, "/Polyfin/UserItems/"+restricted+"/Hidden", app("tv", r.jellyfin), nil); status != http.StatusNoContent {
		t.Fatalf("showing: %d", status)
	}
	r.limit(t, r.member, func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"Crime"} })
	check("Crime blocked")
	r.limit(t, r.member, func(c *accounts.UserChanges) {
		c.BlockedGenres = &[]string{}
		c.Parental = &accounts.ParentalControl{MaxRating: new(13)}
	})
	check("parental control")
}

// The calendar lists the coming episodes of the series the user follows,
// on their air dates, but those of a series they hid.
func TestSiloAppsCalendar(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), airingAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "tonight"},
	}); err != nil {
		t.Fatal(err)
	}
	allowSilo(t, s)
	a := servingSilo(t, s)
	token, profile := a.signIn(t, "member")
	jellyfinToken := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", jellyfinToken, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, jellyfinToken, &page)
	series := map[string]string{}
	for _, show := range page.Items {
		series[show.Name] = show.Id
	}
	if status, _ := a.do(t, http.MethodPut, "/api/v2/favorites/"+series["Charlie"], token, profile, nil); status != http.StatusNoContent {
		t.Fatalf("favorite: %d", status)
	}
	today := time.Now().UTC()
	window := "/api/v2/calendar?start=" + today.Format(time.DateOnly) + "&end=" + today.AddDate(0, 0, 6).Format(time.DateOnly) + "&timezone=Europe/Paris"
	events := func(path string) []string {
		t.Helper()
		var calendar struct {
			Events []struct {
				Items []struct {
					EpisodeTitle string `json:"episode_title"`
				} `json:"items"`
			} `json:"events"`
		}
		a.get(t, path, token, profile, &calendar)
		var titles []string
		for _, day := range calendar.Events {
			for _, event := range day.Items {
				titles = append(titles, event.EpisodeTitle)
			}
		}
		return titles
	}
	if got := events(window + "&filter=following"); !slices.Equal(got, []string{"Charlie tomorrow"}) {
		t.Errorf("following: %v", got)
	}
	if got := events(window + "&filter=trending"); len(got) != 0 {
		t.Errorf("trending: %v", got)
	}
	if status, _ := a.do(t, http.MethodGet, "/api/v2/calendar?start=2026-01-01&end=2026-03-01", token, profile, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a window too long: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Polyfin/UserItems/"+series["Charlie"]+"/Hidden", app("tv", jellyfinToken), nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	if got := events(window); len(got) != 0 {
		t.Errorf("with the series hidden: %v", got)
	}
}

// watchlistNames lists the names of the entries of the user's Watchlist
// playlist, as Jellyfin apps list it; none while the user has none.
func watchlistNames(t *testing.T, s testServer, token string) []string {
	t.Helper()
	var held struct{ PlaylistId string }
	if status := s.get(t, "/Polyfin/UserItems/Watchlist", token, &held); status != http.StatusOK {
		t.Fatalf("watchlist: %d", status)
	}
	if held.PlaylistId == "" {
		return nil
	}
	var listed QueryResult
	if status := s.get(t, "/Items?ParentId="+held.PlaylistId, token, &listed); status != http.StatusOK {
		t.Fatalf("the Watchlist playlist: %d", status)
	}
	return itemNames(listed.Items)
}

// Silo's watchlist button acts as the web player's menu: a series puts
// its aired episodes of regular seasons and shows as on the watchlist
// while one of them is, a season the same for its own episodes, an
// episode goes on and off by itself, and Remove on a season or a series
// takes all its entries off. A series added before any episode aired
// shows as on it, and goes off. The calendar's watchlist follows the
// series of its episodes and those pending. Once the user deleted the
// playlist, the watchlist is empty, and the button makes it again.
func TestSiloAppsWatchlistSeriesAndEpisodes(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), airingAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "tonight"},
	}); err != nil {
		t.Fatal(err)
	}
	allowSilo(t, s)
	a := servingSilo(t, s)
	token, profile := a.signIn(t, "member")
	jellyfinToken := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", jellyfinToken, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, jellyfinToken, &page)
	ids := map[string]string{}
	for _, show := range page.Items {
		ids[show.Name] = show.Id
	}
	bravo := ids["Bravo"]
	var episodes, seasons QueryResult
	s.get(t, "/Shows/"+bravo+"/Episodes", jellyfinToken, &episodes)
	s.get(t, "/Shows/"+bravo+"/Seasons", jellyfinToken, &seasons)
	for _, item := range append(episodes.Items, seasons.Items...) {
		ids[item.Name] = item.Id
	}
	season, specials := "", ""
	for _, item := range seasons.Items {
		switch {
		case item.IndexNumber != nil && *item.IndexNumber == 1:
			season = item.Id
		case item.IndexNumber != nil && *item.IndexNumber == 0:
			specials = item.Id
		}
	}
	if bravo == "" || ids["Bravo hours ago"] == "" || season == "" {
		t.Fatalf("titles: %v", ids)
	}
	member := func(id string) bool {
		t.Helper()
		var got struct {
			IsMember bool `json:"is_member"`
		}
		a.get(t, "/api/v2/watchlist/"+id+"/membership", token, profile, &got)
		return got.IsMember
	}
	mark := func(method, id string) {
		t.Helper()
		if status, data := a.do(t, method, "/api/v2/watchlist/"+id, token, profile, nil); status != http.StatusNoContent {
			t.Fatalf("%s %s: %d %s", method, id, status, data)
		}
	}
	titles := func(path string) []string {
		t.Helper()
		var page siloPage
		a.get(t, path, token, profile, &page)
		return cardTitles(page.Items)
	}
	aired := []string{"Bravo long ago", "Bravo two days ago", "Bravo hours ago"}

	if member(bravo) {
		t.Fatal("Bravo on an empty watchlist")
	}
	mark(http.MethodPut, bravo)
	if got := titles("/api/v2/watchlist"); !slices.Equal(got, aired) {
		t.Errorf("Bravo's episodes: %v", got)
	}
	if got := watchlistNames(t, s, jellyfinToken); !slices.Equal(got, aired) {
		t.Errorf("Bravo's episodes in Jellyfin apps: %v", got)
	}
	if !member(bravo) || !member(season) || !member(ids["Bravo hours ago"]) {
		t.Errorf("Bravo: %v, its season: %v, an episode of it: %v", member(bravo), member(season), member(ids["Bravo hours ago"]))
	}
	var detail struct {
		UserState struct {
			InWatchlist bool `json:"in_watchlist"`
		} `json:"user_state"`
	}
	a.get(t, "/api/v2/catalog/items/"+bravo, token, profile, &detail)
	if !detail.UserState.InWatchlist {
		t.Error("Bravo's page does not tell it is on the watchlist")
	}
	// The calendar lists the series the user follows: of Bravo and
	// Charlie, both favorites, the watchlist's lists Bravo only.
	charlie := ids["Charlie"]
	for _, show := range []string{bravo, charlie} {
		if status, _ := a.do(t, http.MethodPut, "/api/v2/favorites/"+show, token, profile, nil); status != http.StatusNoContent {
			t.Fatalf("favorite: %d", status)
		}
	}
	coming := func() []string {
		t.Helper()
		window := "/api/v2/calendar?filter=watchlist&start=" + time.Now().UTC().Format(time.DateOnly) + "&end=" + time.Now().UTC().AddDate(0, 0, 3).Format(time.DateOnly)
		var calendar struct {
			Events []struct {
				Items []struct {
					EpisodeTitle string `json:"episode_title"`
				} `json:"items"`
			} `json:"events"`
		}
		a.get(t, window, token, profile, &calendar)
		var titles []string
		for _, day := range calendar.Events {
			for _, event := range day.Items {
				titles = append(titles, event.EpisodeTitle)
			}
		}
		return titles
	}
	if got := coming(); !slices.Contains(got, "Bravo tomorrow") || slices.Contains(got, "Charlie tomorrow") {
		t.Errorf("the watchlist's calendar: %v", got)
	}
	// Charlie, none of whose episodes aired, is held pending: it shows as
	// on the watchlist, with no entry yet, and in its calendar.
	if member(charlie) {
		t.Error("Charlie on the watchlist before it was added")
	}
	mark(http.MethodPut, charlie)
	if !member(charlie) {
		t.Error("Charlie, added before any episode aired, is not on the watchlist")
	}
	if got := titles("/api/v2/watchlist"); !slices.Equal(got, aired) {
		t.Errorf("with Charlie pending: %v", got)
	}
	if got := coming(); !slices.Contains(got, "Bravo tomorrow") || !slices.Contains(got, "Charlie tomorrow") {
		t.Errorf("the watchlist's calendar with Charlie pending: %v", got)
	}
	mark(http.MethodDelete, charlie)
	if member(charlie) {
		t.Error("Charlie still on the watchlist once removed")
	}

	// An episode goes off by itself; its series stays while another
	// episode is on, and the episode comes back by itself.
	mark(http.MethodDelete, ids["Bravo hours ago"])
	if member(ids["Bravo hours ago"]) || !member(bravo) {
		t.Errorf("an episode taken off: it %v, Bravo %v", member(ids["Bravo hours ago"]), member(bravo))
	}
	if got := titles("/api/v2/watchlist"); !slices.Equal(got, aired[:2]) {
		t.Errorf("after an episode taken off: %v", got)
	}
	mark(http.MethodPut, ids["Bravo hours ago"])
	if got := titles("/api/v2/watchlist"); !slices.Equal(got, []string{"Bravo hours ago", "Bravo long ago", "Bravo two days ago"}) {
		t.Errorf("an episode put back: %v", got)
	}

	// Remove on the series takes all its entries off, the episode put
	// on by itself among them.
	mark(http.MethodDelete, bravo)
	if member(bravo) || member(season) || member(ids["Bravo long ago"]) {
		t.Error("Bravo still on the watchlist")
	}
	if got := titles("/api/v2/watchlist"); len(got) != 0 {
		t.Errorf("after Bravo taken off: %v", got)
	}

	// A season puts its own aired episodes, and takes them off.
	mark(http.MethodPut, season)
	if got := titles("/api/v2/watchlist"); !slices.Equal(got, aired) || !member(season) || !member(bravo) {
		t.Errorf("Bravo's season: %v", got)
	}
	if specials != "" && member(specials) {
		t.Error("Bravo's specials on the watchlist with its first season")
	}
	mark(http.MethodDelete, season)
	if got := titles("/api/v2/watchlist"); len(got) != 0 || member(season) || member(bravo) {
		t.Errorf("after Bravo's season taken off: %v", got)
	}

	// Deleted, the playlist leaves the watchlist empty; the button makes
	// it again.
	var held struct{ PlaylistId string }
	s.get(t, "/Polyfin/UserItems/Watchlist", jellyfinToken, &held)
	if status, _ := s.call(http.MethodDelete, "/Items/"+held.PlaylistId, app("tv", jellyfinToken), nil); status != http.StatusNoContent {
		t.Fatalf("deleting the playlist: %d", status)
	}
	if got := titles("/api/v2/watchlist"); len(got) != 0 {
		t.Errorf("a deleted watchlist: %v", got)
	}
	if got := titles("/api/v2/catalog?source=watchlist"); len(got) != 0 {
		t.Errorf("a deleted watchlist in the catalog: %v", got)
	}
	if member(bravo) {
		t.Error("Bravo on a deleted watchlist")
	}
	mark(http.MethodPut, ids["Bravo long ago"])
	if got := watchlistNames(t, s, jellyfinToken); !slices.Equal(got, []string{"Bravo long ago"}) {
		t.Errorf("the watchlist made again: %v", got)
	}
	if !member(bravo) {
		t.Error("Bravo not on the watchlist made again")
	}
}

// The documents the apps read at startup tell what Polyfin serves: the
// server's name, its artwork sizes and addresses, and every feature it
// does not serve as unsupported, which the apps hide; a shuffle plays a
// library.
func TestSiloAppsStartupProbes(t *testing.T) {
	t.Parallel()
	r := newSiloRated(t)
	r.setting(t, func(settings *accounts.Settings) {
		settings.ServerName = "Den"
		settings.PublicAddress = "https://media.example.org"
	})
	var branding struct {
		ServerName string `json:"server_name"`
	}
	r.get(t, "/api/v2/theme/branding", "", "", &branding)
	if branding.ServerName != "Den" {
		t.Errorf("branding: %+v", branding)
	}
	var connections struct {
		ServerID  string `json:"server_id"`
		Endpoints []struct {
			URL string `json:"url"`
		} `json:"endpoints"`
	}
	r.get(t, "/api/v2/system/connections", r.token, "", &connections)
	if connections.ServerID != testServerID || len(connections.Endpoints) != 1 || connections.Endpoints[0].URL != "https://media.example.org" {
		t.Errorf("connections: %+v", connections)
	}
	var images struct {
		State string `json:"state"`
	}
	r.get(t, "/api/v2/images/capabilities", r.token, r.profile, &images)
	var home map[string]any
	r.get(t, "/api/v2/home/capabilities", r.token, r.profile, &home)
	for _, path := range []string{"/api/v2/capabilities/downloads", "/api/v2/capabilities/metadata-ai", "/api/v2/diagnostics/capabilities",
		"/api/v2/devices/push/apple/capabilities", "/api/v2/playback/sessions/control/capabilities", "/api/v2/subtitles/providers/status",
		"/api/v2/subtitles/ai/status", "/api/v2/requests/status", "/api/v2/settings/contract/capabilities",
		"/api/v2/auth/external-sign-in/capabilities", "/api/v2/auth/oauth/capabilities"} {
		var doc map[string]any
		r.get(t, path, r.token, r.profile, &doc)
	}
	var providers struct {
		PasswordLogin bool `json:"password_login"`
	}
	r.get(t, "/api/v2/auth/providers", "", "", &providers)
	if !providers.PasswordLogin {
		t.Errorf("password sign-in not offered")
	}
	if status, data := r.do(t, http.MethodGet, "/api/v2/settings/values/effective", r.token, r.profile, nil); status != http.StatusNotImplemented ||
		problemCode(data) != "capability_unsupported" {
		t.Errorf("settings: %d %s", status, data)
	}

	var libraries struct {
		Items []siloLibrary `json:"items"`
	}
	r.get(t, "/api/v2/user/libraries", r.token, r.profile, &libraries)
	status, data := r.do(t, http.MethodPost, "/api/v2/shuffles", r.token, r.profile,
		map[string]any{"scope": map[string]string{"kind": "library", "id": libraries.Items[0].ID}})
	if status != http.StatusCreated {
		t.Fatalf("shuffle: %d %s", status, data)
	}
	var shuffle struct {
		ID      string   `json:"id"`
		Current siloCard `json:"current"`
		Next    siloCard `json:"next"`
	}
	if err := json.Unmarshal(data, &shuffle); err != nil || shuffle.Current.ContentID == "" || shuffle.Next.ContentID == "" {
		t.Fatalf("shuffle: %s", data)
	}
	status, data = r.do(t, http.MethodPost, "/api/v2/shuffles/"+shuffle.ID+"/advance", r.token, r.profile,
		map[string]string{"from_content_id": shuffle.Current.ContentID})
	if err := json.Unmarshal(data, &shuffle); status != http.StatusOK || err != nil || shuffle.Current.ContentID == "" {
		t.Errorf("advance: %d %s", status, data)
	}
	if status := r.status(t, http.MethodDelete, "/api/v2/shuffles/"+shuffle.ID); status != http.StatusNoContent {
		t.Errorf("stopping a shuffle: %d", status)
	}
	if status := r.status(t, http.MethodGet, "/api/v2/shuffles/"+shuffle.ID); status != http.StatusNotFound {
		t.Errorf("a stopped shuffle: %d", status)
	}
}

package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The tests of this file install a user's own addon while its owner is
// an administrator, as no test server is on a public address, which is
// all a member's own addon may reach, and make the owner a member before
// the others look. What is recorded of an addon follows who installed
// it, not what they may do.

// oneChannelAddon serves a live TV catalog of one channel, tv:one, under
// the names given.
func oneChannelAddon(t *testing.T, id, catalog, channel string) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channels := []stremio.Meta{{ID: "tv:one", Type: "tv", Name: channel, Poster: server.URL + "/logo.png"}}
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: id, Name: catalog, Version: "1", Types: []string{"tv"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "tv", ID: "channels", Name: catalog}}})
		case strings.HasPrefix(path, "/catalog/tv/channels"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": channels})
		case strings.HasPrefix(path, "/stream/tv/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Live", URL: server.URL + "/live/one.m3u8"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A channel is one record for every user, under the identifier its addon
// gives it. A user whose own addon lists a channel under the identifier
// of one of the server's, and who turned the server's sources off for
// themselves, lists their own channel, and leaves the server's as it is:
// the others still open it, read its item and play it.
func TestAUsersOwnAddonLeavesTheServersChannelAsItIs(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), oneChannelAddon(t, "shared.tv", "Server TV", "One"), false); err != nil {
		t.Fatal(err)
	}
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("other", nil)
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator, c.UseSharedAddons = new(true), new(false) })
	if _, err := s.addons.Install(t.Context(), addons.Personal(owner.ID), oneChannelAddon(t, "own.tv", "My TV", "Changed One"), false); err != nil {
		t.Fatal(err)
	}
	other, mine := s.signIn("other", "tv"), s.signIn("owner", "phone")
	channels := func(token string) []BaseItemDto {
		t.Helper()
		var list QueryResult
		if status := s.get(t, "/LiveTv/Channels", token, &list); status != http.StatusOK || len(list.Items) != 1 {
			t.Fatalf("channels: %d %v", status, itemNames(list.Items))
		}
		return list.Items
	}
	id := channels(other)[0].Id
	if id != libraryItemID("channel|tv:one").String() {
		t.Fatalf("the channel is %s, which is not derived from its identifier", id)
	}
	// look answers the statuses of the other user's channel, item and
	// PlaybackInfo, and the channel's name.
	look := func() ([3]int, string) {
		t.Helper()
		var channel BaseItemDto
		play, _ := s.call(http.MethodPost, "/Items/"+id+"/PlaybackInfo", app("tv", other), map[string]any{})
		return [3]int{s.get(t, "/LiveTv/Channels/"+id, other, &channel), s.get(t, "/Items/"+id, other, nil), play}, channel.Name
	}
	found := [3]int{http.StatusOK, http.StatusOK, http.StatusOK}
	if got, name := look(); got != found || name != "One" {
		t.Fatalf("the channel before: %v %q", got, name)
	}
	// The owner lists their channels: their own addon's, of the same
	// identifier. They are a member from here.
	if listed := channels(mine); listed[0].Name != "Changed One" || listed[0].Id != id {
		t.Fatalf("the owner's channels: %+v", listed)
	}
	if _, err := s.store.UpdateUser(t.Context(), owner.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if got, name := look(); got != found || name != "One" {
		t.Errorf("the channel once a user's own addon listed its identifier: %v %q", got, name)
	}
}

// manySeriesAddon serves the series given, listed in its catalog "shows"
// and each described with its videos.
func manySeriesAddon(t *testing.T, id, name string, prefixes []string, series ...stremio.Meta) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: id, Name: name, Version: "1", Types: []string{"series"},
				IDPrefixes: prefixes, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "shows", Name: name}}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			listed := make([]stremio.Meta, len(series))
			for i, meta := range series {
				meta.Videos = nil
				listed[i] = meta
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": listed})
		case strings.HasPrefix(r.URL.Path, "/meta/series/"):
			for _, meta := range series {
				if r.URL.Path == "/meta/series/"+meta.ID+".json" {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// An episode is one record for every user, under the identifier its
// description gives its video, and is found through the series its
// record names. A user's own addon that describes a series with a video
// carrying the identifier of an episode of one of the server's series
// moves nothing: neither as a series of its own, nor as its own
// description of another series the server's addons list, read by a user
// who turned the server's sources off for themselves. The others still
// open the episode and play it. Between the server's own addons, the
// series listed last has the episode, as before.
func TestAUsersOwnAddonMovesNoEpisodeOfTheServersSeries(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	pilot := stremio.Video{ID: "tt7000:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"}
	named := func(id, name string, videos ...stremio.Video) stremio.Meta {
		return stremio.Meta{ID: id, Type: "series", Name: name, Videos: videos}
	}
	add := func(scope addons.Scope, manifest string) {
		t.Helper()
		installed, err := s.addons.Install(t.Context(), scope, manifest, false)
		if err != nil {
			t.Fatal(err)
		}
		choices := []addons.LibraryChoice{{AddonID: installed.ID, CatalogType: "series", CatalogID: "shows"}}
		if scope.Owner == nil {
			before, err := s.addons.Libraries(t.Context(), scope)
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range before {
				if l.Enabled && l.AddonID != installed.ID {
					choices = append(choices, addons.LibraryChoice{AddonID: l.AddonID, CatalogType: l.Catalog.Type, CatalogID: l.Catalog.ID})
				}
			}
		}
		if _, err := s.addons.SetLibraries(t.Context(), scope, choices); err != nil {
			t.Fatal(err)
		}
	}
	add(addons.Shared(), manySeriesAddon(t, "shared.shows", "Server Shows", []string{"tt"},
		named("tt7000", "The Server's Show", pilot),
		named("tt8000", "Another Show", stremio.Video{ID: "tt8000:1:1", Title: "First", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"})))
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("other", nil)
	other := s.signIn("other", "tv")
	// episodes lists the episodes of the series named so in a library, as
	// the user of token.
	episodes := func(token, view, series string) []BaseItemDto {
		t.Helper()
		var page, listed QueryResult
		if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)[view], token, &page); status != http.StatusOK {
			t.Fatalf("the library %s: %d", view, status)
		}
		for _, item := range page.Items {
			if item.Name == series {
				if status := s.get(t, "/Shows/"+item.Id+"/Episodes", token, &listed); status != http.StatusOK {
					t.Fatalf("the episodes of %s: %d", series, status)
				}
				return listed.Items
			}
		}
		t.Fatalf("the library %s lists no %s: %v", view, series, itemNames(page.Items))
		return nil
	}
	listed := episodes(other, "Server Shows", "The Server's Show")
	target := library.EpisodeItemID("tt7000:1:1").String()
	if len(listed) != 1 || listed[0].Id != target {
		t.Fatalf("the series' episodes: %+v", listed)
	}
	episodes(other, "Server Shows", "Another Show")
	// held tells the series the episode's record names, and what opening
	// it and asking to play it answer the other user.
	held := func() (string, [2]int) {
		t.Helper()
		var series string
		if err := s.pool.QueryRow(t.Context(), "SELECT data->>'seriesId' FROM items WHERE id = $1", target).Scan(&series); err != nil {
			t.Fatal(err)
		}
		play, _ := s.call(http.MethodPost, "/Items/"+target+"/PlaybackInfo", app("tv", other), map[string]any{})
		return series, [2]int{s.get(t, "/Items/"+target, other, nil), play}
	}
	found := [2]int{http.StatusOK, http.StatusOK}
	if series, got := held(); series != "tt7000" || got != found {
		t.Fatalf("the episode before: of %s, %v", series, got)
	}

	// A user's own series, whose description names the episode.
	first := s.user("first", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	add(addons.Personal(first.ID), manySeriesAddon(t, "own.shows", "Mine", []string{"own:"},
		named("own:show", "Mine", stremio.Video{ID: "tt7000:1:1", Title: "Renamed", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"})))
	if own := episodes(s.signIn("first", "phone"), "Mine", "Mine"); len(own) != 1 || own[0].Id != target {
		t.Fatalf("the user's own series' episodes: %+v", own)
	}
	if series, got := held(); series != "tt7000" || got != found {
		t.Errorf("the episode once a user's own series named it: of %s, %v", series, got)
	}

	// A user's own description of one of the server's series, which names
	// the episode of another: they read their own addons only.
	second := s.user("second", func(c *accounts.UserChanges) { c.IsAdministrator, c.UseSharedAddons = new(true), new(false) })
	add(addons.Personal(second.ID), manySeriesAddon(t, "own.metadata", "Described", []string{"tt"},
		named("tt8000", "Another Show", stremio.Video{ID: "tt7000:1:1", Title: "Renamed", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"})))
	if own := episodes(s.signIn("second", "phone"), "Described", "Another Show"); len(own) != 1 || own[0].Id != target {
		t.Fatalf("the user's own description's episodes: %+v", own)
	}
	if series, got := held(); series != "tt7000" || got != found {
		t.Errorf("the episode once a user's own description of a server series named it: of %s, %v", series, got)
	}
	for _, user := range []accounts.User{first, second} {
		if _, err := s.store.UpdateUser(t.Context(), user.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if series, got := held(); series != "tt7000" || got != found {
		t.Errorf("the episode for the others: of %s, %v", series, got)
	}

	// Another of the server's addons describes a series with the same
	// video: the series listed last has it, and its users open it.
	add(addons.Shared(), manySeriesAddon(t, "shared.more", "More Shows", []string{"tt"}, named("tt9000", "A Third Show", pilot)))
	if again := episodes(other, "More Shows", "A Third Show"); len(again) != 1 || again[0].Id != target {
		t.Fatalf("the other server addon's episodes: %+v", again)
	}
	if series, got := held(); series != "tt9000" || got != found {
		t.Errorf("the episode once another server addon's series listed it: of %s, %v", series, got)
	}
	episodes(other, "Server Shows", "The Server's Show")
	if series, got := held(); series != "tt7000" || got != found {
		t.Errorf("the episode once its first series listed it again: of %s, %v", series, got)
	}
}

// privateMovieAddon serves a catalog of the name given with one movie,
// which it describes too when describes is set, and no other title.
func privateMovieAddon(t *testing.T, id, name string, movie stremio.Meta, describes bool) string {
	t.Helper()
	resources := []stremio.Resource{{Name: "catalog"}}
	if describes {
		resources = append(resources, stremio.Resource{Name: "meta"})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: id, Name: name, Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"private:"}, Resources: resources,
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "mine", Name: name}}})
		case strings.HasPrefix(path, "/catalog/movie/mine"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case describes && r.URL.Path == "/meta/movie/"+movie.ID+".json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// An item's identifier is derived from what its addon calls the title:
// whoever knows that can ask for the item. A title only another user's
// own addon lists is none of a member's: asked by its identifier, alone
// or among others, it answers as a title that does not exist, while its
// owner reads it with their addon's texts, and an administrator, who
// edits every item, as before. Once one of the server's addons lists the
// same identifier, the title is the server's, for everyone.
func TestATitleOfAnotherUsersOwnAddonIsNotFound(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	secret := stremio.Meta{ID: "private:1", Type: "movie", Name: "Secret", Description: "The owner's own.", Genres: []string{"Action"},
		Poster: "https://example.com/secret.jpg", Extras: &stremio.Extras{Cast: []stremio.CastMember{{Name: "Ann Lee"}}}}
	addon, err := s.addons.Install(t.Context(), addons.Personal(owner.ID), privateMovieAddon(t, "own.private", "Mine", secret, true), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Personal(owner.ID), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "mine"}}); err != nil {
		t.Fatal(err)
	}
	mine := s.signIn("owner", "phone")
	var page QueryResult
	if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, mine)["Mine"], mine, &page); status != http.StatusOK || len(page.Items) != 1 {
		t.Fatalf("the owner's library: %d %+v", status, page.Items)
	}
	id := library.TitleItemID(library.KindMovie, "private:1").String()
	if page.Items[0].Id != id {
		t.Fatalf("the item is %s, which is not derived from its identifier", page.Items[0].Id)
	}
	// read answers what the user of token reads of the title by its
	// identifier, and how many titles asking for it among others lists.
	read := func(token string) (int, string, int) {
		t.Helper()
		var item BaseItemDto
		var listed QueryResult
		status := s.get(t, "/Items/"+id, token, &item)
		if listing := s.get(t, "/Items?Ids="+id, token, &listed); listing != http.StatusOK {
			t.Fatalf("/Items?Ids=: %d", listing)
		}
		return status, item.Name, len(listed.Items)
	}
	if status, name, listed := read(mine); status != http.StatusOK || name != "Secret" || listed != 1 {
		t.Fatalf("the owner's own title: %d %q, %d listed", status, name, listed)
	}
	s.user("member", nil)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	member, admin := s.signIn("member", "tv"), s.signIn("admin", "dashboard")
	if status, name, listed := read(member); status != http.StatusNotFound || name != "" || listed != 0 {
		t.Errorf("a member's read of another user's own title: %d %q, %d listed", status, name, listed)
	}
	if status, name, listed := read(admin); status != http.StatusOK || name != "Secret" || listed != 1 {
		t.Errorf("an administrator's read of a user's own title: %d %q, %d listed", status, name, listed)
	}
	// The owner is a member like any other, and still reads their own.
	if _, err := s.store.UpdateUser(t.Context(), owner.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, name, listed := read(mine); status != http.StatusOK || name != "Secret" || listed != 1 {
		t.Errorf("the owner's own title as a member: %d %q, %d listed", status, name, listed)
	}

	// One of the server's addons lists the identifier: once its library
	// is listed, the title is the server's, with the server's texts.
	public := secret
	public.Name, public.Description = "Public", "The server's."
	shared, err := s.addons.Install(t.Context(), addons.Shared(), privateMovieAddon(t, "shared.private", "Everyone's", public, true), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: shared.ID, CatalogType: "movie", CatalogID: "mine"}}); err != nil {
		t.Fatal(err)
	}
	if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, member)["Everyone's"], member, &page); status != http.StatusOK || len(page.Items) != 1 ||
		page.Items[0].Id != id {
		t.Fatalf("the server's library: %d %+v", status, page.Items)
	}
	for who, token := range map[string]string{"a member": member, "an administrator": admin, "the owner": mine} {
		if status, name, listed := read(token); status != http.StatusOK || name != "Public" || listed != 1 {
			t.Errorf("%s's read of the title the server lists: %d %q, %d listed", who, status, name, listed)
		}
	}
}

// Two users' own addons may list a title under the same identifier,
// which makes it one item. It follows the user who listed it last, each
// reading their own addon's texts once they listed it, and neither ever
// the other's: a title the other holds, which none of their addons
// describes, answers as one that does not exist until they list it
// again. Each is an administrator while they list, as their addon, on
// the test's local address, is read then, and a member while the other
// lists.
func TestTwoUsersOwnAddonsOfOneIdentifierShowEachTheirOwn(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	id := library.TitleItemID(library.KindMovie, "private:1").String()
	tokens, users := map[string]string{}, map[string]accounts.ID{}
	for _, name := range []string{"first", "second"} {
		user := s.user(name, func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
		movie := stremio.Meta{ID: "private:1", Type: "movie", Name: "The " + name + " user's", Poster: "https://example.com/" + name + ".jpg"}
		addon, err := s.addons.Install(t.Context(), addons.Personal(user.ID), privateMovieAddon(t, "own."+name, "Mine", movie, false), false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.addons.SetLibraries(t.Context(), addons.Personal(user.ID), []addons.LibraryChoice{
			{AddonID: addon.ID, CatalogType: "movie", CatalogID: "mine"}}); err != nil {
			t.Fatal(err)
		}
		tokens[name], users[name] = s.signIn(name, "phone"), user.ID
	}
	administrator := func(name string, is bool) {
		t.Helper()
		if _, err := s.store.UpdateUser(t.Context(), users[name], accounts.UserChanges{IsAdministrator: new(is)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	list := func(name string) {
		t.Helper()
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, tokens[name])["Mine"], tokens[name], &page); status != http.StatusOK || len(page.Items) != 1 ||
			page.Items[0].Id != id {
			t.Fatalf("the %s user's library: %d %+v", name, status, page.Items)
		}
	}
	read := func(name string) (int, string) {
		t.Helper()
		var item BaseItemDto
		status := s.get(t, "/Items/"+id, tokens[name], &item)
		return status, item.Name
	}
	for _, turn := range [][2]string{{"first", "second"}, {"second", "first"}, {"first", "second"}} {
		lister, other := turn[0], turn[1]
		administrator(lister, true)
		list(lister)
		administrator(other, false)
		if status, name := read(lister); status != http.StatusOK || name != "The "+lister+" user's" {
			t.Errorf("the %s user's own title once they listed it: %d %q", lister, status, name)
		}
		if status, name := read(other); status != http.StatusNotFound || name != "" {
			t.Errorf("the %s user's read of the title the %s user listed last: %d %q", other, lister, status, name)
		}
	}
}

// The pages of a person and of a genre list titles from what is kept of
// them, without describing each: what is kept of a title another user's
// own addon holds is that user's. Such a title is listed to no other
// member, even one whose addons serve its type and identifier: neither
// among a person's titles, nor on a genre's page, and a person it alone
// credits is no one to them. Its owner lists it, and an administrator,
// who edits every item, as before.
func TestListsShowNoTitleAnotherUsersOwnAddonHolds(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("member", nil)
	add := func(scope addons.Scope, id, name string, movie stremio.Meta) {
		t.Helper()
		addon, err := s.addons.Install(t.Context(), scope, privateMovieAddon(t, id, name, movie, true), false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.addons.SetLibraries(t.Context(), scope, []addons.LibraryChoice{{AddonID: addon.ID, CatalogType: "movie", CatalogID: "mine"}}); err != nil {
			t.Fatal(err)
		}
	}
	movie := func(id, name string, cast ...string) stremio.Meta {
		meta := stremio.Meta{ID: id, Type: "movie", Name: name, Genres: []string{"Whodunit"}, Poster: "https://example.com/" + name + ".jpg",
			Extras: &stremio.Extras{}}
		for _, member := range cast {
			meta.Extras.Cast = append(meta.Extras.Cast, stremio.CastMember{Name: member})
		}
		return meta
	}
	// The server's addon serves the identifiers the owner's own addon
	// uses, and knows another title of them.
	add(addons.Shared(), "shared.private", "Everyone's", movie("private:2", "Known", "Ann Lee"))
	add(addons.Personal(owner.ID), "own.private", "Mine", movie("private:1", "Secret", "Ann Lee", "Zed Hidden"))
	mine, member, admin := s.signIn("owner", "phone"), s.signIn("member", "tv"), s.signIn("admin", "dashboard")
	// open lists a library and opens its title, which records its people.
	open := func(token, view string) []BaseItemPerson {
		t.Helper()
		var page QueryResult
		var item BaseItemDto
		if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)[view], token, &page); status != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("the library %s: %d %+v", view, status, page.Items)
		}
		if status := s.get(t, "/Items/"+page.Items[0].Id, token, &item); status != http.StatusOK || item.People == nil {
			t.Fatalf("the title of %s: %d", view, status)
		}
		return *item.People
	}
	people := open(mine, "Mine")
	ann, zed := credit(people, "Ann Lee").Id, credit(people, "Zed Hidden").Id
	if known := credit(open(member, "Everyone's"), "Ann Lee").Id; ann == "" || zed == "" || known != ann {
		t.Fatalf("the people credited: %q, %q and %q", ann, zed, known)
	}
	if _, err := s.store.UpdateUser(t.Context(), owner.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	titles := func(token, query string) string {
		t.Helper()
		names, _ := listed(t, s, token, titlesOf+query)
		return strings.Join(names, ",")
	}
	for what, query := range map[string]string{"a person's titles": "personIds=" + ann, "a genre's page": "genres=Whodunit"} {
		if got := titles(member, query); got != "Known" {
			t.Errorf("%s for another member: %s", what, got)
		}
		for who, token := range map[string]string{"the owner": mine, "an administrator": admin} {
			if got := titles(token, query); got != "Known,Secret" {
				t.Errorf("%s for %s: %s", what, who, got)
			}
		}
	}
	// The person the owner's title alone credits.
	var found QueryResult
	if status := s.get(t, "/Items?personIds="+zed, member, &found); status != http.StatusNotFound && len(found.Items) != 0 {
		t.Errorf("the titles of a person only the owner's title credits, for another member: %d %v", status, itemNames(found.Items))
	}
	if status := s.get(t, "/Items/"+zed, member, nil); status != http.StatusNotFound {
		t.Errorf("a person only the owner's title credits, for another member: %d", status)
	}
	var persons QueryResult
	if status := s.get(t, "/Persons", member, &persons); status != http.StatusOK || slices.Contains(itemNames(persons.Items), "Zed Hidden") ||
		!slices.Contains(itemNames(persons.Items), "Ann Lee") {
		t.Errorf("the people another member lists: %d %v", status, itemNames(persons.Items))
	}
	if got := titles(mine, "personIds="+zed); got != "Secret" {
		t.Errorf("that person's titles for the owner: %s", got)
	}
}

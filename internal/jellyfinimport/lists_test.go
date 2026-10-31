package jellyfinimport

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playlists"
	"github.com/moodiness/polyfin/internal/userdata"
)

// allExtras asks an import for the ratings, the collections and the
// playlists.
var allExtras = Extras{Ratings: true, Collections: true, Playlists: true}

// collectionsOf lists every Polyfin collection, by name.
func (h harness) collectionsOf(t *testing.T) []collections.Collection {
	t.Helper()
	all, err := h.Service.collections.All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// playlistsOf lists the playlists user owns, by name.
func (h harness) playlistsOf(t *testing.T, user accounts.ID) []playlists.Playlist {
	t.Helper()
	visible, err := h.Service.playlists.Visible(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(visible, func(p playlists.Playlist) bool { return p.Owner != user })
}

// itemsOf are the items refs designate, in order.
func (h harness) itemsOf(t *testing.T, refs ...library.TitleRef) []accounts.ID {
	t.Helper()
	ids := make([]accounts.ID, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, h.idOf(t, ref).ID)
	}
	return ids
}

// entryItems are the items of a playlist's entries, in order.
func entryItems(p playlists.Playlist) []accounts.ID {
	ids := make([]accounts.ID, 0, len(p.Entries))
	for _, entry := range p.Entries {
		ids = append(ids, entry.Item)
	}
	return ids
}

// A Jellyfin import brings each user's ratings as they are, but over
// Polyfin's own, and a thumb up without a rating as a favorite; the
// server's collections, created once for every user who sees them, with
// the titles found, an empty one listed; and each user's video playlists,
// as their own, in order. Importing again adds nothing twice, but the
// titles a collection gained since.
func TestJellyfinRatingsCollectionsAndPlaylistsComeOnce(t *testing.T) {
	h := newHarness(t)
	admin, alice, bob := h.user(t, "admin"), h.user(t, "alice"), h.user(t, "bob")
	h.listed(t, "movie", "movie|tmdb:550", `{"meta":{"id":"tmdb:550","type":"movie","name":"Listed by TMDB"}}`)
	h.listed(t, "series", "series|tvdb:81189", `{"meta":{"id":"tvdb:81189","type":"series","name":"Listed by TVDB"}}`)
	h.listed(t, "episode", "episode|tvdb:81189:1:1",
		`{"seriesId":"tvdb:81189","video":{"id":"tvdb:81189:1:1","season":1,"episode":1}}`)
	rated, kept, liked, disliked := library.TitleRef{IMDb: "tt0000001"}, library.TitleRef{IMDb: "tt0000002"},
		library.TitleRef{IMDb: "tt0000003"}, library.TitleRef{IMDb: "tt0000005"}
	byTMDB, series := library.TitleRef{TMDB: 550}, library.TitleRef{Series: true, TVDB: 81189}
	pilot, later := library.TitleRef{Episode: true, TVDB: 81189, Season: 1, Number: 1}, library.TitleRef{IMDb: "tt0000004"}
	// Polyfin already has a rating of the second movie.
	if _, err := h.data.Change(t.Context(), alice.ID, []userdata.Item{h.idOf(t, kept)}, func(d *userdata.Data) { d.Rating = new(3.0) }); err != nil {
		t.Fatal(err)
	}

	h.f.series = map[string]map[string]string{"s-tvdb": {"Tvdb": "81189"}}
	ids := func(imdb string) map[string]string { return map[string]string{"Imdb": imdb} }
	h.f.items["jf-alice"] = map[string][]map[string]any{
		"IsPlayed": {
			movie("m1", "Rated", ids("tt0000001"), map[string]any{"Played": true, "Rating": 7.5, "Likes": true}),
			movie("m2", "Kept", ids("tt0000002"), map[string]any{"Played": true, "Rating": 9.0, "Likes": true}),
			movie("m5", "Disliked", ids("tt0000005"), map[string]any{"Played": true, "Likes": false}),
		},
		// Jellyfin lists the first movie, rated over 6.5, as liked too;
		// Emby keeps a thumb up without a rating.
		"Likes": {
			movie("m1", "Rated", ids("tt0000001"), map[string]any{"Rating": 7.5, "Likes": true}),
			movie("m3", "Liked", ids("tt0000003"), map[string]any{"Likes": true}),
		},
		"BoxSet": {
			{"Id": "bs1", "Name": "Heists", "Type": "BoxSet"},
			{"Id": "bs2", "Name": "Home videos", "Type": "BoxSet"},
		},
		"Playlist": {
			{"Id": "pl1", "Name": "Weekend", "Type": "Playlist", "MediaType": "Video"},
			{"Id": "pl2", "Name": "Songs", "Type": "Playlist", "MediaType": "Audio"},
		},
	}
	h.f.items["jf-bob"] = map[string][]map[string]any{
		"BoxSet":   {{"Id": "bs1", "Name": "Heists", "Type": "BoxSet"}},
		"Playlist": {{"Id": "pl1", "Name": "Weekend", "Type": "Playlist", "MediaType": "Video"}},
	}
	h.f.children["bs1"] = []map[string]any{
		movie("c1", "By TMDB", map[string]string{"Tmdb": "550"}, nil),
		{"Id": "s-tvdb", "Name": "Listed by TVDB", "Type": "Series", "ProviderIds": map[string]string{"Tvdb": "81189"}},
		movie("c2", "Lost", map[string]string{"Tmdb": "999999"}, nil),
	}
	h.f.children["bs2"] = []map[string]any{movie("c3", "Holiday", map[string]string{}, nil)}
	h.f.entries["pl1"] = []map[string]any{
		episode("e1", "s-tvdb", 1, new(1), nil),
		movie("m1", "Rated", ids("tt0000001"), nil),
		{"Id": "a1", "Name": "A song", "Type": "Audio"},
		movie("c2", "Lost", map[string]string{"Tmdb": "999999"}, nil),
		movie("m1", "Rated", ids("tt0000001"), nil),
	}
	h.f.entries["pl2"] = []map[string]any{{"Id": "a1", "Name": "A song", "Type": "Audio"}}

	start := func() Status {
		t.Helper()
		_, err := h.Start(t.Context(), Connection{Kind: Jellyfin, Address: h.f.url, Credentials: Credentials{Key: goodKey}, Extras: allExtras},
			admin, func(Server, []User, SignIn) ([]Target, error) {
				return []Target{{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}, {JellyfinID: "jf-bob", User: bob.ID, UserName: "bob"}}, nil
			})
		if err != nil {
			t.Fatal(err)
		}
		status := h.ended(t)
		if status.State != StateDone || len(status.Users) != 2 {
			t.Fatalf("import: %+v", status)
		}
		return status
	}
	status := start()
	gotAlice, gotBob := status.Users[0], status.Users[1]
	if gotAlice.Ratings != 1 || gotAlice.Favorites != 1 || gotAlice.Collections != 1 || gotAlice.Playlists != 1 ||
		!slices.Equal(gotAlice.Skipped, []SkippedList{{Kind: ListCollection, Name: "Home videos"}}) || gotAlice.NothingAdded != "" {
		t.Errorf("alice: %+v", gotAlice)
	}
	reasons := map[string]string{}
	for _, u := range gotAlice.Unmatched {
		reasons[u.Name] = u.Reason
	}
	if reasons["Lost"] != ReasonNotFound || reasons["Holiday"] != ReasonNoIdentifier || reasons["A song"] != "" {
		t.Errorf("alice's unmatched: %+v", gotAlice.Unmatched)
	}
	// Bob sees the collection Alice's import created, and gets his own
	// copy of the playlist he sees.
	if gotBob.Collections != 0 || gotBob.Playlists != 1 || len(gotBob.Skipped) != 0 {
		t.Errorf("bob: %+v", gotBob)
	}

	check := func(name string, ref library.TitleRef, want func(userdata.Data) bool) {
		t.Helper()
		if d := h.dataOf(t, alice.ID, ref); !want(d) {
			t.Errorf("%s: %+v", name, d)
		}
	}
	check("rated", rated, func(d userdata.Data) bool { return d.Rating != nil && *d.Rating == 7.5 && !d.Favorite })
	check("kept", kept, func(d userdata.Data) bool { return d.Rating != nil && *d.Rating == 3 })
	check("liked", liked, func(d userdata.Data) bool { return d.Rating == nil && d.Favorite })
	check("disliked", disliked, func(d userdata.Data) bool { return d.Rating == nil && !d.Favorite && d.Played })

	all := h.collectionsOf(t)
	if len(all) != 1 || all[0].Name != "Heists" || !slices.Equal(all[0].Items, h.itemsOf(t, byTMDB, series)) {
		t.Fatalf("collections: %+v", all)
	}
	weekend := h.itemsOf(t, pilot, rated, rated)
	for _, user := range []accounts.User{alice, bob} {
		if mine := h.playlistsOf(t, user.ID); len(mine) != 1 || mine[0].Name != "Weekend" || !slices.Equal(entryItems(mine[0]), weekend) {
			t.Errorf("%s's playlists: %+v", user.Name, mine)
		}
	}
	for _, r := range h.f.sent() {
		if strings.Contains(r.URL.Path, "/Playlists/pl2/") || r.URL.Query().Get("ParentId") == "bs1" && r.URL.Query().Get("IncludeItemTypes") != collectionTypes {
			t.Errorf("asked %s", r.URL)
		}
	}

	// The collection gained a movie since: importing again adds it, and
	// nothing else, and says so: Alice's import added a title to a list
	// brought before, and Bob's found both of his lists up to date.
	h.f.children["bs1"] = append(h.f.children["bs1"], movie("c4", "Later", ids("tt0000004"), nil))
	again := start()
	for _, u := range again.Users {
		if u.Ratings+u.Favorites+u.Collections+u.Playlists != 0 {
			t.Errorf("again, %s: %+v", u.UserName, u)
		}
	}
	if got := again.Users[0]; got.ListTitles != 1 || got.NothingAdded != "" {
		t.Errorf("again, alice: %+v", got)
	}
	if got := again.Users[1]; got.ListTitles != 0 || got.UpToDate != 2 || got.NothingAdded != NothingUpToDate {
		t.Errorf("again, bob: %+v", got)
	}
	if all := h.collectionsOf(t); len(all) != 1 || !slices.Equal(all[0].Items, h.itemsOf(t, byTMDB, series, later)) {
		t.Errorf("collections again: %+v", all)
	}
	if mine := h.playlistsOf(t, alice.ID); len(mine) != 1 || !slices.Equal(entryItems(mine[0]), weekend) {
		t.Errorf("alice's playlists again: %+v", mine)
	}

	// A playlist deleted in Polyfin is brought again.
	if err := h.Service.playlists.Delete(t.Context(), h.playlistsOf(t, alice.ID)[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := start().Users[0]; got.Playlists != 1 {
		t.Errorf("after the playlist was deleted: %+v", got)
	}
}

// An import without extras reads no rating, collection or playlist. A
// user's own Emby import brings their thumbs up as favorites, their
// ratings and playlists, and the collections only once they may manage
// collections, as Jellyfin lets only them create one.
func TestAUsersOwnEmbyImportBringsCollectionsOnlyToThoseWhoManageThem(t *testing.T) {
	h := newHarness(t)
	h.f.emby = true
	carol := h.user(t, "carol")
	h.f.users = []map[string]any{{"Id": "jf-carol", "Name": "Carol"}}
	h.f.passwords = map[string]string{"Carol": "carol's password"}
	ids := func(imdb string) map[string]string { return map[string]string{"Imdb": imdb} }
	h.f.items["jf-carol"] = map[string][]map[string]any{
		"IsPlayed": {movie("m2", "Rated", ids("tt0000002"), map[string]any{"Played": true, "Rating": 6.0})},
		"Likes":    {movie("m1", "Liked", ids("tt0000001"), map[string]any{"Likes": true})},
		"BoxSet":   {{"Id": "bs1", "Name": "Picks", "Type": "BoxSet"}},
		"Playlist": {{"Id": "pl1", "Name": "Tonight", "Type": "Playlist", "MediaType": "Video"}},
	}
	h.f.children["bs1"] = []map[string]any{movie("m3", "Picked", ids("tt0000003"), nil)}
	h.f.entries["pl1"] = []map[string]any{movie("m2", "Rated", ids("tt0000002"), nil)}
	own := func(user accounts.User, extras Extras) UserStatus {
		t.Helper()
		snapshot, err := h.ImportOwn(t.Context(), Connection{Kind: Emby, Address: h.f.url,
			Credentials: Credentials{Name: "Carol", Password: "carol's password"}, Extras: extras}, user)
		if err != nil {
			t.Fatal(err)
		}
		status := h.ended(t)
		if status.State != StateDone || status.Extras != snapshot.Extras {
			t.Fatalf("import: %+v", status)
		}
		return status.Users[0]
	}
	asked := func(what string) bool {
		for _, r := range h.f.sent() {
			query := r.URL.Query()
			if query.Get("Filters") == what || query.Get("IncludeItemTypes") == what || strings.HasPrefix(r.URL.Path, "/emby"+what) {
				return true
			}
		}
		return false
	}

	if got := own(carol, Extras{}); got.Played != 1 || got.Ratings+got.Favorites+got.Collections+got.Playlists != 0 {
		t.Errorf("without extras: %+v", got)
	}
	for _, what := range []string{"Likes", "BoxSet", "Playlist", "/Playlists/"} {
		if asked(what) {
			t.Errorf("without extras, asked for %s", what)
		}
	}

	if got := own(carol, allExtras); got.Ratings != 1 || got.Favorites != 1 || got.Collections != 0 || got.Playlists != 1 {
		t.Errorf("Carol: %+v", got)
	}
	if d := h.dataOf(t, carol.ID, library.TitleRef{IMDb: "tt0000001"}); !d.Favorite || d.Rating != nil {
		t.Errorf("liked: %+v", d)
	}
	if asked("BoxSet") || len(h.collectionsOf(t)) != 0 {
		t.Error("Carol, who may not manage collections, brought some")
	}
	manager, err := h.store.UpdateUser(t.Context(), carol.ID, accounts.UserChanges{CollectionManagement: new(true)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := own(manager, allExtras); got.Collections != 1 || got.Playlists != 0 {
		t.Errorf("Carol, managing collections: %+v", got)
	}
	if all := h.collectionsOf(t); len(all) != 1 || all[0].Name != "Picks" {
		t.Errorf("collections: %+v", all)
	}
	if mine := h.playlistsOf(t, carol.ID); len(mine) != 1 || mine[0].Name != "Tonight" {
		t.Errorf("playlists: %+v", mine)
	}
}

// A Plex import brings the collections of the library sections once, read
// with the owner's token, and the video playlists of the accounts read
// with their own token, the owner's here: not those of an account read
// from the playback history. Ratings left out are not read, and importing
// again adds nothing twice.
func TestPlexCollectionsAndPlaylistsComeOnce(t *testing.T) {
	h := newHarness(t)
	p := smallPlex(t)
	admin, owner, carol := h.user(t, "admin"), h.user(t, "owner"), h.user(t, "carol")
	p.items["1"]["1"][0]["userRating"] = 8.0
	p.collections["1"] = []map[string]any{
		{"ratingKey": "100", "type": "collection", "title": "House picks", "subtype": "movie"},
		{"ratingKey": "101", "type": "collection", "title": "Gone", "subtype": "movie"},
	}
	p.collections["2"] = []map[string]any{{"ratingKey": "102", "type": "collection", "title": "Shows", "subtype": "show"}}
	p.children["100"] = []map[string]any{
		{"ratingKey": "12", "type": "movie", "title": "Two"},
		{"ratingKey": "99", "type": "movie", "title": "Deleted"},
		{"ratingKey": "11", "type": "movie", "title": "One"},
	}
	p.children["101"] = []map[string]any{{"ratingKey": "99", "type": "movie", "title": "Deleted"}}
	p.children["102"] = []map[string]any{{"ratingKey": "20", "type": "show", "title": "A show"}}
	p.playlists[plexToken] = []map[string]any{
		{"ratingKey": "200", "type": "playlist", "title": "Movie night", "playlistType": "video"},
		{"ratingKey": "201", "type": "playlist", "title": "Songs", "playlistType": "audio"},
	}
	p.entries["200"] = []map[string]any{
		{"ratingKey": "21", "type": "episode", "title": "Pilot", "grandparentRatingKey": "20", "grandparentTitle": "A show",
			"parentIndex": 1, "index": 1},
		{"ratingKey": "11", "type": "movie", "title": "One"},
	}
	one, two := library.TitleRef{IMDb: "tt0000011"}, library.TitleRef{IMDb: "tt0000012"}
	show, pilot := library.TitleRef{Series: true, IMDb: "tt0100"}, library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 1}

	start := func() Status {
		t.Helper()
		plex := Connection{Kind: Plex, Address: p.url, Credentials: Credentials{Key: plexToken},
			Extras: Extras{Collections: true, Playlists: true}}
		if _, err := h.Start(t.Context(), plex, admin, func(Server, []User, SignIn) ([]Target, error) {
			return []Target{{JellyfinID: "1", User: owner.ID, UserName: "owner"}, {JellyfinID: "5", User: carol.ID, UserName: "carol"}}, nil
		}); err != nil {
			t.Fatal(err)
		}
		status := h.ended(t)
		if status.State != StateDone {
			t.Fatalf("import: %+v", status)
		}
		return status
	}
	status := start()
	gotOwner, gotCarol := status.Users[0], status.Users[1]
	if gotOwner.Collections != 2 || gotOwner.Playlists != 1 || gotOwner.Ratings != 0 ||
		!slices.Equal(gotOwner.Skipped, []SkippedList{{Kind: ListCollection, Name: "Gone"}}) {
		t.Errorf("owner: %+v", gotOwner)
	}
	if len(gotOwner.Unmatched) != 1 || gotOwner.Unmatched[0].Name != "Deleted" || gotOwner.Unmatched[0].Reason != ReasonNotInLibrary {
		t.Errorf("owner's unmatched: %+v", gotOwner.Unmatched)
	}
	if gotCarol.Collections != 0 || gotCarol.Playlists != 0 || len(gotCarol.Skipped) != 0 {
		t.Errorf("Carol: %+v", gotCarol)
	}
	if d := h.dataOf(t, owner.ID, one); d.Rating != nil {
		t.Errorf("a rating left out came: %+v", d)
	}
	byName := map[string][]accounts.ID{}
	for _, c := range h.collectionsOf(t) {
		byName[c.Name] = c.Items
	}
	if len(byName) != 2 || !slices.Equal(byName["House picks"], h.itemsOf(t, two, one)) || !slices.Equal(byName["Shows"], h.itemsOf(t, show)) {
		t.Errorf("collections: %v", byName)
	}
	if mine := h.playlistsOf(t, owner.ID); len(mine) != 1 || mine[0].Name != "Movie night" ||
		!slices.Equal(entryItems(mine[0]), h.itemsOf(t, pilot, one)) {
		t.Errorf("owner's playlists: %+v", mine)
	}
	if mine := h.playlistsOf(t, carol.ID); len(mine) != 0 {
		t.Errorf("Carol's playlists: %+v", mine)
	}

	again := start()
	for _, u := range again.Users {
		if u.Collections+u.Playlists != 0 {
			t.Errorf("again, %s: %+v", u.UserName, u)
		}
	}
	if len(h.collectionsOf(t)) != 2 || len(h.playlistsOf(t, owner.ID)) != 1 || len(h.playlistsOf(t, owner.ID)[0].Entries) != 2 {
		t.Errorf("again: %+v %+v", h.collectionsOf(t), h.playlistsOf(t, owner.ID))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	sections := 0
	for _, r := range p.requests {
		if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/playlists/201") {
			t.Errorf("Plex was sent %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/library/sections/1/collections" {
			sections++
		}
	}
	// Once by each of the two imports.
	if sections != 2 {
		t.Errorf("the collections were read %d times", sections)
	}
}

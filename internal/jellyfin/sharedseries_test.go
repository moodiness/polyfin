package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// ownVaultAddon is a user's own addon, as its author writes it: a
// catalog of a type of its own, "vault", whose one title carries the
// identifier given, and whose two videos carry it followed by ":1:1" and
// ":1:2", as a series' episodes do.
func ownVaultAddon(t *testing.T, id string) string {
	t.Helper()
	title := stremio.Meta{ID: id, Type: "vault", Name: "A user's own title"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "org.example.own", Name: "Own", Version: "1", Types: []string{"vault"},
				Resources: []stremio.Resource{{Name: "catalog", Types: []string{"vault"}}, {Name: "meta", Types: []string{"vault"}}},
				Catalogs:  []stremio.Catalog{{Type: "vault", ID: "mine", Name: "Mine", Extra: []stremio.Extra{{Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/vault/mine"):
			var metas []stremio.Meta
			if !strings.Contains(path, "skip=") {
				metas = []stremio.Meta{title}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(r.URL.Path, "/meta/vault/"):
			meta := title
			meta.Videos = []stremio.Video{{ID: id + ":1:1", Title: "a", Season: 1, Episode: 1}, {ID: id + ":1:2", Title: "b", Season: 1, Episode: 2}}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// ownVault is a user's own library of an ownVaultAddon, with its view
// once shown. Its owner is an administrator while the addon, on a local
// address, is read, as no test server is on a public one.
type ownVault struct {
	testServer
	owner accounts.User
	scope addons.Scope
	addon addons.Addon
	token string
}

func newOwnVault(t *testing.T, s testServer, id string) ownVault {
	t.Helper()
	v := ownVault{testServer: s, owner: s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })}
	v.scope = addons.Personal(v.owner.ID)
	var err error
	if v.addon, err = s.addons.Install(t.Context(), v.scope, ownVaultAddon(t, id), false); err != nil {
		t.Fatal(err)
	}
	v.token = s.signIn("owner", "laptop")
	return v
}

// show saves the user's own libraries as the administration's page does
// (PUT /admin/api/scopes/me/libraries): the library, shown as showAs.
func (v ownVault) show(t *testing.T, showAs string) {
	t.Helper()
	before, err := v.addons.Libraries(t.Context(), v.scope)
	if err != nil {
		t.Fatal(err)
	}
	after, err := v.addons.SetLibraries(t.Context(), v.scope,
		[]addons.LibraryChoice{{AddonID: v.addon.ID, CatalogType: "vault", CatalogID: "mine", ShowAs: showAs}})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.library.FollowTitles(t.Context(), before, after); err != nil {
		t.Fatal(err)
	}
}

// titles lists the library's titles as the apps that list only movies and
// shows ask for them.
func (v ownVault) titles(t *testing.T) []BaseItemDto {
	t.Helper()
	var page QueryResult
	if status := v.get(t, "/Items?ParentId="+v.viewIDs(t, v.token)["Mine"]+"&Recursive=true&IncludeItemTypes=Movie,Series", v.token, &page); status != http.StatusOK {
		t.Fatalf("the user's own library: %d", status)
	}
	return page.Items
}

// demote makes the owner a member, once their addon was read.
func (v ownVault) demote(t *testing.T) {
	t.Helper()
	v.testServer.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	if _, err := v.store.UpdateUser(t.Context(), v.owner.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
}

// position is the resume point, in ticks, a user keeps on the item of
// key: -1 when they keep nothing of it.
func (v ownVault) position(t *testing.T, user accounts.ID, key string) int64 {
	t.Helper()
	var ticks int64
	if err := v.pool.QueryRow(t.Context(), `SELECT coalesce((SELECT u.position_ticks FROM user_data u JOIN items i ON i.id = u.item_id
		WHERE i.key = $1 AND u.user_id = $2), -1)`, key, user).Scan(&ticks); err != nil {
		t.Fatal(err)
	}
	return ticks
}

// Items are shared by every user and identified by what their addon
// calls them. A user's own addon that lists, in a library of theirs
// shown as movies and shows, a title under the identifier of a series of
// the server's library leaves that series as it is: the others still open
// it and its episodes. Switching their library back to home videos moves
// nothing of what the others did: the episode stays on Continue Watching.
func TestAUsersOwnLibraryLeavesTheServersSeriesToItsUsers(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	member, err := accounts.ParseID(tr.user)
	if err != nil {
		t.Fatal(err)
	}
	open := func(id string) (int, string) {
		t.Helper()
		var item BaseItemDto
		status := tr.get(t, "/Users/"+tr.user+"/Items/"+id, tr.token, &item)
		return status, item.Name
	}
	// The member is halfway through the first episode of the series of
	// the server's library.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.episodes[0], "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Progress", map[string]any{"ItemId": tr.episodes[0], "PositionTicks": int64(13_000_000_000)})
	if tr.series != library.TitleItemID(library.KindSeries, "tt0100").String() || tr.episodes[0] != library.EpisodeItemID("tt0100:1:1").String() {
		t.Fatalf("the series %s and its episode %s are not derived from their identifiers", tr.series, tr.episodes[0])
	}

	// Another user's own addon lists a title of the series' identifier,
	// shown as movies and shows: the same item.
	v := newOwnVault(t, tr.testServer, "tt0100")
	v.show(t, addons.ShowAsMoviesAndShows)
	if titles := v.titles(t); len(titles) != 1 || titles[0].Id != tr.series {
		t.Fatalf("the user's own library: %+v", titles)
	}
	v.demote(t)
	if status, name := open(tr.series); status != http.StatusOK || name != "Show" {
		t.Errorf("the server's series for its users: %d %q", status, name)
	}
	if status, _ := open(tr.episodes[0]); status != http.StatusOK {
		t.Errorf("the series' episode for its users: %d", status)
	}

	// The owner switches their library back, then on again.
	for _, showAs := range []string{addons.ShowAsHomeVideos, addons.ShowAsMoviesAndShows} {
		v.show(t, showAs)
		if resume := tr.list(t, "/UserItems/Resume?mediaTypes=Video"); len(resume) != 1 || resume[0] != tr.episodes[0] {
			t.Errorf("the member's Continue Watching once the library shows %s: %v", showAs, resume)
		}
		if on, moved := v.position(t, member, "episode|tt0100:1:1"), v.position(t, member, "foldervideo|tt0100:1:1"); on != 13_000_000_000 || moved != -1 {
			t.Errorf("the member's resume point once the library shows %s: %d on the episode, %d on the user's own video", showAs, on, moved)
		}
	}
	if status, name := open(tr.series); status != http.StatusOK || name != "Show" {
		t.Errorf("the server's series after the switches: %d %q", status, name)
	}
}

// A user's own library that switches between home videos and movies and
// shows gives what its owner did with its titles to the items they
// become, and nothing of anyone else: items are shared, and another user
// may keep a resume point on one, from when one of the server's addons
// listed it.
func TestAUsersOwnLibraryMovesWhatItsOwnerDidAlone(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	other := s.user("other", nil)
	v := newOwnVault(t, s, "own:show")
	v.show(t, addons.ShowAsMoviesAndShows)
	titles := v.titles(t)
	if len(titles) != 1 || titles[0].Id != library.TitleItemID(library.KindSeries, "own:show").String() {
		t.Fatalf("the user's own library: %+v", titles)
	}
	var episodes QueryResult
	if status := s.get(t, "/Shows/"+titles[0].Id+"/Episodes", v.token, &episodes); status != http.StatusOK || len(episodes.Items) != 2 {
		t.Fatalf("the title's episodes: %d %+v", status, episodes.Items)
	}
	v.demote(t)
	// Both are halfway through the first episode.
	at := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	episode := userdata.Item{ID: library.EpisodeItemID("own:show:1:1"), Series: library.TitleItemID(library.KindSeries, "own:show"),
		Season: library.SeasonItemID("own:show", 1)}
	for _, user := range []accounts.ID{v.owner.ID, other.ID} {
		if _, err := userdata.New(s.pool).Change(t.Context(), user, []userdata.Item{episode}, func(d *userdata.Data) {
			d.Position, d.Runtime, d.LastPlayed = 20*time.Minute, time.Hour, &at
		}); err != nil {
			t.Fatal(err)
		}
	}
	const halfway = int64(20 * time.Minute / (100 * time.Nanosecond))
	v.show(t, addons.ShowAsHomeVideos)
	if on, moved := v.position(t, v.owner.ID, "episode|own:show:1:1"), v.position(t, v.owner.ID, "foldervideo|own:show:1:1"); on != 0 || moved != halfway {
		t.Errorf("the owner's resume point: %d on the episode, %d on the video", on, moved)
	}
	if on, moved := v.position(t, other.ID, "episode|own:show:1:1"), v.position(t, other.ID, "foldervideo|own:show:1:1"); on != halfway || moved != -1 {
		t.Errorf("the other user's resume point: %d on the episode, %d on the video", on, moved)
	}
}

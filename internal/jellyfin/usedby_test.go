package jellyfin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// usedByAddon is a fake server addon: a catalog, named catalog, of the
// movie tt1000 and of one only it knows, own, with their descriptions,
// and for both movies a stream of each file named, of the same size
// whatever the addon, so that two addons listing a name list the same
// file, each with a subtitle of its own, and a subtitle file more from
// its subtitles resource. Every subtitle's text names the addon; files
// counts the requests for the files' bytes.
type usedByAddon struct {
	manifest, url string
	files         *atomic.Int32
}

func newUsedByAddon(t *testing.T, name, catalog, own string, files ...string) usedByAddon {
	t.Helper()
	var server *httptest.Server
	var fetched atomic.Int32
	metas := map[string]stremio.Meta{
		"tt1000": {ID: "tt1000", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"},
		own:      {ID: own, Type: "movie", Name: catalog + " Only", Runtime: "1h", Year: "2010"},
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "usedby." + strings.ToLower(strings.ReplaceAll(name, " ", ".")), Name: name,
				Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt", own[:3]},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}, {Name: "subtitles"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: catalog}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{metas["tt1000"], metas[own]}})
		case strings.HasPrefix(path, "/meta/movie/") && metas[id].ID != "":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": metas[id]})
		case strings.HasPrefix(path, "/stream/movie/") && metas[id].ID != "":
			var streams []stremio.Stream
			for _, file := range files {
				streams = append(streams, stremio.Stream{Name: file, Description: "English 1080p", URL: server.URL + "/files/" + file + ".mkv",
					BehaviorHints: stremio.StreamBehavior{Filename: file + ".mkv", VideoSize: 4e9},
					Subtitles:     []stremio.Subtitle{{ID: stremio.Text(name + file), Lang: "eng", URL: server.URL + "/subtitle-files/" + file + ".srt"}}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/subtitles/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{
				{ID: stremio.Text(name + " subtitles"), Lang: "fre", URL: server.URL + "/subtitle-files/more.srt"}}})
		case strings.HasPrefix(path, "/subtitle-files/"):
			_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nWords from " + name + "\n"))
		case strings.HasPrefix(path, "/files/"):
			fetched.Add(1)
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return usedByAddon{manifest: server.URL + "/manifest.json", url: server.URL, files: &fetched}
}

// usedBySetup is a server with two addons, Family Picks, whose catalog
// Picks lists the movie and fam1, streaming the file One, and Friends
// Stream, whose catalog Friends lists the movie and fri1, streaming One
// too and Two. Family Picks is used by the group Family, where alice is,
// Friends Stream by the group Friends, where bob is, keeping his own
// settings; boss, an administrator, is listed by name in both.
type usedBySetup struct {
	testServer
	family, friends                 usedByAddon
	familyAddon, friendsAddon       addons.Addon
	familyGroup, friendsGroup       accounts.Group
	boss, alice, bob                accounts.User
	bossToken, aliceToken, bobToken string
}

func usedByOn(t *testing.T) usedBySetup {
	t.Helper()
	s := usedBySetup{testServer: newTestServer(t, 10),
		family: newUsedByAddon(t, "Family Picks", "Picks", "fam1", "One"), friends: newUsedByAddon(t, "Friends Stream", "Friends", "fri1", "One", "Two")}
	var err error
	if s.familyAddon, err = s.addons.Install(t.Context(), addons.Shared(), s.family.manifest, false); err != nil {
		t.Fatal(err)
	}
	if s.friendsAddon, err = s.addons.Install(t.Context(), addons.Shared(), s.friends.manifest, false); err != nil {
		t.Fatal(err)
	}
	s.boss = s.user("boss", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.alice, s.bob = s.user("alice", nil), s.user("bob", nil)
	for _, user := range []accounts.User{s.boss, s.alice, s.bob} {
		s.permit(t, user, true, true, true)
	}
	if s.familyGroup, err = s.store.CreateGroup(t.Context(), "Family", nil); err != nil {
		t.Fatal(err)
	}
	if s.friendsGroup, err = s.store.CreateGroup(t.Context(), "Friends", nil); err != nil {
		t.Fatal(err)
	}
	for group, user := range map[accounts.ID]accounts.ID{s.familyGroup.ID: s.alice.ID, s.friendsGroup.ID: s.bob.ID} {
		if _, err := s.store.AddGroupMembers(t.Context(), group, []accounts.ID{user}); err != nil {
			t.Fatal(err)
		}
	}
	// A member keeping their own settings still belongs to the group.
	s.limit(t, s.bob, func(c *accounts.UserChanges) { c.FollowGroup = new(false) })
	s.useBy(t, s.familyAddon, addons.UsedBy{Listed: true, Groups: []accounts.ID{s.familyGroup.ID}, Users: []accounts.ID{s.boss.ID}})
	s.useBy(t, s.friendsAddon, addons.UsedBy{Listed: true, Groups: []accounts.ID{s.friendsGroup.ID}, Users: []accounts.ID{s.boss.ID}})
	s.boss, s.alice, s.bob = s.reload(t, s.boss), s.reload(t, s.alice), s.reload(t, s.bob)
	s.bossToken, s.aliceToken, s.bobToken = s.signIn("boss", "desk"), s.signIn("alice", "tv"), s.signIn("bob", "phone")
	return s
}

func (s usedBySetup) useBy(t *testing.T, addon addons.Addon, usedBy addons.UsedBy) {
	t.Helper()
	if _, err := s.addons.SetUsedBy(t.Context(), addon.ID, usedBy); err != nil {
		t.Fatal(err)
	}
}

func (s usedBySetup) reload(t *testing.T, user accounts.User) accounts.User {
	t.Helper()
	reloaded, err := s.store.User(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return reloaded
}

// views lists the names of the libraries token's apps show.
func (s usedBySetup) views(t *testing.T, token string) []string {
	t.Helper()
	var views QueryResult
	if status := s.get(t, "/UserViews", token, &views); status != http.StatusOK {
		t.Fatalf("views: %d", status)
	}
	var names []string
	for _, view := range views.Items {
		names = append(names, view.Name)
	}
	return names
}

// titles maps the titles of the library named library, as token's apps
// list them, by name to identifier.
func (s usedBySetup) titles(t *testing.T, token, library string) map[string]string {
	t.Helper()
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	found := map[string]string{}
	for _, view := range views.Items {
		if view.Name != library {
			continue
		}
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, token, &page)
		for _, item := range page.Items {
			found[item.Name] = item.Id
		}
	}
	return found
}

// files names the files of versions, each with its mirrors' addons.
func files(versions []library.Version) []string {
	var names []string
	for _, version := range versions {
		name := version.Filename
		for _, mirror := range version.Mirrors {
			name += "+" + mirror.Addon
		}
		names = append(names, name)
	}
	return names
}

// Two users in two groups get the libraries, titles, versions and
// subtitles of the addons their groups use, and nothing of the other's;
// an administrator listed in both gets both, a member who keeps their own
// settings still uses their group's.
func TestTwoGroupsGetOnlyTheirAddonsLibrariesAndVersions(t *testing.T) {
	t.Parallel()
	s := usedByOn(t)
	if got := s.views(t, s.aliceToken); !slices.Equal(got, []string{"Picks"}) {
		t.Errorf("alice's libraries: %v", got)
	}
	if got := s.views(t, s.bobToken); !slices.Equal(got, []string{"Friends"}) {
		t.Errorf("bob's libraries: %v", got)
	}
	if got := s.views(t, s.bossToken); !slices.Equal(got, []string{"Picks", "Friends"}) {
		t.Errorf("boss's libraries: %v", got)
	}
	picks, friends := s.titles(t, s.aliceToken, "Picks"), s.titles(t, s.bobToken, "Friends")
	movie, err := accounts.ParseID(picks["Movie"])
	if err != nil || picks["Picks Only"] == "" || friends["Friends Only"] == "" || friends["Movie"] != picks["Movie"] {
		t.Fatalf("Picks %v, Friends %v", picks, friends)
	}
	// A title only the other's addon knows is none of theirs.
	for _, c := range []struct {
		token, item string
		status      int
	}{{s.aliceToken, friends["Friends Only"], http.StatusNotFound}, {s.bobToken, picks["Picks Only"], http.StatusNotFound},
		{s.aliceToken, picks["Picks Only"], http.StatusOK}, {s.bobToken, friends["Friends Only"], http.StatusOK}, {s.bossToken, picks["Picks Only"], http.StatusOK}} {
		if status := s.get(t, "/Items/"+c.item, c.token, nil); status != c.status {
			t.Errorf("item %s: %d, want %d", c.item, status, c.status)
		}
	}
	if status, body := s.call(http.MethodPost, "/Items/"+friends["Friends Only"]+"/PlaybackInfo", app("tv", s.aliceToken), map[string]any{}); status != http.StatusNotFound {
		t.Errorf("alice's PlaybackInfo of Friends Only: %d %.80q", status, body)
	}

	for _, c := range []struct {
		user  accounts.User
		files []string
	}{{s.alice, []string{"One.mkv"}}, {s.bob, []string{"One.mkv", "Two.mkv"}}, {s.boss, []string{"One.mkv+Friends Stream", "Two.mkv"}}} {
		versions, err := s.library.Versions(t.Context(), c.user, movie)
		if got := files(versions); err != nil || !slices.Equal(got, c.files) {
			t.Errorf("%s's versions: %v %v, want %v", c.user.Name, got, err, c.files)
		}
		subtitles, err := s.library.Subtitles(t.Context(), c.user, movie)
		for _, subtitle := range subtitles {
			if c.user.ID == s.alice.ID && strings.HasPrefix(subtitle.URL, s.friends.url) || c.user.ID == s.bob.ID && strings.HasPrefix(subtitle.URL, s.family.url) {
				t.Errorf("%s's subtitles list %s from %s", c.user.Name, subtitle.ID, subtitle.Addon)
			}
		}
		if err != nil || len(subtitles) == 0 {
			t.Errorf("%s's subtitles: %+v %v", c.user.Name, subtitles, err)
		}
	}
	aliceVersions, _ := s.library.Versions(t.Context(), s.alice, movie)
	accessSetup{testServer: s.testServer}.remuxableFile(t, aliceVersions[0])
	answer := s.playbackInfo(t, "tv", s.aliceToken, picks["Movie"], nil)
	if len(answer.MediaSources) != 1 || !strings.Contains(answer.MediaSources[0].Name, "One") {
		t.Errorf("alice's PlaybackInfo: %+v", answer)
	}
	for _, source := range answer.MediaSources {
		for _, stream := range source.MediaStreams {
			if strings.Contains(stream.DisplayTitle, "Friends") || strings.Contains(stream.Title, "Friends") {
				t.Errorf("alice's PlaybackInfo has %s's subtitle %q", s.friendsAddon.Manifest.Name, stream.DisplayTitle)
			}
		}
	}
}

// A user created after an addon was limited is in none of its groups:
// they get nothing of it, until their group or they are listed. An addon
// used by everyone again is everyone's, as before.
func TestANewUserGetsNothingFromALimitedAddon(t *testing.T) {
	t.Parallel()
	s := usedByOn(t)
	movieID := s.titles(t, s.aliceToken, "Picks")["Movie"]
	movie, _ := accounts.ParseID(movieID)
	carol := s.user("carol", nil)
	carolToken := s.signIn("carol", "laptop")
	if got := s.views(t, carolToken); len(got) != 0 {
		t.Errorf("carol's libraries: %v", got)
	}
	if status := s.get(t, "/Items/"+movieID, carolToken, nil); status != http.StatusNotFound {
		t.Errorf("carol's movie: %d", status)
	}
	if versions, err := s.library.Versions(t.Context(), carol, movie); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("carol's versions: %v %v", files(versions), err)
	}
	var found QueryResult
	s.get(t, "/Items?searchTerm=Movie&Recursive=true", carolToken, &found)
	if len(found.Items) != 0 {
		t.Errorf("carol finds %+v", found.Items)
	}

	// Carol joins Family.
	if _, err := s.store.AddGroupMembers(t.Context(), s.familyGroup.ID, []accounts.ID{carol.ID}); err != nil {
		t.Fatal(err)
	}
	carol = s.reload(t, carol)
	if got := s.views(t, carolToken); !slices.Equal(got, []string{"Picks"}) {
		t.Errorf("carol's libraries in Family: %v", got)
	}
	if versions, err := s.library.Versions(t.Context(), carol, movie); err != nil || !slices.Equal(files(versions), []string{"One.mkv"}) {
		t.Errorf("carol's versions in Family: %v %v", files(versions), err)
	}

	// Friends Stream used by everyone again: everyone has it, as an addon
	// never limited.
	s.useBy(t, s.friendsAddon, addons.UsedBy{})
	if got := s.views(t, s.aliceToken); !slices.Equal(got, []string{"Picks", "Friends"}) {
		t.Errorf("alice's libraries once Friends Stream is everyone's: %v", got)
	}
	alice := s.reload(t, s.alice)
	if versions, err := s.library.Versions(t.Context(), alice, movie); err != nil ||
		!slices.Equal(files(versions), []string{"One.mkv+Friends Stream", "Two.mkv"}) {
		t.Errorf("alice's versions once Friends Stream is everyone's: %v %v", files(versions), err)
	}
}

// A deleted group, or user, leaves the addons' lists: an addon whose list
// empties is used by no one, never by everyone.
func TestADeletedGroupLeavesItsAddonsToNoOne(t *testing.T) {
	t.Parallel()
	s := usedByOn(t)
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	if _, err := s.store.DeleteGroup(t.Context(), s.friendsGroup.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.store.DeleteUser(t.Context(), s.boss.ID); err != nil {
		t.Fatal(err)
	}
	installed, err := s.addons.Find(t.Context(), s.friendsAddon.ID)
	if err != nil || !installed.UsedBy.Listed || len(installed.UsedBy.Groups)+len(installed.UsedBy.Users) != 0 {
		t.Fatalf("Friends Stream: %+v %v", installed.UsedBy, err)
	}
	if got := s.views(t, s.bobToken); len(got) != 0 {
		t.Errorf("bob's libraries: %v", got)
	}
	if got := s.views(t, s.aliceToken); !slices.Equal(got, []string{"Picks"}) {
		t.Errorf("alice's libraries: %v", got)
	}
}

// A version of an addon a user does not use is refused on every path, as
// a version that does not exist, even once a user who uses the addon
// listed it: no byte of it is read for them, and their own version of a
// file both addons list has no mirror from the other.
func TestALimitedAddonsVersionIsRefusedOnEveryPath(t *testing.T) {
	t.Parallel()
	s := usedByOn(t)
	movie := s.titles(t, s.aliceToken, "Picks")["Movie"]
	movieID, _ := accounts.ParseID(movie)
	versions, err := s.library.Versions(t.Context(), s.bob, movieID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("bob's versions: %v %v", files(versions), err)
	}
	two := accessVersion(t, versions, "Two")
	a := accessSetup{testServer: s.testServer, movie: movie, movieID: movieID}
	a.remuxableFile(t, two)
	mine, err := s.library.Versions(t.Context(), s.alice, movieID)
	if err != nil || len(mine) != 1 {
		t.Fatalf("alice's versions: %v %v", files(mine), err)
	}
	one := mine[0]
	a.remuxableFile(t, one)
	fetchedBefore := s.friends.files.Load()

	if _, err := s.library.Version(t.Context(), s.alice, movieID, two.ID); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("alice's Two: %v, want ErrNotFound", err)
	}
	if version, err := s.library.Version(t.Context(), s.alice, movieID, one.ID); err != nil || version.Origin.Addon != s.familyAddon.ID || len(version.Mirrors) != 0 {
		t.Errorf("alice's One: from %s with mirrors %v, %v; want Family Picks' link alone", version.Addon, files(version.Mirrors), err)
	}
	answer := s.playbackInfo(t, "tv", s.aliceToken, movie, map[string]any{"MediaSourceId": two.ID.String()})
	for _, source := range answer.MediaSources {
		if source.Id == two.ID.String() || strings.Contains(source.Name, "Two") {
			t.Errorf("PlaybackInfo offered alice %s", source.Name)
		}
	}
	refused := func(path string) {
		t.Helper()
		if status, body := s.call(http.MethodGet, path, app("tv", s.aliceToken), nil); status < 300 && !strings.Contains(string(body), "not found") {
			t.Errorf("%s served alice: %d %.80q", path, status, body)
		}
	}
	refused("/Videos/" + movie + "/stream?static=true&MediaSourceId=" + two.ID.String())
	refused("/Videos/" + movie + "/stream.mkv?static=true&tag=" + two.ID.String())
	refused("/Items/" + two.ID.String() + "/Download")
	refused("/Videos/" + movie + "/" + two.ID.String() + "/Attachments/0")
	for index := range 4 {
		path := "/Videos/" + movie + "/" + two.ID.String() + "/Subtitles/" + strconv.Itoa(index) + "/Stream.srt"
		if _, body := s.call(http.MethodGet, path, app("tv", s.aliceToken), nil); strings.Contains(string(body), "Friends") {
			t.Errorf("%s served alice Friends Stream's subtitle", path)
		}
	}
	oneHLS := a.hlsURL(t, "tv", s.aliceToken, one)
	if status, _, body := fetchText(t, oneHLS); status != http.StatusOK {
		t.Fatalf("alice's own conversion: %d %s", status, body)
	}
	twoHLS := a.withGrant(t, strings.ReplaceAll(oneHLS, one.ID.String(), two.ID.String()), s.alice, two)
	if status, _, body := fetchText(t, twoHLS); status < 400 {
		t.Errorf("alice's conversion of Two: %d %.80q", status, body)
	}
	if fetched := s.friends.files.Load(); fetched != fetchedBefore {
		t.Errorf("%d requests to Friends Stream for files for alice", fetched-fetchedBefore)
	}

	// Bob plays it.
	if status, body := s.call(http.MethodGet, "/Videos/"+movie+"/stream?static=true&MediaSourceId="+two.ID.String(), app("phone", s.bobToken), nil); status >= 300 {
		t.Errorf("bob's Two: %d %s", status, body)
	}
}

// What is kept of a user's listings is theirs alone: another user's
// listing of the same title, through other addons, neither gives them its
// versions, mirrors or subtitles, nor takes the mirrors theirs found; a
// draw of a library at random is drawn again once its addon is no longer
// theirs.
func TestKeptListingsDoNotLeakOneUsersViewToAnother(t *testing.T) {
	t.Parallel()
	s := usedByOn(t)
	movie := s.titles(t, s.aliceToken, "Picks")["Movie"]
	movieID, _ := accounts.ParseID(movie)
	// boss lists the movie first, then alice: the version of One remembered
	// keeps Friends Stream's link for boss.
	versions, err := s.library.Versions(t.Context(), s.boss, movieID)
	if err != nil || !slices.Equal(files(versions), []string{"One.mkv+Friends Stream", "Two.mkv"}) {
		t.Fatalf("boss's versions: %v %v", files(versions), err)
	}
	one := accessVersion(t, versions, "One")
	if _, err := s.library.Versions(t.Context(), s.alice, movieID); err != nil {
		t.Fatal(err)
	}
	if version, err := s.library.Version(t.Context(), s.boss, movieID, one.ID); err != nil || len(version.Mirrors) != 1 ||
		version.Mirrors[0].Origin.Addon != s.friendsAddon.ID {
		t.Errorf("boss's One after alice's listing: mirrors %v, %v; want Friends Stream's", files(version.Mirrors), err)
	}
	if version, err := s.library.Version(t.Context(), s.alice, movieID, one.ID); err != nil || len(version.Mirrors) != 0 {
		t.Errorf("alice's One: mirrors %v, %v; want none", files(version.Mirrors), err)
	}

	// bob's PlaybackInfo describes the subtitles for him; alice asks for
	// them by index.
	s.playbackInfo(t, "phone", s.bobToken, movie, nil)
	for index := range 4 {
		path := "/Videos/" + movie + "/" + one.ID.String() + "/Subtitles/" + strconv.Itoa(index) + "/Stream.srt"
		if _, body := s.call(http.MethodGet, path, app("tv", s.aliceToken), nil); strings.Contains(string(body), "Friends") {
			t.Errorf("%s served alice Friends Stream's subtitle", path)
		}
	}

	// alice draws Picks at random; Family Picks then goes to Friends alone.
	views := map[string]string{}
	var result QueryResult
	s.get(t, "/UserViews", s.aliceToken, &result)
	for _, view := range result.Items {
		views[view.Name] = view.Id
	}
	random := "/Items?ParentId=" + views["Picks"] + "&SortBy=Random&Recursive=true&IncludeItemTypes=Movie"
	var drawn QueryResult
	if status := s.get(t, random, s.aliceToken, &drawn); status != http.StatusOK || len(drawn.Items) != 2 {
		t.Fatalf("alice's draw: %d %+v", status, drawn.Items)
	}
	s.useBy(t, s.familyAddon, addons.UsedBy{Listed: true, Groups: []accounts.ID{s.friendsGroup.ID}})
	drawn = QueryResult{}
	s.get(t, random, s.aliceToken, &drawn)
	if len(drawn.Items) != 0 {
		t.Errorf("alice's draw once Family Picks is not hers: %+v", drawn.Items)
	}
}

// A share link plays as its sharer, and a DLNA device as its user: once
// the addon of the title is not theirs, neither plays it.
func TestShareLinksAndDLNADevicesFollowTheirUsersAddons(t *testing.T) {
	t.Parallel()
	s := sharing(t)
	token, _ := s.link(t, 1)
	d := shareOverDLNA(t, s.testServer)
	d.playbackSetup = s.playbackSetup
	d.browse(t, "0", "BrowseDirectChildren", 0, 0)
	d.show(t, s.user)
	d.force(t, takingProfile)
	before, _ := parseDIDL(t, d.browse(t, "0", "BrowseDirectChildren", 0, 0).didl)
	_, address := d.movieRes(t)

	installed, err := s.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("addons: %v %v", installed, err)
	}
	if _, err := s.addons.SetUsedBy(t.Context(), installed[0].ID, addons.UsedBy{Listed: true}); err != nil {
		t.Fatal(err)
	}
	if status, session := s.start(t, token); status == http.StatusOK {
		t.Errorf("a guest of the link started %s", session)
	}
	if response, _ := fetch(t, address, nil); response.StatusCode < 400 {
		t.Errorf("the DLNA device's movie: %d", response.StatusCode)
	}
	after, _ := parseDIDL(t, d.browse(t, "0", "BrowseDirectChildren", 0, 0).didl)
	if len(after) != len(before)-1 {
		t.Errorf("the DLNA device's views: %v, then %v", titles(before), titles(after))
	}
}

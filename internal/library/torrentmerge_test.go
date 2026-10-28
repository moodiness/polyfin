package library

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// sameFileEnv serves a movie whose file a torrent addon, installed first,
// lists as a torrent, and a link addon, after it, as a link: the same file
// name and size. Torrents are on; the member may not play them, the
// administrator may.
func sameFileEnv(t *testing.T) (env, accounts.ID) {
	t.Helper()
	e := newEnv(t)
	movies := titles("movie", 1)
	index := 0
	hints := stremio.StreamBehavior{Filename: "Some.Movie.2026.1080p.mkv", VideoSize: 2_000_000_000}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "torrent", Name: "Torrent", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0]},
		streams: map[string][]stremio.Stream{"movie/" + movies[0].ID: {{Name: "Torrent 1080p",
			InfoHash: "0123456789abcdef0123456789abcdef01234567", FileIdx: &index, BehaviorHints: hints}}},
	})
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "link", Name: "Link", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "stream"}}},
		streams: map[string][]stremio.Stream{"movie/" + movies[0].ID: {{Name: "Link 1080p", URL: "https://cdn.example/some.mkv",
			BehaviorHints: hints}}},
	})
	e.setting(func(s *accounts.Settings) { s.TorrentStreams, s.TorrentProxy = true, "127.0.0.1:1080" })
	if _, err := e.users.UpdateUser(t.Context(), e.admin.ID, accounts.UserChanges{TorrentStreams: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	admin, err := e.users.User(t.Context(), e.admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	e.admin = admin
	return e, e.children(e.member, "Top", 0, 10).Items[0].ID
}

// sameFileVersions lists the names of the versions of movie for user.
func sameFileVersions(t *testing.T, e env, user accounts.User, movie accounts.ID) ([]string, []Version) {
	t.Helper()
	versions, err := e.service.Versions(t.Context(), user, movie)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, version := range versions {
		names = append(names, version.Name)
	}
	return names, versions
}

// A torrent the user may not play leaves its place to a link to the same
// file listed after it; a user who may play torrents gets the torrent, the
// first listed, as one version.
func TestALinkToTheFileOfATorrentPlaysForUsersWithoutTorrents(t *testing.T) {
	t.Parallel()
	e, movie := sameFileEnv(t)
	if got, _ := sameFileVersions(t, e, e.member, movie); !slices.Equal(got, []string{"Link 1080p"}) {
		t.Errorf("without torrents: %q", got)
	}
	if got, _ := sameFileVersions(t, e, e.admin, movie); !slices.Equal(got, []string{"Torrent 1080p"}) {
		t.Errorf("with torrents: %q", got)
	}
}

// The version a user who may play torrents listed last, a torrent, is
// remembered under the file's identifier: another user who may not play
// torrents still finds their link under it, and the first user keeps the
// torrent.
func TestARememberedTorrentDoesNotHideTheLinkOfAnotherUser(t *testing.T) {
	t.Parallel()
	e, movie := sameFileEnv(t)
	_, links := sameFileVersions(t, e, e.member, movie)
	_, torrents := sameFileVersions(t, e, e.admin, movie)
	if len(links) != 1 || len(torrents) != 1 || links[0].ID != torrents[0].ID {
		t.Fatalf("versions: %+v %+v", links, torrents)
	}
	id := links[0].ID
	if version, err := e.service.Version(t.Context(), e.member, movie, id); err != nil || version.Torrent != nil || version.Name != "Link 1080p" {
		t.Errorf("the member's version: %+v %v", version, err)
	}
	if version, err := e.service.Version(t.Context(), e.admin, movie, id); err != nil || version.Torrent == nil {
		t.Errorf("the administrator's version after the member's: %+v %v", version, err)
	}
}

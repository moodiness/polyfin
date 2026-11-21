package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A torrent version of a movie that names no file carries the movie's
// title and year, which tell its file in a pack of several movies (see
// torrent.Info.File).
func TestATorrentVersionOfAMovieCarriesItsTitleAndYear(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	movies := titles("movie", 1)
	movies[0].Name, movies[0].ReleaseInfo = "Harbor", "2021"
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "torrent", Name: "Torrent", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0]},
		streams: map[string][]stremio.Stream{"movie/" + movies[0].ID: {{Name: "Pack 1080p",
			InfoHash: "0123456789abcdef0123456789abcdef01234567"}}},
	})
	e.setting(func(s *accounts.Settings) { s.TorrentStreams, s.TorrentProxy = true, "127.0.0.1:1080" })
	if _, err := e.users.UpdateUser(t.Context(), e.admin.ID, accounts.UserChanges{TorrentStreams: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	admin, err := e.users.User(t.Context(), e.admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	movie := e.children(admin, "Top", 0, 10).Items[0].ID
	versions, err := e.service.Versions(t.Context(), admin, movie)
	if err != nil || len(versions) != 1 || versions[0].Torrent == nil {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	if got := versions[0].Torrent; got.Title != "Harbor" || got.Year != 2021 || got.Episode != 0 {
		t.Errorf("the torrent: %+v, want the movie Harbor of 2021", *got)
	}
}

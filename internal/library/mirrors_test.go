package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// mirrorEnv serves a movie whose file two addons list as a link each,
// under the same name and size, the first one also listing a file of its
// own.
func mirrorEnv(t *testing.T) (env, accounts.ID, *fakeAddon) {
	t.Helper()
	e := newEnv(t)
	movies := titles("movie", 1)
	hints := stremio.StreamBehavior{Filename: "Some.Movie.2026.1080p.mkv", VideoSize: 2_000_000_000}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "first", Name: "First", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0]},
		streams: map[string][]stremio.Stream{"movie/" + movies[0].ID: {
			{Name: "First 1080p", URL: "https://one.example/some.mkv", BehaviorHints: hints},
			{Name: "First 720p", URL: "https://one.example/other.mkv",
				BehaviorHints: stremio.StreamBehavior{Filename: "Some.Movie.2026.720p.mkv", VideoSize: 900_000_000}},
		}},
	})
	second := &fakeAddon{
		manifest: stremio.Manifest{ID: "second", Name: "Second", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "stream"}}},
		streams: map[string][]stremio.Stream{"movie/" + movies[0].ID: {{Name: "Second 1080p", URL: "https://two.example/token/some.mkv",
			BehaviorHints: hints}}},
	}
	e.install(addons.Shared(), second)
	return e, e.children(e.member, "Top", 0, 10).Items[0].ID, second
}

// The same file listed by another addon is a mirror of the first listing,
// hidden from the versions, and kept with the version remembered by its
// identifier, which playback reads.
func TestAnotherAddonsCopyOfAFileIsAMirror(t *testing.T) {
	t.Parallel()
	e, movie, _ := mirrorEnv(t)
	names, versions := sameFileVersions(t, e, e.member, movie)
	if len(names) != 2 || names[0] != "First 1080p" || names[1] != "First 720p" {
		t.Fatalf("versions: %q", names)
	}
	mirrors := versions[0].Mirrors
	if len(mirrors) != 1 || mirrors[0].URL != "https://two.example/token/some.mkv" || mirrors[0].Addon != "Second" || mirrors[0].ID != versions[0].ID {
		t.Fatalf("mirrors: %+v", mirrors)
	}
	if len(versions[1].Mirrors) != 0 {
		t.Errorf("a file listed once has mirrors: %+v", versions[1].Mirrors)
	}
	known, err := e.service.Version(t.Context(), e.member, movie, versions[0].ID)
	if err != nil || len(known.Mirrors) != 1 || known.Mirrors[0].URL != mirrors[0].URL {
		t.Errorf("the version remembered: %+v %v", known.Mirrors, err)
	}
}

// A mirror's link renewed takes its place among the mirrors: the version
// keeps its own link, and its mirrors.
func TestRenewingAMirrorKeepsTheVersion(t *testing.T) {
	t.Parallel()
	e, movie, second := mirrorEnv(t)
	_, versions := sameFileVersions(t, e, e.member, movie)
	if len(versions) == 0 || len(versions[0].Mirrors) != 1 {
		t.Fatalf("versions: %+v", versions)
	}
	second.mu.Lock()
	for key, streams := range second.streams {
		streams[0].URL = "https://two.example/fresh/some.mkv"
		second.streams[key] = streams
	}
	second.mu.Unlock()
	renewed, err := e.service.Renew(t.Context(), versions[0].Mirrors[0])
	if err != nil || renewed.URL != "https://two.example/fresh/some.mkv" {
		t.Fatalf("renewed: %+v %v", renewed, err)
	}
	known, err := e.service.Version(t.Context(), e.member, movie, versions[0].ID)
	if err != nil || known.URL != "https://one.example/some.mkv" || len(known.Mirrors) != 1 ||
		known.Mirrors[0].URL != "https://two.example/fresh/some.mkv" {
		t.Errorf("the version remembered: %s %+v %v", known.URL, known.Mirrors, err)
	}
}

// A torrent has no mirror and is none: the link to its file is a version
// of its own for a user without torrents, without mirrors.
func TestATorrentHasNoMirror(t *testing.T) {
	t.Parallel()
	e, movie := sameFileEnv(t)
	if _, versions := sameFileVersions(t, e, e.admin, movie); len(versions) != 1 || len(versions[0].Mirrors) != 0 {
		t.Errorf("with torrents: %+v", versions)
	}
	if _, versions := sameFileVersions(t, e, e.member, movie); len(versions) != 1 || len(versions[0].Mirrors) != 0 {
		t.Errorf("without torrents: %+v", versions)
	}
}

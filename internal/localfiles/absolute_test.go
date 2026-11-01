package localfiles

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// harborTales is a series whose catalog lists a special, 40 episodes in
// its first season and 12 in its second, each titled after its absolute
// number: "Chapter 41" is the second season's first.
func harborTales(seasons ...int) stremio.Meta {
	meta := stremio.Meta{ID: "tt0700003", Type: "series", Name: "Harbor Tales", ImdbID: "tt0700003",
		Videos: []stremio.Video{{ID: "tt0700003:0:1", Title: "Special", Season: 0, Episode: 1}}}
	absolute := 0
	for season, count := range seasons {
		for number := 1; number <= count; number++ {
			absolute++
			meta.Videos = append(meta.Videos, stremio.Video{ID: fmt.Sprintf("tt0700003:%d:%d", season+1, number),
				Title: fmt.Sprintf("Chapter %d", absolute), Season: stremio.Number(season + 1), Episode: stremio.Number(number)})
		}
	}
	return meta
}

// newTalesAddon is a metadata addon describing harborTales(40, 12).
func newTalesAddon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.EscapedPath(); {
		case p == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "tales", Name: "Tales", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search"}}}}})
		case strings.HasPrefix(p, "/catalog/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
		case p == "/meta/series/tt0700003.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": harborTales(40, 12)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// streamFiles names the files a folder answers for an episode's video
// identifier.
func (e env) streamFiles(folder accounts.ID, id string) []string {
	e.t.Helper()
	streams, err := e.service.Streams(e.t.Context(), folder, id)
	if err != nil {
		e.t.Fatal(err)
	}
	var names []string
	for _, stream := range streams {
		names = append(names, stream.BehaviorHints.Filename)
	}
	return names
}

// numberings reads how a folder's files were numbered, by name.
func (e env) numberings(folder accounts.ID) map[string]string {
	e.t.Helper()
	rows, err := e.pool.Query(e.t.Context(), "SELECT path, numbering FROM local_files WHERE addon_id = $1", folder)
	if err != nil {
		e.t.Fatal(err)
	}
	result := map[string]string{}
	for rows.Next() {
		var rel, numbering string
		if err := rows.Scan(&rel, &numbering); err != nil {
			e.t.Fatal(err)
		}
		result[path.Base(rel)] = numbering
	}
	return result
}

// A show's file named by an absolute number, in a season folder or at the
// top of its show's folder, is a version of the catalog's episode of that
// place, specials left out, in that episode's season and number; one whose
// number the catalog does not reach is an episode of its own, named after
// it and counted on the folder's page. Names telling a season and an
// episode keep them, and a year is no episode's number.
func TestAbsoluteNamesAreTheCatalogsEpisodes(t *testing.T) {
	t.Parallel()
	e := newEnvWith(t, newTalesAddon(t))
	ctx := t.Context()
	admin, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	addon, _ := e.folder(KindShows, map[string]int{
		"Harbor Tales [imdbid-tt0700003]/Season 02/Harbor Tales - 41.mkv":        10,
		"Harbor Tales [imdbid-tt0700003]/Harbor Tales 12.mkv":                    10,
		"Harbor Tales [imdbid-tt0700003]/[Group] Harbor Tales - 052 [1080p].mkv": 10,
		"Harbor Tales [imdbid-tt0700003]/Harbor Tales - 60.mkv":                  10,
		"Harbor Tales [imdbid-tt0700003]/Harbor Tales 2019.mkv":                  10,
		"Harbor Tales [imdbid-tt0700003]/Specials/Harbor Tales - 01.mkv":         10,
		"Harbor Tales [imdbid-tt0700003]/Season 01/Harbor Tales S01E05.mkv":      10,
		"Harbor Tales [imdbid-tt0700003]/Season 01/Harbor Tales S01E07 - 99.mkv": 10,
		"Harbor Tales [imdbid-tt0700003]/Season 02/Harbor Tales 2x03.mkv":        10,
		"Harbor Tales [imdbid-tt0700003]/Season 02/Harbor Tales Episode 4.mkv":   10,
	})
	files := e.files(addon.ID)
	if got := files["Harbor Tales [imdbid-tt0700003]/Season 02/Harbor Tales - 41.mkv"]; got != "tt0700003" {
		t.Fatalf("the show's files: %v", files)
	}
	if got := files["Harbor Tales [imdbid-tt0700003]/Harbor Tales 2019.mkv"]; got != "unmatched: "+reasonUnreadableName {
		t.Errorf("a year read as an episode: %s", got)
	}

	for id, want := range map[string]string{
		"tt0700003:2:1":  "Harbor Tales - 41.mkv",
		"tt0700003:1:12": "Harbor Tales 12.mkv",
		"tt0700003:2:12": "[Group] Harbor Tales - 052 [1080p].mkv",
		"tt0700003:1:60": "Harbor Tales - 60.mkv",
		"tt0700003:0:1":  "Harbor Tales - 01.mkv",
		"tt0700003:1:5":  "Harbor Tales S01E05.mkv",
		"tt0700003:1:7":  "Harbor Tales S01E07 - 99.mkv",
		"tt0700003:2:3":  "Harbor Tales 2x03.mkv",
		"tt0700003:2:4":  "Harbor Tales Episode 4.mkv",
	} {
		if got := e.streamFiles(addon.ID, id); len(got) != 1 || got[0] != want {
			t.Errorf("the versions of %s: %v, want %s", id, got, want)
		}
	}
	for _, id := range []string{"tt0700003:2:41", "tt0700003:1:41", "tt0700003:2:52", "tt0700003:1:99"} {
		if got := e.streamFiles(addon.ID, id); len(got) != 0 {
			t.Errorf("the versions of %s: %v", id, got)
		}
	}
	if got := e.numberings(addon.ID); got["Harbor Tales - 41.mkv"] != numberingAbsolute || got["Harbor Tales - 60.mkv"] != numberingAbsoluteBeyond ||
		got["Harbor Tales S01E05.mkv"] != "" || got["Harbor Tales - 01.mkv"] != "" {
		t.Errorf("the files' numbering: %v", got)
	}

	// The series lists the catalog's episodes, the file it does not reach
	// as an episode of its own, named after it.
	show, ok := e.shelfSeries(admin)["Harbor Tales"]
	if !ok {
		t.Fatalf("the folder's series: %v", e.shelfSeries(admin))
	}
	episodes, err := e.library.Episodes(ctx, admin, show.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := numbered(episodes)
	if len(names) != 1+40+12+1 || names[[2]int{2, 1}] != "Chapter 41" || names[[2]int{1, 60}] != "Harbor Tales - 60" {
		t.Errorf("the show's episodes: %d %v", len(names), names)
	}
	folder, err := e.service.Folder(ctx, addon.ID)
	if err != nil || folder.Unlisted != 1 || folder.Absolute != 3 {
		t.Errorf("the episodes the catalog does not list, the files matched by their absolute number: %d %d %v", folder.Unlisted,
			folder.Absolute, err)
	}
}

// The catalog a series is described by places its files named by an
// absolute number each time it is: one listing more episodes than when the
// files were scanned, as of a show still airing, takes those it then
// reaches, and one listing fewer leaves them episodes of their own again.
func TestAbsoluteNamesFollowTheCatalog(t *testing.T) {
	t.Parallel()
	e := newEnvWith(t, newTalesAddon(t))
	ctx := t.Context()
	addon, _ := e.folder(KindShows, map[string]int{
		"Harbor Tales [imdbid-tt0700003]/Harbor Tales - 41.mkv": 10,
		"Harbor Tales [imdbid-tt0700003]/Harbor Tales - 60.mkv": 10,
	})
	if got := e.streamFiles(addon.ID, "tt0700003:1:60"); len(got) != 1 {
		t.Fatalf("the file the catalog does not reach: %v", got)
	}
	held := func(catalog stremio.Meta) map[[2]int]bool {
		t.Helper()
		videos, err := e.service.Episodes(ctx, []accounts.ID{addon.ID}, "tt0700003", catalog.Videos)
		if err != nil {
			t.Fatal(err)
		}
		numbers := map[[2]int]bool{}
		for _, video := range videos {
			numbers[[2]int{int(video.Season), video.EpisodeNumber()}] = true
		}
		return numbers
	}
	if got := held(harborTales(40, 12, 10)); len(got) != 2 || !got[[2]int{2, 1}] || !got[[2]int{3, 8}] {
		t.Errorf("the files, the catalog grown: %v", got)
	}
	if got := e.streamFiles(addon.ID, "tt0700003:3:8"); len(got) != 1 || got[0] != "Harbor Tales - 60.mkv" {
		t.Errorf("the versions of the episode the catalog now lists: %v", got)
	}
	if got := e.numberings(addon.ID); got["Harbor Tales - 60.mkv"] != numberingAbsolute {
		t.Errorf("the files' numbering: %v", got)
	}
	if got := held(harborTales(30)); len(got) != 2 || !got[[2]int{1, 41}] || !got[[2]int{1, 60}] {
		t.Errorf("the files, the catalog shrunk: %v", got)
	}
	if got := e.streamFiles(addon.ID, "tt0700003:3:8"); len(got) != 0 {
		t.Errorf("the versions of an episode no longer listed: %v", got)
	}
}

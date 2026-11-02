package localfiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
	"github.com/moodiness/polyfin/internal/userdata"
)

// catalogShow is a series whose catalog lists, in season i+1, the episodes
// seasons[i] runs from and to, each titled after its season and number.
func catalogShow(id, name string, seasons ...[2]int) stremio.Meta {
	meta := stremio.Meta{ID: id, Type: "series", Name: name, ImdbID: id}
	for i, run := range seasons {
		for number := run[0]; number <= run[1]; number++ {
			meta.Videos = append(meta.Videos, stremio.Video{ID: fmt.Sprintf("%s:%d:%d", id, i+1, number),
				Title: fmt.Sprintf("Catalog S%dE%d", i+1, number), Season: stremio.Number(i + 1), Episode: stremio.Number(number)})
		}
	}
	return meta
}

// newShowsAddon is a metadata addon describing metas.
func newShowsAddon(t *testing.T, metas ...stremio.Meta) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.EscapedPath(); {
		case p == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "shows", Name: "Shows", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search"}}}}})
		case strings.HasPrefix(p, "/catalog/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
		case strings.HasPrefix(p, "/meta/series/"):
			id := strings.TrimSuffix(strings.TrimPrefix(p, "/meta/series/"), ".json")
			for _, meta := range metas {
				if meta.ID == id {
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

// numberingCase is a show of a folder whose files are numbered otherwise
// than its catalog, and a user who played it.
type numberingCase struct {
	env
	t      *testing.T
	folder addons.Addon
	dir    string
	id     string
	admin  accounts.User
	user   accounts.User
	data   *userdata.Store
}

func newNumberingCase(t *testing.T, e env, id string, files map[string]int) numberingCase {
	t.Helper()
	ctx := t.Context()
	admin, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	user, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	folder, dir := e.folder(KindShows, files)
	return numberingCase{env: e, t: t, folder: folder, dir: dir, id: id, admin: admin, user: user, data: userdata.New(e.pool)}
}

// episode is the item of the show's episode of season and number.
func (c numberingCase) episode(season, number int) accounts.ID {
	return library.EpisodeItemID(episodeVideoID(c.id, season, number))
}

func (c numberingCase) set(season, number int, change func(*userdata.Data)) {
	c.t.Helper()
	if _, err := c.data.Change(c.t.Context(), c.user.ID, []userdata.Item{{ID: c.episode(season, number)}}, change); err != nil {
		c.t.Fatal(err)
	}
}

func (c numberingCase) get(season, number int) userdata.Data {
	c.t.Helper()
	got, err := c.data.Get(c.t.Context(), c.user.ID, []accounts.ID{c.episode(season, number)})
	if err != nil {
		c.t.Fatal(err)
	}
	return got[c.episode(season, number)]
}

// versions names the files a stream request for the show's episode of
// season and number gets.
func (c numberingCase) versions(season, number int) []string {
	c.t.Helper()
	return c.streamFiles(c.folder.ID, fmt.Sprintf("%s:%d:%d", c.id, season, number))
}

// expect checks that each episode, "S02E13", has the file named as its
// only version, an empty name for none.
func (c numberingCase) expect(when string, want map[[2]int]string) {
	c.t.Helper()
	for place, name := range want {
		got := c.versions(place[0], place[1])
		if name == "" && len(got) != 0 || name != "" && (len(got) != 1 || got[0] != name) {
			c.t.Errorf("%s, the versions of S%02dE%02d: %v, want %q", when, place[0], place[1], got, name)
		}
	}
}

// episodes lists the show's episodes as the folder's library does, by
// season and number.
func (c numberingCase) episodes(name string) map[[2]int]string {
	c.t.Helper()
	show, ok := c.shelfSeries(c.admin)[name]
	if !ok {
		c.t.Fatalf("the folder's series: %v", c.shelfSeries(c.admin))
	}
	episodes, err := c.library.Episodes(c.t.Context(), c.admin, show.ID, nil)
	if err != nil {
		c.t.Fatal(err)
	}
	return numbered(episodes)
}

func (c numberingCase) choose(choice ShowNumbering) {
	c.t.Helper()
	if err := c.service.SetNumbering(c.t.Context(), c.folder.ID, c.id, choice); err != nil {
		c.t.Fatal(err)
	}
}

// show reads the show as the folder's page lists it.
func (c numberingCase) show() FolderShow {
	c.t.Helper()
	shows, err := c.service.Shows(c.t.Context(), c.folder.ID)
	if err != nil {
		c.t.Fatal(err)
	}
	for _, show := range shows {
		if show.StremioID == c.id {
			return show
		}
	}
	c.t.Fatalf("the folder's shows: %+v", shows)
	return FolderShow{}
}

// Files numbered per season, of a show whose catalog numbers on from
// season 1, become versions of the catalog's episodes once an
// administrator says so: S02E01 is the catalog's S02E13. What users did
// with the episodes the files were follows them, a file the catalog's
// season does not reach stays an episode of its own, after the catalog's,
// with its reason, and numbering them as the catalog again puts them back.
func TestShowFilesNumberedPerSeason(t *testing.T) {
	t.Parallel()
	const id = "tt0700011"
	e := newEnvWith(t, newShowsAddon(t, catalogShow(id, "Onward Show", [2]int{1, 12}, [2]int{13, 24})))
	c := newNumberingCase(t, e, id, map[string]int{
		"Onward Show [imdbid-tt0700011]/Season 01/Onward Show S01E05.mkv":     10,
		"Onward Show [imdbid-tt0700011]/Season 02/Onward Show S02E01.mkv":     10,
		"Onward Show [imdbid-tt0700011]/Season 02/Onward Show S02E02.mkv":     10,
		"Onward Show [imdbid-tt0700011]/Season 02/Onward Show S02E03-E04.mkv": 10,
		"Onward Show [imdbid-tt0700011]/Season 02/Onward Show S02E15.mkv":     10,
	})
	asNamed := map[[2]int]string{
		{1, 5}: "Onward Show S01E05.mkv", {2, 1}: "Onward Show S02E01.mkv", {2, 2}: "Onward Show S02E02.mkv",
		{2, 3}: "Onward Show S02E03-E04.mkv", {2, 4}: "Onward Show S02E03-E04.mkv", {2, 15}: "Onward Show S02E15.mkv",
		{2, 13}: "", {2, 14}: "", {2, 16}: "", {2, 25}: "",
	}
	c.expect("as the catalog", asNamed)
	if got := c.episodes("Onward Show"); len(got) != 24+4 {
		t.Fatalf("the show's episodes as the catalog: %d %v", len(got), got)
	}
	played := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	c.set(2, 1, func(d *userdata.Data) { d.MarkPlayed(&played, played) })
	c.set(2, 2, func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 10*time.Minute, time.Hour, &played })
	c.set(2, 3, func(d *userdata.Data) { d.Favorite = true })

	c.choose(ShowNumbering{Numbering: NumberedPerSeason})
	c.expect("per season", map[[2]int]string{
		{1, 5}: "Onward Show S01E05.mkv", {2, 13}: "Onward Show S02E01.mkv", {2, 14}: "Onward Show S02E02.mkv",
		{2, 15}: "Onward Show S02E03-E04.mkv", {2, 16}: "Onward Show S02E03-E04.mkv", {2, 25}: "Onward Show S02E15.mkv",
		{2, 1}: "", {2, 2}: "", {2, 3}: "", {2, 4}: "",
	})
	names := c.episodes("Onward Show")
	if len(names) != 24+1 || names[[2]int{2, 13}] != "Catalog S2E13" || names[[2]int{2, 25}] != "Onward Show S02E15" {
		t.Errorf("the show's episodes per season: %d %v", len(names), names)
	}
	if d := c.get(2, 13); !d.Played {
		t.Errorf("the played mark, with its file: %+v", d)
	}
	if d := c.get(2, 14); d.Position != 10*time.Minute {
		t.Errorf("the resume point, with its file: %+v", d)
	}
	if d := c.get(2, 2); d.Position != 0 {
		t.Errorf("the resume point, moved: %+v", d)
	}
	if d := c.get(2, 15); !d.Favorite {
		t.Errorf("the favorite, with its file: %+v", d)
	}
	show := c.show()
	if show.Numbering.Numbering != NumberedPerSeason || show.Files != 5 || show.Mapped != 4 || len(show.Unmapped) != 1 ||
		show.Unmapped[0] != (UnmappedFile{Path: "Onward Show [imdbid-tt0700011]/Season 02/Onward Show S02E15.mkv", Season: 2, First: 15, Last: 15,
			Listed: [3]int{2, 25, 25}, Reason: unmappedOutsideSeason}) {
		t.Errorf("the show on the folder's page: %+v", show)
	}
	if folder, err := c.service.Folder(t.Context(), c.folder.ID); err != nil || folder.Unlisted != 1 {
		t.Errorf("the episodes only the files hold: %d %v", folder.Unlisted, err)
	}

	c.choose(ShowNumbering{})
	c.expect("as the catalog again", asNamed)
	if d := c.get(2, 2); d.Position != 10*time.Minute {
		t.Errorf("the resume point, back with its file: %+v", d)
	}
	if got := c.show(); got.Numbering != (ShowNumbering{}) || got.Mapped != 0 || len(got.Unmapped) != 0 {
		t.Errorf("the show numbered as the catalog: %+v", got)
	}
	if err := c.service.SetNumbering(t.Context(), c.folder.ID, c.id, ShowNumbering{Numbering: "tvdb"}); !errors.Is(err, ErrInvalidNumbering) {
		t.Errorf("another numbering: %v", err)
	}
	if err := c.service.SetNumbering(t.Context(), c.folder.ID, "tt0700099", ShowNumbering{Numbering: NumberedPerSeason}); !errors.Is(err,
		addons.ErrNotFound) {
		t.Errorf("a show the folder does not hold: %v", err)
	}
}

// Files continuing from season 1, of a show whose catalog numbers each
// season from 1, become versions of the catalog's episodes: S02E13, after
// 12 episodes in season 1, is the catalog's S02E01, as a file scanned
// later is. One past the catalog's season stays an episode of its own.
func TestShowFilesContinuingFromSeason1(t *testing.T) {
	t.Parallel()
	const id = "tt0700012"
	e := newEnvWith(t, newShowsAddon(t, catalogShow(id, "Split Show", [2]int{1, 12}, [2]int{1, 40})))
	c := newNumberingCase(t, e, id, map[string]int{
		"Split Show [imdbid-tt0700012]/Season 01/Split Show S01E03.mkv": 10,
		"Split Show [imdbid-tt0700012]/Season 02/Split Show S02E13.mkv": 10,
		"Split Show [imdbid-tt0700012]/Season 02/Split Show - 14.mkv":   10,
		"Split Show [imdbid-tt0700012]/Season 02/Split Show S02E52.mkv": 10,
		"Split Show [imdbid-tt0700012]/Season 02/Split Show S02E53.mkv": 10,
	})
	c.expect("as the catalog", map[[2]int]string{{2, 13}: "Split Show S02E13.mkv", {2, 52}: "Split Show S02E52.mkv", {2, 1}: ""})
	played := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	c.set(2, 52, func(d *userdata.Data) { d.MarkPlayed(&played, played) })

	c.choose(ShowNumbering{Numbering: NumberedContinuing})
	c.expect("continuing", map[[2]int]string{
		{1, 3}: "Split Show S01E03.mkv", {2, 1}: "Split Show S02E13.mkv", {2, 2}: "Split Show - 14.mkv",
		{2, 40}: "Split Show S02E52.mkv", {2, 53}: "Split Show S02E53.mkv", {2, 13}: "", {2, 52}: "",
	})
	names := c.episodes("Split Show")
	if len(names) != 52+1 || names[[2]int{2, 1}] != "Catalog S2E1" || names[[2]int{2, 53}] != "Split Show S02E53" {
		t.Errorf("the show's episodes continuing: %d %v", len(names), names)
	}
	if d := c.get(2, 40); !d.Played {
		t.Errorf("the played mark, with its file: %+v", d)
	}
	if show := c.show(); len(show.Unmapped) != 1 || show.Unmapped[0].Reason != unmappedOutsideSeason || show.Unmapped[0].Listed != [3]int{2, 53, 53} {
		t.Errorf("the file past the catalog's season: %+v", show)
	}

	write(t, c.dir+"/Split Show [imdbid-tt0700012]/Season 02/Split Show S02E20.mkv", 10)
	if err := c.service.Scan(t.Context(), c.folder.ID); err != nil {
		t.Fatal(err)
	}
	c.expect("a file scanned later", map[[2]int]string{{2, 8}: "Split Show S02E20.mkv", {2, 20}: "", {2, 1}: "Split Show S02E13.mkv"})
}

// Files numbered on across all seasons, or split into seasons otherwise
// than the catalog, are read as absolute places: a season starting again
// counts on from the previous one, and a name telling an absolute number
// alone keeps it. One the catalog does not reach stays an episode of its
// own.
func TestShowFilesNumberedAbsolute(t *testing.T) {
	t.Parallel()
	const id = "tt0700013"
	e := newEnvWith(t, newShowsAddon(t, catalogShow(id, "Long Show", [2]int{1, 12}, [2]int{1, 12})))
	c := newNumberingCase(t, e, id, map[string]int{
		"Long Show [imdbid-tt0700013]/Season 01/Long Show S01E10.mkv": 10,
		"Long Show [imdbid-tt0700013]/Season 01/Long Show S01E14.mkv": 10,
		"Long Show [imdbid-tt0700013]/Season 03/Long Show S03E01.mkv": 10,
		"Long Show [imdbid-tt0700013]/Season 03/Long Show S03E11.mkv": 10,
		"Long Show [imdbid-tt0700013]/Long Show - 20.mkv":             10,
	})
	c.expect("as the catalog", map[[2]int]string{{1, 14}: "Long Show S01E14.mkv", {3, 1}: "Long Show S03E01.mkv", {2, 8}: "Long Show - 20.mkv"})

	c.choose(ShowNumbering{Numbering: NumberedAbsolute})
	c.expect("absolute", map[[2]int]string{
		{1, 10}: "Long Show S01E10.mkv", {2, 2}: "Long Show S01E14.mkv", {2, 3}: "Long Show S03E01.mkv", {2, 8}: "Long Show - 20.mkv",
		{3, 11}: "Long Show S03E11.mkv", {1, 14}: "", {3, 1}: "",
	})
	names := c.episodes("Long Show")
	if len(names) != 24+1 || names[[2]int{3, 11}] != "Long Show S03E11" {
		t.Errorf("the show's episodes, absolute: %d %v", len(names), names)
	}
	if show := c.show(); len(show.Unmapped) != 1 || show.Unmapped[0].Reason != unmappedBeyondCatalog {
		t.Errorf("the file the catalog does not reach: %+v", show)
	}
}

// Files numbered in one of the series' TMDB episode groups are versions
// of the catalog's episodes the group holds at their seasons and numbers.
// A file at a place the group leaves empty, or whose episode the catalog
// does not list, stays an episode of its own, never a version of the
// catalog's episode of its numbers: after the season's other episodes.
func TestShowFilesNumberedInAnEpisodeOrder(t *testing.T) {
	t.Parallel()
	const id = "tt0700014"
	show := catalogShow(id, "Volume Show", [2]int{1, 3}, [2]int{1, 2})
	show.TmdbID = "77"
	e := newEnvWith(t, newShowsAddon(t, show))
	episode := func(season, number, order int) map[string]any {
		return map[string]any{"show_id": 77, "season_number": season, "episode_number": number, "order": order}
	}
	answers := map[string]any{
		"/3/tv/77/episode_groups": map[string]any{"results": []map[string]any{
			{"id": "dvd", "name": "DVD", "type": 3, "episode_count": 5, "group_count": 2},
		}},
		"/3/tv/episode_group/dvd": map[string]any{"id": "dvd", "name": "DVD", "type": 3, "groups": []map[string]any{
			{"name": "Volume 1", "order": 1, "episodes": []any{episode(1, 2, 0), episode(1, 1, 1), episode(2, 1, 2)}},
			{"name": "Volume 2", "order": 2, "episodes": []any{episode(1, 3, 0), episode(9, 9, 1)}},
		}},
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer, ok := answers[r.URL.Path]
		if !ok || r.URL.Query().Get("api_key") != "k3y" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(api.Close)
	e.library.UseTMDB(tmdb.New(api.URL+"/3", api.URL+"/t/p/", "test", e.users.Settings))
	settings := e.users.Settings()
	settings.TMDBKey = "k3y"
	if _, err := e.users.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	c := newNumberingCase(t, e, id, map[string]int{
		"Volume Show [imdbid-tt0700014]/Season 01/Volume Show S01E01.mkv": 10,
		"Volume Show [imdbid-tt0700014]/Season 01/Volume Show S01E02.mkv": 10,
		"Volume Show [imdbid-tt0700014]/Season 01/Volume Show S01E03.mkv": 10,
		"Volume Show [imdbid-tt0700014]/Season 02/Volume Show S02E01.mkv": 10,
		"Volume Show [imdbid-tt0700014]/Season 02/Volume Show S02E02.mkv": 10,
		"Volume Show [imdbid-tt0700014]/Season 02/Volume Show S02E05.mkv": 10,
	})
	groups, err := c.service.NumberingGroups(t.Context(), c.folder.ID, id)
	if err != nil || len(groups) != 1 || groups[0].ID != "dvd" {
		t.Fatalf("the show's episode groups: %+v %v", groups, err)
	}
	if err := c.service.SetNumbering(t.Context(), c.folder.ID, id, ShowNumbering{Numbering: NumberedInOrder, Group: "story"}); !errors.Is(err,
		library.ErrUnknownGroup) {
		t.Errorf("a group the series does not have: %v", err)
	}

	c.choose(ShowNumbering{Numbering: NumberedInOrder, Group: "dvd"})
	c.expect("in the DVD order", map[[2]int]string{
		{1, 2}: "Volume Show S01E01.mkv", {1, 1}: "Volume Show S01E02.mkv", {2, 1}: "Volume Show S01E03.mkv",
		{1, 3}: "Volume Show S02E01.mkv", {2, 6}: "Volume Show S02E02.mkv", {2, 5}: "Volume Show S02E05.mkv", {2, 2}: "",
	})
	names := c.episodes("Volume Show")
	if len(names) != 5+2 || names[[2]int{2, 6}] != "Volume Show S02E02" || names[[2]int{2, 5}] != "Volume Show S02E05" {
		t.Errorf("the show's episodes in the DVD order: %d %v", len(names), names)
	}
	reasons := map[string]string{}
	for _, file := range c.show().Unmapped {
		reasons[file.Path[strings.LastIndex(file.Path, "/")+1:]] = file.Reason
	}
	if len(reasons) != 2 || reasons["Volume Show S02E02.mkv"] != unmappedNotListed || reasons["Volume Show S02E05.mkv"] != unmappedNotInOrder {
		t.Errorf("the files the order does not place: %v", reasons)
	}
}

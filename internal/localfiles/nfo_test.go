package localfiles

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
)

func TestReadNFO(t *testing.T) {
	for _, tc := range []struct {
		name, text, imdb, tmdb string
	}{
		{"unique identifiers", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<movie>
  <title>Night of the Living Dead</title>
  <uniqueid type="imdb" default="true">tt0063350</uniqueid>
  <uniqueid type="tmdb">10331</uniqueid>
</movie>`, "tt0063350", "10331"},
		{"older elements", "<movie><imdbid>tt0063350</imdbid><tmdbid>10331</tmdbid></movie>", "tt0063350", "10331"},
		{"unique identifiers first", `<movie><imdbid>tt0100258</imdbid><uniqueid type="imdb">tt0063350</uniqueid></movie>`, "tt0063350", ""},
		{"a show's", `<tvshow><uniqueid type="TMDB"> 962 </uniqueid></tvshow>`, "", "962"},
		{"another encoding and a mark", "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n<movie><title>Caf\xe9</title>" +
			"<uniqueid type=\"imdb\">TT0063350</uniqueid></movie>", "tt0063350", ""},
		{"not the root's", `<movie><actor><uniqueid type="imdb">tt0100258</uniqueid></actor><set><tmdbid>5</tmdbid></set></movie>`, "", ""},
		{"not identifiers", `<movie><uniqueid type="imdb">nm0001</uniqueid><tmdbid>none</tmdbid></movie>`, "", ""},
		{"broken after", `<movie><uniqueid type="imdb">tt0063350</uniqueid><plot>a < b && c</movie>`, "tt0063350", ""},
		{"an address", "https://titles.example/title/tt0063350/\n", "tt0063350", ""},
		{"an address in a language", "Night of the Living Dead\nhttps://titles.example/fr/title/tt0063350/?ref=nv", "tt0063350", ""},
		{"no address", "Night of the Living Dead (1968)\n", "", ""},
		{"no address in XML", "<movie><trailer>https://titles.example/title/tt0100258/</trailer></movie>", "", ""},
	} {
		imdb, tmdb, err := readNFO(strings.NewReader(tc.text))
		if err != nil || imdb != tc.imdb || tmdb != tc.tmdb {
			t.Errorf("%s: %q %q %v", tc.name, imdb, tmdb, err)
		}
	}
}

// uniqueIDs is an NFO file giving identifiers as Jellyfin writes them.
func uniqueIDs(root, imdb, tmdb string) string {
	text := "<" + root + "><title>Whatever</title>"
	if imdb != "" {
		text += `<uniqueid type="imdb">` + imdb + "</uniqueid>"
	}
	if tmdb != "" {
		text += `<uniqueid type="tmdb">` + tmdb + "</uniqueid>"
	}
	return text + "</" + root + ">"
}

// writeText writes text to path, its folders too, modified at.
func writeText(t *testing.T, path, text string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// matches reads how a folder's files were matched: by path, the title and
// how, or the reason they were not.
func (e env) matches(id accounts.ID) map[string]string {
	e.t.Helper()
	rows, err := e.pool.Query(e.t.Context(), `SELECT path, coalesce(stremio_id || ' by ' || matched_by, 'unmatched: ' || reason)
		FROM local_files WHERE addon_id = $1`, id)
	if err != nil {
		e.t.Fatal(err)
	}
	result := map[string]string{}
	for rows.Next() {
		var rel, match string
		if err := rows.Scan(&rel, &match); err != nil {
			e.t.Fatal(err)
		}
		result[rel] = match
	}
	return result
}

// A movie's NFO file, its own or its folder's movie.nfo when the folder is
// the movie's, a disc's movie.nfo and a show's tvshow.nfo match their unit
// when no link nor name does, before the search.
func TestNFOFilesMatchTheirUnits(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	at := time.Now()
	for rel, size := range map[string]int{
		"Mystery Reel (1968).mkv":                        10,
		"Old Reel/Old Reel.mkv":                          10,
		"The Gold Rush (1930)/The Gold Rush - 1080p.mkv": 10,
		"The Gold Rush (1930)/The Gold Rush - 720p.mkv":  10,
		"Nameless.mkv":                                   10,
		"Disc Thing/BDMV/STREAM/00001.m2ts":              10,
		"Shared/First Thing (1950).mkv":                  10,
		"Shared/Second Thing (1951).mkv":                 10,
		"Nosferatu (1922) {imdb-tt0017925}.mkv":          10,
		"Night of the Living Dead (1968).mkv":            10,
		"Linked Reel.mkv":                                10,
		"Night of the Living Dead (1968) - copy.mkv":     10,
	} {
		write(t, filepath.Join(dir, rel), size)
	}
	for rel, text := range map[string]string{
		"Mystery Reel (1968).nfo":                    uniqueIDs("movie", "tt0063350", "10331"),
		"Old Reel/movie.nfo":                         "<movie><imdbid>tt0013442</imdbid></movie>",
		"The Gold Rush (1930)/MOVIE.NFO":             uniqueIDs("movie", "", "962"),
		"Nameless.NFO":                               "https://titles.example/title/tt0013442/",
		"Disc Thing/movie.nfo":                       uniqueIDs("movie", "tt0017925", ""),
		"Shared/movie.nfo":                           uniqueIDs("movie", "tt0017925", ""),
		"Nosferatu (1922) {imdb-tt0017925}.nfo":      uniqueIDs("movie", "tt0063350", ""),
		"Night of the Living Dead (1968).nfo":        uniqueIDs("movie", "tt0100258", ""),
		"Linked Reel.nfo":                            uniqueIDs("movie", "tt0063350", ""),
		"Night of the Living Dead (1968) - copy.nfo": "not an identifier",
	} {
		writeText(t, filepath.Join(dir, rel), text, at)
	}
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"Mystery Reel (1968).mkv": "tt0063350 by nfo",
		"Old Reel/Old Reel.mkv":   "tt0013442 by nfo",
		// The TMDB identifier, of a title of another year.
		"The Gold Rush (1930)/The Gold Rush - 1080p.mkv": "tt0015864 by nfo",
		"The Gold Rush (1930)/The Gold Rush - 720p.mkv":  "tt0015864 by nfo",
		"Nameless.mkv":                      "tt0013442 by nfo",
		"Disc Thing/BDMV/STREAM/00001.m2ts": "tt0017925 by nfo",
		// Two movies in a folder: its movie.nfo is neither's.
		"Shared/First Thing (1950).mkv":  "unmatched: not_found",
		"Shared/Second Thing (1951).mkv": "unmatched: not_found",
		// The name's identifier wins over the NFO file's, which wins over
		// the search.
		"Nosferatu (1922) {imdb-tt0017925}.mkv": "tt0017925 by name",
		"Night of the Living Dead (1968).mkv":   "tt0100258 by nfo",
		"Linked Reel.mkv":                       "tt0063350 by nfo",
		// An NFO file giving no identifier leaves the search.
		"Night of the Living Dead (1968) - copy.mkv": "tt0063350 by search",
	}
	if got := e.matches(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.MatchedBy != (Matches{Name: 1, NFO: 8, Search: 1}) {
		t.Errorf("folder: %+v %v", folder.MatchedBy, err)
	}

	// A link wins over the NFO file; without it, the NFO file matches
	// again.
	if err := e.service.Link(t.Context(), addon.ID, "Linked Reel.mkv", "tt0100258"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.matches(addon.ID)["Linked Reel.mkv"]; got != "tt0100258 by link" {
		t.Errorf("linked: %s", got)
	}
	if err := e.service.Unlink(t.Context(), addon.ID, "Linked Reel.mkv"); err != nil {
		t.Fatal(err)
	}
	if got := e.matches(addon.ID)["Linked Reel.mkv"]; got != "tt0063350 by nfo" {
		t.Errorf("unlinked: %s", got)
	}

	shows, showDir := e.folder(KindShows, map[string]int{
		"Ranger Tales/Season 01/Ranger Tales - S01E01.mkv": 10,
		"Ranger Tales/Ranger Tales - S01E02.mkv":           10,
		"Other Tales/Other Tales - S01E01.mkv":             10,
	})
	writeText(t, filepath.Join(showDir, "Ranger Tales", "tvshow.nfo"), uniqueIDs("tvshow", "tt0041038", ""), at)
	// An episode's NFO file is not its show's.
	writeText(t, filepath.Join(showDir, "Other Tales", "Other Tales - S01E01.nfo"), uniqueIDs("episodedetails", "tt0041038", ""), at)
	if err := e.service.Scan(t.Context(), shows.ID); err != nil {
		t.Fatal(err)
	}
	want = map[string]string{
		"Ranger Tales/Season 01/Ranger Tales - S01E01.mkv": "tt0041038 by nfo",
		"Ranger Tales/Ranger Tales - S01E02.mkv":           "tt0041038 by nfo",
		"Other Tales/Other Tales - S01E01.mkv":             "unmatched: not_found",
	}
	if got := e.matches(shows.ID); !maps.Equal(got, want) {
		t.Errorf("shows: %v", got)
	}
}

// An NFO file added, changed or removed beside an unchanged video matches
// its unit again at the next scan; removed, it leaves the unit to the
// search. A folder gaining a second movie is no longer the first's own.
func TestNFOChangesMatchAgain(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, map[string]int{
		"Mystery Reel.mkv":     10,
		"Nosferatu (1922).mkv": 10,
		"Solo/Solo Reel.mkv":   10,
	})
	at := time.Now()
	nfo := filepath.Join(dir, "Mystery Reel.nfo")
	scan := func(want map[string]string) {
		t.Helper()
		if err := e.service.Scan(t.Context(), addon.ID); err != nil {
			t.Fatal(err)
		}
		if got := e.matches(addon.ID); !maps.Equal(got, want) {
			t.Errorf("files: %v", got)
		}
	}
	writeText(t, nfo, uniqueIDs("movie", "tt0063350", ""), at)
	writeText(t, filepath.Join(dir, "Nosferatu (1922).nfo"), uniqueIDs("movie", "tt0017925", ""), at)
	writeText(t, filepath.Join(dir, "Solo", "movie.nfo"), uniqueIDs("movie", "tt0015864", ""), at)
	scan(map[string]string{
		"Mystery Reel.mkv":     "tt0063350 by nfo",
		"Nosferatu (1922).mkv": "tt0017925 by nfo",
		"Solo/Solo Reel.mkv":   "tt0015864 by nfo",
	})
	before := e.titles.searches.Load()
	writeText(t, nfo, uniqueIDs("movie", "tt0100258", ""), at.Add(time.Minute))
	write(t, filepath.Join(dir, "Solo", "Other Reel (1950).mkv"), 10)
	scan(map[string]string{
		"Mystery Reel.mkv":           "tt0100258 by nfo",
		"Nosferatu (1922).mkv":       "tt0017925 by nfo",
		"Solo/Solo Reel.mkv":         "unmatched: not_found",
		"Solo/Other Reel (1950).mkv": "unmatched: not_found",
	})
	// The changed NFO file's identifier asks no search: only the movie
	// whose folder's movie.nfo is no longer its own, and the new one, do.
	if searched := e.titles.searches.Load() - before; searched != 2 {
		t.Errorf("searches: %d", searched)
	}
	if err := os.Remove(nfo); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "Nosferatu (1922).nfo")); err != nil {
		t.Fatal(err)
	}
	scan(map[string]string{
		"Mystery Reel.mkv":           "unmatched: not_found",
		"Nosferatu (1922).mkv":       "tt0013442 by search",
		"Solo/Solo Reel.mkv":         "unmatched: not_found",
		"Solo/Other Reel (1950).mkv": "unmatched: not_found",
	})
	var sides int
	if err := e.pool.QueryRow(t.Context(), "SELECT count(*) FROM local_side_files WHERE addon_id = $1", addon.ID).Scan(&sides); err != nil ||
		sides != 0 {
		t.Errorf("NFO files kept: %d %v", sides, err)
	}
}

// A WebDAV folder's NFO files are listed with its folders and read only
// once new or changed, as a local folder's.
func TestShareNFOFiles(t *testing.T) {
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	at := time.Now().Add(-time.Hour).Truncate(time.Second)
	write(t, filepath.Join(dir, "movies", "Mystery Reel.mkv"), 10)
	write(t, filepath.Join(dir, "movies", "Old Reel", "Old Reel.mkv"), 10)
	writeText(t, filepath.Join(dir, "movies", "Mystery Reel.nfo"), uniqueIDs("movie", "tt0063350", ""), at)
	writeText(t, filepath.Join(dir, "movies", "Old Reel", "movie.nfo"), "https://titles.example/title/tt0013442/", at)
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/movies/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"Mystery Reel.mkv":      "tt0063350 by nfo",
		"Old Reel/Old Reel.mkv": "tt0013442 by nfo",
	}
	if got := e.matches(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	writeText(t, filepath.Join(dir, "movies", "Mystery Reel.nfo"), uniqueIDs("movie", "tt0100258", ""), at.Add(time.Minute))
	if err := os.Remove(filepath.Join(dir, "movies", "Old Reel", "movie.nfo")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	want = map[string]string{
		"Mystery Reel.mkv":      "tt0100258 by nfo",
		"Old Reel/Old Reel.mkv": "unmatched: not_found",
	}
	if got := e.matches(addon.ID); !maps.Equal(got, want) {
		t.Errorf("after the changes: %v", got)
	}
}

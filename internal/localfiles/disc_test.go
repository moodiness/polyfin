package localfiles

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/secrets"
)

// A share's discs are read as a local folder's: one movie per disc, a disc
// at the share's root named after the address's last folder.
func TestShareDiscsAreOneMovie(t *testing.T) {
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	for rel, size := range map[string]int{
		"movies/Movie (2001)/BDMV/STREAM/00000.m2ts":        30,
		"movies/Movie (2001)/BDMV/STREAM/00001.m2ts":        300,
		"movies/Movie (2001)/BDMV/index.bdmv":               5,
		"movies/Other (1999)/Disc 1/BDMV/STREAM/00003.m2ts": 40,
		"movies/Other (1999)/Disc 1/BDMV/STREAM/00004.m2ts": 10,
		"The General (1926)/BDMV/STREAM/00000.m2ts":         10,
		"The General (1926)/BDMV/STREAM/00001.m2ts":         70,
	} {
		write(t, filepath.Join(dir, rel), size)
	}
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	movies, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/movies/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(movies.ID)
	want := map[string]string{
		"Movie (2001)/BDMV/STREAM/00001.m2ts":        "unmatched: not_found",
		"Other (1999)/Disc 1/BDMV/STREAM/00003.m2ts": "unmatched: not_found",
	}
	if got := e.files(movies.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	if got := e.reads(movies.ID)["Other (1999)/Disc 1/BDMV/STREAM/00003.m2ts"]; got != (read{unit: "Other (1999)/Disc 1", title: "Other", year: 1999}) {
		t.Errorf("read: %+v", got)
	}
	disc, err := e.service.Add(t.Context(), NewFolder{Name: "Disc", Path: server.URL + "/The%20General%20(1926)", Kind: KindMovies,
		User: "reader", Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(disc.ID)
	if got := e.files(disc.ID); !maps.Equal(got, map[string]string{"BDMV/STREAM/00001.m2ts": "tt0017925"}) {
		t.Errorf("a disc at the share's root: %v", got)
	}
}

// read is what a scan stored of a file: its unit, title and year.
type read struct {
	unit, title string
	year        int
}

// reads reads what a folder's scans stored of its files, by path.
func (e env) reads(id accounts.ID) map[string]read {
	e.t.Helper()
	rows, err := e.pool.Query(e.t.Context(), "SELECT path, unit, title, coalesce(year, 0) FROM local_files WHERE addon_id = $1", id)
	if err != nil {
		e.t.Fatal(err)
	}
	result := map[string]read{}
	for rows.Next() {
		var rel string
		var r read
		if err := rows.Scan(&rel, &r.unit, &r.title, &r.year); err != nil {
			e.t.Fatal(err)
		}
		result[rel] = r
	}
	return result
}

// A Blu-ray disc copied as it is, a folder holding BDMV, is one movie: its
// largest stream, matched by the disc folder's name, the folder above's
// when it only numbers the disc. Its other streams and files are left out;
// a file beside BDMV is a movie of its own.
func TestDiscsAreOneMovie(t *testing.T) {
	e := newEnv(t)
	addon, _ := e.folder(KindMovies, map[string]int{
		"Movie (2001)/BDMV/STREAM/00000.m2ts":        30,
		"Movie (2001)/BDMV/STREAM/00001.m2ts":        300,
		"Movie (2001)/BDMV/STREAM/00002.m2ts":        20,
		"Movie (2001)/BDMV/index.bdmv":               5,
		"Movie (2001)/BDMV/PLAYLIST/00000.mpls":      5,
		"Movie (2001)/CERTIFICATE/id.bdmv":           5,
		"Movie (2001)/Nosferatu (1922).mkv":          10,
		"Other (1999)/Disc 1/BDMV/STREAM/00003.m2ts": 40,
		"Other (1999)/Disc 1/BDMV/STREAM/00004.m2ts": 10,
		"Box/DISC01_xyz/bdmv/stream/00010.M2TS":      10,
		"Box/DISC01_xyz/bdmv/stream/00011.M2TS":      50,
		"The General (1926)/BDMV/STREAM/00800.m2ts":  70,
		"The General (1926)/BDMV/STREAM/00801.m2ts":  70,
	})
	want := map[string]string{
		"Movie (2001)/BDMV/STREAM/00001.m2ts":        "unmatched: not_found",
		"Movie (2001)/Nosferatu (1922).mkv":          "tt0013442",
		"Other (1999)/Disc 1/BDMV/STREAM/00003.m2ts": "unmatched: not_found",
		"Box/DISC01_xyz/bdmv/stream/00011.M2TS":      "unmatched: not_found",
		"The General (1926)/BDMV/STREAM/00800.m2ts":  "tt0017925",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	reads := e.reads(addon.ID)
	for rel, r := range map[string]read{
		"Movie (2001)/BDMV/STREAM/00001.m2ts":        {unit: "Movie (2001)", title: "Movie", year: 2001},
		"Other (1999)/Disc 1/BDMV/STREAM/00003.m2ts": {unit: "Other (1999)/Disc 1", title: "Other", year: 1999},
		"Box/DISC01_xyz/bdmv/stream/00011.M2TS":      {unit: "Box/DISC01_xyz", title: "DISC01 xyz"},
		"Movie (2001)/Nosferatu (1922).mkv":          {unit: "Movie (2001)/Nosferatu (1922).mkv", title: "Nosferatu", year: 1922},
	} {
		if reads[rel] != r {
			t.Errorf("%s: %+v", rel, reads[rel])
		}
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Files != 5 || folder.Matched != 2 || folder.Unmatched != 3 {
		t.Errorf("folder: %+v %v", folder, err)
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID)
	discs := map[string]string{}
	for _, u := range unmatched {
		discs[u.Unit] = u.Disc
	}
	if err != nil || total != 3 || !maps.Equal(discs, map[string]string{"Box/DISC01_xyz": DiscBluRay, "Movie (2001)": DiscBluRay, "Other (1999)/Disc 1": DiscBluRay}) {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}
	// The disc's version is named after its folder, not its stream.
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0017925")
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "The General (1926).m2ts" || filepath.Ext(streams[0].URL) != ".m2ts" {
		t.Errorf("streams: %+v %v", streams, err)
	}
	// Linked by hand, through its stream or its folder, a disc is linked
	// whole, and stays so when another stream becomes its largest.
	if err := e.service.Link(t.Context(), addon.ID, "Box/DISC01_xyz/bdmv/stream/00011.M2TS", "tt0063350"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["Box/DISC01_xyz/bdmv/stream/00011.M2TS"]; got != "tt0063350" {
		t.Errorf("linked: %s", got)
	}
	dir := e.folderPath(addon)
	write(t, filepath.Join(dir, "Box/DISC01_xyz/bdmv/stream/00012.M2TS"), 90)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	files := e.files(addon.ID)
	if _, old := files["Box/DISC01_xyz/bdmv/stream/00011.M2TS"]; old || files["Box/DISC01_xyz/bdmv/stream/00012.M2TS"] != "tt0063350" || len(files) != 5 {
		t.Errorf("another largest stream: %v", files)
	}
	folder, _ = e.service.Folder(t.Context(), addon.ID)
	if !slices.Equal(folder.Links, []Link{{Unit: "Box/DISC01_xyz", IMDb: "tt0063350"}}) {
		t.Errorf("links: %+v", folder.Links)
	}
	if err := e.service.Unlink(t.Context(), addon.ID, "Box/DISC01_xyz"); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["Box/DISC01_xyz/bdmv/stream/00012.M2TS"]; got != "unmatched: not_found" {
		t.Errorf("unlinked: %s", got)
	}
}

// folderPath is a folder's path in the container.
func (e env) folderPath(addon addons.Addon) string {
	e.t.Helper()
	folder, err := e.service.Folder(e.t.Context(), addon.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return folder.Addon.ManifestURL
}

// A disc at a folder's root is named after the folder; a disc in a shows
// folder is left out, its streams being no one episode.
func TestDiscsAtTheRootAndInShows(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(t.TempDir(), "The General (1926)")
	write(t, filepath.Join(dir, "BDMV", "STREAM", "00000.m2ts"), 10)
	write(t, filepath.Join(dir, "BDMV", "STREAM", "00001.m2ts"), 50)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Disc", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if got := e.files(addon.ID); !maps.Equal(got, map[string]string{"BDMV/STREAM/00001.m2ts": "tt0017925"}) {
		t.Errorf("files: %v", got)
	}
	if got := e.reads(addon.ID)["BDMV/STREAM/00001.m2ts"]; got != (read{unit: ".", title: "The General", year: 1926}) {
		t.Errorf("read: %+v", got)
	}
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0017925")
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "The General (1926).m2ts" {
		t.Errorf("streams: %+v %v", streams, err)
	}

	shows, _ := e.folder(KindShows, map[string]int{
		"The Lone Ranger/Season 01/The Lone Ranger - S01E01.mkv": 10,
		"The Lone Ranger/Season 01/BDMV/STREAM/00000.m2ts":       100,
		"The Lone Ranger/Season 01/BDMV/STREAM/00001.m2ts":       10,
		"The Lone Ranger Disc 2/BDMV/STREAM/00000.m2ts":          100,
	})
	if got := e.files(shows.ID); !maps.Equal(got, map[string]string{"The Lone Ranger/Season 01/The Lone Ranger - S01E01.mkv": "tt0041038"}) {
		t.Errorf("shows: %v", got)
	}
	if unmatched, total, err := e.service.Unmatched(t.Context(), shows.ID); err != nil || total != 0 {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}
}

// A folder scanned before discs were read holds a row per stream, each its
// own unit, whatever its state; its next scan leaves a row per disc, read
// as its disc, and moves a link made on a stream to its disc.
func TestDiscsReplaceTheirStreamsOnUpgrade(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, nil)
	files := map[string]int{
		"Movie (2001)/BDMV/STREAM/00000.m2ts":       300,
		"Movie (2001)/BDMV/STREAM/00001.m2ts":       30,
		"Movie (2001)/BDMV/STREAM/00002.m2ts":       20,
		"Nosferatu (1922)/BDMV/STREAM/00000.m2ts":   300,
		"Nosferatu (1922)/BDMV/STREAM/00001.m2ts":   30,
		"The General (1926)/BDMV/STREAM/00000.m2ts": 300,
		"The General (1926)/BDMV/STREAM/00001.m2ts": 30,
	}
	for rel, size := range files {
		write(t, filepath.Join(dir, rel), size)
	}
	// The rows as scans wrote them before: each stream a unit, its title
	// its number, unchanged since.
	old := func(rel, stremioID, by, reason string) {
		t.Helper()
		info, err := os.Stat(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		var id *string
		if stremioID != "" {
			id = &stremioID
		}
		if _, err := e.pool.Exec(t.Context(), `INSERT INTO local_files (addon_id, path, size, modified, unit, title, stremio_id, imdb_id, name,
				matched_by, reason) VALUES ($1, $2, $3, $4, $2, $5, $6, coalesce($6, ''), coalesce($6, ''), $7, $8)`,
			addon.ID, rel, info.Size(), info.ModTime().UTC().Truncate(time.Microsecond), stem(rel), id, by, reason); err != nil {
			t.Fatal(err)
		}
	}
	old("Movie (2001)/BDMV/STREAM/00000.m2ts", "", "", reasonUnreadableName)
	old("Movie (2001)/BDMV/STREAM/00001.m2ts", "", "", reasonNotFound)
	old("Movie (2001)/BDMV/STREAM/00002.m2ts", "tt0063350", byLink, "")
	old("Nosferatu (1922)/BDMV/STREAM/00000.m2ts", "tt0100258", bySearch, "")
	old("Nosferatu (1922)/BDMV/STREAM/00001.m2ts", "", "", reasonNotFound)
	old("The General (1926)/BDMV/STREAM/00000.m2ts", "", "", reasonNotFound)
	old("The General (1926)/BDMV/STREAM/00001.m2ts", "", "", reasonUnreadableName)
	if _, err := e.pool.Exec(t.Context(), "INSERT INTO local_links (addon_id, unit, imdb_id) VALUES ($1, $2, 'tt0063350')",
		addon.ID, "Movie (2001)/BDMV/STREAM/00002.m2ts"); err != nil {
		t.Fatal(err)
	}
	if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Files != 7 {
		t.Fatalf("before: %+v", folder)
	}

	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Movie (2001)/BDMV/STREAM/00000.m2ts":       "tt0063350",
		"Nosferatu (1922)/BDMV/STREAM/00000.m2ts":   "tt0013442",
		"The General (1926)/BDMV/STREAM/00000.m2ts": "tt0017925",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	wantReads := map[string]read{
		"Movie (2001)/BDMV/STREAM/00000.m2ts":       {unit: "Movie (2001)", title: "Movie", year: 2001},
		"Nosferatu (1922)/BDMV/STREAM/00000.m2ts":   {unit: "Nosferatu (1922)", title: "Nosferatu", year: 1922},
		"The General (1926)/BDMV/STREAM/00000.m2ts": {unit: "The General (1926)", title: "The General", year: 1926},
	}
	if got := e.reads(addon.ID); !maps.Equal(got, wantReads) {
		t.Errorf("reads: %v", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Files != 3 || folder.Matched != 3 ||
		!slices.Equal(folder.Links, []Link{{Unit: "Movie (2001)", IMDb: "tt0063350"}}) {
		t.Errorf("folder: %+v %v", folder, err)
	}
	var by string
	if err := e.pool.QueryRow(t.Context(), "SELECT matched_by FROM local_files WHERE addon_id = $1 AND unit = 'Movie (2001)'", addon.ID).
		Scan(&by); err != nil || by != byLink {
		t.Errorf("matched by %q %v", by, err)
	}
}

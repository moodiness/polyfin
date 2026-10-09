package localfiles

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// A file is matched to the one title the metadata addons find of its name
// and year; an identifier in its name wins over the search; several titles,
// another year or none leave it unmatched, with why.
func TestFilesAreMatchedThroughTheMetadataAddons(t *testing.T) {
	e := newEnv(t)
	addon, _ := e.folder(KindMovies, map[string]int{
		"Night of the Living Dead (1968).mkv":                   10,
		"Night of the Living Dead.mkv":                          10,
		"Nosferatu (1930).mkv":                                  10,
		"The General (1926)/The.General.1080p.mkv":              10,
		"Nosferatu (1922) {imdb-tt0017925}.mkv":                 10,
		"The Gold Rush [tmdbid-962].mkv":                        10,
		"An Unknown Picture (1950).mkv":                         10,
		"The General (1926)/Extras/The General - Interview.mkv": 10,
		"notes.txt": 10,
	})
	want := map[string]string{
		"Night of the Living Dead (1968).mkv":      "tt0063350",
		"Night of the Living Dead.mkv":             "unmatched: ambiguous",
		"Nosferatu (1930).mkv":                     "unmatched: other_year",
		"The General (1926)/The.General.1080p.mkv": "tt0017925",
		"Nosferatu (1922) {imdb-tt0017925}.mkv":    "tt0017925",
		"The Gold Rush [tmdbid-962].mkv":           "tt0015864",
		"An Unknown Picture (1950).mkv":            "unmatched: not_found",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Files != 7 || folder.Matched != 4 || folder.Unmatched != 3 || folder.Error != "" || folder.ScannedAt == nil {
		t.Errorf("folder: %+v %v", folder, err)
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID)
	if err != nil || total != 3 || len(unmatched) != 3 || unmatched[0].Path != "An Unknown Picture (1950).mkv" || unmatched[0].Year != 1950 ||
		unmatched[0].Title != "An Unknown Picture" {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}
	// The title matched by its identifier is named as the addon describes
	// it, not as the file is.
	metas, err := e.service.Catalog(t.Context(), addon.ID, "movie", catalogID, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, meta := range metas {
		names[meta.ID] = meta.Name + " " + string(meta.Year)
	}
	if len(metas) != 3 || names["tt0017925"] != "The General 1926" || names["tt0063350"] != "Night of the Living Dead 1968" {
		t.Errorf("catalog: %v", names)
	}
}

// A rescan reads only what changed: new files are matched, changed ones
// read again, files gone forgotten, and a symbolic link leading out of the
// folder is not followed.
func TestScansAreIncremental(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, map[string]int{
		"Night of the Living Dead (1968).mkv": 10,
		"The General (1926).mkv":              10,
		"Nosferatu (1922).mkv":                10,
	})
	before := e.titles.searches.Load()
	write(t, filepath.Join(dir, "The Gold Rush (1925).mkv"), 10)
	write(t, filepath.Join(dir, "The General (1926).mkv"), 20)
	if err := os.Remove(filepath.Join(dir, "Nosferatu (1922).mkv")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	write(t, filepath.Join(outside, "Nosferatu (1922).mkv"), 10)
	if err := os.Symlink(filepath.Join(outside, "Nosferatu (1922).mkv"), filepath.Join(dir, "Nosferatu (1922).mkv")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "Elsewhere")); err != nil {
		t.Fatal(err)
	}
	// A link to a file of the folder is followed.
	if err := os.Symlink(filepath.Join(dir, "The Gold Rush (1925).mkv"), filepath.Join(dir, "The Gold Rush (1925) - copy.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Night of the Living Dead (1968).mkv": "tt0063350",
		"The General (1926).mkv":              "tt0017925",
		"The Gold Rush (1925).mkv":            "tt0015864",
		"The Gold Rush (1925) - copy.mkv":     "tt0015864",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	// Only the new file's title was searched: the others, the file that
	// changed included, kept theirs.
	if searched := e.titles.searches.Load() - before; searched != 1 {
		t.Errorf("searches in the rescan: %d", searched)
	}
	var size int64
	if err := e.pool.QueryRow(t.Context(), "SELECT size FROM local_files WHERE addon_id = $1 AND path = 'The General (1926).mkv'",
		addon.ID).Scan(&size); err != nil || size != 20 {
		t.Errorf("changed file: %d %v", size, err)
	}
}

// A file linked by hand to an IMDb identifier is that title's, and stays
// so through rescans, even once changed; a show's folder links its
// episodes.
func TestLinksSurviveRescans(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, map[string]int{"Night of the Living Dead.mkv": 10})
	if err := e.service.Link(t.Context(), addon.ID, "Night of the Living Dead.mkv", " TT0100258 "); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["Night of the Living Dead.mkv"]; got != "tt0100258" {
		t.Fatalf("linked: %s", got)
	}
	write(t, filepath.Join(dir, "Night of the Living Dead.mkv"), 30)
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "Night of the Living Dead.mkv"), later, later); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["Night of the Living Dead.mkv"]; got != "tt0100258" {
		t.Errorf("after a rescan: %s", got)
	}
	// Moved away and back, it is still linked.
	if err := os.Rename(filepath.Join(dir, "Night of the Living Dead.mkv"), filepath.Join(t.TempDir(), "away.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "Night of the Living Dead.mkv"), 10)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["Night of the Living Dead.mkv"]; got != "tt0100258" {
		t.Errorf("back in the folder: %s", got)
	}
	if err := e.service.Link(t.Context(), addon.ID, "Night of the Living Dead.mkv", "0100258"); err != ErrInvalidIMDb {
		t.Errorf("an identifier without tt: %v", err)
	}

	shows, showDir := e.folder(KindShows, map[string]int{
		"Ranger Tales/Season 01/Ranger Tales - S01E01.mkv": 10,
		"Ranger Tales/Season 01/Ranger Tales - S01E02.mkv": 10,
	})
	if err := e.service.Link(t.Context(), shows.ID, "Ranger Tales/Season 01/Ranger Tales - S01E02.mkv", "tt0041038"); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(showDir, "Ranger Tales/Season 01/Ranger Tales - S01E03.mkv"), 10)
	if err := e.service.Scan(t.Context(), shows.ID); err != nil {
		t.Fatal(err)
	}
	for rel, title := range e.files(shows.ID) {
		if title != "tt0041038" {
			t.Errorf("%s: %s", rel, title)
		}
	}
	folder, _ := e.service.Folder(t.Context(), shows.ID)
	if len(folder.Links) != 1 || folder.Links[0].Unit != "Ranger Tales" || folder.Links[0].IMDb != "tt0041038" {
		t.Errorf("links: %+v", folder.Links)
	}
}

// A folder Polyfin cannot read is reported, and what its scans found is
// kept: a share that is not mounted for a while empties no library.
func TestUnreadableFoldersAreReported(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, map[string]int{"Night of the Living Dead (1968).mkv": 10})
	moved := dir + ".unmounted"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Error != errMissing || folder.Files != 1 || folder.Matched != 1 {
		t.Errorf("missing folder: %+v %v", folder, err)
	}
	if err := os.Rename(moved, dir); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if err := os.Chmod(dir, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if err := e.service.Scan(t.Context(), addon.ID); err != nil {
			t.Fatal(err)
		}
		if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Error != errUnreadable || folder.Files != 1 {
			t.Errorf("folder without permission: %+v", folder)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Error != "" {
		t.Errorf("readable again: %+v", folder)
	}
}

// A show's files are the streams of their episodes, a file holding two
// episodes those of both, named after the folder with their resolution and
// size.
func TestEpisodesAreStreams(t *testing.T) {
	e := newEnv(t)
	addon, _ := e.folder(KindShows, map[string]int{
		"The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E01-E02 - 720p.mkv": 1_500_000,
		"The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E03.mp4":            10,
		"The Lone Ranger (1949)/Season 01/Bonus.mkv":                               10,
	})
	files := e.files(addon.ID)
	if files["The Lone Ranger (1949)/Season 01/Bonus.mkv"] != "unmatched: unreadable_name" ||
		files["The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E03.mp4"] != "tt0041038" {
		t.Errorf("files: %v", files)
	}
	for _, id := range []string{"tt0041038:1:1", "tt0041038:1:2"} {
		streams, err := e.service.Streams(t.Context(), addon.ID, id)
		if err != nil || len(streams) != 1 || streams[0].Name != "Shelf" || streams[0].Description != "720p · 2 MB" ||
			streams[0].BehaviorHints.Filename != "The Lone Ranger - S01E01-E02 - 720p.mkv" || !streams[0].Playable() ||
			filepath.Ext(streams[0].URL) != ".mkv" {
			t.Errorf("%s: %+v %v", id, streams, err)
		}
	}
	if streams, _ := e.service.Streams(t.Context(), addon.ID, "tt0041038:1:4"); len(streams) != 0 {
		t.Errorf("an episode without file: %+v", streams)
	}
	meta, err := e.service.Meta(t.Context(), addon.ID, "tt0041038")
	var ids []string
	for _, video := range meta.Videos {
		ids = append(ids, video.ID)
	}
	if err != nil || !slices.Equal(ids, []string{"tt0041038:1:1", "tt0041038:1:2", "tt0041038:1:3"}) {
		t.Errorf("episodes: %v %v", ids, err)
	}
	if _, err := e.service.Meta(t.Context(), addon.ID, "tt0000001"); err != stremio.ErrNotFound {
		t.Errorf("an unknown title: %v", err)
	}
}

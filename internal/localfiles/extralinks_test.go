package localfiles

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Once a folder reads extras, as at the first scan after an upgrade, a
// file linked by hand stays the title it was linked to, whatever its name,
// and an extra named with a suffix whose movie is not beside it is listed
// unmatched, so that an administrator sees it; linked, it is a title, and
// rescans keep it so. Unlinked, a file named as an extra is one again.
func TestLinksWinOverExtrasNames(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	for rel, size := range map[string]int{
		"Night of the Living Dead (1968)/Night of the Living Dead (1968).mkv": 10,
		"Night of the Living Dead (1968)/Clips/Bloopers.mkv":                  11,
		"Night of the Living Dead (1968)/backdrops/intro.mkv":                 12,
		"Night of the Living Dead (1968)/trailer.mp4":                         13,
		"Nosferatu (1922)-featurette.mkv":                                     14,
	} {
		write(t, filepath.Join(dir, rel), size)
	}
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies, ReadExtras: new(false)})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	// Without extras, the administrator linked two of the files read as
	// titles: one as a version of its movie, one as another movie.
	for rel, imdb := range map[string]string{"Night of the Living Dead (1968)/Clips/Bloopers.mkv": "tt0063350",
		"Night of the Living Dead (1968)/backdrops/intro.mkv": "tt0017925"} {
		if err := e.service.Link(t.Context(), addon.ID, rel, imdb); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := e.service.Update(t.Context(), addon.ID, Changes{ReadExtras: new(true)}); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Night of the Living Dead (1968)/Night of the Living Dead (1968).mkv": "tt0063350",
		"Night of the Living Dead (1968)/Clips/Bloopers.mkv":                  "tt0063350",
		"Night of the Living Dead (1968)/backdrops/intro.mkv":                 "tt0017925",
		"Nosferatu (1922)-featurette.mkv":                                     "unmatched: lone_extra",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Files != 4 || folder.Unmatched != 1 || folder.Extras != 1 || len(folder.Links) != 2 {
		t.Errorf("folder: %+v %v", folder, err)
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID)
	if err != nil || total != 1 || len(unmatched) != 1 || unmatched[0].Path != "Nosferatu (1922)-featurette.mkv" || unmatched[0].Unit != unmatched[0].Path ||
		unmatched[0].Reason != "lone_extra" {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}

	// A rescan keeps the lone extra unmatched, without searching for it.
	searches := e.titles.searches.Load()
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) || e.titles.searches.Load() != searches {
		t.Errorf("after a rescan: %v, %d searches", got, e.titles.searches.Load()-searches)
	}

	// Linked, the lone extra is a title, through rescans.
	if err := e.service.Link(t.Context(), addon.ID, "Nosferatu (1922)-featurette.mkv", "tt0013442"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	want["Nosferatu (1922)-featurette.mkv"] = "tt0013442"
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("the lone extra linked: %v", got)
	}

	// Unlinked, the theme video is its movie's again.
	if err := e.service.Unlink(t.Context(), addon.ID, "Night of the Living Dead (1968)/backdrops/intro.mkv"); err != nil {
		t.Fatal(err)
	}
	delete(want, "Night of the Living Dead (1968)/backdrops/intro.mkv")
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("unlinked: %v", got)
	}
	rows, err := e.pool.Query(t.Context(), "SELECT path FROM local_extras WHERE addon_id = $1 ORDER BY path", addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	extras, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(extras, []string{"Night of the Living Dead (1968)/backdrops/intro.mkv", "Night of the Living Dead (1968)/trailer.mp4"}) {
		t.Errorf("extras: %v", extras)
	}
}

package localfiles

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// The files a scan first finds are added then: a rescan of a changed file
// keeps when it was added, and the files found before this was kept are
// never counted.
func TestAddedFilesAreCountedWhenFirstFound(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	first := time.Now().Add(-time.Second)
	addon, dir := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 10, "Nosferatu (1922).mkv": 10})
	if _, err := e.pool.Exec(ctx, "UPDATE local_files SET added_at = NULL WHERE path = 'Nosferatu (1922).mkv'"); err != nil {
		t.Fatal(err)
	}
	second := time.Now()
	write(t, filepath.Join(dir, "The Gold Rush (1925).mkv"), 10)
	write(t, filepath.Join(dir, "The General (1926).mkv"), 20)
	if err := e.service.Scan(ctx, addon.ID); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Minute)

	added, total, err := e.service.Added(ctx, first, until, 10)
	var names []string
	for _, a := range added {
		if a.Kind != KindMovies || a.Files != 1 {
			t.Errorf("%+v", a)
		}
		names = append(names, a.Name)
	}
	slices.Sort(names)
	if err != nil || total != 2 || !slices.Equal(names, []string{"The General", "The Gold Rush"}) {
		t.Errorf("added since the first scan: %v, %d files, %v", names, total, err)
	}
	if added, total, err := e.service.Added(ctx, second, until, 10); err != nil || total != 1 || len(added) != 1 || added[0].Name != "The Gold Rush" {
		t.Errorf("added since the rescan: %+v, %d files, %v", added, total, err)
	}
	if added, total, err := e.service.Added(ctx, first, until, 1); err != nil || total != 2 || len(added) != 1 {
		t.Errorf("one title asked: %+v, %d files, %v", added, total, err)
	}
}

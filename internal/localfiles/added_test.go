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
	addedWhenFirstFound(t, newEnv(t))
}

// The files are added by the server's clock, the one the windows asked
// for are taken from: the database's clock, another machine's at times,
// behind the server's or ahead, does not move them out of the windows.
func TestAddedFilesAreCountedWhateverTheDatabaseClock(t *testing.T) {
	for name, ahead := range map[string]time.Duration{"database behind": 500 * time.Millisecond, "database ahead": -2 * time.Minute} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.service.now = func() time.Time { return time.Now().Add(ahead) }
			addedWhenFirstFound(t, e)
		})
	}
}

// addedWhenFirstFound scans a folder twice, and asks which files were
// added, the windows taken from the service's clock.
func addedWhenFirstFound(t *testing.T, e env) {
	ctx := t.Context()
	first := e.service.now().Add(-time.Second)
	addon, dir := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 10, "Nosferatu (1922).mkv": 10})
	if _, err := e.pool.Exec(ctx, "UPDATE local_files SET added_at = NULL WHERE path = 'Nosferatu (1922).mkv'"); err != nil {
		t.Fatal(err)
	}
	second := e.service.now()
	write(t, filepath.Join(dir, "The Gold Rush (1925).mkv"), 10)
	write(t, filepath.Join(dir, "The General (1926).mkv"), 20)
	if err := e.service.Scan(ctx, addon.ID); err != nil {
		t.Fatal(err)
	}
	until := e.service.now().Add(time.Minute)

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

package localfiles

import (
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/library"
)

// A file whose name could not be read, stored by a scan before plain
// videos, is stored again by the next scan, which lists it as one.
func TestUnreadableFilesBecomePlainVideos(t *testing.T) {
	e := newEnvWith(t)
	addon, _ := e.folder(KindMovies, map[string]int{"[Group].mkv": 10})
	if got := e.files(addon.ID); got["[Group].mkv"] != "unmatched: "+reasonUnreadableName {
		t.Fatalf("files: %v", got)
	}
	plain := func() []string {
		t.Helper()
		metas, err := e.service.Catalog(t.Context(), addon.ID, "movie", catalogID, 0, "")
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, meta := range metas {
			if strings.HasPrefix(meta.ID, library.PlainIDPrefix) {
				names = append(names, meta.Name)
			}
		}
		return names
	}
	if names := plain(); len(names) != 1 || names[0] != "[Group]" {
		t.Fatalf("the plain videos: %v", names)
	}
	// As a scan before plain videos left it.
	if _, err := e.pool.Exec(t.Context(), "UPDATE local_files SET plain_id = NULL WHERE addon_id = $1", addon.ID); err != nil {
		t.Fatal(err)
	}
	if names := plain(); len(names) != 0 {
		t.Fatalf("the plain videos before: %v", names)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if names := plain(); len(names) != 1 || names[0] != "[Group]" {
		t.Errorf("the plain videos after a scan: %v", names)
	}
}

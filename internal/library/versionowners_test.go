package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A version listed before a restart is still opened as its item after it,
// before anything lists the item again; an identifier no version has is
// none.
func TestAVersionsOwnerSurvivesARestart(t *testing.T) {
	t.Parallel()
	e := newStaleEnv(t)
	e.numbers <- []int{1, 2}
	versions, err := e.service.Versions(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	restarted := e.restarted()
	for _, version := range versions {
		if _, known := restarted.VersionOwner(version.ID); known {
			t.Fatalf("%s remembered in memory after a restart", version.ID)
		}
		if owner, ok := restarted.SavedVersionOwner(t.Context(), version.ID); !ok || owner != e.movie {
			t.Errorf("%s: owner %s %v, want %s", version.ID, owner, ok, e.movie)
		}
	}
	if owner, ok := restarted.SavedVersionOwner(t.Context(), accounts.ID{9}); ok {
		t.Errorf("an unknown identifier is owned by %s", owner)
	}
}

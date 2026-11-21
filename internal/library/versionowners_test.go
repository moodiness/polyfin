package library

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A version listed before a restart is still opened as its item after it,
// before anything lists the item again; an identifier no version has is
// none. The owners a listing notes are saved later, together, in one
// statement.
func TestAVersionsOwnerSurvivesARestart(t *testing.T) {
	t.Parallel()
	e := newStaleEnv(t)
	var mu sync.Mutex
	var saves []func()
	e.service.saveLater = func(_ time.Duration, save func()) {
		mu.Lock()
		defer mu.Unlock()
		saves = append(saves, save)
	}
	e.numbers <- []int{1, 2}
	versions, err := e.service.Versions(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	if owner, ok := e.service.SavedVersionOwner(t.Context(), versions[0].ID); !ok || owner != e.movie {
		t.Errorf("before a restart: owner %s %v", owner, ok)
	}
	if _, ok := e.restarted().SavedVersionOwner(t.Context(), versions[0].ID); ok {
		t.Error("the owners were saved at once")
	}
	mu.Lock()
	due := slices.Clone(saves)
	mu.Unlock()
	if len(due) != 1 {
		t.Fatalf("%d saves due, want 1", len(due))
	}
	due[0]()
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

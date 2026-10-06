package library

import (
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// versionNames names versions.
func versionNames(versions []Version) []string {
	result := make([]string, len(versions))
	for i, version := range versions {
		result[i] = version.Name
	}
	return result
}

// A play lists an expired list at once, while its addon is asked again.
func TestAPlayListsAnExpiredListAtOnce(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond)
	e.expired()
	versions, complete, err := e.service.VersionsToPlay(t.Context(), e.member, e.movie)
	if err != nil || complete || !slices.Equal(versionNames(versions), []string{"Source 1", "Source 2", "Source 3"}) {
		t.Fatalf("listed for a play: %q, complete %v, %v", versionNames(versions), complete, err)
	}
	e.eventually("the addon to be asked again", func() bool { return e.asked("stream") == 3 })
	if got := e.pending(); got != 1 {
		t.Errorf("%d addons pending, want 1", got)
	}
	// Waiting for every version joins the request under way.
	waited := make(chan []string, 1)
	go func() {
		versions, _ := e.service.Versions(t.Context(), e.member, e.movie)
		waited <- versionNames(versions)
	}()
	e.numbers <- []int{1, 2, 3}
	if got := <-waited; len(got) != 3 || e.asked("stream") != 3 {
		t.Errorf("waited for %q, addon asked %d times", got, e.asked("stream"))
	}
	e.eventually("the follow-up", func() bool { return e.asked("stream") == 4 })
	e.numbers <- []int{1, 2, 3}
	e.settles("stream", 4)
}

// A play waits for an addon whose list is not known.
func TestAPlayWaitsForAnAddonNeverAsked(t *testing.T) {
	e := newStaleEnv(t)
	listed := make(chan []Version, 1)
	go func() {
		versions, complete, err := e.service.VersionsToPlay(t.Context(), e.member, e.movie)
		if err != nil || !complete {
			t.Errorf("complete %v, %v", complete, err)
		}
		listed <- versions
	}()
	select {
	case got := <-listed:
		t.Fatalf("listed before the addon answered: %q", versionNames(got))
	case <-time.After(100 * time.Millisecond):
	}
	e.numbers <- []int{1}
	if got := <-listed; !slices.Equal(versionNames(got), []string{"Source 1"}) {
		t.Errorf("listed: %q", versionNames(got))
	}
}

// renewEnv lists the movie's three streams, then gives the version of the
// third.
func renewEnv(t *testing.T, delays ...time.Duration) (staleEnv, Version) {
	t.Helper()
	e := newStaleEnv(t, delays...)
	e.numbers <- []int{1, 2, 3}
	versions, err := e.service.Versions(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	return e, versions[2]
}

// renewed renews old in the background, and gives its outcome.
func (e staleEnv) renewed(old Version) <-chan error {
	done := make(chan error, 1)
	go func() {
		version, err := e.service.Renew(e.t.Context(), old)
		if err == nil && version.ID != old.ID {
			err = errors.New("another version")
		}
		done <- err
	}()
	return done
}

// An addon that gathers other addons' streams may answer a renewal without
// the version's file: its follow-up is asked at once, and its answer
// renews the version. The list never loses the version meanwhile.
func TestARenewalWaitsForTheFollowUpOfAGatheringAddon(t *testing.T) {
	// The first answer's follow-up comes long after the test.
	e, old := renewEnv(t, time.Minute)
	started := time.Now()
	done := e.renewed(old)
	e.eventually("the renewal to ask the addon", func() bool { return e.asked("stream") == 2 })
	e.numbers <- []int{1}
	e.eventually("the follow-up asked at once", func() bool { return e.asked("stream") == 3 })
	if got := e.listed(); !slices.Equal(got, []string{"Source 1", "Source 2", "Source 3"}) {
		t.Errorf("listed while followed up: %q", got)
	}
	e.numbers <- []int{1, 2, 3}
	if err := <-done; err != nil {
		t.Fatalf("renewal: %v", err)
	}
	if elapsed := time.Since(started); elapsed > renewWait/2 {
		t.Errorf("renewed after %s", elapsed)
	}
	if got := e.listed(); !slices.Equal(got, []string{"Source 1", "Source 2", "Source 3"}) {
		t.Errorf("listed once renewed: %q", got)
	}
}

// A file the addon no longer lists, even asked again, is not renewed: the
// list then drops it.
func TestARenewalOfAFileGoneFails(t *testing.T) {
	e, old := renewEnv(t, time.Minute)
	done := e.renewed(old)
	e.eventually("the renewal to ask the addon", func() bool { return e.asked("stream") == 2 })
	e.numbers <- []int{1}
	e.eventually("the follow-up asked at once", func() bool { return e.asked("stream") == 3 })
	e.numbers <- []int{1}
	if err := <-done; !errors.Is(err, ErrNotFound) {
		t.Fatalf("renewal: %v", err)
	}
	e.eventually("the list to end with the addon's answer", func() bool { return slices.Equal(e.listed(), []string{"Source 1"}) })
}

// A renewal joins the request under way for the title, rather than asking
// the addon once more.
func TestARenewalJoinsTheRequestUnderWay(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond)
	e.expired()
	versions, err := e.service.KnownVersions(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	e.reopened()
	done := e.renewed(versions[2])
	time.Sleep(50 * time.Millisecond)
	e.numbers <- []int{1, 2, 3}
	if err := <-done; err != nil {
		t.Fatalf("renewal: %v", err)
	}
	if got := e.asked("stream"); got != 3 {
		t.Errorf("addon asked %d times, want 3", got)
	}
	e.eventually("the follow-up", func() bool { return e.asked("stream") == 4 })
	e.numbers <- []int{1, 2, 3}
	e.settles("stream", 4)
}

// restarted is a new service on the same database, as after a restart.
func (e staleEnv) restarted() *Service {
	s := New(e.service.db, e.addons, e.service.client, slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	s.followUpDelays = nil
	s.now = e.service.now
	return s
}

// The stream lists addons gave are saved: after a restart, a title lists
// the versions known before at once, as stale, while its addon is asked
// again. A refresh deletes them, and a day after they expired they are
// gone.
func TestStreamListsOutliveARestart(t *testing.T) {
	e := newStaleEnv(t)
	e.numbers <- []int{1, 2, 3}
	if versions, err := e.service.Versions(t.Context(), e.member, e.movie); err != nil || len(versions) != 3 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}

	s := e.restarted()
	if versions, complete := s.CachedVersions(t.Context(), e.member, e.movie); !complete || len(versions) != 3 {
		t.Errorf("listings after a restart: %q, complete %v", versionNames(versions), complete)
	}
	versions, complete, err := s.VersionsNow(t.Context(), e.member, e.movie)
	if err != nil || complete || !slices.Equal(versionNames(versions), []string{"Source 1", "Source 2", "Source 3"}) {
		t.Fatalf("details after a restart: %q, complete %v, %v", versionNames(versions), complete, err)
	}
	e.eventually("the addon to be asked again", func() bool { return e.asked("stream") == 2 })
	e.numbers <- []int{1, 2}
	e.eventually("the new answer", func() bool {
		versions, complete, _ := s.VersionsNow(t.Context(), e.member, e.movie)
		return complete && len(versions) == 2
	})
	// The new answer is saved in turn, once the follow-ups end; the list
	// kept in memory is trimmed just before it is saved.
	e.eventually("the new answer saved", func() bool {
		versions, _ := e.restarted().KnownVersions(t.Context(), e.member, e.movie)
		return slices.Equal(versionNames(versions), []string{"Source 1", "Source 2"})
	})
	// A day after the list expired, it is gone.
	e.wait(24*time.Hour + 11*time.Minute)
	if versions, _ := e.restarted().KnownVersions(t.Context(), e.member, e.movie); len(versions) != 0 {
		t.Errorf("a day after: %q", versionNames(versions))
	}
	e.wait(-24*time.Hour - 11*time.Minute)

	if err := s.Refresh(t.Context(), e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	if versions, _ := e.restarted().KnownVersions(t.Context(), e.member, e.movie); len(versions) != 0 {
		t.Errorf("after a refresh and a restart: %q", versionNames(versions))
	}
}

// A title's page, once its details answered, is told how far its versions
// came from what is kept in memory, and each change is told as it happens.
func TestFollowedTitlesAreToldOfTheirVersions(t *testing.T) {
	e := newFollowEnv(t, 20*time.Millisecond)
	var mu sync.Mutex
	var told []accounts.ID
	e.service.OnVersionsChanged(func(item accounts.ID) {
		mu.Lock()
		defer mu.Unlock()
		told = append(told, item)
	})
	if _, _, ok := e.service.FollowedProgress(t.Context(), e.member, e.movie); ok {
		t.Fatal("a title never opened is followed")
	}
	if _, _, err := e.service.VersionsNow(t.Context(), e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	pending, versions, ok := e.service.FollowedProgress(t.Context(), e.member, e.movie)
	if !ok || pending != 1 || len(versions) != 0 {
		t.Fatalf("while asked: %d pending, %d versions, followed %v", pending, len(versions), ok)
	}
	if other, _, ok := e.service.FollowedProgress(t.Context(), e.admin, e.movie); ok || other != 0 {
		t.Error("another user's page is followed")
	}
	e.replies <- 2
	e.eventually("the change to be told", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(told) > 0
	})
	e.replies <- 2
	e.eventually("the follow-up to end", func() bool {
		pending, versions, _ := e.service.FollowedProgress(t.Context(), e.member, e.movie)
		return pending == 0 && len(versions) == 2
	})
	// By one of its versions' identifiers too.
	if pending, got, ok := e.service.FollowedProgress(t.Context(), e.member, versionsOf(t, e)[1].ID); !ok || pending != 0 || len(got) != 2 {
		t.Errorf("by a version: %d pending, %d versions, followed %v", pending, len(got), ok)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, item := range told {
		if item != e.movie {
			t.Errorf("told of %v", item)
		}
	}
}

func versionsOf(t *testing.T, e followEnv) []Version {
	t.Helper()
	versions, err := e.service.KnownVersions(t.Context(), e.member, e.movie)
	if err != nil {
		t.Fatal(err)
	}
	return versions
}

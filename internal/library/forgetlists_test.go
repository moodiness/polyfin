package library

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// savedLists counts the stream lists saved in e's database.
func (e staleEnv) savedLists() int {
	e.t.Helper()
	var n int
	if err := e.service.db.QueryRow(e.t.Context(), "SELECT count(*) FROM stream_lists").Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// Forget the version lists forgets every addon's lists for every title,
// in memory and saved, and tells the title's pages: each title asks its
// addons again, after a restart too.
func TestForgettingTheVersionListsForgetsThemInMemoryAndSaved(t *testing.T) {
	t.Parallel()
	e := newStaleEnv(t)
	ctx := t.Context()
	var mu sync.Mutex
	var told []accounts.ID
	e.service.OnVersionsChanged(func(item accounts.ID) {
		mu.Lock()
		defer mu.Unlock()
		told = append(told, item)
	})
	e.numbers <- []int{1, 2}
	if versions, err := e.service.Versions(ctx, e.member, e.movie); err != nil || len(versions) != 2 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	e.numbers <- []int{1}
	if subtitles, err := e.service.Subtitles(ctx, e.member, e.movie); err != nil || len(subtitles) != 1 {
		t.Fatalf("subtitles: %v %v", subtitles, err)
	}
	if got := e.savedLists(); got != 1 {
		t.Fatalf("%d lists saved, want 1", got)
	}
	mu.Lock()
	told = nil
	mu.Unlock()

	forgotten, err := e.service.ForgetVersionLists(ctx)
	if err != nil || forgotten != 1 {
		t.Fatalf("forgot %d lists: %v, want 1", forgotten, err)
	}
	if got := e.listed(); len(got) != 0 {
		t.Errorf("listed once forgotten: %q", got)
	}
	if subtitles, _, err := e.service.KnownSubtitles(ctx, e.member, e.movie); err != nil || len(subtitles) != 0 {
		t.Errorf("subtitles known once forgotten: %v %v", subtitles, err)
	}
	if n := e.service.streamLists.Len() + e.service.subtitleLists.Len() + e.service.versions.Len(); n != 0 {
		t.Errorf("%d lists and versions kept in memory", n)
	}
	if got := e.savedLists(); got != 0 {
		t.Errorf("%d lists saved once forgotten", got)
	}
	if versions, _ := e.restarted().KnownVersions(ctx, e.member, e.movie); len(versions) != 0 {
		t.Errorf("listed after a restart: %q", versionNames(versions))
	}
	mu.Lock()
	if !slices.Contains(told, e.movie) {
		t.Errorf("told %v, want the movie", told)
	}
	mu.Unlock()

	// Opened again, the title asks its addon again.
	e.numbers <- []int{3}
	if versions, err := e.service.Versions(ctx, e.member, e.movie); err != nil || !slices.Equal(versionNames(versions), []string{"Source 3"}) {
		t.Fatalf("asked again: %q %v", versionNames(versions), err)
	}
	if got := e.asked("stream"); got != 2 {
		t.Errorf("addon asked %d times for streams, want 2", got)
	}
	// A list only saved, as after a restart, is forgotten and counted too.
	if forgotten, err := e.restarted().ForgetVersionLists(ctx); err != nil || forgotten != 1 {
		t.Errorf("forgot %d saved lists: %v, want 1", forgotten, err)
	}
	if got := e.savedLists(); got != 0 {
		t.Errorf("%d lists saved once forgotten after a restart", got)
	}
}

// Answers on their way when the version lists are forgotten are left out:
// a follow-up's, and a request's under way, whose addon is asked anew.
func TestAnAnswerOnItsWayIsNotKeptOnceTheVersionListsAreForgotten(t *testing.T) {
	t.Parallel()
	// Follow-ups wait an hour unless the test hurries them.
	e := newStaleEnv(t, time.Hour, time.Hour)
	ctx := t.Context()
	e.numbers <- []int{1}
	if versions, err := e.service.Versions(ctx, e.member, e.movie); err != nil || len(versions) != 1 {
		t.Fatalf("first answer: %q %v", versionNames(versions), err)
	}
	// The addon holds the follow-up while the lists are forgotten; its
	// answer, which lists more, comes after.
	e.eventually("the follow-up", func() bool { e.hurry(); return e.asked("stream") == 2 })
	if forgotten, err := e.service.ForgetVersionLists(ctx); err != nil || forgotten != 1 {
		t.Fatalf("forgot %d lists: %v, want 1", forgotten, err)
	}
	if e.scheduled() != 0 {
		t.Error("the follow-up still runs once the lists are forgotten")
	}
	e.numbers <- []int{1, 2, 3}
	e.settles("stream", 2)
	if got := e.listed(); len(got) != 0 {
		t.Errorf("the follow-up brought back a forgotten list: %q", got)
	}
	if got := e.savedLists(); got != 0 {
		t.Errorf("the follow-up saved a forgotten list: %d saved", got)
	}

	// Item details ask the addon in the background, and it holds its
	// answer while the lists are forgotten: that answer is left out, and
	// the addon is asked again.
	if _, _, err := e.service.VersionsNow(ctx, e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	e.eventually("the addon to be asked", func() bool { return e.asked("stream") == 3 })
	if _, err := e.service.ForgetVersionLists(ctx); err != nil {
		t.Fatal(err)
	}
	e.numbers <- []int{4}
	e.eventually("the addon to be asked again", func() bool { return e.asked("stream") == 4 })
	e.numbers <- []int{5}
	e.eventually("the answer after the forgetting", func() bool { return slices.Equal(e.listed(), []string{"Source 5"}) })
	if versions, _ := e.restarted().KnownVersions(ctx, e.member, e.movie); !slices.Equal(versionNames(versions), []string{"Source 5"}) {
		t.Errorf("saved: %q, want the answer after the forgetting", versionNames(versions))
	}
}

// A list still being saved when the version lists are forgotten is not
// saved back: forgetting waits for the save, then deletes it, so the
// versions forgotten do not come back, at the next opening nor after a
// restart.
func TestASaveUnderWayDoesNotUndoForgettingTheVersionLists(t *testing.T) {
	t.Parallel()
	e := newStaleEnv(t)
	ctx := t.Context()
	db := e.service.db
	var addon accounts.ID
	if err := db.QueryRow(ctx, "SELECT id FROM addons").Scan(&addon); err != nil {
		t.Fatal(err)
	}
	key := streamKey{addon, "movie", "ttm000"}
	// Another transaction holds a list under the same key, uncommitted:
	// saving the movie's list waits for it, while deleting it does not.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var holder int
	if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO stream_lists (addon_id, content_type, stremio_id, streams, answer, fetched_at)
		VALUES ($1, $2, $3, '{"streams": []}', 0, now())`, key.addon, key.contentType, key.id); err != nil {
		t.Fatal(err)
	}
	listed := make(chan []Version, 1)
	go func() {
		versions, _ := e.service.Versions(ctx, e.member, e.movie)
		listed <- versions
	}()
	e.numbers <- []int{1, 2, 3}
	e.eventually("the answer's save to wait", func() bool {
		var waiting int
		_ = db.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))", holder).Scan(&waiting)
		return waiting > 0
	})
	type result struct {
		forgotten int
		err       error
	}
	forgot := make(chan result, 1)
	go func() {
		n, err := e.service.ForgetVersionLists(ctx)
		forgot <- result{n, err}
	}()
	// Time for the forgetting to delete the list saved, if it does not
	// wait for the save.
	time.Sleep(100 * time.Millisecond)
	select {
	case r := <-forgot:
		t.Fatalf("forgot %d lists while a list was being saved: %v", r.forgotten, r.err)
	default:
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if versions := next(t, listed, "the answer"); len(versions) != 3 {
		t.Errorf("the answer: %q", versionNames(versions))
	}
	if r := next(t, forgot, "the forgetting"); r.err != nil || r.forgotten != 1 {
		t.Fatalf("forgot %d lists: %v, want 1", r.forgotten, r.err)
	}
	if versions, _ := e.service.KnownVersions(ctx, e.member, e.movie); len(versions) != 0 {
		t.Errorf("listed once forgotten: %q", versionNames(versions))
	}
	if versions, _ := e.restarted().KnownVersions(ctx, e.member, e.movie); len(versions) != 0 {
		t.Errorf("listed once forgotten and after a restart: %q", versionNames(versions))
	}
	if got := e.savedLists(); got != 0 {
		t.Errorf("%d lists saved once forgotten", got)
	}
}

// Pins and blocks are choices, not lists: forgetting the version lists
// keeps them, and the versions the addon lists again follow them, the
// blocked file left out and the pinned version found again by its
// identifier, for apps to list it first.
func TestPinsAndBlocksOutliveForgettingTheVersionLists(t *testing.T) {
	t.Parallel()
	e := newStaleEnv(t)
	ctx := t.Context()
	e.numbers <- []int{1, 2, 3}
	versions, err := e.service.Versions(ctx, e.member, e.movie)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	pinned := versions[2].ID
	if err := e.service.PinVersion(ctx, e.admin.ID, versions[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.BlockVersion(ctx, e.admin.ID, versions[0], false, "Broken"); err != nil {
		t.Fatal(err)
	}
	want := []string{"Source 2", "Source 3"}
	if got := e.listed(); !slices.Equal(got, want) {
		t.Fatalf("listed with the block: %q, want %q", got, want)
	}
	if forgotten, err := e.service.ForgetVersionLists(ctx); err != nil || forgotten != 1 {
		t.Fatalf("forgot %d lists: %v, want 1", forgotten, err)
	}
	for _, s := range []*Service{e.service, e.restarted()} {
		if pin, ok := s.PinnedVersion(ctx, e.movie); !ok || pin.Version != pinned {
			t.Errorf("pin once the lists are forgotten: %+v %v", pin, ok)
		}
		if blocks := s.VersionBlocks(ctx, e.movie); len(blocks) != 1 || blocks[0].Reason != "Broken" {
			t.Errorf("blocks once the lists are forgotten: %+v", blocks)
		}
	}
	e.numbers <- []int{1, 2, 3}
	versions, err = e.service.Versions(ctx, e.member, e.movie)
	if err != nil || !slices.Equal(versionNames(versions), want) {
		t.Fatalf("listed again: %q %v, want %q", versionNames(versions), err, want)
	}
	if versions[1].ID != pinned {
		t.Errorf("the pinned version listed again as %v, want %v", versions[1].ID, pinned)
	}
}

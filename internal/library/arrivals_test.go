package library

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// clock is a time tests move by hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// A title that comes into a library's catalog is new in the user's
// libraries for 7 days, the newest first; the titles there at the first
// read, and those of a page never read before, are not. Hidden titles and
// those parental control hides are left out.
func TestTitlesNewInACatalogStayNewForAWeek(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	at := &clock{now: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)}
	e.service.SetClock(at.Now)
	addon := pagedAddon(4, 2)
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	refreshed := withRefreshing(t.Context())
	read := func(start int) {
		t.Helper()
		if _, err := e.service.Children(refreshed, e.member, top.ID, start, 2, ""); err != nil {
			t.Fatal(err)
		}
	}
	arrivals := func(user accounts.User) []string {
		t.Helper()
		page, err := e.service.Arrivals(t.Context(), user, 0, 20, false)
		if err != nil {
			t.Fatal(err)
		}
		return names(page.Items)
	}
	// The first page as first read, then the second, never read before:
	// their titles were there before.
	read(0)
	read(2)
	if got := arrivals(e.member); len(got) != 0 {
		t.Fatalf("new before anything came: %v", got)
	}
	add := func(meta stremio.Meta) {
		addon.mu.Lock()
		addon.catalogs["movie/top"] = append([]stremio.Meta{meta}, addon.catalogs["movie/top"]...)
		addon.metas["movie/"+meta.ID] = meta
		addon.mu.Unlock()
	}
	add(stremio.Meta{ID: "ttnew1", Type: "movie", Name: "First new"})
	at.advance(2 * time.Hour)
	read(0)
	at.advance(time.Hour)
	add(stremio.Meta{ID: "ttnew2", Type: "movie", Name: "Second new", Extras: &stremio.Extras{Certification: "R"}})
	at.advance(2 * time.Hour)
	read(0)
	if got := arrivals(e.member); !slices.Equal(got, []string{"Second new", "First new"}) {
		t.Fatalf("new titles: %v", got)
	}
	page, err := e.service.Arrivals(t.Context(), e.member, 0, 1, true)
	if err != nil || !slices.Equal(names(page.Items), []string{"Second new"}) || !page.More {
		t.Errorf("a row of one: %v, more %v, %v", names(page.Items), page.More, err)
	}
	// Parental control leaves out the rated one.
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	if got := arrivals(child); !slices.Equal(got, []string{"First new"}) {
		t.Errorf("under parental control: %v", got)
	}
	// A title the user hid is left out.
	if err := e.service.userData.Hide(t.Context(), e.member.ID, itemID(titleKey(KindMovie, "ttnew1")), at.Now()); err != nil {
		t.Fatal(err)
	}
	if got := arrivals(e.member); !slices.Equal(got, []string{"Second new"}) {
		t.Errorf("with the first hidden: %v", got)
	}
	// A week after it came, the first is no longer new; the second goes 3
	// hours later.
	at.advance(ArrivalWindow - 2*time.Hour)
	if got := arrivals(e.admin); !slices.Equal(got, []string{"Second new"}) {
		t.Errorf("a week after the first came: %v", got)
	}
	at.advance(3 * time.Hour)
	if got := arrivals(e.admin); len(got) != 0 {
		t.Errorf("a week after the second came: %v", got)
	}
}

// A page read beyond what was read of a catalog does not extend it: the
// pages before it, read later, hold titles there all along, not new ones.
// After the upgrade from 0.37.0, which noted nothing, the pages it kept
// are read again as they go stale, the deeper ones first, as its Warm kept
// first pages fresher.
func TestPagesReadOutOfOrderBringNoArrivals(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	at := &clock{now: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)}
	e.service.SetClock(at.Now)
	addon := pagedAddon(4, 2)
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	refreshed := withRefreshing(t.Context())
	read := func(start int) {
		t.Helper()
		if _, err := e.service.Children(refreshed, e.member, top.ID, start, 2, ""); err != nil {
			t.Fatal(err)
		}
	}
	// What 0.37.0 left: both pages kept, the first refreshed later, and
	// nothing noted of them.
	forget := func() {
		t.Helper()
		if _, err := e.service.db.Exec(t.Context(), "DELETE FROM catalog_reach"); err != nil {
			t.Fatal(err)
		}
	}
	read(0)
	read(2)
	forget()
	at.advance(70 * time.Minute)
	read(0)
	forget()
	// After the upgrade, the second page goes stale first, then the first.
	at.advance(10 * time.Minute)
	read(2)
	at.advance(time.Hour)
	read(0)
	page, err := e.service.Arrivals(t.Context(), e.member, 0, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(page.Items); len(got) != 0 {
		t.Errorf("titles there all along, listed as new: %v", got)
	}
	// The catalog's reach grows again from its first page: a title that
	// comes to it then is new.
	meta := stremio.Meta{ID: "ttnew", Type: "movie", Name: "New"}
	addon.mu.Lock()
	addon.catalogs["movie/top"] = append([]stremio.Meta{meta}, addon.catalogs["movie/top"]...)
	addon.metas["movie/ttnew"] = meta
	addon.mu.Unlock()
	at.advance(2 * time.Hour)
	read(0)
	if page, err = e.service.Arrivals(t.Context(), e.member, 0, 20, false); err != nil || !slices.Equal(names(page.Items), []string{"New"}) {
		t.Errorf("new titles: %v, %v", names(page.Items), err)
	}
}

// Forgetting the titles no read saw for long, as Warm does, while a page
// that holds them again is noted, takes the rows in the same order: both
// end, and the titles seen again are kept.
func TestForgettingArrivalsWhileNotingThem(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	db := e.service.db
	at := &clock{now: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)}
	e.service.SetClock(at.Now)
	e.install(addons.Shared(), pagedAddon(2, 2))
	top := e.library(e.member, "Top")
	refreshed := withRefreshing(ctx)
	if _, err := e.service.Children(refreshed, e.member, top.ID, 0, 2, ""); err != nil {
		t.Fatal(err)
	}
	// The two titles were last seen long ago, and are stored in reverse
	// key order, the order a table scan reads them in; the catalog was
	// read lately.
	rows, _ := db.Query(ctx, "SELECT item_id FROM catalog_arrivals ORDER BY item_id DESC")
	ids, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil || len(ids) != 2 {
		t.Fatalf("titles noted: %v, %v", ids, err)
	}
	if _, err := db.Exec(ctx, `WITH old AS (DELETE FROM catalog_arrivals RETURNING *)
		INSERT INTO catalog_arrivals SELECT * FROM old ORDER BY item_id DESC`); err != nil {
		t.Fatal(err)
	}
	at.advance(arrivalForgotten + time.Hour)
	if _, err := db.Exec(ctx, "UPDATE catalog_reach SET read_at = $1", at.Now()); err != nil {
		t.Fatal(err)
	}
	at.advance(time.Hour)
	// Another transaction holds the first title in key order: noting the
	// page waits for it, then forgetting waits too.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, "SELECT 1 FROM catalog_arrivals WHERE item_id = $1 FOR UPDATE", ids[1]); err != nil {
		t.Fatal(err)
	}
	waiting := func(statement string) func() bool {
		return func() bool {
			var count int
			_ = db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
				AND wait_event_type = 'Lock' AND query LIKE $1`, statement+"%").Scan(&count)
			return count > 0
		}
	}
	noted := make(chan error, 1)
	go func() {
		_, err := e.service.Children(refreshed, e.member, top.ID, 0, 2, "")
		noted <- err
	}()
	eventually(t, "the page's titles to wait", waiting("INSERT INTO catalog_arrivals"))
	forgotten := make(chan error, 1)
	go func() { forgotten <- e.service.forgetArrivals(ctx) }()
	eventually(t, "forgetting to wait", waiting("DELETE FROM catalog_arrivals"))
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-noted; err != nil {
		t.Fatal(err)
	}
	if err := <-forgotten; err != nil {
		t.Errorf("forgetting: %v", err)
	}
	var kept int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM catalog_arrivals WHERE last_seen = $1", at.Now()).Scan(&kept); err != nil || kept != 2 {
		t.Errorf("titles seen again and kept: %d, %v", kept, err)
	}
}

package library

import (
	"slices"
	"sync"
	"testing"
	"time"

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

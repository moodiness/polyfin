package jellyfin

import (
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/library"
)

// scheduleDate is a date as schedules keep them.
func scheduleDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// A row shows only inside its schedule: on its days, in its hours, a night
// past midnight belonging to the day it starts on. A row that is also a
// library follows its dates only: it is a view of /UserViews, and a folder,
// from its first date to its last, at every hour of every day.
func TestRowsShowInsideTheirSchedule(t *testing.T) {
	t.Parallel()
	s, member, _, addon, _ := homeRowsServer(t)
	var clock atomic.Pointer[time.Time]
	set := func(year int, month time.Month, day, hour, minute int) {
		at := time.Date(year, month, day, hour, minute, 0, 0, time.Local)
		clock.Store(&at)
	}
	set(2026, time.November, 6, 20, 0) // a Friday
	s.handler.now = func() time.Time { return *clock.Load() }
	s.addRow(t, homerows.Row{Name: "Friday night", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Everyone: true,
		Schedule: homerows.Schedule{Days: []time.Weekday{time.Friday}, Hours: &homerows.Hours{From: 18, To: 2}}})
	december := s.addRow(t, homerows.Row{Name: "December", Kind: homerows.KindCatalog, Place: homerows.PlaceBottom, Addon: addon,
		CatalogType: "series", CatalogID: "shows", Everyone: true, Library: true,
		Schedule: homerows.Schedule{Start: scheduleDate(2026, time.December, 1), End: scheduleDate(2026, time.December, 31),
			Days: []time.Weekday{time.Saturday}, Hours: &homerows.Hours{From: 9, To: 10}}})
	folder := customRowID(december.ID, accounts.ID{}).String()
	check := func(when string, rows []string, library bool) {
		t.Helper()
		if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, rows) {
			t.Errorf("%s: rows %q", when, got)
		}
		var views QueryResult
		s.get(t, "/UserViews", member, &views)
		if got := slices.Contains(itemNames(views.Items), "December"); got != library {
			t.Errorf("%s: views %v", when, itemNames(views.Items))
		}
		status := s.get(t, "/Items?ParentId="+folder, member, nil)
		if (status == http.StatusOK) != library {
			t.Errorf("%s: the December row's folder: %d", when, status)
		}
	}
	friday := "Friday night @Top: Movie 0, Movie 1, Movie 2"
	check("Friday 20:00", []string{friday}, false)
	set(2026, time.November, 7, 1, 30)
	check("Saturday 1:30, Friday's night", []string{friday}, false)
	set(2026, time.November, 7, 2, 0)
	check("Saturday 2:00", []string{}, false)
	set(2026, time.November, 6, 1, 30)
	check("Friday 1:30, Thursday's night", []string{}, false)
	set(2026, time.November, 6, 17, 59)
	check("Friday 17:59", []string{}, false)
	// A Wednesday of December at 15:00, outside the December row's days
	// and hours: being a library, it follows its dates only.
	set(2026, time.December, 2, 15, 0)
	check("December 2", []string{"December @Bottom: Show"}, true)
	set(2026, time.December, 31, 23, 59)
	check("December 31", []string{"December @Bottom: Show"}, true)
	set(2027, time.January, 1, 0, 0)
	check("January 1", []string{}, false)
}

// The preview of a user's home rows is what the web player signed in as
// them shows: the same rows, in the same order, with the same titles,
// their parental control, hidden titles, rows turned off and schedules
// applied.
func TestPreviewShowsTheRowsAsTheWebPlayerDoes(t *testing.T) {
	t.Parallel()
	s, member, admin, addon, movies := homeRowsServer(t)
	now := time.Date(2026, time.November, 6, 20, 0, 0, 0, time.Local)
	s.handler.now = func() time.Time { return now }
	child := s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	childToken := s.signIn("child", "tablet")
	other := s.user("other", nil)
	otherToken := s.signIn("other", "phone")
	s.addRow(t, homerows.Row{Name: "Series", Kind: homerows.KindCatalog, Place: homerows.PlaceBottom, Addon: addon,
		CatalogType: "series", CatalogID: "shows", Everyone: true})
	s.addRow(t, homerows.Row{Name: "Movies", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Everyone: true})
	s.addRow(t, homerows.Row{Name: "For the other", Kind: homerows.KindCatalog, Place: homerows.PlaceNextUp, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Users: []accounts.ID{other.ID}})
	s.addRow(t, homerows.Row{Name: "Mornings", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Everyone: true, Schedule: homerows.Schedule{Hours: &homerows.Hours{From: 6, To: 12}}})
	turnedOff := s.addRow(t, homerows.Row{Name: "Turned off", Kind: homerows.KindNew, Place: homerows.PlaceTop, Everyone: true})
	var me UserDto
	s.get(t, "/Users/Me", member, &me)
	memberID, _ := accounts.ParseID(me.Id)
	if err := homerows.New(s.pool).SetOff(t.Context(), memberID, turnedOff.ID, true); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.call(http.MethodPost, "/Polyfin/UserItems/"+movies[0].Id+"/Hidden", app("tv", member), nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	s.get(t, "/Users/Me", admin, &me)
	adminID, _ := accounts.ParseID(me.Id)
	// summary tells a row's folder, name, place, kind and titles.
	summary := func(id, name, place, kind string, titles []string) string {
		return id + " " + name + " @" + place + " " + kind + ": " + strings.Join(titles, ", ")
	}
	for _, viewer := range []struct {
		name  string
		id    accounts.ID
		token string
		want  []string
	}{
		{"member", memberID, member, []string{"Series", "Movies"}},
		{"child", child.ID, childToken, []string{"Movies"}},
		{"other", other.ID, otherToken, []string{"Series", "Movies", "For the other"}},
		{"admin", adminID, admin, []string{"Series", "Movies"}},
	} {
		user, err := s.store.User(t.Context(), viewer.id)
		if err != nil {
			t.Fatal(err)
		}
		previewed, err := s.handler.PreviewHomeRows(t.Context(), user)
		if err != nil {
			t.Fatal(err)
		}
		var preview, answered []string
		var names []string
		for _, row := range previewed {
			var titles []string
			for _, title := range row.Titles {
				titles = append(titles, title.ID.String())
			}
			preview = append(preview, summary(row.ID.String(), row.Name, customRowPlaces[row.Row.Place], customRowKinds[row.Row.Kind], titles))
			names = append(names, row.Name)
		}
		for _, row := range s.customRowsOf(t, viewer.token) {
			var titles []string
			for _, item := range row.Items {
				titles = append(titles, item.Id)
			}
			answered = append(answered, summary(row.Id, row.Name, row.Place, row.Kind, titles))
		}
		if !slices.Equal(preview, answered) {
			t.Errorf("%s: the preview %q, the web player %q", viewer.name, preview, answered)
		}
		if !slices.Equal(names, viewer.want) {
			t.Errorf("%s: the rows previewed %q", viewer.name, names)
		}
	}
	// The member hid Movie 0, which the others still see.
	user, err := s.store.User(t.Context(), memberID)
	if err != nil {
		t.Fatal(err)
	}
	previewed, err := s.handler.PreviewHomeRows(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(previewed, func(row HomeRowPreview) bool { return row.Name == "Movies" }); i < 0 ||
		slices.ContainsFunc(previewed[i].Titles, func(title library.Item) bool { return title.ID.String() == movies[0].Id }) {
		t.Errorf("the member's Movies row: %+v", previewed)
	}
}

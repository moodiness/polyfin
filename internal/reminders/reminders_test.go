package reminders

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// fakeGuide is a guide whose programmes tests change, as a guide refreshed
// with programmes moved or gone: each programme is an XMLTV one, whose
// identifier is its channel and start.
type fakeGuide struct {
	mu       sync.Mutex
	programs []library.Item
	channels []library.Item
}

func (g *fakeGuide) Guide(_ context.Context, _ accounts.User, q library.GuideQuery) ([]library.Item, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	var found []library.Item
	for _, p := range g.programs {
		if p.EndDate.After(q.From) && p.StartDate.Before(q.To) && (q.Channels == nil || slices.Contains(q.Channels, p.Channel.ID)) {
			found = append(found, p)
		}
	}
	return found, nil
}

func (g *fakeGuide) Item(_ context.Context, _ accounts.User, id accounts.ID) (library.Item, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range g.programs {
		if p.ID == id {
			return p, nil
		}
	}
	return library.Item{}, library.ErrNotFound
}

func (g *fakeGuide) Channels(context.Context, accounts.User) ([]library.Item, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.channels), nil
}

// set replaces the guide's programmes.
func (g *fakeGuide) set(programs ...library.Item) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.programs = programs
}

// fakeTeller keeps what it was told.
type fakeTeller struct {
	mu   sync.Mutex
	told []Due
}

func (f *fakeTeller) ProgrammeStarting(due Due) (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.told = append(f.told, due)
	return "Starting at " + due.Reminder.Start.Format("15:04") + ": " + due.Reminder.Name, due.Reminder.Name + " starts soon."
}

// take returns what was told since the last take.
func (f *fakeTeller) take() []Due {
	f.mu.Lock()
	defer f.mu.Unlock()
	told := f.told
	f.told = nil
	return told
}

// clock is a time tests move.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

var (
	channelX = library.Item{ID: accounts.ID{0xa1}, Kind: library.KindChannel, Name: "Example TV"}
	channelY = library.Item{ID: accounts.ID{0xb2}, Kind: library.KindChannel, Name: "Other TV"}
	// day is the day the tests start on, at noon, in UTC.
	day = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
)

// program is a programme of channel starting at start, for an hour, its
// identifier made of its channel and start as an XMLTV programme's is.
func program(channel library.Item, name string, start time.Time) library.Item {
	end := start.Add(time.Hour)
	id := accounts.ID{channel.ID[0], 0x50}
	unix := start.Unix()
	for i := range 8 {
		id[8+i] = byte(unix >> (8 * i))
	}
	return library.Item{ID: id, Kind: library.KindProgram, Name: name, StartDate: &start, EndDate: &end, Channel: &channel}
}

type fixture struct {
	*Service
	pool   *pgxpool.Pool
	store  *accounts.Store
	guide  *fakeGuide
	teller *fakeTeller
	clock  *clock
	user   accounts.User
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := migrated.New(t)
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(t.Context(), accounts.NewUser{Name: "viewer", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{pool: pool, store: store, guide: &fakeGuide{channels: []library.Item{channelX, channelY}}, teller: &fakeTeller{},
		clock: &clock{now: day}, user: user}
	f.Service = f.restart()
	return f
}

// restart is another service on the same database, as after a restart.
func (f fixture) restart() *Service {
	return New(Config{DB: f.pool, Guide: f.guide, Users: f.store, Teller: f.teller, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now: f.clock.Now})
}

// at checks the reminders due at t.
func (f fixture) at(t time.Time) {
	f.clock.set(t)
	f.Check(context.Background())
}

func names(told []Due) []string {
	var found []string
	for _, due := range told {
		found = append(found, due.Reminder.Name+" "+due.Reminder.Start.UTC().Format("Jan 2 15:04")+" on "+due.Reminder.ChannelName)
	}
	return found
}

// A reminder is told once, at the lead time the user chose, with the time
// left; the programme's page then shows no reminder waiting.
func TestReminderIsToldOnceAtTheLeadTime(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	show := program(channelX, "Example Show", day.Add(8*time.Hour+30*time.Minute))
	f.guide.set(show)
	if err := f.SetSettings(t.Context(), f.user.ID, Settings{Before: 10}); err != nil {
		t.Fatal(err)
	}
	state, err := f.Create(t.Context(), f.user, show.ID, false, "fr-FR")
	if err != nil || state.Reminder == nil || state.Series != nil || state.Started || state.Reaches {
		t.Fatalf("setting a reminder: %+v %v", state, err)
	}
	if state.Reminder.Language != "fr" || state.Reminder.ChannelName != "Example TV" {
		t.Errorf("the reminder kept %+v", state.Reminder)
	}
	f.at(show.StartDate.Add(-11 * time.Minute))
	if told := f.teller.take(); len(told) != 0 {
		t.Fatalf("told 11 minutes before: %v", names(told))
	}
	f.at(show.StartDate.Add(-10 * time.Minute))
	told := f.teller.take()
	if len(told) != 1 || told[0].Left != 10*time.Minute || told[0].User.ID != f.user.ID || told[0].Reminder.Name != "Example Show" {
		t.Fatalf("told at the lead time: %+v", told)
	}
	f.at(show.StartDate.Add(-5 * time.Minute))
	if told := f.teller.take(); len(told) != 0 {
		t.Errorf("told again: %v", names(told))
	}
	if state, err := f.State(t.Context(), f.user, show.ID); err != nil || state.Reminder != nil {
		t.Errorf("a reminder sent still waits: %+v %v", state, err)
	}
	if err := f.SetSettings(t.Context(), f.user.ID, Settings{Before: 7}); !errors.Is(err, ErrInvalid) {
		t.Errorf("7 minutes before: %v", err)
	}
}

// A series reminder reminds of the airings of its title on its channel at
// its time of day, within ten minutes, as a series timer finds them: not
// of the same title at another time, nor on another channel.
func TestSeriesReminderCoversTheSameTimeOfDayOnTheSameChannel(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first := program(channelX, "Example Show", day.Add(8*time.Hour))
	tomorrow := program(channelX, "example show ", day.Add(32*time.Hour+5*time.Minute))
	third := program(channelX, "Example Show", day.Add(56*time.Hour-3*time.Minute))
	afternoon := program(channelX, "Example Show", day.Add(26*time.Hour))
	elsewhere := program(channelY, "Example Show", day.Add(56*time.Hour))
	other := program(channelX, "Another Show", day.Add(80*time.Hour))
	f.guide.set(first, tomorrow, third, afternoon, elsewhere, other)
	state, err := f.Create(t.Context(), f.user, first.ID, true, "en")
	if err != nil || state.Reminder == nil || state.Series == nil {
		t.Fatalf("setting a series reminder: %+v %v", state, err)
	}
	f.RefreshSeries(t.Context())
	for _, p := range []library.Item{tomorrow, third} {
		if s, err := f.State(t.Context(), f.user, p.ID); err != nil || s.Reminder == nil || s.Series == nil || s.Series.ID != state.Series.ID {
			t.Errorf("%s at %s is not reminded of by the series: %+v %v", p.Name, p.StartDate, s, err)
		}
	}
	for _, p := range []library.Item{afternoon, elsewhere, other} {
		if s, err := f.State(t.Context(), f.user, p.ID); err != nil || s.Reminder != nil {
			t.Errorf("%s at %s on %s has a reminder: %+v %v", p.Name, p.StartDate, p.Channel.Name, s.Reminder, err)
		}
	}
	// A series reminder asked again for one of its airings is the same.
	if again, err := f.Create(t.Context(), f.user, tomorrow.ID, true, "en"); err != nil || again.Series == nil || again.Series.ID != state.Series.ID {
		t.Fatalf("a second series reminder of the same airings: %+v %v", again, err)
	}
	list, series, err := f.List(t.Context(), f.user)
	if err != nil || len(list) != 0 || len(series) != 1 || series[0].Next == nil || !series[0].Next.Equal(*first.StartDate) ||
		series[0].ChannelName != "Example TV" {
		t.Fatalf("the reminders listed: %+v %+v %v", list, series, err)
	}
	for _, p := range []library.Item{first, afternoon, tomorrow, third, elsewhere, other} {
		f.at(p.StartDate.Add(-5 * time.Minute))
	}
	want := []string{"Example Show Oct 14 20:00 on Example TV", "example show Oct 15 20:05 on Example TV", "Example Show Oct 16 19:57 on Example TV"}
	if got := names(f.teller.take()); !slices.Equal(got, want) {
		t.Errorf("told %q, want %q", got, want)
	}
	// Deleting the series deletes what it made, and makes no more.
	if err := f.Delete(t.Context(), f.user.ID, state.Series.ID); err != nil {
		t.Fatal(err)
	}
	f.RefreshSeries(t.Context())
	if _, series, _ := f.List(t.Context(), f.user); len(series) != 0 {
		t.Errorf("a series reminder deleted is listed: %+v", series)
	}
}

// A programme the guide moves within the hour, later or earlier, is told
// at its new start; one gone from the guide is not told.
func TestRemindersFollowTheGuide(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	later := program(channelX, "Later Show", day.Add(8*time.Hour))
	earlier := program(channelX, "Earlier Show", day.Add(10*time.Hour))
	gone := program(channelX, "Gone Show", day.Add(9*time.Hour))
	f.guide.set(later, earlier, gone)
	for _, p := range []library.Item{later, earlier, gone} {
		if _, err := f.Create(t.Context(), f.user, p.ID, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	// The guide is refreshed: an XMLTV programme moved is another
	// identifier.
	movedLater := program(channelX, "Later Show", day.Add(8*time.Hour+40*time.Minute))
	movedEarlier := program(channelX, "Earlier Show", day.Add(9*time.Hour+30*time.Minute))
	f.guide.set(movedLater, movedEarlier)
	f.at(later.StartDate.Add(-5 * time.Minute))
	if told := f.teller.take(); len(told) != 0 {
		t.Fatalf("told at the old start: %v", names(told))
	}
	f.at(movedLater.StartDate.Add(-5 * time.Minute))
	f.at(gone.StartDate.Add(-5 * time.Minute))
	f.at(movedEarlier.StartDate.Add(-5 * time.Minute))
	f.at(earlier.StartDate.Add(-5 * time.Minute))
	told := f.teller.take()
	if got, want := names(told), []string{"Later Show Oct 14 20:40 on Example TV", "Earlier Show Oct 14 21:30 on Example TV"}; !slices.Equal(got, want) {
		t.Fatalf("told %q, want %q", got, want)
	}
	if told[0].Reminder.Program != movedLater.ID || told[1].Left != 5*time.Minute {
		t.Errorf("the programmes told: %+v", told)
	}
	if list, _, _ := f.List(t.Context(), f.user); len(list) != 0 {
		t.Errorf("reminders still wait: %+v", list)
	}
}

// Reminders are kept in the database: after a restart, none is sent
// twice, and one due while Polyfin was off is sent on start only before
// its programme begins.
func TestRestartsSendNothingTwice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sent := program(channelX, "Sent Show", day.Add(time.Hour))
	missed := program(channelX, "Missed Show", day.Add(2*time.Hour))
	late := program(channelX, "Late Show", day.Add(3*time.Hour))
	f.guide.set(sent, missed, late)
	for _, p := range []library.Item{sent, missed, late} {
		if _, err := f.Create(t.Context(), f.user, p.ID, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	f.at(sent.StartDate.Add(-5 * time.Minute))
	if got := names(f.teller.take()); len(got) != 1 {
		t.Fatalf("told before the restart: %q", got)
	}
	// Polyfin was off from the first reminder until two minutes before the
	// second programme, then again until after the third began.
	restarted := f
	restarted.Service = f.restart()
	restarted.at(missed.StartDate.Add(-2 * time.Minute))
	told := f.teller.take()
	if got := names(told); !slices.Equal(got, []string{"Missed Show Oct 14 14:00 on Example TV"}) || told[0].Left != 2*time.Minute {
		t.Fatalf("told after the first restart: %q", got)
	}
	again := f
	again.Service = f.restart()
	again.at(late.StartDate.Add(time.Minute))
	again.at(late.StartDate.Add(30 * time.Minute))
	if got := names(f.teller.take()); len(got) != 0 {
		t.Errorf("told after the second restart: %q", got)
	}
}

// Reminders need the Live TV permission, and follow parental control and
// blocked genres: a user without it cannot set one, and one who loses it,
// or is disabled, is told nothing.
func TestRemindersNeedLiveTvAndFollowParentalControl(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	news := program(channelX, "News", day.Add(time.Hour))
	horror := program(channelX, "Night Show", day.Add(2*time.Hour))
	horror.Genres = []string{"Horror"}
	disabled := program(channelX, "Late News", day.Add(3*time.Hour))
	f.guide.set(news, horror, disabled)
	if _, err := f.Create(t.Context(), f.user, news.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Create(t.Context(), f.user, disabled.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	blocked := []string{"horror"}
	user, err := f.store.UpdateUser(t.Context(), f.user.ID, accounts.UserChanges{BlockedGenres: &blocked}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Create(t.Context(), user, horror.ID, false, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("a reminder of a blocked genre: %v", err)
	}
	off := false
	if user, err = f.store.UpdateUser(t.Context(), f.user.ID, accounts.UserChanges{LiveTv: &off}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Create(t.Context(), user, news.ID, false, ""); !errors.Is(err, ErrForbidden) {
		t.Errorf("a reminder without Live TV: %v", err)
	}
	if list, series, err := f.List(t.Context(), user); err != nil || len(list) != 0 || len(series) != 0 {
		t.Errorf("listed without Live TV: %+v %+v %v", list, series, err)
	}
	f.at(news.StartDate.Add(-5 * time.Minute))
	on, disable := true, true
	if _, err := f.store.UpdateUser(t.Context(), f.user.ID, accounts.UserChanges{LiveTv: &on, IsDisabled: &disable}, nil); err != nil {
		t.Fatal(err)
	}
	f.at(disabled.StartDate.Add(-5 * time.Minute))
	if told := f.teller.take(); len(told) != 0 {
		t.Errorf("told a user who may not watch: %q", names(told))
	}
}

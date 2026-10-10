package plays

import (
	"bytes"
	"encoding/csv"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

type historyHarness struct {
	*History
	store      *accounts.Store
	alice, bob accounts.User
	now        time.Time
}

func newHistoryHarness(t *testing.T) historyHarness {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	h := historyHarness{store: store, now: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	if h.alice, err = store.CreateUser(t.Context(), accounts.NewUser{Name: "alice", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	if h.bob, err = store.CreateUser(t.Context(), accounts.NewUser{Name: "bob", Password: "correct horse"}); err != nil {
		t.Fatal(err)
	}
	h.History = NewHistory(pool, store.Settings, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.History.now = func() time.Time { return h.now }
	return h
}

// played keeps a video user played for played, starting ago before now.
func (h historyHarness) played(t *testing.T, user accounts.User, title Title, ago, played time.Duration, method string) {
	t.Helper()
	start := h.now.Add(-ago)
	h.Record(t.Context(), Change{Kind: Stopped, Playback: Playback{User: user.ID, UserName: user.Name, App: "Web", DeviceName: "Phone",
		Title: title, Started: start, LastReport: start.Add(played), Ended: start.Add(played), Played: played, Method: method}})
}

var (
	pilot = Title{Item: accounts.ID{0x21}, Kind: Episode, Name: "Pilot", SeriesID: accounts.ID{0x20}, SeriesName: "Harbour",
		Season: 1, Episode: 1}
	news = Title{Item: accounts.ID{0x31}, Kind: Channel, Name: "News", ChannelID: accounts.ID{0x31}, ChannelName: "News"}
	song = Title{Item: accounts.ID{0x41}, Kind: Song, Name: "Tide", Artist: "Sailors"}
)

// The statistics of a period sum the playbacks that started in it, per
// user and for everyone, and leave out what a user did not play.
func TestStatisticsSumAPeriodAndAUser(t *testing.T) {
	h := newHistoryHarness(t)
	day := 24 * time.Hour
	h.played(t, h.alice, movie, 2*day, time.Hour, DirectPlay)
	h.played(t, h.alice, pilot, 3*day, 40*time.Minute, Conversion)
	h.played(t, h.bob, movie, 1*day, 30*time.Minute, DirectStream)
	h.played(t, h.bob, news, 20*day, 2*time.Hour, DirectPlay)
	// Neither a song nor a playback too short to count is kept.
	h.played(t, h.bob, song, day, 3*time.Minute, DirectPlay)
	h.played(t, h.bob, other, day, 5*time.Second, DirectPlay)

	week, err := h.Statistics(t.Context(), Filter{Since: h.now.Add(-7 * day)}, Day, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if week.Plays != 3 || week.Played != 130*time.Minute {
		t.Errorf("the week has %d plays, %v; want 3, 2h10m", week.Plays, week.Played)
	}
	if len(week.Users) != 2 || week.Users[0].Name != "alice" || week.Users[0].Played != 100*time.Minute ||
		week.Users[1].Played != 30*time.Minute {
		t.Errorf("users %+v, want alice 1h40m then bob 30m", week.Users)
	}
	if len(week.Movies) != 1 || week.Movies[0].Plays != 2 || week.Movies[0].Users != 2 || week.Movies[0].Played != 90*time.Minute {
		t.Errorf("movies %+v, want Night Train played twice by two users, 1h30m", week.Movies)
	}
	if len(week.Series) != 1 || week.Series[0].Name != "Harbour" || len(week.Channels) != 0 {
		t.Errorf("series %+v and channels %+v, want Harbour and none", week.Series, week.Channels)
	}
	var days time.Duration
	for _, b := range week.Buckets {
		days += b.Played
	}
	var hours time.Duration
	for _, played := range week.Hours {
		hours += played
	}
	if len(week.Buckets) != 3 || days != week.Played || hours.Round(time.Second) != week.Played {
		t.Errorf("%d days summing %v, hours summing %v; want 3 days, both %v", len(week.Buckets), days, hours, week.Played)
	}

	month, err := h.Statistics(t.Context(), Filter{Since: h.now.Add(-30 * day), User: &h.bob.ID}, Day, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if month.Plays != 2 || month.Played != 150*time.Minute || len(month.Users) != 1 || month.Users[0].User != h.bob.ID {
		t.Errorf("bob's month has %d plays, %v, users %+v; want 2, 2h30m, bob alone", month.Plays, month.Played, month.Users)
	}
	if len(month.Channels) != 1 || month.Channels[0].Name != "News" {
		t.Errorf("channels %+v, want News", month.Channels)
	}
	methods := map[string]time.Duration{}
	for _, m := range month.Methods {
		methods[m.Name] = m.Played
	}
	if methods[DirectPlay] != 2*time.Hour || methods[DirectStream] != 30*time.Minute || methods[Conversion] != 0 {
		t.Errorf("methods %v, want 2h played directly and 30m remuxed", methods)
	}
}

// The history keeps the days the settings choose, and nothing while it is
// turned off.
func TestHistoryRetention(t *testing.T) {
	h := newHistoryHarness(t)
	day := 24 * time.Hour
	settings := h.store.Settings()
	settings.PlaybackHistoryDays = 30
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.played(t, h.alice, movie, 31*day, time.Hour, DirectPlay)
	h.played(t, h.alice, pilot, 29*day, time.Hour, DirectPlay)
	deleted, err := h.Sweep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	entries, total, err := h.Entries(t.Context(), Filter{}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || total != 1 || entries[0].Title.Name != "Pilot" {
		t.Errorf("deleted %d, kept %d; want the 31-day-old playback deleted, the 29-day-old kept", deleted, total)
	}

	settings.PlaybackHistory = false
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.played(t, h.alice, movie, day, time.Hour, DirectPlay)
	if _, total, _ := h.Entries(t.Context(), Filter{}, 0, 10); total != 1 {
		t.Errorf("%d playbacks kept, want 1: the history is off", total)
	}
}

// The CSV export holds the playbacks of one user only when asked for
// one, and writes a name that reads as a formula as text.
func TestExportOfOneUser(t *testing.T) {
	h := newHistoryHarness(t)
	formula := movie
	formula.Name = "=HYPERLINK(\"x\")"
	h.played(t, h.alice, formula, time.Hour, time.Hour, DirectPlay)
	h.played(t, h.bob, pilot, time.Hour, time.Hour, Conversion)
	var out bytes.Buffer
	if err := h.WriteCSV(t.Context(), &out, Filter{User: &h.alice.ID}); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[1][2] != "alice" || records[1][4] != "'"+formula.Name {
		t.Errorf("exported %q, want alice's playback alone, its name as text", records)
	}
}

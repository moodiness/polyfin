package recordings

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/testdb"
)

// guide answers the programmes it holds.
type guide struct{ programs []library.Item }

func (g *guide) Programs(context.Context, accounts.User, time.Time, time.Time) ([]library.Item, error) {
	return g.programs, nil
}

func (g *guide) Versions(context.Context, accounts.User, accounts.ID) ([]library.Version, error) {
	return nil, nil
}

// recorder joins parts by concatenating them.
type recorder struct{}

func (recorder) Record(context.Context, accounts.ID, library.Version, string, string) error {
	return nil
}
func (recorder) Failed(accounts.ID) bool { return false }
func (recorder) ForgetFile(accounts.ID)  {}
func (recorder) FinishRecording(_ context.Context, parts []string, path string) error {
	var joined []byte
	for _, part := range parts {
		data, err := os.ReadFile(part)
		if err != nil {
			return err
		}
		joined = append(joined, data...)
	}
	return os.WriteFile(path, joined, 0o600)
}

type fixture struct {
	service *Service
	store   *accounts.Store
	guide   *guide
	user    accounts.User
	dir     string
	// folder is the recordings folder the service is given, f.dir at
	// first; empty turns recording off.
	folder *atomic.Pointer[string]
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(t.Context(), accounts.NewUser{Name: "member", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{store: store, guide: &guide{}, user: user, dir: t.TempDir(), folder: &atomic.Pointer[string]{}}
	f.folder.Store(&f.dir)
	f.service = New(Config{DB: pool, Folder: func() string { return *f.folder.Load() }, Guide: f.guide, Recorder: recorder{}, Users: store,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return f
}

func id(t *testing.T, s string) accounts.ID {
	t.Helper()
	parsed, err := accounts.ParseID(s)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func program(t *testing.T, programID, name string, channel accounts.ID, start time.Time) library.Item {
	t.Helper()
	end := start.Add(time.Hour)
	return library.Item{ID: id(t, programID), Kind: library.KindProgram, Name: name, StartDate: &start, EndDate: &end,
		Channel: &library.Item{ID: channel, Kind: library.KindChannel}}
}

var (
	channelX = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	channelY = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// A series timer records the programmes of its title, on its channel at
// its time of day, or on any channel, or at any time, as Jellyfin's do.
func TestSeriesTimersMatchProgrammesByTitle(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	x, y := id(t, channelX), id(t, channelY)
	st := SeriesTimer{Programme: Programme{Channel: x, Name: "The Show", Start: time.Date(2026, 10, 4, 20, 0, 0, 0, time.UTC)}}
	cases := []struct {
		name        string
		program     library.Item
		strict, any bool
	}{
		{"same title, channel and time", program(t, "01000000000000000000000000000000", " the show ", x, now.Add(32*time.Hour+5*time.Minute)), true, true},
		{"another channel", program(t, "02000000000000000000000000000000", "The Show", y, now.Add(8*time.Hour)), false, true},
		{"another time", program(t, "03000000000000000000000000000000", "The Show", x, now.Add(3*time.Hour)), false, true},
		{"another title", program(t, "04000000000000000000000000000000", "Another Show", x, now.Add(8*time.Hour)), false, false},
		{"ended", program(t, "05000000000000000000000000000000", "The Show", x, now.Add(-16*time.Hour)), false, false},
	}
	for _, tc := range cases {
		if got := Matches(st, tc.program, now); got != tc.strict {
			t.Errorf("%s: matched %v", tc.name, got)
		}
		loose := st
		loose.RecordAnyChannel, loose.RecordAnyTime = true, true
		if got := Matches(loose, tc.program, now); got != tc.any {
			t.Errorf("%s, any channel and time: matched %v", tc.name, got)
		}
	}
	// Ten minutes from the time of day is too far.
	if Matches(st, program(t, "06000000000000000000000000000000", "The Show", x, now.Add(8*time.Hour+10*time.Minute)), now) {
		t.Error("a programme ten minutes late matched")
	}
}

// Series timers make timers from the guide of the user who made them, and
// keep those cancelled by hand cancelled.
func TestSeriesTimersMakeTimersFromTheGuide(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Minute)
	x, y := id(t, channelX), id(t, channelY)
	first := program(t, "01000000000000000000000000000000", "The Show", x, now.Add(time.Hour))
	f.guide.programs = []library.Item{first,
		program(t, "02000000000000000000000000000000", "The Show", x, now.Add(25*time.Hour)),
		program(t, "03000000000000000000000000000000", "The Show", y, now.Add(49*time.Hour)),
		program(t, "04000000000000000000000000000000", "Another", x, now.Add(2*time.Hour)),
	}
	options := Options{KeepUntil: KeepUntil[0], PrePadding: 30}
	st, err := f.service.CreateSeriesTimer(ctx, f.user.ID, ProgrammeOf(first), options, SeriesOptions{RecordAnyTime: true})
	if err != nil {
		t.Fatal(err)
	}
	programs := func() []string {
		timers, err := f.service.Timers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var result []string
		for _, timer := range timers {
			if timer.SeriesTimer == nil || *timer.SeriesTimer != st.ID || timer.PrePadding != 30 || timer.Manual != (timer.Status == StatusCancelled) {
				t.Errorf("a series timer's timer: %+v", timer)
			}
			result = append(result, timer.Program.String()[:2]+string(timer.Status[0]))
		}
		return result
	}
	if got := programs(); !slices.Equal(got, []string{"01N", "02N"}) {
		t.Errorf("timers on the series' channel: %v", got)
	}
	if err := f.service.UpdateSeriesTimer(ctx, st.ID, options, SeriesOptions{RecordAnyTime: true, RecordAnyChannel: true}); err != nil {
		t.Fatal(err)
	}
	if got := programs(); !slices.Equal(got, []string{"01N", "02N", "03N"}) {
		t.Errorf("timers on any channel: %v", got)
	}
	timers, _ := f.service.Timers(ctx)
	if err := f.service.CancelTimer(ctx, timers[1].ID); err != nil {
		t.Fatal(err)
	}
	f.service.RefreshSeriesTimers(ctx)
	timers, _ = f.service.Timers(ctx)
	if len(timers) != 3 || timers[1].Status != StatusCancelled {
		t.Errorf("a timer of the series cancelled by hand, after a refresh: %+v", timers)
	}
	// No longer recording any channel, the timer it made there goes.
	if err := f.service.UpdateSeriesTimer(ctx, st.ID, options, SeriesOptions{RecordAnyTime: true}); err != nil {
		t.Fatal(err)
	}
	if got := len(programs()); got != 2 {
		t.Errorf("timers once on its channel again: %d", got)
	}
}

// A recording a restart interrupted is kept, partial, and its timer
// records the rest of the programme; one that wrote nothing is dropped.
func TestRestartKeepsInterruptedRecordingsAsPartial(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	now := time.Now()
	airing := program(t, "01000000000000000000000000000000", "Airing", id(t, channelX), now.Add(-30*time.Minute))
	ended := program(t, "02000000000000000000000000000000", "Ended", id(t, channelX), now.Add(-3*time.Hour))
	var recorded []Recording
	for _, p := range []library.Item{airing, ended} {
		timer, err := f.service.CreateTimer(ctx, f.user.ID, ProgrammeOf(p), Options{KeepUntil: KeepUntil[0]})
		if err != nil {
			t.Fatal(err)
		}
		var r Recording
		if err := f.service.db.QueryRow(ctx, `INSERT INTO live_recordings (timer_id, user_id, channel_id, program_id, name, start_at, end_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, timer.ID, f.user.ID, timer.Channel, timer.Program, timer.Name, timer.Start, timer.End).
			Scan(&r.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.db.Exec(ctx, "UPDATE live_timers SET status = 'InProgress' WHERE id = $1", timer.ID); err != nil {
			t.Fatal(err)
		}
		recorded = append(recorded, r)
	}
	// The airing programme's recording wrote two parts before the restart.
	for n, data := range []string{"first ", "second"} {
		if err := os.WriteFile(partPath(f.dir, recorded[0].ID, n), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.service.Recover(ctx)

	kept, err := f.service.Recording(ctx, recorded[0].ID)
	if err != nil || kept.InProgress() || !kept.Partial || kept.Size != int64(len("first second")) {
		t.Fatalf("an interrupted recording: %+v %v", kept, err)
	}
	if data, err := os.ReadFile(f.service.Path(kept)); err != nil || string(data) != "first second" {
		t.Errorf("its file: %q %v", data, err)
	}
	if files, _ := filepath.Glob(filepath.Join(f.dir, "*.ts")); len(files) != 0 {
		t.Errorf("parts left: %v", files)
	}
	if _, err := f.service.Recording(ctx, recorded[1].ID); err != ErrNotFound {
		t.Errorf("a recording that wrote nothing: %v", err)
	}
	timers, err := f.service.Timers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]Status{}
	for _, timer := range timers {
		statuses[timer.Name] = timer.Status
	}
	// The programme still airing is recorded again; the one that ended is
	// done.
	if len(statuses) != 1 || statuses["Airing"] != StatusNew {
		t.Errorf("timers after the restart: %v", statuses)
	}
}

// Recordings older than the settings keep them go, with their files; none
// go while the settings keep them forever.
func TestOldRecordingsAreSwept(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	now := time.Now()
	var ids []accounts.ID
	for _, age := range []int{10, 2} {
		var recording accounts.ID
		file := "rec" + string(rune('0'+age)) + ".mkv"
		if err := f.service.db.QueryRow(ctx, `INSERT INTO live_recordings (channel_id, name, start_at, end_at, started_at, ended_at, status, file)
			VALUES ($1, 'Old', $2, $2, $2, $2, 'Completed', $3) RETURNING id`, id(t, channelX), now.AddDate(0, 0, -age), file).Scan(&recording); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.dir, file), []byte("video"), 0o600); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, recording)
	}
	if err := f.service.SweepRecordings(ctx); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.service.Recordings(ctx); len(list) != 2 {
		t.Fatalf("recordings kept forever were swept: %+v", list)
	}
	settings := f.store.Settings()
	settings.RecordingRetentionDays = 7
	if _, err := f.store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := f.service.SweepRecordings(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ := f.service.Recordings(ctx)
	if len(list) != 1 || list[0].ID != ids[1] {
		t.Errorf("recordings after a sweep: %+v", list)
	}
	if files, _ := filepath.Glob(filepath.Join(f.dir, "*")); len(files) != 1 || filepath.Base(files[0]) != "rec2.mkv" {
		t.Errorf("files after a sweep: %v", files)
	}
}

// Recording follows the folder the settings give: off, the service is not
// available, and a timer due does not record; another folder is used at
// once.
func TestRecordingFollowsTheSettingsFolder(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	airing := program(t, "01000000000000000000000000000000", "Airing", id(t, channelX), time.Now().Add(-10*time.Minute))
	timer, err := f.service.CreateTimer(ctx, f.user.ID, ProgrammeOf(airing), Options{KeepUntil: KeepUntil[0]})
	if err != nil {
		t.Fatal(err)
	}
	off := ""
	f.folder.Store(&off)
	if f.service.Available() || f.service.Dir() != "" || f.service.Path(Recording{File: "a.mkv"}) != "" {
		t.Errorf("recording is on without a folder: %q", f.service.Dir())
	}
	later := program(t, "02000000000000000000000000000000", "Later", id(t, channelX), time.Now().Add(time.Hour))
	if _, err := f.service.CreateTimer(ctx, f.user.ID, ProgrammeOf(later), Options{KeepUntil: KeepUntil[0]}); err != ErrUnavailable {
		t.Errorf("a timer made while recording is off: %v", err)
	}
	f.service.startDue(ctx)
	f.service.mu.Lock()
	started := len(f.service.active)
	f.service.mu.Unlock()
	if started != 0 || !f.service.skipped[timer.ID] {
		t.Errorf("a timer due while recording is off: %d started, skipped %v", started, f.service.skipped)
	}
	other := t.TempDir()
	f.folder.Store(&other)
	if !f.service.Available() || f.service.Dir() != other || f.service.Path(Recording{File: "a.mkv"}) != filepath.Join(other, "a.mkv") {
		t.Errorf("another folder: %q", f.service.Dir())
	}
}

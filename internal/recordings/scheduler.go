package recordings

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
)

const (
	// retryDelay is how long a recording whose stream stopped before its
	// end waits before reading it again, as Jellyfin retries.
	retryDelay = 30 * time.Second
	// SeriesRefreshInterval is how often series timers should look for new
	// programmes in the guide (see RefreshSeriesTimers).
	SeriesRefreshInterval = 30 * time.Minute
	// SweepInterval is how often old recordings should be deleted (see
	// SweepRecordings).
	SweepInterval = 24 * time.Hour
	// maxWait bounds how long the scheduler sleeps between looks at its
	// timers.
	maxWait = time.Minute
	// timeOfDayTolerance is how far from a series timer's time of day a
	// programme may start and still be recorded, Jellyfin's.
	timeOfDayTolerance = 10 * time.Minute
	// guideReach is how far ahead series timers look in the guide.
	guideReach = 8 * 24 * time.Hour
	// lateStop is how long before its planned end a recording may stop
	// without being partial.
	lateStop = 15 * time.Second
	// programKind and recordingKind are how parental control names
	// programmes and recordings (accounts.UnratedKinds): Jellyfin's
	// recordings are videos, which it judges as Other.
	programKind   = "LiveTvProgram"
	recordingKind = "Other"
)

// ProgramKind and RecordingKind name programmes and recordings for
// parental control (see Allows).
const (
	ProgramKind   = programKind
	RecordingKind = recordingKind
)

// Guide finds programmes and channels' streams; library.Service is one.
type Guide interface {
	Programs(ctx context.Context, user accounts.User, from, to time.Time) ([]library.Item, error)
	Versions(ctx context.Context, user accounts.User, id accounts.ID) ([]library.Version, error)
}

// Recorder writes streams into files; playback.Service is one.
type Recorder interface {
	Record(ctx context.Context, user accounts.ID, version library.Version, session, path string) error
	FinishRecording(ctx context.Context, parts []string, path string) error
	Failed(version accounts.ID) bool
	ForgetFile(id accounts.ID)
}

// Users are the accounts and settings; accounts.Store is one.
type Users interface {
	User(ctx context.Context, id accounts.ID) (accounts.User, error)
	Settings() accounts.Settings
}

// Config are the dependencies of the recordings service.
type Config struct {
	DB *pgxpool.Pool
	// Folder gives the recordings folder while recording is on, empty
	// while it is off, read whenever it is used.
	Folder   func() string
	Guide    Guide
	Recorder Recorder
	Users    Users
	Logger   *slog.Logger
	// RetryDelay replaces retryDelay when set.
	RetryDelay time.Duration
}

// Service keeps timers, series timers and recordings, and records.
type Service struct {
	db       *pgxpool.Pool
	folder   func() string
	guide    Guide
	recorder Recorder
	users    Users
	logger   *slog.Logger
	retry    time.Duration
	now      func() time.Time

	wake chan struct{}
	wg   sync.WaitGroup
	mu   sync.Mutex
	// active are the recordings under way, by timer.
	active map[accounts.ID]*active
	// skipped are the timers due while recording is off, logged once.
	skipped map[accounts.ID]bool
	// recovered tells whether the recordings a shutdown interrupted were
	// finished, which waits for recording to be on.
	recovered bool
}

// active is a recording under way.
type active struct {
	recording accounts.ID
	cancel    context.CancelCauseFunc
	done      chan struct{}
	// dir is the folder the recording started in, which keeps its parts
	// and its file until it is finished.
	dir string
	// part is the file written now.
	part string
}

// errStopped stops a recording by hand: it is kept, unlike one stopped
// by the server shutting down, which is finished when it starts again.
var errStopped = errors.New("the recording was stopped")

// New returns the recordings service.
func New(c Config) *Service {
	s := &Service{db: c.DB, folder: c.Folder, guide: c.Guide, recorder: c.Recorder, users: c.Users, logger: c.Logger,
		retry: retryDelay, now: time.Now, wake: make(chan struct{}, 1), active: map[accounts.ID]*active{}, skipped: map[accounts.ID]bool{}}
	if s.folder == nil {
		s.folder = func() string { return "" }
	}
	if c.RetryDelay > 0 {
		s.retry = c.RetryDelay
	}
	return s
}

// Available reports whether the server records: recording is on, with a
// folder.
func (s *Service) Available() bool {
	return s.Dir() != ""
}

// Dir is the recordings folder, empty while recording is off.
func (s *Service) Dir() string {
	if s == nil {
		return ""
	}
	return s.folder()
}

// Path is the file of a finished recording, in the current recordings
// folder; empty while recording is off.
func (s *Service) Path(r Recording) string {
	dir := s.Dir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, r.File)
}

// Wake has the scheduler look at its timers now.
func (s *Service) Wake() {
	if s == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run records until ctx ends: it first finishes the recordings a restart
// interrupted, once recording is on, then starts each timer at its time.
// Recordings under way when ctx ends are left to be finished, as partial,
// when it runs again. Series timers and old recordings are seen to by
// RefreshSeriesTimers and SweepRecordings, which the server's tasks run.
func (s *Service) Run(ctx context.Context) {
	for {
		if !s.recovered && s.Available() {
			s.Recover(ctx)
			s.recovered = true
		}
		wait := s.startDue(ctx)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.wg.Wait()
			return
		case <-s.wake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// startDue starts the timers whose time came, deletes those whose time
// passed, and returns how long until the next one starts.
func (s *Service) startDue(ctx context.Context) time.Duration {
	now := s.now()
	// Like Jellyfin, a timer whose programme ended goes once it does not
	// record.
	if _, err := s.db.Exec(ctx, `DELETE FROM live_timers
		WHERE end_at + post_padding * interval '1 second' < $1 AND status <> 'InProgress'`, now); err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Past timers could not be deleted", "error", err)
		}
		return maxWait
	}
	rows, err := s.db.Query(ctx, "SELECT "+timerColumns+" FROM live_timers WHERE status = 'New' ORDER BY start_at")
	var timers []Timer
	if err == nil {
		timers, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Timer, error) { return scanTimer(row) })
	}
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Timers could not be read", "error", err)
		}
		return maxWait
	}
	wait := maxWait
	dir := s.Dir()
	skipped := map[accounts.ID]bool{}
	for _, t := range timers {
		switch from := t.From(); {
		case !from.After(now) && now.Before(t.Until()) && dir == "":
			// While recording is off, a timer due does not record, said
			// once; its programme ending deletes it.
			skipped[t.ID] = true
			if !s.skipped[t.ID] {
				s.logger.Info("A timer is due while recording is off: it does not record", "timer", t.ID.String(), "name", t.Name)
			}
		case !from.After(now) && now.Before(t.Until()):
			s.start(ctx, t, dir)
		case from.After(now):
			wait = min(wait, from.Sub(now))
		}
	}
	s.skipped = skipped
	return wait
}

// start records t into dir, unless it records already.
func (s *Service) start(ctx context.Context, t Timer, dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, running := s.active[t.ID]; running {
		return
	}
	recordCtx, cancel := context.WithCancelCause(ctx)
	a := &active{cancel: cancel, done: make(chan struct{}), dir: dir}
	s.active[t.ID] = a
	s.wg.Go(func() {
		defer close(a.done)
		defer func() {
			s.mu.Lock()
			delete(s.active, t.ID)
			s.mu.Unlock()
			cancel(nil)
		}()
		s.record(recordCtx, t, a)
	})
}

// stop stops the recording of a timer, which is kept.
func (s *Service) stop(timer accounts.ID) {
	s.mu.Lock()
	a := s.active[timer]
	s.mu.Unlock()
	if a != nil {
		a.cancel(errStopped)
	}
}

// stopAndWait stops the recording of a timer and waits for it to finish.
func (s *Service) stopAndWait(timer accounts.ID) {
	s.mu.Lock()
	a := s.active[timer]
	s.mu.Unlock()
	if a != nil {
		a.cancel(errStopped)
		<-a.done
	}
}

// Active returns the file a recording writes now, and a channel closed once
// it stops; id names the recording or its timer. ok is false when it is
// not under way.
func (s *Service) Active(id accounts.ID) (part string, done <-chan struct{}, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for timer, a := range s.active {
		if (timer == id || a.recording == id) && a.part != "" {
			return a.part, a.done, true
		}
	}
	return "", nil, false
}

// partPath is the nth part of a recording written into dir.
func partPath(dir string, recording accounts.ID, n int) string {
	return filepath.Join(dir, recording.String()+".part"+strconv.Itoa(n)+".ts")
}

// record writes t's recording until its end: the stream is read again
// after a while when it stops before.
func (s *Service) record(ctx context.Context, t Timer, a *active) {
	var id accounts.ID
	err := s.db.QueryRow(ctx, `INSERT INTO live_recordings (timer_id, series_timer_id, user_id, channel_id, program_id, name, overview,
			genres, rating, image, start_at, end_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		t.ID, t.SeriesTimer, t.User, t.Channel, t.Program, t.Name, t.Overview, nonNil(t.Genres), t.Rating, t.Image, t.Start, t.End).Scan(&id)
	if err == nil {
		_, err = s.db.Exec(ctx, "UPDATE live_timers SET status = 'InProgress' WHERE id = $1", t.ID)
	}
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("A recording could not start", "error", err)
		}
		return
	}
	s.mu.Lock()
	a.recording = id
	s.mu.Unlock()
	s.logger.Info("A recording started", "recording", id.String(), "until", t.Until().Format(time.RFC3339))
	until, cancel := context.WithDeadline(ctx, t.Until())
	defer cancel()
	interrupted := false
	for n := 0; until.Err() == nil; {
		path := partPath(a.dir, id, n)
		s.mu.Lock()
		a.part = path
		s.mu.Unlock()
		err := s.recordPart(until, t, id, path)
		if until.Err() != nil {
			break
		}
		if size(path) > 0 {
			n++
		}
		interrupted = true
		s.logger.Info("A recording's stream stopped before its end; it is read again shortly", "recording", id.String(), "error", err)
		select {
		case <-until.Done():
		case <-time.After(s.retry):
		}
	}
	// A shutdown leaves the recording to be finished when Polyfin starts
	// again; a recording stopped by hand, or at its end, is finished now.
	if ctx.Err() != nil && !errors.Is(context.Cause(ctx), errStopped) {
		return
	}
	stopped := errors.Is(context.Cause(ctx), errStopped) && s.now().Before(t.Until().Add(-lateStop))
	s.finish(context.WithoutCancel(ctx), a.dir, id, t.Until(), interrupted || stopped)
}

// recordPart records t's channel into path with the first of its streams
// that plays, as live playback picks one.
func (s *Service) recordPart(ctx context.Context, t Timer, recording accounts.ID, path string) error {
	user, err := s.users.User(ctx, t.User)
	if err != nil {
		return err
	}
	if user.IsDisabled || !user.LiveTv {
		return errors.New("the user who made the timer may not watch Live TV")
	}
	versions, err := s.guide.Versions(ctx, user, t.Channel)
	if err != nil {
		return err
	}
	tries := s.users.Settings().VersionAttempts
	err = errors.New("the channel has no stream")
	for _, version := range versions {
		if s.recorder.Failed(version.ID) {
			continue
		}
		if tries--; tries < 0 {
			break
		}
		err = s.recorder.Record(ctx, t.User, version, recording.String(), path)
		// A stream that recorded something, or a busy server, is not a
		// reason to try the next.
		if ctx.Err() != nil || size(path) > 0 || errors.Is(err, hls.ErrBusy) {
			return err
		}
		s.logger.Info("A channel's stream could not be recorded", "addon", version.Addon, "error", err)
	}
	return err
}

// finish joins the parts a recording wrote into dir into its file there,
// marks it complete, and its timer with it. A recording that wrote nothing
// is deleted, and its timer marked failed.
func (s *Service) finish(ctx context.Context, dir string, id accounts.ID, until time.Time, partial bool) {
	parts := parts(dir, id)
	if len(parts) == 0 {
		s.logger.Warn("A recording wrote nothing", "recording", id.String())
		var timer *accounts.ID
		if err := s.db.QueryRow(ctx, "DELETE FROM live_recordings WHERE id = $1 RETURNING timer_id", id).Scan(&timer); err != nil {
			s.logger.Warn("An empty recording could not be deleted", "error", err)
		}
		if timer != nil {
			if _, err := s.db.Exec(ctx, "UPDATE live_timers SET status = 'Error' WHERE id = $1 AND status = 'InProgress'", *timer); err != nil {
				s.logger.Warn("A failed timer could not be marked", "error", err)
			}
		}
		return
	}
	if info, err := os.Stat(parts[len(parts)-1]); err == nil && info.ModTime().Before(until.Add(-lateStop)) {
		partial = true
	}
	file := id.String() + ".mkv"
	if err := s.recorder.FinishRecording(ctx, parts, filepath.Join(dir, file)); err != nil {
		// The parts stay readable as they are: the first is kept.
		s.logger.Warn("A recording could not be made into one file; its first part is kept", "recording", id.String(), "error", err)
		file = id.String() + ".ts"
		if err := os.Rename(parts[0], filepath.Join(dir, file)); err != nil {
			s.logger.Warn("A recording's part could not be kept", "error", err)
		}
		partial = partial || len(parts) > 1
	}
	var timer, series *accounts.ID
	err := s.db.QueryRow(ctx, `UPDATE live_recordings SET status = 'Completed', ended_at = $2, partial = $3, file = $4, size = $5
		WHERE id = $1 RETURNING timer_id, series_timer_id`, id, s.now(), partial, file, size(filepath.Join(dir, file))).Scan(&timer, &series)
	for _, part := range parts {
		_ = os.Remove(part)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Deleted while it was finished.
		_ = os.Remove(filepath.Join(dir, file))
		return
	}
	if err != nil {
		s.logger.Warn("A recording could not be marked complete", "error", err)
		return
	}
	s.logger.Info("A recording finished", "recording", id.String(), "partial", partial)
	if timer != nil {
		if _, err := s.db.Exec(ctx, "UPDATE live_timers SET status = 'Completed' WHERE id = $1 AND status = 'InProgress'", *timer); err != nil {
			s.logger.Warn("A timer could not be marked complete", "error", err)
		}
	}
	if series != nil {
		s.keepUpTo(ctx, *series)
	}
}

// parts lists the parts of a recording in dir that hold something, in
// order.
func parts(dir string, id accounts.ID) []string {
	var parts []string
	for n := 0; ; n++ {
		path := partPath(dir, id, n)
		if _, err := os.Stat(path); err != nil {
			return parts
		}
		if size(path) > 0 {
			parts = append(parts, path)
		} else {
			_ = os.Remove(path)
		}
	}
}

// keepUpTo deletes the oldest recordings of a series timer past its
// KeepUpTo, 0 keeping them all, as Jellyfin does.
func (s *Service) keepUpTo(ctx context.Context, series accounts.ID) {
	rows, err := s.db.Query(ctx, `SELECT r.id FROM live_recordings r JOIN live_series_timers st ON st.id = r.series_timer_id
		WHERE r.series_timer_id = $1 AND st.keep_up_to > 0 AND r.status = 'Completed'
		ORDER BY r.started_at DESC OFFSET (SELECT keep_up_to FROM live_series_timers WHERE id = $1)`, series)
	var old []accounts.ID
	if err == nil {
		old, err = pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	}
	if err != nil {
		s.logger.Warn("A series timer's old recordings could not be listed", "error", err)
		return
	}
	for _, id := range old {
		if err := s.delete(ctx, id); err != nil && !errors.Is(err, ErrNotFound) {
			s.logger.Warn("An old recording could not be deleted", "error", err)
		}
	}
}

// Recover finishes, as partial, the recordings a shutdown interrupted, from
// their parts in the current recordings folder, and has their timers
// record the rest of their programme while it airs.
func (s *Service) Recover(ctx context.Context) {
	dir := s.Dir()
	rows, err := s.db.Query(ctx, "SELECT "+recordingColumns+" FROM live_recordings WHERE status = 'InProgress'")
	var interrupted []Recording
	if err == nil {
		interrupted, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Recording, error) { return scanRecording(row) })
	}
	if err != nil {
		s.logger.Warn("Interrupted recordings could not be listed", "error", err)
		return
	}
	// The timers first: once finished, their recordings no longer tell
	// them to complete.
	if _, err := s.db.Exec(ctx, `UPDATE live_timers SET status = CASE WHEN end_at + post_padding * interval '1 second' > $1
		THEN 'New' ELSE 'Completed' END WHERE status = 'InProgress'`, s.now()); err != nil {
		s.logger.Warn("Interrupted timers could not be scheduled again", "error", err)
	}
	for _, r := range interrupted {
		s.finish(ctx, dir, r.ID, r.End, true)
	}
}

// SweepRecordings deletes the recordings older than the settings keep
// them, if they limit it.
func (s *Service) SweepRecordings(ctx context.Context) error {
	if !s.Available() {
		return nil
	}
	days := s.users.Settings().RecordingRetentionDays
	if days <= 0 {
		return nil
	}
	rows, err := s.db.Query(ctx, "SELECT id FROM live_recordings WHERE status = 'Completed' AND ended_at < $1",
		s.now().AddDate(0, 0, -days))
	if err != nil {
		return err
	}
	old, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return err
	}
	for _, id := range old {
		if err := s.delete(ctx, id); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	if len(old) > 0 {
		s.logger.Info("Old recordings were deleted", "count", len(old))
	}
	return nil
}

// RefreshSeriesTimers has every series timer make timers for the new
// programmes of the guide.
func (s *Service) RefreshSeriesTimers(ctx context.Context) error {
	if !s.Available() {
		return nil
	}
	series, err := s.SeriesTimers(ctx)
	if err != nil {
		return err
	}
	for _, st := range series {
		s.refreshSeries(ctx, st)
	}
	return ctx.Err()
}

// refreshSeries makes timers for the upcoming programmes st records, read
// from the guide of the user who made it, and deletes those it made for
// programmes it no longer records.
func (s *Service) refreshSeries(ctx context.Context, st SeriesTimer) {
	user, err := s.users.User(ctx, st.User)
	if err != nil || user.IsDisabled || !user.LiveTv {
		return
	}
	now := s.now()
	programs, err := s.guide.Programs(ctx, user, now, now.Add(guideReach))
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The guide could not be read for a series timer", "error", err)
		}
		return
	}
	matched := []accounts.ID{}
	for _, item := range programs {
		if !Matches(st, item, now) || !Allows(user, programKind, item.OfficialRating, item.Genres) {
			continue
		}
		p := ProgrammeOf(item)
		_, err := s.db.Exec(ctx, `INSERT INTO live_timers (user_id, series_timer_id, channel_id, program_id, name, overview, genres, rating,
				image, start_at, end_at, pre_padding, post_padding, keep_until, priority, is_manual)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, false)
			ON CONFLICT (program_id) DO UPDATE SET series_timer_id = excluded.series_timer_id
				WHERE live_timers.series_timer_id IS NULL`,
			st.User, st.ID, p.Channel, p.Program, p.Name, p.Overview, nonNil(p.Genres), p.Rating, p.Image, p.Start, p.End,
			st.PrePadding, st.PostPadding, st.KeepUntil, st.Priority)
		if err != nil {
			s.logger.Warn("A series timer's timer could not be made", "error", err)
			return
		}
		matched = append(matched, *p.Program)
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM live_timers WHERE series_timer_id = $1 AND NOT is_manual AND status = 'New'
		AND start_at > $2 AND NOT (program_id = ANY($3))`, st.ID, now, matched); err != nil {
		s.logger.Warn("A series timer's old timers could not be deleted", "error", err)
	}
	s.Wake()
}

// Matches reports whether a series timer records a programme of the
// guide, as Jellyfin decides: a programme of the same title, still to end,
// on the series timer's channel unless RecordAnyChannel, starting within
// ten minutes of its time of day unless RecordAnyTime. RecordNewOnly
// leaves out repeats, which the guide does not mark: every programme
// counts as new. Like Jellyfin, the days of the week are not looked at.
func Matches(st SeriesTimer, program library.Item, now time.Time) bool {
	if program.Kind != library.KindProgram || program.StartDate == nil || program.EndDate == nil || program.Channel == nil ||
		!program.EndDate.After(now) || !strings.EqualFold(strings.TrimSpace(program.Name), strings.TrimSpace(st.Name)) {
		return false
	}
	if !st.RecordAnyChannel && program.Channel.ID != st.Channel {
		return false
	}
	if !st.RecordAnyTime {
		a, b := timeOfDay(*program.StartDate), timeOfDay(st.Start)
		if (a - b).Abs() >= timeOfDayTolerance {
			return false
		}
	}
	return true
}

func timeOfDay(t time.Time) time.Duration {
	t = t.UTC()
	return t.Sub(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))
}

// removeFiles deletes a recording's file and parts from the current
// recordings folder; while recording is off, they are left there.
func (s *Service) removeFiles(r Recording) {
	dir := s.Dir()
	if dir == "" {
		return
	}
	if r.File != "" {
		if err := os.Remove(filepath.Join(dir, r.File)); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("A recording's file could not be deleted", "error", err)
		}
	}
	for n := 0; ; n++ {
		if err := os.Remove(partPath(dir, r.ID, n)); err != nil {
			return
		}
	}
}

func size(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

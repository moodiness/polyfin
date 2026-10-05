package trackers

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Titles finds the items of titles named by other services' identifiers
// (see library.Service.Resolve).
type Titles interface {
	Resolve(ctx context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error)
}

var (
	// ErrNotConnected reports a service the user did not connect.
	ErrNotConnected = errors.New("tracking service not connected")
	// ErrImportOff reports an import asked of a service whose history is
	// not imported.
	ErrImportOff = errors.New("watch history import is off")
)

// ProblemRateLimited is a last import the service kept asking to wait
// for longer than an import waits.
const ProblemRateLimited = "rate_limited"

// importBatch is how many items an import writes at a time.
const importBatch = 500

// ImportResult is how an import of a watch history went.
type ImportResult struct {
	// At is when it ended.
	At time.Time
	// Played counts the titles it marked played, Resumed the resume
	// points it set, and Unmapped the titles of the history Polyfin could
	// not identify.
	Played, Resumed, Unmapped int
	// Problem is empty, ProblemReconnect, ProblemUnreachable or
	// ProblemRateLimited: the history could not be read whole, and what
	// was read was imported.
	Problem string
}

// importRun is an import running.
type importRun struct {
	cancel context.CancelFunc
}

// importStatus completes status with the import of its service.
func (s *Service) importStatus(ctx context.Context, key laneKey, status *Status) error {
	var last *time.Time
	var result ImportResult
	var problem *string
	err := s.db.QueryRow(ctx, `SELECT enabled, last_import_at, played, resumed, unmapped, problem FROM tracking_imports
		WHERE user_id = $1 AND service = $2`, key.user, key.service).Scan(&status.ImportHistory, &last, &result.Played, &result.Resumed, &result.Unmapped, &problem)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if last != nil {
		result.At = *last
		if problem != nil {
			result.Problem = *problem
		}
		status.LastImport = &result
	}
	s.mu.Lock()
	_, status.Importing = s.importing[key]
	s.mu.Unlock()
	return nil
}

// SetImport turns the import of user's watch history from service on or
// off. Turned on, it imports at once, then every 6 hours.
func (s *Service) SetImport(ctx context.Context, user accounts.ID, service string, on bool) (Status, error) {
	if !known(service) {
		return Status{}, ErrUnknownService
	}
	key := laneKey{user, service}
	if _, ok, err := s.connection(ctx, user, service); err != nil {
		return Status{}, err
	} else if !ok {
		return Status{}, ErrNotConnected
	}
	var was bool
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, "SELECT enabled FROM tracking_imports WHERE user_id = $1 AND service = $2 FOR UPDATE", user, service).Scan(&was)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO tracking_imports (user_id, service, enabled) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, service) DO UPDATE SET enabled = excluded.enabled`, user, service, on)
		return err
	})
	if err != nil {
		return Status{}, err
	}
	switch {
	case on && !was:
		s.startImport(key)
	case !on:
		s.stopImport(key)
	}
	return s.status(ctx, user, service)
}

// ImportNow imports user's watch history from service at once, unless an
// import of it runs already.
func (s *Service) ImportNow(ctx context.Context, user accounts.ID, service string) (Status, error) {
	if !known(service) {
		return Status{}, ErrUnknownService
	}
	if _, ok, err := s.connection(ctx, user, service); err != nil {
		return Status{}, err
	} else if !ok {
		return Status{}, ErrNotConnected
	}
	var on bool
	err := s.db.QueryRow(ctx, "SELECT enabled FROM tracking_imports WHERE user_id = $1 AND service = $2", user, service).Scan(&on)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Status{}, err
	}
	if !on {
		return Status{}, ErrImportOff
	}
	s.startImport(laneKey{user, service})
	return s.status(ctx, user, service)
}

// startImport imports the history of key in the background, unless an
// import of it runs already.
func (s *Service) startImport(key laneKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.titles == nil || s.userData == nil || s.importing[key] != nil {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	run := &importRun{cancel: cancel}
	s.importing[key] = run
	if !s.spawn(func() {
		defer func() {
			s.mu.Lock()
			if s.importing[key] == run {
				delete(s.importing, key)
			}
			s.mu.Unlock()
			cancel()
		}()
		s.runImport(ctx, key)
	}) {
		delete(s.importing, key)
		cancel()
	}
}

// stopImport stops the import of key running, if any.
func (s *Service) stopImport(key laneKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.importing[key]; run != nil {
		run.cancel()
		delete(s.importing, key)
	}
}

// forgetImport stops the imports of key, as its service is disconnected:
// the next connection starts with them off.
func (s *Service) forgetImport(ctx context.Context, key laneKey) error {
	s.stopImport(key)
	_, err := s.db.Exec(ctx, "DELETE FROM tracking_imports WHERE user_id = $1 AND service = $2", key.user, key.service)
	return err
}

// importDue starts the imports due: turned on, of a connection that works,
// and not run for importEvery.
func (s *Service) importDue() {
	if s.titles == nil || s.userData == nil {
		return
	}
	rows, err := s.db.Query(s.ctx, `SELECT i.user_id, i.service FROM tracking_imports i
		JOIN tracking_connections c ON c.user_id = i.user_id AND c.service = i.service
		WHERE i.enabled AND c.problem IS DISTINCT FROM 'reconnect' AND (i.last_import_at IS NULL OR i.last_import_at <= $1)`,
		s.now().Add(-s.timing.importEvery))
	if err != nil {
		s.logger.Warn("The watch history imports due could not be read", "error", err)
		return
	}
	due, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (laneKey, error) {
		var key laneKey
		return key, row.Scan(&key.user, &key.service)
	})
	if err != nil {
		s.logger.Warn("The watch history imports due could not be read", "error", err)
		return
	}
	settings := s.settings()
	for _, key := range due {
		if Available(key.service, settings) {
			s.startImport(key)
		}
	}
}

// watchedEntry is a title a history says the user watched, at a date when
// it gives one.
type watchedEntry struct {
	ref library.TitleRef
	at  *time.Time
}

// resumeEntry is a resume point a service keeps: at Percent of the title,
// or at Position of Runtime when the service tells them, set at At.
type resumeEntry struct {
	ref               library.TitleRef
	percent           float64
	position, runtime time.Duration
	at                time.Time
}

// history is what an import read of a service.
type watchHistory struct {
	watched []watchedEntry
	resumes []resumeEntry
}

// runImport imports the watch history of key: reads it from the service,
// finds its titles, and adds what Polyfin lacks to the user's data.
func (s *Service) runImport(ctx context.Context, key laneKey) {
	var cursor string
	var cursorFor *time.Time
	err := s.db.QueryRow(ctx, "SELECT cursor, cursor_for FROM tracking_imports WHERE user_id = $1 AND service = $2 AND enabled",
		key.user, key.service).Scan(&cursor, &cursorFor)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Warn("A watch history import could not start", "service", key.service, "error", err)
		}
		return
	}
	conn, ok, err := s.connection(ctx, key.user, key.service)
	if err != nil || !ok || conn.problem == ProblemReconnect || !Available(key.service, s.settings()) {
		return
	}
	// A cursor tells where the history of the account it was read from
	// stood: a new connection reads it whole.
	if cursorFor == nil || !cursorFor.Equal(conn.connectedAt) {
		cursor = ""
	}
	r := &reader{s: s, key: key, token: conn.token, settings: s.settings()}
	read, next, readErr := r.read(ctx, cursor)
	if ctx.Err() != nil {
		// Turned off, disconnected, or the server stops.
		return
	}
	result := ImportResult{Problem: importProblem(readErr)}
	if readErr != nil {
		next = cursor
		s.logger.Info("A watch history could not be read whole: what was read is imported", "user_id", key.user.String(),
			"service", key.service, "problem", result.Problem)
	}
	if err := s.merge(ctx, r, read, &result); err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("A watch history could not be imported", "service", key.service, "error", err)
		}
		return
	}
	result.At = s.now().UTC()
	var problem *string
	if result.Problem != "" {
		problem = &result.Problem
	}
	if _, err := s.db.Exec(ctx, `UPDATE tracking_imports SET last_import_at = $3, played = $4, resumed = $5, unmapped = $6, problem = $7,
			cursor = $8, cursor_for = $9
		WHERE user_id = $1 AND service = $2`, key.user, key.service, result.At, result.Played, result.Resumed, result.Unmapped, problem,
		next, conn.connectedAt); err != nil && ctx.Err() == nil {
		s.logger.Warn("A watch history import could not be recorded", "service", key.service, "error", err)
	}
	s.logger.Debug("Imported a watch history", "user_id", key.user.String(), "service", key.service, "played", result.Played,
		"resumed", result.Resumed, "unmapped", result.Unmapped)
}

// importProblem names what stopped reading a history.
func importProblem(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errTokenRefused):
		return ProblemReconnect
	case errors.Is(err, errRateLimited):
		return ProblemRateLimited
	}
	return ProblemUnreachable
}

// played is an item an import marks played, at the latest date the history
// gives, nil when it gives none.
type played struct {
	item userdata.Item
	at   *time.Time
}

// resumed is a resume point an import sets.
type resumed struct {
	item              userdata.Item
	percent           float64
	position, runtime time.Duration
	at                time.Time
}

// merge finds the titles of h and adds them to the user's data: played
// marks, then resume points of the titles still unplayed.
func (s *Service) merge(ctx context.Context, r *reader, h watchHistory, result *ImportResult) error {
	key := r.key
	refs := make([]library.TitleRef, 0, len(h.watched)+len(h.resumes))
	for _, w := range h.watched {
		refs = append(refs, w.ref)
	}
	for _, point := range h.resumes {
		refs = append(refs, point.ref)
	}
	targets, err := s.resolve(ctx, r, refs)
	if err != nil {
		return err
	}
	unmapped := map[library.TitleRef]bool{}
	marks := map[accounts.ID]*played{}
	var markOrder []accounts.ID
	for i, w := range h.watched {
		if len(targets[i]) == 0 {
			unmapped[w.ref] = true
		}
		for _, t := range targets[i] {
			item := userdata.Item{ID: t.ID, Series: t.Series, Season: t.Season}
			p := marks[t.ID]
			if p == nil {
				p = &played{item: item}
				marks[t.ID] = p
				markOrder = append(markOrder, t.ID)
			}
			if w.at != nil && (p.at == nil || w.at.After(*p.at)) {
				p.at = w.at
			}
		}
	}
	points := map[accounts.ID]*resumed{}
	var pointOrder []accounts.ID
	for i, r := range h.resumes {
		found := targets[len(h.watched)+i]
		if len(found) == 0 {
			unmapped[r.ref] = true
		}
		for _, t := range found {
			if p := points[t.ID]; p != nil && !r.at.After(p.at) {
				continue
			}
			if _, listed := points[t.ID]; !listed {
				pointOrder = append(pointOrder, t.ID)
			}
			runtime := r.runtime
			if runtime <= 0 {
				runtime = t.Runtime
			}
			points[t.ID] = &resumed{item: userdata.Item{ID: t.ID, Series: t.Series, Season: t.Season}, percent: r.percent,
				position: r.position, runtime: runtime, at: r.at}
		}
	}
	result.Unmapped = len(unmapped)

	for batch := range chunks(markOrder) {
		items := make([]userdata.Item, 0, len(batch))
		for _, id := range batch {
			items = append(items, marks[id].item)
		}
		if _, err := s.userData.ChangeEach(ctx, key.user, items, func(item userdata.Item, d *userdata.Data) {
			if markPlayed(d, marks[item.ID].at) {
				result.Played++
			}
		}); err != nil {
			return err
		}
	}
	settings := s.settings()
	thresholds := userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}
	for batch := range chunks(pointOrder) {
		items := make([]userdata.Item, 0, len(batch))
		for _, id := range batch {
			items = append(items, points[id].item)
		}
		if _, err := s.userData.ChangeEach(ctx, key.user, items, func(item userdata.Item, d *userdata.Data) {
			switch resume(d, points[item.ID], thresholds) {
			case resumePlayed:
				result.Played++
			case resumeSet:
				result.Resumed++
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// chunks yields ids importBatch at a time.
func chunks(ids []accounts.ID) func(func([]accounts.ID) bool) {
	return func(yield func([]accounts.ID) bool) {
		for start := 0; start < len(ids); start += importBatch {
			if !yield(ids[start:min(start+importBatch, len(ids))]) {
				return
			}
		}
	}
}

// markPlayed adds a played mark a history gives to d, watched at at: the
// item is played, at least once, on the later of its date and at; an
// earlier resume point goes. It reports whether the item was not played.
func markPlayed(d *userdata.Data, at *time.Time) bool {
	was := d.Played
	d.Played = true
	d.PlayCount = max(d.PlayCount, 1)
	if at != nil && (d.LastPlayed == nil || d.LastPlayed.Before(*at)) {
		// Watched elsewhere after the resume point was set here.
		d.Position = 0
		d.LastPlayed = new(at.UTC())
	}
	return !was
}

// What resume did with a resume point.
const (
	resumeKept = iota
	resumeSet
	resumePlayed
)

// resume adds a resume point a service keeps to d, under the thresholds
// of the settings: none for a played title, nor when Polyfin's is newer;
// past the played threshold the title is played, before the resume one
// nothing is kept.
func resume(d *userdata.Data, p *resumed, thresholds userdata.Thresholds) int {
	if d.Played || d.Position > 0 && d.LastPlayed != nil && !d.LastPlayed.Before(p.at) {
		return resumeKept
	}
	position, runtime := p.position, p.runtime
	if runtime <= 0 {
		// The runtime Polyfin measured its own resume point against.
		runtime = d.Runtime
	}
	if position <= 0 && runtime > 0 {
		position = time.Duration(p.percent / 100 * float64(runtime))
	}
	switch {
	case runtime > 0 && thresholds.Reaches(position, runtime),
		runtime <= 0 && p.percent > float64(thresholds.Played):
		markPlayed(d, &p.at)
		return resumePlayed
	case runtime <= 0 || float64(position)/float64(runtime)*100 < float64(thresholds.Resume):
		// Without a runtime there is no position to resume from.
		return resumeKept
	}
	d.Position, d.Runtime = position, runtime
	d.LastPlayed = new(p.at.UTC())
	return resumeSet
}

// resolve finds the items of refs. PublicMetaDB names titles by their
// TMDB identifier only: those Polyfin does not know by it are looked up
// by the IMDb identifier PublicMetaDB maps them to.
func (s *Service) resolve(ctx context.Context, r *reader, refs []library.TitleRef) ([][]library.TitleTarget, error) {
	targets, err := s.titles.Resolve(ctx, refs)
	if err != nil || r.key.service != PublicMetaDB {
		return targets, err
	}
	var missing []int
	var again []library.TitleRef
	for i, ref := range refs {
		if len(targets[i]) > 0 || ref.IMDb != "" || ref.TMDB == 0 {
			continue
		}
		imdb := s.mapPublicMetaDB(ctx, r, ref)
		if imdb == "" {
			continue
		}
		ref.IMDb = imdb
		missing, again = append(missing, i), append(again, ref)
	}
	if len(again) == 0 || ctx.Err() != nil {
		return targets, ctx.Err()
	}
	found, err := s.titles.Resolve(ctx, again)
	if err != nil {
		return nil, err
	}
	for j, i := range missing {
		targets[i] = found[j]
	}
	return targets, nil
}

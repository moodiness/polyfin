package trackers

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/metrics"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// Titles finds the items of titles named by other services' identifiers
// (see library.Service.Resolve).
type Titles interface {
	Resolve(ctx context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error)
}

// ErrImportOff reports an import asked of a service whose history is not
// imported. ErrNotConnected (see connections.go) reports a service the
// user did not connect.
var ErrImportOff = errors.New("watch history import is off")

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
	// points it set, Watchlist the titles it added to the user's
	// watchlist, and Unmapped the titles of the history and the watchlist
	// Polyfin could not identify.
	Played, Resumed, Watchlist, Unmapped int
	// Problem is empty, ProblemReconnect, ProblemUnreachable or
	// ProblemRateLimited: the history could not be read whole, and what
	// was read was imported.
	Problem string
}

// importRun is an import running.
type importRun struct {
	cancel context.CancelFunc
}

// importStatus completes status with the imports of its service, and when
// the next one runs: the last import plus the interval the settings give,
// while an import is on and the connection works.
func (s *Service) importStatus(ctx context.Context, key laneKey, status *Status) error {
	var last *time.Time
	var result ImportResult
	var problem *string
	err := s.db.QueryRow(ctx, `SELECT enabled, watchlist, last_import_at, played, resumed, watchlisted, unmapped, problem FROM tracking_imports
		WHERE user_id = $1 AND service = $2`, key.user, key.service).Scan(&status.ImportHistory, &status.ImportWatchlist, &last, &result.Played,
		&result.Resumed, &result.Watchlist, &result.Unmapped, &problem)
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
		if (status.ImportHistory || status.ImportWatchlist) && status.Available && status.Problem != ProblemReconnect {
			next := last.Add(s.settings().ImportEvery())
			status.NextImport = &next
		}
	}
	s.mu.Lock()
	_, status.Importing = s.importing[key]
	s.mu.Unlock()
	return nil
}

// SetImport turns the import of user's watch history from service on or
// off. Turned on, it imports at once, then as often as the settings say.
// The music services have no watch history: ErrUnknownService.
func (s *Service) SetImport(ctx context.Context, user accounts.ID, service string, on bool) (Status, error) {
	if !imports(service) {
		return Status{}, ErrUnknownService
	}
	return s.setImport(ctx, user, service, "enabled", on)
}

// SetWatchlistImport turns the import of user's watchlist from service on
// or off: Trakt's watchlist, Simkl's Plan to Watch, imported with the
// imports of the history, whether those import the history or not.
// Turned on, it imports at once. Turned off, the titles the list alone
// held leave the watchlist. The other services have no watchlist Polyfin
// imports: ErrUnknownService.
func (s *Service) SetWatchlistImport(ctx context.Context, user accounts.ID, service string, on bool) (Status, error) {
	if !watchlists(service) {
		return Status{}, ErrUnknownService
	}
	return s.setImport(ctx, user, service, "watchlist", on)
}

// setImport turns the import of what column of tracking_imports names,
// "enabled" for the history or "watchlist", on or off. Turned on, it
// imports at once; the imports stop once both are off.
func (s *Service) setImport(ctx context.Context, user accounts.ID, service, column string, on bool) (Status, error) {
	key := laneKey{user, service}
	if _, ok, err := s.connection(ctx, user, service); err != nil {
		return Status{}, err
	} else if !ok {
		return Status{}, ErrNotConnected
	}
	var history, list bool
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, "SELECT enabled, watchlist FROM tracking_imports WHERE user_id = $1 AND service = $2 FOR UPDATE",
			user, service).Scan(&history, &list)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO tracking_imports (user_id, service, `+column+`) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, service) DO UPDATE SET `+column+` = excluded.`+column, user, service, on)
		return err
	})
	if err != nil {
		return Status{}, err
	}
	was, other := history, list
	if column == "watchlist" {
		was, other = list, history
	}
	switch {
	case on && !was:
		s.startImport(key)
	case !on && (!other || column == "watchlist"):
		// An import running may be reading the list: it stops, and the
		// history, if on, is imported again when due.
		s.stopImport(key)
	}
	if !on && column == "watchlist" {
		if err := s.watchlist.Drop(ctx, user, watchlist.Tracking(service)); err != nil {
			return Status{}, err
		}
	}
	return s.status(ctx, user, service)
}

// ImportNow imports user's watch history, or watchlist, from service at
// once, unless an import of it runs already.
func (s *Service) ImportNow(ctx context.Context, user accounts.ID, service string) (Status, error) {
	if !imports(service) {
		return Status{}, ErrUnknownService
	}
	if _, ok, err := s.connection(ctx, user, service); err != nil {
		return Status{}, err
	} else if !ok {
		return Status{}, ErrNotConnected
	}
	var on bool
	err := s.db.QueryRow(ctx, "SELECT enabled OR watchlist FROM tracking_imports WHERE user_id = $1 AND service = $2", user, service).Scan(&on)
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
// the next connection starts with them off, and the titles its watchlist
// alone held leave the user's watchlist.
func (s *Service) forgetImport(ctx context.Context, key laneKey) error {
	s.stopImport(key)
	if _, err := s.db.Exec(ctx, "DELETE FROM tracking_imports WHERE user_id = $1 AND service = $2", key.user, key.service); err != nil {
		return err
	}
	return s.watchlist.Drop(ctx, key.user, watchlist.Tracking(key.service))
}

// importDue starts the imports due: turned on, of a connection that works,
// and not run for the interval the settings give. A new interval applies
// at once, to every import.
func (s *Service) importDue() {
	if s.titles == nil || s.userData == nil {
		return
	}
	settings := s.settings()
	rows, err := s.db.Query(s.ctx, `SELECT i.user_id, i.service FROM tracking_imports i
		JOIN tracking_connections c ON c.user_id = i.user_id AND c.service = i.service
		WHERE (i.enabled OR i.watchlist) AND c.problem IS DISTINCT FROM 'reconnect' AND (i.last_import_at IS NULL OR i.last_import_at <= $1)`,
		s.now().Add(-settings.ImportEvery()))
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

// history is what an import read of a service: what it watched and its
// resume points, and the titles it names that no mapping knows, which
// count as not found.
type watchHistory struct {
	watched []watchedEntry
	resumes []resumeEntry
	unknown map[string]bool
}

// runImport imports the watch history of key, and its watchlist, as
// turned on: reads them from the service, finds their titles, and adds
// what Polyfin lacks to the user's data and watchlist.
func (s *Service) runImport(ctx context.Context, key laneKey) {
	var cursor string
	var cursorFor *time.Time
	var history, list bool
	err := s.db.QueryRow(ctx, `SELECT cursor, cursor_for, enabled, watchlist FROM tracking_imports
		WHERE user_id = $1 AND service = $2 AND (enabled OR watchlist)`,
		key.user, key.service).Scan(&cursor, &cursorFor, &history, &list)
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
	var read watchHistory
	var listed watchlistRead
	next := cursor
	readErr := r.authorize(ctx)
	if readErr == nil && history {
		read, next, readErr = r.read(ctx, cursor)
	}
	if readErr == nil && list && watchlists(key.service) {
		listed, readErr = r.readWatchlist(ctx)
	}
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
	err = s.merge(ctx, r, read, &result)
	if err == nil && list && watchlists(key.service) {
		err = s.mergeWatchlist(ctx, r, listed, &result)
	}
	if err != nil {
		if ctx.Err() == nil {
			metrics.Imported(key.service, false)
			s.logger.Warn("A watch history could not be imported", "service", key.service, "error", err)
		}
		return
	}
	metrics.Imported(key.service, result.Problem == "")
	result.At = s.now().UTC()
	var problem *string
	if result.Problem != "" {
		problem = &result.Problem
	}
	if _, err := s.db.Exec(ctx, `UPDATE tracking_imports SET last_import_at = $3, played = $4, resumed = $5, unmapped = $6, problem = $7,
			cursor = $8, cursor_for = $9, watchlisted = $10
		WHERE user_id = $1 AND service = $2`, key.user, key.service, result.At, result.Played, result.Resumed, result.Unmapped, problem,
		next, conn.connectedAt, result.Watchlist); err != nil && ctx.Err() == nil {
		s.logger.Warn("A watch history import could not be recorded", "service", key.service, "error", err)
	}
	s.logger.Debug("Imported a watch history", "user_id", key.user.String(), "service", key.service, "played", result.Played,
		"resumed", result.Resumed, "watchlist", result.Watchlist, "unmapped", result.Unmapped)
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
// gives, nil when it gives none. counts tells whether marking it counts in
// the import's result (see counts).
type played struct {
	item   userdata.Item
	at     *time.Time
	counts bool
}

// resumed is a resume point an import sets.
type resumed struct {
	item   userdata.Item
	counts bool
	userdata.ImportedResume
}

// counts reports whether t, among the targets found of one title, counts
// in the import's result: the title an anime catalog lists under the
// title's anime entry is the one also found by its usual identifiers,
// counted once, unless it was found that way only.
func counts(found []library.TitleTarget, t library.TitleTarget) bool {
	return !t.Anime || !slices.ContainsFunc(found, func(o library.TitleTarget) bool { return !o.Anime })
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
			p.counts = p.counts || counts(targets[i], t)
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
			if p := points[t.ID]; p != nil && !r.at.After(p.At) {
				continue
			}
			if _, listed := points[t.ID]; !listed {
				pointOrder = append(pointOrder, t.ID)
			}
			runtime := r.runtime
			if runtime <= 0 {
				runtime = t.Runtime
			}
			points[t.ID] = &resumed{item: userdata.Item{ID: t.ID, Series: t.Series, Season: t.Season}, counts: counts(found, t),
				ImportedResume: userdata.ImportedResume{Percent: r.percent, Position: r.position, Runtime: runtime, At: r.at}}
		}
	}
	result.Unmapped = len(unmapped) + len(h.unknown)

	for batch := range chunks(markOrder) {
		items := make([]userdata.Item, 0, len(batch))
		for _, id := range batch {
			items = append(items, marks[id].item)
		}
		if _, err := s.userData.ChangeEach(ctx, key.user, items, func(item userdata.Item, d *userdata.Data) {
			if d.ImportPlayed(marks[item.ID].at) && marks[item.ID].counts {
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
			point := points[item.ID]
			switch d.ImportResume(point.ImportedResume, thresholds) {
			case userdata.ResumePlayed:
				if point.counts {
					result.Played++
				}
			case userdata.ResumeSet:
				if point.counts {
					result.Resumed++
				}
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

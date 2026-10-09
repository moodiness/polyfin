// Package lyrics finds the lyrics of music tracks on LRCLIB, a free and
// open lyrics database, synced line by line when it has them. It asks
// LRCLIB about one track at a time, by its artist, title, album and length,
// and keeps what LRCLIB answered in the database, lyrics or not, so that
// each track is asked about once. A request that fails is not an answer:
// the track is asked about again later, once LRCLIB has been left alone
// for a while.
package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

const (
	// LRCLIB is the address of LRCLIB's API.
	LRCLIB = "https://lrclib.net"
	// repository names Polyfin in the User-Agent, as LRCLIB asks of the
	// apps that use it.
	repository = "https://github.com/moodiness/polyfin"
	// askTimeout bounds each request to LRCLIB, which may look a track
	// up in other sources before it answers. Its answer is kept even once
	// the request that asked stopped waiting for it.
	askTimeout = 10 * time.Second
	// failedFor leaves LRCLIB unasked for a while after a request failed,
	// so that apps do not wait on a server that is down, and maxPause
	// bounds how long a Retry-After leaves it unasked.
	failedFor = time.Minute
	maxPause  = time.Hour
	// missingFor is how long LRCLIB not knowing a track is believed:
	// lyrics are added to it every day, so a track it did not know is
	// asked about again after that.
	missingFor = 30 * 24 * time.Hour
	// maxAsking bounds the requests to LRCLIB under way at once: tracks
	// asked for past it are asked about at their next request.
	maxAsking = 4
	// maxAnswer bounds the bytes read of an answer, a song's lyrics twice
	// over at most.
	maxAnswer = 1 << 20
	// saveTimeout bounds the write of an answer to the database.
	saveTimeout = 5 * time.Second
)

// The kinds of answer kept for a track, as the track_lyrics table names
// them: synced or plain lyrics, an instrumental, or a track LRCLIB does
// not know.
const (
	kindSynced       = "synced"
	kindPlain        = "plain"
	kindInstrumental = "instrumental"
	kindMissing      = "missing"
)

// Track is what LRCLIB is asked about: a music track, by its item, artist,
// title, album (empty when unknown) and length (0 when unknown).
type Track struct {
	ID       accounts.ID
	Artist   string
	Title    string
	Album    string
	Duration time.Duration
}

// Lyrics are a track's lyrics, line by line. Synced lyrics give each
// line's start; plain lyrics give none.
type Lyrics struct {
	Synced bool
	Lines  []Line
}

// Line is a line of lyrics, possibly empty between verses, and when it
// starts in the track when the lyrics are synced.
type Line struct {
	Text  string
	Start time.Duration
}

// Service finds lyrics on LRCLIB while the settings turn lyrics on. A nil
// Service finds none.
type Service struct {
	db *pgxpool.Pool
	// base is LRCLIB's address, without a trailing slash.
	base string
	// settings turn lyrics on, read at each request so that a change
	// applies at once.
	settings  func() accounts.Settings
	client    *http.Client
	userAgent string
	logger    *slog.Logger
	now       func() time.Time

	mu sync.Mutex
	// asking holds the tracks LRCLIB is being asked about.
	asking map[accounts.ID]*lookup
	// pausedUntil is when LRCLIB may be asked again after it failed.
	pausedUntil time.Time
}

// lookup is a request to LRCLIB about a track under way. Once done is
// closed, answer is what LRCLIB said when answered is true; a request that
// failed answered nothing.
type lookup struct {
	done     chan struct{}
	answer   answer
	answered bool
}

// answer is what LRCLIB said of a track, as kept: the kind of answer, the
// lyrics of synced and plain answers (LRC text for synced ones), and when
// LRCLIB was asked.
type answer struct {
	kind   string
	lyrics string
	at     time.Time
}

// New returns a service asking LRCLIB at base, and keeping its answers in
// db, while settings turn lyrics on. version is Polyfin's, which the
// User-Agent sent to LRCLIB names with Polyfin's repository.
func New(db *pgxpool.Pool, base, version string, logger *slog.Logger, settings func() accounts.Settings) *Service {
	return &Service{
		db:        db,
		base:      strings.TrimSuffix(base, "/"),
		settings:  settings,
		client:    &http.Client{},
		userAgent: "Polyfin/" + version + " (" + repository + ")",
		logger:    logger,
		now:       time.Now,
		asking:    map[accounts.ID]*lookup{},
	}
}

// on reports whether lyrics are served: a service exists and the settings
// turn lyrics on.
func (s *Service) on() bool {
	return s != nil && s.settings().Lyrics
}

// Lyrics returns the lyrics of track. LRCLIB is asked about a track it was
// never asked about, or that it did not know missingFor ago, and Lyrics
// waits for its answer wait at most, or until ctx is done: an answer that
// comes later is kept for the next request. A track without lyrics, an
// instrumental, one LRCLIB does not know or could not be asked about, or
// one without an artist or title, has none, as do all tracks while the
// settings turn lyrics off.
func (s *Service) Lyrics(ctx context.Context, track Track, wait time.Duration) (Lyrics, bool) {
	if !s.on() {
		return Lyrics{}, false
	}
	kept, ok := s.load(ctx, track.ID)
	if !ok || s.due(kept) {
		if l := s.ask(track); l != nil && wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-l.done:
				if l.answered {
					kept, ok = l.answer, true
				}
			case <-timer.C:
			case <-ctx.Done():
			}
		}
	}
	if !ok {
		return Lyrics{}, false
	}
	return kept.parse()
}

// LookUp asks LRCLIB about track in the background, when it would be asked
// for its lyrics (see Lyrics), so that they are known by the time an app
// asks for them.
func (s *Service) LookUp(track Track) {
	if !s.on() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		defer cancel()
		_, _ = s.Lyrics(ctx, track, 0)
	}()
}

// Known reports which of the tracks ids have lyrics, as LRCLIB gave them
// when it was asked about them; none while the settings turn lyrics off.
// It never asks LRCLIB.
func (s *Service) Known(ctx context.Context, ids []accounts.ID) map[accounts.ID]bool {
	if !s.on() || len(ids) == 0 {
		return nil
	}
	rows, err := s.db.Query(ctx, "SELECT item_id FROM track_lyrics WHERE item_id = ANY($1) AND kind IN ('synced', 'plain')", ids)
	if err == nil {
		var found []accounts.ID
		if found, err = pgx.CollectRows(rows, pgx.RowTo[accounts.ID]); err == nil {
			known := make(map[accounts.ID]bool, len(found))
			for _, id := range found {
				known[id] = true
			}
			return known
		}
	}
	if ctx.Err() == nil {
		s.logger.Warn("Reading which tracks have lyrics failed", "error", err)
	}
	return nil
}

// load reads what LRCLIB answered of a track when it was asked; nothing
// when it was not, or the database cannot be read.
func (s *Service) load(ctx context.Context, id accounts.ID) (answer, bool) {
	var kept answer
	err := s.db.QueryRow(ctx, "SELECT kind, lyrics, looked_up_at FROM track_lyrics WHERE item_id = $1", id).
		Scan(&kept.kind, &kept.lyrics, &kept.at)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Warn("Reading a track's lyrics failed", "error", err)
		}
		return answer{}, false
	}
	return kept, true
}

// due reports whether a track LRCLIB answered about is asked about again:
// one it did not know missingFor ago.
func (s *Service) due(kept answer) bool {
	return kept.kind == kindMissing && s.now().Sub(kept.at) >= missingFor
}

// ask starts asking LRCLIB about track, or joins the request about it
// under way. It starts none, and returns nil, for a track without an
// artist or title, while LRCLIB is left alone after a failure, or while
// maxAsking requests are under way.
func (s *Service) ask(track Track) *lookup {
	if strings.TrimSpace(track.Artist) == "" || strings.TrimSpace(track.Title) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, asking := s.asking[track.ID]; asking {
		return l
	}
	if s.now().Before(s.pausedUntil) || len(s.asking) >= maxAsking {
		return nil
	}
	l := &lookup{done: make(chan struct{})}
	s.asking[track.ID] = l
	go s.run(track, l)
	return l
}

// run asks LRCLIB about track and keeps its answer. When LRCLIB fails,
// nothing is kept, and LRCLIB is left alone for a while.
func (s *Service) run(track Track, l *lookup) {
	defer close(l.done)
	ctx, cancel := context.WithTimeout(context.Background(), askTimeout)
	found, err := s.fetch(ctx, track)
	cancel()
	if err != nil {
		s.mu.Lock()
		delete(s.asking, track.ID)
		s.pause(err)
		s.mu.Unlock()
		return
	}
	l.answer, l.answered = found.answer(s.now()), true
	ctx, cancel = context.WithTimeout(context.Background(), saveTimeout)
	_, err = s.db.Exec(ctx, `INSERT INTO track_lyrics (item_id, kind, lyrics, looked_up_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (item_id) DO UPDATE SET kind = excluded.kind, lyrics = excluded.lyrics, looked_up_at = excluded.looked_up_at`,
		track.ID, l.answer.kind, l.answer.lyrics, l.answer.at)
	cancel()
	if err != nil {
		s.logger.Warn("Saving a track's lyrics failed", "error", err)
	}
	// Forgotten once kept, so that a request after this one reads the
	// answer rather than asking again.
	s.mu.Lock()
	delete(s.asking, track.ID)
	s.mu.Unlock()
	s.logger.Debug("LRCLIB answered about a track", "track", track.ID, "kind", l.answer.kind)
}

// pause leaves LRCLIB unasked for failedFor after a request failed, or as
// long as the Retry-After of a 429 says. It is called with mu held.
func (s *Service) pause(err error) {
	until := s.now().Add(failedFor)
	var limited *rateLimited
	if errors.As(err, &limited) {
		until = limited.until
	}
	if until.After(s.pausedUntil) {
		s.pausedUntil = until
	}
	s.logger.Info("LRCLIB did not answer", "error", err, "until", until)
}

// rateLimited reports a 429 with a Retry-After, and when LRCLIB may be
// asked again.
type rateLimited struct {
	until time.Time
}

func (e *rateLimited) Error() string {
	return "HTTP 429 until " + e.until.Format(time.RFC3339)
}

// record is what LRCLIB knows of a track: its lyrics, plain and synced as
// LRC, either empty when it has none, and whether it is an instrumental.
type record struct {
	Instrumental bool   `json:"instrumental"`
	PlainLyrics  string `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
}

// answer is what is kept of a record: synced lyrics when they hold a
// timed line, else plain lyrics, else that the track is an instrumental. A
// record without any is kept as a track LRCLIB does not know.
func (r record) answer(at time.Time) answer {
	switch {
	case len(parseLRC(r.SyncedLyrics)) > 0:
		return answer{kind: kindSynced, lyrics: r.SyncedLyrics, at: at}
	case strings.TrimSpace(r.PlainLyrics) != "":
		return answer{kind: kindPlain, lyrics: r.PlainLyrics, at: at}
	case r.Instrumental:
		return answer{kind: kindInstrumental, at: at}
	}
	return answer{kind: kindMissing, at: at}
}

// parse returns the lyrics a kept answer holds.
func (a answer) parse() (Lyrics, bool) {
	switch a.kind {
	case kindSynced:
		lines := parseLRC(a.lyrics)
		return Lyrics{Synced: true, Lines: lines}, len(lines) > 0
	case kindPlain:
		return Lyrics{Lines: plainLines(a.lyrics)}, true
	}
	return Lyrics{}, false
}

// fetch asks LRCLIB for the record of track: an empty one, without error,
// when LRCLIB does not know it.
func (s *Service) fetch(ctx context.Context, track Track) (*record, error) {
	query := url.Values{"artist_name": {track.Artist}, "track_name": {track.Title}}
	if track.Album != "" {
		query.Set("album_name", track.Album)
	}
	// LRCLIB matches lengths to within 2 seconds.
	if seconds := int(math.Round(track.Duration.Seconds())); seconds > 0 {
		query.Set("duration", strconv.Itoa(seconds))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/api/get?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", s.userAgent)
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		// Without the address, which names the track.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, err
	}
	defer response.Body.Close()
	switch status := response.StatusCode; {
	case status == http.StatusNotFound:
		return &record{}, nil
	case status == http.StatusTooManyRequests:
		if until, ok := retryAfter(response.Header.Get("Retry-After"), s.now()); ok {
			return nil, &rateLimited{until: until}
		}
		return nil, fmt.Errorf("HTTP %d", status)
	case status != http.StatusOK:
		return nil, fmt.Errorf("HTTP %d", status)
	}
	var found record
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(&found); err != nil {
		return nil, fmt.Errorf("read the answer: %w", err)
	}
	return &found, nil
}

// retryAfter reads a Retry-After header, in seconds or as a date, into when
// LRCLIB may be asked again, at most maxPause after now.
func retryAfter(value string, now time.Time) (time.Time, bool) {
	var until time.Time
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		until = now.Add(time.Duration(seconds) * time.Second)
	} else if date, err := http.ParseTime(value); err == nil {
		until = date
	} else {
		return time.Time{}, false
	}
	if latest := now.Add(maxPause); until.After(latest) {
		until = latest
	}
	return until, true
}

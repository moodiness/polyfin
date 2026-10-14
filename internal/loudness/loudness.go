// Package loudness evens the volume from one song to the next: apps are
// told how much to raise or lower each song and album, as Jellyfin's
// NormalizationGain tells them, in dB relative to Reference. A song's gain
// is its ReplayGain track tag, else its loudness measured with FFmpeg's
// ebur128 filter (EBU R128); an album's, its album tag when its songs agree
// on one, else the loudness of all its songs together once each is known.
//
// The songs of local music folders are read and measured with their
// folder (see package localfiles); the songs of music addons are measured
// here, once, after their first play, by reading their stream as playback
// does. A Meter bounds the songs measured at once over both.
package loudness

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

const (
	// Reference is the loudness gains bring songs to: -18 LUFS, that of
	// ReplayGain 2.0 and of Jellyfin's gains.
	Reference = -18.0
	// r128Reference is the loudness Opus's R128 gains bring songs to, EBU
	// R128's: their gains are moved by the difference.
	r128Reference = -23.0
	// Silence is the loudness ebur128 gives a silent song: it has no gain,
	// as no gain would make it heard.
	Silence = -70.0
	// MaxLength bounds the songs measured: longer ones, as podcasts' long
	// episodes are, never are.
	MaxLength = 15 * time.Minute
	// RetryAfter is how long a song that could not be measured waits for
	// its next attempt, and MaxAttempts bounds its attempts.
	RetryAfter  = 24 * time.Hour
	MaxAttempts = 3
	// meters bounds the songs measured at once.
	meters = 2
	// measureTimeout bounds one song's measure, read from a slow source
	// too.
	measureTimeout = 15 * time.Minute
	// saveTimeout bounds the write of a measure to the database.
	saveTimeout = 5 * time.Second
	// maxLog bounds what is kept of FFmpeg's log, its end: the summary.
	maxLog = 64 << 10
)

// Gain is the gain of a song or album of integrated loudness l, in LUFS,
// rounded to a hundredth of a dB; none for a silent one.
func Gain(l float64) (float64, bool) {
	if l <= Silence || math.IsNaN(l) || math.IsInf(l, 0) {
		return 0, false
	}
	return math.Round((Reference-l)*100) / 100, true
}

// maxTagGain bounds the gains read from tags: more is not a gain a
// scanner writes.
const maxTagGain = 64

// ReplayGain reads a ReplayGain tag, such as "-6.54 dB": a gain relative
// to Reference.
func ReplayGain(tag string) (float64, bool) {
	value := strings.TrimSpace(tag)
	if len(value) > 2 && strings.EqualFold(value[len(value)-2:], "db") {
		value = strings.TrimSpace(value[:len(value)-2])
	}
	gain, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(gain) || math.Abs(gain) > maxTagGain {
		return 0, false
	}
	return gain, true
}

// R128Gain reads an Opus R128 tag (R128_TRACK_GAIN, R128_ALBUM_GAIN): a
// gain in 256ths of a dB relative to EBU R128's reference, moved to
// Reference.
func R128Gain(tag string) (float64, bool) {
	q, err := strconv.Atoi(strings.TrimSpace(tag))
	if err != nil || q < math.MinInt16 || q > math.MaxInt16 {
		return 0, false
	}
	return float64(q)/256 + Reference - r128Reference, true
}

// TogetherSQL is the SQL of the integrated loudness of songs together, in
// an aggregate over their rows: the mean of their energies, each weighed
// by its length (length, in seconds, a second at least), of their
// loudness (in LUFS).
func TogetherSQL(loudness, length string) string {
	weight := "greatest(" + length + ", 1)"
	return "10 * log(sum(" + weight + " * power(10, " + loudness + " / 10)) / sum(" + weight + "))"
}

// Meter measures the loudness of songs with FFmpeg, meters at a time.
type Meter struct {
	ffmpeg string
	slots  chan struct{}
}

// NewMeter returns a Meter running the FFmpeg at ffmpeg.
func NewMeter(ffmpeg string) *Meter {
	return &Meter{ffmpeg: ffmpeg, slots: make(chan struct{}, meters)}
}

// integrated is the integrated loudness of ebur128's summary.
var integrated = regexp.MustCompile(`Integrated loudness:\s+I:\s+(-?(?:[0-9.]+|inf)) LUFS`)

// Measure returns the integrated loudness, in LUFS, of the first audio
// track of the file at address, read with FFmpeg's input options, as
// FFmpeg's ebur128 filter measures it, its audio alone decoded, at the
// lowest priority. It waits for one of the meter's places, or for ctx.
func (m *Meter) Measure(ctx context.Context, address string, options ...string) (float64, error) {
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, measureTimeout)
	defer cancel()
	args := append([]string{"-nostdin", "-hide_banner", "-nostats", "-loglevel", "info"}, options...)
	args = append(args, "-i", address, "-map", "0:a:0", "-af", "ebur128=framelog=quiet", "-f", "null", "-")
	cmd := exec.CommandContext(ctx, m.ffmpeg, args...)
	log := &tail{}
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	media.LowerPriority(cmd.Process.Pid)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("ffmpeg: %w: %s", err, lastLine(log.String()))
	}
	found := integrated.FindAllStringSubmatch(log.String(), -1)
	if len(found) == 0 {
		return 0, errors.New("ffmpeg gave no integrated loudness")
	}
	value := found[len(found)-1][1]
	if strings.HasSuffix(value, "inf") {
		return Silence, nil
	}
	l, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return max(l, Silence), nil
}

// tail keeps the last maxLog bytes written to it.
type tail struct{ data []byte }

func (t *tail) Write(p []byte) (int, error) {
	t.data = append(t.data, p...)
	if extra := len(t.data) - maxLog; extra > 0 {
		t.data = append(t.data[:0], t.data[extra:]...)
	}
	return len(p), nil
}

func (t *tail) String() string { return string(t.data) }

// lastLine is the last line of a log that is not blank.
func lastLine(log string) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Track is a song of a music addon played: its item, its album's (zero
// for none), and its length (zero when unknown).
type Track struct {
	ID, Album accounts.ID
	Length    time.Duration
}

// Album is an album of a music addon whose gain is asked for: its item,
// and how many songs it holds (zero when unknown).
type Album struct {
	ID     accounts.ID
	Tracks int
}

// Reader reads a song's stream as playback reads it, through its relay
// and with its link renewed: read is given the address FFmpeg reads it
// from, and the input options it reads it with.
type Reader func(ctx context.Context, read func(ctx context.Context, input string, options []string) error) error

// Service gives the gains of the songs of music addons, and measures them
// after their first play while the settings ask for it. A nil Service
// gives none and measures none.
type Service struct {
	db       *pgxpool.Pool
	meter    *Meter
	logger   *slog.Logger
	settings func() accounts.Settings
	now      func() time.Time

	// ctx ends the measures under way, which wg waits for.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.Mutex
	// measuring holds the songs being measured.
	measuring map[accounts.ID]bool
}

// New returns a service keeping its measures in db, measuring with meter
// while settings ask for it; Close stops it.
func New(db *pgxpool.Pool, meter *Meter, logger *slog.Logger, settings func() accounts.Settings) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{db: db, meter: meter, logger: logger, settings: settings, now: time.Now, ctx: ctx, cancel: cancel,
		measuring: map[accounts.ID]bool{}}
}

// Close stops the measures under way and waits for them.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

// Wait waits for the measures under way.
func (s *Service) Wait() {
	if s != nil {
		s.wg.Wait()
	}
}

// Even reports whether apps are given gains: Even the volume between
// songs is on.
func (s *Service) Even() bool {
	return s != nil && s.settings().EvenVolume
}

// Measures reports whether the songs of local music folders, of music
// addons when addons is set, have their loudness measured as the
// settings ask (see accounts.Settings.MeasureLoudness).
func Measures(settings accounts.Settings, addons bool) bool {
	if !settings.EvenVolume {
		return false
	}
	return settings.MeasureLoudness == "all" || !addons && settings.MeasureLoudness == "local"
}

// Played measures, in the background, the loudness of a song of a music
// addon whose play started, through reader, when the settings measure
// addons' songs: once, unless it could not be measured, which is tried
// again after RetryAfter, MaxAttempts times at most. A song longer than
// MaxLength, or whose length is unknown, never is.
func (s *Service) Played(track Track, reader Reader) {
	if s == nil || !Measures(s.settings(), true) || track.Length <= 0 || track.Length > MaxLength {
		return
	}
	s.mu.Lock()
	if s.measuring[track.ID] {
		s.mu.Unlock()
		return
	}
	s.measuring[track.ID] = true
	s.mu.Unlock()
	s.wg.Go(func() {
		defer func() {
			s.mu.Lock()
			delete(s.measuring, track.ID)
			s.mu.Unlock()
		}()
		if !s.due(track.ID) {
			return
		}
		var loudness float64
		err := reader(s.ctx, func(ctx context.Context, input string, options []string) error {
			var err error
			loudness, err = s.meter.Measure(ctx, input, options...)
			return err
		})
		if s.ctx.Err() != nil {
			return
		}
		s.keep(track, loudness, err)
	})
}

// due reports whether a song is to be measured: never measured, or
// failed fewer than MaxAttempts times, the last RetryAfter ago.
func (s *Service) due(id accounts.ID) bool {
	ctx, cancel := context.WithTimeout(s.ctx, saveTimeout)
	defer cancel()
	var loudness *float64
	var failures int
	var retry *time.Time
	err := s.db.QueryRow(ctx, "SELECT loudness, failures, retry_at FROM track_loudness WHERE item_id = $1", id).Scan(&loudness, &failures, &retry)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return true
	case err != nil:
		if s.ctx.Err() == nil {
			s.logger.Warn("Reading a song's loudness failed", "error", err)
		}
		return false
	}
	return loudness == nil && failures < MaxAttempts && (retry == nil || !s.now().Before(*retry))
}

// keep saves a song's loudness, or that it could not be measured.
func (s *Service) keep(track Track, loudness float64, failure error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), saveTimeout)
	defer cancel()
	var album *accounts.ID
	if track.Album != (accounts.ID{}) {
		album = &track.Album
	}
	now := s.now()
	var err error
	if failure != nil {
		s.logger.Info("A song's loudness could not be measured", "error", failure)
		_, err = s.db.Exec(ctx, `INSERT INTO track_loudness (item_id, album_id, length, failures, retry_at, measured_at)
			VALUES ($1, $2, $3, 1, $4, $5)
			ON CONFLICT (item_id) DO UPDATE SET album_id = excluded.album_id, length = excluded.length,
				failures = track_loudness.failures + 1, retry_at = excluded.retry_at, measured_at = excluded.measured_at`,
			track.ID, album, track.Length.Seconds(), now.Add(RetryAfter), now)
	} else {
		_, err = s.db.Exec(ctx, `INSERT INTO track_loudness (item_id, album_id, length, loudness, failures, retry_at, measured_at)
			VALUES ($1, $2, $3, $4, 0, NULL, $5)
			ON CONFLICT (item_id) DO UPDATE SET album_id = excluded.album_id, length = excluded.length, loudness = excluded.loudness,
				failures = 0, retry_at = NULL, measured_at = excluded.measured_at`,
			track.ID, album, track.Length.Seconds(), loudness, now)
		s.logger.Debug("Measured a song's loudness", "track", track.ID, "loudness", loudness)
	}
	if err != nil {
		s.logger.Warn("Saving a song's loudness failed", "error", err)
	}
}

// Gains returns the gains known of the songs and albums of music addons,
// by item, while Even the volume between songs is on: a song's, from its
// loudness; an album's, from the loudness of its songs together, once
// each of its tracks is measured. It never measures.
func (s *Service) Gains(ctx context.Context, songs []accounts.ID, albums []Album) map[accounts.ID]float64 {
	if !s.Even() || len(songs) == 0 && len(albums) == 0 {
		return nil
	}
	gains := map[accounts.ID]float64{}
	if err := s.songGains(ctx, songs, gains); err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Reading the songs' gains failed", "error", err)
		}
		return nil
	}
	if err := s.albumGains(ctx, albums, gains); err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Reading the albums' gains failed", "error", err)
		}
		return nil
	}
	return gains
}

func (s *Service) songGains(ctx context.Context, songs []accounts.ID, gains map[accounts.ID]float64) error {
	if len(songs) == 0 {
		return nil
	}
	rows, err := s.db.Query(ctx, "SELECT item_id, loudness FROM track_loudness WHERE item_id = ANY($1) AND loudness IS NOT NULL", songs)
	if err != nil {
		return err
	}
	var id accounts.ID
	var loudness float64
	_, err = pgx.ForEachRow(rows, []any{&id, &loudness}, func() error {
		if gain, ok := Gain(loudness); ok {
			gains[id] = gain
		}
		return nil
	})
	return err
}

func (s *Service) albumGains(ctx context.Context, albums []Album, gains map[accounts.ID]float64) error {
	tracks := map[accounts.ID]int{}
	var ids []accounts.ID
	for _, a := range albums {
		if a.Tracks > 0 {
			tracks[a.ID] = a.Tracks
			ids = append(ids, a.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.db.Query(ctx, `SELECT album_id, count(*), `+TogetherSQL("loudness", "length")+`
		FROM track_loudness WHERE album_id = ANY($1) AND loudness IS NOT NULL GROUP BY album_id`, ids)
	if err != nil {
		return err
	}
	var id accounts.ID
	var measured int
	var loudness float64
	_, err = pgx.ForEachRow(rows, []any{&id, &measured, &loudness}, func() error {
		if gain, ok := Gain(loudness); ok && measured >= tracks[id] {
			gains[id] = gain
		}
		return nil
	})
	return err
}

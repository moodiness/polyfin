// Package audiosegments finds the intros of the episodes of local folders
// in their audio, where neither their chapters nor the segment databases
// give one: an intro is the stretch of audio the episodes of a season
// share near their start.
//
// When an episode of a local folder plays without an intro, its season is
// compared in the background, one season at a time: the start of each of
// its files, its first quarter and maxListened at most, is decoded by
// FFmpeg, at the lowest priority, to mono samples Polyfin fingerprints
// itself (see fingerprint), so that any build of FFmpeg serves. Each file
// is then compared with each other file of the season but those of the
// same episodes: the longest stretch two files share, from shortestIntro
// to longestIntro, is each one's intro (see shared), the longest of those
// a file shares with any other.
//
// Fingerprints and the intros found are kept in the database by file, its
// path, size and modification time: a file is read once, and again only
// when it changes. A season is compared again when one of its episodes
// plays once its files changed, as when an episode is added.
package audiosegments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/mediasegments"
)

const (
	// maxListened bounds the start of an episode fingerprinted, else its
	// first quarter: intros come within minutes of the start.
	maxListened = 10 * time.Minute
	// An intro lasts from shortestIntro to longestIntro: less is a sound
	// effect or a jingle, more is no intro.
	shortestIntro = 15 * time.Second
	longestIntro  = 2 * time.Minute
	// queueLength bounds the seasons waiting to be compared; one left out
	// is queued again by the next play of one of its episodes.
	queueLength = 16
	// readTimeout bounds the reading of a file, from a slow share too.
	readTimeout = 15 * time.Minute
)

// look is what a comparison of a season looks for: a type of segment, in
// a part of each file, lasting from shortest to longest.
type look struct {
	kind mediasegments.Type
	part string
	// listened is the part of a file lasting length that is fingerprinted.
	listened          func(length time.Duration) (start, end time.Duration)
	shortest, longest time.Duration
}

// intros looks for intros at the start of the files.
var intros = look{
	kind: mediasegments.Intro,
	part: "start",
	listened: func(length time.Duration) (time.Duration, time.Duration) {
		return 0, min(length/4, maxListened)
	},
	shortest: shortestIntro,
	longest:  longestIntro,
}

// Files finds the files of local episodes and their seasons.
type Files interface {
	// EpisodeFile returns the file a version's address names, if it is the
	// file of episodes of a local folder, read as one file.
	EpisodeFile(ctx context.Context, address string) (localfiles.EpisodeFile, bool, error)
	// SeasonFiles lists the files of the season of a file EpisodeFile
	// returned, itself included.
	SeasonFiles(ctx context.Context, file localfiles.EpisodeFile) ([]localfiles.EpisodeFile, error)
}

// Options are what a Service needs.
type Options struct {
	DB *pgxpool.Pool
	// FFmpeg decodes the files' audio; FFprobe tells how long they last.
	FFmpeg, FFprobe string
	Files           Files
	// Settings turn the search on: AudioIntros, with SkipButtons.
	Settings func() accounts.Settings
	Logger   *slog.Logger
}

// Service compares the seasons of the local episodes played, one at a
// time, and gives the intros found.
type Service struct {
	Options
	queue  chan localfiles.EpisodeFile
	mu     sync.Mutex
	queued map[string]bool
	cancel context.CancelFunc
	done   chan struct{}
}

// New starts a Service; Close stops it.
func New(options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{Options: options, queue: make(chan localfiles.EpisodeFile, queueLength), queued: map[string]bool{},
		cancel: cancel, done: make(chan struct{})}
	go s.run(ctx)
	return s
}

// Close stops the comparison under way, and waits for it.
func (s *Service) Close() {
	s.cancel()
	<-s.done
}

// on tells whether the settings turn the search on.
func (s *Service) on() bool {
	settings := s.Settings()
	return settings.SkipButtons && settings.AudioIntros
}

// Intro returns the intro found in the audio of the file a version's
// address names, if the file has not changed since and the settings turn
// the search on.
func (s *Service) Intro(ctx context.Context, address string) (mediasegments.Segment, bool) {
	if !s.on() {
		return mediasegments.Segment{}, false
	}
	file, ok, err := s.Files.EpisodeFile(ctx, address)
	if err != nil {
		s.Logger.Warn("The file of a local episode could not be read from the database", "error", err)
	}
	if !ok {
		return mediasegments.Segment{}, false
	}
	return s.found(ctx, file, intros)
}

// found returns the segment found in a file by the comparison of its
// season, as long as the file has not changed.
func (s *Service) found(ctx context.Context, file localfiles.EpisodeFile, l look) (mediasegments.Segment, bool) {
	var start, end *int64
	err := s.DB.QueryRow(ctx, `SELECT start_ms, end_ms FROM audio_segments WHERE path = $1 AND type = $2 AND size = $3 AND modified = $4`,
		file.Path, string(l.kind), file.Size, file.Modified).Scan(&start, &end)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.Logger.Warn("A segment found in a local episode's audio could not be read", "error", err)
	}
	if err != nil || start == nil {
		return mediasegments.Segment{}, false
	}
	return mediasegments.Segment{Type: l.kind, Start: time.Duration(*start) * time.Millisecond, End: time.Duration(*end) * time.Millisecond}, true
}

// Reads reports whether the search reads the file a version's address
// names, while the settings turn it on: whether its play may have its
// season compared (see Played).
func (s *Service) Reads(ctx context.Context, address string) bool {
	if !s.on() {
		return false
	}
	_, ok, err := s.Files.EpisodeFile(ctx, address)
	if err != nil {
		s.Logger.Warn("The file of a local episode could not be read from the database", "error", err)
	}
	return ok
}

// Played has the season of the file a version's address names compared
// in the background, once the others queued before it are, unless it was
// compared with the files the season has now, or the settings turn the
// search off. It is for a version played without an intro from its
// chapters or a segment database.
func (s *Service) Played(ctx context.Context, address string) {
	if !s.on() {
		return
	}
	file, ok, err := s.Files.EpisodeFile(ctx, address)
	if err != nil {
		s.Logger.Warn("The file of a local episode could not be read from the database", "error", err)
	}
	if !ok {
		return
	}
	files, err := s.Files.SeasonFiles(ctx, file)
	if err != nil {
		s.Logger.Warn("The files of a local season could not be listed", "error", err)
		return
	}
	var compared string
	err = s.DB.QueryRow(ctx, `SELECT season FROM audio_segments WHERE path = $1 AND type = $2 AND size = $3 AND modified = $4`,
		file.Path, string(intros.kind), file.Size, file.Modified).Scan(&compared)
	if err == nil && compared == seasonKey(files) {
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.Logger.Warn("A segment found in a local episode's audio could not be read", "error", err)
		return
	}
	key := file.Folder.String() + "|" + file.Title + "|" + strconv.Itoa(file.Season)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queued[key] {
		return
	}
	select {
	case s.queue <- file:
		s.queued[key] = true
	default:
	}
}

// run compares the seasons queued, one at a time, until ctx ends.
func (s *Service) run(ctx context.Context) {
	defer close(s.done)
	for {
		select {
		case <-ctx.Done():
			return
		case file := <-s.queue:
			if s.on() {
				if err := s.compare(ctx, file, intros); err != nil && ctx.Err() == nil {
					s.Logger.Warn("The audio of a local season could not be compared", "error", err)
				}
			}
			s.mu.Lock()
			delete(s.queued, file.Folder.String()+"|"+file.Title+"|"+strconv.Itoa(file.Season))
			s.mu.Unlock()
		}
	}
}

// seasonKey tells the files of a season apart from the same season with a
// file added, removed or changed.
func seasonKey(files []localfiles.EpisodeFile) string {
	hash := sha256.New()
	for _, file := range files {
		hash.Write([]byte(file.Path + "\x00" + strconv.FormatInt(file.Size, 10) + "\x00" +
			strconv.FormatInt(file.Modified.UnixMicro(), 10) + "\n"))
	}
	return hex.EncodeToString(hash.Sum(nil)[:16])
}

// compare compares the files of the season of played with one another,
// for what l looks for, and keeps what it finds in each, none included. A
// file that cannot be read is left out, to be read at the next comparison.
func (s *Service) compare(ctx context.Context, played localfiles.EpisodeFile, l look) error {
	files, err := s.Files.SeasonFiles(ctx, played)
	if err != nil {
		return err
	}
	prints := make([]Fingerprint, len(files))
	read := make([]bool, len(files))
	for i, file := range files {
		prints[i], err = s.fingerprint(ctx, file, l)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			s.Logger.Info("The audio of a local episode could not be read", "path", file.Path, "error", err)
			continue
		}
		read[i] = true
	}
	best := make([]span, len(files))
	for i := range files {
		for j := i + 1; j < len(files); j++ {
			if !read[i] || !read[j] || files[i].First <= files[j].Last && files[j].First <= files[i].Last {
				continue
			}
			m, ok := shared(prints[i], prints[j], l.shortest, l.longest)
			if !ok {
				continue
			}
			if m.a.length() > best[i].length() {
				best[i] = m.a
			}
			if m.b.length() > best[j].length() {
				best[j] = m.b
			}
		}
	}
	season := seasonKey(files)
	batch := &pgx.Batch{}
	found := 0
	for i, file := range files {
		if !read[i] {
			continue
		}
		var start, end *int64
		if best[i].length() > 0 {
			start, end = new(best[i].Start.Milliseconds()), new(best[i].End.Milliseconds())
			found++
		}
		batch.Queue(`INSERT INTO audio_segments (path, type, size, modified, season, start_ms, end_ms) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (path, type) DO UPDATE SET size = $3, modified = $4, season = $5, start_ms = $6, end_ms = $7`,
			file.Path, string(l.kind), file.Size, file.Modified, season, start, end)
	}
	if err := s.DB.SendBatch(ctx, batch).Close(); err != nil {
		return err
	}
	s.Logger.Debug("The audio of a local season was compared", "files", len(files), "found", found, "type", string(l.kind))
	return nil
}

// fingerprint returns the fingerprint of the part of a file l looks in:
// the one kept, if the file has not changed since, else read now and
// kept.
func (s *Service) fingerprint(ctx context.Context, file localfiles.EpisodeFile, l look) (Fingerprint, error) {
	var start int64
	var frames []byte
	err := s.DB.QueryRow(ctx, `SELECT start_ms, frames FROM audio_fingerprints WHERE path = $1 AND part = $2 AND size = $3 AND modified = $4`,
		file.Path, l.part, file.Size, file.Modified).Scan(&start, &frames)
	if err == nil {
		return Fingerprint{Start: time.Duration(start) * time.Millisecond, Frames: decode(frames)}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Fingerprint{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	length, err := duration(ctx, s.FFprobe, file.URL)
	if err != nil {
		return Fingerprint{}, err
	}
	from, to := l.listened(length)
	// Kept in whole milliseconds, as it is stored.
	from = from.Truncate(time.Millisecond)
	f, err := listen(ctx, s.FFmpeg, file.URL, from, to-from)
	if err != nil {
		return Fingerprint{}, err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO audio_fingerprints (path, part, size, modified, start_ms, frames) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (path, part) DO UPDATE SET size = $3, modified = $4, start_ms = $5, frames = $6`,
		file.Path, l.part, file.Size, file.Modified, from.Milliseconds(), encode(f.Frames))
	return f, err
}

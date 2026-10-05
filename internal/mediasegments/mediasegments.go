// Package mediasegments finds where the intro, recap, credits and preview of
// a movie or an episode are, for the skip buttons of Jellyfin apps. It asks
// community databases, TheIntroDB, IntroDB and PublicMetaDB, by the title's
// IMDb or TMDB identifier, and keeps their answers in the database.
package mediasegments

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The databases, by the names POLYFIN_SEGMENTS gives them.
const (
	TheIntroDB   = "theintrodb"
	IntroDB      = "introdb"
	PublicMetaDB = "publicmetadb"
)

// publicURLs are the base URLs of the databases' APIs.
var publicURLs = map[string]string{
	TheIntroDB:   "https://api.theintrodb.org/v3",
	IntroDB:      "https://api.introdb.app",
	PublicMetaDB: "https://publicmetadb.com",
}

const (
	// foundFor keeps segments found: they rarely move once a few people
	// agreed on them.
	foundFor = 30 * 24 * time.Hour
	// nothingFor asks again a day after a database knew nothing, as
	// people keep adding titles.
	nothingFor = 24 * time.Hour
	// failedFor asks again an hour after a failure or a refusal, such as
	// a rate limit, without hammering a database that is down.
	failedFor = time.Hour
	// askTimeout bounds each database's answer: apps ask for segments
	// when playback starts, and do not wait long.
	askTimeout = 5 * time.Second
	// maxAnswer bounds the bytes read from a database.
	maxAnswer = 1 << 20
	// maxRateLimit bounds how long a database's Retry-After keeps it
	// unasked.
	maxRateLimit = 24 * time.Hour
	// skipRecords is how many PublicMetaDB records of a title are read,
	// the most a page holds.
	skipRecords = 200
)

// Type is a kind of segment, named as Jellyfin names them.
type Type string

// The kinds of segment the databases know.
const (
	Intro   Type = "Intro"
	Recap   Type = "Recap"
	Outro   Type = "Outro"
	Preview Type = "Preview"
)

// Segment is a part of a title that apps offer to skip.
type Segment struct {
	Type       Type
	Start, End time.Duration
}

// Title identifies a movie or an episode to the databases.
type Title struct {
	// Item is the title's identifier in Polyfin, under which the answers
	// are kept.
	Item accounts.ID
	// IMDb ("tt…") and TMDB identify the movie, or the series of an
	// episode; either may be empty.
	IMDb, TMDB string
	// Season and Episode number an episode within its series; both are
	// zero for a movie.
	Season, Episode int
	// Runtime is the title's length when known: it picks the release the
	// segments were timed on, and ends the segments that last until the
	// end.
	Runtime time.Duration
}

func (t Title) movie() bool { return t.Season == 0 && t.Episode == 0 }

// Source is a database and the base URL of its API.
type Source struct {
	Name, URL string
}

// Sources returns the databases named, at their public addresses, in the
// order given. Unknown names are left out.
func Sources(names []string) []Source {
	sources := make([]Source, 0, len(names))
	for _, name := range names {
		if base, ok := publicURLs[name]; ok {
			sources = append(sources, Source{Name: name, URL: base})
		}
	}
	return sources
}

// ErrKeyRefused reports an API key a database refused.
var ErrKeyRefused = errors.New("API key refused")

// Service finds the segments of titles.
type Service struct {
	db *pgxpool.Pool
	// sources are the databases asked, the preferred first.
	sources []Source
	// settings hold the PublicMetaDB key, read at each request so that a
	// key saved, changed or removed applies at once.
	settings  func() accounts.Settings
	client    *http.Client
	userAgent string
	logger    *slog.Logger
	flight    singleflight.Group
	now       func() time.Time

	mu sync.Mutex
	// refusedKey is the last key PublicMetaDB refused: PublicMetaDB is not
	// asked with it again.
	refusedKey string
	// limited holds, by database, when one that answered 429 with a
	// Retry-After may be asked again.
	limited map[string]time.Time
}

// New returns a service asking sources, in order of preference, and
// keeping their answers in db. Without sources it finds nothing.
// PublicMetaDB is asked only while settings hold a key for it.
func New(db *pgxpool.Pool, sources []Source, version string, logger *slog.Logger, settings func() accounts.Settings) *Service {
	return &Service{db: db, sources: sources, settings: settings, client: &http.Client{}, userAgent: "Polyfin/" + version, logger: logger,
		now: time.Now, limited: map[string]time.Time{}}
}

// mark is a segment as a database gave it.
type mark struct {
	Type    Type  `json:"type"`
	StartMS int64 `json:"start_ms"`
	// EndMS is nil for a segment that lasts until the end of the title.
	EndMS *int64 `json:"end_ms,omitempty"`
}

// answer is what a database said about a title, and when to ask again.
type answer struct {
	Marks   []mark    `json:"marks"`
	Expires time.Time `json:"expires"`
}

var (
	imdbPattern = regexp.MustCompile(`^tt[0-9]{7,8}$`)
	tmdbPattern = regexp.MustCompile(`^[1-9][0-9]{0,7}$`)
)

// Segments returns the segments of a title, sorted by start, from what the
// databases answered: for each type, those of the first database in order
// of preference that has any. A database that fails only leaves its
// segments out.
func (s *Service) Segments(ctx context.Context, title Title) []Segment {
	if !imdbPattern.MatchString(title.IMDb) {
		title.IMDb = ""
	}
	if !tmdbPattern.MatchString(title.TMDB) {
		title.TMDB = ""
	}
	key := s.settings().PublicMetaDBKey
	sources := s.used(key)
	if len(sources) == 0 || (title.IMDb == "" && title.TMDB == "") ||
		(!title.movie() && (title.Season < 1 || title.Episode < 1)) {
		return nil
	}
	answers := s.load(ctx, title.Item)
	if stale := s.stale(sources, answers, key); len(stale) > 0 {
		result, _, _ := s.flight.Do(title.Item.String(), func() (any, error) {
			// The answers are kept even once the request that asked is
			// canceled, as when an app stops waiting.
			return s.refresh(context.WithoutCancel(ctx), title, answers, stale, key), nil
		})
		answers = result.(map[string]answer)
	}
	return merge(answers, sources, title.Runtime)
}

// used are the sources whose answers count: all of them, but PublicMetaDB
// while no key for it is saved.
func (s *Service) used(key string) []Source {
	if key != "" {
		return s.sources
	}
	return slices.DeleteFunc(slices.Clone(s.sources), func(source Source) bool { return source.Name == PublicMetaDB })
}

// load reads the answers kept for a title; none when there are none or the
// database cannot be read, so that the sources are asked.
func (s *Service) load(ctx context.Context, item accounts.ID) map[string]answer {
	var data []byte
	err := s.db.QueryRow(ctx, "SELECT answers FROM media_segments WHERE item_id = $1", item).Scan(&data)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Warn("Reading media segments failed", "error", err)
		}
		return map[string]answer{}
	}
	answers := map[string]answer{}
	if err := json.Unmarshal(data, &answers); err != nil {
		return map[string]answer{}
	}
	return answers
}

// stale returns the sources to ask about a title: those never asked, or
// whose answer expired. A database is not asked while the rate limit it set
// lasts, nor PublicMetaDB with a key it refused; as they were not asked,
// they are once they may be.
func (s *Service) stale(sources []Source, answers map[string]answer, key string) []Source {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var stale []Source
	for _, source := range sources {
		if kept, ok := answers[source.Name]; ok && now.Before(kept.Expires) {
			continue
		}
		if now.Before(s.limited[source.Name]) || (source.Name == PublicMetaDB && key == s.refusedKey) {
			continue
		}
		stale = append(stale, source)
	}
	return stale
}

// refresh asks the stale sources, all at once, and keeps their answers with
// the others'. A source that fails keeps the segments it gave before, if
// any, and is asked again after failedFor, or once the rate limit its
// Retry-After sets ends if later. A PublicMetaDB refusing the key leaves
// no answer: the title is asked again with the next key.
func (s *Service) refresh(ctx context.Context, title Title, answers map[string]answer, stale []Source, key string) map[string]answer {
	now := s.now()
	fresh := maps.Clone(answers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, source := range stale {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, askTimeout)
			defer cancel()
			marks, err := s.ask(ctx, source, title, key)
			result := answer{Marks: marks, Expires: now.Add(nothingFor)}
			var limited *rateLimited
			switch {
			case source.Name == PublicMetaDB && errors.Is(err, ErrKeyRefused):
				s.refuse(key)
				return
			case errors.As(err, &limited):
				s.logger.Info("A segment database limits requests", "database", source.Name, "until", limited.until)
				s.limit(source.Name, limited.until)
				result = answer{Marks: answers[source.Name].Marks, Expires: now.Add(failedFor)}
			case err != nil:
				s.logger.Info("A segment database did not answer", "database", source.Name, "error", err)
				result = answer{Marks: answers[source.Name].Marks, Expires: now.Add(failedFor)}
			case len(marks) > 0:
				result.Expires = now.Add(foundFor)
			}
			mu.Lock()
			fresh[source.Name] = result
			mu.Unlock()
		})
	}
	wg.Wait()
	data, err := json.Marshal(fresh)
	if err == nil {
		_, err = s.db.Exec(ctx, `INSERT INTO media_segments (item_id, answers) VALUES ($1, $2)
			ON CONFLICT (item_id) DO UPDATE SET answers = excluded.answers, updated_at = now()`, title.Item, data)
	}
	if err != nil {
		s.logger.Warn("Saving media segments failed", "error", err)
	}
	return fresh
}

// refuse remembers that PublicMetaDB refused key, which it is not asked
// with again, and logs it once per key rather than once per title.
func (s *Service) refuse(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refusedKey == key {
		return
	}
	s.refusedKey = key
	s.logger.Warn("PublicMetaDB refused the API key: it is not asked for segments until another key is saved in the settings")
}

// limit leaves a database unasked until until.
func (s *Service) limit(name string, until time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if until.After(s.limited[name]) {
		s.limited[name] = until
	}
}

// merge picks, for each type, the segments of the first source that has
// any, and places them in the title. Segments lasting until the end of a
// title of unknown runtime are left out, as their end is unknown; the next
// source may then have that type.
func merge(answers map[string]answer, sources []Source, runtime time.Duration) []Segment {
	var segments []Segment
	for _, kind := range []Type{Recap, Intro, Outro, Preview} {
		for _, source := range sources {
			before := len(segments)
			for _, m := range answers[source.Name].Marks {
				if m.Type != kind {
					continue
				}
				segment := Segment{Type: kind, Start: time.Duration(m.StartMS) * time.Millisecond, End: runtime}
				if m.EndMS != nil {
					segment.End = time.Duration(*m.EndMS) * time.Millisecond
				}
				if segment.End > segment.Start {
					segments = append(segments, segment)
				}
			}
			if len(segments) > before {
				break
			}
		}
	}
	slices.SortStableFunc(segments, func(a, b Segment) int { return cmp.Compare(a.Start, b.Start) })
	return segments
}

// ask asks a source for a title's segments: none when it knows nothing of
// the title.
func (s *Service) ask(ctx context.Context, source Source, title Title, key string) ([]mark, error) {
	switch source.Name {
	case TheIntroDB:
		return s.askTheIntroDB(ctx, source.URL, title)
	case IntroDB:
		return s.askIntroDB(ctx, source.URL, title)
	case PublicMetaDB:
		return s.askPublicMetaDB(ctx, source.URL, title, key)
	default:
		return nil, nil
	}
}

// span is a segment as TheIntroDB gives it, in milliseconds. A missing
// start is the beginning of the title; a missing end, its end.
type span struct {
	StartMS *int64 `json:"start_ms"`
	EndMS   *int64 `json:"end_ms"`
}

// askTheIntroDB asks TheIntroDB, by TMDB identifier when known as it
// prefers, else by IMDb identifier.
func (s *Service) askTheIntroDB(ctx context.Context, base string, title Title) ([]mark, error) {
	query := url.Values{}
	if title.TMDB != "" {
		query.Set("tmdb_id", title.TMDB)
	} else {
		query.Set("imdb_id", title.IMDb)
	}
	if !title.movie() {
		query.Set("season", strconv.Itoa(title.Season))
		query.Set("episode", strconv.Itoa(title.Episode))
	}
	if title.Runtime > 0 {
		query.Set("duration_ms", strconv.FormatInt(title.Runtime.Milliseconds(), 10))
	}
	var body struct {
		Intro   []span `json:"intro"`
		Recap   []span `json:"recap"`
		Credits []span `json:"credits"`
		Preview []span `json:"preview"`
	}
	if found, err := s.get(ctx, base+"/media?"+query.Encode(), "", &body); err != nil || !found {
		return nil, err
	}
	var marks []mark
	// TheIntroDB marks "no segment" with an intro or recap ending at 0,
	// or credits or a preview starting at 0.
	for kind, spans := range map[Type][]span{Intro: body.Intro, Recap: body.Recap} {
		for _, sp := range spans {
			if sp.EndMS != nil && *sp.EndMS > 0 {
				marks = append(marks, mark{Type: kind, StartMS: deref(sp.StartMS), EndMS: sp.EndMS})
			}
		}
	}
	for kind, spans := range map[Type][]span{Outro: body.Credits, Preview: body.Preview} {
		for _, sp := range spans {
			if sp.StartMS != nil && *sp.StartMS > 0 {
				marks = append(marks, mark{Type: kind, StartMS: *sp.StartMS, EndMS: sp.EndMS})
			}
		}
	}
	return marks, nil
}

func deref(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

// askIntroDB asks IntroDB, which knows titles by IMDb identifier only. Its
// post-credits scenes are left out: Jellyfin has no such type, and they are
// what a viewer stays for.
func (s *Service) askIntroDB(ctx context.Context, base string, title Title) ([]mark, error) {
	if title.IMDb == "" {
		return nil, nil
	}
	query := url.Values{"imdb_id": {title.IMDb}}
	if title.movie() {
		query.Set("is_movie", "true")
	} else {
		query.Set("season", strconv.Itoa(title.Season))
		query.Set("episode", strconv.Itoa(title.Episode))
	}
	type segment struct {
		StartMS int64 `json:"start_ms"`
		EndMS   int64 `json:"end_ms"`
	}
	// IntroDB answers null for the segments it does not know.
	var body struct {
		Intro *segment `json:"intro"`
		Recap *segment `json:"recap"`
		Outro *segment `json:"outro"`
	}
	if found, err := s.get(ctx, base+"/segments?"+query.Encode(), "", &body); err != nil || !found {
		return nil, err
	}
	var marks []mark
	for kind, sg := range map[Type]*segment{Intro: body.Intro, Recap: body.Recap, Outro: body.Outro} {
		if sg != nil && sg.EndMS > sg.StartMS {
			marks = append(marks, mark{Type: kind, StartMS: sg.StartMS, EndMS: new(sg.EndMS)})
		}
	}
	return marks, nil
}

// skipRecord is a contributor's skip timestamps for a title on PublicMetaDB,
// timed on a streaming or a physical release, in milliseconds. A segment
// not submitted has a null start and end.
type skipRecord struct {
	ID             string   `json:"id"`
	Source         string   `json:"source"`
	Created        string   `json:"created"`
	IntroStartMS   *float64 `json:"intro_start_ms"`
	IntroEndMS     *float64 `json:"intro_end_ms"`
	RecapStartMS   *float64 `json:"recap_start_ms"`
	RecapEndMS     *float64 `json:"recap_end_ms"`
	CreditsStartMS *float64 `json:"credits_start_ms"`
	CreditsEndMS   *float64 `json:"credits_end_ms"`
	PreviewStartMS *float64 `json:"preview_start_ms"`
	PreviewEndMS   *float64 `json:"preview_end_ms"`
}

// marks are the segments a record gives: those with a start and an end
// after it. Its credits are the outro.
func (r skipRecord) marks() []mark {
	var marks []mark
	for _, sp := range []struct {
		kind       Type
		start, end *float64
	}{
		{Intro, r.IntroStartMS, r.IntroEndMS},
		{Recap, r.RecapStartMS, r.RecapEndMS},
		{Outro, r.CreditsStartMS, r.CreditsEndMS},
		{Preview, r.PreviewStartMS, r.PreviewEndMS},
	} {
		if sp.start == nil || sp.end == nil {
			continue
		}
		start, end := int64(math.Round(*sp.start)), int64(math.Round(*sp.end))
		if start >= 0 && end > start {
			marks = append(marks, mark{Type: sp.kind, StartMS: start, EndMS: &end})
		}
	}
	return marks
}

// askPublicMetaDB asks PublicMetaDB, which knows titles by TMDB identifier
// only, with the server's key.
func (s *Service) askPublicMetaDB(ctx context.Context, base string, title Title, key string) ([]mark, error) {
	if title.TMDB == "" {
		return nil, nil
	}
	query := url.Values{"tmdb_id": {title.TMDB}, "media_type": {"movie"}, "perPage": {strconv.Itoa(skipRecords)}}
	if !title.movie() {
		query.Set("media_type", "tv")
		query.Set("season", strconv.Itoa(title.Season))
		query.Set("episode", strconv.Itoa(title.Episode))
	}
	var body struct {
		Items []skipRecord `json:"items"`
	}
	if found, err := s.get(ctx, base+"/api/external/skips?"+query.Encode(), key, &body); err != nil || !found {
		return nil, err
	}
	return pickSkips(body.Items), nil
}

// pickSkips returns the segments of the one record a title's segments are
// taken from. PublicMetaDB keeps a record per contributor and release kind,
// a contributor posting again updating theirs, and lists them all without
// their votes. Records are not mixed, as segments timed on different
// releases do not line up. Among the records giving segments, preferred
// are in turn: one timed on a streaming release, as the versions addons
// offer mostly are; one giving more segments; the latest created; and the
// smallest identifier, so that an answer gives the same segments whatever
// the order of its records.
func pickSkips(records []skipRecord) []mark {
	type candidate struct {
		marks     []mark
		streaming bool
		created   time.Time
		id        string
	}
	var best *candidate
	for _, record := range records {
		c := candidate{marks: record.marks(), streaming: record.Source == "streaming", id: record.ID}
		if len(c.marks) == 0 {
			continue
		}
		// An unreadable date counts as the oldest.
		c.created, _ = time.Parse(time.RFC3339, record.Created)
		if best == nil || cmp.Or(
			-compareBool(c.streaming, best.streaming),
			-cmp.Compare(len(c.marks), len(best.marks)),
			-c.created.Compare(best.created),
			cmp.Compare(c.id, best.id),
		) < 0 {
			best = &c
		}
	}
	if best == nil {
		return nil
	}
	return best.marks
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}

// CheckPublicMetaDBKey asks PublicMetaDB one question with key, the skip
// timestamps of a movie: ErrKeyRefused when it refuses the key, another
// error when it could not be asked. A key it accepts is asked with again,
// even one it refused before.
func (s *Service) CheckPublicMetaDBKey(ctx context.Context, key string) error {
	base := publicURLs[PublicMetaDB]
	for _, source := range s.sources {
		if source.Name == PublicMetaDB {
			base = source.URL
		}
	}
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	query := url.Values{"tmdb_id": {"550"}, "media_type": {"movie"}, "perPage": {"1"}}
	var body struct {
		Items []json.RawMessage `json:"items"`
	}
	if _, err := s.get(ctx, base+"/api/external/skips?"+query.Encode(), key, &body); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refusedKey == key {
		s.refusedKey = ""
	}
	return nil
}

// rateLimited reports a database that answered 429 with a Retry-After, and
// when it may be asked again.
type rateLimited struct {
	until time.Time
}

func (e *rateLimited) Error() string {
	return "HTTP 429 until " + e.until.Format(time.RFC3339)
}

// retryAfter reads a Retry-After header, in seconds or as a date, into
// when a database may be asked again, at most maxRateLimit after now.
func retryAfter(value string, now time.Time) (time.Time, bool) {
	var until time.Time
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		until = now.Add(time.Duration(seconds) * time.Second)
	} else if date, err := http.ParseTime(value); err == nil {
		until = date
	} else {
		return time.Time{}, false
	}
	if latest := now.Add(maxRateLimit); until.After(latest) {
		until = latest
	}
	return until, true
}

// get reads a database's JSON answer into body, sending key, if any, as a
// bearer token. found is false when the database answered that it knows
// nothing of the title. A refused key is ErrKeyRefused, and a 429 with a
// Retry-After a *rateLimited.
func (s *Service) get(ctx context.Context, target, key string, body any) (found bool, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("User-Agent", s.userAgent)
	request.Header.Set("Accept", "application/json")
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch status := response.StatusCode; {
	case status == http.StatusNotFound:
		return false, nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return false, fmt.Errorf("HTTP %d: %w", status, ErrKeyRefused)
	case status == http.StatusTooManyRequests:
		if until, ok := retryAfter(response.Header.Get("Retry-After"), s.now()); ok {
			return false, &rateLimited{until: until}
		}
		return false, fmt.Errorf("HTTP %d", status)
	case status != http.StatusOK:
		return false, fmt.Errorf("HTTP %d", status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(body); err != nil {
		return false, fmt.Errorf("read the answer: %w", err)
	}
	return true, nil
}

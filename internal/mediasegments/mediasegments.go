// Package mediasegments finds where the intro, recap, credits and preview of
// a movie or an episode are, for the skip buttons of Jellyfin apps. It asks
// community databases, TheIntroDB and IntroDB, by the title's IMDb or TMDB
// identifier, and keeps their answers in the database.
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
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The databases, by the names POLYFIN_SEGMENTS gives them.
const (
	TheIntroDB = "theintrodb"
	IntroDB    = "introdb"
)

// publicURLs are the base URLs of the databases' APIs.
var publicURLs = map[string]string{
	TheIntroDB: "https://api.theintrodb.org/v3",
	IntroDB:    "https://api.introdb.app",
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

// Service finds the segments of titles.
type Service struct {
	db *pgxpool.Pool
	// sources are the databases asked, the preferred first.
	sources   []Source
	client    *http.Client
	userAgent string
	logger    *slog.Logger
	flight    singleflight.Group
	now       func() time.Time
}

// New returns a service asking sources, in order of preference, and
// keeping their answers in db. Without sources it finds nothing.
func New(db *pgxpool.Pool, sources []Source, version string, logger *slog.Logger) *Service {
	return &Service{db: db, sources: sources, client: &http.Client{}, userAgent: "Polyfin/" + version, logger: logger, now: time.Now}
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
	if len(s.sources) == 0 || (title.IMDb == "" && title.TMDB == "") ||
		(!title.movie() && (title.Season < 1 || title.Episode < 1)) {
		return nil
	}
	answers := s.load(ctx, title.Item)
	if s.stale(answers) {
		result, _, _ := s.flight.Do(title.Item.String(), func() (any, error) {
			// The answers are kept even once the request that asked is
			// canceled, as when an app stops waiting.
			return s.refresh(context.WithoutCancel(ctx), title, answers), nil
		})
		answers = result.(map[string]answer)
	}
	return s.merge(answers, title.Runtime)
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

// stale reports whether a source must be asked: it never was, or its
// answer expired.
func (s *Service) stale(answers map[string]answer) bool {
	now := s.now()
	return slices.ContainsFunc(s.sources, func(source Source) bool {
		kept, ok := answers[source.Name]
		return !ok || !now.Before(kept.Expires)
	})
}

// refresh asks the sources whose answers are stale, all at once, and keeps
// their answers with the others'. A source that fails keeps the segments it
// gave before, if any, and is asked again after failedFor.
func (s *Service) refresh(ctx context.Context, title Title, answers map[string]answer) map[string]answer {
	now := s.now()
	fresh := maps.Clone(answers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, source := range s.sources {
		if kept, ok := answers[source.Name]; ok && now.Before(kept.Expires) {
			continue
		}
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, askTimeout)
			defer cancel()
			marks, err := s.ask(ctx, source, title)
			result := answer{Marks: marks, Expires: now.Add(nothingFor)}
			switch {
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

// merge picks, for each type, the segments of the first source that has
// any, and places them in the title. Segments lasting until the end of a
// title of unknown runtime are left out, as their end is unknown; the next
// source may then have that type.
func (s *Service) merge(answers map[string]answer, runtime time.Duration) []Segment {
	var segments []Segment
	for _, kind := range []Type{Recap, Intro, Outro, Preview} {
		for _, source := range s.sources {
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
func (s *Service) ask(ctx context.Context, source Source, title Title) ([]mark, error) {
	switch source.Name {
	case TheIntroDB:
		return s.askTheIntroDB(ctx, source.URL, title)
	case IntroDB:
		return s.askIntroDB(ctx, source.URL, title)
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
	if found, err := s.get(ctx, base+"/media?"+query.Encode(), &body); err != nil || !found {
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
	if found, err := s.get(ctx, base+"/segments?"+query.Encode(), &body); err != nil || !found {
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

// get reads a database's JSON answer into body. found is false when the
// database answered that it knows nothing of the title.
func (s *Service) get(ctx context.Context, target string, body any) (found bool, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("User-Agent", s.userAgent)
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound:
		return false, nil
	case response.StatusCode != http.StatusOK:
		return false, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(body); err != nil {
		return false, fmt.Errorf("read the answer: %w", err)
	}
	return true, nil
}

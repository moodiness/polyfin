package library

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Items' lengths.
//
// A movie's or an episode's length, which apps show with the time it would
// end if played now (Jellyfin's RunTimeTicks), comes from the first of
// these that gives one:
//   - its description: a movie's runtime ("112 min", "1h52min"), each of a
//     series' episodes' own;
//   - TMDB, while the settings hold the server's key. A movie's runtime
//     comes with its credits and details (see titlecredits.go), with no
//     request of its own. An episode's comes with its series' seasons:
//     when a series' or a season's page opens and some of its aired
//     episodes have none, the seasons TMDB lists are asked once, in the
//     queue of TMDB's lookups (see AwaitEpisodeLengths), and the lengths
//     kept for episodeLengthsFresh, by the addon's season and number, as
//     the series' numbering pairs them with TMDB's (see
//     EpisodeNumbering.ToTMDB). An episode it pairs with none, a special
//     among them, is left without one. No pass reads them: a series never
//     opened asks nothing;
//   - for a title a local folder holds, how long its file plays, as the
//     folder's scan probed it (see TitleDurations and EpisodeDurations).
//
// A version's analysis tells how long that version plays, which its media
// source gives and players use as they play it, but never the title's: a
// file of another cut, or a sample, would change it, and from one user to
// the next. The lengths are the server's, whoever opens the title. A
// series shows none of its own, as jellyfin-web leaves a series' runtime
// out.
//
// What it costs: a movie, nothing but its credits. A series whose aired
// episodes all have a length, nothing. Another, once every 30 days at
// most, while one of its pages opens: TMDB's find, for a series known by
// its IMDb identifier only that TMDB's numbering of its episodes did not
// learn, the series' details, which list its seasons, unless they were
// read lately, and one request for up to 20 seasons and 400 episodes (see
// tmdb.Client.SeasonEpisodes).

const (
	// episodeLengthsFresh is how long the lengths TMDB gives a series'
	// episodes, or TMDB not knowing the series, are kept before TMDB is
	// asked again.
	episodeLengthsFresh = 30 * 24 * time.Hour
	// keptLengthsMost bounds the series whose kept lengths
	// episodeLengthsMemory remembers.
	keptLengthsMost = 2048
)

// keptEpisodeLengths is what is kept of the lengths TMDB gives a series'
// episodes, by the addon's season and number, and when TMDB was asked.
type keptEpisodeLengths struct {
	lengths map[episodePlace]time.Duration
	asked   time.Time
}

// episodeLengthsMemory remembers the lengths kept of the episodes of the
// series listed lately, and the series of none: each listing of a series'
// episodes missing one reads them, while they change only when TMDB is
// asked. Only this file writes them, and each write remembers what it
// wrote: a read that began before a write remembers nothing.
type episodeLengthsMemory struct {
	mu sync.Mutex
	// known holds the lengths kept by series, nil for a series of none;
	// it is made on first use.
	known *cache.Cache[string, *keptEpisodeLengths]
	// writes counts the writes.
	writes uint64
}

// localLengths gives the movies of titles that have no length yet the
// one their file plays, when a local folder holds it.
func (s *Service) localLengths(ctx context.Context, titles []*Item) {
	if s.local == nil {
		return
	}
	var ids []string
	for _, title := range titles {
		if title.Kind == KindMovie && title.Runtime == 0 && title.StremioID != "" {
			ids = append(ids, title.StremioID)
		}
	}
	if len(ids) == 0 {
		return
	}
	durations, err := s.local.TitleDurations(ctx, ids)
	if err != nil {
		s.logger.Debug("The lengths of local movies could not be read", "error", err)
		return
	}
	for _, title := range titles {
		if title.Kind == KindMovie && title.Runtime == 0 {
			title.Runtime = durations[title.StremioID]
		}
	}
}

// episodeLengthsOf gives the episodes of the series of the Stremio
// identifier series that have no length yet the one TMDB gives, as kept,
// else the one their file plays, when a local folder holds it. The
// episodes are numbered as the addon numbers them.
func (s *Service) episodeLengthsOf(ctx context.Context, series string, episodes []Item) {
	missing := func() bool {
		for _, episode := range episodes {
			if episode.Runtime == 0 {
				return true
			}
		}
		return false
	}
	if !missing() {
		return
	}
	if s.lookups.Enabled() {
		kept, err := s.keptEpisodeLengths(ctx, series)
		if err != nil {
			s.logger.Debug("The lengths of a series' episodes could not be read", "series", series, "error", err)
		} else if kept != nil {
			for i, episode := range episodes {
				if episode.Runtime == 0 {
					episodes[i].Runtime = kept.lengths[episodePlace{episode.ParentIndexNumber, episode.IndexNumber}]
				}
			}
		}
	}
	if s.local == nil || !missing() {
		return
	}
	durations, err := s.local.EpisodeDurations(ctx, series)
	if err != nil {
		s.logger.Debug("The lengths of local episodes could not be read", "series", series, "error", err)
		return
	}
	for i, episode := range episodes {
		if episode.Runtime == 0 {
			episodes[i].Runtime = durations[[2]int{episode.ParentIndexNumber, episode.IndexNumber}]
		}
	}
}

// AwaitEpisodeLengths has the lengths of a series' episodes read on TMDB
// as a page of the series, or one of its seasons (item), opens for user,
// when some of its aired episodes have none and the lengths kept, if any,
// are old, waiting s.creditsWait at most, as for a title's credits (see
// AwaitTitleCredits): the episodes listed next show them.
func (s *Service) AwaitEpisodeLengths(ctx context.Context, user accounts.User, item Item) {
	if !s.lookups.Enabled() {
		return
	}
	seriesID := item.ID
	switch item.Kind {
	case KindSeries:
	case KindSeason:
		seriesID = item.SeriesID
	default:
		return
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return
	}
	_, _, meta, err := s.series(ctx, v, seriesID)
	if err != nil {
		return
	}
	now := s.now()
	places := make([]episodePlace, 0, len(meta.Videos))
	missing := false
	for _, video := range meta.Videos {
		places = append(places, episodePlace{int(video.Season), video.EpisodeNumber()})
		missing = missing || released(video, now) && parseRuntime(string(video.Runtime)) == 0
	}
	if !missing {
		return
	}
	kept, err := s.keptEpisodeLengths(ctx, meta.ID)
	if err != nil || kept != nil && now.Sub(kept.asked) < episodeLengthsFresh {
		return
	}
	lookup := tmdb.Lookup{Key: "episode lengths|" + meta.ID, Run: func(ctx context.Context) error {
		return s.lookUpEpisodeLengths(ctx, seriesID, places)
	}}
	s.lookups.Add([]tmdb.Lookup{lookup}, true)
	s.lookups.Wait(ctx, []string{lookup.Key}, s.creditsWait)
}

// lookUpEpisodeLengths reads on TMDB the lengths of the episodes of the
// series Polyfin records as seriesID, the addon numbering them at places,
// and keeps them. A series TMDB does not know is kept as such; a lookup
// that fails keeps nothing, and fails with tmdb.RateLimited when TMDB
// refused it for its rate.
func (s *Service) lookUpEpisodeLengths(ctx context.Context, seriesID accounts.ID, places []episodePlace) error {
	if !s.tmdb.Enabled() {
		return nil
	}
	r, err := s.load(ctx, seriesID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.Meta == nil || r.Kind != KindSeries && !r.shownTitle() {
		return nil
	}
	series := r.Meta.ID
	ids := s.providerIDs(r.ID, *r.Meta)
	tmdbID, imdb := describedNumber(stremio.Text(ids["Tmdb"])), cmp.Or(r.Meta.ImdbID, series)
	known, err := s.knownEpisodes(ctx, []string{series})
	if err != nil {
		return err
	}
	// Numbered as TMDB's numbering of the series' episodes pairs them,
	// which TMDB's seasons may tell.
	numbering, err := s.numbering(ctx, s.serverView(ctx), series, tmdbID, imdb, known[series])
	if err != nil {
		return err
	}
	if tmdbID == 0 {
		if tmdbID, err = s.seriesTMDB(ctx, series, imdb); err != nil {
			return err
		}
	}
	lengths := map[string]int{}
	var found *int
	if tmdbID > 0 {
		seasons, err := s.tmdb.SeriesSeasons(ctx, tmdbID)
		if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
			return err
		}
		if err == nil {
			episodes, err := s.tmdb.SeasonEpisodes(ctx, tmdbID, seasons)
			if err != nil {
				return err
			}
			aired := make(map[episodePlace]int, len(episodes))
			for _, e := range episodes {
				aired[episodePlace{e.Season, e.Episode}] = e.Runtime
			}
			for _, p := range places {
				if season, number, ok := numbering.ToTMDB(p.season, p.number); ok && aired[episodePlace{season, number}] > 0 {
					lengths[placeKey(p)] = aired[episodePlace{season, number}]
				}
			}
			found = &tmdbID
		}
	}
	return s.keepEpisodeLengths(ctx, series, found, lengths)
}

// seriesTMDB returns the TMDB identifier of the series of the Stremio
// identifier series, whose IMDb identifier is imdb, when its description
// gives none: its own, of a "tmdb:" identifier, else the one its
// numbering, credits or TMDB's find learnt; 0 when TMDB does not know it.
func (s *Service) seriesTMDB(ctx context.Context, series, imdb string) (int, error) {
	if id, ok := strings.CutPrefix(series, "tmdb:"); ok {
		return describedNumber(stremio.Text(id)), nil
	}
	aligned, err := s.keptAlignment(ctx, series)
	if err != nil {
		return 0, err
	}
	if aligned != nil && aligned.tmdb > 0 {
		return aligned.tmdb, nil
	}
	if !imdbID.MatchString(imdb) {
		return 0, nil
	}
	id := s.learntTMDB(ctx, creditsKey{kind: tmdb.TV, provider: "imdb", id: imdb})
	if id == "" {
		if id, err = s.tmdb.FindByIMDb(ctx, tmdb.TV, imdb); errors.Is(err, tmdb.ErrNotFound) {
			return 0, nil
		} else if err != nil {
			return 0, err
		}
	}
	return describedNumber(stremio.Text(id)), nil
}

// placeKey is how an episode's place is kept: "2:8".
func placeKey(p episodePlace) string {
	return strconv.Itoa(p.season) + ":" + strconv.Itoa(p.number)
}

// keepEpisodeLengths keeps the lengths in minutes, by placeKey, TMDB
// gives the episodes of a series, whose TMDB identifier is tmdbID, nil
// for a series TMDB does not know, and remembers them.
func (s *Service) keepEpisodeLengths(ctx context.Context, series string, tmdbID *int, lengths map[string]int) error {
	raw, err := json.Marshal(lengths)
	if err != nil {
		return err
	}
	now := s.now()
	m := &s.episodeLengths
	_, err = s.db.Exec(ctx, `INSERT INTO tmdb_episode_lengths (series, tmdb_id, lengths, asked_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (series) DO UPDATE SET tmdb_id = excluded.tmdb_id, lengths = excluded.lengths, asked_at = excluded.asked_at`,
		series, tmdbID, raw, now)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writes++
	if m.known == nil {
		m.known = cache.New[string, *keptEpisodeLengths](keptLengthsMost, math.MaxInt64)
	}
	if err != nil {
		m.known.Delete(series)
		return err
	}
	// Remembered as a read gives it: the database keeps microseconds.
	m.known.Put(series, &keptEpisodeLengths{lengths: readLengths(lengths), asked: now.Truncate(time.Microsecond)})
	return nil
}

// keptEpisodeLengths returns the lengths kept of the episodes of a
// series, nil when none are: as remembered, else read from the database,
// then remembered.
func (s *Service) keptEpisodeLengths(ctx context.Context, series string) (*keptEpisodeLengths, error) {
	m := &s.episodeLengths
	m.mu.Lock()
	if m.known == nil {
		m.known = cache.New[string, *keptEpisodeLengths](keptLengthsMost, math.MaxInt64)
	}
	if k, ok := m.known.Get(series); ok {
		m.mu.Unlock()
		return k, nil
	}
	writes := m.writes
	m.mu.Unlock()
	var raw []byte
	var asked time.Time
	var k *keptEpisodeLengths
	err := s.db.QueryRow(ctx, "SELECT lengths, asked_at FROM tmdb_episode_lengths WHERE series = $1", series).Scan(&raw, &asked)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, err
	default:
		var lengths map[string]int
		if err := json.Unmarshal(raw, &lengths); err != nil {
			return nil, err
		}
		k = &keptEpisodeLengths{lengths: readLengths(lengths), asked: asked}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if writes == m.writes {
		m.known.Put(series, k)
	}
	return k, nil
}

// readLengths reads the lengths kept in minutes by placeKey.
func readLengths(kept map[string]int) map[episodePlace]time.Duration {
	lengths := make(map[episodePlace]time.Duration, len(kept))
	for key, minutes := range kept {
		season, number, _ := strings.Cut(key, ":")
		s, errSeason := strconv.Atoi(season)
		n, errNumber := strconv.Atoi(number)
		if errSeason == nil && errNumber == nil && minutes > 0 {
			lengths[episodePlace{s, n}] = time.Duration(minutes) * time.Minute
		}
	}
	return lengths
}

// forgetEpisodeLengths has the lengths kept of a series' episodes read
// again on TMDB the next time one of its pages opens, as a refresh of the
// series asks: they show until the new ones come.
func (s *Service) forgetEpisodeLengths(ctx context.Context, series string) error {
	_, err := s.db.Exec(ctx, "UPDATE tmdb_episode_lengths SET asked_at = 'epoch' WHERE series = $1", series)
	m := &s.episodeLengths
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writes++
	if m.known != nil {
		m.known.Delete(series)
	}
	return err
}

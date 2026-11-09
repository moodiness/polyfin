package library

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Episode numberings.
//
// Tracking services number a series' episodes as TMDB numbers them, as
// aired, and the addon Polyfin lists them from may number them otherwise:
// an episode one of them leaves out, or counts among the specials, moves
// the others along. The same "tt…:2:8" names TMDB's episode on one server
// and another episode on the next, whose addon follows another source.
//
// An EpisodeNumbering pairs the two, for the imports, which name episodes
// as the service numbers them (TitleRef.TMDBNumbered), and for what is
// sent to the services (see TMDBNumbering). TMDB's numbering comes from,
// in order:
//   - the addon's own video identifiers, when they are TMDB's
//     ("tmdb:1399:1:2"): exact, and asked nothing;
//   - the description of the series a server addon gives as "tmdb:<id>",
//     numbered as TMDB numbers it, kept as other descriptions are;
//   - TMDB, with the server's key: the series' seasons, then, unless they
//     agree with the addon's, their episodes in one request (see
//     tmdb.Client.SeasonEpisodes).
//
// The episodes are paired by the day they aired, as an addon may date an
// evening episode by the next day in UTC: within a day of each other.
// First, the episodes of TMDB aired the same day as the same episodes of
// the addon, as many on both sides, such as a streaming service's part
// released whole on one day or a double episode, pair in order, whatever
// their names. Then one of the addon's within a day, with the same name,
// the specials included; then the only one within a day. A series whose
// regular seasons hold as many episodes on both sides, aired the same
// days, keeps its numbers. In a series numbered otherwise, an episode
// nothing pairs surely has no episode on the other side: none is guessed
// for it, as its numbers may name another episode there. Without any of
// these sources, every episode keeps its numbers.
//
// The pairs are kept by series (episode_alignments), for alignmentFresh,
// or until the addon gives another count of episodes.

// alignmentFresh is how long the pairs of a series' episodes are kept
// before they are made again.
const alignmentFresh = 7 * 24 * time.Hour

// EpisodeNumbering pairs the episodes of a series as its addon numbers
// them with TMDB's numbering of them. Its zero value numbers them alike,
// for want of a source (see Sourced).
type EpisodeNumbering struct {
	// toAddon and toTMDB hold the pairs, each way, of a series numbered
	// otherwise; both are nil for one numbered alike.
	toAddon, toTMDB map[episodePlace]episodePlace
	// sourced tells that one of TMDB's numbering's sources told it.
	sourced bool
}

// numberingOf makes the numbering a source told of the pairs (see
// alignmentRow.places): nil for a series numbered alike.
func numberingOf(places [][4]int) EpisodeNumbering {
	if places == nil {
		return EpisodeNumbering{sourced: true}
	}
	n := EpisodeNumbering{toAddon: make(map[episodePlace]episodePlace, len(places)), toTMDB: make(map[episodePlace]episodePlace, len(places)),
		sourced: true}
	for _, p := range places {
		at, as := episodePlace{p[0], p[1]}, episodePlace{p[2], p[3]}
		n.toAddon[at], n.toTMDB[as] = as, at
	}
	return n
}

// Sourced reports whether one of the sources of TMDB's numbering told
// it (see the package's comment on episode numberings), a series Polyfin
// names by its TMDB identifier included: false for a numbering taken
// alike for want of one, such as a series of an addon that gives no
// TMDB identifiers, on a server without a key for TMDB.
func (n EpisodeNumbering) Sourced() bool { return n.sourced }

// FromTMDB returns the season and number the addon gives the episode
// TMDB numbers so, and false when none of the addon's is surely it.
func (n EpisodeNumbering) FromTMDB(season, number int) (int, int, bool) {
	return across(n.toAddon, season, number)
}

// ToTMDB returns the season and number TMDB gives the episode the addon
// numbers so, and false when none of TMDB's is surely it.
func (n EpisodeNumbering) ToTMDB(season, number int) (int, int, bool) {
	return across(n.toTMDB, season, number)
}

// across numbers an episode on the other side: as pairs tells, and none
// when it pairs it with none; its own numbers when pairs is nil.
func across(pairs map[episodePlace]episodePlace, season, number int) (int, int, bool) {
	if pairs == nil {
		return season, number, true
	}
	other, ok := pairs[episodePlace{season, number}]
	return other.season, other.number, ok
}

// airedEpisode is an episode as one side numbers it: its place, the day
// it aired, zero when unknown, and its name, normalized (see
// airedName).
type airedEpisode struct {
	place episodePlace
	day   time.Time
	name  string
}

// episodeDay reads the day an episode aired, in UTC: from a date and time
// ("2006-01-09T06:00:00.000Z"), as addons give it, or a date
// ("2006-01-08"), as TMDB does; zero when there is none.
func episodeDay(text string) time.Time {
	text = strings.TrimSpace(text)
	for _, layout := range [...]string{time.RFC3339Nano, time.DateOnly} {
		if t, err := time.Parse(layout, text); err == nil {
			y, m, d := t.UTC().Date()
			return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		}
	}
	return time.Time{}
}

// airedName normalizes an episode's name for comparison: its letters
// and digits, in lower case.
func airedName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// videoEpisodes are the episodes of videos, each place once, at the
// places tmdbNumbers gives them when tmdb, as an addon numbering as TMDB
// does names them, else as the addon numbers them.
func videoEpisodes(videos []stremio.Video, tmdb bool) []airedEpisode {
	episodes := make([]airedEpisode, 0, len(videos))
	for _, video := range videos {
		place := episodePlace{int(video.Season), video.EpisodeNumber()}
		if tmdb {
			numbers := tmdbNumbers(video)
			place = episodePlace{numbers[0], numbers[1]}
		}
		if place.season < 0 || place.number <= 0 || slices.ContainsFunc(episodes, func(e airedEpisode) bool { return e.place == place }) {
			continue
		}
		episodes = append(episodes, airedEpisode{place: place, day: episodeDay(video.Released), name: airedName(video.DisplayName())})
	}
	return episodes
}

// listedEpisodes are the episodes Polyfin listed, as videoEpisodes are.
func listedEpisodes(listed episodeList) []airedEpisode {
	episodes := make([]airedEpisode, 0, len(listed))
	for _, e := range listed {
		place := episodePlace{e.season, e.number}
		if place.season < 0 || place.number <= 0 || slices.ContainsFunc(episodes, func(a airedEpisode) bool { return a.place == place }) {
			continue
		}
		episodes = append(episodes, airedEpisode{place: place, day: e.day, name: e.name})
	}
	return episodes
}

// tmdbVideoPlaces pairs the episodes Polyfin listed whose video
// identifiers are TMDB's ("tmdb:1399:1:2") with the places they name, as
// alignEpisodes does, and reports whether any is.
func tmdbVideoPlaces(listed episodeList) ([][4]int, bool) {
	places := [][4]int{}
	named, differ := false, false
	for _, e := range listed {
		if !strings.HasPrefix(e.videoID, "tmdb:") {
			continue
		}
		numbers := tmdbNumbers(stremio.Video{ID: e.videoID, Season: stremio.Number(e.season), Episode: stremio.Number(e.number)})
		named = true
		differ = differ || numbers != [2]int{e.season, e.number}
		places = append(places, [4]int{numbers[0], numbers[1], e.season, e.number})
	}
	if !differ {
		return nil, named
	}
	return places, named
}

// nearDays reports whether two episodes aired within a day of each other.
func nearDays(a, b airedEpisode) bool {
	if a.day.IsZero() || b.day.IsZero() {
		return false
	}
	return max(a.day.Sub(b.day), b.day.Sub(a.day)) <= 24*time.Hour
}

// regularCounts counts the regular episodes of each season.
func regularCounts(episodes []airedEpisode) map[int]int {
	counts := map[int]int{}
	for _, e := range episodes {
		if e.place.season > 0 {
			counts[e.place.season]++
		}
	}
	return counts
}

// sameCounts reports whether two counts of episodes by season are equal.
func sameCounts(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for season, count := range a {
		if b[season] != count {
			return false
		}
	}
	return true
}

// episodesAgree reports whether the regular seasons of the addon and of
// TMDB hold as many episodes, each aired within a day of its namesake
// when both tell the day.
func episodesAgree(addon, aired []airedEpisode) bool {
	if !sameCounts(regularCounts(addon), regularCounts(aired)) {
		return false
	}
	for _, t := range aired {
		if t.place.season <= 0 {
			continue
		}
		i := slices.IndexFunc(addon, func(a airedEpisode) bool { return a.place == t.place })
		if i < 0 || !addon[i].day.IsZero() && !t.day.IsZero() && !nearDays(addon[i], t) {
			return false
		}
	}
	return true
}

// seasonsAgree reports whether the regular seasons of the addon and those
// TMDB lists hold as many episodes, each season's first aired within a day
// of the addon's when both tell the day.
func seasonsAgree(addon []airedEpisode, seasons []tmdb.SeasonSummary) bool {
	counts := map[int]int{}
	for _, s := range seasons {
		if s.Season > 0 && s.Episodes > 0 {
			counts[s.Season] = s.Episodes
		}
	}
	if !sameCounts(regularCounts(addon), counts) {
		return false
	}
	for _, s := range seasons {
		first := airedEpisode{day: episodeDay(s.AirDate)}
		if s.Season <= 0 || first.day.IsZero() {
			continue
		}
		var earliest airedEpisode
		for _, a := range addon {
			if a.place.season == s.Season && !a.day.IsZero() && (earliest.day.IsZero() || a.day.Before(earliest.day)) {
				earliest = a
			}
		}
		if !earliest.day.IsZero() && !nearDays(earliest, first) {
			return false
		}
	}
	return true
}

// alignEpisodes pairs TMDB's episodes (aired) with the addon's, as the
// package's comment tells, and returns the pairs: [TMDB season, TMDB
// number, addon season, addon number] each; nil when the two number the
// episodes alike.
func alignEpisodes(addon, aired []airedEpisode) [][4]int {
	if episodesAgree(addon, aired) {
		return nil
	}
	// The addon's episode each of TMDB's is paired with, -1 for none, and
	// the addon's episodes taken: paired, or claimed by several.
	paired := make([]int, len(aired))
	taken := make([]bool, len(addon))
	for i := range paired {
		paired[i] = -1
	}
	// settle keeps the claims (by TMDB's episode) on episodes of the
	// addon claimed once, and takes those claimed more.
	settle := func(claims map[int]int) {
		count := map[int]int{}
		for _, j := range claims {
			count[j]++
		}
		for i, j := range claims {
			if count[j] == 1 {
				paired[i] = j
			}
			taken[j] = true
		}
	}
	// byDay pairs in order TMDB's episodes left within a day of the same
	// episodes of the addon left, as many of them, and at least least.
	byDay := func(least int) {
		near := map[string][]int{}
		candidates := map[string][]int{}
		var groups []string
		for i, t := range aired {
			if paired[i] >= 0 {
				continue
			}
			var nearest []int
			var key strings.Builder
			for j, a := range addon {
				if !taken[j] && nearDays(a, t) {
					nearest = append(nearest, j)
					key.WriteString(strconv.Itoa(j))
					key.WriteByte(',')
				}
			}
			if len(nearest) < least {
				continue
			}
			if _, ok := near[key.String()]; !ok {
				groups = append(groups, key.String())
			}
			near[key.String()] = append(near[key.String()], i)
			candidates[key.String()] = nearest
		}
		claims := map[int]int{}
		for _, key := range groups {
			tmdbSide, addonSide := near[key], candidates[key]
			if len(tmdbSide) != len(addonSide) {
				continue
			}
			slices.SortFunc(tmdbSide, func(a, b int) int { return comparePlaces(aired[a].place, aired[b].place) })
			slices.SortFunc(addonSide, func(a, b int) int { return comparePlaces(addon[a].place, addon[b].place) })
			for k, i := range tmdbSide {
				claims[i] = addonSide[k]
			}
		}
		settle(claims)
	}
	// The episodes released together on both sides, by their order.
	byDay(2)
	// Then by name, within a day.
	claims := map[int]int{}
	for i, t := range aired {
		if paired[i] >= 0 || t.name == "" {
			continue
		}
		found := -1
		for j, a := range addon {
			if taken[j] || a.name != t.name || !nearDays(a, t) {
				continue
			}
			if found >= 0 {
				found = -1
				break
			}
			found = j
		}
		if found >= 0 {
			claims[i] = found
		}
	}
	settle(claims)
	// Then the only one within a day.
	byDay(1)
	places := [][4]int{}
	for i, j := range paired {
		if j >= 0 {
			places = append(places, [4]int{aired[i].place.season, aired[i].place.number, addon[j].place.season, addon[j].place.number})
		}
	}
	slices.SortFunc(places, func(a, b [4]int) int { return slices.Compare(a[:], b[:]) })
	return places
}

// alignmentRow is the kept alignment of a series (episode_alignments).
type alignmentRow struct {
	tmdb, episodes int
	places         [][4]int
	askedAt        time.Time
}

// keptAlignment loads the alignment kept of a series, nil for none.
func (s *Service) keptAlignment(ctx context.Context, series string) (*alignmentRow, error) {
	var row alignmentRow
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT tmdb_id, episodes, places, asked_at FROM episode_alignments WHERE series = $1`, series).
		Scan(&row.tmdb, &row.episodes, &raw, &row.askedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(raw, &row.places); err != nil {
		return nil, err
	}
	return &row, nil
}

// keepAlignment keeps the alignment of a series.
func (s *Service) keepAlignment(ctx context.Context, series string, row alignmentRow) error {
	// null for a series numbered alike.
	raw, err := json.Marshal(row.places)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO episode_alignments (series, tmdb_id, episodes, places, asked_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (series) DO UPDATE SET tmdb_id = excluded.tmdb_id, episodes = excluded.episodes, places = excluded.places,
			asked_at = excluded.asked_at`, series, row.tmdb, row.episodes, raw, row.askedAt)
	return err
}

// errUnaligned reports that the sources of TMDB's numbering failed to
// give it: the episodes keep their numbers until they are asked again.
var errUnaligned = errors.New("TMDB's numbering of a series' episodes could not be read")

// numbering returns the numbering of the episodes of the series Polyfin
// names by the Stremio identifier series, whose TMDB identifier is
// tmdbID, 0 when unknown, and IMDb's imdb, empty when unknown, against
// TMDB's, as the package's comment tells: listed are its episodes Polyfin
// listed, else those the server's addons describe, and server the
// server's view, made once when first needed. Without the series' TMDB
// identifier, it is the one kept, else, with a key, the one TMDB finds by
// its IMDb one. When a source fails, it returns the zero numbering and an
// error wrapping errUnaligned.
func (s *Service) numbering(ctx context.Context, server func() (view, error), series string, tmdbID int, imdb string,
	listed episodeList) (EpisodeNumbering, error) {
	if strings.HasPrefix(series, "tmdb:") {
		return EpisodeNumbering{sourced: true}, nil
	}
	if places, ok := tmdbVideoPlaces(listed); ok {
		return numberingOf(places), nil
	}
	addon := listedEpisodes(listed)
	kept, err := s.keptAlignment(ctx, series)
	if err != nil {
		return EpisodeNumbering{}, err
	}
	if kept != nil && (tmdbID == 0 || tmdbID == kept.tmdb) && s.now().Sub(kept.askedAt) < alignmentFresh &&
		(len(addon) == 0 || len(addon) == kept.episodes) {
		return numberingOf(kept.places), nil
	}
	if tmdbID == 0 && kept != nil {
		tmdbID = kept.tmdb
	}
	if tmdbID == 0 && imdbID.MatchString(imdb) && s.tmdb.Enabled() {
		found, err := s.tmdb.FindByIMDb(ctx, tmdb.TV, imdb)
		switch {
		case err == nil:
			tmdbID, _ = strconv.Atoi(found)
		case !errors.Is(err, tmdb.ErrNotFound):
			return EpisodeNumbering{}, errors.Join(errUnaligned, err)
		}
	}
	if tmdbID <= 0 {
		return EpisodeNumbering{}, nil
	}
	v, err := server()
	if err != nil {
		return EpisodeNumbering{}, err
	}
	// A server addon's description of "tmdb:<id>" is numbered as TMDB
	// numbers it.
	var aired []airedEpisode
	if meta, ok := s.serverMeta(ctx, v, string(KindSeries), "tmdb:"+strconv.Itoa(tmdbID)); ok {
		aired = videoEpisodes(meta.Videos, true)
	}
	if len(aired) == 0 && !s.tmdb.Enabled() {
		return EpisodeNumbering{}, nil
	}
	if len(addon) == 0 {
		if meta, ok := s.serverMeta(ctx, v, string(KindSeries), series); ok {
			addon = videoEpisodes(meta.Videos, false)
		}
		if len(addon) == 0 {
			return EpisodeNumbering{}, nil
		}
	}
	if len(aired) == 0 {
		var found bool
		if aired, found, err = s.tmdbEpisodes(ctx, tmdbID, addon); err != nil {
			return EpisodeNumbering{}, errors.Join(errUnaligned, err)
		} else if !found {
			return EpisodeNumbering{}, nil
		}
	}
	row := alignmentRow{tmdb: tmdbID, episodes: len(addon), askedAt: s.now()}
	if aired != nil {
		row.places = alignEpisodes(addon, aired)
	}
	if err := s.keepAlignment(ctx, series, row); err != nil {
		return EpisodeNumbering{}, err
	}
	return numberingOf(row.places), nil
}

// tmdbEpisodes returns the episodes of the series TMDB identifies as id,
// as TMDB gives them with the server's key: nil when TMDB's seasons agree
// with those of the addon's episodes, which then are not asked. It reports
// false when TMDB does not know the series.
func (s *Service) tmdbEpisodes(ctx context.Context, id int, addon []airedEpisode) ([]airedEpisode, bool, error) {
	seasons, err := s.tmdb.SeriesSeasons(ctx, id)
	switch {
	case errors.Is(err, tmdb.ErrNotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	case seasonsAgree(addon, seasons):
		return nil, true, nil
	}
	episodes, err := s.tmdb.SeasonEpisodes(ctx, id, seasons)
	if err != nil {
		return nil, false, err
	}
	aired := make([]airedEpisode, 0, len(episodes))
	for _, e := range episodes {
		if e.Season >= 0 && e.Episode > 0 {
			aired = append(aired, airedEpisode{place: episodePlace{e.Season, e.Episode}, day: episodeDay(e.AirDate), name: airedName(e.Name)})
		}
	}
	return aired, true, nil
}

// serverView returns what makes the server's view once, when first
// needed: a view without a user is the server's addons'.
func (s *Service) serverView(ctx context.Context) func() (view, error) {
	return sync.OnceValues(func() (view, error) { return s.view(ctx, accounts.User{}) })
}

// numberings returns the numberings of the series of tmdbIDs, by their
// Stremio identifiers, with their TMDB identifiers, 0 when unknown, from
// the episodes Polyfin listed of them (known), a few series at a time. A
// series whose sources fail keeps its numbers, which a warning tells.
func (s *Service) numberings(ctx context.Context, tmdbIDs map[string]int, known map[string]episodeList) (map[string]EpisodeNumbering, error) {
	if len(tmdbIDs) == 0 {
		return nil, nil
	}
	server := s.serverView(ctx)
	series := make([]string, 0, len(tmdbIDs))
	for id := range tmdbIDs {
		series = append(series, id)
	}
	found := make([]EpisodeNumbering, len(series))
	failures := make([]error, len(series))
	var group errgroup.Group
	group.SetLimit(describedFetches)
	for i, id := range series {
		group.Go(func() error {
			imdb := ""
			if imdbID.MatchString(id) {
				imdb = id
			}
			found[i], failures[i] = s.numbering(ctx, server, id, tmdbIDs[id], imdb, known[id])
			if errors.Is(failures[i], errUnaligned) {
				return nil
			}
			return failures[i]
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if err := errors.Join(failures...); err != nil && ctx.Err() == nil {
		s.logger.Warn("TMDB's numbering of episodes could not be read: they keep their numbers until it is asked again", "error", err)
	}
	numberings := make(map[string]EpisodeNumbering, len(series))
	for i, id := range series {
		numberings[id] = found[i]
	}
	return numberings, nil
}

// TMDBNumbering returns how TMDB numbers the episodes of the series
// Polyfin lists as seriesID, against how its addon does, as the tracking
// services number them: the zero numbering, alike, for a series nothing
// numbers as TMDB does (see the package's comment on episode numberings),
// or whose sources fail, which a warning tells.
func (s *Service) TMDBNumbering(ctx context.Context, seriesID accounts.ID) (EpisodeNumbering, error) {
	r, err := s.load(ctx, seriesID)
	if err != nil || r.Kind != KindSeries {
		return EpisodeNumbering{}, err
	}
	series := strings.TrimPrefix(r.Key, string(KindSeries)+"|")
	tmdbID, imdb := 0, series
	if r.Meta != nil {
		tmdbID = describedNumber(stremio.Text(s.providerIDs(r.ID, *r.Meta)["Tmdb"]))
		imdb = cmp.Or(r.Meta.ImdbID, imdb)
	}
	known, err := s.knownEpisodes(ctx, []string{series})
	if err != nil {
		return EpisodeNumbering{}, err
	}
	numbering, err := s.numbering(ctx, s.serverView(ctx), series, tmdbID, imdb, known[series])
	if errors.Is(err, errUnaligned) {
		if ctx.Err() == nil {
			s.logger.Warn("TMDB's numbering of episodes could not be read: they keep their numbers until it is asked again", "series", series,
				"error", err)
		}
		return EpisodeNumbering{}, nil
	}
	return numbering, err
}

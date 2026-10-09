package library

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// TitleRef names a movie, a series, or an episode by its series and
// numbers, by the identifiers another service knows it by, as a watch
// history gives them.
type TitleRef struct {
	// Episode names an episode of the series the identifiers name, Series
	// the series itself; neither names a movie.
	Episode, Series bool
	// IMDb ("tt…"), TMDB and TVDB identify the movie or the series; any
	// may be missing. Movies are not looked up by TVDB.
	IMDb       string
	TMDB, TVDB int
	// Season and Number place an episode in its series.
	Season, Number int
	// Name is the title's name as the other service gives it, the series'
	// for an episode; empty when it gives none. It names the record of a
	// title Polyfin had none of until its addons describe it.
	Name string
}

// kind is the kind of the title the identifiers of r name: a movie, or a
// series for an episode or a series.
func (r TitleRef) kind() Kind {
	if r.Episode || r.Series {
		return KindSeries
	}
	return KindMovie
}

// TitleTarget is an item a TitleRef designates, as Polyfin keeps the
// user's data of it: an episode with its series and season. Runtime is
// the title's runtime when the addons told it, zero otherwise.
type TitleTarget struct {
	ID, Series, Season accounts.ID
	Runtime            time.Duration
}

var (
	imdbID = regexp.MustCompile(`^tt\d+$`)
	// conventionalVideo matches the video identifiers of episodes that
	// metadata addons make of their series' identifier and the episode's
	// numbers: "<series>:<season>:<episode>".
	conventionalVideo = regexp.MustCompile(`^(.+):(\d+):(\d+)$`)
)

// Resolve finds the items each ref designates, in order: the titles
// Polyfin listed under one of the ref's identifiers (a title listed by
// several catalogs, under several identifiers, gives several items), else
// the one its IMDb identifier names as the usual metadata addons name
// titles ("tt…" for a movie or series, "tt…:<season>:<episode>" for an
// episode). An episode of a series listed under another identifier is
// found among the episodes Polyfin listed, else named the way the listed
// ones of its series are. A ref nothing designates gets no target.
//
// Nothing is asked of the addons. The items Resolve names that Polyfin
// has no record of, as a title no catalog listed, are recorded as the
// addons would name them: a movie or series by its IMDb identifier and the
// ref's name, and an episode with its season. The users' data of them then
// shows in their lists at once, and the addons describe them when asked.
// A record already kept is left as it is, and a later listing replaces
// these with full ones.
func (s *Service) Resolve(ctx context.Context, refs []TitleRef) ([][]TitleTarget, error) {
	titles, err := s.knownTitles(ctx, refs)
	if err != nil {
		return nil, err
	}
	// The Stremio identifiers of the series each episode ref belongs to.
	seriesOf := make([][]string, len(refs))
	var seriesIDs []string
	var unlisted []record
	result := make([][]TitleTarget, len(refs))
	for i, ref := range refs {
		kind := ref.kind()
		var found []knownTitle
		for _, title := range titles {
			if title.kind == kind && title.matches(ref) {
				found = append(found, title)
			}
		}
		if len(found) == 0 && imdbID.MatchString(ref.IMDb) {
			found = append(found, knownTitle{kind: kind, stremioID: ref.IMDb})
			unlisted = append(unlisted, unlistedTitle(kind, ref))
		}
		for _, title := range found {
			if !ref.Episode {
				result[i] = append(result[i], TitleTarget{ID: itemID(titleKey(kind, title.stremioID)), Runtime: title.runtime})
				continue
			}
			seriesOf[i] = append(seriesOf[i], title.stremioID)
			if !slices.Contains(seriesIDs, title.stremioID) {
				seriesIDs = append(seriesIDs, title.stremioID)
			}
		}
	}
	if len(seriesIDs) == 0 {
		return result, s.saveNew(ctx, unlisted)
	}
	known, err := s.knownEpisodes(ctx, seriesIDs)
	if err != nil {
		return nil, err
	}
	runtimes := map[string]time.Duration{}
	for _, title := range titles {
		if title.kind == KindSeries {
			runtimes[title.stremioID] = title.runtime
		}
	}
	for i, ref := range refs {
		for _, series := range seriesOf[i] {
			episodes := known[series]
			target := TitleTarget{Series: itemID(titleKey(KindSeries, series)), Season: itemID(seasonKey(series, ref.Season)),
				Runtime: runtimes[series]}
			if episode, ok := episodes.find(ref.Season, ref.Number); ok {
				target.ID = episode.id
				if episode.runtime > 0 {
					target.Runtime = episode.runtime
				}
			} else if imdbID.MatchString(series) || len(episodes) > 0 && episodes.conventional(series) {
				videoID := series + ":" + strconv.Itoa(ref.Season) + ":" + strconv.Itoa(ref.Number)
				target.ID = itemID(episodeKey(videoID))
				// Recorded as Episodes records the episodes of a series.
				video := &stremio.Video{ID: videoID, Season: stremio.Number(ref.Season), Episode: stremio.Number(ref.Number)}
				unlisted = append(unlisted,
					record{ID: target.Season, Key: seasonKey(series, ref.Season), Kind: KindSeason, Parent: new(target.Series),
						SeriesID: series, Season: ref.Season},
					record{ID: target.ID, Key: episodeKey(videoID), Kind: KindEpisode, Parent: new(target.Season), SeriesID: series,
						Season: ref.Season, Video: video})
			} else {
				continue
			}
			result[i] = append(result[i], target)
		}
	}
	return result, s.saveNew(ctx, unlisted)
}

// unlistedTitle is the record of the movie or series of kind that ref
// names by its IMDb identifier, which no catalog listed: what ref tells of
// it, which the addons complete when asked to describe it.
func unlistedTitle(kind Kind, ref TitleRef) record {
	meta := &stremio.Meta{ID: ref.IMDb, Type: string(kind), Name: ref.Name, ImdbID: ref.IMDb}
	if ref.TMDB > 0 {
		meta.TmdbID = stremio.Text(strconv.Itoa(ref.TMDB))
	}
	if ref.TVDB > 0 && kind == KindSeries {
		meta.TvdbID = stremio.Text(strconv.Itoa(ref.TVDB))
	}
	return record{ID: itemID(titleKey(kind, ref.IMDb)), Key: titleKey(kind, ref.IMDb), Kind: kind, Meta: meta}
}

// knownTitle is a movie or series Polyfin listed.
type knownTitle struct {
	kind             Kind
	stremioID        string
	imdb, tmdb, tvdb string
	runtime          time.Duration
}

// matches reports whether the title is the one ref names, by one of their
// identifiers.
func (t knownTitle) matches(ref TitleRef) bool {
	tmdb, tvdb := "", ""
	if ref.TMDB > 0 {
		tmdb = strconv.Itoa(ref.TMDB)
	}
	if ref.TVDB > 0 && ref.kind() == KindSeries {
		tvdb = strconv.Itoa(ref.TVDB)
	}
	switch {
	case ref.IMDb != "" && (t.stremioID == ref.IMDb || t.imdb == ref.IMDb):
		return true
	case tmdb != "" && (t.stremioID == "tmdb:"+tmdb || t.tmdb == tmdb):
		return true
	case tvdb != "" && (t.stremioID == "tvdb:"+tvdb || t.tvdb == tvdb):
		return true
	}
	return false
}

// knownTitles loads the movies and series Polyfin listed under one of the
// identifiers of refs.
func (s *Service) knownTitles(ctx context.Context, refs []TitleRef) ([]knownTitle, error) {
	var keys, imdbs, tmdbs, tvdbs []string
	for _, ref := range refs {
		kind := ref.kind()
		if ref.IMDb != "" {
			keys, imdbs = append(keys, titleKey(kind, ref.IMDb)), append(imdbs, ref.IMDb)
		}
		if ref.TMDB > 0 {
			id := strconv.Itoa(ref.TMDB)
			keys, tmdbs = append(keys, titleKey(kind, "tmdb:"+id)), append(tmdbs, id)
		}
		if ref.TVDB > 0 && kind == KindSeries {
			id := strconv.Itoa(ref.TVDB)
			keys, tvdbs = append(keys, titleKey(kind, "tvdb:"+id)), append(tvdbs, id)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT kind, key, data->'meta' FROM items
		WHERE kind IN ('movie', 'series') AND (key = ANY($1) OR data->'meta'->>'imdb_id' = ANY($2)
			OR data->'meta'->>'_tmdbId' = ANY($3) OR data->'meta'->>'_tvdbId' = ANY($4))`, keys, imdbs, tmdbs, tvdbs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (knownTitle, error) {
		var kind, key string
		var raw []byte
		if err := row.Scan(&kind, &key, &raw); err != nil {
			return knownTitle{}, err
		}
		title := knownTitle{kind: Kind(kind), stremioID: strings.TrimPrefix(key, kind+"|")}
		var meta stremio.Meta
		if len(raw) > 0 && json.Unmarshal(raw, &meta) == nil {
			title.imdb, title.tmdb, title.tvdb = meta.ImdbID, strings.TrimSpace(string(meta.TmdbID)), strings.TrimSpace(string(meta.TvdbID))
			title.runtime = parseRuntime(string(meta.Runtime))
		}
		return title, nil
	})
}

// knownEpisode is an episode Polyfin listed.
type knownEpisode struct {
	id             accounts.ID
	videoID        string
	season, number int
	runtime        time.Duration
}

type episodeList []knownEpisode

func (l episodeList) find(season, number int) (knownEpisode, bool) {
	i := slices.IndexFunc(l, func(e knownEpisode) bool { return e.season == season && e.number == number })
	if i < 0 {
		return knownEpisode{}, false
	}
	return l[i], true
}

// conventional reports whether the episodes listed of series are named
// "<series>:<season>:<episode>", as the others then are.
func (l episodeList) conventional(series string) bool {
	for _, e := range l {
		parts := conventionalVideo.FindStringSubmatch(e.videoID)
		if parts == nil || parts[1] != series || parts[2] != strconv.Itoa(e.season) || parts[3] != strconv.Itoa(e.number) {
			return false
		}
	}
	return true
}

// knownEpisodes loads the episodes Polyfin listed of the series, by their
// Stremio identifiers.
func (s *Service) knownEpisodes(ctx context.Context, series []string) (map[string]episodeList, error) {
	rows, err := s.db.Query(ctx, `SELECT id, data->>'seriesId', data->'video' FROM items
		WHERE kind = 'episode' AND data->>'seriesId' = ANY($1)`, series)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]episodeList{}
	for rows.Next() {
		var id accounts.ID
		var seriesID string
		var raw []byte
		if err := rows.Scan(&id, &seriesID, &raw); err != nil {
			return nil, err
		}
		var video stremio.Video
		if len(raw) == 0 || json.Unmarshal(raw, &video) != nil {
			continue
		}
		known[seriesID] = append(known[seriesID], knownEpisode{id: id, videoID: video.ID, season: int(video.Season),
			number: video.EpisodeNumber(), runtime: parseRuntime(string(video.Runtime))})
	}
	return known, rows.Err()
}

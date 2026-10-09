package library

import (
	"cmp"
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
	// Season and Number place an episode in its series. When Absolute,
	// Number counts the episode among the regular episodes of its series,
	// from the first of its first season, across its seasons (TVDB's
	// absolute numbering), and Season is not read.
	Season, Number int
	Absolute       bool
	// Anime names the anime entry as anime catalogs list it, when the
	// ref is of one.
	Anime AnimeRef
	// Name is the title's name as the other service gives it, the series'
	// for an episode; empty when it gives none. It names the record of a
	// title Polyfin had none of until its addons describe it.
	Name string
}

// AnimeRef names an anime entry, as AniDB, Kitsu and MyAnimeList split
// series into entries, the way anime catalogs list them: "kitsu:<id>",
// "mal:<id>" or "anidb:<id>", and an episode as the entry numbers it,
// from 1, "kitsu:<id>:<episode>". Zero identifiers are unknown.
type AnimeRef struct {
	Kitsu, MyAnimeList, AniDB int
	// Episode is the episode's number in the entry, for an episode ref.
	Episode int
}

// animePrefixes begin the Stremio identifiers of anime catalogs' titles.
var animePrefixes = [...]string{"kitsu:", "mal:", "anidb:"}

// ids are the Stremio identifiers of the entry.
func (a AnimeRef) ids() []string {
	var ids []string
	for i, id := range [...]int{a.Kitsu, a.MyAnimeList, a.AniDB} {
		if id > 0 {
			ids = append(ids, animePrefixes[i]+strconv.Itoa(id))
		}
	}
	return ids
}

// animeNamed reports whether an anime catalog names the title id.
func animeNamed(id string) bool {
	return slices.ContainsFunc(animePrefixes[:], func(prefix string) bool { return strings.HasPrefix(id, prefix) })
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
// user's data of it: an episode with its series and season, which it is
// numbered Number of, SeasonNumber. Runtime is the title's runtime when
// the addons told it, zero otherwise. Anime is set for an item an anime
// catalog listed under the ref's AnimeRef.
type TitleTarget struct {
	ID, Series, Season   accounts.ID
	SeasonNumber, Number int
	Runtime              time.Duration
	Anime                bool
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
// ones of its series are. An episode numbered as TVDB's absolute
// numbering is the one of that place among the regular episodes Polyfin
// listed of its series, in order. The titles an anime catalog listed
// under the ref's AnimeRef are found too, an episode by its number in the
// entry ("kitsu:<id>:<episode>"), and only once listed. A ref nothing
// designates gets no target.
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
	// The Stremio identifiers of the series each episode ref belongs to,
	// and of those an anime catalog listed under its AnimeRef.
	seriesOf := make([][]string, len(refs))
	animeOf := make([][]string, len(refs))
	var seriesIDs []string
	var unlisted []record
	result := make([][]TitleTarget, len(refs))
	addSeries := func(id string) {
		if !slices.Contains(seriesIDs, id) {
			seriesIDs = append(seriesIDs, id)
		}
	}
	for i, ref := range refs {
		kind := ref.kind()
		animeIDs := ref.Anime.ids()
		var found, anime []knownTitle
		for _, title := range titles {
			switch {
			case title.kind != kind:
			case slices.Contains(animeIDs, title.stremioID):
				anime = append(anime, title)
			// An anime catalog numbers the episodes of each entry apart:
			// another entry's are not those of the series' seasons.
			case len(animeIDs) > 0 && ref.Episode && animeNamed(title.stremioID):
			case title.matches(ref):
				found = append(found, title)
			}
		}
		// An episode numbered absolutely is placed among those listed only.
		if len(found) == 0 && imdbID.MatchString(ref.IMDb) && !(ref.Episode && ref.Absolute) {
			found = append(found, knownTitle{kind: kind, stremioID: ref.IMDb})
			unlisted = append(unlisted, unlistedTitle(kind, ref))
		}
		for _, title := range found {
			if !ref.Episode {
				result[i] = append(result[i], TitleTarget{ID: itemID(titleKey(kind, title.stremioID)), Runtime: title.runtime})
				continue
			}
			seriesOf[i] = append(seriesOf[i], title.stremioID)
			addSeries(title.stremioID)
		}
		for _, title := range anime {
			switch {
			case !ref.Episode:
				result[i] = append(result[i], TitleTarget{ID: itemID(titleKey(kind, title.stremioID)), Runtime: title.runtime, Anime: true})
			case ref.Anime.Episode > 0:
				animeOf[i] = append(animeOf[i], title.stremioID)
				addSeries(title.stremioID)
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
			season, number := ref.Season, ref.Number
			if ref.Absolute {
				episode, ok := episodes.absolute(ref.Number)
				if !ok {
					continue
				}
				season, number = episode.season, episode.number
			}
			target := TitleTarget{Series: itemID(titleKey(KindSeries, series)), Season: itemID(seasonKey(series, season)),
				SeasonNumber: season, Number: number, Runtime: runtimes[series]}
			if episode, ok := episodes.find(season, number); ok {
				target.ID = episode.id
				if episode.runtime > 0 {
					target.Runtime = episode.runtime
				}
			} else if imdbID.MatchString(series) || len(episodes) > 0 && episodes.conventional(series) {
				videoID := series + ":" + strconv.Itoa(season) + ":" + strconv.Itoa(number)
				target.ID = itemID(episodeKey(videoID))
				// Recorded as Episodes records the episodes of a series.
				video := &stremio.Video{ID: videoID, Season: stremio.Number(season), Episode: stremio.Number(number)}
				unlisted = append(unlisted,
					record{ID: target.Season, Key: seasonKey(series, season), Kind: KindSeason, Parent: new(target.Series),
						SeriesID: series, Season: season},
					record{ID: target.ID, Key: episodeKey(videoID), Kind: KindEpisode, Parent: new(target.Season), SeriesID: series,
						Season: season, Video: video})
			} else {
				continue
			}
			result[i] = append(result[i], target)
		}
		for _, series := range animeOf[i] {
			episode, ok := known[series].video(series + ":" + strconv.Itoa(ref.Anime.Episode))
			if !ok {
				continue
			}
			target := TitleTarget{ID: episode.id, Series: itemID(titleKey(KindSeries, series)), Season: itemID(seasonKey(series, episode.season)),
				SeasonNumber: episode.season, Number: episode.number, Runtime: cmp.Or(episode.runtime, runtimes[series]), Anime: true}
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
// identifiers of refs, or that an anime catalog listed under their
// AnimeRef.
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
		for _, id := range ref.Anime.ids() {
			keys = append(keys, titleKey(kind, id))
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

// video finds the episode whose video identifier is id.
func (l episodeList) video(id string) (knownEpisode, bool) {
	i := slices.IndexFunc(l, func(e knownEpisode) bool { return e.videoID == id })
	if i < 0 {
		return knownEpisode{}, false
	}
	return l[i], true
}

// absolute finds the episode numbered n in the absolute numbering of its
// series: the nth of its regular episodes listed, by season then number.
func (l episodeList) absolute(n int) (knownEpisode, bool) {
	regular := make([]knownEpisode, 0, len(l))
	for _, e := range l {
		if e.season > 0 && e.number > 0 {
			regular = append(regular, e)
		}
	}
	place := func(a, b knownEpisode) int {
		return cmp.Or(cmp.Compare(a.season, b.season), cmp.Compare(a.number, b.number))
	}
	slices.SortFunc(regular, place)
	regular = slices.CompactFunc(regular, func(a, b knownEpisode) bool { return place(a, b) == 0 })
	if n <= 0 || n > len(regular) {
		return knownEpisode{}, false
	}
	return regular[n-1], true
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

package library

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A stream addon's manifest tells, by its idPrefixes, the identifiers it
// lists streams for: some take IMDb's only ("tt"), some TMDB's ("tmdb:"),
// some an anime database's ("kitsu:", "mal:", "anidb:"). A title is asked
// for by its own identifier when the addon accepts it; else by the first
// of its other identifiers the addon accepts, in this order: its IMDb
// identifier, its TMDB one, then those of its anime entry, which the
// anime mapping gives (see package anime), as tracking uses it.

// UseAnime sets the anime mapping, which gives the identifiers anime
// addons accept for titles named otherwise, and the reverse; it is called
// before the service is used. Without it, a title is asked for by the
// identifiers its description gives only.
func (s *Service) UseAnime(mapping *anime.Service) { s.anime = mapping }

// streamSource is an addon that lists streams for a title, and what it is
// asked for them: the title's own type and identifier, or another it
// accepts (see streamRequests). Its lists are kept, and its versions
// renewed, by what it is asked.
type streamSource struct {
	installed
	contentType, id string
}

// key is what the addon's stream list for the title is kept under.
func (source streamSource) key() streamKey {
	return streamKey{source.addon.ID, source.contentType, source.id}
}

// streamRequest is a type and identifier an addon may be asked streams
// for.
type streamRequest struct {
	contentType, id string
}

// streamServing lists the addons of v that list streams for t, each with
// what it is asked: t's own identifier when it accepts it, else the first
// of t's other identifiers it accepts (see streamRequests). Only Stremio
// addons are asked by another identifier: an IPTV source or a local folder
// answers for the titles it lists, by their own identifiers.
func (s *Service) streamServing(ctx context.Context, v view, t target) []streamSource {
	var (
		serving []streamSource
		others  []streamRequest
		mapped  bool
	)
	for _, entry := range v.addons {
		// An IPTV source's channel is only its source's to play.
		if entry.addon.Stremio() && strings.HasPrefix(t.id, iptv.IDPrefix) {
			continue
		}
		if serves(entry, "stream", t.metaType, t.id) {
			serving = append(serving, streamSource{entry, t.metaType, t.id})
			continue
		}
		if !entry.addon.Stremio() {
			continue
		}
		if !mapped {
			others, mapped = s.streamRequests(ctx, v, t), true
		}
		for _, request := range others {
			if entry.addon.Manifest.Serves("stream", request.contentType, request.id) {
				serving = append(serving, streamSource{entry, request.contentType, request.id})
				break
			}
		}
	}
	return serving
}

// streamRequests lists the other identifiers a movie or an episode may be
// asked streams by, in order: the IMDb one, the TMDB one, then those of
// its anime entry, Kitsu's, MyAnimeList's and AniDB's, each typed "movie"
// or "series" as Stremio addons expect. An episode's identifiers carry its
// season and number, as Stremio's do ("tt…:1:5"), and an anime entry's
// its own episode number ("kitsu:…:5"), as AniDB and the services that
// follow it count each season apart.
//
// A title named by IMDb or TMDB has the identifiers its description
// gives, the episode numbered as it is; its anime entry is the one the
// mapping gives for its IMDb or TMDB identifier, or, for an episode, the
// AniDB entry that holds its TVDB season and number (see
// anime.Service.Entry). A title named by an anime entry has the
// identifiers the mapping gives: for an episode, IMDb's in TVDB's
// numbering, and no TMDB one, as TMDB may number the series' seasons
// otherwise. An identifier the mapping gives is only used when one entry
// alone matches: a movie or a series spread over entries that disagree
// is asked by none. Home videos and plain files are only their folder's.
func (s *Service) streamRequests(ctx context.Context, v view, t target) []streamRequest {
	if t.kind != KindMovie && t.kind != KindEpisode || strings.HasPrefix(t.id, PlainIDPrefix) || strings.HasPrefix(t.id, VideoIDPrefix) {
		return nil
	}
	movie := t.kind == KindMovie
	contentType := "series"
	if movie {
		contentType = "movie"
	}
	ids := t.ids
	series := t.described
	if !movie && series == nil && t.holder != nil {
		// The series' complete description may give identifiers its
		// listing does not, and lists the episodes absolute numbering
		// counts.
		if meta, _, ok := s.describedMeta(ctx, v, *t.holder, false, inDatabase); ok {
			series = &meta
		}
	}
	if series != nil {
		ids = withMissing(ids, providerIDs(*series))
	}
	var requests []streamRequest
	add := func(id string) {
		request := streamRequest{contentType, id}
		if (id != t.id || contentType != t.metaType) && !slices.Contains(requests, request) {
			requests = append(requests, request)
		}
	}
	// An anime catalog may type its titles otherwise ("anime"): an addon
	// may accept the identifier with Stremio's type.
	add(t.id)
	if source, entry, number, ok := anime.StremioID(t.id); ok {
		if !movie && number <= 0 {
			return requests
		}
		mapped, known := s.anime.Title(source, entry, false, number, movie)
		switch {
		case movie:
			imdb, tmdb := ids["Imdb"], ids["Tmdb"]
			if known && mapped.Movie {
				imdb = cmp.Or(imdb, mapped.IMDb)
				if tmdb == "" && mapped.TMDB > 0 {
					tmdb = strconv.Itoa(mapped.TMDB)
				}
			}
			if imdb != "" {
				add(imdb)
			}
			if tmdb != "" {
				add("tmdb:" + tmdb)
			}
		case known && !mapped.Movie && !mapped.Absolute && mapped.IMDb != "" && mapped.Number > 0:
			add(mapped.IMDb + episodeSuffix(mapped.Season, mapped.Number))
		}
		if found := s.anime.IDs(source, strconv.Itoa(entry)); len(found) == 1 {
			for _, id := range animeIDs(found[0], number, source) {
				add(id)
			}
		}
		return requests
	}
	if !movie && (t.season < 0 || t.episode <= 0) {
		return requests
	}
	suffix := ""
	if !movie {
		suffix = episodeSuffix(t.season, t.episode)
	}
	imdb, tmdb := ids["Imdb"], ids["Tmdb"]
	if imdb != "" {
		add(imdb + suffix)
	}
	if tmdb != "" {
		add("tmdb:" + tmdb + suffix)
	}
	if found, number, ok := s.animeEntry(t, imdb, tmdb, ids["Tvdb"], series); ok {
		for _, id := range animeIDs(found, number, "") {
			add(id)
		}
	}
	return requests
}

// animeEntry finds the anime entry of a movie or an episode named by IMDb
// or TMDB, and the episode's number there, as tracking finds it: a movie
// by its IMDb, else TMDB, identifier; an episode by the AniDB entry that
// holds its TVDB season and number, its TVDB series the one its
// description gives, else the one its IMDb identifier's entries agree on.
// It reports false unless one entry alone matches.
func (s *Service) animeEntry(t target, imdb, tmdb, tvdbID string, series *stremio.Meta) (anime.IDs, int, bool) {
	if s.anime == nil {
		return anime.IDs{}, 0, false
	}
	if t.kind == KindMovie {
		movies := func(found []anime.IDs) []anime.IDs {
			return slices.DeleteFunc(found, func(ids anime.IDs) bool { return !ids.Movie && len(ids.TMDBMovies) == 0 })
		}
		var found []anime.IDs
		if imdb != "" {
			found = movies(s.anime.IDs(anime.IMDb, imdb))
		}
		if len(found) == 0 && tmdb != "" {
			found = movies(s.anime.IDs(anime.TMDBMovie, tmdb))
		}
		if found = oneByAniDB(found); len(found) != 1 {
			return anime.IDs{}, 0, false
		}
		return found[0], 0, true
	}
	tvdb, _ := strconv.Atoi(tvdbID)
	if tvdb <= 0 && imdb != "" {
		for _, ids := range s.anime.IDs(anime.IMDb, imdb) {
			if ids.Movie || len(ids.TMDBMovies) > 0 {
				continue
			}
			if ids.TVDB <= 0 || tvdb > 0 && ids.TVDB != tvdb {
				return anime.IDs{}, 0, false
			}
			tvdb = ids.TVDB
		}
	}
	if tvdb <= 0 {
		return anime.IDs{}, 0, false
	}
	absolute := 0
	if series != nil && s.anime.Absolute(tvdb) {
		absolute = absoluteEpisode(series.Videos, t.season, t.episode)
	}
	anidb, number, ok := s.anime.Entry(tvdb, t.season, t.episode, absolute)
	if !ok {
		return anime.IDs{}, 0, false
	}
	found := s.anime.IDs(anime.AniDB, strconv.Itoa(anidb))
	if len(found) != 1 {
		return anime.IDs{}, 0, false
	}
	return found[0], number, true
}

// otherStreamKeys lists the stream lists the addons of v keep for a title
// r, a movie, an episode, or a series or season's episodes, under another
// identifier than the title's own (see streamServing): those a refresh
// forgets too. title is the movie, or the episodes' series, and described
// the series' complete description, nil when unknown.
func (s *Service) otherStreamKeys(ctx context.Context, v view, r, title record, described *stremio.Meta) []streamKey {
	var targets []target
	ids := providerIDs(*title.Meta)
	episode := func(video stremio.Video) target {
		return target{kind: KindEpisode, metaType: title.Meta.Type, id: video.ID, season: int(video.Season), episode: video.EpisodeNumber(),
			ids: ids, described: described, holder: &title}
	}
	switch r.Kind {
	case KindMovie:
		meta := *title.Meta
		if full, _, ok := s.describedMeta(ctx, v, title, false, inDatabase); ok {
			meta = full
		}
		targets = append(targets, target{kind: KindMovie, metaType: meta.Type, id: meta.ID, ids: providerIDs(meta)})
	case KindEpisode:
		if r.Video != nil {
			targets = append(targets, episode(*r.Video))
		}
	case KindSeries, KindSeason:
		if described != nil {
			for _, video := range described.Videos {
				if r.covers(video) {
					targets = append(targets, episode(video))
				}
			}
		}
	}
	var keys []streamKey
	for _, t := range targets {
		for _, source := range s.streamServing(ctx, v, t) {
			if source.id != t.id || source.contentType != t.metaType {
				keys = append(keys, source.key())
			}
		}
	}
	return keys
}

// animeIDs are the Stremio identifiers of an anime entry, or of its
// episode number when not 0, by Kitsu, MyAnimeList and AniDB, but for
// skip's, the title's own.
func animeIDs(ids anime.IDs, number int, skip anime.Source) []string {
	var result []string
	for _, entry := range []struct {
		source anime.Source
		id     int
	}{{anime.Kitsu, ids.Kitsu}, {anime.MyAnimeList, ids.MyAnimeList}, {anime.AniDB, ids.AniDB}} {
		if entry.id <= 0 || entry.source == skip {
			continue
		}
		id := string(entry.source) + ":" + strconv.Itoa(entry.id)
		if number > 0 {
			id += ":" + strconv.Itoa(number)
		}
		result = append(result, id)
	}
	return result
}

// oneByAniDB leaves one of the entries found per AniDB entry.
func oneByAniDB(found []anime.IDs) []anime.IDs {
	var result []anime.IDs
	seen := map[int]bool{}
	for _, ids := range found {
		if ids.AniDB > 0 && seen[ids.AniDB] {
			continue
		}
		seen[ids.AniDB] = true
		result = append(result, ids)
	}
	return result
}

// episodeSuffix ends the Stremio identifier of an episode of a series.
func episodeSuffix(season, number int) string {
	return ":" + strconv.Itoa(season) + ":" + strconv.Itoa(number)
}

// episodePlace is an episode's season and number.
type episodePlace struct{ season, number int }

// comparePlaces orders episodes by season, then number.
func comparePlaces(a, b episodePlace) int {
	return cmp.Or(cmp.Compare(a.season, b.season), cmp.Compare(a.number, b.number))
}

// regularEpisodes are the regular episodes of videos, specials left out,
// once each, by season then number: TVDB's absolute numbering, as
// tracking counts it (see episodeList.absolute).
func regularEpisodes(videos []stremio.Video) []episodePlace {
	var regular []episodePlace
	for _, video := range videos {
		if video.Season > 0 && video.EpisodeNumber() > 0 {
			regular = append(regular, episodePlace{int(video.Season), video.EpisodeNumber()})
		}
	}
	slices.SortFunc(regular, comparePlaces)
	return slices.Compact(regular)
}

// absoluteEpisode is the place of episode number of season among the
// regular episodes of videos, by season then number, from 1: TVDB's
// absolute numbering. It is 0 for a special or an episode not listed.
func absoluteEpisode(videos []stremio.Video, season, number int) int {
	if season <= 0 || number <= 0 {
		return 0
	}
	i, found := slices.BinarySearchFunc(regularEpisodes(videos), episodePlace{season, number}, comparePlaces)
	if !found {
		return 0
	}
	return i + 1
}

// AbsoluteEpisodes lists the season and number of the regular episodes
// of videos, a series' description's, in the absolute numbering
// absoluteEpisode counts: the nth is the series' episode n. Local folders
// match the files named by an absolute number to them.
func AbsoluteEpisodes(videos []stremio.Video) [][2]int {
	regular := regularEpisodes(videos)
	places := make([][2]int, len(regular))
	for i, place := range regular {
		places[i] = [2]int{place.season, place.number}
	}
	return places
}

// withMissing returns ids with the identifiers more gives that ids lacks.
func withMissing(ids, more map[string]string) map[string]string {
	result := make(map[string]string, len(ids)+len(more))
	for name, value := range more {
		result[name] = value
	}
	for name, value := range ids {
		if value != "" {
			result[name] = value
		}
	}
	return result
}

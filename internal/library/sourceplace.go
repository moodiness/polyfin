package library

import (
	"context"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/stremio"
)

// Episodes another media server holds.
//
// An import from another Jellyfin, Emby or Plex server names each episode
// by the season and number that server gives it, as its own metadata and
// episode order number it: a season split otherwise, an episode left out,
// or another order, such as a DVD's, make those numbers name another of
// the addon's episodes, or none. Such a ref carries the episodes the
// server holds of the series (TitleRef.Source), and its episode is placed
// among those the addon lists, in order:
//   - by its own TMDB or TVDB identifier, on the addon's episode of that
//     identifier (a video's moviedb_id or tvdb_id); identifiers that name
//     two of the addon's episodes, or one the server's episodes share,
//     place it nowhere;
//   - else by the day it aired and its name, the server's episodes
//     standing for TMDB's in the pairing of episode numberings (see
//     alignEpisodes): episodes released the same day on both sides, as
//     many of them, in order, then by name, then the only one within a
//     day; a series whose episodes agree keeps its numbers;
//   - else at its own numbers, when the addon's episode there aired within
//     a day of it, as in a season released whole in one day, of which the
//     server holds some episodes, named in another language;
//   - else nowhere in a series shown numbered otherwise, as an episode
//     placed at other numbers, or a server showing the series in another
//     order than as aired, tell; in another, at its own numbers.
//
// The server's times are rounded to the nearest day: Jellyfin keeps the
// day an episode aired as midnight where the server is, in UTC, which is
// the evening before east of Greenwich.
//
// An episode is placed only on one the addon lists, never recorded
// otherwise, when the addon describes the series: its list is that
// description's. Without one, the episodes Polyfin listed stand for it,
// some of the series' only, as a season opened: an episode placed where
// none is listed is named the way the listed ones are, as before.

// SourceEpisode is an episode a media server holds: its numbers there, its
// own TMDB and TVDB identifiers, zero when unknown, the time it aired, zero
// when unknown, and its name.
type SourceEpisode struct {
	Season, Number int
	TMDB, TVDB     int
	Aired          time.Time
	Name           string
}

// SourceEpisodes are the episodes a media server holds of a series.
// OtherOrder tells that it shows the series in another order than as
// aired.
type SourceEpisodes struct {
	Episodes   []SourceEpisode
	OtherOrder bool
}

// nearestDay is the day nearest t, in UTC; zero for none.
func nearestDay(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	y, m, d := t.UTC().Add(12 * time.Hour).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// idEpisode is an episode the addon lists, with its own TMDB and TVDB
// identifiers, zero when it gives none.
type idEpisode struct {
	airedEpisode
	tmdb, tvdb int
}

// placement places the episodes a server holds of a series among the
// addon's, as the comment on episodes another media server holds tells.
type placement struct {
	// placed are the addon's places of the server's episodes placed, by
	// their place on the server, refused the server's episodes placed
	// nowhere, and taken the addon's places placed on.
	placed  map[episodePlace]episodePlace
	refused map[episodePlace]bool
	taken   map[episodePlace]bool
	// source and addon are each side's episodes, by place.
	source, addon map[episodePlace]airedEpisode
	// otherwise tells the series is shown numbered otherwise.
	otherwise bool
}

// placeSource places the episodes source holds among the addon's.
func placeSource(addon []idEpisode, source *SourceEpisodes) placement {
	p := placement{placed: map[episodePlace]episodePlace{}, refused: map[episodePlace]bool{}, taken: map[episodePlace]bool{},
		source: map[episodePlace]airedEpisode{}, addon: map[episodePlace]airedEpisode{}, otherwise: source.OtherOrder}
	var held []SourceEpisode
	sourceAired := make([]airedEpisode, 0, len(source.Episodes))
	for _, e := range source.Episodes {
		place := episodePlace{e.Season, e.Number}
		if _, seen := p.source[place]; seen || place.season < 0 || place.number <= 0 {
			continue
		}
		aired := airedEpisode{place: place, day: nearestDay(e.Aired), name: airedName(e.Name)}
		p.source[place] = aired
		sourceAired = append(sourceAired, aired)
		held = append(held, e)
	}
	addonAired := make([]airedEpisode, 0, len(addon))
	for _, a := range addon {
		if _, seen := p.addon[a.place]; !seen {
			p.addon[a.place] = a.airedEpisode
			addonAired = append(addonAired, a.airedEpisode)
		}
	}
	// By the episodes' own identifiers.
	claims := map[episodePlace][]episodePlace{}
	for _, e := range held {
		place := episodePlace{e.Season, e.Number}
		var found []episodePlace
		for _, a := range addon {
			if (e.TMDB > 0 && a.tmdb == e.TMDB || e.TVDB > 0 && a.tvdb == e.TVDB) && !slices.Contains(found, a.place) {
				found = append(found, a.place)
			}
		}
		switch len(found) {
		case 0:
		case 1:
			claims[found[0]] = append(claims[found[0]], place)
		default:
			p.refused[place] = true
		}
	}
	for to, from := range claims {
		if len(from) > 1 {
			for _, place := range from {
				p.refused[place] = true
			}
			continue
		}
		p.placed[from[0]], p.taken[to] = to, true
		p.otherwise = p.otherwise || from[0] != to
	}
	// Then by the day they aired and their names.
	pairs := alignEpisodes(addonAired, sourceAired)
	if pairs == nil {
		for _, e := range sourceAired {
			pairs = append(pairs, [4]int{e.place.season, e.place.number, e.place.season, e.place.number})
		}
	}
	for _, pair := range pairs {
		from, to := episodePlace{pair[0], pair[1]}, episodePlace{pair[2], pair[3]}
		if _, placed := p.placed[from]; placed || p.refused[from] || p.taken[to] {
			continue
		}
		if _, listed := p.addon[to]; !listed {
			continue
		}
		p.placed[from], p.taken[to] = to, true
		p.otherwise = p.otherwise || from != to
	}
	return p
}

// place returns the addon's place of the server's episode numbered so,
// and false when none is surely it.
func (p placement) place(season, number int) (episodePlace, bool) {
	at := episodePlace{season, number}
	if p.refused[at] {
		return at, false
	}
	if to, ok := p.placed[at]; ok {
		return to, true
	}
	if a, ok := p.addon[at]; ok && !p.taken[at] && nearDays(a, p.source[at]) {
		return at, true
	}
	return at, !p.otherwise
}

// sourceKey names the placement of a server's episodes of a series among
// those the addon lists of the series of a Stremio identifier.
type sourceKey struct {
	series string
	source *SourceEpisodes
}

// addonList is the list of the episodes the addon gives of a series, with
// their own identifiers; described tells it is the addon's description
// of the series, which lists them all.
type addonList struct {
	episodes  []idEpisode
	described bool
}

// lists reports whether the list holds an episode at place.
func (l addonList) lists(place episodePlace) bool {
	return slices.ContainsFunc(l.episodes, func(e idEpisode) bool { return e.place == place })
}

// addonLists returns the episodes the addon lists of each of series, by
// their Stremio identifiers: those of the addon's description of the
// series, which videos holds when asked already, else those the server's
// addons give, a few at a time, added to videos; else the episodes
// Polyfin listed (known).
func (s *Service) addonLists(ctx context.Context, series []string, known map[string]episodeList,
	videos map[string][]stremio.Video) (map[string]addonList, error) {
	var asked []string
	for _, id := range series {
		if len(videos[id]) == 0 {
			asked = append(asked, id)
		}
	}
	if len(asked) > 0 {
		v, err := s.serverView(ctx)()
		if err != nil {
			return nil, err
		}
		described := make([][]stremio.Video, len(asked))
		var group errgroup.Group
		group.SetLimit(describedFetches)
		for j, id := range asked {
			group.Go(func() error {
				if meta, ok := s.serverMeta(ctx, v, string(KindSeries), id); ok {
					described[j] = meta.Videos
				}
				return nil
			})
		}
		_ = group.Wait()
		for j, id := range asked {
			if len(described[j]) > 0 {
				videos[id] = described[j]
			}
		}
	}
	lists := make(map[string]addonList, len(series))
	for _, id := range series {
		var list addonList
		if len(videos[id]) > 0 {
			list.described = true
			for _, video := range videos[id] {
				list.episodes = append(list.episodes, idEpisode{airedEpisode: airedEpisode{place: episodePlace{int(video.Season), video.EpisodeNumber()},
					day: episodeDay(video.Released), name: airedName(video.DisplayName())},
					tmdb: video.MoviedbID.ID(), tvdb: video.TvdbID.ID()})
			}
		} else {
			for _, e := range known[id] {
				list.episodes = append(list.episodes, idEpisode{airedEpisode: airedEpisode{place: episodePlace{e.season, e.number}, day: e.day,
					name: e.name}, tmdb: e.tmdb, tvdb: e.tvdb})
			}
		}
		list.episodes = slices.DeleteFunc(list.episodes, func(e idEpisode) bool { return e.place.season < 0 || e.place.number <= 0 })
		lists[id] = list
	}
	return lists, nil
}

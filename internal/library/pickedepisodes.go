package library

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// EpisodeOutline is what choosing among a series' episodes reads of each
// one, before its item is described: its identifier, where it shows in
// its series' order, whether it is out, and when it aired, as its item
// tells them (see Item.Outline). Next Up chooses one or two episodes of
// each of up to 50 series of thousands of episodes: describing them all
// to choose made tens of megabytes of items at each call.
type EpisodeOutline struct {
	ID           accounts.ID
	Place        Place
	Available    bool
	PremiereDate *time.Time
}

// Outline is what choosing among episodes reads of an episode (see
// EpisodeOutline).
func (i Item) Outline() EpisodeOutline {
	return EpisodeOutline{ID: i.ID, Place: i.ShownPlace(), Available: i.Available, PremiereDate: i.PremiereDate}
}

// Outlines are the outlines of episodes, in order.
func Outlines(episodes []Item) []EpisodeOutline {
	outlines := make([]EpisodeOutline, len(episodes))
	for i, episode := range episodes {
		outlines[i] = episode.Outline()
	}
	return outlines
}

// outlinedEpisodes is a series' episodes as orderedEpisodes lists them for a
// view, of no season in particular, before their items are described:
// what describes them, their videos in the order they aired, and where
// each listed episode comes among those, in the series' order, with its
// outline. Its records and items are made from it on demand, the same as
// orderedEpisodes makes them.
type outlinedEpisodes struct {
	series      Item
	shown, meta stremio.Meta
	posters     map[int]string
	layout      *episodeLayout
	words       words
	now         time.Time
	// aired are the indexes of shown's videos, as aired; listed are the
	// places among aired of the episodes listed, in the series' order,
	// and outlines theirs.
	aired    []int
	listed   []int
	outlines []EpisodeOutline
}

// listedEpisodes lists a series' episodes for v as orderedEpisodes does,
// of no season in particular, with their outlines (see outlinedEpisodes).
func (s *Service) listedEpisodes(ctx context.Context, v view, seriesID accounts.ID) (*outlinedEpisodes, error) {
	series, shown, meta, err := s.series(ctx, v, seriesID)
	if err != nil {
		return nil, err
	}
	l := &outlinedEpisodes{series: series, shown: shown, meta: meta, posters: seasonPosters(meta), words: v.words, now: s.now(),
		aired: episodeOrder(shown, nil)}
	if !inAiredOrder(ctx) {
		l.layout = s.layoutOf(ctx, series.ID, meta, v.words)
	}
	if l.layout == nil {
		l.listed = make([]int, len(l.aired))
		for i := range l.listed {
			l.listed[i] = i
		}
	} else {
		l.listed, _ = l.layout.arrangement(len(l.aired), func(i int) string { return l.video(i).ID },
			func(i int) accounts.ID { return itemID(seasonKey(shown.ID, int(l.video(i).Season))) }, nil)
	}
	set := s.overridesNow()
	seasons := map[int]Place{}
	l.outlines = make([]EpisodeOutline, len(l.listed))
	for i, at := range l.listed {
		l.outlines[i] = l.outline(set, seasons, at)
	}
	return l, nil
}

// video is the video of the episode at place among aired.
func (l *outlinedEpisodes) video(place int) stremio.Video {
	return l.shown.Videos[l.aired[place]]
}

// outline is the outline of the episode at place among aired, as the item
// orderedEpisodes describes of it tells it, what administrators changed
// included (see overrideSet.outline), without describing it: an item
// takes a kilobyte, and every episode of the series is outlined. seasons
// holds the places of the seasons as aired, by number, made once each.
func (l *outlinedEpisodes) outline(set *overrideSet, seasons map[int]Place, place int) EpisodeOutline {
	video := l.video(place)
	number := int(video.Season)
	season, ok := seasons[number]
	if !ok {
		season = Place{SeasonID: itemID(seasonKey(l.shown.ID, number)), SeasonName: l.words.seasonName(number), Season: number}
		seasons[number] = season
	}
	date := parseDate(video.Released)
	o := EpisodeOutline{ID: itemID(episodeKey(video.ID)), Place: season, Available: releasedOn(video, date, l.now), PremiereDate: date}
	o.Place.Episode = video.EpisodeNumber()
	if l.layout != nil {
		if p, ok := l.layout.places[video.ID]; ok {
			o.Place = p
		}
	}
	set.outline(&o)
	return o
}

// item describes the episode at place among aired, as orderedEpisodes
// lists it, before its length and what administrators changed.
func (l *outlinedEpisodes) item(place int) Item {
	item := episodeItem(l.series, l.shown, l.posters, l.words, l.now, l.video(place))
	if l.layout != nil {
		l.layout.place(&item)
	}
	return item
}

// listedItems describes the episodes of l listed at the given places of
// its outlines, in that order, as orderedEpisodes describes them, with
// what administrators changed.
func (s *Service) listedItems(ctx context.Context, l *outlinedEpisodes, places []int) []Item {
	if len(places) == 0 {
		return nil
	}
	items := make([]Item, len(places))
	for i, place := range places {
		items[i] = l.item(l.listed[place])
	}
	s.episodeLengthsOf(ctx, l.meta.ID, items)
	return s.overridden(items)
}

// records are the records orderedEpisodes makes of the list: of the
// series' seasons, of its episodes as aired, the hidden specials
// included, and of the seasons of its order. With keep set, those of
// the episodes it keeps only.
func (l *outlinedEpisodes) records(keep func(accounts.ID) bool) []record {
	numbers := seasonNumbers(l.meta)
	records := make([]record, 0, len(numbers)+len(l.aired))
	for _, number := range numbers {
		records = append(records, record{ID: itemID(seasonKey(l.meta.ID, number)), Key: seasonKey(l.meta.ID, number), Kind: KindSeason,
			Parent: &l.series.ID, SeriesID: l.meta.ID, Season: number, Poster: l.posters[number]})
	}
	// The record of an episode keeps the description's video of the same
	// identifier, the last one of it as orderedEpisodes reads them.
	var videos map[string]int
	if keep == nil {
		videos = make(map[string]int, len(l.meta.Videos))
		for i, video := range l.meta.Videos {
			videos[video.ID] = i
		}
	}
	described := func(id string) stremio.Video {
		if videos != nil {
			if i, ok := videos[id]; ok {
				return l.meta.Videos[i]
			}
			return stremio.Video{}
		}
		for i := len(l.meta.Videos) - 1; i >= 0; i-- {
			if l.meta.Videos[i].ID == id {
				return l.meta.Videos[i]
			}
		}
		return stremio.Video{}
	}
	for place := range l.aired {
		shown := l.video(place)
		id := itemID(episodeKey(shown.ID))
		if keep != nil && !keep(id) {
			continue
		}
		video := described(shown.ID)
		season := itemID(seasonKey(l.shown.ID, int(shown.Season)))
		records = append(records, record{ID: id, Key: episodeKey(video.ID), Kind: KindEpisode, Parent: &season, SeriesID: l.meta.ID,
			Season: int(shown.Season), Video: &video})
	}
	if l.layout != nil {
		records = append(records, l.layout.records(l.series.ID, l.meta.ID)...)
	}
	return records
}

// PickedEpisodes lists the episodes of each of series as Episodes does,
// those pick keeps of each, given the outlines of all of them in order and
// returning the places of those it keeps among them; a series it cannot
// list is left out. The episodes kept, described in the order pick gives,
// are recorded with the series' seasons before it returns, and the others
// after the request, once per list (see episodeRecording). Next Up keeps
// one or two episodes of each of up to 50 series, of thousands of
// episodes, and describes only those. It fails when the episodes kept
// could not be recorded.
func (s *Service) PickedEpisodes(ctx context.Context, user accounts.User, series []accounts.ID,
	pick func(series accounts.ID, episodes []EpisodeOutline) []int) (map[accounts.ID][]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	if ctx, err = s.withPreloaded(ctx, series); err != nil {
		return nil, err
	}
	found := make([]*outlinedEpisodes, len(series))
	var g errgroup.Group
	g.SetLimit(6)
	for i, id := range series {
		g.Go(func() error {
			if l, err := s.listedEpisodes(ctx, v, id); err == nil {
				found[i] = l
			}
			return nil
		})
	}
	_ = g.Wait()
	result := make(map[accounts.ID][]Item, len(series))
	var records []record
	later := map[accounts.ID]*outlinedEpisodes{}
	for i, id := range series {
		l := found[i]
		if l == nil {
			continue
		}
		places := pick(id, l.outlines)
		result[id] = s.listedItems(ctx, l, places)
		kept := make(map[accounts.ID]bool, len(places))
		for _, place := range places {
			kept[l.outlines[place].ID] = true
		}
		records = append(records, l.records(func(id accounts.ID) bool { return kept[id] })...)
		if len(places) < len(l.outlines) {
			later[id] = l
		}
	}
	// The others are recorded after the request, as the series' listing
	// would (see episodeRecording).
	s.recordEpisodesLater(ctx, later)
	return result, s.save(ctx, records)
}

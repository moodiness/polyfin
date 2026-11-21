package library

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Episodes asked for streams by another numbering.
//
// A stream addon asked for an episode by its series' IMDb or TMDB
// identifier (see streamRequests) reads its season and number as that
// identifier's numbering has them. The metadata addon that lists the
// series may number its episodes otherwise: in TVDB's order, a season
// split in two or two seasons merged, specials counted among the regular
// episodes, or one absolute run. Such an episode is asked by the season
// and number the same episode has on the other side, as the pairing of
// episode numberings finds it, by the day episodes aired and their names
// (see episodealign.go):
//   - by TMDB's identifier ("tmdb:<id>:<season>:<number>"), as TMDB
//     numbers it, the alignment tracking uses (see TMDBNumbering);
//   - by IMDb's ("tt…:<season>:<number>"), as the server's metadata
//     addons number the series described by its IMDb identifier: the
//     numbers Polyfin asks for when it lists the series by that
//     identifier. Polyfin has no other source of IMDb's numbering.
//
// Only an episode surely paired is asked by other numbers. A series
// numbered alike on both sides, one nothing numbers on the other side
// yet, and an episode the pairing leaves alone are asked by their own
// numbers, as before. A video without an episode number is paired with
// none. A series named by its IMDb or TMDB identifier is asked by that
// identifier as its own addon numbers it.
//
// The pairs of a series are pinned in memory for streamPinned, made from
// what is kept only: the series' alignment on TMDB's numbering, a week
// old at most, else older for as many episodes, while it is made again
// (episode_alignments), and the description of its IMDb identifier kept
// as other descriptions are. Asking for streams never waits on TMDB or an
// addon for them: what is missing is asked in the background, once at a
// time per series, and the series' next request uses it.

// streamPinned is how long the pairs a series' stream requests are
// numbered by are kept in memory before they are read again from what is
// kept: an addon's new episodes are paired once read again.
const streamPinned = time.Hour

// streamPinsHeld bounds how many series' pairs are held: past it, all are
// read again.
const streamPinsHeld = 4096

// streamPin is how a series' episodes are numbered in its stream requests
// by IMDb's identifier and by TMDB's, from the addon's numbering: each
// zero, alike, when nothing tells otherwise; made when it was made.
type streamPin struct {
	imdb, tmdb EpisodeNumbering
	made       time.Time
}

// streamNumberings are the pins of the series, by their Stremio
// identifiers. Its zero value holds none.
type streamNumberings struct {
	mu   sync.Mutex
	pins map[string]streamPin
	// filled, when tests set it, is called with the series once the
	// background work a pin asked for ends.
	filled func(series string)
}

// get returns the pin of series, unless older than streamPinned at now.
func (n *streamNumberings) get(series string, now time.Time) (streamPin, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	pin, ok := n.pins[series]
	if !ok || now.Sub(pin.made) >= streamPinned {
		return streamPin{}, false
	}
	return pin, true
}

// put pins the pairs of series.
func (n *streamNumberings) put(series string, pin streamPin) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.pins == nil || len(n.pins) >= streamPinsHeld {
		n.pins = map[string]streamPin{}
	}
	n.pins[series] = pin
}

// forget drops the pin of series: its next request makes it again.
func (n *streamNumberings) forget(series string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.pins, series)
}

// asked returns the season and number an episode the addon numbers so is
// asked by on n's other side: those it pairs it with, else its own.
func (n EpisodeNumbering) asked(season, number int) (int, int) {
	if other, otherNumber, ok := n.ToTMDB(season, number); ok {
		return other, otherNumber
	}
	return season, number
}

// streamNumbering returns how the episode t's stream requests by IMDb's
// identifier imdb and by TMDB's tmdbID, each empty when unknown, number
// it: the pin of its series, made when missing from what is kept. series
// is the series' complete description, nil when unknown.
func (s *Service) streamNumbering(ctx context.Context, t target, imdb, tmdbID string, series *stremio.Meta) streamPin {
	if t.kind != KindEpisode || t.holder == nil || t.holder.Kind != KindSeries {
		return streamPin{}
	}
	id := strings.TrimPrefix(t.holder.Key, string(KindSeries)+"|")
	number := describedNumber(stremio.Text(tmdbID))
	if number <= 0 && (!imdbID.MatchString(imdb) || imdb == id) {
		return streamPin{}
	}
	if pin, ok := s.streamNumberings.get(id, s.now()); ok {
		return pin
	}
	pin, ok := s.makeStreamPin(ctx, id, imdb, number, series)
	if ok {
		s.streamNumberings.put(id, pin)
	}
	return pin
}

// makeStreamPin makes the pin of the series Polyfin names by the Stremio
// identifier series, whose IMDb identifier is imdb and TMDB's tmdbID,
// empty and 0 when unknown, from what is kept, and asks in the background
// for what is not. described is the series' complete description, nil
// when unknown. It reports false when what is kept could not be read.
func (s *Service) makeStreamPin(ctx context.Context, series, imdb string, tmdbID int, described *stremio.Meta) (streamPin, bool) {
	known, err := s.knownEpisodes(ctx, []string{series})
	if err != nil {
		return streamPin{}, false
	}
	listed := known[series]
	pin := streamPin{made: s.now()}
	if tmdbID > 0 {
		n, exact := exactNumbering(series, listed)
		fresh := exact
		if !exact {
			kept, err := s.keptAlignment(ctx, series)
			if err != nil {
				return streamPin{}, false
			}
			if kept.pairs(tmdbID, len(listedEpisodes(listed))) {
				n, fresh = numberingOf(kept.places), s.now().Sub(kept.askedAt) < alignmentFresh
			}
		}
		pin.tmdb = n
		if !fresh {
			// Made again, a kept alignment forgets the pin.
			s.fillStreamPin(ctx, "tmdb", series, func(ctx context.Context) {
				_, _ = s.numbering(ctx, s.serverView(ctx), series, tmdbID, imdb, listed)
			})
		}
	}
	if imdbID.MatchString(imdb) && imdb != series {
		v, err := s.view(ctx, accounts.User{})
		if err != nil {
			return streamPin{}, false
		}
		meta, kept, served := s.keptServerMeta(ctx, v, string(KindSeries), imdb)
		switch {
		case kept:
			addon := listedEpisodes(listed)
			if described != nil {
				addon = videoEpisodes(described.Videos, false)
			}
			if other := videoEpisodes(meta.Videos, false); len(addon) > 0 && len(other) > 0 {
				pin.imdb = numberingOf(alignEpisodes(addon, other))
			}
		case served:
			s.fillStreamPin(ctx, "imdb", series, func(ctx context.Context) {
				if _, ok := s.serverMeta(ctx, v, string(KindSeries), imdb); ok {
					s.streamNumberings.forget(series)
				}
			})
		}
	}
	return pin, true
}

// keptServerMeta returns the description serverMeta gives of a title, as
// far as it is kept, asking no addon: that of the first of the addons of
// v, the server's view, that describe the title. served reports whether
// any does, kept whether its description is kept.
func (s *Service) keptServerMeta(ctx context.Context, v view, metaType, id string) (meta stremio.Meta, kept, served bool) {
	for _, candidate := range v.addons {
		if own(candidate.addon) || !v.factual(candidate, metaType, id, false) || !serves(candidate, "meta", metaType, id) {
			continue
		}
		meta, kept = s.keptDescription(ctx, candidate, metaKey{candidate.addon.ID, metaType, id}, inDatabase)
		return meta, kept, true
	}
	return stremio.Meta{}, false, false
}

// fillStreamPin runs fill in the background for the pin of series, once
// at a time for what, as background work (see background).
func (s *Service) fillStreamPin(ctx context.Context, what, series string, fill func(context.Context)) {
	s.background(ctx, "stream numbering "+what+" "+series, func() bool { return true }, func(ctx context.Context) error {
		fill(ctx)
		if filled := s.streamNumberings.filled; filled != nil {
			filled(series)
		}
		return nil
	})
}

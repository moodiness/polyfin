package library

import (
	"bytes"
	"cmp"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/xmltv"
)

// airingSpan is the longest listing of programmes read as one that airs
// within a short time (see airingGuidePrograms): what airs now on each
// channel, which channel listings, On Now and players ask for.
const airingSpan = time.Hour

// guideSpan is how long the longest programme of a guide's download
// lasts, by download.
type guideSpan struct {
	generation int
	longest    time.Duration
}

// guideSpans remembers the longest programme of each guide's current
// download (see airingGuidePrograms): a download never changes once
// current, and a new one has its own generation.
var guideSpans = struct {
	sync.Mutex
	byGuide map[accounts.ID]guideSpan
}{byGuide: map[accounts.ID]guideSpan{}}

// longestProgrammes returns the guide span of each of guides' current
// downloads, read once per download.
func (s *Service) longestProgrammes(ctx context.Context, guides []accounts.ID) (map[accounts.ID]guideSpan, error) {
	rows, err := s.db.Query(ctx, "SELECT id, generation FROM live_guides WHERE id = ANY($1::uuid[])", guides)
	if err != nil {
		return nil, err
	}
	spans := make(map[accounts.ID]guideSpan, len(guides))
	var missing []accounts.ID
	var generations []int
	var id accounts.ID
	var generation int
	guideSpans.Lock()
	_, err = pgx.ForEachRow(rows, []any{&id, &generation}, func() error {
		if span, ok := guideSpans.byGuide[id]; ok && span.generation == generation {
			spans[id] = span
		} else {
			missing, generations = append(missing, id), append(generations, generation)
		}
		return nil
	})
	guideSpans.Unlock()
	if err != nil || len(missing) == 0 {
		return spans, err
	}
	rows, err = s.db.Query(ctx, `SELECT m.guide_id, m.generation, coalesce(extract(epoch FROM max(p.ends_at - p.starts_at)), 0)::bigint
		FROM unnest($1::uuid[], $2::int[]) AS m (guide_id, generation)
		LEFT JOIN live_guide_programmes p ON p.guide_id = m.guide_id AND p.generation = m.generation
		GROUP BY m.guide_id, m.generation`, missing, generations)
	if err != nil {
		return nil, err
	}
	var seconds int64
	read := map[accounts.ID]guideSpan{}
	if _, err := pgx.ForEachRow(rows, []any{&id, &generation, &seconds}, func() error {
		read[id] = guideSpan{generation: generation, longest: time.Duration(seconds) * time.Second}
		return nil
	}); err != nil {
		return nil, err
	}
	guideSpans.Lock()
	defer guideSpans.Unlock()
	for id, span := range read {
		guideSpans.byGuide[id] = span
		spans[id] = span
	}
	return spans, nil
}

// airingGuidePrograms lists what guidePrograms lists for a window of at
// most airingSpan, read from each channel's programmes that may still air
// in it: those that started after the window's start, less the guide's
// longest programme. A programme read otherwise from every one of the
// channel's programmes since yesterday, to find the one airing now.
func (s *Service) airingGuidePrograms(ctx context.Context, v view, channels map[string]Item, skip map[string]bool, from, to time.Time,
	keep func(Item) bool) ([]Item, error) {
	order := map[addons.LibraryKey]int{}
	for i, src := range v.channels {
		if src.guide {
			key := addons.LibraryKey{AddonID: src.addon.addon.ID, CatalogType: src.catalog.Type, CatalogID: src.catalog.ID}
			if _, ok := order[key]; !ok {
				order[key] = i
			}
		}
	}
	if len(order) == 0 {
		return nil, nil
	}
	byID := map[accounts.ID]Item{}
	ids := make([]accounts.ID, 0, len(channels))
	for stremioID, channel := range channels {
		if !skip[stremioID] {
			byID[channel.ID] = channel
			ids = append(ids, channel.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	// Each channel's mapping: that of the first of its catalogs, as
	// guidePrograms maps it.
	rows, err := s.db.Query(ctx, `SELECT channel_id, addon_id, catalog_type, catalog_id, guide_id, xmltv_id FROM live_guide_maps
		WHERE channel_id = ANY($1::uuid[]) AND guide_id IS NOT NULL`, ids)
	if err != nil {
		return nil, err
	}
	type mapping struct {
		guide   accounts.ID
		xmltvID string
		catalog int
	}
	mapped := map[accounts.ID]mapping{}
	var channel accounts.ID
	var key addons.LibraryKey
	var m mapping
	if _, err := pgx.ForEachRow(rows, []any{&channel, &key.AddonID, &key.CatalogType, &key.CatalogID, &m.guide, &m.xmltvID}, func() error {
		index, ok := order[key]
		if current, seen := mapped[channel]; ok && (!seen || index < current.catalog) {
			m.catalog = index
			mapped[channel] = m
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if len(mapped) == 0 {
		return nil, nil
	}
	channelIDs := slices.Collect(maps.Keys(mapped))
	slices.SortFunc(channelIDs, func(a, b accounts.ID) int {
		return cmp.Or(CompareChannelNumbers(byID[a].Number, byID[b].Number), bytes.Compare(a[:], b[:]))
	})
	guides := map[accounts.ID]bool{}
	for _, m := range mapped {
		guides[m.guide] = true
	}
	spans, err := s.longestProgrammes(ctx, slices.Collect(maps.Keys(guides)))
	if err != nil {
		return nil, err
	}
	n := len(channelIDs)
	guideIDs, xmltvIDs, ranks := make([]accounts.ID, n), make([]string, n), make([]int, n)
	generations, since := make([]int, n), make([]time.Time, n)
	for i, id := range channelIDs {
		m := mapped[id]
		guideIDs[i], xmltvIDs[i], ranks[i] = m.guide, m.xmltvID, i
		// A guide downloaded anew since its span was read is read whole
		// from yesterday, as guidePrograms reads it.
		span, ok := spans[m.guide]
		if !ok {
			span.generation = -1
		}
		generations[i], since[i] = span.generation, from.Add(-span.longest-time.Second)
	}
	// Each channel's programmes are read by the guide's index, from the
	// earliest start that may still air in the window.
	rows, err = s.db.Query(ctx, `SELECT c.channel_id, c.guide_id, c.xmltv_id, p.starts_at, p.ends_at, p.title, p.subtitle, p.description,
		p.categories, coalesce(p.season, 0), coalesce(p.episode, 0), p.icon, p.is_new, p.previously_shown
		FROM unnest($1::uuid[], $2::uuid[], $3::text[], $4::int[], $5::int[], $6::timestamptz[])
			AS c (channel_id, guide_id, xmltv_id, rank, generation, since)
		JOIN live_guides g ON g.id = c.guide_id
		JOIN live_guide_programmes p ON p.guide_id = g.id AND p.generation = g.generation AND p.xmltv_id = c.xmltv_id
			AND p.starts_at > CASE WHEN g.generation = c.generation THEN c.since ELSE '-infinity' END
		WHERE p.ends_at > $7 AND p.starts_at < $8
		ORDER BY p.starts_at, c.rank`, channelIDs, guideIDs, xmltvIDs, ranks, generations, since, from, to)
	if err != nil {
		return nil, err
	}
	type start struct {
		channel accounts.ID
		at      int64
	}
	starts := map[start]bool{}
	var items []Item
	var p xmltv.Programme
	var guide accounts.ID
	var xmltvID string
	_, err = pgx.ForEachRow(rows, []any{&channel, &guide, &xmltvID, &p.Start, &p.Stop, &p.Title, &p.SubTitle, &p.Description, &p.Categories,
		&p.Season, &p.Episode, &p.Icon, &p.New, &p.PreviouslyShown}, func() error {
		if starts[start{channel, p.Start.UnixNano()}] {
			return nil
		}
		starts[start{channel, p.Start.UnixNano()}] = true
		program, ok := guideItem(byID[channel], p)
		if !ok {
			return nil
		}
		s.rememberProgram(program.ID, programRef{channel: byID[channel].StremioID, guide: guide, xmltvID: xmltvID, start: p.Start})
		if keep(program) {
			items = append(items, program)
		}
		return nil
	})
	return items, err
}

// firstChannels is how many channels a listing by channel number with a
// limit reads the programmes of first (see firstChannelsPrograms).
const firstChannels = 100

// firstChannelsPrograms lists, for a listing by channel number with a
// limit, the programmes of the first of channels by number, a slice of
// them at a time, first of at least first channels, twice as many each
// time, until q.Limit are kept. A slice ends with every channel of its
// last one's number: the channels left all have higher numbers, so the
// first q.Limit programmes are those a listing of every channel would
// list first.
func firstChannelsPrograms(channels map[string]Item, q GuideQuery, first int,
	listed func(map[string]Item, GuideQuery) ([]Item, error)) ([]Item, error) {
	ordered := slices.SortedFunc(maps.Values(channels), func(a, b Item) int {
		return cmp.Or(CompareChannelNumbers(a.Number, b.Number), bytes.Compare(a.ID[:], b.ID[:]))
	})
	var programs []Item
	for start, size := 0, max(q.Limit, first); start < len(ordered); size *= 2 {
		end := min(start+size, len(ordered))
		for end < len(ordered) && CompareChannelNumbers(ordered[end].Number, ordered[end-1].Number) == 0 {
			end++
		}
		part := make(map[string]Item, end-start)
		for _, channel := range ordered[start:end] {
			part[channel.StremioID] = channel
		}
		found, err := listed(part, GuideQuery{From: q.From, To: q.To, Keep: q.Keep})
		if err != nil {
			return nil, err
		}
		if programs = append(programs, found...); len(programs) >= q.Limit {
			break
		}
		start = end
	}
	return programs, nil
}

// sortPrograms orders programmes by start time then channel number, or,
// byChannel, by channel number then start time. Listings mostly read them
// in order already.
func sortPrograms(programs []Item, byChannel bool) {
	compare := func(a, b Item) int {
		if c := a.StartDate.Compare(*b.StartDate); c != 0 {
			return c
		}
		return CompareChannelNumbers(a.Channel.Number, b.Channel.Number)
	}
	if byChannel {
		compare = func(a, b Item) int {
			if c := CompareChannelNumbers(a.Channel.Number, b.Channel.Number); c != 0 {
				return c
			}
			return a.StartDate.Compare(*b.StartDate)
		}
	}
	if !slices.IsSortedFunc(programs, compare) {
		slices.SortStableFunc(programs, compare)
	}
}

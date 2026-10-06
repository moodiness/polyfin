package library

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/xmltv"
)

// guideDays is how many days of programmes a guide may span from today,
// as far as Jellyfin's guide goes by default.
const guideDays = 7

// LiveCatalog reports whether catalogs of a Stremio type list live TV
// channels rather than titles: the tv type, whose streams are live.
func LiveCatalog(catalogType string) bool { return catalogType == "tv" }

// HasChannels reports whether the user has live TV catalogs, without asking
// their addons.
func (s *Service) HasChannels(ctx context.Context, user accounts.User) (bool, error) {
	v, err := s.view(ctx, user)
	return len(v.channels) > 0, err
}

// Channels lists the user's live TV channels: those of their live TV
// catalogs, in catalog order, each once. Stremio gives channels no number:
// a channel is numbered by its place in the list, unless its IPTV source
// gives it one.
func (s *Service) Channels(ctx context.Context, user accounts.User) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	channels, records := s.channels(ctx, v)
	return s.overridden(channels), s.save(ctx, records)
}

// channels lists the channels of a view, with the records that find them
// again. A catalog that cannot be read is left out, as from a collection.
func (s *Service) channels(ctx context.Context, v view) ([]Item, []record) {
	lists := make([][]stremio.Meta, len(v.channels))
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for i, src := range v.channels {
		group.Go(func() error {
			metas, _, err := s.window(ctx, v, src, 0, v.limit(src))
			if err != nil && ctx.Err() == nil {
				s.logger.Warn("A live TV catalog could not be listed", "catalog", src.catalog.ID, "error", err)
			}
			lists[i] = metas
			return nil
		})
	}
	_ = group.Wait()
	var items []Item
	var records []record
	seen := map[string]bool{}
	for i, metas := range lists {
		src := v.channels[i]
		addon := src.addon.addon.ID
		for _, meta := range metas {
			if meta.ID == "" || seen[meta.ID] {
				continue
			}
			seen[meta.ID] = true
			if meta.Type == "" {
				meta.Type = src.catalog.Type
			}
			number := len(items) + 1
			if meta.ChannelNumber > 0 {
				number = meta.ChannelNumber
			}
			item := channelItem(meta, number)
			preview := meta
			items = append(items, item)
			records = append(records, record{ID: item.ID, Key: channelKey(meta.ID), Kind: KindChannel, Addon: &addon,
				CatalogType: src.catalog.Type, CatalogID: src.catalog.ID, Meta: &preview, Number: number, Confined: src.addon.confined})
		}
	}
	return items, records
}

// channelItem describes a channel from its catalog entry. TV addons give a
// channel's logo as its poster, or only as its logo.
func channelItem(meta stremio.Meta, number int) Item {
	item := Item{ID: itemID(channelKey(meta.ID)), Kind: KindChannel, Number: strconv.Itoa(number), Available: true}
	fromMeta(&item, meta)
	item.Images = channelImages(meta)
	// A channel has no release, runtime or credits of its own.
	item.ProductionYear, item.PremiereDate, item.EndDate, item.Runtime, item.People = 0, nil, nil, 0, nil
	return item
}

func channelImages(meta stremio.Meta) Images {
	return Images{Primary: cmp.Or(meta.Poster, meta.Logo), Backdrop: meta.Background}
}

// channel describes a channel listed before, from what Polyfin remembers
// of it, when the user still has the live TV catalog that listed it:
// players ask for it with every segment, which must not list the catalogs
// again. An IPTV source answers at once whether it still shows the
// channel: its groups may have been chosen since.
func (s *Service) channel(ctx context.Context, v view, r record) (Item, error) {
	index := slices.IndexFunc(v.channels, func(src source) bool {
		return r.Addon != nil && src.addon.addon.ID == *r.Addon && src.catalog.Type == r.CatalogType && src.catalog.ID == r.CatalogID
	})
	if r.Kind != KindChannel || r.Meta == nil || index < 0 {
		return Item{}, ErrNotFound
	}
	if src := v.channels[index]; src.addon.addon.IPTV() {
		if _, err := s.fetchMeta(ctx, src.addon, r.CatalogType, r.Meta.ID); errors.Is(err, stremio.ErrNotFound) {
			return Item{}, ErrNotFound
		} else if err != nil {
			return Item{}, err
		}
	}
	return channelItem(*r.Meta, r.Number), nil
}

// Programs lists the programmes of the user's channels that overlap
// [from, to), by start time (see Guide).
func (s *Service) Programs(ctx context.Context, user accounts.User, from, to time.Time) ([]Item, error) {
	return s.Guide(ctx, user, GuideQuery{From: from, To: to})
}

// GuideQuery narrows a listing of programmes.
type GuideQuery struct {
	// From and To bound the programmes listed: those overlapping them.
	From, To time.Time
	// Channels keeps the programmes of these channels; nil keeps every
	// channel's.
	Channels []accounts.ID
	// Keep keeps the programmes it accepts; nil keeps all.
	Keep func(Item) bool
	// Limit, when positive, lists the first Limit programmes kept, by
	// start time then channel number, and reads the guide no further.
	Limit int
}

// Guide lists the programmes of the user's channels that q asks for, by
// start time, from the guides of the addons that publish one: Stremio's
// Native EPG, where a live TV catalog asked for a UTC day lists its
// channels with that day's programmes. Only the days from yesterday to
// guideDays ahead are asked for, each day's pages once in a while (see
// catalogLife). The channels without Native EPG programmes take those of
// their catalog's XMLTV guide, when it has one (see guidePrograms).
//
// Programmes of XMLTV guides are read from the guide each time and kept as
// no item: one opened by its identifier is found again from the guide (see
// unsavedProgram). Native EPG programmes, which only their addon gives
// again, are kept as items once listed.
func (s *Service) Guide(ctx context.Context, user accounts.User, q GuideQuery) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	all, channelRecords := s.channels(ctx, v)
	var wanted map[accounts.ID]bool
	if q.Channels != nil {
		wanted = make(map[accounts.ID]bool, len(q.Channels))
		for _, id := range q.Channels {
			wanted[id] = true
		}
	}
	byStremioID := make(map[string]Item, len(all))
	for _, channel := range all {
		if wanted == nil || wanted[channel.ID] {
			byStremioID[channel.StremioID] = channel
		}
	}
	keep := func(p Item) bool { return q.Keep == nil || q.Keep(p) }
	from, to := q.From, q.To
	today := s.now().UTC().Truncate(24 * time.Hour)
	var guides []source
	first, last := from.UTC().Truncate(24*time.Hour), to.UTC()
	if earliest := today.AddDate(0, 0, -1); first.Before(earliest) {
		first = earliest
	}
	if latest := today.AddDate(0, 0, guideDays+1); last.After(latest) {
		last = latest
	}
	for _, src := range v.channels {
		if !src.addon.addon.Manifest.BehaviorHints.EpgProvider ||
			!slices.ContainsFunc(src.catalog.Extra, func(extra stremio.Extra) bool { return extra.Name == "date" }) {
			continue
		}
		for day := first; day.Before(last); day = day.AddDate(0, 0, 1) {
			dated := src
			dated.date = day.Format(time.DateOnly)
			guides = append(guides, dated)
		}
	}
	var mu sync.Mutex
	var programs []Item
	var records []record
	seen := map[accounts.ID]bool{}
	// native are the channels with Native EPG programmes, aired or not.
	native := map[string]bool{}
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for _, src := range guides {
		group.Go(func() error {
			metas, _, err := s.window(ctx, v, src, 0, v.limit(src))
			if err != nil && ctx.Err() == nil {
				s.logger.Warn("A live TV guide could not be read", "catalog", src.catalog.ID, "date", src.date, "error", err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, meta := range metas {
				channel, ok := byStremioID[meta.ID]
				if !ok {
					continue
				}
				for _, video := range meta.Videos {
					program, ok := programItem(channel, video, "")
					if ok {
						native[meta.ID] = true
					}
					// A programme over midnight is on the guide of both days.
					if !ok || seen[program.ID] || !program.EndDate.After(from) || !program.StartDate.Before(to) || !keep(program) {
						continue
					}
					seen[program.ID] = true
					programs = append(programs, program)
					records = append(records, record{ID: program.ID, Key: programKey(channel.StremioID, programmeID(video)), Kind: KindProgram,
						Parent: &channel.ID, Channel: channel.StremioID, Video: &video, Confined: src.addon.confined})
				}
			}
			return nil
		})
	}
	_ = group.Wait()
	guided, err := s.guidePrograms(ctx, v, byStremioID, native, q, keep)
	if err != nil {
		return nil, err
	}
	for _, program := range guided {
		if !seen[program.ID] {
			seen[program.ID] = true
			programs = append(programs, program)
		}
	}
	slices.SortStableFunc(programs, func(a, b Item) int {
		if c := a.StartDate.Compare(*b.StartDate); c != 0 {
			return c
		}
		return comparedNumbers(a.Channel.Number, b.Channel.Number)
	})
	if q.Limit > 0 && len(programs) > q.Limit {
		programs = programs[:q.Limit]
	}
	// Only the Native EPG programmes listed are kept, and the channels of
	// the programmes listed, which their programmes are found again by.
	listed, channels := map[accounts.ID]bool{}, map[string]bool{}
	for _, program := range programs {
		listed[program.ID], channels[program.Channel.StremioID] = true, true
	}
	records = slices.DeleteFunc(records, func(r record) bool { return !listed[r.ID] })
	for _, r := range channelRecords {
		if r.Meta != nil && channels[r.Meta.ID] {
			records = append(records, r)
		}
	}
	return s.overridden(programs), s.save(ctx, records)
}

func comparedNumbers(a, b string) int {
	x, _ := strconv.Atoi(a)
	y, _ := strconv.Atoi(b)
	return x - y
}

// programmeID identifies a programme within its channel: by the addon's
// identifier, which the Native EPG requires, else by its start.
func programmeID(video stremio.Video) string {
	return cmp.Or(video.ID, video.StartTime)
}

// programItem describes a programme of a channel's guide, with the title
// of its episode when the guide gives one. A video is a programme when it
// has a start and a later end.
func programItem(channel Item, video stremio.Video, episodeTitle string) (Item, bool) {
	start, end := parseDate(video.StartTime), parseDate(video.EndTime)
	if start == nil || end == nil || !end.After(*start) {
		return Item{}, false
	}
	item := Item{
		ID:                itemID(programKey(channel.StremioID, programmeID(video))),
		Kind:              KindProgram,
		Name:              video.DisplayName(),
		ParentID:          channel.ID,
		Overview:          video.Overview,
		Genres:            slices.Clip(video.Genres),
		StartDate:         start,
		EndDate:           end,
		Runtime:           end.Sub(*start),
		Images:            Images{Primary: video.Thumbnail},
		IndexNumber:       video.EpisodeNumber(),
		ParentIndexNumber: int(video.Season),
		Available:         true,
		StremioType:       channel.StremioType,
		StremioID:         channel.StremioID,
		Channel:           &channel,
		EpisodeTitle:      episodeTitle,
	}
	return item, true
}

// program describes a programme listed before, on one of the user's
// channels.
func (s *Service) program(ctx context.Context, v view, r record) (Item, error) {
	if r.Video == nil {
		return Item{}, ErrNotFound
	}
	stored, err := s.load(ctx, itemID(channelKey(r.Channel)))
	if err != nil {
		return Item{}, err
	}
	channel, err := s.channel(ctx, v, stored)
	if err != nil {
		return Item{}, err
	}
	program, ok := programItem(channel, *r.Video, r.EpisodeTitle)
	if !ok {
		return Item{}, ErrNotFound
	}
	return program, nil
}

// programRef finds an XMLTV programme again: its channel's Stremio
// identifier, and the guide channel and start it has in the guide.
type programRef struct {
	channel string
	guide   accounts.ID
	xmltvID string
	start   time.Time
}

// programRefs remembers the XMLTV programmes listed lately by their
// identifier, for apps that open one; past it, or after a restart, the
// guide is searched (see unsavedProgram).
var programRefs = cache.New[accounts.ID, programRef](200_000, 24*time.Hour)

func rememberProgram(id accounts.ID, ref programRef) { programRefs.Put(id, ref) }

// unsavedProgram describes an XMLTV programme of one of the user's
// channels by its identifier, which no item keeps: from the programmes
// listed lately, else by searching the guides' programmes of the channels
// mapped to one. An identifier of no programme is ErrNotFound.
func (s *Service) unsavedProgram(ctx context.Context, v view, id accounts.ID) (Item, error) {
	ref, ok := programRefs.Get(id)
	if !ok {
		var found bool
		var err error
		if ref, found, err = s.searchProgram(ctx, id); err != nil || !found {
			if err == nil {
				err = ErrNotFound
			}
			return Item{}, err
		}
	}
	stored, err := s.load(ctx, itemID(channelKey(ref.channel)))
	if err != nil {
		return Item{}, err
	}
	channel, err := s.channel(ctx, v, stored)
	if err != nil {
		return Item{}, err
	}
	var p xmltv.Programme
	err = s.db.QueryRow(ctx, `SELECT p.starts_at, p.ends_at, p.title, p.subtitle, p.description, p.categories, coalesce(p.season, 0),
		coalesce(p.episode, 0), p.icon FROM live_guide_programmes p JOIN live_guides g ON g.id = p.guide_id AND p.generation = g.generation
		WHERE p.guide_id = $1 AND p.xmltv_id = $2 AND p.starts_at = $3 LIMIT 1`, ref.guide, ref.xmltvID, ref.start).
		Scan(&p.Start, &p.Stop, &p.Title, &p.SubTitle, &p.Description, &p.Categories, &p.Season, &p.Episode, &p.Icon)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	program, ok := programItem(channel, guideVideo(p), p.SubTitle)
	if !ok || program.ID != id {
		return Item{}, ErrNotFound
	}
	rememberProgram(id, ref)
	return s.overriddenItem(program), nil
}

// searchProgram finds the guide programme an identifier names among those
// of the channels mapped to a guide: an identifier is a hash of its
// channel's Stremio identifier and its start (see programKey).
func (s *Service) searchProgram(ctx context.Context, id accounts.ID) (programRef, bool, error) {
	var ref programRef
	err := s.db.QueryRow(ctx, `SELECT c.data->'meta'->>'id', p.guide_id, p.xmltv_id, p.starts_at
		FROM live_guide_maps m JOIN items c ON c.id = m.channel_id AND c.kind = 'channel'
		JOIN live_guides g ON g.id = m.guide_id
		JOIN live_guide_programmes p ON p.guide_id = g.id AND p.generation = g.generation AND p.xmltv_id = m.xmltv_id
		WHERE substring(sha256(convert_to('polyfin:item:program|' || (c.data->'meta'->>'id') || '|' ||
			to_char(p.starts_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), 'UTF8')) FOR 16) = $1 LIMIT 1`, id[:]).
		Scan(&ref.channel, &ref.guide, &ref.xmltvID, &ref.start)
	if errors.Is(err, pgx.ErrNoRows) {
		return programRef{}, false, nil
	}
	return ref, err == nil, err
}

// SweepPrograms deletes the programmes kept as items that ended more than
// two days ago and that no timer or recording names: Native EPG programmes
// listed, and those of guides kept before they no longer were.
func (s *Service) SweepPrograms(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM items WHERE kind = 'program'
		AND CASE WHEN data->'video'->>'endTime' ~ '^\d{4}-\d\d-\d\dT\d\d:\d\d' THEN (data->'video'->>'endTime')::timestamptz < $1 ELSE false END
		AND id NOT IN (SELECT program_id FROM live_timers WHERE program_id IS NOT NULL
			UNION SELECT program_id FROM live_series_timers WHERE program_id IS NOT NULL
			UNION SELECT program_id FROM live_recordings WHERE program_id IS NOT NULL)`, s.now().Add(-48*time.Hour))
	return tag.RowsAffected(), err
}

package library

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
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
// a channel is numbered by its place in the list.
func (s *Service) Channels(ctx context.Context, user accounts.User) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	channels, records := s.channels(ctx, v)
	return channels, s.save(ctx, records)
}

// channels lists the channels of a view, with the records that find them
// again. A catalog that cannot be read is left out, as from a collection.
func (s *Service) channels(ctx context.Context, v view) ([]Item, []record) {
	lists := make([][]stremio.Meta, len(v.channels))
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for i, src := range v.channels {
		group.Go(func() error {
			metas, _, err := s.window(ctx, src, 0, maxCrawl)
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
			item := channelItem(meta, len(items)+1)
			preview := meta
			items = append(items, item)
			records = append(records, record{ID: item.ID, Key: channelKey(meta.ID), Kind: KindChannel, Addon: &addon,
				CatalogType: src.catalog.Type, CatalogID: src.catalog.ID, Meta: &preview, Confined: src.addon.confined})
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

// channel finds one of the user's channels.
func (s *Service) channel(ctx context.Context, v view, id accounts.ID) (Item, error) {
	channels, _ := s.channels(ctx, v)
	i := slices.IndexFunc(channels, func(c Item) bool { return c.ID == id })
	if i < 0 {
		return Item{}, ErrNotFound
	}
	return channels[i], nil
}

// Programs lists the programmes of the user's channels that overlap
// [from, to), by start time, from the guides of the addons that publish
// one: Stremio's Native EPG, where a live TV catalog asked for a UTC day
// lists its channels with that day's programmes. Only the days from
// yesterday to guideDays ahead are asked for, each day's pages once in a
// while (see pageTTL).
func (s *Service) Programs(ctx context.Context, user accounts.User, from, to time.Time) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	channels, records := s.channels(ctx, v)
	byStremioID := make(map[string]Item, len(channels))
	for _, channel := range channels {
		byStremioID[channel.StremioID] = channel
	}
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
	seen := map[accounts.ID]bool{}
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for _, src := range guides {
		group.Go(func() error {
			metas, _, err := s.window(ctx, src, 0, maxCrawl)
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
					program, ok := programItem(channel, video)
					// A programme over midnight is on the guide of both days.
					if !ok || seen[program.ID] || !program.EndDate.After(from) || !program.StartDate.Before(to) {
						continue
					}
					seen[program.ID] = true
					programs = append(programs, program)
					records = append(records, record{ID: program.ID, Key: programKey(channel.StremioID, programmeID(video)), Kind: KindProgram,
						Parent: &channel.ID, Channel: channel.StremioID, Video: &video})
				}
			}
			return nil
		})
	}
	_ = group.Wait()
	slices.SortStableFunc(programs, func(a, b Item) int {
		if c := a.StartDate.Compare(*b.StartDate); c != 0 {
			return c
		}
		return comparedNumbers(a.Channel.Number, b.Channel.Number)
	})
	return programs, s.save(ctx, records)
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

// programItem describes a programme of a channel's guide. A video is a
// programme when it has a start and a later end.
func programItem(channel Item, video stremio.Video) (Item, bool) {
	start, end := parseDate(video.StartTime), parseDate(video.EndTime)
	if start == nil || end == nil || !end.After(*start) {
		return Item{}, false
	}
	item := Item{
		ID:          itemID(programKey(channel.StremioID, programmeID(video))),
		Kind:        KindProgram,
		Name:        video.DisplayName(),
		ParentID:    channel.ID,
		Overview:    video.Overview,
		Genres:      slices.Clip(video.Genres),
		StartDate:   start,
		EndDate:     end,
		Runtime:     end.Sub(*start),
		Images:      Images{Primary: video.Thumbnail},
		Available:   true,
		StremioType: channel.StremioType,
		StremioID:   channel.StremioID,
		Channel:     &channel,
	}
	return item, true
}

// program describes a programme listed before, on one of the user's
// channels.
func (s *Service) program(ctx context.Context, v view, r record) (Item, error) {
	if r.Video == nil {
		return Item{}, ErrNotFound
	}
	channel, err := s.channel(ctx, v, itemID(channelKey(r.Channel)))
	if err != nil {
		return Item{}, err
	}
	program, ok := programItem(channel, *r.Video)
	if !ok {
		return Item{}, ErrNotFound
	}
	return program, nil
}

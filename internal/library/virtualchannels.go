package library

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// The channels Polyfin airs itself (see package virtualtv) are listed
// after the channels of the user's live TV catalogs, to the users with the
// Live TV permission, and their programmes in the guide with the others'.
// A channel is shown to a user only when every movie and episode it airs
// is one their parental control and blocked genres let them reach, as a
// programme cannot be left out of a stream that others watch too; the
// programmes of the titles a user hid are left out of their guide. Their
// programmes are computed from the schedule each time and kept as no
// item: one opened by its identifier is found again in the schedule (see
// virtualProgram).

// Virtual answers for the channels Polyfin airs (see UseVirtual): every
// channel, and the one version of each, which plays its playout.
type Virtual interface {
	Channels(ctx context.Context) ([]virtualtv.Channel, error)
	Version(item accounts.ID, channel virtualtv.Channel) Version
}

// UseVirtual sets what answers for the channels Polyfin airs; it is called
// before the service is used. Without it, there are none.
func (s *Service) UseVirtual(virtual Virtual) { s.virtual = virtual }

// virtualRef finds a programme of a channel Polyfin airs again: its
// channel, its start and the title it airs, whose image it shows.
type virtualRef struct {
	channel accounts.ID
	start   time.Time
	title   accounts.ID
}

// virtualRefs remembers the programmes listed lately by their identifier,
// for their images and for apps that open one; past it, the schedule is
// searched (see virtualProgram).
var virtualRefs = cache.New[accounts.ID, virtualRef](200_000, 24*time.Hour)

// virtualChannels sets the channels Polyfin airs that user sees in v: the
// channels on, for a user with the Live TV permission, but those airing a
// title their parental control or blocked genres hide.
func (s *Service) virtualChannels(ctx context.Context, v *view, user accounts.User) error {
	if s.virtual == nil || !user.LiveTv {
		return nil
	}
	all, err := s.virtual.Channels(ctx)
	if err != nil {
		return err
	}
	for _, c := range all {
		if c.Enabled && v.allowsVirtual(c) {
			v.virtual = append(v.virtual, c)
		}
	}
	return nil
}

// allowsVirtual reports whether the user may reach every movie and episode
// a channel airs, by the ratings and genres they had when it was saved.
func (v view) allowsVirtual(c virtualtv.Channel) bool {
	if !v.restricted() {
		return true
	}
	entries := slices.Clone(c.Entries)
	for _, b := range c.Blocks {
		entries = append(entries, b.Entries...)
	}
	for _, e := range entries {
		kind := KindMovie
		if e.Kind == virtualtv.EntryEpisode {
			kind = KindEpisode
		}
		if !v.allows(kind, traits{rating: e.Rating, genres: e.Genres, ratingKnown: true, genresKnown: true}) {
			return false
		}
	}
	return true
}

// virtualChannelItem describes a channel Polyfin airs, numbered number
// unless it has its own.
func virtualChannelItem(c virtualtv.Channel, number int) Item {
	if c.Number > 0 {
		number = c.Number
	}
	return Item{ID: itemID(channelKey(c.StremioID())), Kind: KindChannel, Name: c.Name, Number: strconv.Itoa(number), Available: true,
		StremioType: "tv", StremioID: c.StremioID()}
}

// virtualChannelRecord is the record that finds a channel Polyfin airs
// again, with the number it was listed with.
func virtualChannelRecord(c virtualtv.Channel, item Item, number int) record {
	return record{ID: item.ID, Key: channelKey(c.StremioID()), Kind: KindChannel, Meta: &stremio.Meta{ID: c.StremioID(), Type: "tv", Name: c.Name},
		Number: number}
}

// virtualChannel describes a channel Polyfin airs that the user sees, from
// its record.
func (v view) virtualChannel(r record) (Item, bool) {
	if r.Meta == nil {
		return Item{}, false
	}
	id, ok := virtualtv.ChannelID(r.Meta.ID)
	if !ok {
		return Item{}, false
	}
	i := slices.IndexFunc(v.virtual, func(c virtualtv.Channel) bool { return c.ID == id })
	if i < 0 {
		return Item{}, false
	}
	return virtualChannelItem(v.virtual[i], r.Number), true
}

// unlistedVirtualChannel describes a channel Polyfin airs that the user
// sees by its item's identifier, before it was ever listed: its number is
// found by listing the channels.
func (s *Service) unlistedVirtualChannel(ctx context.Context, v view, id accounts.ID) (Item, bool, error) {
	if !slices.ContainsFunc(v.virtual, func(c virtualtv.Channel) bool { return itemID(channelKey(c.StremioID())) == id }) {
		return Item{}, false, nil
	}
	channels, records := s.channels(ctx, v)
	i := slices.IndexFunc(channels, func(c Item) bool { return c.ID == id })
	if i < 0 {
		return Item{}, false, nil
	}
	return channels[i], true, s.save(ctx, records)
}

// virtualVersion returns the one version of a channel Polyfin airs that
// user sees, ok false for any other item.
func (s *Service) virtualVersion(ctx context.Context, user accounts.User, id accounts.ID) (version Version, ok bool, err error) {
	if s.virtual == nil {
		return Version{}, false, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return Version{}, false, err
	}
	for _, c := range v.virtual {
		if itemID(channelKey(c.StremioID())) == id {
			return s.virtual.Version(id, c), true, nil
		}
	}
	return Version{}, false, nil
}

// virtualProgram describes a programme of a channel Polyfin airs.
func virtualProgram(channel Item, a virtualtv.Airing) Item {
	start, end := a.Start, a.End
	category := "Movie"
	if a.Entry.Kind == virtualtv.EntryEpisode {
		category = "Series"
	}
	return Item{
		ID:                itemID(programKey(channel.StremioID, a.ProgrammeID())),
		Kind:              KindProgram,
		Name:              a.Entry.Name,
		ParentID:          channel.ID,
		Overview:          a.Entry.Overview,
		Genres:            append(slices.Clone(a.Entry.Genres), category),
		ProductionYear:    a.Entry.Year,
		OfficialRating:    a.Entry.Rating,
		StartDate:         &start,
		EndDate:           &end,
		Runtime:           end.Sub(start),
		Images:            Images{Primary: a.Entry.Image},
		IndexNumber:       a.Entry.Number,
		ParentIndexNumber: a.Entry.Season,
		Available:         true,
		StremioType:       channel.StremioType,
		StremioID:         channel.StremioID,
		Channel:           &channel,
		EpisodeTitle:      a.Entry.Episode,
	}
}

// virtualPrograms lists the programmes over [from, to) of the channels
// Polyfin airs among channels, by their Stremio identifiers, that keep
// keeps, but those of the titles the user hid.
func (s *Service) virtualPrograms(v view, channels map[string]Item, from, to time.Time, keep func(Item) bool) []Item {
	var programs []Item
	for _, c := range v.virtual {
		channel, ok := channels[c.StremioID()]
		if !ok {
			continue
		}
		for _, a := range c.Airings(from, to, time.Local) {
			if v.hidden[a.Entry.Title] {
				continue
			}
			program := virtualProgram(channel, a)
			if !keep(program) {
				continue
			}
			virtualRefs.Put(program.ID, virtualRef{channel: c.ID, start: a.Start, title: a.Entry.Title})
			programs = append(programs, program)
		}
	}
	return programs
}

// findVirtualProgram describes a programme of a channel Polyfin airs that
// the user sees by its identifier: as listed lately, else searched in the
// schedules of the days the guide spans.
func (s *Service) findVirtualProgram(ctx context.Context, v view, id accounts.ID) (Item, bool, error) {
	if len(v.virtual) == 0 {
		return Item{}, false, nil
	}
	today := s.now().UTC().Truncate(24 * time.Hour)
	from, to := today.AddDate(0, 0, -1), today.AddDate(0, 0, guideDays+1)
	if ref, ok := virtualRefs.Get(id); ok {
		from, to = ref.start, ref.start.Add(time.Nanosecond)
	}
	for _, c := range v.virtual {
		var channel *Item
		for _, a := range c.Airings(from, to, time.Local) {
			if itemID(programKey(c.StremioID(), a.ProgrammeID())) != id || v.hidden[a.Entry.Title] {
				continue
			}
			if channel == nil {
				item, ok, err := s.listedVirtualChannel(ctx, v, c)
				if !ok || err != nil {
					return Item{}, false, err
				}
				channel = &item
			}
			program := virtualProgram(*channel, a)
			virtualRefs.Put(id, virtualRef{channel: c.ID, start: a.Start, title: a.Entry.Title})
			return program, true, nil
		}
	}
	return Item{}, false, nil
}

// listedVirtualChannel describes a channel Polyfin airs that the user
// sees, as listed.
func (s *Service) listedVirtualChannel(ctx context.Context, v view, c virtualtv.Channel) (Item, bool, error) {
	id := itemID(channelKey(c.StremioID()))
	r, err := s.load(ctx, id)
	if err == nil {
		item, ok := v.virtualChannel(r)
		return item, ok, nil
	}
	return s.unlistedVirtualChannel(ctx, v, id)
}

// virtualArtwork returns the image of a programme of a channel Polyfin
// airs listed lately: its title's.
func (s *Service) virtualArtwork(ctx context.Context, id accounts.ID, imageType string) (Artwork, bool, error) {
	ref, ok := virtualRefs.Get(id)
	if !ok {
		return Artwork{}, false, nil
	}
	artwork, err := s.Artwork(ctx, ref.title, imageType)
	return artwork, true, err
}

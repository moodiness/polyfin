// Package virtualtv keeps the channels Polyfin airs itself: 24/7 channels
// built from titles of the library, a collection, a smart list, a series
// or titles picked by hand. The movies and episodes they hold are frozen
// into entries when a channel is saved, so that its guide stays stable;
// its schedule is a pure function of its start, its seed and its entries
// (see Channel.Airings), which survives restarts and is kept nowhere.
//
// The library lists the channels and their programmes beside the other
// channels and guides; playback airs them, the programme of the moment
// from where it is, as one live HLS stream (see package playback).
package virtualtv

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
)

// IDPrefix starts the Stremio identifier the library gives the channel
// of a Channel: IDPrefix followed by its identifier.
const IDPrefix = "polyfin-tv:"

const (
	// MaxNameLength bounds a channel's name, in characters.
	MaxNameLength = 64
	// MaxEntries bounds the movies and episodes a channel, or one of its
	// blocks, airs: a channel is read whole for each guide listing.
	MaxEntries = 5000
	// MaxBlocks bounds a channel's daily time blocks.
	MaxBlocks = 12
	// MaxNumber bounds a channel's number.
	MaxNumber = 9999
	// minutesPerDay is where the last block of a day may end.
	minutesPerDay = 24 * 60
)

// Kinds of sources a channel or a block is built from.
const (
	SourceCollection = "collection"
	SourceSmartList  = "smartlist"
	SourceSeries     = "series"
	SourceTitles     = "titles"
)

// Kinds of entries.
const (
	EntryMovie   = "movie"
	EntryEpisode = "episode"
)

var (
	// ErrNotFound reports a channel that does not exist.
	ErrNotFound = errors.New("channel not found")
	// ErrInvalidName reports an empty name, or one longer than
	// MaxNameLength.
	ErrInvalidName = errors.New("a channel's name has 1 to 64 characters")
	// ErrInvalidNumber reports a number outside 1 to MaxNumber.
	ErrInvalidNumber = errors.New("a channel's number is a whole number from 1 to 9,999")
	// ErrInvalidSource reports a source of an unknown kind, or naming no
	// item, or more than one for a collection, smart list or series.
	ErrInvalidSource = errors.New("a channel is built from one collection, smart list or series, or from titles")
	// ErrNoEntries reports a channel or block whose source holds no movie
	// nor episode with a runtime, or more than MaxEntries.
	ErrNoEntries = errors.New("a channel airs 1 to 5,000 movies and episodes")
	// ErrInvalidBlocks reports blocks that overlap, end before they start,
	// fall outside the day, or are more than MaxBlocks.
	ErrInvalidBlocks = errors.New("a channel's time blocks are at most 12, within the day, apart from each other")
)

// Source is what a channel or a block is built from: one collection,
// smart list or series, or the titles picked, by their items, with how
// the builder named them.
type Source struct {
	Kind  string
	Items []SourceItem
}

// SourceItem is an item a source names: a collection's, a smart list's,
// a series' or a title's, a movie or a series (Kind).
type SourceItem struct {
	ID   accounts.ID
	Name string
	Kind string
}

// Entry is a movie or an episode a channel airs, as it was when the
// channel was saved: what the guide shows of it, and its runtime, which
// the schedule gives it.
type Entry struct {
	// Item is the movie's or episode's item, which plays; Title the
	// movie's or its series' item, which users hide and rate.
	Item  accounts.ID `json:"-"`
	Title accounts.ID `json:"-"`
	Kind  string      `json:"kind"`
	// Name is the movie's or the series' name, and Episode the episode's
	// own title, with its season and number.
	Name    string `json:"name"`
	Episode string `json:"episode,omitempty"`
	Season  int    `json:"season,omitempty"`
	Number  int    `json:"number,omitempty"`
	// Runtime is how long the schedule gives it, positive.
	Runtime  time.Duration `json:"runtime"`
	Overview string        `json:"overview,omitempty"`
	Image    string        `json:"image,omitempty"`
	Year     int           `json:"year,omitempty"`
	// Rating and Genres are the title's, which parental control and the
	// genres users block judge.
	Rating string   `json:"rating,omitempty"`
	Genres []string `json:"genres,omitempty"`
}

// Block is a daily time block: from Start to End, in minutes past
// midnight in the server's time zone, the channel airs the block's own
// entries, in order or shuffled.
type Block struct {
	Start, End int
	Source     Source
	Entries    []Entry
	Shuffle    bool
}

// Channel is a channel Polyfin airs.
type Channel struct {
	ID   accounts.ID
	Name string
	// Number is the channel's number, 0 to follow the other channels.
	Number  int
	Enabled bool
	Source  Source
	Entries []Entry
	// Shuffle orders the entries anew each round, by Seed; otherwise they
	// air in their order.
	Shuffle bool
	Seed    int64
	Blocks  []Block
	// Epoch is when the schedule starts: the midnight of the day the
	// channel's entries were last frozen.
	Epoch     time.Time
	UpdatedAt time.Time
}

// StremioID is the identifier the library gives the channel's item.
func (c Channel) StremioID() string { return IDPrefix + c.ID.String() }

// ChannelID returns the identifier of the channel a Stremio identifier
// names, and whether it names one.
func ChannelID(stremioID string) (accounts.ID, bool) {
	raw, ok := strings.CutPrefix(stremioID, IDPrefix)
	if !ok {
		return accounts.ID{}, false
	}
	id, err := accounts.ParseID(raw)
	return id, err == nil
}

// Validate reports what is wrong with a channel, nil when nothing is.
func (c Channel) Validate() error {
	if name := strings.TrimSpace(c.Name); name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return ErrInvalidName
	}
	if c.Number < 0 || c.Number > MaxNumber {
		return ErrInvalidNumber
	}
	if err := c.Source.Validate(); err != nil {
		return err
	}
	if err := validEntries(c.Entries); err != nil {
		return err
	}
	if len(c.Blocks) > MaxBlocks {
		return ErrInvalidBlocks
	}
	blocks := slices.Clone(c.Blocks)
	slices.SortFunc(blocks, func(a, b Block) int { return a.Start - b.Start })
	for i, b := range blocks {
		if b.Start < 0 || b.End > minutesPerDay || b.End <= b.Start || i > 0 && b.Start < blocks[i-1].End {
			return ErrInvalidBlocks
		}
		if err := b.Source.Validate(); err != nil {
			return err
		}
		if err := validEntries(b.Entries); err != nil {
			return err
		}
	}
	return nil
}

// Validate reports a source of an unknown kind, or naming no item, or
// more than one for a collection, smart list or series.
func (s Source) Validate() error {
	switch s.Kind {
	case SourceCollection, SourceSmartList, SourceSeries:
		if len(s.Items) != 1 {
			return ErrInvalidSource
		}
	case SourceTitles:
		if len(s.Items) == 0 || len(s.Items) > MaxEntries {
			return ErrInvalidSource
		}
	default:
		return ErrInvalidSource
	}
	return nil
}

func validEntries(entries []Entry) error {
	if len(entries) == 0 || len(entries) > MaxEntries {
		return ErrNoEntries
	}
	for _, e := range entries {
		if e.Runtime <= 0 || e.Kind != EntryMovie && e.Kind != EntryEpisode {
			return ErrNoEntries
		}
	}
	return nil
}

// DayStart is the midnight starting t's day in loc, where a channel's
// schedule starts when its entries are frozen at t.
func DayStart(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

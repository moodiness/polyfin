package accounts

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInvalidChannelRule reports a Live TV channel limit whose mode is not
// one of ChannelModes, that lists more than MaxChannelKeys groups,
// channels or known groups, or a key that is empty, longer than
// MaxChannelKeyLength or unprintable.
var ErrInvalidChannelRule = errors.New("invalid Live TV channel limit")

// The modes of a ChannelRule: every channel, only the groups and channels
// it lists, or every channel but those.
const (
	ChannelsAll    = "all"
	ChannelsOnly   = "only"
	ChannelsExcept = "except"
)

// ChannelModes are the modes a ChannelRule takes.
var ChannelModes = []string{ChannelsAll, ChannelsOnly, ChannelsExcept}

// The bounds of a ChannelRule: line-ups reach tens of thousands of
// channels, a few of which a limit names one by one.
const (
	MaxChannelKeys      = 10_000
	MaxChannelKeyLength = 512
)

// ChannelRule chooses the Live TV channels of the server's sources a user
// sees, by channel group (an IPTV source's category, or a TV catalog's
// genre) and by channel, both by the keys their sources keep across
// refreshes, so that renumbering or regrouping keeps the limit. A user's
// own sources are not limited: they are theirs.
type ChannelRule struct {
	// Mode is one of ChannelModes; empty reads as ChannelsAll.
	Mode string `json:"mode"`
	// Groups and Channels are what ChannelsOnly shows and ChannelsExcept
	// hides: a channel shows under ChannelsOnly when it, or its group, is
	// listed, and is hidden under ChannelsExcept when it, or its group, is.
	Groups   []string `json:"groups,omitempty"`
	Channels []string `json:"channels,omitempty"`
	// HideNewGroups also hides, under ChannelsExcept, the channels of the
	// groups not among KnownGroups: those the server's sources had when
	// the limit was saved, so that groups added later stay hidden.
	HideNewGroups bool     `json:"hideNewGroups,omitempty"`
	KnownGroups   []string `json:"knownGroups,omitempty"`
}

// Limited reports whether the limit may hide channels.
func (c ChannelRule) Limited() bool {
	return c.Mode == ChannelsOnly || c.Mode == ChannelsExcept
}

// ChannelFilter is a ChannelRule ready to judge a line-up's channels, each
// in constant time; nil allows every channel.
type ChannelFilter struct {
	except, hideNew         bool
	groups, channels, known map[string]bool
}

// Filter returns the filter of the limit, nil when it hides nothing.
func (c ChannelRule) Filter() *ChannelFilter {
	if !c.Limited() {
		return nil
	}
	set := func(keys []string) map[string]bool {
		result := make(map[string]bool, len(keys))
		for _, key := range keys {
			result[key] = true
		}
		return result
	}
	return &ChannelFilter{except: c.Mode == ChannelsExcept, hideNew: c.Mode == ChannelsExcept && c.HideNewGroups,
		groups: set(c.Groups), channels: set(c.Channels), known: set(c.KnownGroups)}
}

// Allows reports whether a channel of the server's sources shows, by its
// group's key and its own: under ChannelsOnly when it, or its group, is
// listed; under ChannelsExcept unless it, or its group, is listed, or its
// group is new while new groups are hidden.
func (f *ChannelFilter) Allows(group, channel string) bool {
	switch {
	case f == nil:
		return true
	case !f.except:
		return f.groups[group] || f.channels[channel]
	}
	return !f.groups[group] && !f.channels[channel] && (!f.hideNew || f.known[group])
}

// equal reports whether c and o show the same channels, as they are kept.
func (c ChannelRule) equal(o ChannelRule) bool {
	return c.mode() == o.mode() && slices.Equal(c.Groups, o.Groups) && slices.Equal(c.Channels, o.Channels) &&
		c.HideNewGroups == o.HideNewGroups && slices.Equal(c.KnownGroups, o.KnownGroups)
}

func (c ChannelRule) mode() string {
	if c.Mode == "" {
		return ChannelsAll
	}
	return c.Mode
}

// normalized checks the limit and keeps what its mode reads: each key once,
// in the order given, nothing listed under ChannelsAll, and known groups
// only while new groups are hidden under ChannelsExcept.
func (c ChannelRule) normalized() (ChannelRule, error) {
	mode := c.mode()
	if !slices.Contains(ChannelModes, mode) {
		return ChannelRule{}, ErrInvalidChannelRule
	}
	result := ChannelRule{Mode: mode}
	if mode == ChannelsAll {
		return result, nil
	}
	var err error
	if result.Groups, err = channelKeys(c.Groups); err != nil {
		return ChannelRule{}, err
	}
	if result.Channels, err = channelKeys(c.Channels); err != nil {
		return ChannelRule{}, err
	}
	if mode == ChannelsExcept && c.HideNewGroups {
		result.HideNewGroups = true
		if result.KnownGroups, err = channelKeys(c.KnownGroups); err != nil {
			return ChannelRule{}, err
		}
	}
	return result, nil
}

// channelKeys checks keys and keeps each once, in the order given.
func channelKeys(keys []string) ([]string, error) {
	if len(keys) > MaxChannelKeys {
		return nil, ErrInvalidChannelRule
	}
	seen := make(map[string]bool, len(keys))
	var result []string
	for _, key := range keys {
		if key == "" || utf8.RuneCountInString(key) > MaxChannelKeyLength || !utf8.ValidString(key) ||
			strings.ContainsFunc(key, func(r rune) bool { return !unicode.IsPrint(r) }) {
			return nil, ErrInvalidChannelRule
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, key)
		}
	}
	return result, nil
}

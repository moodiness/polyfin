package library

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Live TV channel rules.
//
// A user's channel rule (accounts.User.LiveTvChannels) hides some of the
// channels of the server's sources, by channel group and by channel,
// wherever their channels are reached: the channel lists and the guide,
// built from the channels a view lists (see channels), and everything
// that finds a channel again by its record (see channel): programmes,
// Replay, playback, timers, recordings and reminders. A user's own sources
// are theirs, and never limited.
//
// A channel is keyed by its Stremio identifier, which its source keeps
// across refreshes. A group is keyed by its catalog and, for an IPTV
// source, the key of the line-up category the channel shows in, which its
// refreshes keep too (a provider group's title, a country, or the
// administrator's own category); for an addon's catalog, by the first
// genre the addon gives the channel, or by the catalog alone.

// channelGroup returns the key of a channel's group and its name.
func channelGroup(src source, meta stremio.Meta) (key, name string) {
	prefix := src.addon.addon.ID.String() + "/" + src.catalog.ID + "/"
	genre := ""
	if len(meta.Genres) > 0 {
		genre = strings.TrimSpace(meta.Genres[0])
	}
	switch {
	case meta.ChannelGroup != "":
		return prefix + meta.ChannelGroup, cmp.Or(genre, meta.ChannelGroup)
	case genre != "":
		return prefix + "genre:" + genre, genre
	}
	return prefix, cmp.Or(src.catalog.Name, src.catalog.ID)
}

// allowsChannel reports whether the user's channel rule shows a channel of
// src: always for one of their own sources.
func (v view) allowsChannel(src source, group, channel string) bool {
	return !src.addon.shared || v.channelFilter.Allows(group, channel)
}

// HiddenChannels reports which of the channels ids names the user's channel
// rule hides, wherever they come from: a recording of such a channel is
// not shown even to the user who scheduled it. A channel no live TV
// catalog of theirs lists is not told, nor one of their own sources.
func (s *Service) HiddenChannels(ctx context.Context, user accounts.User, ids []accounts.ID) (map[accounts.ID]bool, error) {
	hidden := map[accounts.ID]bool{}
	if !user.LiveTvChannels.Limited() || len(ids) == 0 {
		return hidden, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.Kind != KindChannel || r.Meta == nil || r.Addon == nil {
			continue
		}
		index := slices.IndexFunc(v.channels, func(src source) bool {
			return src.addon.addon.ID == *r.Addon && src.catalog.Type == r.CatalogType && src.catalog.ID == r.CatalogID
		})
		if index < 0 {
			continue
		}
		src := v.channels[index]
		group := r.ChannelGroup
		if src.addon.addon.IPTV() {
			// The category it shows in now; a channel gone from the
			// line-up keeps the one it had.
			if current, err := s.fetchMeta(ctx, src.addon, r.CatalogType, r.Meta.ID); err == nil {
				group, _ = channelGroup(src, current)
			} else if !errors.Is(err, stremio.ErrNotFound) {
				return nil, err
			}
		} else {
			group, _ = channelGroup(src, *r.Meta)
		}
		if !v.allowsChannel(src, group, r.Meta.ID) {
			hidden[r.ID] = true
		}
	}
	return hidden, nil
}

// ChannelGroupChoice is a channel group of the server's sources, which a
// channel rule may name.
type ChannelGroupChoice struct {
	Key  string
	Name string
	// Source names the IPTV source or the addon's catalog listing it.
	Source string
	// Channels counts its channels.
	Channels int
}

// ChannelChoice is a channel of the server's sources, which a channel rule
// may name, with its group.
type ChannelChoice struct {
	Key    string
	Name   string
	Number string
	Group  string
}

// ChannelChoices lists the channel groups and the channels of the server's
// live TV catalogs, in the order Live TV lists them, for the
// administrators who choose them: every channel, whatever user would see
// it.
func (s *Service) ChannelChoices(ctx context.Context) ([]ChannelGroupChoice, []ChannelChoice, error) {
	v, err := s.buildView(ctx, accounts.User{LiveTv: true})
	if err != nil {
		return nil, nil, err
	}
	sources := make(map[addons.LibraryKey]source, len(v.channels))
	for _, src := range v.channels {
		key := addons.LibraryKey{AddonID: src.addon.addon.ID, CatalogType: src.catalog.Type, CatalogID: src.catalog.ID}
		if _, ok := sources[key]; !ok {
			sources[key] = src
		}
	}
	items, records := s.channels(ctx, v)
	groups := []ChannelGroupChoice{}
	byKey := map[string]int{}
	channels := make([]ChannelChoice, 0, len(items))
	for i, item := range items {
		r := records[i]
		if r.Addon == nil {
			// A channel Polyfin airs is no channel of a line-up.
			continue
		}
		src, ok := sources[addons.LibraryKey{AddonID: *r.Addon, CatalogType: r.CatalogType, CatalogID: r.CatalogID}]
		if !ok || !src.addon.shared {
			continue
		}
		key, name := channelGroup(src, *r.Meta)
		index, ok := byKey[key]
		if !ok {
			index = len(groups)
			byKey[key] = index
			source := cmp.Or(src.addon.addon.Manifest.Name, src.catalog.Name)
			if catalog := src.catalog.Name; catalog != "" && catalog != source {
				source += " › " + catalog
			}
			groups = append(groups, ChannelGroupChoice{Key: key, Name: name, Source: source})
		}
		groups[index].Channels++
		channels = append(channels, ChannelChoice{Key: item.StremioID, Name: item.Name, Number: item.Number, Group: key})
	}
	return groups, channels, nil
}

package library

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// Items are shared by every user, and identified by what their addon
// calls them: a user's own addon may name the identifiers of the titles,
// channels and episodes the server's addons list. What a user's own addon
// records must then change nothing for the others. A save keeps an item
// one of the server's addons holds (see savedData), and an episode with
// its series (see movedAway); a title only another user's own addon
// holds is none of a user's (see othersOwn).

// othersOwn are the addons users other than user installed for
// themselves, in use or not: every addon but the server's and user's
// own. What such an addon recorded of a title, its type and its
// preview, is its owner's alone. It reads the addons as views do, from
// memory while their changes are followed.
func (s *Service) othersOwn(ctx context.Context, user accounts.ID) (map[accounts.ID]bool, error) {
	all, err := s.addons.All(ctx)
	if err != nil {
		return nil, err
	}
	others := make(map[accounts.ID]bool, len(all))
	for _, addon := range all {
		others[addon.ID] = true
	}
	for _, scope := range []addons.Scope{addons.Shared(), addons.Personal(user)} {
		list, err := s.addons.Addons(ctx, scope)
		if err != nil {
			return nil, err
		}
		for _, addon := range list {
			delete(others, addon.ID)
		}
	}
	return others, nil
}

// ownedOutside tells whether the item recorded as r is held by a user's
// own addon that is none of the view's, the reader's own included: its
// type is that addon's word alone. One of the server's addons, in use or
// not, is not, nor an addon removed since.
func (s *Service) ownedOutside(ctx context.Context, v view, r record) bool {
	if r.Addon == nil {
		return false
	}
	if _, ok := v.addon(*r.Addon); ok {
		return false
	}
	_, owner, err := s.addons.FindOwner(ctx, *r.Addon)
	return err == nil && owner != accounts.ID{}
}

// movedAway tells which of records, about to be saved, would give an
// episode, or a folder's video, to another series or folder than the one
// it is stored with, while one of the server's addons holds that one
// (see savedData): nil when none would. A video's identifier is what its
// description calls it, so the description of one title can name the
// videos of another, whose users then no longer find them: the episode
// is found through the series its record names. Such a record is left
// out, the item staying as it is, unless one of the server's addons
// describes the new series with that video: between the server's own
// descriptions the one listed last has the video, as before, and a
// user's own addon, describing its own series or one of the server's,
// moves no episode the server's users play. A season is never moved: its
// key holds its series.
func (s *Service) movedAway(ctx context.Context, records []*record) ([]bool, error) {
	var ids []accounts.ID
	var series []string
	for _, r := range records {
		if r.SeriesID != "" && r.Video != nil && (r.Kind == KindEpisode || r.Kind == KindVideo) {
			ids, series = append(ids, r.ID), append(series, r.SeriesID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	// The stored series, or folder, is found by its key (see titleKey).
	rows, err := s.db.Query(ctx, `SELECT moved.id FROM unnest($1::uuid[], $2::text[]) AS moved(id, series)
		JOIN items ON items.id = moved.id AND items.data->>'seriesId' <> moved.series
		JOIN items held ON held.key = CASE WHEN items.kind = $3 THEN $3 ELSE $4 END || '|' || (items.data->>'seriesId')
		WHERE `+recordOwner("held.data")+` = `+serverOwner, ids, series, string(KindVideo), string(KindSeries))
	if err != nil {
		return nil, err
	}
	moved := map[accounts.ID]bool{}
	var id accounts.ID
	if _, err := pgx.ForEachRow(rows, []any{&id}, func() error {
		moved[id] = true
		return nil
	}); err != nil || len(moved) == 0 {
		return nil, err
	}
	// The videos the server's addons describe each new series with, read
	// once per series.
	described := map[accounts.ID]map[string]bool{}
	var left []bool
	for i, r := range records {
		if !moved[r.ID] {
			continue
		}
		holder := r.seriesItemID()
		if r.Kind == KindVideo {
			holder = r.folderItemID()
		}
		videos, ok := described[holder]
		if !ok {
			videos = s.sharedVideos(ctx, holder)
			described[holder] = videos
		}
		if !videos[r.Video.ID] {
			if left == nil {
				left = make([]bool, len(records))
			}
			left[i] = true
		}
	}
	return left, nil
}

// sharedVideos are the identifiers of the videos the server's addons
// describe the title recorded as id with, a series or a folder, in the
// descriptions kept of it: no addon is asked. A listing that records
// the videos of a description read it a moment before, which kept it.
// IPTV sources and local folders name their own videos, which no other
// title has.
func (s *Service) sharedVideos(ctx context.Context, id accounts.ID) map[string]bool {
	videos := map[string]bool{}
	title, err := s.load(ctx, id)
	if err != nil || title.Meta == nil {
		return videos
	}
	list, err := s.addons.Addons(ctx, addons.Shared())
	if err != nil {
		return videos
	}
	for _, addon := range list {
		entry := installed{addon: addon, shared: true}
		if !addon.Enabled || own(addon) || !serves(entry, "meta", title.Meta.Type, title.Meta.ID) {
			continue
		}
		if meta, ok := s.keptDescription(ctx, entry, metaKey{addon.ID, title.Meta.Type, title.Meta.ID}, inDatabase); ok {
			for _, video := range meta.Videos {
				videos[video.ID] = true
			}
		}
	}
	return videos
}

// OwnArtwork tells whether the artwork of the item recorded as id is a
// user's own: that of an item one of their own addons holds, or of a
// season or an episode of a series so held. holder is that item, the
// series of a season or an episode. Item identifiers derive from what
// addons call items (see itemID), so that anyone may compute one: such
// artwork is shown to whoever reaches holder (see ReachesArtwork), and
// to others only with its tag, which only its listings give. An item not
// recorded is not one, nor one of the server's addons holds, nor one
// whose addon was removed: no one holds it (see recordOwner).
func (s *Service) OwnArtwork(ctx context.Context, id accounts.ID) (holder accounts.ID, own bool, err error) {
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return accounts.ID{}, false, nil
	}
	if err != nil {
		return accounts.ID{}, false, err
	}
	if s.ownAddon(ctx, r.Addon) {
		return r.ID, true, nil
	}
	if r.Kind != KindSeason && r.Kind != KindEpisode {
		return accounts.ID{}, false, nil
	}
	series, err := s.load(ctx, r.seriesItemID())
	if errors.Is(err, ErrNotFound) {
		return accounts.ID{}, false, nil
	}
	if err != nil {
		return accounts.ID{}, false, err
	}
	return series.ID, s.ownAddon(ctx, series.Addon), nil
}

// ownAddon tells whether addon is a user's own, as recordOwner does.
func (s *Service) ownAddon(ctx context.Context, addon *accounts.ID) bool {
	if addon == nil {
		return false
	}
	_, owner, err := s.addons.FindOwner(ctx, *addon)
	return err == nil && owner != accounts.ID{}
}

// ReachesArtwork tells whether user's view reaches the item recorded as
// holder (see view.reaches), whose artwork OwnArtwork told a user's own.
func (s *Service) ReachesArtwork(ctx context.Context, user accounts.User, holder accounts.ID) (bool, error) {
	r, err := s.load(ctx, holder)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return false, err
	}
	return v.reaches(r), nil
}

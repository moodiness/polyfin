package library

import (
	"context"
	"errors"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Identifying an item (jellyfin-web's Identify) binds it to the title an
// administrator chose, as Jellyfin 12.4 sets the item's provider
// identifiers and refreshes it. A title of the local folders is linked to
// the title chosen, as the folder's page links it: a plain video, a file
// or show folder a local folder left unmatched (see PlainIDPrefix), and
// as well a title the files of local folders were matched to, by a scan
// or by hand; its movie's files or discs, or its show's whole folder,
// from any of its items, the series, a season or an episode. A title the
// addons alone list cannot be bound to another: it is their title.

// ErrNotLocal reports an item Identify cannot bind to another title: one
// that is neither a plain video of a local folder nor a title files of
// local folders were matched to.
var ErrNotLocal = errors.New("the item is its addon's title")

// Identify links the local files of item id to the title of an IMDb
// identifier, as Jellyfin's Identify binds an item to the provider
// identifiers chosen: the unit of the plain video it is, or is a season or
// episode of; else every unit of the movies or shows folders matched to
// the movie or series it is, or is a season or episode of. It answers
// ErrNotLocal for any other item, and ErrNotFound for a plain video no
// longer listed, as once it was matched. The caller checks that the user
// reaches the item.
func (s *Service) Identify(ctx context.Context, id accounts.ID, imdb string) error {
	r, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if s.local == nil {
		return ErrNotLocal
	}
	if plain, ok := r.plainVideo(); ok {
		err = s.local.LinkPlain(ctx, plain, imdb)
		if errors.Is(err, addons.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	kind, stremioID, ok := r.titleOf()
	if !ok {
		return ErrNotLocal
	}
	err = s.local.LinkMatched(ctx, kind, stremioID, imdb)
	if errors.Is(err, addons.ErrNotFound) {
		return ErrNotLocal
	}
	return err
}

// titleOf tells the movie or series a record is, or is a season or
// episode of, by its kind and Stremio identifier.
func (r record) titleOf() (Kind, string, bool) {
	switch r.Kind {
	case KindMovie, KindSeries:
		if r.Meta != nil && r.Meta.ID != "" {
			return r.Kind, r.Meta.ID, true
		}
	case KindSeason, KindEpisode:
		if r.SeriesID != "" {
			return KindSeries, r.SeriesID, true
		}
	}
	return "", "", false
}

// RemoteLookup describes, as RemoteSearch lists it, the title of kind the
// server's addons that describe titles know under an identifier: an IMDb
// identifier, or another the addons take, such as "tmdb:" and a TMDB
// identifier. It is what Identify finds by an identifier, as Jellyfin
// 12.4's TMDB providers look a title up by its identifiers before its
// name (TmdbSeriesProvider.cs lines 55-127). found is false when none of
// them describes a title of kind under it.
func (s *Service) RemoteLookup(ctx context.Context, kind Kind, id string) (result RemoteResult, found bool, err error) {
	id = strings.TrimSpace(id)
	if id == "" || kind != KindMovie && kind != KindSeries {
		return RemoteResult{}, false, nil
	}
	// A view without a user is the server's addons'.
	v, err := s.view(ctx, accounts.User{})
	if err != nil {
		return RemoteResult{}, false, err
	}
	metaType := string(kind)
	for _, candidate := range v.addons {
		if own(candidate.addon) || !v.factual(candidate, metaType, id, false) || !serves(candidate, "meta", metaType, id) {
			continue
		}
		kept, err := s.keptMeta(ctx, candidate, metaType, id)
		if err != nil {
			if ctx.Err() != nil {
				return RemoteResult{}, false, ctx.Err()
			}
			continue
		}
		meta := kept.value
		// An addon that describes a title of the other kind under the
		// identifier describes no title of kind.
		if described, ok := titleKind(meta.Type); meta.Type != "" && (!ok || described != kind) {
			continue
		}
		if meta.ID == "" {
			meta.ID = id
		}
		meta.Videos = nil
		item, _, err := titleItem(candidate, stremio.Catalog{Type: metaType}, meta, accounts.ID{})
		if err != nil {
			continue
		}
		return RemoteResult{Item: item, Provider: candidate.addon.Manifest.Name}, true, nil
	}
	return RemoteResult{}, false, nil
}

// MatchedPlain describes, as Item does, the title a plain video's item
// became once its unit was matched, by a scan or by hand: the movie or
// series it was matched to, or for one of its seasons or episodes, the
// series' season or episode of the same numbers. Jellyfin keeps an item's
// identifier when it is identified again, and apps reopen it by that
// identifier; Polyfin answers with the title, under the title's own
// identifier. It answers ErrNotFound for any other item, while the plain
// video is still listed, and once its files are gone. Lists never show a
// plain video matched (see goneVideos): only an item asked by its
// identifier is answered so.
func (s *Service) MatchedPlain(ctx context.Context, user accounts.User, id accounts.ID) (Item, error) {
	r, err := s.load(ctx, id)
	if err != nil {
		return Item{}, err
	}
	plain, ok := r.plainVideo()
	if !ok || s.local == nil {
		return Item{}, ErrNotFound
	}
	folder, stremioID, err := s.local.PlainMatch(ctx, plain)
	if err != nil {
		return Item{}, err
	}
	if stremioID == "" {
		return Item{}, ErrNotFound
	}
	return s.becameTitle(ctx, user, r, folder, stremioID)
}

// RelinkedTitle describes, as MatchedPlain does, the title the files of a
// local folder were relinked to by Identify (see Identify) from the title
// item id is, or is a season or episode of, when the folder listed that
// title last and none of its files is matched to it any more: jellyfin-web
// reopens the item by its identifier right after Identify. It answers
// ErrNotFound for any other item.
func (s *Service) RelinkedTitle(ctx context.Context, user accounts.User, id accounts.ID) (Item, error) {
	if s.local == nil {
		return Item{}, ErrNotFound
	}
	r, err := s.load(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if r.Kind == KindSeason || r.Kind == KindEpisode {
		if series, err := s.load(ctx, r.seriesItemID()); err == nil {
			r.Addon = series.Addon
		}
	}
	_, former, ok := r.titleOf()
	if !ok || r.Addon == nil {
		return Item{}, ErrNotFound
	}
	stremioID, err := s.local.Relinked(ctx, *r.Addon, former)
	if err != nil {
		return Item{}, err
	}
	if stremioID == "" {
		return Item{}, ErrNotFound
	}
	return s.becameTitle(ctx, user, r, *r.Addon, stremioID)
}

// becameTitle describes the title of stremioID, which folder lists, that
// the item of r became: for a season or episode, the title's season or
// episode of the same numbers.
func (s *Service) becameTitle(ctx context.Context, user accounts.User, r record, folder accounts.ID, stremioID string) (Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Item{}, err
	}
	kind := r.Kind
	if kind == KindSeason || kind == KindEpisode {
		kind = KindSeries
	}
	if kind != KindMovie && kind != KindSeries {
		return Item{}, ErrNotFound
	}
	title := TitleItemID(kind, stremioID)
	// The folder lists the title, which may not be recorded yet: a link
	// made a moment ago lists it at the folder's next page.
	if _, err := s.load(ctx, title); errors.Is(err, ErrNotFound) {
		if err := s.save(ctx, []record{{ID: title, Key: titleKey(kind, stremioID), Kind: kind, Addon: &folder,
			Meta: &stremio.Meta{ID: stremioID, Type: string(kind)}}}); err != nil {
			return Item{}, err
		}
	} else if err != nil {
		return Item{}, err
	}
	target := title
	if r.Kind == KindSeason || r.Kind == KindEpisode {
		episodes, err := s.episodes(ctx, v, title, nil)
		if err != nil {
			return Item{}, err
		}
		target = accounts.ID{}
		for _, episode := range episodes {
			if episode.ParentIndexNumber != r.Season {
				continue
			}
			if r.Kind == KindSeason {
				target = episode.SeasonID
				break
			}
			if r.Video != nil && episode.IndexNumber == r.Video.EpisodeNumber() {
				target = episode.ID
				break
			}
		}
		if target == (accounts.ID{}) {
			return Item{}, ErrNotFound
		}
	}
	item, err := s.item(ctx, v, target)
	if err != nil {
		return Item{}, err
	}
	return s.overriddenItem(item), nil
}

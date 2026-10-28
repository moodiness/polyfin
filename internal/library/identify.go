package library

import (
	"context"
	"errors"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Identifying an item (jellyfin-web's Identify) binds it to the title an
// administrator chose. An item is its addon's title, which cannot be bound
// to another; but a plain video, a file or show folder a local folder left
// unmatched (see PlainIDPrefix), is linked to the title chosen, as the
// folder's page links it: its movie's file or disc, or its show's whole
// folder, from any of its items, the series, a season or an episode.

// ErrNotPlain reports an item Identify cannot bind to another title: one
// that is not a plain video of a local folder.
var ErrNotPlain = errors.New("the item is its addon's title")

// IdentifyPlain links the unit of the plain video item id is, or is a
// season or episode of, to the title of an IMDb identifier, as Jellyfin's
// Identify binds an item to the provider identifiers chosen: ErrNotPlain
// for any other item, ErrNotFound when its unit is no longer listed, as
// once it was matched. The caller checks that the user reaches the item.
func (s *Service) IdentifyPlain(ctx context.Context, id accounts.ID, imdb string) error {
	r, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	plain, ok := r.plainVideo()
	if !ok || s.local == nil {
		return ErrNotPlain
	}
	err = s.local.LinkPlain(ctx, plain, imdb)
	if errors.Is(err, addons.ErrNotFound) {
		return ErrNotFound
	}
	return err
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

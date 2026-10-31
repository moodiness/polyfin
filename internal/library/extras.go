package library

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Extras are the videos and theme songs a local movies or shows folder
// keeps beside its titles, as Jellyfin reads them (see package
// localfiles): a movie's making-of, a series' deleted scenes, a local
// trailer, a theme song or a theme video. Each is an item of its own,
// KindExtra or, for a theme song, KindThemeSong, of its own Stremio type
// and identifier, which only its folder describes and streams (see
// serves): no catalog lists it, and no addon hears of it. An extra is
// reached through its title: the user must see the title, and the folder
// must be one of the user's addons, as for the title's versions.

// The Stremio type and the prefix of the identifiers of extras.
const (
	ExtraType     = "polyfin.extra"
	ExtraIDPrefix = "polyfin-extra:"
)

// Jellyfin's ExtraType values Polyfin tells apart: local trailers, and
// theme songs and videos, which are not special features.
const (
	ExtraTrailer    = "Trailer"
	ExtraThemeSong  = "ThemeSong"
	ExtraThemeVideo = "ThemeVideo"
)

// Extra is an extra a local folder holds: its item, its folder, its
// Jellyfin ExtraType and name, the kind of the titles it belongs to,
// KindMovie or KindSeries, their Stremio identifiers, and with Season the
// season of the series it belongs to, nil for the series itself.
type Extra struct {
	ID     accounts.ID
	Folder accounts.ID
	Type   string
	Name   string
	Kind   Kind
	Owners []string
	Season *int
}

// ownerItems lists the items of the titles, or seasons, an extra belongs
// to.
func (e Extra) ownerItems() []accounts.ID {
	items := make([]accounts.ID, 0, len(e.Owners))
	for _, owner := range e.Owners {
		if e.Season != nil {
			items = append(items, SeasonItemID(owner, *e.Season))
		} else {
			items = append(items, TitleItemID(e.Kind, owner))
		}
	}
	return items
}

// ExtraItemID is the item of the extra at rel in a local folder: the same
// at every scan, so that what users did with it stays.
func ExtraItemID(folder accounts.ID, rel string) accounts.ID {
	return itemID("extra|" + folder.String() + "|" + rel)
}

// ExtraStremioID is the Stremio identifier of an extra's item.
func ExtraStremioID(id accounts.ID) string { return ExtraIDPrefix + id.String() }

// specialFeature reports whether an extra of type extraType is one of
// Jellyfin's special features: neither a trailer nor theme media.
func specialFeature(extraType string) bool {
	return extraType != ExtraTrailer && extraType != ExtraThemeSong && extraType != ExtraThemeVideo
}

// SpecialFeature reports whether an extra item is one of the special
// features apps list under its title (see Extras).
func SpecialFeature(item Item) bool { return specialFeature(item.ExtraType) }

// localFolders lists the local folders among v's addons.
func localFolders(v view) []accounts.ID {
	var folders []accounts.ID
	for _, entry := range v.addons {
		if entry.addon.Local() {
			folders = append(folders, entry.addon.ID)
		}
	}
	return folders
}

// Extras lists the extras of a movie, a series or a season the user
// sees, from their local folders, by name; other items have none of
// their own.
func (s *Service) Extras(ctx context.Context, user accounts.User, id accounts.ID) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	item, err := s.item(ctx, v, id)
	if err != nil {
		return nil, err
	}
	return s.extrasOf(ctx, v, item)
}

// ExtraCounts counts the local trailers and the special features of a
// movie, a series or a season the user sees, as its details show them.
func (s *Service) ExtraCounts(ctx context.Context, user accounts.User, item Item) (trailers, features int, err error) {
	if _, ok := extraOwner(item); !ok || s.local == nil {
		return 0, 0, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return 0, 0, err
	}
	extras, err := s.extrasOf(ctx, v, item)
	for _, extra := range extras {
		switch {
		case extra.ExtraType == ExtraTrailer:
			trailers++
		case specialFeature(extra.ExtraType):
			features++
		}
	}
	return trailers, features, err
}

// ExtraOwner is what extras belong to: the item of a movie, a series or a
// season, with the kind of its title, its Stremio identifier, and the
// season's number.
type ExtraOwner struct {
	Item      accounts.ID
	Kind      Kind
	StremioID string
	Season    *int
}

// extraOwner is what extras of item belong to, false for an item that has
// none of its own.
func extraOwner(item Item) (ExtraOwner, bool) {
	if item.StremioID == "" {
		return ExtraOwner{}, false
	}
	switch item.Kind {
	case KindMovie, KindSeries:
		return ExtraOwner{Item: item.ID, Kind: item.Kind, StremioID: item.StremioID}, true
	case KindSeason:
		// A season of an episode order is none of the series' own.
		if item.EpisodeGroup != "" {
			return ExtraOwner{}, false
		}
		return ExtraOwner{Item: item.ID, Kind: KindSeries, StremioID: item.StremioID, Season: new(item.IndexNumber)}, true
	}
	return ExtraOwner{}, false
}

// extrasOf lists the extras of item, described in v.
func (s *Service) extrasOf(ctx context.Context, v view, item Item) ([]Item, error) {
	owner, ok := extraOwner(item)
	if !ok || s.local == nil {
		return nil, nil
	}
	folders := localFolders(v)
	if len(folders) == 0 {
		return nil, nil
	}
	extras, err := s.local.Extras(ctx, folders, owner)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(extras))
	seen := map[accounts.ID]bool{}
	for _, extra := range extras {
		if seen[extra.ID] {
			continue
		}
		seen[extra.ID] = true
		items = append(items, extraItem(extra, item))
	}
	slices.SortStableFunc(items, func(a, b Item) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return items, nil
}

// extraItem describes an extra of the title, or season, owner. It shows
// its owner's backdrop, which Artwork serves by the extra's item too (see
// extraArtwork).
func extraItem(extra Extra, owner Item) Item {
	kind := KindExtra
	if extra.Type == ExtraThemeSong {
		kind = KindThemeSong
	}
	item := Item{ID: extra.ID, Kind: kind, Name: extra.Name, ParentID: owner.ID, ExtraType: extra.Type, Available: true,
		StremioType: ExtraType, StremioID: ExtraStremioID(extra.ID), OfficialRating: owner.OfficialRating,
		Images: Images{Backdrop: owner.Images.Backdrop}}
	switch owner.Kind {
	case KindSeries:
		item.SeriesID, item.SeriesName = owner.ID, owner.Name
	case KindSeason:
		item.SeriesID, item.SeriesName, item.SeasonID, item.SeasonName = owner.SeriesID, owner.SeriesName, owner.ID, owner.Name
	}
	return item
}

// extra describes the extra of item id, when one of v's local folders
// holds it and v sees one of the titles it belongs to; ErrNotFound
// otherwise.
func (s *Service) extra(ctx context.Context, v view, id accounts.ID) (Item, error) {
	if s.local == nil {
		return Item{}, ErrNotFound
	}
	extra, err := s.local.Extra(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if _, ok := v.addon(extra.Folder); !ok {
		return Item{}, ErrNotFound
	}
	for _, ownerID := range extra.ownerItems() {
		title, err := s.item(ctx, v, ownerID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return Item{}, err
		}
		return extraItem(extra, title), nil
	}
	return Item{}, ErrNotFound
}

// extraArtwork is the Primary or Backdrop image of the extra of item id:
// that of the first title, or season, it belongs to that has one, as its
// listing shows its title's backdrop (see extraItem). False when id is no
// extra's item, or imageType another image.
func (s *Service) extraArtwork(ctx context.Context, id accounts.ID, imageType string) (Artwork, bool, error) {
	if s.local == nil || !strings.EqualFold(imageType, "Primary") && !strings.EqualFold(imageType, "Backdrop") {
		return Artwork{}, false, nil
	}
	extra, err := s.local.Extra(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Artwork{}, false, nil
	}
	if err != nil {
		return Artwork{}, true, err
	}
	for _, owner := range extra.ownerItems() {
		found, err := s.Artwork(ctx, owner, imageType)
		if !errors.Is(err, ErrNotFound) {
			return found, true, err
		}
	}
	return Artwork{}, true, ErrNotFound
}

// taggedExtraArtwork is TaggedArtwork for the extra of item id: the image
// of a title, or season, it belongs to whose tag is tag.
func (s *Service) taggedExtraArtwork(ctx context.Context, id accounts.ID, tag string) (Artwork, bool) {
	if s.local == nil {
		return Artwork{}, false
	}
	extra, err := s.local.Extra(ctx, id)
	if err != nil {
		return Artwork{}, false
	}
	for _, owner := range extra.ownerItems() {
		if found, ok := s.TaggedArtwork(ctx, owner, tag); ok {
			return found, true
		}
	}
	return Artwork{}, false
}

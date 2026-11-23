package library

import (
	"cmp"
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Addon trailers are the trailers a movie's or a series' description
// links to as videos, rather than as pages of a video site, which stay
// its Trailers (see stremio.Meta.TrailerLinks). Each is a local trailer of
// the title, as Jellyfin has them: an extra of type ExtraTrailer, an item
// of its own, which plays its link as a stream its title's description
// embeds (see embed). The link is the addon's that described the title:
// it plays, is relayed and is sent to its source as that addon's streams
// are, and no addon is asked for its streams.
//
// Nothing is kept of an addon trailer: its item is its title's, with the
// bits of a mask of its number flipped, and is found back through its
// title's description.

// TrailerIDPrefix prefixes the Stremio identifier of an addon trailer,
// followed by its number among its title's.
const TrailerIDPrefix = "polyfin-trailer:"

// maxAddonTrailers bounds the addon trailers of a title.
const maxAddonTrailers = 8

// trailerMasks are the masks of the items of the addon trailers of a
// title, by number (see addonTrailerID).
var trailerMasks = func() (masks [maxAddonTrailers]accounts.ID) {
	for n := range masks {
		masks[n] = itemID("addon-trailer|" + strconv.Itoa(n))
	}
	return masks
}()

// addonTrailerID is the item of the addon trailer number n of the title
// of item id, or, as flipping the same bits again undoes it, the item of
// the title of addon trailer number n of item id.
func addonTrailerID(id accounts.ID, n int) accounts.ID {
	for i := range id {
		id[i] ^= trailerMasks[n][i]
	}
	return id
}

// addonTrailer is an addon trailer: its item, the stream it plays, the
// addon that described its title, and the title's Stremio identifier.
type addonTrailer struct {
	item   Item
	stream stremio.Stream
	by     installed
	title  string
}

// trailerStreams lists the streams of the addon trailers a description
// gives, in its order, up to maxAddonTrailers.
func trailerStreams(meta stremio.Meta) []stremio.Stream {
	var streams []stremio.Stream
	for _, link := range meta.TrailerLinks() {
		if link.Stream.URL != "" && len(streams) < maxAddonTrailers {
			streams = append(streams, link.Stream)
		}
	}
	return streams
}

// trailerNumber reads the number of an addon trailer from its Stremio
// identifier; false for another identifier.
func trailerNumber(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, TrailerIDPrefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n >= 0 && n < maxAddonTrailers
}

// addonTrailers lists the addon trailers of owner, a movie or a series v
// sees, as the description kept of it gives them; none for another item,
// or a title no addon's description is kept of.
func (s *Service) addonTrailers(ctx context.Context, v view, owner Item) ([]addonTrailer, error) {
	if owner.Kind != KindMovie && owner.Kind != KindSeries {
		return nil, nil
	}
	r, err := s.load(ctx, owner.ID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil || r.Meta == nil {
		return nil, err
	}
	meta, by, ok := s.describedMeta(ctx, v, r, false, inDatabase)
	if !ok {
		return nil, nil
	}
	streams := trailerStreams(meta)
	trailers := make([]addonTrailer, 0, len(streams))
	for n, stream := range streams {
		name := cmp.Or(strings.TrimSpace(cmp.Or(stream.Title, stream.Name)), owner.Name)
		item := extraItem(Extra{ID: addonTrailerID(owner.ID, n), Type: ExtraTrailer, Name: name}, owner)
		item.StremioType, item.StremioID = meta.Type, TrailerIDPrefix+strconv.Itoa(n)
		trailers = append(trailers, addonTrailer{item: item, stream: stream, by: by, title: meta.ID})
	}
	return trailers, nil
}

// trailerOwners lists the records of the titles item id may be an addon
// trailer of (see addonTrailerID).
func (s *Service) trailerOwners(ctx context.Context, id accounts.ID) ([]record, error) {
	var owners []accounts.ID
	for n := range maxAddonTrailers {
		owners = append(owners, addonTrailerID(id, n))
	}
	records, err := s.loadAll(ctx, owners)
	if err != nil {
		return nil, err
	}
	titles := records[:0]
	for _, r := range records {
		if r.Kind == KindMovie || r.Kind == KindSeries {
			titles = append(titles, r)
		}
	}
	return titles, nil
}

// addonTrailer finds the addon trailer of item id, when v sees its title;
// ErrNotFound otherwise.
func (s *Service) addonTrailer(ctx context.Context, v view, id accounts.ID) (addonTrailer, error) {
	owners, err := s.trailerOwners(ctx, id)
	if err != nil {
		return addonTrailer{}, err
	}
	for _, r := range owners {
		title, err := s.item(ctx, v, r.ID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return addonTrailer{}, err
		}
		trailers, err := s.addonTrailers(ctx, v, title)
		if err != nil {
			return addonTrailer{}, err
		}
		for _, trailer := range trailers {
			if trailer.item.ID == id {
				return trailer, nil
			}
		}
	}
	return addonTrailer{}, ErrNotFound
}

// target is what plays the addon trailer: the stream its title's
// description embeds for it.
func (a addonTrailer) target() target {
	return target{item: a.item.ID, kind: KindExtra, metaType: a.item.StremioType, id: a.item.StremioID, name: a.item.Name,
		embedded: []stremio.Stream{a.stream}, embeddedBy: &a.by, embeddedIn: a.title}
}

// trailerArtwork is the image of type imageType of the title the addon
// trailer of item id belongs to, as its listing shows its title's
// backdrop; false when id is no addon trailer's item.
func (s *Service) trailerArtwork(ctx context.Context, id accounts.ID, imageType string) (Artwork, bool, error) {
	owners, err := s.trailerOwners(ctx, id)
	if err != nil {
		return Artwork{}, true, err
	}
	if len(owners) == 0 {
		return Artwork{}, false, nil
	}
	found, err := s.Artwork(ctx, owners[0].ID, imageType)
	return found, true, err
}

// taggedTrailerArtwork is TaggedArtwork for the addon trailer of item id:
// the image of its title whose tag is tag.
func (s *Service) taggedTrailerArtwork(ctx context.Context, id accounts.ID, tag string) (Artwork, bool) {
	owners, err := s.trailerOwners(ctx, id)
	if err != nil || len(owners) == 0 {
		return Artwork{}, false
	}
	return s.TaggedArtwork(ctx, owners[0].ID, tag)
}

package library

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Images to choose from.
//
// jellyfin-web's Edit images searches image providers for an item's
// artwork (see the jellyfin package's remote images): the descriptions
// the server's metadata addons give of a title, and TMDB. ImageSources
// gathers what both need: each addon's artwork for a movie or series, and
// what TMDB is asked with. The addons are those artwork is found with
// (see artworkView), the addon that listed the title among them when it is
// a user's own, and an addon's artwork is downloaded under its
// confinement. Only http and https addresses are given: never one in the
// scheme of generated images, nor any of Polyfin's own.

// AddonImages is the artwork one addon gives in its description of a
// title: its poster (Primary), background (Backdrop), logo (Logo) and wide
// poster (Thumb). Downloading it is confined to public addresses when
// Confined.
type AddonImages struct {
	Addon    string
	Images   Images
	Confined bool
}

// ImageSources are what an item's artwork is searched with.
type ImageSources struct {
	Kind Kind
	// Addons are the addons' artwork of a movie or series, by addon name.
	Addons []AddonImages
	// IMDb and TMDB identify a movie or series, or a season's or episode's
	// series, as its listing or the addons' descriptions give them, either
	// one empty when none does.
	IMDb, TMDB string
	// Season and Episode number a season or an episode.
	Season, Episode int
	// Person is a person's name.
	Person string
}

// imageSearchDescriptions bounds the addons asked at once for their
// descriptions of a title.
const imageSearchDescriptions = 4

// ImageSources returns what an item's artwork is searched with: nothing
// but its kind for an item that is neither a title, a season, an episode
// nor a person.
func (s *Service) ImageSources(ctx context.Context, id accounts.ID) (ImageSources, error) {
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return ImageSources{}, nil
	}
	if err != nil {
		return ImageSources{}, err
	}
	sources := ImageSources{Kind: r.Kind}
	title := r
	switch {
	case r.Kind == KindPerson && r.Person != nil:
		sources.Person = r.Person.Name
		return sources, nil
	case r.Kind == KindSeason && r.Group != "":
		// A season of an episode order is none of TMDB's seasons.
		return sources, nil
	case r.Kind == KindSeason, r.Kind == KindEpisode && r.Video != nil:
		if r.Kind == KindSeason {
			sources.Season = r.Season
		} else {
			sources.Season, sources.Episode = int(r.Video.Season), int(r.Video.Episode)
		}
		title, err = s.load(ctx, r.seriesItemID())
		if errors.Is(err, ErrNotFound) || err == nil && title.Meta == nil {
			return sources, nil
		}
		if err != nil {
			return ImageSources{}, err
		}
	case (r.Kind == KindMovie || r.Kind == KindSeries) && r.Meta != nil:
	default:
		return sources, nil
	}
	v, err := s.artworkView(ctx, title)
	if err != nil {
		return ImageSources{}, err
	}
	addons, ids := s.addonImages(ctx, v, title)
	sources.IMDb, sources.TMDB = ids["Imdb"], ids["Tmdb"]
	if r.Kind == KindMovie || r.Kind == KindSeries {
		sources.Addons = addons
	}
	return sources, nil
}

// addonImages asks each addon of v that describes a title, Polyfin's own
// sources left out, for its description, and returns their artwork, by
// addon name, with the title's provider identifiers: its listing's, else
// the first description's that gives one.
func (s *Service) addonImages(ctx context.Context, v view, title record) ([]AddonImages, map[string]string) {
	var candidates []installed
	for _, candidate := range v.addons {
		if !own(candidate.addon) && serves(candidate, "meta", title.Meta.Type, title.Meta.ID) {
			candidates = append(candidates, candidate)
		}
	}
	type described struct {
		images AddonImages
		ids    map[string]string
		ok     bool
	}
	found := make([]described, len(candidates))
	var g errgroup.Group
	g.SetLimit(imageSearchDescriptions)
	for i, candidate := range candidates {
		g.Go(func() error {
			kept, err := s.keptMeta(ctx, candidate, title.Meta.Type, title.Meta.ID)
			if err != nil {
				s.logger.Debug("An addon could not describe a title", "addon", candidate.addon.Manifest.Name, "error", err)
				return nil
			}
			meta := kept.value
			found[i] = described{ids: providerIDs(meta), ok: true, images: AddonImages{
				Addon: candidate.addon.Manifest.Name,
				Images: Images{Primary: remoteImage(meta.Poster), Backdrop: remoteImage(meta.Background),
					Logo: remoteImage(meta.Logo), Thumb: remoteImage(meta.LandscapePoster)},
				Confined: candidate.confined || title.Confined && title.Addon != nil && candidate.addon.ID == *title.Addon,
			}}
			return nil
		})
	}
	_ = g.Wait()
	ids := providerIDs(*title.Meta)
	var images []AddonImages
	for _, d := range found {
		if !d.ok {
			continue
		}
		for name, value := range d.ids {
			if ids[name] == "" {
				ids[name] = value
			}
		}
		if d.images.Images != (Images{}) {
			images = append(images, d.images)
		}
	}
	slices.SortStableFunc(images, func(a, b AddonImages) int {
		return cmp.Compare(strings.ToLower(a.Addon), strings.ToLower(b.Addon))
	})
	return images, ids
}

// remoteImage is address when it is an http or https address, else "":
// artwork in the scheme of generated images, or any of Polyfin's own, is
// no image to download.
func remoteImage(address string) string {
	address = strings.TrimSpace(address)
	if strings.HasPrefix(address, generatedScheme) {
		return ""
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return address
}

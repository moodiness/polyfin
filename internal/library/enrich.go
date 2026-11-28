package library

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/stremio"
)

// enrich completes the description of an IPTV source's movie or series
// with that of the first of the user's other addons (the server's only
// when shared is set) that describes its IMDb or TMDB identifier, those
// given a language only when they stand in (see standIn), when the
// source's options ask for it: its artwork, overview, cast, ratings and
// age rating, the source's own description filling what it lacks. The
// title's identifiers, category and episodes stay the source's, which
// play; episodes take the addon's artwork and overviews where they have
// none. Its name and its episodes' titles stay the source's too, unless
// the source names its titles after the metadata addons (see
// iptv.Service.LearnName): then a title described by an addon that may
// name it (see namesFor) takes the addon's name, which the source keeps
// for its listings, and its episodes the addon's titles.
// how says how far the addons' descriptions are looked for:
// the addons are asked (fromAddons) for a request opening the title (see
// iptv.Opening), once, as descriptions are kept; else only the kept ones
// count, in memory or in the database too. ok reports whether an addon's
// description completed the source's.
func (s *Service) enrich(ctx context.Context, v view, source installed, meta stremio.Meta, shared bool, how reach) (_ stremio.Meta, ok bool) {
	meta, _, ok = s.enrichBy(ctx, v, source, meta, shared, how)
	return meta, ok
}

// enrichBy is enrich, which also returns the addon whose description
// completed the source's: the artwork it gave is reached as that addon
// may.
func (s *Service) enrichBy(ctx context.Context, v view, source installed, meta stremio.Meta, shared bool, how reach) (_ stremio.Meta, by installed, ok bool) {
	if s.iptv == nil || meta.Type != "movie" && meta.Type != "series" {
		return meta, installed{}, false
	}
	enriching, naming := s.iptv.Enrichment(ctx, source.addon.ID)
	if !enriching {
		return meta, installed{}, false
	}
	var ids []string
	if meta.ImdbID != "" {
		ids = append(ids, meta.ImdbID)
	}
	if meta.TmdbID != "" {
		ids = append(ids, "tmdb:"+string(meta.TmdbID))
	}
	for _, id := range ids {
		for _, candidate := range v.addons {
			if own(candidate.addon) || !v.factual(candidate, meta.Type, id, shared) || shared && !candidate.shared || !candidate.addon.Manifest.Serves("meta", meta.Type, id) {
				continue
			}
			var described stremio.Meta
			if how == fromAddons {
				var err error
				if described, err = s.meta(ctx, candidate, meta.Type, id); err != nil {
					s.logger.Debug("An addon could not describe an IPTV title", "addon", candidate.addon.LogName(), "error", err)
					continue
				}
			} else if kept, ok := s.keptDescription(ctx, candidate, metaKey{candidate.addon.ID, meta.Type, id}, how); ok {
				described = kept
			} else {
				continue
			}
			described.Name = strings.TrimSpace(described.Name)
			named := naming && namesFor(source, candidate) && described.Name != ""
			if named && described.Name != meta.Name {
				if err := s.iptv.LearnName(ctx, source.addon.ID, meta.ID, described.Name); err != nil {
					s.logger.Warn("An IPTV title's name could not be kept", "source", source.addon.LogName(), "error", err)
				}
			}
			return enriched(meta, described, named), candidate, true
		}
	}
	return meta, installed{}, false
}

// namesFor reports whether candidate's descriptions may name the titles
// of IPTV source, whose names every user of the source sees: an addon
// without a language, of the user's own for a user's own source, else
// one of the server's that everyone uses.
func namesFor(source, candidate installed) bool {
	return !translator(candidate) && (!source.shared || candidate.shared && !candidate.addon.UsedBy.Listed)
}

// enriched is an IPTV title's description completed by an addon's, named
// after it, with its episodes' titles, when named is set.
func enriched(own, addon stremio.Meta, named bool) stremio.Meta {
	result := own
	if named {
		result.Name = addon.Name
	}
	if addon.Poster != "" {
		result.Poster, result.PosterShape = addon.Poster, addon.PosterShape
	}
	result.Background = cmp.Or(addon.Background, own.Background)
	result.Logo = cmp.Or(addon.Logo, own.Logo)
	result.LandscapePoster = cmp.Or(addon.LandscapePoster, own.LandscapePoster)
	result.Description = cmp.Or(addon.Description, own.Description)
	result.ReleaseInfo = cmp.Or(addon.ReleaseInfo, own.ReleaseInfo)
	result.Year = cmp.Or(addon.Year, own.Year)
	result.Released = cmp.Or(addon.Released, own.Released)
	result.Runtime = cmp.Or(addon.Runtime, own.Runtime)
	result.ImdbRating = cmp.Or(addon.ImdbRating, own.ImdbRating)
	result.ImdbID = cmp.Or(own.ImdbID, addon.ImdbID)
	result.TmdbID, result.TmdbFromMoviedb = cmp.Or(own.TmdbID, addon.TmdbID), own.TmdbID == "" && addon.TmdbFromMoviedb
	result.TvdbID = cmp.Or(own.TvdbID, addon.TvdbID)
	for _, names := range []struct{ into, from *stremio.Names }{{&result.Cast, &addon.Cast}, {&result.Director, &addon.Director},
		{&result.Writer, &addon.Writer}} {
		if len(*names.from) > 0 {
			*names.into = *names.from
		}
	}
	// The source's category stays first: libraries narrow by it.
	result.Genres = slices.Clone(own.Genres)
	for _, genre := range addon.Genres {
		if !slices.Contains(result.Genres, genre) {
			result.Genres = append(result.Genres, genre)
		}
	}
	// An addon's links to trailers are its own to play (see
	// addontrailers.go): only its pages of video sites complete the
	// source's description.
	if trailers := addon.SiteTrailers(); len(trailers) > 0 {
		result.Trailers, result.TrailerStreams = trailers, nil
	}
	if addon.Extras != nil {
		extras := *addon.Extras
		if own.Extras != nil && len(extras.SeasonPosterByNumber) == 0 {
			extras.SeasonPosterByNumber = own.Extras.SeasonPosterByNumber
		}
		result.Extras = &extras
	}
	if len(own.Videos) > 0 && len(addon.Videos) > 0 {
		type number struct{ season, episode int }
		byNumber := map[number]stremio.Video{}
		for _, video := range addon.Videos {
			byNumber[number{int(video.Season), int(video.Episode)}] = video
		}
		result.Videos = slices.Clone(own.Videos)
		for i, video := range result.Videos {
			if match, ok := byNumber[number{int(video.Season), int(video.Episode)}]; ok {
				if named {
					result.Videos[i].Title = cmp.Or(match.Title, match.Name, video.Title)
				} else {
					result.Videos[i].Title = cmp.Or(video.Title, match.Title, match.Name)
				}
				result.Videos[i].Thumbnail = cmp.Or(video.Thumbnail, match.Thumbnail)
				result.Videos[i].Overview = cmp.Or(video.Overview, match.Overview)
				result.Videos[i].Released = cmp.Or(video.Released, match.Released)
			}
		}
	}
	return result
}

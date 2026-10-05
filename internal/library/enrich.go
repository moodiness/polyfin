package library

import (
	"cmp"
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
)

// enrich completes the description of an IPTV source's movie or series
// with that of the first of the user's other addons (the server's only
// when shared is set) that describes its IMDb or TMDB identifier, when the
// source's options ask for it: its artwork, overview, cast, ratings and
// age rating, the source's own description filling what it lacks. The
// title's identifiers, name, category and episodes stay the source's,
// which play; episodes take the addon's artwork and overviews where they
// have none. Only a request opening the title asks the addon (see
// iptv.Opening), once, as descriptions are cached; others take what is
// cached.
func (s *Service) enrich(ctx context.Context, v view, source installed, meta stremio.Meta, shared bool) stremio.Meta {
	if s.iptv == nil || meta.Type != "movie" && meta.Type != "series" || !s.iptv.Enrichment(ctx, source.addon.ID) {
		return meta
	}
	var ids []string
	if meta.ImdbID != "" {
		ids = append(ids, meta.ImdbID)
	}
	if meta.TmdbID != "" {
		ids = append(ids, "tmdb:"+string(meta.TmdbID))
	}
	opening := iptv.IsOpening(ctx)
	for _, id := range ids {
		for _, candidate := range v.addons {
			if candidate.addon.IPTV() || shared && !candidate.shared || !candidate.addon.Manifest.Serves("meta", meta.Type, id) {
				continue
			}
			var described stremio.Meta
			if opening {
				var err error
				if described, err = s.meta(ctx, candidate, meta.Type, id); err != nil {
					s.logger.Debug("An addon could not describe an IPTV title", "addon", candidate.addon.Manifest.Name, "error", err)
					continue
				}
			} else if cached, ok := s.metas.Get(metaKey{candidate.addon.ID, meta.Type, id}); ok {
				described = cached
			} else {
				continue
			}
			return enriched(meta, described)
		}
	}
	return meta
}

// enriched is an IPTV title's description completed by an addon's.
func enriched(own, addon stremio.Meta) stremio.Meta {
	result := own
	result.Poster = cmp.Or(addon.Poster, own.Poster)
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
	result.TmdbID = cmp.Or(own.TmdbID, addon.TmdbID)
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
	if len(addon.Trailers) > 0 {
		result.Trailers = addon.Trailers
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
				result.Videos[i].Title = cmp.Or(video.Title, match.Title, match.Name)
				result.Videos[i].Thumbnail = cmp.Or(video.Thumbnail, match.Thumbnail)
				result.Videos[i].Overview = cmp.Or(video.Overview, match.Overview)
				result.Videos[i].Released = cmp.Or(video.Released, match.Released)
			}
		}
	}
	return result
}

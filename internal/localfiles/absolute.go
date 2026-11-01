package localfiles

import (
	"context"
	"strconv"

	"github.com/moodiness/polyfin/internal/library"
)

// A show's file named by an absolute number alone, as "Show - 41.mkv" or
// "[Group] Show - 041 [1080p].mkv" (see episodename.Absolute), in a season
// folder or at the top of its show's folder, is the catalog's episode of
// that place: the 41st regular episode the series' metadata addon lists,
// specials left out, by season then number, as tracking counts them (see
// library.AbsoluteEpisodes). It is then a version of that episode, at the
// episode's season and number (numberingAbsolute). A file whose number
// the catalog does not reach, as of a series no metadata addon describes,
// is an episode of its own, at its season folder's season, else the
// first, and its number (numberingAbsoluteBeyond), as Jellyfin lists it.
// A name telling a season and an episode keeps them: its numbers are
// never read as absolute ones.
//
// Jellyfin has no catalog: its episodes are its files, at the season and
// number their names tell. It reads such a name as the episode of its
// number in the season its folder tells, else none, which its TMDB
// provider takes as the first season's
// (Emby.Server.Implementations/Library/Resolvers/TV/EpisodeResolver.cs and
// MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbEpisodeProvider.cs, lines
// 99-100, at v12.4). Polyfin lists the catalog's episodes, so a file
// numbered otherwise would be listed twice, as the catalog's episode and
// as its own: it is matched to the catalog's instead.
//
// The catalog is read when a scan stores the file, from the server's
// metadata addons (see catalogPlaces), and again each time the series is
// described (see Episodes): a catalog that lists more episodes later, as
// of a show still airing, takes the files it then reaches.

// absolutePlace is the season and number of the episode a file named by
// the absolute number n is, places being the catalog's regular episodes
// in their absolute numbering (see library.AbsoluteEpisodes), with how it
// was numbered; fallback is the season the file is listed in when the
// catalog does not reach n.
func absolutePlace(places [][2]int, n, fallback int) (season, episode int, numbering string) {
	if n >= 1 && n <= len(places) {
		return places[n-1][0], places[n-1][1], numberingAbsolute
	}
	return fallback, n, numberingAbsoluteBeyond
}

// catalogPlaces are the regular episodes the server's metadata addons
// list for the series of stremioID, in their absolute numbering; none
// when no addon describes it.
func (s *Service) catalogPlaces(ctx context.Context, stremioID string) [][2]int {
	meta, ok := s.titles.TitleMeta(ctx, "series", stremioID)
	if !ok {
		return nil
	}
	return library.AbsoluteEpisodes(meta.Videos)
}

// scannedEpisodes is what a scan compares of the episodes a file's name
// tells, to tell whether it changed: its absolute number when it tells
// one, the place the catalog gives it being no change of its name.
func scannedEpisodes(n name) string {
	if n.absolute > 0 {
		return "A" + strconv.Itoa(n.absolute)
	}
	return formatEpisode(n)
}

package appimport

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/library"
)

var imdbID = regexp.MustCompile(`^tt\d+$`)

// TitleRef names the title that an app built on Stremio addons names by
// id, the identifier of a movie or series as metadata addons give it, and
// for an episode its season and episode numbers (episode zero names the
// title itself). Apps name titles by IMDb ("tt…"), TMDB ("tmdb:…") and
// Kitsu ("kitsu:…") identifiers, the last found through the anime
// mapping, and as anime catalogs list the entry; an episode of a Kitsu
// entry is numbered in the entry. It reports false for other identifiers:
// the entry is then unmatched (see Identified).
func TitleRef(mapping *anime.Service, id string, movie bool, season, episode int) (library.TitleRef, bool) {
	ref := library.TitleRef{Episode: !movie && episode > 0, Series: !movie && episode <= 0}
	if ref.Episode {
		ref.Season, ref.Number = season, episode
	}
	if imdbID.MatchString(id) {
		ref.IMDb = id
		return ref, true
	}
	if tmdb, ok := strings.CutPrefix(id, "tmdb:"); ok {
		number, err := strconv.Atoi(tmdb)
		if err != nil || number <= 0 {
			return library.TitleRef{}, false
		}
		ref.TMDB = number
		return ref, true
	}
	source, entry, _, ok := anime.StremioID(id)
	if !ok || source != anime.Kitsu {
		return library.TitleRef{}, false
	}
	return kitsuRef(mapping, entry, movie, episode), true
}

// kitsuRef names the Kitsu entry, or its episode number when not zero: by
// the identifiers and numbering of Polyfin's titles that the mapping
// gives, when it knows the entry, and as anime catalogs list it, by its
// Kitsu identifier and those the mapping completes.
func kitsuRef(mapping *anime.Service, entry int, movie bool, episode int) library.TitleRef {
	ref := library.TitleRef{Episode: !movie && episode > 0, Series: !movie && episode <= 0}
	if mapped, ok := mapping.Title(anime.Kitsu, entry, false, episode, movie); ok {
		switch {
		case mapped.Movie:
			ref = library.TitleRef{IMDb: mapped.IMDb, TMDB: mapped.TMDB}
		case ref.Episode:
			ref = library.TitleRef{Episode: true, IMDb: mapped.IMDb, TVDB: mapped.TVDB, Season: mapped.Season, Number: mapped.Number,
				Absolute: mapped.Absolute}
		default:
			ref = library.TitleRef{Series: true, IMDb: mapped.IMDb, TVDB: mapped.TVDB}
		}
	}
	listed := library.AnimeRef{Kitsu: entry}
	if found := mapping.IDs(anime.Kitsu, strconv.Itoa(entry)); len(found) > 0 {
		listed.MyAnimeList, listed.AniDB = found[0].MyAnimeList, found[0].AniDB
	}
	if ref.Episode {
		listed.Episode = episode
	}
	ref.Anime = listed
	return ref
}

package trackers

import (
	"context"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/library"
)

// Anime identifies to the services the movie or episode item that an
// anime addon names by stremioID, by its Kitsu, MyAnimeList or AniDB
// identifier ("kitsu:<id>", "kitsu:<id>:<episode>"): by the identifiers
// and TVDB numbering the anime mapping gives (see anime.Service.Title).
// An episode numbered as TVDB's absolute numbering is placed among the
// episodes Polyfin listed of its series. It reports false for other
// identifiers, and for what the mapping does not know.
func (s *Service) Anime(ctx context.Context, item accounts.ID, stremioID string, movie bool) (Title, bool) {
	if s == nil || s.anime == nil {
		return Title{}, false
	}
	source, id, number, ok := anime.StremioID(stremioID)
	if !ok || !movie && number == 0 {
		return Title{}, false
	}
	mapped, ok := s.anime.Title(source, id, false, number, movie)
	if !ok {
		return Title{}, false
	}
	if mapped.Movie {
		return Title{Item: item, IMDb: mapped.IMDb, TMDB: mapped.TMDB}, true
	}
	title := Title{Item: item, IMDb: mapped.IMDb, TVDB: mapped.TVDB, Episode: true, Season: mapped.Season, Number: mapped.Number}
	if mapped.Absolute {
		if s.titles == nil {
			return Title{}, false
		}
		found, err := s.titles.Resolve(ctx, []library.TitleRef{animeRef(mapped, "")})
		if err != nil || len(found) == 0 || len(found[0]) == 0 {
			return Title{}, false
		}
		title.Season, title.Number = found[0][0].SeasonNumber, found[0][0].Number
	}
	return title, title.Season >= 0 && title.Number > 0
}

// animeRef names the title the mapping found, for the library.
func animeRef(t anime.Title, name string) library.TitleRef {
	if t.Movie {
		return library.TitleRef{IMDb: t.IMDb, TMDB: t.TMDB, Name: name}
	}
	return library.TitleRef{Episode: true, IMDb: t.IMDb, TVDB: t.TVDB, Season: t.Season, Number: t.Number, Absolute: t.Absolute}
}

// animeIDsJSON are the identifiers Simkl gives an anime by.
type animeIDsJSON struct {
	AniDB   flexInt `json:"anidb"`
	MAL     flexInt `json:"mal"`
	Kitsu   flexInt `json:"kitsu"`
	AniList flexInt `json:"anilist"`
}

// animeJSON is an anime as Simkl gives it.
type animeJSON struct {
	Title     string       `json:"title"`
	AnimeType string       `json:"anime_type"`
	IDs       animeIDsJSON `json:"ids"`
}

// entry is the anime entry the identifiers name: by AniDB's, which the
// episode numbers follow, else by the first the mapping may know it by.
func (i animeIDsJSON) entry() (anime.Source, int, bool) {
	for _, id := range []struct {
		source anime.Source
		id     flexInt
	}{{anime.AniDB, i.AniDB}, {anime.MyAnimeList, i.MAL}, {anime.Kitsu, i.Kitsu}, {anime.AniList, i.AniList}} {
		if id.id > 0 {
			return id.source, int(id.id), true
		}
	}
	return "", 0, false
}

// animeEntry is what reading an anime history needs of the mapping, and
// what it found nothing of.
type animeEntry struct {
	mapping *anime.Service
	// unknown collects what can be named no way, one key per title.
	unknown map[string]bool
}

// ref names the anime a: the movie it is when movie, else its episode
// number of season (0 for AniDB's specials). It names it by the
// identifiers and numbering of Polyfin's titles that the mapping gives,
// and as anime catalogs list the entry, by its Kitsu, MyAnimeList and
// AniDB identifiers, which the mapping completes, with the episode as the
// entry numbers it; specials only the first way, as the lists place
// them. It reports false, and counts the title as unknown, when it can
// name it neither way.
func (e animeEntry) ref(a animeJSON, movie bool, season, number int) (library.TitleRef, bool) {
	special := season == 0 && !movie
	ref := library.TitleRef{Episode: !movie}
	source, id, ok := a.IDs.entry()
	if ok {
		if mapped, found := e.mapping.Title(source, id, special, number, movie); found {
			name := ""
			if mapped.Movie {
				name = strings.TrimSpace(a.Title)
			}
			ref = animeRef(mapped, name)
		}
		if listed := e.listed(a.IDs, source, id); !special && listed != (library.AnimeRef{}) {
			if ref.Episode {
				listed.Episode = number
			}
			ref.Anime = listed
		}
	}
	if usable(ref) || ref.Anime != (library.AnimeRef{}) {
		return ref, true
	}
	key := string(source) + "/" + strconv.Itoa(id)
	if source == "" {
		key = "title/" + a.Title
	}
	if !movie {
		key += "/" + strconv.Itoa(season) + "/" + strconv.Itoa(number)
	}
	e.unknown[key] = true
	return library.TitleRef{}, false
}

// listed is the entry ids and the mapping name, as anime catalogs list
// it: by the identifiers Simkl gives, else those the mapping knows.
func (e animeEntry) listed(ids animeIDsJSON, source anime.Source, id int) library.AnimeRef {
	listed := library.AnimeRef{Kitsu: int(ids.Kitsu), MyAnimeList: int(ids.MAL), AniDB: int(ids.AniDB)}
	if found := e.mapping.IDs(source, strconv.Itoa(id)); len(found) > 0 {
		listed.Kitsu = cmpOr(listed.Kitsu, found[0].Kitsu)
		listed.MyAnimeList = cmpOr(listed.MyAnimeList, found[0].MyAnimeList)
		listed.AniDB = cmpOr(listed.AniDB, found[0].AniDB)
	}
	return listed
}

// simklAnime is an anime of an all-items answer of Simkl, under "show" or
// "anime".
type simklAnime struct {
	LastWatchedAt string     `json:"last_watched_at"`
	AnimeType     string     `json:"anime_type"`
	Show          *animeJSON `json:"show"`
	Anime         *animeJSON `json:"anime"`
	Seasons       []struct {
		Number   int `json:"number"`
		Episodes []struct {
			Number    int    `json:"number"`
			WatchedAt string `json:"watched_at"`
		} `json:"episodes"`
	} `json:"seasons"`
}

// addAnime adds to h the movies and episodes the anime list of Simkl
// says the user watched, numbered as AniDB numbers them.
func (h *watchHistory) addAnime(e animeEntry, items []simklAnime) {
	for _, item := range items {
		a := item.Anime
		if a == nil {
			a = item.Show
		}
		if a == nil {
			continue
		}
		a.AnimeType = cmpOr(a.AnimeType, item.AnimeType)
		if strings.EqualFold(a.AnimeType, "movie") {
			if ref, ok := e.ref(*a, true, 1, 1); ok {
				h.watched = append(h.watched, watchedEntry{ref: ref, at: date(item.LastWatchedAt)})
			}
			continue
		}
		for _, season := range item.Seasons {
			for _, episode := range season.Episodes {
				if episode.Number <= 0 {
					continue
				}
				if ref, ok := e.ref(*a, false, season.Number, episode.Number); ok {
					h.watched = append(h.watched, watchedEntry{ref: ref, at: date(episode.WatchedAt)})
				}
			}
		}
	}
}

// animePlaybackJSON is a paused playback of an anime on Simkl: of the
// anime, a movie, or one of its episodes, numbered as AniDB numbers them.
type animePlaybackJSON struct {
	Progress flexFloat  `json:"progress"`
	PausedAt string     `json:"paused_at"`
	Type     string     `json:"type"`
	Anime    *animeJSON `json:"anime"`
	Episode  *struct {
		Season  *int    `json:"season"`
		Number  flexInt `json:"number"`
		Episode flexInt `json:"episode"`
	} `json:"episode"`
}

// addAnimePlayback adds to h the resume points of the anime among a
// playback list of Simkl.
func (h *watchHistory) addAnimePlayback(e animeEntry, items []animePlaybackJSON) {
	for _, item := range items {
		at := date(item.PausedAt)
		if item.Anime == nil || at == nil {
			continue
		}
		movie := item.Type == "movie" || strings.EqualFold(item.Anime.AnimeType, "movie") || item.Episode == nil
		season, number := 1, 1
		if !movie {
			number = int(cmpOr(item.Episode.Episode, item.Episode.Number))
			if item.Episode.Season != nil && *item.Episode.Season == 0 {
				season = 0
			}
			if number <= 0 {
				continue
			}
		}
		if ref, ok := e.ref(*item.Anime, movie, season, number); ok {
			h.resumes = append(h.resumes, resumeEntry{ref: ref, percent: float64(item.Progress), at: *at})
		}
	}
}

// animeCursor splits the cursor of a Simkl import: where its movies and
// shows stood, and where its anime stood, which an import read only once
// the mapping was.
func animeCursor(cursor string) (shows, animeAt string) {
	shows, animeAt, _ = strings.Cut(cursor, "|")
	return shows, animeAt
}

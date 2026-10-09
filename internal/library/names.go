package library

import (
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/eclipse"
)

// words are the names Polyfin generates, in one server language.
type words struct {
	season      string // followed by the season number
	specials    string // season 0
	episode     string // followed by the episode number, for untitled episodes
	chapter     string // followed by the chapter number, for untitled chapters
	movies      string
	shows       string
	collections string
	// The content of music rows: songs, albums, artists and playlists.
	songs, albums, artists, playlists string
	// myMusic names a music addon's My music (see eclipse.MyMusic).
	myMusic string
	// replay names the Replay view (see ReplayViewID).
	replay string
}

var vocabulary = map[string]words{
	"en": {season: "Season", specials: "Specials", episode: "Episode", chapter: "Chapter",
		movies: "Movies", shows: "Shows", collections: "Collections",
		songs: "Songs", albums: "Albums", artists: "Artists", playlists: "Playlists", myMusic: "My music", replay: "Replay"},
	"fr": {season: "Saison", specials: "Épisodes spéciaux", episode: "Épisode", chapter: "Chapitre",
		movies: "Films", shows: "Séries", collections: "Collections",
		songs: "Titres", albums: "Albums", artists: "Artistes", playlists: "Playlists", myMusic: "Ma musique", replay: "Replay"},
}

// vocabularyOf returns the words of a server language, English when the
// language is unknown.
func vocabularyOf(language string) words {
	if w, ok := vocabulary[language]; ok {
		return w
	}
	return vocabulary["en"]
}

// seasonName names a season as Jellyfin does.
func (w words) seasonName(number int) string {
	if number == 0 {
		return w.specials
	}
	return w.season + " " + strconv.Itoa(number)
}

// episodeName names an episode without a title.
func (w words) episodeName(number int) string {
	return w.episode + " " + strconv.Itoa(number)
}

// ChapterName names the chapter at a position, counted from 1, in a server
// language, as Jellyfin names chapters a file leaves untitled.
func ChapterName(language string, number int) string {
	return vocabularyOf(language).chapter + " " + strconv.Itoa(number)
}

// contentType names the content of a catalog type for a library name, as
// collectionType groups them, music rows by what they list; other types
// are named as they are.
func (w words) contentType(catalogType string) string {
	switch catalogType {
	case eclipse.TypeTrack:
		return w.songs
	case eclipse.TypeAlbum:
		return w.albums
	case eclipse.TypeArtist:
		return w.artists
	case eclipse.TypePlaylist:
		return w.playlists
	}
	switch collectionType(catalogType) {
	case "movies":
		return w.movies
	case "tvshows":
		return w.shows
	case "boxsets":
		return w.collections
	default:
		return catalogType
	}
}

// LibraryNames returns the names Jellyfin apps show for libraries, in the
// same order: libraries is everything one user browses, in their order. A
// library is named after its catalog, and the genre it is narrowed to
// ("Popular · Comedy"), unless it was given a name. Jellyfin apps never
// show two libraries with the same name, so names that collide (ignoring
// surrounding spaces and letter case) are told apart, one step after the
// other while collisions remain:
//
//  1. Each colliding library named after its catalog gets its content type
//     in the server language: "Popular (Movies)" and "Popular (Shows)". A
//     custom name is kept: a library renamed "Popular" stays so, and only
//     the catalogs it collides with are suffixed.
//  2. Libraries that still collide under their own name, mostly custom
//     names given twice, get their content type the same way.
//  3. Libraries that still collide with their content type, the same type
//     from two addons, get their addon's name too: "Popular (Movies, Cinemeta)".
//  4. Every remaining duplicate after the first gets a counter, skipping
//     names in use: "Popular (Movies, Cinemeta) (2)".
//
// Names depend only on the libraries and their order. Only names change:
// item identifiers come from catalog keys.
func LibraryNames(libraries []addons.Library, language string) []string {
	w := vocabularyOf(language)
	// level is how much of the suffix a library's name has: none, its
	// content type, or its content type and addon.
	level := make([]int, len(libraries))
	name := func(i int) string {
		l := libraries[i]
		base := strings.TrimSpace(l.Catalog.Name)
		if l.Catalog.ID == eclipse.MyMusic {
			// Polyfin's own catalog, named in the server's language.
			base = w.myMusic
		}
		if l.Name != nil {
			base = strings.TrimSpace(*l.Name)
		} else if genre := strings.TrimSpace(l.Genre); genre != "" {
			base += " · " + genre
		}
		switch level[i] {
		case 0:
			return base
		case 1:
			return base + " (" + w.contentType(l.Catalog.Type) + ")"
		default:
			return base + " (" + w.contentType(l.Catalog.Type) + ", " + strings.TrimSpace(l.AddonName) + ")"
		}
	}
	names := make([]string, len(libraries))
	current := func() map[string]int {
		counts := map[string]int{}
		for i := range libraries {
			names[i] = name(i)
			counts[nameKey(names[i])]++
		}
		return counts
	}
	// raise adds the next suffix to the colliding libraries that qualify.
	raise := func(qualifies func(i int) bool) {
		counts := current()
		for i := range libraries {
			if counts[nameKey(names[i])] > 1 && qualifies(i) {
				level[i]++
			}
		}
	}
	raise(func(i int) bool { return level[i] == 0 && libraries[i].Name == nil })
	raise(func(i int) bool { return level[i] == 0 })
	raise(func(i int) bool { return level[i] == 1 })

	used := current()
	taken := map[string]bool{}
	for i, base := range names {
		if key := nameKey(base); !taken[key] {
			taken[key] = true
			continue
		}
		for n := 2; ; n++ {
			candidate := base + " (" + strconv.Itoa(n) + ")"
			if key := nameKey(candidate); used[key] == 0 && !taken[key] {
				names[i] = candidate
				taken[key] = true
				break
			}
		}
	}
	return names
}

// nameKey is what two library names are compared by.
func nameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Package library turns the catalogs of a user's Stremio addons into the
// libraries, collections, titles, seasons and episodes Jellyfin apps browse.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Kind is what an item is.
type Kind string

const (
	// KindLibrary is a catalog shown as a library.
	KindLibrary Kind = "library"
	// KindCollection is an entry of a collection catalog: a group of
	// catalogs, shown as a Jellyfin collection (BoxSet).
	KindCollection Kind = "collection"
	KindMovie      Kind = "movie"
	KindSeries     Kind = "series"
	KindSeason     Kind = "season"
	KindEpisode    Kind = "episode"
	// KindPerson is someone credited for titles, known by name.
	KindPerson Kind = "person"
	// KindChannel is a live TV channel, listed by a tv catalog; KindProgram
	// a programme of its guide.
	KindChannel Kind = "channel"
	KindProgram Kind = "program"
	// KindRecording is a Live TV recording, a file of Polyfin's own (see
	// the recordings package), never listed by the library.
	KindRecording Kind = "recording"
	// The items of Eclipse music addons (see music.go): artists, albums,
	// tracks, the tracks of audiobook addons, and the addons' playlists.
	KindArtist        Kind = "artist"
	KindAlbum         Kind = "album"
	KindTrack         Kind = "track"
	KindAudiobook     Kind = "audiobook"
	KindMusicPlaylist Kind = "musicplaylist"
)

// Item is something a Jellyfin app can browse.
type Item struct {
	ID   accounts.ID
	Kind Kind
	Name string
	// ParentID is the folder the item was listed in; zero for libraries.
	ParentID accounts.ID
	// CollectionType is the content of a library: "movies", "tvshows",
	// "boxsets", or empty for mixed content.
	CollectionType  string
	Overview        string
	Genres          []string
	ProductionYear  int
	PremiereDate    *time.Time
	EndDate         *time.Time
	Runtime         time.Duration
	CommunityRating float64
	OfficialRating  string
	// Status is "Continuing" or "Ended" for series.
	Status      string
	ProviderIDs map[string]string
	People      []Person
	Trailers    []Trailer
	Images      Images
	// Series and season of seasons and episodes.
	SeriesID    accounts.ID
	SeriesName  string
	SeasonID    accounts.ID
	SeasonName  string
	IndexNumber int
	// ParentIndexNumber is the season number of an episode.
	ParentIndexNumber int
	// SeriesPoster is the Primary artwork of a season's or episode's series;
	// SeasonPoster that of an episode's season, empty when the season has
	// none of its own.
	SeriesPoster string
	SeasonPoster string
	// Contents describes the episodes of a series or season, once the
	// series' episodes are known.
	Contents *Contents
	// Available is false for an episode not released yet.
	Available bool
	// StremioType and StremioID identify the title (or the episode's video)
	// for the addons.
	StremioType string
	StremioID   string
	// Number is a channel's number: its place among the user's channels.
	Number string
	// StartDate is when a programme begins; it ends at EndDate.
	StartDate *time.Time
	// Channel is the channel a programme is on.
	Channel *Item
	// EpisodeTitle is the title of the episode a programme airs, when its
	// guide gives one.
	EpisodeTitle string
	// Music: a track's album, by name and item, and its artwork; a track's
	// or album's artists, and the artist of a track's album; whether it is
	// explicit; and how many tracks an album or playlist holds, or albums
	// an artist, when known. ISRC identifies a track's recording.
	Album       string
	AlbumID     accounts.ID
	AlbumPoster string
	Artists     []Credit
	AlbumArtist *Credit
	Explicit    bool
	ChildCount  *int
	ISRC        string
}

// Credit names an artist and the item that stands for them.
type Credit struct {
	ID   accounts.ID
	Name string
}

// Contents describes the episodes under a series or season.
type Contents struct {
	// Children counts the seasons of a series, or the episodes of a season.
	Children int
	// Released counts the episodes already released, which last came out
	// at LastReleased and last Runtime together.
	Released     int
	Runtime      time.Duration
	LastReleased *time.Time
}

// Images are artwork URLs; empty when the item has none.
type Images struct {
	Primary  string
	Backdrop string
	Logo     string
	Thumb    string
}

// URL returns the artwork of a Jellyfin image type.
func (i Images) URL(imageType string) string {
	switch strings.ToLower(imageType) {
	case "primary":
		return i.Primary
	case "backdrop":
		return i.Backdrop
	case "logo":
		return i.Logo
	case "thumb":
		return i.Thumb
	default:
		return ""
	}
}

// Person is someone credited for a title. ID identifies them by name across
// titles.
type Person struct {
	ID    accounts.ID `json:"-"`
	Name  string      `json:"name"`
	Role  string      `json:"-"`
	Type  string      `json:"-"`
	Image string      `json:"image,omitempty"`
}

// Trailer is a video presenting a title.
type Trailer struct {
	Name string
	URL  string
}

// itemID derives an item's identifier from its stable key.
func itemID(key string) accounts.ID {
	sum := sha256.Sum256([]byte("polyfin:item:" + key))
	var id accounts.ID
	copy(id[:], sum[:16])
	return id
}

func libraryKey(addon accounts.ID, catalogType, catalogID string) string {
	return "library|" + addon.String() + "|" + catalogType + "|" + catalogID
}

func collectionKey(addon accounts.ID, metaID string) string {
	return "collection|" + addon.String() + "|" + metaID
}

func titleKey(kind Kind, stremioID string) string { return string(kind) + "|" + stremioID }

func seasonKey(seriesID string, season int) string {
	return "season|" + seriesID + "|" + strconv.Itoa(season)
}

func episodeKey(videoID string) string { return "episode|" + videoID }

func personKey(name string) string { return "person|" + strings.ToLower(name) }

func channelKey(stremioID string) string { return "channel|" + stremioID }

func programKey(channel, programme string) string { return "program|" + channel + "|" + programme }

// ImageTag identifies a version of an artwork for HTTP caching; it is empty
// when there is no artwork.
func ImageTag(url string) string {
	if url == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:16])
}

// titleKind maps a Stremio type to a playable title kind. The tv type is
// live TV, which lists channels rather than titles (see LiveCatalog).
func titleKind(stremioType string) (Kind, bool) {
	switch stremioType {
	case "movie", "anime.movie":
		return KindMovie, true
	case "series", "anime.series", "anime":
		return KindSeries, true
	default:
		return "", false
	}
}

// collectionType maps a catalog type to the content of its library. An
// Eclipse addon's rows are music libraries, its audiobook rows aside (see
// musicCollectionType).
func collectionType(catalogType string) string {
	switch catalogType {
	case "movie", "anime.movie":
		return "movies"
	case "series", "anime.series":
		return "tvshows"
	case "collection":
		return "boxsets"
	case eclipse.TypeTrack, eclipse.TypeAlbum, eclipse.TypeArtist, eclipse.TypePlaylist:
		return "music"
	default:
		return ""
	}
}

var (
	yearPattern    = regexp.MustCompile(`\d{4}`)
	runtimePattern = regexp.MustCompile(`(?i)(\d+)\s*(h|hr|hrs|hour|hours|m|min|mins|minute|minutes)?`)
)

// parseRuntime reads durations such as "142 min", "1h 42min", "55min" or "102".
func parseRuntime(text string) time.Duration {
	var total time.Duration
	for _, match := range runtimePattern.FindAllStringSubmatch(text, -1) {
		value, _ := strconv.Atoi(match[1])
		if unit := strings.ToLower(match[2]); strings.HasPrefix(unit, "h") {
			total += time.Duration(value) * time.Hour
		} else {
			total += time.Duration(value) * time.Minute
		}
	}
	return total
}

func parseDate(text string) *time.Time {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02", "2006"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

// parseYears reads the first and, for "2010-2014", last year of a release.
func parseYears(text string) (first, last int) {
	years := yearPattern.FindAllString(text, 2)
	if len(years) > 0 {
		first, _ = strconv.Atoi(years[0])
	}
	if len(years) > 1 {
		last, _ = strconv.Atoi(years[1])
	}
	return first, last
}

// providerIDs collects the external identifiers of a title, under the names
// Jellyfin uses.
func providerIDs(meta stremio.Meta) map[string]string {
	ids := map[string]string{}
	add := func(name, value string) {
		if value != "" && ids[name] == "" {
			ids[name] = value
		}
	}
	id := meta.ID
	switch {
	case strings.HasPrefix(id, "tt"):
		add("Imdb", strings.SplitN(id, ":", 2)[0])
	case strings.HasPrefix(id, "tmdb:"):
		add("Tmdb", strings.SplitN(strings.TrimPrefix(id, "tmdb:"), ":", 2)[0])
	case strings.HasPrefix(id, "tvdb:"):
		add("Tvdb", strings.SplitN(strings.TrimPrefix(id, "tvdb:"), ":", 2)[0])
	}
	add("Imdb", meta.ImdbID)
	add("Tmdb", string(meta.TmdbID))
	add("Tvdb", string(meta.TvdbID))
	return ids
}

// fromMeta fills an item with a title's or collection's metadata.
func fromMeta(item *Item, meta stremio.Meta) {
	// Some addons wrap names in direction marks, which would upset sorting.
	item.Name = strings.TrimFunc(meta.Name, func(r rune) bool { return unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) })
	item.Overview = meta.Description
	item.Genres = slices.Clip(meta.Genres)
	first, last := parseYears(string(meta.ReleaseInfo))
	if first == 0 {
		first, _ = parseYears(string(meta.Year))
	}
	item.ProductionYear = first
	item.PremiereDate = parseDate(meta.Released)
	if item.PremiereDate == nil && first > 0 {
		date := time.Date(first, time.January, 1, 0, 0, 0, 0, time.UTC)
		item.PremiereDate = &date
	}
	if last > 0 {
		end := time.Date(last, time.December, 31, 0, 0, 0, 0, time.UTC)
		item.EndDate = &end
	}
	item.Runtime = parseRuntime(string(meta.Runtime))
	item.CommunityRating, _ = strconv.ParseFloat(strings.TrimSpace(string(meta.ImdbRating)), 64)
	item.OfficialRating = certification(meta)
	switch strings.ToLower(meta.Status) {
	case "ended", "canceled", "cancelled":
		item.Status = "Ended"
	case "":
	default:
		item.Status = "Continuing"
	}
	item.ProviderIDs = providerIDs(meta)
	item.Images = Images{Primary: meta.Poster, Backdrop: meta.Background, Logo: meta.Logo, Thumb: meta.LandscapePoster}
	item.People = people(meta)
	for _, trailer := range meta.Trailers {
		if trailer.Source == "" {
			continue
		}
		name := trailer.Name
		if name == "" {
			name = trailer.Type
		}
		item.Trailers = append(item.Trailers, Trailer{Name: name, URL: "https://www.youtube.com/watch?v=" + trailer.Source})
	}
	item.StremioType, item.StremioID = meta.Type, meta.ID
}

func people(meta stremio.Meta) []Person {
	var result []Person
	add := func(name, role, kind, image string) {
		result = append(result, Person{ID: itemID(personKey(name)), Name: name, Role: role, Type: kind, Image: image})
	}
	if meta.Extras != nil && len(meta.Extras.Cast) > 0 {
		for _, member := range meta.Extras.Cast {
			add(member.Name, member.Character, "Actor", member.Photo)
		}
	} else {
		for _, name := range meta.Cast {
			add(name, "", "Actor", "")
		}
	}
	for _, name := range meta.Director {
		add(name, "Director", "Director", "")
	}
	for _, name := range meta.Writer {
		add(name, "Writer", "Writer", "")
	}
	return result
}

// released reports whether an episode can already be watched.
func released(video stremio.Video, now time.Time) bool {
	if video.Available != nil {
		return *video.Available
	}
	date := parseDate(video.Released)
	return date == nil || !date.After(now)
}

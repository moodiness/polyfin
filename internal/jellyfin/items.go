package jellyfin

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// UserItemData is what a user did with an item: Jellyfin's
// UserItemDataDto, in its key order.
type UserItemData struct {
	Rating                *float64 `json:",omitempty"`
	PlayedPercentage      *float64 `json:",omitempty"`
	UnplayedItemCount     *int     `json:",omitempty"`
	PlaybackPositionTicks int64
	PlayCount             int
	IsFavorite            bool
	Likes                 *bool `json:",omitempty"`
	LastPlayedDate        *Time `json:",omitempty"`
	Played                bool
	Key                   string
	ItemId                string
}

type NameGuidPair struct {
	Name string
	Id   string
}

type NameValuePair struct {
	Name  string
	Value string
}

type MediaUrl struct {
	Url  string
	Name string
}

type BaseItemPerson struct {
	Name            string
	Id              string
	Role            string
	Type            string
	PrimaryImageTag string `json:",omitempty"`
}

// BaseItemDto is Jellyfin's description of any item. Fields Jellyfin only
// sends on request (the Fields parameter) or in an item's own description
// are pointers or omitted when empty.
type BaseItemDto struct {
	Name           string
	OriginalTitle  string `json:",omitempty"`
	ServerId       string
	Id             string
	Etag           string      `json:",omitempty"`
	PlaylistItemId string      `json:",omitempty"` // the entry a playlist lists the item as
	DateCreated    *Time       `json:",omitempty"`
	CanDelete      *bool       `json:",omitempty"`
	CanDownload    *bool       `json:",omitempty"`
	SortName       string      `json:",omitempty"`
	PremiereDate   *Time       `json:",omitempty"`
	ExternalUrls   *[]MediaUrl `json:",omitempty"`
	// Path is the name a movie's or an episode's version downloads as:
	// Polyfin has no file of its own, and apps take the file name of a
	// download from it.
	Path                     string             `json:",omitempty"`
	EnableMediaSourceDisplay *bool              `json:",omitempty"`
	OfficialRating           string             `json:",omitempty"`
	ChannelId                *string            // always sent: null but for a programme's channel
	ChannelName              string             `json:",omitempty"`
	Overview                 *string            `json:",omitempty"`
	Taglines                 *[]string          `json:",omitempty"`
	Genres                   *[]string          `json:",omitempty"`
	CommunityRating          *float64           `json:",omitempty"`
	RunTimeTicks             *int64             `json:",omitempty"`
	PlayAccess               string             `json:",omitempty"`
	ProductionYear           *int               `json:",omitempty"`
	IndexNumber              *int               `json:",omitempty"`
	ParentIndexNumber        *int               `json:",omitempty"`
	RemoteTrailers           *[]MediaUrl        `json:",omitempty"`
	ProviderIds              *map[string]string `json:",omitempty"`
	IsFolder                 bool
	ParentId                 string `json:",omitempty"`
	Type                     string
	People                   *[]BaseItemPerson `json:",omitempty"`
	Studios                  *[]NameGuidPair   `json:",omitempty"`
	GenreItems               *[]NameGuidPair   `json:",omitempty"`
	ParentBackdropItemId     string            `json:",omitempty"`
	ParentBackdropImageTags  []string          `json:",omitempty"`
	LocalTrailerCount        *int              `json:",omitempty"`
	UserData                 UserItemData      `json:",omitzero"`
	RecursiveItemCount       *int              `json:",omitempty"`
	ChildCount               *int              `json:",omitempty"`
	SeriesName               string            `json:",omitempty"`
	SeriesId                 string            `json:",omitempty"`
	SeasonId                 string            `json:",omitempty"`
	SpecialFeatureCount      *int              `json:",omitempty"`
	DisplayPreferencesId     string            `json:",omitempty"`
	Status                   string            `json:",omitempty"`
	AirDays                  *[]string         `json:",omitempty"`
	Tags                     *[]string         `json:",omitempty"`
	PrimaryImageAspectRatio  *float64          `json:",omitempty"`
	SeriesPrimaryImageTag    string            `json:",omitempty"`
	SeasonName               string            `json:",omitempty"`
	CollectionType           string            `json:",omitempty"`
	DisplayOrder             string            `json:",omitempty"`
	ImageTags                map[string]string
	BackdropImageTags        []string
	ImageBlurHashes          map[string]map[string]string
	ParentPrimaryImageItemId string `json:",omitempty"`
	ParentPrimaryImageTag    string `json:",omitempty"`
	// Chapters are those of the version the item plays first, once Polyfin
	// analyzed it.
	Chapters               *[]ChapterInfo `json:",omitempty"`
	LocationType           string
	MediaType              string
	VideoType              string    `json:",omitempty"`
	EndDate                *Time     `json:",omitempty"`
	LockedFields           *[]string `json:",omitempty"`
	LockData               *bool     `json:",omitempty"`
	CumulativeRunTimeTicks *int64    `json:",omitempty"`
	DateLastMediaAdded     *Time     `json:",omitempty"`
	// Number, ChannelNumber and ChannelType describe a channel, and
	// CurrentProgram its programme airing now; the others a programme.
	Number                 string       `json:",omitempty"`
	ChannelNumber          string       `json:",omitempty"`
	ChannelPrimaryImageTag string       `json:",omitempty"`
	StartDate              *Time        `json:",omitempty"`
	CurrentProgram         *BaseItemDto `json:",omitempty"`
	IsMovie                *bool        `json:",omitempty"`
	IsSeries               *bool        `json:",omitempty"`
	IsNews                 *bool        `json:",omitempty"`
	IsKids                 *bool        `json:",omitempty"`
	IsSports               *bool        `json:",omitempty"`
	ChannelType            string       `json:",omitempty"`
	// EpisodeTitle is the title of the episode a programme airs.
	EpisodeTitle string `json:",omitempty"`
	// The fields of music (see describeMusic): a track's album and its
	// artwork, the artists of a track or an album and the artist of its
	// album, whether a track has lyrics, and what an artist's page counts.
	Album                string          `json:",omitempty"`
	AlbumId              string          `json:",omitempty"`
	AlbumPrimaryImageTag string          `json:",omitempty"`
	AlbumArtist          string          `json:",omitempty"`
	AlbumArtists         *[]NameGuidPair `json:",omitempty"`
	ArtistItems          *[]NameGuidPair `json:",omitempty"`
	Artists              *[]string       `json:",omitempty"`
	HasLyrics            *bool           `json:",omitempty"`
	SongCount            *int            `json:",omitempty"`
	AlbumCount           *int            `json:",omitempty"`
	ArtistCount          *int            `json:",omitempty"`
	MovieCount           *int            `json:",omitempty"`
	SeriesCount          *int            `json:",omitempty"`
	EpisodeCount         *int            `json:",omitempty"`
	TrailerCount         *int            `json:",omitempty"`
	MusicVideoCount      *int            `json:",omitempty"`
	ProgramCount         *int            `json:",omitempty"`
	// Container, MediaSources, MediaStreams, HasSubtitles, Width, Height
	// and Trickplay describe a movie's or episode's versions.
	Container    string                  `json:",omitempty"`
	MediaSources *[]MediaSourceInfo      `json:",omitempty"`
	MediaStreams *[]playback.MediaStream `json:",omitempty"`
	HasSubtitles *bool                   `json:",omitempty"`
	Width        *int                    `json:",omitempty"`
	Height       *int                    `json:",omitempty"`
	// Trickplay holds the scrubbing thumbnails of its versions, by media
	// source and width.
	Trickplay *map[string]map[int]TrickplayInfo `json:",omitempty"`
	// TimerId and SeriesTimerId are the timers recording a programme, or
	// a recording; CompletionPercentage is how far a recording under way
	// is.
	TimerId              string   `json:",omitempty"`
	SeriesTimerId        string   `json:",omitempty"`
	CompletionPercentage *float64 `json:",omitempty"`
}

// addMediaSources describes a movie's or episode's versions in its DTO, as
// Jellyfin does in item details and, when asked, in listings. opened is the
// identifier the item was asked by: its own, or one of its versions'. Item
// details ask the addons for streams; listings only show what is known.
// A channel's details describe its streams as a placeholder, as Jellyfin's
// do: PlaybackInfo describes them once the channel plays.
func (h *Handler) addMediaSources(r *http.Request, user accounts.User, dto *BaseItemDto, item library.Item, opened accounts.ID, detail bool) {
	if item.Kind == library.KindChannel && detail {
		dto.MediaSources = &[]MediaSourceInfo{channelPlaceholder(item)}
		dto.MediaStreams = &[]playback.MediaStream{}
		if sources := h.lineupSources(r, user, item, opened); len(sources) > 0 {
			dto.MediaSources = &sources
		}
		return
	}
	if library.AudioKind(item.Kind) {
		h.addAudioSources(r, user, dto, item, detail)
		return
	}
	if item.Kind != library.KindMovie && item.Kind != library.KindEpisode && item.Kind != library.KindRecording {
		return
	}
	var sources []MediaSourceInfo
	var p playable
	if detail {
		var err error
		p, err = h.playable(r.Context(), user, item)
		if err != nil && r.Context().Err() == nil {
			h.Logger.Warn("The versions of a title could not be listed", "error", err)
		}
		sources = h.mediaSources(r, p, opened)
		if item.Kind != library.KindRecording {
			h.setDownload(r, user, dto, item, p.ordered(opened), true)
		}
		h.prepareOpened(r.Context(), user, item, p.ordered(opened))
	} else if p = h.cachedPlayable(r.Context(), user, item); len(p.versions) > 0 {
		sources = h.mediaSources(r, p, opened)
	} else {
		sources = []MediaSourceInfo{h.placeholderSource(r, item)}
	}
	h.setChapters(r.Context(), dto, p.ordered(opened))
	dto.Id = opened.String()
	dto.MediaSources = &sources
	// Recordings get no thumbnails (see queueImages): no Trickplay field.
	if detail && item.Kind != library.KindRecording {
		dto.Trickplay = h.trickplayManifest(r.Context(), user, item, p.ordered(opened), opened)
	}
	if len(sources) == 0 {
		dto.MediaStreams = &[]playback.MediaStream{}
		return
	}
	first := sources[0]
	dto.MediaStreams = &first.MediaStreams
	dto.Container = first.Container
	for _, stream := range first.MediaStreams {
		switch stream.Type {
		case "Subtitle":
			// Jellyfin leaves the flag out for titles without subtitles.
			if detail {
				dto.HasSubtitles = new(true)
			}
		case "Video":
			if dto.Width == nil {
				dto.Width, dto.Height = stream.Width, stream.Height
			}
		}
	}
}

// fieldSet holds the optional fields a request asked for.
type fieldSet map[string]bool

func requestedFields(r *http.Request) fieldSet {
	fields := fieldSet{}
	for key, values := range r.URL.Query() {
		if !strings.EqualFold(key, "fields") {
			continue
		}
		for _, value := range values {
			for field := range strings.SplitSeq(value, ",") {
				if field = strings.TrimSpace(field); field != "" {
					fields[strings.ToLower(field)] = true
				}
			}
		}
	}
	return fields
}

func (f fieldSet) has(name string) bool { return f[strings.ToLower(name)] }

// nameID derives the identifier of a named thing (a genre, a person) the
// way Polyfin identifies items: stable for the same name.
func nameID(kind, name string) string {
	sum := sha256.Sum256([]byte("polyfin:" + kind + ":" + strings.ToLower(name)))
	return hex.EncodeToString(sum[:16])
}

var itemTypes = map[library.Kind]string{
	library.KindLibrary:    "CollectionFolder",
	library.KindCollection: "BoxSet",
	library.KindMovie:      "Movie",
	library.KindSeries:     "Series",
	library.KindSeason:     "Season",
	library.KindEpisode:    "Episode",
	library.KindPerson:     "Person",
	library.KindChannel:    "TvChannel",
	library.KindProgram:    "Program",
	// Jellyfin's recordings are videos of its recordings folders.
	library.KindRecording:     "Video",
	library.KindArtist:        "MusicArtist",
	library.KindAlbum:         "MusicAlbum",
	library.KindTrack:         "Audio",
	library.KindAudiobook:     "AudioBook",
	library.KindMusicPlaylist: "Playlist",
}

// isFolder reports whether items of a kind hold other items.
func isFolder(kind library.Kind) bool {
	switch kind {
	case library.KindLibrary, library.KindCollection, library.KindSeries, library.KindSeason,
		library.KindArtist, library.KindAlbum, library.KindMusicPlaylist:
		return true
	}
	return false
}

// newItemDto describes an item, with what the user did with it in state.
// detail is true for an item's own description, which carries every field;
// listings carry the base fields plus those in fields.
func (h *Handler) newItemDto(item library.Item, fields fieldSet, detail bool, state userState) BaseItemDto {
	playable := item.Kind == library.KindMovie || item.Kind == library.KindEpisode || item.Kind == library.KindRecording
	folder := isFolder(item.Kind)
	dto := BaseItemDto{
		Name:              item.Name,
		ServerId:          h.ServerID,
		Id:                item.ID.String(),
		OfficialRating:    item.OfficialRating,
		IsFolder:          folder,
		Type:              itemTypes[item.Kind],
		Status:            item.Status,
		CollectionType:    item.CollectionType,
		SeriesName:        item.SeriesName,
		ImageTags:         map[string]string{},
		BackdropImageTags: []string{},
		ImageBlurHashes:   map[string]map[string]string{},
		LocationType:      "FileSystem",
		MediaType:         mediaType(item.Kind),
		UserData:          state.of(item),
	}
	if item.Kind == library.KindEpisode && !item.Available {
		dto.LocationType = "Virtual"
	}
	if item.PremiereDate != nil {
		dto.PremiereDate = new(Time(*item.PremiereDate))
	}
	if item.Kind == library.KindSeries {
		if item.EndDate != nil {
			dto.EndDate = new(Time(*item.EndDate))
		} else if item.Status == "Ended" && item.Contents != nil && item.Contents.LastReleased != nil {
			dto.EndDate = new(Time(*item.Contents.LastReleased))
		}
	}
	if item.CommunityRating > 0 {
		dto.CommunityRating = new(item.CommunityRating)
	}
	if item.Runtime > 0 {
		dto.RunTimeTicks = new(int64(item.Runtime / 100))
	}
	if item.ProductionYear > 0 {
		dto.ProductionYear = new(item.ProductionYear)
	}
	switch item.Kind {
	case library.KindSeason:
		dto.IndexNumber = new(item.IndexNumber)
		dto.SeriesPrimaryImageTag = library.ImageTag(item.SeriesPoster)
	case library.KindEpisode:
		dto.IndexNumber = new(item.IndexNumber)
		dto.ParentIndexNumber = new(item.ParentIndexNumber)
		dto.SeasonId = item.SeasonID.String()
		dto.SeasonName = item.SeasonName
		dto.SeriesPrimaryImageTag = library.ImageTag(item.SeriesPoster)
		// Jellyfin shows the nearest parent artwork: the season's, else the
		// series'.
		if item.SeasonPoster != "" {
			dto.ParentPrimaryImageItemId, dto.ParentPrimaryImageTag = item.SeasonID.String(), library.ImageTag(item.SeasonPoster)
		} else if item.SeriesPoster != "" {
			dto.ParentPrimaryImageItemId, dto.ParentPrimaryImageTag = item.SeriesID.String(), library.ImageTag(item.SeriesPoster)
		}
	case library.KindSeries:
		dto.AirDays = &[]string{}
	case library.KindCollection:
		dto.DisplayOrder = "PremiereDate"
	}
	if playable {
		dto.VideoType = "VideoFile"
	}
	if item.SeriesID != (accounts.ID{}) {
		dto.SeriesId = item.SeriesID.String()
		dto.ParentBackdropItemId = item.SeriesID.String()
		dto.ParentBackdropImageTags = []string{}
	}
	h.setImages(&dto, item)

	if item.Overview != "" && (detail || fields.has("Overview")) {
		dto.Overview = new(item.Overview)
	}
	if detail || fields.has("Genres") {
		dto.Genres = new(nonNil(item.Genres))
	}
	if detail || fields.has("ProviderIds") {
		providers := item.ProviderIDs
		if providers == nil {
			providers = map[string]string{}
		}
		dto.ProviderIds = &providers
	}
	if detail || fields.has("DateCreated") {
		dto.DateCreated = new(Time(time.Unix(0, 0).UTC()))
	}
	if detail || fields.has("CanDelete") {
		dto.CanDelete = new(false)
	}
	if detail || fields.has("CanDownload") {
		// Until its versions are known; see setDownload.
		dto.CanDownload = new(false)
	}
	if detail || fields.has("SortName") {
		dto.SortName = strings.ToLower(item.Name)
	}
	if detail || fields.has("People") {
		dto.People = new(people(item.People))
	}
	if detail || fields.has("Chapters") {
		dto.Chapters = &[]ChapterInfo{}
	}
	if (detail || fields.has("ParentId")) && item.ParentID != (accounts.ID{}) {
		dto.ParentId = item.ParentID.String()
	}
	if detail {
		if item.Kind == library.KindMovie || item.Kind == library.KindSeries {
			dto.OriginalTitle = item.Name
		}
		dto.Etag = nameID("etag", item.ID.String())
		dto.ExternalUrls = new(externalURLs(item.ProviderIDs, item.Kind))
		dto.EnableMediaSourceDisplay = new(true)
		dto.Taglines = &[]string{}
		dto.PlayAccess = "Full"
		dto.RemoteTrailers = new(trailers(item.Trailers))
		dto.Studios = &[]NameGuidPair{}
		dto.GenreItems = new(genreItems(item.Genres))
		dto.LocalTrailerCount = new(0)
		dto.SpecialFeatureCount = new(0)
		dto.DisplayPreferencesId = item.ID.String()
		dto.Tags = &[]string{}
		dto.LockedFields = &[]string{}
		dto.LockData = new(false)
		if item.Kind == library.KindLibrary {
			// Jellyfin reports this date unset on libraries.
			dto.DateLastMediaAdded = new(Time(time.Time{}))
		}
		if c := item.Contents; c != nil {
			dto.ChildCount = new(c.Children)
			dto.RecursiveItemCount = new(c.Released)
			if c.LastReleased != nil {
				dto.DateLastMediaAdded = new(Time(*c.LastReleased))
			} else {
				dto.DateLastMediaAdded = new(Time(time.Time{}))
			}
			if item.Kind == library.KindSeries {
				dto.CumulativeRunTimeTicks = new(int64(c.Runtime / 100))
			}
		}
	}
	describeLive(&dto, item, fields, detail)
	describeMusic(&dto, item, fields, detail)
	return dto
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (h *Handler) setImages(dto *BaseItemDto, item library.Item) {
	if item.Images.Primary != "" {
		dto.ImageTags["Primary"] = library.ImageTag(item.Images.Primary)
		ratio := 2.0 / 3.0
		switch item.Kind {
		case library.KindEpisode, library.KindProgram, library.KindRecording:
			ratio = 16.0 / 9.0
		case library.KindChannel:
			// Channel logos are square, as Jellyfin's tuners give them.
			ratio = 1
		case library.KindArtist, library.KindAlbum, library.KindTrack, library.KindAudiobook, library.KindMusicPlaylist:
			// Covers are square.
			ratio = 1
		}
		dto.PrimaryImageAspectRatio = new(ratio)
	}
	if item.Images.Logo != "" {
		dto.ImageTags["Logo"] = library.ImageTag(item.Images.Logo)
	}
	if item.Images.Thumb != "" && item.Kind != library.KindEpisode {
		dto.ImageTags["Thumb"] = library.ImageTag(item.Images.Thumb)
	}
	if item.Images.Backdrop != "" {
		tag := library.ImageTag(item.Images.Backdrop)
		if item.Kind == library.KindSeason || item.Kind == library.KindEpisode {
			dto.ParentBackdropImageTags = []string{tag}
		} else {
			dto.BackdropImageTags = []string{tag}
		}
	}
}

func people(credits []library.Person) []BaseItemPerson {
	result := make([]BaseItemPerson, 0, len(credits))
	for _, person := range credits {
		entry := BaseItemPerson{Name: person.Name, Id: person.ID.String(), Role: person.Role, Type: person.Type}
		if person.Image != "" {
			entry.PrimaryImageTag = library.ImageTag(person.Image)
		}
		result = append(result, entry)
	}
	return result
}

func trailers(list []library.Trailer) []MediaUrl {
	result := make([]MediaUrl, 0, len(list))
	for _, trailer := range list {
		result = append(result, MediaUrl{Url: trailer.URL, Name: trailer.Name})
	}
	return result
}

func genreItems(genres []string) []NameGuidPair {
	result := make([]NameGuidPair, 0, len(genres))
	for _, genre := range genres {
		result = append(result, NameGuidPair{Name: genre, Id: nameID("genre", genre)})
	}
	return result
}

// externalURLs links a title to the sites of its provider identifiers.
func externalURLs(ids map[string]string, kind library.Kind) []MediaUrl {
	result := []MediaUrl{}
	if id := ids["Imdb"]; id != "" {
		result = append(result, MediaUrl{Name: "IMDb", Url: "https://www.imdb.com/title/" + id})
	}
	if id := ids["Tmdb"]; id != "" {
		path := "movie"
		if kind != library.KindMovie {
			path = "tv"
		}
		result = append(result, MediaUrl{Name: "TMDB", Url: "https://www.themoviedb.org/" + path + "/" + id})
	}
	if id := ids["Tvdb"]; id != "" && kind != library.KindMovie {
		result = append(result, MediaUrl{Name: "TheTVDB", Url: "https://thetvdb.com/?tab=series&id=" + id})
	}
	return result
}

// QueryResult is Jellyfin's paged list.
type QueryResult struct {
	Items            []BaseItemDto
	TotalRecordCount int
	StartIndex       int
}

// itemTypeFilter keeps the item types a request includes and drops those it
// excludes; mediaTypes further keeps only items of those media types, and
// the IsFolder and IsNotFolder filters only folders or the others.
func itemTypeFilter(r *http.Request) func(library.Item) bool {
	include := listQuery(r, "includeItemTypes")
	exclude := listQuery(r, "excludeItemTypes")
	media := listQuery(r, "mediaTypes")
	filters := listQuery(r, "filters")
	matches := func(list []string, value string) bool {
		return slices.ContainsFunc(list, func(t string) bool { return strings.EqualFold(t, value) })
	}
	folders, others := matches(filters, "IsFolder"), matches(filters, "IsNotFolder")
	return func(item library.Item) bool {
		itemType := itemTypes[item.Kind]
		folder := isFolder(item.Kind)
		return (len(include) == 0 || matches(include, itemType)) && !matches(exclude, itemType) &&
			(len(media) == 0 || matches(media, mediaType(item.Kind))) && (!folders || folder) && (!others || !folder)
	}
}

// mediaType is the kind of media an item plays: titles, episodes and
// channels are videos, and programmes, which play their channel; folders
// have none.
func mediaType(kind library.Kind) string {
	switch kind {
	case library.KindMovie, library.KindEpisode, library.KindChannel, library.KindProgram, library.KindRecording:
		return "Video"
	case library.KindTrack, library.KindAudiobook, library.KindMusicPlaylist:
		return "Audio"
	}
	return "Unknown"
}

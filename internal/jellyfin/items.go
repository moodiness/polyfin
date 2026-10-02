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
)

// UserItemData is a user's state for an item. Polyfin does not record
// playback yet: every item is unplayed and not a favorite.
type UserItemData struct {
	PlayedPercentage      *float64 `json:",omitempty"`
	UnplayedItemCount     *int     `json:",omitempty"`
	PlaybackPositionTicks int64
	PlayCount             int
	IsFavorite            bool
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
	Name                     string
	OriginalTitle            string `json:",omitempty"`
	ServerId                 string
	Id                       string
	Etag                     string             `json:",omitempty"`
	DateCreated              *Time              `json:",omitempty"`
	CanDelete                *bool              `json:",omitempty"`
	CanDownload              *bool              `json:",omitempty"`
	SortName                 string             `json:",omitempty"`
	PremiereDate             *Time              `json:",omitempty"`
	ExternalUrls             *[]MediaUrl        `json:",omitempty"`
	EnableMediaSourceDisplay *bool              `json:",omitempty"`
	OfficialRating           string             `json:",omitempty"`
	ChannelId                *string            // always sent, null
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
	UserData                 UserItemData
	RecursiveItemCount       *int      `json:",omitempty"`
	ChildCount               *int      `json:",omitempty"`
	SeriesName               string    `json:",omitempty"`
	SeriesId                 string    `json:",omitempty"`
	SeasonId                 string    `json:",omitempty"`
	SpecialFeatureCount      *int      `json:",omitempty"`
	DisplayPreferencesId     string    `json:",omitempty"`
	Status                   string    `json:",omitempty"`
	AirDays                  *[]string `json:",omitempty"`
	Tags                     *[]string `json:",omitempty"`
	PrimaryImageAspectRatio  *float64  `json:",omitempty"`
	SeriesPrimaryImageTag    string    `json:",omitempty"`
	SeasonName               string    `json:",omitempty"`
	CollectionType           string    `json:",omitempty"`
	DisplayOrder             string    `json:",omitempty"`
	ImageTags                map[string]string
	BackdropImageTags        []string
	ImageBlurHashes          map[string]map[string]string
	ParentPrimaryImageItemId string `json:",omitempty"`
	ParentPrimaryImageTag    string `json:",omitempty"`
	// Chapters is always empty: Polyfin knows no chapters.
	Chapters               *[]struct{} `json:",omitempty"`
	LocationType           string
	MediaType              string
	VideoType              string    `json:",omitempty"`
	EndDate                *Time     `json:",omitempty"`
	LockedFields           *[]string `json:",omitempty"`
	LockData               *bool     `json:",omitempty"`
	CumulativeRunTimeTicks *int64    `json:",omitempty"`
	DateLastMediaAdded     *Time     `json:",omitempty"`
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
}

// newItemDto describes an item. detail is true for an item's own
// description, which carries every field; listings carry the base fields
// plus those in fields.
func (h *Handler) newItemDto(item library.Item, fields fieldSet, detail bool) BaseItemDto {
	playable := item.Kind == library.KindMovie || item.Kind == library.KindEpisode
	folder := !playable && item.Kind != library.KindPerson
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
		UserData:          UserItemData{Key: hyphenated(item.ID), ItemId: item.ID.String()},
	}
	if item.Kind == library.KindEpisode && !item.Available {
		dto.LocationType = "Virtual"
	}
	if folder && item.Kind != library.KindLibrary {
		dto.UserData.PlayedPercentage = new(0.0)
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
	if item.Contents != nil {
		// Nothing is played yet: every released episode is unplayed.
		dto.UserData.UnplayedItemCount = new(item.Contents.Released)
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
	if detail || fields.has("SortName") {
		dto.SortName = strings.ToLower(item.Name)
	}
	if detail || fields.has("People") {
		dto.People = new(people(item.People))
	}
	if (detail || fields.has("ParentId")) && item.ParentID != (accounts.ID{}) {
		dto.ParentId = item.ParentID.String()
	}
	if detail {
		if item.Kind == library.KindMovie || item.Kind == library.KindSeries {
			dto.OriginalTitle = item.Name
		}
		dto.Etag = nameID("etag", item.ID.String())
		dto.CanDownload = new(false)
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
		dto.Chapters = &[]struct{}{}
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
		if item.Kind == library.KindEpisode {
			ratio = 16.0 / 9.0
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
// excludes; mediaTypes further keeps only items of those media types.
func itemTypeFilter(r *http.Request) func(library.Item) bool {
	include := listQuery(r, "includeItemTypes")
	exclude := listQuery(r, "excludeItemTypes")
	media := listQuery(r, "mediaTypes")
	return func(item library.Item) bool {
		matches := func(list []string, value string) bool {
			return slices.ContainsFunc(list, func(t string) bool { return strings.EqualFold(t, value) })
		}
		itemType := itemTypes[item.Kind]
		return (len(include) == 0 || matches(include, itemType)) && !matches(exclude, itemType) &&
			(len(media) == 0 || matches(media, mediaType(item.Kind)))
	}
}

// mediaType is the kind of media an item plays: only titles and episodes
// are videos, folders have none.
func mediaType(kind library.Kind) string {
	if kind == library.KindMovie || kind == library.KindEpisode {
		return "Video"
	}
	return "Unknown"
}

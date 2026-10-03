package jellyfin

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Genre, studio and year pages.
//
// Jellyfin apps open the page of a genre, studio or year a title carries,
// then list its titles with genreIds or genres, studioIds or studios, or
// years. Polyfin's titles come from addon catalogs, which can only be
// narrowed through their genre extra; addons offer genres there, and some
// also years, studios or networks. The titles of a page are therefore those
// the user's libraries list for an option of that name (see
// library.Service.Narrowed).

// namedItemDto describes a genre, studio or year as Jellyfin does on its
// page, in Jellyfin's key order.
type namedItemDto struct {
	Name                     string
	ServerId                 string
	Id                       string
	Etag                     string
	DateCreated              Time
	CanDelete                bool
	CanDownload              bool
	SortName                 string
	ExternalUrls             []MediaUrl
	EnableMediaSourceDisplay bool
	ChannelId                *string
	Taglines                 []string
	Genres                   []string
	PlayAccess               string `json:",omitempty"`
	RemoteTrailers           []MediaUrl
	ProviderIds              map[string]string
	ParentId                 *string
	Type                     string
	People                   []BaseItemPerson
	Studios                  []NameGuidPair
	GenreItems               []NameGuidPair
	LocalTrailerCount        int
	UserData                 *UserItemData `json:",omitempty"`
	ChildCount               int
	SpecialFeatureCount      int
	DisplayPreferencesId     string
	Tags                     []string
	ImageTags                map[string]string
	BackdropImageTags        []string
	ImageBlurHashes          map[string]map[string]string
	Chapters                 []struct{}
	LocationType             string
	MediaType                string
	LockedFields             []string
	TrailerCount             int
	MovieCount               int
	SeriesCount              int
	ProgramCount             int
	EpisodeCount             int
	SongCount                int
	AlbumCount               int
	ArtistCount              int
	MusicVideoCount          int
	LockData                 bool
}

// namedPage is the page being answered: the item's Jellyfin type, its name
// and identifier, how it sorts, and which genre options are its. refused
// is set for a year before 1, which does not exist.
type namedPage struct {
	itemType, name, id, sortName string
	matches                      func(option string) bool
	refused                      bool
}

func (h *Handler) genrePage(w http.ResponseWriter, r *http.Request) {
	h.namedPageByName(w, r, "Genre", "genreName", "genre")
}

func (h *Handler) studioPage(w http.ResponseWriter, r *http.Request) {
	h.namedPageByName(w, r, "Studio", "name", "studio")
}

// namedPageByName answers the page of a genre or studio, whose name is the
// route's parameter param and whose identifier is that of its kind, as in
// the titles' GenreItems and in /Genres.
func (h *Handler) namedPageByName(w http.ResponseWriter, r *http.Request, itemType, param, kind string) {
	b := bindErrors{}
	name := r.PathValue(param)
	if strings.TrimSpace(name) == "" {
		b.add(param, fmt.Sprintf("The %s field is required.", param))
	}
	h.writeNamedPage(w, r, b, namedPage{
		itemType: itemType, name: name, id: nameID(kind, name), sortName: strings.ToLower(name),
		matches: func(option string) bool { return strings.EqualFold(strings.TrimSpace(option), strings.TrimSpace(name)) },
	})
}

func (h *Handler) yearPage(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	raw := r.PathValue("year")
	year := 0
	if strings.TrimSpace(raw) == "" {
		b.add("year", fmt.Sprintf("The value '%s' is invalid.", raw))
	} else if value, message := convertInt32(raw); message != "" {
		b.add("year", message)
	} else {
		year = value
	}
	name := strconv.Itoa(year)
	h.writeNamedPage(w, r, b, namedPage{
		itemType: "Year", name: name, id: nameID("year", name), sortName: fmt.Sprintf("%010d", year),
		matches: func(option string) bool { return strings.TrimSpace(option) == name },
		refused: year < 1,
	})
}

// writeNamedPage answers a page once its parameters are bound. Like
// Jellyfin, which creates such an item the first time it is named, any name
// has a page: one that no catalog offers just has no titles. For a user
// that does not exist, Jellyfin answers the page without user data.
func (h *Handler) writeNamedPage(w http.ResponseWriter, r *http.Request, b bindErrors, page namedPage) {
	id, set := b.userID(r)
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	if page.refused {
		processingError(w, http.StatusBadRequest)
		return
	}
	dto := h.namedItem(page)
	user, ok := h.targetUser(w, r, id, set, func(w http.ResponseWriter) { writeJSON(w, http.StatusOK, dto) })
	if !ok {
		return
	}
	// Apps only show the sections of a page whose count is not zero. A
	// catalog's size is unknown until it is read through, so the counts are
	// those of a first page of titles, plus one when more follow: enough to
	// tell whether there are any, and the listing that follows reads the
	// same catalog pages again from cache.
	for _, count := range []struct {
		kind  library.Kind
		total *int
	}{{library.KindMovie, &dto.MovieCount}, {library.KindSeries, &dto.SeriesCount}} {
		titles, err := h.Library.Narrowed(r.Context(), user, page.matches, []library.Kind{count.kind}, 0, defaultPageSize)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		*count.total = titles.Total
	}
	dto.PlayAccess = "Full"
	dto.UserData = &UserItemData{Key: page.name, ItemId: page.id}
	writeJSON(w, http.StatusOK, dto)
}

// namedItem describes a page's item, without user data. Such an item has
// no artwork, files or metadata of its own.
func (h *Handler) namedItem(page namedPage) namedItemDto {
	return namedItemDto{
		Name:                     page.name,
		ServerId:                 h.ServerID,
		Id:                       page.id,
		Etag:                     nameID("etag", page.id),
		DateCreated:              Time(time.Unix(0, 0).UTC()),
		SortName:                 page.sortName,
		ExternalUrls:             []MediaUrl{},
		EnableMediaSourceDisplay: true,
		Taglines:                 []string{},
		Genres:                   []string{},
		RemoteTrailers:           []MediaUrl{},
		ProviderIds:              map[string]string{},
		Type:                     page.itemType,
		People:                   []BaseItemPerson{},
		Studios:                  []NameGuidPair{},
		GenreItems:               []NameGuidPair{},
		DisplayPreferencesId:     page.id,
		Tags:                     []string{},
		ImageTags:                map[string]string{},
		BackdropImageTags:        []string{},
		ImageBlurHashes:          map[string]map[string]string{},
		Chapters:                 []struct{}{},
		LocationType:             "FileSystem",
		MediaType:                "Unknown",
		LockedFields:             []string{},
	}
}

// nameFilter is what a listing asks its titles to be narrowed to: genres
// (genres, pipe-separated, or genreIds), studios (studios, pipe-separated,
// or studioIds) and years. A genre option matches when it is any of them.
type nameFilter struct {
	names, years, genreIDs, studioIDs []string
}

// nameFilterOf reads the filter of a listing; ok is false when a year is
// not a number, which Jellyfin refuses.
func nameFilterOf(r *http.Request) (f nameFilter, ok bool) {
	for _, param := range []string{"genres", "studios"} {
		for name := range strings.SplitSeq(query(r, param), "|") {
			if name = strings.TrimSpace(name); name != "" {
				f.names = append(f.names, name)
			}
		}
	}
	for _, raw := range listQuery(r, "years") {
		year, message := convertInt32(raw)
		if message != "" {
			return nameFilter{}, false
		}
		f.years = append(f.years, strconv.Itoa(year))
	}
	// Identifiers are compared in the form nameID gives them, whatever form
	// the app sent.
	ids := func(param string) []string {
		var result []string
		for _, raw := range listQuery(r, param) {
			if id, ok := parseGUID(raw); ok {
				result = append(result, id.String())
			}
		}
		return result
	}
	f.genreIDs, f.studioIDs = ids("genreIds"), ids("studioIds")
	return f, true
}

// requested reports whether the listing is narrowed at all.
func (f nameFilter) requested() bool {
	return len(f.names) > 0 || len(f.years) > 0 || len(f.genreIDs) > 0 || len(f.studioIDs) > 0
}

// matches reports whether a catalog's genre option is one the listing asks
// for. A catalog accepts a single option, so a listing that asks for
// several, of one kind or not, gets the titles of the first one a catalog
// offers.
func (f nameFilter) matches(option string) bool {
	option = strings.TrimSpace(option)
	return slices.ContainsFunc(f.names, func(name string) bool { return strings.EqualFold(name, option) }) ||
		slices.Contains(f.years, option) ||
		len(f.genreIDs) > 0 && slices.Contains(f.genreIDs, nameID("genre", option)) ||
		len(f.studioIDs) > 0 && slices.Contains(f.studioIDs, nameID("studio", option))
}

// narrowedListing lists, across the user's libraries, the titles of a genre,
// studio or year.
func (h *Handler) narrowedListing(w http.ResponseWriter, r *http.Request, user accounts.User, f nameFilter, start, limit int) {
	keep := itemTypeFilter(r)
	page, err := h.Library.Narrowed(r.Context(), user, f.matches, searchKinds(keep), max(start, 0), limit)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	items, err := h.dtos(r, user, page.Items, requestedFields(r), keep)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: page.Total, StartIndex: start})
}

package jellyfin

import (
	"cmp"
	"errors"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// The music of Eclipse addons reaches Jellyfin apps as Jellyfin's music
// libraries do: artists, albums, songs (Audio), audiobooks and playlists,
// listed by /Items and the artist routes, with instant mixes and lyrics.

func (h *Handler) musicRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/Artists", h.artists)
	signedIn(http.MethodGet, "/Artists/AlbumArtists", h.artists)
	signedIn(http.MethodGet, "/Artists/{name}", h.artistByName)
	signedIn(http.MethodGet, "/Artists/InstantMix", h.instantMix)
	signedIn(http.MethodGet, "/MusicGenres/InstantMix", h.instantMix)
	for _, prefix := range []string{"/Items", "/Albums", "/Artists", "/Songs", "/Playlists", "/MusicGenres"} {
		signedIn(http.MethodGet, prefix+"/{itemId}/InstantMix", h.instantMix)
	}
	// Addons give no genres per track: the music genre lists are empty, as
	// on a Jellyfin server whose music has no genre tags.
	signedIn(http.MethodGet, "/MusicGenres", h.emptyQueryResult)
	signedIn(http.MethodGet, "/Audio/{itemId}/Lyrics", h.lyrics)
	signedIn(http.MethodGet, "/Audio/{itemId}/RemoteSearch/Lyrics", h.remoteLyrics)
}

// credits names artists as Jellyfin lists them with their items.
func credits(list []library.Credit) *[]NameGuidPair {
	result := make([]NameGuidPair, 0, len(list))
	for _, c := range list {
		result = append(result, NameGuidPair{Name: c.Name, Id: c.ID.String()})
	}
	return &result
}

func creditNames(list []library.Credit) *[]string {
	names := make([]string, 0, len(list))
	for _, c := range list {
		names = append(names, c.Name)
	}
	return &names
}

// describeMusic adds to a DTO what describes music: a song's album and
// artists, an album's artists and, in its details, its tracks and length,
// as Jellyfin's do; an artist's details count their albums.
func describeMusic(dto *BaseItemDto, item library.Item, fields fieldSet, detail bool) {
	if !library.MusicKind(item.Kind) {
		return
	}
	albumArtists := []library.Credit{}
	if item.AlbumArtist != nil {
		albumArtists = []library.Credit{*item.AlbumArtist}
		dto.AlbumArtist = item.AlbumArtist.Name
	}
	children := 0
	if item.ChildCount != nil {
		children = *item.ChildCount
	}
	switch item.Kind {
	case library.KindTrack, library.KindAudiobook:
		dto.HasLyrics = new(false)
		if dto.Container == "" && item.Kind == library.KindTrack {
			// Until its stream is described: the format its addon labels it with.
			dto.Container = audioContainer(item.Container, "")
		}
		dto.Album = item.Album
		if item.AlbumID != (accounts.ID{}) {
			dto.AlbumId = item.AlbumID.String()
			dto.AlbumPrimaryImageTag = library.ImageTag(item.AlbumPoster)
		}
		if item.IndexNumber > 0 {
			dto.IndexNumber = new(item.IndexNumber)
		}
		dto.Artists, dto.ArtistItems, dto.AlbumArtists = creditNames(item.Artists), credits(item.Artists), credits(albumArtists)
	case library.KindAlbum:
		dto.Artists, dto.ArtistItems, dto.AlbumArtists = creditNames(item.Artists), credits(item.Artists), credits(albumArtists)
		if detail {
			dto.ChildCount, dto.RecursiveItemCount = new(children), new(children)
			dto.CumulativeRunTimeTicks = new(int64(item.Runtime / 100))
			dto.DateLastMediaAdded = new(Time(time.Time{}))
		}
	case library.KindArtist:
		// Jellyfin gives artists no length of their own.
		dto.RunTimeTicks = new(int64(0))
		if detail {
			dto.ChildCount, dto.RecursiveItemCount, dto.AlbumCount = new(children), new(0), new(children)
			dto.SongCount, dto.ArtistCount, dto.MovieCount, dto.SeriesCount = new(0), new(0), new(0), new(0)
			dto.EpisodeCount, dto.TrailerCount, dto.MusicVideoCount, dto.ProgramCount = new(0), new(0), new(0), new(0)
			dto.CumulativeRunTimeTicks = new(int64(0))
			dto.DateLastMediaAdded = new(Time(time.Time{}))
		}
	case library.KindMusicPlaylist:
		if detail {
			dto.ChildCount, dto.RecursiveItemCount = new(children), new(children)
			dto.CumulativeRunTimeTicks = new(int64(item.Runtime / 100))
		}
	}
}

// musicKinds are the item types of music, by the name Jellyfin gives
// them.
var musicKinds = map[string]library.Kind{
	"musicartist": library.KindArtist, "musicalbum": library.KindAlbum, "audio": library.KindTrack,
	"audiobook": library.KindAudiobook, "playlist": library.KindMusicPlaylist,
}

// requestedMusicKinds lists the kinds of music a listing's item types
// include, and whether it includes nothing else.
func requestedMusicKinds(r *http.Request) (kinds []library.Kind, onlyMusic bool) {
	include := listQuery(r, "includeItemTypes")
	onlyMusic = len(include) > 0
	for _, name := range include {
		if kind, ok := musicKinds[strings.ToLower(name)]; ok {
			if !slices.Contains(kinds, kind) {
				kinds = append(kinds, kind)
			}
		} else {
			onlyMusic = false
		}
	}
	return kinds, onlyMusic
}

// musicSearchKinds lists the kinds of music a search can return given the
// request's type filters.
func musicSearchKinds(keep func(library.Item) bool) []library.Kind {
	var kinds []library.Kind
	for _, kind := range []library.Kind{library.KindTrack, library.KindAudiobook, library.KindAlbum, library.KindArtist, library.KindMusicPlaylist} {
		if keep(library.Item{Kind: kind}) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// guidsOf binds the identifiers of list parameters, leaving out those that
// are not.
func guidsOf(r *http.Request, names ...string) []accounts.ID {
	var ids []accounts.ID
	for _, name := range names {
		for _, raw := range listQuery(r, name) {
			if id, ok := parseGUID(raw); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// musicListing answers the listings of music: a music library's, an
// album's, an artist's or a playlist's children, the tracks or albums of
// artists and albums named, or, without a folder, the music of every music
// library when only music is asked for. It reports whether it answered.
func (h *Handler) musicListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, hasParent bool, start, limit int) bool {
	kinds, onlyMusic := requestedMusicKinds(r)
	recursive, _ := boolQuery(r, "recursive")
	q := library.MusicQuery{Parent: parent, Kinds: kinds, Recursive: recursive,
		ArtistIDs: guidsOf(r, "artistIds", "albumArtistIds", "contributingArtistIds"), AlbumIDs: guidsOf(r, "albumIds")}
	switch {
	case len(q.ArtistIDs) > 0 || len(q.AlbumIDs) > 0:
		if !hasParent {
			q.Parent = accounts.ID{}
		}
	case hasParent:
		folder, err := h.Library.MusicFolder(r.Context(), user, parent)
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		if !folder {
			return false
		}
	case onlyMusic && recursive:
	default:
		return false
	}
	items, err := h.Library.Music(r.Context(), user, q)
	if errors.Is(err, library.ErrNotFound) {
		processingError(w, http.StatusBadRequest)
		return true
	}
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	h.writeMusic(w, r, user, items, start, limit, nil)
	return true
}

// writeMusic answers a page of music, kept by the request's type, name and
// state filters, and sorted as it asks, else by sorting.
func (h *Handler) writeMusic(w http.ResponseWriter, r *http.Request, user accounts.User, items []library.Item, start, limit int, sorting []string) {
	keep := itemTypeFilter(r)
	prefix := strings.ToLower(query(r, "nameStartsWith"))
	items = slices.DeleteFunc(items, func(item library.Item) bool {
		return !keep(item) || prefix != "" && !strings.HasPrefix(sortName(item), prefix)
	})
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if f, ok := stateFilterOf(r); ok {
		items = slices.DeleteFunc(items, func(item library.Item) bool { return !f.keeps(item, state) })
	}
	if keys := listQuery(r, "sortBy"); len(keys) > 0 {
		sorting = keys
	}
	sortMusic(items, sorting, listQuery(r, "sortOrder"), state)
	total := len(items)
	from := min(max(start, 0), total)
	page := items[from:min(from+max(limit, 0), total)]
	writeJSON(w, http.StatusOK, QueryResult{Items: h.listDtos(r, user, page, requestedFields(r), state), TotalRecordCount: total, StartIndex: start})
}

// sortName is what music sorts by name on.
func sortName(item library.Item) string {
	return strings.ToLower(strings.TrimSpace(item.Name))
}

// sortMusic sorts music by Jellyfin's sort keys, in order, each ascending
// unless its sort order says Descending. Without keys, or for keys music
// has no value for, the addon's order is kept: DateCreated, which counts
// as the order the addon lists its items in, keeps it too.
func sortMusic(items []library.Item, keys, orders []string, state userState) {
	descending := func(i int) bool {
		if i >= len(orders) {
			i = len(orders) - 1
		}
		return i >= 0 && strings.EqualFold(orders[i], "Descending")
	}
	if slices.ContainsFunc(keys, func(k string) bool { return strings.EqualFold(k, "Random") }) {
		rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
		return
	}
	artist := func(item library.Item) string {
		if len(item.Artists) > 0 {
			return strings.ToLower(item.Artists[0].Name)
		}
		return ""
	}
	albumArtist := func(item library.Item) string {
		if item.AlbumArtist != nil {
			return strings.ToLower(item.AlbumArtist.Name)
		}
		return artist(item)
	}
	slices.SortStableFunc(items, func(a, b library.Item) int {
		for i, key := range keys {
			var c int
			switch strings.ToLower(key) {
			case "sortname", "name":
				c = strings.Compare(sortName(a), sortName(b))
			case "album":
				c = strings.Compare(strings.ToLower(a.Album), strings.ToLower(b.Album))
			case "albumartist":
				c = strings.Compare(albumArtist(a), albumArtist(b))
			case "artist":
				c = strings.Compare(artist(a), artist(b))
			case "productionyear", "premieredate":
				c = cmp.Compare(a.ProductionYear, b.ProductionYear)
			case "runtime":
				c = cmp.Compare(a.Runtime, b.Runtime)
			case "indexnumber":
				c = cmp.Compare(a.IndexNumber, b.IndexNumber)
			case "playcount":
				c = cmp.Compare(state.data[a.ID].PlayCount, state.data[b.ID].PlayCount)
			case "dateplayed":
				c = compareTimes(state.data[a.ID].LastPlayed, state.data[b.ID].LastPlayed)
			case "isfavoriteorliked":
				c = compareFlags(state.of(a).IsFavorite, state.of(b).IsFavorite)
			}
			if c != 0 {
				if descending(i) {
					return -c
				}
				return c
			}
		}
		return 0
	})
}

func compareTimes(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return a.Compare(*b)
}

func compareFlags(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// artists lists the artists of the user's music, of one music library or
// folder with parentId, or those the user's music addons find for
// searchTerm: /Artists and /Artists/AlbumArtists alike, as music addons
// tell no album artist apart from the artists of the tracks.
func (h *Handler) artists(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, defaultPageSize)
	parent, hasParent := b.guid(r, "parentId")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	if hasParent && h.hiddenLibrary(w, r, user, parent) {
		return
	}
	var items []library.Item
	var err error
	if term := strings.TrimSpace(query(r, "searchTerm")); term != "" {
		items, err = h.Library.SearchMusic(r.Context(), user, term, []library.Kind{library.KindArtist}, max(start, 0)+limit)
	} else {
		if hasParent {
			folder, folderErr := h.Library.MusicFolder(r.Context(), user, parent)
			if folderErr != nil || !folder {
				writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
				return
			}
		}
		items, err = h.Library.Music(r.Context(), user, library.MusicQuery{Parent: parent, Kinds: []library.Kind{library.KindArtist}, Recursive: true})
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	items = slices.DeleteFunc(items, func(item library.Item) bool { return item.Kind != library.KindArtist })
	h.writeMusic(w, r, user, items, start, limit, []string{"SortName"})
}

// artistByName describes the artist of a name, which Jellyfin apps open
// artists by.
func (h *Handler) artistByName(w http.ResponseWriter, r *http.Request) {
	user, ok := h.viewer(w, r, bindErrors{}, notFoundProblem)
	if !ok {
		return
	}
	item, err := h.Library.ArtistByName(r.Context(), user, r.PathValue("name"))
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	state, err := h.userState(r.Context(), user, []library.Item{item})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.newItemDto(item, requestedFields(r), true, state))
}

// instantMixLimit is how many tracks an instant mix holds when the app
// sets no limit, as in Jellyfin.
const instantMixLimit = 200

// instantMix answers a mix of tracks made from an item: a track, an album,
// an artist, a playlist or a music library (see library.InstantMix). As in
// Jellyfin, TotalRecordCount counts the whole mix when limit cuts it. The
// item is in the route, or the id parameter.
func (h *Handler) instantMix(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	var id accounts.ID
	if r.PathValue("itemId") != "" {
		id = b.pathID(r, "itemId")
	} else {
		id, _ = b.guid(r, "id")
	}
	limit, limited := b.int32(r, "limit")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	mix, err := h.Library.InstantMix(r.Context(), user, id, instantMixLimit)
	if errors.Is(err, library.ErrNotFound) {
		if _, itemErr := h.Library.Item(r.Context(), user, id); itemErr == nil {
			// Something the user reaches that holds no music: no mix.
			mix, err = nil, nil
		}
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	total := len(mix)
	if limited && limit >= 0 {
		mix = mix[:min(limit, len(mix))]
	}
	dtos, err := h.dtos(r, user, mix, requestedFields(r), nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: total})
}

// lyrics answers as Jellyfin does for a track without lyrics, which every
// track of a music addon is: addons give none.
func (h *Handler) lyrics(w http.ResponseWriter, r *http.Request) {
	notFoundProblem(w)
}

// remoteLyrics answers a lyrics search: Polyfin has no lyrics provider,
// as a Jellyfin server without a lyrics plugin.
func (h *Handler) remoteLyrics(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	if _, err := h.Library.Item(r.Context(), user, id); err != nil {
		h.browseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, []struct{}{})
}

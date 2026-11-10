package jellyfin

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playlists"
)

// Playlists.
//
// Users make playlists of movies and episodes. The owner and the users a
// playlist is shared with to edit change it; the users it is shared with,
// or everyone when it is open, see it. To anyone else it does not exist, as
// in Jellyfin.

func (h *Handler) playlistRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodPost, "/Playlists", h.createPlaylist)
	signedIn(http.MethodGet, "/Playlists/{playlistId}", h.playlist)
	signedIn(http.MethodPost, "/Playlists/{playlistId}", h.updatePlaylist)
	signedIn(http.MethodGet, "/Playlists/{playlistId}/Items", h.playlistItems)
	signedIn(http.MethodPost, "/Playlists/{playlistId}/Items", h.addToPlaylist)
	signedIn(http.MethodDelete, "/Playlists/{playlistId}/Items", h.removeFromPlaylist)
	signedIn(http.MethodPost, "/Playlists/{playlistId}/Items/{itemId}/Move/{newIndex}", h.movePlaylistEntry)
	signedIn(http.MethodGet, "/Playlists/{playlistId}/Users", h.playlistUsers)
	signedIn(http.MethodGet, "/Playlists/{playlistId}/Users/{userId}", h.playlistUser)
	signedIn(http.MethodPost, "/Playlists/{playlistId}/Users/{userId}", h.sharePlaylist)
	signedIn(http.MethodDelete, "/Playlists/{playlistId}/Users/{userId}", h.unsharePlaylist)
	signedIn(http.MethodDelete, "/Items/{itemId}", h.deleteItem)
}

// PlaylistCreationResult identifies a new playlist.
type PlaylistCreationResult struct {
	Id string
}

// PlaylistUserPermissions is a user a playlist is shared with.
type PlaylistUserPermissions struct {
	UserId  string
	CanEdit bool
}

// PlaylistDto describes who may see a playlist and the titles it holds.
type PlaylistDto struct {
	OpenAccess bool
	Shares     []PlaylistUserPermissions
	ItemIds    []string
}

// createPlaylistRequest is Jellyfin's CreatePlaylistDto. Identifiers stay
// text until they are checked.
type createPlaylistRequest struct {
	Name      *string
	Ids       []string
	UserId    *string
	MediaType *string
	Users     []PlaylistUserPermissions
	IsPublic  bool
}

// updatePlaylistRequest is Jellyfin's UpdatePlaylistDto: null fields keep
// what the playlist has.
type updatePlaylistRequest struct {
	Name     *string
	Ids      *[]string
	Users    *[]PlaylistUserPermissions
	IsPublic *bool
}

// playlistsViewID identifies the Playlists view, the same on every server.
var playlistsViewID, _ = accounts.ParseID(nameID("view", "playlists"))

// mediaTypes are the values of Jellyfin's MediaType. Polyfin's playlists
// only hold videos, whatever an app names.
var mediaTypes = []string{"Unknown", "Video", "Audio", "Photo", "Book"}

func playlistNotFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, "Playlist not found")
}

// visiblePlaylist loads the playlist id as user sees it. One user may not
// see is answered as missing. ok is false once w was answered.
func (h *Handler) visiblePlaylist(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) (playlists.Playlist, bool) {
	p, err := h.Playlists.Get(r.Context(), id)
	if errors.Is(err, playlists.ErrNotFound) || err == nil && !p.Visible(user.ID) {
		playlistNotFound(w)
		return playlists.Playlist{}, false
	}
	if err != nil {
		h.internalError(w, r, err)
		return playlists.Playlist{}, false
	}
	return p, true
}

// editablePlaylist loads the playlist id for user to change, and refuses
// with an empty 403, as Jellyfin does, a user who only sees it.
func (h *Handler) editablePlaylist(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) (playlists.Playlist, bool) {
	p, ok := h.visiblePlaylist(w, r, user, id)
	if ok && !p.Editable(user.ID) {
		w.WriteHeader(http.StatusForbidden)
		return playlists.Playlist{}, false
	}
	return p, ok
}

// playlistChanged answers a change to a playlist.
func (h *Handler) playlistChanged(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, playlists.ErrNotFound):
		// Another request deleted it meanwhile.
		playlistNotFound(w)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// readJSONBody reads an optional JSON body into into. present is false for
// an empty body; ok is false once w was answered, required naming the
// parameter whose binding failed.
func readJSONBody(w http.ResponseWriter, r *http.Request, into any, required string) (present, ok bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return false, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return false, true
	}
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return false, false
	}
	if err := json.Unmarshal(body, into); err != nil {
		problem := map[string][]string{"$": {"The JSON value could not be converted."}}
		if required != "" {
			problem[required] = []string{"The " + required + " field is required."}
		}
		validationProblem(w, problem)
		return false, false
	}
	return true, true
}

// requireBody answers a required body that was not sent.
func requireBody(w http.ResponseWriter, parameter string) {
	validationProblem(w, map[string][]string{
		"":        {"A non-empty request body is required."},
		parameter: {"The " + parameter + " field is required."},
	})
}

// bodyIDs converts the identifiers of a JSON body, collecting those that do
// not convert under path.
func (b bindErrors) bodyIDs(path string, raw []string) []accounts.ID {
	ids := make([]accounts.ID, 0, len(raw))
	for i, value := range raw {
		id, ok := parseGUID(value)
		if !ok {
			b.add(fmt.Sprintf("%s[%d]", path, i), jsonCannotConvert("System.Guid"))
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// bodyShares converts the users of a JSON body.
func (b bindErrors) bodyShares(users []PlaylistUserPermissions) []playlists.Share {
	shares := make([]playlists.Share, 0, len(users))
	for i, user := range users {
		id, ok := parseGUID(user.UserId)
		if !ok {
			b.add(fmt.Sprintf("$.Users[%d].UserId", i), jsonCannotConvert("System.Guid"))
			continue
		}
		shares = append(shares, playlists.Share{User: id, CanEdit: user.CanEdit})
	}
	return shares
}

// queryIDs binds a list of identifiers, repeated or comma-separated; values
// that are not identifiers are left out.
func queryIDs(r *http.Request, name string) []accounts.ID {
	var ids []accounts.ID
	for _, raw := range listQuery(r, name) {
		if id, ok := parseGUID(raw); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func isMediaType(raw string) bool {
	return slices.ContainsFunc(mediaTypes, func(name string) bool { return strings.EqualFold(name, strings.TrimSpace(raw)) })
}

// playlistTitles lists the movies and episodes ids name, as Jellyfin
// expands folders into playlists: a series adds its aired episodes of
// regular seasons, a season its aired episodes, in order (see
// library.PlaylistEpisodes). Other items, and those user cannot reach,
// are left out.
func (h *Handler) playlistTitles(ctx context.Context, user accounts.User, ids []accounts.ID) ([]accounts.ID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	items, err := h.Library.Items(ctx, user, ids)
	if err != nil {
		return nil, err
	}
	var titles []accounts.ID
	for _, item := range items {
		targets, err := h.playlistTargets(ctx, user, item)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			titles = append(titles, target.ID)
		}
	}
	return titles, nil
}

// playlistTargets lists what a playlist takes for item: a series' or a
// season's episodes as playlistTitles tells, what a played mark applies to
// for anything else.
func (h *Handler) playlistTargets(ctx context.Context, user accounts.User, item library.Item) ([]library.Item, error) {
	switch item.Kind {
	case library.KindSeries:
		return h.Library.PlaylistEpisodes(ctx, user, item.ID, nil)
	case library.KindSeason:
		return h.Library.PlaylistEpisodes(ctx, user, item.SeriesID, &item.ID)
	}
	return h.markTargets(ctx, user, item)
}

// createPlaylist creates a playlist from the CreatePlaylistDto body or, as
// older apps send it, from query parameters, which take precedence.
func (h *Handler) createPlaylist(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	userID, userSet := b.guid(r, "userId")
	if raw := query(r, "mediaType"); strings.TrimSpace(raw) != "" && !isMediaType(raw) {
		b.add("mediaType", notValid(raw))
	}
	var request createPlaylistRequest
	if _, ok := readJSONBody(w, r, &request, ""); !ok {
		return
	}
	if request.MediaType != nil && !isMediaType(*request.MediaType) {
		b.add("$.MediaType", jsonCannotConvert("Jellyfin.Data.Enums.MediaType"))
	}
	ids := queryIDs(r, "ids")
	if len(ids) == 0 {
		ids = b.bodyIDs("$.Ids", request.Ids)
	}
	shares := b.bodyShares(request.Users)
	if !userSet && request.UserId != nil {
		if id, ok := parseGUID(*request.UserId); ok {
			userID, userSet = id, id != accounts.ID{}
		} else {
			b.add("$.UserId", jsonCannotConvert("System.Nullable`1[System.Guid]"))
		}
	}
	name := query(r, "name")
	if name == "" && request.Name != nil {
		name = *request.Name
	}
	if name == "" {
		b.add("Name", "The Name field is required.")
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user, ok := h.targetUser(w, r, userID, userSet, notFoundProblem)
	if !ok {
		return
	}
	titles, err := h.playlistTitles(r.Context(), user, ids)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	id, err := h.Playlists.Create(r.Context(), playlists.Draft{
		Owner: user.ID, Name: name, OpenAccess: request.IsPublic, Shares: shares, Items: titles,
	})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, PlaylistCreationResult{Id: id.String()})
}

func (h *Handler) playlist(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "playlistId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	p, ok := h.visiblePlaylist(w, r, callerFrom(r.Context()).User, id)
	if !ok {
		return
	}
	dto := PlaylistDto{OpenAccess: p.OpenAccess, Shares: sharesOf(p), ItemIds: make([]string, 0, len(p.Entries))}
	for _, entry := range p.Entries {
		dto.ItemIds = append(dto.ItemIds, entry.Item.String())
	}
	writeJSON(w, http.StatusOK, dto)
}

func sharesOf(p playlists.Playlist) []PlaylistUserPermissions {
	shares := make([]PlaylistUserPermissions, 0, len(p.Shares))
	for _, share := range p.Shares {
		shares = append(shares, PlaylistUserPermissions{UserId: share.User.String(), CanEdit: share.CanEdit})
	}
	return shares
}

// updatePlaylist renames a playlist, replaces its titles or users, or opens
// or closes it.
func (h *Handler) updatePlaylist(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "playlistId")
	var request updatePlaylistRequest
	present, ok := readJSONBody(w, r, &request, "updatePlaylistRequest")
	if !ok {
		return
	}
	if !present {
		requireBody(w, "updatePlaylistRequest")
		return
	}
	changes := playlists.Changes{Name: request.Name, OpenAccess: request.IsPublic}
	var ids []accounts.ID
	if request.Ids != nil {
		ids = b.bodyIDs("$.Ids", *request.Ids)
	}
	if request.Users != nil {
		changes.Shares = new(b.bodyShares(*request.Users))
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user := callerFrom(r.Context()).User
	p, ok := h.editablePlaylist(w, r, user, id)
	if !ok {
		return
	}
	if p.Watchlist && request.Name != nil {
		// jellyfin-web's edit dialog sends back the name it showed: the
		// default name, as the caller sees it, stays the default, which
		// follows each viewer's language.
		shown, err := h.playlistName(r.Context(), user, playlists.Playlist{Owner: p.Owner, Watchlist: true})
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if name := strings.TrimSpace(*request.Name); name == "" || name == shown {
			changes.Name = new("")
		}
	}
	if request.Ids != nil && !p.Watchlist {
		titles, err := h.playlistTitles(r.Context(), user, ids)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		changes.Items = &titles
	}
	if err := h.Playlists.Update(r.Context(), p.ID, changes); err != nil || request.Ids == nil || !p.Watchlist {
		h.playlistChanged(w, r, err)
		return
	}
	groups, err := h.watchlistItems(r.Context(), user, ids)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	h.playlistChanged(w, r, h.Library.ReplaceWatchlist(r.Context(), p.Owner, groups))
}

func (h *Handler) playlistItems(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "playlistId")
	start, limit := b.paging(r, -1)
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	// A music addon's playlist lists its tracks.
	if folder, err := h.Library.MusicFolder(r.Context(), user, id); err == nil && folder {
		tracks, err := h.Library.Music(r.Context(), user, library.MusicQuery{Parent: id})
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		if limit < 0 {
			limit = len(tracks)
		}
		h.writeMusic(w, r, user, tracks, start, limit, nil)
		return
	}
	p, ok := h.visiblePlaylist(w, r, user, id)
	if !ok {
		return
	}
	shown, err := h.shownPlaylists(r.Context(), user, []playlists.Playlist{p})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if len(shown) == 0 {
		playlistNotFound(w)
		return
	}
	h.writePlaylistEntries(w, r, user, p, start, limit, false)
}

// writePlaylistEntries answers the titles of a playlist that user can
// reach, in order, each with the entry it is listed as. Listed as its
// children (sorted), as Jellyfin does, it follows sortBy when asked, Random
// included, which Shuffle asks for.
func (h *Handler) writePlaylistEntries(w http.ResponseWriter, r *http.Request, user accounts.User, p playlists.Playlist, start, limit int,
	sorted bool) {
	titles, err := h.playlistContents(r.Context(), user, p)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	entries, err := h.shownEntries(r.Context(), user, p, titles)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	// SortBy=None, which one of Jellyfin's official apps asks and Jellyfin
	// drops, keeps the playlist's order.
	keys := slices.DeleteFunc(listQuery(r, "sortBy"), func(key string) bool { return strings.EqualFold(key, "None") })
	if sorted && len(keys) > 0 && len(entries) > 1 {
		all := make([]library.Item, len(entries))
		for i, entry := range entries {
			all[i] = titles[entry.Item]
		}
		state, err := h.userState(r.Context(), user, all)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		order := sortedOrder(r, all, state)
		ordered := make([]playlists.Entry, len(entries))
		for i, k := range order {
			ordered[i] = entries[k]
		}
		entries = ordered
	}
	from, to := bounds(len(entries), start, limit)
	page := entries[from:to]
	items := make([]library.Item, len(page))
	for i, entry := range page {
		items[i] = titles[entry.Item]
	}
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	dtos := h.listDtos(r, user, items, requestedFields(r), state)
	for i := range dtos {
		dtos[i].PlaylistItemId = page[i].ID.String()
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(entries), StartIndex: start})
}

// bounds returns the part of n results a page from start shows; a negative
// limit shows them all.
func bounds(n, start, limit int) (from, to int) {
	from = min(max(start, 0), n)
	if limit < 0 {
		return from, n
	}
	return from, min(from+limit, n)
}

// playlistContents describes, by identifier, the titles of playlists that
// user can reach, and the request's rating filter keeps.
func (h *Handler) playlistContents(ctx context.Context, user accounts.User, lists ...playlists.Playlist) (map[accounts.ID]library.Item, error) {
	var ids []accounts.ID
	seen := map[accounts.ID]bool{}
	for _, p := range lists {
		for _, entry := range p.Entries {
			if !seen[entry.Item] {
				seen[entry.Item] = true
				ids = append(ids, entry.Item)
			}
		}
	}
	titles := make(map[accounts.ID]library.Item, len(ids))
	if len(ids) == 0 {
		return titles, nil
	}
	items, err := h.Library.Items(ctx, user, ids)
	if err == nil {
		items, err = h.Library.RatingFiltered(ctx, user, items)
	}
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		titles[item.ID] = item
	}
	return titles, nil
}

// reachable lists the entries of p whose titles are in titles: a title
// from an addon the user does not have is not shown to them.
func reachable(p playlists.Playlist, titles map[accounts.ID]library.Item) []playlists.Entry {
	entries := make([]playlists.Entry, 0, len(p.Entries))
	for _, entry := range p.Entries {
		if _, ok := titles[entry.Item]; ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// shownEntries lists the entries of p user is shown, whose titles are in
// titles (see reachable): a watchlist leaves out the titles user hid.
func (h *Handler) shownEntries(ctx context.Context, user accounts.User, p playlists.Playlist, titles map[accounts.ID]library.Item) ([]playlists.Entry, error) {
	entries := reachable(p, titles)
	if !p.Watchlist || len(entries) == 0 {
		return entries, nil
	}
	hidden, err := h.Library.HiddenTitles(ctx, user)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(entries, func(e playlists.Entry) bool { return hidden.Hides(titles[e.Item]) }), nil
}

// posterTitles lists the titles whose posters entries show, in order,
// each once: an episode shows its series'.
func posterTitles(entries []playlists.Entry, titles map[accounts.ID]library.Item) []accounts.ID {
	ids := make([]accounts.ID, 0, len(entries))
	for _, entry := range entries {
		title := titles[entry.Item]
		id := title.ID
		if title.Kind == library.KindEpisode && title.SeriesID != (accounts.ID{}) {
			id = title.SeriesID
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// kindPlaylist is the kind of a playlist described as an item to the user
// data routes (see playlistItem).
const kindPlaylist library.Kind = "playlist"

// shownTitles loads the playlist id that user sees, and lists the titles
// it shows them, in order, once per entry. ok is false when user sees no
// such playlist.
func (h *Handler) shownTitles(ctx context.Context, user accounts.User, id accounts.ID) (p playlists.Playlist, shown []library.Item, ok bool, err error) {
	p, err = h.Playlists.Get(ctx, id)
	if errors.Is(err, playlists.ErrNotFound) || err == nil && !p.Visible(user.ID) {
		return playlists.Playlist{}, nil, false, nil
	}
	if err != nil {
		return playlists.Playlist{}, nil, false, err
	}
	titles, err := h.playlistContents(ctx, user, p)
	if err != nil {
		return playlists.Playlist{}, nil, false, err
	}
	entries, err := h.shownEntries(ctx, user, p, titles)
	if err != nil {
		return playlists.Playlist{}, nil, false, err
	}
	shown = make([]library.Item, len(entries))
	for i, entry := range entries {
		shown[i] = titles[entry.Item]
	}
	return p, shown, true, nil
}

// playlistItem describes the playlist id that user sees as an item of the
// user data routes, which jellyfin-web's playlist page marks played and
// favorite: a folder of the titles it shows user, played once they all
// are. library.ErrNotFound names none.
func (h *Handler) playlistItem(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error) {
	p, shown, ok, err := h.shownTitles(ctx, user, id)
	if err != nil || !ok {
		return library.Item{}, cmp.Or(err, library.ErrNotFound)
	}
	name, err := h.playlistName(ctx, user, p)
	if err != nil {
		return library.Item{}, err
	}
	contents := &library.Contents{Children: len(shown), Released: len(shown), Videos: make([]accounts.ID, len(shown))}
	for i, title := range shown {
		contents.Videos[i] = title.ID
	}
	return library.Item{ID: p.ID, Kind: kindPlaylist, Name: name, Contents: contents}, nil
}

// playlistMarkTargets lists what marking the playlist item played marks:
// each title it shows user, once, as Jellyfin marks a playlist's
// children.
func (h *Handler) playlistMarkTargets(ctx context.Context, user accounts.User, item library.Item) ([]library.Item, error) {
	_, shown, _, err := h.shownTitles(ctx, user, item.ID)
	seen := map[accounts.ID]bool{}
	return slices.DeleteFunc(shown, func(title library.Item) bool {
		was := seen[title.ID]
		seen[title.ID] = true
		return was
	}), err
}

// playItems replaces, among ids, the playlists user sees with the titles
// they show user, in order, as Jellyfin's play commands play a playlist:
// the session told gets them rather than the playlist. Other identifiers
// stay as they are.
func (h *Handler) playItems(ctx context.Context, user accounts.User, ids []accounts.ID) ([]accounts.ID, error) {
	result := make([]accounts.ID, 0, len(ids))
	for _, id := range ids {
		_, shown, ok, err := h.shownTitles(ctx, user, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			result = append(result, id)
			continue
		}
		for _, title := range shown {
			result = append(result, title.ID)
		}
	}
	return result, nil
}

// playlistsAsked tells whether a listing asks the user's own playlists
// (videos) and those of local music folders, from its item and media
// types and filters; asked is false when it asks for no playlist at all,
// as a listing without parent that names no Playlist type does.
func playlistsAsked(r *http.Request, hasParent bool) (own, folders, asked bool) {
	matches := func(list []string, value string) bool {
		return slices.ContainsFunc(list, func(t string) bool { return strings.EqualFold(t, value) })
	}
	include := listQuery(r, "includeItemTypes")
	if !hasParent && !matches(include, "Playlist") {
		return false, false, false
	}
	media := listQuery(r, "mediaTypes")
	excluded := len(include) > 0 && !matches(include, "Playlist") || matches(listQuery(r, "excludeItemTypes"), "Playlist") ||
		matches(listQuery(r, "filters"), "IsNotFolder")
	return !excluded && (len(media) == 0 || matches(media, "Video")), !excluded && (len(media) == 0 || matches(media, "Audio")), true
}

// searchPlaylists answers the playlists of user's own a search finds, by
// the name user sees them by, as Jellyfin's search finds playlists: when
// it names the Playlist type or none, across the server or in the
// Playlists view. Those of local music folders come with the music.
func (h *Handler) searchPlaylists(r *http.Request, user accounts.User, term string, parent accounts.ID, hasParent bool) ([]BaseItemDto, error) {
	if hasParent && parent != playlistsViewID {
		return nil, nil
	}
	if len(listQuery(r, "includeItemTypes")) == 0 {
		// A search naming no type asks for playlists too.
		hasParent = true
	}
	if own, _, _ := playlistsAsked(r, hasParent); !own {
		return nil, nil
	}
	lists, err := h.Playlists.Visible(r.Context(), user.ID)
	if err == nil {
		lists, err = h.shownPlaylists(r.Context(), user, lists)
	}
	if err == nil {
		err = h.namePlaylists(r.Context(), user, lists)
	}
	if err != nil {
		return nil, err
	}
	folded := foldName(term)
	lists = slices.DeleteFunc(lists, func(p playlists.Playlist) bool { return !strings.Contains(foldName(p.Name), folded) })
	f, filtered := stateFilterOf(r)
	dtos, err := h.playlistDtos(r, user, lists, requestedFields(r), false)
	if err != nil || !filtered {
		return dtos, err
	}
	return slices.DeleteFunc(dtos, func(dto BaseItemDto) bool { return !f.keepsData(dto.UserData, false) }), nil
}

// foldName is what playlists sort, narrow and are searched by: their name
// without regard to case or accents, as Jellyfin sorts and searches names,
// so that "À regarder" sorts among the A.
func foldName(name string) string {
	decomposed := norm.NFD.String(strings.ToLower(strings.TrimSpace(name)))
	return norm.NFC.String(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, decomposed))
}

// addToPlaylist adds titles at position, or last; a watchlist's go first
// unless a position is given.
func (h *Handler) addToPlaylist(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "playlistId")
	position, positioned := b.int32(r, "position")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	p, ok := h.editablePlaylist(w, r, user, id)
	if !ok {
		return
	}
	if p.Watchlist {
		groups, err := h.watchlistItems(r.Context(), user, queryIDs(r, "ids"))
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		if !positioned {
			position = 0
		}
		h.playlistChanged(w, r, h.Library.AddToWatchlist(r.Context(), p.Owner, groups, position))
		return
	}
	titles, err := h.playlistTitles(r.Context(), user, queryIDs(r, "ids"))
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	if !positioned {
		position = math.MaxInt32
	}
	h.playlistChanged(w, r, h.Playlists.Add(r.Context(), p.ID, titles, position))
}

// routePlaylist binds the playlist of the routes Jellyfin takes its
// identifier as text for, and loads it for the caller to change. Jellyfin
// knows no playlist by an identifier that does not parse either.
func (h *Handler) routePlaylist(w http.ResponseWriter, r *http.Request) (playlists.Playlist, bool) {
	id, ok := parseGUID(r.PathValue("playlistId"))
	if !ok {
		playlistNotFound(w)
		return playlists.Playlist{}, false
	}
	return h.editablePlaylist(w, r, callerFrom(r.Context()).User, id)
}

// removeFromPlaylist removes entries, named by the PlaylistItemId they are
// listed with.
func (h *Handler) removeFromPlaylist(w http.ResponseWriter, r *http.Request) {
	p, ok := h.routePlaylist(w, r)
	if !ok {
		return
	}
	entries := queryIDs(r, "entryIds")
	if p.Watchlist {
		h.playlistChanged(w, r, h.Library.RemoveWatchlistEntries(r.Context(), p.Owner, entries))
		return
	}
	h.playlistChanged(w, r, h.Playlists.Remove(r.Context(), p.ID, entries))
}

// movePlaylistEntry moves an entry to newIndex among the entries the
// caller sees, as Jellyfin does: entries whose titles the caller cannot
// reach keep their place relative to the others.
func (h *Handler) movePlaylistEntry(w http.ResponseWriter, r *http.Request) {
	index, message := convertInt32(r.PathValue("newIndex"))
	if message != "" {
		validationProblem(w, map[string][]string{"newIndex": {message}})
		return
	}
	p, ok := h.routePlaylist(w, r)
	if !ok {
		return
	}
	entry, ok := parseGUID(r.PathValue("itemId"))
	if !ok {
		// Like an entry the playlist does not have, it moves nothing.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	user := callerFrom(r.Context()).User
	titles, err := h.playlistContents(r.Context(), user, p)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	shown, err := h.shownEntries(r.Context(), user, p, titles)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	others := slices.DeleteFunc(shown, func(e playlists.Entry) bool { return e.ID == entry })
	var before accounts.ID
	if index < len(others) {
		before = others[max(index, 0)].ID
	}
	if p.Watchlist {
		h.playlistChanged(w, r, h.Library.MoveInWatchlist(r.Context(), p.Owner, entry, before))
		return
	}
	h.playlistChanged(w, r, h.Playlists.Move(r.Context(), p.ID, entry, before))
}

// playlistUsers lists the users a playlist is shared with, to its owner
// only.
func (h *Handler) playlistUsers(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "playlistId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user := callerFrom(r.Context()).User
	p, ok := h.visiblePlaylist(w, r, user, id)
	if !ok {
		return
	}
	if p.Owner != user.ID {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, sharesOf(p))
}

// playlistUser describes what a user may do with a playlist. Like Jellyfin,
// it answers the owner that they may edit, whoever they ask about, and
// tells other users only about themselves unless they may edit.
func (h *Handler) playlistUser(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "playlistId")
	target := b.pathID(r, "userId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user := callerFrom(r.Context()).User
	p, ok := h.visiblePlaylist(w, r, user, id)
	if !ok {
		return
	}
	if p.Owner == user.ID {
		writeJSON(w, http.StatusOK, PlaylistUserPermissions{UserId: user.ID.String(), CanEdit: true})
		return
	}
	if !p.Editable(user.ID) && target != user.ID {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	share, shared := p.Share(target)
	if !shared {
		writeJSON(w, http.StatusNotFound, "User permissions not found")
		return
	}
	writeJSON(w, http.StatusOK, PlaylistUserPermissions{UserId: share.User.String(), CanEdit: share.CanEdit})
}

// sharePlaylist shares a playlist with a user, or changes whether they may
// edit it. Only the owner shares.
func (h *Handler) sharePlaylist(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "playlistId")
	target := b.pathID(r, "userId")
	var request struct{ CanEdit *bool }
	present, ok := readJSONBody(w, r, &request, "updatePlaylistUserRequest")
	if !ok {
		return
	}
	if !present {
		b.add("updatePlaylistUserRequest", "The updatePlaylistUserRequest field is required.")
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user := callerFrom(r.Context()).User
	p, ok := h.visiblePlaylist(w, r, user, id)
	if !ok {
		return
	}
	if p.Owner != user.ID {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if _, err := h.Accounts.User(r.Context(), target); err != nil {
		if errors.Is(err, accounts.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, "User not found")
		} else {
			h.internalError(w, r, err)
		}
		return
	}
	share := playlists.Share{User: target, CanEdit: request.CanEdit != nil && *request.CanEdit}
	h.playlistChanged(w, r, h.Playlists.SetShare(r.Context(), p.ID, share))
}

// unsharePlaylist stops sharing a playlist with a user; those who may edit
// it may.
func (h *Handler) unsharePlaylist(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "playlistId")
	target := b.pathID(r, "userId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	p, ok := h.editablePlaylist(w, r, callerFrom(r.Context()).User, id)
	if !ok {
		return
	}
	if _, shared := p.Share(target); !shared {
		writeJSON(w, http.StatusNotFound, "User permissions not found")
		return
	}
	h.playlistChanged(w, r, h.Playlists.RemoveShare(r.Context(), p.ID, target))
}

// deleteItem deletes a playlist, which its owner or an administrator may
// do, or a collection (see deleteCollection). A watchlist forgets its
// owner's own list, and comes back empty at the next need (see
// watchlist.Store.Delete). Titles come from addons and are never deleted:
// like Jellyfin for a user who may not delete an item, Polyfin answers
// 401.
func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user := callerFrom(r.Context()).User
	if h.deleteCollection(w, r, user, id) {
		return
	}
	p, err := h.Playlists.Get(r.Context(), id)
	switch {
	case err == nil && p.Visible(user.ID):
		if p.Owner != user.ID && !user.IsAdministrator {
			writeJSON(w, http.StatusUnauthorized, "Unauthorized access")
			return
		}
		if p.Watchlist {
			err = h.Library.DeleteWatchlist(r.Context(), p.Owner)
		} else {
			err = h.Playlists.Delete(r.Context(), p.ID)
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	case err == nil:
		notFoundProblem(w)
		return
	case !errors.Is(err, playlists.ErrNotFound):
		h.internalError(w, r, err)
		return
	}
	if _, err := h.Library.Item(r.Context(), user, id); err != nil {
		h.browseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusUnauthorized, "Unauthorized access")
}

// Browsing playlists.

// playlistsViewItem is the Playlists view, among the user's libraries,
// named in the server's language, with its emblem.
func (h *Handler) playlistsViewItem() library.Item {
	return withEmblem(library.Item{ID: playlistsViewID, Kind: library.KindLibrary,
		Name: library.PlaylistsName(h.Accounts.Settings().Language), CollectionType: "playlists"})
}

// playlistsView describes the Playlists view.
func (h *Handler) playlistsView(r *http.Request, user accounts.User) (BaseItemDto, error) {
	dtos, err := h.folderDtos(r, user, []library.Item{h.playlistsViewItem()})
	if err != nil {
		return BaseItemDto{}, err
	}
	dtos[0].Type = "ManualPlaylistsFolder"
	return dtos[0], nil
}

// addPlaylistsView adds the Playlists view to views once user may see a
// playlist, as Jellyfin shows it once there is one: every user has their
// watchlist, until they delete it.
func (h *Handler) addPlaylistsView(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	visible, err := h.Playlists.AnyVisible(r.Context(), user.ID)
	if err != nil || !visible {
		return views, err
	}
	view, err := h.playlistsView(r, user)
	if err != nil {
		return nil, err
	}
	return append(views, view), nil
}

// playlistListing answers the listings that hold playlists: the Playlists
// view's, a listing of playlists across the server, and a playlist's
// titles listed as its children. The first two list the user's playlists,
// their watchlist always among them, and those of the local music folders
// they reach, under the names the user sees (see playlistName), narrowed
// by Filters and sorted by SortBy as Jellyfin does (by name unless asked
// otherwise). It reports whether it answered.
func (h *Handler) playlistListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, hasParent bool, start, limit int) bool {
	if hasParent && parent != playlistsViewID {
		p, err := h.Playlists.Get(r.Context(), parent)
		if errors.Is(err, playlists.ErrNotFound) || err == nil && !p.Visible(user.ID) {
			return false
		}
		var shown []playlists.Playlist
		if err == nil {
			shown, err = h.shownPlaylists(r.Context(), user, []playlists.Playlist{p})
		}
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		if len(shown) == 0 {
			notFoundProblem(w)
			return true
		}
		h.writePlaylistEntries(w, r, user, p, start, limit, true)
		return true
	}
	own, folders, asked := playlistsAsked(r, hasParent)
	if !asked {
		return false
	}
	var lists []playlists.Playlist
	var music []library.Item
	var err error
	if own {
		lists, err = h.Playlists.Visible(r.Context(), user.ID)
		if err == nil {
			lists, err = h.shownPlaylists(r.Context(), user, lists)
		}
		if err == nil {
			err = h.namePlaylists(r.Context(), user, lists)
		}
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
	}
	if folders {
		if music, err = h.Library.FolderPlaylists(r.Context(), user); err != nil {
			h.internalError(w, r, err)
			return true
		}
	}
	// A row is a user's playlist, its index in lists, or a folder's, -1
	// less its index in music.
	type row struct {
		index   int
		name    string
		created time.Time
	}
	inRange := letterRange(r)
	rows := make([]row, 0, len(lists)+len(music))
	for i, p := range lists {
		rows = append(rows, row{index: i, name: foldName(p.Name), created: p.Created})
	}
	for j, item := range music {
		rows = append(rows, row{index: -1 - j, name: foldName(sortName(item))})
	}
	rows = slices.DeleteFunc(rows, func(x row) bool { return !inRange(x.name) })
	fields := requestedFields(r)
	// Filtered, every playlist is described to tell what the user did
	// with it; else only those of the page are.
	var ownDtos []BaseItemDto
	if f, filtered := stateFilterOf(r); filtered {
		state, err := h.userState(r.Context(), user, music)
		if err == nil {
			ownDtos, err = h.playlistDtos(r, user, lists, fields, false)
		}
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		rows = slices.DeleteFunc(rows, func(x row) bool {
			if x.index >= 0 {
				return !f.keepsData(ownDtos[x.index].UserData, false)
			}
			return !f.keeps(music[-1-x.index], state)
		})
	}
	keys := slices.DeleteFunc(listQuery(r, "sortBy"), func(key string) bool { return strings.EqualFold(key, "None") })
	orders := listQuery(r, "sortOrder")
	slices.SortStableFunc(rows, func(a, b row) int {
		for i, key := range keys {
			var result int
			switch strings.ToLower(key) {
			case "sortname", "name":
				result = cmp.Compare(a.name, b.name)
			case "datecreated":
				result = a.created.Compare(b.created)
			}
			if descendingAt(orders, i) {
				result = -result
			}
			if result != 0 {
				return result
			}
		}
		// By name, a user's playlist before a folder's of the same name.
		return cmp.Or(cmp.Compare(a.name, b.name), cmp.Compare(min(b.index, 0), min(a.index, 0)))
	})
	if slices.ContainsFunc(keys, func(key string) bool { return strings.EqualFold(key, "Random") }) {
		rand.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	}
	from, to := bounds(len(rows), start, limit)
	page := rows[from:to]
	var pageLists []playlists.Playlist
	var pageMusic []library.Item
	for _, x := range page {
		switch {
		case x.index < 0:
			pageMusic = append(pageMusic, music[-1-x.index])
		case ownDtos == nil:
			pageLists = append(pageLists, lists[x.index])
		}
	}
	described, err := h.playlistDtos(r, user, pageLists, fields, false)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	state, err := h.userState(r.Context(), user, pageMusic)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	musicDtos := h.listDtos(r, user, pageMusic, fields, state)
	dtos := make([]BaseItemDto, 0, len(page))
	for _, x := range page {
		switch {
		case x.index < 0:
			dtos, musicDtos = append(dtos, musicDtos[0]), musicDtos[1:]
		case ownDtos != nil:
			dtos = append(dtos, ownDtos[x.index])
		default:
			dtos, described = append(dtos, described[0]), described[1:]
		}
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(rows), StartIndex: start})
	return true
}

// describePlaylist answers the description of the Playlists view or of a
// playlist user may see. It reports whether id named one.
func (h *Handler) describePlaylist(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	if id == playlistsViewID {
		view, err := h.playlistsView(r, user)
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		writeJSON(w, http.StatusOK, view)
		return true
	}
	p, err := h.Playlists.Get(r.Context(), id)
	if errors.Is(err, playlists.ErrNotFound) || err == nil && !p.Visible(user.ID) {
		return false
	}
	var shown []playlists.Playlist
	if err == nil {
		shown, err = h.shownPlaylists(r.Context(), user, []playlists.Playlist{p})
	}
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if len(shown) == 0 {
		return false
	}
	dtos, err := h.playlistDtos(r, user, []playlists.Playlist{p}, requestedFields(r), true)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, dtos[0])
	return true
}

// playlistAncestors answers the folders above a playlist user may see: the
// Playlists view. It reports whether id named one.
func (h *Handler) playlistAncestors(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	p, err := h.Playlists.Get(r.Context(), id)
	if errors.Is(err, playlists.ErrNotFound) || err == nil && !p.Visible(user.ID) {
		return false
	}
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	view, err := h.playlistsView(r, user)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, []BaseItemDto{view})
	return true
}

// namePlaylists gives lists the names user sees them by (see
// playlistName), and sorts them by those names.
func (h *Handler) namePlaylists(ctx context.Context, user accounts.User, lists []playlists.Playlist) error {
	for i := range lists {
		name, err := h.playlistName(ctx, user, lists[i])
		if err != nil {
			return err
		}
		lists[i].Name = name
	}
	slices.SortStableFunc(lists, func(a, b playlists.Playlist) int { return strings.Compare(foldName(a.Name), foldName(b.Name)) })
	return nil
}

// playlistDtos describes playlists from the titles user can reach in them:
// their number, runtime and genres, and how many the user played; whether
// the user made them a favorite, or liked them, is their own. Each shows a
// mosaic of its first titles user is shown, as Jellyfin draws one for its
// playlists, unless an administrator uploaded an image for it.
func (h *Handler) playlistDtos(r *http.Request, user accounts.User, lists []playlists.Playlist, fields fieldSet, detail bool) ([]BaseItemDto, error) {
	titles, err := h.playlistContents(r.Context(), user, lists...)
	if err != nil {
		return nil, err
	}
	ids := make([]accounts.ID, 0, len(titles)+len(lists))
	for id := range titles {
		ids = append(ids, id)
	}
	for _, p := range lists {
		ids = append(ids, p.ID)
	}
	data, err := h.UserData.Get(r.Context(), user.ID, ids)
	if err != nil {
		return nil, err
	}
	result := make([]BaseItemDto, 0, len(lists))
	for _, p := range lists {
		entries, err := h.shownEntries(r.Context(), user, p, titles)
		if err != nil {
			return nil, err
		}
		name, err := h.playlistName(r.Context(), user, p)
		if err != nil {
			return nil, err
		}
		var runtime time.Duration
		var genres []string
		played := 0
		for _, entry := range entries {
			title := titles[entry.Item]
			runtime += title.Runtime
			for _, genre := range title.Genres {
				if !slices.Contains(genres, genre) {
					genres = append(genres, genre)
				}
			}
			if data[entry.Item].Played {
				played++
			}
		}
		// A playlist shows administrators' edits as items do.
		item := h.Library.Overridden(library.Item{ID: p.ID, Kind: library.KindMusicPlaylist, Name: name, Genres: genres})[0]
		genres = item.Genres
		if item.Images.Primary == "" {
			// A playlist with nothing to draw, such as a new member's empty
			// Watchlist, shows the Playlists view's emblem, not a blank tile.
			image := h.Library.PlaylistImage(r.Context(), user, p.ID, item.Name, posterTitles(entries, titles))
			if image == "" {
				image = library.EmblemImage(viewEmblems[playlistsViewID]).URL()
			}
			item.Images.Primary, item.WidePrimary = image, true
		}
		dto := BaseItemDto{
			Name:              item.Name,
			ServerId:          h.ServerID,
			Id:                p.ID.String(),
			IsFolder:          true,
			Type:              "Playlist",
			ChildCount:        new(len(entries)),
			ImageTags:         map[string]string{},
			BackdropImageTags: []string{},
			ImageBlurHashes:   map[string]map[string]string{},
			LocationType:      "FileSystem",
			MediaType:         "Video",
			UserData: UserItemData{
				Key: hyphenated(p.ID), ItemId: p.ID.String(),
				PlayedPercentage: new(0.0), UnplayedItemCount: new(len(entries) - played),
				Played:     len(entries) > 0 && played == len(entries),
				IsFavorite: data[p.ID].Favorite, Rating: data[p.ID].Rating, Likes: data[p.ID].Likes(),
			},
		}
		if len(entries) > 0 {
			dto.UserData.PlayedPercentage = new(float64(played) / float64(len(entries)) * 100)
		}
		if runtime > 0 {
			dto.RunTimeTicks = new(int64(runtime / 100))
		}
		dto.OfficialRating = item.OfficialRating
		if item.CommunityRating > 0 {
			dto.CommunityRating = new(item.CommunityRating)
		}
		if item.PremiereDate != nil {
			dto.PremiereDate = new(Time(*item.PremiereDate))
		}
		if item.EndDate != nil {
			dto.EndDate = new(Time(*item.EndDate))
		}
		if item.ProductionYear > 0 {
			dto.ProductionYear = new(item.ProductionYear)
		}
		if item.Overview != "" && (detail || fields.has("Overview")) {
			dto.Overview = new(item.Overview)
		}
		h.setImages(&dto, item, nil)
		if detail || fields.has("Genres") {
			dto.Genres = new(nonNil(genres))
		}
		if detail || fields.has("ProviderIds") {
			dto.ProviderIds = &map[string]string{}
		}
		if detail || fields.has("DateCreated") {
			dto.DateCreated = new(Time(p.Created))
		}
		if detail || fields.has("CanDelete") {
			dto.CanDelete = new(p.Owner == user.ID || user.IsAdministrator)
		}
		describeEdited(&dto, item, fields, detail)
		if detail || fields.has("ParentId") {
			dto.ParentId = playlistsViewID.String()
		}
		if detail {
			dto.Etag = nameID("etag", p.ID.String())
			dto.CanDownload = new(false)
			dto.ExternalUrls = &[]MediaUrl{}
			dto.EnableMediaSourceDisplay = new(true)
			dto.PlayAccess = "Full"
			dto.RemoteTrailers = &[]MediaUrl{}
			dto.People = &[]BaseItemPerson{}
			dto.GenreItems = new(genreItems(genres))
			dto.LocalTrailerCount = new(0)
			dto.SpecialFeatureCount = new(0)
			dto.DisplayPreferencesId = p.ID.String()
			dto.LockedFields = &[]string{}
			dto.LockData = new(false)
			dto.Chapters = &[]ChapterInfo{}
			dto.RecursiveItemCount = new(len(entries))
			dto.CumulativeRunTimeTicks = new(int64(runtime / 100))
			added := time.Time{}
			if p.LastAdded != nil {
				added = *p.LastAdded
			}
			dto.DateLastMediaAdded = new(Time(added))
		}
		result = append(result, dto)
	}
	return result, nil
}

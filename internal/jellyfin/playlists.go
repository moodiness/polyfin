package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

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
// expands folders into playlists: a series or a season adds its released
// episodes in order. Other items, and those user cannot reach, are left
// out.
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
		targets, err := h.markTargets(ctx, user, item)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			titles = append(titles, target.ID)
		}
	}
	return titles, nil
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
	if request.Ids != nil {
		titles, err := h.playlistTitles(r.Context(), user, ids)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		changes.Items = &titles
	}
	h.playlistChanged(w, r, h.Playlists.Update(r.Context(), p.ID, changes))
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
	h.writePlaylistEntries(w, r, user, p, start, limit)
}

// writePlaylistEntries answers the titles of a playlist that user can
// reach, in order, each with the entry it is listed as.
func (h *Handler) writePlaylistEntries(w http.ResponseWriter, r *http.Request, user accounts.User, p playlists.Playlist, start, limit int) {
	titles, err := h.playlistContents(r.Context(), user, p)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	entries := reachable(p, titles)
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
// user can reach.
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

// addToPlaylist adds titles at position, or last.
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
	h.playlistChanged(w, r, h.Playlists.Remove(r.Context(), p.ID, queryIDs(r, "entryIds")))
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
	titles, err := h.playlistContents(r.Context(), callerFrom(r.Context()).User, p)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	others := slices.DeleteFunc(reachable(p, titles), func(e playlists.Entry) bool { return e.ID == entry })
	var before accounts.ID
	if index < len(others) {
		before = others[max(index, 0)].ID
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
// do, or a collection (see deleteCollection). Titles come from addons and
// are never deleted: like Jellyfin for a user who may not delete an item,
// Polyfin answers 401.
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
		if err := h.Playlists.Delete(r.Context(), p.ID); err != nil {
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

// playlistsViewItem is the Playlists view, among the user's libraries.
func playlistsViewItem() library.Item {
	return library.Item{ID: playlistsViewID, Kind: library.KindLibrary, Name: "Playlists", CollectionType: "playlists"}
}

// playlistsView describes the Playlists view.
func (h *Handler) playlistsView(r *http.Request, user accounts.User) (BaseItemDto, error) {
	dtos, err := h.folderDtos(r, user, []library.Item{playlistsViewItem()})
	if err != nil {
		return BaseItemDto{}, err
	}
	dtos[0].Type = "ManualPlaylistsFolder"
	return dtos[0], nil
}

// addPlaylistsView adds the Playlists view to views once user may see a
// playlist, as Jellyfin shows it once there is one.
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
// titles listed as its children. It reports whether it answered.
func (h *Handler) playlistListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, hasParent bool, start, limit int) bool {
	if hasParent && parent != playlistsViewID {
		p, err := h.Playlists.Get(r.Context(), parent)
		if errors.Is(err, playlists.ErrNotFound) || err == nil && !p.Visible(user.ID) {
			return false
		}
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		h.writePlaylistEntries(w, r, user, p, start, limit)
		return true
	}
	matches := func(list []string, value string) bool {
		return slices.ContainsFunc(list, func(t string) bool { return strings.EqualFold(t, value) })
	}
	include := listQuery(r, "includeItemTypes")
	if !hasParent && !matches(include, "Playlist") {
		return false
	}
	media := listQuery(r, "mediaTypes")
	if len(include) > 0 && !matches(include, "Playlist") || matches(listQuery(r, "excludeItemTypes"), "Playlist") ||
		len(media) > 0 && !matches(media, "Video") || matches(listQuery(r, "filters"), "IsNotFolder") {
		writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
		return true
	}
	lists, err := h.Playlists.Visible(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	inRange := letterRange(r)
	lists = slices.DeleteFunc(lists, func(p playlists.Playlist) bool { return !inRange(strings.ToLower(strings.TrimSpace(p.Name))) })
	from, to := bounds(len(lists), start, limit)
	dtos, err := h.playlistDtos(r, user, lists[from:to], requestedFields(r), false)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(lists), StartIndex: start})
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
	if err != nil {
		h.internalError(w, r, err)
		return true
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

// playlistDtos describes playlists from the titles user can reach in them:
// their number, runtime and genres, and how many the user played.
func (h *Handler) playlistDtos(r *http.Request, user accounts.User, lists []playlists.Playlist, fields fieldSet, detail bool) ([]BaseItemDto, error) {
	titles, err := h.playlistContents(r.Context(), user, lists...)
	if err != nil {
		return nil, err
	}
	ids := make([]accounts.ID, 0, len(titles))
	for id := range titles {
		ids = append(ids, id)
	}
	data, err := h.UserData.Get(r.Context(), user.ID, ids)
	if err != nil {
		return nil, err
	}
	result := make([]BaseItemDto, 0, len(lists))
	for _, p := range lists {
		entries := reachable(p, titles)
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
		item := h.Library.Overridden(library.Item{ID: p.ID, Kind: library.KindMusicPlaylist, Name: p.Name, Genres: genres})[0]
		genres = item.Genres
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
				Played: len(entries) > 0 && played == len(entries),
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
		h.setImages(&dto, item)
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

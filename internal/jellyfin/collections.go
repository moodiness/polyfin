package jellyfin

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Collections.
//
// Users allowed to manage collections (Jellyfin's EnableCollectionManagement)
// group titles into collections, Jellyfin's BoxSets, which every user sees
// in the Collections view. Each user sees, and counts, only the titles of a
// collection they may see: parental control and blocked genres hide the
// others, as they do everywhere.

func (h *Handler) collectionRoutes(rt *router) {
	managing := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(collectionManagement(handler)))
	}
	managing(http.MethodPost, "/Collections", h.createCollection)
	managing(http.MethodPost, "/Collections/{collectionId}/Items", h.addToCollection)
	managing(http.MethodDelete, "/Collections/{collectionId}/Items", h.removeFromCollection)
}

// collectionManagement refuses the collection routes to users who may not
// manage collections with an empty 403, as Jellyfin's CollectionManagement
// policy does, administrators included.
func collectionManagement(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !callerFrom(r.Context()).User.CollectionManagement {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// CollectionCreationResult identifies a new collection.
type CollectionCreationResult struct {
	Id string
}

// collectionsViewID identifies the Collections view, the same on every
// server.
var collectionsViewID, _ = accounts.ParseID(nameID("view", "collections"))

// ownView reports whether id is one of the views Polyfin answers itself
// that hold no title of a library: the Live TV view, the Playlists view
// and the recordings folder. No item has their identifier, and looking
// one up would search the guide's programmes before failing (see
// library.Service.Item), so listings that ask what they hold name them
// first.
func ownView(id accounts.ID) bool {
	return id == liveTvViewID || id == playlistsViewID || id == recordingsFolderID
}

// CollectionKinds are the items a collection holds. Like Jellyfin's, a
// collection holds titles, home videos among them, their seasons and
// episodes, folders of videos, other groups and channels; Polyfin leaves
// out people and programmes, which its guide replaces. The collections
// users make may hold each other too (see collectionItems). The admin
// app's collection pages add the same.
var CollectionKinds = []library.Kind{library.KindMovie, library.KindVideo, library.KindSeries, library.KindSeason, library.KindEpisode,
	library.KindFolder, library.KindCollection, library.KindChannel}

// CollectionSearch finds the items whose name holds term that the admin
// app's collection pages offer to add, as jellyfin-web's search finds
// them for user (see the searchTerm listing): the movies, series, videos
// and folders of videos of their libraries, whether an addon's, an IPTV
// source's or a local folder's (see library.Service.Search), then their
// Live TV channels, case and accents aside; at most limit of each. The
// rating filter of ctx applies, as to a listing.
func (h *Handler) CollectionSearch(ctx context.Context, user accounts.User, term string, limit int) ([]library.Item, error) {
	found, err := h.Library.Search(ctx, user, term, searchKinds(func(library.Item) bool { return true }), limit)
	if err != nil {
		return nil, err
	}
	has, err := h.Library.HasChannels(ctx, user)
	if err != nil {
		return nil, err
	}
	if has {
		folded := iptv.Fold(term)
		channels, err := h.Library.SearchChannels(ctx, user, func(name string) bool { return strings.Contains(iptv.Fold(name), folded) }, limit)
		if err != nil {
			return nil, err
		}
		found = append(found, channels...)
	}
	return h.Library.RatingFiltered(ctx, user, found)
}

// collectionItems checks the items ids names for a collection: each must be
// an item user can reach that a collection may hold, or a collection made
// by users that user is shown. Jellyfin answers an item it does not know
// with a 400 and adds nothing, which Polyfin also does for one the user
// may not see. ok is false once w was answered.
func (h *Handler) collectionItems(w http.ResponseWriter, r *http.Request, user accounts.User, ids []accounts.ID) ([]accounts.ID, bool) {
	if len(ids) == 0 {
		return nil, true
	}
	nested, err := h.Collections.Some(r.Context(), ids)
	if err == nil {
		nested, err = h.shownCollections(r.Context(), user, nested)
	}
	if err != nil {
		h.internalError(w, r, err)
		return nil, false
	}
	found := make(map[accounts.ID]bool, len(ids))
	for _, c := range nested {
		found[c.ID] = true
	}
	titles := slices.DeleteFunc(slices.Clone(ids), func(id accounts.ID) bool { return found[id] })
	if len(titles) > 0 {
		items, err := h.Library.Items(r.Context(), user, titles)
		if err != nil {
			h.internalError(w, r, err)
			return nil, false
		}
		for _, item := range items {
			found[item.ID] = slices.Contains(CollectionKinds, item.Kind)
		}
	}
	for _, id := range ids {
		if !found[id] {
			processingError(w, http.StatusBadRequest)
			return nil, false
		}
	}
	return ids, true
}

// createCollection creates a collection, with titles, collections or
// empty. Like Jellyfin, it puts it among the server's collections whatever
// parentId names. Unlike Jellyfin, which then fails with a 500, Polyfin
// refuses a collection without a name, an identifier that is not one, or
// collections nested too deep, with a 400, and creates nothing.
func (h *Handler) createCollection(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	b.guid(r, "parentId")
	isLocked, _ := b.bool(r, "isLocked")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	name := query(r, "name")
	raw := listQuery(r, "ids")
	ids := queryIDs(r, "ids")
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > collections.MaxNameLength || len(ids) != len(raw) {
		processingError(w, http.StatusBadRequest)
		return
	}
	ids, ok := h.collectionItems(w, r, callerFrom(r.Context()).User, ids)
	if !ok {
		return
	}
	id, err := h.Collections.Create(r.Context(), name, isLocked, ids)
	if errors.Is(err, collections.ErrTooDeep) {
		processingError(w, http.StatusBadRequest)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CollectionCreationResult{Id: id.String()})
}

// changeCollection binds the collection of a change and the items it names,
// which, as in Jellyfin, leave out the values that are not identifiers.
// Jellyfin answers a collection it does not know with a 400.
func (h *Handler) changeCollection(w http.ResponseWriter, r *http.Request) (accounts.ID, []accounts.ID, bool) {
	b := bindErrors{}
	id := b.pathID(r, "collectionId")
	if len(b) > 0 {
		validationProblem(w, b)
		return accounts.ID{}, nil, false
	}
	if _, err := h.Collections.Get(r.Context(), id); err != nil {
		h.collectionChanged(w, r, err)
		return accounts.ID{}, nil, false
	}
	return id, queryIDs(r, "ids"), true
}

// collectionChanged answers a change to a collection. A collection put
// within itself, at any depth, or collections nested deeper than
// collections.MaxDepth, are refused with a 400, as Jellyfin refuses an
// item it cannot add.
func (h *Handler) collectionChanged(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, collections.ErrNotFound), errors.Is(err, collections.ErrCycle), errors.Is(err, collections.ErrTooDeep):
		processingError(w, http.StatusBadRequest)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// addToCollection adds titles and collections after those the collection
// has; those it already has keep their place. A smart list's titles are
// its rules': adding to one is refused with an empty 403.
func (h *Handler) addToCollection(w http.ResponseWriter, r *http.Request) {
	if h.smartCollection(w, r) {
		return
	}
	id, ids, ok := h.changeCollection(w, r)
	if !ok {
		return
	}
	if ids, ok = h.collectionItems(w, r, callerFrom(r.Context()).User, ids); !ok {
		return
	}
	h.collectionChanged(w, r, h.Collections.Add(r.Context(), id, ids))
}

// smartCollection answers a change to a smart list the caller sees with an
// empty 403, as Jellyfin answers a change it forbids, and reports whether
// it did.
func (h *Handler) smartCollection(w http.ResponseWriter, r *http.Request) bool {
	id, ok := parseGUID(r.PathValue("collectionId"))
	if !ok {
		return false
	}
	smart, err := h.Library.IsSmartList(r.Context(), callerFrom(r.Context()).User, id)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if smart {
		w.WriteHeader(http.StatusForbidden)
	}
	return smart
}

// removeFromCollection takes titles and collections out of a collection;
// those it does not have are ignored. A smart list's are refused, as
// adding is.
func (h *Handler) removeFromCollection(w http.ResponseWriter, r *http.Request) {
	if h.smartCollection(w, r) {
		return
	}
	id, ids, ok := h.changeCollection(w, r)
	if !ok {
		return
	}
	h.collectionChanged(w, r, h.Collections.Remove(r.Context(), id, ids))
}

// deleteCollection deletes the collection id, which also takes it out of
// the collections that held it. As in Jellyfin's DELETE /Items/{itemId},
// administrators and the users who may manage collections may; anyone
// else is answered 401. It reports whether id named a collection.
func (h *Handler) deleteCollection(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	_, err := h.Collections.Get(r.Context(), id)
	switch {
	case errors.Is(err, collections.ErrNotFound):
		return false
	case err != nil:
		h.internalError(w, r, err)
	case !mayDeleteCollections(user):
		writeJSON(w, http.StatusUnauthorized, "Unauthorized access")
	default:
		err = h.Collections.Delete(r.Context(), id)
		if errors.Is(err, collections.ErrNotFound) {
			// Another request deleted it meanwhile.
			notFoundProblem(w)
		} else if err != nil {
			h.internalError(w, r, err)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}
	return true
}

// mayDeleteCollections reports whether user may delete collections, as
// Jellyfin's BoxSet lets administrators and the users who may manage
// collections.
func mayDeleteCollections(user accounts.User) bool {
	return user.IsAdministrator || user.CollectionManagement
}

// Browsing collections.

// collectionsViewItem is the Collections view, among the user's libraries,
// named in the server's language, with its emblem.
func (h *Handler) collectionsViewItem() library.Item {
	return withEmblem(library.Item{ID: collectionsViewID, Kind: library.KindLibrary, Name: library.CollectionsName(h.Accounts.Settings().Language),
		CollectionType: "boxsets"})
}

// addCollectionsView adds the Collections view to views once a collection
// exists, as Jellyfin shows it once there is one: one made by users, or a
// smart list the user sees as a collection, whether or not it holds a
// title they may see, which would cost a read of each list.
func (h *Handler) addCollectionsView(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	exists, err := h.Collections.Any(r.Context())
	if err != nil {
		return nil, err
	}
	if !exists {
		smart, err := h.Library.SmartCollections(r.Context(), user)
		if err != nil || len(smart) == 0 {
			return views, err
		}
	}
	view, err := h.folderDtos(r, user, []library.Item{h.collectionsViewItem()})
	if err != nil {
		return nil, err
	}
	return append(views, view...), nil
}

// describeCollection answers the description of the Collections view or of
// a collection. It reports whether id named one.
func (h *Handler) describeCollection(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	if id == collectionsViewID {
		views, err := h.folderDtos(r, user, []library.Item{h.collectionsViewItem()})
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		writeJSON(w, http.StatusOK, views[0])
		return true
	}
	c, err := h.Collections.Get(r.Context(), id)
	if errors.Is(err, collections.ErrNotFound) {
		return false
	}
	var shown []collections.Collection
	if err == nil {
		shown, err = h.shownCollections(r.Context(), user, []collections.Collection{c})
	}
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if len(shown) == 0 {
		return false
	}
	dtos, err := h.collectionDtos(r, user, []collections.Collection{c}, requestedFields(r), true)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, dtos[0])
	return true
}

// collectionAncestors answers the folders above a collection: the
// Collections view. It reports whether id named a collection.
func (h *Handler) collectionAncestors(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	_, err := h.Collections.Get(r.Context(), id)
	if errors.Is(err, collections.ErrNotFound) {
		return false
	}
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	views, err := h.folderDtos(r, user, []library.Item{h.collectionsViewItem()})
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, views)
	return true
}

// collectionListing answers the listings that hold collections: the
// Collections view's, a listing of collections across the server or of the
// collections with titles from a library, and a collection's titles and
// collections listed as its children. Like Jellyfin, a listing asking only
// for BoxSet lists collections whatever folder it names, keeping, for a
// library that does not hold collections, those with a title listed in
// it. A listing across the server lists the addons' collections too (see
// writeCollectionsAcrossServer). It reports whether it answered.
func (h *Handler) collectionListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, hasParent bool, start, limit int) bool {
	if hasParent && parent != collectionsViewID {
		c, err := h.Collections.Get(r.Context(), parent)
		var shown []collections.Collection
		if err == nil {
			shown, err = h.shownCollections(r.Context(), user, []collections.Collection{c})
		}
		if err == nil {
			if len(shown) == 0 {
				notFoundProblem(w)
				return true
			}
			if wholeListing(r) {
				limit = library.WholeListing
			}
			h.writeCollectionTitles(w, r, user, c, start, limit)
			return true
		}
		if !errors.Is(err, collections.ErrNotFound) {
			h.internalError(w, r, err)
			return true
		}
	}
	include := listQuery(r, "includeItemTypes")
	onlyCollections := len(include) == 1 && strings.EqualFold(include[0], "BoxSet")
	var within accounts.ID
	if hasParent && parent != collectionsViewID {
		if !onlyCollections {
			return false
		}
		// Polyfin's own views are no library: they list as they do.
		if ownView(parent) {
			return false
		}
		folder, err := h.Library.Item(r.Context(), user, parent)
		if err != nil || folder.Kind != library.KindLibrary || folder.CollectionType == "boxsets" {
			// An addon's groups, or a folder of titles, list as they do.
			return false
		}
		within = parent
	} else if !hasParent && !onlyCollections {
		return false
	}
	if !itemTypeFilter(r)(library.Item{Kind: library.KindCollection}) {
		writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
		return true
	}
	all, err := h.Collections.All(r.Context())
	if err == nil && within != (accounts.ID{}) {
		all, err = h.listedUnder(r.Context(), all, within)
	}
	if err == nil {
		all, err = h.shownCollections(r.Context(), user, all)
	}
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if !hasParent && !webApp(r) {
		h.writeCollectionsAcrossServer(w, r, user, all, start, limit)
		return true
	}
	fields := requestedFields(r)
	filter, filtered := stateFilterOf(r)
	// The Collections view lists the smart lists the user is shown as
	// collections beside those made by users, by name.
	var smart []library.Item
	if within == (accounts.ID{}) && hasParent {
		if smart, err = h.Library.ShownSmartCollections(r.Context(), user); err != nil {
			h.internalError(w, r, err)
			return true
		}
	}
	if !filtered && len(smart) == 0 {
		from, to := bounds(len(all), start, limit)
		dtos, err := h.collectionDtos(r, user, all[from:to], fields, false)
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(all), StartIndex: start})
		return true
	}
	dtos, err := h.collectionDtos(r, user, all, fields, false)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if len(smart) > 0 {
		listed, err := h.dtos(r, user, smart, fields, nil)
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		dtos = append(dtos, listed...)
		slices.SortStableFunc(dtos, func(a, b BaseItemDto) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	}
	if filtered {
		dtos = slices.DeleteFunc(dtos, func(dto BaseItemDto) bool { return !filter.keepsData(dto.UserData, false) })
	}
	from, to := bounds(len(dtos), start, limit)
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos[from:to], TotalRecordCount: len(dtos), StartIndex: start})
	return true
}

// listedUnder keeps the collections with a title of their own listed in
// folder: one holding only collections is listed in no library.
func (h *Handler) listedUnder(ctx context.Context, all []collections.Collection, folder accounts.ID) ([]collections.Collection, error) {
	var kept []collections.Collection
	for _, c := range all {
		listed, err := h.Library.ListedUnder(ctx, c.Items, folder)
		if err != nil {
			return nil, err
		}
		if listed {
			kept = append(kept, c)
		}
	}
	return kept, nil
}

// writeCollectionsAcrossServer answers a listing of collections across the
// server as Jellyfin answers one, with every BoxSet the user sees: the
// collections made by users, those the user's collection libraries list,
// each once, and the smart lists the user is shown as collections, by name.
// Strand builds its shelves from such a listing. jellyfin-web makes one
// only for its Add to collection dialog, where a title can go to
// collections made by users alone: collectionListing answers it with
// those.
func (h *Handler) writeCollectionsAcrossServer(w http.ResponseWriter, r *http.Request, user accounts.User, made []collections.Collection, start, limit int) {
	fromAddons, err := h.addonCollections(r.Context(), user)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	smart, err := h.Library.ShownSmartCollections(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	fields := requestedFields(r)
	dtos, err := h.collectionDtos(r, user, made, fields, false)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	listed, err := h.dtos(r, user, append(fromAddons, smart...), fields, nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	dtos = append(dtos, listed...)
	if filter, filtered := stateFilterOf(r); filtered {
		dtos = slices.DeleteFunc(dtos, func(dto BaseItemDto) bool { return !filter.keepsData(dto.UserData, false) })
	}
	orders := listQuery(r, "sortOrder")
	descending := len(orders) > 0 && strings.EqualFold(orders[0], "Descending")
	slices.SortStableFunc(dtos, func(a, b BaseItemDto) int {
		order := cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		if descending {
			return -order
		}
		return order
	})
	if wholeListing(r) {
		limit = library.WholeListing
	}
	from, to := bounds(len(dtos), start, limit)
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos[from:to], TotalRecordCount: len(dtos), StartIndex: start})
}

// addonCollections lists the collections of the user's collection
// libraries, as each lists them, each once.
func (h *Handler) addonCollections(ctx context.Context, user accounts.User) ([]library.Item, error) {
	libraries, err := h.Library.Libraries(ctx, user)
	if err != nil {
		return nil, err
	}
	var listed []library.Item
	seen := map[accounts.ID]bool{}
	for _, l := range libraries {
		if l.CollectionType != "boxsets" {
			continue
		}
		page, err := h.Library.Children(ctx, user, l.ID, 0, library.WholeListing, "")
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if item.Kind == library.KindCollection && !seen[item.ID] {
				seen[item.ID] = true
				listed = append(listed, item)
			}
		}
	}
	return listed, nil
}

// writeCollectionTitles answers the titles of a collection that user may
// see, and the collections in it they are shown, as BoxSets, in Jellyfin's
// order for collections: by premiere date, a collection's being its
// earliest title's, then in the collection's order. A collection whose
// display order an administrator set to the chosen one lists in its own
// order alone. Listed recursively, as apps play a collection, a
// collection in it also brings its own, after it, and a series or season
// its seasons and released episodes. The request's type filters apply.
func (h *Handler) writeCollectionTitles(w http.ResponseWriter, r *http.Request, user accounts.User, c collections.Collection, start, limit int) {
	view, err := h.viewCollections(r.Context(), user, c)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	listed := view.entries(c)
	if recursive, _ := boolQuery(r, "recursive"); recursive {
		if listed, err = h.withDescendants(r.Context(), user, view.withNested(listed)); err != nil {
			h.browseError(w, r, err)
			return
		}
	}
	keep := itemTypeFilter(r)
	listed = slices.DeleteFunc(listed, func(item library.Item) bool { return !keep(item) })
	if filter, filtered := stateFilterOf(r); filtered {
		state, err := h.userState(r.Context(), user, listed)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		listed = slices.DeleteFunc(listed, func(item library.Item) bool { return !filter.keeps(item, state) })
	}
	if c.DisplayOrder != collections.OrderChosen {
		slices.SortStableFunc(listed, func(a, b library.Item) int { return premiere(a).Compare(premiere(b)) })
	}
	from, to := bounds(len(listed), start, limit)
	dtos, err := h.entryDtos(r, user, view, listed[from:to])
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(listed), StartIndex: start})
}

// entryDtos describes the entries of a collection of view: its titles as
// any, and the collections in it as collections are (see collectionDtos).
func (h *Handler) entryDtos(r *http.Request, user accounts.User, view *collectionView, entries []library.Item) ([]BaseItemDto, error) {
	fields := requestedFields(r)
	var titles []library.Item
	var nested []collections.Collection
	for _, entry := range entries {
		if c, ok := view.nested(entry); ok {
			nested = append(nested, c)
		} else {
			titles = append(titles, entry)
		}
	}
	titleDtos, err := h.dtos(r, user, titles, fields, nil)
	if err != nil {
		return nil, err
	}
	var nestedDtos []BaseItemDto
	if len(nested) > 0 {
		if nestedDtos, err = h.collectionDtos(r, user, nested, fields, false); err != nil {
			return nil, err
		}
	}
	result := make([]BaseItemDto, 0, len(entries))
	for _, entry := range entries {
		if _, ok := view.nested(entry); ok {
			result, nestedDtos = append(result, nestedDtos[0]), nestedDtos[1:]
		} else {
			result, titleDtos = append(result, titleDtos[0]), titleDtos[1:]
		}
	}
	return result, nil
}

// withDescendants adds, after each series and season of titles, its
// seasons and released episodes; a title named twice is listed once.
func (h *Handler) withDescendants(ctx context.Context, user accounts.User, titles []library.Item) ([]library.Item, error) {
	seen := map[accounts.ID]bool{}
	var result []library.Item
	add := func(items ...library.Item) {
		for _, item := range items {
			if !seen[item.ID] {
				seen[item.ID] = true
				result = append(result, item)
			}
		}
	}
	for _, title := range titles {
		add(title)
		if title.Kind == library.KindSeries {
			seasons, err := h.Library.Seasons(ctx, user, title.ID)
			if err != nil {
				return nil, err
			}
			add(seasons...)
		}
		episodes, err := h.markTargets(ctx, user, title)
		if err != nil {
			return nil, err
		}
		add(episodes...)
	}
	return result, nil
}

// premiere is when a title came out, as Jellyfin sorts a collection: its
// premiere date, else the start of its year; the earliest for neither.
func premiere(item library.Item) time.Time {
	switch {
	case item.PremiereDate != nil:
		return *item.PremiereDate
	case item.ProductionYear > 0:
		return time.Date(item.ProductionYear, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Time{}
}

// collectionTree is collections by identifier, with the collections nested
// in them at any depth.
type collectionTree map[accounts.ID]collections.Collection

// collectionTree returns lists with the collections nested in them, at
// any depth.
func (h *Handler) collectionTree(ctx context.Context, lists []collections.Collection) (collectionTree, error) {
	tree := make(collectionTree, len(lists))
	for _, c := range lists {
		tree[c.ID] = c
	}
	next := lists
	// Collections nest collections.MaxDepth levels deep at most.
	for range collections.MaxDepth {
		var ids []accounts.ID
		for _, c := range next {
			for _, id := range c.Collections {
				if _, ok := tree[id]; !ok && !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
		}
		if len(ids) == 0 {
			break
		}
		found, err := h.Collections.Some(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, c := range found {
			tree[c.ID] = c
		}
		next = found
	}
	return tree, nil
}

// titles lists the titles of the collections of the tree, each once.
func (t collectionTree) titles() []accounts.ID {
	var ids []accounts.ID
	seen := map[accounts.ID]bool{}
	for _, c := range t {
		for _, id := range c.Items {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// collectionView is collections as a user sees them: those nested in
// them, the titles of each the user may see, and whether the user is
// shown each.
type collectionView struct {
	tree   collectionTree
	titles map[accounts.ID]library.Item
	shown  map[accounts.ID]bool
	// country scores ratings (see collectionItem).
	country string
	// data and counts are what the user did with the titles, and the
	// episodes they played of series and seasons; describing a collection
	// adds to counts the movies and episodes it counts played.
	data   map[accounts.ID]userdata.Data
	counts map[accounts.ID]userdata.Counts
	// described holds the collections described, the zero item while one
	// is.
	described map[accounts.ID]library.Item
}

// viewCollections returns lists as user sees them under ctx: the titles
// user hid are left out, and those the request's rating filter does not
// keep, and the collections nested in them that user is not shown (see
// shownInTree).
func (h *Handler) viewCollections(ctx context.Context, user accounts.User, lists ...collections.Collection) (*collectionView, error) {
	tree, err := h.collectionTree(ctx, lists)
	if err != nil {
		return nil, err
	}
	shown, err := h.shownInTree(ctx, user, tree)
	if err != nil {
		return nil, err
	}
	view := &collectionView{tree: tree, titles: map[accounts.ID]library.Item{}, shown: shown, country: h.Accounts.RatingCountry(),
		counts: map[accounts.ID]userdata.Counts{}, described: map[accounts.ID]library.Item{}}
	if ids := tree.titles(); len(ids) > 0 {
		items, err := h.Library.Items(ctx, user, ids)
		if err == nil {
			items, err = h.Library.RatingFiltered(ctx, user, items)
		}
		if err != nil {
			return nil, err
		}
		hidden, err := h.Library.HiddenTitles(ctx, user)
		if err != nil {
			return nil, err
		}
		for _, item := range hidden.Without(items) {
			view.titles[item.ID] = item
		}
	}
	return view, nil
}

// entries lists the entries of c the user may see, in the order they were
// added: its titles, and the collections in it the user is shown,
// described (see describe).
func (v *collectionView) entries(c collections.Collection) []library.Item {
	var result []library.Item
	for _, id := range c.Entries {
		if item, ok := v.titles[id]; ok {
			result = append(result, item)
		} else if inner, ok := v.tree[id]; ok && v.shown[id] {
			if item, ok := v.describe(inner); ok {
				result = append(result, item)
			}
		}
	}
	return result
}

// nested returns the collection of the view entry is, when it is one.
func (v *collectionView) nested(entry library.Item) (collections.Collection, bool) {
	if entry.Kind != library.KindCollection {
		return collections.Collection{}, false
	}
	c, ok := v.tree[entry.ID]
	return c, ok
}

// describe describes c from its entries the user may see (see
// collectionItem): a collection in it counts the videos it counts, and
// those played. ok is false while c is being described, within itself,
// which nesting refuses.
func (v *collectionView) describe(c collections.Collection) (item library.Item, ok bool) {
	if item, ok := v.described[c.ID]; ok {
		return item, item.ID == c.ID
	}
	v.described[c.ID] = library.Item{}
	entries := v.entries(c)
	item, played := collectionItem(c, entries, v.data, v.counts, v.country)
	for _, entry := range entries {
		if _, ok := v.nested(entry); ok {
			item.Contents.Released += entry.Contents.Released
			played += v.counts[entry.ID].Played
		}
	}
	v.counts[c.ID] = userdata.Counts{Played: played}
	v.described[c.ID] = item
	return item, true
}

// withNested adds, after each collection of the view among items, the
// entries in it the user may see, and theirs; an entry named twice is
// listed once.
func (v *collectionView) withNested(items []library.Item) []library.Item {
	seen := map[accounts.ID]bool{}
	var result []library.Item
	var add func(items []library.Item)
	add = func(items []library.Item) {
		for _, item := range items {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			result = append(result, item)
			if c, ok := v.nested(item); ok {
				add(v.entries(c))
			}
		}
	}
	add(items)
	return result
}

// collectionDtos describes collections from the titles user may see in
// them, and the collections in them they are shown, as Jellyfin derives a
// collection's genres, rating and date from its entries: their number,
// and the movies and episodes under them the user played.
func (h *Handler) collectionDtos(r *http.Request, user accounts.User, lists []collections.Collection, fields fieldSet, detail bool) ([]BaseItemDto, error) {
	view, err := h.viewCollections(r.Context(), user, lists...)
	if err != nil {
		return nil, err
	}
	var ids, parents []accounts.ID
	for _, c := range lists {
		ids = append(ids, c.ID)
	}
	for _, title := range view.titles {
		ids = append(ids, title.ID)
		if title.Kind == library.KindSeries || title.Kind == library.KindSeason {
			parents = append(parents, title.ID)
		}
		if title.Kind == library.KindFolder && title.Contents != nil {
			ids = append(ids, title.Contents.Videos...)
		}
	}
	if view.data, err = h.UserData.Get(r.Context(), user.ID, ids); err != nil {
		return nil, err
	}
	counts, err := h.UserData.EpisodeCounts(r.Context(), user.ID, parents)
	if err != nil {
		return nil, err
	}
	maps.Copy(view.counts, counts)
	state := userState{reader: user.ID, data: view.data, counts: map[accounts.ID]userdata.Counts{}}
	result := make([]BaseItemDto, 0, len(lists))
	for _, c := range lists {
		item, _ := view.describe(c)
		state.counts[c.ID] = view.counts[c.ID]
		dto := h.newItemDto(item, fields, detail, state)
		if !detail && fields.has("ChildCount") {
			dto.ChildCount = new(item.Contents.Children)
		}
		if !detail && fields.has("RecursiveItemCount") {
			dto.RecursiveItemCount = new(item.Contents.Released)
		}
		if detail || fields.has("DateCreated") {
			dto.DateCreated = new(Time(c.Created))
		}
		if detail || fields.has("CanDelete") {
			dto.CanDelete = new(mayDeleteCollections(user))
		}
		result = append(result, dto)
	}
	return result, nil
}

// collectionItem describes a collection from the entries in it a user may
// see, and counts the movies and episodes under its titles the user
// played. Like Jellyfin, a collection takes its entries' genres, their
// least restrictive rating, scored under country, the server's rating
// country, and the earliest premiere date or year; its poster is that of
// its first entry with one, a collection in it showing its own (see
// collectionArtwork).
func collectionItem(c collections.Collection, titles []library.Item, data map[accounts.ID]userdata.Data, counts map[accounts.ID]userdata.Counts,
	country string) (library.Item, int) {
	item := library.Item{ID: c.ID, Kind: library.KindCollection, Name: c.Name, ParentID: collectionsViewID,
		Contents: &library.Contents{Children: len(titles), LastReleased: c.LastAdded}}
	played := 0
	var ratings []string
	for _, title := range titles {
		switch title.Kind {
		case library.KindMovie, library.KindVideo, library.KindEpisode:
			item.Contents.Released++
			if data[title.ID].Played {
				played++
			}
		case library.KindSeries, library.KindSeason:
			if title.Contents != nil {
				item.Contents.Released += title.Contents.Released
				played += min(counts[title.ID].Played, title.Contents.Released)
			}
		case library.KindFolder:
			if title.Contents != nil {
				item.Contents.Released += title.Contents.Released
				for _, video := range title.Contents.Videos {
					if data[video].Played {
						played++
					}
				}
			}
		}
		for _, genre := range title.Genres {
			if !slices.ContainsFunc(item.Genres, func(g string) bool { return strings.EqualFold(g, genre) }) {
				item.Genres = append(item.Genres, genre)
			}
		}
		if title.OfficialRating != "" {
			ratings = append(ratings, title.OfficialRating)
		}
		if title.PremiereDate != nil && (item.PremiereDate == nil || title.PremiereDate.Before(*item.PremiereDate)) {
			item.PremiereDate = title.PremiereDate
		}
		if title.ProductionYear > 0 && (item.ProductionYear == 0 || title.ProductionYear < item.ProductionYear) {
			item.ProductionYear = title.ProductionYear
		}
		if item.Images.Primary == "" {
			item.Images.Primary = library.CollectionPoster(title)
		}
	}
	if item.PremiereDate != nil {
		item.ProductionYear = item.PremiereDate.Year()
	}
	if len(ratings) > 0 {
		item.OfficialRating = slices.MinFunc(ratings, func(a, b string) int { return compareRatings(a, b, country) })
	}
	return item, played
}

// compareRatings orders ratings from the least restrictive, scored under
// country, those Polyfin does not know last, as Jellyfin picks a
// collection's rating.
func compareRatings(a, b, country string) int {
	rank := func(rating string) (int, int) {
		score, ok := localization.RatingScore(rating, country)
		if !ok {
			return 1001, 1001
		}
		if score.SubScore == nil {
			return score.Score, 0
		}
		return score.Score, *score.SubScore
	}
	scoreA, subA := rank(a)
	scoreB, subB := rank(b)
	return cmp.Or(cmp.Compare(scoreA, scoreB), cmp.Compare(subA, subB))
}

// collectionArtwork returns the Primary image of the collection id: the
// poster of the title among its own, or those of the collections in it,
// that tag names, as a user's listing tagged it from the first title they
// see, else its first title's poster, a collection in it before it giving
// its own first. ok is false when id names no collection.
func (h *Handler) collectionArtwork(r *http.Request, id accounts.ID, imageType string) (found library.Artwork, ok bool, err error) {
	c, err := h.Collections.Get(r.Context(), id)
	if errors.Is(err, collections.ErrNotFound) {
		return library.Artwork{}, false, nil
	}
	if err != nil {
		return library.Artwork{}, true, err
	}
	if !strings.EqualFold(imageType, "Primary") {
		return library.Artwork{}, true, library.ErrNotFound
	}
	tree, err := h.collectionTree(r.Context(), []collections.Collection{c})
	if err != nil {
		return library.Artwork{}, true, err
	}
	tag := query(r, "tag")
	var first library.Artwork
	searched := map[accounts.ID]bool{}
	// search reports whether it found the poster tag names among those of
	// c, in the order of its entries.
	var search func(c collections.Collection) (bool, error)
	search = func(c collections.Collection) (bool, error) {
		searched[c.ID] = true
		for _, entry := range c.Entries {
			if inner, ok := tree[entry]; ok {
				if searched[entry] {
					continue
				}
				if done, err := search(inner); done || err != nil {
					return done, err
				}
				continue
			}
			poster, err := h.Library.Poster(r.Context(), entry)
			if errors.Is(err, library.ErrNotFound) {
				continue
			}
			if err != nil {
				return false, err
			}
			if tag == "" || library.ImageTag(poster.URL) == tag {
				found = poster
				return true, nil
			}
			if first.URL == "" {
				first = poster
			}
		}
		return false, nil
	}
	if done, err := search(c); done || err != nil {
		return found, true, err
	}
	if first.URL == "" {
		return library.Artwork{}, true, library.ErrNotFound
	}
	return first, true, nil
}

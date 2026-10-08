package jellyfin

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
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

// collectionKinds are the items a collection holds. Like Jellyfin's, a
// collection holds titles, their seasons and episodes, other groups and
// channels; Polyfin leaves out people and programmes, which its guide
// replaces.
var collectionKinds = []library.Kind{library.KindMovie, library.KindSeries, library.KindSeason, library.KindEpisode,
	library.KindCollection, library.KindChannel}

// collectionItems checks the items ids names for a collection: each must be
// an item user can reach that a collection may hold. Jellyfin answers an
// item it does not know with a 400 and adds nothing, which Polyfin also
// does for one the user may not see. ok is false once w was answered.
func (h *Handler) collectionItems(w http.ResponseWriter, r *http.Request, user accounts.User, ids []accounts.ID) ([]accounts.ID, bool) {
	if len(ids) == 0 {
		return nil, true
	}
	items, err := h.Library.Items(r.Context(), user, ids)
	if err != nil {
		h.internalError(w, r, err)
		return nil, false
	}
	found := make(map[accounts.ID]bool, len(items))
	for _, item := range items {
		found[item.ID] = slices.Contains(collectionKinds, item.Kind)
	}
	for _, id := range ids {
		if !found[id] {
			processingError(w, http.StatusBadRequest)
			return nil, false
		}
	}
	return ids, true
}

// createCollection creates a collection, with titles or empty. Like
// Jellyfin, it puts it among the server's collections whatever parentId
// names. Unlike Jellyfin, which then fails with a 500, Polyfin refuses a
// collection without a name, or an identifier that is not one, with a 400,
// and creates nothing.
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

// collectionChanged answers a change to a collection.
func (h *Handler) collectionChanged(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, collections.ErrNotFound):
		processingError(w, http.StatusBadRequest)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// addToCollection adds titles after those the collection has; those it
// already has keep their place.
func (h *Handler) addToCollection(w http.ResponseWriter, r *http.Request) {
	id, ids, ok := h.changeCollection(w, r)
	if !ok {
		return
	}
	if ids, ok = h.collectionItems(w, r, callerFrom(r.Context()).User, ids); !ok {
		return
	}
	h.collectionChanged(w, r, h.Collections.Add(r.Context(), id, ids))
}

// removeFromCollection takes titles out of a collection; those it does not
// have are ignored.
func (h *Handler) removeFromCollection(w http.ResponseWriter, r *http.Request) {
	id, ids, ok := h.changeCollection(w, r)
	if !ok {
		return
	}
	h.collectionChanged(w, r, h.Collections.Remove(r.Context(), id, ids))
}

// deleteCollection deletes the collection id, which, as in Jellyfin's
// DELETE /Items/{itemId}, administrators and the users who may manage
// collections may do; anyone else is answered 401. It reports whether id
// named a collection.
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

// collectionsViewItem is the Collections view, among the user's libraries.
func collectionsViewItem() library.Item {
	return library.Item{ID: collectionsViewID, Kind: library.KindLibrary, Name: "Collections", CollectionType: "boxsets"}
}

// addCollectionsView adds the Collections view to views once a collection
// exists, as Jellyfin shows it once there is one.
func (h *Handler) addCollectionsView(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	exists, err := h.Collections.Any(r.Context())
	if err != nil || !exists {
		return views, err
	}
	view, err := h.folderDtos(r, user, []library.Item{collectionsViewItem()})
	if err != nil {
		return nil, err
	}
	return append(views, view...), nil
}

// describeCollection answers the description of the Collections view or of
// a collection. It reports whether id named one.
func (h *Handler) describeCollection(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	if id == collectionsViewID {
		views, err := h.folderDtos(r, user, []library.Item{collectionsViewItem()})
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
	if err != nil {
		h.internalError(w, r, err)
		return true
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
	views, err := h.folderDtos(r, user, []library.Item{collectionsViewItem()})
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, views)
	return true
}

// collectionListing answers the listings that hold collections: the
// Collections view's, a listing of collections across the server or of the
// collections with titles from a library, and a collection's titles listed
// as its children. Like Jellyfin, a listing asking only for BoxSet lists
// collections whatever folder it names, keeping, for a library that does
// not hold collections, those with a title listed in it. A listing across
// the server lists the addons' collections too (see
// writeCollectionsAcrossServer). It reports whether it answered.
func (h *Handler) collectionListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, hasParent bool, start, limit int) bool {
	if hasParent && parent != collectionsViewID {
		c, err := h.Collections.Get(r.Context(), parent)
		if err == nil {
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
	if !filtered {
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
	dtos = slices.DeleteFunc(dtos, func(dto BaseItemDto) bool { return !filter.keepsData(dto.UserData, false) })
	from, to := bounds(len(dtos), start, limit)
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos[from:to], TotalRecordCount: len(dtos), StartIndex: start})
	return true
}

// listedUnder keeps the collections with a title listed in folder.
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
// collections made by users and those the user's collection libraries
// list, each once, by name. Strand builds its shelves from such a listing.
// jellyfin-web makes one only for its Add to collection dialog, where a
// title can go to collections made by users alone: collectionListing
// answers it with those.
func (h *Handler) writeCollectionsAcrossServer(w http.ResponseWriter, r *http.Request, user accounts.User, made []collections.Collection, start, limit int) {
	fromAddons, err := h.addonCollections(r.Context(), user)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	fields := requestedFields(r)
	dtos, err := h.collectionDtos(r, user, made, fields, false)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	listed, err := h.dtos(r, user, fromAddons, fields, nil)
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
// see, in Jellyfin's order for collections: by premiere date. Listed
// recursively, as apps play a collection, a series or season also brings
// its seasons and released episodes. The request's type filters apply.
func (h *Handler) writeCollectionTitles(w http.ResponseWriter, r *http.Request, user accounts.User, c collections.Collection, start, limit int) {
	titles, err := h.collectionTitles(r.Context(), user, c)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	listed := titles[0]
	if recursive, _ := boolQuery(r, "recursive"); recursive {
		if listed, err = h.withDescendants(r.Context(), user, listed); err != nil {
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
	slices.SortStableFunc(listed, func(a, b library.Item) int { return premiere(a).Compare(premiere(b)) })
	from, to := bounds(len(listed), start, limit)
	dtos, err := h.dtos(r, user, listed[from:to], requestedFields(r), nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(listed), StartIndex: start})
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

// collectionTitles lists, for each collection, the titles in it user may
// see, in the order they were added.
func (h *Handler) collectionTitles(ctx context.Context, user accounts.User, lists ...collections.Collection) ([][]library.Item, error) {
	var ids []accounts.ID
	seen := map[accounts.ID]bool{}
	for _, c := range lists {
		for _, id := range c.Items {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	byID := map[accounts.ID]library.Item{}
	if len(ids) > 0 {
		items, err := h.Library.Items(ctx, user, ids)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			byID[item.ID] = item
		}
	}
	result := make([][]library.Item, len(lists))
	for i, c := range lists {
		for _, id := range c.Items {
			if item, ok := byID[id]; ok {
				result[i] = append(result[i], item)
			}
		}
	}
	return result, nil
}

// collectionDtos describes collections from the titles user may see in
// them, as Jellyfin derives a collection's genres, rating and date from its
// titles: their number, and the movies and episodes under them the user
// played.
func (h *Handler) collectionDtos(r *http.Request, user accounts.User, lists []collections.Collection, fields fieldSet, detail bool) ([]BaseItemDto, error) {
	titles, err := h.collectionTitles(r.Context(), user, lists...)
	if err != nil {
		return nil, err
	}
	var ids, parents []accounts.ID
	for i, c := range lists {
		ids = append(ids, c.ID)
		for _, title := range titles[i] {
			ids = append(ids, title.ID)
			if title.Kind == library.KindSeries || title.Kind == library.KindSeason {
				parents = append(parents, title.ID)
			}
		}
	}
	data, err := h.UserData.Get(r.Context(), user.ID, ids)
	if err != nil {
		return nil, err
	}
	counts, err := h.UserData.EpisodeCounts(r.Context(), user.ID, parents)
	if err != nil {
		return nil, err
	}
	state := userState{data: data, counts: map[accounts.ID]userdata.Counts{}}
	result := make([]BaseItemDto, 0, len(lists))
	for i, c := range lists {
		item, played := collectionItem(c, titles[i], data, counts)
		state.counts[c.ID] = userdata.Counts{Played: played}
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

// collectionItem describes a collection from the titles in it a user may
// see, and counts the movies and episodes under them the user played. Like
// Jellyfin, a collection takes its titles' genres, their least restrictive
// rating, and the earliest premiere date or year; its poster is that of
// its first title with one (see collectionArtwork).
func collectionItem(c collections.Collection, titles []library.Item, data map[accounts.ID]userdata.Data, counts map[accounts.ID]userdata.Counts) (library.Item, int) {
	item := library.Item{ID: c.ID, Kind: library.KindCollection, Name: c.Name, ParentID: collectionsViewID,
		Contents: &library.Contents{Children: len(titles), LastReleased: c.LastAdded}}
	played := 0
	var ratings []string
	for _, title := range titles {
		switch title.Kind {
		case library.KindMovie, library.KindEpisode:
			item.Contents.Released++
			if data[title.ID].Played {
				played++
			}
		case library.KindSeries, library.KindSeason:
			if title.Contents != nil {
				item.Contents.Released += title.Contents.Released
				played += min(counts[title.ID].Played, title.Contents.Released)
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
		item.OfficialRating = slices.MinFunc(ratings, compareRatings)
	}
	return item, played
}

// compareRatings orders ratings from the least restrictive, those Polyfin
// does not know last, as Jellyfin picks a collection's rating.
func compareRatings(a, b string) int {
	rank := func(rating string) (int, int) {
		score, ok := localization.RatingScore(rating)
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
// poster of the title among its own that tag names, as a user's listing
// tagged it from the first title they see, else its first title's poster.
// ok is false when id names no collection.
func (h *Handler) collectionArtwork(r *http.Request, id accounts.ID, imageType string) (url string, confined, ok bool, err error) {
	c, err := h.Collections.Get(r.Context(), id)
	if errors.Is(err, collections.ErrNotFound) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, true, err
	}
	if !strings.EqualFold(imageType, "Primary") {
		return "", false, true, library.ErrNotFound
	}
	tag := query(r, "tag")
	first, firstConfined := "", false
	for _, item := range c.Items {
		poster, confined, err := h.Library.Poster(r.Context(), item)
		if errors.Is(err, library.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", false, true, err
		}
		if tag == "" || library.ImageTag(poster) == tag {
			return poster, confined, true, nil
		}
		if first == "" {
			first, firstConfined = poster, confined
		}
	}
	if first == "" {
		return "", false, true, library.ErrNotFound
	}
	return first, firstConfined, true, nil
}

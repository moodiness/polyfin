package jellyfin

import (
	"cmp"
	"context"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// within reports whether item lies under the folder parent: the series or
// season of an episode, or the library or collection a title was listed
// in.
func (h *Handler) within(ctx context.Context, user accounts.User, item library.Item, parent accounts.ID) bool {
	if item.ParentID == parent || item.SeriesID == parent || item.SeasonID == parent {
		return true
	}
	folders, err := h.Library.Ancestors(ctx, user, item.ID)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(folders, func(folder library.Item) bool { return folder.ID == parent })
}

// keepWithin keeps the items whose folder, as of, lies under parent.
func (h *Handler) keepWithin(ctx context.Context, user accounts.User, items []library.Item, parent accounts.ID, of func(library.Item) library.Item) []library.Item {
	inside := make([]bool, len(items))
	var wg sync.WaitGroup
	limit := make(chan struct{}, 8)
	for i, item := range items {
		wg.Go(func() {
			limit <- struct{}{}
			defer func() { <-limit }()
			inside[i] = h.within(ctx, user, of(item), parent)
		})
	}
	wg.Wait()
	kept := items[:0]
	for i, item := range items {
		if inside[i] {
			kept = append(kept, item)
		}
	}
	return kept
}

func itself(item library.Item) library.Item { return item }

// entryIDs lists the items of entries, each once.
func entryIDs(entries []userdata.Entry) []accounts.ID {
	ids := make([]accounts.ID, 0, len(entries))
	seen := make(map[accounts.ID]bool, len(entries))
	for _, e := range entries {
		if !seen[e.Item] {
			seen[e.Item] = true
			ids = append(ids, e.Item)
		}
	}
	return ids
}

// window returns items[start:start+limit], all of them from start when
// limit is negative.
func window(items []library.Item, start, limit int) []library.Item {
	from := min(max(start, 0), len(items))
	if limit < 0 {
		return items[from:]
	}
	return items[from:min(from+limit, len(items))]
}

// writeItems answers items as a page, whose TotalRecordCount is total.
func (h *Handler) writeItems(w http.ResponseWriter, r *http.Request, user accounts.User, items []library.Item, start, total int) {
	dtos, err := h.dtos(r, user, items, requestedFields(r), nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: total, StartIndex: start})
}

// resume lists what the user can resume: movies and episodes with a resume
// point, played or not, most recently played first.
func (h *Handler) resume(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	parent, hasParent := b.guid(r, "parentId")
	excludeActive, _ := b.bool(r, "excludeActiveSessions")
	countAll := true
	if value, set := b.bool(r, "enableTotalRecordCount"); set {
		countAll = value
	}
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	entries, err := h.UserData.Resumable(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	// Only movies and episodes resume, of the types asked: the others are
	// left out before anything is described.
	keep := itemTypeFilter(r)
	var kinds []library.Kind
	for _, kind := range []library.Kind{library.KindMovie, library.KindEpisode} {
		if keep(library.Item{Kind: kind}) {
			kinds = append(kinds, kind)
		}
	}
	if len(kinds) == 0 {
		h.writeItems(w, r, user, nil, start, 0)
		return
	}
	items, err := h.Library.ItemsOf(r.Context(), user, entryIDs(entries), kinds)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	term := strings.ToLower(strings.TrimSpace(query(r, "searchTerm")))
	var playing map[accounts.ID]bool
	if excludeActive {
		playing = h.playingItems(r, user)
	}
	items = slices.DeleteFunc(items, func(item library.Item) bool {
		return (item.Kind != library.KindMovie && item.Kind != library.KindEpisode) || !keep(item) || playing[item.ID] ||
			(term != "" && !strings.Contains(strings.ToLower(item.Name), term))
	})
	if hasParent {
		items = h.keepWithin(r.Context(), user, items, parent, itself)
	}
	page := window(items, start, limit)
	total := len(items)
	if !countAll {
		// Jellyfin then counts what it returns.
		total = len(page)
	}
	// The titles are queued before the answer, which never waits for their
	// lists.
	h.listAhead(user, page)
	h.writeItems(w, r, user, page, start, total)
}

// playingItems lists what the user's devices play now.
func (h *Handler) playingItems(r *http.Request, user accounts.User) map[accounts.ID]bool {
	devices, err := h.Accounts.Devices(r.Context(), user.ID)
	if err != nil {
		return nil
	}
	playing := map[accounts.ID]bool{}
	for _, device := range devices {
		if now, ok := h.sessions.Playing(device.ID); ok {
			playing[now.Item] = true
			if owner, isVersion := h.Library.VersionOwner(now.Item); isVersion {
				playing[owner] = true
			}
		}
	}
	return playing
}

// watchedSeriesLimit bounds the series Next Up and Upcoming look at among
// those the user played, the most recently played first: each needs its
// description from its addon, and a history imported from a tracking
// service can hold hundreds of series.
const watchedSeriesLimit = 50

// watching is a series the user watches: the episode furthest into it the
// user played, as Next Up starts from it, and the latest date the user
// played one of its episodes.
type watching struct {
	series accounts.ID
	latest *time.Time
}

// nextUp lists, for each series the user is watching, the episode to watch
// next: the first unplayed one after the furthest episode played. Series
// come in the order their furthest played episode was played.
func (h *Handler) nextUp(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	seriesID, oneSeries := b.guid(r, "seriesId")
	parent, hasParent := b.guid(r, "parentId")
	resumable := true
	if value, set := b.bool(r, "enableResumable"); set {
		resumable = value
	}
	countAll := true
	if value, set := b.bool(r, "enableTotalRecordCount"); set {
		countAll = value
	}
	var cutoff *time.Time
	if raw := strings.TrimSpace(query(r, "nextUpDateCutoff")); raw != "" {
		if parsed, ok := parseTime(raw); ok {
			cutoff = &parsed
		} else {
			b.add("nextUpDateCutoff", notValid(raw))
		}
	}
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	entries, err := h.UserData.Episodes(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	data := make(map[accounts.ID]userdata.Data, len(entries))
	var candidates []watching
	seen := map[accounts.ID]bool{}
	for _, e := range entries {
		data[e.Item] = e.Data
		// A series is watched once one of its episodes was played on a
		// date; entries come latest first.
		if e.Played && e.LastPlayed != nil && !seen[e.Series] {
			seen[e.Series] = true
			candidates = append(candidates, watching{series: e.Series, latest: e.LastPlayed})
		}
	}
	if oneSeries {
		// A series asked for is considered whatever the user did with it.
		candidates = []watching{{series: seriesID}}
	} else {
		if cutoff != nil {
			candidates = slices.DeleteFunc(candidates, func(c watching) bool { return c.latest.Before(*cutoff) })
		}
		candidates = candidates[:min(len(candidates), watchedSeriesLimit)]
	}
	type entry struct {
		episode library.Item
		date    *time.Time
	}
	found := make([]*entry, len(candidates))
	var wg sync.WaitGroup
	limiter := make(chan struct{}, 6)
	for i, candidate := range candidates {
		wg.Go(func() {
			limiter <- struct{}{}
			defer func() { <-limiter }()
			if hasParent {
				series, err := h.Library.Item(r.Context(), user, candidate.series)
				if err != nil || !h.within(r.Context(), user, series, parent) {
					return
				}
			}
			episodes, err := h.Library.Episodes(r.Context(), user, candidate.series, nil)
			if err != nil {
				return
			}
			if episode, date, ok := nextEpisode(episodes, data, resumable); ok {
				found[i] = &entry{episode: episode, date: date}
			}
		})
	}
	wg.Wait()
	var next []entry
	for _, e := range found {
		if e != nil {
			next = append(next, *e)
		}
	}
	slices.SortStableFunc(next, func(a, b entry) int {
		switch {
		case before(b.date, a.date):
			return -1
		case before(a.date, b.date):
			return 1
		}
		return 0
	})
	items := make([]library.Item, 0, len(next))
	for _, e := range next {
		items = append(items, e.episode)
	}
	page := window(items, start, limit)
	total := len(items)
	if !countAll {
		total = 0
	}
	h.listAhead(user, page)
	h.writeItems(w, r, user, page, start, total)
}

// upcoming lists the episodes airing from yesterday on, UTC, of the series
// the user watches or marked favorite, soonest first.
func (h *Handler) upcoming(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	parent, hasParent := b.guid(r, "parentId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	watched, err := h.UserData.Episodes(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	favorites, err := h.UserData.Favorites(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	var candidates []accounts.ID
	seen := map[accounts.ID]bool{}
	for _, e := range watched {
		if e.Played && !seen[e.Series] && len(candidates) < watchedSeriesLimit {
			seen[e.Series] = true
			candidates = append(candidates, e.Series)
		}
	}
	// Favorites are items of every kind. Favorite episodes, which name
	// their series, are left out; the other kinds are told apart below,
	// where only series have episodes.
	for _, e := range favorites {
		if e.Series == (accounts.ID{}) && !seen[e.Item] {
			seen[e.Item] = true
			candidates = append(candidates, e.Item)
		}
	}
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
	found := make([][]library.Item, len(candidates))
	var wg sync.WaitGroup
	limiter := make(chan struct{}, 6)
	for i, candidate := range candidates {
		wg.Go(func() {
			limiter <- struct{}{}
			defer func() { <-limiter }()
			// Episodes are listed first: it turns down a favorite that is
			// not a series without asking its addon for it.
			episodes, err := h.Library.Episodes(r.Context(), user, candidate, nil)
			if err != nil {
				return
			}
			if hasParent {
				series, err := h.Library.Item(r.Context(), user, candidate)
				if err != nil || !h.within(r.Context(), user, series, parent) {
					return
				}
			}
			found[i] = slices.DeleteFunc(episodes, func(e library.Item) bool {
				return e.PremiereDate == nil || e.PremiereDate.Before(from)
			})
		})
	}
	wg.Wait()
	items := slices.Concat(found...)
	slices.SortStableFunc(items, func(a, b library.Item) int {
		return cmp.Or(a.PremiereDate.Compare(*b.PremiereDate),
			cmp.Compare(strings.ToLower(a.SeriesName), strings.ToLower(b.SeriesName)),
			cmp.Compare(a.ParentIndexNumber, b.ParentIndexNumber),
			cmp.Compare(a.IndexNumber, b.IndexNumber))
	})
	page := window(items, start, limit)
	// Jellyfin counts the episodes of the page, not all of them.
	h.writeItems(w, r, user, page, start, len(page))
}

// nextEpisode picks the episode to watch next among a series' episodes, in
// order: the first released, unplayed episode after the furthest one
// played, or the first episode when none was played. Specials are left
// out. A next episode already started is only offered when resumable is
// set. date is when the furthest played episode was played.
func nextEpisode(episodes []library.Item, data map[accounts.ID]userdata.Data, resumable bool) (library.Item, *time.Time, bool) {
	regular := regularEpisodes(episodes)
	furthest := -1
	for i, episode := range regular {
		if data[episode.ID].Played {
			furthest = i
		}
	}
	var date *time.Time
	if furthest >= 0 {
		date = data[regular[furthest].ID].LastPlayed
	}
	for _, episode := range regular[furthest+1:] {
		if !episode.Available {
			break
		}
		d := data[episode.ID]
		if d.Played {
			continue
		}
		if d.Position > 0 && !resumable {
			return library.Item{}, nil, false
		}
		return episode, date, true
	}
	return library.Item{}, nil, false
}

// regularEpisodes are a series' episodes, in order, without the specials,
// which Next Up leaves out.
func regularEpisodes(episodes []library.Item) []library.Item {
	return slices.DeleteFunc(slices.Clone(episodes), func(e library.Item) bool { return e.ParentIndexNumber == 0 })
}

// before reports whether date a comes strictly before date b, a missing
// date coming before any other.
func before(a, b *time.Time) bool {
	switch {
	case a == nil:
		return b != nil
	case b == nil:
		return false
	}
	return a.Before(*b)
}

// stateFilter narrows a listing to what the user did with its items.
type stateFilter struct {
	favorite, notFavorite, played, unplayed, resumable, likes, dislikes, favoriteOrLikes bool
}

// stateFilterOf reads the user-state conditions of a listing, from Filters
// (repeated or comma-separated) and from isFavorite and isPlayed.
func stateFilterOf(r *http.Request) (stateFilter, bool) {
	var f stateFilter
	for _, raw := range listQuery(r, "filters") {
		switch strings.ToLower(raw) {
		case "isfavorite":
			f.favorite = true
		case "isplayed":
			f.played = true
		case "isunplayed":
			f.unplayed = true
		case "isresumable":
			f.resumable = true
		case "likes":
			f.likes = true
		case "dislikes":
			f.dislikes = true
		case "isfavoriteorlikes":
			f.favoriteOrLikes = true
		}
	}
	if value, set := boolQuery(r, "isFavorite"); set {
		f.favorite, f.notFavorite = f.favorite || value, f.notFavorite || !value
	}
	if value, set := boolQuery(r, "isPlayed"); set {
		f.played, f.unplayed = f.played || value, f.unplayed || !value
	}
	return f, f != stateFilter{}
}

// keeps reports whether the user's state lets the filter keep item: all
// conditions must hold.
func (f stateFilter) keeps(item library.Item, state userState) bool {
	return f.keepsData(state.of(item), state.resumable(item))
}

// keepsData reports whether what the user did with an item, data, and
// whether they can resume it let the filter keep the item.
func (f stateFilter) keepsData(data UserItemData, resumable bool) bool {
	likes := data.Likes != nil && *data.Likes
	return (!f.favorite || data.IsFavorite) && (!f.notFavorite || !data.IsFavorite) &&
		(!f.played || data.Played) && (!f.unplayed || !data.Played) &&
		(!f.resumable || resumable) && (!f.likes || likes) && (!f.dislikes || !likes) &&
		(!f.favoriteOrLikes || data.IsFavorite || likes)
}

// stateCandidates lists, from the user's data, the items that may pass the
// filter: those the user marked, with the series and seasons of marked
// episodes. ok is false when the filter only excludes items, which the
// user's data cannot list.
func (h *Handler) stateCandidates(ctx context.Context, user accounts.User, f stateFilter) ([]library.Item, bool, error) {
	var entries []userdata.Entry
	var err error
	switch {
	case f.favorite || f.favoriteOrLikes || f.likes:
		var favorites, liked []userdata.Entry
		if favorites, err = h.UserData.Favorites(ctx, user.ID); err == nil && (f.likes || f.favoriteOrLikes) {
			liked, err = h.UserData.Liked(ctx, user.ID)
		}
		entries = append(favorites, liked...)
	case f.resumable:
		entries, err = h.UserData.Resumable(ctx, user.ID)
	case f.played:
		entries, err = h.UserData.Played(ctx, user.ID)
	default:
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	ids := entryIDs(entries)
	if f.played || f.resumable {
		// Series and seasons are played, or resumable, through their
		// episodes.
		seen := map[accounts.ID]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		for _, e := range entries {
			for _, parent := range []accounts.ID{e.Season, e.Series} {
				if parent != (accounts.ID{}) && !seen[parent] {
					seen[parent] = true
					ids = append(ids, parent)
				}
			}
		}
	}
	items, err := h.Library.Items(ctx, user, ids)
	return items, true, err
}

// stateListing answers a listing narrowed to what the user did.
func (h *Handler) stateListing(w http.ResponseWriter, r *http.Request, user accounts.User, f stateFilter, parent accounts.ID, hasParent bool, start, limit int) {
	items, listed, err := h.stateCandidates(r.Context(), user, f)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if !listed {
		h.excludingListing(w, r, user, f, parent, hasParent, start, limit)
		return
	}
	keep := itemTypeFilter(r)
	items = slices.DeleteFunc(items, func(item library.Item) bool { return !keep(item) })
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	items = slices.DeleteFunc(items, func(item library.Item) bool { return !f.keeps(item, state) })
	if hasParent {
		items = h.keepWithin(r.Context(), user, items, parent, itself)
	}
	sortItems(r, items, state)
	page := window(items, start, limit)
	writeJSON(w, http.StatusOK, QueryResult{Items: h.listDtos(r, user, page, requestedFields(r), state), TotalRecordCount: len(items), StartIndex: start})
}

// scanLimit bounds how many of a folder's children a listing that excludes
// items reads to fill a page.
const scanLimit = 2000

// excludingListing lists the children of a folder the filter keeps, for
// filters that exclude items (unplayed, not favorite, not liked). The
// catalog is read from its start, from cache, so that each page starts
// where the previous one ended.
func (h *Handler) excludingListing(w http.ResponseWriter, r *http.Request, user accounts.User, f stateFilter, parent accounts.ID, hasParent bool, start, limit int) {
	if !hasParent {
		writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
		return
	}
	genre, ok := h.genreFilter(r, user, parent)
	if !ok {
		writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
		return
	}
	keep := itemTypeFilter(r)
	var kept []library.Item
	var states []userState
	total, excluded, offset := 0, 0, 0
	for offset < scanLimit && len(kept) < max(start, 0)+limit {
		page, err := h.Library.Children(r.Context(), user, parent, offset, defaultPageSize, genre)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		state, err := h.userState(r.Context(), user, page.Items)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		total = page.Total
		for _, item := range page.Items {
			if !f.keeps(item, state) {
				excluded++
			} else if keep(item) {
				kept = append(kept, item)
				states = append(states, state)
			}
		}
		offset += len(page.Items)
		if !page.More || len(page.Items) == 0 {
			break
		}
	}
	from := min(max(start, 0), len(kept))
	to := min(from+limit, len(kept))
	dtos := make([]BaseItemDto, 0, to-from)
	for i := from; i < to; i++ {
		dtos = append(dtos, h.listDto(r, user, kept[i], requestedFields(r), states[i]))
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: max(total-excluded, len(kept)), StartIndex: start})
}

// sortItems orders items by the request's sortBy and sortOrder, by name
// when none is given, and by name among equals.
func sortItems(r *http.Request, items []library.Item, state userState) {
	keys := listQuery(r, "sortBy")
	if len(keys) == 0 {
		keys = []string{"SortName"}
	}
	orders := listQuery(r, "sortOrder")
	descending := func(i int) bool {
		if i < len(orders) {
			return strings.EqualFold(orders[i], "Descending")
		}
		return len(orders) == 1 && strings.EqualFold(orders[0], "Descending")
	}
	slices.SortStableFunc(items, func(a, b library.Item) int {
		for i, key := range keys {
			result := compareBy(key, a, b, state)
			if descending(i) {
				result = -result
			}
			if result != 0 {
				return result
			}
		}
		return compareBy("SortName", a, b, state)
	})
	if slices.ContainsFunc(keys, func(key string) bool { return strings.EqualFold(key, "Random") }) {
		rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	}
}

func compareBy(key string, a, b library.Item, state userState) int {
	da, db := state.data[a.ID], state.data[b.ID]
	switch strings.ToLower(key) {
	case "sortname", "name", "seriessortname":
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	case "dateplayed", "seriesdateplayed":
		switch {
		case before(da.LastPlayed, db.LastPlayed):
			return -1
		case before(db.LastPlayed, da.LastPlayed):
			return 1
		}
	case "playcount":
		return cmp.Compare(da.PlayCount, db.PlayCount)
	case "isplayed":
		return compareBool(state.of(a).Played, state.of(b).Played)
	case "isunplayed":
		return compareBool(!state.of(a).Played, !state.of(b).Played)
	case "isfavoriteorliked":
		likes := func(d userdata.Data) bool { l := d.Likes(); return l != nil && *l }
		return compareBool(da.Favorite || likes(da), db.Favorite || likes(db))
	case "premieredate", "productionyear":
		switch {
		case before(a.PremiereDate, b.PremiereDate):
			return -1
		case before(b.PremiereDate, a.PremiereDate):
			return 1
		}
	case "communityrating":
		return cmp.Compare(a.CommunityRating, b.CommunityRating)
	case "runtime":
		return cmp.Compare(a.Runtime, b.Runtime)
	}
	return 0
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

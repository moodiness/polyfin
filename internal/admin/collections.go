package admin

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
)

// Collections, under Content › Libraries.
//
// The collections users make are the server's, as Jellyfin's BoxSets: the
// users who may manage collections make and change them from their apps,
// and administrators from the admin app, which also orders their entries
// and chooses the order apps list them in (collections.DisplayOrder).
// Polyfin keeps no collection of a user's own: the admin app lists every
// collection.

// CollectionTitles describes the titles of collections, as an
// administrator sees them (see library.Service).
type CollectionTitles interface {
	Items(ctx context.Context, user accounts.User, ids []accounts.ID) ([]library.Item, error)
	IsSmartList(ctx context.Context, user accounts.User, id accounts.ID) (bool, error)
}

// CollectionSearch finds what a collection's page offers to add, as
// jellyfin-web's search finds it (see jellyfin.Handler).
type CollectionSearch interface {
	CollectionSearch(ctx context.Context, user accounts.User, term string, limit int) ([]library.Item, error)
}

// collectionChoicesLimit bounds the titles of each kind a search of the
// picker answers.
const collectionChoicesLimit = 30

// collectionSummaryJSON is a collection as Content › Libraries lists it.
type collectionSummaryJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Entries      int    `json:"entries"`
	Titles       int    `json:"titles"`
	Collections  int    `json:"collections"`
	DisplayOrder string `json:"displayOrder"`
}

// collectionEntryJSON is an entry of a collection's page. Kind is
// "collection" for a collection held, whose Entries it counts; else the
// title's kind, "smartList" or "addonCollection" for those groups, and
// "missing" for a title the library no longer has. PosterID is the item
// whose Primary image is its poster, served by Polyfin's own image route.
type collectionEntryJSON struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Year     *int    `json:"year"`
	PosterID *string `json:"posterId"`
	Entries  *int    `json:"entries"`
}

type collectionRefJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// collectionDetailJSON is a collection's page: its entries in its order,
// and the collections that hold it.
type collectionDetailJSON struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	DisplayOrder string                `json:"displayOrder"`
	Entries      []collectionEntryJSON `json:"entries"`
	Holders      []collectionRefJSON   `json:"holders"`
}

// collectionChoiceTitleJSON is a title the picker offers; Held tells it
// is in the collection already.
type collectionChoiceTitleJSON struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Year     *int    `json:"year"`
	PosterID *string `json:"posterId"`
	Held     bool    `json:"held"`
}

// collectionChoiceJSON is a collection the picker offers. Refusal tells
// why it may not go in: "self", "holds" (it holds this one, at any
// depth) or "tooDeep" (collections would nest deeper than
// collections.MaxDepth); nil when it may.
type collectionChoiceJSON struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Entries int     `json:"entries"`
	Held    bool    `json:"held"`
	Refusal *string `json:"refusal"`
}

func newCollectionSummaryJSON(c collections.Collection) collectionSummaryJSON {
	return collectionSummaryJSON{ID: c.ID.String(), Name: c.Name, Entries: len(c.Entries), Titles: len(c.Items),
		Collections: len(c.Collections), DisplayOrder: string(c.DisplayOrder)}
}

// collectionError answers the errors of a change to a collection; it
// reports whether it did.
func collectionError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{collections.ErrNotFound, http.StatusNotFound, "not_found"},
		{collections.ErrCycle, http.StatusConflict, "collection_cycle"},
		{collections.ErrTooDeep, http.StatusConflict, "collection_too_deep"},
		{collections.ErrEntriesChanged, http.StatusConflict, "entries_changed"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// collectionName reads a collection's name, answering 400 invalid_name
// for one empty or too long.
func collectionName(w http.ResponseWriter, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > collections.MaxNameLength {
		writeError(w, http.StatusBadRequest, "invalid_name")
		return "", false
	}
	return name, true
}

// collectionsOn answers 404 when the server keeps no collections, and
// reports whether it does.
func (h *handler) collectionsOn(w http.ResponseWriter) bool {
	if h.Collections == nil || h.CollectionTitles == nil || h.CollectionSearch == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return false
	}
	return true
}

// listCollections lists the server's collections, by name.
func (h *handler) listCollections(w http.ResponseWriter, r *http.Request) {
	if !h.collectionsOn(w) {
		return
	}
	all, err := h.Collections.All(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]collectionSummaryJSON, 0, len(all))
	for _, c := range all {
		result = append(result, newCollectionSummaryJSON(c))
	}
	writeJSON(w, http.StatusOK, result)
}

// createCollection creates an empty collection, and answers its page.
func (h *handler) createCollection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !h.collectionsOn(w) || !decode(w, r, &body) {
		return
	}
	name, ok := collectionName(w, body.Name)
	if !ok {
		return
	}
	id, err := h.Collections.Create(r.Context(), name, false, nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeCollection(w, r, id, http.StatusCreated)
}

// collectionOf answers a collection's page.
func (h *handler) collectionOf(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if ok && h.collectionsOn(w) {
		h.writeCollection(w, r, id, http.StatusOK)
	}
}

// changeCollection renames a collection, or sets its display order.
func (h *handler) changeCollection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         *string `json:"name"`
		DisplayOrder *string `json:"displayOrder"`
	}
	id, ok := pathID(w, r, "id")
	if !ok || !h.collectionsOn(w) || !decode(w, r, &body) {
		return
	}
	var changes collections.Changes
	if body.Name != nil {
		name, ok := collectionName(w, *body.Name)
		if !ok {
			return
		}
		changes.Name = &name
	}
	if body.DisplayOrder != nil {
		order := collections.DisplayOrder(*body.DisplayOrder)
		if !order.Valid() {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		changes.DisplayOrder = &order
	}
	if err := h.Collections.Change(r.Context(), id, changes); err != nil {
		if !collectionError(w, err) {
			h.internalError(w, r, err)
		}
		return
	}
	h.writeCollection(w, r, id, http.StatusOK)
}

// deleteCollection deletes a collection, which also takes it out of the
// collections that held it.
func (h *handler) deleteCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok || !h.collectionsOn(w) {
		return
	}
	if err := h.Collections.Delete(r.Context(), id); err != nil {
		if !collectionError(w, err) {
			h.internalError(w, r, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// collectionEntriesBody is the entries a change names, or every entry of a
// collection in a new order.
type collectionEntriesBody struct {
	IDs []string `json:"ids"`
}

// entryIDs parses the identifiers of a body, answering 400 for one that
// is not one.
func entryIDs(w http.ResponseWriter, raw []string) ([]accounts.ID, bool) {
	ids := make([]accounts.ID, 0, len(raw))
	for _, value := range raw {
		id, err := accounts.ParseID(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

// addCollectionEntries adds titles and collections after a collection's
// entries. Each must be a collection, or a title the administrator sees
// that a collection may hold (jellyfin.CollectionKinds), else nothing is
// added (400 unknown_entry). A collection that would hold itself, at any
// depth, or nest too deep is refused (409).
func (h *handler) addCollectionEntries(w http.ResponseWriter, r *http.Request) {
	var body collectionEntriesBody
	id, ok := pathID(w, r, "id")
	if !ok || !h.collectionsOn(w) || !decode(w, r, &body) {
		return
	}
	ids, ok := entryIDs(w, body.IDs)
	if !ok {
		return
	}
	nested, err := h.Collections.Some(r.Context(), ids)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	found := map[accounts.ID]bool{}
	for _, c := range nested {
		found[c.ID] = true
	}
	titles := slices.DeleteFunc(slices.Clone(ids), func(id accounts.ID) bool { return found[id] })
	if len(titles) > 0 {
		items, err := h.CollectionTitles.Items(r.Context(), sessionFrom(r.Context()).User, titles)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, item := range items {
			found[item.ID] = slices.Contains(jellyfin.CollectionKinds, item.Kind)
		}
	}
	for _, entry := range ids {
		if !found[entry] {
			writeError(w, http.StatusBadRequest, "unknown_entry")
			return
		}
	}
	h.changedCollection(w, r, id, h.Collections.Add(r.Context(), id, ids))
}

// removeCollectionEntries takes titles and collections out of a
// collection; those it does not hold are ignored.
func (h *handler) removeCollectionEntries(w http.ResponseWriter, r *http.Request) {
	var body collectionEntriesBody
	id, ok := pathID(w, r, "id")
	if !ok || !h.collectionsOn(w) || !decode(w, r, &body) {
		return
	}
	if ids, ok := entryIDs(w, body.IDs); ok {
		h.changedCollection(w, r, id, h.Collections.Remove(r.Context(), id, ids))
	}
}

// orderCollectionEntries puts a collection's entries in the order the
// body names every one of them in; 409 entries_changed when it does not
// name them all, as when they changed meanwhile.
func (h *handler) orderCollectionEntries(w http.ResponseWriter, r *http.Request) {
	var body collectionEntriesBody
	id, ok := pathID(w, r, "id")
	if !ok || !h.collectionsOn(w) || !decode(w, r, &body) {
		return
	}
	if ids, ok := entryIDs(w, body.IDs); ok {
		h.changedCollection(w, r, id, h.Collections.Reorder(r.Context(), id, ids))
	}
}

// changedCollection answers a change to a collection with its page.
func (h *handler) changedCollection(w http.ResponseWriter, r *http.Request, id accounts.ID, err error) {
	if err != nil {
		if !collectionError(w, err) {
			h.internalError(w, r, err)
		}
		return
	}
	h.writeCollection(w, r, id, http.StatusOK)
}

// writeCollection answers a collection's page with status.
func (h *handler) writeCollection(w http.ResponseWriter, r *http.Request, id accounts.ID, status int) {
	ctx := r.Context()
	user := sessionFrom(ctx).User
	c, err := h.Collections.Get(ctx, id)
	if err != nil {
		if !collectionError(w, err) {
			h.internalError(w, r, err)
		}
		return
	}
	nested, err := h.Collections.Some(ctx, c.Collections)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	held := make(map[accounts.ID]collections.Collection, len(nested))
	for _, inner := range nested {
		held[inner.ID] = inner
	}
	items, err := h.CollectionTitles.Items(ctx, user, c.Items)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	titles := make(map[accounts.ID]library.Item, len(items))
	for _, item := range items {
		titles[item.ID] = item
	}
	result := collectionDetailJSON{ID: c.ID.String(), Name: c.Name, DisplayOrder: string(c.DisplayOrder),
		Entries: []collectionEntryJSON{}, Holders: []collectionRefJSON{}}
	for _, entry := range c.Entries {
		if inner, ok := held[entry]; ok {
			result.Entries = append(result.Entries, collectionEntryJSON{ID: entry.String(), Kind: "collection", Name: inner.Name,
				Entries: new(len(inner.Entries))})
			continue
		}
		item, ok := titles[entry]
		if !ok {
			result.Entries = append(result.Entries, collectionEntryJSON{ID: entry.String(), Kind: "missing"})
			continue
		}
		shown := collectionEntryJSON{ID: entry.String(), Kind: string(item.Kind), Name: item.Name, PosterID: new(entry.String())}
		if item.ProductionYear > 0 {
			shown.Year = new(item.ProductionYear)
		}
		if item.Kind == library.KindCollection {
			smart, err := h.CollectionTitles.IsSmartList(ctx, user, entry)
			if err != nil {
				h.internalError(w, r, err)
				return
			}
			shown.Kind = "addonCollection"
			if smart {
				shown.Kind, shown.PosterID = "smartList", nil
			}
		}
		result.Entries = append(result.Entries, shown)
	}
	holders, err := h.Collections.Holders(ctx, id)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, holder := range holders {
		result.Holders = append(result.Holders, collectionRefJSON{ID: holder.ID.String(), Name: holder.Name})
	}
	writeJSON(w, status, result)
}

// collectionChoices answers what the picker of a collection's page offers:
// what jellyfin-web's search finds for q (see CollectionSearch), and
// every other collection, those whose name holds q when it is set, each
// telling whether the collection holds it already, and for a collection
// why it may not go in.
func (h *handler) collectionChoices(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok || !h.collectionsOn(w) {
		return
	}
	ctx := r.Context()
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	all, err := h.Collections.All(ctx)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	index := slices.IndexFunc(all, func(c collections.Collection) bool { return c.ID == id })
	if index < 0 {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	c := all[index]
	result := struct {
		Titles      []collectionChoiceTitleJSON `json:"titles"`
		Collections []collectionChoiceJSON      `json:"collections"`
	}{[]collectionChoiceTitleJSON{}, []collectionChoiceJSON{}}
	if term != "" {
		found, err := h.CollectionSearch.CollectionSearch(ctx, sessionFrom(ctx).User, term, collectionChoicesLimit)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, item := range found {
			title := collectionChoiceTitleJSON{ID: item.ID.String(), Name: item.Name, Kind: string(item.Kind),
				PosterID: new(item.ID.String()), Held: slices.Contains(c.Items, item.ID)}
			if item.ProductionYear > 0 {
				title.Year = new(item.ProductionYear)
			}
			result.Titles = append(result.Titles, title)
		}
	}
	lower := strings.ToLower(term)
	for _, other := range all {
		if other.ID == id || term != "" && !strings.Contains(strings.ToLower(other.Name), lower) {
			continue
		}
		choice := collectionChoiceJSON{ID: other.ID.String(), Name: other.Name, Entries: len(other.Entries),
			Held: slices.Contains(c.Collections, other.ID)}
		if !choice.Held {
			switch err := collections.Refusal(all, id, other.ID); {
			case errors.Is(err, collections.ErrCycle):
				choice.Refusal = new("holds")
			case errors.Is(err, collections.ErrTooDeep):
				choice.Refusal = new("tooDeep")
			}
		}
		result.Collections = append(result.Collections, choice)
	}
	writeJSON(w, http.StatusOK, result)
}

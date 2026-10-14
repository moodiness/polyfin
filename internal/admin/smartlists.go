package admin

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/smartlists"
)

// SmartPreviews judges smart lists before they are saved, and tells the
// catalogs they may draw from (see library.Service).
type SmartPreviews interface {
	PreviewSmartList(ctx context.Context, user accounts.User, list smartlists.List) (library.SmartPreview, error)
	SmartCatalogs(ctx context.Context, user accounts.User, shared bool) ([]library.SmartCatalog, error)
}

// smartListJSON is a smart list. ItemID is its item in Jellyfin apps.
type smartListJSON struct {
	ID        string               `json:"id"`
	ItemID    string               `json:"itemId"`
	Name      string               `json:"name"`
	ShownAs   string               `json:"shownAs"`
	Catalogs  []smartlists.Catalog `json:"catalogs"`
	Rules     smartlists.Rule      `json:"rules"`
	Sort      string               `json:"sort"`
	MaxItems  *int                 `json:"maxItems"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

func newSmartListJSON(l smartlists.List) smartListJSON {
	result := smartListJSON{ID: l.ID.String(), ItemID: library.SmartListItemID(l.ID).String(), Name: l.Name, ShownAs: l.ShownAs,
		Catalogs: l.Catalogs, Rules: l.Rules, Sort: l.Sort, UpdatedAt: l.UpdatedAt.UTC()}
	if result.Catalogs == nil {
		result.Catalogs = []smartlists.Catalog{}
	}
	if l.MaxItems > 0 {
		result.MaxItems = new(l.MaxItems)
	}
	return result
}

// smartListRequest is a smart list as the admin app sends it. MaxItems is
// read as any number, so that one that is not a whole number is refused
// as a maximum, not as a request.
type smartListRequest struct {
	Name     string               `json:"name"`
	ShownAs  string               `json:"shownAs"`
	Catalogs []smartlists.Catalog `json:"catalogs"`
	Rules    smartlists.Rule      `json:"rules"`
	Sort     string               `json:"sort"`
	MaxItems *float64             `json:"maxItems"`
}

// smartList reads the list a request sends for scope, answering it when
// it is not one.
func smartList(w http.ResponseWriter, r *http.Request, scope addons.Scope) (smartlists.List, bool) {
	var body smartListRequest
	if !decode(w, r, &body) {
		return smartlists.List{}, false
	}
	list := smartlists.List{Owner: scope.Owner, Name: body.Name, ShownAs: body.ShownAs, Catalogs: body.Catalogs, Rules: body.Rules, Sort: body.Sort}
	if max := body.MaxItems; max != nil {
		if *max != math.Trunc(*max) || *max < 1 || *max > smartlists.MaxItems {
			writeError(w, http.StatusBadRequest, "invalid_smart_list_max_items")
			return smartlists.List{}, false
		}
		list.MaxItems = int(*max)
	}
	return list, true
}

// smartListError answers the errors of smart lists and reports whether err
// was one of them.
func smartListError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{smartlists.ErrNotFound, http.StatusNotFound, "not_found"},
		{smartlists.ErrInvalidName, http.StatusBadRequest, "invalid_smart_list_name"},
		{smartlists.ErrInvalidShownAs, http.StatusBadRequest, "invalid_smart_list_shown_as"},
		{smartlists.ErrInvalidCatalogs, http.StatusBadRequest, "invalid_smart_list_catalogs"},
		{smartlists.ErrInvalidRules, http.StatusBadRequest, "invalid_smart_list_rules"},
		{smartlists.ErrInvalidSort, http.StatusBadRequest, "invalid_smart_list_sort"},
		{smartlists.ErrInvalidMaxItems, http.StatusBadRequest, "invalid_smart_list_max_items"},
		{smartlists.ErrTooMany, http.StatusConflict, "too_many_smart_lists"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// answerSmartList answers a list saved, or why it was not.
func (h *handler) answerSmartList(w http.ResponseWriter, r *http.Request, status int, list smartlists.List, err error) {
	if smartListError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, status, newSmartListJSON(list))
}

// smartLists answers the smart lists of a scope, in the order they were
// made: the server's for administrators, or the caller's own.
func (h *handler) smartLists(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	lists, err := h.SmartLists.Lists(r.Context(), scope.Owner)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]smartListJSON, 0, len(lists))
	for _, l := range lists {
		result = append(result, newSmartListJSON(l))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) smartListOf(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := h.SmartLists.Get(r.Context(), scope.Owner, id)
	h.answerSmartList(w, r, http.StatusOK, list, err)
}

// createSmartList makes a list in a scope. A user whose own addons are
// turned off makes none: their apps would not show it.
func (h *handler) createSmartList(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	list, ok := smartList(w, r, scope)
	if !ok {
		return
	}
	list, err := h.SmartLists.Create(r.Context(), list)
	h.answerSmartList(w, r, http.StatusCreated, list, err)
}

func (h *handler) updateSmartList(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, ok := smartList(w, r, scope)
	if !ok {
		return
	}
	list.ID = id
	list, err := h.SmartLists.Update(r.Context(), list)
	h.answerSmartList(w, r, http.StatusOK, list, err)
}

func (h *handler) deleteSmartList(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.SmartLists.Delete(r.Context(), scope.Owner, id)
	if smartListError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// smartTitleJSON is a title of a list's preview. Languages are those its
// description gives, by their ISO 639-1 code when they have one.
type smartTitleJSON struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Year      *int     `json:"year"`
	Rating    *float64 `json:"rating"`
	Genres    []string `json:"genres"`
	Languages []string `json:"languages"`
}

// previewSmartList answers the titles a list sent, saved or not, lists for
// the caller now: the first ones, how many it lists, how many may match
// once their description is read, and whether every catalog was read in
// time (complete).
func (h *handler) previewSmartList(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	list, ok := smartList(w, r, scope)
	if !ok {
		return
	}
	if err := list.Validate(); smartListError(w, err) {
		return
	}
	preview, err := h.SmartPreviews.PreviewSmartList(r.Context(), sessionFrom(r.Context()).User, list)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	titles := make([]smartTitleJSON, 0, len(preview.Titles))
	for i, item := range preview.Titles {
		title := smartTitleJSON{ID: item.ID.String(), Name: item.Name, Type: string(item.Kind), Genres: item.Genres, Languages: []string{}}
		if title.Genres == nil {
			title.Genres = []string{}
		}
		for _, code := range preview.Languages[i] {
			title.Languages = append(title.Languages, localization.LanguageTag(code))
		}
		if item.ProductionYear > 0 {
			title.Year = new(item.ProductionYear)
		}
		if item.CommunityRating > 0 {
			title.Rating = new(item.CommunityRating)
		}
		titles = append(titles, title)
	}
	writeJSON(w, http.StatusOK, struct {
		Titles   []smartTitleJSON `json:"titles"`
		Listed   int              `json:"listed"`
		Unknown  int              `json:"unknown"`
		Complete bool             `json:"complete"`
	}{titles, preview.Listed, preview.Unknown, preview.Complete})
}

// smartCatalogJSON is a catalog a scope's lists may draw from. Shared is
// set for one of the server's addons, which a user's own list may draw
// from while they use the server's sources.
type smartCatalogJSON struct {
	AddonID     string   `json:"addonId"`
	AddonName   string   `json:"addonName"`
	Shared      bool     `json:"shared"`
	CatalogType string   `json:"catalogType"`
	CatalogID   string   `json:"catalogId"`
	CatalogName string   `json:"catalogName"`
	Genres      []string `json:"genres"`
}

// smartCatalogs answers the catalogs a scope's lists may draw from, and
// the languages their rules may name, by ISO 639-1 code.
func (h *handler) smartCatalogs(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	catalogs, err := h.SmartPreviews.SmartCatalogs(r.Context(), sessionFrom(r.Context()).User, scope.Owner == nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]smartCatalogJSON, 0, len(catalogs))
	for _, c := range catalogs {
		genres := c.Genres
		if genres == nil {
			genres = []string{}
		}
		result = append(result, smartCatalogJSON{AddonID: c.AddonID.String(), AddonName: c.AddonName, Shared: c.Shared,
			CatalogType: c.Catalog.Type, CatalogID: c.Catalog.ID, CatalogName: c.Catalog.Name, Genres: genres})
	}
	writeJSON(w, http.StatusOK, struct {
		Catalogs  []smartCatalogJSON `json:"catalogs"`
		Languages []string           `json:"languages"`
	}{result, smartlists.Languages()})
}

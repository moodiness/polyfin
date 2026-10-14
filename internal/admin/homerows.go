package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/jellyfin"
)

// HomeRowChoices tells the collections a home row may list, as the Jellyfin
// API describes them to user.
type HomeRowChoices interface {
	HomeRowCollections(ctx context.Context, user accounts.User) ([]jellyfin.HomeRowCollection, error)
}

// homeRowJSON is a row of the web player's home page (Settings › Web
// player › Home rows, see package homerows). addonId, catalogType and
// catalogId name a catalog row's catalog; collectionId a collection
// row's collection; count is how many rows a Because you watched row
// makes; users are those a row not shown to everyone is shown to.
type homeRowJSON struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Place        string   `json:"place"`
	AddonID      string   `json:"addonId"`
	CatalogType  string   `json:"catalogType"`
	CatalogID    string   `json:"catalogId"`
	CollectionID string   `json:"collectionId"`
	Count        int      `json:"count"`
	Everyone     bool     `json:"everyone"`
	Users        []string `json:"users"`
	Library      bool     `json:"library"`
}

func newHomeRowJSON(r homerows.Row) homeRowJSON {
	result := homeRowJSON{ID: r.ID.String(), Name: r.Name, Kind: string(r.Kind), Place: string(r.Place), CatalogType: r.CatalogType,
		CatalogID: r.CatalogID, Count: r.Count, Everyone: r.Everyone, Users: []string{}, Library: r.Library}
	if r.Addon != (accounts.ID{}) {
		result.AddonID = r.Addon.String()
	}
	if r.Collection != (accounts.ID{}) {
		result.CollectionID = r.Collection.String()
	}
	for _, user := range r.Users {
		result.Users = append(result.Users, user.String())
	}
	return result
}

// row reads a row from its JSON: a count left out is the default one, an
// identifier that is not one leaves the row invalid.
func (j homeRowJSON) row() (homerows.Row, bool) {
	r := homerows.Row{Name: j.Name, Kind: homerows.Kind(j.Kind), Place: homerows.Place(j.Place), CatalogType: j.CatalogType,
		CatalogID: j.CatalogID, Count: j.Count, Everyone: j.Everyone, Library: j.Library}
	if r.Count == 0 {
		r.Count = homerows.DefaultCount
	}
	ok := true
	parse := func(raw string, into *accounts.ID) {
		if raw == "" {
			return
		}
		id, err := accounts.ParseID(raw)
		if err != nil {
			ok = false
		}
		*into = id
	}
	parse(j.AddonID, &r.Addon)
	parse(j.CollectionID, &r.Collection)
	for _, raw := range j.Users {
		var user accounts.ID
		parse(raw, &user)
		r.Users = append(r.Users, user)
	}
	if r.Everyone {
		r.Users = nil
	}
	return r, ok && r.Valid()
}

// homeRowError answers the errors of the rows' store, and reports whether
// it did.
func homeRowError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, homerows.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, homerows.ErrTooMany):
		writeError(w, http.StatusConflict, "too_many_home_rows")
	case errors.Is(err, homerows.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_home_row")
	default:
		return false
	}
	return true
}

// homeRows answers the rows, in order.
func (h *handler) homeRows(w http.ResponseWriter, r *http.Request) {
	rows, err := h.HomeRows.Rows(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]homeRowJSON, 0, len(rows))
	for _, row := range rows {
		result = append(result, newHomeRowJSON(row))
	}
	writeJSON(w, http.StatusOK, struct {
		Rows []homeRowJSON `json:"rows"`
	}{result})
}

// homeRowCatalogJSON is a catalog of the server's addons a row may list.
type homeRowCatalogJSON struct {
	AddonID     string `json:"addonId"`
	AddonName   string `json:"addonName"`
	CatalogType string `json:"catalogType"`
	CatalogID   string `json:"catalogId"`
	CatalogName string `json:"catalogName"`
}

// homeRowCollectionJSON is a collection a row may list, and where it comes
// from: users, smart (a smart list) or addon.
type homeRowCollectionJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

// homeRowChoices answers the catalogs and collections a row may list: the
// catalogs of the server's addons that can be browsed, and the
// collections the administrator sees.
func (h *handler) homeRowChoices(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	catalogs := []homeRowCatalogJSON{}
	if h.SmartPreviews != nil {
		found, err := h.SmartPreviews.SmartCatalogs(r.Context(), user, true)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, c := range found {
			if c.Shared {
				catalogs = append(catalogs, homeRowCatalogJSON{AddonID: c.AddonID.String(), AddonName: c.AddonName, CatalogType: c.Catalog.Type,
					CatalogID: c.Catalog.ID, CatalogName: c.Catalog.Name})
			}
		}
	}
	collections := []homeRowCollectionJSON{}
	if h.HomeRowChoices != nil {
		found, err := h.HomeRowChoices.HomeRowCollections(r.Context(), user)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, c := range found {
			collections = append(collections, homeRowCollectionJSON{ID: c.ID.String(), Name: c.Name, Source: c.Source})
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Catalogs    []homeRowCatalogJSON    `json:"catalogs"`
		Collections []homeRowCollectionJSON `json:"collections"`
	}{catalogs, collections})
}

// createHomeRow adds a row after the others: 201 with the row.
func (h *handler) createHomeRow(w http.ResponseWriter, r *http.Request) {
	var body homeRowJSON
	if !decode(w, r, &body) {
		return
	}
	row, ok := body.row()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_home_row")
		return
	}
	created, err := h.HomeRows.Create(r.Context(), row)
	if homeRowError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newHomeRowJSON(created))
}

// updateHomeRow changes a row.
func (h *handler) updateHomeRow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body homeRowJSON
	if !decode(w, r, &body) {
		return
	}
	row, ok := body.row()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_home_row")
		return
	}
	row.ID = id
	updated, err := h.HomeRows.Update(r.Context(), row)
	if homeRowError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newHomeRowJSON(updated))
}

// deleteHomeRow deletes a row: 204.
func (h *handler) deleteHomeRow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.HomeRows.Delete(r.Context(), id)
	if homeRowError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// orderHomeRows puts the rows in the order of the identifiers given; the
// others keep their order after them. It answers the rows.
func (h *handler) orderHomeRows(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	ids := make([]accounts.ID, 0, len(body.IDs))
	for _, raw := range body.IDs {
		id, err := accounts.ParseID(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		ids = append(ids, id)
	}
	if err := h.HomeRows.Order(r.Context(), ids); err != nil {
		h.internalError(w, r, err)
		return
	}
	h.homeRows(w, r)
}

// ownHomeRowJSON is a row shown to the user signed in, and whether they
// turned it off.
type ownHomeRowJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Place string `json:"place"`
	Off   bool   `json:"off"`
}

// ownHomeRows answers the rows shown to the user signed in, in order.
func (h *handler) ownHomeRows(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	result := []ownHomeRowJSON{}
	if h.HomeRows == nil {
		writeJSON(w, http.StatusOK, struct {
			Rows []ownHomeRowJSON `json:"rows"`
		}{result})
		return
	}
	rows, err := h.HomeRows.Rows(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	off, err := h.HomeRows.Off(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, row := range rows {
		if row.ShownTo(user.ID) {
			result = append(result, ownHomeRowJSON{ID: row.ID.String(), Name: row.Name, Kind: string(row.Kind), Place: string(row.Place),
				Off: off[row.ID]})
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Rows []ownHomeRowJSON `json:"rows"`
	}{result})
}

// setOwnHomeRow turns a row shown to the user signed in off, or on again:
// 204.
func (h *handler) setOwnHomeRow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Off bool `json:"off"`
	}
	if !decode(w, r, &body) {
		return
	}
	if h.HomeRows == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	err := h.HomeRows.SetOff(r.Context(), sessionFrom(r.Context()).User.ID, id, body.Off)
	if homeRowError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package admin

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
)

// HomeRowChoices tells the collections a home row may list, as the Jellyfin
// API describes them to user.
type HomeRowChoices interface {
	HomeRowCollections(ctx context.Context, user accounts.User) ([]jellyfin.HomeRowCollection, error)
}

// HomeRowPreviews tells the rows a user sees now on the web player's home
// page, each with its first titles, as the Jellyfin API answers them to
// the web player.
type HomeRowPreviews interface {
	PreviewHomeRows(ctx context.Context, user accounts.User) ([]jellyfin.HomeRowPreview, error)
}

// homeRowJSON is a row of the web player's home page (Settings › Web
// player › Home rows, see package homerows). addonId, catalogType and
// catalogId name a catalog row's catalog; collectionId a collection
// row's collection; count is how many rows a Because you watched row
// makes; users are those a row not shown to everyone is shown to;
// schedule is when it shows. shownNow, in answers only, tells whether the
// schedule shows it now.
type homeRowJSON struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Kind         string              `json:"kind"`
	Place        string              `json:"place"`
	AddonID      string              `json:"addonId"`
	CatalogType  string              `json:"catalogType"`
	CatalogID    string              `json:"catalogId"`
	CollectionID string              `json:"collectionId"`
	Count        int                 `json:"count"`
	Everyone     bool                `json:"everyone"`
	Users        []string            `json:"users"`
	Library      bool                `json:"library"`
	Schedule     homeRowScheduleJSON `json:"schedule"`
	ShownNow     bool                `json:"shownNow"`
}

// homeRowScheduleJSON is when a row shows, in the server's time zone
// (see homerows.Schedule): from startDate to endDate, YYYY-MM-DD, empty
// for no bound; on its days, 0 for Sunday to 6 for Saturday, none for
// every day; from startHour, 0 to 23, to endHour, 1 to 24, both null for
// all day.
type homeRowScheduleJSON struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Days      []int  `json:"days"`
	StartHour *int   `json:"startHour"`
	EndHour   *int   `json:"endHour"`
}

func newHomeRowJSON(r homerows.Row, now time.Time) homeRowJSON {
	result := homeRowJSON{ID: r.ID.String(), Name: r.Name, Kind: string(r.Kind), Place: string(r.Place), CatalogType: r.CatalogType,
		CatalogID: r.CatalogID, Count: r.Count, Everyone: r.Everyone, Users: []string{}, Library: r.Library,
		Schedule: homeRowScheduleJSON{Days: []int{}}, ShownNow: r.ShowsAt(now)}
	if r.Addon != (accounts.ID{}) {
		result.AddonID = r.Addon.String()
	}
	if r.Collection != (accounts.ID{}) {
		result.CollectionID = r.Collection.String()
	}
	for _, user := range r.Users {
		result.Users = append(result.Users, user.String())
	}
	s := r.Schedule
	if !s.Start.IsZero() {
		result.Schedule.StartDate = s.Start.Format(time.DateOnly)
	}
	if !s.End.IsZero() {
		result.Schedule.EndDate = s.End.Format(time.DateOnly)
	}
	for _, day := range s.Days {
		result.Schedule.Days = append(result.Schedule.Days, int(day))
	}
	if s.Hours != nil {
		result.Schedule.StartHour, result.Schedule.EndHour = &s.Hours.From, &s.Hours.To
	}
	return result
}

// schedule reads a schedule from its JSON, and reports whether it could:
// dates that are not ones, or a single hour, cannot be read. Days come in
// any order.
func (j homeRowScheduleJSON) schedule() (homerows.Schedule, bool) {
	var s homerows.Schedule
	ok := true
	date := func(raw string) time.Time {
		if raw == "" {
			return time.Time{}
		}
		t, err := time.Parse(time.DateOnly, raw)
		ok = ok && err == nil
		return t
	}
	s.Start, s.End = date(j.StartDate), date(j.EndDate)
	for _, day := range slices.Compact(slices.Sorted(slices.Values(j.Days))) {
		s.Days = append(s.Days, time.Weekday(day))
	}
	switch {
	case j.StartHour != nil && j.EndHour != nil:
		s.Hours = &homerows.Hours{From: *j.StartHour, To: *j.EndHour}
	case j.StartHour != nil || j.EndHour != nil:
		ok = false
	}
	return s, ok
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
	schedule, read := j.Schedule.schedule()
	r.Schedule = schedule
	return r, ok && read && r.Valid()
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
	now := h.now()
	for _, row := range rows {
		result = append(result, newHomeRowJSON(row, now))
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

// homeRowCatalogs lists the catalogs a row may list: those of the server's
// addons that can be browsed.
func (h *handler) homeRowCatalogs(ctx context.Context, user accounts.User) ([]library.SmartCatalog, error) {
	if h.SmartPreviews == nil {
		return nil, nil
	}
	found, err := h.SmartPreviews.SmartCatalogs(ctx, user, true)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(found, func(c library.SmartCatalog) bool { return !c.Shared }), nil
}

// offeredHomeRow reports whether row lists what homeRowChoices offers: for
// a catalog row, one of homeRowCatalogs. A row of another catalog would
// never be drawn.
func (h *handler) offeredHomeRow(ctx context.Context, user accounts.User, row homerows.Row) (bool, error) {
	if row.Kind != homerows.KindCatalog {
		return true, nil
	}
	catalogs, err := h.homeRowCatalogs(ctx, user)
	return slices.ContainsFunc(catalogs, func(c library.SmartCatalog) bool {
		return c.AddonID == row.Addon && c.Catalog.Type == row.CatalogType && c.Catalog.ID == row.CatalogID
	}), err
}

// homeRowChoices answers the catalogs and collections a row may list: the
// catalogs of the server's addons that can be browsed, and the
// collections the administrator sees.
func (h *handler) homeRowChoices(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	found, err := h.homeRowCatalogs(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	catalogs := make([]homeRowCatalogJSON, 0, len(found))
	for _, c := range found {
		catalogs = append(catalogs, homeRowCatalogJSON{AddonID: c.AddonID.String(), AddonName: c.AddonName, CatalogType: c.Catalog.Type,
			CatalogID: c.Catalog.ID, CatalogName: c.Catalog.Name})
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

// homeRowFrom reads the row of a request's body, and answers 400 when it
// is invalid or lists a catalog homeRowChoices does not offer. It reports
// whether it read one.
func (h *handler) homeRowFrom(w http.ResponseWriter, r *http.Request) (homerows.Row, bool) {
	var body homeRowJSON
	if !decode(w, r, &body) {
		return homerows.Row{}, false
	}
	row, ok := body.row()
	if ok {
		offered, err := h.offeredHomeRow(r.Context(), sessionFrom(r.Context()).User, row)
		if err != nil {
			h.internalError(w, r, err)
			return homerows.Row{}, false
		}
		ok = offered
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_home_row")
	}
	return row, ok
}

// createHomeRow adds a row after the others: 201 with the row.
func (h *handler) createHomeRow(w http.ResponseWriter, r *http.Request) {
	row, ok := h.homeRowFrom(w, r)
	if !ok {
		return
	}
	created, err := h.HomeRows.Create(r.Context(), row)
	if homeRowError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newHomeRowJSON(created, h.now()))
}

// updateHomeRow changes a row.
func (h *handler) updateHomeRow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	row, ok := h.homeRowFrom(w, r)
	if !ok {
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
	writeJSON(w, http.StatusOK, newHomeRowJSON(updated, h.now()))
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

// homeRowPreviewJSON is a row of a user's home page in the web player:
// its folder in Jellyfin apps (id), the row it comes from (rowId), its
// name there, its kind and place as the admin API names them, and its
// first titles.
type homeRowPreviewJSON struct {
	ID     string             `json:"id"`
	RowID  string             `json:"rowId"`
	Name   string             `json:"name"`
	Kind   string             `json:"kind"`
	Place  string             `json:"place"`
	Titles []homeRowTitleJSON `json:"titles"`
}

// homeRowTitleJSON is a title of a row's preview. posterId is the item
// whose Primary image is its poster: the series of an episode.
type homeRowTitleJSON struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Year     int    `json:"year"`
	PosterID string `json:"posterId"`
}

// previewHomeRows answers the rows the user of the path sees now on the
// web player's home page, in order, each with its first titles, as the web
// player signed in as them shows them (Preview as).
func (h *handler) previewHomeRows(w http.ResponseWriter, r *http.Request) {
	if h.HomeRowPreviews == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	rows, err := h.HomeRowPreviews.PreviewHomeRows(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]homeRowPreviewJSON, 0, len(rows))
	for _, row := range rows {
		preview := homeRowPreviewJSON{ID: row.ID.String(), RowID: row.Row.ID.String(), Name: row.Name, Kind: string(row.Row.Kind),
			Place: string(row.Row.Place), Titles: make([]homeRowTitleJSON, 0, len(row.Titles))}
		for _, title := range row.Titles {
			poster := title.ID
			if title.Kind == library.KindEpisode && title.SeriesID != (accounts.ID{}) {
				poster = title.SeriesID
			}
			preview.Titles = append(preview.Titles, homeRowTitleJSON{ID: title.ID.String(), Kind: string(title.Kind), Name: title.Name,
				Year: title.ProductionYear, PosterID: poster.String()})
		}
		result = append(result, preview)
	}
	writeJSON(w, http.StatusOK, struct {
		Rows []homeRowPreviewJSON `json:"rows"`
	}{result})
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

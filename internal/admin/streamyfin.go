package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/streamyfin"
)

// StreamyfinItems tells what the rows of Streamyfin's home screen can show:
// the server's libraries, and the collections of its collection libraries,
// as the administrator reaches them.
type StreamyfinItems interface {
	ServerLibraries(ctx context.Context) ([]library.ServerLibrary, error)
	Item(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error)
	Children(ctx context.Context, user accounts.User, parent accounts.ID, start, count int, genre string) (library.Page, error)
	Ancestors(ctx context.Context, user accounts.User, id accounts.ID) ([]library.Item, error)
}

// maxStreamyfinCollections bounds the collections listed for a library.
const maxStreamyfinCollections = 500

type streamyfinJSON struct {
	Rows      []streamyfinRowJSON     `json:"rows"`
	Libraries []streamyfinLibraryJSON `json:"libraries"`
}

// streamyfinRowJSON is a row: Name is the one Streamyfin shows, the title
// else DefaultName, the name of its library or collection, or the default
// name of the user's own lists; LibraryName is a collection's library.
// Available is false for a library or collection that is gone or that the
// administrator no longer reaches: the row is kept but shows nothing.
type streamyfinRowJSON struct {
	Kind        string  `json:"kind"`
	ItemID      *string `json:"itemId"`
	Title       *string `json:"title"`
	Name        string  `json:"name"`
	DefaultName string  `json:"defaultName"`
	LibraryName *string `json:"libraryName"`
	Available   bool    `json:"available"`
}

// streamyfinLibraryJSON is a library a row can show; Collections tells a
// collection library, whose collections a row can show too.
type streamyfinLibraryJSON struct {
	ItemID      string `json:"itemId"`
	Name        string `json:"name"`
	Collections bool   `json:"collections"`
}

type streamyfinItemJSON struct {
	ItemID string `json:"itemId"`
	Name   string `json:"name"`
}

// streamyfinKinds are the kinds of rows, as the admin app names them.
var streamyfinKinds = map[string]streamyfin.Kind{
	"resume": streamyfin.KindResume, "nextUp": streamyfin.KindNextUp,
	"library": streamyfin.KindLibrary, "collection": streamyfin.KindCollection,
}

func streamyfinKindName(kind streamyfin.Kind) string {
	for name, k := range streamyfinKinds {
		if k == kind {
			return name
		}
	}
	return string(kind)
}

// streamyfinLibraries lists the server's libraries a row can show, in
// order: those of movies, series and collections, not music.
func (h *handler) streamyfinLibraries(ctx context.Context) ([]library.ServerLibrary, error) {
	all, err := h.StreamyfinItems.ServerLibraries(ctx)
	if err != nil {
		return nil, err
	}
	var result []library.ServerLibrary
	for _, l := range all {
		switch l.CollectionType {
		case "movies", "tvshows", "boxsets":
			result = append(result, l)
		}
	}
	return result, nil
}

// streamyfinCollection returns a collection the administrator reaches, and
// the library it belongs to, one of libraries.
func (h *handler) streamyfinCollection(ctx context.Context, user accounts.User, id accounts.ID, libraries []library.ServerLibrary) (library.Item, library.ServerLibrary, bool) {
	item, err := h.StreamyfinItems.Item(ctx, user, id)
	if err != nil || item.Kind != library.KindCollection {
		return library.Item{}, library.ServerLibrary{}, false
	}
	ancestors, err := h.StreamyfinItems.Ancestors(ctx, user, id)
	if err != nil {
		return library.Item{}, library.ServerLibrary{}, false
	}
	for _, ancestor := range ancestors {
		for _, l := range libraries {
			if l.ID == ancestor.ID {
				return item, l, true
			}
		}
	}
	return library.Item{}, library.ServerLibrary{}, false
}

// streamyfinRows answers the rows and the libraries they can show.
func (h *handler) streamyfinRows(w http.ResponseWriter, r *http.Request) {
	if h.Streamyfin == nil || h.StreamyfinItems == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	rows, err := h.Streamyfin.Rows(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeStreamyfin(w, r, rows)
}

func (h *handler) writeStreamyfin(w http.ResponseWriter, r *http.Request, rows []streamyfin.Row) {
	ctx := r.Context()
	user := sessionFrom(ctx).User
	libraries, err := h.streamyfinLibraries(ctx)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	language := h.Accounts.Settings().Language
	result := streamyfinJSON{Rows: make([]streamyfinRowJSON, 0, len(rows)), Libraries: make([]streamyfinLibraryJSON, 0, len(libraries))}
	for _, l := range libraries {
		result.Libraries = append(result.Libraries, streamyfinLibraryJSON{ItemID: l.ID.String(), Name: l.Name, Collections: l.CollectionType == "boxsets"})
	}
	for _, row := range rows {
		entry := streamyfinRowJSON{Kind: streamyfinKindName(row.Kind), DefaultName: row.ItemName}
		switch row.Kind {
		case streamyfin.KindResume, streamyfin.KindNextUp:
			entry.DefaultName, entry.Available = streamyfin.DefaultName(row.Kind, language), true
		case streamyfin.KindLibrary:
			for _, l := range libraries {
				if l.ID == row.Item {
					entry.DefaultName, entry.Available = l.Name, true
				}
			}
		case streamyfin.KindCollection:
			if item, l, ok := h.streamyfinCollection(ctx, user, row.Item, libraries); ok {
				entry.DefaultName, entry.Available, entry.LibraryName = item.Name, true, new(l.Name)
			}
		}
		if row.Item != (accounts.ID{}) {
			entry.ItemID = new(row.Item.String())
		}
		entry.Name = entry.DefaultName
		if row.Title != "" {
			entry.Title, entry.Name = new(row.Title), row.Title
		}
		result.Rows = append(result.Rows, entry)
	}
	writeJSON(w, http.StatusOK, result)
}

// saveStreamyfinRows replaces the rows, in order: a library row shows one
// of streamyfinLibraries, a collection row a collection of one of them.
func (h *handler) saveStreamyfinRows(w http.ResponseWriter, r *http.Request) {
	if h.Streamyfin == nil || h.StreamyfinItems == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		Rows []struct {
			Kind   string  `json:"kind"`
			ItemID *string `json:"itemId"`
			Title  *string `json:"title"`
		} `json:"rows"`
	}
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	user := sessionFrom(ctx).User
	libraries, err := h.streamyfinLibraries(ctx)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	rows := make([]streamyfin.Row, 0, len(body.Rows))
	for _, sent := range body.Rows {
		kind, ok := streamyfinKinds[sent.Kind]
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_streamyfin_row")
			return
		}
		row := streamyfin.Row{Kind: kind}
		if sent.Title != nil {
			row.Title = *sent.Title
		}
		if sent.ItemID != nil {
			id, err := accounts.ParseID(*sent.ItemID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_streamyfin_row")
				return
			}
			row.Item = id
		}
		switch kind {
		case streamyfin.KindLibrary:
			found := false
			for _, l := range libraries {
				if l.ID == row.Item {
					row.ItemName, found = l.Name, true
				}
			}
			if !found {
				writeError(w, http.StatusBadRequest, "invalid_streamyfin_row")
				return
			}
		case streamyfin.KindCollection:
			item, _, ok := h.streamyfinCollection(ctx, user, row.Item, libraries)
			if !ok {
				writeError(w, http.StatusBadRequest, "invalid_streamyfin_row")
				return
			}
			row.ItemName = item.Name
		}
		rows = append(rows, row)
	}
	if err := h.Streamyfin.SetRows(ctx, rows); err != nil {
		switch {
		case errors.Is(err, streamyfin.ErrInvalidRow):
			writeError(w, http.StatusBadRequest, "invalid_streamyfin_row")
		case errors.Is(err, streamyfin.ErrTooManyRows):
			writeError(w, http.StatusBadRequest, "too_many_streamyfin_rows")
		default:
			h.internalError(w, r, err)
		}
		return
	}
	saved, err := h.Streamyfin.Rows(ctx)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeStreamyfin(w, r, saved)
}

// streamyfinCollections lists the collections of one of the collection
// libraries a row can show, as the administrator reaches them, at most
// maxStreamyfinCollections.
func (h *handler) streamyfinCollections(w http.ResponseWriter, r *http.Request) {
	if h.Streamyfin == nil || h.StreamyfinItems == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	id, err := accounts.ParseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	libraries, err := h.streamyfinLibraries(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	found := false
	for _, l := range libraries {
		found = found || l.ID == id && l.CollectionType == "boxsets"
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	page, err := h.StreamyfinItems.Children(r.Context(), sessionFrom(r.Context()).User, id, 0, maxStreamyfinCollections, "")
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]streamyfinItemJSON, 0, len(page.Items))
	for _, item := range page.Items {
		if item.Kind == library.KindCollection {
			result = append(result, streamyfinItemJSON{ItemID: item.ID.String(), Name: item.Name})
		}
	}
	writeJSON(w, http.StatusOK, result)
}

package admin

import (
	"context"
	"net/http"
)

// LibraryOrders tells how many users keep an order of libraries of their
// own, saved from a Jellyfin app such as jellyfin-web's Settings › Home,
// which wins over the order of Content › Libraries, and clears them all
// (see jellyfin.Handler.ClearLibraryOrders).
type LibraryOrders interface {
	LibraryOrders(ctx context.Context) (int, error)
	ClearLibraryOrders(ctx context.Context) (int, error)
}

// libraryOrdersJSON is how many users keep an order of their own.
type libraryOrdersJSON struct {
	Users int `json:"users"`
}

// libraryOrders answers how many users keep an order of libraries of
// their own, for Apply this order to every user to say before it acts.
func (h *handler) libraryOrders(w http.ResponseWriter, r *http.Request) {
	users := 0
	if h.LibraryOrders != nil {
		var err error
		if users, err = h.LibraryOrders.LibraryOrders(r.Context()); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, libraryOrdersJSON{Users: users})
}

// clearLibraryOrders is Apply this order to every user: it forgets the
// orders of libraries users saved, so that every user's apps list the
// libraries in the order of Content › Libraries, and answers how many
// users it changed. The activity log records it, with that count.
func (h *handler) clearLibraryOrders(w http.ResponseWriter, r *http.Request) {
	users := 0
	if h.LibraryOrders != nil {
		var err error
		users, err = h.LibraryOrders.ClearLibraryOrders(r.Context())
		if err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	h.Activity.LibraryOrderApplied(r.Context(), sessionFrom(r.Context()).User.Name, users)
	writeJSON(w, http.StatusOK, libraryOrdersJSON{Users: users})
}

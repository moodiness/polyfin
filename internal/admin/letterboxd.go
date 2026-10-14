package admin

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfinimport"
	"github.com/moodiness/polyfin/internal/letterboxd"
)

// A Letterboxd member's export file comes as a multipart form: the ZIP
// under file, beside kind, letterboxd, and the switches watchlist, ratings
// and likesAsFavorites. The ZIP is read in memory, never written to disk,
// and dropped once read: what the import needs of it goes with the run.
// The user imports it under My account, while the setting that lets users
// import their watch history allows it; an administrator, into a user's
// account, whatever the setting says.

// letterboxdKind is the kind of import of a Letterboxd export.
const letterboxdKind = "letterboxd"

// multipartForm reports whether r sends a multipart form.
func multipartForm(r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && media == "multipart/form-data"
}

// letterboxdForm reads the export and the switches r sends, or answers why
// it could not: invalid_request for a form without the file or of another
// kind, letterboxd_export_too_large for a file over the bound or naming
// more than letterboxd.MaxMovies movies, and letterboxd_export_invalid for
// a file that is not an export. A switch not sent keeps its default: the
// watchlist and the ratings on, the movies liked off.
func letterboxdForm(w http.ResponseWriter, r *http.Request) (letterboxd.Export, letterboxd.Choice, bool) {
	choice := letterboxd.Choice{Watchlist: true, Ratings: true}
	if !multipartForm(r) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return letterboxd.Export{}, choice, false
	}
	// The other fields and the form's own lines take little room.
	r.Body = http.MaxBytesReader(w, r.Body, letterboxd.MaxExportBytes+64<<10)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return letterboxd.Export{}, choice, false
	}
	var kind string
	var file []byte
	sent := false
	failed := func(err error) (letterboxd.Export, letterboxd.Choice, bool) {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			writeError(w, http.StatusRequestEntityTooLarge, "letterboxd_export_too_large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request")
		}
		return letterboxd.Export{}, choice, false
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return failed(err)
		}
		name := part.FormName()
		if name == "file" {
			file, err = io.ReadAll(io.LimitReader(part, letterboxd.MaxExportBytes+1))
			if err != nil {
				return failed(err)
			}
			if len(file) > letterboxd.MaxExportBytes {
				writeError(w, http.StatusRequestEntityTooLarge, "letterboxd_export_too_large")
				return letterboxd.Export{}, choice, false
			}
			sent = true
			continue
		}
		value, err := io.ReadAll(io.LimitReader(part, 64))
		if err != nil {
			return failed(err)
		}
		text := strings.TrimSpace(string(value))
		var on *bool
		switch name {
		case "kind":
			kind = text
		case "watchlist":
			on = &choice.Watchlist
		case "ratings":
			on = &choice.Ratings
		case "likesAsFavorites":
			on = &choice.LikesAsFavorites
		}
		if on != nil {
			if *on, err = strconv.ParseBool(text); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request")
				return letterboxd.Export{}, choice, false
			}
		}
	}
	if kind != letterboxdKind || !sent {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return letterboxd.Export{}, choice, false
	}
	export, err := letterboxd.Parse(file)
	if errors.Is(err, letterboxd.ErrTooManyMovies) {
		writeError(w, http.StatusRequestEntityTooLarge, "letterboxd_export_too_large")
		return letterboxd.Export{}, choice, false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "letterboxd_export_invalid")
		return letterboxd.Export{}, choice, false
	}
	return export, choice, true
}

// startLetterboxd imports the export r sends into user's account, which
// by starts, in the background, and answers it. One import into an
// account runs at a time: while the user's own import from a server, or
// their Letterboxd import, runs, or another export for the account is
// being received, it answers 409 server_import_running before reading the
// file.
func (h *handler) startLetterboxd(w http.ResponseWriter, r *http.Request, by, user accounts.User) {
	if h.ownImportRunning(user.ID) {
		writeError(w, http.StatusConflict, "server_import_running")
		return
	}
	release, err := h.Letterboxd.Reserve(user.ID)
	if err != nil {
		writeError(w, http.StatusConflict, "server_import_running")
		return
	}
	defer release()
	export, choice, ok := letterboxdForm(w, r)
	if !ok {
		return
	}
	status, err := h.Letterboxd.Start(by, user, export, choice)
	if errors.Is(err, letterboxd.ErrRunning) {
		writeError(w, http.StatusConflict, "server_import_running")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Import *letterboxdImportJSON `json:"import"`
	}{newLetterboxdImportJSON(status)})
}

// ownImportRunning reports whether an import into user's account from
// another server, of their own, or from Letterboxd runs.
func (h *handler) ownImportRunning(user accounts.ID) bool {
	if h.Letterboxd != nil && h.Letterboxd.Running(user) {
		return true
	}
	if h.JellyfinImport != nil {
		if own := h.JellyfinImport.Own(user); own != nil && own.State == jellyfinimport.StateRunning {
			return true
		}
	}
	return false
}

// letterboxdSearchable reports whether the server's metadata addons search
// movies, without which no movie of an export is found; false when that
// could not be read.
func (h *handler) letterboxdSearchable(r *http.Request) bool {
	searchable, err := h.Letterboxd.Searchable(r.Context())
	if err != nil {
		h.Logger.Warn("The addons searching movies could not be read", "error", err)
	}
	return searchable
}

// letterboxdUser is the user the path names, for an administrator's
// import, or false once it answered why not.
func (h *handler) letterboxdUser(w http.ResponseWriter, r *http.Request) (accounts.User, bool) {
	if h.Letterboxd == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return accounts.User{}, false
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return accounts.User{}, false
	}
	user, err := h.Accounts.User(r.Context(), id)
	if accountError(w, err) {
		return accounts.User{}, false
	}
	if err != nil {
		h.internalError(w, r, err)
		return accounts.User{}, false
	}
	return user, true
}

// userLetterboxdImport answers, to an administrator, whether the server's
// metadata addons search movies, and the Letterboxd import into the
// account of the user the path names running, else the last one, null
// when none.
func (h *handler) userLetterboxdImport(w http.ResponseWriter, r *http.Request) {
	user, ok := h.letterboxdUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Searchable bool                  `json:"searchable"`
		Import     *letterboxdImportJSON `json:"import"`
	}{h.letterboxdSearchable(r), newLetterboxdImportJSON(h.Letterboxd.Current(user.ID))})
}

// startUserLetterboxdImport imports, for an administrator, the export sent
// into the account of the user the path names.
func (h *handler) startUserLetterboxdImport(w http.ResponseWriter, r *http.Request) {
	user, ok := h.letterboxdUser(w, r)
	if !ok {
		return
	}
	h.startLetterboxd(w, r, sessionFrom(r.Context()).User, user)
}

// letterboxdImportJSON is how an import of a Letterboxd export goes, or
// went: how many of its movies were looked up, then what it added, and
// the movies not found.
type letterboxdImportJSON struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	User struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	StartedBy struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"startedBy"`
	Watchlist        bool                      `json:"watchlist"`
	Ratings          bool                      `json:"ratings"`
	LikesAsFavorites bool                      `json:"likesAsFavorites"`
	State            string                    `json:"state"`
	Problem          *string                   `json:"problem"`
	StartedAt        time.Time                 `json:"startedAt"`
	EndedAt          *time.Time                `json:"endedAt"`
	Movies           int                       `json:"movies"`
	Searched         int                       `json:"searched"`
	Added            letterboxdAddedJSON       `json:"added"`
	UnmatchedCount   int                       `json:"unmatchedCount"`
	Unmatched        []letterboxdUnmatchedJSON `json:"unmatched"`
}

// letterboxdAddedJSON counts what an import added: movies marked played,
// Watchlist titles, ratings and favorites.
type letterboxdAddedJSON struct {
	Played    int `json:"played"`
	Watchlist int `json:"watchlist"`
	Ratings   int `json:"ratings"`
	Favorites int `json:"favorites"`
}

// letterboxdUnmatchedJSON is a movie of the export no title matched: its
// name, its year (null when the export gives none), its Letterboxd link
// (null when it gives none), and why.
type letterboxdUnmatchedJSON struct {
	Name   string  `json:"name"`
	Year   *int    `json:"year"`
	Link   *string `json:"link"`
	Reason string  `json:"reason"`
}

func newLetterboxdImportJSON(status *letterboxd.Status) *letterboxdImportJSON {
	if status == nil {
		return nil
	}
	result := &letterboxdImportJSON{ID: status.ID, Kind: letterboxdKind, Watchlist: status.Choice.Watchlist,
		Ratings: status.Choice.Ratings, LikesAsFavorites: status.Choice.LikesAsFavorites, State: status.State,
		Problem: optional(status.Problem), StartedAt: status.StartedAt.UTC().Truncate(time.Second), EndedAt: utcSeconds(status.EndedAt),
		Movies: status.Movies, Searched: status.Searched, UnmatchedCount: status.UnmatchedCount,
		Added: letterboxdAddedJSON{Played: status.Played, Watchlist: status.Watchlist, Ratings: status.Ratings,
			Favorites: status.Favorites},
		Unmatched: make([]letterboxdUnmatchedJSON, 0, len(status.Unmatched))}
	result.User.ID, result.User.Name = status.User.String(), status.UserName
	result.StartedBy.ID, result.StartedBy.Name = status.StartedBy.String(), status.StartedByName
	for _, u := range status.Unmatched {
		entry := letterboxdUnmatchedJSON{Name: u.Name, Link: optional(u.Link), Reason: u.Reason}
		if u.Year > 0 {
			entry.Year = &u.Year
		}
		result.Unmatched = append(result.Unmatched, entry)
	}
	return result
}

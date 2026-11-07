package admin

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/importlinks"
)

// importTitleSource is what a source is named by: imports name theirs in
// lowercase letters only.
var importTitleSource = regexp.MustCompile(`^[a-z]+$`)

// importTitlesTarget finds the account whose kept titles a request reads
// or links, or answers why there is none.
type importTitlesTarget func(w http.ResponseWriter, r *http.Request) (accounts.User, bool)

// ownImportTitlesTarget is the caller's account. Unlike the imports
// themselves, the lists stay open with the server imports setting off: a
// user links what an earlier import kept.
func (h *handler) ownImportTitlesTarget(w http.ResponseWriter, r *http.Request) (accounts.User, bool) {
	if h.ImportTitles == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return accounts.User{}, false
	}
	return sessionFrom(r.Context()).User, true
}

// userImportTitlesTarget is the account of the user the path names.
func (h *handler) userImportTitlesTarget(w http.ResponseWriter, r *http.Request) (accounts.User, bool) {
	if h.ImportTitles == nil {
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

// importTitleSourceOf is the source the path names, or false, answered,
// when it names none an import could have.
func importTitleSourceOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	source := r.PathValue("source")
	if !importTitleSource.MatchString(source) {
		writeError(w, http.StatusNotFound, "not_found")
		return "", false
	}
	return source, true
}

type importTitleSourceJSON struct {
	Source   string `json:"source"`
	NotFound int    `json:"notFound"`
	Linked   int    `json:"linked"`
}

type importTitleLinkedByJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type importTitleLinkJSON struct {
	Kind     string                   `json:"kind"`
	IMDb     string                   `json:"imdb"`
	TMDB     int                      `json:"tmdb"`
	Name     string                   `json:"name"`
	Year     *int                     `json:"year"`
	LinkedAt time.Time                `json:"linkedAt"`
	LinkedBy *importTitleLinkedByJSON `json:"linkedBy"`
}

type importTitleJSON struct {
	Key     string               `json:"key"`
	Kind    string               `json:"kind"`
	Name    string               `json:"name"`
	Year    *int                 `json:"year"`
	IDs     string               `json:"ids"`
	Season  int                  `json:"season"`
	Episode int                  `json:"episode"`
	Reason  string               `json:"reason"`
	Link    *importTitleLinkJSON `json:"link"`
}

// knownYear is year, nil when unknown.
func knownYear(year int) *int {
	if year <= 0 {
		return nil
	}
	return &year
}

func newImportTitleLinkJSON(link importlinks.Link) *importTitleLinkJSON {
	out := &importTitleLinkJSON{Kind: link.Kind, IMDb: link.IMDb, TMDB: link.TMDB, Name: link.Name, Year: knownYear(link.Year),
		LinkedAt: link.LinkedAt.UTC()}
	if link.LinkedBy != nil {
		out.LinkedBy = &importTitleLinkedByJSON{ID: link.LinkedBy.String(), Name: link.LinkedByName}
	}
	return out
}

// importTitleSources answers the sources of the account's kept titles,
// with how many are not linked and how many are.
func (h *handler) importTitleSources(target importTitlesTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := target(w, r)
		if !ok {
			return
		}
		counts, err := h.ImportTitles.Counts(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		sources := make([]importTitleSourceJSON, 0, len(counts))
		for _, c := range counts {
			sources = append(sources, importTitleSourceJSON{Source: c.Source, NotFound: c.NotFound, Linked: c.Linked})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
	}
}

// importTitles answers the titles the account's imports of a source did
// not find, with their links.
func (h *handler) importTitles(target importTitlesTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := target(w, r)
		if !ok {
			return
		}
		source, ok := importTitleSourceOf(w, r)
		if !ok {
			return
		}
		rows, err := h.ImportTitles.List(r.Context(), user.ID, source)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		titles := make([]importTitleJSON, 0, len(rows))
		for _, row := range rows {
			title := importTitleJSON{Key: row.Key, Kind: row.Kind, Name: row.Name, Year: knownYear(row.Year), IDs: row.IDs,
				Season: row.Season, Episode: row.Episode, Reason: row.Reason}
			if row.Link != nil {
				title.Link = newImportTitleLinkJSON(*row.Link)
			}
			titles = append(titles, title)
		}
		writeJSON(w, http.StatusOK, map[string]any{"titles": titles})
	}
}

// importTitleError answers the error of a link, and reports whether err
// was one the store names.
func importTitleError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{importlinks.ErrUnknownEntry, http.StatusNotFound, "not_found"},
		{importlinks.ErrInvalidTarget, http.StatusBadRequest, "invalid_target"},
		{importlinks.ErrKindMismatch, http.StatusBadRequest, "kind_mismatch"},
		{importlinks.ErrTargetNotFound, http.StatusNotFound, "target_not_found"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// linkImportTitle links a title the account's imports of a source did not
// find to the IMDb or TMDB title the caller names, which applies at once.
func (h *handler) linkImportTitle(target importTitlesTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := target(w, r)
		if !ok {
			return
		}
		source, ok := importTitleSourceOf(w, r)
		if !ok {
			return
		}
		var body struct {
			Key    string `json:"key"`
			Target string `json:"target"`
		}
		if !decode(w, r, &body) {
			return
		}
		by := sessionFrom(r.Context()).User.ID
		link, err := h.ImportTitles.Link(r.Context(), by, user.ID, source, body.Key, body.Target)
		if importTitleError(w, err) {
			return
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"link": newImportTitleLinkJSON(link)})
	}
}

// unlinkImportTitle removes the link of a title: later imports no longer
// apply it.
func (h *handler) unlinkImportTitle(target importTitlesTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := target(w, r)
		if !ok {
			return
		}
		source, ok := importTitleSourceOf(w, r)
		if !ok {
			return
		}
		var body struct {
			Key string `json:"key"`
		}
		if !decode(w, r, &body) {
			return
		}
		err := h.ImportTitles.Unlink(r.Context(), user.ID, source, body.Key)
		if importTitleError(w, err) {
			return
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

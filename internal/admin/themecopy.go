package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/themecopy"
)

// themeCopyJSON is the copy of the theme's files Polyfin keeps, as the
// admin API sends it: null for none, then the files missing from its
// folder, and the addresses the Custom texts load from elsewhere.
type themeCopyJSON struct {
	Copy    *themeCopyFilesJSON `json:"copy"`
	Missing []string            `json:"missing"`
	Outside []string            `json:"outside"`
	Limits  themeCopyLimitsJSON `json:"limits"`
}

type themeCopyFilesJSON struct {
	SavedAt time.Time           `json:"savedAt"`
	Size    int64               `json:"size"`
	Files   []themecopy.File    `json:"files"`
	Unsaved []themecopy.Unsaved `json:"unsaved"`
}

type themeCopyLimitsJSON struct {
	FileBytes  int `json:"fileBytes"`
	TotalBytes int `json:"totalBytes"`
	Files      int `json:"files"`
	Depth      int `json:"depth"`
}

func newThemeCopyJSON(status themecopy.Status) themeCopyJSON {
	answer := themeCopyJSON{Missing: status.Missing, Outside: status.Outside, Limits: themeCopyLimitsJSON{
		FileBytes: themecopy.MaxFileBytes, TotalBytes: themecopy.MaxTotalBytes, Files: themecopy.MaxFiles, Depth: themecopy.MaxDepth,
	}}
	if kept := status.Copy; kept != nil {
		answer.Copy = &themeCopyFilesJSON{SavedAt: kept.SavedAt, Size: kept.Size(), Files: kept.Files, Unsaved: kept.Unsaved}
		if answer.Copy.Unsaved == nil {
			answer.Copy.Unsaved = []themecopy.Unsaved{}
		}
	}
	return answer
}

// themeCopy answers the copy kept of the theme's files.
func (h *handler) themeCopy(w http.ResponseWriter, r *http.Request) {
	if h.ThemeCopier == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	status, err := h.ThemeCopier.Status(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newThemeCopyJSON(status))
}

// keepThemeCopy keeps a copy of the files the Custom texts load from
// elsewhere, or makes the copy kept again, and answers it. 409
// theme_copy_running: a copy is being made or removed; 400
// theme_copy_nothing: the texts load nothing from elsewhere; 422
// theme_copy_failed, with unsaved: no file could be saved, and no copy was
// made; 409 custom_texts_changed: the texts changed while the files were
// downloaded, and the copy was dropped.
func (h *handler) keepThemeCopy(w http.ResponseWriter, r *http.Request) {
	if h.ThemeCopier == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	status, err := h.ThemeCopier.Keep(r.Context())
	if h.themeCopyFailed(w, r, err) {
		return
	}
	h.Activity.SettingsSaved(r.Context(), sessionFrom(r.Context()).User.Name)
	writeJSON(w, http.StatusOK, newThemeCopyJSON(status))
}

// removeThemeCopy removes the copy kept: the Custom texts name again the
// addresses its files came from. 404 not_found: no copy is kept.
func (h *handler) removeThemeCopy(w http.ResponseWriter, r *http.Request) {
	if h.ThemeCopier == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	status, err := h.ThemeCopier.Remove(r.Context())
	if h.themeCopyFailed(w, r, err) {
		return
	}
	h.Activity.SettingsSaved(r.Context(), sessionFrom(r.Context()).User.Name)
	writeJSON(w, http.StatusOK, newThemeCopyJSON(status))
}

// themeCopyFailed answers err, when there is one, and reports whether
// there was.
func (h *handler) themeCopyFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	var nothingSaved *themecopy.NothingSavedError
	switch {
	case err == nil:
		return false
	case errors.As(err, &nothingSaved):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "theme_copy_failed", "unsaved": nothingSaved.Unsaved})
	case errors.Is(err, themecopy.ErrRunning):
		writeError(w, http.StatusConflict, "theme_copy_running")
	case errors.Is(err, themecopy.ErrNothingToSave):
		writeError(w, http.StatusBadRequest, "theme_copy_nothing")
	case errors.Is(err, themecopy.ErrNoCopy):
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, accounts.ErrCustomTextsChanged):
		writeError(w, http.StatusConflict, "custom_texts_changed")
	case errors.Is(err, accounts.ErrInvalidCustomCss):
		writeError(w, http.StatusBadRequest, "invalid_custom_css")
	case errors.Is(err, accounts.ErrInvalidCustomJs):
		writeError(w, http.StatusBadRequest, "invalid_custom_js")
	default:
		h.internalError(w, r, err)
	}
	return true
}

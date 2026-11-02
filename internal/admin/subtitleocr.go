package admin

import (
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/ocr"
)

// downloadLanguage downloads the data of a language Tesseract lacks into
// POLYFIN_TESSDATA_DIR, as the Download button of its Health warning
// asks: 204 once it is there, the tracks waiting for it then read. A
// language with no data to download is 404; a download that cannot be
// kept names its cause in detail: no folder set, a folder that cannot be
// written, a download that failed or a file whose SHA-256 is not the one
// listed.
func (h *handler) downloadLanguage(w http.ResponseWriter, r *http.Request) {
	if h.Health.DownloadLanguage == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	err := h.Health.DownloadLanguage(r.Context(), r.PathValue("language"))
	var folder *ocr.FolderError
	var fetch *ocr.FetchError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ocr.ErrUnknownLanguage):
		writeError(w, http.StatusNotFound, "tessdata_unknown_language")
	case errors.Is(err, ocr.ErrNoDataDir):
		writeError(w, http.StatusConflict, "tessdata_no_folder")
	case errors.As(err, &folder):
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tessdata_folder", "detail": folder.Error()})
	case errors.Is(err, ocr.ErrChecksum):
		writeError(w, http.StatusBadGateway, "tessdata_checksum")
	case errors.As(err, &fetch):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "tessdata_download", "detail": fetch.Error()})
	default:
		h.internalError(w, r, err)
	}
}

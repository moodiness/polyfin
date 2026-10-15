package admin

import (
	"net/http"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
)

// metadataLanguages lists the languages of the server's addons, which
// users choose their metadata language among, for any signed-in user.
func (h *handler) metadataLanguages(w http.ResponseWriter, r *http.Request) {
	languages, err := h.Addons.Languages(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"languages": languages})
}

// metadataLanguageOffered reports whether a user may choose language as
// their metadata language: empty, for the server's descriptions, or one
// of the server's addons' languages. It answers the request otherwise.
func (h *handler) metadataLanguageOffered(w http.ResponseWriter, r *http.Request, language string) bool {
	if language == "" {
		return true
	}
	languages, err := h.Addons.Languages(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return false
	}
	if !slices.Contains(languages, language) {
		writeError(w, http.StatusBadRequest, "invalid_language")
		return false
	}
	return true
}

// ownMetadataLanguage answers the caller's metadata language, with the
// languages they may choose.
func (h *handler) ownMetadataLanguage(w http.ResponseWriter, r *http.Request) {
	user, err := h.Accounts.User(r.Context(), sessionFrom(r.Context()).User.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeMetadataLanguage(w, r, user)
}

// setOwnMetadataLanguage changes the caller's metadata language.
func (h *handler) setOwnMetadataLanguage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Language *string `json:"language"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Language == nil {
		writeError(w, http.StatusBadRequest, "invalid_language")
		return
	}
	if !h.metadataLanguageOffered(w, r, *body.Language) {
		return
	}
	session := sessionFrom(r.Context())
	user, err := h.Accounts.UpdateUser(r.Context(), session.User.ID, accounts.UserChanges{MetadataLanguage: body.Language}, session.TokenHash)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeMetadataLanguage(w, r, user)
}

func (h *handler) writeMetadataLanguage(w http.ResponseWriter, r *http.Request, user accounts.User) {
	languages, err := h.Addons.Languages(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"language": user.MetadataLanguage, "languages": languages})
}

package admin

import (
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/forwarded"
)

// setupCodeHeader carries the setup code of a backup sent from the setup
// page, whose body is the backup.
const setupCodeHeader = "X-Polyfin-Setup-Code"

// setupGate answers, and reports false, unless the setup page may restore
// a backup: no administrator exists yet, code is the setup code, and
// Polyfin makes backups. A wrong code counts as a failed setup does, the
// first administrator's creation and a restore sharing the same limit.
func (h *handler) setupGate(w http.ResponseWriter, r *http.Request, code string) bool {
	attempt := h.reserve(w, forwarded.Client(r))
	if attempt == nil {
		return false
	}
	defer attempt.Cancel()
	if required, err := h.Accounts.SetupRequired(r.Context()); err != nil {
		h.internalError(w, r, err)
		return false
	} else if !required {
		writeError(w, http.StatusConflict, "setup_complete")
		return false
	}
	if !setupCodeMatches(h.SetupCode, code) {
		attempt.Fail()
		writeError(w, http.StatusBadRequest, "invalid_setup_code")
		return false
	}
	attempt.Succeed()
	if h.Backups == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return false
	}
	return true
}

// setupRestoreUpload takes a backup sent from the setup page as the
// request's body, named by the name query parameter, the setup code in
// setupCodeHeader, checked before the body is read: the plan, locked when
// the backup is encrypted (see setupRestoreUnlock).
func (h *handler) setupRestoreUpload(w http.ResponseWriter, r *http.Request) {
	if !h.setupGate(w, r, r.Header.Get(setupCodeHeader)) {
		return
	}
	plan, err := h.Backups.PrepareSetupUpload(r.Context(), r.URL.Query().Get("name"), http.MaxBytesReader(w, r.Body, maxUpload))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	h.answerPlan(w, r, plan, err)
}

// setupRestoreUnlock opens the encrypted backup sent from the setup page
// with its passphrase, as unlockRestore does.
func (h *handler) setupRestoreUnlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SetupCode  string `json:"setupCode"`
		ID         string `json:"id"`
		Passphrase string `json:"passphrase"`
	}
	if !decode(w, r, &body) || !h.setupGate(w, r, body.SetupCode) {
		return
	}
	plan, err := h.Backups.Unlock(r.Context(), body.ID, body.Passphrase)
	h.answerUnlocked(w, r, plan, err)
}

// setupRestoreDiscard drops the backup sent from the setup page, with its
// file, when the page goes back: 204.
func (h *handler) setupRestoreDiscard(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SetupCode string `json:"setupCode"`
		ID        string `json:"id"`
	}
	if !decode(w, r, &body) || !h.setupGate(w, r, body.SetupCode) {
		return
	}
	h.Backups.Discard(body.ID)
	w.WriteHeader(http.StatusNoContent)
}

// setupRestoreStart restores the backup sent from the setup page: 202,
// then Polyfin stops, restores it, and starts again on it, as a restore
// from the admin app does, without backing up first the database, which
// holds no administrator. anyway goes ahead when the backup holds keys
// POLYFIN_SECRET_KEY cannot decrypt.
func (h *handler) setupRestoreStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SetupCode string `json:"setupCode"`
		ID        string `json:"id"`
		Anyway    bool   `json:"anyway"`
	}
	if !decode(w, r, &body) || !h.setupGate(w, r, body.SetupCode) {
		return
	}
	err := h.Backups.StartSetup(r.Context(), body.ID, body.Anyway)
	if backupError(w, err) {
		return
	} else if err != nil {
		h.Logger.Warn("A restore from the setup page could not start", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "restore_failed", "message": err.Error()})
		return
	}
	h.Logger.Info("A backup is restored from the setup page")
	writeJSON(w, http.StatusAccepted, map[string]bool{"restoring": true})
}

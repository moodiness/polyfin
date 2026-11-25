package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/moonfin"
)

// Moonfin's companion (see package moonfin): the administrator's defaults
// for the settings Moonfin's apps sync, and what each user's apps synced,
// on their page and on My account.

// moonfinSyncJSON tells what a user's synced settings hold: whether they
// have any, how many settings over which profiles, and when and by which
// app they were last saved, null and empty when never.
type moonfinSyncJSON struct {
	Synced    bool       `json:"synced"`
	Settings  int        `json:"settings"`
	Profiles  []string   `json:"profiles"`
	UpdatedAt *time.Time `json:"updatedAt"`
	UpdatedBy string     `json:"updatedBy"`
}

func newMoonfinSyncJSON(sync moonfin.Sync) moonfinSyncJSON {
	profiles := sync.Profiles
	if profiles == nil {
		profiles = []string{}
	}
	return moonfinSyncJSON{Synced: sync.Synced, Settings: sync.Settings, Profiles: profiles, UpdatedAt: sync.UpdatedAt, UpdatedBy: sync.UpdatedBy}
}

// moonfinOffered reports whether Polyfin keeps Moonfin's settings, and
// answers 404 when it does not.
func (h *handler) moonfinOffered(w http.ResponseWriter) bool {
	if h.Moonfin == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return false
	}
	return true
}

// moonfinStoreError answers the store's errors the admin app tells apart,
// and reports whether err was one of them.
func moonfinStoreError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, moonfin.ErrNotFound):
		writeError(w, http.StatusNotFound, "moonfin_settings_not_found")
	case errors.Is(err, moonfin.ErrInvalidProfile):
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, moonfin.ErrTooLarge):
		writeError(w, http.StatusBadRequest, "invalid_moonfin_defaults")
	default:
		return false
	}
	return true
}

// writeMoonfinSync answers what the synced settings of user hold.
func (h *handler) writeMoonfinSync(w http.ResponseWriter, r *http.Request, user accounts.ID) {
	sync, err := h.Moonfin.SyncOf(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newMoonfinSyncJSON(sync))
}

// userMoonfin answers what the synced settings of the user the path names
// hold.
func (h *handler) userMoonfin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok || !h.moonfinOffered(w) {
		return
	}
	if _, err := h.Accounts.User(r.Context(), id); errors.Is(err, accounts.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeMoonfinSync(w, r, id)
}

// deleteUserMoonfin forgets the synced settings of the user the path
// names: their apps then start over from the defaults, or from their own.
func (h *handler) deleteUserMoonfin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok || !h.moonfinOffered(w) {
		return
	}
	if err := h.Moonfin.Delete(r.Context(), id); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ownMoonfinJSON is the caller's Moonfin: their choice, whether the
// administrator keeps them from changing it, the server's default, and
// what their synced settings hold.
type ownMoonfinJSON struct {
	Moonfin       string          `json:"moonfin"`
	Locked        bool            `json:"locked"`
	ServerDefault bool            `json:"serverDefault"`
	Sync          moonfinSyncJSON `json:"sync"`
}

func (h *handler) writeOwnMoonfin(w http.ResponseWriter, r *http.Request, user accounts.User) {
	sync, err := h.Moonfin.SyncOf(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ownMoonfinJSON{Moonfin: user.Moonfin, Locked: user.MoonfinLocked,
		ServerDefault: h.Accounts.Settings().Moonfin, Sync: newMoonfinSyncJSON(sync)})
}

// ownMoonfin answers the caller's Moonfin.
func (h *handler) ownMoonfin(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	user, err := h.Accounts.User(r.Context(), sessionFrom(r.Context()).User.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeOwnMoonfin(w, r, user)
}

// setOwnMoonfin changes the caller's Moonfin, unless the administrator
// keeps them from it.
func (h *handler) setOwnMoonfin(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	var body struct {
		Moonfin *string `json:"moonfin"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Moonfin == nil {
		writeError(w, http.StatusBadRequest, "invalid_moonfin")
		return
	}
	user, err := h.Accounts.SetOwnMoonfin(r.Context(), sessionFrom(r.Context()).User.ID, *body.Moonfin)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeOwnMoonfin(w, r, user)
}

// deleteOwnMoonfinSettings forgets the caller's synced settings.
func (h *handler) deleteOwnMoonfinSettings(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	if err := h.Moonfin.Delete(r.Context(), sessionFrom(r.Context()).User.ID); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// moonfinSyncedUserJSON is a user whose apps synced settings, whose
// profiles the administrator may take the defaults from.
type moonfinSyncedUserJSON struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Sync moonfinSyncJSON `json:"sync"`
}

// moonfinDefaults answers the administrator's defaults, by profile, and
// the users whose apps synced settings, by name.
func (h *handler) moonfinDefaults(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	h.writeMoonfinDefaults(w, r)
}

func (h *handler) writeMoonfinDefaults(w http.ResponseWriter, r *http.Request) {
	defaults, err := h.Moonfin.Defaults(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	synced, err := h.Moonfin.Synced(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	listed := []moonfinSyncedUserJSON{}
	for _, user := range users {
		if sync, ok := synced[user.ID]; ok {
			listed = append(listed, moonfinSyncedUserJSON{ID: user.ID.String(), Name: user.Name, Sync: newMoonfinSyncJSON(sync)})
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Profiles map[string]moonfin.Settings `json:"profiles"`
		Users    []moonfinSyncedUserJSON     `json:"users"`
	}{defaults, listed})
}

// setMoonfinDefaults replaces the administrator's defaults for the profile
// the path names with the JSON object of the body, up to
// moonfin.MaxDefaultsBytes; a user's own API keys and hidden titles are
// left out.
func (h *handler) setMoonfinDefaults(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	profile, ok := moonfin.ProfileName(r.PathValue("profile"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var settings moonfin.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, moonfin.MaxDefaultsBytes)).Decode(&settings); err != nil || settings == nil {
		writeError(w, http.StatusBadRequest, "invalid_moonfin_defaults")
		return
	}
	err := h.Moonfin.SetDefaults(r.Context(), profile, settings)
	if moonfinStoreError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeMoonfinDefaults(w, r)
}

// copyMoonfinDefaults makes a profile of a user's synced settings the
// defaults for the profile the path names.
func (h *handler) copyMoonfinDefaults(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	profile, ok := moonfin.ProfileName(r.PathValue("profile"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		UserID  string `json:"userId"`
		Profile string `json:"profile"`
	}
	if !decode(w, r, &body) {
		return
	}
	user, err := accounts.ParseID(body.UserID)
	from, known := moonfin.ProfileName(body.Profile)
	if err != nil || !known {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	err = h.Moonfin.CopyToDefaults(r.Context(), user, from, profile)
	if moonfinStoreError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeMoonfinDefaults(w, r)
}

// applyMoonfinDefaults writes the defaults into the synced settings of
// every user who has some, or with reset makes them start over from the
// defaults, and answers how many users there were.
func (h *handler) applyMoonfinDefaults(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	var body struct {
		Reset bool `json:"reset"`
	}
	if !decode(w, r, &body) {
		return
	}
	count, err := h.Moonfin.Apply(r.Context(), body.Reset)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Users int `json:"users"`
	}{count})
}

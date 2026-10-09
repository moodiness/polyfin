package admin

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/trackers"
)

// serverSecrets are the server's secrets an administrator can read again,
// by the names the settings JSON gives them.
var serverSecrets = map[string]func(accounts.Settings) string{
	"publicMetaDbKey":   func(s accounts.Settings) string { return s.PublicMetaDBKey },
	"theIntroDbKey":     func(s accounts.Settings) string { return s.TheIntroDBKey },
	"traktClientSecret": func(s accounts.Settings) string { return s.TraktClientSecret },
	"lastFmSecret":      func(s accounts.Settings) string { return s.LastFMSecret },
}

// secretJSON is a secret revealed on demand. Answers never cache it.
type secretJSON struct {
	Value string `json:"value"`
}

// revealServerSecret answers one of the server's saved secrets, for an
// administrator to read it again, and records who did in the activity log.
// 404 not_found names no such secret, 404 not_set one not saved, or one
// the key cannot decrypt.
func (h *handler) revealServerSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	read, ok := serverSecrets[name]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	value := read(h.Accounts.Settings())
	if value == "" {
		writeError(w, http.StatusNotFound, "not_set")
		return
	}
	h.Activity.SecretRevealed(r.Context(), sessionFrom(r.Context()).User, name, false)
	writeJSON(w, http.StatusOK, secretJSON{Value: value})
}

// revealTrackingKey answers the API key or user token the signed-in user
// connected a key service with, MDBList, PublicMetaDB or ListenBrainz, and
// records it in the activity log. The tokens and session keys of the
// services connected on their own sites are never revealed: 400
// not_revealable. 404 not_set: no connection, or one the key cannot
// decrypt.
func (h *handler) revealTrackingKey(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if h.Trackers == nil || !slices.Contains(trackers.Services, service) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if !trackers.ByKey(service) {
		writeError(w, http.StatusBadRequest, "not_revealable")
		return
	}
	user := sessionFrom(r.Context()).User
	key, err := h.Trackers.Key(r.Context(), user.ID, service)
	switch {
	case errors.Is(err, trackers.ErrNotConnected):
		writeError(w, http.StatusNotFound, "not_set")
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	h.Activity.SecretRevealed(r.Context(), user, service, true)
	writeJSON(w, http.StatusOK, secretJSON{Value: key})
}

// secretsHealthJSON is how the stored secrets stand: whether
// POLYFIN_SECRET_KEY is set, how many are stored unencrypted, and which
// the key cannot decrypt, by name.
type secretsHealthJSON struct {
	Encrypted  bool             `json:"encrypted"`
	Plaintext  int              `json:"plaintext"`
	Unreadable []unreadableJSON `json:"unreadable"`
}

// unreadableJSON is a secret the key cannot decrypt: one of the server's,
// by its settings name, or a user's tracking connection.
type unreadableJSON struct {
	Setting *string `json:"setting"`
	Service *string `json:"service"`
	User    *string `json:"user"`
}

// secretsHealth reads how the stored secrets stand; nil when it cannot.
func (h *handler) secretsHealth(ctx context.Context) *secretsHealthJSON {
	if h.Health.Secrets == nil {
		return nil
	}
	report, err := h.Health.Secrets(ctx)
	if err != nil {
		h.Logger.Warn("The stored secrets could not be inspected", "error", err)
		return nil
	}
	result := &secretsHealthJSON{Encrypted: h.Health.SecretKey, Plaintext: report.Plaintext, Unreadable: []unreadableJSON{}}
	for _, unreadable := range report.Unreadable {
		result.Unreadable = append(result.Unreadable, newUnreadableJSON(unreadable))
	}
	return result
}

func newUnreadableJSON(u secrets.Unreadable) unreadableJSON {
	if u.Setting != "" {
		return unreadableJSON{Setting: &u.Setting}
	}
	return unreadableJSON{Service: &u.Service, User: &u.User}
}

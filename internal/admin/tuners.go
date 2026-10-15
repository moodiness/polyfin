package admin

import (
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/addons"
)

// tunerRefused answers 403 forbidden and reports true unless scope is the
// server's and the caller an administrator: a tuner is on the local
// network, which only administrators' sources may reach, and its tuners
// are the server's to share.
func tunerRefused(w http.ResponseWriter, r *http.Request, scope addons.Scope) bool {
	if scope.Owner == nil && sessionFrom(r.Context()).User.IsAdministrator {
		return false
	}
	writeError(w, http.StatusForbidden, "forbidden")
	return true
}

type foundTunerJSON struct {
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"`
	Model    string `json:"model"`
	Tuners   int    `json:"tuners"`
	Address  string `json:"address"`
	// Source is the source the tuner is already, null for none.
	Source *string `json:"source"`
}

// findTuners asks the local network for HDHomeRun tuners and lists those
// that answer.
func (h *handler) findTuners(w http.ResponseWriter, r *http.Request) {
	found, err := h.IPTV.FindTuners(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	items := make([]foundTunerJSON, 0, len(found))
	for _, t := range found {
		item := foundTunerJSON{DeviceID: t.DeviceID, Name: t.FriendlyName, Model: t.ModelNumber, Tuners: t.TunerCount, Address: t.BaseURL}
		if t.Source != nil {
			id := t.Source.String()
			item.Source = &id
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// scanTuner has a tuner of the server scan for channels; its line-up is
// read again once the scan ends.
func (h *handler) scanTuner(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, true)
	if !ok || tunerRefused(w, r, scope) {
		return
	}
	if _, err := h.IPTV.Scan(context.WithoutCancel(r.Context()), scope, source); h.answerFailure(w, r, err) {
		return
	}
	addon, err := h.Addons.Find(r.Context(), source)
	h.answerAddon(w, r, scope, http.StatusAccepted, addon, err)
}

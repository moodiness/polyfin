package admin

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/trackers"
)

// trackingServiceJSON is how one of the signed-in user's tracking services
// stands. Tokens and keys are never sent.
type trackingServiceJSON struct {
	Service     string            `json:"service"`
	Connection  string            `json:"connection"`
	Available   bool              `json:"available"`
	Connected   bool              `json:"connected"`
	Account     *string           `json:"account"`
	ConnectedAt *time.Time        `json:"connectedAt"`
	LastSentAt  *time.Time        `json:"lastSentAt"`
	Problem     *string           `json:"problem"`
	Code        *trackingCodeJSON `json:"code"`
}

// trackingCodeJSON is a code waiting for the user to enter it on the
// service's site.
type trackingCodeJSON struct {
	UserCode        string    `json:"userCode"`
	VerificationURL string    `json:"verificationUrl"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

func newTrackingServiceJSON(status trackers.Status) trackingServiceJSON {
	result := trackingServiceJSON{Service: status.Service, Connection: "key", Available: status.Available, Connected: status.Connected,
		ConnectedAt: utcSeconds(status.ConnectedAt), LastSentAt: utcSeconds(status.LastSentAt)}
	if status.ByCode {
		result.Connection = "code"
	}
	if status.Account != "" {
		result.Account = &status.Account
	}
	if status.Problem != "" {
		result.Problem = &status.Problem
	}
	if status.Code != nil {
		result.Code = &trackingCodeJSON{UserCode: status.Code.UserCode, VerificationURL: status.Code.VerificationURL,
			ExpiresAt: status.Code.ExpiresAt.UTC().Truncate(time.Second)}
	}
	return result
}

// utcSeconds is t in UTC to the second, as the admin app shows dates.
func utcSeconds(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	return new(t.UTC().Truncate(time.Second))
}

// ownTracking lists the signed-in user's tracking services.
func (h *handler) ownTracking(w http.ResponseWriter, r *http.Request) {
	if h.Trackers == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	statuses, err := h.Trackers.Statuses(r.Context(), sessionFrom(r.Context()).User.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := struct {
		Services []trackingServiceJSON `json:"services"`
	}{Services: make([]trackingServiceJSON, 0, len(statuses))}
	for _, status := range statuses {
		result.Services = append(result.Services, newTrackingServiceJSON(status))
	}
	writeJSON(w, http.StatusOK, result)
}

// connectTracking connects the signed-in user to a service: with the API
// key the body holds, or by starting a code the user enters on its site.
func (h *handler) connectTracking(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if h.Trackers == nil || !slices.Contains(trackers.Services, service) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	user := sessionFrom(r.Context()).User.ID
	var (
		status trackers.Status
		err    error
	)
	if trackers.ByCode(service) {
		status, err = h.Trackers.StartCode(r.Context(), user, service)
	} else {
		var body struct {
			Key string `json:"key"`
		}
		if !decode(w, r, &body) {
			return
		}
		status, err = h.Trackers.ConnectKey(r.Context(), user, service, body.Key)
	}
	switch {
	case errors.Is(err, trackers.ErrInvalidKey):
		writeError(w, http.StatusBadRequest, "invalid_key")
	case errors.Is(err, trackers.ErrNotAvailable):
		writeError(w, http.StatusConflict, "not_available")
	case errors.Is(err, trackers.ErrAppRefused):
		writeError(w, http.StatusConflict, "app_refused")
	case errors.Is(err, trackers.ErrUnreachable):
		writeError(w, http.StatusBadGateway, "service_unreachable")
	case err != nil:
		h.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, newTrackingServiceJSON(status))
	}
}

// disconnectTracking forgets the signed-in user's connection to a service.
func (h *handler) disconnectTracking(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if h.Trackers == nil || !slices.Contains(trackers.Services, service) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err := h.Trackers.Disconnect(r.Context(), sessionFrom(r.Context()).User.ID, service); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

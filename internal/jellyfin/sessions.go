package jellyfin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/quickconnect"
)

const quickConnectDisabled = "Quick connect is disabled"

func newQuickConnectResult(request quickconnect.Request) QuickConnectResult {
	return QuickConnectResult{
		Authenticated: request.User != nil,
		Secret:        request.Secret,
		Code:          request.Code,
		DeviceId:      request.DeviceID,
		DeviceName:    request.DeviceName,
		AppName:       request.AppName,
		AppVersion:    request.AppVersion,
		DateAdded:     Time(request.CreatedAt),
	}
}

func (h *Handler) quickConnectEnabled(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.Accounts.Settings().QuickConnectEnabled)
}

func (h *Handler) quickConnectInitiate(w http.ResponseWriter, r *http.Request) {
	if !h.Accounts.Settings().QuickConnectEnabled {
		writeJSON(w, http.StatusUnauthorized, quickConnectDisabled)
		return
	}
	c := readCredentials(r, h.Accounts.Settings().LegacyAuthorization)
	if !c.complete() {
		processingError(w, http.StatusBadRequest)
		return
	}
	request, err := h.QuickConnect.Initiate(c.DeviceID, c.Device, c.Client, c.Version)
	if errors.Is(err, quickconnect.ErrFull) {
		processingError(w, http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newQuickConnectResult(request))
}

func (h *Handler) quickConnectConnect(w http.ResponseWriter, r *http.Request) {
	if !h.Accounts.Settings().QuickConnectEnabled {
		writeJSON(w, http.StatusUnauthorized, quickConnectDisabled)
		return
	}
	request, err := h.QuickConnect.BySecret(query(r, "secret"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, "Unknown secret")
		return
	}
	writeJSON(w, http.StatusOK, newQuickConnectResult(request))
}

// quickConnectAuthorize approves a code for the caller or, for an
// administrator, for the user named by userId.
func (h *Handler) quickConnectAuthorize(w http.ResponseWriter, r *http.Request) {
	if !h.Accounts.Settings().QuickConnectEnabled {
		writeJSON(w, http.StatusUnauthorized, quickConnectDisabled)
		return
	}
	caller := callerFrom(r.Context())
	user := caller.User.ID
	if raw := query(r, "userId"); raw != "" {
		id, err := accounts.ParseID(raw)
		if err != nil {
			processingError(w, http.StatusBadRequest)
			return
		}
		if id != user && !caller.User.IsAdministrator {
			processingError(w, http.StatusForbidden)
			return
		}
		user = id
	}
	if err := h.QuickConnect.Authorize(query(r, "code"), user); err != nil {
		processingError(w, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, true)
}

// authenticateWithQuickConnect signs in the app holding an approved
// request's secret. Like Jellyfin, the secret stays valid until the request
// expires, so an app retrying after a network error is not refused.
func (h *Handler) authenticateWithQuickConnect(w http.ResponseWriter, r *http.Request) {
	c := readCredentials(r, h.Accounts.Settings().LegacyAuthorization)
	var body struct{ Secret string }
	if !c.complete() || decode(r, &body) != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	if !h.Accounts.Settings().QuickConnectEnabled {
		writeJSON(w, http.StatusUnauthorized, quickConnectDisabled)
		return
	}
	request, err := h.QuickConnect.BySecret(body.Secret)
	if err != nil || request.User == nil {
		processingError(w, http.StatusNotFound)
		return
	}
	user, err := h.Accounts.User(r.Context(), *request.User)
	if errors.Is(err, accounts.ErrNotFound) {
		processingError(w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if user.IsDisabled {
		processingError(w, http.StatusForbidden)
		return
	}
	h.signIn(w, r, user, c)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.Accounts.SignOutDevice(r.Context(), callerFrom(r.Context()).Token); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// capabilities records the capabilities an app reports in query parameters.
func (h *Handler) capabilities(w http.ResponseWriter, r *http.Request) {
	capabilities := accounts.Capabilities{
		PlayableMediaTypes:           listQuery(r, "playableMediaTypes"),
		SupportedCommands:            listQuery(r, "supportedCommands"),
		SupportsPersistentIdentifier: true,
	}
	if value, set := boolQuery(r, "supportsMediaControl"); set {
		capabilities.SupportsMediaControl = value
	}
	if value, set := boolQuery(r, "supportsPersistentIdentifier"); set {
		capabilities.SupportsPersistentIdentifier = value
	}
	h.saveCapabilities(w, r, capabilities)
}

// fullCapabilities records the capabilities an app reports in a JSON body.
func (h *Handler) fullCapabilities(w http.ResponseWriter, r *http.Request) {
	var body ClientCapabilities
	if err := decode(r, &body); err != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	h.saveCapabilities(w, r, accounts.Capabilities{
		PlayableMediaTypes:           body.PlayableMediaTypes,
		SupportedCommands:            body.SupportedCommands,
		SupportsMediaControl:         body.SupportsMediaControl,
		SupportsPersistentIdentifier: body.SupportsPersistentIdentifier,
	})
}

func (h *Handler) saveCapabilities(w http.ResponseWriter, r *http.Request, capabilities accounts.Capabilities) {
	if err := h.Accounts.SetCapabilities(r.Context(), callerFrom(r.Context()).Device.ID, capabilities); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listQuery reads an array parameter sent repeated or comma-separated, as
// ASP.NET accepts both.
func listQuery(r *http.Request, name string) []string {
	values := []string{}
	for key, raw := range r.URL.Query() {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, value := range raw {
			for item := range strings.SplitSeq(value, ",") {
				if item = strings.TrimSpace(item); item != "" {
					values = append(values, item)
				}
			}
		}
	}
	return values
}

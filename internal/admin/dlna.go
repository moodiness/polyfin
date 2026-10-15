package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/dlna"
)

// DLNA is what the admin API manages of the sharing over DLNA (see
// dlna.Service): the devices seen, and the profiles added.
type DLNA interface {
	Devices(ctx context.Context) ([]dlna.Device, error)
	UpdateDevice(ctx context.Context, id accounts.ID, changes dlna.DeviceChanges) (dlna.Device, error)
	RemoveDevice(ctx context.Context, id accounts.ID) error
	Profiles(ctx context.Context) ([]dlna.StoredProfile, error)
	Profile(ctx context.Context, id accounts.ID) (dlna.StoredProfile, error)
	SaveProfile(ctx context.Context, id *accounts.ID, data []byte) (dlna.StoredProfile, error)
	RemoveProfile(ctx context.Context, id accounts.ID) error
}

func (h *handler) dlnaRoutes(mux *http.ServeMux) {
	mux.Handle("GET /admin/api/dlna/devices", h.administrator(h.dlnaDevices))
	mux.Handle("PATCH /admin/api/dlna/devices/{id}", h.administrator(h.updateDLNADevice))
	mux.Handle("DELETE /admin/api/dlna/devices/{id}", h.administrator(h.removeDLNADevice))
	mux.Handle("GET /admin/api/dlna/profiles", h.administrator(h.dlnaProfiles))
	mux.Handle("GET /admin/api/dlna/profiles/{id}", h.administrator(h.dlnaProfileXML))
	mux.Handle("POST /admin/api/dlna/profiles", h.administrator(h.addDLNAProfile))
	mux.Handle("PUT /admin/api/dlna/profiles/{id}", h.administrator(h.replaceDLNAProfile))
	mux.Handle("DELETE /admin/api/dlna/profiles/{id}", h.administrator(h.removeDLNAProfile))
}

// dlnaUserJSON is the user new DLNA devices show the libraries of, as the
// settings give it: their ID, or null for no one.
func dlnaUserJSON(user *accounts.ID) json.RawMessage {
	if user == nil {
		return json.RawMessage("null")
	}
	return json.RawMessage(`"` + user.String() + `"`)
}

// dlnaNewDevicesUser reads the user new DLNA devices show the libraries
// of from a settings save: left out keeps current, null is no one, else a
// user's ID, which must exist. It answers the request itself when it is
// not valid.
func (h *handler) dlnaNewDevicesUser(w http.ResponseWriter, r *http.Request, raw json.RawMessage, current *accounts.ID) (*accounts.ID, bool) {
	if len(raw) == 0 {
		return current, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true
	}
	id, ok := h.existingUser(r.Context(), raw)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_dlna_user")
		return nil, false
	}
	return &id, true
}

// existingUser reads a user's ID from a JSON string, and reports whether
// that user exists.
func (h *handler) existingUser(ctx context.Context, raw json.RawMessage) (accounts.ID, bool) {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return accounts.ID{}, false
	}
	id, err := accounts.ParseID(text)
	if err != nil {
		return accounts.ID{}, false
	}
	if _, err := h.Accounts.User(ctx, id); err != nil {
		return accounts.ID{}, false
	}
	return id, true
}

// dlnaDeviceJSON is a DLNA device as the admin app shows it. UserID is the
// user whose libraries it shows, null for no one; Profile the profile
// forced for it, empty for none, else "builtin:" and a built-in profile's
// name, or an added profile's ID; MatchedProfile the name of the one it
// gets now, forced or chosen from what it last sent.
type dlnaDeviceJSON struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Manufacturer   string    `json:"manufacturer"`
	Model          string    `json:"model"`
	Address        string    `json:"address"`
	UserAgent      string    `json:"userAgent"`
	ClientInfo     string    `json:"clientInfo"`
	UserID         *string   `json:"userId"`
	UserName       string    `json:"userName,omitempty"`
	Blocked        bool      `json:"blocked"`
	Profile        string    `json:"profile"`
	MatchedProfile string    `json:"matchedProfile"`
	FirstSeen      time.Time `json:"firstSeen"`
	LastSeen       time.Time `json:"lastSeen"`
}

// dlnaDeviceOf describes a device, with the profile it gets among added.
func (h *handler) dlnaDeviceOf(ctx context.Context, d dlna.Device, added []dlna.StoredProfile) dlnaDeviceJSON {
	device := dlnaDeviceJSON{ID: d.ID.String(), Name: d.Name, Manufacturer: d.Manufacturer, Model: d.Model, Address: d.Address,
		UserAgent: d.UserAgent, ClientInfo: d.ClientInfo, Blocked: d.Blocked, Profile: d.Profile, FirstSeen: d.FirstSeen, LastSeen: d.LastSeen}
	if d.User != nil {
		device.UserID = new(d.User.String())
		if user, err := h.Accounts.User(ctx, *d.User); err == nil {
			device.UserName = user.Name
		}
	}
	var profiles []dlna.Profile
	for _, p := range added {
		profiles = append(profiles, p.Profile)
		if d.Profile == p.ID.String() {
			device.MatchedProfile = p.Name
		}
	}
	if name, ok := strings.CutPrefix(d.Profile, dlna.BuiltinPrefix); ok {
		device.MatchedProfile = name
	}
	if device.MatchedProfile == "" {
		headers := http.Header{}
		headers.Set("User-Agent", d.UserAgent)
		headers.Set("X-AV-Client-Info", d.ClientInfo)
		builtins := dlna.Builtins()
		device.MatchedProfile = dlna.Choose(append(profiles, builtins[1:]...),
			dlna.Seen{Name: d.Name, Manufacturer: d.Manufacturer, Model: d.Model, Headers: headers}).Name
	}
	return device
}

// dlnaDevices answers GET /admin/api/dlna/devices: the devices seen, the
// latest first.
func (h *handler) dlnaDevices(w http.ResponseWriter, r *http.Request) {
	if h.DLNA == nil {
		writeJSON(w, http.StatusOK, []dlnaDeviceJSON{})
		return
	}
	devices, err := h.DLNA.Devices(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	added, err := h.DLNA.Profiles(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]dlnaDeviceJSON, 0, len(devices))
	for _, d := range devices {
		result = append(result, h.dlnaDeviceOf(r.Context(), d, added))
	}
	writeJSON(w, http.StatusOK, result)
}

// updateDLNADevice answers PATCH /admin/api/dlna/devices/{id}: userId (a
// user's ID, or null for no one), blocked and profile, each kept when left
// out.
func (h *handler) updateDLNADevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok || h.DLNA == nil {
		return
	}
	var body struct {
		UserID  json.RawMessage `json:"userId"`
		Blocked *bool           `json:"blocked"`
		Profile *string         `json:"profile"`
	}
	if !decode(w, r, &body) {
		return
	}
	changes := dlna.DeviceChanges{Blocked: body.Blocked, Profile: body.Profile}
	if len(body.UserID) > 0 {
		var user *accounts.ID
		if !bytes.Equal(bytes.TrimSpace(body.UserID), []byte("null")) {
			found, ok := h.existingUser(r.Context(), body.UserID)
			if !ok {
				writeError(w, http.StatusBadRequest, "invalid_dlna_user")
				return
			}
			user = &found
		}
		changes.User = &user
	}
	device, err := h.DLNA.UpdateDevice(r.Context(), id, changes)
	switch {
	case errors.Is(err, dlna.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
		return
	case errors.Is(err, dlna.ErrUnknownProfile):
		writeError(w, http.StatusBadRequest, "invalid_dlna_profile")
		return
	case errors.Is(err, dlna.ErrUnknownUser):
		writeError(w, http.StatusBadRequest, "invalid_dlna_user")
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	added, err := h.DLNA.Profiles(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.dlnaDeviceOf(r.Context(), device, added))
}

// removeDLNADevice answers DELETE /admin/api/dlna/devices/{id}: the device
// is forgotten, and new when seen again.
func (h *handler) removeDLNADevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok || h.DLNA == nil {
		return
	}
	err := h.DLNA.RemoveDevice(r.Context(), id)
	switch {
	case errors.Is(err, dlna.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// dlnaProfileJSON is a profile as the admin app lists it: ID is
// "builtin:" and its name for a built-in profile, else the added
// profile's ID.
type dlnaProfileJSON struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Builtin   bool       `json:"builtin"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

// dlnaProfiles answers GET /admin/api/dlna/profiles: the built-in
// profiles, the generic one first, then the added ones by name.
func (h *handler) dlnaProfiles(w http.ResponseWriter, r *http.Request) {
	result := []dlnaProfileJSON{}
	for _, p := range dlna.Builtins() {
		result = append(result, dlnaProfileJSON{ID: dlna.BuiltinPrefix + p.Name, Name: p.Name, Builtin: true})
	}
	if h.DLNA != nil {
		added, err := h.DLNA.Profiles(r.Context())
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, p := range added {
			result = append(result, dlnaProfileJSON{ID: p.ID.String(), Name: p.Name, CreatedAt: new(p.CreatedAt)})
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// dlnaProfileXML answers GET /admin/api/dlna/profiles/{id}: a profile's
// XML, built-in or added.
func (h *handler) dlnaProfileXML(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	var data []byte
	if name, ok := strings.CutPrefix(raw, dlna.BuiltinPrefix); ok {
		builtin, found := dlna.BuiltinXML(name)
		if !found {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		data = builtin
	} else {
		id, ok := pathID(w, r, "id")
		if !ok || h.DLNA == nil {
			return
		}
		p, err := h.DLNA.Profile(r.Context(), id)
		if errors.Is(err, dlna.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		data = []byte(p.XML)
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write(data)
}

// readProfile reads the XML of a profile from a request's body.
func readProfile(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, dlna.MaxProfileBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large")
		return nil, false
	}
	return data, true
}

// saveDLNAProfile adds or replaces a profile and answers with it, or with
// invalid_dlna_profile and why in message.
func (h *handler) saveDLNAProfile(w http.ResponseWriter, r *http.Request, id *accounts.ID, status int) {
	data, ok := readProfile(w, r)
	if !ok {
		return
	}
	p, err := h.DLNA.SaveProfile(r.Context(), id, data)
	switch {
	case errors.Is(err, dlna.ErrInvalidProfile):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_dlna_profile", "message": err.Error()})
		return
	case errors.Is(err, dlna.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, status, dlnaProfileJSON{ID: p.ID.String(), Name: p.Name, CreatedAt: new(p.CreatedAt)})
}

// addDLNAProfile answers POST /admin/api/dlna/profiles, whose body is the
// profile's XML.
func (h *handler) addDLNAProfile(w http.ResponseWriter, r *http.Request) {
	if h.DLNA == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	h.saveDLNAProfile(w, r, nil, http.StatusCreated)
}

// replaceDLNAProfile answers PUT /admin/api/dlna/profiles/{id}.
func (h *handler) replaceDLNAProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok || h.DLNA == nil {
		return
	}
	h.saveDLNAProfile(w, r, &id, http.StatusOK)
}

// removeDLNAProfile answers DELETE /admin/api/dlna/profiles/{id}: the
// devices forced to it get theirs chosen again.
func (h *handler) removeDLNAProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok || h.DLNA == nil {
		return
	}
	err := h.DLNA.RemoveProfile(r.Context(), id)
	switch {
	case errors.Is(err, dlna.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

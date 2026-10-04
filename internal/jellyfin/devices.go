package jellyfin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Devices.
//
// Administrators see every signed-in device, rename them and sign them
// out, as Jellyfin's dashboard does. A device is the identifier its app
// sends: each user signed in on it is one entry, as in Jellyfin, and
// signing it out signs out all of them.

// DeviceInfoDto describes a signed-in device. Like Jellyfin, it never
// carries the device's access token.
type DeviceInfoDto struct {
	Name             string
	CustomName       *string `json:",omitempty"`
	Id               string
	LastUserName     string
	AppName          string
	AppVersion       string
	LastUserId       string
	DateLastActivity Time
	Capabilities     ClientCapabilities
}

func newDeviceInfo(device accounts.ListedDevice) DeviceInfoDto {
	capabilities := device.Capabilities
	return DeviceInfoDto{
		Name:             device.DeviceName,
		CustomName:       device.CustomName,
		Id:               device.DeviceID,
		LastUserName:     device.UserName,
		AppName:          device.Client,
		AppVersion:       device.ClientVersion,
		LastUserId:       device.UserID.String(),
		DateLastActivity: Time(device.LastActivityAt),
		Capabilities: ClientCapabilities{
			PlayableMediaTypes:           capabilities.PlayableMediaTypes,
			SupportedCommands:            capabilities.SupportedCommands,
			SupportsMediaControl:         capabilities.SupportsMediaControl,
			SupportsPersistentIdentifier: capabilities.SupportsPersistentIdentifier,
		},
	}
}

// DeviceOptionsDto is what an administrator set for a device.
type DeviceOptionsDto struct {
	Id         int
	DeviceId   string
	CustomName *string `json:",omitempty"`
}

// devices lists every signed-in device. Jellyfin filters them by the
// devices the named user may use; Polyfin's users may use any, as
// Jellyfin's do by default, so the user is only checked to exist.
func (h *Handler) devices(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id, set := b.guid(r, "userId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	if set {
		if _, err := h.Accounts.User(r.Context(), id); errors.Is(err, accounts.ErrNotFound) {
			processingError(w, http.StatusNotFound)
			return
		} else if err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	listed, err := h.Accounts.AllDevices(r.Context(), "")
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := listResult[DeviceInfoDto]{Items: make([]DeviceInfoDto, 0, len(listed)), TotalRecordCount: len(listed)}
	for _, device := range listed {
		result.Items = append(result.Items, newDeviceInfo(device))
	}
	writeJSON(w, http.StatusOK, result)
}

// requiredID binds the required id parameter of the device endpoints.
func requiredID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := query(r, "id")
	if strings.TrimSpace(id) == "" {
		validationProblem(w, map[string][]string{"id": {"The id field is required."}})
		return "", false
	}
	return id, true
}

// deviceInfo describes a device by its identifier: its most recently
// active user's entry.
func (h *Handler) deviceInfo(w http.ResponseWriter, r *http.Request) {
	id, ok := requiredID(w, r)
	if !ok {
		return
	}
	listed, err := h.Accounts.AllDevices(r.Context(), id)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if len(listed) == 0 {
		notFoundProblem(w)
		return
	}
	writeJSON(w, http.StatusOK, newDeviceInfo(listed[0]))
}

// deviceOptions answers what an administrator set for a device; like
// Jellyfin, a device never named has none.
func (h *Handler) deviceOptions(w http.ResponseWriter, r *http.Request) {
	id, ok := requiredID(w, r)
	if !ok {
		return
	}
	options, err := h.Accounts.DeviceOptions(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		notFoundProblem(w)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, DeviceOptionsDto{Id: options.ID, DeviceId: options.DeviceID, CustomName: options.CustomName})
}

// updateDeviceOptions names a device for administrators; a name left out
// removes it. Like Jellyfin, any identifier may be named, signed in or
// not.
func (h *Handler) updateDeviceOptions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CustomName *string
	}
	present, ok := readJSONBody(w, r, &body, "deviceOptions")
	if !ok {
		return
	}
	id := query(r, "id")
	errs := bindErrors{}
	if strings.TrimSpace(id) == "" {
		errs.add("id", "The id field is required.")
	}
	if !present {
		errs.add("", "A non-empty request body is required.")
		errs.add("deviceOptions", "The deviceOptions field is required.")
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	err := h.Accounts.SetDeviceName(r.Context(), id, body.CustomName)
	if errors.Is(err, accounts.ErrInvalidDeviceName) {
		validationProblem(w, map[string][]string{"CustomName": {"The device name must be at most 256 characters."}})
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteDevices signs out the devices the id parameters name, every user
// on each, through the same path as any other sign-out. Like Jellyfin,
// nothing is signed out when one of them is not signed in.
func (h *Handler) deleteDevices(w http.ResponseWriter, r *http.Request) {
	var ids []string
	for key, values := range r.URL.Query() {
		if strings.EqualFold(key, "id") {
			ids = append(ids, values...)
		}
	}
	for _, id := range ids {
		if id == "" {
			badRequestProblem(w)
			return
		}
		listed, err := h.Accounts.AllDevices(r.Context(), id)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if len(listed) == 0 {
			badRequestProblem(w)
			return
		}
	}
	for _, id := range ids {
		if _, err := h.Accounts.SignOutDeviceID(r.Context(), id); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

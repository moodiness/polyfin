package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/quickconnect"
)

type userJSON struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	IsAdministrator bool                `json:"isAdministrator"`
	IsHidden        bool                `json:"isHidden"`
	IsDisabled      bool                `json:"isDisabled"`
	CreatedAt       time.Time           `json:"createdAt"`
	LastLoginAt     *time.Time          `json:"lastLoginAt"`
	LastActivityAt  *time.Time          `json:"lastActivityAt"`
	ParentalControl parentalControlJSON `json:"parentalControl"`
	// Transcoding is set when the user may have both video and audio
	// converted; Downloads, when they may download.
	Transcoding bool `json:"transcoding"`
	Downloads   bool `json:"downloads"`
	// PersonalAddons is set when the user may add and use their own addons.
	PersonalAddons bool `json:"personalAddons"`
	// BlockedUntil is when the block of the account for wrong passwords
	// ends, null when it is not blocked.
	BlockedUntil *time.Time `json:"blockedUntil"`
}

// parentalControlJSON is a user's parental control: the highest rating
// score (and subscore at that score) the user may reach, null for no
// limit, and the kinds of items (Jellyfin's UnratedItem names) hidden when
// unrated.
type parentalControlJSON struct {
	MaxRating    *int     `json:"maxRating"`
	MaxSubRating *int     `json:"maxSubRating"`
	BlockUnrated []string `json:"blockUnrated"`
}

func newUserJSON(user accounts.User) userJSON {
	return userJSON{
		ID:              user.ID.String(),
		Name:            user.Name,
		IsAdministrator: user.IsAdministrator,
		IsHidden:        user.IsHidden,
		IsDisabled:      user.IsDisabled,
		CreatedAt:       user.CreatedAt,
		LastLoginAt:     user.LastLoginAt,
		LastActivityAt:  user.LastActivityAt,
		ParentalControl: parentalControlJSON{
			MaxRating:    user.Parental.MaxRating,
			MaxSubRating: user.Parental.MaxSubRating,
			BlockUnrated: append([]string{}, user.Parental.BlockUnrated...),
		},
		Transcoding: user.VideoTranscoding && user.AudioTranscoding,
		Downloads:   user.ContentDownloading,
		// Whether the user may have their own addons, and their block.
		PersonalAddons: user.PersonalAddons,
		BlockedUntil:   blockedUntil(user),
	}
}

// blockedUntil is when the block of user's account ends, nil when it is not
// blocked.
func blockedUntil(user accounts.User) *time.Time {
	if !user.Blocked(time.Now()) {
		return nil
	}
	return user.BlockedUntil
}

// ratingJSON is a rating the admin app offers as a user's limit.
type ratingJSON struct {
	Name     string `json:"name"`
	Score    int    `json:"score"`
	SubScore *int   `json:"subScore"`
}

// parentalRatings lists the ratings a user's limit can be set to, those
// Jellyfin apps offer, without the entry for unrated titles: blocking them
// is a choice of its own.
func (h *handler) parentalRatings(w http.ResponseWriter, _ *http.Request) {
	var result []ratingJSON
	for _, rating := range localization.Ratings() {
		if rating.Score != nil {
			result = append(result, ratingJSON{Name: rating.Name, Score: rating.Score.Score, SubScore: rating.Score.SubScore})
		}
	}
	writeJSON(w, http.StatusOK, result)
}

type deviceJSON struct {
	ID             string    `json:"id"`
	DeviceName     string    `json:"deviceName"`
	Client         string    `json:"client"`
	ClientVersion  string    `json:"clientVersion"`
	RemoteAddress  string    `json:"remoteAddress"`
	CreatedAt      time.Time `json:"createdAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
}

// settingsJSON are the settings the admin interface reads and saves.
// Chapters and PrepareAhead are always sent; a save without them keeps
// their current value, so that a page or script older than them leaves them
// alone.
type settingsJSON struct {
	ServerName          string `json:"serverName"`
	QuickConnectEnabled bool   `json:"quickConnectEnabled"`
	LegacyAuthorization bool   `json:"legacyAuthorization"`
	Language            string `json:"language"`
	Chapters            *bool  `json:"chapters"`
	PrepareAhead        *bool  `json:"prepareAhead"`
	// Transcoding, Downloads, CatalogLimit and ChannelLimit keep their
	// current values when a PUT leaves them out.
	Transcoding  *bool `json:"transcoding"`
	Downloads    *bool `json:"downloads"`
	CatalogLimit *int  `json:"catalogLimit"`
	ChannelLimit *int  `json:"channelLimit"`
	// The content settings keep their current values too when a PUT
	// leaves them out.
	SkipButtons           *bool `json:"skipButtons"`
	SimilarTitles         *bool `json:"similarTitles"`
	PlayedPercent         *int  `json:"playedPercent"`
	ResumePercent         *int  `json:"resumePercent"`
	VersionListMinutes    *int  `json:"versionListMinutes"`
	CatalogRefreshMinutes *int  `json:"catalogRefreshMinutes"`
	// The security settings keep their current values when a PUT leaves
	// them out, too.
	PersonalAddons     *bool `json:"personalAddons"`
	LoginAttempts      *int  `json:"loginAttempts"`
	InactiveDeviceDays *int  `json:"inactiveDeviceDays"`
	DetailedLog        *bool `json:"detailedLog"`
}

func newSettingsJSON(settings accounts.Settings) settingsJSON {
	return settingsJSON{
		ServerName:          settings.ServerName,
		QuickConnectEnabled: settings.QuickConnectEnabled,
		LegacyAuthorization: settings.LegacyAuthorization,
		Language:            settings.Language,
		Chapters:            &settings.Chapters,
		PrepareAhead:        &settings.PrepareAhead,
		Transcoding:         &settings.Transcoding,
		Downloads:           &settings.Downloads,
		CatalogLimit:        &settings.CatalogLimit,
		ChannelLimit:        &settings.ChannelLimit,

		SkipButtons:           &settings.SkipButtons,
		SimilarTitles:         &settings.SimilarTitles,
		PlayedPercent:         &settings.PlayedPercent,
		ResumePercent:         &settings.ResumePercent,
		VersionListMinutes:    &settings.VersionListMinutes,
		CatalogRefreshMinutes: &settings.CatalogRefreshMinutes,
		PersonalAddons:        &settings.PersonalAddons,
		LoginAttempts:         &settings.LoginAttempts,
		InactiveDeviceDays:    &settings.InactiveDeviceDays,
		DetailedLog:           &settings.DetailedLog,
	}
}

// pathID parses an identifier path value, answering 404 when it is malformed.
func pathID(w http.ResponseWriter, r *http.Request, name string) (accounts.ID, bool) {
	id, err := accounts.ParseID(r.PathValue(name))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return id, false
	}
	return id, true
}

func (h *handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decode(w, r, &body) {
		return
	}
	session := sessionFrom(r.Context())
	err := h.Accounts.ChangePassword(r.Context(), session.User.ID, body.CurrentPassword, body.NewPassword, session.TokenHash)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) writeDevices(w http.ResponseWriter, r *http.Request, user accounts.ID) {
	devices, err := h.Accounts.Devices(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]deviceJSON, 0, len(devices))
	for _, device := range devices {
		result = append(result, deviceJSON{
			ID:             device.ID.String(),
			DeviceName:     device.DeviceName,
			Client:         device.Client,
			ClientVersion:  device.ClientVersion,
			RemoteAddress:  device.RemoteAddress,
			CreatedAt:      device.CreatedAt,
			LastActivityAt: device.LastActivityAt,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) revokeDevice(w http.ResponseWriter, r *http.Request, user accounts.ID, name string) {
	device, ok := pathID(w, r, name)
	if !ok {
		return
	}
	err := h.Accounts.RevokeDevice(r.Context(), user, device)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) ownDevices(w http.ResponseWriter, r *http.Request) {
	h.writeDevices(w, r, sessionFrom(r.Context()).User.ID)
}

func (h *handler) revokeOwnDevice(w http.ResponseWriter, r *http.Request) {
	h.revokeDevice(w, r, sessionFrom(r.Context()).User.ID, "id")
}

func (h *handler) quickConnectRequest(w http.ResponseWriter, r *http.Request) {
	if !h.Accounts.Settings().QuickConnectEnabled {
		writeError(w, http.StatusConflict, "quick_connect_disabled")
		return
	}
	request, err := h.QuickConnect.ByCode(r.PathValue("code"))
	if errors.Is(err, quickconnect.ErrUnknown) {
		writeError(w, http.StatusNotFound, "unknown_code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deviceName":  request.DeviceName,
		"appName":     request.AppName,
		"appVersion":  request.AppVersion,
		"requestedAt": request.CreatedAt,
	})
}

func (h *handler) quickConnectApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !h.Accounts.Settings().QuickConnectEnabled {
		writeError(w, http.StatusConflict, "quick_connect_disabled")
		return
	}
	if err := h.QuickConnect.Authorize(body.Code, sessionFrom(r.Context()).User.ID); err != nil {
		writeError(w, http.StatusNotFound, "unknown_code")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) users(w http.ResponseWriter, r *http.Request) {
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]userJSON, 0, len(users))
	for _, user := range users {
		result = append(result, newUserJSON(user))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string `json:"name"`
		Password        string `json:"password"`
		IsAdministrator bool   `json:"isAdministrator"`
		IsHidden        bool   `json:"isHidden"`
	}
	if !decode(w, r, &body) {
		return
	}
	user, err := h.Accounts.CreateUser(r.Context(), accounts.NewUser{
		Name: body.Name, Password: body.Password, IsAdministrator: body.IsAdministrator, IsHidden: body.IsHidden,
	})
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newUserJSON(user))
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Name            *string              `json:"name"`
		Password        *string              `json:"password"`
		IsAdministrator *bool                `json:"isAdministrator"`
		IsHidden        *bool                `json:"isHidden"`
		IsDisabled      *bool                `json:"isDisabled"`
		ParentalControl *parentalControlJSON `json:"parentalControl"`
		// Transcoding sets both video and audio conversion.
		Transcoding *bool `json:"transcoding"`
		Downloads   *bool `json:"downloads"`
		// PersonalAddons lets the user add and use their own addons.
		PersonalAddons *bool `json:"personalAddons"`
	}
	if !decode(w, r, &body) {
		return
	}
	session := sessionFrom(r.Context())
	var keep []byte
	if id == session.User.ID {
		keep = session.TokenHash
	}
	changes := accounts.UserChanges{
		Name:               body.Name,
		Password:           body.Password,
		IsAdministrator:    body.IsAdministrator,
		IsHidden:           body.IsHidden,
		IsDisabled:         body.IsDisabled,
		VideoTranscoding:   body.Transcoding,
		AudioTranscoding:   body.Transcoding,
		ContentDownloading: body.Downloads,
		PersonalAddons:     body.PersonalAddons,
	}
	if p := body.ParentalControl; p != nil {
		changes.Parental = &accounts.ParentalControl{MaxRating: p.MaxRating, MaxSubRating: p.MaxSubRating, BlockUnrated: p.BlockUnrated}
	}
	user, err := h.Accounts.UpdateUser(r.Context(), id, changes, keep)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newUserJSON(user))
}

func (h *handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.Accounts.DeleteUser(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) userDevices(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.Accounts.User(r.Context(), id); accountError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeDevices(w, r, id)
}

func (h *handler) revokeUserDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	h.revokeDevice(w, r, id, "deviceId")
}

// unblockUser ends the block of a user's account for wrong passwords.
func (h *handler) unblockUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := h.Accounts.Unblock(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newUserJSON(user))
}

func (h *handler) settings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, newSettingsJSON(h.Accounts.Settings()))
}

func (h *handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var body settingsJSON
	if !decode(w, r, &body) {
		return
	}
	current := h.Accounts.Settings()
	settings, err := h.Accounts.UpdateSettings(r.Context(), accounts.Settings{
		ServerName:          body.ServerName,
		QuickConnectEnabled: body.QuickConnectEnabled,
		LegacyAuthorization: body.LegacyAuthorization,
		Language:            body.Language,
		Chapters:            valueOr(body.Chapters, current.Chapters),
		PrepareAhead:        valueOr(body.PrepareAhead, current.PrepareAhead),
		Transcoding:         valueOr(body.Transcoding, current.Transcoding),
		Downloads:           valueOr(body.Downloads, current.Downloads),
		CatalogLimit:        valueOr(body.CatalogLimit, current.CatalogLimit),
		ChannelLimit:        valueOr(body.ChannelLimit, current.ChannelLimit),

		SkipButtons:           valueOr(body.SkipButtons, current.SkipButtons),
		SimilarTitles:         valueOr(body.SimilarTitles, current.SimilarTitles),
		PlayedPercent:         valueOr(body.PlayedPercent, current.PlayedPercent),
		ResumePercent:         valueOr(body.ResumePercent, current.ResumePercent),
		VersionListMinutes:    valueOr(body.VersionListMinutes, current.VersionListMinutes),
		CatalogRefreshMinutes: valueOr(body.CatalogRefreshMinutes, current.CatalogRefreshMinutes),
		PersonalAddons:        valueOr(body.PersonalAddons, current.PersonalAddons),
		LoginAttempts:         valueOr(body.LoginAttempts, current.LoginAttempts),
		InactiveDeviceDays:    valueOr(body.InactiveDeviceDays, current.InactiveDeviceDays),
		DetailedLog:           valueOr(body.DetailedLog, current.DetailedLog),
	})
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if !settings.QuickConnectEnabled {
		h.QuickConnect.Clear()
	}
	writeJSON(w, http.StatusOK, newSettingsJSON(settings))
}

// valueOr is the value value points to, else fallback.
func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

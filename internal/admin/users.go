package admin

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
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
	// MaxPlaybacks is how many of the user's other devices may be playing
	// when one more starts, 0 for no limit; MaxBitrate, the highest bitrate
	// of their playback in bits per second, 0 for no limit. LiveTv lets them
	// watch Live TV; SyncPlay is CreateAndJoinGroups, JoinGroups or None;
	// RemoteControl lets them control other users' apps.
	MaxPlaybacks  int    `json:"maxPlaybacks"`
	MaxBitrate    int    `json:"maxBitrate"`
	LiveTv        bool   `json:"liveTv"`
	SyncPlay      string `json:"syncPlay"`
	RemoteControl bool   `json:"remoteControl"`
	// HiddenLibraries are the identifiers of the server's libraries the
	// user's apps do not show; BlockedGenres, the genres whose titles are
	// hidden; AccessSchedules, the hours the user may use the server in.
	HiddenLibraries []string             `json:"hiddenLibraries"`
	BlockedGenres   []string             `json:"blockedGenres"`
	AccessSchedules []accessScheduleJSON `json:"accessSchedules"`
	// CollectionManagement lets the user create, change and delete the
	// collections every user sees.
	CollectionManagement bool `json:"collectionManagement"`
	// PasswordResetPin is the PIN the user asked for from a Jellyfin app's
	// forgotten password screen, while it is valid, null otherwise: their
	// password becomes the PIN once they enter it.
	PasswordResetPin *passwordResetPinJSON `json:"passwordResetPin"`
}

type passwordResetPinJSON struct {
	Pin       string    `json:"pin"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// withPins adds to users the password reset PINs still valid.
func (h *handler) withPins(r *http.Request, users ...userJSON) ([]userJSON, error) {
	pins, err := h.Accounts.PasswordResetPINs(r.Context())
	if err != nil {
		return nil, err
	}
	for i, user := range users {
		id, _ := accounts.ParseID(user.ID)
		if pin, ok := pins[id]; ok {
			users[i].PasswordResetPin = &passwordResetPinJSON{Pin: pin.PIN, ExpiresAt: pin.ExpiresAt}
		}
	}
	return users, nil
}

// writeUser answers with user and their password reset PIN.
func (h *handler) writeUser(w http.ResponseWriter, r *http.Request, status int, user accounts.User) {
	result, err := h.withPins(r, newUserJSON(user))
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, status, result[0])
}

// accessScheduleJSON is a span of hours on a day, one of Jellyfin's
// DynamicDayOfWeek names (accounts.ScheduleDays).
type accessScheduleJSON struct {
	Day       string  `json:"day"`
	StartHour float64 `json:"startHour"`
	EndHour   float64 `json:"endHour"`
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
		// The user's playback and access limits.
		MaxPlaybacks:  user.MaxPlaybacks,
		MaxBitrate:    user.MaxBitrate,
		LiveTv:        user.LiveTv,
		SyncPlay:      string(user.SyncPlay),
		RemoteControl: user.RemoteControl,
		// The user's content settings.
		HiddenLibraries: idStrings(user.HiddenLibraries),
		BlockedGenres:   append([]string{}, user.BlockedGenres...),
		AccessSchedules: schedulesJSON(user.AccessSchedules),
		// The user's permission to manage collections.
		CollectionManagement: user.CollectionManagement,
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

func idStrings(ids []accounts.ID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, id.String())
	}
	return result
}

func schedulesJSON(schedules []accounts.AccessSchedule) []accessScheduleJSON {
	result := make([]accessScheduleJSON, 0, len(schedules))
	for _, s := range schedules {
		result = append(result, accessScheduleJSON{Day: s.Day, StartHour: s.StartHour, EndHour: s.EndHour})
	}
	return result
}

// libraryChoiceJSON is one of the server's libraries a user may see.
type libraryChoiceJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// userContentChoices lists what the admin app offers for a user's content:
// the server's libraries, in order, and the genres the server's libraries
// can be narrowed to, which name their genre pages, sorted.
func (h *handler) userContentChoices(w http.ResponseWriter, r *http.Request) {
	libraries, err := library.ServerLibraries(r.Context(), h.Addons, h.Accounts.Settings().Language)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := struct {
		Libraries []libraryChoiceJSON `json:"libraries"`
		Genres    []string            `json:"genres"`
	}{Libraries: []libraryChoiceJSON{}, Genres: []string{}}
	for _, l := range libraries {
		result.Libraries = append(result.Libraries, libraryChoiceJSON{ID: l.ID.String(), Name: l.Name})
		for _, genre := range l.Genres {
			genre = strings.TrimSpace(genre)
			if genre != "" && !slices.ContainsFunc(result.Genres, func(known string) bool { return strings.EqualFold(known, genre) }) {
				result.Genres = append(result.Genres, genre)
			}
		}
	}
	slices.SortFunc(result.Genres, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	writeJSON(w, http.StatusOK, result)
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
	// AnalysisTimeout, VersionAttempts, PreferDirectPlay, MaxConversions
	// and MaxConversionHeight keep their current values when a PUT leaves
	// them out.
	AnalysisTimeout     *int  `json:"analysisTimeout"`
	VersionAttempts     *int  `json:"versionAttempts"`
	PreferDirectPlay    *bool `json:"preferDirectPlay"`
	MaxConversions      *int  `json:"maxConversions"`
	MaxConversionHeight *int  `json:"maxConversionHeight"`
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
		AnalysisTimeout:       &settings.AnalysisTimeout,
		VersionAttempts:       &settings.VersionAttempts,
		PreferDirectPlay:      &settings.PreferDirectPlay,
		MaxConversions:        &settings.MaxConversions,
		MaxConversionHeight:   &settings.MaxConversionHeight,
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
	h.Activity.PasswordChanged(r.Context(), session.User)
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
	if result, err = h.withPins(r, result...); err != nil {
		h.internalError(w, r, err)
		return
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
	h.Activity.UserCreated(r.Context(), user)
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
		// The user's playback and access limits, see userJSON.
		MaxPlaybacks  *int                     `json:"maxPlaybacks"`
		MaxBitrate    *int                     `json:"maxBitrate"`
		LiveTv        *bool                    `json:"liveTv"`
		SyncPlay      *accounts.SyncPlayAccess `json:"syncPlay"`
		RemoteControl *bool                    `json:"remoteControl"`
		// The user's content settings; see userJSON.
		HiddenLibraries *[]string             `json:"hiddenLibraries"`
		BlockedGenres   *[]string             `json:"blockedGenres"`
		AccessSchedules *[]accessScheduleJSON `json:"accessSchedules"`
		// CollectionManagement lets the user manage collections.
		CollectionManagement *bool `json:"collectionManagement"`
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
		MaxPlaybacks:       body.MaxPlaybacks,
		MaxBitrate:         body.MaxBitrate,
		LiveTv:             body.LiveTv,
		SyncPlay:           body.SyncPlay,
		RemoteControl:      body.RemoteControl,
		BlockedGenres:      body.BlockedGenres,
		// The user's permission to manage collections.
		CollectionManagement: body.CollectionManagement,
	}
	if p := body.ParentalControl; p != nil {
		changes.Parental = &accounts.ParentalControl{MaxRating: p.MaxRating, MaxSubRating: p.MaxSubRating, BlockUnrated: p.BlockUnrated}
	}
	if body.HiddenLibraries != nil {
		hidden := make([]accounts.ID, 0, len(*body.HiddenLibraries))
		for _, raw := range *body.HiddenLibraries {
			id, err := accounts.ParseID(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_hidden_libraries")
				return
			}
			hidden = append(hidden, id)
		}
		changes.HiddenLibraries = &hidden
	}
	if body.AccessSchedules != nil {
		schedules := make([]accounts.AccessSchedule, 0, len(*body.AccessSchedules))
		for _, s := range *body.AccessSchedules {
			schedules = append(schedules, accounts.AccessSchedule{Day: s.Day, StartHour: s.StartHour, EndHour: s.EndHour})
		}
		changes.AccessSchedules = &schedules
	}
	user, err := h.Accounts.UpdateUser(r.Context(), id, changes, keep)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if changes.Password != nil {
		h.Activity.PasswordChanged(r.Context(), user)
	}
	if changes != (accounts.UserChanges{Password: changes.Password}) {
		h.Activity.UserChanged(r.Context(), user)
	}
	h.writeUser(w, r, http.StatusOK, user)
}

func (h *handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if err == nil {
		err = h.Accounts.DeleteUser(r.Context(), id)
	}
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.UserDeleted(r.Context(), user.Name)
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
	h.writeUser(w, r, http.StatusOK, user)
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
		AnalysisTimeout:       valueOr(body.AnalysisTimeout, current.AnalysisTimeout),
		VersionAttempts:       valueOr(body.VersionAttempts, current.VersionAttempts),
		PreferDirectPlay:      valueOr(body.PreferDirectPlay, current.PreferDirectPlay),
		MaxConversions:        valueOr(body.MaxConversions, current.MaxConversions),
		MaxConversionHeight:   valueOr(body.MaxConversionHeight, current.MaxConversionHeight),
	})
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.SettingsSaved(r.Context(), sessionFrom(r.Context()).User.Name)
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

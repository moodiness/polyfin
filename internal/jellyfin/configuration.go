package jellyfin

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// User configuration.
//
// Jellyfin apps save the user's playback preferences (audio and subtitle
// languages, subtitle mode) and home screen settings as the user's
// configuration, and read it back in every user DTO. Polyfin stores it,
// normalised, as JSON, and applies the settings Jellyfin applies on the
// server: the default tracks of versions, the order and visibility of
// libraries, and played titles among the latest.

// subtitleModes are Jellyfin's SubtitlePlaybackMode values, by number.
var subtitleModes = []string{"Default", "Always", "OnlyForced", "None", "Smart"}

func (h *Handler) configurationRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodPost, "/Users/Configuration", h.updateUserConfiguration)
	// Apps written for older Jellyfin versions name the user in the path.
	signedIn(http.MethodPost, "/Users/{userId}/Configuration", h.updateUserConfiguration)
}

// defaultUserConfiguration is a new Jellyfin user's configuration, which
// users keep until they save one.
func defaultUserConfiguration() UserConfiguration {
	return UserConfiguration{
		PlayDefaultAudioTrack:      true,
		GroupedFolders:             []string{},
		SubtitleMode:               subtitleModes[0],
		OrderedViews:               []string{},
		LatestItemsExcludes:        []string{},
		MyMediaExcludes:            []string{},
		HidePlayedInLatest:         true,
		RememberAudioSelections:    true,
		RememberSubtitleSelections: true,
		EnableNextEpisodeAutoPlay:  true,
		CastReceiverId:             castReceivers[0].Id,
	}
}

// userConfiguration returns the configuration user saved, or a new user's.
func (h *Handler) userConfiguration(ctx context.Context, user accounts.ID) (UserConfiguration, error) {
	if configuration, ok := h.configurations.Get(user); ok {
		return configuration, nil
	}
	raw, found, err := h.Preferences.Configuration(ctx, user)
	if err != nil {
		return UserConfiguration{}, err
	}
	configuration := defaultUserConfiguration()
	if found {
		if err := json.Unmarshal(raw, &configuration); err != nil {
			return UserConfiguration{}, fmt.Errorf("configuration of user %s: %w", user, err)
		}
	}
	h.configurations.Put(user, configuration)
	return configuration, nil
}

// userDto describes user with the configuration they saved, and their
// count of wrong passwords with the server's limit, -1 when there is none.
// The public list of users, which anyone may read, leaves those out.
func (h *Handler) userDto(ctx context.Context, user accounts.User) (UserDto, error) {
	configuration, err := h.userConfiguration(ctx, user.ID)
	if err != nil {
		return UserDto{}, err
	}
	dto := newUserDto(user, h.ServerID)
	dto.Configuration = configuration
	dto.Policy.InvalidLoginAttemptCount = user.InvalidLoginAttempts
	if limit := h.Accounts.Settings().LoginAttempts; limit > 0 {
		dto.Policy.LoginAttemptsBeforeLockout = limit
	}
	if len(user.HiddenLibraries) > 0 {
		libraries, err := h.Library.ServerLibraries(ctx)
		if err != nil {
			return UserDto{}, err
		}
		libraryAccess(&dto.Policy, user, libraries)
	}
	return dto, nil
}

// updateUserConfiguration replaces the configuration of the caller, or of
// the user an administrator names, as Jellyfin does: settings left out of
// the body take a new user's values, not the previous ones.
func (h *Handler) updateUserConfiguration(w http.ResponseWriter, r *http.Request) {
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	configuration, bodyErrors := parseUserConfiguration(body)
	b := bindErrors{}
	id, set := b.userID(r)
	for name, messages := range bodyErrors {
		b[name] = append(b[name], messages...)
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user, ok := h.targetUser(w, r, id, set, notFoundProblem)
	if !ok {
		return
	}
	value, _ := json.Marshal(configuration)
	if err := h.Preferences.PutConfiguration(r.Context(), user.ID, value); err != nil {
		h.internalError(w, r, err)
		return
	}
	h.configurations.Put(user.ID, configuration)
	w.WriteHeader(http.StatusNoContent)
}

// parseUserConfiguration reads a UserConfiguration as Jellyfin's JSON
// settings do: property names in any letter case, the last of duplicates
// winning, unknown properties ignored, strings also read from numbers and
// booleans, the subtitle mode from its name or number. A Cast receiver
// Jellyfin does not offer leaves the first one. Errors are keyed like
// ASP.NET's.
func parseUserConfiguration(body []byte) (UserConfiguration, map[string][]string) {
	configuration := defaultUserConfiguration()
	required := []string{"The userConfig field is required."}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return configuration, map[string][]string{
			"":           {"A non-empty request body is required."},
			"userConfig": required,
		}
	}
	fail := func(path, message string, offset int64) (UserConfiguration, map[string][]string) {
		line, column := jsonPosition(body, offset)
		return configuration, map[string][]string{
			path:         {fmt.Sprintf("%s Path: %s | LineNumber: %d | BytePositionInLine: %d.", message, path, line, column)},
			"userConfig": required,
		}
	}
	failSyntax := func(err error, expectingName bool) (UserConfiguration, map[string][]string) {
		message, offset := syntaxProblem(body, err, expectingName)
		return fail("$", message, offset)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if token, err := decoder.Token(); err != nil {
		return failSyntax(err, false)
	} else if token != json.Delim('{') {
		return fail("$", jsonCannotConvert("MediaBrowser.Model.Configuration.UserConfiguration"), decoder.InputOffset())
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return failSyntax(err, true)
		}
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return failSyntax(err, false)
		}
		message := ""
		switch strings.ToLower(name) {
		case "audiolanguagepreference":
			configuration.AudioLanguagePreference, message = jsonString(raw)
		case "subtitlelanguagepreference":
			var text *string
			text, message = jsonString(raw)
			configuration.SubtitleLanguagePreference = ""
			if text != nil {
				configuration.SubtitleLanguagePreference = *text
			}
		case "subtitlemode":
			var mode int
			mode, message = jsonEnum(raw, subtitleModes, "Jellyfin.Database.Implementations.Enums.SubtitlePlaybackMode")
			if message == "" && (mode < 0 || mode >= len(subtitleModes)) {
				message = jsonCannotConvert("Jellyfin.Database.Implementations.Enums.SubtitlePlaybackMode")
			} else if message == "" {
				configuration.SubtitleMode = subtitleModes[mode]
			}
		case "castreceiverid":
			var text *string
			text, message = jsonString(raw)
			configuration.CastReceiverId = castReceivers[0].Id
			if text != nil && slices.ContainsFunc(castReceivers, func(c CastReceiverApplication) bool { return c.Id == *text }) {
				configuration.CastReceiverId = *text
			}
		case "playdefaultaudiotrack":
			configuration.PlayDefaultAudioTrack, message = jsonBool(raw)
		case "displaymissingepisodes":
			configuration.DisplayMissingEpisodes, message = jsonBool(raw)
		case "displaycollectionsview":
			configuration.DisplayCollectionsView, message = jsonBool(raw)
		case "enablelocalpassword":
			configuration.EnableLocalPassword, message = jsonBool(raw)
		case "hideplayedinlatest":
			configuration.HidePlayedInLatest, message = jsonBool(raw)
		case "rememberaudioselections":
			configuration.RememberAudioSelections, message = jsonBool(raw)
		case "remembersubtitleselections":
			configuration.RememberSubtitleSelections, message = jsonBool(raw)
		case "enablenextepisodeautoplay":
			configuration.EnableNextEpisodeAutoPlay, message = jsonBool(raw)
		case "groupedfolders":
			configuration.GroupedFolders, message = jsonGUIDs(raw)
		case "orderedviews":
			configuration.OrderedViews, message = jsonGUIDs(raw)
		case "latestitemsexcludes":
			configuration.LatestItemsExcludes, message = jsonGUIDs(raw)
		case "mymediaexcludes":
			configuration.MyMediaExcludes, message = jsonGUIDs(raw)
		}
		if message != "" {
			return fail("$."+name, message, decoder.InputOffset())
		}
	}
	if _, err := decoder.Token(); err != nil {
		return failSyntax(err, true)
	}
	// .NET reports the first byte after the object.
	trailing := bytes.TrimLeft(body[decoder.InputOffset():], " \t\r\n")
	if len(trailing) > 0 {
		offset := int64(len(body) - len(trailing))
		return fail("$", fmt.Sprintf("'%c' is invalid after a single JSON value. Expected end of data.", trailing[0]), offset)
	}
	return configuration, nil
}

// jsonGUIDs reads a list of identifiers, null giving an empty one. They are
// kept in Polyfin's form, the one item DTOs carry.
func jsonGUIDs(raw json.RawMessage) ([]string, string) {
	var texts []string
	if err := json.Unmarshal(raw, &texts); err != nil {
		return nil, jsonCannotConvert("System.Guid[]")
	}
	ids := make([]string, 0, len(texts))
	for _, text := range texts {
		id, ok := parseGUID(text)
		if !ok {
			return nil, jsonCannotConvert("System.Guid[]")
		}
		ids = append(ids, id.String())
	}
	return ids, ""
}

// arrangeViews applies the user's home screen settings to their libraries,
// as Jellyfin does: the ones left out of My Media are hidden unless
// includeHidden asks for them (the settings screen lists them all), and the
// ones the user ordered come first, in that order, the others after them in
// their usual order.
func arrangeViews(views []BaseItemDto, configuration UserConfiguration, includeHidden bool) []BaseItemDto {
	if !includeHidden {
		views = slices.DeleteFunc(views, func(view BaseItemDto) bool { return slices.Contains(configuration.MyMediaExcludes, view.Id) })
	}
	position := func(view BaseItemDto) int {
		if i := slices.Index(configuration.OrderedViews, view.Id); i >= 0 {
			return i
		}
		return len(configuration.OrderedViews)
	}
	slices.SortStableFunc(views, func(a, b BaseItemDto) int { return cmp.Compare(position(a), position(b)) })
	return views
}

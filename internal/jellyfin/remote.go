package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/coder/websocket"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The names Jellyfin's enumerations take in remote control requests.
var (
	playCommands      = []string{"PlayNow", "PlayNext", "PlayLast", "PlayInstantMix", "PlayShuffle"}
	playstateCommands = []string{"Stop", "Pause", "Unpause", "NextTrack", "PreviousTrack", "Seek", "Rewind", "FastForward", "PlayPause"}
	generalCommands   = []string{"MoveUp", "MoveDown", "MoveLeft", "MoveRight", "PageUp", "PageDown", "PreviousLetter",
		"NextLetter", "ToggleOsd", "ToggleContextMenu", "Select", "Back", "TakeScreenshot", "SendKey", "SendString", "GoHome",
		"GoToSettings", "VolumeUp", "VolumeDown", "Mute", "Unmute", "ToggleMute", "SetVolume", "SetAudioStreamIndex",
		"SetSubtitleStreamIndex", "ToggleFullscreen", "DisplayContent", "GoToSearch", "DisplayMessage", "SetRepeatMode",
		"ChannelUp", "ChannelDown", "Guide", "ToggleStats", "PlayMediaSource", "PlayTrailers", "SetShuffleQueue", "PlayState",
		"PlayNext", "ToggleOsdMenu", "Play", "SetMaxStreamingBitrate", "SetPlaybackOrder"}
	itemKinds = []string{"AggregateFolder", "Audio", "AudioBook", "BasePluginFolder", "Book", "BoxSet", "Channel",
		"ChannelFolderItem", "CollectionFolder", "Episode", "Folder", "Genre", "ManualPlaylistsFolder", "Movie",
		"LiveTvChannel", "LiveTvProgram", "MusicAlbum", "MusicArtist", "MusicGenre", "MusicVideo", "Person", "Photo",
		"PhotoAlbum", "Playlist", "PlaylistsFolder", "Program", "Recording", "Season", "Series", "Studio", "Trailer",
		"TvChannel", "TvProgram", "UserRootFolder", "UserView", "Video", "Year"}
)

// PlayRequest is the data of a Play message: what the controlled app is to
// play, and how.
type PlayRequest struct {
	ItemIds             []string
	StartPositionTicks  *int64 `json:",omitempty"`
	PlayCommand         string
	ControllingUserId   string
	SubtitleStreamIndex *int   `json:",omitempty"`
	AudioStreamIndex    *int   `json:",omitempty"`
	MediaSourceId       string `json:",omitempty"`
	StartIndex          *int   `json:",omitempty"`
}

// PlaystateRequest is the data of a Playstate message, which acts on what
// the controlled app plays.
type PlaystateRequest struct {
	Command           string
	SeekPositionTicks *int64 `json:",omitempty"`
	ControllingUserId string
}

// GeneralCommand is the data of a GeneralCommand message: anything else an
// app can be asked, such as showing a message or going home.
type GeneralCommand struct {
	Name              string
	ControllingUserId string
	Arguments         map[string]string
}

func (h *Handler) remoteRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodPost, "/Sessions/{sessionId}/Playing", h.remotePlay)
	signedIn(http.MethodPost, "/Sessions/{sessionId}/Playing/{command}", h.remotePlaystate)
	signedIn(http.MethodPost, "/Sessions/{sessionId}/Command", h.remoteGeneralCommand)
	signedIn(http.MethodPost, "/Sessions/{sessionId}/Command/{command}", h.remoteNamedCommand)
	signedIn(http.MethodPost, "/Sessions/{sessionId}/System/{command}", h.remoteNamedCommand)
	signedIn(http.MethodPost, "/Sessions/{sessionId}/Message", h.remoteMessage)
	signedIn(http.MethodPost, "/Sessions/{sessionId}/Viewing", h.remoteViewing)
}

// controllable reports whether a device takes commands: its app declared
// media control, and keeps a socket open for the commands to reach it.
func (h *Handler) controllable(device accounts.Device) bool {
	return device.Capabilities.SupportsMediaControl && h.sockets.latest(device.ID) != nil
}

// mayControl reports whether controller may send commands to the sessions
// of owner. Controlling another user's apps takes Jellyfin's policy
// EnableRemoteControlOfOtherUsers, which Polyfin grants administrators only
// (see newUserDto).
func mayControl(controller accounts.User, owner accounts.ID) bool {
	return controller.ID == owner || controller.IsAdministrator
}

// remotePlay asks a session to play items, now or queued.
func (h *Handler) remotePlay(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	raw, _ := queryParam(r, "playCommand")
	command := b.enum("playCommand", raw, playCommands)
	// Like the identifiers of other lists, those that do not parse are
	// skipped; a request left with none has nothing to play.
	items := []string{}
	for _, value := range listQuery(r, "itemIds") {
		if id, ok := parseGUID(value); ok {
			items = append(items, id.String())
		}
	}
	if len(items) == 0 {
		b.add("itemIds", "The itemIds field is required.")
	}
	request := PlayRequest{
		ItemIds:             items,
		StartPositionTicks:  b.ticks(r, "startPositionTicks"),
		PlayCommand:         command,
		ControllingUserId:   callerFrom(r.Context()).User.ID.String(),
		AudioStreamIndex:    b.optionalInt32(r, "audioStreamIndex"),
		SubtitleStreamIndex: b.optionalInt32(r, "subtitleStreamIndex"),
		MediaSourceId:       query(r, "mediaSourceId"),
		StartIndex:          b.optionalInt32(r, "startIndex"),
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	h.command(w, r, "Play", request)
}

// remotePlaystate acts on what a session plays: pausing, seeking, stopping.
func (h *Handler) remotePlaystate(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	request := PlaystateRequest{
		Command:           b.enum("command", r.PathValue("command"), playstateCommands),
		SeekPositionTicks: b.ticks(r, "seekPositionTicks"),
		ControllingUserId: callerFrom(r.Context()).User.ID.String(),
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	h.command(w, r, "Playstate", request)
}

// remoteNamedCommand sends a general command named in the route, without
// arguments.
func (h *Handler) remoteNamedCommand(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	name := b.enum("command", r.PathValue("command"), generalCommands)
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	h.generalCommand(w, r, name, map[string]string{})
}

// remoteGeneralCommand sends a general command described in the body, with
// its arguments.
func (h *Handler) remoteGeneralCommand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string
		Arguments map[string]string
	}
	if !commandBody(w, r, &body) {
		return
	}
	b := bindErrors{}
	name := b.enum("Name", body.Name, generalCommands)
	if len(b) > 0 {
		// Jellyfin reads the name as an enumeration: one it does not know
		// does not convert, which fails the whole body.
		validationProblem(w, map[string][]string{"$.Name": {"The JSON value could not be converted."}, "command": {"The command field is required."}})
		return
	}
	if body.Arguments == nil {
		body.Arguments = map[string]string{}
	}
	h.generalCommand(w, r, name, body.Arguments)
}

// remoteMessage has a session show a message, until it is dismissed or for
// TimeoutMs.
func (h *Handler) remoteMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Header    string
		Text      string
		TimeoutMs *int64
	}
	if !commandBody(w, r, &body) {
		return
	}
	if body.Text == "" {
		validationProblem(w, map[string][]string{"Text": {"The Text field is required."}})
		return
	}
	arguments := map[string]string{"Text": body.Text}
	if body.Header != "" {
		arguments["Header"] = body.Header
	}
	if body.TimeoutMs != nil {
		arguments["TimeoutMs"] = strconv.FormatInt(*body.TimeoutMs, 10)
	}
	h.generalCommand(w, r, "DisplayMessage", arguments)
}

// remoteViewing has a session show an item's page.
func (h *Handler) remoteViewing(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	raw, _ := queryParam(r, "itemType")
	kind := b.enum("itemType", raw, itemKinds)
	arguments := map[string]string{"ItemType": kind}
	for argument, parameter := range map[string]string{"ItemId": "itemId", "ItemName": "itemName"} {
		value := query(r, parameter)
		if strings.TrimSpace(value) == "" {
			b.add(parameter, "The "+parameter+" field is required.")
		}
		arguments[argument] = value
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	h.generalCommand(w, r, "DisplayContent", arguments)
}

func (h *Handler) generalCommand(w http.ResponseWriter, r *http.Request, name string, arguments map[string]string) {
	h.command(w, r, "GeneralCommand", GeneralCommand{
		Name:              name,
		ControllingUserId: callerFrom(r.Context()).User.ID.String(),
		Arguments:         arguments,
	})
}

// command delivers a message to the session named by the route, on the
// socket its device opened last. Like Jellyfin, a session that cannot take
// commands, for lack of a socket or of declared media control, is not told
// anything, and the request succeeds all the same. An unknown session is
// not found; another user's session is refused to a user who may not
// control it, before anything is sent.
func (h *Handler) command(w http.ResponseWriter, r *http.Request, kind string, data any) {
	id, ok := parseGUID(r.PathValue("sessionId"))
	if !ok {
		processingError(w, http.StatusNotFound)
		return
	}
	target, err := h.Accounts.Device(r.Context(), id)
	switch {
	case errors.Is(err, accounts.ErrNotFound):
		processingError(w, http.StatusNotFound)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	case !mayControl(callerFrom(r.Context()).User, target.UserID):
		processingError(w, http.StatusForbidden)
		return
	}
	if conn := h.sockets.latest(target.ID); conn != nil && target.Capabilities.SupportsMediaControl {
		// The command was accepted: it goes out even if the app sending it
		// hangs up, within the time a write to an app is given.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), socketWrite)
		defer cancel()
		if err := conn.Write(ctx, websocket.MessageText, socketPayload(kind, data)); err != nil {
			h.Logger.Debug("A command could not reach an app", "kind", kind, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// commandBody decodes the JSON body Jellyfin binds as its command
// parameter, answering as ASP.NET does when it is missing or malformed.
func commandBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return false
	}
	required := []string{"The command field is required."}
	if len(bytes.TrimSpace(body)) == 0 {
		validationProblem(w, map[string][]string{"": {"A non-empty request body is required."}, "command": required})
		return false
	}
	if json.Unmarshal(body, into) != nil {
		validationProblem(w, map[string][]string{"$": {"The JSON value could not be converted."}, "command": required})
		return false
	}
	return true
}

// enum binds a required value to one of names, compared without regard to
// case as ASP.NET binds enumerations, and returns the name as declared.
func (b bindErrors) enum(name, raw string, names []string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		b.add(name, "The "+name+" field is required.")
		return ""
	}
	for _, known := range names {
		if strings.EqualFold(known, raw) {
			return known
		}
	}
	b.add(name, notValid(raw))
	return ""
}

// optionalInt32 binds an optional integer, nil when unset.
func (b bindErrors) optionalInt32(r *http.Request, name string) *int {
	value, set := b.int32(r, name)
	if !set {
		return nil
	}
	return &value
}

// ticks binds an optional position in ticks, which overflows 32 bits after
// four minutes; nil when unset.
func (b bindErrors) ticks(r *http.Request, name string) *int64 {
	raw, _ := queryParam(r, name)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		b.add(name, notValid(raw))
		return nil
	}
	return &value
}

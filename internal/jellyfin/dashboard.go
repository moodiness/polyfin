package jellyfin

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// LiveSession is a device playing, as the admin dashboard shows it.
type LiveSession struct {
	User   accounts.User
	Device accounts.Device
	// Controllable tells whether the device's app takes commands, such as
	// stopping or showing a message.
	Controllable bool
	// Playing is what the device reported, its position moved on since.
	Playing playback.NowPlaying
	// Item is the movie, episode, channel or recording played; Found is
	// false when it could not be resolved for the user.
	Item  library.Item
	Found bool
	// Source is what the version played holds, when it was analyzed.
	Source *media.Analysis
	// Encodings are the remuxes and conversions of the play session: none
	// when the app reads the file as it is.
	Encodings []hls.Running
	// Reasons are why the play session streamed over HLS, Jellyfin's
	// TranscodeReasons, when known.
	Reasons []string
}

// LiveSessions lists the devices playing, the latest started first.
func (h *Handler) LiveSessions(ctx context.Context) ([]LiveSession, error) {
	playing := h.sessions.All()
	if len(playing) == 0 {
		return []LiveSession{}, nil
	}
	encodings := map[string][]hls.Running{}
	for _, running := range h.Playback.Encodings() {
		encodings[running.Key.Session] = append(encodings[running.Key.Session], running)
	}
	users := map[accounts.ID]accounts.User{}
	sessions := make([]LiveSession, 0, len(playing))
	for deviceID := range playing {
		now, ok := h.sessions.Playing(deviceID)
		if !ok {
			continue
		}
		device, err := h.Accounts.Device(ctx, deviceID)
		if errors.Is(err, accounts.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		user, ok := users[now.User]
		if !ok {
			if user, err = h.Accounts.User(ctx, now.User); errors.Is(err, accounts.ErrNotFound) {
				continue
			} else if err != nil {
				return nil, err
			}
			users[now.User] = user
		}
		session := LiveSession{User: user, Device: device, Controllable: h.controllable(device), Playing: now,
			Encodings: encodings[now.PlaySessionID]}
		if now.PlaySessionID != "" {
			session.Reasons, _ = h.reasons.Get(now.PlaySessionID)
		}
		if item, err := h.played(ctx, user, now.Item); err == nil {
			session.Item, session.Found = item, true
		}
		// The version played is the one PlaybackInfo chose, which the play
		// session names, else the media source the app reports. Apps report
		// a title's own identifier for its first version, which has no
		// analysis of its own.
		version, err := accounts.ParseID(now.MediaSourceID)
		if grant, grantErr := h.Playback.Signer().Verify(now.PlaySessionID); grantErr == nil {
			version, err = grant.Version, nil
		}
		if err == nil {
			if analysis, ok := h.Playback.Analyzed(ctx, version); ok {
				session.Source = &analysis
			}
		}
		sessions = append(sessions, session)
	}
	slices.SortFunc(sessions, func(a, b LiveSession) int {
		return cmp.Or(b.Playing.Started.Compare(a.Playing.Started), cmp.Compare(a.Device.ID.String(), b.Device.ID.String()))
	})
	return sessions, nil
}

// ErrNotControllable reports a session whose app takes no commands: it
// keeps no socket open, or did not declare media control.
var ErrNotControllable = errors.New("the app takes no commands")

// StopPlayback asks the app of session to stop playing, as Jellyfin's
// remote control does, on behalf of by.
func (h *Handler) StopPlayback(ctx context.Context, by accounts.User, session accounts.ID) error {
	return h.sent(h.deliver(ctx, by, session, "Playstate", PlaystateRequest{Command: "Stop", ControllingUserId: by.ID.String()}))
}

// SendMessage has the app of session show a message, with an optional
// header, for timeout or until it is dismissed when timeout is 0, on
// behalf of by.
func (h *Handler) SendMessage(ctx context.Context, by accounts.User, session accounts.ID, header, text string, timeout time.Duration) error {
	arguments := map[string]string{"Text": text}
	if header != "" {
		arguments["Header"] = header
	}
	if timeout > 0 {
		arguments["TimeoutMs"] = strconv.FormatInt(timeout.Milliseconds(), 10)
	}
	return h.sent(h.deliver(ctx, by, session, "GeneralCommand",
		GeneralCommand{Name: "DisplayMessage", ControllingUserId: by.ID.String(), Arguments: arguments}))
}

// sent turns deliver's answer into an error, ErrNotControllable for a
// command no app could take.
func (h *Handler) sent(delivered bool, err error) error {
	if err == nil && !delivered {
		return ErrNotControllable
	}
	return err
}

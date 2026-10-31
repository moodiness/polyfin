package jellyfin

import (
	"context"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

const (
	// idlePlayback is how long a device's playback lasts without a report
	// from its app: as Jellyfin, a playback whose app reported nothing for
	// five minutes is stopped. Apps report every ten seconds while they
	// play, paused or not; one that stops reporting closed, crashed or lost
	// its network.
	idlePlayback = 5 * time.Minute
	// idleCheck is how often playbacks are checked for reports. Jellyfin
	// checks every five minutes, so stops a playback five to ten minutes
	// after its last report; Polyfin checks every minute, as the playback
	// holds a place of its user's Playbacks at once.
	idleCheck = time.Minute
)

// Run stops, until ctx is done, the playbacks of devices whose apps
// stopped reporting (see endIdlePlayback), and preloads the next episodes
// of the series users watch while it is on (see preloadNext).
func (h *Handler) Run(ctx context.Context) {
	go h.preloadLoop(ctx)
	ticker := time.NewTicker(idleCheck)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.endIdlePlayback(ctx)
		}
	}
}

// endIdlePlayback stops the playbacks of the devices that reported nothing
// for idlePlayback, as Jellyfin does, as if their apps had reported the
// stop at the position they last reported: the resume point is kept there,
// the activity and the playbacks Polyfin follows end, the conversions and
// the torrents the device left end as at a stop.
func (h *Handler) endIdlePlayback(ctx context.Context) {
	for device, playing := range h.sessions.Idle(idlePlayback) {
		c := caller{User: accounts.User{ID: playing.User}, Device: accounts.Device{ID: device}}
		if user, err := h.Accounts.User(ctx, playing.User); err == nil {
			c.User = user
		}
		if found, err := h.Accounts.Device(ctx, device); err == nil {
			c.Device = found
		}
		// A report that came meanwhile keeps the playback.
		if !h.sessions.StopIdle(device, playing.CheckedIn) {
			continue
		}
		h.Logger.Info("A playback is stopped: its app stopped reporting it", "user", c.User.Name,
			"device", c.Device.DeviceName, "app", c.Device.Client, "position", playing.Position.Round(time.Second),
			"last_report", playing.CheckedIn.Format(time.TimeOnly))
		h.recorded(context.WithValue(context.WithoutCancel(ctx), callerKey{}, c), playbackStopped, playing.PlayState, true, playing, true)
	}
}

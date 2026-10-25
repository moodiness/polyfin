package jellyfin

import (
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// chosenVersions remembers, by device, the version its last PlaybackInfo
// chose and that version's title: apps playing a title's first version as
// it is, as jellyfin-web does, report the title's own identifier and no
// play session, which name no version (see playedVersion).
type chosenVersions struct {
	mu     sync.Mutex
	chosen map[accounts.ID]chosenVersion
}

// chosenVersion is a version PlaybackInfo chose for a device, and its
// title.
type chosenVersion struct {
	title, version accounts.ID
}

func newChosenVersions() *chosenVersions {
	return &chosenVersions{chosen: map[accounts.ID]chosenVersion{}}
}

// choseVersion notes the version PlaybackInfo chose for a device's play
// of a title.
func (h *Handler) choseVersion(device, title, version accounts.ID) {
	if device == (accounts.ID{}) {
		return
	}
	h.chosen.mu.Lock()
	h.chosen.chosen[device] = chosenVersion{title: title, version: version}
	h.chosen.mu.Unlock()
}

// playedVersion is the version a device's playback reads: the one its
// play session names; else its media source, when that is a version
// listed; else the version the device's last PlaybackInfo chose for the
// title it plays. jellyfin-web playing a title's first version as it is
// reports no play session, and the title's own identifier as its media
// source, which stands for the first version PlaybackInfo offered, not
// always the title's first. Failing those, the media source as reported;
// ok is false when it names nothing.
func (h *Handler) playedVersion(device accounts.ID, playing playback.NowPlaying) (accounts.ID, bool) {
	if grant, err := h.Playback.Signer().Verify(playing.PlaySessionID); err == nil {
		return grant.Version, true
	}
	source, err := accounts.ParseID(playing.MediaSourceID)
	if err == nil && h.Library != nil {
		if _, known := h.Library.KnownVersion(source); known {
			return source, true
		}
	}
	h.chosen.mu.Lock()
	chosen, ok := h.chosen.chosen[device]
	h.chosen.mu.Unlock()
	if ok && chosen.title == h.titleOf(playing.Item) {
		return chosen.version, true
	}
	return source, err == nil
}

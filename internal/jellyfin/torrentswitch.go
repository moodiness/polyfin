package jellyfin

import (
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// torrentPlays remembers the torrent each device last played, and for
// which user and title, so that a playback of another title by the same
// device or user ends that torrent's linger at once (see
// leaveTorrents): the device moved on, and the swarm is left rather than
// read beside the new one.
type torrentPlays struct {
	mu    sync.Mutex
	plays map[accounts.ID]deviceTorrent
	// chosen is, by device, the version its last PlaybackInfo chose and
	// that version's title: apps playing a title's first version as it is,
	// as jellyfin-web does, report the title's own identifier and no play
	// session, which name no version (see torrentOf).
	chosen map[accounts.ID]chosenVersion
}

// chosenVersion is a version PlaybackInfo chose for a device, and its
// title.
type chosenVersion struct {
	title, version accounts.ID
}

// deviceTorrent is a torrent a device played: its info hash, the user and
// the title, and the last report of it.
type deviceTorrent struct {
	hash       string
	user, item accounts.ID
	at         time.Time
}

func newTorrentPlays() *torrentPlays {
	return &torrentPlays{plays: map[accounts.ID]deviceTorrent{}, chosen: map[accounts.ID]chosenVersion{}}
}

// choseVersion notes the version PlaybackInfo chose for a device's play
// of a title.
func (h *Handler) choseVersion(device, title, version accounts.ID) {
	if device == (accounts.ID{}) {
		return
	}
	h.torrents.mu.Lock()
	h.torrents.chosen[device] = chosenVersion{title: title, version: version}
	h.torrents.mu.Unlock()
}

// othersRecently is how long a torrent another user stopped stays theirs:
// their linger is left alone, as they may play it on.
const othersRecently = time.Minute

// followTorrents notes the torrent the device plays, from a report of its
// playback, and has a playback of another title end the linger of the
// torrents the device or its user left (see leaveTorrents). A stop ends
// nothing: jellyfin-web may send it late, after the next playback of the
// same title started. Nor does a late report of a title the device left
// (current false, see leaveTitle), which leaves the torrent it plays and
// the one it left as they are; switched tells that the report changed the
// device's title.
func (h *Handler) followTorrents(device, user accounts.ID, event playbackEvent, current, switched, had bool) {
	now := h.now()
	h.torrents.mu.Lock()
	if event == playbackStopped {
		if play, ok := h.torrents.plays[device]; ok {
			play.at = now
			h.torrents.plays[device] = play
		}
		h.torrents.mu.Unlock()
		return
	}
	h.torrents.mu.Unlock()
	playing, ok := h.sessions.Playing(device)
	if !ok || !current {
		return
	}
	title := h.titleOf(playing.Item)
	hash := h.torrentOf(device, playing)
	if event == playbackStarted || !had || switched {
		h.leaveTorrents(device, user, title, hash)
	}
	if hash != "" {
		h.torrents.mu.Lock()
		h.torrents.plays[device] = deviceTorrent{hash: hash, user: user, item: title, at: now}
		h.torrents.mu.Unlock()
	}
}

// leaveTorrents ends the linger of the torrents device or user played for
// another title than the one they play now, whose torrent, keep, goes on:
// those no other playback reads now, and no other user stopped lately.
// A pause, a seek, another audio track or a replay of the same title
// leave its torrent running, as does a title of the same torrent, as the
// episodes of a season pack.
// It returns the torrents it ended.
func (h *Handler) leaveTorrents(device, user, title accounts.ID, keep string) []string {
	read := h.torrentsRead(device)
	now := h.now()
	h.torrents.mu.Lock()
	left := map[string]bool{}
	for d, play := range h.torrents.plays {
		switch {
		case !h.Playback.TorrentRunning(play.hash):
			delete(h.torrents.plays, d)
		case d != device && play.user != user || play.hash == keep || play.item == title:
		case d != device && read[play.hash]:
			// The user's other device may play it still.
		default:
			// The device, or the user, moved on.
			delete(h.torrents.plays, d)
			if !read[play.hash] {
				left[play.hash] = true
			}
		}
	}
	for _, play := range h.torrents.plays {
		if play.user != user && now.Sub(play.at) < othersRecently {
			// Another user stopped it lately, and may play it on.
			delete(left, play.hash)
		}
	}
	h.torrents.mu.Unlock()
	ended := make([]string, 0, len(left))
	for hash := range left {
		h.Playback.EndTorrent(hash)
		ended = append(ended, hash)
	}
	return ended
}

// torrentsRead are the torrents the playbacks under way read, but those of
// device: those of other devices, DLNA devices, guests through share links
// and converted downloads.
func (h *Handler) torrentsRead(device accounts.ID) map[string]bool {
	read := map[string]bool{}
	add := func(version accounts.ID) {
		if hash := h.versionTorrent(version); hash != "" {
			read[hash] = true
		}
	}
	for d, playing := range h.sessions.All() {
		if d != device {
			if hash := h.torrentOf(d, playing); hash != "" {
				read[hash] = true
			}
		}
	}
	for _, play := range h.guests.all() {
		add(play.version)
	}
	for _, play := range h.dlna.all() {
		add(play.version)
	}
	for _, play := range h.downloads.all() {
		add(play.version)
	}
	return read
}

// torrentOf is the info hash of the torrent a device's playback reads,
// empty for none: that of the version its play session names, else of
// its media source when that is a version, else of the version the
// device's PlaybackInfo chose for the title it plays. jellyfin-web
// playing a title's first version as it is reports no play session, and
// the title's own identifier as its media source.
func (h *Handler) torrentOf(device accounts.ID, playing playback.NowPlaying) string {
	if grant, err := h.Playback.Signer().Verify(playing.PlaySessionID); err == nil {
		return h.versionTorrent(grant.Version)
	}
	if id, err := accounts.ParseID(playing.MediaSourceID); err == nil {
		if hash := h.versionTorrent(id); hash != "" {
			return hash
		}
	}
	h.torrents.mu.Lock()
	chosen, ok := h.torrents.chosen[device]
	h.torrents.mu.Unlock()
	if ok && chosen.title == h.titleOf(playing.Item) {
		return h.versionTorrent(chosen.version)
	}
	return ""
}

// versionTorrent is the info hash of the version of id, empty for a link
// or a version not known.
func (h *Handler) versionTorrent(id accounts.ID) string {
	if h.Library == nil {
		return ""
	}
	if version, ok := h.Library.KnownVersion(id); ok && version.Torrent != nil {
		return version.Torrent.InfoHash
	}
	return ""
}

// titleOf is the title of an item played: the item, or the title of a
// version opened as an item.
func (h *Handler) titleOf(item accounts.ID) accounts.ID {
	if h.Library != nil {
		if owner, ok := h.Library.VersionOwner(item); ok {
			return owner
		}
	}
	return item
}

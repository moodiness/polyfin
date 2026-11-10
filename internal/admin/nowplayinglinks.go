package admin

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
)

// nowPlayingLinks keeps whether the title of each playback under way links
// to the web player for each user reading Now playing. It is decided once
// per playback, as Now playing reads the playbacks every 3 seconds.
type nowPlayingLinks struct {
	mu     sync.Mutex
	linked map[nowPlayingLink]bool
}

// nowPlayingLink is a playback, the item it plays, as a user reads it:
// started, in nanoseconds since 1970, tells a new playback on the same
// device apart.
type nowPlayingLink struct {
	reader, item accounts.ID
	session      string
	started      int64
}

// linkTitles marks the titles of sessions that user reaches while the web
// player is served, as Recent activity links its titles (reachedTitles):
// what another user plays may be out of user's view. A playback decided
// before is not looked up again; when titles cannot be told, they are not
// linked, and are looked up again at the next read.
func (h *handler) linkTitles(ctx context.Context, user accounts.User, sessions []liveSessionJSON) {
	if h.ActivityTitles == nil || !h.WebClient {
		return
	}
	links := &h.nowPlayingLinks
	keys := make([]nowPlayingLink, len(sessions))
	var missing []accounts.ID
	links.mu.Lock()
	for i, s := range sessions {
		if s.Item == nil {
			continue
		}
		id, err := accounts.ParseID(s.Item.ID)
		if err != nil {
			continue
		}
		keys[i] = nowPlayingLink{reader: user.ID, item: id, session: s.ID, started: s.StartedAt.UnixNano()}
		if _, known := links.linked[keys[i]]; !known && !slices.Contains(missing, id) {
			missing = append(missing, id)
		}
	}
	links.mu.Unlock()
	reached := map[accounts.ID]bool{}
	lookedUp := len(missing) > 0
	if lookedUp {
		items, err := h.ActivityTitles.Items(ctx, user, missing)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				h.Logger.Warn("The titles playing could not be found", "error", err)
			}
			lookedUp = false
		}
		for _, item := range items {
			reached[item.ID] = true
		}
	}
	links.mu.Lock()
	defer links.mu.Unlock()
	if links.linked == nil {
		links.linked = map[nowPlayingLink]bool{}
	}
	// The user's playbacks that ended are forgotten.
	for key := range links.linked {
		if key.reader == user.ID && !slices.Contains(keys, key) {
			delete(links.linked, key)
		}
	}
	for i, key := range keys {
		if key == (nowPlayingLink{}) {
			continue
		}
		linked, known := links.linked[key]
		if !known && lookedUp && slices.Contains(missing, key.item) {
			linked = reached[key.item]
			links.linked[key] = linked
		}
		sessions[i].Item.Linked = linked
	}
}

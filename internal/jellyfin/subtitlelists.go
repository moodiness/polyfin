package jellyfin

import (
	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// listedSubtitles are a title's subtitle files as they were described for
// User: users see different sources (see addons.UsedBy), and so different
// files, at different indexes. Its fields are exported for the cache to
// size them (see cache.JSONSize).
type listedSubtitles struct {
	User  accounts.ID
	Files []library.ExternalSubtitle
}

// keepSubtitles remembers the subtitle files of item described for user.
func (h *Handler) keepSubtitles(user accounts.User, item accounts.ID, files []library.ExternalSubtitle) {
	h.subtitleFiles.Put(item, listedSubtitles{User: user.ID, Files: files})
}

// keptSubtitles returns the subtitle files of item remembered for user,
// and whether they were: those described for another user are not theirs.
func (h *Handler) keptSubtitles(user accounts.User, item accounts.ID) ([]library.ExternalSubtitle, bool) {
	kept, ok := h.subtitleFiles.Get(item)
	if !ok || kept.User != user.ID {
		return nil, false
	}
	return kept.Files, true
}

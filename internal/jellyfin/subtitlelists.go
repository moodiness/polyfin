package jellyfin

import (
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// listedKey names the subtitle files of an item described for a user:
// users see different sources (see addons.UsedBy), and so different files,
// at different indexes, and the description for one user does not replace
// another's.
type listedKey struct {
	User, Item accounts.ID
}

// listedSubtitles are a title's subtitle files as they were described for
// a user. Its fields are exported for the cache to size them (see
// cache.JSONSize).
type listedSubtitles struct {
	Files []library.ExternalSubtitle
}

// keepSubtitles remembers the subtitle files of item described for user,
// and returns those to describe: the files kept before, at their index
// still, then those of files not among them, in their order. An index an
// app was given thus names the same file for as long as the list is kept,
// whatever the addons list meanwhile: its subtitle and stream requests,
// which name files by index, come after the PlaybackInfo that described
// them. A file an addon no longer lists stays in place; a file a user adds
// or deletes forgets the lists of its title (see forgetSubtitles).
func (h *Handler) keepSubtitles(user accounts.User, item accounts.ID, files []library.ExternalSubtitle) []library.ExternalSubtitle {
	var described []library.ExternalSubtitle
	// A list too large for the cache is described all the same, unkept.
	h.subtitleFiles.Update(listedKey{user.ID, item}, func(kept listedSubtitles, _ bool) (listedSubtitles, bool) {
		described = appendNewFiles(kept.Files, files)
		return listedSubtitles{Files: described}, true
	})
	return described
}

// shownSubtitles returns the subtitle files of item to describe for user
// where nothing plays, as item details: those kept, then the others of
// files, as keepSubtitles would keep them, but kept only when none were,
// and complete, a list missing no addon's. The files a play was described
// with are thus served by their indexes whatever the details listed
// since; the next PlaybackInfo keeps the new ones at the indexes shown.
func (h *Handler) shownSubtitles(user accounts.User, item accounts.ID, files []library.ExternalSubtitle, complete bool) []library.ExternalSubtitle {
	if kept, ok := h.keptSubtitles(user, item); ok {
		return appendNewFiles(kept, files)
	}
	if complete {
		return h.keepSubtitles(user, item, files)
	}
	return files
}

// appendNewFiles returns kept, its files updated with those of files of
// the same identifier, followed by the other files of files in order.
func appendNewFiles(kept, files []library.ExternalSubtitle) []library.ExternalSubtitle {
	if len(kept) == 0 {
		return files
	}
	at := make(map[accounts.ID]int, len(kept)+len(files))
	result := make([]library.ExternalSubtitle, len(kept), len(kept)+len(files))
	copy(result, kept)
	for i, file := range kept {
		at[file.ID] = i
	}
	for _, file := range files {
		if i, ok := at[file.ID]; ok {
			result[i] = file
			continue
		}
		at[file.ID] = len(result)
		result = append(result, file)
	}
	return slices.Clip(result)
}

// keptSubtitles returns the subtitle files of item remembered for user,
// and whether they were: those described for another user are not theirs.
func (h *Handler) keptSubtitles(user accounts.User, item accounts.ID) ([]library.ExternalSubtitle, bool) {
	kept, ok := h.subtitleFiles.Get(listedKey{user.ID, item})
	return kept.Files, ok
}

// forgetSubtitles forgets the subtitle files of item described for every
// user, and returns them.
func (h *Handler) forgetSubtitles(item accounts.ID) []library.ExternalSubtitle {
	var forgotten []library.ExternalSubtitle
	h.subtitleFiles.DeleteFunc(func(key listedKey, kept listedSubtitles) bool {
		if key.Item != item {
			return false
		}
		forgotten = append(forgotten, kept.Files...)
		return true
	})
	return forgotten
}

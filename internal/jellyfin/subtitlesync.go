package jellyfin

import (
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// Addons find subtitles for a title, not for the version played: Polyfin
// syncs them to the version once it knows when the version has speech,
// found in the background at the version's first play (see
// playback.Service.FindSpeech and SyncSubtitle).

// findSpeech has the speech of the version a playback started with found,
// for a movie or an episode, the titles addons offer subtitles for, when
// the settings sync them.
func (h *Handler) findSpeech(ctx context.Context, user accounts.User, item library.Item, mediaSource string) {
	if item.Kind != library.KindMovie && item.Kind != library.KindEpisode || !h.Accounts.Settings().SyncAddonSubtitles {
		return
	}
	if version, ok := h.startedVersion(ctx, user, item, mediaSource); ok {
		h.Playback.FindSpeech(version)
	}
}

// addonSubtitle reports whether a subtitle file is an addon's, which is
// synced: not one a user added, nor one beside a local file, which is
// that file's own.
func addonSubtitle(file library.ExternalSubtitle) bool {
	return !file.Uploaded && file.Format == ""
}

// syncedSubtitle is the text of a subtitle file served for a version: an
// addon's synced to the version, once its speech is known, the others as
// they are.
func (h *Handler) syncedSubtitle(ctx context.Context, version accounts.ID, file library.ExternalSubtitle, text subtitleText) subtitleText {
	if !addonSubtitle(file) {
		return text
	}
	timing, ok := h.Playback.SyncSubtitle(ctx, version, file.ID, text.cues)
	if !ok || !timing.Changes() {
		return text
	}
	return text.retimed(timing)
}

// markSynced ends the names of the addon subtitles synced to a version
// with "(synced)" in the server language. files are the version's
// subtitle files, the first of its streams, and shown the stream the app
// shows, -1 for none. Once the version's speech is known, the subtitles
// not synced yet are synced first when their text is at hand: those read
// lately, and the one shown, read now.
func (h *Handler) markSynced(ctx context.Context, version accounts.ID, files []library.ExternalSubtitle, streams []playback.MediaStream, shown int) {
	if !h.Accounts.Settings().SyncAddonSubtitles {
		return
	}
	timings := h.Playback.SubtitleTimings(ctx, version)
	if _, known := h.Playback.Speech(ctx, version); known {
		for i, file := range files {
			if _, synced := timings[file.ID]; synced || !addonSubtitle(file) {
				continue
			}
			text, read := h.subtitleCache.Get(file.ID)
			if !read && i == shown {
				var err error
				text, err = h.subtitleFile(ctx, file)
				read = err == nil
			}
			if read {
				h.Playback.SyncSubtitle(ctx, version, file.ID, text.cues)
			}
		}
		timings = h.Playback.SubtitleTimings(ctx, version)
	}
	if len(timings) == 0 {
		return
	}
	language := h.Accounts.Settings().Language
	for i := range streams {
		stream := &streams[i]
		if stream.Type != "Subtitle" || !stream.IsExternal || stream.Index >= len(files) || !addonSubtitle(files[stream.Index]) {
			continue
		}
		if timing, ok := timings[files[stream.Index].ID]; ok && timing.Changes() {
			stream.DisplayTitle = playback.SyncedTitle(stream.DisplayTitle, language)
		}
	}
}

// subtitleVersion is the version a subtitle URL names by its media
// source: one listed earlier by its own identifier, else, for a signed-in
// caller, the version the title's identifier stands for.
func (h *Handler) subtitleVersion(r *http.Request, item accounts.ID) (accounts.ID, bool) {
	if wanted, ok := parseGUID(r.PathValue("mediaSourceId")); ok {
		if owner, ok := h.Library.VersionOwner(wanted); ok && owner == item {
			return wanted, true
		}
	}
	version, ok := h.sourceVersion(r, item)
	return version.ID, ok
}

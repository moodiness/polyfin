package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// download serves a version as a file to save, for the apps that keep
// titles for offline viewing: /Items/{itemId}/Download, where the item is a
// title or one of its versions. A title stands for the version its grant
// names, else for its first version that has not failed, as in item
// details and PlaybackInfo. It is served as the stream route serves it,
// redirected or relayed, under the version's file name.
//
// Jellyfin downloads only files on its own disks; Polyfin's versions are
// all remote, and download as its streams play. Jellyfin also refuses HEAD
// here, which Polyfin answers as it answers GET, as on its other routes.
func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	h.serveFile(w, r, true)
}

// itemFile serves a version as /Items/{itemId}/File does: as the download
// route does, under the same permissions, but to be read rather than
// saved, without a file name. Jellyfin serves any signed-in user the file
// on its disk; Polyfin's files are the addons' streams, which the download
// permissions guard.
func (h *Handler) itemFile(w http.ResponseWriter, r *http.Request) {
	h.serveFile(w, r, false)
}

// serveFile serves the version a request names, as a file to save when
// attachment is set.
func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, attachment bool) {
	opened, ok := parseGUID(r.PathValue("itemId"))
	if !ok {
		validationProblem(w, map[string][]string{"itemId": {notValid(r.PathValue("itemId"))}})
		return
	}
	user, grant, ok := h.streamAccess(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	// Like Jellyfin's download policy, refused before anything is looked
	// up, whatever the credentials.
	if !h.Accounts.MayDownload(user) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	item, err := h.Library.Item(r.Context(), user, opened)
	if errors.Is(err, library.ErrNotFound) {
		if owner, isVersion := h.Library.VersionOwner(opened); isVersion {
			item, err = h.Library.Item(r.Context(), user, owner)
		}
	}
	if errors.Is(err, library.ErrNotFound) {
		// A finished recording is a file of Polyfin's own, served as it
		// is, under the same permissions.
		if recording, ok := h.recordingTitle(r.Context(), user, opened); ok {
			h.serveRecordingFile(w, r, recording, attachment)
			return
		}
		notFoundProblem(w)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	wanted := opened
	if opened == item.ID {
		wanted = grant.Version
		if wanted == (accounts.ID{}) {
			wanted = h.firstWorkingVersion(r.Context(), user, item)
		}
	}
	version, err := h.Library.Version(r.Context(), user, item.ID, wanted)
	if err != nil {
		// Jellyfin refuses to download what has no file, a series or a
		// season, as a request it cannot process.
		processingError(w, http.StatusBadRequest)
		return
	}
	name := h.downloadName(r, item, version)
	delivery := playback.Delivery{
		// A player that authenticates with a header might send it to the
		// source too: relay instead of redirecting it.
		Relay:       grant.Relay || strings.Contains(r.Header.Get("Authorization"), "Token="),
		ContentType: mimeTypes[containerOfName(name)],
	}
	if attachment {
		delivery.Attachment = name
	}
	if err := h.Playback.Serve(w, r, version, delivery); err != nil && r.Context().Err() == nil {
		h.Logger.Warn("A download could not be served", "addon", version.Addon, "error", err)
	}
}

// firstWorkingVersion is the version a title's own identifier stands for:
// its first that has not recently failed, else the title's identifier,
// which stands for its first.
func (h *Handler) firstWorkingVersion(ctx context.Context, user accounts.User, item library.Item) accounts.ID {
	versions, err := h.Library.Versions(ctx, user, item.ID)
	if err != nil {
		return item.ID
	}
	if i := slices.IndexFunc(versions, func(v library.Version) bool { return !h.Playback.Failed(v.ID) }); i >= 0 {
		return versions[i].ID
	}
	return item.ID
}

// setDownload tells apps that a movie or an episode with versions can be
// downloaded, when they asked and the user may download, and with withPath
// names in Path the file its first version, the one it was opened as,
// downloads as.
func (h *Handler) setDownload(r *http.Request, user accounts.User, dto *BaseItemDto, item library.Item, versions []library.Version, withPath bool) {
	if len(versions) == 0 {
		return
	}
	if dto.CanDownload != nil && h.Accounts.MayDownload(user) {
		dto.CanDownload = new(true)
	}
	if withPath {
		dto.Path = h.downloadName(r, item, versions[0])
	}
}

// downloadName is the name a version is saved as: its file name, else one
// made from the title and the version's container.
func (h *Handler) downloadName(r *http.Request, item library.Item, version library.Version) string {
	if name := safeFileName(path.Base(strings.ReplaceAll(version.Filename, `\`, "/"))); name != "" && name != "." && name != "/" {
		return name
	}
	title := item.Name
	if item.Kind == library.KindEpisode {
		title = fmt.Sprintf("%s S%02dE%02d %s", item.SeriesName, item.ParentIndexNumber, item.IndexNumber, item.Name)
	}
	container := containerOfName(version.Filename)
	if analysis, ok := h.Playback.Analyzed(r.Context(), version.ID); ok {
		container = playback.DisplayContainer(analysis, version.Filename)
	}
	return safeFileName(strings.NewReplacer("/", "-", `\`, "-").Replace(title)) + "." + container
}

// safeFileName drops what a file name may not carry on the devices apps
// save to: control characters and quotes, which Jellyfin also drops.
func safeFileName(name string) string {
	return strings.TrimSpace(strings.Map(func(c rune) rune {
		if c < ' ' || c == 0x7f || c == '"' {
			return -1
		}
		return c
	}, name))
}

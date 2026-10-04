package jellyfin

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// Jellyfin apps search subtitle providers for a title in a language, and
// have Jellyfin download the one the user picks next to the media. Polyfin's
// providers are the user's subtitle addons, whose subtitles it already lists
// as external streams of every version of the title: a search shows them,
// and downloading one changes nothing.
//
// Jellyfin keeps these routes to users who may manage subtitles (see
// subtitleUploadRoutes). Polyfin serves them to every user, as before that
// permission existed: they list only what every version already offers.
// Apps show the search only to users who have the permission.

func (h *Handler) remoteSubtitleRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/Items/{itemId}/RemoteSearch/Subtitles/{language}", h.searchRemoteSubtitles)
	signedIn(http.MethodPost, "/Items/{itemId}/RemoteSearch/Subtitles/{subtitleId}", h.downloadRemoteSubtitle)
	signedIn(http.MethodGet, "/Providers/Subtitles/Subtitles/{subtitleId}", h.remoteSubtitleFile)
}

// RemoteSubtitleInfo is a subtitle a search found. Addons tell only a
// subtitle's language and file: the fields Jellyfin's providers fill from
// what they know of a file (author, rating, downloads, forced, hearing
// impaired) are left out, as Jellyfin leaves out unknown values.
type RemoteSubtitleInfo struct {
	ThreeLetterISOLanguageName string
	// Id names the title and the subtitle, never the addon's URLs.
	Id string
	// ProviderName is the name of the addon offering the subtitle.
	ProviderName string
	// Name is how the subtitle's stream is shown among the title's.
	Name string
	// Format is the format Polyfin serves addon subtitles in.
	Format string
	// IsHashMatch is false: addons are asked for a title's subtitles, not
	// for those made for a file.
	IsHashMatch bool
}

// remoteSubtitleID identifies an addon's subtitle for a title: the title's
// identifier, then the subtitle's, which is derived from the addon and the
// file without revealing either.
func remoteSubtitleID(item accounts.ID, subtitle library.ExternalSubtitle) string {
	return item.String() + subtitle.ID.String()
}

// parseRemoteSubtitleID reads the title and the subtitle a
// remoteSubtitleID names.
func parseRemoteSubtitleID(raw string) (item, subtitle accounts.ID, ok bool) {
	if len(raw) != 64 {
		return item, subtitle, false
	}
	item, errItem := accounts.ParseID(raw[:32])
	subtitle, errSubtitle := accounts.ParseID(raw[32:])
	return item, subtitle, errItem == nil && errSubtitle == nil
}

// searchRemoteSubtitles lists the subtitles the user's addons offer for a
// movie or an episode in a language, which apps give as an ISO 639-2/B or
// /T code or an ISO 639-1 code.
func (h *Handler) searchRemoteSubtitles(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	perfect, _ := b.bool(r, "isPerfectMatch")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user := callerFrom(r.Context()).User
	item, err := h.title(r.Context(), user, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	results := []RemoteSubtitleInfo{}
	// A perfect match is a subtitle made for the very file, which addons
	// asked by title never claim.
	if perfect {
		writeJSON(w, http.StatusOK, results)
		return
	}
	files, err := h.Library.Subtitles(r.Context(), user, item.ID)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	// The files users added are the title's already, and no provider's.
	files = slices.DeleteFunc(files, func(file library.ExternalSubtitle) bool { return file.Uploaded })
	// The streams the files become give the language and the name players
	// show for them.
	streams := playback.ExternalStreams(playable{subtitles: files}.externals(), h.Accounts.Settings().Language)
	language := r.PathValue("language")
	for i, file := range files {
		if !playback.SameLanguage(file.Language, language) {
			continue
		}
		results = append(results, RemoteSubtitleInfo{
			ThreeLetterISOLanguageName: streams[i].Language,
			Id:                         remoteSubtitleID(item.ID, file),
			ProviderName:               file.Addon,
			Name:                       streams[i].DisplayTitle,
			Format:                     "srt",
		})
	}
	writeJSON(w, http.StatusOK, results)
}

// downloadRemoteSubtitle is Jellyfin's download of a subtitle next to the
// media, after which it is one of the title's subtitles. Every subtitle the
// user's addons offer already is, as long as they offer it: the download
// only checks that the title's addons still do.
func (h *Handler) downloadRemoteSubtitle(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user := callerFrom(r.Context()).User
	item, err := h.title(r.Context(), user, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	of, subtitle, ok := parseRemoteSubtitleID(r.PathValue("subtitleId"))
	if !ok || of != item.ID {
		notFoundProblem(w)
		return
	}
	if _, err := h.remoteSubtitle(r.Context(), user, item.ID, subtitle); err != nil {
		h.browseError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// remoteSubtitleFile serves a subtitle a search found, as SubRip, the
// format the search gave.
func (h *Handler) remoteSubtitleFile(w http.ResponseWriter, r *http.Request) {
	item, subtitle, ok := parseRemoteSubtitleID(r.PathValue("subtitleId"))
	if !ok {
		notFoundProblem(w)
		return
	}
	file, err := h.remoteSubtitle(r.Context(), callerFrom(r.Context()).User, item, subtitle)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	text, err := h.subtitleFile(r.Context(), file)
	if err != nil {
		if r.Context().Err() == nil {
			h.Logger.Warn("A subtitle could not be read", "addon", file.Addon, "error", err)
		}
		processingError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-subrip")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(subtitles.SubRip(text.cues)))
}

// remoteSubtitle finds a subtitle among those the user's addons offer for a
// title, library.ErrNotFound when they no longer do.
func (h *Handler) remoteSubtitle(ctx context.Context, user accounts.User, item, subtitle accounts.ID) (library.ExternalSubtitle, error) {
	files, err := h.Library.Subtitles(ctx, user, item)
	if err != nil {
		return library.ExternalSubtitle{}, err
	}
	for _, file := range files {
		if file.ID == subtitle && !file.Uploaded {
			return file, nil
		}
	}
	return library.ExternalSubtitle{}, library.ErrNotFound
}

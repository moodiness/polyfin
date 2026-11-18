package jellyfin

import (
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// keptFromSource reports whether the app a request plays for is kept from
// a version's source: its user may not be told where the version comes
// from (see playback.Service.SendsToSource), so its bytes go through
// Polyfin whatever the request's grant asks. Every route that hands a
// version to playback.Service.Serve or ServeChannel asks here first,
// unless it always relays: the stream route, the download routes and the
// track route.
func (h *Handler) keptFromSource(r *http.Request, user accounts.User, version library.Version) bool {
	return !h.Playback.SendsToSource(r.Context(), user, version)
}

// namesSource answers a request of an app kept from a version's source
// when relaying the version as it is would tell it the source's addresses
// (see playback.Service.NamesSource), and reports whether it did: a
// manifest that would reach the app as its source wrote it is refused as
// a version that is not one file is, and a source whose first bytes
// cannot be read as one that does not answer. linked tells a route that
// names the files of an HLS playlist at Polyfin's own addresses (see
// playback.Delivery.Link), where such a playlist is relayed.
func (h *Handler) namesSource(w http.ResponseWriter, r *http.Request, version library.Version, linked bool) bool {
	names, err := h.Playback.NamesSource(r.Context(), version, linked)
	switch {
	case err != nil:
		http.Error(w, "source unavailable", http.StatusBadGateway)
		if r.Context().Err() == nil {
			h.Logger.Warn("A stream could not be served", "addon", version.AddonLog, "error", err)
		}
	case names:
		http.Error(w, "not one file", http.StatusNotFound)
	default:
		return false
	}
	return true
}

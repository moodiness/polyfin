package jellyfin

import (
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// lineupSources stands for each enabled stream of an IPTV channel in its
// details, best first, those of the user's quality group: a channel
// merging several entries offers them as versions, which PlaybackInfo
// plays by their identifiers, the first being the channel's. Listing them
// reads Polyfin's line-up only; a Stremio channel's streams would cost its
// addon requests, and keep the placeholder.
func (h *Handler) lineupSources(r *http.Request, user accounts.User, item library.Item, opened accounts.ID) []MediaSourceInfo {
	if !strings.HasPrefix(item.StremioID, iptv.IDPrefix) {
		return nil
	}
	versions, err := h.Library.Versions(r.Context(), user, item.ID)
	if err != nil {
		return nil
	}
	versions = h.inGroup(r.Context(), user, slices.DeleteFunc(versions, func(v library.Version) bool { return h.Playback.Failed(v.ID) }))
	sources := make([]MediaSourceInfo, 0, len(versions))
	at := openedIndex(opened, versions)
	for i, version := range versions {
		source := channelPlaceholder(item)
		source.Id, source.Name, source.ETag = sourceID(opened, version, i == at).String(), version.Name, version.ID.String()
		sources = append(sources, source)
	}
	return sources
}

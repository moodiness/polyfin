package jellyfin

import (
	"cmp"
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/library"
)

// Polyfin's jellyfin-web script shows the badges of a version (see
// library.VersionBadges) under a title page's Version menu, for the
// version picked, and in the playback controls, for the version played,
// while Settings › Web player › Version badges is on. It asks
// them of /Polyfin/Items/{itemId}/Badges and /Polyfin/Playback/Badges, in
// the language of jellyfin-web (?language=), the server language
// otherwise. The Version menu's tooltip names the version picked as its
// addon does, whatever the setting.

// versionBadgesJSON is a version as /Polyfin/Items/{itemId}/Badges lists
// it: its identifier as item details name its source, its ETag, the
// addon's name for it, and its badges.
type versionBadgesJSON struct {
	Id     string
	ETag   string
	Name   string
	Badges []library.Badge
}

// itemBadgesJSON answers /Polyfin/Items/{itemId}/Badges: whether the web
// player shows badges (Settings.VersionBadges), and the title's versions.
type itemBadgesJSON struct {
	Show     bool
	Versions []versionBadgesJSON
}

// playingBadgesJSON answers /Polyfin/Playback/Badges: whether the web
// player shows badges, and the version the caller's device plays: its
// title, the addon's name for it and its badges; empty when it plays
// none Polyfin knows.
type playingBadgesJSON struct {
	Show   bool
	ItemId string
	Name   string
	Badges []library.Badge
}

// itemBadges answers /Polyfin/Items/{itemId}/Badges for a title opened as
// itemId, its own identifier or one of its versions', with the checks of
// its details: the versions known now, as its details list them and name
// their sources, the badges of each in the language asked. An item
// without versions lists none.
func (h *Handler) itemBadges(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	opened := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	item, ok := h.versionedItem(w, r, user, opened)
	if !ok {
		return
	}
	answer := itemBadgesJSON{Show: h.Accounts.Settings().VersionBadges, Versions: []versionBadgesJSON{}}
	if versioned(item.Kind) {
		versions, err := h.Library.KnownVersions(r.Context(), user, item.ID)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		versions = h.offered(r.Context(), user, item, versions)
		at, language := openedIndex(opened, versions), h.badgeLanguage(r)
		for i, version := range versions {
			answer.Versions = append(answer.Versions, versionBadgesJSON{Id: sourceID(opened, version, i == at).String(),
				ETag: version.ID.String(), Name: version.Name, Badges: h.versionBadges(r.Context(), version, language)})
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

// playingBadges answers /Polyfin/Playback/Badges with the version the
// caller's own device plays (see playedVersion), the one the play went on
// with once another failed.
func (h *Handler) playingBadges(w http.ResponseWriter, r *http.Request) {
	c := callerFrom(r.Context())
	answer := playingBadgesJSON{Show: h.Accounts.Settings().VersionBadges, Badges: []library.Badge{}}
	if now, ok := h.sessions.Playing(c.Device.ID); ok && now.User == c.User.ID {
		if id, ok := h.playedVersion(c.Device.ID, now); ok {
			if switched, ok := h.Playback.SwitchedTo(now.PlaySessionID); ok && now.PlaySessionID != "" {
				id = switched.ID
			}
			if version, ok := h.Library.KnownVersion(id); ok {
				answer.ItemId, answer.Name = version.Item.String(), version.Name
				answer.Badges = h.versionBadges(r.Context(), version, h.badgeLanguage(r))
			}
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

// versionBadges are a version's badges in language, from what describes
// its file (see knownAnalyses), never nil.
func (h *Handler) versionBadges(ctx context.Context, version library.Version, language string) []library.Badge {
	if badges := library.VersionBadges(version, language, h.knownAnalyses(ctx, version)...); badges != nil {
		return badges
	}
	return []library.Badge{}
}

// badgeLanguage is the language badges are asked in (?language=, as
// jellyfin-web's document names it), the server language when Polyfin
// does not speak it.
func (h *Handler) badgeLanguage(r *http.Request) string {
	return cmp.Or(i18n.Match(query(r, "language")), h.Accounts.Settings().Language)
}

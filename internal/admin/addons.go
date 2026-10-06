package admin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

type addonJSON struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	Description  string    `json:"description"`
	Logo         *string   `json:"logo"`
	ManifestURL  string    `json:"manifestUrl"`
	Enabled      bool      `json:"enabled"`
	Resources    []string  `json:"resources"`
	Types        []string  `json:"types"`
	CatalogCount int       `json:"catalogCount"`
	RefreshedAt  time.Time `json:"refreshedAt"`
	// Kind is "stremio" for a Stremio addon, "eclipse" for an Eclipse
	// music addon, which Music then describes, or "m3u" or "xtream" for an
	// IPTV source, which Source then describes.
	Kind   string      `json:"kind"`
	Source *sourceJSON `json:"source"`
	Music  *musicJSON  `json:"music"`
}

// sourceJSON describes an IPTV source: its address, redacted, as it holds
// credentials; how many entries its list has, how it was last fetched and
// when it is fetched again; its import options and its line-up's counts.
type sourceJSON struct {
	Address   string      `json:"address"`
	Channels  int         `json:"channels"`
	CheckedAt *time.Time  `json:"checkedAt"`
	FetchedAt *time.Time  `json:"fetchedAt"`
	NextAt    *time.Time  `json:"nextAt"`
	Error     string      `json:"error"`
	Options   optionsJSON `json:"options"`
	Lineup    lineupJSON  `json:"lineup"`
	VOD       vodJSON     `json:"vod"`
}

type optionsJSON struct {
	Categories   string   `json:"categories"`
	Channels     string   `json:"channels"`
	Excluded     []string `json:"excluded"`
	NewChannels  bool     `json:"newChannels"`
	Numbering    string   `json:"numbering"`
	LiveTv       bool     `json:"liveTv"`
	Movies       bool     `json:"movies"`
	Series       bool     `json:"series"`
	VODExcluded  []string `json:"vodExcluded"`
	VODLibraries string   `json:"vodLibraries"`
	Enrichment   bool     `json:"enrichment"`
}

// vodJSON counts an IPTV source's movies and series (see iptv.VODCounts).
type vodJSON struct {
	Movies           int `json:"movies"`
	Series           int `json:"series"`
	Episodes         int `json:"episodes"`
	ShownMovies      int `json:"shownMovies"`
	ShownSeries      int `json:"shownSeries"`
	MovieCategories  int `json:"movieCategories"`
	SeriesCategories int `json:"seriesCategories"`
}

type lineupJSON struct {
	Categories        int `json:"categories"`
	EnabledCategories int `json:"enabledCategories"`
	Channels          int `json:"channels"`
	EnabledChannels   int `json:"enabledChannels"`
	ShownChannels     int `json:"shownChannels"`
	Mapped            int `json:"mapped"`
	Unmapped          int `json:"unmapped"`
}

func newAddonJSON(addon addons.Addon) addonJSON {
	var logo *string
	if addon.Manifest.Logo != "" {
		logo = &addon.Manifest.Logo
	}
	types, resources := addon.Manifest.Types, addon.Manifest.ResourceNames()
	if addon.Eclipse() {
		types, resources = addon.Music.Types, addon.Music.Resources
	}
	if types == nil {
		types = []string{}
	}
	manifestURL := stremio.RedactManifestURL(addon.ManifestURL)
	if !addon.Stremio() && !addon.Eclipse() {
		manifestURL = iptv.Redact(addon.ManifestURL)
	}
	return addonJSON{
		ID:           addon.ID.String(),
		Name:         addon.Manifest.Name,
		Version:      addon.Manifest.Version,
		Description:  addon.Manifest.Description,
		Logo:         logo,
		ManifestURL:  manifestURL,
		Enabled:      addon.Enabled,
		Resources:    resources,
		Types:        types,
		CatalogCount: len(addon.Manifest.Catalogs),
		RefreshedAt:  addon.RefreshedAt,
		Kind:         addon.Kind,
		Music:        newMusicJSON(addon),
	}
}

// addonJSON describes an addon, and an IPTV source's list.
func (h *handler) addonJSON(r *http.Request, scope addons.Scope, addon addons.Addon) (addonJSON, error) {
	result := newAddonJSON(addon)
	if addon.Stremio() || addon.Eclipse() {
		return result, nil
	}
	source, err := h.IPTV.Source(r.Context(), scope, addon.ID)
	if err != nil {
		return addonJSON{}, err
	}
	o, n, vod := source.Options, source.Lineup, source.VOD
	result.Source = &sourceJSON{Address: result.ManifestURL, Channels: source.Channels, CheckedAt: source.CheckedAt, FetchedAt: source.FetchedAt,
		NextAt: source.NextAt, Error: source.Error,
		Options: optionsJSON{Categories: o.Categories, Channels: o.Channels, Excluded: o.Excluded, NewChannels: o.NewChannels, Numbering: o.Numbering,
			LiveTv: o.LiveTv, Movies: o.Movies, Series: o.Series, VODExcluded: o.VODExcluded, VODLibraries: o.VODLibraries, Enrichment: o.Enrichment},
		Lineup: lineupJSON{Categories: n.Categories, EnabledCategories: n.EnabledCategories, Channels: n.Channels, EnabledChannels: n.EnabledChannels,
			ShownChannels: n.ShownChannels, Mapped: n.Mapped, Unmapped: n.Unmapped},
		VOD: vodJSON(vod)}
	return result, nil
}

type libraryJSON struct {
	AddonID     string  `json:"addonId"`
	AddonName   string  `json:"addonName"`
	CatalogType string  `json:"catalogType"`
	CatalogID   string  `json:"catalogId"`
	CatalogName string  `json:"catalogName"`
	Name        *string `json:"name"`
	// AppName is the name Jellyfin apps show, which tells apart libraries
	// with the same name; nil when apps do not show the library.
	AppName   *string `json:"appName"`
	Enabled   bool    `json:"enabled"`
	Browsable bool    `json:"browsable"`
	// ItemID is the library's item in Jellyfin apps, null for a catalog
	// that is not an enabled library. Image is how it finds the image apps
	// show on its tile: "none", "automatic", or "custom" when one was
	// uploaded for it; ImageTag tags that image, served as the item's
	// Primary image, null when it shows none.
	ItemID   *string `json:"itemId"`
	Image    string  `json:"image"`
	ImageTag *string `json:"imageTag"`
	// Guide is the first XMLTV guide of an enabled live TV catalog (an
	// empty address when it has none), with the catalog's channels and
	// those mapped; Guides are all of them. Both are null for any other
	// library.
	Guide  *libraryGuideJSON `json:"guide"`
	Guides []guideJSON       `json:"guides"`
}

// libraryGuideJSON describes a live TV catalog's first XMLTV guide, as
// v0.8 did: its address, redacted as it may embed credentials (empty when
// it has none), its last download, and the catalog's channels and those
// mapped to a guide channel.
type libraryGuideJSON struct {
	URL       string     `json:"url"`
	CheckedAt *time.Time `json:"checkedAt"`
	FetchedAt *time.Time `json:"fetchedAt"`
	Channels  int        `json:"channels"`
	Matched   int        `json:"matched"`
	Error     string     `json:"error"`
	// NextAt is when the guide is fetched again: the settings'
	// LiveTvRefreshHours after its last attempt, sooner after a failure,
	// within the 5 minutes the guides due are looked for.
	NextAt *time.Time `json:"nextAt"`
}

// guideJSON describes an XMLTV guide of a live TV catalog: its address,
// redacted, its last download and what it held.
type guideJSON struct {
	ID         string     `json:"id"`
	Position   int        `json:"position"`
	URL        string     `json:"url"`
	CheckedAt  *time.Time `json:"checkedAt"`
	FetchedAt  *time.Time `json:"fetchedAt"`
	NextAt     *time.Time `json:"nextAt"`
	Channels   int        `json:"channels"`
	Programmes int        `json:"programmes"`
	Error      string     `json:"error"`
}

func nextGuideFetch(guide addons.Guide, refreshHours int) *time.Time {
	// A failed download is tried again sooner (see iptv.Backoff).
	if at, ok := library.GuideRetry(guide.ID); ok {
		return &at
	}
	if guide.CheckedAt == nil {
		return nil
	}
	return new(guide.CheckedAt.Add(time.Duration(refreshHours) * time.Hour))
}

func newGuideJSON(guide addons.Guide, refreshHours int) guideJSON {
	return guideJSON{ID: guide.ID.String(), Position: guide.Position, URL: redactGuideURL(guide.URL), CheckedAt: guide.CheckedAt,
		FetchedAt: guide.FetchedAt, NextAt: nextGuideFetch(guide, refreshHours), Channels: guide.Channels, Programmes: guide.Programmes,
		Error: guide.Error}
}

func newGuidesJSON(guides []addons.Guide, refreshHours int) []guideJSON {
	if guides == nil {
		return nil
	}
	result := make([]guideJSON, 0, len(guides))
	for _, g := range guides {
		result = append(result, newGuideJSON(g, refreshHours))
	}
	return result
}

// newLibraryGuideJSON describes a library's first guide, nil for a library
// that is not an enabled live TV catalog.
func newLibraryGuideJSON(l addons.Library, refreshHours int) *libraryGuideJSON {
	if l.Guides == nil {
		return nil
	}
	result := &libraryGuideJSON{Channels: l.GuideChannels, Matched: l.GuideMapped}
	if len(l.Guides) > 0 {
		first := l.Guides[0]
		result.URL, result.CheckedAt, result.FetchedAt, result.Error = redactGuideURL(first.URL), first.CheckedAt, first.FetchedAt, first.Error
		result.NextAt = nextGuideFetch(first, refreshHours)
	}
	return result
}

// redactGuideURL keeps a guide address's scheme and host: its path, query
// and credentials may hold the user's.
func redactGuideURL(guideURL string) string {
	if guideURL == "" {
		return ""
	}
	parsed, err := url.Parse(guideURL)
	if err != nil {
		return "…"
	}
	if (parsed.Path == "" || parsed.Path == "/") && parsed.RawQuery == "" && parsed.User == nil {
		return parsed.Scheme + "://" + parsed.Host + "/"
	}
	return parsed.Scheme + "://" + parsed.Host + "/…"
}

// writeLibraries answers a scope's libraries with the names apps show
// them under. A user's own libraries follow the server's when they use
// them, as in their apps; the server's are named as for a user without
// libraries of their own. Live TV catalogs list channels, not a library:
// they have no such name.
func (h *handler) writeLibraries(w http.ResponseWriter, r *http.Request, scope addons.Scope, libraries []addons.Library) {
	var shown []addons.Library
	// A scope with an owner is the caller's own (see scope).
	if scope.Owner != nil && sessionFrom(r.Context()).User.UseSharedAddons {
		list, err := h.Addons.Libraries(r.Context(), addons.Shared())
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		shown = slices.DeleteFunc(list, func(l addons.Library) bool {
			return !l.Enabled || !l.AddonActive || library.LiveCatalog(l.Catalog.Type)
		})
	}
	first := len(shown)
	var positions []int // libraries index of each shown library of the scope
	for i, l := range libraries {
		if l.Enabled && l.AddonActive && !library.LiveCatalog(l.Catalog.Type) {
			shown = append(shown, l)
			positions = append(positions, i)
		}
	}
	appNames := make([]*string, len(libraries))
	for i, name := range library.LibraryNames(shown, h.Accounts.Settings().Language)[first:] {
		appNames[positions[i]] = &name
	}
	var images []library.LibraryImage
	if h.LibraryImages != nil {
		var err error
		if images, err = h.LibraryImages.LibraryImages(r.Context(), scope, confined(r), libraries); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	result := make([]libraryJSON, 0, len(libraries))
	for i, l := range libraries {
		entry := libraryJSON{
			AddonID:     l.AddonID.String(),
			AddonName:   l.AddonName,
			CatalogType: l.Catalog.Type,
			CatalogID:   l.Catalog.ID,
			CatalogName: l.Catalog.Name,
			Name:        l.Name,
			AppName:     appNames[i],
			Enabled:     l.Enabled,
			Browsable:   l.Catalog.Browsable(),
			Image:       l.Image,
			Guide:       newLibraryGuideJSON(l, h.Accounts.Settings().LiveTvRefreshHours),
			Guides:      newGuidesJSON(l.Guides, h.Accounts.Settings().LiveTvRefreshHours),
		}
		if images != nil && images[i].ID != (accounts.ID{}) {
			entry.ItemID = new(images[i].ID.String())
			if images[i].Uploaded {
				entry.Image = "custom"
			}
			if images[i].URL != "" {
				entry.ImageTag = new(library.ImageTag(images[i].URL))
			}
		}
		result = append(result, entry)
	}
	writeJSON(w, http.StatusOK, result)
}

// scope resolves the {scope} path value: the server's addons for
// administrators, or the caller's own.
func (h *handler) scope(w http.ResponseWriter, r *http.Request) (addons.Scope, bool) {
	user := sessionFrom(r.Context()).User
	switch r.PathValue("scope") {
	case "shared":
		if !user.IsAdministrator {
			writeError(w, http.StatusForbidden, "forbidden")
			return addons.Scope{}, false
		}
		return addons.Shared(), true
	case "me":
		return addons.Personal(user.ID), true
	default:
		writeError(w, http.StatusNotFound, "not_found")
		return addons.Scope{}, false
	}
}

// confined reports whether the caller's addon requests must stay on public
// addresses: only administrators may make the server reach local networks.
func confined(r *http.Request) bool {
	return !sessionFrom(r.Context()).User.IsAdministrator
}

// personalAddonsRefused answers 403 personal_addons_disabled and reports
// true when scope is the caller's own addons and the server or the caller's
// own permission turned them off: such addons are kept, and may be turned
// off or removed, but not added, replaced, refreshed or turned on.
func (h *handler) personalAddonsRefused(w http.ResponseWriter, r *http.Request, scope addons.Scope) bool {
	if scope.Owner == nil || h.Accounts.Settings().PersonalAddonsAllowed(sessionFrom(r.Context()).User) {
		return false
	}
	writeError(w, http.StatusForbidden, "personal_addons_disabled")
	return true
}

// addonError answers the client-facing addon errors and reports whether err
// was one of them.
func addonError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{stremio.ErrInvalidManifestURL, http.StatusBadRequest, "invalid_manifest_url"},
		{stremio.ErrPrivateNetwork, http.StatusForbidden, "private_network"},
		{stremio.ErrInvalidManifest, http.StatusUnprocessableEntity, "invalid_manifest"},
		{stremio.ErrUnreachable, http.StatusBadGateway, "addon_unreachable"},
		{addons.ErrExists, http.StatusConflict, "addon_exists"},
		{addons.ErrNotFound, http.StatusNotFound, "not_found"},
		{addons.ErrInvalidOrder, http.StatusBadRequest, "invalid_order"},
		{addons.ErrInvalidLibrary, http.StatusBadRequest, "invalid_library"},
		{addons.ErrInvalidLibraryName, http.StatusBadRequest, "invalid_library_name"},
		{addons.ErrInvalidGuideURL, http.StatusBadRequest, "invalid_guide_url"},
		{addons.ErrNotStremio, http.StatusBadRequest, "invalid_request"},
		{addons.ErrNotEclipse, http.StatusBadRequest, "invalid_request"},
		{addons.ErrKindChanged, http.StatusUnprocessableEntity, "addon_kind_changed"},
		{eclipse.ErrInvalidSettings, http.StatusBadRequest, "invalid_addon_settings"},
		{iptv.ErrInvalidName, http.StatusBadRequest, "invalid_source_name"},
		{iptv.ErrInvalidAddress, http.StatusBadRequest, "invalid_source_address"},
		{iptv.ErrInvalidList, http.StatusUnprocessableEntity, "invalid_channel_list"},
		{iptv.ErrTooLarge, http.StatusUnprocessableEntity, "channel_list_too_large"},
		{iptv.ErrLoginRefused, http.StatusUnprocessableEntity, "iptv_login_refused"},
		{iptv.ErrInvalidOptions, http.StatusBadRequest, "invalid_options"},
		{iptv.ErrInvalidCategoryName, http.StatusBadRequest, "invalid_category_name"},
		{iptv.ErrCategoryNotCustom, http.StatusBadRequest, "category_not_custom"},
		{iptv.ErrInvalidOrder, http.StatusBadRequest, "invalid_order"},
		{iptv.ErrInvalidChannelName, http.StatusBadRequest, "invalid_channel_name"},
		{iptv.ErrInvalidLogo, http.StatusBadRequest, "invalid_logo"},
		{iptv.ErrInvalidDescription, http.StatusBadRequest, "invalid_description"},
		{iptv.ErrInvalidCategory, http.StatusBadRequest, "invalid_category"},
		{iptv.ErrInvalidNumber, http.StatusBadRequest, "invalid_number"},
		{iptv.ErrInvalidMove, http.StatusBadRequest, "invalid_move"},
		{iptv.ErrInvalidBulk, http.StatusBadRequest, "invalid_bulk"},
		{iptv.ErrInvalidStreams, http.StatusBadRequest, "invalid_streams"},
		{iptv.ErrInvalidStreamURL, http.StatusBadRequest, "invalid_stream_url"},
		// The admin app bounds labels: only a hand-made request is refused.
		{iptv.ErrInvalidStreamLabel, http.StatusBadRequest, "invalid_request"},
		{iptv.ErrStreamNotCustom, http.StatusBadRequest, "stream_not_custom"},
		{addons.ErrTooManyGuides, http.StatusBadRequest, "too_many_guides"},
		{addons.ErrInvalidGuide, http.StatusBadRequest, "invalid_guide"},
		{library.ErrInvalidMode, http.StatusBadRequest, "invalid_mode"},
		{library.ErrInvalidMapping, http.StatusBadRequest, "invalid_mapping"},
		{library.ErrNotFound, http.StatusNotFound, "not_found"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

func (h *handler) answerAddon(w http.ResponseWriter, r *http.Request, scope addons.Scope, status int, addon addons.Addon, err error) {
	if addonError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result, err := h.addonJSON(r, scope, addon)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, status, result)
}

func (h *handler) listAddons(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	installed, err := h.Addons.Addons(r.Context(), scope)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]addonJSON, 0, len(installed))
	for _, addon := range installed {
		described, err := h.addonJSON(r, scope, addon)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		result = append(result, described)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) installAddon(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	var body struct {
		ManifestURL string `json:"manifestUrl"`
	}
	if !decode(w, r, &body) {
		return
	}
	addon, err := h.Addons.Install(r.Context(), scope, body.ManifestURL, confined(r))
	if err == nil {
		h.Activity.AddonInstalled(r.Context(), sessionFrom(r.Context()).User, addon.Manifest.Name, addon.Manifest.Version, scope.Owner == nil)
	}
	h.answerAddon(w, r, scope, http.StatusCreated, addon, err)
}

func (h *handler) updateAddon(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Enabled     *bool   `json:"enabled"`
		ManifestURL *string `json:"manifestUrl"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.ManifestURL == nil && body.Enabled == nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if (body.ManifestURL != nil || (body.Enabled != nil && *body.Enabled)) && h.personalAddonsRefused(w, r, scope) {
		return
	}
	var addon addons.Addon
	var err error
	if body.ManifestURL != nil {
		addon, err = h.Addons.Replace(r.Context(), scope, id, *body.ManifestURL, confined(r))
	}
	if err == nil && body.Enabled != nil {
		addon, err = h.Addons.SetEnabled(r.Context(), scope, id, *body.Enabled)
	}
	h.answerAddon(w, r, scope, http.StatusOK, addon, err)
}

func (h *handler) refreshAddon(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	// An IPTV source fetches its channel list again; how it went is in the
	// answer.
	if _, err := h.IPTV.Source(r.Context(), scope, id); err == nil {
		err := h.IPTV.Refresh(context.WithoutCancel(r.Context()), scope, id, confined(r))
		addon, findErr := h.Addons.Find(r.Context(), id)
		h.answerAddon(w, r, scope, http.StatusOK, addon, errors.Join(err, findErr))
		return
	}
	addon, err := h.Addons.Refresh(r.Context(), scope, id, confined(r))
	h.answerAddon(w, r, scope, http.StatusOK, addon, err)
}

func (h *handler) removeAddon(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	// The addon is named in the activity log, which needs its name first.
	installed, err := h.Addons.Addons(r.Context(), scope)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	err = h.Addons.Remove(r.Context(), scope, id)
	if addonError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, addon := range installed {
		if addon.ID == id {
			h.Activity.AddonRemoved(r.Context(), sessionFrom(r.Context()).User, addon.Manifest.Name, scope.Owner == nil)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) reorderAddons(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	ids := make([]accounts.ID, 0, len(body.IDs))
	for _, raw := range body.IDs {
		id, err := accounts.ParseID(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_order")
			return
		}
		ids = append(ids, id)
	}
	err := h.Addons.Reorder(r.Context(), scope, ids)
	if addonError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) listLibraries(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	libraries, err := h.Addons.Libraries(r.Context(), scope)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeLibraries(w, r, scope, libraries)
}

func (h *handler) saveLibraries(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var body struct {
		Libraries []struct {
			AddonID     string  `json:"addonId"`
			CatalogType string  `json:"catalogType"`
			CatalogID   string  `json:"catalogId"`
			Name        *string `json:"name"`
		} `json:"libraries"`
	}
	if !decode(w, r, &body) {
		return
	}
	choices := make([]addons.LibraryChoice, 0, len(body.Libraries))
	for _, library := range body.Libraries {
		id, err := accounts.ParseID(library.AddonID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_library")
			return
		}
		choices = append(choices, addons.LibraryChoice{AddonID: id, CatalogType: library.CatalogType,
			CatalogID: library.CatalogID, Name: library.Name})
	}
	libraries, err := h.Addons.SetLibraries(r.Context(), scope, choices)
	if addonError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeLibraries(w, r, scope, libraries)
}

type guideRequest struct {
	AddonID     string `json:"addonId"`
	CatalogType string `json:"catalogType"`
	CatalogID   string `json:"catalogId"`
	URL         string `json:"url"`
}

// guideTarget reads the catalog a guide request is for.
func guideTarget(w http.ResponseWriter, body guideRequest) (addons.LibraryKey, bool) {
	id, err := accounts.ParseID(body.AddonID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_library")
		return addons.LibraryKey{}, false
	}
	return addons.LibraryKey{AddonID: id, CatalogType: body.CatalogType, CatalogID: body.CatalogID}, true
}

// saveGuide sets the XMLTV guide address of one of the scope's enabled live
// TV catalogs, an empty one removing it, and fetches the guide at once. It
// answers the scope's libraries, which tell how the fetch went.
func (h *handler) saveGuide(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var body guideRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := guideTarget(w, body)
	if !ok || (body.URL != "" && h.personalAddonsRefused(w, r, scope)) {
		return
	}
	_, err := h.Addons.SetGuide(r.Context(), scope, key, body.URL)
	if err == nil && body.URL != "" {
		// A turned-off addon's guide is kept, and fetched once it is on.
		if err = h.fetchGuide(r, scope, key); errors.Is(err, addons.ErrInvalidLibrary) {
			err = nil
		}
	}
	h.answerGuide(w, r, scope, err)
}

// refreshGuide fetches the XMLTV guide of one of the scope's live TV
// catalogs now, answering the scope's libraries.
func (h *handler) refreshGuide(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	var body guideRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := guideTarget(w, body)
	if !ok {
		return
	}
	h.answerGuide(w, r, scope, h.fetchGuide(r, scope, key))
}

// fetchGuide fetches a guide for a request, to its end even when the admin
// app leaves: the download is worth keeping.
func (h *handler) fetchGuide(r *http.Request, scope addons.Scope, key addons.LibraryKey) error {
	if h.Guides == nil {
		return errors.New("no guide refresher")
	}
	return h.Guides.RefreshGuide(context.WithoutCancel(r.Context()), scope, key)
}

// answerGuide answers a guide request with the scope's libraries.
func (h *handler) answerGuide(w http.ResponseWriter, r *http.Request, scope addons.Scope, err error) {
	if addonError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	libraries, err := h.Addons.Libraries(r.Context(), scope)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeLibraries(w, r, scope, libraries)
}

// addonPreferencesJSON is whether a user sees the server's addons.
// ParentalControl is set while the user's parental control or blocked
// genres hide titles: they then see the server's addons, and only them, in
// their apps.
// PersonalAddons is unset while the server or the user's own permission
// turned their own addons off: they then see the server's addons, and only
// them, too.
type addonPreferencesJSON struct {
	UseSharedAddons bool `json:"useSharedAddons"`
	ParentalControl bool `json:"parentalControl"`
	PersonalAddons  bool `json:"personalAddons"`
}

func (h *handler) addonPreferences(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	restricted := user.Restricted()
	personal := h.Accounts.Settings().PersonalAddonsAllowed(user)
	writeJSON(w, http.StatusOK, addonPreferencesJSON{UseSharedAddons: user.UseSharedAddons || restricted || !personal, ParentalControl: restricted,
		PersonalAddons: personal})
}

func (h *handler) saveAddonPreferences(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UseSharedAddons bool `json:"useSharedAddons"`
	}
	if !decode(w, r, &body) {
		return
	}
	user := sessionFrom(r.Context()).User
	// The server's addons give the ratings and genres that parental control
	// and blocked genres hide titles by.
	if user.Restricted() && !body.UseSharedAddons {
		writeError(w, http.StatusConflict, "parental_control")
		return
	}
	if _, err := h.Accounts.UpdateUser(r.Context(), user.ID, accounts.UserChanges{UseSharedAddons: &body.UseSharedAddons}, nil); err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, addonPreferencesJSON{UseSharedAddons: body.UseSharedAddons, ParentalControl: user.Restricted(),
		PersonalAddons: h.Accounts.Settings().PersonalAddonsAllowed(user)})
}

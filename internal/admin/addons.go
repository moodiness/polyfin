package admin

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
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
}

func newAddonJSON(addon addons.Addon) addonJSON {
	var logo *string
	if addon.Manifest.Logo != "" {
		logo = &addon.Manifest.Logo
	}
	types := addon.Manifest.Types
	if types == nil {
		types = []string{}
	}
	return addonJSON{
		ID:           addon.ID.String(),
		Name:         addon.Manifest.Name,
		Version:      addon.Manifest.Version,
		Description:  addon.Manifest.Description,
		Logo:         logo,
		ManifestURL:  stremio.RedactManifestURL(addon.ManifestURL),
		Enabled:      addon.Enabled,
		Resources:    addon.Manifest.ResourceNames(),
		Types:        types,
		CatalogCount: len(addon.Manifest.Catalogs),
		RefreshedAt:  addon.RefreshedAt,
	}
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
}

// writeLibraries answers a scope's libraries with the names apps show
// them under. A user's own libraries follow the server's when they use
// them, as in their apps; the server's are named as for a user without
// libraries of their own. Live TV catalogs list channels, not a library:
// they have no such name.
func (h *handler) writeLibraries(w http.ResponseWriter, r *http.Request, scope addons.Scope, libraries []addons.Library) {
	var shown []addons.Library
	if scope.Owner != nil {
		shared, err := h.Addons.UsesSharedAddons(r.Context(), *scope.Owner)
		if err == nil && shared {
			var list []addons.Library
			list, err = h.Addons.Libraries(r.Context(), addons.Shared())
			shown = slices.DeleteFunc(list, func(l addons.Library) bool {
				return !l.Enabled || !l.AddonActive || library.LiveCatalog(l.Catalog.Type)
			})
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
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
	result := make([]libraryJSON, 0, len(libraries))
	for i, l := range libraries {
		result = append(result, libraryJSON{
			AddonID:     l.AddonID.String(),
			AddonName:   l.AddonName,
			CatalogType: l.Catalog.Type,
			CatalogID:   l.Catalog.ID,
			CatalogName: l.Catalog.Name,
			Name:        l.Name,
			AppName:     appNames[i],
			Enabled:     l.Enabled,
			Browsable:   l.Catalog.Browsable(),
		})
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
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

func (h *handler) answerAddon(w http.ResponseWriter, r *http.Request, status int, addon addons.Addon, err error) {
	if addonError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, status, newAddonJSON(addon))
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
		result = append(result, newAddonJSON(addon))
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
	h.answerAddon(w, r, http.StatusCreated, addon, err)
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
	h.answerAddon(w, r, http.StatusOK, addon, err)
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
	addon, err := h.Addons.Refresh(r.Context(), scope, id, confined(r))
	h.answerAddon(w, r, http.StatusOK, addon, err)
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
	err := h.Addons.Remove(r.Context(), scope, id)
	if addonError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
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
	uses, err := h.Addons.UsesSharedAddons(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	restricted := user.Restricted()
	personal := h.Accounts.Settings().PersonalAddonsAllowed(user)
	writeJSON(w, http.StatusOK, addonPreferencesJSON{UseSharedAddons: uses || restricted || !personal, ParentalControl: restricted, PersonalAddons: personal})
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
	if err := h.Addons.SetUsesSharedAddons(r.Context(), user.ID, body.UseSharedAddons); err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, addonPreferencesJSON{UseSharedAddons: body.UseSharedAddons, ParentalControl: user.Restricted(),
		PersonalAddons: h.Accounts.Settings().PersonalAddonsAllowed(user)})
}

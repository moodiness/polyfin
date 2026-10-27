package silo

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
)

// What the server offers: the documents the apps read to show or hide
// their features, the server's name and addresses.
//
// Silo's contract answers each optional feature's discovery with a
// document whose state says whether it is available; the apps hide a
// feature whose state is not, as they do one whose discovery fails. Polyfin
// answers available for what it serves, unsupported for the rest, and
// refuses the operations of a feature it does not serve as the contract
// refuses those of a server that cannot provide them: 501
// capability_unsupported.

func (s *Server) probeRoutes() {
	s.mux.HandleFunc("GET /api/v2/theme/branding", s.branding)
	s.mux.HandleFunc("GET /api/v2/system/connections", s.signedIn(s.connections))
	s.mux.HandleFunc("GET /api/v2/images/capabilities", s.signedIn(s.imageCapabilities))
	s.mux.HandleFunc("GET /api/v2/settings/overlay-config", s.signedIn(s.overlayConfig))
	s.mux.HandleFunc("GET /api/v2/auth/providers", s.authProviders)
	for path, doc := range unsupportedDocuments {
		handler := func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, doc) }
		if strings.HasPrefix(path, "/api/v2/auth/") || path == "/api/v2/invitations/capabilities" {
			// Read before signing in.
			s.mux.HandleFunc("GET "+path, handler)
		} else {
			s.mux.HandleFunc("GET "+path, s.signedIn(func(w http.ResponseWriter, r *http.Request, _ viewer) { handler(w, r) }))
		}
	}
	for _, pattern := range unsupportedOperations {
		s.mux.HandleFunc(pattern, s.signedIn(unsupported("This server does not offer this feature to Silo's apps.")))
	}
	s.mux.HandleFunc("PUT /api/v2/home/dismissals/{surface}/{id}", s.profiled(s.dismiss))
}

// unsupported answers an operation of a feature Polyfin does not serve.
func unsupported(detail string) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, _ *http.Request, _ viewer) {
		problem(w, http.StatusNotImplemented, "capability_unsupported", detail)
	}
}

// revision is the revision of the documents Polyfin answers: they change
// only with Polyfin.
const revision = "polyfin-1"

// unsupportedDocuments are the discovery documents of the features Silo's
// apps offer that Polyfin does not: downloads, AI translations and
// subtitles, books, diagnostics uploads, push notifications and the
// notification inbox, remote control, subtitle searches and their timing,
// requests, external sign-in, invitations, watch-together, theme music,
// settings synced from the apps and the apps' onboarding tour. Each holds
// what the contract requires, all off.
var unsupportedDocuments = map[string]map[string]any{
	"/api/v2/capabilities/downloads": {"bounded_creation": false, "subscription_mutations": false, "bounded_subscription_sync": false,
		"subscription_reads": false, "bounded_manifests": false, "file_delivery": false, "enabled": false, "download_allowed": false,
		"proxy_delivery": false, "ordered_status": false, "quality_presets": []any{}, "quality_options": []any{}, "transcode_enabled": false,
		"transcode_user_allowed": false, "season_download": false, "series_monitoring": false, "monitoring_modes": []any{},
		"bulk_quality": false, "monitor_quality": false, "preparation_progress": false, "direct_download_links": false,
		"prepare_again": false, "revoked_removes_local": false},
	"/api/v2/capabilities/metadata-ai": {"on_view": "off"},
	"/api/v2/capabilities/ebooks": {"guarded_annotations": false, "reader_files": false, "guarded_config": false, "ordered_progress": false,
		"kindle_conversion": false, "source_formats": []any{}, "served_format": "", "header": "", "header_failed_value": ""},
	"/api/v2/diagnostics/capabilities": {"status": "", "server_instance_id": "", "accepted_schema_versions": []any{},
		"max_bundle_bytes": 0, "max_manifest_bytes": 0, "retention_days": 0, "consent_notice_version": 0, "upload_chunk_bytes": 0},
	"/api/v2/devices/push/apple/capabilities":         {"registration_available": false},
	"/api/v2/notifications/push/devices/capabilities": {"registration_available": false, "platforms": []any{}},
	"/api/v2/notifications/capabilities": {"in_app": map[string]any{"enabled": false},
		"apple_push":   map[string]any{"available": false, "provider": "", "supported_modes": []any{}},
		"android_push": map[string]any{"available": false, "provider": "", "supported_modes": []any{}},
		"web_push":     map[string]any{"available": false},
		"webhooks":     map[string]any{"available": false, "max_per_profile": 0, "supported_types": []any{}},
		"email":        map[string]any{"available": false, "modes": []any{}, "digest_hour": 0},
		"discord":      map[string]any{"available": false, "modes": []any{}, "digest_hour": 0},
		"series":       map[string]any{"available": false, "mute": false}},
	"/api/v2/playback/sessions/control/capabilities": {"available": false, "protocol": ""},
	"/api/v2/subtitles/providers/status":             {"schema_version": 1, "enabled": false, "providers": []any{}},
	"/api/v2/subtitles/ai/status":                    {"enabled": false, "transcribe_enabled": false},
	"/api/v2/subtitles/sync/status":                  {"auto_sync": false, "external": false},
	"/api/v2/requests/status": {"requests_enabled": false, "rating_restrictions_enforced": false, "follow_supported": false,
		"season_requests_supported": false, "missing_seasons_requestable": false, "download_progress_supported": false,
		"watchlist_titles_supported": false, "watchlist_requests": false},
	"/api/v2/auth/external-sign-in/capabilities": {"available": false, "identities": false, "admin_identities": false, "break_glass": false,
		"connection_test": false, "live_provider_changes": false, "provider_recheck": false, "credentials_linking": false,
		"network_sign_in": false, "network_link_keeps_password": false},
	"/api/v2/auth/oauth/capabilities":  {"available": false, "native": false, "linking": false, "provider_logout": false, "select_account": false},
	"/api/v2/invitations/capabilities": {"default_profile": false, "profileless": false},
	"/api/v2/watch-together/capabilities": {"staged_selection": false, "lobby_ready": false, "connection_replaced": false,
		"selection_mode_switch": false, "member_state": false, "picker": false, "vote_host_override": false, "stop_playback": false,
		"max_member_state_ids": 0, "socket_protocol": ""},
	"/api/v2/catalog/themes/capabilities": {"delivery": "local_direct_play", "transcode": false, "cluster_routing": false,
		"grant_lifetime_seconds": 0},
	"/api/v2/settings/contract/capabilities": {"api_version": 0, "manifest_revision": 0, "contract_etag": "", "definition_count": 0,
		"scopes": []any{}, "client_families": []any{}, "supports_batched_effective": false, "supports_idempotent_writes": false,
		"supports_atomic_shortcuts": false, "supports_device_rename": false},
	"/api/v2/onboarding/capabilities": {"available": false, "revision_guarded": false},
}

// publicDocuments are the documents read before signing in, which carry
// no allowed: there is no user to allow.
var publicDocuments = []string{"/api/v2/auth/external-sign-in/capabilities", "/api/v2/auth/oauth/capabilities",
	"/api/v2/invitations/capabilities"}

func init() {
	for path, doc := range unsupportedDocuments {
		doc["state"], doc["revision"] = "unsupported", revision
		if !slices.Contains(publicDocuments, path) {
			doc["allowed"] = false
		}
	}
}

// unsupportedOperations are the operations the apps call regardless of a
// discovery document, of features Polyfin does not serve: settings synced
// from the apps, and the onboarding tour, which the apps skip on this
// answer.
var unsupportedOperations = []string{
	"GET /api/v2/settings/values/effective", "POST /api/v2/settings/values/effective", "GET /api/v2/settings/values",
	"GET /api/v2/settings/values/{key}", "PUT /api/v2/settings/values/{key}", "DELETE /api/v2/settings/values/{key}",
	"PUT /api/v2/settings/values/nav.shortcuts/item", "GET /api/v2/settings/contract",
	"GET /api/v2/onboarding/state", "GET /api/v2/onboarding/flow", "PUT /api/v2/onboarding/progress",
}

// branding names the server, before signing in: the apps show it.
func (s *Server) branding(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		ServerName       string `json:"server_name"`
		LoginSubtitle    string `json:"login_subtitle"`
		StorageAvailable bool   `json:"storage_available"`
	}{cmp.Or(s.options.Accounts.Settings().ServerName, "Polyfin"), "", false})
}

type endpoint struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// connections lists the addresses the server is reached at, which the
// administrator gave in its settings: its public address, then its address
// on the local network. Without either, it lists none: the apps keep the
// address they use.
func (s *Server) connections(w http.ResponseWriter, _ *http.Request, _ viewer) {
	settings := s.options.Accounts.Settings()
	endpoints := []endpoint{}
	for _, address := range []string{settings.PublicAddress, settings.LocalAddress} {
		if address != "" && !slices.ContainsFunc(endpoints, func(e endpoint) bool { return e.URL == address }) {
			endpoints = append(endpoints, endpoint{Kind: "public", URL: address})
		}
	}
	writeJSON(w, http.StatusOK, struct {
		State     string            `json:"state"`
		Allowed   bool              `json:"allowed"`
		Revision  string            `json:"revision"`
		ServerID  string            `json:"server_id"`
		Current   map[string]string `json:"current"`
		Endpoints []endpoint        `json:"endpoints"`
	}{"available", true, revision, s.options.ServerID, map[string]string{"kind": "default"}, endpoints})
}

// artworkSlots are the artwork of the answers whose width the apps may
// ask, each by its own parameter (see artworkWidth).
var artworkSlots = []string{"poster", "backdrop", "logo", "still", "photo"}

// imageCapabilities tells the apps the artwork sizes they may ask: Polyfin
// resizes artwork to the width asked, rounded up to the widths it keeps
// (see the Jellyfin API's images), as the image sizes small, medium and
// large, or the width of a slot.
func (s *Server) imageCapabilities(w http.ResponseWriter, _ *http.Request, _ viewer) {
	widths := map[string]map[string]int{}
	params := map[string]string{}
	for _, slot := range artworkSlots {
		widths[slot] = map[string]int{"small": artworkWidths["small"], "medium": artworkWidths["medium"], "large": artworkWidths["large"]}
		params[slot] = slot + "_width"
	}
	writeJSON(w, http.StatusOK, struct {
		State                  string                    `json:"state"`
		Allowed                bool                      `json:"allowed"`
		Revision               string                    `json:"revision"`
		Param                  string                    `json:"param"`
		Sizes                  []string                  `json:"sizes"`
		Widths                 map[string]map[string]int `json:"widths"`
		OriginalMaxWidth       int                       `json:"original_max_width_px"`
		SlotWidthParams        map[string]string         `json:"slot_width_params"`
		SeasonListArtworkParam string                    `json:"season_list_artwork_param"`
		EpisodeStillProvenance bool                      `json:"episode_still_provenance"`
		ExtendedItemArtwork    bool                      `json:"extended_item_artwork"`
		Delivery               string                    `json:"delivery"`
		StorageBackend         string                    `json:"storage_backend"`
	}{"available", true, revision, "image_size", []string{"small", "medium", "large", "original"}, widths, maxArtworkWidth, params,
		"include_artwork", true, false, "server", "local"})
}

// authProviders tells the apps how users sign in: with a password, there
// being no other provider. A TV also signs in with a code (see
// deviceLoginRoutes).
func (s *Server) authProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Items         []struct{} `json:"items"`
		PasswordLogin bool       `json:"password_login"`
	}{[]struct{}{}, true})
}

// dismiss takes a card off Continue Watching: its resume point goes, as
// the web player's "Remove from Continue Watching" takes it off. Polyfin
// keeps no way to take a series off Next Up but playing it.
func (s *Server) dismiss(w http.ResponseWriter, r *http.Request, v viewer) {
	switch r.PathValue("surface") {
	case resumeSection:
		var body struct{}
		if r.ContentLength != 0 && !decode(w, r, &body) {
			return
		}
		i, ok := s.itemOf(w, r, v, r.PathValue("id"))
		if !ok {
			return
		}
		a, err := s.call(r, v, http.MethodDelete, "/Polyfin/UserItems/"+i.Id+"/Resume", nil, nil)
		switch {
		case err != nil:
			s.internalError(w, r, err)
		case a.status == http.StatusBadRequest:
			// Nothing to resume: the card is gone already.
			w.WriteHeader(http.StatusNoContent)
		case !a.ok():
			s.jellyfinRefusal(w, r, a)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	case nextUpSection:
		problem(w, http.StatusNotImplemented, "capability_unsupported", "This server takes series off Next Up only once they are played.")
	default:
		validationProblem(w, "path.surface", "unsupported", "surface is continue_watching or next_up.")
	}
}

// overlayConfig tells the apps the server's defaults for the badges and
// quick actions of their cards: off, Polyfin's cards carrying no badges;
// the mode quick actions take once a user turns them on is the contract's
// first, both favorite and watched.
func (s *Server) overlayConfig(w http.ResponseWriter, _ *http.Request, _ viewer) {
	writeJSON(w, http.StatusOK, struct {
		Enabled             bool   `json:"enabled"`
		QuickActionsEnabled bool   `json:"quick_actions_enabled"`
		QuickActionsDefault string `json:"quick_actions_default"`
	}{false, false, "both"})
}

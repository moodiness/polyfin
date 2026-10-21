package admin

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/diskspace"
	"github.com/moodiness/polyfin/internal/https"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/notifications"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/torrent"
)

// Below this much free space, or this share of the disk, a folder's disk is
// short of room.
const (
	lowDiskBytes = 2_000_000_000
	lowDiskShare = 0.05
)

// lowOnSpace reports whether a disk with free and used bytes is short of
// room; free is -1 when the disk could not be measured.
func lowOnSpace(free, used int64) bool {
	return free >= 0 && (free < lowDiskBytes || float64(free) < float64(free+used)*lowDiskShare)
}

// The codes of the problems System › Health shows as needing attention.
const (
	problemDatabase          = "database"
	problemDisk              = "disk"
	problemAddon             = "addon"
	problemConversionsFull   = "conversions_full"
	problemThumbnailsPaused  = "thumbnails_paused"
	problemSecretsUnreadable = "secrets_unreadable"
	problemSecretsPlaintext  = "secrets_plaintext"
	problemBackupFailed      = "backup_failed"
	problemBackupStale       = "backup_stale"
	// A destination's last upload failed, or its newest backup is old.
	problemDestinationFailed = "backup_destination_failed"
	problemDestinationStale  = "backup_destination_stale"
	problemIPTV              = "iptv"
	// A tuner does not answer, or refused a stream for another reason
	// than its tuners being in use.
	problemTuner             = "tuner"
	problemFolder            = "folder"
	problemFolderUnwatched   = "folder_unwatched"
	problemGuide             = "guide"
	problemTask              = "task"
	problemUnknownProxy      = "unknown_proxy"
	problemDiscoveryPort     = "discovery_port"
	problemDiscoveryAddress  = "discovery_address"
	problemDLNAPort          = "dlna_port"
	problemProxyName         = "proxy_name_unresolved"
	problemConnectionStopped = "connection_stopped"
	// HTTPS cannot listen on its port; its certificate files are missing,
	// unreadable or mismatched; the last attempt to obtain or renew the
	// certificate of Let's Encrypt failed; the certificate served ends
	// within https.ExpiresSoon.
	problemHTTPSPort    = "https_port"
	problemHTTPSFiles   = "https_files"
	problemHTTPSRenewal = "https_renewal"
	problemHTTPSExpiry  = "https_expiry"
	// Picture subtitles in a language Tesseract has no data for are not
	// read into text.
	problemOCRLanguage = "subtitle_ocr_language"
	// Torrent streams do not play: their route cannot carry them, Failure
	// tells why (a torrent.RouteError's kind, or "no_route") and Name the
	// interface or the proxy; or their proxy offers no UDP ASSOCIATE, so
	// UDP trackers and the DHT are off; or a torrent did not start in time
	// lately, Failure telling why (see torrentStartFailure).
	problemTorrentRoute = "torrent_route"
	problemTorrentUDP   = "torrent_udp"
	problemTorrentStart = "torrent_start"
	// Every metadata addon of the server has a language: Name and To are
	// the first one's, whose Language of its descriptions should be None.
	problemMetadataLanguages = "metadata_languages"
	// A local folder's files are left unmatched as their titles'
	// searches failed: Addon names the addon that failed most of them,
	// empty for another failure, and Failure tells why (see
	// searchFailureJSON).
	problemFolderSearch = "folder_search"
)

// healthProblemJSON is something System › Health shows as needing
// attention. Key names it for as long as it lasts; Code tells what it is,
// with the fields it fills: Folder and Free (bytes) for a disk; Name and
// Owner (nil for the server's) for an addon, IPTV source, local folder or
// guide, Name alone for a host name of Known proxies that resolves to
// nothing or for the server's metadata addon whose language should be
// None, with Failure, the failure's code, for an addon or a local folder,
// or why a local folder is not watched, and Share, the kind of network
// share a folder is ("smb", "webdav"); Host for paused thumbnails; Address
// for an address that sends forwarded headers without being a known
// proxy, or for a container's address discovery announces; Port for the
// UDP port discovery cannot listen on, or the TCP port HTTPS cannot; Task,
// in the language asked, for a task; Secrets for those the key cannot
// decrypt; Kind ("app" or "tracking"), Name, the app or service as the API
// names it, and Owner, its user, for a stopped connection; Failure
// ("missing", "unreadable", "mismatch") and File for certificate files;
// Error, Let's Encrypt's, for a renewal that failed; Expires for a
// certificate close to its end, an error once it ended; Addon, with Name
// and Failure, for a local folder whose searches failed. To is the
// admin app's page that describes it, with its anchor. Transient ones are
// not told as health problems: they come and go with the load, or, for
// stopped connections, the connection_stopped event told them already.
type healthProblemJSON struct {
	Key       string           `json:"key"`
	Code      string           `json:"code"`
	Tone      string           `json:"tone"`
	To        string           `json:"to"`
	Transient bool             `json:"transient"`
	Folder    string           `json:"folder,omitempty"`
	Free      *int64           `json:"free,omitempty"`
	Name      string           `json:"name,omitempty"`
	Owner     *ownerJSON       `json:"owner,omitempty"`
	Failure   string           `json:"failure,omitempty"`
	Addon     string           `json:"addon,omitempty"`
	Share     string           `json:"share,omitempty"`
	Host      string           `json:"host,omitempty"`
	Address   string           `json:"address,omitempty"`
	Port      int              `json:"port,omitempty"`
	Task      string           `json:"task,omitempty"`
	Secrets   []unreadableJSON `json:"secrets,omitempty"`
	Kind      string           `json:"kind,omitempty"`
	File      string           `json:"file,omitempty"`
	Error     string           `json:"error,omitempty"`
	Expires   *time.Time       `json:"expires,omitempty"`
}

// healthProblemsRoute answers the problems System › Health shows, errors
// first, with the tasks named in the language asked, the server's by
// default.
func (h *handler) healthProblemsRoute(w http.ResponseWriter, r *http.Request) {
	language := r.URL.Query().Get("language")
	if language == "" {
		language = h.Accounts.Settings().Language
	}
	problems, err := h.healthProblems(r.Context(), language)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Problems []healthProblemJSON `json:"problems"`
	}{problems})
}

// healthProblems finds the problems System › Health shows, errors first,
// from what the server's parts record: nothing is requested outside the
// server, but for the SOCKS5 proxy of torrents, checked as their kill
// switch checks it. With the database down, that alone is told: the rest
// is read from it.
func (h *handler) healthProblems(ctx context.Context, language string) ([]healthProblemJSON, error) {
	problems := []healthProblemJSON{}
	add := func(p healthProblemJSON) { problems = append(problems, p) }
	warning := func(key, code, anchor string) healthProblemJSON {
		return healthProblemJSON{Key: key, Code: code, Tone: notifications.SeverityWarning, To: "/system/health#" + anchor}
	}

	pingCtx, cancel := context.WithTimeout(ctx, readyWait)
	reachable := h.Database.Ping(pingCtx) == nil
	cancel()
	if !reachable {
		return []healthProblemJSON{{Key: "database", Code: problemDatabase, Tone: notifications.SeverityError, To: "/system/health#database"}}, nil
	}

	for _, folder := range []struct{ name, path string }{{"cache", h.Health.CacheDir}, {"recordings", h.Recordings.Dir()}, {"backups", h.Backups.Dir()}} {
		if folder.path == "" {
			continue
		}
		if free, used, _, ok := diskspace.Measure(folder.path); ok && lowOnSpace(free, used) {
			p := warning("disk:"+folder.name, problemDisk, "disks")
			p.Folder, p.Free = folder.name, &free
			add(p)
		}
	}

	// The server's addons, sources and guides come first, then each
	// user's own: those of addons, then of sources, then of guides.
	scopes, err := h.ownedScopes(ctx)
	if err != nil {
		return nil, err
	}
	var sources, guides []healthProblemJSON
	for _, scope := range scopes {
		list, err := h.Addons.Addons(ctx, scope.Scope)
		if err != nil {
			return nil, err
		}
		if scope.Owner == nil {
			if p, ok := metadataLanguagesProblem(list); ok {
				add(p)
			}
		}
		for _, addon := range list {
			if !addon.Enabled {
				continue
			}
			if addon.Stremio() || addon.Eclipse() {
				if h.Health.Addons == nil {
					continue
				}
				if health, ok := h.Health.Addons.Health(addon.ManifestURL); ok && health.Failure != "" {
					p := warning("addon:"+addon.ID.String(), problemAddon, "addons")
					p.Name, p.Owner, p.Failure = addon.Manifest.Name, scope.Owner, health.Failure
					add(p)
				}
				continue
			}
			if addon.Local() {
				if h.Folders == nil {
					continue
				}
				folder, err := h.Folders.Folder(ctx, addon.ID)
				if err != nil {
					return nil, err
				}
				if folder.Error != "" {
					sources = append(sources, healthProblemJSON{Key: "folder:" + addon.ID.String(), Code: problemFolder,
						Tone: notifications.SeverityError, To: "/sources/shared/" + addon.ID.String(), Name: addon.Manifest.Name,
						Failure: folder.Error, Share: folder.Share})
				} else if folder.Unwatched != "" {
					sources = append(sources, healthProblemJSON{Key: "folder_unwatched:" + addon.ID.String(), Code: problemFolderUnwatched,
						Tone: notifications.SeverityWarning, To: "/sources/shared/" + addon.ID.String(), Name: addon.Manifest.Name,
						Failure: folder.Unwatched, Share: folder.Share})
				}
				if folder.SearchFailed > 0 {
					sources = append(sources, healthProblemJSON{Key: "folder_search:" + addon.ID.String(), Code: problemFolderSearch,
						Tone: notifications.SeverityWarning, To: "/sources/shared/" + addon.ID.String(), Name: addon.Manifest.Name,
						Addon: folder.SearchAddon, Failure: folder.SearchFailure})
				}
				continue
			}
			if h.IPTV == nil {
				continue
			}
			source, err := h.IPTV.Source(ctx, scope.Scope, addon.ID)
			if err != nil {
				return nil, err
			}
			switch {
			case addon.Tuner() && (source.Error != "" || source.Tuner != nil && source.Tuner.Failure != ""):
				failure := "unreachable"
				if source.Error == "" {
					failure = source.Tuner.Failure
				}
				sources = append(sources, healthProblemJSON{Key: "tuner:" + addon.ID.String(), Code: problemTuner,
					Tone: notifications.SeverityWarning, To: "/sources/shared/" + addon.ID.String(), Name: addon.Manifest.Name, Failure: failure})
			case source.Error != "":
				p := warning("iptv:"+addon.ID.String(), problemIPTV, "iptv")
				p.Name, p.Owner = addon.Manifest.Name, scope.Owner
				sources = append(sources, p)
			}
		}
		libraries, err := h.Addons.Libraries(ctx, scope.Scope)
		if err != nil {
			return nil, err
		}
		for _, l := range libraries {
			if !slices.ContainsFunc(l.Guides, func(g addons.Guide) bool { return g.Error != "" }) {
				continue
			}
			p := warning("guide:"+l.AddonID.String()+":"+l.Catalog.Type+":"+l.Catalog.ID, problemGuide, "guides")
			p.Name, p.Owner = l.Catalog.Name, scope.Owner
			if l.Name != nil && *l.Name != "" {
				p.Name = *l.Name
			}
			guides = append(guides, p)
		}
	}

	if encoder := h.Health.Encoder; encoder != nil {
		if conversions, limit := encoder.Conversions(); limit > 0 && conversions >= limit {
			p := warning("conversions", problemConversionsFull, "transcoder")
			p.Transient = true
			add(p)
		}
	}
	if h.Health.Thumbnails != nil {
		for _, paused := range h.Health.Thumbnails.Status().Paused {
			p := warning("thumbnails:"+paused.Host, problemThumbnailsPaused, "thumbnails")
			p.Host, p.Transient = paused.Host, true
			add(p)
		}
	}
	if h.Health.Proxies != nil {
		for _, address := range h.Health.Proxies.Unknown() {
			p := healthProblemJSON{Key: "unknown_proxy:" + address.String(), Code: problemUnknownProxy, Tone: notifications.SeverityWarning,
				To: "/settings/security#known-proxies", Address: address.String()}
			add(p)
		}
		for _, name := range h.Health.Proxies.Names() {
			if len(name.Addresses) == 0 {
				add(healthProblemJSON{Key: "proxy_name:" + name.Name, Code: problemProxyName, Tone: notifications.SeverityWarning,
					To: "/settings/security#known-proxies", Name: name.Name})
			}
		}
	}
	if h.Health.Discovery != nil {
		status := h.Health.Discovery()
		if status.Unavailable {
			add(healthProblemJSON{Key: "discovery:port", Code: problemDiscoveryPort, Tone: notifications.SeverityWarning,
				To: "/settings/general#local-discovery", Port: status.Port})
		}
		if status.ContainerAddress != "" {
			add(healthProblemJSON{Key: "discovery:address", Code: problemDiscoveryAddress, Tone: notifications.SeverityWarning,
				To: "/settings/general#local-address", Address: status.ContainerAddress})
		}
	}
	if h.Health.DLNA != nil {
		if status := h.Health.DLNA(); status.Unavailable {
			add(healthProblemJSON{Key: "dlna:port", Code: problemDLNAPort, Tone: notifications.SeverityWarning,
				To: "/settings/dlna#dlna", Port: status.Port})
		}
	}
	if h.Health.HTTPS != nil {
		problems = append(problems, httpsProblems(h.Health.HTTPS(), h.now())...)
	}
	if h.Health.Torrents != nil {
		if status := h.Health.Torrents.Status(ctx); status.Enabled {
			if status.RouteError != nil {
				p := healthProblemJSON{Key: "torrents:route", Code: problemTorrentRoute, Tone: notifications.SeverityError,
					To: "/settings/torrents#torrent-route", Failure: "no_route"}
				var routeErr *torrent.RouteError
				if errors.As(status.RouteError, &routeErr) {
					p.Failure, p.Name = routeErr.Kind, routeErr.Route
				}
				add(p)
			} else if status.NoUDP {
				add(healthProblemJSON{Key: "torrents:udp", Code: problemTorrentUDP, Tone: notifications.SeverityWarning,
					To: "/settings/torrents#torrent-route", Name: h.Accounts.Settings().TorrentProxy})
			}
			if u := status.Unstarted; u != nil && status.RouteError == nil && h.now().Sub(status.UnstartedAt) < unstartedFor {
				add(healthProblemJSON{Key: "torrents:start", Code: problemTorrentStart, Tone: notifications.SeverityWarning,
					To: "/settings/torrents#torrent-route", Failure: torrentStartFailure(u)})
			}
		}
	}
	if h.Health.MissingLanguages != nil {
		for _, code := range h.Health.MissingLanguages(ctx) {
			add(healthProblemJSON{Key: "subtitle_ocr_language:" + code, Code: problemOCRLanguage, Tone: notifications.SeverityWarning,
				To: "/settings/conversion#subtitle-ocr", Name: ocrLanguageName(code), File: code + ".traineddata"})
		}
	}

	if h.Health.Secrets != nil {
		report, err := h.Health.Secrets(ctx)
		if err != nil {
			return nil, err
		}
		if len(report.Unreadable) > 0 {
			p := healthProblemJSON{Key: "secrets:unreadable", Code: problemSecretsUnreadable, Tone: notifications.SeverityError,
				To: "/system/health#secrets"}
			for _, unreadable := range report.Unreadable {
				p.Secrets = append(p.Secrets, newUnreadableJSON(unreadable))
			}
			add(p)
		}
		if !h.Health.SecretKey && report.Plaintext > 0 {
			add(warning("secrets:plaintext", problemSecretsPlaintext, "secrets"))
		}
	}

	backup, err := h.backupStatus(ctx)
	if err != nil {
		return nil, err
	}
	if backup != nil && backup.Problem {
		if backup.Error != "" {
			add(healthProblemJSON{Key: "backup", Code: problemBackupFailed, Tone: notifications.SeverityError, To: "/system/health#backups"})
		} else {
			add(warning("backup", problemBackupStale, "backups"))
		}
	}
	if backup != nil {
		for _, d := range backup.Destinations {
			switch {
			case d.Problem == nil:
			case *d.Problem == "failed":
				add(healthProblemJSON{Key: "backup_destination:" + d.ID, Code: problemDestinationFailed, Tone: notifications.SeverityError,
					To: "/system/health#backups", Name: d.Name})
			default:
				p := warning("backup_destination:"+d.ID, problemDestinationStale, "backups")
				p.Name = d.Name
				add(p)
			}
		}
	}
	problems = append(problems, sources...)
	problems = append(problems, guides...)

	if h.Tasks != nil {
		for _, task := range h.Tasks.Tasks(language) {
			// A failed backup is told above, with what failed.
			if task.Key == "BackUpDatabase" && backup != nil && backup.Error != "" {
				continue
			}
			if task.Last != nil && task.Last.Status == tasks.Failed {
				add(healthProblemJSON{Key: "task:" + task.Key, Code: problemTask, Tone: notifications.SeverityWarning,
					To: "/system/schedule", Task: task.Name})
			}
		}
	}

	stopped, err := h.stoppedConnections(ctx)
	if err != nil {
		return nil, err
	}
	problems = append(problems, stopped...)

	slices.SortStableFunc(problems, func(a, b healthProblemJSON) int {
		return cmp.Compare(toneRank(a.Tone), toneRank(b.Tone))
	})
	return problems, nil
}

// toneRank orders errors before warnings.
func toneRank(tone string) int {
	if tone == notifications.SeverityError {
		return 0
	}
	return 1
}

// metadataLanguagesProblem tells, from the server's addons, when every
// enabled metadata addon among them has a language: they then describe
// titles in their language for every user, whatever the user's (see
// library's languages.go). It opens the first one's page.
func metadataLanguagesProblem(list []addons.Addon) (healthProblemJSON, bool) {
	var first *addons.Addon
	for i, addon := range list {
		if !addon.Enabled || !addon.Stremio() || !addon.Manifest.HasResource("meta") {
			continue
		}
		if addon.Language == "" {
			return healthProblemJSON{}, false
		}
		if first == nil {
			first = &list[i]
		}
	}
	if first == nil {
		return healthProblemJSON{}, false
	}
	return healthProblemJSON{Key: "metadata_languages", Code: problemMetadataLanguages, Tone: notifications.SeverityWarning,
		To: "/sources/shared/" + first.ID.String(), Name: first.Manifest.Name}, true
}

// stoppedConnections lists the app connections and tracking services that
// stopped, the apps or services having refused what Polyfin kept, by user
// name, then app or service. Each opens its user's page.
func (h *handler) stoppedConnections(ctx context.Context) ([]healthProblemJSON, error) {
	var stopped []healthProblemJSON
	add := func(user accounts.ID, name, kind, service string) {
		stopped = append(stopped, healthProblemJSON{Key: "connection_stopped:" + kind + ":" + service + ":" + user.String(),
			Code: problemConnectionStopped, Tone: notifications.SeverityWarning, To: "/users/" + user.String(), Transient: true,
			Name: service, Owner: &ownerJSON{ID: user.String(), Name: name}, Kind: kind})
	}
	if h.AppImport != nil {
		apps, err := h.AppImport.Stopped(ctx)
		if err != nil {
			return nil, err
		}
		for _, c := range apps {
			add(c.User, c.UserName, notifications.ConnectionApp, c.App)
		}
	}
	if h.Trackers != nil {
		services, err := h.Trackers.Stopped(ctx)
		if err != nil {
			return nil, err
		}
		for _, c := range services {
			add(c.User, c.UserName, notifications.ConnectionTracking, c.Service)
		}
	}
	slices.SortStableFunc(stopped, func(a, b healthProblemJSON) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Owner.Name), strings.ToLower(b.Owner.Name)), cmp.Compare(a.Owner.ID, b.Owner.ID),
			cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Name, b.Name))
	})
	return stopped, nil
}

// HealthProblems returns what finds, for notifications, the problems System
// › Health shows, named in the server language, but for the transient ones:
// those that come and go with the load, and the stopped connections, which
// their own event tells.
func HealthProblems(options Options) func(context.Context) ([]notifications.Problem, error) {
	h := &handler{Options: options, now: time.Now}
	if options.Now != nil {
		h.now = options.Now
	}
	return func(ctx context.Context) ([]notifications.Problem, error) {
		language := h.Accounts.Settings().Language
		found, err := h.healthProblems(ctx, language)
		if err != nil {
			return nil, err
		}
		problems := []notifications.Problem{}
		for _, p := range found {
			if !p.Transient {
				problems = append(problems, notifications.Problem{Key: p.Key, Severity: p.Tone, Text: p.text(language), Page: p.To})
			}
		}
		return problems, nil
	}
}

// addonFailures name the failures of addons' requests, in English and
// French, as System › Health names them.
var addonFailures = map[string][2]string{
	"unreachable":      {"could not be reached", "injoignable"},
	"timeout":          {"did not answer in time", "n’a pas répondu à temps"},
	"private_network":  {"is on a local network", "est sur un réseau local"},
	"rate_limited":     {"asked to slow down (too many requests)", "a demandé de ralentir (trop de requêtes)"},
	"server_error":     {"answered with a server error", "a répondu par une erreur serveur"},
	"http_error":       {"refused the request", "a refusé la requête"},
	"invalid_response": {"sent an answer too large", "a envoyé une réponse trop grande"},
}

// searchFailures name the failures of search catalogs' requests, in
// English and French, as the folder pages name them: an answer that
// could not be read is too large or not of the expected form.
var searchFailures = map[string][2]string{
	"unreachable":      {"could not be reached", "est injoignable"},
	"timeout":          {"did not answer in time", "n’a pas répondu à temps"},
	"private_network":  {"is on a local network", "est sur un réseau local"},
	"rate_limited":     {"asked to slow down (too many requests)", "a demandé de ralentir (trop de requêtes)"},
	"server_error":     {"answered with a server error", "a répondu par une erreur serveur"},
	"http_error":       {"refused the request", "a refusé la requête"},
	"invalid_response": {"sent an answer Polyfin cannot read", "a envoyé une réponse que Polyfin ne sait pas lire"},
}

// diskFolders name the folders whose disks are measured, in English and
// French.
var diskFolders = map[string][2]string{
	"cache":      {"cache", "du cache"},
	"recordings": {"recordings", "des enregistrements"},
	"backups":    {"backups", "des sauvegardes"},
}

// text tells a problem notifications send in English or French. The
// transient ones are never sent: they have none.
func (p healthProblemJSON) text(language string) string {
	pick := func(words [2]string) string {
		if language == "fr" {
			return words[1]
		}
		return words[0]
	}
	say := func(english, french string, args ...any) string {
		return fmt.Sprintf(pick([2]string{english, french}), args...)
	}
	name := p.Name
	if p.Owner != nil {
		name = say("%s (%s’s addons)", "%s (Addons de %s)", p.Name, p.Owner.Name)
	}
	switch p.Code {
	case problemDatabase:
		return say("The database does not answer.", "La base de données ne répond pas.")
	case problemDisk:
		return say("Little space left for the %s: %s free.", "Il reste peu de place pour le dossier %s : %s libres.",
			pick(diskFolders[p.Folder]), gigabytes(*p.Free, language))
	case problemAddon:
		failure, known := addonFailures[p.Failure]
		if !known {
			failure = [2]string{p.Failure, p.Failure}
		}
		return say("%s: %s", "%s : %s", name, pick(failure))
	case problemIPTV:
		return say("%s: the last channel list download failed.", "%s : le dernier téléchargement de la liste a échoué.", name)
	case problemTuner:
		if p.Failure == "unreachable" {
			return say("%s: the tuner does not answer.", "%s : le tuner ne répond pas.", name)
		}
		return say("%s: the tuner failed to play a channel.", "%s : le tuner n’a pas pu lire une chaîne.", name)
	case problemFolder:
		switch {
		case p.Failure == "unreachable":
			return say("%s: Polyfin cannot reach this network share.", "%s : Polyfin ne peut pas joindre ce partage réseau.", name)
		case p.Failure == "refused":
			return say("%s: the network share refuses Polyfin’s user or password.",
				"%s : le partage réseau refuse l’utilisateur ou le mot de passe de Polyfin.", name)
		case p.Share != "":
			return say("%s: Polyfin cannot read this network share. Check its address, and that its user may read it.",
				"%s : Polyfin ne peut pas lire ce partage réseau. Vérifiez son adresse, et que son utilisateur peut le lire.", name)
		}
		return say("%s: Polyfin cannot read this local folder. Check that it is mounted and readable by user 65532.",
			"%s : Polyfin ne peut pas lire ce dossier local. Vérifiez qu’il est monté et lisible par l’utilisateur 65532.", name)
	case problemFolderUnwatched:
		if p.Share != "" {
			switch p.Failure {
			case "unreachable":
				return say("%s: Polyfin cannot watch this network share for changes: it cannot be reached. "+
					"It is scanned on its schedule, and again once it answers.",
					"%s : Polyfin ne peut pas surveiller les changements de ce partage réseau : il est injoignable. "+
						"Il est analysé selon sa planification, et de nouveau dès qu’il répond.", name)
			case "refused":
				return say("%s: Polyfin cannot watch this network share for changes: it refuses Polyfin’s user or password. "+
					"Check them, and enter them again.",
					"%s : Polyfin ne peut pas surveiller les changements de ce partage réseau : il refuse l’utilisateur ou le mot de passe de Polyfin. "+
						"Vérifiez-les, et saisissez-les à nouveau.", name)
			}
			return say("%s: Polyfin cannot watch this network share for changes. It is still scanned on its schedule.",
				"%s : Polyfin ne peut pas surveiller les changements de ce partage réseau. Il est toujours analysé selon sa planification.", name)
		}
		switch p.Failure {
		case "watch_limit":
			return say("%s: Polyfin cannot watch this local folder for changes: the system’s limit of watched folders is reached. "+
				"It is still scanned on its schedule. Raise fs.inotify.max_user_watches on the host.",
				"%s : Polyfin ne peut pas surveiller les changements de ce dossier local : la limite de dossiers surveillés du système est atteinte. "+
					"Il est toujours analysé selon sa planification. Augmentez fs.inotify.max_user_watches sur l’hôte.", name)
		case "instance_limit":
			return say("%s: Polyfin cannot watch this local folder for changes: the system’s limit of watchers or of open files is reached. "+
				"It is still scanned on its schedule. Raise fs.inotify.max_user_instances on the host, or the container’s limit of open files.",
				"%s : Polyfin ne peut pas surveiller les changements de ce dossier local : la limite d’observateurs ou de fichiers ouverts du système est atteinte. "+
					"Il est toujours analysé selon sa planification. Augmentez fs.inotify.max_user_instances sur l’hôte, ou la limite de fichiers ouverts du conteneur.", name)
		case "open_files_limit":
			return say("%s: Polyfin cannot watch this local folder for changes: the system’s limit of open files is reached, as macOS watches each folder and file through one. "+
				"It is still scanned on its schedule. Raise kern.maxfilesperproc and kern.maxfiles with sysctl.",
				"%s : Polyfin ne peut pas surveiller les changements de ce dossier local : la limite de fichiers ouverts du système est atteinte, macOS surveillant chaque dossier et fichier par l’un d’eux. "+
					"Il est toujours analysé selon sa planification. Augmentez kern.maxfilesperproc et kern.maxfiles avec sysctl.", name)
		}
		return say("%s: Polyfin cannot watch this local folder for changes. It is still scanned on its schedule.",
			"%s : Polyfin ne peut pas surveiller les changements de ce dossier local. Il est toujours analysé selon sa planification.", name)
	case problemFolderSearch:
		why := ""
		if failure, known := searchFailures[p.Failure]; known && p.Addon != "" {
			why = say(" (%s: %s)", " (%s : %s)", p.Addon, pick(failure))
		} else if p.Addon != "" {
			why = " (" + p.Addon + ")"
		}
		return say("%s: the titles of its files could not be searched%s. They stay unmatched until a search goes through, which Polyfin tries again on its own, after a few minutes, then less often.",
			"%s : les titres de ses fichiers n’ont pas pu être recherchés%s. Ils restent non associés jusqu’à ce qu’une recherche aboutisse, ce que Polyfin réessaie de lui-même, après quelques minutes, puis moins souvent.",
			name, why)
	case problemGuide:
		return say("%s: the last programme guide fetch failed.", "%s : la dernière récupération du guide a échoué.", name)
	case problemSecretsUnreadable:
		if len(p.Secrets) == 1 {
			return say("1 stored key cannot be decrypted with POLYFIN_SECRET_KEY and counts as not set.",
				"1 clé enregistrée ne peut pas être déchiffrée avec POLYFIN_SECRET_KEY et compte comme absente.")
		}
		return say("%d stored keys cannot be decrypted with POLYFIN_SECRET_KEY and count as not set.",
			"%d clés enregistrées ne peuvent pas être déchiffrées avec POLYFIN_SECRET_KEY et comptent comme absentes.", len(p.Secrets))
	case problemSecretsPlaintext:
		return say("Keys and tokens are stored unencrypted. Set POLYFIN_SECRET_KEY to encrypt them.",
			"Les clés et jetons sont enregistrés sans chiffrement. Définissez POLYFIN_SECRET_KEY pour les chiffrer.")
	case problemBackupFailed:
		return say("The last database backup failed.", "La dernière sauvegarde de la base de données a échoué.")
	case problemBackupStale:
		return say("The last database backup is more than two days old.", "La dernière sauvegarde de la base de données date de plus de deux jours.")
	case problemDestinationFailed:
		return say("%s: the last upload of the database backups failed.", "%s : le dernier envoi des sauvegardes de la base de données a échoué.",
			name)
	case problemDestinationStale:
		return say("%s: the newest database backup there is more than two days old.",
			"%s : la sauvegarde de la base de données la plus récente qui s’y trouve date de plus de deux jours.", name)
	case problemTask:
		return say("The task “%s” failed.", "La tâche « %s » a échoué.", p.Task)
	case problemUnknownProxy:
		return say("%s forwards requests (X-Forwarded-For) but is not a known proxy, so its clients all count as %s. "+
			"If it is your reverse proxy, add it to Known proxies under Settings › Security.",
			"%s transmet des requêtes (X-Forwarded-For) sans être un proxy connu : tous ses clients comptent donc comme %s. "+
				"Si c’est votre proxy inverse, ajoutez-le aux Proxys connus dans Paramètres › Sécurité.", p.Address, p.Address)
	case problemProxyName:
		return say("The known proxy %s resolves to no address, so no request is trusted to come from it. "+
			"Check its name under Settings › Security, and that Polyfin shares a network with it.",
			"Le proxy connu %s ne correspond à aucune adresse : aucune requête n’est donc reconnue comme venant de lui. "+
				"Vérifiez son nom dans Paramètres › Sécurité, et que Polyfin partage un réseau avec lui.", p.Name)
	case problemDiscoveryPort:
		return say("Polyfin cannot listen on UDP port %d, so Jellyfin apps on the local network do not find it by themselves. "+
			"Another program probably uses it, such as a Jellyfin server on the same host. Saving the settings tries again.",
			"Polyfin ne peut pas écouter sur le port UDP %d : les applications Jellyfin du réseau local ne le trouvent donc pas d’elles-mêmes. "+
				"Un autre programme l’utilise sans doute, comme un serveur Jellyfin sur le même hôte. Enregistrer les paramètres réessaie.", p.Port)
	case problemTorrentRoute:
		var why [2]string
		switch p.Failure {
		case torrent.InterfaceMissing:
			why = [2]string{"the network interface " + p.Name + " is missing", "l’interface réseau " + p.Name + " est absente"}
		case torrent.InterfaceDown:
			why = [2]string{"the network interface " + p.Name + " is down", "l’interface réseau " + p.Name + " est désactivée"}
		case torrent.InterfaceNoAddress:
			why = [2]string{"the network interface " + p.Name + " has no address", "l’interface réseau " + p.Name + " n’a pas d’adresse"}
		case torrent.InterfaceUnsupported:
			why = [2]string{"a network interface works on Linux only", "une interface réseau ne fonctionne que sous Linux"}
		case torrent.ProxyUnreachable:
			why = [2]string{"the SOCKS5 proxy " + p.Name + " does not answer", "le proxy SOCKS5 " + p.Name + " ne répond pas"}
		case torrent.ProxyRefused:
			why = [2]string{"the SOCKS5 proxy " + p.Name + " refused the user and password", "le proxy SOCKS5 " + p.Name + " a refusé l’utilisateur et le mot de passe"}
		case torrent.WireGuardEndpoint:
			why = [2]string{"the WireGuard server " + p.Name + " cannot be looked up", "le serveur WireGuard " + p.Name + " est introuvable"}
		case torrent.WireGuardDown:
			why = [2]string{"the WireGuard tunnel to " + p.Name + " cannot start", "le tunnel WireGuard vers " + p.Name + " ne peut pas démarrer"}
		case torrent.WireGuardHandshake:
			why = [2]string{"the WireGuard server " + p.Name + " does not answer the handshake: it is unreachable, or a key of the configuration is wrong",
				"le serveur WireGuard " + p.Name + " ne répond pas à la négociation : il est injoignable, ou une clé de la configuration est erronée"}
		default:
			why = [2]string{"no route is set", "aucune route n’est définie"}
		}
		return say("Torrent streams do not play: %s. Nothing is sent another way; check the route under Settings › Torrents.",
			"Les flux torrent ne sont pas lus : %s. Rien n’est envoyé par un autre chemin ; vérifiez la route dans Paramètres › Torrents.", pick(why))
	case problemTorrentUDP:
		return say("The SOCKS5 proxy %s does not offer UDP ASSOCIATE: torrents play over TCP, with UDP trackers and the DHT off.",
			"Le proxy SOCKS5 %s ne propose pas UDP ASSOCIATE : les torrents sont lus en TCP, sans trackers UDP ni DHT.", p.Name)
	case problemTorrentStart:
		why := map[string][2]string{
			"no_udp": {"no peer was found, and the route carries no UDP, which the DHT and UDP trackers need",
				"aucun pair n’a été trouvé, et la route ne transporte pas l’UDP, dont ont besoin le DHT et les trackers UDP"},
			"refused": {"its trackers refused the announce or did not answer, and the DHT found no peer",
				"ses trackers ont refusé l’annonce ou n’ont pas répondu, et le DHT n’a trouvé aucun pair"},
			"no_peers": {"its trackers and the DHT found no peer", "ses trackers et le DHT n’ont trouvé aucun pair"},
			"silent":   {"peers were found, but none sent it", "des pairs ont été trouvés, mais aucun ne l’a envoyé"},
		}[p.Failure]
		return say("A torrent did not start in time: %s. Check that the route reaches the internet; the log tells each tracker’s answer at debug level.",
			"Un torrent n’a pas démarré à temps : %s. Vérifiez que la route atteint Internet ; le journal donne la réponse de chaque tracker au niveau débogage.", pick(why))
	case problemDLNAPort:
		return say("Polyfin cannot listen on UDP port %d, so DLNA devices on the local network do not find it by themselves. "+
			"Another program probably uses it, such as another DLNA server on the same host. Saving the settings tries again.",
			"Polyfin ne peut pas écouter sur le port UDP %d : les appareils DLNA du réseau local ne le trouvent donc pas d’eux-mêmes. "+
				"Un autre programme l’utilise sans doute, comme un autre serveur DLNA sur le même hôte. Enregistrer les paramètres réessaie.", p.Port)
	case problemDiscoveryAddress:
		return say("Jellyfin apps that look for servers on the local network are given %s, an address of Polyfin’s container they cannot reach. "+
			"Set Address on the local network under Settings › General.",
			"Les applications Jellyfin qui cherchent des serveurs sur le réseau local reçoivent %s, une adresse du conteneur de Polyfin qu’elles ne peuvent pas joindre. "+
				"Renseignez Adresse sur le réseau local dans Paramètres › Général.", p.Address)
	case problemHTTPSPort:
		return say("Polyfin cannot listen on TCP port %d, so HTTPS is not served. HTTP keeps working. "+
			"Another program probably uses it, or it is below 1024. Choose another HTTPS port under Settings › Security.",
			"Polyfin ne peut pas écouter sur le port TCP %d : HTTPS n’est donc pas servi. HTTP fonctionne toujours. "+
				"Un autre programme l’utilise sans doute, ou il est inférieur à 1024. Choisissez un autre port HTTPS dans Paramètres › Sécurité.", p.Port)
	case problemHTTPSFiles:
		switch p.Failure {
		case https.FilesMissing:
			return say("The HTTPS certificate file %s does not exist. The certificate last read stays in use.",
				"Le fichier de certificat HTTPS %s n’existe pas. Le dernier certificat lu reste utilisé.", p.File)
		case https.FilesMismatch:
			return say("The HTTPS private key does not belong to the certificate. The certificate last read stays in use.",
				"La clé privée HTTPS ne correspond pas au certificat. Le dernier certificat lu reste utilisé.")
		}
		return say("Polyfin cannot read the HTTPS certificate file %s, or it holds no PEM certificate or key it reads. "+
			"The certificate last read stays in use.",
			"Polyfin ne peut pas lire le fichier de certificat HTTPS %s, ou il ne contient aucun certificat ou clé PEM qu’il sait lire. "+
				"Le dernier certificat lu reste utilisé.", p.File)
	case problemHTTPSRenewal:
		return say("Let’s Encrypt did not give the HTTPS certificate: %s", "Let’s Encrypt n’a pas délivré le certificat HTTPS : %s", p.Error)
	case problemHTTPSExpiry:
		if p.Tone == notifications.SeverityError {
			return say("The HTTPS certificate expired on %s.", "Le certificat HTTPS a expiré le %s.", p.Expires.Format(time.DateOnly))
		}
		return say("The HTTPS certificate expires on %s.", "Le certificat HTTPS expire le %s.", p.Expires.Format(time.DateOnly))
	case problemOCRLanguage:
		return say("Picture subtitles in %s are not read into text: Tesseract has no data for this language. "+
			"Add the file %s to the folder POLYFIN_TESSDATA_DIR names.",
			"Les sous-titres en images en %s ne sont pas lus en texte : Tesseract n’a pas de données pour cette langue. "+
				"Ajoutez le fichier %s au dossier que nomme POLYFIN_TESSDATA_DIR.", p.Name, p.File)
	case problemMetadataLanguages:
		return say("%s: every metadata addon of the server has a language, so they describe titles in their language for every user, whatever their metadata language. "+
			"Keep Language of its descriptions at None on this addon, or on one more metadata addon.",
			"%s : chaque addon de métadonnées du serveur a une langue : ils décrivent donc les titres dans leur langue pour tous les utilisateurs, quelle que soit leur langue des métadonnées. "+
				"Laissez Langue de ses descriptions sur Aucune pour cet addon, ou pour un addon de métadonnées de plus.", p.Name)
	}
	return ""
}

// ocrLanguageName names a language by its Tesseract code, in English:
// "jpn" is Japanese. A code no language has is its own name.
func ocrLanguageName(code string) string {
	base, _, _ := strings.Cut(code, "_")
	bibliographic := localization.LanguageCode(base)
	for _, l := range localization.Languages() {
		if l.Bibliographic == bibliographic {
			return l.Name
		}
	}
	return code
}

// gigabytes writes bytes in gigabytes with one decimal, as French or
// English writes it.
func gigabytes(bytes int64, language string) string {
	text := strconv.FormatFloat(float64(bytes)/1e9, 'f', 1, 64)
	if language == "fr" {
		return strings.Replace(text, ".", ",", 1) + " Go"
	}
	return text + " GB"
}

// unstartedFor is how long Health tells of a torrent that did not start,
// unless a torrent starts meanwhile: as long as a play leaves its version
// out.
const unstartedFor = 15 * time.Minute

// torrentStartFailure is the code of why a torrent did not start: peers
// found that sent nothing ("silent"), none found over a route without UDP
// ("no_udp"), trackers that all refused or did not answer ("refused"), or
// none found where it was looked for ("no_peers").
func torrentStartFailure(err *torrent.StartError) string {
	switch {
	case err.Found > 0:
		return "silent"
	case err.NoUDP:
		return "no_udp"
	case err.Trackers > 0 && err.Answered == 0:
		return "refused"
	}
	return "no_peers"
}

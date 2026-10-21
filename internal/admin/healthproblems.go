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
	"github.com/moodiness/polyfin/internal/i18n"
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
		// The route's check has Health's deadline: a WireGuard tunnel whose
		// server does not answer waits out the whole of it for a handshake,
		// which tells the same failure as a longer wait, and the admin app
		// asks for the problems every few seconds.
		routeCtx, cancelRoute := context.WithTimeout(ctx, readyWait)
		status := h.Health.Torrents.Status(routeCtx)
		cancelRoute()
		if status.Enabled {
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

// addonFailures name the failures of addons' requests, as System › Health
// names them.
var addonFailures = map[string]i18n.Text{
	"unreachable":      {"could not be reached", "injoignable", "war nicht erreichbar", "no se pudo contactar", "non è raggiungibile", "não pôde ser acessado"},
	"timeout":          {"did not answer in time", "n’a pas répondu à temps", "hat nicht rechtzeitig geantwortet", "no respondió a tiempo", "non ha risposto in tempo", "não respondeu a tempo"},
	"private_network":  {"is on a local network", "est sur un réseau local", "befindet sich in einem lokalen Netzwerk", "está en una red local", "si trova su una rete locale", "está em uma rede local"},
	"rate_limited":     {"asked to slow down (too many requests)", "a demandé de ralentir (trop de requêtes)", "hat um Verlangsamung gebeten (zu viele Anfragen)", "pidió reducir el ritmo (demasiadas solicitudes)", "ha chiesto di rallentare (troppe richieste)", "pediu para diminuir o ritmo (requisições demais)"},
	"server_error":     {"answered with a server error", "a répondu par une erreur serveur", "hat mit einem Serverfehler geantwortet", "respondió con un error del servidor", "ha risposto con un errore del server", "respondeu com um erro do servidor"},
	"http_error":       {"refused the request", "a refusé la requête", "hat die Anfrage abgelehnt", "rechazó la solicitud", "ha rifiutato la richiesta", "recusou a requisição"},
	"invalid_response": {"sent an answer too large", "a envoyé une réponse trop grande", "hat eine zu große Antwort gesendet", "envió una respuesta demasiado grande", "ha inviato una risposta troppo grande", "enviou uma resposta grande demais"},
}

// searchFailures name the failures of search catalogs' requests, as the
// folder pages name them: an answer that could not be read is too large
// or not of the expected form.
var searchFailures = map[string]i18n.Text{
	"unreachable":      {"could not be reached", "est injoignable", "war nicht erreichbar", "no se pudo contactar", "non è raggiungibile", "não pôde ser acessado"},
	"timeout":          {"did not answer in time", "n’a pas répondu à temps", "hat nicht rechtzeitig geantwortet", "no respondió a tiempo", "non ha risposto in tempo", "não respondeu a tempo"},
	"private_network":  {"is on a local network", "est sur un réseau local", "befindet sich in einem lokalen Netzwerk", "está en una red local", "si trova su una rete locale", "está em uma rede local"},
	"rate_limited":     {"asked to slow down (too many requests)", "a demandé de ralentir (trop de requêtes)", "hat um Verlangsamung gebeten (zu viele Anfragen)", "pidió reducir el ritmo (demasiadas solicitudes)", "ha chiesto di rallentare (troppe richieste)", "pediu para diminuir o ritmo (requisições demais)"},
	"server_error":     {"answered with a server error", "a répondu par une erreur serveur", "hat mit einem Serverfehler geantwortet", "respondió con un error del servidor", "ha risposto con un errore del server", "respondeu com um erro do servidor"},
	"http_error":       {"refused the request", "a refusé la requête", "hat die Anfrage abgelehnt", "rechazó la solicitud", "ha rifiutato la richiesta", "recusou a requisição"},
	"invalid_response": {"sent an answer Polyfin cannot read", "a envoyé une réponse que Polyfin ne sait pas lire", "hat eine Antwort gesendet, die Polyfin nicht lesen kann", "envió una respuesta que Polyfin no puede leer", "ha inviato una risposta che Polyfin non riesce a leggere", "enviou uma resposta que o Polyfin não consegue ler"},
}

// diskFolders name the folders whose disks are measured.
var diskFolders = map[string]i18n.Text{
	"cache":      {"cache", "du cache", "den Cache", "la caché", "la cache", "o cache"},
	"recordings": {"recordings", "des enregistrements", "die Aufnahmen", "las grabaciones", "le registrazioni", "as gravações"},
	"backups":    {"backups", "des sauvegardes", "die Backups", "las copias de seguridad", "i backup", "os backups"},
}

// text tells a problem notifications send in a server language. The
// transient ones are never sent: they have none.
func (p healthProblemJSON) text(language string) string {
	say := func(text i18n.Text, args ...any) string {
		return fmt.Sprintf(text.In(language), args...)
	}
	name := p.Name
	if p.Owner != nil {
		name = say(i18n.Text{"%s (%s’s addons)", "%s (Addons de %s)", "%s (Add-ons von %s)", "%s (Extensiones de %s)", "%s (Add-on di %s)", "%s (Complementos de %s)"},
			p.Name, p.Owner.Name)
	}
	switch p.Code {
	case problemDatabase:
		return say(i18n.Text{"The database does not answer.", "La base de données ne répond pas.", "Die Datenbank antwortet nicht.", "La base de datos no responde.", "Il database non risponde.", "O banco de dados não responde."})
	case problemDisk:
		return say(i18n.Text{"Little space left for the %s: %s free.", "Il reste peu de place pour le dossier %s : %s libres.",
			"Wenig Speicherplatz für %s: %s frei.", "Queda poco espacio para %s: %s libres.", "Poco spazio rimasto per %s: %s liberi.", "Pouco espaço restante para %s: %s livres."},
			diskFolders[p.Folder].In(language), gigabytes(*p.Free, language))
	case problemAddon:
		failure, known := addonFailures[p.Failure]
		if !known {
			return say(i18n.Text{"%s: %s", "%s : %s", "%s: %s", "%s: %s", "%s: %s", "%s: %s"}, name, p.Failure)
		}
		return say(i18n.Text{"%s: %s", "%s : %s", "%s: %s", "%s: %s", "%s: %s", "%s: %s"}, name, failure.In(language))
	case problemIPTV:
		return say(i18n.Text{"%s: the last channel list download failed.", "%s : le dernier téléchargement de la liste a échoué.", "%s: Der letzte Download der Kanalliste ist fehlgeschlagen.", "%s: ha fallado la última descarga de la lista de canales.", "%s: l’ultimo download dell’elenco canali non è riuscito.", "%s: o último download da lista de canais falhou."}, name)
	case problemTuner:
		if p.Failure == "unreachable" {
			return say(i18n.Text{"%s: the tuner does not answer.", "%s : le tuner ne répond pas.", "%s: Der Tuner antwortet nicht.", "%s: el sintonizador no responde.", "%s: il sintonizzatore non risponde.", "%s: o sintonizador não responde."}, name)
		}
		return say(i18n.Text{"%s: the tuner failed to play a channel.", "%s : le tuner n’a pas pu lire une chaîne.", "%s: Der Tuner konnte einen Kanal nicht abspielen.", "%s: el sintonizador no ha podido reproducir un canal.", "%s: il sintonizzatore non è riuscito a riprodurre un canale.", "%s: o sintonizador não conseguiu reproduzir um canal."}, name)
	case problemFolder:
		switch {
		case p.Failure == "unreachable":
			return say(i18n.Text{"%s: Polyfin cannot reach this network share.", "%s : Polyfin ne peut pas joindre ce partage réseau.", "%s: Polyfin kann diese Netzwerkfreigabe nicht erreichen.", "%s: Polyfin no puede acceder a este recurso compartido de red.", "%s: Polyfin non riesce a raggiungere questa condivisione di rete.", "%s: o Polyfin não consegue acessar este compartilhamento de rede."}, name)
		case p.Failure == "refused":
			return say(i18n.Text{"%s: the network share refuses Polyfin’s user or password.",
				"%s : le partage réseau refuse l’utilisateur ou le mot de passe de Polyfin.", "%s: Die Netzwerkfreigabe lehnt den Benutzer oder das Passwort von Polyfin ab.", "%s: el recurso compartido de red rechaza el usuario o la contraseña de Polyfin.", "%s: la condivisione di rete rifiuta l’utente o la password di Polyfin.", "%s: o compartilhamento de rede recusa o usuário ou a senha do Polyfin."}, name)
		case p.Share != "":
			return say(i18n.Text{"%s: Polyfin cannot read this network share. Check its address, and that its user may read it.",
				"%s : Polyfin ne peut pas lire ce partage réseau. Vérifiez son adresse, et que son utilisateur peut le lire.", "%s: Polyfin kann diese Netzwerkfreigabe nicht lesen. Prüfe ihre Adresse und ob ihr Benutzer sie lesen darf.", "%s: Polyfin no puede leer este recurso compartido de red. Compruebe su dirección y que su usuario tiene permiso de lectura.", "%s: Polyfin non riesce a leggere questa condivisione di rete. Verifica il suo indirizzo e che il suo utente possa leggerla.", "%s: o Polyfin não consegue ler este compartilhamento de rede. Verifique o endereço dele e se o usuário dele tem permissão de leitura."}, name)
		}
		return say(i18n.Text{"%s: Polyfin cannot read this local folder. Check that it is mounted and readable by user 65532.",
			"%s : Polyfin ne peut pas lire ce dossier local. Vérifiez qu’il est monté et lisible par l’utilisateur 65532.", "%s: Polyfin kann diesen lokalen Ordner nicht lesen. Prüfe, ob er eingehängt und für den Benutzer 65532 lesbar ist.", "%s: Polyfin no puede leer esta carpeta local. Compruebe que está montada y que el usuario 65532 puede leerla.", "%s: Polyfin non riesce a leggere questa cartella locale. Verifica che sia montata e leggibile dall’utente 65532.", "%s: o Polyfin não consegue ler esta pasta local. Verifique se ela está montada e pode ser lida pelo usuário 65532."}, name)
	case problemFolderUnwatched:
		if p.Share != "" {
			switch p.Failure {
			case "unreachable":
				return say(i18n.Text{"%s: Polyfin cannot watch this network share for changes: it cannot be reached. " +
					"It is scanned on its schedule, and again once it answers.",
					"%s : Polyfin ne peut pas surveiller les changements de ce partage réseau : il est injoignable. " +
						"Il est analysé selon sa planification, et de nouveau dès qu’il répond.", "%s: Polyfin kann diese Netzwerkfreigabe nicht auf Änderungen überwachen: Sie ist nicht erreichbar. Sie wird nach ihrem Zeitplan gescannt und erneut, sobald sie antwortet.", "%s: Polyfin no puede vigilar los cambios de este recurso compartido de red: no se puede acceder a él. Se escanea según su programación, y de nuevo en cuanto responda.", "%s: Polyfin non riesce a monitorare le modifiche di questa condivisione di rete: non è raggiungibile. Viene scansionata secondo la sua pianificazione e di nuovo non appena risponde.", "%s: o Polyfin não consegue monitorar alterações neste compartilhamento de rede: ele está inacessível. Ele é rastreado conforme o agendamento e novamente assim que responder."}, name)
			case "refused":
				return say(i18n.Text{"%s: Polyfin cannot watch this network share for changes: it refuses Polyfin’s user or password. " +
					"Check them, and enter them again.",
					"%s : Polyfin ne peut pas surveiller les changements de ce partage réseau : il refuse l’utilisateur ou le mot de passe de Polyfin. " +
						"Vérifiez-les, et saisissez-les à nouveau.", "%s: Polyfin kann diese Netzwerkfreigabe nicht auf Änderungen überwachen: Sie lehnt den Benutzer oder das Passwort von Polyfin ab. Prüfe beides und gib es erneut ein.", "%s: Polyfin no puede vigilar los cambios de este recurso compartido de red: rechaza el usuario o la contraseña de Polyfin. Compruébelos e introdúzcalos de nuevo.", "%s: Polyfin non riesce a monitorare le modifiche di questa condivisione di rete: rifiuta l’utente o la password di Polyfin. Verificali e inseriscili di nuovo.", "%s: o Polyfin não consegue monitorar alterações neste compartilhamento de rede: ele recusa o usuário ou a senha do Polyfin. Verifique-os e digite-os novamente."}, name)
			}
			return say(i18n.Text{"%s: Polyfin cannot watch this network share for changes. It is still scanned on its schedule.",
				"%s : Polyfin ne peut pas surveiller les changements de ce partage réseau. Il est toujours analysé selon sa planification.", "%s: Polyfin kann diese Netzwerkfreigabe nicht auf Änderungen überwachen. Sie wird weiterhin nach ihrem Zeitplan gescannt.", "%s: Polyfin no puede vigilar los cambios de este recurso compartido de red. Se sigue escaneando según su programación.", "%s: Polyfin non riesce a monitorare le modifiche di questa condivisione di rete. Viene comunque scansionata secondo la sua pianificazione.", "%s: o Polyfin não consegue monitorar alterações neste compartilhamento de rede. Ele continua sendo rastreado conforme o agendamento."}, name)
		}
		switch p.Failure {
		case "watch_limit":
			return say(i18n.Text{"%s: Polyfin cannot watch this local folder for changes: the system’s limit of watched folders is reached. " +
				"It is still scanned on its schedule. Raise fs.inotify.max_user_watches on the host.",
				"%s : Polyfin ne peut pas surveiller les changements de ce dossier local : la limite de dossiers surveillés du système est atteinte. " +
					"Il est toujours analysé selon sa planification. Augmentez fs.inotify.max_user_watches sur l’hôte.", "%s: Polyfin kann diesen lokalen Ordner nicht auf Änderungen überwachen: Das Systemlimit für überwachte Ordner ist erreicht. Er wird weiterhin nach seinem Zeitplan gescannt. Erhöhe fs.inotify.max_user_watches auf dem Host.", "%s: Polyfin no puede vigilar los cambios de esta carpeta local: se ha alcanzado el límite de carpetas vigiladas del sistema. Se sigue escaneando según su programación. Aumente fs.inotify.max_user_watches en el host.", "%s: Polyfin non riesce a monitorare le modifiche di questa cartella locale: è stato raggiunto il limite di cartelle monitorate del sistema. Viene comunque scansionata secondo la sua pianificazione. Aumenta fs.inotify.max_user_watches sull’host.", "%s: o Polyfin não consegue monitorar alterações nesta pasta local: o limite de pastas monitoradas do sistema foi atingido. Ela continua sendo rastreada conforme o agendamento. Aumente fs.inotify.max_user_watches no host."}, name)
		case "instance_limit":
			return say(i18n.Text{"%s: Polyfin cannot watch this local folder for changes: the system’s limit of watchers or of open files is reached. " +
				"It is still scanned on its schedule. Raise fs.inotify.max_user_instances on the host, or the container’s limit of open files.",
				"%s : Polyfin ne peut pas surveiller les changements de ce dossier local : la limite d’observateurs ou de fichiers ouverts du système est atteinte. " +
					"Il est toujours analysé selon sa planification. Augmentez fs.inotify.max_user_instances sur l’hôte, ou la limite de fichiers ouverts du conteneur.", "%s: Polyfin kann diesen lokalen Ordner nicht auf Änderungen überwachen: Das Systemlimit für Überwachungsinstanzen oder offene Dateien ist erreicht. Er wird weiterhin nach seinem Zeitplan gescannt. Erhöhe fs.inotify.max_user_instances auf dem Host oder das Limit für offene Dateien des Containers.", "%s: Polyfin no puede vigilar los cambios de esta carpeta local: se ha alcanzado el límite de observadores o de archivos abiertos del sistema. Se sigue escaneando según su programación. Aumente fs.inotify.max_user_instances en el host, o el límite de archivos abiertos del contenedor.", "%s: Polyfin non riesce a monitorare le modifiche di questa cartella locale: è stato raggiunto il limite di osservatori o di file aperti del sistema. Viene comunque scansionata secondo la sua pianificazione. Aumenta fs.inotify.max_user_instances sull’host, oppure il limite di file aperti del container.", "%s: o Polyfin não consegue monitorar alterações nesta pasta local: o limite de monitores ou de arquivos abertos do sistema foi atingido. Ela continua sendo rastreada conforme o agendamento. Aumente fs.inotify.max_user_instances no host, ou o limite de arquivos abertos do contêiner."}, name)
		case "open_files_limit":
			return say(i18n.Text{"%s: Polyfin cannot watch this local folder for changes: the system’s limit of open files is reached, as macOS watches each folder and file through one. " +
				"It is still scanned on its schedule. Raise kern.maxfilesperproc and kern.maxfiles with sysctl.",
				"%s : Polyfin ne peut pas surveiller les changements de ce dossier local : la limite de fichiers ouverts du système est atteinte, macOS surveillant chaque dossier et fichier par l’un d’eux. " +
					"Il est toujours analysé selon sa planification. Augmentez kern.maxfilesperproc et kern.maxfiles avec sysctl.", "%s: Polyfin kann diesen lokalen Ordner nicht auf Änderungen überwachen: Das Systemlimit für offene Dateien ist erreicht, da macOS jeden Ordner und jede Datei über eine offene Datei überwacht. Er wird weiterhin nach seinem Zeitplan gescannt. Erhöhe kern.maxfilesperproc und kern.maxfiles mit sysctl.", "%s: Polyfin no puede vigilar los cambios de esta carpeta local: se ha alcanzado el límite de archivos abiertos del sistema, ya que macOS vigila cada carpeta y archivo mediante uno. Se sigue escaneando según su programación. Aumente kern.maxfilesperproc y kern.maxfiles con sysctl.", "%s: Polyfin non riesce a monitorare le modifiche di questa cartella locale: è stato raggiunto il limite di file aperti del sistema, poiché macOS monitora ogni cartella e ogni file tramite uno di essi. Viene comunque scansionata secondo la sua pianificazione. Aumenta kern.maxfilesperproc e kern.maxfiles con sysctl.", "%s: o Polyfin não consegue monitorar alterações nesta pasta local: o limite de arquivos abertos do sistema foi atingido, pois o macOS monitora cada pasta e arquivo por meio de um deles. Ela continua sendo rastreada conforme o agendamento. Aumente kern.maxfilesperproc e kern.maxfiles com sysctl."}, name)
		}
		return say(i18n.Text{"%s: Polyfin cannot watch this local folder for changes. It is still scanned on its schedule.",
			"%s : Polyfin ne peut pas surveiller les changements de ce dossier local. Il est toujours analysé selon sa planification.", "%s: Polyfin kann diesen lokalen Ordner nicht auf Änderungen überwachen. Er wird weiterhin nach seinem Zeitplan gescannt.", "%s: Polyfin no puede vigilar los cambios de esta carpeta local. Se sigue escaneando según su programación.", "%s: Polyfin non riesce a monitorare le modifiche di questa cartella locale. Viene comunque scansionata secondo la sua pianificazione.", "%s: o Polyfin não consegue monitorar alterações nesta pasta local. Ela continua sendo rastreada conforme o agendamento."}, name)
	case problemFolderSearch:
		why := ""
		if failure, known := searchFailures[p.Failure]; known && p.Addon != "" {
			why = say(i18n.Text{" (%s: %s)", " (%s : %s)", " (%s: %s)", " (%s: %s)", " (%s: %s)", " (%s: %s)"}, p.Addon, failure.In(language))
		} else if p.Addon != "" {
			why = " (" + p.Addon + ")"
		}
		return say(i18n.Text{"%s: the titles of its files could not be searched%s. They stay unmatched until a search goes through, which Polyfin tries again on its own, after a few minutes, then less often.",
			"%s : les titres de ses fichiers n’ont pas pu être recherchés%s. Ils restent non associés jusqu’à ce qu’une recherche aboutisse, ce que Polyfin réessaie de lui-même, après quelques minutes, puis moins souvent.",
			"%s: Die Titel seiner Dateien konnten nicht gesucht werden%s. Sie bleiben nicht zugeordnet, bis eine Suche gelingt, was Polyfin von selbst erneut versucht, erst nach ein paar Minuten, dann seltener.",
			"%s: no se pudieron buscar los títulos de sus archivos%s. Siguen sin asociar hasta que una búsqueda salga bien, lo que Polyfin vuelve a intentar por sí solo, al cabo de unos minutos y luego con menos frecuencia.",
			"%s: non è stato possibile cercare i titoli dei suoi file%s. Restano non associati finché una ricerca non va a buon fine, cosa che Polyfin riprova da solo, dopo qualche minuto, poi meno spesso.",
			"%s: não foi possível pesquisar os títulos dos arquivos%s. Eles continuam sem associação até que uma pesquisa dê certo, o que o Polyfin tenta de novo sozinho, depois de alguns minutos e, então, com menos frequência."},
			name, why)
	case problemGuide:
		return say(i18n.Text{"%s: the last programme guide fetch failed.", "%s : la dernière récupération du guide a échoué.", "%s: Der letzte Abruf des Fernsehprogramms ist fehlgeschlagen.", "%s: ha fallado la última obtención de la guía de programación.", "%s: l’ultimo recupero della guida TV non è riuscito.", "%s: a última obtenção do guia de programação falhou."}, name)
	case problemSecretsUnreadable:
		if len(p.Secrets) == 1 {
			return say(i18n.Text{"1 stored key cannot be decrypted with POLYFIN_SECRET_KEY and counts as not set.",
				"1 clé enregistrée ne peut pas être déchiffrée avec POLYFIN_SECRET_KEY et compte comme absente.",
				"1 gespeicherter Schlüssel kann mit POLYFIN_SECRET_KEY nicht entschlüsselt werden und gilt als nicht festgelegt.",
				"1 clave guardada no se puede descifrar con POLYFIN_SECRET_KEY y se considera no definida.",
				"1 chiave salvata non può essere decifrata con POLYFIN_SECRET_KEY e viene considerata non impostata.",
				"1 chave armazenada não pode ser descriptografada com POLYFIN_SECRET_KEY e é considerada não definida."})
		}
		return say(i18n.Text{"%d stored keys cannot be decrypted with POLYFIN_SECRET_KEY and count as not set.",
			"%d clés enregistrées ne peuvent pas être déchiffrées avec POLYFIN_SECRET_KEY et comptent comme absentes.",
			"%d gespeicherte Schlüssel können mit POLYFIN_SECRET_KEY nicht entschlüsselt werden und gelten als nicht festgelegt.",
			"%d claves guardadas no se pueden descifrar con POLYFIN_SECRET_KEY y se consideran no definidas.",
			"%d chiavi salvate non possono essere decifrate con POLYFIN_SECRET_KEY e vengono considerate non impostate.",
			"%d chaves armazenadas não podem ser descriptografadas com POLYFIN_SECRET_KEY e são consideradas não definidas."}, len(p.Secrets))
	case problemSecretsPlaintext:
		return say(i18n.Text{"Keys and tokens are stored unencrypted. Set POLYFIN_SECRET_KEY to encrypt them.",
			"Les clés et jetons sont enregistrés sans chiffrement. Définissez POLYFIN_SECRET_KEY pour les chiffrer.", "Schlüssel und Tokens werden unverschlüsselt gespeichert. Lege POLYFIN_SECRET_KEY fest, um sie zu verschlüsseln.", "Las claves y los tokens se guardan sin cifrar. Defina POLYFIN_SECRET_KEY para cifrarlos.", "Chiavi e token sono salvati senza cifratura. Imposta POLYFIN_SECRET_KEY per cifrarli.", "Chaves e tokens são armazenados sem criptografia. Defina POLYFIN_SECRET_KEY para criptografá-los."})
	case problemBackupFailed:
		return say(i18n.Text{"The last database backup failed.", "La dernière sauvegarde de la base de données a échoué.", "Das letzte Datenbank-Backup ist fehlgeschlagen.", "Ha fallado la última copia de seguridad de la base de datos.", "L’ultimo backup del database non è riuscito.", "O último backup do banco de dados falhou."})
	case problemBackupStale:
		return say(i18n.Text{"The last database backup is more than two days old.", "La dernière sauvegarde de la base de données date de plus de deux jours.", "Das letzte Datenbank-Backup ist älter als zwei Tage.", "La última copia de seguridad de la base de datos tiene más de dos días.", "L’ultimo backup del database risale a più di due giorni fa.", "O último backup do banco de dados tem mais de dois dias."})
	case problemDestinationFailed:
		return say(i18n.Text{"%s: the last upload of the database backups failed.", "%s : le dernier envoi des sauvegardes de la base de données a échoué.",
			"%s: Der letzte Upload der Datenbank-Backups ist fehlgeschlagen.", "%s: ha fallado la última subida de las copias de seguridad de la base de datos.", "%s: l’ultimo caricamento dei backup del database non è riuscito.", "%s: o último envio dos backups do banco de dados falhou."}, name)
	case problemDestinationStale:
		return say(i18n.Text{"%s: the newest database backup there is more than two days old.",
			"%s : la sauvegarde de la base de données la plus récente qui s’y trouve date de plus de deux jours.", "%s: Das neueste Datenbank-Backup dort ist älter als zwei Tage.", "%s: la copia de seguridad de la base de datos más reciente que hay ahí tiene más de dos días.", "%s: il backup del database più recente presente lì risale a più di due giorni fa.", "%s: o backup do banco de dados mais recente nesse destino tem mais de dois dias."}, name)
	case problemTask:
		return say(i18n.Text{"The task “%s” failed.", "La tâche « %s » a échoué.", "Die Aufgabe „%s“ ist fehlgeschlagen.", "Ha fallado la tarea «%s».", "L’operazione «%s» non è riuscita.", "A tarefa “%s” falhou."}, p.Task)
	case problemUnknownProxy:
		return say(i18n.Text{"%s forwards requests (X-Forwarded-For) but is not a known proxy, so its clients all count as %s. " +
			"If it is your reverse proxy, add it to Known proxies under Settings › Security.",
			"%s transmet des requêtes (X-Forwarded-For) sans être un proxy connu : tous ses clients comptent donc comme %s. " +
				"Si c’est votre proxy inverse, ajoutez-le aux Proxys connus dans Paramètres › Sécurité.", "%s leitet Anfragen weiter (X-Forwarded-For), ist aber kein bekannter Proxy, daher zählen alle seine Clients als %s. Wenn es dein Reverse-Proxy ist, füge ihn unter Einstellungen › Sicherheit zu „Bekannte Proxys“ hinzu.", "%s reenvía solicitudes (X-Forwarded-For) pero no es un proxy conocido, así que todos sus clientes cuentan como %s. Si es su proxy inverso, añádalo a «Proxies conocidos» en Ajustes › Seguridad.", "%s inoltra richieste (X-Forwarded-For) ma non è un proxy noto, quindi tutti i suoi client vengono considerati come %s. Se è il tuo reverse proxy, aggiungilo a Proxy noti in Impostazioni › Sicurezza.", "%s encaminha requisições (X-Forwarded-For), mas não é um proxy conhecido, então todos os clientes dele contam como %s. Se for o seu proxy reverso, adicione-o em Proxies conhecidos, em Configurações › Segurança."}, p.Address, p.Address)
	case problemProxyName:
		return say(i18n.Text{"The known proxy %s resolves to no address, so no request is trusted to come from it. " +
			"Check its name under Settings › Security, and that Polyfin shares a network with it.",
			"Le proxy connu %s ne correspond à aucune adresse : aucune requête n’est donc reconnue comme venant de lui. " +
				"Vérifiez son nom dans Paramètres › Sécurité, et que Polyfin partage un réseau avec lui.", "Der bekannte Proxy %s wird zu keiner Adresse aufgelöst, daher wird keiner Anfrage vertraut, die von ihm kommt. Prüfe seinen Namen unter Einstellungen › Sicherheit und ob Polyfin ein Netzwerk mit ihm teilt.", "El proxy conocido %s no se resuelve en ninguna dirección, así que no se confía en que ninguna solicitud proceda de él. Compruebe su nombre en Ajustes › Seguridad y que Polyfin comparte una red con él.", "Il proxy noto %s non corrisponde a nessun indirizzo, quindi nessuna richiesta viene considerata attendibile come proveniente da esso. Verifica il suo nome in Impostazioni › Sicurezza e che Polyfin condivida una rete con esso.", "O proxy conhecido %s não resolve para nenhum endereço, então nenhuma requisição é considerada confiável como vinda dele. Verifique o nome dele em Configurações › Segurança e se o Polyfin compartilha uma rede com ele."}, p.Name)
	case problemDiscoveryPort:
		return say(i18n.Text{"Polyfin cannot listen on UDP port %d, so Jellyfin apps on the local network do not find it by themselves. " +
			"Another program probably uses it, such as a Jellyfin server on the same host. Saving the settings tries again.",
			"Polyfin ne peut pas écouter sur le port UDP %d : les applications Jellyfin du réseau local ne le trouvent donc pas d’elles-mêmes. " +
				"Un autre programme l’utilise sans doute, comme un serveur Jellyfin sur le même hôte. Enregistrer les paramètres réessaie.", "Polyfin kann nicht auf UDP-Port %d lauschen, daher finden Jellyfin-Apps im lokalen Netzwerk den Server nicht von selbst. Wahrscheinlich nutzt ein anderes Programm den Port, etwa ein Jellyfin-Server auf demselben Host. Beim Speichern der Einstellungen wird es erneut versucht.", "Polyfin no puede escuchar en el puerto UDP %d, así que las aplicaciones de Jellyfin de la red local no lo encuentran por sí solas. Probablemente otro programa lo está usando, como un servidor Jellyfin en el mismo host. Al guardar los ajustes se vuelve a intentar.", "Polyfin non riesce a mettersi in ascolto sulla porta UDP %d, quindi le applicazioni Jellyfin sulla rete locale non lo trovano da sole. Probabilmente la usa un altro programma, ad esempio un server Jellyfin sullo stesso host. Salvando le impostazioni si riprova.", "O Polyfin não consegue escutar na porta UDP %d, então os aplicativos Jellyfin na rede local não o encontram sozinhos. Provavelmente outro programa a está usando, como um servidor Jellyfin no mesmo host. Salvar as configurações tenta novamente."}, p.Port)
	case problemTorrentRoute:
		var why i18n.Text
		switch p.Failure {
		case torrent.InterfaceMissing:
			why = i18n.Text{"the network interface " + p.Name + " is missing", "l’interface réseau " + p.Name + " est absente", "die Netzwerkschnittstelle " + p.Name + " fehlt", "no se encuentra la interfaz de red " + p.Name, "l’interfaccia di rete " + p.Name + " non esiste", "a interface de rede " + p.Name + " não existe"}
		case torrent.InterfaceDown:
			why = i18n.Text{"the network interface " + p.Name + " is down", "l’interface réseau " + p.Name + " est désactivée", "die Netzwerkschnittstelle " + p.Name + " ist deaktiviert", "la interfaz de red " + p.Name + " está desactivada", "l’interfaccia di rete " + p.Name + " è disattivata", "a interface de rede " + p.Name + " está inativa"}
		case torrent.InterfaceNoAddress:
			why = i18n.Text{"the network interface " + p.Name + " has no address", "l’interface réseau " + p.Name + " n’a pas d’adresse", "die Netzwerkschnittstelle " + p.Name + " hat keine Adresse", "la interfaz de red " + p.Name + " no tiene dirección", "l’interfaccia di rete " + p.Name + " non ha un indirizzo", "a interface de rede " + p.Name + " não tem endereço"}
		case torrent.InterfaceUnsupported:
			why = i18n.Text{"a network interface works on Linux only", "une interface réseau ne fonctionne que sous Linux", "eine Netzwerkschnittstelle funktioniert nur unter Linux", "las interfaces de red solo funcionan en Linux", "un’interfaccia di rete funziona solo su Linux", "uma interface de rede só funciona no Linux"}
		case torrent.ProxyUnreachable:
			why = i18n.Text{"the SOCKS5 proxy " + p.Name + " does not answer", "le proxy SOCKS5 " + p.Name + " ne répond pas", "der SOCKS5-Proxy " + p.Name + " antwortet nicht", "el proxy SOCKS5 " + p.Name + " no responde", "il proxy SOCKS5 " + p.Name + " non risponde", "o proxy SOCKS5 " + p.Name + " não responde"}
		case torrent.ProxyRefused:
			why = i18n.Text{"the SOCKS5 proxy " + p.Name + " refused the user and password", "le proxy SOCKS5 " + p.Name + " a refusé l’utilisateur et le mot de passe", "der SOCKS5-Proxy " + p.Name + " hat Benutzer und Passwort abgelehnt", "el proxy SOCKS5 " + p.Name + " ha rechazado el usuario y la contraseña", "il proxy SOCKS5 " + p.Name + " ha rifiutato l’utente e la password", "o proxy SOCKS5 " + p.Name + " recusou o usuário e a senha"}
		case torrent.WireGuardEndpoint:
			why = i18n.Text{"the WireGuard server " + p.Name + " cannot be looked up", "le serveur WireGuard " + p.Name + " est introuvable",
				"der WireGuard-Server " + p.Name + " kann nicht aufgelöst werden", "no se encuentra el servidor WireGuard " + p.Name,
				"il server WireGuard " + p.Name + " non può essere risolto", "o servidor WireGuard " + p.Name + " não pode ser encontrado"}
		case torrent.WireGuardDown:
			why = i18n.Text{"the WireGuard tunnel to " + p.Name + " cannot start", "le tunnel WireGuard vers " + p.Name + " ne peut pas démarrer",
				"der WireGuard-Tunnel zu " + p.Name + " kann nicht starten", "el túnel WireGuard hacia " + p.Name + " no puede iniciarse",
				"il tunnel WireGuard verso " + p.Name + " non può avviarsi", "o túnel WireGuard para " + p.Name + " não pode iniciar"}
		case torrent.WireGuardHandshake:
			why = i18n.Text{"the WireGuard server " + p.Name + " does not answer the handshake: it is unreachable, or a key of the configuration is wrong",
				"le serveur WireGuard " + p.Name + " ne répond pas à la négociation : il est injoignable, ou une clé de la configuration est erronée",
				"der WireGuard-Server " + p.Name + " antwortet nicht auf den Handshake: Er ist nicht erreichbar, oder ein Schlüssel der Konfiguration ist falsch",
				"el servidor WireGuard " + p.Name + " no responde a la negociación: no se puede contactar o una clave de la configuración es incorrecta",
				"il server WireGuard " + p.Name + " non risponde all’handshake: non è raggiungibile oppure una chiave della configurazione è errata",
				"o servidor WireGuard " + p.Name + " não responde ao handshake: ele está inacessível ou uma chave da configuração está errada"}
		default:
			why = i18n.Text{"no route is set", "aucune route n’est définie", "es ist keine Route festgelegt", "no hay ninguna ruta definida", "nessun percorso impostato", "nenhuma rota está definida"}
		}
		return say(i18n.Text{"Torrent streams do not play: %s. Nothing is sent another way; check the route under Settings › Torrents.",
			"Les flux torrent ne sont pas lus : %s. Rien n’est envoyé par un autre chemin ; vérifiez la route dans Paramètres › Torrents.", "Torrent-Streams werden nicht abgespielt: %s. Es wird nichts auf anderem Weg gesendet; prüfe die Route unter Einstellungen › Torrents.", "Las transmisiones torrent no se reproducen: %s. No se envía nada por otra vía; compruebe la ruta en Ajustes › Torrents.", "Gli stream torrent non vengono riprodotti: %s. Nulla viene inviato per altra via; verifica il percorso in Impostazioni › Torrent.", "As streams de torrent não são reproduzidas: %s. Nada é enviado por outro caminho; verifique a rota em Configurações › Torrents."}, why.In(language))
	case problemTorrentUDP:
		return say(i18n.Text{"The SOCKS5 proxy %s does not offer UDP ASSOCIATE: torrents play over TCP, with UDP trackers and the DHT off.",
			"Le proxy SOCKS5 %s ne propose pas UDP ASSOCIATE : les torrents sont lus en TCP, sans trackers UDP ni DHT.", "Der SOCKS5-Proxy %s bietet kein UDP ASSOCIATE an: Torrents werden über TCP abgespielt, ohne UDP-Tracker und DHT.", "El proxy SOCKS5 %s no ofrece UDP ASSOCIATE: los torrents se reproducen por TCP, sin trackers UDP ni DHT.", "Il proxy SOCKS5 %s non supporta UDP ASSOCIATE: i torrent vengono riprodotti tramite TCP, con tracker UDP e DHT disattivati.", "O proxy SOCKS5 %s não oferece UDP ASSOCIATE: os torrents são reproduzidos via TCP, com os trackers UDP e o DHT desligados."}, p.Name)
	case problemTorrentStart:
		why := map[string]i18n.Text{
			"no_udp": {"no peer was found, and the route carries no UDP, which the DHT and UDP trackers need",
				"aucun pair n’a été trouvé, et la route ne transporte pas l’UDP, dont ont besoin le DHT et les trackers UDP",
				"es wurde kein Peer gefunden, und die Route überträgt kein UDP, das DHT und UDP-Tracker brauchen",
				"no se encontró ningún par y la ruta no transporta UDP, que necesitan la DHT y los trackers UDP",
				"non è stato trovato nessun peer e il percorso non trasporta UDP, necessario alla DHT e ai tracker UDP",
				"nenhum peer foi encontrado, e a rota não transporta UDP, de que a DHT e os trackers UDP precisam"},
			"refused": {"its trackers refused the announce or did not answer, and the DHT found no peer",
				"ses trackers ont refusé l’annonce ou n’ont pas répondu, et le DHT n’a trouvé aucun pair",
				"seine Tracker haben die Ankündigung abgelehnt oder nicht geantwortet, und das DHT hat keinen Peer gefunden",
				"sus trackers rechazaron el anuncio o no respondieron, y la DHT no encontró ningún par",
				"i suoi tracker hanno rifiutato l’annuncio o non hanno risposto, e la DHT non ha trovato nessun peer",
				"os trackers recusaram o anúncio ou não responderam, e a DHT não encontrou nenhum peer"},
			"no_peers": {"its trackers and the DHT found no peer", "ses trackers et le DHT n’ont trouvé aucun pair",
				"seine Tracker und das DHT haben keinen Peer gefunden", "sus trackers y la DHT no encontraron ningún par",
				"i suoi tracker e la DHT non hanno trovato nessun peer", "os trackers e a DHT não encontraram nenhum peer"},
			"silent": {"peers were found, but none sent it", "des pairs ont été trouvés, mais aucun ne l’a envoyé",
				"es wurden Peers gefunden, aber keiner hat ihn gesendet", "se encontraron pares, pero ninguno lo envió",
				"sono stati trovati dei peer, ma nessuno l’ha inviato", "foram encontrados peers, mas nenhum o enviou"},
		}[p.Failure]
		return say(i18n.Text{"A torrent did not start in time: %s. Check that the route reaches the internet; the log tells each tracker’s answer at debug level.",
			"Un torrent n’a pas démarré à temps : %s. Vérifiez que la route atteint Internet ; le journal donne la réponse de chaque tracker au niveau débogage.",
			"Ein Torrent ist nicht rechtzeitig gestartet: %s. Prüfe, ob die Route das Internet erreicht; das Protokoll nennt auf der Debug-Stufe die Antwort jedes Trackers.",
			"Un torrent no se inició a tiempo: %s. Compruebe que la ruta llega a Internet; el registro indica la respuesta de cada tracker en el nivel de depuración.",
			"Un torrent non è partito in tempo: %s. Verifica che il percorso raggiunga Internet; il log riporta la risposta di ogni tracker a livello di debug.",
			"Um torrent não começou a tempo: %s. Verifique se a rota alcança a internet; o registro mostra a resposta de cada tracker no nível de depuração."},
			why.In(language))
	case problemDLNAPort:
		return say(i18n.Text{"Polyfin cannot listen on UDP port %d, so DLNA devices on the local network do not find it by themselves. " +
			"Another program probably uses it, such as another DLNA server on the same host. Saving the settings tries again.",
			"Polyfin ne peut pas écouter sur le port UDP %d : les appareils DLNA du réseau local ne le trouvent donc pas d’eux-mêmes. " +
				"Un autre programme l’utilise sans doute, comme un autre serveur DLNA sur le même hôte. Enregistrer les paramètres réessaie.", "Polyfin kann nicht auf UDP-Port %d lauschen, daher finden DLNA-Geräte im lokalen Netzwerk den Server nicht von selbst. Wahrscheinlich nutzt ein anderes Programm den Port, etwa ein anderer DLNA-Server auf demselben Host. Beim Speichern der Einstellungen wird es erneut versucht.", "Polyfin no puede escuchar en el puerto UDP %d, así que los dispositivos DLNA de la red local no lo encuentran por sí solos. Probablemente otro programa lo está usando, como otro servidor DLNA en el mismo host. Al guardar los ajustes se vuelve a intentar.", "Polyfin non riesce a mettersi in ascolto sulla porta UDP %d, quindi i dispositivi DLNA sulla rete locale non lo trovano da soli. Probabilmente la usa un altro programma, ad esempio un altro server DLNA sullo stesso host. Salvando le impostazioni si riprova.", "O Polyfin não consegue escutar na porta UDP %d, então os dispositivos DLNA na rede local não o encontram sozinhos. Provavelmente outro programa a está usando, como outro servidor DLNA no mesmo host. Salvar as configurações tenta novamente."}, p.Port)
	case problemDiscoveryAddress:
		return say(i18n.Text{"Jellyfin apps that look for servers on the local network are given %s, an address of Polyfin’s container they cannot reach. " +
			"Set Address on the local network under Settings › General.",
			"Les applications Jellyfin qui cherchent des serveurs sur le réseau local reçoivent %s, une adresse du conteneur de Polyfin qu’elles ne peuvent pas joindre. " +
				"Renseignez Adresse sur le réseau local dans Paramètres › Général.", "Jellyfin-Apps, die im lokalen Netzwerk nach Servern suchen, erhalten %s, eine Adresse des Polyfin-Containers, die sie nicht erreichen können. Lege „Adresse im lokalen Netzwerk“ unter Einstellungen › Allgemein fest.", "Las aplicaciones de Jellyfin que buscan servidores en la red local reciben %s, una dirección del contenedor de Polyfin a la que no pueden acceder. Defina «Dirección en la red local» en Ajustes › General.", "Le applicazioni Jellyfin che cercano server sulla rete locale ricevono %s, un indirizzo del container di Polyfin che non possono raggiungere. Imposta Indirizzo sulla rete locale in Impostazioni › Generale.", "Os aplicativos Jellyfin que procuram servidores na rede local recebem %s, um endereço do contêiner do Polyfin que eles não conseguem acessar. Defina Endereço na rede local em Configurações › Geral."}, p.Address)
	case problemHTTPSPort:
		return say(i18n.Text{"Polyfin cannot listen on TCP port %d, so HTTPS is not served. HTTP keeps working. " +
			"Another program probably uses it, or it is below 1024. Choose another HTTPS port under Settings › Security.",
			"Polyfin ne peut pas écouter sur le port TCP %d : HTTPS n’est donc pas servi. HTTP fonctionne toujours. " +
				"Un autre programme l’utilise sans doute, ou il est inférieur à 1024. Choisissez un autre port HTTPS dans Paramètres › Sécurité.", "Polyfin kann nicht auf TCP-Port %d lauschen, daher wird HTTPS nicht bereitgestellt. HTTP funktioniert weiterhin. Wahrscheinlich nutzt ein anderes Programm den Port, oder er liegt unter 1024. Wähle unter Einstellungen › Sicherheit einen anderen HTTPS-Port.", "Polyfin no puede escuchar en el puerto TCP %d, así que no se sirve HTTPS. HTTP sigue funcionando. Probablemente otro programa lo está usando, o es inferior a 1024. Elija otro puerto HTTPS en Ajustes › Seguridad.", "Polyfin non riesce a mettersi in ascolto sulla porta TCP %d, quindi HTTPS non è disponibile. HTTP continua a funzionare. Probabilmente la usa un altro programma, oppure è inferiore a 1024. Scegli un’altra porta HTTPS in Impostazioni › Sicurezza.", "O Polyfin não consegue escutar na porta TCP %d, então o HTTPS não é servido. O HTTP continua funcionando. Provavelmente outro programa a está usando, ou ela é inferior a 1024. Escolha outra porta HTTPS em Configurações › Segurança."}, p.Port)
	case problemHTTPSFiles:
		switch p.Failure {
		case https.FilesMissing:
			return say(i18n.Text{"The HTTPS certificate file %s does not exist. The certificate last read stays in use.",
				"Le fichier de certificat HTTPS %s n’existe pas. Le dernier certificat lu reste utilisé.", "Die HTTPS-Zertifikatsdatei %s existiert nicht. Das zuletzt gelesene Zertifikat bleibt in Verwendung.", "El archivo de certificado HTTPS %s no existe. Se sigue usando el último certificado leído.", "Il file del certificato HTTPS %s non esiste. Resta in uso l’ultimo certificato letto.", "O arquivo de certificado HTTPS %s não existe. O último certificado lido continua em uso."}, p.File)
		case https.FilesMismatch:
			return say(i18n.Text{"The HTTPS private key does not belong to the certificate. The certificate last read stays in use.",
				"La clé privée HTTPS ne correspond pas au certificat. Le dernier certificat lu reste utilisé.", "Der private HTTPS-Schlüssel gehört nicht zum Zertifikat. Das zuletzt gelesene Zertifikat bleibt in Verwendung.", "La clave privada HTTPS no corresponde al certificado. Se sigue usando el último certificado leído.", "La chiave privata HTTPS non corrisponde al certificato. Resta in uso l’ultimo certificato letto.", "A chave privada HTTPS não corresponde ao certificado. O último certificado lido continua em uso."})
		}
		return say(i18n.Text{"Polyfin cannot read the HTTPS certificate file %s, or it holds no PEM certificate or key it reads. " +
			"The certificate last read stays in use.",
			"Polyfin ne peut pas lire le fichier de certificat HTTPS %s, ou il ne contient aucun certificat ou clé PEM qu’il sait lire. " +
				"Le dernier certificat lu reste utilisé.", "Polyfin kann die HTTPS-Zertifikatsdatei %s nicht lesen, oder sie enthält kein lesbares PEM-Zertifikat bzw. keinen lesbaren PEM-Schlüssel. Das zuletzt gelesene Zertifikat bleibt in Verwendung.", "Polyfin no puede leer el archivo de certificado HTTPS %s, o no contiene ningún certificado ni clave PEM que pueda leer. Se sigue usando el último certificado leído.", "Polyfin non riesce a leggere il file del certificato HTTPS %s, oppure non contiene un certificato o una chiave PEM leggibile. Resta in uso l’ultimo certificato letto.", "O Polyfin não consegue ler o arquivo de certificado HTTPS %s, ou ele não contém nenhum certificado ou chave PEM que o Polyfin consiga ler. O último certificado lido continua em uso."}, p.File)
	case problemHTTPSRenewal:
		return say(i18n.Text{"Let’s Encrypt did not give the HTTPS certificate: %s", "Let’s Encrypt n’a pas délivré le certificat HTTPS : %s", "Let’s Encrypt hat das HTTPS-Zertifikat nicht ausgestellt: %s", "Let’s Encrypt no ha emitido el certificado HTTPS: %s", "Let’s Encrypt non ha rilasciato il certificato HTTPS: %s", "O Let’s Encrypt não emitiu o certificado HTTPS: %s"}, p.Error)
	case problemHTTPSExpiry:
		if p.Tone == notifications.SeverityError {
			return say(i18n.Text{"The HTTPS certificate expired on %s.", "Le certificat HTTPS a expiré le %s.", "Das HTTPS-Zertifikat ist am %s abgelaufen.", "El certificado HTTPS caducó el %s.", "Il certificato HTTPS è scaduto il %s.", "O certificado HTTPS expirou em %s."}, p.Expires.Format(time.DateOnly))
		}
		return say(i18n.Text{"The HTTPS certificate expires on %s.", "Le certificat HTTPS expire le %s.", "Das HTTPS-Zertifikat läuft am %s ab.", "El certificado HTTPS caduca el %s.", "Il certificato HTTPS scade il %s.", "O certificado HTTPS expira em %s."}, p.Expires.Format(time.DateOnly))
	case problemOCRLanguage:
		return say(i18n.Text{"Picture subtitles in %s are not read into text: Tesseract has no data for this language. " +
			"Add the file %s to the folder POLYFIN_TESSDATA_DIR names.",
			"Les sous-titres en images en %s ne sont pas lus en texte : Tesseract n’a pas de données pour cette langue. " +
				"Ajoutez le fichier %s au dossier que nomme POLYFIN_TESSDATA_DIR.", "Bilduntertitel in %s werden nicht in Text umgewandelt: Tesseract hat keine Daten für diese Sprache. Füge die Datei %s dem Ordner hinzu, den POLYFIN_TESSDATA_DIR angibt.", "Los subtítulos en imagen en %s no se convierten a texto: Tesseract no tiene datos para este idioma. Añada el archivo %s a la carpeta que indica POLYFIN_TESSDATA_DIR.", "I sottotitoli grafici in %s non vengono convertiti in testo: Tesseract non ha dati per questa lingua. Aggiungi il file %s alla cartella indicata da POLYFIN_TESSDATA_DIR.", "As legendas em imagem em %s não são convertidas em texto: o Tesseract não tem dados para este idioma. Adicione o arquivo %s à pasta indicada por POLYFIN_TESSDATA_DIR."}, p.Name, p.File)
	case problemMetadataLanguages:
		return say(i18n.Text{"%s: every metadata addon of the server has a language, so they describe titles in their language for every user, whatever their metadata language. " +
			"Keep Language of its descriptions at None on this addon, or on one more metadata addon.",
			"%s : chaque addon de métadonnées du serveur a une langue : ils décrivent donc les titres dans leur langue pour tous les utilisateurs, quelle que soit leur langue des métadonnées. " +
				"Laissez Langue de ses descriptions sur Aucune pour cet addon, ou pour un addon de métadonnées de plus.", "%s: Jedes Metadaten-Add-on des Servers hat eine Sprache, daher beschreiben sie Titel für jeden Benutzer in ihrer Sprache, unabhängig von seiner Metadatensprache. Belasse „Sprache der Beschreibungen“ bei diesem Add-on oder bei einem weiteren Metadaten-Add-on auf „Keine“.", "%s: todas las extensiones de metadatos del servidor tienen un idioma, así que describen los títulos en su idioma para todos los usuarios, sea cual sea su idioma de los metadatos. Deje «Idioma de sus descripciones» en «Ninguno» en esta extensión o en otra extensión de metadatos más.", "%s: ogni add-on di metadati del server ha una lingua, quindi descrivono i titoli nella propria lingua per tutti gli utenti, qualunque sia la loro lingua dei metadati. Lascia Lingua delle descrizioni su Nessuna in questo add-on, o in un altro add-on di metadati.", "%s: todos os complementos de metadados do servidor têm um idioma, então eles descrevem os títulos no idioma deles para todos os usuários, seja qual for o idioma dos metadados de cada um. Mantenha Idioma das descrições como Nenhum neste complemento, ou em mais um complemento de metadados."}, p.Name)
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

// gigabytes writes bytes in gigabytes with one decimal, as language
// writes it: a decimal point in English, a comma in the others, and "Go"
// in French.
func gigabytes(bytes int64, language string) string {
	text := strconv.FormatFloat(float64(bytes)/1e9, 'f', 1, 64)
	switch language {
	case "en":
		return text + " GB"
	case "fr":
		return strings.Replace(text, ".", ",", 1) + " Go"
	}
	return strings.Replace(text, ".", ",", 1) + " GB"
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

package admin

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/diskspace"
	"github.com/moodiness/polyfin/internal/notifications"
	"github.com/moodiness/polyfin/internal/tasks"
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
	problemIPTV              = "iptv"
	problemFolder            = "folder"
	problemGuide             = "guide"
	problemTask              = "task"
)

// healthProblemJSON is something System › Health shows as needing
// attention. Key names it for as long as it lasts; Code tells what it is,
// with the fields it fills: Folder and Free (bytes) for a disk; Name and
// Owner (nil for the server's) for an addon, IPTV source, local folder or
// guide, with Failure, the failure's code, for an addon or a local folder,
// and Share, the kind of network share a folder is ("smb", "webdav"); Host
// for paused thumbnails; Task, in the language asked, for a task; Secrets
// for those the key cannot decrypt. To is the admin app's page that
// describes it, with its anchor. Transient ones come and go with the load:
// notifications leave them out.
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
	Share     string           `json:"share,omitempty"`
	Host      string           `json:"host,omitempty"`
	Task      string           `json:"task,omitempty"`
	Secrets   []unreadableJSON `json:"secrets,omitempty"`
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
// server. With the database down, that alone is told: the rest is read from
// it.
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
			if source.Error != "" {
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

// HealthProblems returns what finds, for notifications, the problems System
// › Health shows, named in the server language, but for those that come
// and go with the load.
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

// diskFolders name the folders whose disks are measured, in English and
// French.
var diskFolders = map[string][2]string{
	"cache":      {"cache", "du cache"},
	"recordings": {"recordings", "des enregistrements"},
	"backups":    {"backups", "des sauvegardes"},
}

// text tells a problem notifications send in English or French. Those
// that come and go with the load are never sent: they have none.
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
	case problemTask:
		return say("The task “%s” failed.", "La tâche « %s » a échoué.", p.Task)
	}
	return ""
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

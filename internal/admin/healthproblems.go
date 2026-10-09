package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/diskspace"
	"github.com/moodiness/polyfin/internal/notifications"
	"github.com/moodiness/polyfin/internal/tasks"
)

// Below this much free space, or this share of the disk, a folder's disk is
// short of room, as System › Health tells.
const (
	lowDiskBytes = 2_000_000_000
	lowDiskShare = 0.05
)

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

// HealthProblems returns what finds the problems System › Health shows as
// needing attention, for notifications: the same as the admin app finds
// from the health page's data, named in the server language, but for those
// that come and go with the load (conversions at their limit, thumbnails
// paused for a host). It reads what options give, as the health page does,
// and sends no request outside the server.
func HealthProblems(options Options) func(context.Context) ([]notifications.Problem, error) {
	h := &handler{Options: options, now: time.Now}
	if options.Now != nil {
		h.now = options.Now
	}
	return h.healthProblems
}

func (h *handler) healthProblems(ctx context.Context) ([]notifications.Problem, error) {
	settings := h.Accounts.Settings()
	pick := func(words [2]string) string {
		if settings.Language == "fr" {
			return words[1]
		}
		return words[0]
	}
	say := func(english, french string, args ...any) string {
		return fmt.Sprintf(pick([2]string{english, french}), args...)
	}
	problems := []notifications.Problem{}
	add := func(key, severity, text string) {
		problems = append(problems, notifications.Problem{Key: key, Severity: severity, Text: text})
	}

	pingCtx, cancel := context.WithTimeout(ctx, readyWait)
	reachable := h.Database.Ping(pingCtx) == nil
	cancel()
	if !reachable {
		// The rest is read from the database.
		add("database", notifications.SeverityError, say("The database does not answer.", "La base de données ne répond pas."))
		return problems, nil
	}

	for _, folder := range []struct{ name, path string }{{"cache", h.Health.CacheDir}, {"recordings", h.Recordings.Dir()}, {"backups", h.Backups.Dir()}} {
		if folder.path == "" {
			continue
		}
		free, used, _, ok := diskspace.Measure(folder.path)
		if ok && free >= 0 && (free < lowDiskBytes || float64(free) < float64(free+used)*lowDiskShare) {
			add("disk:"+folder.name, notifications.SeverityWarning, say("Little space left for the %s: %s free.",
				"Il reste peu de place pour le dossier %s : %s libres.", pick(diskFolders[folder.name]), gigabytes(free, settings.Language)))
		}
	}

	scopes, err := h.ownedScopes(ctx)
	if err != nil {
		return nil, err
	}
	for _, scope := range scopes {
		owned := func(name string) string {
			if scope.Owner == nil {
				return name
			}
			return say("%s (%s’s addons)", "%s (Addons de %s)", name, scope.Owner.Name)
		}
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
				health, ok := h.Health.Addons.Health(addon.ManifestURL)
				if !ok || health.Failure == "" {
					continue
				}
				failure, known := addonFailures[health.Failure]
				if !known {
					failure = [2]string{health.Failure, health.Failure}
				}
				add("addon:"+addon.ID.String(), notifications.SeverityWarning, say("%s: %s", "%s : %s", owned(addon.Manifest.Name), pick(failure)))
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
				add("iptv:"+addon.ID.String(), notifications.SeverityWarning, say("%s: the last channel list download failed.",
					"%s : le dernier téléchargement de la liste a échoué.", owned(addon.Manifest.Name)))
			}
		}
		libraries, err := h.Addons.Libraries(ctx, scope.Scope)
		if err != nil {
			return nil, err
		}
		for _, l := range libraries {
			failed := false
			for _, guide := range l.Guides {
				failed = failed || guide.Error != ""
			}
			if !failed {
				continue
			}
			name := l.Catalog.Name
			if l.Name != nil && *l.Name != "" {
				name = *l.Name
			}
			add("guide:"+l.AddonID.String()+":"+l.Catalog.Type+":"+l.Catalog.ID, notifications.SeverityWarning,
				say("%s: the last programme guide fetch failed.", "%s : la dernière récupération du guide a échoué.", owned(name)))
		}
	}

	backupFailed := false
	backup, err := h.backupStatus(ctx)
	if err != nil {
		return nil, err
	}
	if backup != nil && backup.Problem {
		if backup.Error != "" {
			backupFailed = true
			add("backup", notifications.SeverityError, say("The last database backup failed.", "La dernière sauvegarde de la base de données a échoué."))
		} else {
			add("backup", notifications.SeverityWarning, say("The last database backup is more than two days old.",
				"La dernière sauvegarde de la base de données date de plus de deux jours."))
		}
	}
	if h.Tasks != nil {
		for _, task := range h.Tasks.Tasks(settings.Language) {
			// A failed backup is told above.
			if task.Key == "BackUpDatabase" && backupFailed {
				continue
			}
			if task.Last != nil && task.Last.Status == tasks.Failed {
				add("task:"+task.Key, notifications.SeverityWarning, say("The task “%s” failed.", "La tâche « %s » a échoué.", task.Name))
			}
		}
	}

	if h.Health.Secrets != nil {
		report, err := h.Health.Secrets(ctx)
		if err != nil {
			return nil, err
		}
		if len(report.Unreadable) > 0 {
			add("secrets:unreadable", notifications.SeverityError, say("%d stored keys cannot be decrypted with POLYFIN_SECRET_KEY and count as not set.",
				"%d clés enregistrées ne peuvent pas être déchiffrées avec POLYFIN_SECRET_KEY et comptent comme absentes.", len(report.Unreadable)))
		}
		if !h.Health.SecretKey && report.Plaintext > 0 {
			add("secrets:plaintext", notifications.SeverityWarning, say("Keys and tokens are stored unencrypted. Set POLYFIN_SECRET_KEY to encrypt them.",
				"Les clés et jetons sont enregistrés sans chiffrement. Définissez POLYFIN_SECRET_KEY pour les chiffrer."))
		}
	}
	return problems, nil
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

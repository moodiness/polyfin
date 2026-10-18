package main

import (
	"fmt"

	"github.com/moodiness/polyfin/internal/desktop/app"
)

// texts is the tray's text in one language.
type texts struct {
	starting, running, stopping, stopped, failed string
	downloading, updating                        string
	keyStore, tools, updateFailed                string
	setupCode                                    string
	openAdmin, openWeb, openLogs                 string
	start, stop, startWithComputer               string
	install, quit                                string
	copyKey, keyCopied, keyHint                  string
}

var english = texts{
	starting:          "Polyfin is starting",
	running:           "Polyfin is running",
	stopping:          "Polyfin is stopping",
	stopped:           "Polyfin is stopped",
	failed:            "Polyfin could not start: see the log folder",
	downloading:       "Downloading FFmpeg: %d%%",
	updating:          "Downloading the update: %d%%",
	keyStore:          "The system's key store could not be read",
	tools:             "FFmpeg could not be downloaded",
	updateFailed:      "The update could not be installed",
	setupCode:         "Setup code: %s (click to copy)",
	openAdmin:         "Open the admin app",
	openWeb:           "Open the web client",
	openLogs:          "Open the log folder",
	start:             "Start Polyfin",
	stop:              "Stop Polyfin",
	startWithComputer: "Start with the computer",
	install:           "Install Polyfin %s",
	quit:              "Quit Polyfin",
	copyKey:           "Copy the secret key",
	keyCopied:         "Secret key copied: keep it with your backups",
	keyHint:           "Copies the key the stored keys and tokens are encrypted with, to keep with your backups",
}

var french = texts{
	starting:          "Polyfin démarre",
	running:           "Polyfin fonctionne",
	stopping:          "Polyfin s’arrête",
	stopped:           "Polyfin est arrêté",
	failed:            "Polyfin n’a pas pu démarrer : voir le dossier des journaux",
	downloading:       "Téléchargement de FFmpeg : %d %%",
	updating:          "Téléchargement de la mise à jour : %d %%",
	keyStore:          "Le trousseau du système n’a pas pu être lu",
	tools:             "FFmpeg n’a pas pu être téléchargé",
	updateFailed:      "La mise à jour n’a pas pu être installée",
	setupCode:         "Code d’installation : %s (cliquer pour copier)",
	openAdmin:         "Ouvrir l’appli d’administration",
	openWeb:           "Ouvrir le client web",
	openLogs:          "Ouvrir le dossier des journaux",
	start:             "Démarrer Polyfin",
	stop:              "Arrêter Polyfin",
	startWithComputer: "Démarrer avec l’ordinateur",
	install:           "Installer Polyfin %s",
	quit:              "Quitter Polyfin",
	copyKey:           "Copier la clé secrète",
	keyCopied:         "Clé secrète copiée : gardez-la avec vos sauvegardes",
	keyHint:           "Copie la clé qui chiffre les clés et jetons enregistrés, à garder avec vos sauvegardes",
}

// formatted fills a text's %s.
func formatted(text, value string) string {
	return fmt.Sprintf(text, value)
}

func textsFor(language string) texts {
	if language == "fr" {
		return french
	}
	return english
}

// phase is the line that tells what Polyfin is doing.
func (c texts) phase(s app.Status) string {
	switch s.Phase {
	case app.Downloading:
		return fmt.Sprintf(c.downloading, s.Percent)
	case app.Updating:
		return fmt.Sprintf(c.updating, s.Percent)
	case app.Running:
		return c.running
	case app.Stopping:
		return c.stopping
	case app.Stopped:
		return c.stopped
	case app.Failed:
		return c.failed
	default:
		return c.starting
	}
}

// problems are the lines of what went wrong.
func (c texts) problems(s app.Status) []string {
	var lines []string
	for _, p := range s.Problems {
		switch p {
		case app.NoKeyStore:
			lines = append(lines, c.keyStore)
		case app.NoTools:
			lines = append(lines, c.tools)
		case app.UpdateFailed:
			lines = append(lines, c.updateFailed)
		}
	}
	return lines
}

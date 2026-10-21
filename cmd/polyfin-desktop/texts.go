package main

import (
	"fmt"

	"github.com/moodiness/polyfin/internal/desktop/app"
)

// texts is the tray's text in one language.
type texts struct {
	starting, running, stopping, stopped, failed string
	downloading, updating, rebuilding            string
	keyStore, tools, updateFailed                string
	setupCode                                    string
	openAdmin, openWeb, openLogs                 string
	start, stop, startWithComputer               string
	install, quit                                string
	copyKey, keyCopied, keyHint                  string
}

// trayTexts are the tray's texts in each language Polyfin speaks.
var trayTexts = map[string]texts{
	"en": {
		starting:          "Polyfin is starting",
		running:           "Polyfin is running",
		stopping:          "Polyfin is stopping",
		stopped:           "Polyfin is stopped",
		failed:            "Polyfin could not start: see the log folder",
		downloading:       "Downloading FFmpeg: %d%%",
		updating:          "Downloading the update: %d%%",
		rebuilding:        "Rebuilding the database's indexes",
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
	},
	"fr": {
		starting:          "Polyfin démarre",
		running:           "Polyfin fonctionne",
		stopping:          "Polyfin s’arrête",
		stopped:           "Polyfin est arrêté",
		failed:            "Polyfin n’a pas pu démarrer : voir le dossier des journaux",
		downloading:       "Téléchargement de FFmpeg : %d %%",
		updating:          "Téléchargement de la mise à jour : %d %%",
		rebuilding:        "Reconstruction des index de la base de données",
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
	},
	"de": {
		starting:          "Polyfin wird gestartet",
		running:           "Polyfin läuft",
		stopping:          "Polyfin wird angehalten",
		stopped:           "Polyfin ist angehalten",
		failed:            "Polyfin konnte nicht starten: siehe Protokollordner",
		downloading:       "FFmpeg wird heruntergeladen: %d %%",
		updating:          "Update wird heruntergeladen: %d %%",
		rebuilding:        "Die Indizes der Datenbank werden neu aufgebaut",
		keyStore:          "Der Schlüsselbund des Systems konnte nicht gelesen werden",
		tools:             "FFmpeg konnte nicht heruntergeladen werden",
		updateFailed:      "Das Update konnte nicht installiert werden",
		setupCode:         "Einrichtungscode: %s (zum Kopieren klicken)",
		openAdmin:         "Admin-App öffnen",
		openWeb:           "Web-Client öffnen",
		openLogs:          "Protokollordner öffnen",
		start:             "Polyfin starten",
		stop:              "Polyfin anhalten",
		startWithComputer: "Mit dem Computer starten",
		install:           "Polyfin %s installieren",
		quit:              "Polyfin beenden",
		copyKey:           "Geheimen Schlüssel kopieren",
		keyCopied:         "Geheimer Schlüssel kopiert: Bewahre ihn bei deinen Backups auf",
		keyHint:           "Kopiert den Schlüssel, mit dem die gespeicherten Schlüssel und Tokens verschlüsselt sind, zum Aufbewahren bei deinen Backups",
	},
	"es": {
		starting:          "Polyfin se está iniciando",
		running:           "Polyfin está en marcha",
		stopping:          "Polyfin se está deteniendo",
		stopped:           "Polyfin está detenido",
		failed:            "Polyfin no ha podido iniciarse: consulte la carpeta de registros",
		downloading:       "Descargando FFmpeg: %d %%",
		updating:          "Descargando la actualización: %d %%",
		rebuilding:        "Reconstruyendo los índices de la base de datos",
		keyStore:          "No se ha podido leer el almacén de claves del sistema",
		tools:             "No se ha podido descargar FFmpeg",
		updateFailed:      "No se ha podido instalar la actualización",
		setupCode:         "Código de configuración: %s (haga clic para copiarlo)",
		openAdmin:         "Abrir la aplicación de administración",
		openWeb:           "Abrir el cliente web",
		openLogs:          "Abrir la carpeta de registros",
		start:             "Iniciar Polyfin",
		stop:              "Detener Polyfin",
		startWithComputer: "Iniciar con el ordenador",
		install:           "Instalar Polyfin %s",
		quit:              "Salir de Polyfin",
		copyKey:           "Copiar la clave secreta",
		keyCopied:         "Clave secreta copiada: guárdela con sus copias de seguridad",
		keyHint:           "Copia la clave con la que se cifran las claves y los tokens guardados, para guardarla con sus copias de seguridad",
	},
	"it": {
		starting:          "Polyfin si sta avviando",
		running:           "Polyfin è in esecuzione",
		stopping:          "Polyfin si sta arrestando",
		stopped:           "Polyfin è arrestato",
		failed:            "Impossibile avviare Polyfin: vedi la cartella dei log",
		downloading:       "Download di FFmpeg: %d%%",
		updating:          "Download dell’aggiornamento: %d%%",
		rebuilding:        "Ricostruzione degli indici del database",
		keyStore:          "Impossibile leggere il portachiavi del sistema",
		tools:             "Impossibile scaricare FFmpeg",
		updateFailed:      "Impossibile installare l’aggiornamento",
		setupCode:         "Codice di configurazione: %s (fai clic per copiarlo)",
		openAdmin:         "Apri l’applicazione di amministrazione",
		openWeb:           "Apri il client web",
		openLogs:          "Apri la cartella dei log",
		start:             "Avvia Polyfin",
		stop:              "Arresta Polyfin",
		startWithComputer: "Avvia con il computer",
		install:           "Installa Polyfin %s",
		quit:              "Esci da Polyfin",
		copyKey:           "Copia la chiave segreta",
		keyCopied:         "Chiave segreta copiata: conservala con i tuoi backup",
		keyHint:           "Copia la chiave con cui sono cifrati le chiavi e i token salvati, da conservare con i tuoi backup",
	},
	"pt-BR": {
		starting:          "O Polyfin está iniciando",
		running:           "O Polyfin está em execução",
		stopping:          "O Polyfin está parando",
		stopped:           "O Polyfin está parado",
		failed:            "O Polyfin não pôde iniciar: veja a pasta de registros",
		downloading:       "Baixando o FFmpeg: %d%%",
		updating:          "Baixando a atualização: %d%%",
		rebuilding:        "Reconstruindo os índices do banco de dados",
		keyStore:          "Não foi possível ler o chaveiro do sistema",
		tools:             "Não foi possível baixar o FFmpeg",
		updateFailed:      "Não foi possível instalar a atualização",
		setupCode:         "Código de configuração: %s (clique para copiar)",
		openAdmin:         "Abrir o aplicativo de administração",
		openWeb:           "Abrir o cliente web",
		openLogs:          "Abrir a pasta de registros",
		start:             "Iniciar o Polyfin",
		stop:              "Parar o Polyfin",
		startWithComputer: "Iniciar com o computador",
		install:           "Instalar o Polyfin %s",
		quit:              "Sair do Polyfin",
		copyKey:           "Copiar a chave secreta",
		keyCopied:         "Chave secreta copiada: guarde-a com seus backups",
		keyHint:           "Copia a chave que criptografa as chaves e os tokens armazenados, para guardar com seus backups",
	},
}

// formatted fills a text's %s.
func formatted(text, value string) string {
	return fmt.Sprintf(text, value)
}

// textsFor returns the tray's texts in language, one of i18n.Languages,
// in English for any other.
func textsFor(language string) texts {
	if text, ok := trayTexts[language]; ok {
		return text
	}
	return trayTexts["en"]
}

// phase is the line that tells what Polyfin is doing.
func (c texts) phase(s app.Status) string {
	switch s.Phase {
	case app.Downloading:
		return fmt.Sprintf(c.downloading, s.Percent)
	case app.Updating:
		return fmt.Sprintf(c.updating, s.Percent)
	case app.Rebuilding:
		return c.rebuilding
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

package jellyfin

import (
	"fmt"

	"github.com/moodiness/polyfin/internal/library"
)

// shareText is what the page of a share link says, in one language.
type shareText struct {
	// PageTitle names the browser's tab; SharedFrom says where the title
	// comes from, before the server's name.
	PageTitle  string
	SharedFrom string
	Play       string
	Expires    string
	Minutes    string
	NoTitle    string
	// episodeFormat names an episode by its season, number and name.
	episodeFormat     string
	episodeFormatBare string
	// refusals say why a link opens nothing, or why a playback does not
	// start, by the codes of openShare and startGuest.
	refusals map[string]string
}

// refusal says why a link opens nothing, by its code.
func (t shareText) refusal(code string) string {
	if text, ok := t.refusals[code]; ok {
		return text
	}
	return t.refusals["error"]
}

// messages are what the page's script may say.
func (t shareText) messages() map[string]string {
	return t.refusals
}

// episode names an episode below its series.
func (t shareText) episode(item library.Item) string {
	if item.Name == "" {
		return fmt.Sprintf(t.episodeFormatBare, item.ParentIndexNumber, item.IndexNumber)
	}
	return fmt.Sprintf(t.episodeFormat, item.ParentIndexNumber, item.IndexNumber, item.Name)
}

var shareTexts = map[string]shareText{
	"en": {
		PageTitle:         "Shared with you",
		SharedFrom:        "Shared from",
		Play:              "Play",
		Expires:           "This link expires on",
		Minutes:           "min",
		NoTitle:           "Nothing to watch here",
		episodeFormat:     "Season %d, episode %d · %s",
		episodeFormatBare: "Season %d, episode %d",
		refusals: map[string]string{
			"unknown":           "This link does not exist. Check that it was copied whole.",
			"expired":           "This link has expired. Ask the person who shared it for a new one.",
			"revoked":           "This link was revoked.",
			"off":               "This link no longer works: sharing is turned off for it.",
			"unavailable":       "This title cannot be played right now.",
			"too_many_attempts": "Too many attempts from this address. Try again in a few minutes.",
			"limit":             "The owner of this link is already watching on as many devices as allowed. Try again later.",
			"viewers":           "As many people as this link allows are already watching. Try again later.",
			"hours":             "This link cannot be watched at this hour.",
			"busy":              "The server is converting as many videos as it can. Try again in a moment.",
			"ended":             "This playback ended. Press Play to watch again.",
			"error":             "Something went wrong. Try again.",
		},
	},
	"fr": {
		PageTitle:         "Partagé avec vous",
		SharedFrom:        "Partagé depuis",
		Play:              "Lecture",
		Expires:           "Ce lien expire le",
		Minutes:           "min",
		NoTitle:           "Rien à regarder ici",
		episodeFormat:     "Saison %d, épisode %d · %s",
		episodeFormatBare: "Saison %d, épisode %d",
		refusals: map[string]string{
			"unknown":           "Ce lien n’existe pas. Vérifiez qu’il a été copié en entier.",
			"expired":           "Ce lien a expiré. Demandez-en un nouveau à la personne qui l’a partagé.",
			"revoked":           "Ce lien a été révoqué.",
			"off":               "Ce lien ne fonctionne plus : le partage est désactivé.",
			"unavailable":       "Ce titre ne peut pas être lu pour le moment.",
			"too_many_attempts": "Trop de tentatives depuis cette adresse. Réessayez dans quelques minutes.",
			"limit":             "Le propriétaire de ce lien regarde déjà sur autant d’appareils que permis. Réessayez plus tard.",
			"viewers":           "Autant de personnes que ce lien le permet regardent déjà. Réessayez plus tard.",
			"hours":             "Ce lien ne peut pas être regardé à cette heure-ci.",
			"busy":              "Le serveur convertit déjà autant de vidéos qu’il le peut. Réessayez dans un instant.",
			"ended":             "Cette lecture est terminée. Appuyez sur Lecture pour regarder à nouveau.",
			"error":             "Une erreur est survenue. Réessayez.",
		},
	},
	"de": {
		PageTitle:         "Mit dir geteilt",
		SharedFrom:        "Geteilt von",
		Play:              "Abspielen",
		Expires:           "Dieser Link ist gültig bis",
		Minutes:           "Min.",
		NoTitle:           "Hier gibt es nichts zu sehen",
		episodeFormat:     "Staffel %d, Folge %d · %s",
		episodeFormatBare: "Staffel %d, Folge %d",
		refusals: map[string]string{
			"unknown":           "Diesen Link gibt es nicht. Prüfe, ob er vollständig kopiert wurde.",
			"expired":           "Dieser Link ist abgelaufen. Bitte die Person, die ihn geteilt hat, um einen neuen.",
			"revoked":           "Dieser Link wurde widerrufen.",
			"off":               "Dieser Link funktioniert nicht mehr: Das Teilen ist dafür deaktiviert.",
			"unavailable":       "Dieser Titel kann gerade nicht abgespielt werden.",
			"too_many_attempts": "Zu viele Versuche von dieser Adresse. Versuche es in ein paar Minuten erneut.",
			"limit":             "Der Besitzer dieses Links schaut bereits auf so vielen Geräten wie erlaubt. Versuche es später erneut.",
			"viewers":           "Es schauen bereits so viele Personen, wie dieser Link erlaubt. Versuche es später erneut.",
			"hours":             "Dieser Link kann zu dieser Uhrzeit nicht angesehen werden.",
			"busy":              "Der Server konvertiert bereits so viele Videos, wie er kann. Versuche es gleich noch einmal.",
			"ended":             "Diese Wiedergabe ist beendet. Drücke auf „Abspielen“, um erneut zu schauen.",
			"error":             "Etwas ist schiefgelaufen. Versuche es erneut.",
		},
	},
	"es": {
		PageTitle:         "Compartido con usted",
		SharedFrom:        "Compartido desde",
		Play:              "Reproducir",
		Expires:           "Este enlace caduca el",
		Minutes:           "min",
		NoTitle:           "No hay nada que ver aquí",
		episodeFormat:     "Temporada %d, episodio %d · %s",
		episodeFormatBare: "Temporada %d, episodio %d",
		refusals: map[string]string{
			"unknown":           "Este enlace no existe. Compruebe que se ha copiado entero.",
			"expired":           "Este enlace ha caducado. Pida uno nuevo a la persona que lo compartió.",
			"revoked":           "Este enlace se ha revocado.",
			"off":               "Este enlace ya no funciona: se ha desactivado el uso compartido.",
			"unavailable":       "Este título no se puede reproducir en este momento.",
			"too_many_attempts": "Demasiados intentos desde esta dirección. Vuelva a intentarlo dentro de unos minutos.",
			"limit":             "El propietario de este enlace ya está viendo contenido en tantos dispositivos como se permite. Vuelva a intentarlo más tarde.",
			"viewers":           "Ya hay tantas personas viendo como permite este enlace. Vuelva a intentarlo más tarde.",
			"hours":             "Este enlace no se puede ver a esta hora.",
			"busy":              "El servidor ya está convirtiendo tantos vídeos como puede. Vuelva a intentarlo en un momento.",
			"ended":             "Esta reproducción ha terminado. Pulse «Reproducir» para volver a verla.",
			"error":             "Algo ha fallado. Vuelva a intentarlo.",
		},
	},
	"it": {
		PageTitle:         "Condiviso con te",
		SharedFrom:        "Condiviso da",
		Play:              "Riproduci",
		Expires:           "Questo link scade il",
		Minutes:           "min",
		NoTitle:           "Niente da guardare qui",
		episodeFormat:     "Stagione %d, episodio %d · %s",
		episodeFormatBare: "Stagione %d, episodio %d",
		refusals: map[string]string{
			"unknown":           "Questo link non esiste. Verifica che sia stato copiato per intero.",
			"expired":           "Questo link è scaduto. Chiedine uno nuovo alla persona che l’ha condiviso.",
			"revoked":           "Questo link è stato revocato.",
			"off":               "Questo link non funziona più: la condivisione è disattivata.",
			"unavailable":       "Questo titolo non può essere riprodotto al momento.",
			"too_many_attempts": "Troppi tentativi da questo indirizzo. Riprova tra qualche minuto.",
			"limit":             "Il proprietario di questo link sta già guardando su tutti i dispositivi consentiti. Riprova più tardi.",
			"viewers":           "Stanno già guardando tutte le persone consentite da questo link. Riprova più tardi.",
			"hours":             "Questo link non può essere guardato a quest’ora.",
			"busy":              "Il server sta già convertendo tutti i video che può. Riprova tra un momento.",
			"ended":             "Questa riproduzione è terminata. Premi «Riproduci» per guardare di nuovo.",
			"error":             "Si è verificato un errore. Riprova.",
		},
	},
	"pt-BR": {
		PageTitle:         "Compartilhado com você",
		SharedFrom:        "Compartilhado de",
		Play:              "Reproduzir",
		Expires:           "Este link expira em",
		Minutes:           "min",
		NoTitle:           "Nada para assistir aqui",
		episodeFormat:     "Temporada %d, episódio %d · %s",
		episodeFormatBare: "Temporada %d, episódio %d",
		refusals: map[string]string{
			"unknown":           "Este link não existe. Verifique se ele foi copiado inteiro.",
			"expired":           "Este link expirou. Peça um novo à pessoa que o compartilhou.",
			"revoked":           "Este link foi revogado.",
			"off":               "Este link não funciona mais: o compartilhamento está desativado.",
			"unavailable":       "Este título não pode ser reproduzido agora.",
			"too_many_attempts": "Tentativas demais deste endereço. Tente novamente em alguns minutos.",
			"limit":             "O dono deste link já está assistindo em tantos dispositivos quanto permitido. Tente novamente mais tarde.",
			"viewers":           "Já há tantas pessoas assistindo quanto este link permite. Tente novamente mais tarde.",
			"hours":             "Este link não pode ser assistido neste horário.",
			"busy":              "O servidor já está convertendo tantos vídeos quanto pode. Tente novamente em instantes.",
			"ended":             "Esta reprodução terminou. Clique em “Reproduzir” para assistir de novo.",
			"error":             "Algo deu errado. Tente novamente.",
		},
	},
}

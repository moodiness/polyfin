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
			"revoked":           "This link was revoked by the person who shared it.",
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
			"revoked":           "Ce lien a été révoqué par la personne qui l’a partagé.",
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
}

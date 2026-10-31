package notifications

import (
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/plays"
)

// FailureJSON is a play that failed: what was played, as an item Jellyfin
// apps know, of a Kind as PlaybackJSON's, with its series, season and
// number for an episode, null when not known; the app and device it was
// played on, as they signed in, empty when not known; and why, as Reason
// tells: "no_compatible_stream" when none of the title's versions plays
// on the app now, "limit" when the user's playbacks at once or a
// provider's connections refused it, "stream_error" when its stream broke
// or could not be served.
type FailureJSON struct {
	ItemID     *string `json:"itemId"`
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	SeriesID   *string `json:"seriesId"`
	SeriesName *string `json:"seriesName"`
	Season     *int    `json:"season"`
	Number     *int    `json:"number"`
	App        string  `json:"app"`
	Device     string  `json:"device"`
	Reason     string  `json:"reason"`
}

// failureKey names a user's failed plays of a title.
type failureKey struct {
	user, item accounts.ID
}

// PlaybackFailed tells the server's targets and administrators' own that a
// play failed, once an hour per user and title: an app that tries again,
// or asks for each segment of a broken stream, is told of once. It
// returns at once.
func (s *Service) PlaybackFailed(f plays.Failure) {
	ev := s.failureEvent(f)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, at := range s.failures {
		if now.Sub(at) >= s.timing.failureQuiet {
			delete(s.failures, key)
		}
	}
	key := failureKey{user: f.User, item: f.Title.Item}
	if _, told := s.failures[key]; told {
		return
	}
	s.failures[key] = now
	for id, t := range s.targets {
		if t.wants(ev.Type) && (t.owner == nil || s.admins[*t.owner]) {
			s.push(id, ev)
		}
	}
}

// failureEvent tells of a failed play, in the server language.
func (s *Service) failureEvent(f plays.Failure) Event {
	ev := s.newEvent(PlaybackFailed)
	if !f.At.IsZero() {
		ev.At = f.At.UTC()
	}
	t := f.Title
	ev.User = &UserJSON{ID: f.User.String(), Name: f.UserName}
	j := &FailureJSON{Kind: t.Kind, Name: t.Name, App: f.App, Device: f.DeviceName, Reason: f.Reason}
	if t.Item != (accounts.ID{}) {
		j.ItemID = new(t.Item.String())
		ev.URL = s.webLink(t.Item)
	}
	if t.Kind == plays.Episode {
		j.SeriesName, j.Season, j.Number = &t.SeriesName, &t.Season, &t.Episode
		if t.SeriesID != (accounts.ID{}) {
			j.SeriesID = new(t.SeriesID.String())
		}
	}
	ev.Failure = j

	if name := playedName(t); name != "" {
		ev.Title = s.phrase(i18n.Text{"%s could not play %s", "%s n’a pas pu lire %s", "%s konnte %s nicht abspielen", "%s no pudo reproducir %s", "%s non è riuscito a riprodurre %s", "%s não conseguiu reproduzir %s"}, f.UserName, name)
	} else {
		ev.Title = s.phrase(i18n.Text{"%s could not play a title", "%s n’a pas pu lire un titre", "%s konnte einen Titel nicht abspielen", "%s no pudo reproducir un título", "%s non è riuscito a riprodurre un titolo", "%s não conseguiu reproduzir um título"}, f.UserName)
	}
	var parts []string
	switch f.Reason {
	case plays.FailedLimit:
		parts = append(parts, s.phrase(i18n.Text{"A limit refused it: the user's playbacks at once, or the provider's connections.", "Une limite l’a refusée : les lectures simultanées de l’utilisateur, ou les connexions du fournisseur.", "Ein Limit hat sie abgelehnt: die gleichzeitigen Wiedergaben des Benutzers oder die Verbindungen des Anbieters.", "Un límite la rechazó: las reproducciones simultáneas del usuario o las conexiones del proveedor.", "Un limite l’ha rifiutata: le riproduzioni simultanee dell’utente o le connessioni del fornitore.", "Um limite a recusou: as reproduções simultâneas do usuário ou as conexões do provedor."}))
	case plays.FailedStream:
		parts = append(parts, s.phrase(i18n.Text{"Its stream broke, or could not be served.", "Son flux s’est interrompu, ou n’a pas pu être servi.", "Der Stream ist abgebrochen oder konnte nicht bereitgestellt werden.", "Su flujo se cortó o no se pudo servir.", "Il flusso si è interrotto o non è stato possibile fornirlo.", "O fluxo foi interrompido ou não pôde ser servido."}))
	default:
		parts = append(parts, s.phrase(i18n.Text{"None of its versions plays on this app now.", "Aucune de ses versions ne se lit sur cette appli pour l’instant.", "Keine seiner Versionen lässt sich derzeit in dieser App abspielen.", "Ninguna de sus versiones se reproduce ahora en esta aplicación.", "Nessuna delle sue versioni si riproduce ora su questa app.", "Nenhuma de suas versões é reproduzida neste app agora."}))
	}
	switch {
	case f.App != "" && f.DeviceName != "":
		parts = append(parts, s.phrase(i18n.Text{"%s on %s", "%s sur %s", "%s auf %s", "%s en %s", "%s su %s", "%s em %s"}, f.App, f.DeviceName))
	case f.App != "" || f.DeviceName != "":
		parts = append(parts, f.App+f.DeviceName)
	}
	ev.Message = strings.Join(parts, " · ")
	return ev
}

package notifications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/recordings"
)

// EventVersion is the version of Event's JSON. Fields may be added within a
// version; a field removed or changed in meaning makes a new version.
const EventVersion = 1

// Event is what a generic webhook receives, as JSON: one event, described
// for people by Title, Message and URL, in the server language, and for
// programs by Type and the object of its type. docs/notifications.md
// describes it for app developers.
type Event struct {
	Version int `json:"version"`
	// ID is unique to the event: a webhook told twice of an event, after
	// a retry, can tell.
	ID   string    `json:"id"`
	Type string    `json:"type"`
	At   time.Time `json:"at"`
	// Server is the server the event comes from.
	Server ServerJSON `json:"server"`
	Title  string     `json:"title"`
	// Message is the body of the message, Title's details.
	Message string `json:"message"`
	// URL opens what the event is about, when the server's public address
	// is set; null otherwise.
	URL *string `json:"url"`
	// User is the user the event is about, null for health events, new
	// versions, new episodes sent to the server's targets, which are told
	// once whoever follows the series, and the server's summaries.
	User *UserJSON `json:"user"`
	// One of these is set, by Type.
	Episode   *EpisodeJSON   `json:"episode,omitempty"`
	Recording *RecordingJSON `json:"recording,omitempty"`
	Problem   *ProblemJSON   `json:"problem,omitempty"`
	Invite    *InviteJSON    `json:"invite,omitempty"`
	Playback  *PlaybackJSON  `json:"playback,omitempty"`
	Release   *ReleaseJSON   `json:"release,omitempty"`
	Summary   *SummaryJSON   `json:"summary,omitempty"`
	// Connection is the connection a connection_stopped event tells of.
	Connection *ConnectionJSON `json:"connection,omitempty"`
	SignIn     *SignInJSON     `json:"signIn,omitempty"`
	Block      *BlockJSON      `json:"block,omitempty"`
	// Share is the share link a share_played event tells of.
	Share *ShareJSON `json:"share,omitempty"`
	// Programme is the programme a programme_reminder event tells of.
	Programme *ProgrammeJSON `json:"programme,omitempty"`
	// WatchlistTitle is the title a title_available event tells of.
	WatchlistTitle *TitleJSON `json:"watchlistTitle,omitempty"`
}

// ServerJSON identifies a server: its ID as Jellyfin apps know it, its
// name, and its public address, null when unset.
type ServerJSON struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	URL  *string `json:"url"`
}

// UserJSON is a user, by their ID as Jellyfin apps know it and their name.
type UserJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EpisodeJSON is a new episode: its item and its series' as Jellyfin apps
// know them, its season and number, and when it aired.
type EpisodeJSON struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	SeriesID     string            `json:"seriesId"`
	SeriesName   string            `json:"seriesName"`
	Season       int               `json:"season"`
	Number       int               `json:"number"`
	PremiereDate *time.Time        `json:"premiereDate"`
	ProviderIDs  map[string]string `json:"providerIds"`
}

// RecordingJSON is a recording that ended: its item, as Jellyfin apps know
// it (gone once it failed), its programme's name, its channel, when its
// programme was planned, and whether part of it is missing.
type RecordingJSON struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ChannelID   string    `json:"channelId"`
	ChannelName string    `json:"channelName"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Partial     bool      `json:"partial"`
}

// ProblemJSON is a problem System › Health found or no longer finds: Key
// names it the same way while it lasts.
type ProblemJSON struct {
	Key      string    `json:"key"`
	Severity string    `json:"severity"`
	Text     string    `json:"text"`
	Since    time.Time `json:"since"`
}

// InviteJSON is the invite a user joined through: its ID, and the
// administrator who created it, null once deleted.
type InviteJSON struct {
	ID        string    `json:"id"`
	CreatedBy *UserJSON `json:"createdBy"`
}

// ReleaseJSON is a new version of Polyfin: its version, the address of its
// release notes, and the version the server runs.
type ReleaseJSON struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Current string `json:"current"`
}

// ConnectionJSON is a user's connection that stopped: Kind is "app" for a
// connection to an app built on addons, "tracking" for a tracking service;
// Service names the app or service as the API does ("stremio", "trakt"),
// Name as people do.
type ConnectionJSON struct {
	Kind    string `json:"kind"`
	Service string `json:"service"`
	Name    string `json:"name"`
}

// The kinds of connections that stop (see ConnectionJSON).
const (
	ConnectionApp      = "app"
	ConnectionTracking = "tracking"
)

// connectionNames name the apps and tracking services as people do, by
// the names the API gives them.
var connectionNames = map[string]string{
	"nuvio": "Nuvio", "stremio": "Stremio",
	"trakt": "Trakt", "simkl": "Simkl", "mdblist": "MDBList", "publicmetadb": "PublicMetaDB", "lastfm": "Last.fm", "listenbrainz": "ListenBrainz",
	"anilist": "AniList", "myanimelist": "MyAnimeList",
}

// PlaybackJSON is a playback that started, paused, resumed or stopped.
// What plays is an item as Jellyfin apps know it, of a Kind among
// "movie", "episode", "channel", "recording", "replay" (a Replay
// programme), "song" and "audiobook", with its series, season and number
// for an episode, its channel for a channel, a Replay programme or a
// recording, and its artist for a song or an audiobook; null otherwise.
// App and Device name what it plays on, as they signed in. Position is
// where it is, in seconds; Played how long it played since StartedAt,
// pauses left out. Method tells how it reaches the app: "direct_play",
// "direct_stream" (remuxed, nothing converted) or "conversion", null when
// not known yet; Converted is true for "conversion". Shared is true for a
// guest playing through a share link of the user's, App then "Shared
// link".
type PlaybackJSON struct {
	ItemID      string    `json:"itemId"`
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	SeriesID    *string   `json:"seriesId"`
	SeriesName  *string   `json:"seriesName"`
	Season      *int      `json:"season"`
	Number      *int      `json:"number"`
	ChannelID   *string   `json:"channelId"`
	ChannelName *string   `json:"channelName"`
	Artist      *string   `json:"artist"`
	App         string    `json:"app"`
	Device      string    `json:"device"`
	Position    int64     `json:"position"`
	Paused      bool      `json:"paused"`
	Method      *string   `json:"method"`
	Converted   bool      `json:"converted"`
	StartedAt   time.Time `json:"startedAt"`
	Played      int64     `json:"played"`
	Shared      bool      `json:"shared"`
}

// newEvent starts an event of type kind, now.
func (s *Service) newEvent(kind string) Event {
	var id [16]byte
	_, _ = rand.Read(id[:])
	settings := s.accounts.Settings()
	ev := Event{Version: EventVersion, ID: hex.EncodeToString(id[:]), Type: kind, At: s.now().UTC(),
		Server: ServerJSON{ID: s.serverID, Name: settings.ServerName}}
	if settings.PublicAddress != "" {
		ev.Server.URL = &settings.PublicAddress
	}
	return ev
}

// phrase says something in the server language, from its text in each
// language, with %s for args.
func (s *Service) phrase(text i18n.Text, args ...any) string {
	return fmt.Sprintf(text.In(s.accounts.Settings().Language), args...)
}

// webLink is the address of item's page in the web client, nil without a
// public address or a web client.
func (s *Service) webLink(item accounts.ID) *string {
	base := s.accounts.Settings().PublicAddress
	if base == "" || !s.webClient {
		return nil
	}
	link := base + "/web/#/details?" + url.Values{"id": {item.String()}, "serverId": {s.serverID}}.Encode()
	return &link
}

// healthLink is the address of the admin app's page that shows a problem,
// System › Health when it names none; nil without a public address.
func (s *Service) healthLink(page string) *string {
	if page == "" {
		page = "/system/health"
	}
	return s.adminLink(page)
}

// adminLink is the address of a page of the admin app, nil without a
// public address.
func (s *Service) adminLink(page string) *string {
	base := s.accounts.Settings().PublicAddress
	if base == "" {
		return nil
	}
	link := base + "/admin" + page
	return &link
}

// testEvent is the message "Send a test" sends.
func (s *Service) testEvent() Event {
	ev := s.newEvent(Test)
	ev.Title = s.phrase(i18n.Text{"Test message", "Message de test", "Testnachricht", "Mensaje de prueba", "Messaggio di prova", "Mensagem de teste"})
	ev.Message = s.phrase(i18n.Text{"Notifications from %s reach this target.", "Les notifications de %s arrivent bien ici.", "Benachrichtigungen von %s erreichen dieses Ziel.", "Las notificaciones de %s llegan a este destino.", "Le notifiche di %s raggiungono questa destinazione.", "As notificações de %s chegam a este destino."}, ev.Server.Name)
	return ev
}

// episodeEvent tells of episode, new for user, nil for the server's
// targets.
func (s *Service) episodeEvent(user *accounts.User, episode library.Item) Event {
	ev := s.newEvent(NewEpisode)
	if user != nil {
		ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
	}
	ev.Episode = &EpisodeJSON{ID: episode.ID.String(), Name: episode.Name, SeriesID: episode.SeriesID.String(), SeriesName: episode.SeriesName,
		Season: episode.ParentIndexNumber, Number: episode.IndexNumber, PremiereDate: episode.PremiereDate, ProviderIDs: episode.ProviderIDs}
	if ev.Episode.ProviderIDs == nil {
		ev.Episode.ProviderIDs = map[string]string{}
	}
	ev.Title = s.phrase(i18n.Text{"New episode of %s", "Nouvel épisode de %s", "Neue Folge von %s", "Nuevo episodio de %s", "Nuovo episodio di %s", "Novo episódio de %s"}, episode.SeriesName)
	ev.Message = fmt.Sprintf("S%02dE%02d", episode.ParentIndexNumber, episode.IndexNumber)
	if episode.Name != "" {
		ev.Message += " · " + episode.Name
	}
	ev.URL = s.webLink(episode.ID)
	return ev
}

// recordingEvent tells that a recording of user, nil once deleted, on
// channel ended.
func (s *Service) recordingEvent(ended recordings.Ended, user *accounts.User, channel string) Event {
	kind := RecordingFinished
	if ended.Failed {
		kind = RecordingFailed
	}
	ev := s.newEvent(kind)
	who := s.phrase(i18n.Text{"a deleted user", "un utilisateur supprimé", "einen gelöschten Benutzer", "un usuario eliminado", "un utente eliminato", "um usuário excluído"})
	if user != nil {
		ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
		who = user.Name
	}
	ev.Recording = &RecordingJSON{ID: ended.Recording.String(), Name: ended.Name, ChannelID: ended.Channel.String(), ChannelName: channel,
		Start: ended.Start.UTC(), End: ended.End.UTC(), Partial: ended.Partial}
	if channel == "" {
		channel = s.phrase(i18n.Text{"its channel", "sa chaîne", "seinem Kanal", "su canal", "un canale", "seu canal"})
	}
	switch {
	case ended.Failed:
		ev.Title = s.phrase(i18n.Text{"Recording failed: %s", "Échec de l’enregistrement : %s", "Aufnahme fehlgeschlagen: %s", "Grabación fallida: %s", "Registrazione non riuscita: %s", "Falha na gravação: %s"}, ended.Name)
		ev.Message = s.phrase(i18n.Text{"Nothing could be recorded on %s, for %s.", "Rien n’a pu être enregistré sur %s, pour %s.", "Auf %s konnte für %s nichts aufgenommen werden.", "No se pudo grabar nada en %s, para %s.", "Non è stato possibile registrare nulla su %s, per %s.", "Nada pôde ser gravado em %s, para %s."}, channel, who)
		if ended.NoTuner {
			ev.Message += " " + s.phrase(i18n.Text{"Every tuner was recording another channel.", "Tous les tuners enregistraient une autre chaîne.", "Alle Tuner nahmen einen anderen Kanal auf.", "Todos los sintonizadores estaban grabando otro canal.", "Tutti i sintonizzatori stavano registrando un altro canale.", "Todos os sintonizadores estavam gravando outro canal."})
		}
	default:
		ev.Title = s.phrase(i18n.Text{"Recording finished: %s", "Enregistrement terminé : %s", "Aufnahme beendet: %s", "Grabación terminada: %s", "Registrazione terminata: %s", "Gravação concluída: %s"}, ended.Name)
		ev.Message = s.phrase(i18n.Text{"Recorded on %s, for %s.", "Enregistré sur %s, pour %s.", "Aufgenommen auf %s, für %s.", "Grabado en %s, para %s.", "Registrato su %s, per %s.", "Gravado em %s, para %s."}, channel, who)
		if ended.Partial {
			ev.Message += " " + s.phrase(i18n.Text{"Part of the programme is missing.", "Une partie de l’émission manque.", "Ein Teil der Sendung fehlt.", "Falta parte del programa.", "Manca una parte del programma.", "Falta parte do programa."})
		}
		ev.URL = s.webLink(ended.Recording)
	}
	return ev
}

// healthEventOf tells that problem was found, or solved.
func (s *Service) healthEventOf(problem Problem, since time.Time, solved bool) Event {
	kind, title := HealthProblem, s.phrase(i18n.Text{"Problem found", "Problème détecté", "Problem erkannt", "Problema detectado", "Problema rilevato", "Problema detectado"})
	if solved {
		kind, title = HealthSolved, s.phrase(i18n.Text{"Problem solved", "Problème résolu", "Problem behoben", "Problema resuelto", "Problema risolto", "Problema resolvido"})
	}
	ev := s.newEvent(kind)
	ev.Problem = &ProblemJSON{Key: problem.Key, Severity: problem.Severity, Text: problem.Text, Since: since.UTC()}
	ev.Title, ev.Message, ev.URL = title, problem.Text, s.healthLink(problem.Page)
	return ev
}

// userJoinedEvent tells that user created their account through invite.
func (s *Service) userJoinedEvent(user accounts.User, invite accounts.Invite) Event {
	ev := s.newEvent(UserJoined)
	ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
	ev.Invite = &InviteJSON{ID: invite.ID.String()}
	ev.Title = s.phrase(i18n.Text{"New user: %s", "Nouvel utilisateur : %s", "Neuer Benutzer: %s", "Nuevo usuario: %s", "Nuovo utente: %s", "Novo usuário: %s"}, user.Name)
	if creator := invite.CreatedBy; creator != nil {
		ev.Invite.CreatedBy = &UserJSON{ID: creator.ID.String(), Name: creator.Name}
		ev.Message = s.phrase(i18n.Text{"%s joined through %s's invite.", "%s a rejoint le serveur grâce à l’invitation de %s.", "%s ist über die Einladung von %s beigetreten.", "%s se ha unido mediante la invitación de %s.", "%s ha aderito tramite l’invito di %s.", "%s entrou pelo convite de %s."}, user.Name, creator.Name)
	} else {
		ev.Message = s.phrase(i18n.Text{"%s joined through an invite.", "%s a rejoint le serveur grâce à une invitation.", "%s ist über eine Einladung beigetreten.", "%s se ha unido mediante una invitación.", "%s ha aderito tramite un invito.", "%s entrou por um convite."}, user.Name)
	}
	ev.URL = s.adminLink("/users/" + user.ID.String())
	return ev
}

// newVersionEvent tells that Polyfin version is out, with its release
// notes at link.
func (s *Service) newVersionEvent(version, link string) Event {
	ev := s.newEvent(NewVersion)
	ev.Release = &ReleaseJSON{Version: version, URL: link, Current: s.version}
	ev.Title = s.phrase(i18n.Text{"New version: Polyfin %s", "Nouvelle version : Polyfin %s", "Neue Version: Polyfin %s", "Nueva versión: Polyfin %s", "Nuova versione: Polyfin %s", "Nova versão: Polyfin %s"}, version)
	ev.Message = s.phrase(i18n.Text{"Polyfin %s is available. This server runs version %s.", "Polyfin %s est disponible. Ce serveur est en version %s.", "Polyfin %s ist verfügbar. Dieser Server läuft mit Version %s.", "Polyfin %s está disponible. Este servidor usa la versión %s.", "Polyfin %s è disponibile. Questo server usa la versione %s.", "O Polyfin %s está disponível. Este servidor está na versão %s."},
		version, s.version)
	ev.URL = &link
	return ev
}

// connectionStoppedEvent tells that the connection of user to service, of
// kind ConnectionApp or ConnectionTracking, stopped: what to do, and where.
func (s *Service) connectionStoppedEvent(user accounts.User, kind, service string) Event {
	ev := s.newEvent(ConnectionStopped)
	name := connectionNames[service]
	if name == "" {
		name = service
	}
	ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
	ev.Connection = &ConnectionJSON{Kind: kind, Service: service, Name: name}
	ev.Title = s.phrase(i18n.Text{"Connection stopped: %s", "Connexion arrêtée : %s", "Verbindung beendet: %s", "Conexión detenida: %s", "Connessione interrotta: %s", "Conexão interrompida: %s"}, name)
	if kind == ConnectionApp {
		ev.Message = s.phrase(i18n.Text{"%s refused the sign-in Polyfin kept for %s: nothing is imported from it until %s signs in again " +
			"under My account › Import from an app, with Stay connected ticked.", "%s a refusé la connexion que Polyfin gardait pour %s : plus rien n’en est importé tant que %s ne s’y reconnecte pas " +
			"sous Mon compte › Importer depuis une appli, avec « Rester connecté » coché.", "%s hat die Anmeldung abgelehnt, die Polyfin für %s gespeichert hat: Von dort wird nichts importiert, bis sich %s unter Mein Konto › Aus einer App importieren erneut anmeldet, mit aktiviertem „Verbunden bleiben“.", "%s ha rechazado el inicio de sesión que Polyfin guardaba para %s: no se importa nada de ahí hasta que %s vuelva a iniciar sesión en Mi cuenta › Importar desde una aplicación, con «Permanecer conectado» marcado.", "%s ha rifiutato l’accesso che Polyfin conservava per %s: da lì non viene importato nulla finché %s non accede di nuovo in Il mio account › Importa da un’applicazione, con «Resta connesso» selezionato.", "%s recusou o login que o Polyfin mantinha para %s: nada é importado de lá até que %s entre novamente em Minha conta › Importar de um aplicativo, com “Permanecer conectado” marcado."}, name, user.Name, user.Name)
		ev.URL = s.adminLink("/me/account#app-import")
		return ev
	}
	ev.Message = s.phrase(i18n.Text{"%s refused the token or key Polyfin kept for %s: nothing is sent to it or imported from it until %s " +
		"presses Connect again under My account › Tracking.", "%s a refusé le jeton ou la clé que Polyfin gardait pour %s : plus rien ne lui est envoyé ni n’en est importé tant que %s " +
		"n’appuie pas sur Reconnecter sous Mon compte › Suivi.", "%s hat das Token oder den Schlüssel abgelehnt, den Polyfin für %s gespeichert hat: Es wird nichts dorthin gesendet oder von dort importiert, bis %s unter Mein Konto › Tracking auf „Erneut verbinden“ klickt.", "%s ha rechazado el token o la clave que Polyfin guardaba para %s: no se le envía ni se importa nada hasta que %s pulse «Volver a conectar» en Mi cuenta › Seguimiento.", "%s ha rifiutato il token o la chiave che Polyfin conservava per %s: non viene inviato né importato nulla finché %s non preme «Collega di nuovo» in Il mio account › Monitoraggio.", "%s recusou o token ou a chave que o Polyfin mantinha para %s: nada é enviado a ele nem importado dele até que %s clique em “Conectar novamente” em Minha conta › Acompanhamento."}, name, user.Name, user.Name)
	ev.URL = s.adminLink("/me/account#tracking")
	return ev
}

// NewVersion tells the server's targets and administrators' own that
// Polyfin version is out, with its release notes at link. It returns at
// once, as UserJoined does.
func (s *Service) NewVersion(version, link string) {
	ev := s.newVersionEvent(version, link)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawn(func() {
		// The targets read last stay in use when the database does not answer.
		if err := s.reload(s.ctx); err != nil && s.ctx.Err() == nil {
			s.logger.Debug("The notification targets could not be read again", "error", err)
		}
		s.dispatchAdministrators(ev)
	})
}

// UserJoined tells the server's targets and administrators' own that user
// created their account through invite, and keeps it for the weekly
// summary. It returns once it is kept: who the administrators are is read
// again, and the message sent, in the background.
func (s *Service) UserJoined(ctx context.Context, user accounts.User, invite accounts.Invite) {
	if _, err := s.db.Exec(context.WithoutCancel(ctx), `INSERT INTO notification_joins (user_id, invite_id, joined_at) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, user.ID, invite.ID, s.now()); err != nil {
		s.logger.Warn("A user who joined could not be kept for the weekly summary", "user_id", user.ID.String(), "error", err)
	}
	ev := s.userJoinedEvent(user, invite)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawn(func() {
		// The targets read last stay in use when the database does not answer.
		if err := s.reload(s.ctx); err != nil && s.ctx.Err() == nil {
			s.logger.Debug("The notification targets could not be read again", "error", err)
		}
		s.dispatchAdministrators(ev)
	})
}

// RecordingEnded tells the targets of the user who made a recording, and
// the server's, that it finished or failed. It returns at once: the
// message is prepared and sent in the background.
func (s *Service) RecordingEnded(_ context.Context, ended recordings.Ended) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawn(func() {
		ctx := s.ctx
		var user *accounts.User
		channel := ""
		if ended.User != nil {
			if found, err := s.accounts.User(ctx, *ended.User); err == nil {
				user = &found
				if item, err := s.library.Item(ctx, found, ended.Channel); err == nil {
					channel = item.Name
				}
			}
		}
		ev := s.recordingEvent(ended, user, channel)
		s.dispatch(ev, func(t target) bool {
			return t.owner == nil || user != nil && *t.owner == user.ID
		})
	})
}

// ConnectionStopped tells the targets of user, and the server's, that the
// user's connection to service, of kind ConnectionApp or
// ConnectionTracking, stopped: the app or service refused what Polyfin
// kept. Callers tell it once, as the connection stops. It returns at once:
// the message is prepared and sent in the background.
func (s *Service) ConnectionStopped(user accounts.ID, kind, service string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawn(func() {
		found, err := s.accounts.User(s.ctx, user)
		if err != nil {
			if s.ctx.Err() == nil {
				s.logger.Debug("The user of a stopped connection could not be read", "user_id", user.String(), "error", err)
			}
			return
		}
		s.dispatch(s.connectionStoppedEvent(found, kind, service), func(t target) bool {
			return t.owner == nil || *t.owner == user
		})
	})
}

package notifications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
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
	// User is the user the event is about, null for health events and new
	// versions, and for new episodes sent to the server's targets, which
	// are told once whoever follows the series.
	User *UserJSON `json:"user"`
	// One of these is set, by Type.
	Episode   *EpisodeJSON   `json:"episode,omitempty"`
	Recording *RecordingJSON `json:"recording,omitempty"`
	Problem   *ProblemJSON   `json:"problem,omitempty"`
	Invite    *InviteJSON    `json:"invite,omitempty"`
	Playback  *PlaybackJSON  `json:"playback,omitempty"`
	Release   *ReleaseJSON   `json:"release,omitempty"`
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
// not known yet; Converted is true for "conversion".
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

// phrase says something in the server language, from the English and
// French patterns, each with %s for args.
func (s *Service) phrase(english, french string, args ...any) string {
	pattern := english
	if s.accounts.Settings().Language == "fr" {
		pattern = french
	}
	return fmt.Sprintf(pattern, args...)
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
	ev.Title = s.phrase("Test message", "Message de test")
	ev.Message = s.phrase("Notifications from %s reach this target.", "Les notifications de %s arrivent bien ici.", ev.Server.Name)
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
	ev.Title = s.phrase("New episode of %s", "Nouvel épisode de %s", episode.SeriesName)
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
	who := s.phrase("a deleted user", "un utilisateur supprimé")
	if user != nil {
		ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
		who = user.Name
	}
	ev.Recording = &RecordingJSON{ID: ended.Recording.String(), Name: ended.Name, ChannelID: ended.Channel.String(), ChannelName: channel,
		Start: ended.Start.UTC(), End: ended.End.UTC(), Partial: ended.Partial}
	if channel == "" {
		channel = s.phrase("its channel", "sa chaîne")
	}
	switch {
	case ended.Failed:
		ev.Title = s.phrase("Recording failed: %s", "Échec de l’enregistrement : %s", ended.Name)
		ev.Message = s.phrase("Nothing could be recorded on %s, for %s.", "Rien n’a pu être enregistré sur %s, pour %s.", channel, who)
	default:
		ev.Title = s.phrase("Recording finished: %s", "Enregistrement terminé : %s", ended.Name)
		ev.Message = s.phrase("Recorded on %s, for %s.", "Enregistré sur %s, pour %s.", channel, who)
		if ended.Partial {
			ev.Message += " " + s.phrase("Part of the programme is missing.", "Une partie de l’émission manque.")
		}
		ev.URL = s.webLink(ended.Recording)
	}
	return ev
}

// healthEventOf tells that problem was found, or solved.
func (s *Service) healthEventOf(problem Problem, since time.Time, solved bool) Event {
	kind, title := HealthProblem, s.phrase("Problem found", "Problème détecté")
	if solved {
		kind, title = HealthSolved, s.phrase("Problem solved", "Problème résolu")
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
	ev.Title = s.phrase("New user: %s", "Nouvel utilisateur : %s", user.Name)
	if creator := invite.CreatedBy; creator != nil {
		ev.Invite.CreatedBy = &UserJSON{ID: creator.ID.String(), Name: creator.Name}
		ev.Message = s.phrase("%s joined through %s's invite.", "%s a rejoint le serveur grâce à l’invitation de %s.", user.Name, creator.Name)
	} else {
		ev.Message = s.phrase("%s joined through an invite.", "%s a rejoint le serveur grâce à une invitation.", user.Name)
	}
	ev.URL = s.adminLink("/users/" + user.ID.String())
	return ev
}

// newVersionEvent tells that Polyfin version is out, with its release
// notes at link.
func (s *Service) newVersionEvent(version, link string) Event {
	ev := s.newEvent(NewVersion)
	ev.Release = &ReleaseJSON{Version: version, URL: link, Current: s.version}
	ev.Title = s.phrase("New version: Polyfin %s", "Nouvelle version : Polyfin %s", version)
	ev.Message = s.phrase("Polyfin %s is available. This server runs version %s.", "Polyfin %s est disponible. Ce serveur est en version %s.",
		version, s.version)
	ev.URL = &link
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
// created their account through invite. It returns at once: who the
// administrators are is read again, and the message sent, in the
// background.
func (s *Service) UserJoined(_ context.Context, user accounts.User, invite accounts.Invite) {
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

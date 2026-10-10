// Package notifications tells users and administrators when something they
// care about happens: a new episode of a series a user follows, a recording
// that finished or failed, a problem System › Health found or that was
// solved, a user who joined through an invite, a playback that started,
// paused, resumed or stopped. Messages go to targets: generic webhooks,
// which receive a versioned JSON event (see Event); Discord webhooks, ntfy
// topics, Telegram chats, Gotify servers and Pushover users, which receive
// messages formatted for them; and email addresses, through the SMTP server
// of the settings. Administrators add the server's targets, which receive
// the events of every user; each user adds their own, which receive their
// own events, and the health and joining events for administrators.
//
// Nothing is sent while a request is answered: each target has a queue,
// sent in order in the background, tried again after network errors and
// server errors, as long as a target asks to wait, and given up after a
// while. A target that refuses Polyfin is marked so for the admin app.
package notifications

import (
	"context"
	"crypto/x509"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// The kinds of targets.
const (
	// Webhook receives each event as JSON (see Event).
	Webhook = "webhook"
	// Discord is a Discord channel's webhook, which receives an embed.
	Discord = "discord"
	// Ntfy is a topic of an ntfy server, which receives a title, a message,
	// tags and a link.
	Ntfy = "ntfy"
	// Email is an email address, which receives a message in plain text
	// and HTML through the SMTP server of the settings.
	Email = "email"
	// Telegram is a chat a Telegram bot posts to, through the Bot API.
	Telegram = "telegram"
	// Gotify is an application of a Gotify server.
	Gotify = "gotify"
	// Pushover is a Pushover user, through an application of theirs.
	Pushover = "pushover"
)

// Kinds lists the kinds of targets in the order the admin app shows them.
var Kinds = []string{Webhook, Discord, Ntfy, Email, Telegram, Gotify, Pushover}

// The events, by the names targets choose them by and Event.Type gives.
const (
	NewEpisode        = "new_episode"
	RecordingFinished = "recording_finished"
	RecordingFailed   = "recording_failed"
	HealthProblem     = "health_problem"
	HealthSolved      = "health_solved"
	UserJoined        = "user_joined"
	// The playbacks of videos and audio: their start, pause, resumption
	// and stop, one event each.
	PlaybackStarted = "playback_started"
	PlaybackPaused  = "playback_paused"
	PlaybackResumed = "playback_resumed"
	PlaybackStopped = "playback_stopped"
	// Test is the message "Send a test" sends; targets do not choose it.
	Test = "test"
)

// Events lists the events targets choose from, in the order the admin app
// shows them.
var Events = []string{NewEpisode, RecordingFinished, RecordingFailed, HealthProblem, HealthSolved, UserJoined, PlaybackStarted,
	PlaybackPaused, PlaybackResumed, PlaybackStopped}

// administratorsEvent reports whether event is one only the server's
// targets and administrators' own receive: about System › Health, or a
// user who joined through an invite.
func administratorsEvent(event string) bool {
	return event == HealthProblem || event == HealthSolved || event == UserJoined
}

// EventsFor lists the events the targets of owner may receive: every event
// for the server's targets (owner nil) and administrators', all but the
// administrators' events for other users.
func EventsFor(owner *accounts.User) []string {
	var events []string
	for _, event := range Events {
		if !administratorsEvent(event) || owner == nil || owner.IsAdministrator {
			events = append(events, event)
		}
	}
	return events
}

// The problems a target can have.
const (
	// ProblemRefused: the target answered 401, 403, 404 or 410 to the
	// last message: its address or token is wrong, or it was deleted.
	ProblemRefused = "refused"
	// ProblemRejected: the target refused the last message with another
	// 4xx status.
	ProblemRejected = "rejected"
	// ProblemUnreachable: recent messages could not be delivered: the
	// target did not answer, or answered with server errors.
	ProblemUnreachable = "unreachable"
	// ProblemUnreadable: the target's address or token cannot be
	// decrypted with POLYFIN_SECRET_KEY; nothing is sent to it until it
	// is entered again.
	ProblemUnreadable = "unreadable"
)

var (
	// ErrNotFound reports a target the owner has none of.
	ErrNotFound = errors.New("notification target not found")
	// ErrInvalidKind reports a kind that is not one of Kinds.
	ErrInvalidKind = errors.New("invalid notification target kind")
	// ErrInvalidName reports an empty name, or one longer than MaxName.
	ErrInvalidName = errors.New("invalid notification target name")
	// ErrInvalidAddress reports an address that is not an http or https
	// URL with a host, or longer than maxAddress, or an address given to a
	// Telegram or Pushover target, which have none.
	ErrInvalidAddress = errors.New("invalid notification target address")
	// ErrPrivateAddress reports an address on a local network, for a
	// target of a user who is not an administrator.
	ErrPrivateAddress = errors.New("notification target on a local network")
	// ErrInvalidEmail reports an email target's address that is not an
	// email address (see accounts.ValidEmail).
	ErrInvalidEmail = errors.New("invalid email address")
	// ErrEmailUnavailable reports an email target added while no SMTP
	// server is set (see accounts.Settings.SMTPAvailable).
	ErrEmailUnavailable = errors.New("no SMTP server for email notifications")
	// ErrInvalidTopic reports an ntfy topic that is not 1 to 64 letters,
	// digits, dashes and underscores.
	ErrInvalidTopic = errors.New("invalid ntfy topic")
	// ErrInvalidChat reports a Telegram chat that is neither a chat's
	// number nor a public channel's @name.
	ErrInvalidChat = errors.New("invalid Telegram chat")
	// ErrInvalidToken reports an access token that is not printable ASCII
	// without spaces, of at most maxToken bytes, or empty for a kind that
	// needs one.
	ErrInvalidToken = errors.New("invalid access token")
	// ErrInvalidUserKey reports a Pushover user key that is empty or not
	// letters and digits, of at most maxToken bytes.
	ErrInvalidUserKey = errors.New("invalid Pushover user key")
	// ErrInvalidEvents reports an event that is not one of those the
	// owner's targets may receive (see EventsFor).
	ErrInvalidEvents = errors.New("invalid notification events")
	// ErrTooManyTargets reports an owner with MaxTargets targets already.
	ErrTooManyTargets = errors.New("too many notification targets")
	// ErrUnreadable reports a target whose secret the key cannot decrypt.
	ErrUnreadable = errors.New("notification target cannot be decrypted")
)

// Accounts are the users and the settings; accounts.Store is one.
type Accounts interface {
	Users(ctx context.Context) ([]accounts.User, error)
	User(ctx context.Context, id accounts.ID) (accounts.User, error)
	Settings() accounts.Settings
}

// Library lists the episodes of series as a user sees them, and names the
// channels of recordings; library.Service is one.
type Library interface {
	Episodes(ctx context.Context, user accounts.User, seriesID accounts.ID, seasonID *accounts.ID) ([]library.Item, error)
	Item(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error)
}

// UserData tells which series each user follows: those they played an
// episode of, and their favorites; userdata.Store is one.
type UserData interface {
	Episodes(ctx context.Context, user accounts.ID) ([]userdata.Entry, error)
	Favorites(ctx context.Context, user accounts.ID) ([]userdata.Entry, error)
}

// Problem is something System › Health shows as needing attention. Key
// names it the same way from one check to the next; Text describes it in
// the server language; Page is the page of the admin app that shows it,
// as System › Health links it (such as "/system/health#addons").
type Problem struct {
	Key      string
	Severity string
	Text     string
	Page     string
}

// The severities of problems.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Options are the dependencies of the service.
type Options struct {
	DB       *pgxpool.Pool
	Accounts Accounts
	Library  Library
	UserData UserData
	// Secrets seals the targets' addresses and tokens in the database, and
	// opens them; nil stores them as they are.
	Secrets *secrets.Box
	// Version is Polyfin's, which targets are told; ServerID identifies
	// the server in events and links.
	Version  string
	ServerID string
	// WebClient tells whether Polyfin serves jellyfin-web at /web/, which
	// links to episodes and recordings open.
	WebClient bool
	// Problems finds what System › Health shows as needing attention; nil
	// sends no health event.
	Problems func(context.Context) ([]Problem, error)
	Logger   *slog.Logger
}

// timing is how long the service waits for what. Tests shorten it.
type timing struct {
	// request bounds each request to a target.
	request time.Duration
	// retryFirst is the first wait after a failed delivery, doubling up
	// to retryMax; a message older than giveUp is dropped.
	retryFirst, retryMax, giveUp time.Duration
	// unreachableAfter is how many deliveries in a row fail before the
	// target shows as unreachable.
	unreachableAfter int
	// episodesFirst is how long after start the series are first looked
	// at, and episodesEvery how often after that.
	episodesFirst, episodesEvery time.Duration
	// healthEvery is how often the health problems are looked at; a
	// problem is told once healthConfirm checks in a row found it, and
	// solved once as many no longer did.
	healthEvery   time.Duration
	healthConfirm int
}

var defaultTiming = timing{
	request:          10 * time.Second,
	retryFirst:       30 * time.Second,
	retryMax:         10 * time.Minute,
	giveUp:           time.Hour,
	unreachableAfter: 3,
	episodesFirst:    5 * time.Minute,
	episodesEvery:    2 * time.Hour,
	healthEvery:      5 * time.Minute,
	healthConfirm:    2,
}

const (
	// maxQueue bounds the messages waiting for one target: past it, the
	// oldest is dropped.
	maxQueue = 100
	// maxSending bounds the requests to targets under way at once.
	maxSending = 4
	// maxReply bounds the bytes read from a target's answer.
	maxReply = 64 << 10
)

// The addresses of Telegram's Bot API and of Pushover's messages.
const (
	defaultTelegramAPI = "https://api.telegram.org"
	defaultPushoverAPI = "https://api.pushover.net/1/messages.json"
)

// Service keeps the targets, follows the events and delivers them.
type Service struct {
	db        *pgxpool.Pool
	accounts  Accounts
	library   Library
	userData  UserData
	box       *secrets.Box
	version   string
	serverID  string
	webClient bool
	problems  func(context.Context) ([]Problem, error)
	logger    *slog.Logger
	now       func() time.Time
	timing    timing
	// trusted sends the server's and administrators' messages; confined
	// those of other users, which may only reach public addresses, as
	// checkPublic checks their addresses when they are saved.
	trusted, confined *http.Client
	checkPublic       func(ctx context.Context, host string) error
	sending           chan struct{}
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup

	// telegramAPI and pushoverAPI are the addresses of Telegram's Bot API
	// and Pushover's messages, which only tests change; smtpRoots are the
	// certificates SMTP servers are checked against, nil for the system's.
	telegramAPI, pushoverAPI string
	smtpRoots                *x509.CertPool

	mu     sync.Mutex
	closed bool
	// targets are every target, as last read from the database, and
	// admins the users who are administrators and not disabled: health
	// events reach their own targets.
	targets map[accounts.ID]target
	admins  map[accounts.ID]bool
	// lanes are the messages waiting, by target.
	lanes map[accounts.ID]*lane
	// health are the problems found, by key (see health.go).
	health map[string]*healthState
	// episodesMu and healthMu serialize the looks at the series, and at
	// the problems.
	episodesMu, healthMu sync.Mutex
}

// New returns a service keeping targets in the database of options. Run
// starts following the series and the health problems.
func New(options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		db:          options.DB,
		accounts:    options.Accounts,
		library:     options.Library,
		userData:    options.UserData,
		box:         options.Secrets,
		version:     options.Version,
		serverID:    options.ServerID,
		webClient:   options.WebClient,
		problems:    options.Problems,
		logger:      options.Logger,
		now:         time.Now,
		timing:      defaultTiming,
		trusted:     stremio.HTTPClient(false),
		confined:    stremio.HTTPClient(true),
		checkPublic: stremio.CheckPublic,
		telegramAPI: defaultTelegramAPI,
		pushoverAPI: defaultPushoverAPI,
		sending:     make(chan struct{}, maxSending),
		ctx:         ctx,
		cancel:      cancel,
		targets:     map[accounts.ID]target{},
		admins:      map[accounts.ID]bool{},
		lanes:       map[accounts.ID]*lane{},
	}
}

// Run reads the targets, then looks at the series users follow and at the
// health problems when due, each on its own schedule, until ctx ends; it
// then stops sending and waits for the sends under way.
func (s *Service) Run(ctx context.Context) {
	if err := s.reload(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("The notification targets could not be read", "error", err)
	}
	var wg sync.WaitGroup
	wg.Go(func() { every(ctx, s.timing.episodesFirst, s.timing.episodesEvery, s.CheckEpisodes) })
	wg.Go(func() { every(ctx, s.timing.healthEvery, s.timing.healthEvery, s.CheckHealth) })
	wg.Wait()
	s.Close()
}

// every runs check after first, then every interval, until ctx ends.
func every(ctx context.Context, first, interval time.Duration, check func(context.Context)) {
	timer := time.NewTimer(first)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			check(ctx)
			timer.Reset(interval)
		}
	}
}

// Close stops sending and waits for the sends under way. Messages not
// sent yet are dropped.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
}

// spawn runs f on a goroutine Close waits for, unless the service is
// closed. It is called with s.mu held.
func (s *Service) spawn(f func()) bool {
	if s.closed {
		return false
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		f()
	}()
	return true
}

// sleep waits for d, and reports false if the service stopped first.
func (s *Service) sleep(d time.Duration) bool {
	if d <= 0 {
		return s.ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

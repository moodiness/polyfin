package notifications

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

const (
	// MaxName is the longest name of a target, in characters.
	MaxName = 64
	// MaxTargets bounds the targets of the server, and of each user.
	MaxTargets = 20
	// maxAddress is the longest address, in bytes, and maxToken the
	// longest access token.
	maxAddress = 2048
	maxToken   = 256
	// DefaultNtfyServer is the ntfy server an ntfy target uses when none
	// is given.
	DefaultNtfyServer = "https://ntfy.sh"
)

// ntfyTopic is what ntfy accepts as a topic name.
var ntfyTopic = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

// target is a target as the database holds it, its secret sealed or not.
type target struct {
	id    accounts.ID
	owner *accounts.ID
	kind  string
	name  string
	// address is an ntfy target's server, and a webhook's or Discord
	// target's scheme and host, shown in place of its secret address.
	address string
	topic   string
	// secret is a webhook's or Discord target's address, an ntfy target's
	// access token, as stored.
	secret        string
	events        []string
	enabled       bool
	createdAt     time.Time
	lastSent      *time.Time
	problem       string
	problemStatus *int
	problemAt     *time.Time
}

const targetColumns = "id, user_id, kind, name, address, topic, secret, events, enabled, created_at, last_sent_at, coalesce(problem, ''), problem_status, problem_at"

func scanTarget(row pgx.Row) (target, error) {
	var t target
	err := row.Scan(&t.id, &t.owner, &t.kind, &t.name, &t.address, &t.topic, &t.secret, &t.events, &t.enabled, &t.createdAt,
		&t.lastSent, &t.problem, &t.problemStatus, &t.problemAt)
	return t, err
}

// wants reports whether t receives event.
func (t target) wants(event string) bool {
	return t.enabled && slices.Contains(t.events, event)
}

// Target is a target as the admin API shows it: its secret address and
// token are never part of it.
type Target struct {
	ID   accounts.ID
	Kind string
	Name string
	// Address is an ntfy target's server, and only the scheme and host of
	// a webhook's or Discord target's address, which is secret.
	Address string
	// Topic is an ntfy target's topic; TokenSet tells whether it has an
	// access token.
	Topic    string
	TokenSet bool
	Events   []string
	Enabled  bool
	// CreatedAt is when it was added; LastSentAt when it last accepted a
	// message.
	CreatedAt  time.Time
	LastSentAt *time.Time
	// Problem is empty or one of the Problem constants, ProblemStatus the
	// status the target answered, when it answered, and ProblemAt when
	// the problem was found.
	Problem       string
	ProblemStatus *int
	ProblemAt     *time.Time
}

func (s *Service) describe(t target) Target {
	result := Target{ID: t.id, Kind: t.kind, Name: t.name, Address: t.address, Topic: t.topic, TokenSet: t.kind == Ntfy && t.secret != "",
		Events: slices.Clone(t.events), Enabled: t.enabled, CreatedAt: t.createdAt, LastSentAt: t.lastSent,
		Problem: t.problem, ProblemStatus: t.problemStatus, ProblemAt: t.problemAt}
	if _, err := s.box.Open(t.secret); err != nil {
		result.Problem, result.ProblemStatus, result.ProblemAt = ProblemUnreadable, nil, nil
	}
	return result
}

// Draft is what an owner gives to add a target or change one. On a
// change, nil fields keep their values; Kind is only read when adding.
// Address is a webhook's or Discord target's address, or an ntfy
// target's server, empty for DefaultNtfyServer; Token is an ntfy target's
// access token, empty for none.
type Draft struct {
	Kind    string
	Name    *string
	Address *string
	Topic   *string
	Token   *string
	Events  []string
	Enabled *bool
}

// ownerID is the identifier of owner, nil for the server.
func ownerID(owner *accounts.User) *accounts.ID {
	if owner == nil {
		return nil
	}
	return &owner.ID
}

// Targets lists the targets of owner, nil for the server's, oldest first.
func (s *Service) Targets(ctx context.Context, owner *accounts.User) ([]Target, error) {
	rows, err := s.db.Query(ctx, "SELECT "+targetColumns+" FROM notification_targets WHERE user_id IS NOT DISTINCT FROM $1 ORDER BY created_at, id",
		ownerID(owner))
	if err != nil {
		return nil, err
	}
	stored, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (target, error) { return scanTarget(row) })
	if err != nil {
		return nil, err
	}
	result := make([]Target, 0, len(stored))
	for _, t := range stored {
		result = append(result, s.describe(t))
	}
	return result, nil
}

// load reads the target id of owner: ErrNotFound when owner has none.
func (s *Service) load(ctx context.Context, owner *accounts.User, id accounts.ID) (target, error) {
	t, err := scanTarget(s.db.QueryRow(ctx, "SELECT "+targetColumns+" FROM notification_targets WHERE id = $1 AND user_id IS NOT DISTINCT FROM $2",
		id, ownerID(owner)))
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// Create adds a target for owner, nil for the server.
func (s *Service) Create(ctx context.Context, owner *accounts.User, d Draft) (Target, error) {
	if !slices.Contains(Kinds, d.Kind) {
		return Target{}, ErrInvalidKind
	}
	t := target{kind: d.Kind, owner: ownerID(owner), enabled: true, events: []string{}}
	switch {
	case d.Name == nil:
		return Target{}, ErrInvalidName
	case d.Kind == Ntfy && d.Topic == nil:
		return Target{}, ErrInvalidTopic
	case d.Kind != Ntfy && d.Address == nil:
		return Target{}, ErrInvalidAddress
	}
	if d.Kind == Ntfy && d.Address == nil {
		d.Address = new(string)
	}
	if err := s.apply(ctx, owner, &t, d); err != nil {
		return Target{}, err
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// The owner's targets are counted under a lock on the owner, or on
		// the settings for the server's, so that two additions at once do
		// not pass the bound together.
		lock := "SELECT 1 FROM settings FOR UPDATE"
		args := []any{}
		if owner != nil {
			lock, args = "SELECT 1 FROM users WHERE id = $1 FOR UPDATE", []any{owner.ID}
		}
		if _, err := tx.Exec(ctx, lock, args...); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM notification_targets WHERE user_id IS NOT DISTINCT FROM $1", t.owner).Scan(&count); err != nil {
			return err
		}
		if count >= MaxTargets {
			return ErrTooManyTargets
		}
		row := tx.QueryRow(ctx, `INSERT INTO notification_targets (user_id, kind, name, address, topic, secret, events, enabled, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+targetColumns,
			t.owner, t.kind, t.name, t.address, t.topic, t.secret, t.events, t.enabled, s.now())
		var err error
		t, err = scanTarget(row)
		return err
	})
	if err != nil {
		return Target{}, err
	}
	s.reloadQuietly(ctx)
	s.logger.Info("A notification target was added", "target", t.id.String(), "kind", t.kind, "user_id", ownerLog(owner))
	return s.describe(t), nil
}

// Update changes the target id of owner.
func (s *Service) Update(ctx context.Context, owner *accounts.User, id accounts.ID, d Draft) (Target, error) {
	t, err := s.load(ctx, owner, id)
	if err != nil {
		return Target{}, err
	}
	before := t
	if err := s.apply(ctx, owner, &t, d); err != nil {
		return Target{}, err
	}
	// A new address or token is another target as far as its problem
	// goes.
	clear := t.secret != before.secret || t.address != before.address || t.topic != before.topic
	row := s.db.QueryRow(ctx, `UPDATE notification_targets SET name = $2, address = $3, topic = $4, secret = $5, events = $6, enabled = $7,
			problem = CASE WHEN $8 THEN NULL ELSE problem END,
			problem_status = CASE WHEN $8 THEN NULL ELSE problem_status END,
			problem_at = CASE WHEN $8 THEN NULL ELSE problem_at END
		WHERE id = $1 RETURNING `+targetColumns,
		id, t.name, t.address, t.topic, t.secret, t.events, t.enabled, clear)
	if t, err = scanTarget(row); errors.Is(err, pgx.ErrNoRows) {
		return Target{}, ErrNotFound
	} else if err != nil {
		return Target{}, err
	}
	s.reloadQuietly(ctx)
	return s.describe(t), nil
}

// Delete deletes the target id of owner, and the messages waiting for it.
func (s *Service) Delete(ctx context.Context, owner *accounts.User, id accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM notification_targets WHERE id = $1 AND user_id IS NOT DISTINCT FROM $2", id, ownerID(owner))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	s.mu.Lock()
	if l := s.lanes[id]; l != nil {
		l.queue = nil
	}
	s.mu.Unlock()
	s.reloadQuietly(ctx)
	s.logger.Info("A notification target was deleted", "target", id.String(), "user_id", ownerLog(owner))
	return nil
}

// ownerLog names owner in the log: the user's identifier, "server" for
// the server's targets.
func ownerLog(owner *accounts.User) string {
	if owner == nil {
		return "server"
	}
	return owner.ID.String()
}

// apply checks d and writes it into t, the secret sealed.
func (s *Service) apply(ctx context.Context, owner *accounts.User, t *target, d Draft) error {
	if d.Name != nil {
		name := strings.TrimSpace(*d.Name)
		if name == "" || utf8.RuneCountInString(name) > MaxName || strings.ContainsFunc(name, unicode.IsControl) {
			return ErrInvalidName
		}
		t.name = name
	}
	confined := owner != nil && !owner.IsAdministrator
	if d.Address != nil {
		address := strings.TrimSpace(*d.Address)
		if t.kind == Ntfy {
			address = strings.TrimRight(address, "/")
			if address == "" {
				address = DefaultNtfyServer
			}
		}
		parsed, err := s.checkAddress(ctx, address, confined)
		if err != nil {
			return err
		}
		if t.kind == Ntfy {
			// The server is not secret; a query or fragment has no place
			// in it.
			if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
				return ErrInvalidAddress
			}
			t.address = address
		} else {
			t.address = parsed.Scheme + "://" + parsed.Host
			t.secret = s.box.Seal(address)
		}
	}
	if d.Topic != nil {
		if t.kind != Ntfy {
			return ErrInvalidTopic
		}
		topic := strings.TrimSpace(*d.Topic)
		if !ntfyTopic.MatchString(topic) {
			return ErrInvalidTopic
		}
		t.topic = topic
	}
	if d.Token != nil {
		if t.kind != Ntfy || !validToken(*d.Token) {
			return ErrInvalidToken
		}
		t.secret = s.box.Seal(*d.Token)
	}
	if d.Events != nil {
		allowed := EventsFor(owner)
		events := []string{}
		for _, event := range d.Events {
			if !slices.Contains(allowed, event) {
				return ErrInvalidEvents
			}
			if !slices.Contains(events, event) {
				events = append(events, event)
			}
		}
		// Kept in the order the admin app shows them.
		slices.SortFunc(events, func(a, b string) int { return slices.Index(Events, a) - slices.Index(Events, b) })
		t.events = events
	}
	if d.Enabled != nil {
		t.enabled = *d.Enabled
	}
	return nil
}

// checkAddress parses an http or https address with a host. A confined
// owner's may not be on a local network, as far as its name tells now:
// the requests sent to it check the address they connect to too.
func (s *Service) checkAddress(ctx context.Context, address string, confined bool) (*url.URL, error) {
	if len(address) > maxAddress || strings.ContainsFunc(address, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return nil, ErrInvalidAddress
	}
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, ErrInvalidAddress
	}
	if confined && s.checkPublic(ctx, parsed.Hostname()) != nil {
		return nil, ErrPrivateAddress
	}
	return parsed, nil
}

// validToken reports whether token may be an access token: empty, or
// printable ASCII without spaces of at most maxToken bytes.
func validToken(token string) bool {
	if len(token) > maxToken {
		return false
	}
	for i := range len(token) {
		if token[i] <= ' ' || token[i] > '~' {
			return false
		}
	}
	return true
}

// reload reads every target, and which users are administrators, into
// memory: messages are delivered from there.
func (s *Service) reload(ctx context.Context) error {
	rows, err := s.db.Query(ctx, "SELECT "+targetColumns+" FROM notification_targets")
	if err != nil {
		return err
	}
	stored, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (target, error) { return scanTarget(row) })
	if err != nil {
		return err
	}
	users, err := s.accounts.Users(ctx)
	if err != nil {
		return err
	}
	targets := make(map[accounts.ID]target, len(stored))
	for _, t := range stored {
		targets[t.id] = t
	}
	admins := map[accounts.ID]bool{}
	for _, user := range users {
		if user.IsAdministrator && !user.IsDisabled {
			admins[user.ID] = true
		}
	}
	s.mu.Lock()
	s.targets, s.admins = targets, admins
	s.mu.Unlock()
	return nil
}

// reloadQuietly reloads the targets after a change, logging a failure: the
// change is saved, and the next reload sees it.
func (s *Service) reloadQuietly(ctx context.Context) {
	if err := s.reload(context.WithoutCancel(ctx)); err != nil {
		s.logger.Warn("The notification targets could not be read again", "error", err)
	}
}

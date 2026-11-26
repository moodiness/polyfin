package moonfin

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Messages from the administrator.
//
// The administrator writes messages to the users of Moonfin's apps, which
// show them in a messages window: an unread badge on the menu button, or
// the window opening on its own. Each message is meant for an audience,
// and shows between its dates; GET /Moonfin/Messages gives a user those of
// theirs that show now, so that no message meant for another reaches them.
// The apps keep which they read themselves: Polyfin keeps no read state.
// Those who watch a user's messages (see WatchMessages) are told when a
// message of theirs is saved or deleted, and when one starts or stops
// showing.

// MessageColor is the color an app gives a message.
type MessageColor string

// The colors of a message.
const (
	ColorWhite  MessageColor = "white"
	ColorBlue   MessageColor = "blue"
	ColorGreen  MessageColor = "green"
	ColorYellow MessageColor = "yellow"
	ColorRed    MessageColor = "red"
)

// MessageColors lists the colors of a message.
var MessageColors = []MessageColor{ColorWhite, ColorBlue, ColorGreen, ColorYellow, ColorRed}

// Delivery is how an app tells its user of a message they did not read.
type Delivery string

const (
	// DeliveryInbox marks the menu button unread.
	DeliveryInbox Delivery = "inbox"
	// DeliveryPopup opens the messages window on its own, once.
	DeliveryPopup Delivery = "popup"
)

// Audience is whom a message is meant for.
type Audience string

const (
	// AudienceAll is every user.
	AudienceAll Audience = "all"
	// AudienceUsers is the message's chosen users.
	AudienceUsers Audience = "users"
	// AudienceAdmins is the administrators.
	AudienceAdmins Audience = "admins"
)

const (
	// MaxMessages bounds how many messages are kept.
	MaxMessages = 50
	// MaxTitleLength bounds a message's title, in characters.
	MaxTitleLength = 120
	// MaxBodyLength bounds a message's body, in characters: enough for a
	// note, short enough for an app's window.
	MaxBodyLength = 2000
	// MaxLinkLabelLength bounds the label of a message's link button.
	MaxLinkLabelLength = 60
	// MaxLinkURLLength bounds the address of a message's link button.
	MaxLinkURLLength = 2000
)

var (
	// ErrMessageNotFound reports a message that does not exist.
	ErrMessageNotFound = errors.New("no such Moonfin message")
	// ErrInvalidMessage reports a message Valid refuses.
	ErrInvalidMessage = errors.New("invalid Moonfin message")
	// ErrTooManyMessages reports a message past MaxMessages.
	ErrTooManyMessages = errors.New("too many Moonfin messages")
)

// Message is a message from the administrator. Title or Body, Markdown,
// is set. Users are those of AudienceUsers, none for another audience. It
// shows from Start until End, each zero for no bound. LinkLabel and
// LinkURL, an http or https address, make a button, both set or neither.
type Message struct {
	ID        accounts.ID
	Title     string
	Body      string
	Color     MessageColor
	Delivery  Delivery
	Audience  Audience
	Users     []accounts.ID
	Start     time.Time
	End       time.Time
	LinkLabel string
	LinkURL   string
	Created   time.Time
	Updated   time.Time
}

// Cleaned is m with its texts trimmed of spaces around them, its users
// sorted once each, none but for AudienceUsers.
func (m Message) Cleaned() Message {
	m.Title, m.Body = strings.TrimSpace(m.Title), strings.TrimSpace(m.Body)
	m.LinkLabel, m.LinkURL = strings.TrimSpace(m.LinkLabel), strings.TrimSpace(m.LinkURL)
	if m.Audience == AudienceUsers {
		m.Users = slices.Compact(slices.SortedFunc(slices.Values(m.Users), compareIDs))
	} else {
		m.Users = nil
	}
	return m
}

func compareIDs(a, b accounts.ID) int {
	return strings.Compare(a.String(), b.String())
}

// Valid reports whether m, Cleaned, may be kept: a title or a body, each
// within its bounds, of text a database keeps; a known color, delivery
// and audience, with users for AudienceUsers; an end after the start; a
// link of a label and an http or https address, or none.
func (m Message) Valid() bool {
	texts := []struct {
		text string
		max  int
	}{{m.Title, MaxTitleLength}, {m.Body, MaxBodyLength}, {m.LinkLabel, MaxLinkLabelLength}, {m.LinkURL, MaxLinkURLLength}}
	for _, t := range texts {
		if !utf8.ValidString(t.text) || strings.ContainsRune(t.text, 0) || utf8.RuneCountInString(t.text) > t.max {
			return false
		}
	}
	switch {
	case m.Title == "" && m.Body == "",
		!slices.Contains(MessageColors, m.Color),
		m.Delivery != DeliveryInbox && m.Delivery != DeliveryPopup,
		m.Audience != AudienceAll && m.Audience != AudienceUsers && m.Audience != AudienceAdmins,
		m.Audience == AudienceUsers && len(m.Users) == 0,
		!m.Start.IsZero() && !m.End.IsZero() && !m.End.After(m.Start),
		(m.LinkLabel == "") != (m.LinkURL == ""):
		return false
	}
	return m.LinkURL == "" || WebLink(m.LinkURL)
}

// WebLink reports whether raw is an http or https address, the only ones
// a message's link opens on a user's device.
func WebLink(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && u.Host != "" &&
		!strings.ContainsAny(raw, " \t\r\n")
}

// ShowsAt reports whether m shows at now, between its dates.
func (m Message) ShowsAt(now time.Time) bool {
	return (m.Start.IsZero() || !now.Before(m.Start)) && (m.End.IsZero() || now.Before(m.End))
}

// Messages lists every message, in order, whatever its dates.
func (s *Store) Messages(ctx context.Context) ([]Message, error) {
	return s.loadMessages(ctx, "")
}

// Message returns a message.
func (s *Store) Message(ctx context.Context, id accounts.ID) (Message, error) {
	messages, err := s.loadMessages(ctx, "WHERE id = $1", id)
	if err != nil {
		return Message{}, err
	}
	if len(messages) == 0 {
		return Message{}, ErrMessageNotFound
	}
	return messages[0], nil
}

// MessagesFor lists, in order, the messages meant for user that show now.
func (s *Store) MessagesFor(ctx context.Context, user accounts.User) ([]Message, error) {
	return s.loadMessages(ctx, `WHERE (start_at IS NULL OR start_at <= $1) AND (end_at IS NULL OR end_at > $1)
		AND (audience = 'all' OR audience = 'admins' AND $2
			OR audience = 'users' AND EXISTS (SELECT FROM moonfin_message_users u WHERE u.message_id = id AND u.user_id = $3))`,
		s.now(), user.IsAdministrator, user.ID)
}

func (s *Store) loadMessages(ctx context.Context, where string, args ...any) ([]Message, error) {
	rows, _ := s.db.Query(ctx, `SELECT id, title, body, color, delivery, audience, start_at, end_at, link_label, link_url, created_at, updated_at
		FROM moonfin_messages `+where+` ORDER BY position, created_at, id`, args...)
	var result []Message
	var m Message
	var start, end *time.Time
	if _, err := pgx.ForEachRow(rows, []any{&m.ID, &m.Title, &m.Body, &m.Color, &m.Delivery, &m.Audience, &start, &end, &m.LinkLabel,
		&m.LinkURL, &m.Created, &m.Updated}, func() error {
		message := m
		if start != nil {
			message.Start = *start
		}
		if end != nil {
			message.End = *end
		}
		result = append(result, message)
		return nil
	}); err != nil || len(result) == 0 {
		return result, err
	}
	index := make(map[accounts.ID]int, len(result))
	ids := make([]accounts.ID, len(result))
	for i, message := range result {
		index[message.ID], ids[i] = i, message.ID
	}
	var message, user accounts.ID
	users, _ := s.db.Query(ctx, "SELECT message_id, user_id FROM moonfin_message_users WHERE message_id = ANY($1) ORDER BY user_id", ids)
	if _, err := pgx.ForEachRow(users, []any{&message, &user}, func() error {
		result[index[message]].Users = append(result[index[message]].Users, user)
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// optionalTime is t for the database: NULL when zero.
func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// setMessageUsers keeps the users of m; those that no longer exist are
// left out.
func setMessageUsers(ctx context.Context, tx pgx.Tx, m Message) error {
	if _, err := tx.Exec(ctx, "DELETE FROM moonfin_message_users WHERE message_id = $1", m.ID); err != nil || len(m.Users) == 0 {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO moonfin_message_users (message_id, user_id)
		SELECT $1, id FROM users WHERE id = ANY($2) ORDER BY id`, m.ID, m.Users)
	return err
}

// CreateMessage adds m, Cleaned, before the others, as the newest, and
// tells those it is meant for.
func (s *Store) CreateMessage(ctx context.Context, m Message) (Message, error) {
	m = m.Cleaned()
	if !m.Valid() {
		return Message{}, ErrInvalidMessage
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Messages added at once count one after the other.
		if _, err := tx.Exec(ctx, "LOCK TABLE moonfin_messages IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
		var count, first int
		if err := tx.QueryRow(ctx, "SELECT count(*), coalesce(min(position), 1) FROM moonfin_messages").Scan(&count, &first); err != nil {
			return err
		}
		if count >= MaxMessages {
			return ErrTooManyMessages
		}
		now := s.now()
		if err := tx.QueryRow(ctx, `INSERT INTO moonfin_messages (position, title, body, color, delivery, audience, start_at, end_at,
			link_label, link_url, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11) RETURNING id`,
			first-1, m.Title, m.Body, m.Color, m.Delivery, m.Audience, optionalTime(m.Start), optionalTime(m.End), m.LinkLabel, m.LinkURL,
			now).Scan(&m.ID); err != nil {
			return err
		}
		return setMessageUsers(ctx, tx, m)
	})
	if err != nil {
		return Message{}, err
	}
	created, err := s.Message(ctx, m.ID)
	if err != nil {
		return Message{}, err
	}
	s.messagesChanged(ctx, created)
	return created, nil
}

// UpdateMessage changes a message to m, Cleaned, its place in the list
// kept, and tells those it was and is meant for.
func (s *Store) UpdateMessage(ctx context.Context, m Message) (Message, error) {
	m = m.Cleaned()
	if !m.Valid() {
		return Message{}, ErrInvalidMessage
	}
	before, err := s.Message(ctx, m.ID)
	if err != nil {
		return Message{}, err
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE moonfin_messages SET title = $2, body = $3, color = $4, delivery = $5, audience = $6, start_at = $7,
			end_at = $8, link_label = $9, link_url = $10, updated_at = $11 WHERE id = $1`,
			m.ID, m.Title, m.Body, m.Color, m.Delivery, m.Audience, optionalTime(m.Start), optionalTime(m.End), m.LinkLabel, m.LinkURL, s.now())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrMessageNotFound
		}
		return setMessageUsers(ctx, tx, m)
	})
	if err != nil {
		return Message{}, err
	}
	updated, err := s.Message(ctx, m.ID)
	if err != nil {
		return Message{}, err
	}
	s.messagesChanged(ctx, before, updated)
	return updated, nil
}

// DeleteMessage deletes a message, and tells those it was meant for.
func (s *Store) DeleteMessage(ctx context.Context, id accounts.ID) error {
	before, err := s.Message(ctx, id)
	if err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, "DELETE FROM moonfin_messages WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMessageNotFound
	}
	s.messagesChanged(ctx, before)
	return nil
}

// OrderMessages puts the messages in the order of ids; those ids leaves
// out keep their order after them. Every watcher is told.
func (s *Store) OrderMessages(ctx context.Context, ids []accounts.ID) error {
	_, err := s.db.Exec(ctx, `UPDATE moonfin_messages m SET position = o.position
		FROM (SELECT id, row_number() OVER (ORDER BY array_position($1::uuid[], id) IS NULL, array_position($1::uuid[], id), position,
			created_at, id) AS position FROM moonfin_messages) o
		WHERE m.id = o.id`, ids)
	if err == nil {
		s.tellMessages(true, nil)
	}
	return err
}

// messageWatch keeps those who watch users' messages, and the timer that
// tells them when the next message starts or stops showing.
type messageWatch struct {
	mu       sync.Mutex
	watchers map[accounts.ID]map[chan struct{}]struct{}
	// armed tells that timer, if any, is set for the next start or end.
	armed bool
	timer *time.Timer
}

// WatchMessages returns a channel told each time a message meant for
// user is saved or deleted, or starts or stops showing, and the function
// that stops it. A change told while the last is still unread is one
// with it.
func (s *Store) WatchMessages(ctx context.Context, user accounts.ID) (<-chan struct{}, func()) {
	changed := make(chan struct{}, 1)
	w := &s.messages
	w.mu.Lock()
	if w.watchers == nil {
		w.watchers = map[accounts.ID]map[chan struct{}]struct{}{}
	}
	if w.watchers[user] == nil {
		w.watchers[user] = map[chan struct{}]struct{}{}
	}
	w.watchers[user][changed] = struct{}{}
	armed := w.armed
	w.mu.Unlock()
	if !armed {
		s.armMessages(ctx)
	}
	return changed, func() {
		w.mu.Lock()
		delete(w.watchers[user], changed)
		if len(w.watchers[user]) == 0 {
			delete(w.watchers, user)
		}
		w.mu.Unlock()
	}
}

// tellMessages tells the watchers of users, or every watcher with all.
func (s *Store) tellMessages(all bool, users []accounts.ID) {
	w := &s.messages
	w.mu.Lock()
	defer w.mu.Unlock()
	tell := func(watchers map[chan struct{}]struct{}) {
		for changed := range watchers {
			select {
			case changed <- struct{}{}:
			default:
			}
		}
	}
	if all {
		for _, watchers := range w.watchers {
			tell(watchers)
		}
		return
	}
	for _, user := range users {
		tell(w.watchers[user])
	}
}

// messagesChanged tells the watchers of those messages are meant for, and
// sets the timer again for the next start or end.
func (s *Store) messagesChanged(ctx context.Context, messages ...Message) {
	s.tellAudiences(ctx, messages)
	s.armMessages(ctx)
}

// tellAudiences tells the watchers of those messages are meant for: every
// user, the administrators, or their users.
func (s *Store) tellAudiences(ctx context.Context, messages []Message) {
	var users []accounts.ID
	admins := false
	for _, m := range messages {
		switch m.Audience {
		case AudienceAll:
			s.tellMessages(true, nil)
			return
		case AudienceAdmins:
			admins = true
		case AudienceUsers:
			users = append(users, m.Users...)
		}
	}
	if admins {
		rows, _ := s.db.Query(ctx, "SELECT id FROM users WHERE is_administrator")
		administrators, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		if err != nil {
			// Better every watcher than an administrator left untold.
			slog.Default().Warn("moonfin: listing the administrators to tell of a message", "error", err)
			s.tellMessages(true, nil)
			return
		}
		users = append(users, administrators...)
	}
	s.tellMessages(false, users)
}

// armMessages sets the timer for the next start or end of a message after
// now, when it tells those the messages starting or stopping are meant
// for, then sets itself again.
func (s *Store) armMessages(ctx context.Context) {
	now := s.now()
	var next *time.Time
	err := s.db.QueryRow(ctx, `SELECT min(at) FROM (SELECT start_at AS at FROM moonfin_messages UNION ALL SELECT end_at FROM moonfin_messages) b
		WHERE at > $1`, now).Scan(&next)
	w := &s.messages
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		// The next watcher tries again.
		slog.Default().Warn("moonfin: finding when the next message starts or ends", "error", err)
		w.armed = false
		return
	}
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.armed = true
	if next != nil {
		w.timer = time.AfterFunc(next.Sub(now), func() { s.messagesTurned(now) })
	}
}

// messagesTurned tells those meant for the messages that started or
// stopped showing since then, and sets the timer again.
func (s *Store) messagesTurned(since time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	turned, err := s.loadMessages(ctx, "WHERE start_at > $1 AND start_at <= $2 OR end_at > $1 AND end_at <= $2", since, s.now())
	if err != nil {
		slog.Default().Warn("moonfin: listing the messages that started or ended", "error", err)
		s.messages.mu.Lock()
		s.messages.armed = false
		s.messages.mu.Unlock()
		return
	}
	s.messagesChanged(ctx, turned...)
}

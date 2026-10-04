// Package activity keeps the server's activity log, as Jellyfin's: who
// signed in or failed to, what was played, and the changes administrators
// made to users, settings and addons. Entries are named in the server
// language when they are written, as Jellyfin names them, and kept
// Retention.
package activity

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Retention is how long entries are kept.
const Retention = 30 * 24 * time.Hour

// SweepInterval is how often entries older than Retention are deleted.
const SweepInterval = 24 * time.Hour

// Severities, as Jellyfin's LogLevel names.
const (
	Information = "Information"
	Warning     = "Warning"
	Error       = "Error"
)

// Entry is one thing that happened.
type Entry struct {
	ID            int64
	Name          string
	Overview      *string
	ShortOverview *string
	// Type is the kind of entry, as Jellyfin's activity types where it has
	// one.
	Type   string
	ItemID *string
	// UserID is the user the entry is about, nil when none is.
	UserID   *accounts.ID
	Date     time.Time
	Severity string
}

// Store records and lists entries.
type Store struct {
	db       *pgxpool.Pool
	settings func() accounts.Settings
	logger   *slog.Logger
	now      func() time.Time
}

// New returns a store; settings gives the language entries are named in.
func New(db *pgxpool.Pool, settings func() accounts.Settings, logger *slog.Logger) *Store {
	return &Store{db: db, settings: settings, logger: logger, now: time.Now}
}

// record writes an entry. The activity log is a record, not part of what
// is being done: a failure is logged, never returned.
func (s *Store) record(ctx context.Context, e Entry) {
	if s == nil {
		return
	}
	if e.Severity == "" {
		e.Severity = Information
	}
	_, err := s.db.Exec(context.WithoutCancel(ctx), `INSERT INTO activity_log (name, overview, short_overview, type, item_id, user_id, date, severity)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		clip(e.Name, 1024), clipped(e.Overview, 4096), clipped(e.ShortOverview, 1024), e.Type, e.ItemID, e.UserID, s.now(), e.Severity)
	if err != nil {
		s.logger.Warn("An activity could not be recorded", "type", e.Type, "error", err)
	}
}

// clip shortens s to at most n characters.
func clip(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func clipped(s *string, n int) *string {
	if s == nil {
		return nil
	}
	return new(clip(*s, n))
}

// Query selects entries, as Jellyfin's activity log query does.
type Query struct {
	Start int
	// Limit is how many entries to return; 0 returns all.
	Limit int
	// MinDate keeps entries from that date; the zero time keeps all.
	MinDate time.Time
	// HasUserID keeps the entries about a user when true, the others when
	// false; nil keeps both.
	HasUserID *bool
}

// Page is the entries a query selected, newest first, with how many it
// selects in all.
type Page struct {
	Entries []Entry
	Total   int
}

// Entries lists the entries q selects; a nil store has none.
func (s *Store) Entries(ctx context.Context, q Query) (Page, error) {
	if s == nil {
		return Page{Entries: []Entry{}}, nil
	}
	var minDate *time.Time
	if !q.MinDate.IsZero() {
		minDate = &q.MinDate
	}
	var limit *int
	if q.Limit > 0 {
		limit = &q.Limit
	}
	const where = "WHERE ($1::timestamptz IS NULL OR date >= $1) AND ($2::boolean IS NULL OR (user_id IS NOT NULL) = $2)"
	rows, err := s.db.Query(ctx, `SELECT id, name, overview, short_overview, type, item_id, user_id, date, severity
		FROM activity_log `+where+` ORDER BY date DESC, id DESC OFFSET $3 LIMIT $4`, minDate, q.HasUserID, max(q.Start, 0), limit)
	if err != nil {
		return Page{}, err
	}
	entries, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Entry])
	if err != nil {
		return Page{}, err
	}
	page := Page{Entries: append([]Entry{}, entries...)}
	err = s.db.QueryRow(ctx, "SELECT count(*) FROM activity_log "+where, minDate, q.HasUserID).Scan(&page.Total)
	return page, err
}

// Sweep deletes the entries older than Retention and returns how many.
func (s *Store) Sweep(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, "DELETE FROM activity_log WHERE date < $1", s.now().Add(-Retention))
	return tag.RowsAffected(), err
}

// phrase names an entry in the server language from the English and
// French patterns, each with %s for args. The methods below record
// nothing on a nil store.
func (s *Store) phrase(english, french string, args ...any) string {
	if s == nil {
		return ""
	}
	pattern := english
	if s.settings().Language == "fr" {
		pattern = french
	}
	return fmt.Sprintf(pattern, args...)
}

func (s *Store) address(address string) *string {
	return new(s.phrase("Address: %s", "Adresse : %s", address))
}

// SignedIn records that user signed in from address, from a Jellyfin app
// or the admin interface.
func (s *Store) SignedIn(ctx context.Context, user accounts.User, address string) {
	s.record(ctx, Entry{Name: s.phrase("%s signed in", "Connexion de %s", user.Name), Type: "AuthenticationSucceeded",
		UserID: &user.ID, ShortOverview: s.address(address)})
}

// SignInFailed records a sign-in refused for name from address.
func (s *Store) SignInFailed(ctx context.Context, name, address string) {
	s.record(ctx, Entry{Name: s.phrase("Failed sign-in as %s", "Échec de connexion en tant que %s", name), Type: "AuthenticationFailed",
		ShortOverview: s.address(address), Severity: Error})
}

// PlaybackStarted records that user started playing title, the item item,
// on device.
func (s *Store) PlaybackStarted(ctx context.Context, user accounts.User, title, item, device string) {
	s.record(ctx, Entry{Name: s.phrase("%s is playing %s on %s", "%s regarde %s sur %s", user.Name, title, device),
		Type: "VideoPlayback", UserID: &user.ID, ItemID: &item})
}

// PlaybackStopped records that user stopped playing title on device.
func (s *Store) PlaybackStopped(ctx context.Context, user accounts.User, title, item, device string) {
	s.record(ctx, Entry{Name: s.phrase("%s stopped playing %s on %s", "%s a arrêté %s sur %s", user.Name, title, device),
		Type: "VideoPlaybackStopped", UserID: &user.ID, ItemID: &item})
}

// UserCreated records a new account.
func (s *Store) UserCreated(ctx context.Context, user accounts.User) {
	s.record(ctx, Entry{Name: s.phrase("User %s was created", "Utilisateur %s créé", user.Name), Type: "UserCreated", UserID: &user.ID})
}

// UserDeleted records a deleted account. Like Jellyfin's, the entry is
// about no user: the user no longer exists.
func (s *Store) UserDeleted(ctx context.Context, name string) {
	s.record(ctx, Entry{Name: s.phrase("User %s was deleted", "Utilisateur %s supprimé", name), Type: "UserDeleted"})
}

// UserChanged records a change to an account other than its password.
func (s *Store) UserChanged(ctx context.Context, user accounts.User) {
	s.record(ctx, Entry{Name: s.phrase("User %s was changed", "Utilisateur %s modifié", user.Name), Type: "UserPolicyUpdated", UserID: &user.ID})
}

// PasswordChanged records a new password for user.
func (s *Store) PasswordChanged(ctx context.Context, user accounts.User) {
	s.record(ctx, Entry{Name: s.phrase("The password of %s was changed", "Mot de passe de %s changé", user.Name),
		Type: "UserPasswordChanged", UserID: &user.ID})
}

// SettingsSaved records that administrator saved the server settings.
func (s *Store) SettingsSaved(ctx context.Context, administrator string) {
	s.record(ctx, Entry{Name: s.phrase("%s saved the server settings", "%s a enregistré les paramètres du serveur", administrator),
		Type: "ServerConfigurationUpdated"})
}

// AddonInstalled records that by installed the addon name at version,
// for the server or for themselves.
func (s *Store) AddonInstalled(ctx context.Context, by accounts.User, name, version string, shared bool) {
	s.record(ctx, Entry{Name: s.addonPhrase(by, name, shared, "installed", "installé"), Type: "AddonInstalled",
		ShortOverview: new(s.phrase("Version %s", "Version %s", version)), UserID: s.owner(by, shared)})
}

// AddonRemoved records that by removed the addon name.
func (s *Store) AddonRemoved(ctx context.Context, by accounts.User, name string, shared bool) {
	s.record(ctx, Entry{Name: s.addonPhrase(by, name, shared, "removed", "retiré"), Type: "AddonUninstalled", UserID: s.owner(by, shared)})
}

func (s *Store) addonPhrase(by accounts.User, name string, shared bool, english, french string) string {
	if shared {
		return s.phrase("%s "+english+" the addon %s for the server", "%s a "+french+" l’addon %s pour le serveur", by.Name, name)
	}
	return s.phrase("%s "+english+" the addon %s for themselves", "%s a "+french+" l’addon %s pour son compte", by.Name, name)
}

// owner is the user an addon entry is about: the user whose own addon it
// is, none for the server's.
func (s *Store) owner(by accounts.User, shared bool) *accounts.ID {
	if shared {
		return nil
	}
	return &by.ID
}

// RecordingScheduled records that user scheduled the recording of title,
// one programme or a series.
func (s *Store) RecordingScheduled(ctx context.Context, user accounts.User, title string) {
	s.record(ctx, Entry{Name: s.phrase("%s scheduled the recording of %s", "%s a programmé l’enregistrement de %s", user.Name, title),
		Type: "RecordingScheduled", UserID: &user.ID})
}

// RecordingDeleted records that user deleted the recording of title.
func (s *Store) RecordingDeleted(ctx context.Context, user accounts.User, title string) {
	s.record(ctx, Entry{Name: s.phrase("%s deleted the recording of %s", "%s a supprimé l’enregistrement de %s", user.Name, title),
		Type: "RecordingDeleted", UserID: &user.ID})
}

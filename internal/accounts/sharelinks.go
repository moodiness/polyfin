package accounts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// The expiries a share link may have, the default, and how many guests
// may watch through one at once.
var ShareLinkExpiries = []time.Duration{time.Hour, 24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}

const (
	DefaultShareLinkExpiry = 24 * time.Hour
	MaxShareLinkViewers    = 5
	// ShareLinkNotifyEvery is how often share_played tells of one link at
	// most.
	ShareLinkNotifyEvery = time.Hour
	// ShareLinkCheckInterval is how often the links that expired are
	// looked for, to record them (see ExpiredShareLinks). The expiry
	// itself applies at once.
	ShareLinkCheckInterval = 10 * time.Minute
	// shareLinksKept is how long a link that expired or was revoked is
	// still listed, before it is deleted.
	shareLinksKept = 30 * 24 * time.Hour
	// maxShareLinkTitle bounds the name a link keeps of its title, in
	// bytes, as the table does.
	maxShareLinkTitle = 1024
)

// The kinds of titles a share link opens.
const (
	ShareMovie   = "movie"
	ShareEpisode = "episode"
)

// The states of a share link.
const (
	ShareLinkActive  = "active"
	ShareLinkExpired = "expired"
	ShareLinkRevoked = "revoked"
)

var (
	ErrInvalidShareExpiry  = errors.New("invalid share link expiry")
	ErrInvalidShareViewers = errors.New("invalid number of share link viewers")
	ErrInvalidShareTitle   = errors.New("invalid share link title")
	// ErrShareLinksOff refuses to make a link while the server or the
	// user may not share, and every link meanwhile.
	ErrShareLinksOff = errors.New("share links are off")
	// ErrShareLinkUnknown reports a token that is no link's; the other
	// errors, a link that no longer opens its title.
	ErrShareLinkUnknown = errors.New("unknown share link")
	ErrShareLinkExpired = errors.New("share link expired")
	ErrShareLinkRevoked = errors.New("share link revoked")
)

// ShareLink is a link that opens one movie or one episode of User's to
// guests without an account, at most MaxViewers of them at once, until
// ExpiresAt, unless revoked.
type ShareLink struct {
	ID   ID
	User ShareLinkUser
	// Item is the title, of Kind ShareMovie or ShareEpisode, named Title
	// as it was when the link was made.
	Item       ID
	Kind       string
	Title      string
	MaxViewers int
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	// Plays counts the guests' playbacks.
	Plays int
}

// ShareLinkUser names the user who made a link.
type ShareLinkUser struct {
	ID   ID
	Name string
}

// State tells whether the link opens its title at now: active, or else
// revoked or expired, in that order.
func (l ShareLink) State(now time.Time) string {
	switch {
	case l.RevokedAt != nil:
		return ShareLinkRevoked
	case !now.Before(l.ExpiresAt):
		return ShareLinkExpired
	}
	return ShareLinkActive
}

// NewShareLink describes a link to make: by whom, for which title, how
// long it lasts, one of ShareLinkExpiries, and how many guests may watch
// at once, from 1 to MaxShareLinkViewers.
type NewShareLink struct {
	User       ID
	Item       ID
	Kind       string
	Title      string
	Expiry     time.Duration
	MaxViewers int
}

const (
	shareLinkColumns = "l.id, l.user_id, u.name, l.item_id, l.kind, l.title, l.max_viewers, l.created_at, l.expires_at, l.revoked_at, l.plays"
	shareLinkJoins   = " FROM share_links l JOIN users u ON u.id = l.user_id"
)

func scanShareLink(row pgx.Row, more ...any) (ShareLink, error) {
	var link ShareLink
	err := row.Scan(append([]any{&link.ID, &link.User.ID, &link.User.Name, &link.Item, &link.Kind, &link.Title, &link.MaxViewers,
		&link.CreatedAt, &link.ExpiresAt, &link.RevokedAt, &link.Plays}, more...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return link, ErrNotFound
	}
	return link, err
}

// MayShare reports whether user may make share links now: the server
// allows them, the user may share, and is neither disabled nor ended.
func (s *Store) MayShare(user User) bool {
	return s.Settings().ShareLinks && user.CanShare && !user.Refused(s.now())
}

// CreateShareLink makes a link, and returns it with its token, which is
// given this once: only its hash is kept. The user must be allowed to
// share (see MayShare): ErrShareLinksOff otherwise. Whether they may see
// the title is for the caller to check.
func (s *Store) CreateShareLink(ctx context.Context, link NewShareLink) (ShareLink, string, error) {
	valid := false
	for _, expiry := range ShareLinkExpiries {
		valid = valid || link.Expiry == expiry
	}
	if !valid {
		return ShareLink{}, "", ErrInvalidShareExpiry
	}
	if link.MaxViewers < 1 || link.MaxViewers > MaxShareLinkViewers {
		return ShareLink{}, "", ErrInvalidShareViewers
	}
	if link.Kind != ShareMovie && link.Kind != ShareEpisode || link.Title == "" {
		return ShareLink{}, "", ErrInvalidShareTitle
	}
	if len(link.Title) > maxShareLinkTitle {
		link.Title = clipText(link.Title, maxShareLinkTitle)
	}
	secret, hash := newLinkSecret()
	var created ShareLink
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// The user is read, and kept, while the link is saved: a link is
		// never made by a user who just lost the right to.
		user, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR SHARE", link.User))
		if err != nil {
			return err
		}
		if !s.MayShare(user) {
			return ErrShareLinksOff
		}
		var id ID
		if err := tx.QueryRow(ctx, `INSERT INTO share_links (user_id, item_id, kind, title, secret_hash, max_viewers, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, link.User, link.Item, link.Kind, link.Title, hash[:], link.MaxViewers,
			s.now().Add(link.Expiry)).Scan(&id); err != nil {
			return err
		}
		created, err = scanShareLink(tx.QueryRow(ctx, "SELECT "+shareLinkColumns+shareLinkJoins+" WHERE l.id = $1", id))
		return err
	})
	if err != nil {
		return ShareLink{}, "", err
	}
	return created, linkToken(created.ID, secret), nil
}

// clipText shortens s to at most n bytes, without splitting a character.
func clipText(s string, n int) string {
	for n > 0 && n < len(s) && s[n]&0xc0 == 0x80 {
		n--
	}
	return s[:n]
}

// ShareLinks lists the links of user, or everyone's when user is nil, the
// newest first. Links that expired or were revoked more than 30 days ago
// are no longer kept (see SweepShareLinks).
func (s *Store) ShareLinks(ctx context.Context, user *ID) ([]ShareLink, error) {
	rows, err := s.db.Query(ctx, "SELECT "+shareLinkColumns+shareLinkJoins+" WHERE $1::uuid IS NULL OR l.user_id = $1 ORDER BY l.created_at DESC, l.id",
		user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ShareLink, error) { return scanShareLink(row) })
}

// ShareLink returns a link by its ID.
func (s *Store) ShareLink(ctx context.Context, id ID) (ShareLink, error) {
	return scanShareLink(s.db.QueryRow(ctx, "SELECT "+shareLinkColumns+shareLinkJoins+" WHERE l.id = $1", id))
}

// RevokeShareLink stops an active link from opening its title, and returns
// it with whether this revoked it: revoking it again, or once expired,
// changes nothing. owner, when set, is the only user whose link may be
// revoked: another's is ErrNotFound, as an unknown one.
func (s *Store) RevokeShareLink(ctx context.Context, id ID, owner *ID) (ShareLink, bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE share_links SET revoked_at = $3 WHERE id = $1 AND revoked_at IS NULL AND expires_at > $3
		AND ($2::uuid IS NULL OR user_id = $2)`, id, owner, s.now())
	if err != nil {
		return ShareLink{}, false, err
	}
	link, err := s.ShareLink(ctx, id)
	if err == nil && owner != nil && link.User.ID != *owner {
		return ShareLink{}, false, ErrNotFound
	}
	return link, tag.RowsAffected() > 0, err
}

// revokeShareLinks revokes the active links of users, taking them in the
// order of their keys.
func revokeShareLinks(ctx context.Context, tx pgx.Tx, users ...ID) error {
	if len(users) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE share_links SET revoked_at = now() WHERE id IN (
		SELECT id FROM share_links WHERE user_id = ANY($1) AND revoked_at IS NULL ORDER BY id FOR UPDATE)`, users)
	return err
}

// ShareLinkByToken returns the link of token while it opens its title,
// with its user as they are now. ErrShareLinkUnknown reports a token that
// is no link's; ErrShareLinkRevoked and ErrShareLinkExpired a link that
// no longer opens its title, and ErrShareLinksOff one the server or its
// user no longer allows, disabled or ended accounts included.
func (s *Store) ShareLinkByToken(ctx context.Context, token string) (ShareLink, User, error) {
	id, secret, ok := parseLinkToken(token)
	if !ok {
		return ShareLink{}, User{}, ErrShareLinkUnknown
	}
	var stored []byte
	link, err := scanShareLink(s.db.QueryRow(ctx, "SELECT "+shareLinkColumns+", l.secret_hash"+shareLinkJoins+" WHERE l.id = $1", id), &stored)
	if errors.Is(err, ErrNotFound) {
		return ShareLink{}, User{}, ErrShareLinkUnknown
	}
	if err != nil {
		return ShareLink{}, User{}, err
	}
	if !secretMatches(secret, stored) {
		return ShareLink{}, User{}, ErrShareLinkUnknown
	}
	switch link.State(s.now()) {
	case ShareLinkRevoked:
		return link, User{}, ErrShareLinkRevoked
	case ShareLinkExpired:
		return link, User{}, ErrShareLinkExpired
	}
	user, err := s.User(ctx, link.User.ID)
	if errors.Is(err, ErrNotFound) {
		return ShareLink{}, User{}, ErrShareLinkUnknown
	}
	if err != nil {
		return ShareLink{}, User{}, err
	}
	if !s.MayShare(user) {
		return link, user, ErrShareLinksOff
	}
	return link, user, nil
}

// CountSharePlay counts a guest's playback through a link.
func (s *Store) CountSharePlay(ctx context.Context, id ID) error {
	_, err := s.db.Exec(ctx, "UPDATE share_links SET plays = plays + 1 WHERE id = $1", id)
	return err
}

// ShareLinkToNotify reports whether share_played may tell of a playback
// through a link now, and if so notes it: at most once ShareLinkNotifyEvery.
func (s *Store) ShareLinkToNotify(ctx context.Context, id ID) (bool, error) {
	now := s.now()
	tag, err := s.db.Exec(ctx, "UPDATE share_links SET notified_at = $2 WHERE id = $1 AND (notified_at IS NULL OR notified_at <= $3)",
		id, now, now.Add(-ShareLinkNotifyEvery))
	return tag.RowsAffected() > 0, err
}

// ExpiredShareLinks returns the links that expired, unrevoked, and were not
// returned yet, once each, for the activity log, and deletes those that
// expired or were revoked more than 30 days ago.
func (s *Store) ExpiredShareLinks(ctx context.Context) ([]ShareLink, error) {
	now := s.now()
	if _, err := s.db.Exec(ctx, `DELETE FROM share_links WHERE id IN (SELECT id FROM share_links
		WHERE least(expires_at, coalesce(revoked_at, expires_at)) < $1 ORDER BY id FOR UPDATE)`, now.Add(-shareLinksKept)); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `WITH expired AS (
			UPDATE share_links SET expiry_recorded = true WHERE id IN (SELECT id FROM share_links
				WHERE expires_at <= $1 AND NOT expiry_recorded ORDER BY id FOR UPDATE) RETURNING *)
		SELECT `+shareLinkColumns+` FROM expired l JOIN users u ON u.id = l.user_id WHERE l.revoked_at IS NULL OR l.revoked_at > l.expires_at
		ORDER BY l.expires_at, l.id`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ShareLink, error) { return scanShareLink(row) })
}

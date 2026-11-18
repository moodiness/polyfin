package accounts

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrNoProviderAccount reports someone the identity provider signed
	// in to whom no account is linked, while none may be linked or
	// created for them.
	ErrNoProviderAccount = errors.New("no account for this identity")
	// ErrProviderLinked reports a link that cannot be made: the person is
	// linked to another account, or the account to another person.
	ErrProviderLinked = errors.New("the account is linked to someone else")
	// ErrProviderLinkRefused reports the permission to link at the next
	// sign-in asked for an administrator, or for an account with two-step
	// sign-in: those link themselves from their own session.
	ErrProviderLinkRefused = errors.New("this account links itself from its own session")
)

// ProviderIdentity is someone the identity provider signed in: the
// provider's issuer and the subject naming them there for good, the
// claim naming their Polyfin account and their verified email, either
// possibly empty, and whether they are in the provider's administrators'
// group.
type ProviderIdentity struct {
	Issuer        string
	Subject       string
	Name          string
	Email         string
	Administrator bool
}

// ProviderLinked tells whether the account is linked to someone at the
// provider of settings. A link made at another issuer, which a change of
// the provider left behind, signs nobody in and does not count.
func (u User) ProviderLinked(settings Settings) bool {
	return u.ProviderIssuer != "" && u.ProviderIssuer == settings.ProviderIssuer
}

// ProviderUser returns the account of identity, and whether it was
// created. It is the account linked to identity's issuer and subject,
// whatever its name and identity's claims say now.
//
// Someone not linked yet never gets an existing account on the word of
// their claims, which name it but do not prove whose it is. The account
// named as identity's name claim, else as its email, without regard to
// case, is linked to them only while an administrator allows it for that
// account (User.ProviderLinkAllowed), which the link turns off again, and
// only when the account is no administrator's, has no two-step sign-in
// and is not linked at the issuer already. Any other account of that name
// is ErrNoProviderAccount, as no account at all is: it links itself from
// its own session (see LinkProvider).
//
// When no account bears either name and create is set, a new account
// named after the name claim, or after the email when the claim is not a
// valid name, is created with the permissions of an account created in
// the admin app without a group, linked to identity, signing in through
// the provider only; otherwise ErrNoProviderAccount.
//
// identity.Administrator makes the account an administrator, unless it is
// disabled or has ended. Leaving the group does not make an administrator
// a member again: that is for the Users page.
func (s *Store) ProviderUser(ctx context.Context, identity ProviderIdentity, create bool) (User, bool, error) {
	var user User
	created := false
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// The administrators' lock serializes the provider's sign-ins as
		// the changes to administrators it may make.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		var err error
		user, err = scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE provider_issuer = $1 AND provider_subject = $2",
			identity.Issuer, identity.Subject))
		if errors.Is(err, ErrNotFound) {
			user, created, err = matchProviderUser(ctx, tx, identity, create)
		}
		if err != nil {
			return err
		}
		if identity.Administrator && !user.IsAdministrator && !user.Refused(s.now()) {
			user, _, err = applyUserChanges(ctx, tx, user.ID, UserChanges{IsAdministrator: new(true)}, userUpdate{}, signIn{})
		}
		return err
	})
	if err != nil {
		return User{}, false, err
	}
	s.forgetSignIns()
	return user, created, nil
}

// matchProviderUser finds the account identity names, by its name claim
// then its email, and links it to identity when it may be linked so (see
// ProviderUser); without an account of either name, it creates one when
// create is set.
func matchProviderUser(ctx context.Context, tx pgx.Tx, identity ProviderIdentity, create bool) (User, bool, error) {
	for _, name := range []string{identity.Name, identity.Email} {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		user, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE lower(name) = lower($1) FOR UPDATE", name))
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return User{}, false, err
		}
		// The permission is read as the account stands now: one that
		// became an administrator, or took two-step sign-in, since it
		// was given is not linked on it.
		if !user.ProviderLinkAllowed || user.IsAdministrator || user.TwoStep || user.ProviderIssuer == identity.Issuer {
			return User{}, false, ErrNoProviderAccount
		}
		user, err = linkProvider(ctx, tx, user.ID, identity)
		return user, false, err
	}
	if !create {
		return User{}, false, ErrNoProviderAccount
	}
	name := identity.Name
	if _, err := normalizeName(name); err != nil {
		name = identity.Email
	}
	// The account is a member's, as CreateUser makes it, with a password
	// nobody knows.
	user, err := createUser(ctx, tx, NewUser{Name: name, ProviderOnly: true})
	if err != nil {
		return User{}, false, err
	}
	user, err = linkProvider(ctx, tx, user.ID, identity)
	return user, err == nil, err
}

// linkProvider links the account id to identity, which uses up the
// permission to link it.
func linkProvider(ctx context.Context, tx pgx.Tx, id ID, identity ProviderIdentity) (User, error) {
	return scanUser(tx.QueryRow(ctx,
		"UPDATE users SET provider_issuer = $2, provider_subject = $3, provider_link_allowed = false WHERE id = $1 RETURNING "+userColumns,
		id, identity.Issuer, identity.Subject))
}

// LinkProvider links the account user to identity, for the admin session
// that asked for the link, named by its token's hash, and returns the
// account. The session stands for the account's own sign-in, its password
// and its code: it is ErrNotFound once that session ended, or the account
// is gone, disabled or ended.
//
// A person is linked to one account, and an account to one person at an
// issuer: identity linked to another account, and the account linked to
// someone else at identity's issuer, are ErrProviderLinked. A link is
// what says whose the account is at the provider, so a session does not
// replace it. A link to another issuer, which a change of the provider
// left behind, is replaced; the account already linked to identity is
// returned as it is.
//
// The link changes nothing else: nobody is signed in, and nobody is made
// an administrator, whatever identity.Administrator says.
func (s *Store) LinkProvider(ctx context.Context, user ID, session []byte, identity ProviderIdentity) (User, error) {
	var linked User
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// The administrators' lock serializes the links, as in
		// ProviderUser.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		var open bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM admin_sessions WHERE token_hash = $1 AND user_id = $2 AND expires_at > now())",
			session, user).Scan(&open); err != nil {
			return err
		}
		if !open {
			return ErrNotFound
		}
		current, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR UPDATE", user))
		if err != nil {
			return err
		}
		if current.Refused(s.now()) {
			return ErrNotFound
		}
		var holder ID
		err = tx.QueryRow(ctx, "SELECT id FROM users WHERE provider_issuer = $1 AND provider_subject = $2", identity.Issuer, identity.Subject).Scan(&holder)
		switch {
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		case err == nil && holder == user:
			linked = current
			return nil
		case err == nil, current.ProviderIssuer == identity.Issuer:
			return ErrProviderLinked
		}
		linked, err = linkProvider(ctx, tx, user, identity)
		return err
	})
	if err != nil {
		return User{}, err
	}
	s.forgetSignIns()
	return linked, nil
}

// allowProviderLink sets an administrator's permission to link user at
// the next sign-in through the provider of the person it is named after
// (see ProviderUser). It is not given for an administrator, nor for an
// account with two-step sign-in, whose own session alone links them:
// ErrProviderLinkRefused.
func allowProviderLink(ctx context.Context, tx pgx.Tx, user User, allowed bool) (User, error) {
	if allowed && (user.IsAdministrator || user.TwoStep) {
		return User{}, ErrProviderLinkRefused
	}
	return scanUser(tx.QueryRow(ctx, "UPDATE users SET provider_link_allowed = $2 WHERE id = $1 RETURNING "+userColumns, user.ID, allowed))
}

// unknowablePassword is a password nobody knows: 32 random bytes.
func unknowablePassword() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}

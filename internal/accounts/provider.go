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
	// in whom no account matches, while the provider creates none.
	ErrNoProviderAccount = errors.New("no account for this identity")
	// ErrProviderLinked reports an account matched by name or email that
	// is linked to another person at the provider.
	ErrProviderLinked = errors.New("the account is linked to someone else")
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

// ProviderUser returns the account of identity, and whether it was
// created. It is the account linked to identity's issuer and subject,
// else the account named as identity's name claim, else as its email,
// without regard to case, which is then linked to it; an account matched
// so but linked to someone else is ErrProviderLinked. Without either,
// when create is set, a new account named after the name claim, or after
// the email when the claim is not a valid name, is created with the
// permissions of an account created in the admin app without a group,
// linked to identity, signing in through the provider only; otherwise
// ErrNoProviderAccount.
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
// then its email, and links it to identity, or creates one when create
// is set.
func matchProviderUser(ctx context.Context, tx pgx.Tx, identity ProviderIdentity, create bool) (User, bool, error) {
	for _, name := range []string{identity.Name, identity.Email} {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var linked bool
		var user User
		err := tx.QueryRow(ctx, "SELECT provider_subject IS NOT NULL, "+userColumns+" FROM users WHERE lower(name) = lower($1) FOR UPDATE",
			name).Scan(append([]any{&linked}, user.fields()...)...)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return User{}, false, err
		}
		if linked {
			return User{}, false, ErrProviderLinked
		}
		user, err = scanUser(tx.QueryRow(ctx, "UPDATE users SET provider_issuer = $2, provider_subject = $3 WHERE id = $1 RETURNING "+userColumns,
			user.ID, identity.Issuer, identity.Subject))
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
	user, err = scanUser(tx.QueryRow(ctx, "UPDATE users SET provider_issuer = $2, provider_subject = $3 WHERE id = $1 RETURNING "+userColumns,
		user.ID, identity.Issuer, identity.Subject))
	return user, err == nil, err
}

// unknowablePassword is a password nobody knows: 32 random bytes.
func unknowablePassword() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}

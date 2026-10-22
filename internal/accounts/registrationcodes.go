package accounts

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaxRegistrationCodes bounds the usable registration codes kept at once.
const MaxRegistrationCodes = 500

// ErrRegistrationCodeUnknown reports a code that is no unused registration
// code's; ErrTooManyRegistrationCodes, one more than MaxRegistrationCodes.
var (
	ErrRegistrationCodeUnknown  = errors.New("unknown registration code")
	ErrTooManyRegistrationCodes = errors.New("too many registration codes")
)

// RegistrationCode is a code a visitor of /register types before
// registering, when Settings.RegistrationCodes asks for one. It creates
// up to MaxUses accounts, without limit when 0; Uses counts those created,
// UsedAt and UsedBy tell the last.
type RegistrationCode struct {
	Code      string
	CreatedAt time.Time
	UsedAt    *time.Time
	UsedBy    *string
	Uses      int
	MaxUses   int
}

// MaxRegistrationCodeUses bounds a code's uses, 0 being no limit.
const MaxRegistrationCodeUses = 10000

// ErrInvalidRegistrationCodeUses refuses a limit out of bounds.
var ErrInvalidRegistrationCodeUses = errors.New("invalid registration code uses")

// usable is the condition of a code that may create one more account.
const usableCode = "(max_uses = 0 OR uses < max_uses)"

// SetRegistrationCodeUses changes how many accounts a code may create.
func (s *Store) SetRegistrationCodeUses(ctx context.Context, code string, maxUses int) error {
	if maxUses < 0 || maxUses > MaxRegistrationCodeUses {
		return ErrInvalidRegistrationCodeUses
	}
	tag, err := s.db.Exec(ctx, "UPDATE registration_codes SET max_uses = $2 WHERE code = $1", NormalizeRegistrationCode(code), maxUses)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrRegistrationCodeUnknown
	}
	return err
}

// codeAlphabet leaves out the letters and digits read for one another.
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newRegistrationCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	out := make([]byte, 0, 9)
	for i, c := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, codeAlphabet[int(c)%len(codeAlphabet)])
	}
	return string(out)
}

// NormalizeRegistrationCode reads a code as typed: in any case, with or
// without its dash and spaces.
func NormalizeRegistrationCode(code string) string {
	var out []byte
	for _, c := range []byte(code) {
		switch {
		case 'a' <= c && c <= 'z':
			out = append(out, c-'a'+'A')
		case 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
			out = append(out, c)
		}
	}
	if len(out) == 8 {
		return string(out[:4]) + "-" + string(out[4:])
	}
	return string(out)
}

// CreateRegistrationCodes makes count new codes.
func (s *Store) CreateRegistrationCodes(ctx context.Context, count, maxUses int) ([]RegistrationCode, error) {
	if maxUses < 0 || maxUses > MaxRegistrationCodeUses {
		return nil, ErrInvalidRegistrationCodeUses
	}
	var created []RegistrationCode
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		created = nil
		var unused int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM registration_codes WHERE "+usableCode).Scan(&unused); err != nil {
			return err
		}
		if unused+count > MaxRegistrationCodes {
			return ErrTooManyRegistrationCodes
		}
		for range count {
			var code RegistrationCode
			err := tx.QueryRow(ctx, "INSERT INTO registration_codes (code, max_uses) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING code, created_at, max_uses",
				newRegistrationCode(), maxUses).Scan(&code.Code, &code.CreatedAt, &code.MaxUses)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			created = append(created, code)
		}
		return nil
	})
	return created, err
}

// RegistrationCodes lists the codes, newest first.
func (s *Store) RegistrationCodes(ctx context.Context) ([]RegistrationCode, error) {
	rows, err := s.db.Query(ctx, "SELECT c.code, c.created_at, c.used_at, u.name, c.uses, c.max_uses FROM registration_codes c "+
		"LEFT JOIN users u ON u.id = c.used_by ORDER BY c.created_at DESC, c.code")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (RegistrationCode, error) {
		var code RegistrationCode
		err := row.Scan(&code.Code, &code.CreatedAt, &code.UsedAt, &code.UsedBy, &code.Uses, &code.MaxUses)
		return code, err
	})
}

// DeleteRegistrationCode deletes a code, used or not.
func (s *Store) DeleteRegistrationCode(ctx context.Context, code string) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM registration_codes WHERE code = $1", NormalizeRegistrationCode(code))
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrRegistrationCodeUnknown
	}
	return err
}

// RegistrationCodeUsable reports whether code may create an account.
func (s *Store) RegistrationCodeUsable(ctx context.Context, code string) (bool, error) {
	var usable bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM registration_codes WHERE code = $1 AND "+usableCode+")",
		NormalizeRegistrationCode(code)).Scan(&usable)
	return usable, err
}

// ClaimRegistrationCode marks an unused code used, before its account is
// created; ReleaseRegistrationCode gives it back when that fails, and
// RegistrationCodeUsedBy names the account it created.
func (s *Store) ClaimRegistrationCode(ctx context.Context, code string) error {
	tag, err := s.db.Exec(ctx, "UPDATE registration_codes SET uses = uses + 1, used_at = now() WHERE code = $1 AND "+usableCode,
		NormalizeRegistrationCode(code))
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrRegistrationCodeUnknown
	}
	return err
}

func (s *Store) ReleaseRegistrationCode(ctx context.Context, code string) error {
	_, err := s.db.Exec(ctx, "UPDATE registration_codes SET uses = greatest(uses - 1, 0) WHERE code = $1",
		NormalizeRegistrationCode(code))
	return err
}

func (s *Store) RegistrationCodeUsedBy(ctx context.Context, code string, user ID) error {
	_, err := s.db.Exec(ctx, "UPDATE registration_codes SET used_by = $2 WHERE code = $1", NormalizeRegistrationCode(code), user)
	return err
}

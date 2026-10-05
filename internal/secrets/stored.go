package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The secrets Polyfin stores are the server's keys in the settings, named
// below as the admin API names them, and the tokens and API keys of the
// users' tracking connections. Nothing else is sealed: addon addresses,
// IPTV passwords and guide addresses are stored as they are.
var settingSecrets = []struct{ column, name string }{
	{"publicmetadb_key", "publicMetaDbKey"},
	{"theintrodb_key", "theIntroDbKey"},
	{"trakt_client_secret", "traktClientSecret"},
}

// Unreadable is a stored secret the key cannot open: a setting, by the
// admin API's name of it, or the tracking connection of a user to a
// service.
type Unreadable struct {
	Setting       string
	Service, User string
}

func (u Unreadable) String() string {
	if u.Setting != "" {
		return "setting " + u.Setting
	}
	return fmt.Sprintf("%s connection of %s", u.Service, u.User)
}

// Report is how the stored secrets stand.
type Report struct {
	// Plaintext counts the secrets stored unsealed.
	Plaintext int
	// Unreadable are the sealed secrets the key cannot open.
	Unreadable []Unreadable
}

// Prepare seals the secrets stored as plaintext, then logs once what the
// key cannot read, by name and never by value, or, without a key, that
// secrets are stored unencrypted. Polyfin calls it at startup.
func (b *Box) Prepare(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) error {
	sealed, err := b.SealStored(ctx, db)
	if err != nil {
		return err
	}
	if sealed > 0 {
		logger.Info("Stored secrets encrypted with POLYFIN_SECRET_KEY", "count", sealed)
	}
	report, err := b.Inspect(ctx, db)
	if err != nil {
		return err
	}
	if len(report.Unreadable) > 0 {
		names := make([]string, len(report.Unreadable))
		for i, unreadable := range report.Unreadable {
			names[i] = unreadable.String()
		}
		logger.Error("Stored secrets cannot be decrypted with POLYFIN_SECRET_KEY: they count as not set until the key they were "+
			"encrypted with is set again, or they are entered again", "secrets", strings.Join(names, ", "))
	}
	if !b.Enabled() && report.Plaintext > 0 {
		logger.Warn("Keys and tokens are stored unencrypted: set POLYFIN_SECRET_KEY to 32 random bytes in base64 "+
			"(openssl rand -base64 32) to encrypt them", "count", report.Plaintext)
	}
	return nil
}

// SealStored seals, with the key, the secrets stored as plaintext, in one
// transaction, and returns how many. Sealed ones are left alone, so that
// running it again changes nothing. Without a key, it does nothing.
func (b *Box) SealStored(ctx context.Context, db *pgxpool.Pool) (int, error) {
	if b == nil {
		return 0, nil
	}
	sealed := 0
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		sealed = 0
		values, err := settingValues(ctx, tx, " FOR UPDATE")
		if err != nil {
			return err
		}
		for i, value := range values {
			if value == "" || Sealed(value) {
				continue
			}
			if _, err := tx.Exec(ctx, "UPDATE settings SET "+settingSecrets[i].column+" = $1", b.Seal(value)); err != nil {
				return err
			}
			sealed++
		}

		type row struct {
			user           [16]byte
			service        string
			token, refresh string
		}
		rows, err := tx.Query(ctx, "SELECT user_id, service, token, refresh_token FROM tracking_connections FOR UPDATE")
		if err != nil {
			return err
		}
		connections, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
			var c row
			return c, r.Scan(&c.user, &c.service, &c.token, &c.refresh)
		})
		if err != nil {
			return err
		}
		for _, c := range connections {
			token, refresh := c.token, c.refresh
			for _, value := range []*string{&token, &refresh} {
				if *value != "" && !Sealed(*value) {
					*value = b.Seal(*value)
					sealed++
				}
			}
			if token == c.token && refresh == c.refresh {
				continue
			}
			if _, err := tx.Exec(ctx, "UPDATE tracking_connections SET token = $3, refresh_token = $4 WHERE user_id = $1 AND service = $2",
				c.user, c.service, token, refresh); err != nil {
				return err
			}
		}
		return nil
	})
	return sealed, err
}

// settingValues reads the secrets of the settings, in the order of
// settingSecrets; suffix ends the query.
func settingValues(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, suffix string) ([]string, error) {
	values := make([]string, len(settingSecrets))
	targets := make([]any, len(settingSecrets))
	columns := make([]string, len(settingSecrets))
	for i, secret := range settingSecrets {
		targets[i], columns[i] = &values[i], secret.column
	}
	err := db.QueryRow(ctx, "SELECT "+strings.Join(columns, ", ")+" FROM settings"+suffix).Scan(targets...)
	return values, err
}

// Inspect reports how the stored secrets stand with the key: how many are
// stored unsealed, and which sealed ones it cannot open.
func (b *Box) Inspect(ctx context.Context, db *pgxpool.Pool) (Report, error) {
	report := Report{Unreadable: []Unreadable{}}
	check := func(value string, unreadable Unreadable) {
		switch {
		case value == "":
		case !Sealed(value):
			report.Plaintext++
		default:
			if _, err := b.Open(value); err != nil {
				report.Unreadable = append(report.Unreadable, unreadable)
			}
		}
	}
	values, err := settingValues(ctx, db, "")
	if err != nil {
		return Report{}, err
	}
	for i, value := range values {
		check(value, Unreadable{Setting: settingSecrets[i].name})
	}
	rows, err := db.Query(ctx, `SELECT u.name, c.service, c.token, c.refresh_token
		FROM tracking_connections c JOIN users u ON u.id = c.user_id ORDER BY u.name, c.service`)
	if err != nil {
		return Report{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var user, service, token, refresh string
		if err := rows.Scan(&user, &service, &token, &refresh); err != nil {
			return Report{}, err
		}
		before := len(report.Unreadable)
		check(token, Unreadable{Service: service, User: user})
		if len(report.Unreadable) == before {
			check(refresh, Unreadable{Service: service, User: user})
		}
	}
	return report, rows.Err()
}

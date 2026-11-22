package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The secrets Polyfin stores are the server's keys and passwords in the
// settings, named below as the admin API names them, the tokens and API
// keys of the users' tracking connections, what the users' app connections
// sign in with (see appimport.SignIn), the addresses, tokens and keys
// of the notification targets, the passwords, secret keys, private keys
// and passphrases of the network shares, the
// secret keys, passwords and private keys of the backup destinations, the
// account key and private key of the certificate of Let's Encrypt, the
// secrets of the users' two-step sign-in, the addresses of the addons,
// IPTV sources and guides (see SealsAddress), which embed their keys and
// passwords, and the addresses the .strm files of the local folders name,
// which may too.
var settingSecrets = []struct{ column, name string }{
	{"publicmetadb_key", "publicMetaDbKey"},
	{"theintrodb_key", "theIntroDbKey"},
	{"mdblist_ratings_key", "mdblistRatingsKey"},
	{"tmdb_key", "tmdbKey"},
	{"trakt_client_secret", "traktClientSecret"},
	{"lastfm_secret", "lastFmSecret"},
	{"anilist_client_secret", "aniListClientSecret"},
	{"myanimelist_client_secret", "myAnimeListClientSecret"},
	{"smtp_password", "smtpPassword"},
	{"backup_passphrase", "backupPassphrase"},
	{"metrics_token", "metricsToken"},
	{"torrent_proxy_password", "torrentProxyPassword"},
	{"torrent_wireguard", "torrentWireGuard"},
	{"update_tool_token", "updateToolToken"},
	{"provider_client_secret", "providerClientSecret"},
}

// Unreadable is a stored secret the key cannot open: a setting, by the
// admin API's name of it, the tracking connection of a user to a service,
// the connection of a user to an app, a notification target, by its name,
// of a user or of the server (User empty), the password, secret key,
// private key or passphrase of a network share, by its folder's name, a backup destination, by what it shows
// (see DestinationName), the keys of the certificate of Let's Encrypt,
// the two-step sign-in of a user, or the address of an addon or an IPTV
// source, by its name, of a user or of the server (User empty), of one of
// its guides (Guide true), or the addresses a local folder's .strm files
// name, by the folder's name.
type Unreadable struct {
	Setting       string
	Service, User string
	App           string
	Target        string
	Folder        string
	Destination   string
	Certificate   bool
	TwoStep       bool
	Addon         string
	Guide         bool
}

func (u Unreadable) String() string {
	switch {
	case u.Setting != "":
		return "setting " + u.Setting
	case u.Certificate:
		return "keys of the certificate of Let's Encrypt"
	case u.TwoStep:
		return "two-step sign-in of " + u.User
	case u.Destination != "":
		return "backup destination " + u.Destination
	case u.Folder != "":
		return fmt.Sprintf("password or key of the share %s", u.Folder)
	case u.Addon != "":
		what := "address of " + u.Addon
		if u.Guide {
			what = "guide address of " + u.Addon
		}
		if u.User != "" {
			what += " of " + u.User
		}
		return what
	case u.Target != "" && u.User == "":
		return fmt.Sprintf("server notification target %s", u.Target)
	case u.Target != "":
		return fmt.Sprintf("notification target %s of %s", u.Target, u.User)
	case u.App != "":
		return fmt.Sprintf("%s app connection of %s", u.App, u.User)
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
		if err := sealAppConnections(ctx, b, tx, &sealed); err != nil {
			return err
		}
		if err := sealTargets(ctx, b, tx, &sealed); err != nil {
			return err
		}
		if err := sealShares(ctx, b, tx, &sealed); err != nil {
			return err
		}
		if err := sealDestinations(ctx, b, tx, &sealed); err != nil {
			return err
		}
		if err := sealCertificate(ctx, b, tx, &sealed); err != nil {
			return err
		}
		if err := sealTwoStep(ctx, b, tx, &sealed); err != nil {
			return err
		}
		return sealAddresses(ctx, b, tx, &sealed)
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
	if err := rows.Err(); err != nil {
		return Report{}, err
	}
	apps, err := db.Query(ctx, `SELECT u.name, c.app, c.sign_in FROM app_connections c JOIN users u ON u.id = c.user_id
		ORDER BY u.name, c.app`)
	if err != nil {
		return Report{}, err
	}
	defer apps.Close()
	for apps.Next() {
		var user, app, signIn string
		if err := apps.Scan(&user, &app, &signIn); err != nil {
			return Report{}, err
		}
		check(signIn, Unreadable{App: app, User: user})
	}
	if err := apps.Err(); err != nil {
		return Report{}, err
	}
	targets, err := db.Query(ctx, `SELECT coalesce(u.name, ''), t.name, t.secret
		FROM notification_targets t LEFT JOIN users u ON u.id = t.user_id ORDER BY u.name NULLS FIRST, t.created_at`)
	if err != nil {
		return Report{}, err
	}
	defer targets.Close()
	for targets.Next() {
		var user, name, secret string
		if err := targets.Scan(&user, &name, &secret); err != nil {
			return Report{}, err
		}
		check(secret, Unreadable{Target: name, User: user})
	}
	if err := targets.Err(); err != nil {
		return Report{}, err
	}
	shares, err := db.Query(ctx, `SELECT a.manifest->>'name', f.share_password, f.share_private_key, f.share_passphrase
		FROM local_folders f JOIN addons a ON a.id = f.addon_id ORDER BY a.manifest->>'name'`)
	if err != nil {
		return Report{}, err
	}
	defer shares.Close()
	for shares.Next() {
		var name string
		var secrets [3]string
		if err := shares.Scan(&name, &secrets[0], &secrets[1], &secrets[2]); err != nil {
			return Report{}, err
		}
		// A share counts once, whichever of its secrets the key cannot
		// open.
		before := len(report.Unreadable)
		for _, secret := range secrets {
			if len(report.Unreadable) == before {
				check(secret, Unreadable{Folder: name})
			}
		}
	}
	if err := shares.Err(); err != nil {
		return Report{}, err
	}
	destinations, err := db.Query(ctx, `SELECT kind, address, port, bucket, folder, username, secret, private_key
		FROM backup_destinations ORDER BY created_at`)
	if err != nil {
		return Report{}, err
	}
	defer destinations.Close()
	for destinations.Next() {
		var kind, address, bucket, folder, username, secret, privateKey string
		var port int
		if err := destinations.Scan(&kind, &address, &port, &bucket, &folder, &username, &secret, &privateKey); err != nil {
			return Report{}, err
		}
		unreadable := Unreadable{Destination: DestinationName(kind, address, port, bucket, folder, username)}
		before := len(report.Unreadable)
		check(secret, unreadable)
		if len(report.Unreadable) == before {
			check(privateKey, unreadable)
		}
	}
	if err := destinations.Err(); err != nil {
		return Report{}, err
	}
	var accountKey, privateKey string
	err = db.QueryRow(ctx, "SELECT account_key, private_key FROM acme_certificate").Scan(&accountKey, &privateKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Report{}, err
	}
	before := len(report.Unreadable)
	check(accountKey, Unreadable{Certificate: true})
	if len(report.Unreadable) == before {
		check(privateKey, Unreadable{Certificate: true})
	}
	twoSteps, err := db.Query(ctx, "SELECT name, two_step_secret FROM users WHERE two_step ORDER BY name")
	if err != nil {
		return Report{}, err
	}
	defer twoSteps.Close()
	for twoSteps.Next() {
		var user, secret string
		if err := twoSteps.Scan(&user, &secret); err != nil {
			return Report{}, err
		}
		check(secret, Unreadable{TwoStep: true, User: user})
	}
	if err := twoSteps.Err(); err != nil {
		return Report{}, err
	}
	if err := b.inspectAddresses(ctx, db, &report); err != nil {
		return Report{}, err
	}
	return report, nil
}

// inspectAddresses counts into report the addresses of the addons and
// IPTV sources (see SealsAddress), of the server's then of each user's,
// then those of their guides, then those the .strm files of the local
// folders name, stored unsealed, and names those the key cannot open: each
// addon's own, its guides' once, and a folder's .strm files' once.
func (b *Box) inspectAddresses(ctx context.Context, db *pgxpool.Pool, report *Report) error {
	inspect := func(address string, unreadable Unreadable) bool {
		if !Sealed(address) {
			if address != "" {
				report.Plaintext++
			}
			return false
		}
		if _, err := b.Open(address); err != nil {
			report.Unreadable = append(report.Unreadable, unreadable)
			return true
		}
		return false
	}
	rows, err := db.Query(ctx, `SELECT coalesce(u.name, ''), coalesce(a.name, a.manifest->>'name', ''), a.manifest_url
		FROM addons a LEFT JOIN users u ON u.id = a.owner_id
		WHERE a.kind NOT IN (`+plainAddressKinds+`) ORDER BY u.name NULLS FIRST, a.position, a.created_at`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var user, name, address string
		if err := rows.Scan(&user, &name, &address); err != nil {
			return err
		}
		inspect(address, Unreadable{Addon: name, User: user})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	guides, err := db.Query(ctx, `SELECT a.id, coalesce(u.name, ''), coalesce(a.name, a.manifest->>'name', ''), g.url
		FROM live_guides g JOIN addons a ON a.id = g.addon_id LEFT JOIN users u ON u.id = a.owner_id
		ORDER BY u.name NULLS FIRST, a.position, a.created_at, a.id, g.position`)
	if err != nil {
		return err
	}
	defer guides.Close()
	var named [16]byte
	for guides.Next() {
		var id [16]byte
		var user, name, address string
		if err := guides.Scan(&id, &user, &name, &address); err != nil {
			return err
		}
		if id == named && Sealed(address) {
			continue
		}
		if inspect(address, Unreadable{Addon: name, User: user, Guide: true}) {
			named = id
		}
	}
	if err := guides.Err(); err != nil {
		return err
	}
	streams, err := db.Query(ctx, `SELECT f.addon_id, coalesce(a.name, a.manifest->>'name', ''), f.stream_url
		FROM local_files f JOIN addons a ON a.id = f.addon_id WHERE f.stream_url IS NOT NULL
		ORDER BY a.position, a.created_at, a.id, f.path`)
	if err != nil {
		return err
	}
	defer streams.Close()
	named = [16]byte{}
	for streams.Next() {
		var id [16]byte
		var name, address string
		if err := streams.Scan(&id, &name, &address); err != nil {
			return err
		}
		if id == named && Sealed(address) {
			continue
		}
		if inspect(address, Unreadable{Addon: name}) {
			named = id
		}
	}
	return streams.Err()
}

// DestinationName is what a backup destination shows: an S3 bucket and its
// prefix at its endpoint's host, an SFTP server's user, host, port when
// not 22, and folder, or a WebDAV folder's address.
func DestinationName(kind, address string, port int, bucket, folder, username string) string {
	switch kind {
	case "s3":
		name := bucket
		if folder != "" {
			name += "/" + folder
		}
		host := address
		if u, err := url.Parse(address); err == nil && u.Host != "" {
			host = u.Host
		}
		return name + " (" + host + ")"
	case "sftp":
		name := address
		if username != "" {
			name = username + "@" + name
		}
		if port != 22 && port != 0 {
			name += ":" + strconv.Itoa(port)
		}
		if folder != "" {
			name += ":" + folder
		}
		return name
	}
	return address
}

// The kinds of stored secrets, as a backup about to be restored counts
// those the key cannot open.
const (
	KindSetting     = "settings"
	KindTracking    = "tracking"
	KindApp         = "apps"
	KindTarget      = "notifications"
	KindShare       = "shares"
	KindDestination = "destinations"
	KindCertificate = "certificates"
	KindTwoStep     = "twoStep"
	KindAddress     = "addresses"
)

// sealedTables are the columns of the stored secrets, by table, and the
// kind of secret they hold.
var sealedTables = map[string]struct {
	kind    string
	columns []string
}{
	"settings":             {KindSetting, settingColumns()},
	"tracking_connections": {KindTracking, []string{"token", "refresh_token"}},
	"app_connections":      {KindApp, []string{"sign_in"}},
	"notification_targets": {KindTarget, []string{"secret"}},
	"local_folders":        {KindShare, []string{"share_password", "share_private_key", "share_passphrase"}},
	"backup_destinations":  {KindDestination, []string{"secret", "private_key"}},
	"acme_certificate":     {KindCertificate, []string{"account_key", "private_key"}},
	"users":                {KindTwoStep, []string{"two_step_secret"}},
	"addons":               {KindAddress, []string{"manifest_url"}},
	"live_guides":          {KindAddress, []string{"url"}},
	"local_files":          {KindAddress, []string{"stream_url"}},
}

// plainAddressKinds are the kinds of addons whose address is stored as it
// is, in SQL: a local folder's path and an HDHomeRun tuner's address,
// which hold no credentials, and which queries compare (see SealsAddress).
const plainAddressKinds = "'local', 'hdhomerun'"

// SealsAddress reports whether the address of an addon of kind is sealed
// with the key: a Stremio or Eclipse addon's manifest address, an M3U
// list's or an Xtream account's, which embed their keys and passwords;
// not a local folder's path, nor a tuner's address.
func SealsAddress(kind string) bool { return kind != "local" && kind != "hdhomerun" }

func settingColumns() []string {
	columns := make([]string, len(settingSecrets))
	for i, secret := range settingSecrets {
		columns[i] = secret.column
	}
	return columns
}

// SealedTables names the tables that hold stored secrets.
func SealedTables() []string {
	return slices.Sorted(maps.Keys(sealedTables))
}

// UnreadableIn counts the secrets the key cannot open in a row of table,
// its values by column, as Inspect does: each of the settings, and once
// for a row of the other tables. It returns their kind, empty for a table
// that holds none.
func (b *Box) UnreadableIn(table string, row map[string]string) (kind string, count int) {
	sealed, ok := sealedTables[table]
	if !ok {
		return "", 0
	}
	// A secret being set up signs in no one.
	if table == "users" && row["two_step"] != "t" && row["two_step"] != "true" {
		return sealed.kind, 0
	}
	for _, column := range sealed.columns {
		value := row[column]
		if !Sealed(value) {
			continue
		}
		if _, err := b.Open(value); err != nil {
			count++
			if table != "settings" {
				break
			}
		}
	}
	return sealed.kind, count
}

// sealAppConnections seals what the app connections stored as plaintext
// sign in with, counting them into sealed.
func sealAppConnections(ctx context.Context, b *Box, tx pgx.Tx, sealed *int) error {
	rows, err := tx.Query(ctx, "SELECT user_id, app, sign_in FROM app_connections WHERE sign_in NOT LIKE $1 FOR UPDATE", prefix+"%")
	if err != nil {
		return err
	}
	type row struct {
		user        [16]byte
		app, signIn string
	}
	plain, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var c row
		return c, r.Scan(&c.user, &c.app, &c.signIn)
	})
	if err != nil {
		return err
	}
	for _, c := range plain {
		if _, err := tx.Exec(ctx, "UPDATE app_connections SET sign_in = $3 WHERE user_id = $1 AND app = $2", c.user, c.app,
			b.Seal(c.signIn)); err != nil {
			return err
		}
		*sealed++
	}
	return nil
}

// sealTargets seals the addresses, access tokens and keys of the
// notification targets stored as plaintext, counting them into sealed.
func sealTargets(ctx context.Context, b *Box, tx pgx.Tx, sealed *int) error {
	rows, err := tx.Query(ctx, "SELECT id, secret FROM notification_targets WHERE secret <> '' AND secret NOT LIKE $1 FOR UPDATE", prefix+"%")
	if err != nil {
		return err
	}
	type row struct {
		id     [16]byte
		secret string
	}
	plain, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var t row
		return t, r.Scan(&t.id, &t.secret)
	})
	if err != nil {
		return err
	}
	for _, t := range plain {
		if _, err := tx.Exec(ctx, "UPDATE notification_targets SET secret = $2 WHERE id = $1", t.id, b.Seal(t.secret)); err != nil {
			return err
		}
		*sealed++
	}
	return nil
}

// sealShares seals the passwords, private keys and passphrases of the
// network shares stored as plaintext, counting them into sealed.
func sealShares(ctx context.Context, b *Box, tx pgx.Tx, sealed *int) error {
	for _, column := range []string{"share_password", "share_private_key", "share_passphrase"} {
		rows, err := tx.Query(ctx, "SELECT addon_id, "+column+" FROM local_folders WHERE "+column+" <> '' AND "+column+
			" NOT LIKE $1 ORDER BY addon_id FOR UPDATE", prefix+"%")
		if err != nil {
			return err
		}
		type row struct {
			id     [16]byte
			secret string
		}
		plain, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
			var f row
			return f, r.Scan(&f.id, &f.secret)
		})
		if err != nil {
			return err
		}
		for _, f := range plain {
			if _, err := tx.Exec(ctx, "UPDATE local_folders SET "+column+" = $2 WHERE addon_id = $1", f.id, b.Seal(f.secret)); err != nil {
				return err
			}
			*sealed++
		}
	}
	return nil
}

// sealDestinations seals the secret keys, passwords and private keys of
// the backup destinations stored as plaintext, counting them into sealed.
func sealDestinations(ctx context.Context, b *Box, tx pgx.Tx, sealed *int) error {
	rows, err := tx.Query(ctx, `SELECT id, secret, private_key FROM backup_destinations
		WHERE (secret <> '' AND secret NOT LIKE $1) OR (private_key <> '' AND private_key NOT LIKE $1) FOR UPDATE`, prefix+"%")
	if err != nil {
		return err
	}
	type row struct {
		id                 [16]byte
		secret, privateKey string
	}
	plain, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var d row
		return d, r.Scan(&d.id, &d.secret, &d.privateKey)
	})
	if err != nil {
		return err
	}
	for _, d := range plain {
		for _, value := range []*string{&d.secret, &d.privateKey} {
			if *value != "" && !Sealed(*value) {
				*value = b.Seal(*value)
				*sealed++
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE backup_destinations SET secret = $2, private_key = $3 WHERE id = $1", d.id, d.secret,
			d.privateKey); err != nil {
			return err
		}
	}
	return nil
}

// sealCertificate seals the account key and private key of the
// certificate of Let's Encrypt stored as plaintext, counting them into
// sealed.
func sealCertificate(ctx context.Context, b *Box, tx pgx.Tx, sealed *int) error {
	var accountKey, privateKey string
	err := tx.QueryRow(ctx, "SELECT account_key, private_key FROM acme_certificate FOR UPDATE").Scan(&accountKey, &privateKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	changed := false
	for _, value := range []*string{&accountKey, &privateKey} {
		if *value != "" && !Sealed(*value) {
			*value = b.Seal(*value)
			*sealed++
			changed = true
		}
	}
	if !changed {
		return nil
	}
	_, err = tx.Exec(ctx, "UPDATE acme_certificate SET account_key = $1, private_key = $2", accountKey, privateKey)
	return err
}

// sealTwoStep seals the users' two-step sign-in secrets stored as
// plaintext, counting them into sealed. The users are taken in the order
// of their keys.
func sealTwoStep(ctx context.Context, b *Box, tx pgx.Tx, sealed *int) error {
	rows, err := tx.Query(ctx, "SELECT id, two_step_secret FROM users WHERE two_step_secret <> '' AND two_step_secret NOT LIKE $1 ORDER BY id FOR UPDATE",
		prefix+"%")
	if err != nil {
		return err
	}
	type plain struct {
		id     [16]byte
		secret string
	}
	secrets, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (plain, error) {
		var p plain
		return p, r.Scan(&p.id, &p.secret)
	})
	if err != nil {
		return err
	}
	for _, p := range secrets {
		if _, err := tx.Exec(ctx, "UPDATE users SET two_step_secret = $2 WHERE id = $1", p.id, b.Seal(p.secret)); err != nil {
			return err
		}
		*sealed++
	}
	return nil
}

// sealAddresses seals the addresses of the addons and IPTV sources (see
// SealsAddress), of the guides, and those the .strm files of the local
// folders name, stored as plaintext, counting them into sealed. An
// addon's address takes its fingerprint (see Fingerprint), which tells it
// installed twice in its scope, unless an addon of the scope sealed
// earlier with the key holds it already, as when the address was
// installed again while the key was not set: both stay, as they were. No
// query looks a .strm file up by its address: it takes no fingerprint.
func sealAddresses(ctx context.Context, b *Box, tx pgx.Tx, sealed *int) error {
	type plain struct {
		id      [16]byte
		address string
	}
	scan := func(r pgx.CollectableRow) (plain, error) {
		var p plain
		return p, r.Scan(&p.id, &p.address)
	}
	rows, err := tx.Query(ctx, `SELECT id, manifest_url FROM addons WHERE kind NOT IN (`+plainAddressKinds+`) AND manifest_url NOT LIKE $1
		ORDER BY id FOR UPDATE`, prefix+"%")
	if err != nil {
		return err
	}
	addresses, err := pgx.CollectRows(rows, scan)
	if err != nil {
		return err
	}
	for _, p := range addresses {
		if _, err := tx.Exec(ctx, `UPDATE addons a SET manifest_url = $2, address_key = CASE WHEN EXISTS (SELECT 1 FROM addons o
			WHERE o.owner_id IS NOT DISTINCT FROM a.owner_id AND o.address_key = $3) THEN NULL ELSE $3 END WHERE id = $1`,
			p.id, b.Seal(p.address), b.Fingerprint(p.address)); err != nil {
			return err
		}
		*sealed++
	}
	rows, err = tx.Query(ctx, "SELECT id, url FROM live_guides WHERE url NOT LIKE $1 ORDER BY id FOR UPDATE", prefix+"%")
	if err != nil {
		return err
	}
	guides, err := pgx.CollectRows(rows, scan)
	if err != nil {
		return err
	}
	for _, p := range guides {
		if _, err := tx.Exec(ctx, "UPDATE live_guides SET url = $2 WHERE id = $1", p.id, b.Seal(p.address)); err != nil {
			return err
		}
		*sealed++
	}
	rows, err = tx.Query(ctx, "SELECT id, stream_url FROM local_files WHERE stream_url NOT LIKE $1 ORDER BY id FOR UPDATE", prefix+"%")
	if err != nil {
		return err
	}
	streams, err := pgx.CollectRows(rows, scan)
	if err != nil {
		return err
	}
	for _, p := range streams {
		if _, err := tx.Exec(ctx, "UPDATE local_files SET stream_url = $2 WHERE id = $1", p.id, b.Seal(p.address)); err != nil {
			return err
		}
		*sealed++
	}
	return nil
}

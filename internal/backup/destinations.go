package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/ssh"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
)

// The kinds of destinations backups are sent to besides the folder.
const (
	KindS3     = "s3"
	KindSFTP   = "sftp"
	KindWebDAV = "webdav"
)

// Kinds are the kinds of destinations, in the order they are offered.
var Kinds = []string{KindS3, KindSFTP, KindWebDAV}

// MaxDestinations bounds the destinations.
const MaxDestinations = 10

// The bounds of what a destination is given, in bytes.
const (
	maxAddress    = 2048
	maxFolder     = 512
	maxUsername   = 256
	maxSecret     = 4096
	maxPrivateKey = 16384
)

// The errors of a destination given or asked for.
var (
	ErrDestinationNotFound       = errors.New("no such backup destination")
	ErrTooManyDestinations       = errors.New("too many backup destinations")
	ErrInvalidDestinationKind    = errors.New("invalid backup destination kind")
	ErrInvalidDestinationAddress = errors.New("invalid backup destination address")
	ErrInvalidBucket             = errors.New("invalid bucket")
	ErrInvalidDestinationFolder  = errors.New("invalid backup destination folder")
	ErrInvalidRegion             = errors.New("invalid region")
	ErrInvalidPort               = errors.New("invalid port")
	ErrInvalidUsername           = errors.New("invalid user")
	ErrInvalidSecret             = errors.New("invalid password or secret key")
	ErrInvalidPrivateKey         = errors.New("invalid private key: an OpenSSH or PEM private key without a passphrase")
	ErrInvalidDestinationKept    = errors.New("invalid number of backups kept")
	// ErrDestinationUnreadable reports a destination whose password,
	// secret key or private key POLYFIN_SECRET_KEY cannot decrypt.
	ErrDestinationUnreadable = errors.New("the destination's password or key cannot be decrypted with POLYFIN_SECRET_KEY: enter it again")
)

// bucketName is what S3-compatible storages accept as a bucket's name.
var bucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$`)

// regionName is an S3 region, such as eu-west-3, or auto.
var regionName = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// hostName is an SFTP server's host name or address.
var hostName = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,253}$`)

// Destination is a place backups are sent to, as the admin API shows it:
// its password, secret key and private key are never part of it.
type Destination struct {
	ID   accounts.ID
	Kind string
	// Address is an S3 endpoint, such as https://s3.eu-west-3.amazonaws.com,
	// an SFTP server's host, or a WebDAV folder's address.
	Address string
	// Port is an SFTP server's port, 0 for the other kinds.
	Port int
	// Bucket is an S3 bucket; Folder an S3 prefix or an SFTP folder, empty
	// for the bucket's root or the user's home; Region an S3 region, empty
	// for the default one.
	Bucket, Folder, Region string
	// Username is an S3 access key or the user signing in.
	Username string
	// SecretSet tells whether an S3 secret key or a password is set,
	// PrivateKeySet whether an SFTP private key is.
	SecretSet, PrivateKeySet bool
	// HostKey is the SFTP server's key saved at the first connection, in
	// the authorized_keys format, empty before it.
	HostKey string
	// Kept is how many backups are kept there.
	Kept      int
	CreatedAt time.Time
	// RanAt is when the last upload started, nil before the first; Error
	// why it failed, empty after a success. NewestFile is the newest backup
	// there after it, NewestAt when that backup was made.
	RanAt      *time.Time
	Error      string
	NewestFile string
	NewestAt   *time.Time
	// Unreadable tells that POLYFIN_SECRET_KEY cannot decrypt its password
	// or keys: nothing can be sent until they are entered again.
	Unreadable bool
}

// Name is what the destination shows: see secrets.DestinationName.
func (d Destination) Name() string {
	return secrets.DestinationName(d.Kind, d.Address, d.Port, d.Bucket, d.Folder, d.Username)
}

// The problems of a destination.
const (
	// ProblemFailed: its last upload failed.
	ProblemFailed = "failed"
	// ProblemStale: its newest backup is older than StaleAfter.
	ProblemStale = "stale"
)

// Problem is what an administrator should look at, at now: its last
// upload failed, or its newest backup is older than StaleAfter; empty
// before the first upload, and when all is well.
func (d Destination) Problem(now time.Time) string {
	switch {
	case d.RanAt == nil:
		return ""
	case d.Error != "":
		return ProblemFailed
	case d.NewestAt == nil || now.Sub(*d.NewestAt) > StaleAfter:
		return ProblemStale
	}
	return ""
}

// HostKeyFingerprint is the SHA-256 fingerprint of HostKey, as ssh-keygen
// shows it, empty without one.
func (d Destination) HostKeyFingerprint() string {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(d.HostKey))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(key)
}

// Draft is what an administrator gives to add a destination or change one.
// On a change, nil fields keep their values and Kind is ignored. HostKey
// may only be emptied, to accept the key the server offers at the next
// connection; changing an SFTP server's address or port empties it too.
type Draft struct {
	Kind                   string
	Address                *string
	Port                   *int
	Bucket, Folder, Region *string
	Username               *string
	Secret, PrivateKey     *string
	HostKey                *string
	Kept                   *int
}

// stored is a destination as the database holds it, its secrets sealed or
// not.
type stored struct {
	Destination
	secret, privateKey string
}

const destinationColumns = `id, kind, address, port, bucket, folder, region, username, secret, private_key, host_key, kept, created_at,
	ran_at, error, newest_file, newest_at`

func scanDestination(row pgx.Row) (stored, error) {
	var d stored
	err := row.Scan(&d.ID, &d.Kind, &d.Address, &d.Port, &d.Bucket, &d.Folder, &d.Region, &d.Username, &d.secret, &d.privateKey,
		&d.HostKey, &d.Kept, &d.CreatedAt, &d.RanAt, &d.Error, &d.NewestFile, &d.NewestAt)
	return d, err
}

// describe fills what the admin API shows of d.
func (s *Service) describe(d stored) Destination {
	result := d.Destination
	result.SecretSet, result.PrivateKeySet = d.secret != "", d.privateKey != ""
	for _, value := range []string{d.secret, d.privateKey} {
		if _, err := s.cfg.Secrets.Open(value); err != nil {
			result.Unreadable = true
		}
	}
	return result
}

func (s *Service) loadDestinations(ctx context.Context) ([]stored, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, nil
	}
	rows, err := s.cfg.DB.Query(ctx, "SELECT "+destinationColumns+" FROM backup_destinations ORDER BY created_at, id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (stored, error) { return scanDestination(row) })
}

func (s *Service) loadDestination(ctx context.Context, id accounts.ID) (stored, error) {
	if s == nil || s.cfg.DB == nil {
		return stored{}, ErrDestinationNotFound
	}
	d, err := scanDestination(s.cfg.DB.QueryRow(ctx, "SELECT "+destinationColumns+" FROM backup_destinations WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, ErrDestinationNotFound
	}
	return d, err
}

// Destinations lists the destinations, oldest first.
func (s *Service) Destinations(ctx context.Context) ([]Destination, error) {
	all, err := s.loadDestinations(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Destination, len(all))
	for i, d := range all {
		result[i] = s.describe(d)
	}
	return result, nil
}

// Destination returns the destination id names.
func (s *Service) Destination(ctx context.Context, id accounts.ID) (Destination, error) {
	d, err := s.loadDestination(ctx, id)
	if err != nil {
		return Destination{}, err
	}
	return s.describe(d), nil
}

// CreateDestination adds a destination, keeping DefaultBackupsKept
// backups there unless the draft says.
func (s *Service) CreateDestination(ctx context.Context, draft Draft) (Destination, error) {
	if !slices.Contains(Kinds, draft.Kind) {
		return Destination{}, ErrInvalidDestinationKind
	}
	d := stored{Destination: Destination{Kind: draft.Kind, Kept: accounts.DefaultBackupsKept}}
	if draft.Kind == KindSFTP {
		d.Port = 22
	}
	if err := s.apply(&d, draft); err != nil {
		return Destination{}, err
	}
	var count int
	if err := s.cfg.DB.QueryRow(ctx, "SELECT count(*) FROM backup_destinations").Scan(&count); err != nil {
		return Destination{}, err
	}
	if count >= MaxDestinations {
		return Destination{}, ErrTooManyDestinations
	}
	saved, err := scanDestination(s.cfg.DB.QueryRow(ctx, `INSERT INTO backup_destinations
		(kind, address, port, bucket, folder, region, username, secret, private_key, host_key, kept)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING `+destinationColumns,
		d.Kind, d.Address, d.Port, d.Bucket, d.Folder, d.Region, d.Username, d.secret, d.privateKey, d.HostKey, d.Kept))
	if err != nil {
		return Destination{}, err
	}
	s.cfg.Logger.Info("A backup destination was added", "destination", saved.Name())
	return s.describe(saved), nil
}

// UpdateDestination changes the destination id names.
func (s *Service) UpdateDestination(ctx context.Context, id accounts.ID, draft Draft) (Destination, error) {
	d, err := s.loadDestination(ctx, id)
	if err != nil {
		return Destination{}, err
	}
	if err := s.apply(&d, draft); err != nil {
		return Destination{}, err
	}
	saved, err := scanDestination(s.cfg.DB.QueryRow(ctx, `UPDATE backup_destinations SET address = $2, port = $3, bucket = $4, folder = $5,
		region = $6, username = $7, secret = $8, private_key = $9, host_key = $10, kept = $11 WHERE id = $1 RETURNING `+destinationColumns,
		id, d.Address, d.Port, d.Bucket, d.Folder, d.Region, d.Username, d.secret, d.privateKey, d.HostKey, d.Kept))
	if errors.Is(err, pgx.ErrNoRows) {
		return Destination{}, ErrDestinationNotFound
	}
	if err != nil {
		return Destination{}, err
	}
	return s.describe(saved), nil
}

// DeleteDestination removes the destination id names; the backups sent
// there stay.
func (s *Service) DeleteDestination(ctx context.Context, id accounts.ID) error {
	if s == nil || s.cfg.DB == nil {
		return ErrDestinationNotFound
	}
	tag, err := s.cfg.DB.Exec(ctx, "DELETE FROM backup_destinations WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDestinationNotFound
	}
	return nil
}

// apply checks draft and puts it into d.
func (s *Service) apply(d *stored, draft Draft) error {
	trimmed := func(value *string, current string) string {
		if value == nil {
			return current
		}
		return strings.TrimSpace(*value)
	}
	address := trimmed(draft.Address, d.Address)
	port := d.Port
	if draft.Port != nil {
		port = *draft.Port
	}
	if d.Kind == KindSFTP && (address != d.Address || port != d.Port) {
		// Another server: its key is saved at the next connection.
		d.HostKey = ""
	}
	if draft.HostKey != nil {
		if *draft.HostKey != "" {
			return ErrInvalidDestinationAddress
		}
		d.HostKey = ""
	}
	d.Address, d.Port = address, port
	d.Bucket = trimmed(draft.Bucket, d.Bucket)
	d.Folder = trimmed(draft.Folder, d.Folder)
	d.Region = trimmed(draft.Region, d.Region)
	d.Username = trimmed(draft.Username, d.Username)
	if draft.Kept != nil {
		d.Kept = *draft.Kept
	}
	if draft.Secret != nil {
		if len(*draft.Secret) > maxSecret || strings.ContainsAny(*draft.Secret, "\x00\r\n") {
			return ErrInvalidSecret
		}
		d.secret = s.cfg.Secrets.Seal(*draft.Secret)
	}
	if draft.PrivateKey != nil {
		key := strings.TrimSpace(*draft.PrivateKey)
		if key != "" {
			if len(key) > maxPrivateKey {
				return ErrInvalidPrivateKey
			}
			if _, err := ssh.ParsePrivateKey([]byte(key)); err != nil {
				return ErrInvalidPrivateKey
			}
			key += "\n"
		}
		d.privateKey = s.cfg.Secrets.Seal(key)
	}
	if d.Kept < accounts.MinBackupsKept || d.Kept > accounts.MaxBackupsKept {
		return ErrInvalidDestinationKept
	}
	if len(d.Username) > maxUsername || strings.ContainsFunc(d.Username, unicode.IsControl) {
		return ErrInvalidUsername
	}
	switch d.Kind {
	case KindS3:
		if !webAddress(d.Address, true) {
			return ErrInvalidDestinationAddress
		}
		d.Address = strings.TrimSuffix(d.Address, "/")
		if !bucketName.MatchString(d.Bucket) {
			return ErrInvalidBucket
		}
		folder, ok := cleanPrefix(d.Folder)
		if !ok {
			return ErrInvalidDestinationFolder
		}
		d.Folder = folder
		if d.Region != "" && !regionName.MatchString(d.Region) {
			return ErrInvalidRegion
		}
		if d.Username == "" {
			return ErrInvalidUsername
		}
		if d.secret == "" {
			return ErrInvalidSecret
		}
		d.Port, d.privateKey, d.HostKey = 0, "", ""
	case KindSFTP:
		host := strings.TrimSuffix(strings.TrimPrefix(d.Address, "["), "]")
		if !hostName.MatchString(host) {
			return ErrInvalidDestinationAddress
		}
		d.Address = host
		if d.Port < 1 || d.Port > 65535 {
			return ErrInvalidPort
		}
		folder := strings.TrimSuffix(d.Folder, "/")
		if d.Folder == "/" {
			folder = "/"
		}
		if len(folder) > maxFolder || strings.ContainsFunc(folder, unicode.IsControl) {
			return ErrInvalidDestinationFolder
		}
		d.Folder = folder
		if d.Username == "" {
			return ErrInvalidUsername
		}
		if d.secret == "" && d.privateKey == "" {
			return ErrInvalidSecret
		}
		d.Bucket, d.Region = "", ""
	case KindWebDAV:
		if !webAddress(d.Address, false) {
			return ErrInvalidDestinationAddress
		}
		d.Port, d.Bucket, d.Folder, d.Region, d.privateKey, d.HostKey = 0, "", "", "", "", ""
	}
	return nil
}

// webAddress reports whether address is an http or https address without
// user, query or fragment, and, for an S3 endpoint, no path either.
func webAddress(address string, endpoint bool) bool {
	u, err := url.Parse(address)
	if err != nil || len(address) > maxAddress || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || strings.ContainsFunc(address, unicode.IsSpace) {
		return false
	}
	return !endpoint || u.Path == "" || u.Path == "/"
}

// cleanPrefix returns an S3 prefix without slashes around it, and
// whether it is one: segments neither empty, "." nor "..", no control
// characters, at most maxFolder bytes.
func cleanPrefix(prefix string) (string, bool) {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return "", true
	}
	if len(prefix) > maxFolder || strings.ContainsFunc(prefix, unicode.IsControl) {
		return "", false
	}
	for segment := range strings.SplitSeq(prefix, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
	}
	return prefix, true
}

// open connects to d's storage.
func (s *Service) open(ctx context.Context, d stored) (remote, error) {
	secret, err := s.cfg.Secrets.Open(d.secret)
	if err != nil {
		return nil, ErrDestinationUnreadable
	}
	privateKey, err := s.cfg.Secrets.Open(d.privateKey)
	if err != nil {
		return nil, ErrDestinationUnreadable
	}
	switch d.Kind {
	case KindS3:
		return newS3Remote(d.Destination, secret, s.idle)
	case KindWebDAV:
		return newWebDAVRemote(d.Destination, secret, s.idle)
	}
	return dialSFTP(ctx, d.Destination, secret, privateKey, s.idle, func(hostKey string) {
		// Saved once: a key saved meanwhile is kept.
		if _, err := s.cfg.DB.Exec(context.WithoutCancel(ctx), "UPDATE backup_destinations SET host_key = $2 WHERE id = $1 AND host_key = ''",
			d.ID, hostKey); err != nil {
			s.cfg.Logger.Warn("Could not save an SFTP server's host key", "destination", d.Name(), "error", err)
		}
	})
}

// testSize is the size of the file Test writes.
const testSize = 4096

// TestDestination writes a small file to the destination id names,
// reads it back and deletes it. An SFTP server's host key is saved at its
// first connection. It returns the destination as it stands after, and
// why the test failed.
func (s *Service) TestDestination(ctx context.Context, id accounts.ID) (Destination, error) {
	d, err := s.loadDestination(ctx, id)
	if err != nil {
		return Destination{}, err
	}
	failure := s.test(ctx, d)
	if failure != nil && ctx.Err() == nil {
		s.cfg.Logger.Warn("A backup destination's test failed", "destination", d.Name(), "error", failure)
	}
	after, err := s.Destination(ctx, id)
	if err != nil {
		return Destination{}, err
	}
	return after, failure
}

func (s *Service) test(ctx context.Context, d stored) error {
	r, err := s.open(ctx, d)
	if err != nil {
		return err
	}
	defer r.close()
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	name := ".polyfin-test-" + hex.EncodeToString(random)
	data := make([]byte, testSize)
	_, _ = rand.Read(data)
	if err := r.put(ctx, name, bytes.NewReader(data), int64(len(data))); err != nil {
		return err
	}
	defer func() { _ = r.remove(context.WithoutCancel(ctx), name) }()
	// The file read back is the size written: a server sending more fails
	// the test rather than filling the memory.
	var read bytes.Buffer
	if err := r.get(ctx, name, &cappedWriter{w: &read, max: int64(len(data))}); err != nil {
		return fmt.Errorf("read back the test file: %w", err)
	}
	if !bytes.Equal(read.Bytes(), data) {
		return errors.New("the file read back differs from the one written")
	}
	if err := r.remove(ctx, name); err != nil {
		return fmt.Errorf("delete the test file: %w", err)
	}
	return nil
}

// send uploads to every destination the newest of the folder's backups
// that each keeps but does not have yet, oldest first, then deletes the
// oldest backups there past those it keeps, and records how each went.
// A backup that failed to upload is so sent again at the next run. It
// returns how many destinations failed.
func (s *Service) send(ctx context.Context, dir string) int {
	destinations, err := s.loadDestinations(ctx)
	if err != nil {
		s.cfg.Logger.Warn("Could not read the backup destinations", "error", err)
		return 0
	}
	local, err := localBackups(dir)
	if err != nil {
		s.cfg.Logger.Warn("Could not list the backups to send", "folder", dir, "error", err)
		return 0
	}
	failed := 0
	for _, d := range destinations {
		started := s.now()
		// Each destination has a time of its own, after which it fails and
		// the next is sent to: what it is sent bounds it.
		limit := sendDeadline(dir, local[max(len(local)-d.Kept, 0):])
		destinationCtx, cancel := context.WithTimeout(ctx, limit)
		newest, err := s.sendTo(destinationCtx, d, dir, local)
		cancel()
		if ctx.Err() != nil {
			return failed
		}
		if err != nil && errors.Is(destinationCtx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("the destination did not take the backups within %s: %w", limit.Round(time.Second), err)
		}
		if err != nil {
			failed++
			s.cfg.Logger.Warn("Database backups could not be sent to a destination", "destination", d.Name(), "error", err)
		}
		s.recordSent(ctx, d.ID, started, newest, err)
	}
	return failed
}

// The time a destination is given to take the backups: sendBase, and the
// time their bytes take at minSendRate, a slow connection's.
const (
	sendBase    = 10 * time.Minute
	minSendRate = 32 << 10
)

// sendDeadline is the time a destination is given to take names, the
// newest backups of dir it keeps.
func sendDeadline(dir string, names []string) time.Duration {
	var bytes int64
	for _, name := range names {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
			bytes += info.Size()
		}
	}
	return sendBase + time.Duration(bytes/minSendRate)*time.Second
}

// localBackups lists Polyfin's backups in dir, oldest first.
func localBackups(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var backups []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && backupName.MatchString(entry.Name()) {
			backups = append(backups, entry.Name())
		}
	}
	slices.Sort(backups)
	return backups, nil
}

// sendTo sends the backups local names in dir to d, and returns the
// newest backup there after.
func (s *Service) sendTo(ctx context.Context, d stored, dir string, local []string) (string, error) {
	r, err := s.open(ctx, d)
	if err != nil {
		return "", err
	}
	defer r.close()
	files, err := r.list(ctx)
	if err != nil {
		return "", err
	}
	there := map[string]bool{}
	var errs []error
	for _, file := range files {
		switch {
		case backupName.MatchString(file.name):
			there[file.name] = true
		case partialName.MatchString(file.name):
			// What an interrupted upload left: runs never overlap.
			if err := r.remove(ctx, file.name); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, name := range local[max(len(local)-d.Kept, 0):] {
		if there[name] {
			continue
		}
		if err := upload(ctx, r, filepath.Join(dir, name), name); err != nil {
			errs = append(errs, fmt.Errorf("upload %s: %w", name, err))
			continue
		}
		there[name] = true
		s.cfg.Logger.Info("A database backup was sent", "destination", d.Name(), "file", name)
	}
	names := slices.Sorted(func(yield func(string) bool) {
		for name := range there {
			if !yield(name) {
				return
			}
		}
	})
	var deleted []string
	for _, name := range names[:max(len(names)-d.Kept, 0)] {
		if err := r.remove(ctx, name); err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", name, err))
			continue
		}
		deleted = append(deleted, name)
	}
	if len(deleted) > 0 {
		s.cfg.Logger.Info("Deleted old database backups from a destination", "destination", d.Name(), "files", deleted)
	}
	newest := ""
	if len(names) > 0 {
		newest = names[len(names)-1]
	}
	return newest, errors.Join(errs...)
}

// upload sends the file at path as name.
func upload(ctx context.Context, r remote, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return r.put(ctx, name, f, info.Size())
}

// recordSent saves how the upload to the destination id names, started
// at started, went: newest is the newest backup there after it, kept from
// before when empty.
func (s *Service) recordSent(ctx context.Context, id accounts.ID, started time.Time, newest string, failure error) {
	message := ""
	if failure != nil {
		message = failure.Error()
	}
	var newestAt *time.Time
	if made, ok := madeAt(newest); ok {
		newestAt = &made
	}
	if _, err := s.cfg.DB.Exec(ctx, `UPDATE backup_destinations SET ran_at = $2, error = $3,
		newest_file = CASE WHEN $4 = '' THEN newest_file ELSE $4 END, newest_at = CASE WHEN $4 = '' THEN newest_at ELSE $5 END
		WHERE id = $1`, id, started, message, newest, newestAt); err != nil {
		s.cfg.Logger.Warn("Could not record how sending the database backups went", "error", err)
	}
}

// madeAt is when the backup named name was made, from its name.
func madeAt(name string) (time.Time, bool) {
	match := restorableName.FindStringSubmatch(name)
	if match == nil {
		return time.Time{}, false
	}
	at, err := time.ParseInLocation(nameLayout, match[1], time.Local)
	return at, err == nil
}

// copyTo writes r's content to a new file at path, readable only by its
// owner, and returns its size; a failure leaves no file.
func copyTo(path string, write func(io.Writer) error) (int64, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	err = write(f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	var info os.FileInfo
	if err == nil {
		info, err = os.Stat(path)
	}
	if err != nil {
		_ = os.Remove(path)
		return 0, err
	}
	return info.Size(), nil
}

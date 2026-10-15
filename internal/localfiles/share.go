package localfiles

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"io"
	"io/fs"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/source"
)

// Kinds of network shares a folder may be: an SMB share, whose address is
// smb://host[:port]/share[/path], a WebDAV folder, http(s)://host/path, an
// S3 bucket, s3://bucket[/prefix], reached at an endpoint of its own, an
// SFTP folder, sftp://host[:port]/path, or an rclone remote,
// rclone://host[:port]/path, the address of an rclone instance serving its
// remote control interface and the path in the remote it reads. A folder
// whose address is a path in the container is no share.
const (
	ShareSMB    = "smb"
	ShareWebDAV = "webdav"
	ShareS3     = "s3"
	ShareSFTP   = "sftp"
	ShareRclone = "rclone"
)

// The failures of a share's scan beside those of a folder: the share
// cannot be reached, it refuses the user, password or key, or an SFTP
// server's host key is no longer the one saved at the first connection.
const (
	errUnreachable = "unreachable"
	errRefused     = "refused"
	errHostKey     = "host_key_changed"
)

var (
	ErrInvalidAddress = errors.New("a share's address is smb://host/share/path, http(s)://host/path, s3://bucket/prefix, sftp://host/path " +
		"or rclone://host:port/path, without a user or password in it")
	ErrInvalidUser = errors.New("share users are at most 256 printable characters, passwords at most 1024")
	// ErrInvalidEndpoint reports an S3 bucket without an endpoint, an
	// http(s) address with a host and no user, query or fragment, or a
	// region that is not a few printable characters.
	ErrInvalidEndpoint = errors.New("an S3 bucket's endpoint is an http(s) address, its region at most 64 printable characters")
	// ErrInvalidKey reports an SFTP folder's private key that is not an
	// OpenSSH or PEM private key, or whose passphrase does not open it.
	ErrInvalidKey = errors.New("an SFTP folder's private key is an OpenSSH or PEM private key, its passphrase the one that opens it")
	// ErrInvalidRemote reports an rclone remote without a name, or with a
	// name rclone would not give one.
	ErrInvalidRemote = errors.New("an rclone remote is named by 1 to 128 printable characters, without a colon or a slash")
	// ErrInvalidPollMinutes reports a time between the listings of a
	// share out of 0 to 1440 minutes.
	ErrInvalidPollMinutes = errors.New("a share is listed again every 0 to 1440 minutes")
)

var (
	// errShareUnreachable reports a share that does not answer: its host
	// cannot be reached, or its server failed.
	errShareUnreachable = errors.New("the share cannot be reached")
	// errShareRefused reports a share refusing the user or password.
	errShareRefused = errors.New("the share refuses the user or password")
	// errShareHostKey reports an SFTP server offering another host key
	// than the one saved at the first connection.
	errShareHostKey = errors.New("the SFTP server's host key is not the one saved")
	// errIsFile reports a share's address naming a file.
	errIsFile = errors.New("not a folder")
)

const (
	// connectTimeout bounds connecting to a share, signing in included.
	connectTimeout = 15 * time.Second
	// listTimeout bounds the listing of one folder of a share.
	listTimeout = time.Minute
	// maxPollMinutes bounds the time between the listings of a share.
	maxPollMinutes = 1440
)

// shareKind tells the kind of share an address is, empty for a path in the
// container.
func shareKind(address string) string {
	scheme, _, ok := strings.Cut(address, "://")
	if !ok {
		return ""
	}
	switch strings.ToLower(scheme) {
	case "smb":
		return ShareSMB
	case "http", "https":
		return ShareWebDAV
	case "s3":
		return ShareS3
	case "sftp":
		return ShareSFTP
	case "rclone":
		return ShareRclone
	}
	return ""
}

// cachedKind reports whether the files of a kind of share are read through
// the source cache, straight from the storage (see cachedRemote): those of
// S3 buckets, SFTP folders and rclone remotes. SMB shares and WebDAV
// folders are read through the loopback.
func cachedKind(kind string) bool {
	return kind == ShareS3 || kind == ShareSFTP || kind == ShareRclone
}

// defaultPollMinutes is how often a kind of share is listed again to find
// its changes, when the administrator did not say: every 30 minutes for
// an S3 bucket, as providers may bill each listing, every 2 for SFTP
// folders and rclone remotes. SMB shares and WebDAV folders are listed
// every shareEvery, which no setting changes: nil.
func defaultPollMinutes(kind string) *int {
	switch kind {
	case ShareS3:
		return new(30)
	case ShareSFTP, ShareRclone:
		return new(2)
	}
	return nil
}

// validLocation cleans a folder's location: a path in the container, or a
// share's address.
func validLocation(location string) (string, error) {
	location = strings.TrimSpace(location)
	if shareKind(location) == "" {
		return validPath(location)
	}
	return validAddress(location)
}

// validAddress cleans a share's address: a host, an SMB share's name, no
// user, password, query or fragment, and a clean path without its last
// slash. An S3 bucket's host is the bucket's name, without a port.
func validAddress(address string) (string, error) {
	if len(address) > 4096 || strings.ContainsFunc(address, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return "", ErrInvalidAddress
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", ErrInvalidAddress
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Path = strings.TrimSuffix(path.Clean("/"+u.Path), "/")
	u.RawPath = ""
	switch {
	case u.Scheme == "smb" && u.Path == "":
		return "", ErrInvalidAddress
	case u.Scheme == "s3" && (u.Port() != "" || strings.ContainsAny(u.Host, ":[]")):
		return "", ErrInvalidAddress
	case u.Port() != "":
		if port, err := strconv.Atoi(u.Port()); err != nil || port < 1 || port > 65535 {
			return "", ErrInvalidAddress
		}
	}
	return u.String(), nil
}

// validCredentials checks a share's user and password.
func validCredentials(user, password string) (string, error) {
	user = strings.TrimSpace(user)
	if len(user) > 256 || len(password) > 1024 || strings.ContainsFunc(user, func(r rune) bool { return !unicode.IsPrint(r) }) ||
		strings.ContainsFunc(password, unicode.IsControl) {
		return "", ErrInvalidUser
	}
	return user, nil
}

// validEndpoint cleans an S3 bucket's endpoint: an http(s) address with a
// host, no user, query or fragment, without its last slash.
func validEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	u, err := url.Parse(endpoint)
	if err != nil || len(endpoint) > 1024 || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", ErrInvalidEndpoint
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

// validRegion checks an S3 bucket's region, empty for the default one.
func validRegion(region string) (string, error) {
	region = strings.TrimSpace(region)
	if len(region) > 64 || strings.ContainsFunc(region, func(r rune) bool { return !unicode.IsPrint(r) || unicode.IsSpace(r) }) {
		return "", ErrInvalidEndpoint
	}
	return region, nil
}

// validRemote checks an rclone remote's name, as rclone names them.
func validRemote(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" || len(remote) > 128 || strings.ContainsAny(remote, ":/\\") ||
		strings.ContainsFunc(remote, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return "", ErrInvalidRemote
	}
	return remote, nil
}

// validPollMinutes checks the minutes between a share's listings: 0 lists
// it only at its scans.
func validPollMinutes(minutes int) error {
	if minutes < 0 || minutes > maxPollMinutes {
		return ErrInvalidPollMinutes
	}
	return nil
}

// shareExtra is what an S3 bucket, an SFTP folder or an rclone remote is
// read with beside its user and password, the private key and passphrase
// opened, and how often it is listed again for changes; the other folders
// keep none of it.
type shareExtra struct {
	endpoint, region, remote string
	privateKey, passphrase   string
	pollMinutes              *int
}

// same reports whether two shareExtra are the same.
func (e shareExtra) same(other shareExtra) bool {
	a, b := e, other
	a.pollMinutes, b.pollMinutes = nil, nil
	return a == b && (e.pollMinutes == nil) == (other.pollMinutes == nil) &&
		(e.pollMinutes == nil || *e.pollMinutes == *other.pollMinutes)
}

// validExtra checks what a folder of a kind of share is read with beside
// its user and password, and keeps only what its kind reads: an S3
// bucket's endpoint, required, and region; an SFTP folder's private key
// and its passphrase; an rclone remote's name, required; and for those
// three how often they are listed again, their kind's default when nil.
func validExtra(share string, e shareExtra) (shareExtra, error) {
	kept := shareExtra{}
	var err error
	switch share {
	case ShareS3:
		if kept.endpoint, err = validEndpoint(e.endpoint); err != nil {
			return shareExtra{}, err
		}
		if kept.region, err = validRegion(e.region); err != nil {
			return shareExtra{}, err
		}
	case ShareSFTP:
		if err := validPrivateKey(e.privateKey, e.passphrase); err != nil {
			return shareExtra{}, err
		}
		kept.privateKey, kept.passphrase = e.privateKey, e.passphrase
	case ShareRclone:
		if kept.remote, err = validRemote(e.remote); err != nil {
			return shareExtra{}, err
		}
	default:
		return kept, nil
	}
	kept.pollMinutes = defaultPollMinutes(share)
	if e.pollMinutes != nil {
		if err := validPollMinutes(*e.pollMinutes); err != nil {
			return shareExtra{}, err
		}
		kept.pollMinutes = new(*e.pollMinutes)
	}
	return kept, nil
}

// storedExtra reads what a folder is read with beside its user and
// password, its private key and passphrase opened: those the key cannot
// open are none.
func (s *Service) storedExtra(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, id accounts.ID) (shareExtra, error) {
	var e shareExtra
	var sealedKey, sealedPassphrase string
	if err := q.QueryRow(ctx, `SELECT share_endpoint, share_region, share_remote, share_private_key, share_passphrase, poll_minutes
		FROM local_folders WHERE addon_id = $1`, id).Scan(&e.endpoint, &e.region, &e.remote, &sealedKey, &sealedPassphrase,
		&e.pollMinutes); err != nil {
		return shareExtra{}, err
	}
	e.privateKey, _ = s.box.Open(sealedKey)
	e.passphrase, _ = s.box.Open(sealedPassphrase)
	return e, nil
}

// serverOf is the server an address names, host and port: an SFTP
// folder's host key is the one of the server, kept while the folder stays
// on it.
func serverOf(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// remoteEntry is an entry of a share's folder: a folder, or a file with
// its size and modification time.
type remoteEntry struct {
	name     string
	folder   bool
	size     int64
	modified time.Time
}

// remote reads a share, keeping its connections between requests and
// connecting again when they fail. Paths are relative to the folder the
// address names, with slashes, "" for that folder.
type remote interface {
	// list lists a folder: errIsFile when it is a file.
	list(ctx context.Context, rel string) ([]remoteEntry, error)
	// open opens a file for reading, with its modification time.
	open(ctx context.Context, rel string) (io.ReadSeekCloser, time.Time, error)
	close()
}

// cachedRemote is a share whose files the source cache reads by ranges,
// straight from the storage, rather than through the loopback (see
// Service.Source): an S3 bucket, an SFTP folder or an rclone remote.
type cachedRemote interface {
	remote
	// location is where the cache reads a file of the folder, and how.
	location(rel string) source.Location
}

// shareAccess is what a share is read with: its address, user and
// password, an S3 bucket's endpoint and region (its user and password
// being its access key and secret key), an SFTP folder's private key and
// its passphrase, and an rclone remote's name.
type shareAccess struct {
	address, user, password  string
	endpoint, region, remote string
	privateKey, passphrase   string
}

// hostKeys keeps an SFTP server's host key: saved is the one saved,
// empty before the first connection, save saves the one the first
// connection was offered, and offered tells the one a server offers in
// place of the one saved, which an administrator may accept.
type hostKeys struct {
	saved   string
	save    func(key string)
	offered func(key string)
}

// newRemote returns the reader of a share; it connects when first used.
func newRemote(access shareAccess, keys hostKeys) (remote, error) {
	u, err := url.Parse(access.address)
	if err != nil {
		return nil, err
	}
	switch shareKind(access.address) {
	case ShareSMB:
		return newSMB(u, access.user, access.password), nil
	case ShareS3:
		return newS3(u, access)
	case ShareSFTP:
		return newSFTP(u, access, keys)
	case ShareRclone:
		return newRclone(u, access), nil
	}
	return newWebDAV(u, access.user, access.password), nil
}

// shareFailure tells why a share cannot be read, as a folder's Error.
func shareFailure(err error) string {
	switch {
	case errors.Is(err, errShareRefused):
		return errRefused
	case errors.Is(err, errShareHostKey):
		return errHostKey
	case errors.Is(err, errIsFile):
		return errNotFolder
	case errors.Is(err, fs.ErrNotExist):
		return errMissing
	case errors.Is(err, fs.ErrPermission):
		return errUnreadable
	}
	return errUnreachable
}

// pooledRemote is a share's reader and what it was made with.
type pooledRemote struct {
	access shareAccess
	remote remote
}

// remote returns the reader of a folder's share, kept between requests,
// made again once what it is read with changed. An SFTP server's host key
// is kept by its reader: one accepted by an administrator forgets the
// reader (see forgetRemote).
func (s *Service) remote(id accounts.ID, access shareAccess, keys hostKeys) (remote, error) {
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	if p, ok := s.shares[id]; ok {
		if p.access == access {
			return p.remote, nil
		}
		p.remote.close()
		delete(s.shares, id)
	}
	r, err := newRemote(access, keys)
	if err != nil {
		return nil, err
	}
	s.shares[id] = &pooledRemote{access: access, remote: r}
	return r, nil
}

// forgetRemotes closes the readers of the shares keep does not name.
func (s *Service) forgetRemotes(keep func(accounts.ID) bool) {
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	for id, p := range s.shares {
		if !keep(id) {
			p.remote.close()
			delete(s.shares, id)
		}
	}
}

// forgetRemote closes the reader of a folder's share: the next request
// makes it again.
func (s *Service) forgetRemote(id accounts.ID) {
	s.forgetRemotes(func(other accounts.ID) bool { return other != id })
}

// folderRemote returns the reader of a folder's share, its password
// opened, and for an S3 bucket, an SFTP folder or an rclone remote what
// else it is read with, read from the folder.
func (s *Service) folderRemote(ctx context.Context, id accounts.ID, address, user, sealed string) (remote, error) {
	password, err := s.box.Open(sealed)
	if err != nil {
		// A password the key cannot open is no password: the share
		// refuses Polyfin until it is entered again.
		return nil, errShareRefused
	}
	access := shareAccess{address: address, user: user, password: password}
	var keys hostKeys
	if cachedKind(shareKind(address)) {
		var sealedKey, sealedPassphrase string
		err := s.db.QueryRow(ctx, `SELECT share_endpoint, share_region, share_remote, share_private_key, share_passphrase, share_host_key
			FROM local_folders WHERE addon_id = $1`, id).Scan(&access.endpoint, &access.region, &access.remote, &sealedKey, &sealedPassphrase,
			&keys.saved)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fs.ErrNotExist
		}
		if err != nil {
			return nil, err
		}
		if access.privateKey, err = s.box.Open(sealedKey); err != nil {
			return nil, errShareRefused
		}
		if access.passphrase, err = s.box.Open(sealedPassphrase); err != nil {
			return nil, errShareRefused
		}
		keys.save = func(key string) { s.saveHostKey(id, address, key) }
		keys.offered = func(key string) { s.offerHostKey(id, address, key) }
	}
	return s.remote(id, access, keys)
}

// saveHostKey saves the host key an SFTP server offered at its folder's
// first connection, unless one was saved meanwhile.
func (s *Service) saveHostKey(id accounts.ID, address, key string) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `UPDATE local_folders SET share_host_key = $3, share_offered_key = '' FROM addons a
		WHERE addon_id = $1 AND a.id = addon_id AND a.manifest_url = $2 AND share_host_key = ''`, id, address, key); err != nil {
		s.logger.Warn("The host key of an SFTP folder could not be saved", "address", address, "error", err)
	}
}

// offerHostKey keeps the host key an SFTP server offers in place of the
// one saved, which an administrator may accept (see Changes.AcceptHostKey).
func (s *Service) offerHostKey(id accounts.ID, address, key string) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `UPDATE local_folders SET share_offered_key = $3 FROM addons a
		WHERE addon_id = $1 AND a.id = addon_id AND a.manifest_url = $2 AND share_host_key <> '' AND share_host_key <> $3`, id, address,
		key); err != nil {
		s.logger.Warn("The new host key of an SFTP folder could not be kept", "address", address, "error", err)
	}
}

// walkShare lists the files a scan of a folder of kind reads under a
// share's folder, disc images, the files of DVDs and Blu-ray discs'
// playlists among them (see walked), and their side files, one folder
// after the other, from the listings alone, with the print of each folder
// it listed (see folderPrint). The folders it could not list, missing or
// refused, are skipped. A share that stops answering fails the walk.
func walkShare(ctx context.Context, r remote, root []remoteEntry, kind string) (listing, error) {
	list := listing{files: map[string]found{}, sides: map[string]found{}, prints: map[string]uint64{},
		open: func(ctx context.Context, rel string) (io.ReadCloser, error) {
			file, _, err := r.open(ctx, rel)
			return file, err
		}}
	type pending struct {
		rel     string
		listed  bool
		entries []remoteEntry
	}
	queue := []pending{{rel: "", listed: true, entries: root}}
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return listing{}, ctx.Err()
		}
		next := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		entries := next.entries
		if !next.listed {
			listCtx, cancel := context.WithTimeout(ctx, listTimeout)
			var err error
			entries, err = r.list(listCtx, next.rel)
			cancel()
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, errIsFile) {
				list.skipped = append(list.skipped, next.rel)
				continue
			}
			if err != nil {
				return listing{}, err
			}
		}
		list.prints[next.rel] = folderPrint(kind, next.rel, entries)
		for _, e := range entries {
			rel := e.name
			if next.rel != "" {
				rel = next.rel + "/" + e.name
			}
			if e.folder {
				if !skippedIn(kind, e.name) {
					// Listed once its turn comes, so that one folder's
					// entries at most are held at once.
					queue = append(queue, pending{rel: rel})
				}
				continue
			}
			switch file, side := walked(kind, rel); {
			case file:
				list.files[rel] = found{size: e.size, modified: e.modified.UTC().Truncate(time.Microsecond)}
			case side:
				list.sides[rel] = found{size: e.size, modified: e.modified.UTC().Truncate(time.Microsecond)}
			}
		}
	}
	return list, nil
}

// folderPrint sums what a scan of a folder of kind reads of a share's
// folder at rel: the names of the folders it enters, and the names, sizes
// and modification times of its files and side files, disc images and
// the files of discs among them (see walked). Two listings of the folder
// have the same print, in whatever order they list it, unless one of
// those changed; the other entries count for nothing.
func folderPrint(kind, rel string, entries []remoteEntry) uint64 {
	h := fnv.New64a()
	var sum uint64
	var number [8]byte
	for _, e := range entries {
		h.Reset()
		switch file, side := walked(kind, path.Join(rel, e.name)); {
		case e.folder:
			if skippedIn(kind, e.name) {
				continue
			}
			h.Write([]byte{'d'})
		case file || side:
			h.Write([]byte{'f'})
			binary.LittleEndian.PutUint64(number[:], uint64(e.size))
			h.Write(number[:])
			binary.LittleEndian.PutUint64(number[:], uint64(e.modified.UTC().Truncate(time.Microsecond).UnixMicro()))
			h.Write(number[:])
		default:
			continue
		}
		h.Write([]byte(e.name))
		sum += h.Sum64()
	}
	return sum
}

// shareFiles lists a share's files for a folder of kind, or tells why it
// cannot.
func (s *Service) shareFiles(ctx context.Context, id accounts.ID, address, user, sealed, kind string) (list listing, failure string, err error) {
	r, err := s.folderRemote(ctx, id, address, user, sealed)
	if err != nil {
		return listing{}, shareFailure(err), nil
	}
	listCtx, cancel := context.WithTimeout(ctx, listTimeout)
	root, err := r.list(listCtx, "")
	cancel()
	if err == nil {
		list, err = walkShare(ctx, r, root, kind)
	}
	if err != nil {
		if ctx.Err() != nil {
			return listing{}, "", ctx.Err()
		}
		s.logger.Warn("A network share cannot be read", "address", address, "error", err)
		return listing{}, shareFailure(err), nil
	}
	return list, "", nil
}

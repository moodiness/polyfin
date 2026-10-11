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
	"strings"
	"time"
	"unicode"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Kinds of network shares a folder may be: an SMB share, whose address is
// smb://host[:port]/share[/path], or a WebDAV folder, http(s)://host/path.
// A folder whose address is a path in the container is no share.
const (
	ShareSMB    = "smb"
	ShareWebDAV = "webdav"
)

// The failures of a share's scan beside those of a folder: the share
// cannot be reached, or it refuses the user or password.
const (
	errUnreachable = "unreachable"
	errRefused     = "refused"
)

var (
	ErrInvalidAddress = errors.New("a share's address is smb://host/share/path or http(s)://host/path, without a user or password in it")
	ErrInvalidUser    = errors.New("share users are at most 256 printable characters, passwords at most 1024")
)

var (
	// errShareUnreachable reports a share that does not answer: its host
	// cannot be reached, or its server failed.
	errShareUnreachable = errors.New("the share cannot be reached")
	// errShareRefused reports a share refusing the user or password.
	errShareRefused = errors.New("the share refuses the user or password")
	// errIsFile reports a share's address naming a file.
	errIsFile = errors.New("not a folder")
)

const (
	// connectTimeout bounds connecting to a share, signing in included.
	connectTimeout = 15 * time.Second
	// listTimeout bounds the listing of one folder of a share.
	listTimeout = time.Minute
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
	}
	return ""
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
// slash.
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
	if u.Scheme == "smb" && u.Path == "" {
		return "", ErrInvalidAddress
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

// newRemote returns the reader of a share; it connects when first used.
func newRemote(address, user, password string) (remote, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if shareKind(address) == ShareSMB {
		return newSMB(u, user, password), nil
	}
	return newWebDAV(u, user, password), nil
}

// shareFailure tells why a share cannot be read, as a folder's Error.
func shareFailure(err error) string {
	switch {
	case errors.Is(err, errShareRefused):
		return errRefused
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
	address, user, password string
	remote                  remote
}

// remote returns the reader of a folder's share, kept between requests,
// made again once its address, user or password changed.
func (s *Service) remote(id accounts.ID, address, user, password string) (remote, error) {
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	if p, ok := s.shares[id]; ok {
		if p.address == address && p.user == user && p.password == password {
			return p.remote, nil
		}
		p.remote.close()
		delete(s.shares, id)
	}
	r, err := newRemote(address, user, password)
	if err != nil {
		return nil, err
	}
	s.shares[id] = &pooledRemote{address: address, user: user, password: password, remote: r}
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

// folderRemote returns the reader of a folder's share, its password
// opened.
func (s *Service) folderRemote(id accounts.ID, address, user, sealed string) (remote, error) {
	password, err := s.box.Open(sealed)
	if err != nil {
		// A password the key cannot open is no password: the share
		// refuses Polyfin until it is entered again.
		return nil, errShareRefused
	}
	return s.remote(id, address, user, password)
}

// walkShare lists the files a scan of a folder of kind reads under a
// share's folder, and their side files, one folder after the other, from
// the listings alone, with the print of each folder it listed (see
// folderPrint). The folders it could not list, missing or refused, are
// skipped. A share that stops answering fails the walk.
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
		list.prints[next.rel] = folderPrint(kind, entries)
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
			switch file, side := listed(kind, e.name); {
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
// folder: the names of the folders it enters, and the names, sizes and
// modification times of its files and side files. Two listings of the
// folder have the same print, in whatever order they list it, unless one
// of those changed; the other entries count for nothing.
func folderPrint(kind string, entries []remoteEntry) uint64 {
	h := fnv.New64a()
	var sum uint64
	var number [8]byte
	for _, e := range entries {
		h.Reset()
		switch file, side := listed(kind, e.name); {
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
	r, err := s.folderRemote(id, address, user, sealed)
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

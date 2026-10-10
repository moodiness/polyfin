package localfiles

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cloudsoda/go-smb2"
)

// The NTSTATUS codes an SMB server answers a sign-in it refuses with, the
// share name it does not know with, and a session it ended with.
const (
	statusLogonFailure          = 0xC000006D
	statusWrongPassword         = 0xC000006A
	statusNoSuchUser            = 0xC0000064
	statusAccountRestriction    = 0xC000006E
	statusPasswordExpired       = 0xC0000071
	statusAccountDisabled       = 0xC0000072
	statusAccountLockedOut      = 0xC0000234
	statusBadNetworkName        = 0xC00000CC
	statusNetworkNameDeleted    = 0xC00000C9
	statusUserSessionDeleted    = 0xC0000203
	statusNetworkSessionExpired = 0xC000035C
)

// readAhead is how much of a file of an SMB share one read asks for: the
// players' small reads would each wait for the server.
const readAhead = 1 << 20

// smbShare reads an SMB share, its session kept open between requests and
// opened again once it fails.
type smbShare struct {
	// addr is the server's host and port, name the share's, and dir the
	// folder in it, with slashes, empty for its root.
	addr, name, dir string
	user, domain    string
	password        string

	mu      sync.Mutex
	session *smb2.Session
	share   *smb2.Share
	// alive lasts as long as the session: the files opened in it are read
	// with it, so that ending the session ends their reads too.
	alive context.Context
	end   context.CancelFunc
}

func newSMB(u *url.URL, user, password string) *smbShare {
	port := u.Port()
	if port == "" {
		port = "445"
	}
	name, dir, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	// DOMAIN\user signs in to a domain.
	domain, account, ok := strings.Cut(user, `\`)
	if !ok {
		domain, account = "", user
	}
	return &smbShare{addr: net.JoinHostPort(u.Hostname(), port), name: name, dir: dir, user: account, domain: domain, password: password}
}

// path is a file's path in the share.
func (c *smbShare) path(rel string) string {
	switch {
	case c.dir == "":
		return rel
	case rel == "":
		return c.dir
	}
	return c.dir + "/" + rel
}

// mounted returns the share and the session's context (see alive),
// signing in to the server when no session is open.
func (c *smbShare) mounted(ctx context.Context) (*smb2.Share, context.Context, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.share != nil {
		return c.share, c.alive, nil
	}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	dialer := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: c.user, Domain: c.domain, Password: c.password}}
	session, err := dialer.Dial(ctx, c.addr)
	if err != nil {
		return nil, nil, smbError(err)
	}
	share, err := session.WithContext(ctx).Mount(c.name)
	if err != nil {
		_ = session.WithContext(ctx).Logoff()
		return nil, nil, smbError(err)
	}
	c.session, c.share = session, share
	c.alive, c.end = context.WithCancel(context.Background())
	return share, c.alive, nil
}

// drop closes the session share was mounted in, unless another one
// replaced it already.
func (c *smbShare) drop(share *smb2.Share) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.share != share || c.session == nil {
		return
	}
	c.logoff()
}

// logoff ends the session, briefly: a server that stopped answering is
// not waited for. The reads under way in it end.
func (c *smbShare) logoff() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.session.WithContext(ctx).Logoff()
	c.end()
	c.session, c.share, c.alive, c.end = nil, nil, nil, nil
}

func (c *smbShare) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		c.logoff()
	}
}

// do runs op on the share, signing in again once when the session failed:
// the server restarted, or ended it. op gets the share bound to ctx, and
// the session's context.
func (c *smbShare) do(ctx context.Context, op func(share *smb2.Share, alive context.Context) error) error {
	for attempt := 0; ; attempt++ {
		share, alive, err := c.mounted(ctx)
		if err != nil {
			return err
		}
		err = op(share.WithContext(ctx), alive)
		if err == nil || !brokenSession(err) || attempt > 0 || ctx.Err() != nil {
			return smbError(err)
		}
		c.drop(share)
	}
}

// brokenSession reports whether err tells of a session that no longer
// works, rather than of a file or folder.
func brokenSession(err error) bool {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, errIsFile) {
		return false
	}
	var response *smb2.ResponseError
	if errors.As(err, &response) {
		switch response.Code {
		case statusNetworkNameDeleted, statusUserSessionDeleted, statusNetworkSessionExpired:
			return true
		}
		return false
	}
	return true
}

// smbError classes what the server answered (see shareFailure).
func smbError(err error) error {
	if err == nil {
		return nil
	}
	var response *smb2.ResponseError
	if errors.As(err, &response) {
		switch response.Code {
		case statusLogonFailure, statusWrongPassword, statusNoSuchUser, statusAccountRestriction, statusPasswordExpired,
			statusAccountDisabled, statusAccountLockedOut:
			return fmt.Errorf("%w: %v", errShareRefused, err)
		case statusBadNetworkName:
			return fmt.Errorf("%w: %v", fs.ErrNotExist, err)
		}
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, errIsFile) {
		return err
	}
	return fmt.Errorf("%w: %v", errShareUnreachable, err)
}

func (c *smbShare) list(ctx context.Context, rel string) ([]remoteEntry, error) {
	var entries []remoteEntry
	err := c.do(ctx, func(share *smb2.Share, _ context.Context) error {
		entries = nil
		dir, err := share.Open(c.path(rel))
		if err != nil {
			return err
		}
		defer dir.Close()
		if info, err := dir.Stat(); err != nil {
			return err
		} else if !info.IsDir() {
			return errIsFile
		}
		for {
			infos, err := dir.Readdir(256)
			for _, info := range infos {
				switch name := info.Name(); {
				case name == "." || name == "..":
				case info.IsDir():
					entries = append(entries, remoteEntry{name: name, folder: true})
				case info.Mode().IsRegular():
					entries = append(entries, remoteEntry{name: name, size: info.Size(), modified: info.ModTime()})
				}
			}
			if errors.Is(err, io.EOF) || err == nil && len(infos) == 0 {
				return nil
			}
			if err != nil {
				return err
			}
		}
	})
	return entries, err
}

func (c *smbShare) open(ctx context.Context, rel string) (io.ReadSeekCloser, time.Time, error) {
	var file *smbFile
	var modified time.Time
	// The file is read, then closed, as long as the session lasts, however
	// the request ends.
	err := c.do(ctx, func(share *smb2.Share, alive context.Context) error {
		f, err := share.WithContext(alive).Open(c.path(rel))
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			f.Close()
			if err == nil {
				err = fs.ErrNotExist
			}
			return err
		}
		file, modified = &smbFile{file: f, size: info.Size()}, info.ModTime()
		return nil
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	return file, modified, nil
}

// readers are the buffers the files of SMB shares are read through.
var readers = sync.Pool{New: func() any { return bufio.NewReaderSize(nil, readAhead) }}

// smbFile reads a file of an SMB share ahead, readAhead bytes per request
// to the server.
type smbFile struct {
	file   *smb2.File
	size   int64
	offset int64
	// reader reads from offset on; nil until read, and after a seek.
	reader *bufio.Reader
}

func (f *smbFile) Read(p []byte) (int, error) {
	if f.reader == nil {
		f.reader = readers.Get().(*bufio.Reader)
		f.reader.Reset(io.NewSectionReader(f.file, f.offset, max(f.size-f.offset, 0)))
	}
	n, err := f.reader.Read(p)
	f.offset += int64(n)
	return n, err
}

func (f *smbFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		offset += f.offset
	case io.SeekEnd:
		offset += f.size
	}
	if offset < 0 {
		return 0, errors.New("seek before the start of the file")
	}
	if offset != f.offset {
		f.release()
		f.offset = offset
	}
	return offset, nil
}

// release gives the buffer back.
func (f *smbFile) release() {
	if f.reader != nil {
		f.reader.Reset(nil)
		readers.Put(f.reader)
		f.reader = nil
	}
}

func (f *smbFile) Close() error {
	f.release()
	return f.file.Close()
}

package localfiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/moodiness/polyfin/internal/source"
)

// sftpShare reads a folder of an SFTP server over one SSH connection,
// kept between requests and made again once it fails. The server's host
// key is saved at the first connection; another one later is refused,
// and kept for an administrator to accept (see hostKeys).
type sftpShare struct {
	// host is the server's host and port, folder the folder's absolute
	// path on it, "/" for its root.
	host   string
	folder string
	config ssh.ClientConfig
	keys   hostKeys
	// bounds bound what is read and kept of one folder's listing, answer
	// being the bytes of all the answers listing it.
	bounds listBounds
	// openTimeout bounds the opening of the channel for listings (see
	// sftpOpenTimeout).
	openTimeout time.Duration
	// logger logs a panic of the SSH client waiting on the connection,
	// which nothing stands above (see connect).
	logger *slog.Logger

	mu sync.Mutex
	// saved is the host key saved, client the connection open, nil
	// before the first request and once it failed.
	saved  string
	conn   *ssh.Client
	client *sftp.Client

	// listing has one folder listed at a time, over lister, the channel
	// of the connection for listings, nil until one is listed. single is
	// the connection's client whose server opened no such channel: its
	// listings are read by the client, until the next connection.
	listing sync.Mutex
	lister  *sftpLister
	single  *sftp.Client
}

// errSingleSession is a connection whose server opens no channel for
// listings (see sftpShare.listed).
var errSingleSession = errors.New("the SFTP server opens no second session")

// errOpenLate is a channel for listings the server did not open within
// its time (see sftpShare.openLister).
var errOpenLate = errors.New("the SFTP server did not open the session for listings in time")

func newSFTP(u *url.URL, access shareAccess, keys hostKeys, logger *slog.Logger) (*sftpShare, error) {
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "22")
	}
	folder := u.Path
	if folder == "" {
		folder = "/"
	}
	s := &sftpShare{host: host, folder: folder, keys: keys, saved: keys.saved, bounds: folderBounds, openTimeout: sftpOpenTimeout,
		logger: logger}
	var auth []ssh.AuthMethod
	if access.privateKey != "" {
		signer, err := parsePrivateKey(access.privateKey, access.passphrase)
		if err != nil {
			return nil, err
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if access.password != "" {
		password := access.password
		auth = append(auth, ssh.Password(password), ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = password
			}
			return answers, nil
		}))
	}
	s.config = ssh.ClientConfig{User: access.user, Auth: auth, Timeout: connectTimeout}
	return s, nil
}

// parsePrivateKey reads an OpenSSH or PEM private key, opened with its
// passphrase when it has one.
func parsePrivateKey(key, passphrase string) (ssh.Signer, error) {
	var signer ssh.Signer
	var err error
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(key), []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(key))
	}
	if err != nil {
		return nil, ErrInvalidKey
	}
	return signer, nil
}

// validPrivateKey checks an SFTP folder's private key and passphrase.
func validPrivateKey(key, passphrase string) error {
	if len(key) > 64<<10 || len(passphrase) > 1024 {
		return ErrInvalidKey
	}
	if key == "" {
		if passphrase != "" {
			return ErrInvalidKey
		}
		return nil
	}
	_, err := parsePrivateKey(key, passphrase)
	return err
}

// authorizedKey writes a host key in the authorized_keys format.
func authorizedKey(key ssh.PublicKey) string {
	return string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(key)))
}

// Fingerprint is the SHA-256 fingerprint of a host key in the
// authorized_keys format, as ssh shows it; empty for none.
func Fingerprint(key string) string {
	if key == "" {
		return ""
	}
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(parsed)
}

// connect returns the connection open, connecting first when there is
// none. The first connection saves the host key the server offers, unless
// the folder moved to another server since its key was saved: that key is
// refused, and kept to be accepted, as another key than the one saved is.
func (s *sftpShare) connect(ctx context.Context) (*sftp.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client, nil
	}
	var offered, first string
	config := s.config
	config.HostKeyCallback = func(_ string, _ net.Addr, key ssh.PublicKey) error {
		line := authorizedKey(key)
		if s.saved == "" {
			if s.keys.forgotten {
				offered = line
				return errShareHostKey
			}
			first = line
			return nil
		}
		saved, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s.saved))
		if err != nil || !bytes.Equal(saved.Marshal(), key.Marshal()) {
			offered = line
			return errShareHostKey
		}
		return nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", s.host)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
	}
	// Signing in ends with the dial's time.
	stop := context.AfterFunc(dialCtx, func() { raw.Close() })
	conn, channels, requests, err := ssh.NewClientConn(raw, s.host, &config)
	stopped := stop()
	if err != nil {
		raw.Close()
		switch {
		case offered != "":
			if s.keys.offered != nil {
				s.keys.offered(offered)
			}
			return nil, fmt.Errorf("%w: it now offers %s", errShareHostKey, Fingerprint(offered))
		case !stopped && ctx.Err() != nil:
			return nil, ctx.Err()
		case strings.Contains(err.Error(), "unable to authenticate"):
			return nil, fmt.Errorf("%w: %v", errShareRefused, err)
		}
		return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
	}
	sshClient := ssh.NewClient(conn, channels, requests)
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
	}
	if first != "" {
		s.saved = first
		if s.keys.save != nil {
			s.keys.save(first)
		}
	}
	s.conn, s.client = sshClient, client
	go func() {
		// A connection the server or the network closed is made again at
		// the next request, as is one whose SSH client panicked: the panic
		// is logged with its stack, and the server goes on.
		defer s.drop(client)
		defer func() {
			if cause := recover(); cause != nil {
				s.logger.Error("A network share's connection ended on a panic", "host", s.host, "panic", fmt.Sprint(cause),
					"stack", string(debug.Stack()))
			}
		}()
		_ = sshClient.Wait()
	}()
	return client, nil
}

// drop closes the connection client, when it is still the one open.
func (s *sftpShare) drop(client *sftp.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != client || client == nil {
		return
	}
	s.client.Close()
	s.conn.Close()
	s.client, s.conn = nil, nil
}

func (s *sftpShare) close() {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	s.drop(client)
}

// do runs a request over the connection, connecting first. A request the
// connection failed under, as once the server restarted, is run again
// once over a new connection. The request reads the server's answers in
// the caller's goroutine, a scan's or a request's, but for the opening of
// the channel for listings (see openLister): a panic of what reads them,
// the SSH and SFTP clients or the listing's own reader (see sftpLister),
// fails the request (see panicked).
func (s *sftpShare) do(ctx context.Context, request func(client *sftp.Client) error) (err error) {
	var client *sftp.Client
	defer func() {
		if cause := recover(); cause != nil {
			err = s.panicked(client, cause)
		}
	}()
	for attempt := 0; ; attempt++ {
		client, err = s.connect(ctx)
		if err != nil {
			return err
		}
		err = request(client)
		var status *sftp.StatusError
		var bound *shareBound
		switch {
		case err == nil:
			return nil
		case errors.Is(err, errIsFile), errors.As(err, &bound):
			return err
		case errors.Is(err, fs.ErrNotExist):
			return fs.ErrNotExist
		case errors.Is(err, fs.ErrPermission):
			return fs.ErrPermission
		case errors.As(err, &status):
			return fmt.Errorf("%w: %v", errShareUnreachable, err)
		}
		s.drop(client)
		if attempt > 0 || ctx.Err() != nil {
			return fmt.Errorf("%w: %v", errShareUnreachable, err)
		}
	}
}

// panicked logs a panic of the SSH or SFTP client with its stack, drops
// the connection client, whose state is then not known, when there is
// one, and returns the error of the request the panic ended: the server
// goes on, and the next request connects again.
func (s *sftpShare) panicked(client *sftp.Client, cause any) error {
	if client != nil {
		s.drop(client)
	}
	s.logger.Error("A network share's request ended on a panic", "host", s.host, "panic", fmt.Sprint(cause),
		"stack", string(debug.Stack()))
	return fmt.Errorf("%w: the request ended on a panic: %v", errShareUnreachable, cause)
}

// file is the path on the server of a file or folder of the folder.
func (s *sftpShare) file(rel string) string {
	return path.Join(s.folder, rel)
}

func (s *sftpShare) list(ctx context.Context, rel string) ([]remoteEntry, error) {
	dir := s.file(rel)
	var kept folderList
	err := s.do(ctx, func(client *sftp.Client) error {
		// A listing out of time is the connection's failure: SFTP's
		// requests have no deadline of their own.
		stop := context.AfterFunc(ctx, func() {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				s.drop(client)
			}
		})
		defer stop()
		info, err := client.Stat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errIsFile
		}
		kept = folderList{bounds: s.bounds}
		each := func(entry sftpListEntry) error {
			if err := kept.another(); err != nil {
				return err
			}
			if entry.name == "" || entry.name == "." || entry.name == ".." {
				return nil
			}
			folder := entry.kind == sftpTypeDir
			switch entry.kind {
			case sftpTypeLink:
				// A link is what it leads to, on the server.
				target, err := client.Stat(path.Join(dir, entry.name))
				if err != nil {
					return nil
				}
				if target.IsDir() && leadsBack(client, dir, entry.name) {
					// Unless it leads to a folder that holds this one:
					// its folders would be listed again under it.
					return nil
				}
				switch {
				case target.IsDir():
					folder = true
				case target.Mode().IsRegular():
					entry.size, entry.modified = target.Size(), target.ModTime()
				default:
					return nil
				}
			case sftpTypeDir, sftpTypeFile, 0:
			default:
				return nil
			}
			if folder {
				return kept.add(remoteEntry{name: entry.name, folder: true})
			}
			return kept.add(remoteEntry{name: entry.name, size: entry.size, modified: entry.modified})
		}
		err = s.listed(ctx, client, dir, each)
		if errors.Is(err, errSingleSession) {
			// The server's answers are read whole, then bounded.
			err = readDir(client, dir, each)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return kept.entries, nil
}

// listed lists the folder at dir over the connection client is on, as
// each entry comes (see sftpLister): errSingleSession when its server
// opens no channel for listings, or none within s.openTimeout, which is
// tried once a connection.
func (s *sftpShare) listed(ctx context.Context, client *sftp.Client, dir string, each func(entry sftpListEntry) error) error {
	s.listing.Lock()
	defer s.listing.Unlock()
	if s.lister != nil && s.lister.client != client {
		s.lister.close()
		s.lister = nil
	}
	if s.lister == nil {
		if s.single == client {
			return errSingleSession
		}
		s.mu.Lock()
		conn := s.conn
		open := s.client == client
		s.mu.Unlock()
		if !open {
			return errors.New("the connection closed")
		}
		lister, err := s.openLister(ctx, conn, client)
		if err != nil {
			// A server that still answers over the client's channel
			// refused the one for listings, ended it at once, or left it
			// unanswered.
			if _, alive := client.Getwd(); alive != nil || ctx.Err() != nil {
				return err
			}
			s.single = client
			if s.keys.single != nil {
				s.keys.single(s.host)
			}
			return errSingleSession
		}
		s.lister = lister
	}
	lister := s.lister
	// A listing whose context ends closes its channel, which ends the
	// reads it waits on.
	stop := context.AfterFunc(ctx, lister.close)
	err := lister.list(dir, s.bounds, each)
	var status *sftp.StatusError
	if !stop() || err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrPermission) && !errors.As(err, &status) {
		lister.close()
		s.lister = nil
	}
	return err
}

// openLister opens the channel of conn for listings, beside client's (see
// openSFTPLister), within s.openTimeout: errOpenLate past it. Closing a
// channel whose server leaves a request unanswered does not end the wait
// for the answer; only the connection's end does. So the channel is opened
// in a goroutine of its own, which closes it when it opens after the
// caller left, and ends with the connection when it never opens. A panic
// there is logged with its stack and drops the connection, as one in a
// request does (see do).
func (s *sftpShare) openLister(ctx context.Context, conn *ssh.Client, client *sftp.Client) (*sftpLister, error) {
	type opening struct {
		lister *sftpLister
		err    error
	}
	opened, left := make(chan opening), make(chan struct{})
	go func() {
		var o opening
		defer func() {
			if cause := recover(); cause != nil {
				s.drop(client)
				s.logger.Error("A network share's request ended on a panic", "host", s.host, "panic", fmt.Sprint(cause),
					"stack", string(debug.Stack()))
				o = opening{err: fmt.Errorf("the opening of the session for listings ended on a panic: %v", cause)}
			}
			select {
			case opened <- o:
			case <-left:
				if o.lister != nil {
					o.lister.close()
				}
			}
		}()
		o.lister, o.err = openSFTPLister(conn, client)
	}()
	timer := time.NewTimer(s.openTimeout)
	defer timer.Stop()
	select {
	case o := <-opened:
		return o.lister, o.err
	case <-timer.C:
		close(left)
		return nil, errOpenLate
	case <-ctx.Done():
		close(left)
		return nil, ctx.Err()
	}
}

// readDir lists the folder at dir through client, which reads all of its
// entries before it gives any, and gives each entry to each in turn.
func readDir(client *sftp.Client, dir string, each func(entry sftpListEntry) error) error {
	infos, err := client.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, info := range infos {
		entry := sftpListEntry{name: info.Name(), size: info.Size(), modified: info.ModTime()}
		switch mode := info.Mode(); {
		case mode.IsDir():
			entry.kind = sftpTypeDir
		case mode&fs.ModeSymlink != 0:
			entry.kind = sftpTypeLink
		case mode.IsRegular():
			entry.kind = sftpTypeFile
		default:
			// Another type, which a listing leaves out.
			entry.kind = sftpTypeMask
		}
		if err := each(entry); err != nil {
			return err
		}
	}
	return nil
}

// leadsBack tells whether a link of the folder dir leads to dir or to a
// folder above it: entered, it would list the same folders again under
// itself, without end. The target is the one the server resolves the
// link to and, for a server that resolves none, the one the link names;
// the folder is at dir, and where the server resolves dir to. A link
// that leads back by another way is entered, and the bounds of the walk
// end what it starts.
func leadsBack(client *sftp.Client, dir, name string) bool {
	link := path.Join(dir, name)
	dirs := []string{dir}
	if real, err := client.RealPath(dir); err == nil && real != dir {
		dirs = append(dirs, real)
	}
	holds := func(target string) bool {
		target = path.Clean(target)
		for _, dir := range dirs {
			if target == "/" || target == dir || strings.HasPrefix(dir, target+"/") {
				return true
			}
		}
		return false
	}
	if real, err := client.RealPath(link); err == nil && holds(real) {
		return true
	}
	target, err := client.ReadLink(link)
	if err != nil {
		return false
	}
	if !path.IsAbs(target) {
		target = path.Join(dir, target)
	}
	return holds(target)
}

func (s *sftpShare) open(ctx context.Context, rel string) (io.ReadSeekCloser, time.Time, error) {
	file, info, err := s.openFile(ctx, s.file(rel))
	if err != nil {
		return nil, time.Time{}, err
	}
	return file, info.ModTime(), nil
}

// openFile opens a file of the server, by its path there.
func (s *sftpShare) openFile(ctx context.Context, name string) (*sftpFile, fs.FileInfo, error) {
	var file *sftpFile
	var info fs.FileInfo
	err := s.do(ctx, func(client *sftp.Client) error {
		opened, err := client.Open(name)
		if err != nil {
			return err
		}
		if info, err = opened.Stat(); err != nil {
			opened.Close()
			return err
		}
		if !info.Mode().IsRegular() {
			opened.Close()
			return fs.ErrNotExist
		}
		file = &sftpFile{share: s, client: client, file: opened}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
}

// sftpFile reads a file of an SFTP folder over the connection client. Its
// reads run the SFTP client outside do, in the reader's goroutine, which
// no request's handler or scan may stand above: a panic of the client
// fails the read, the seek or the close, and drops the connection (see
// sftpShare.panicked).
type sftpFile struct {
	share  *sftpShare
	client *sftp.Client
	// file is the file the client opened, an *sftp.File.
	file interface {
		io.ReadSeekCloser
		io.ReaderAt
	}
}

func (f *sftpFile) Read(p []byte) (n int, err error) {
	defer f.failOnPanic(&err)
	return f.file.Read(p)
}

func (f *sftpFile) ReadAt(p []byte, off int64) (n int, err error) {
	defer f.failOnPanic(&err)
	return f.file.ReadAt(p, off)
}

func (f *sftpFile) Seek(offset int64, whence int) (at int64, err error) {
	defer f.failOnPanic(&err)
	return f.file.Seek(offset, whence)
}

func (f *sftpFile) Close() (err error) {
	defer f.failOnPanic(&err)
	return f.file.Close()
}

// failOnPanic, deferred by a method of the file, turns a panic of it into
// its error.
func (f *sftpFile) failOnPanic(err *error) {
	if cause := recover(); cause != nil {
		*err = f.share.panicked(f.client, cause)
	}
}

// location reads a file through the source cache in blocks over the
// folder's SSH connection (see source.FileOpener).
func (s *sftpShare) location(rel string) source.Location {
	u := url.URL{Scheme: "sftp", Host: s.host, Path: s.file(rel)}
	return source.Location{URL: u.String(), Opener: source.FileOpener(func(ctx context.Context, target string) (source.RandomFile, int64, error) {
		parsed, err := url.Parse(target)
		if err != nil {
			return nil, 0, err
		}
		file, info, err := s.openFile(ctx, parsed.Path)
		if err != nil {
			return nil, 0, err
		}
		return file, info.Size(), nil
	})}
}

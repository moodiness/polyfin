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

func newSFTP(u *url.URL, access shareAccess, keys hostKeys, logger *slog.Logger) (*sftpShare, error) {
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "22")
	}
	folder := u.Path
	if folder == "" {
		folder = "/"
	}
	s := &sftpShare{host: host, folder: folder, keys: keys, saved: keys.saved, bounds: folderBounds, logger: logger}
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
// none. The first connection saves the host key the server offers.
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
// the caller's goroutine, a scan's or a request's: a panic of what reads
// them, the SSH and SFTP clients or the listing's own reader (see
// sftpLister), is logged with its stack and fails the request, and the
// connection, whose state is then not known, is dropped.
func (s *sftpShare) do(ctx context.Context, request func(client *sftp.Client) error) (err error) {
	var client *sftp.Client
	defer func() {
		if cause := recover(); cause != nil {
			if client != nil {
				s.drop(client)
			}
			s.logger.Error("A network share's request ended on a panic", "host", s.host, "panic", fmt.Sprint(cause),
				"stack", string(debug.Stack()))
			err = fmt.Errorf("%w: the request ended on a panic: %v", errShareUnreachable, cause)
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
// opens no channel for listings, which is tried once a connection.
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
		lister, err := openSFTPLister(conn, client)
		if err != nil {
			// A server that still answers over the client's channel
			// refused the one for listings, or ended it at once.
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
func (s *sftpShare) openFile(ctx context.Context, name string) (*sftp.File, fs.FileInfo, error) {
	var file *sftp.File
	var info fs.FileInfo
	err := s.do(ctx, func(client *sftp.Client) error {
		var err error
		if file, err = client.Open(name); err != nil {
			return err
		}
		if info, err = file.Stat(); err != nil {
			file.Close()
			return err
		}
		if !info.Mode().IsRegular() {
			file.Close()
			return fs.ErrNotExist
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
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

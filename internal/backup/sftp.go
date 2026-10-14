package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpDialTimeout bounds connecting and signing in to an SFTP server.
const sftpDialTimeout = 30 * time.Second

// ErrHostKeyChanged reports an SFTP server whose host key is not the one
// saved at the first connection.
var ErrHostKeyChanged = errors.New("the SFTP server's host key is not the one saved at the first connection")

// sftpRemote is a folder of an SFTP server.
type sftpRemote struct {
	conn   *ssh.Client
	client *sftp.Client
	folder string
	stop   func() bool
}

// dialSFTP signs in to the SFTP server of d with password or privateKey.
// The server's host key must be d.HostKey; without one, any is accepted,
// and seen is called with it, in the authorized_keys format. A server that
// moves no byte for idle, while being asked or answering, fails what was
// asked (see idleConn).
func dialSFTP(ctx context.Context, d Destination, password, privateKey string, idle time.Duration,
	seen func(hostKey string)) (*sftpRemote, error) {
	var auth []ssh.AuthMethod
	if privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return nil, ErrInvalidPrivateKey
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if password != "" {
		auth = append(auth, ssh.Password(password), ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = password
			}
			return answers, nil
		}))
	}
	var offered string
	config := &ssh.ClientConfig{
		User: d.Username,
		Auth: auth,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			line := string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(key)))
			if d.HostKey == "" {
				offered = line
				return nil
			}
			saved, _, _, _, err := ssh.ParseAuthorizedKey([]byte(d.HostKey))
			if err != nil || !bytes.Equal(saved.Marshal(), key.Marshal()) {
				return fmt.Errorf("%w: it now offers %s", ErrHostKeyChanged, ssh.FingerprintSHA256(key))
			}
			return nil
		},
		Timeout: sftpDialTimeout,
	}
	address := net.JoinHostPort(d.Address, strconv.Itoa(d.Port))
	dialCtx, cancel := context.WithTimeout(ctx, sftpDialTimeout)
	defer cancel()
	raw, err := dialIdle(dialCtx, "tcp", address, idle)
	if err != nil {
		return nil, err
	}
	// Signing in, like every later request, ends with ctx.
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	conn, channels, requests, err := ssh.NewClientConn(raw, address, config)
	if err != nil {
		stop()
		raw.Close()
		return nil, err
	}
	sshClient := ssh.NewClient(conn, channels, requests)
	client, err := sftp.NewClient(sshClient, sftp.UseConcurrentWrites(true))
	if err != nil {
		stop()
		sshClient.Close()
		return nil, err
	}
	if offered != "" && seen != nil {
		seen(offered)
	}
	return &sftpRemote{conn: sshClient, client: client, folder: d.Folder, stop: stop}, nil
}

func (s *sftpRemote) close() error {
	s.stop()
	s.client.Close()
	return s.conn.Close()
}

// file is name's path on the server: in the folder, or in the user's
// home without one.
func (s *sftpRemote) file(name string) string {
	if s.folder == "" {
		return name
	}
	return path.Join(s.folder, name)
}

func (s *sftpRemote) put(_ context.Context, name string, body io.ReadSeeker, _ int64) error {
	temporary := s.file(partial(name))
	f, err := s.client.OpenFile(temporary, writeFlags)
	if err != nil {
		return fmt.Errorf("create %s: %w", temporary, err)
	}
	_, err = f.ReadFrom(body)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = s.client.Remove(temporary)
		return fmt.Errorf("write %s: %w", temporary, err)
	}
	// Backups are never overwritten: the rename fails when name exists,
	// as with SFTP's own rename, which the POSIX one is used in place of
	// only when the server offers it.
	if _, err := s.client.Lstat(s.file(name)); err == nil {
		_ = s.client.Remove(temporary)
		return fmt.Errorf("%s already exists", name)
	}
	if err := s.client.Rename(temporary, s.file(name)); err != nil {
		_ = s.client.Remove(temporary)
		return fmt.Errorf("rename %s: %w", temporary, err)
	}
	return nil
}

// writeFlags creates a file, or empties one a failed upload left.
const writeFlags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC

func (s *sftpRemote) list(context.Context) ([]remoteFile, error) {
	folder := s.folder
	if folder == "" {
		folder = "."
	}
	entries, err := s.client.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", folder, err)
	}
	var files []remoteFile
	for _, entry := range entries {
		if entry.Mode().IsRegular() {
			files = append(files, remoteFile{name: entry.Name(), size: entry.Size()})
		}
	}
	return files, nil
}

func (s *sftpRemote) get(_ context.Context, name string, w io.Writer) error {
	f, err := s.client.Open(s.file(name))
	if errors.Is(err, fs.ErrNotExist) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteTo(w)
	return err
}

func (s *sftpRemote) remove(_ context.Context, name string) error {
	err := s.client.Remove(s.file(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

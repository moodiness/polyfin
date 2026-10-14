package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// remote is a destination's storage, its folder opened: S3-compatible
// storage, an SFTP server or a WebDAV server. Every name is a file's base
// name in the destination's folder.
type remote interface {
	// put writes size bytes of body as name. SFTP and WebDAV write it under
	// name's partial name first, and rename it once complete; an S3 object
	// only appears once whole.
	put(ctx context.Context, name string, body io.ReadSeeker, size int64) error
	// list returns the files of the folder, without the folders in it.
	list(ctx context.Context) ([]remoteFile, error)
	// get writes name's content to w.
	get(ctx context.Context, name string, w io.Writer) error
	// remove deletes name; a name already gone is not an error.
	remove(ctx context.Context, name string) error
	close() error
}

// remoteFile is a file in a destination's folder: its size is unknownSize
// when the destination does not tell it.
type remoteFile struct {
	name string
	size int64
}

// unknownSize is the size of a file whose destination does not tell it.
const unknownSize = -1

// maxDownload bounds a download whose size the destination does not
// tell, as a backup an administrator sends is.
const maxDownload = 64 << 30

// partial is the name a file is written under before it is complete, as in
// the backup folder.
func partial(name string) string {
	return "." + name + ".partial"
}

// DefaultIdleTimeout is how long a destination may move no byte, in either
// direction, before its connection is closed and what it was doing fails:
// a server that stalls mid-upload or mid-answer is a failure of that
// destination, never a backup run held forever.
const DefaultIdleTimeout = 2 * time.Minute

// idleConn is a connection that fails once idle passes without a byte
// moved in either direction. A byte moved one way pushes back the
// deadline of both: net/http keeps a read pending while it sends a
// request's body, and SSH a read for as long as the connection is open,
// and those reads must not time out while the upload goes on.
type idleConn struct {
	net.Conn
	idle time.Duration
}

// progress pushes back the deadline of reads and writes, pending ones
// included.
func (c idleConn) progress() error {
	return c.Conn.SetDeadline(time.Now().Add(c.idle))
}

func (c idleConn) Read(p []byte) (int, error) {
	if err := c.progress(); err != nil {
		return 0, err
	}
	n, err := c.Conn.Read(p)
	if n > 0 && err == nil {
		err = c.progress()
	}
	return n, err
}

func (c idleConn) Write(p []byte) (int, error) {
	if err := c.progress(); err != nil {
		return 0, err
	}
	n, err := c.Conn.Write(p)
	if n > 0 && err == nil {
		err = c.progress()
	}
	return n, err
}

// dialIdle dials address, the connection closed when idle passes without
// a byte moved (see idleConn).
func dialIdle(ctx context.Context, network, address string, idle time.Duration) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: idle}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return idleConn{Conn: conn, idle: idle}, nil
}

// newHTTPClient is the client of S3 and WebDAV destinations: a server that
// moves no byte for idle, while being asked, sent a body or answering,
// fails the request rather than holding the run forever. Redirects are not
// followed: a request is signed for its address, and a body is not sent
// twice.
func newHTTPClient(idle time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialIdle(ctx, network, address, idle)
	}
	transport.ResponseHeaderTimeout = idle
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// closeHTTP closes the idle connections of client, those of a destination
// done with.
func closeHTTP(client *http.Client) error {
	client.CloseIdleConnections()
	return nil
}

// errTooLarge reports a file larger than expected from a destination.
var errTooLarge = errors.New("the destination sends more bytes than the file has")

// cappedWriter writes at most max bytes to w, and fails with errTooLarge
// beyond.
type cappedWriter struct {
	w   io.Writer
	max int64
	n   int64
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.n+int64(len(p)) > c.max {
		return 0, errTooLarge
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// maxErrorBody bounds what is read of an HTTP server's error answer.
const maxErrorBody = 4096

// httpError is the error of an HTTP answer with an unexpected status: what
// was asked, the status, and the start of the answer, a line at most.
func httpError(server, what string, response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	detail := strings.TrimSpace(string(body))
	if code, message := xmlErrorText(body); code != "" || message != "" {
		detail = strings.TrimSpace(code + " " + message)
	}
	if line, _, _ := strings.Cut(detail, "\n"); len(line) > 200 {
		detail = line[:200] + "…"
	} else {
		detail = line
	}
	if detail != "" {
		return fmt.Errorf("%s answered %s to %s: %s", server, response.Status, what, detail)
	}
	return fmt.Errorf("%s answered %s to %s", server, response.Status, what)
}

// errNotFound reports a file a destination does not have.
var errNotFound = errors.New("no such file at the destination")

package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// remoteFile is a file in a destination's folder.
type remoteFile struct {
	name string
	size int64
}

// partial is the name a file is written under before it is complete, as in
// the backup folder.
func partial(name string) string {
	return "." + name + ".partial"
}

// httpTimeout bounds waiting for an S3 or WebDAV server to start
// answering: a backup's upload itself may take long.
const httpTimeout = 2 * time.Minute

// newHTTPClient is the client of S3 and WebDAV destinations: a server that
// stops answering fails the upload rather than holding the run forever.
// Redirects are not followed: a request is signed for its address, and a
// body is not sent twice.
func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = httpTimeout
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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

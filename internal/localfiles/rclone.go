package localfiles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/source"
)

// rcloneShare reads a folder of an rclone remote through an rclone
// instance serving its remote control interface with --rc-serve (rclone
// rcd --rc-serve): its folders listed with operations/list, its files read
// by ranges from the objects the instance serves, over HTTP, signed in as
// the instance's user (--rc-user and --rc-pass) when it has one.
type rcloneShare struct {
	// base is the instance's address; folder the folder's path in the
	// remote, without its slashes.
	base           *url.URL
	remote, folder string
	user, password string
	client         *http.Client
}

func newRclone(u *url.URL, access shareAccess) *rcloneShare {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &rcloneShare{base: &url.URL{Scheme: "http", Host: u.Host}, remote: access.remote, folder: strings.Trim(u.Path, "/"),
		user: access.user, password: access.password, client: &http.Client{Transport: transport}}
}

func (c *rcloneShare) close() { c.client.CloseIdleConnections() }

// send sends a request to the instance, signed in when it has a user or
// a password.
func (c *rcloneShare) send(ctx context.Context, method string, u *url.URL, header http.Header) (*http.Response, error) {
	return c.sendBody(ctx, method, u, header, nil)
}

func (c *rcloneShare) sendBody(ctx context.Context, method string, u *url.URL, header http.Header, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	for name, values := range header {
		request.Header[name] = values
	}
	if c.user != "" || c.password != "" {
		request.SetBasicAuth(c.user, c.password)
	}
	response, err := c.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
	}
	return response, nil
}

// Open sends the source cache's requests of the objects the instance
// serves, signed in (see source.Location.Opener).
func (c *rcloneShare) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	return c.send(ctx, method, u, header)
}

// errRclone is an error the instance answered a call with: status and
// what it said.
type errRclone struct {
	status  int
	message string
}

func (e *errRclone) Error() string { return fmt.Sprintf("rclone answered %d: %s", e.status, e.message) }

// call calls a method of the remote control interface with input, and
// reads its answer into output. A method the instance refuses the user
// for is errShareRefused, one finding nothing fs.ErrNotExist; another
// error it answers is an *errRclone.
func (c *rcloneShare) call(ctx context.Context, method string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	u := *c.base
	u.Path = "/" + method
	response, err := c.sendBody(ctx, http.MethodPost, &u, http.Header{"Content-Type": {"application/json"}}, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(io.LimitReader(response.Body, 256<<20)).Decode(output); err != nil {
			return fmt.Errorf("%w: %v", errShareUnreachable, err)
		}
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return errShareRefused
	case http.StatusNotFound:
		return fs.ErrNotExist
	}
	var answer struct {
		Error string `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answer) != nil || answer.Error == "" {
		return statusError(response.StatusCode)
	}
	return &errRclone{status: response.StatusCode, message: answer.Error}
}

// RcloneRemotes lists the remotes the rclone instance at address knows,
// http://host:port or an rclone folder's address, signed in as user with
// password; a password left out is the one stored for the folder, when
// one is given, as a folder's form lists them again. failure tells why
// the instance could not list them, "unreachable" or "refused", as a
// folder's Error; the error is the request's, ErrInvalidAddress for an
// address without a host.
func (s *Service) RcloneRemotes(ctx context.Context, address, user string, password *string, folder *accounts.ID) (remotes []string,
	failure string, err error) {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Hostname() == "" || u.User != nil || len(address) > 4096 {
		return nil, "", ErrInvalidAddress
	}
	if user, err = validCredentials(user, ""); err != nil {
		return nil, "", err
	}
	secret := ""
	switch {
	case password != nil:
		secret = *password
	case folder != nil:
		var sealed string
		err := s.db.QueryRow(ctx, "SELECT share_password FROM local_folders WHERE addon_id = $1", *folder).Scan(&sealed)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", addons.ErrNotFound
		}
		if err != nil {
			return nil, "", err
		}
		if secret, err = s.box.Open(sealed); err != nil {
			return nil, errRefused, nil
		}
	}
	listCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	c := newRclone(&url.URL{Host: u.Host}, shareAccess{user: user, password: secret})
	defer c.close()
	remotes, err = c.remotes(listCtx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if failure = shareFailure(err); failure != errRefused {
			failure = errUnreachable
		}
		return nil, failure, nil
	}
	return remotes, "", nil
}

// remotes lists the remotes the instance knows.
func (c *rcloneShare) remotes(ctx context.Context) ([]string, error) {
	var answer struct {
		Remotes []string `json:"remotes"`
	}
	if err := c.call(ctx, "config/listremotes", struct{}{}, &answer); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// An address answering without the interface.
			return nil, fmt.Errorf("%w: no remote control interface", errShareUnreachable)
		}
		return nil, err
	}
	slices.Sort(answer.Remotes)
	return answer.Remotes, nil
}

// rcloneItem is an entry operations/list and operations/stat answer.
type rcloneItem struct {
	Name    string `json:"Name"`
	Size    int64  `json:"Size"`
	ModTime string `json:"ModTime"`
	IsDir   bool   `json:"IsDir"`
}

// fsName is the remote as rclone names it, and remotePath a path in it.
func (c *rcloneShare) fsName() string { return c.remote + ":" }

func (c *rcloneShare) remotePath(rel string) string {
	return strings.TrimPrefix(path.Join(c.folder, rel), "/")
}

func (c *rcloneShare) list(ctx context.Context, rel string) ([]remoteEntry, error) {
	var answer struct {
		List []rcloneItem `json:"list"`
	}
	err := c.call(ctx, "operations/list", map[string]string{"fs": c.fsName(), "remote": c.remotePath(rel)}, &answer)
	if err != nil {
		return nil, c.listFailure(ctx, rel, err)
	}
	entries := make([]remoteEntry, 0, len(answer.List))
	for _, item := range answer.List {
		if item.Name == "" || strings.Contains(item.Name, "/") {
			continue
		}
		entry := remoteEntry{name: item.Name, folder: item.IsDir}
		if !item.IsDir {
			entry.size = item.Size
			entry.modified, _ = time.Parse(time.RFC3339Nano, item.ModTime)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// listFailure tells why a folder could not be listed: a remote the
// instance does not know is missing, as a folder missing in it is, and a
// folder that is a file is no folder.
func (c *rcloneShare) listFailure(ctx context.Context, rel string, err error) error {
	var answered *errRclone
	switch {
	case errors.Is(err, fs.ErrNotExist):
		var stat struct {
			Item *rcloneItem `json:"item"`
		}
		if c.call(ctx, "operations/stat", map[string]string{"fs": c.fsName(), "remote": c.remotePath(rel)}, &stat) == nil && stat.Item != nil &&
			!stat.Item.IsDir {
			return errIsFile
		}
		return fs.ErrNotExist
	case errors.As(err, &answered):
		remotes, listErr := c.remotes(ctx)
		if listErr == nil && !slices.Contains(remotes, c.remote) {
			return fmt.Errorf("%w: rclone knows no remote %q", fs.ErrNotExist, c.remote)
		}
		return fmt.Errorf("%w: %v", errShareUnreachable, err)
	}
	return err
}

// object is the address the instance serves a file of the folder at:
// /[remote:]/path.
func (c *rcloneShare) object(rel string) *url.URL {
	u := *c.base
	u.Path = "/[" + c.fsName() + "]/" + c.remotePath(rel)
	return &u
}

func (c *rcloneShare) open(ctx context.Context, rel string) (io.ReadSeekCloser, time.Time, error) {
	u := c.object(rel)
	response, err := c.send(ctx, http.MethodHead, u, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength < 0 {
		if response.StatusCode == http.StatusOK {
			return nil, time.Time{}, fmt.Errorf("%w: no length", errShareUnreachable)
		}
		return nil, time.Time{}, statusError(response.StatusCode)
	}
	modified, _ := http.ParseTime(response.Header.Get("Last-Modified"))
	return &httpFile{send: c.send, ctx: ctx, address: u, size: response.ContentLength}, modified, nil
}

func (c *rcloneShare) location(rel string) source.Location {
	return source.Location{URL: c.object(rel).String(), Opener: c}
}

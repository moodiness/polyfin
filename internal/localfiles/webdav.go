package localfiles

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// propfind asks a WebDAV server for what a scan reads of a folder's
// entries.
const propfind = `<?xml version="1.0" encoding="utf-8"?>` +
	`<propfind xmlns="DAV:"><prop><resourcetype/><getcontentlength/><getlastmodified/></prop></propfind>`

// drainLimit is how much of a file's answer left unread is read to the end
// so that its connection serves the next request; more, the connection is
// closed.
const drainLimit = 256 << 10

// webdavShare reads a WebDAV folder through connections kept alive
// between requests.
type webdavShare struct {
	// base is the folder's address, its path ending with a slash.
	base           *url.URL
	user, password string
	client         *http.Client
	// bounds bound what is read and kept of one folder's listing.
	bounds listBounds
	// deep tells whether the share is listed deep (see deepRemote).
	deep deepLists
}

func newWebDAV(u *url.URL, user, password string) *webdavShare {
	base := *u
	base.Path = strings.TrimSuffix(base.Path, "/") + "/"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &webdavShare{base: &base, user: user, password: password, client: &http.Client{Transport: transport}, bounds: folderBounds}
}

func (c *webdavShare) close() { c.client.CloseIdleConnections() }

// address is the address of a file of the folder, or of a folder of it
// when folder.
func (c *webdavShare) address(rel string, folder bool) *url.URL {
	u := *c.base
	if rel != "" {
		u.Path += rel
		if folder {
			u.Path += "/"
		}
	}
	return &u
}

// request sends a request to the server, signed in when the share has a
// user or password.
func (c *webdavShare) request(ctx context.Context, method string, u *url.URL, header http.Header, body string) (*http.Response, error) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
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

// statusError classes a status the server answered with (see
// shareFailure).
func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return errShareRefused
	case http.StatusForbidden:
		return fs.ErrPermission
	case http.StatusNotFound, http.StatusGone:
		return fs.ErrNotExist
	}
	return fmt.Errorf("%w: status %d", errShareUnreachable, status)
}

// davResponse is one entry of a PROPFIND's answer: Status is set for one
// the server could not list.
type davResponse struct {
	Href     string `xml:"DAV: href"`
	Status   string `xml:"DAV: status"`
	Propstat []struct {
		Status string `xml:"DAV: status"`
		Prop   struct {
			ResourceType struct {
				Collection *struct{} `xml:"DAV: collection"`
			} `xml:"DAV: resourcetype"`
			Length   string `xml:"DAV: getcontentlength"`
			Modified string `xml:"DAV: getlastmodified"`
		} `xml:"DAV: prop"`
	} `xml:"DAV: propstat"`
}

// propfind lists the folder at rel to depth, "1" or "infinity", within
// b: each is given each entry of the answer in turn, with its path,
// decoded and clean, and its properties, nil when the server gave none;
// failed tells an entry the server answered with a failure of its own.
// The answer is read entry by entry, never whole, each within the bounds
// of one. read tells an error met reading the answer, returned as is; the
// others are classed (see statusError).
func (c *webdavShare) propfind(ctx context.Context, rel, depth string, b listBounds,
	each func(entryPath string, entry *remoteEntry, failed bool) error) (read bool, err error) {
	response, err := c.request(ctx, "PROPFIND", c.address(rel, true),
		http.Header{"Depth": {depth}, "Content-Type": {"application/xml; charset=utf-8"}}, propfind)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMultiStatus {
		return false, statusError(response.StatusCode)
	}
	return true, eachElement(response.Body, b, func(decoder *xml.Decoder, start xml.StartElement) error {
		if start.Name.Space != "DAV:" || start.Name.Local != "response" {
			return decoder.Skip()
		}
		var r davResponse
		if err := decoder.DecodeElement(&r, &start); err != nil {
			return err
		}
		href, err := url.Parse(r.Href)
		if err != nil {
			return nil
		}
		entryPath := path.Clean(href.Path)
		var entry *remoteEntry
		for _, p := range r.Propstat {
			if !strings.Contains(p.Status, " 200 ") {
				continue
			}
			size, _ := strconv.ParseInt(strings.TrimSpace(p.Prop.Length), 10, 64)
			modified, _ := http.ParseTime(strings.TrimSpace(p.Prop.Modified))
			entry = &remoteEntry{name: path.Base(entryPath), folder: p.Prop.ResourceType.Collection != nil, size: size, modified: modified}
		}
		return each(entryPath, entry, r.Status != "" && !strings.Contains(r.Status, " 200 "))
	})
}

func (c *webdavShare) list(ctx context.Context, rel string) ([]remoteEntry, error) {
	kept := folderList{bounds: c.bounds}
	self := path.Clean(c.address(rel, true).Path)
	read, err := c.propfind(ctx, rel, "1", c.bounds, func(entryPath string, entry *remoteEntry, _ bool) error {
		if err := kept.another(); err != nil {
			return err
		}
		switch {
		case entry == nil:
		case entryPath == self:
			if !entry.folder {
				return errIsFile
			}
		case path.Dir(entryPath) == self:
			return kept.keep(*entry)
		}
		return nil
	})
	var bound *shareBound
	switch {
	case err == nil:
		return kept.entries, nil
	case !read, errors.Is(err, errIsFile), errors.As(err, &bound):
		return nil, err
	}
	return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
}

// listDeep lists the share's folder with PROPFIND at infinite depth,
// within deepBounds. A server that sends nothing for listTimeout ends it,
// as one ends the listing of a folder. Servers that refuse such a listing
// answer it with a failure, a 403 most often.
func (c *webdavShare) listDeep(ctx context.Context, each func(dir string, e remoteEntry) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(listTimeout, cancel)
	defer idle.Stop()
	self := path.Clean(c.base.Path)
	prefix := strings.TrimSuffix(self, "/") + "/"
	entries := 0
	_, err := c.propfind(ctx, "", "infinity", deepBounds, func(entryPath string, entry *remoteEntry, failed bool) error {
		idle.Reset(listTimeout)
		if entries++; entries > deepBounds.entries {
			return &shareBound{failure: errBoundListing, detail: fmt.Sprintf("more than %d entries in a deep listing", deepBounds.entries)}
		}
		rel, under := strings.CutPrefix(entryPath, prefix)
		switch {
		case failed:
			return fmt.Errorf("the server could not list %.200q", entryPath)
		case entry == nil:
			return nil
		case entryPath == self:
			if !entry.folder {
				return errIsFile
			}
			return nil
		case !under:
			return nil
		case len(entry.name) > deepBounds.name:
			return &shareBound{failure: errBoundListing, detail: fmt.Sprintf("a name of more than %d bytes", deepBounds.name)}
		}
		dir := path.Dir(rel)
		if dir == "." {
			dir = ""
		}
		e := *entry
		e.name = strings.Clone(e.name)
		return each(dir, e)
	})
	return err
}

func (c *webdavShare) deepLists() *deepLists { return &c.deep }

func (c *webdavShare) open(ctx context.Context, rel string) (io.ReadSeekCloser, time.Time, error) {
	u := c.address(rel, false)
	response, err := c.request(ctx, http.MethodHead, u, nil, "")
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

// send sends a request without a body.
func (c *webdavShare) send(ctx context.Context, method string, u *url.URL, header http.Header) (*http.Response, error) {
	return c.request(ctx, method, u, header, "")
}

// sender sends a request of a share's file without a body, signed in as
// the share asks.
type sender func(ctx context.Context, method string, u *url.URL, header http.Header) (*http.Response, error)

// httpFile reads a file of a share served over HTTP, a WebDAV folder, an
// S3 bucket or an rclone remote: from an offset on, through one request
// until the next seek.
type httpFile struct {
	send    sender
	ctx     context.Context
	address *url.URL
	size    int64
	offset  int64
	// body is the answer read from offset on; nil until read, and after a
	// seek. left is what it has not given yet.
	body io.ReadCloser
	left int64
}

func (f *httpFile) Read(p []byte) (int, error) {
	if f.offset >= f.size {
		return 0, io.EOF
	}
	if f.body == nil {
		if err := f.request(); err != nil {
			return 0, err
		}
	}
	n, err := f.body.Read(p)
	f.offset += int64(n)
	f.left -= int64(n)
	if errors.Is(err, io.EOF) && f.offset < f.size {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

// request asks for the file from offset on.
func (f *httpFile) request() error {
	response, err := f.send(f.ctx, http.MethodGet, f.address, http.Header{"Range": {fmt.Sprintf("bytes=%d-", f.offset)}})
	if err != nil {
		return err
	}
	switch response.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		// A server ignoring ranges answers the whole file.
		if _, err := io.CopyN(io.Discard, response.Body, f.offset); err != nil {
			response.Body.Close()
			return fmt.Errorf("%w: %v", errShareUnreachable, err)
		}
	default:
		response.Body.Close()
		return statusError(response.StatusCode)
	}
	f.body, f.left = response.Body, f.size-f.offset
	return nil
}

func (f *httpFile) Seek(offset int64, whence int) (int64, error) {
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

// release ends the request under way: its connection serves the next
// request when little of the answer is left.
func (f *httpFile) release() {
	if f.body == nil {
		return
	}
	if f.left <= drainLimit {
		_, _ = io.Copy(io.Discard, io.LimitReader(f.body, drainLimit))
	}
	f.body.Close()
	f.body = nil
}

func (f *httpFile) Close() error {
	f.release()
	return nil
}

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
}

func newWebDAV(u *url.URL, user, password string) *webdavShare {
	base := *u
	base.Path = strings.TrimSuffix(base.Path, "/") + "/"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &webdavShare{base: &base, user: user, password: password, client: &http.Client{Transport: transport}}
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

// davResponse is one entry of a PROPFIND's answer.
type davResponse struct {
	Href     string `xml:"DAV: href"`
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

func (c *webdavShare) list(ctx context.Context, rel string) ([]remoteEntry, error) {
	folder := c.address(rel, true)
	response, err := c.request(ctx, "PROPFIND", folder,
		http.Header{"Depth": {"1"}, "Content-Type": {"application/xml; charset=utf-8"}}, propfind)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMultiStatus {
		return nil, statusError(response.StatusCode)
	}
	// The answer is read entry by entry, never whole.
	decoder := xml.NewDecoder(response.Body)
	var entries []remoteEntry
	self := path.Clean(folder.Path)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Space != "DAV:" || start.Name.Local != "response" {
			continue
		}
		var r davResponse
		if err := decoder.DecodeElement(&r, &start); err != nil {
			return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
		}
		href, err := url.Parse(r.Href)
		if err != nil {
			continue
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
		switch {
		case entry == nil:
		case entryPath == self:
			if !entry.folder {
				return nil, errIsFile
			}
		case path.Dir(entryPath) == self:
			entries = append(entries, *entry)
		}
	}
}

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
	return &webdavFile{share: c, ctx: ctx, address: u, size: response.ContentLength}, modified, nil
}

// webdavFile reads a file of a WebDAV folder: from an offset on, through
// one request until the next seek.
type webdavFile struct {
	share   *webdavShare
	ctx     context.Context
	address *url.URL
	size    int64
	offset  int64
	// body is the answer read from offset on; nil until read, and after a
	// seek. left is what it has not given yet.
	body io.ReadCloser
	left int64
}

func (f *webdavFile) Read(p []byte) (int, error) {
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
func (f *webdavFile) request() error {
	response, err := f.share.request(f.ctx, http.MethodGet, f.address, http.Header{"Range": {fmt.Sprintf("bytes=%d-", f.offset)}}, "")
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

func (f *webdavFile) Seek(offset int64, whence int) (int64, error) {
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
func (f *webdavFile) release() {
	if f.body == nil {
		return
	}
	if f.left <= drainLimit {
		_, _ = io.Copy(io.Discard, io.LimitReader(f.body, drainLimit))
	}
	f.body.Close()
	f.body = nil
}

func (f *webdavFile) Close() error {
	f.release()
	return nil
}

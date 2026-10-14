package backup

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// webdavRemote is a folder of a WebDAV server, signed in to with basic
// authentication when a user is given.
type webdavRemote struct {
	client   *http.Client
	folder   *url.URL
	user     string
	password string
}

func newWebDAVRemote(d Destination, password string) (*webdavRemote, error) {
	folder, err := url.Parse(d.Address)
	if err != nil {
		return nil, ErrInvalidDestinationAddress
	}
	if !strings.HasSuffix(folder.Path, "/") {
		folder.Path += "/"
		folder.RawPath = ""
	}
	return &webdavRemote{client: newHTTPClient(), folder: folder, user: d.Username, password: password}, nil
}

func (w *webdavRemote) close() error { return nil }

// webdavServer names WebDAV destinations in errors.
const webdavServer = "The WebDAV server"

// address is name's address in the folder.
func (w *webdavRemote) address(name string) string {
	return w.folder.JoinPath(name).String()
}

func (w *webdavRemote) do(ctx context.Context, method, address string, body io.Reader, size int64, header http.Header) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.ContentLength = size
	}
	for key, values := range header {
		request.Header[key] = values
	}
	if w.user != "" || w.password != "" {
		request.SetBasicAuth(w.user, w.password)
	}
	return w.client.Do(request)
}

func (w *webdavRemote) put(ctx context.Context, name string, body io.ReadSeeker, size int64) error {
	temporary := partial(name)
	response, err := w.do(ctx, http.MethodPut, w.address(temporary), io.NopCloser(body), size, nil)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		return httpError(webdavServer, "the upload of "+name, response)
	}
	response, err = w.do(ctx, "MOVE", w.address(temporary), nil, 0, http.Header{"Destination": {w.address(name)}, "Overwrite": {"F"}})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusNoContent {
		return httpError(webdavServer, "the renaming of "+temporary, response)
	}
	return nil
}

// multistatus is the answer of PROPFIND.
type multistatus struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Prop struct {
				Length       string `xml:"getcontentlength"`
				ResourceType struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
			} `xml:"prop"`
			Status string `xml:"status"`
		} `xml:"propstat"`
	} `xml:"response"`
}

const propfind = `<?xml version="1.0" encoding="utf-8"?><propfind xmlns="DAV:"><prop><resourcetype/><getcontentlength/></prop></propfind>`

func (w *webdavRemote) list(ctx context.Context) ([]remoteFile, error) {
	response, err := w.do(ctx, "PROPFIND", w.folder.String(), strings.NewReader(propfind), int64(len(propfind)),
		http.Header{"Depth": {"1"}, "Content-Type": {"application/xml; charset=utf-8"}})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMultiStatus {
		return nil, httpError(webdavServer, "the list of the backups", response)
	}
	var result multistatus
	if err := xml.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result); err != nil {
		return nil, err
	}
	var files []remoteFile
	for _, r := range result.Responses {
		href, err := url.Parse(r.Href)
		if err != nil {
			continue
		}
		dir, name := path.Split(strings.TrimSuffix(href.Path, "/"))
		// The folder itself, and what is not directly in it.
		if strings.TrimSuffix(dir, "/") != strings.TrimSuffix(w.folder.Path, "/") || name == "" {
			continue
		}
		file := remoteFile{name: name}
		collection := false
		for _, propstat := range r.Propstat {
			if !strings.Contains(propstat.Status, " 200 ") && propstat.Status != "" {
				continue
			}
			if propstat.Prop.ResourceType.Collection != nil {
				collection = true
			}
			if size, err := strconv.ParseInt(strings.TrimSpace(propstat.Prop.Length), 10, 64); err == nil {
				file.size = size
			}
		}
		if !collection {
			files = append(files, file)
		}
	}
	return files, nil
}

func (w *webdavRemote) get(ctx context.Context, name string, writer io.Writer) error {
	response, err := w.do(ctx, http.MethodGet, w.address(name), nil, 0, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if response.StatusCode != http.StatusOK {
		return httpError(webdavServer, "the download of "+name, response)
	}
	_, err = io.Copy(writer, response.Body)
	return err
}

func (w *webdavRemote) remove(ctx context.Context, name string) error {
	response, err := w.do(ctx, http.MethodDelete, w.address(name), nil, 0, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNotFound {
		return httpError(webdavServer, "the deletion of "+name, response)
	}
	return nil
}

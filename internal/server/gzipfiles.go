package server

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"sync"
	"time"
)

// gzipFiles serves the files of a web application compressed: each file
// that gains from it is compressed once, on its first request, at the best
// level, and the compressed bytes are kept in memory for as long as the
// file keeps its size and modification time.
type gzipFiles struct {
	files   fs.FS
	mu      sync.Mutex
	entries map[string]*gzipFile
}

// gzipFile is the compressed copy of one version of a file.
type gzipFile struct {
	modTime time.Time
	size    int64
	once    sync.Once
	// compressible tells whether the file's type gains from compression.
	compressible bool
	// body is the compressed file, nil when the file could not be read or
	// compressing it saves nothing.
	body        []byte
	contentType string
	etag        string
	err         error
}

func newGzipFiles(files fs.FS) *gzipFiles {
	return &gzipFiles{files: files, entries: map[string]*gzipFile{}}
}

// serve answers with the compressed copy of the file name, whose
// information is info, and reports whether it did; when it did not, the
// caller sends the file as it is. contentType is the type the caller gives
// the file, empty for the one of its extension. Range requests and clients
// that do not take gzip get the file as it is. The compressed answer keeps
// the headers already set, such as Cache-Control, and has an ETag of its
// own, which If-None-Match is checked against.
func (g *gzipFiles) serve(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, contentType string) bool {
	if info.Size() < minCompressed {
		return false
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(name))
	}
	if contentType != "" && !compressibleType(contentType) {
		return false
	}
	header := w.Header()
	addVary(header)
	if r.Header.Get("Range") != "" || !acceptsGzip(r.Header) {
		return false
	}
	file := g.file(name, info, contentType)
	if file.body == nil {
		if file.compressible {
			keepIdentity(w)
		}
		return false
	}
	header.Set("Content-Type", file.contentType)
	header.Set("Content-Encoding", "gzip")
	header.Set("ETag", file.etag)
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(file.body))
	return true
}

// file returns the compressed copy of the version of name info describes,
// compressing it on first use; concurrent first requests wait for the one
// compression. A file that could not be read is tried again next time.
func (g *gzipFiles) file(name string, info fs.FileInfo, contentType string) *gzipFile {
	g.mu.Lock()
	file := g.entries[name]
	if file == nil || file.size != info.Size() || !file.modTime.Equal(info.ModTime()) {
		file = &gzipFile{modTime: info.ModTime(), size: info.Size()}
		g.entries[name] = file
	}
	g.mu.Unlock()
	file.once.Do(func() { file.compress(g.files, name, contentType) })
	if file.err != nil {
		g.mu.Lock()
		if g.entries[name] == file {
			delete(g.entries, name)
		}
		g.mu.Unlock()
	}
	return file
}

// compress reads the file and keeps its compressed copy, when it is
// smaller. A file of unknown type gets the type ServeContent would find.
func (f *gzipFile) compress(files fs.FS, name, contentType string) {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		f.err = err
		return
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	f.contentType = contentType
	if f.compressible = compressibleType(contentType); !f.compressible {
		return
	}
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	_, _ = w.Write(data)
	_ = w.Close()
	if b.Len() >= len(data) {
		return
	}
	// The buffer's spare capacity would stay allocated with it.
	f.body = bytes.Clone(b.Bytes())
	f.etag = gzipTag(etag(data))
}

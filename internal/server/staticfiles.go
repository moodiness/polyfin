package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
)

// staticFiles serves the files of a web application compressed: each file
// that gains from it is compressed with gzip once, on its first request,
// at the best level, and the compressed bytes are kept in memory for as
// long as the file keeps its size and modification time.
//
// Polyfin's own files (see newOwnFiles) also get:
//   - brotli, at its best level, made once in the background after the
//     file's first request, as it takes seconds for the admin app's
//     script: gzip goes to the clients that take brotli until it is made;
//   - an ETag of their content on every answer, compressed or not, so
//     that a file not cached for good is checked again with a 304.
type staticFiles struct {
	files fs.FS
	own   bool
	// turn is the turn the brotli copies of own files take (see
	// brotliTurn).
	turn chan struct{}
	mu   sync.Mutex
	// entries holds the files read, by name.
	entries map[string]*staticFile
}

// staticFile is what is kept of one version of a file.
type staticFile struct {
	modTime time.Time
	size    int64
	once    sync.Once
	// contentType is the type of the file.
	contentType string
	// tag is the ETag of the file as it is.
	tag string
	// compressible tells whether the file is large enough and of a type
	// that gains from compression.
	compressible bool
	// gzip is the compressed file, nil when compressing it saves nothing.
	gzip []byte
	// brotli is the brotli copy, nil until made, and when it saves nothing
	// over gzip; made once, for own files that have a gzip copy.
	brotli     atomic.Pointer[[]byte]
	brotliOnce sync.Once
	err        error
}

// brotliTurn lets one brotli compression run at a time: each takes a core
// for up to seconds, and memory, and none is urgent, as gzip is sent
// meanwhile. Tests give a server a turn of its own (see
// Options.brotliTurn), so that none waits for another's copies.
var brotliTurn = make(chan struct{}, 1)

// newGzipFiles serves the files of files, which may change while Polyfin
// runs, such as jellyfin-web's: gzip only, and a file that does not gain
// from it is left to the caller.
func newGzipFiles(files fs.FS) *staticFiles {
	return &staticFiles{files: files, entries: map[string]*staticFile{}}
}

// newOwnFiles serves Polyfin's own files: brotli and gzip, and an ETag on
// every file, which serve then always answers with. Their brotli copies
// are made in turn, which brotliTurn is but in tests.
func newOwnFiles(files fs.FS, turn chan struct{}) *staticFiles {
	return &staticFiles{files: files, own: true, turn: turn, entries: map[string]*staticFile{}}
}

// serve answers with the file name, whose information is info, and reports
// whether it did; when it did not, the caller sends the file as it is.
// contentType is the type the caller gives the file, empty for the one of
// its extension. Range requests, and clients that take neither brotli nor
// gzip, get the file as it is. The answer keeps the headers already set,
// such as Cache-Control, and each encoding has an ETag of its own, which
// If-None-Match is checked against.
//
// The files of newGzipFiles are served compressed only: serve leaves the
// others to the caller. Those of newOwnFiles are always served, unless
// they cannot be read.
func (s *staticFiles) serve(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, contentType string) bool {
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(name))
	}
	worth := info.Size() >= minCompressed && (contentType == "" || compressibleType(contentType))
	if !s.own && !worth {
		return false
	}
	header := w.Header()
	if worth {
		addVary(header)
	}
	ranged := r.Header.Get("Range") != ""
	if !s.own && (ranged || !acceptsGzip(r.Header)) {
		return false
	}
	file := s.file(name, info, contentType)
	if file.err != nil {
		return false
	}
	if held := heldVariant(r, file, s.own); held != "" && !ranged {
		header.Set("ETag", held)
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	var body []byte
	var encoding, tag string
	if !ranged && file.gzip != nil {
		if s.own && acceptsBrotli(r.Header) {
			if body = file.brotliCopy(s.files, name, s.turn); body != nil {
				encoding, tag = "br", codingTag(file.tag, "br")
			}
		}
		if body == nil && acceptsGzip(r.Header) {
			body, encoding, tag = file.gzip, "gzip", gzipTag(file.tag)
		}
	}
	if body == nil {
		if file.compressible {
			keepIdentity(w)
		}
		if !s.own {
			return false
		}
		return serveIdentity(w, r, s.files, name, info, file)
	}
	header.Set("Content-Type", file.contentType)
	header.Set("Content-Encoding", encoding)
	header.Set("ETag", tag)
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(body))
	return true
}

// versioned reports whether version names the content of the file name:
// the ETag of the file, without its quotes. A page that links a file with
// its version lets it be cached for good.
func (s *staticFiles) versioned(name string, info fs.FileInfo, version string) bool {
	if version == "" {
		return false
	}
	file := s.file(name, info, mime.TypeByExtension(path.Ext(name)))
	return file.err == nil && file.tag == `"`+version+`"`
}

// heldVariant returns the ETag, from the request's If-None-Match, of a
// variant of the file the client holds and still takes: as it is (own
// files only), in gzip, or in brotli (own files only), even while the
// brotli copy is being made, as each is as current as the file. A client
// that took gzip before brotli was made thus keeps its copy.
func heldVariant(r *http.Request, file *staticFile, own bool) string {
	list := strings.Join(r.Header.Values("If-None-Match"), ",")
	switch {
	case list == "":
		return ""
	case own && hasTag(list, file.tag):
		return file.tag
	case file.gzip == nil:
		return ""
	}
	if tag := gzipTag(file.tag); acceptsGzip(r.Header) && hasTag(list, tag) {
		return tag
	}
	if tag := codingTag(file.tag, "br"); own && acceptsBrotli(r.Header) && hasTag(list, tag) {
		return tag
	}
	return ""
}

// serveIdentity sends the file as it is, with its ETag.
func serveIdentity(w http.ResponseWriter, r *http.Request, files fs.FS, name string, info fs.FileInfo, file *staticFile) bool {
	opened, err := files.Open(name)
	if err != nil {
		return false
	}
	defer opened.Close()
	content, ok := opened.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(opened)
		if err != nil {
			return false
		}
		content = bytes.NewReader(data)
	}
	header := w.Header()
	header.Set("Content-Type", file.contentType)
	header.Set("ETag", file.tag)
	http.ServeContent(w, r, name, info.ModTime(), content)
	return true
}

// file returns what is kept of the version of name info describes,
// reading the file on first use; concurrent first requests wait for the
// one read. A file that could not be read is tried again next time.
func (s *staticFiles) file(name string, info fs.FileInfo, contentType string) *staticFile {
	s.mu.Lock()
	file := s.entries[name]
	if file == nil || file.size != info.Size() || !file.modTime.Equal(info.ModTime()) {
		file = &staticFile{modTime: info.ModTime(), size: info.Size()}
		s.entries[name] = file
	}
	s.mu.Unlock()
	file.once.Do(func() { file.read(s.files, name, contentType) })
	if file.err != nil {
		s.mu.Lock()
		if s.entries[name] == file {
			delete(s.entries, name)
		}
		s.mu.Unlock()
	}
	return file
}

// read reads the file, tags it and keeps its compressed copy, when it is
// smaller. A file of unknown type gets the type ServeContent would find.
func (f *staticFile) read(files fs.FS, name, contentType string) {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		f.err = err
		return
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	f.contentType = contentType
	f.tag = etag(data)
	if f.compressible = len(data) >= minCompressed && compressibleType(contentType); !f.compressible {
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
	f.gzip = bytes.Clone(b.Bytes())
}

// brotliCopy returns the brotli copy of the file, nil until it is made;
// the first call starts making it, in turn.
func (f *staticFile) brotliCopy(files fs.FS, name string, turn chan struct{}) []byte {
	f.brotliOnce.Do(func() { go f.makeBrotli(files, name, turn) })
	if body := f.brotli.Load(); body != nil {
		return *body
	}
	return nil
}

// makeBrotli compresses the file with brotli, in turn, and keeps the copy
// when it is smaller than the gzip one; a file changed since it was read
// is left to its new version.
func (f *staticFile) makeBrotli(files fs.FS, name string, turn chan struct{}) {
	turn <- struct{}{}
	defer func() { <-turn }()
	data, err := fs.ReadFile(files, name)
	if err != nil || etag(data) != f.tag {
		return
	}
	var b bytes.Buffer
	w := brotli.NewWriterLevel(&b, brotli.BestCompression)
	_, _ = w.Write(data)
	_ = w.Close()
	if b.Len() >= len(f.gzip) {
		return
	}
	body := bytes.Clone(b.Bytes())
	f.brotli.Store(&body)
}

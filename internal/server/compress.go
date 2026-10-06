package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const (
	// minCompressed is the smallest body worth compressing: below it, the
	// gzip framing and the work cost more than the bytes saved.
	minCompressed = 1024
	// compressionLevel is the level of answers compressed as they are
	// sent. On Jellyfin JSON of 20 to 200 KB, level 5 gives answers 1 to 4%
	// larger than the default level 6, for 20 to 30% less work; files
	// compressed once are compressed at the best level (see gzipFiles).
	compressionLevel = 5
)

// gzipWriters are reused between answers: each holds about 750 KB of
// compression state, too much to allocate per answer.
var gzipWriters = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(io.Discard, compressionLevel)
	return w
}}

// compress gzips the answers that gain from it, for clients that accept
// gzip: JSON, text, JavaScript, CSS and XML of at least minCompressed
// bytes. Range requests, HEAD requests, the WebSocket and HLS playlists and
// segments pass untouched, as do answers that are already encoded, media
// and binary answers, and 204, 206 and 304 answers. Compressible answers
// get Vary: Accept-Encoding whether compressed or not, and a strong ETag
// gets a -gzip suffix when compressed, so that caches tell the two apart.
func compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" || r.Header.Get("Upgrade") != "" ||
			socketPath(r.URL.Path) || hlsPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		c := &compressWriter{w: w, request: r, accepts: acceptsGzip(r.Header)}
		next.ServeHTTP(c, r)
		c.finish()
	})
}

// socketPath reports whether path is Jellyfin's WebSocket, matched as the
// Jellyfin router matches paths.
func socketPath(path string) bool {
	return strings.EqualFold(strings.Trim(path, "/"), "socket")
}

// hlsPath reports whether path is an HLS playlist or segment of the
// Jellyfin API: any .m3u8 file, and the files under hls1/, hls/ and live/
// of /Videos/{itemId} and /Audio/{itemId}, whatever their type. Players
// fetch them as they play; WebVTT segments are text, yet stay as they are.
func hlsPath(path string) bool {
	first, rest, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if !strings.EqualFold(first, "videos") && !strings.EqualFold(first, "audio") {
		return false
	}
	last := rest[strings.LastIndexByte(rest, '/')+1:]
	if len(last) >= len(".m3u8") && strings.EqualFold(last[len(last)-len(".m3u8"):], ".m3u8") {
		return true
	}
	_, rest, _ = strings.Cut(rest, "/")
	folder, file, nested := strings.Cut(rest, "/")
	return nested && file != "" &&
		(strings.EqualFold(folder, "hls1") || strings.EqualFold(folder, "hls") || strings.EqualFold(folder, "live"))
}

// acceptsGzip reports whether the client accepts gzip: named, as gzip or
// x-gzip, or through *, with a quality above zero.
func acceptsGzip(header http.Header) bool {
	gzipQuality, anyQuality := -1.0, -1.0
	for _, value := range header.Values("Accept-Encoding") {
		for element := range strings.SplitSeq(value, ",") {
			coding, params, _ := strings.Cut(element, ";")
			coding = strings.TrimSpace(coding)
			quality := 1.0
			for param := range strings.SplitSeq(params, ";") {
				name, text, ok := strings.Cut(param, "=")
				if !ok || !strings.EqualFold(strings.TrimSpace(name), "q") {
					continue
				}
				// An unreadable quality refuses, as the safe reading.
				parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
				if err != nil {
					parsed = 0
				}
				quality = parsed
			}
			switch {
			case strings.EqualFold(coding, "gzip"), strings.EqualFold(coding, "x-gzip"):
				gzipQuality = max(gzipQuality, quality)
			case coding == "*":
				anyQuality = max(anyQuality, quality)
			}
		}
	}
	if gzipQuality >= 0 {
		return gzipQuality > 0
	}
	return anyQuality > 0
}

// compressibleType reports whether answers of contentType gain from
// compression: text but event streams, which are sent as they come, JSON,
// JavaScript, XML and SVG, and WebAssembly.
func compressibleType(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	switch {
	case mediaType == "text/event-stream":
		return false
	case strings.HasPrefix(mediaType, "text/"), strings.HasSuffix(mediaType, "+json"), strings.HasSuffix(mediaType, "+xml"):
		return true
	}
	switch mediaType {
	case "application/json", "application/javascript", "application/x-javascript", "application/ecmascript",
		"application/xml", "application/wasm":
		return true
	}
	return false
}

// compressibleStatus reports whether an answer of status may carry a
// compressed body: not 204 or 304, which have none, nor 206, a range of
// the identity body.
func compressibleStatus(status int) bool {
	return status >= 200 && status != http.StatusNoContent && status != http.StatusPartialContent &&
		status != http.StatusNotModified
}

// addVary adds Accept-Encoding to the header's Vary, once.
func addVary(header http.Header) {
	for _, value := range header.Values("Vary") {
		for name := range strings.SplitSeq(value, ",") {
			if name = strings.TrimSpace(name); name == "*" || strings.EqualFold(name, "Accept-Encoding") {
				return
			}
		}
	}
	header.Add("Vary", "Accept-Encoding")
}

// gzipTag is the ETag of the compressed variant of an answer whose strong
// ETag is tag; weak ETags, and headers without one, stay as they are: a
// weak ETag already covers equivalent bodies.
func gzipTag(tag string) string {
	if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return tag
	}
	return tag[:len(tag)-1] + `-gzip"`
}

// hasTag reports whether the If-None-Match list holds tag, compared
// weakly as RFC 9110 asks for If-None-Match.
func hasTag(list, tag string) bool {
	tag = strings.TrimPrefix(tag, "W/")
	for {
		list = strings.TrimLeft(list, " \t,")
		list = strings.TrimPrefix(list, "W/")
		if len(list) < 2 || list[0] != '"' {
			return false
		}
		end := strings.IndexByte(list[1:], '"')
		if end < 0 {
			return false
		}
		if list[:end+2] == tag {
			return true
		}
		list = list[end+2:]
	}
}

// keepIdentity asks compress to send the answer w is about to write as it
// is; a handler that knows its body does not gain from compression calls
// it before writing.
func keepIdentity(w http.ResponseWriter) {
	for {
		switch writer := w.(type) {
		case *compressWriter:
			writer.identity = true
			return
		case interface{ Unwrap() http.ResponseWriter }:
			w = writer.Unwrap()
		default:
			return
		}
	}
}

type writerState int

const (
	// statePending: the handler has written neither header nor body.
	statePending writerState = iota
	// stateBuffering: the answer may be compressed once its size, or its
	// type, is known from the first bytes, which are held.
	stateBuffering
	// statePlain: the answer goes through as written.
	statePlain
	// stateCompressing: the body goes through gzip.
	stateCompressing
	// stateDiscarding: compress answered 304 itself; the body is dropped.
	stateDiscarding
)

// compressWriter decides, once the handler's headers and, when needed, its
// first bytes are known, whether the answer is compressed.
type compressWriter struct {
	w        http.ResponseWriter
	request  *http.Request
	accepts  bool
	identity bool
	state    writerState
	status   int
	held     []byte
	gzip     *gzip.Writer
}

// Header returns the header of the answer.
func (c *compressWriter) Header() http.Header { return c.w.Header() }

// Unwrap returns the underlying writer, for http.ResponseController.
func (c *compressWriter) Unwrap() http.ResponseWriter { return c.w }

// WriteHeader decides what can be decided from the headers: a typed answer
// of known length is sent plain or compressed at once; others wait for
// their first bytes.
func (c *compressWriter) WriteHeader(status int) {
	if c.state != statePending {
		return
	}
	if status >= 100 && status <= 199 {
		if status == http.StatusSwitchingProtocols {
			c.state = statePlain
		}
		c.w.WriteHeader(status)
		return
	}
	c.status = status
	header := c.w.Header()
	if !compressibleStatus(status) || c.identity || header.Get("Content-Encoding") != "" {
		c.sendPlain()
		return
	}
	if _, typed := header["Content-Type"]; !typed {
		// Go would find the type from the body: wait for it.
		c.state = stateBuffering
		return
	}
	if !c.eligible() {
		c.sendPlain()
		return
	}
	if c.revalidated() {
		return
	}
	if length, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64); err == nil {
		if length >= minCompressed {
			c.sendCompressed()
		} else {
			c.sendPlain()
		}
		return
	}
	c.state = stateBuffering
}

// Write sends p as decided, holding the first bytes of an answer whose
// size or type is not known yet.
func (c *compressWriter) Write(p []byte) (int, error) {
	if c.state == statePending {
		c.WriteHeader(http.StatusOK)
	}
	if c.state == stateBuffering {
		if len(c.held)+len(p) < minCompressed {
			if c.held == nil {
				c.held = make([]byte, 0, minCompressed)
			}
			c.held = append(c.held, p...)
			return len(p), nil
		}
		if err := c.release(true, p); err != nil {
			return 0, err
		}
	}
	switch c.state {
	case stateCompressing:
		return c.gzip.Write(p)
	case stateDiscarding:
		return len(p), nil
	default:
		return c.w.Write(p)
	}
}

// ReadFrom lets a plain answer use the underlying writer's ReadFrom, which
// sends files without copying them through user space.
func (c *compressWriter) ReadFrom(src io.Reader) (int64, error) {
	if c.state == statePending {
		c.WriteHeader(http.StatusOK)
	}
	if from, ok := c.w.(io.ReaderFrom); ok && c.state == statePlain {
		return from.ReadFrom(src)
	}
	return io.Copy(writerOnly{c}, src)
}

// writerOnly hides compressWriter's ReadFrom from io.Copy.
type writerOnly struct{ io.Writer }

// Flush sends what was written so far: a held answer is decided as a
// large one, since a handler that flushes streams, then the gzip writer
// and the underlying one are flushed.
func (c *compressWriter) Flush() {
	if c.state == statePending {
		c.WriteHeader(http.StatusOK)
	}
	if c.state == stateBuffering {
		if err := c.release(true, nil); err != nil {
			return
		}
	}
	if c.state == stateCompressing {
		if err := c.gzip.Flush(); err != nil {
			return
		}
	}
	_ = http.NewResponseController(c.w).Flush()
}

// release ends holding: large tells whether the body reached
// minCompressed, or is streamed; next is the write that ended it, if any,
// which comes after the held bytes.
func (c *compressWriter) release(large bool, next []byte) error {
	header := c.w.Header()
	if _, typed := header["Content-Type"]; !typed {
		sniffed := c.held
		if len(sniffed) < 512 && len(next) > 0 {
			sniffed = append(sniffed[:len(sniffed):len(sniffed)], next[:min(len(next), 512-len(sniffed))]...)
		}
		switch {
		case len(sniffed) == 0:
			// Nothing to find a type from, as Go sends it.
			c.sendPlain()
		default:
			header.Set("Content-Type", http.DetectContentType(sniffed))
			if !c.eligible() {
				c.sendPlain()
			} else if c.revalidated() {
				return nil
			}
		}
	}
	if c.state == stateBuffering {
		if large {
			c.sendCompressed()
		} else {
			c.sendPlain()
		}
	}
	held := c.held
	c.held = nil
	if len(held) == 0 {
		return nil
	}
	var err error
	if c.state == stateCompressing {
		_, err = c.gzip.Write(held)
	} else {
		_, err = c.w.Write(held)
	}
	return err
}

// eligible reports whether the typed answer may be compressed: a
// compressible type, for a client that takes gzip. A compressible answer
// gets Vary: Accept-Encoding either way.
func (c *compressWriter) eligible() bool {
	header := c.w.Header()
	if !compressibleType(header.Get("Content-Type")) {
		return false
	}
	addVary(header)
	return c.accepts
}

// revalidated answers 304 when the client holds the compressed variant of
// the 200 answer about to be sent: the handler only knows the identity
// ETag, so it cannot tell.
func (c *compressWriter) revalidated() bool {
	header := c.w.Header()
	tag := header.Get("ETag")
	if c.status != http.StatusOK || !strings.HasPrefix(tag, `"`) {
		return false
	}
	variant := gzipTag(tag)
	if !hasTag(strings.Join(c.request.Header.Values("If-None-Match"), ","), variant) {
		return false
	}
	header.Del("Content-Type")
	header.Del("Content-Length")
	header.Set("ETag", variant)
	c.state = stateDiscarding
	c.w.WriteHeader(http.StatusNotModified)
	return true
}

func (c *compressWriter) sendPlain() {
	c.state = statePlain
	c.w.WriteHeader(c.status)
}

func (c *compressWriter) sendCompressed() {
	header := c.w.Header()
	header.Del("Content-Length")
	// Ranges of the compressed body are not served.
	header.Del("Accept-Ranges")
	header.Set("Content-Encoding", "gzip")
	if tag := header.Get("ETag"); tag != "" {
		header.Set("ETag", gzipTag(tag))
	}
	c.gzip = gzipWriters.Get().(*gzip.Writer)
	c.gzip.Reset(c.w)
	c.state = stateCompressing
	c.w.WriteHeader(c.status)
}

// finish sends a held answer, which is then small, and ends the gzip
// stream.
func (c *compressWriter) finish() {
	if c.state == stateBuffering {
		_ = c.release(false, nil)
	}
	if c.state == stateCompressing {
		_ = c.gzip.Close()
		c.gzip.Reset(io.Discard)
		gzipWriters.Put(c.gzip)
		c.gzip = nil
	}
}

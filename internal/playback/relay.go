package playback

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
)

// ErrSourceUnavailable reports a source that did not answer with media.
var ErrSourceUnavailable = errors.New("source unavailable")

var (
	// forwardedRequest are the player's headers a source needs to answer
	// byte ranges.
	forwardedRequest = []string{"Range", "If-Range"}
	// forwardedResponse are the source's headers a player needs.
	forwardedResponse = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"}
)

var copyBuffers = sync.Pool{New: func() any { return new([256 << 10]byte) }}

// relay serves a version to a player, byte ranges included, renewing its
// link once when it expired. The delivery's content type, when set,
// replaces the source's: players recognize media by it, and sources often
// answer application/octet-stream. When the source cannot be reached or
// does not answer with content, the player receives a 502 and the error is
// returned for logging.
func (s *Service) relay(w http.ResponseWriter, r *http.Request, version library.Version, delivery Delivery) error {
	response, err := s.openForPlayer(r, version)
	if err == nil && expired(response.StatusCode) && s.renew != nil {
		response.Body.Close()
		if fresh, renewErr := s.renew(r.Context(), version); renewErr == nil {
			response, err = s.openForPlayer(r, fresh)
		} else {
			err = fmt.Errorf("%w: HTTP %d, and the link could not be renewed: %v", ErrSourceUnavailable, response.StatusCode, renewErr)
		}
	}
	if err != nil {
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
	default:
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return fmt.Errorf("%w: HTTP %d", ErrSourceUnavailable, response.StatusCode)
	}
	for _, name := range forwardedResponse {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	if delivery.ContentType != "" {
		w.Header().Set("Content-Type", delivery.ContentType)
	}
	setAttachment(w, delivery.Attachment)
	w.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return nil
	}
	buffer := copyBuffers.Get().(*[256 << 10]byte)
	defer copyBuffers.Put(buffer)
	_, err = io.CopyBuffer(w, response.Body, buffer[:])
	return err
}

// openForPlayer requests a version as the player asked, with the headers
// the source requires.
func (s *Service) openForPlayer(r *http.Request, version library.Version) (*http.Response, error) {
	header := http.Header{}
	for name, value := range version.Headers {
		header.Set(name, value)
	}
	for _, name := range forwardedRequest {
		if value := r.Header.Get(name); value != "" {
			header.Set(name, value)
		}
	}
	method := http.MethodGet
	if r.Method == http.MethodHead {
		method = http.MethodHead
	}
	response, err := s.opener.Open(r.Context(), method, version.URL, header, version.Confined)
	if err == nil && method == http.MethodHead && (response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented) {
		// Some servers only answer GET: its headers are enough.
		response.Body.Close()
		response, err = s.opener.Open(r.Context(), http.MethodGet, version.URL, header, version.Confined)
	}
	return response, err
}

// expired reports whether a status means a link no longer works, which a
// fresh link from the addon may fix.
func expired(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusGone:
		return true
	}
	return false
}

// loopback serves cached sources on the loopback interface, so that
// ffprobe and FFmpeg read them through Polyfin: confined sources stay
// confined, headers are sent, and what they read is cached.
type loopback struct {
	listener net.Listener
	server   *http.Server

	mu      sync.Mutex
	sources map[string]*source.Source
}

func newLoopback() (*loopback, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	l := &loopback{listener: listener, sources: map[string]*source.Source{}}
	l.server = &http.Server{Handler: http.HandlerFunc(l.serve), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = l.server.Serve(listener) }()
	return l, nil
}

// register makes a source readable at the returned URL until release is
// called.
func (l *loopback) register(src *source.Source) (string, func()) {
	var token [24]byte
	_, _ = rand.Read(token[:])
	key := hex.EncodeToString(token[:])
	l.mu.Lock()
	l.sources[key] = src
	l.mu.Unlock()
	release := func() {
		l.mu.Lock()
		delete(l.sources, key)
		l.mu.Unlock()
	}
	return "http://" + l.listener.Addr().String() + "/" + key, release
}

func (l *loopback) serve(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	src, ok := l.sources[strings.TrimPrefix(r.URL.Path, "/")]
	l.mu.Unlock()
	if !ok || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	src.ServeHTTP(w, r)
}

func (l *loopback) Close() error {
	return l.server.Close()
}

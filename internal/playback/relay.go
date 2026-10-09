package playback

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
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
// returned for logging; when its source's connections are all in use, a
// 503. A file analyzed as one, not a live stream, is served from the
// source cache, which reads it the way remuxes do.
func (s *Service) relay(w http.ResponseWriter, r *http.Request, version library.Version, delivery Delivery) error {
	release, err := s.Hold(r.Context(), version)
	if err != nil {
		http.Error(w, "the provider's connections are all in use", http.StatusServiceUnavailable)
		return err
	}
	defer release()
	if s.cacheable(r.Context(), version) {
		return s.relayCached(w, r, version, delivery)
	}
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

// cacheable reports whether a version is a file the source cache keeps:
// analyzed, with a size and a duration, and not a live stream or playlist.
func (s *Service) cacheable(ctx context.Context, version library.Version) bool {
	analysis, ok := s.Analyzed(ctx, version.ID)
	return ok && analysis.Duration > 0 && analysis.Size > 0 && !Manifest(analysis)
}

// relayCached serves a version from the source cache: the bytes ffprobe
// and the remuxes read are served without a request, the others are read
// through the address its origin redirected the server's reads to, and
// kept for the next seek. A player's seek is a reader of its own, which
// the cache serves first, as a playback.
func (s *Service) relayCached(w http.ResponseWriter, r *http.Request, version library.Version, delivery Delivery) error {
	src := s.open(version)
	defer src.Release()
	defer src.Urge()()
	if delivery.ContentType != "" {
		w.Header().Set("Content-Type", delivery.ContentType)
	}
	setAttachment(w, delivery.Attachment)
	if err := src.Relay(w, r); err != nil {
		return fmt.Errorf("%w: %w", ErrSourceUnavailable, err)
	}
	return nil
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
// confined, headers are sent, and what they read is cached. It serves live
// streams too, through live, uncached.
type loopback struct {
	listener net.Listener
	server   *http.Server
	live     func(w http.ResponseWriter, r *http.Request, version library.Version, target string, link func(string) string) error

	mu      sync.Mutex
	sources map[string]*source.Source
	feeds   map[string]library.Version
	// files are Polyfin's own files, recordings, by version (see
	// FileVersion), served under fileKey, as the files of local folders
	// are, which local opens (see LocalFiles).
	files   map[accounts.ID]string
	local   func(ctx context.Context, key string) (*os.File, error)
	fileKey string
}

func newLoopback() (*loopback, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	l := &loopback{listener: listener, sources: map[string]*source.Source{}, feeds: map[string]library.Version{},
		files: map[accounts.ID]string{}, fileKey: randomKey()}
	l.server = &http.Server{Handler: http.HandlerFunc(l.serve), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = l.server.Serve(listener) }()
	return l, nil
}

// randomKey names what the loopback interface serves, unguessably.
func randomKey() string {
	var token [24]byte
	_, _ = rand.Read(token[:])
	return hex.EncodeToString(token[:])
}

// register makes a source readable at the returned URL until release is
// called.
func (l *loopback) register(src *source.Source) (string, func()) {
	key := randomKey()
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
	if rest, ok := strings.CutPrefix(r.URL.Path, "/live/"); ok {
		l.serveLive(w, r, rest)
		return
	}
	if rest, ok := strings.CutPrefix(r.URL.Path, "/file/"); ok {
		l.serveFile(w, r, rest)
		return
	}
	if rest, ok := strings.CutPrefix(r.URL.Path, "/local/"); ok {
		l.serveLocal(w, r, rest)
		return
	}
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

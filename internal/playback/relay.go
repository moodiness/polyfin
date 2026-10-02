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
	"strings"
	"sync"
	"time"
)

// ErrSourceUnavailable reports a source that did not answer with media.
var ErrSourceUnavailable = errors.New("source unavailable")

// Source is where a version's bytes come from.
type Source struct {
	URL     string
	Headers map[string]string
	// Confined restricts requests to public addresses.
	Confined bool
}

// Opener requests sources; *stremio.Client is one.
type Opener interface {
	Open(ctx context.Context, method, target string, header http.Header, confined bool) (*http.Response, error)
}

var (
	// forwardedRequest are the player's headers a source needs to answer
	// byte ranges.
	forwardedRequest = []string{"Range", "If-Range"}
	// forwardedResponse are the source's headers a player needs.
	forwardedResponse = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"}
)

var copyBuffers = sync.Pool{New: func() any { return new([256 << 10]byte) }}

// Relay serves a source to a player, byte ranges included. contentType,
// when set, replaces the source's: players recognize media by it, and
// sources often answer application/octet-stream. When the source cannot be
// reached or does not answer with content, the player receives a 502 and
// the error is returned for logging.
func Relay(w http.ResponseWriter, r *http.Request, opener Opener, source Source, contentType string) error {
	header := http.Header{}
	for name, value := range source.Headers {
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
	response, err := opener.Open(r.Context(), method, source.URL, header, source.Confined)
	if err == nil && method == http.MethodHead && (response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented) {
		// Some servers only answer GET: its headers are enough.
		response.Body.Close()
		response, err = opener.Open(r.Context(), http.MethodGet, source.URL, header, source.Confined)
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
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return nil
	}
	buffer := copyBuffers.Get().(*[256 << 10]byte)
	defer copyBuffers.Put(buffer)
	_, err = io.CopyBuffer(w, response.Body, buffer[:])
	return err
}

// sourceServer serves sources on the loopback interface, so that ffprobe
// reads them through Polyfin's connections: confined sources stay confined
// and their headers are sent.
type sourceServer struct {
	opener   Opener
	listener net.Listener
	server   *http.Server

	mu      sync.Mutex
	sources map[string]Source
}

func newSourceServer(opener Opener) (*sourceServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &sourceServer{opener: opener, listener: listener, sources: map[string]Source{}}
	s.server = &http.Server{Handler: http.HandlerFunc(s.serve), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = s.server.Serve(listener) }()
	return s, nil
}

// register makes a source readable at the returned URL until release is
// called.
func (s *sourceServer) register(source Source) (string, func()) {
	var token [24]byte
	_, _ = rand.Read(token[:])
	key := hex.EncodeToString(token[:])
	s.mu.Lock()
	s.sources[key] = source
	s.mu.Unlock()
	release := func() {
		s.mu.Lock()
		delete(s.sources, key)
		s.mu.Unlock()
	}
	return "http://" + s.listener.Addr().String() + "/" + key, release
}

func (s *sourceServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	source, ok := s.sources[strings.TrimPrefix(r.URL.Path, "/")]
	s.mu.Unlock()
	if !ok || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	// Errors only mean the source or ffprobe went away; ffprobe reports
	// what it could not read.
	_ = Relay(w, r, s.opener, source, "")
}

func (s *sourceServer) Close() error {
	return s.server.Close()
}

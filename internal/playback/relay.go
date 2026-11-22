package playback

import (
	"bytes"
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
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/relayed"
	"github.com/moodiness/polyfin/internal/source"
)

// ErrSourceUnavailable reports a source that did not answer with media.
var ErrSourceUnavailable = errors.New("source unavailable")

var (
	// forwardedRequest are the player's headers a source needs to answer
	// byte ranges.
	forwardedRequest = []string{"Range", "If-Range"}
	// forwardedResponse are the source's headers a player needs. Its
	// Content-Type is forwarded only when it names media (see
	// relayed.MediaType).
	forwardedResponse = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"}
)

var copyBuffers = sync.Pool{New: func() any { return new([256 << 10]byte) }}

// relay serves a version to a player, byte ranges included, renewing its
// link once when it expired. The delivery's content type, when set,
// replaces the source's: players recognize media by it, and sources often
// answer application/octet-stream. The source's own is kept only when it
// names media, and every answer is sandboxed (see package relayed): a
// page a source answers is never one of Polyfin's. When the source cannot
// be reached or does not answer with content, the player receives a 502
// and the error is returned for logging; when its source's connections
// are all in use, a 503. A file analyzed as one, not a live stream, is
// served from the
// source cache, which reads it the way remuxes do, and so is a file with
// mirrors, which only the cache reads on from (see open). A version
// analyzed as an HLS playlist is relayed as a channel's is (see
// ServeLive), every address it names one of the delivery's links: the
// player would otherwise resolve a relative address against Polyfin's,
// and fetch a full one without the headers the source needs (#1197).
func (s *Service) relay(w http.ResponseWriter, r *http.Request, version library.Version, delivery Delivery) error {
	relayed.Sandbox(w.Header())
	release, err := s.Hold(r.Context(), version)
	if err != nil {
		http.Error(w, "the provider's connections are all in use", http.StatusServiceUnavailable)
		return err
	}
	defer release()
	if analysis, ok := s.Analyzed(r.Context(), version.ID); ok && HLS(analysis) && delivery.Link != nil {
		return s.serveLive(w, r, version, version.URL, liveRelay{link: delivery.Link})
	}
	if s.cacheable(r.Context(), version) || version.Torrent != nil || len(version.Mirrors) > 0 {
		// A torrent's file is read from its swarm alone, through the cache.
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
	body := s.countReceived(version.ID, response.Body)
	defer body.Close()
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
	w.Header().Set("Content-Type", relayed.MediaType(response.Header.Get("Content-Type")))
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
	_, err = io.CopyBuffer(w, body, buffer[:])
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
	for name, value := range version.RequestHeaders() {
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
// streams too, through live, uncached, and so the files an HLS playlist a
// version's source answers names (see servePlaylist).
type loopback struct {
	listener net.Listener
	server   *http.Server
	live     func(w http.ResponseWriter, r *http.Request, version library.Version, target string, relay liveRelay) error

	mu      sync.Mutex
	sources map[string]*registered
	feeds   map[string]*liveTarget
	// files are Polyfin's own files, recordings, by version (see
	// FileVersion), served under fileKey, as the files of local folders
	// and network shares are, which local opens (see LocalFiles).
	files map[accounts.ID]string
	local func(ctx context.Context, key string) (io.ReadSeekCloser, time.Time, error)
	// stored finds where the source cache reads the files of local
	// folders read straight from their storage (see LocalSources).
	stored  func(ctx context.Context, key string) (source.Location, bool)
	fileKey string
	// inputs finds what FFmpeg reads a version of a local folder that is
	// not one file from (see LocalInputs).
	inputs Inputs
	// playout serves the files of the playout of a channel Polyfin airs
	// (see UsePlayouts), nil until set.
	playout func(w http.ResponseWriter, r *http.Request, channel accounts.ID, file string)
}

func newLoopback() (*loopback, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	l := &loopback{listener: listener, sources: map[string]*registered{}, feeds: map[string]*liveTarget{},
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

// registered is a version's source registered on the loopback interface.
type registered struct {
	version library.Version
	src     *source.Source
	// budget counts what is read of the source, nil for nothing (see
	// registerWithin); asked serves it without reading ahead (see
	// registerAsked); bound holds its reads within the source's first
	// bytes until lifted, nil for none (see registerBounded).
	budget *readBudget
	asked  bool
	bound  *source.Bound
	// head is what the source's first bytes were found to be once read:
	// headUnread, headPlaylist or headFile.
	head atomic.Int32
}

// What a registered source's first bytes were found to be: not read yet,
// an HLS playlist, or a file's.
const (
	headUnread int32 = iota
	headPlaylist
	headFile
)

// register makes a version's source, src, readable at the returned URL
// until release is called.
func (l *loopback) register(version library.Version, src *source.Source) (string, func()) {
	return l.add(&registered{version: version, src: src})
}

// registerAsked is register, the source served without reading ahead (see
// source.Source.ServeAsked): what is fetched of it is what is served,
// within a block a request.
func (l *loopback) registerAsked(version library.Version, src *source.Source) (string, func()) {
	return l.add(&registered{version: version, src: src, asked: true})
}

// registerWithin is register, what is read of the source counted by
// budget, which refuses to serve more than its limit; nil counts nothing.
// A source read within a budget is served as registerAsked serves it.
func (l *loopback) registerWithin(version library.Version, src *source.Source, budget *readBudget) (string, func()) {
	return l.add(&registered{version: version, src: src, budget: budget, asked: budget != nil})
}

// registerBounded is register, the source served within bound until it is
// lifted (see source.Bound): FFmpeg reads past it only what the source
// cache holds, and waits for the rest.
func (l *loopback) registerBounded(version library.Version, src *source.Source, bound *source.Bound) (string, func()) {
	return l.add(&registered{version: version, src: src, bound: bound})
}

// add serves reg until release is called, at the URL returned.
func (l *loopback) add(reg *registered) (string, func()) {
	key := randomKey()
	l.mu.Lock()
	l.sources[key] = reg
	l.mu.Unlock()
	release := func() {
		l.mu.Lock()
		delete(l.sources, key)
		delete(l.feeds, key)
		l.mu.Unlock()
	}
	return "http://" + l.listener.Addr().String() + "/" + key, release
}

// serve answers what the loopback interface serves. FFmpeg and ffprobe
// are given the listener as their HTTP proxy (see media.Opens), so that
// an address they would fetch themselves is asked here instead: a request
// for another host than the listener's own, or for a tunnel to one, is
// refused.
func (l *loopback) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect || r.URL.Host != "" && r.URL.Host != l.listener.Addr().String() {
		http.Error(w, "the loopback interface relays its own addresses only", http.StatusForbidden)
		return
	}
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
	if rest, ok := strings.CutPrefix(r.URL.Path, "/playout/"); ok {
		l.servePlayout(w, r, rest)
		return
	}
	l.mu.Lock()
	key := strings.TrimPrefix(r.URL.Path, "/")
	reg, ok := l.sources[key]
	l.mu.Unlock()
	if !ok || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	if reg.budget != nil {
		w = budgetWriter{w, reg.budget}
	}
	if l.servePlaylist(w, r, key, reg) {
		return
	}
	if reg.asked {
		reg.src.ServeAsked(w, r)
		return
	}
	if reg.bound != nil {
		reg.src.ServeWithin(w, r, reg.bound)
		return
	}
	reg.src.ServeHTTP(w, r)
}

// servePlaylist serves a version's source that answers an HLS playlist
// as the live relay serves one (see Service.ServeLive), and reports
// whether it did: rewritten, with every address it names on the loopback
// interface under the source's key, so that FFmpeg and ffprobe fetch the
// variants, segments, keys and initialization sections it names through
// Polyfin too, with the headers the source needs and within its
// confinement. Unrewritten, ffprobe would resolve a relative address
// against the loopback interface, and fetch a full one itself, without
// those headers (#1197). A finished playlist stays one: its tags are kept,
// #EXT-X-ENDLIST included. The source is told a playlist by its first
// bytes, which the source cache reads once, at the first request from
// the start, as FFmpeg asks for a playlist; requests further in are of a
// file. Torrents' files are never playlists.
func (l *loopback) servePlaylist(w http.ResponseWriter, r *http.Request, key string, reg *registered) bool {
	if reg.version.Torrent != nil || reg.head.Load() == headFile || !fromStart(r) {
		return false
	}
	if reg.head.Load() == headUnread {
		head := make([]byte, 16)
		n, err := reg.src.ReadAt(r.Context(), head, 0)
		if n == 0 && err != nil {
			// The source's own answer tells why it cannot be read.
			return false
		}
		found := headFile
		if bytes.HasPrefix(bytes.TrimPrefix(head[:n], []byte("\xef\xbb\xbf")), []byte("#EXTM3U")) {
			found = headPlaylist
		}
		reg.head.Store(found)
		if found == headFile {
			return false
		}
	}
	l.mu.Lock()
	if _, registered := l.sources[key]; registered && l.feeds[key] == nil {
		l.feeds[key] = &liveTarget{version: reg.version}
	}
	l.mu.Unlock()
	relay := liveRelay{link: func(file, name string) string { return l.liveURL(key, file, name) }}
	_ = l.live(w, r, reg.version, reg.version.URL, relay)
	return true
}

// fromStart reports whether a request asks for a file from its first
// byte: all of it, or a range from 0.
func fromStart(r *http.Request) bool {
	asked := r.Header.Get("Range")
	return asked == "" || strings.HasPrefix(strings.TrimSpace(asked), "bytes=0-")
}

// servePlayout answers the requests for the files of a channel's playout:
// /playout/{fileKey}/{channel}/{file}.
func (l *loopback) servePlayout(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	l.mu.Lock()
	serve := l.playout
	l.mu.Unlock()
	if len(parts) != 3 || parts[0] != l.fileKey || serve == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	channel, err := accounts.ParseID(parts[1])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serve(w, r, channel, parts[2])
}

func (l *loopback) Close() error {
	return l.server.Close()
}

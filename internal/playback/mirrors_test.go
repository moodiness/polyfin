package playback

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// mirrorHosts serves the same file at two hosts: the one named dying
// goes down once it sent cut bytes of an answer, and refuses every
// request after; the other answers every request, which it counts.
type mirrorHosts struct {
	data     []byte
	dying    string
	cut      int
	dead     atomic.Bool
	mirrored atomic.Int32
}

func (h *mirrorHosts) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	dying := strings.HasPrefix(target, h.dying)
	if dying && h.dead.Load() {
		return nil, errors.New("connection refused")
	}
	if !dying {
		h.mirrored.Add(1)
	}
	request := httptest.NewRequestWithContext(ctx, method, target, nil)
	request.Header = header.Clone()
	recorder := httptest.NewRecorder()
	http.ServeContent(recorder, request, "", time.Time{}, bytes.NewReader(h.data))
	response := recorder.Result()
	response.Request = request
	if dying {
		h.dead.Store(true)
		response.Body = io.NopCloser(io.MultiReader(io.LimitReader(response.Body, int64(h.cut)), failingReader{}))
	}
	return response, nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// mirrored is a version on a local network's host, which players reach
// through the server, with a copy of its file at another host.
func mirrored(dying, other string, size int) library.Version {
	version := library.Version{ID: accounts.ID{71}, URL: dying + "/movie.mkv", Filename: "movie.mkv", Size: int64(size), Addon: "First"}
	mirror := version
	mirror.URL, mirror.Addon = other+"/movie.mkv", "Second"
	version.Mirrors = []library.Version{mirror}
	return version
}

// A file relayed to a player reads on from the copy another addon lists
// once its host goes down mid-read: the player gets the whole file.
func TestARelayedReadGoesOnFromAMirror(t *testing.T) {
	data := make([]byte, 5<<20+321)
	_, _ = rand.Read(data)
	hosts := &mirrorHosts{data: data, dying: "http://192.168.1.10", cut: 2<<20 + 17}
	s := newService(t, hosts, "ffprobe", nil)
	version := mirrored("http://192.168.1.10", "http://192.168.1.11", len(data))
	response := httptest.NewRecorder()
	if err := s.Serve(response, httptest.NewRequest(http.MethodGet, "/", nil), version, Delivery{}); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), data) {
		t.Errorf("relayed: %d, %d bytes of %d, the same: %v", response.Code, response.Body.Len(), len(data), bytes.Equal(response.Body.Bytes(), data))
	}
	if hosts.mirrored.Load() == 0 {
		t.Error("the mirror was not read")
	}
}

// Direct play sends the player to the version's own link: a player that
// reads the source itself cannot be carried on from a copy.
func TestDirectPlayIgnoresMirrors(t *testing.T) {
	hosts := &mirrorHosts{data: make([]byte, 1000), dying: "https://93.184.216.34", cut: 1000}
	s := newService(t, hosts, "ffprobe", nil)
	version := mirrored("https://93.184.216.34", "https://93.184.216.35", 1000)
	response := httptest.NewRecorder()
	if err := s.Serve(response, httptest.NewRequest(http.MethodGet, "/", nil), version, Delivery{}); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusFound || response.Header().Get("Location") != version.URL {
		t.Errorf("%d to %q", response.Code, response.Header().Get("Location"))
	}
}

package playback

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/testdb"
)

// fakeSource answers like a remote file server, recording requests. Links
// listed in expired answer 403.
type fakeSource struct {
	status  int
	body    []byte
	expired map[string]bool

	mu       sync.Mutex
	requests []http.Header
	targets  []string
	confined []bool
}

func (f *fakeSource) Open(_ context.Context, _, target string, header http.Header, confined bool) (*http.Response, error) {
	f.mu.Lock()
	f.requests = append(f.requests, header.Clone())
	f.targets = append(f.targets, target)
	f.confined = append(f.confined, confined)
	f.mu.Unlock()
	status := f.status
	if status == 0 {
		status = http.StatusPartialContent
	}
	if f.expired[target] {
		status = http.StatusForbidden
	}
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type":   {"application/octet-stream"},
			"Content-Range":  {"bytes 0-3/1000"},
			"Content-Length": {"4"},
			"Accept-Ranges":  {"bytes"},
			"Set-Cookie":     {"secret=1"},
		},
		Body: io.NopCloser(bytes.NewReader(f.body)),
	}, nil
}

func newService(t *testing.T, opener source.Opener, ffprobe string, renew Renewer) *Service {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sources, err := source.New(t.TempDir(), 1<<30, opener, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sources.Close() })
	segments, err := hls.NewManager("ffmpeg", t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(segments.Close)
	s, err := New(pool, opener, ffprobe, NewSigner([]byte("test secret")), sources, segments, renew, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestServeRedirectsOnlyWhenThePlayerCanFetchTheSource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		version  library.Version
		relay    bool
		redirect bool
	}{
		{"public source", library.Version{URL: "https://93.184.216.34/movie.mkv"}, false, true},
		{"source needing headers", library.Version{URL: "https://93.184.216.34/movie.mkv", Headers: map[string]string{"Referer": "https://site.example/"}}, false, false},
		{"source on the local network", library.Version{URL: "http://192.168.0.10:3000/movie.mkv"}, false, false},
		{"player that cannot follow redirects", library.Version{URL: "https://93.184.216.34/movie.mkv"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origin := &fakeSource{body: []byte("\x1a\x45\xdf\xa3")}
			s := newService(t, origin, "ffprobe", nil)
			tc.version.ID = accounts.ID{1}
			request := httptest.NewRequest(http.MethodGet, "/Videos/x/stream", nil)
			request.Header.Set("Range", "bytes=0-3")
			response := httptest.NewRecorder()
			if err := s.Serve(response, request, tc.version, tc.relay, "video/x-matroska"); err != nil {
				t.Fatal(err)
			}
			if tc.redirect {
				if response.Code != http.StatusFound || response.Header().Get("Location") != tc.version.URL {
					t.Fatalf("got %d to %q", response.Code, response.Header().Get("Location"))
				}
				// The source was checked before the player was sent to it.
				if len(origin.requests) != 1 || origin.requests[0].Get("Range") != "bytes=0-0" {
					t.Errorf("checks: %v", origin.requests)
				}
				return
			}
			if response.Code != http.StatusPartialContent || response.Body.String() != "\x1a\x45\xdf\xa3" ||
				response.Header().Get("Content-Range") != "bytes 0-3/1000" || response.Header().Get("Content-Type") != "video/x-matroska" {
				t.Fatalf("relay: %d %v %q", response.Code, response.Header(), response.Body.String())
			}
			if response.Header().Get("Set-Cookie") != "" {
				t.Error("a source's cookie reached the player")
			}
			sent := origin.requests[0]
			if sent.Get("Range") != "bytes=0-3" || sent.Get("Referer") != tc.version.Headers["Referer"] {
				t.Errorf("headers sent to the source: %v", sent)
			}
		})
	}
}

func TestServeRefusesSourcesThatDoNotAnswer(t *testing.T) {
	origin := &fakeSource{status: http.StatusNotFound}
	s := newService(t, origin, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{2}, URL: "https://93.184.216.34/gone.mkv"}
	response := httptest.NewRecorder()
	err := s.Serve(response, httptest.NewRequest(http.MethodGet, "/", nil), version, false, "")
	if !errors.Is(err, ErrSourceUnavailable) || response.Code != http.StatusBadGateway {
		t.Fatalf("got %d, %v", response.Code, err)
	}
	if !s.Failed(version.ID) {
		t.Error("a dead source is not remembered as failed")
	}
}

func TestExpiredLinksAreRenewedBeforeServing(t *testing.T) {
	const old, fresh = "https://93.184.216.34/old.mkv", "https://93.184.216.34/new.mkv"
	for _, relay := range []bool{false, true} {
		origin := &fakeSource{body: []byte("\x1a\x45\xdf\xa3"), expired: map[string]bool{old: true}}
		renewals := 0
		s := newService(t, origin, "ffprobe", func(_ context.Context, version library.Version) (library.Version, error) {
			renewals++
			version.URL = fresh
			return version, nil
		})
		response := httptest.NewRecorder()
		version := library.Version{ID: accounts.ID{3}, URL: old}
		if err := s.Serve(response, httptest.NewRequest(http.MethodGet, "/", nil), version, relay, ""); err != nil {
			t.Fatalf("relay %v: %v", relay, err)
		}
		switch {
		case renewals != 1:
			t.Errorf("relay %v: %d renewals", relay, renewals)
		case !relay && (response.Code != http.StatusFound || response.Header().Get("Location") != fresh):
			t.Errorf("redirect: %d to %q", response.Code, response.Header().Get("Location"))
		case relay && (response.Code != http.StatusPartialContent || origin.targets[len(origin.targets)-1] != fresh):
			t.Errorf("relay: %d from %v", response.Code, origin.targets)
		}
	}
}

// fakeProbe writes an ffprobe stand-in that prints output and counts its
// runs, or fails.
func fakeProbe(t *testing.T, output string, fail bool) (string, func() int) {
	t.Helper()
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs")
	result := filepath.Join(dir, "output.json")
	if err := os.WriteFile(result, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho run >> " + runs + "\n"
	if fail {
		script += "echo 'Invalid data found when processing input' >&2\nexit 1\n"
	} else {
		script += "cat " + result + "\n"
	}
	path := filepath.Join(dir, "ffprobe")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path, func() int {
		data, _ := os.ReadFile(runs)
		return strings.Count(string(data), "run")
	}
}

func TestAnalysesAreKept(t *testing.T) {
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	path, runs := fakeProbe(t, string(probe), false)
	s := newService(t, &fakeSource{}, path, nil)
	version := library.Version{ID: accounts.ID{3}, URL: "https://93.184.216.34/movie.mp4", Size: 1234}
	first, err := s.Analyze(t.Context(), version)
	if err != nil {
		t.Fatal(err)
	}
	if first.Format == "" || len(first.Streams) == 0 {
		t.Fatalf("analysis: %+v", first)
	}
	again, err := s.Analyze(t.Context(), version)
	if err != nil || runs() != 1 || len(again.Streams) != len(first.Streams) {
		t.Fatalf("second analysis ran ffprobe again: %d runs, %v", runs(), err)
	}
	// Another process finds it in the database.
	other, err := New(s.db, &fakeSource{}, path, s.signer, s.sources, s.segments, nil, s.logger)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if stored, ok := other.Analyzed(t.Context(), version.ID); !ok || stored.Format != first.Format {
		t.Errorf("stored analysis: %+v %v", stored, ok)
	}
}

func TestAnalysesAreSavedWhenTheAppStopsWaiting(t *testing.T) {
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	path, _ := fakeProbe(t, string(probe), false)
	s := newService(t, &fakeSource{}, path, nil)
	version := library.Version{ID: accounts.ID{8}, URL: "https://93.184.216.34/movie.mp4", Size: 1234}
	// The app stopped waiting before the analysis ended.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Analyze(ctx, version); err != nil {
		t.Fatal(err)
	}
	// Polyfin, once restarted, finds it in the database.
	restarted, err := New(s.db, &fakeSource{}, path, s.signer, s.sources, s.segments, nil, s.logger)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if _, ok := restarted.Analyzed(t.Context(), version.ID); !ok {
		t.Error("the analysis was not saved")
	}
}

func TestShortClipsStandingInForTheTitleAreRefused(t *testing.T) {
	clip := `{"format": {"filename": "http://127.0.0.1/x", "format_name": "mov,mp4", "duration": "30.000000"},
		"streams": [{"index": 0, "codec_type": "video", "codec_name": "h264", "width": 3840, "height": 2160}]}`
	path, runs := fakeProbe(t, clip, false)
	s := newService(t, &fakeSource{}, path, nil)
	movie := library.Version{ID: accounts.ID{5}, URL: "https://93.184.216.34/movie.mkv", Runtime: 2 * time.Hour}
	if _, err := s.Analyze(t.Context(), movie); !errors.Is(err, ErrStandIn) {
		t.Fatalf("a 30 s clip for a 2 h movie: %v", err)
	}
	if _, stored := s.Analyzed(t.Context(), movie.ID); stored || !s.Failed(movie.ID) || runs() != 1 {
		t.Errorf("stored %v, failed %v, runs %d", stored, s.Failed(movie.ID), runs())
	}
	// A short title, or one without a known runtime, is what it is.
	for _, version := range []library.Version{
		{ID: accounts.ID{6}, URL: "https://93.184.216.34/short.mp4", Runtime: 4 * time.Minute},
		{ID: accounts.ID{7}, URL: "https://93.184.216.34/unknown.mp4"},
	} {
		if _, err := s.Analyze(t.Context(), version); err != nil {
			t.Errorf("runtime %s: %v", version.Runtime, err)
		}
	}
}

func TestUnreadableVersionsAreNotAnalyzedAgain(t *testing.T) {
	path, runs := fakeProbe(t, "", true)
	s := newService(t, &fakeSource{}, path, nil)
	version := library.Version{ID: accounts.ID{4}, URL: "https://93.184.216.34/page.html"}
	for range 2 {
		if _, err := s.Analyze(t.Context(), version); !errors.Is(err, media.ErrNotMedia) {
			t.Fatalf("got %v", err)
		}
	}
	if runs() != 1 || !s.Failed(version.ID) {
		t.Errorf("runs %d, failed %v", runs(), s.Failed(version.ID))
	}
}

func TestSessionsFollowReports(t *testing.T) {
	now := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	sessions := NewSessions()
	sessions.now = func() time.Time { return now }
	device, user, item := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}
	sessions.Start(device, user, PlayState{Item: item, MediaSourceID: "source", PlayMethod: "DirectPlay", CanSeek: true, Position: time.Minute})

	now = now.Add(10 * time.Second)
	playing, ok := sessions.Playing(device)
	if !ok || playing.Position != time.Minute+10*time.Second || playing.RepeatMode != "RepeatNone" {
		t.Fatalf("while playing: %+v", playing)
	}
	// A progress report keeps what it does not repeat, except CanSeek.
	sessions.Progress(device, user, PlayState{Position: 2 * time.Minute, Paused: true})
	pausedAt := now
	now = now.Add(time.Minute)
	sessions.Progress(device, user, PlayState{Position: 2 * time.Minute, Paused: true})
	now = now.Add(time.Minute)
	playing, _ = sessions.Playing(device)
	if playing.Position != 2*time.Minute || playing.MediaSourceID != "source" || playing.PlayMethod != "DirectPlay" || playing.CanSeek {
		t.Errorf("paused: %+v", playing)
	}
	// The pause dates from its first report, and ends with playback.
	if !playing.PausedAt.Equal(pausedAt) {
		t.Errorf("paused at %v, want %v", playing.PausedAt, pausedAt)
	}
	sessions.Progress(device, user, PlayState{Position: 2 * time.Minute})
	if playing, _ := sessions.Playing(device); !playing.PausedAt.IsZero() {
		t.Errorf("still paused since %v", playing.PausedAt)
	}
	// A report for another item starts it.
	sessions.Progress(device, user, PlayState{Item: accounts.ID{9}})
	if playing, _ := sessions.Playing(device); playing.Item != (accounts.ID{9}) || playing.MediaSourceID != "" {
		t.Errorf("next item: %+v", playing)
	}
	sessions.Stop(device)
	if _, ok := sessions.Playing(device); ok {
		t.Error("still playing after the stop report")
	}
}

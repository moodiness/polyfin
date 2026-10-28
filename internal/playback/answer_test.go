package playback

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// answeringHost answers every request for a file with the same status,
// type and body, whatever range it asks, as a site that answers a page
// or an error instead of a video does.
type answeringHost struct {
	status      int
	contentType string
	body        []byte
}

func (h answeringHost) Open(ctx context.Context, method, target string, _ http.Header, _ bool) (*http.Response, error) {
	header := http.Header{"Content-Length": {strconv.Itoa(len(h.body))}}
	if h.contentType != "" {
		header.Set("Content-Type", h.contentType)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: h.status, Header: header, ContentLength: int64(len(h.body)), Request: request,
		Body: io.NopCloser(bytes.NewReader(h.body))}, nil
}

// A source that answers no video, but a web page, JSON, nothing, or an
// error with a page, fails its analysis with what it answered: its kind,
// status and type, never its address nor its body.
func TestAnalysisTellsWhatTheSourceAnswered(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	page := []byte("<!DOCTYPE html><html><head><title>Access denied: secret-token</title></head><body>" +
		strings.Repeat(" ", 2048) + "</body></html>")
	for _, tc := range []struct {
		name  string
		host  answeringHost
		notes []string
	}{
		{"a page", answeringHost{http.StatusOK, "text/html; charset=utf-8", page}, []string{"a web page", "HTTP 200", "text/html"}},
		{"a page sent as bytes", answeringHost{http.StatusOK, "application/octet-stream", page}, []string{"a web page", "application/octet-stream"}},
		{"JSON", answeringHost{http.StatusOK, "application/json", []byte(`{"error": "secret-token not allowed"}`)}, []string{"JSON", "HTTP 200"}},
		{"nothing", answeringHost{http.StatusOK, "video/mp4", nil}, []string{"an empty body"}},
		{"a refusal with a page", answeringHost{http.StatusForbidden, "text/html", page}, []string{"HTTP 403", "a web page", "text/html"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newService(t, tc.host, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), nil)
			version := library.Version{ID: accounts.ID{1}, URL: "https://93.184.216.34/stream/secret-path", Addon: "Sport"}
			_, err := s.Analyze(t.Context(), version)
			if err == nil {
				t.Fatal("analyzed")
			}
			t.Log(err)
			for _, note := range tc.notes {
				if !strings.Contains(err.Error(), note) {
					t.Errorf("%q does not tell %q", err, note)
				}
			}
			if strings.Contains(err.Error(), "secret") {
				t.Errorf("%q tells the address or the body", err)
			}
			if tc.host.status == http.StatusOK && !errors.Is(err, media.ErrNotMedia) || tc.host.status != http.StatusOK && !errors.Is(err, source.ErrUnavailable) {
				t.Errorf("%v is not what it was", err)
			}
			// The failure kept is the same, without analyzing again.
			if _, again := s.Analyze(t.Context(), version); again == nil || again.Error() != err.Error() {
				t.Errorf("the failure kept: %v", again)
			}
		})
	}
}

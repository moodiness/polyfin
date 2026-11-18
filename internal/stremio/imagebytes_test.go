package stremio

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Artwork is downloaded only when its bytes are a raster picture's,
// whatever type its server gives it, and the type returned is the bytes':
// an SVG image, a document, is refused as an image its server does not
// have.
func TestArtworkIsDownloadedByWhatItsBytesAre(t *testing.T) {
	t.Parallel()
	const (
		png = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
		svg = `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>document.title = 'ran'</script></svg>`
	)
	answers := map[string][2]string{
		"/poster.png":      {"image/png", png},
		"/mislabelled.svg": {"image/svg+xml", png},
		"/poster.svg":      {"image/svg+xml", svg},
		"/labelled.png":    {"image/png", svg},
		"/page.jpg":        {"image/jpeg", "<!doctype html><html><body></body></html>"},
		"/empty.jpg":       {"image/jpeg", ""},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer, ok := answers[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", answer[0])
		_, _ = w.Write([]byte(answer[1]))
	}))
	t.Cleanup(server.Close)
	client := NewClient("test")
	for _, path := range []string{"/poster.png", "/mislabelled.svg"} {
		body, contentType, err := client.Image(t.Context(), server.URL+path, false)
		if err != nil || string(body) != png || contentType != "image/png" {
			t.Errorf("%s: %q %q %v, want the picture as image/png", path, body, contentType, err)
		}
	}
	for _, path := range []string{"/poster.svg", "/labelled.png", "/page.jpg", "/empty.jpg"} {
		body, contentType, err := client.Image(t.Context(), server.URL+path, false)
		if body != nil || contentType != "" || !errors.Is(err, ErrNoPicture) {
			t.Errorf("%s: %q %q %v, want no picture", path, body, contentType, err)
		}
		// To its callers, an image its server does not have.
		if !errors.Is(err, ErrUnreachable) || StatusOf(err) != http.StatusNotFound {
			t.Errorf("%s: %v, status %d, want a missing image", path, err, StatusOf(err))
		}
	}
}

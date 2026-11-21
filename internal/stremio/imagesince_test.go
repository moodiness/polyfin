package stremio

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Artwork asked with the validators its server gave is answered 304 with
// no picture when it did not change, by its ETag or its Last-Modified;
// the validators answered, else those sent, are returned. A server that
// sends the picture anyway gives it in full.
func TestArtworkAskedWhetherItChanged(t *testing.T) {
	t.Parallel()
	const (
		png      = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
		etag     = `"v1"`
		modified = "Sat, 21 Nov 2026 08:00:00 GMT"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tagged.png":
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		case "/dated.png":
			if r.Header.Get("If-Modified-Since") == modified {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Last-Modified", modified)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(png))
	}))
	t.Cleanup(server.Close)
	client := NewClient("test")
	for path, want := range map[string]Validators{"/tagged.png": {ETag: etag}, "/dated.png": {LastModified: modified}} {
		first, err := client.ImageSince(t.Context(), server.URL+path, false, Validators{})
		if err != nil || string(first.Body) != png || first.Unchanged || first.Validators != want {
			t.Fatalf("%s: %+v %v, want the picture with %+v", path, first, err, want)
		}
		again, err := client.ImageSince(t.Context(), server.URL+path, false, first.Validators)
		if err != nil || !again.Unchanged || again.Body != nil || again.Validators != want {
			t.Errorf("%s asked again: %+v %v, want unchanged with %+v", path, again, err, want)
		}
	}
	// A server that ignores the validators sends the picture.
	plain, err := client.ImageSince(t.Context(), server.URL+"/plain.png", false, Validators{ETag: etag})
	if err != nil || string(plain.Body) != png || plain.Unchanged {
		t.Errorf("a server without validators: %+v %v, want the picture", plain, err)
	}
}

package hls

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// A subtitle file to burn in is read from the address its source gave,
// which may hold the source's key: the host an answer redirects to is not
// told that address, for the .idx file nor for the .sub file beside it.
func TestARedirectedSubtitleFileDoesNotTellTheAddressItComesFrom(t *testing.T) {
	t.Parallel()
	served := t.TempDir()
	idx, sub := sentinelRectangle.VobSub()
	for name, data := range map[string][]byte{"a.sup": sentinelRectangle.PGS(), "b.idx": []byte(idx), "b.sub": sub} {
		if err := os.WriteFile(filepath.Join(served, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var received []http.Header
	files := http.FileServer(http.Dir(served))
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Clone())
		mu.Unlock()
		files.ServeHTTP(w, r)
	}))
	defer other.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/"+filepath.Base(r.URL.Path), http.StatusFound)
	}))
	defer source.Close()
	for _, name := range []string{"a.sup", "b.idx"} {
		original := source.URL + "/key-1/subtitles/" + name
		if _, err := writeBurnCopy(t.Context(), original, filepath.Join(t.TempDir(), "burn"), sentinelTime(30*time.Second)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) < 3 {
		t.Fatalf("the other host was asked %d times, want the three files", len(received))
	}
	for _, header := range received {
		if referer, set := header["Referer"]; set {
			t.Errorf("the other host was sent Referer %q", referer)
		}
	}
}

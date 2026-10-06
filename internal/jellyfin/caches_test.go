package jellyfin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

func TestSubtitleTextsAreKeptUpTo32MB(t *testing.T) {
	s := newTestServer(t, 0)
	// A SubRip file of about 7 MB, under the 8 MB a download may take: a
	// few of them go past the 32 MB kept.
	var file strings.Builder
	for n := 0; file.Len() < 7<<20; n++ {
		fmt.Fprintf(&file, "%d\n%02d:%02d:%02d,000 --> %02d:%02d:%02d,500\n%s\n\n", n+1, n/3600, n/60%60, n%60, n/3600, n/60%60, n%60,
			strings.Repeat("A line of dialogue. ", 5))
	}
	var mu sync.Mutex
	downloads := map[string]int{}
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		downloads[r.URL.Path]++
		mu.Unlock()
		_, _ = w.Write([]byte(file.String()))
	}))
	t.Cleanup(files.Close)
	read := func(n int) {
		t.Helper()
		subtitle := library.ExternalSubtitle{ID: accounts.ID{byte(n + 1)}, URL: fmt.Sprintf("%s/%d.srt", files.URL, n)}
		if _, err := s.handler.subtitleFile(t.Context(), subtitle); err != nil {
			t.Fatal(err)
		}
	}
	for n := range 5 {
		read(n)
	}
	read(4)
	read(0)
	mu.Lock()
	defer mu.Unlock()
	if downloads["/4.srt"] != 1 {
		t.Errorf("the file read last was downloaded %d times", downloads["/4.srt"])
	}
	if downloads["/0.srt"] != 2 {
		t.Errorf("the file read first, past 32 MB, was downloaded %d times", downloads["/0.srt"])
	}
	if kept := s.handler.subtitleCache.Bytes(); kept > 32<<20 || kept < 16<<20 {
		t.Errorf("%d bytes of subtitle text kept", kept)
	}
}

func TestSubtitleListsAreKeptUpTo16MB(t *testing.T) {
	s := newTestServer(t, 0)
	// Lists of about 1 MB: a thousand files of about 1 KB.
	files := make([]library.ExternalSubtitle, 1000)
	for i := range files {
		files[i] = library.ExternalSubtitle{ID: accounts.ID{byte(i), byte(i >> 8)}, Language: "eng",
			URL: "https://subtitles.test/" + strings.Repeat("x", 1000)}
	}
	for n := range 20 {
		s.handler.subtitleFiles.Put(accounts.ID{byte(n + 1)}, files)
	}
	if _, ok := s.handler.subtitleFiles.Get(accounts.ID{1}); ok {
		t.Error("the list stored first was kept past 16 MB")
	}
	if _, ok := s.handler.subtitleFiles.Get(accounts.ID{20}); !ok {
		t.Error("the list stored last was not kept")
	}
	if kept := s.handler.subtitleFiles.Bytes(); kept > 16<<20 || kept < 8<<20 {
		t.Errorf("%d bytes of subtitle lists kept", kept)
	}
}

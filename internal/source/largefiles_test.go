package source

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// largeFiles serves files of any size without holding them: the path
// /<size> is a file of size bytes, the byte at each offset its offset
// modulo 251, served by ranges.
func largeFiles(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		size, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/"), 10, 64)
		first, last, ranged := strings.Cut(strings.TrimPrefix(r.Header.Get("Range"), "bytes="), "-")
		start, startErr := strconv.ParseInt(first, 10, 64)
		if err != nil || !ranged || startErr != nil || strings.Contains(last, ",") {
			http.Error(w, "a single range is served", http.StatusBadRequest)
			return
		}
		end := size - 1
		if last != "" {
			if asked, err := strconv.ParseInt(last, 10, 64); err == nil {
				end = min(end, asked)
			}
		}
		if start > end {
			w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(size, 10))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		// By chunks, until the range ends or the client goes away: an open
		// range asks gigabytes.
		chunk := make([]byte, 64<<10)
		for off := start; off <= end; off += int64(len(chunk)) {
			part := chunk[:min(int64(len(chunk)), end-off+1)]
			for i := range part {
				part[i] = largeByte(off + int64(i))
			}
			if _, err := w.Write(part); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func largeByte(off int64) byte { return byte(off % 251) }

// The sizes an addon announces past 2 GiB, and past 4 GiB, reach the
// source cache whole on every target: a file of the size announced is
// read, at offsets past 4 GiB too, and one of another size is refused once
// the addon's sizes are trusted. On 32-bit targets, the size decoded into
// an int was cut to 2^31-1 or -2^31, so that the first refused the right
// file and the second checked nothing.
func TestAnnouncedSizesPast2GiBAreChecked(t *testing.T) {
	t.Parallel()
	server := largeFiles(t)
	cache := newCache(t, 1<<30)
	var streams []stremio.Stream
	if err := json.Unmarshal([]byte(`[
		{"url": "/5000000000", "behaviorHints": {"videoSize": 5e9}},
		{"url": "/3000000007", "behaviorHints": {"videoSize": 3000000000}},
		{"url": "/2400000000", "behaviorHints": {"videoSize": "2400000000"}},
		{"url": "/4000000000", "behaviorHints": {"videoSize": 6e9}}
	]`), &streams); err != nil {
		t.Fatal(err)
	}
	open := func(i int) *Source {
		stream := streams[i]
		s := cache.Open(accounts.ID{byte(i + 1)}, Location{URL: server.URL + stream.URL, Size: int64(stream.BehaviorHints.VideoSize), Announcer: "addon"}, nil)
		t.Cleanup(s.Release)
		return s
	}
	for i, want := range []int64{5_000_000_000, 3_000_000_007, 2_400_000_000} {
		s := open(i)
		for _, off := range []int64{0, want - 100, 4_500_000_000 % want} {
			got := make([]byte, 10)
			if _, err := s.ReadAt(t.Context(), got, off); err != nil {
				t.Fatalf("a file of %d bytes, at %d: %v", want, off, err)
			}
			for j, b := range got {
				if b != largeByte(off+int64(j)) {
					t.Fatalf("a file of %d bytes: byte %d is %d", want, off+int64(j), b)
				}
			}
		}
		if known, _ := s.KnownSize(); known != want {
			t.Errorf("a file of %d bytes taken as %d", want, known)
		}
	}
	if !cache.sizeTrusted("addon") {
		t.Fatal("the addon's sizes past 2 GiB are not trusted")
	}
	_, err := open(3).ReadAt(t.Context(), make([]byte, 10), 0)
	if !errors.Is(err, ErrUnavailable) || Answer(err) != "an answer of another size than the file" {
		t.Errorf("a file of 4000000000 bytes announced as 6000000000: %v", err)
	}
}

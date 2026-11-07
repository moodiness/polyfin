package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// pattern is the bytes of the files outageHost answers, made as they are
// read: byte off % 251 at off.
type pattern struct{}

func (pattern) ReadAt(p []byte, off int64) (int, error) {
	for i := range p {
		p[i] = byte((off + int64(i)) % 251)
	}
	return len(p), nil
}

// outageHost answers every link with a file of size bytes, or, while down,
// with the same error file of errorSize bytes, as a host whose service is
// down answers a short video for every link. Its bodies are read as they
// are sent: a file of gigabytes takes no memory.
type outageHost struct {
	size, errorSize int64
	down            atomic.Bool
}

func (h *outageHost) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	size := h.size
	if h.down.Load() {
		size = h.errorSize
	}
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: request}
	start, end := int64(0), size-1
	if spec, ok := strings.CutPrefix(header.Get("Range"), "bytes="); ok {
		from, to, _ := strings.Cut(spec, "-")
		start, _ = strconv.ParseInt(from, 10, 64)
		if last, err := strconv.ParseInt(to, 10, 64); err == nil {
			end = min(last, end)
		}
		if start >= size {
			response.StatusCode = http.StatusRequestedRangeNotSatisfiable
			response.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			response.Body = io.NopCloser(bytes.NewReader(nil))
			return response, nil
		}
		response.StatusCode = http.StatusPartialContent
		response.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	}
	response.ContentLength = end - start + 1
	response.Header.Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	response.Header.Set("Content-Type", "video/x-matroska")
	response.Body = io.NopCloser(io.NewSectionReader(pattern{}, start, response.ContentLength))
	return response, nil
}

// hostCache is a cache in dir reading its sources from host, keeping its
// blocks across restarts, closed at the end of the test.
func hostCache(t *testing.T, dir string, host Opener) *Cache {
	t.Helper()
	cache, err := New(dir, func() Policy { return Policy{Limit: 1 << 30, Keep: 24 * time.Hour} }, host,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return cache
}

// fileLink is the link of the file of id on the outage host.
func fileLink(id byte) string {
	return "https://files.example:8443/" + strconv.Itoa(int(id)) + ".mkv"
}

// readFirst opens the source of id at its link, its addon announcing size,
// reads its first bytes, and releases it.
func readFirst(t *testing.T, cache *Cache, id byte, size int64) error {
	t.Helper()
	s := cache.Open(accounts.ID{id}, Location{URL: fileLink(id), Size: size, Announcer: "addon"}, nil)
	defer s.Release()
	_, err := s.ReadAt(t.Context(), make([]byte, 10), 0)
	return err
}

// refused reports whether err refuses an answer for another file.
func refused(err error) bool {
	return errors.Is(err, ErrUnavailable) && Answer(err) == "an answer of another size than the file"
}

// One host answering the same small size for several files, each far
// over it, answers an error file: told from the third file, not for one
// alone, by its name, as far as the versions of that host. Once it answers
// a file of its own size, it is no longer told, and the files refused
// meanwhile are tried again at once: the one that took the error file for
// its own, before the host was found answering it, reads its file anew,
// once the reader that held it through the recovery releases it.
func TestAHostAnsweringAnErrorFileIsToldUntilItAnswersAFile(t *testing.T) {
	const size, errorSize = 32 * blockSize, 1_822_520
	host := &outageHost{size: size, errorSize: errorSize}
	host.down.Store(true)
	cache := hostCache(t, t.TempDir(), host)
	var mu sync.Mutex
	var retried []accounts.ID
	cache.OnRecovered(func(sources []accounts.ID) {
		mu.Lock()
		defer mu.Unlock()
		retried = append(retried, sources...)
	})

	started := time.Now()
	// One file answered small, even twice: a link may name another file.
	// Its addon's sizes are not trusted yet: the answer is taken.
	for range 2 {
		if err := readFirst(t, cache, 1, size); err != nil {
			t.Fatal(err)
		}
	}
	if told := cache.ErrorFiles(); len(told) != 0 {
		t.Fatalf("one file told %+v", told)
	}
	held := cache.Open(accounts.ID{1}, Location{URL: fileLink(1), Size: size, Announcer: "addon"}, nil)
	second := time.Now()
	if err := readFirst(t, cache, 2, size); !refused(err) {
		t.Fatalf("a second file of the same size: %v", err)
	}
	if told := cache.ErrorFiles(); len(told) != 0 {
		t.Fatalf("two files told %+v", told)
	}
	if err := readFirst(t, cache, 3, size); !refused(err) {
		t.Fatalf("a third file of the same size: %v", err)
	}
	told := cache.ErrorFiles()
	if len(told) != 1 || told[0].Host != "files.example" || told[0].Size != errorSize ||
		told[0].Since.Before(started) || !told[0].Since.Before(second) {
		t.Fatalf("told %+v, the first answer between %v and %v", told, started, second)
	}
	for _, c := range []struct {
		id   byte
		link string
		told bool
	}{
		{2, "", true},
		{9, fileLink(9), true},
		{9, "https://other.example/9.mkv", false},
	} {
		if _, ok := cache.ErrorFileOf(accounts.ID{c.id}, c.link); ok != c.told {
			t.Errorf("the file %d at %q: told %v", c.id, c.link, ok)
		}
	}

	host.down.Store(false)
	if err := readFirst(t, cache, 4, size); err != nil {
		t.Fatal(err)
	}
	if told := cache.ErrorFiles(); len(told) != 0 {
		t.Fatalf("still told %+v", told)
	}
	mu.Lock()
	slices.SortFunc(retried, func(a, b accounts.ID) int { return bytes.Compare(a[:], b[:]) })
	if want := []accounts.ID{{1}, {2}, {3}}; !slices.Equal(retried, want) {
		t.Errorf("tried again %v, want %v", retried, want)
	}
	mu.Unlock()
	if known, _ := held.KnownSize(); known != errorSize {
		t.Errorf("the reader holding the file through the recovery reads %d bytes", known)
	}
	held.Release()
	for id := byte(1); id <= 3; id++ {
		s := cache.Open(accounts.ID{id}, Location{URL: fileLink(id), Size: size, Announcer: "addon"}, nil)
		got, err := s.Size(t.Context())
		s.Release()
		if err != nil || got != size {
			t.Errorf("the file %d after the outage: %d bytes, %v", id, got, err)
		}
	}
}

// Versions that all answer the same real episode, hundreds of megabytes,
// under the size of a pack their addon announced for each, are no error
// file, however many: a video is never that small.
func TestRealEpisodesUnderAPackSizeAreNoErrorFile(t *testing.T) {
	host := &outageHost{errorSize: 350_000_000}
	host.down.Store(true)
	cache := hostCache(t, t.TempDir(), host)
	for id := byte(1); id <= 4; id++ {
		if err := readFirst(t, cache, id, 10_000_000_000); err != nil {
			t.Fatalf("the episode %d: %v", id, err)
		}
	}
	if told := cache.ErrorFiles(); len(told) != 0 {
		t.Errorf("told %+v", told)
	}
}

// A host answering an error file never makes the sizes an addon
// announces untrusted: they refuse its error file all through the
// outage, and another file at its first byte after it. A file whose
// count was taken back counts again once its own size is answered.
func TestAnErrorFileLeavesTheAnnouncedSizesOn(t *testing.T) {
	const size = 32 * blockSize
	host := &outageHost{size: size, errorSize: 1_822_520}
	cache := hostCache(t, t.TempDir(), host)
	if err := readFirst(t, cache, 1, size); err != nil {
		t.Fatal(err)
	}
	host.down.Store(true)
	for id := byte(2); id < 9; id++ {
		if err := readFirst(t, cache, id, size); !refused(err) {
			t.Fatalf("the file %d during the outage: %v", id, err)
		}
	}
	if !cache.sizeTrusted("addon") {
		t.Fatal("the outage made the addon's sizes untrusted")
	}
	host.down.Store(false)
	if err := readFirst(t, cache, 20, 48*blockSize); !refused(err) {
		t.Errorf("another file after the outage: %v", err)
	}
	again := cache.Open(accounts.ID{2}, Location{URL: fileLink(2), Size: size, Announcer: "addon"}, nil)
	again.Retry()
	_, err := again.ReadAt(t.Context(), make([]byte, 10), 0)
	again.Release()
	if err != nil {
		t.Fatal(err)
	}
	if record := cache.sizes["addon"]; record.agreed != 2 {
		t.Errorf("the addon's record: %+v", *record)
	}
}

// With the size its analysis found, a source refuses an error file at its
// first byte, before its host is found answering one, and keeps the
// blocks a previous run kept of its file, as it does when its addon
// announced their size. The file's own answer then reads the blocks kept.
func TestAnErrorFileLeavesTheBlocksKept(t *testing.T) {
	const size, errorSize = 8_350_994_751, 1_822_520
	for _, c := range []struct {
		name     string
		location Location
	}{
		{"analyzed and announced", Location{Analyzed: size, Size: 8_400_000_000, Announcer: "addon"}},
		{"analyzed", Location{Analyzed: size}},
		{"announced", Location{Size: size, Announcer: "addon"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			host := &outageHost{size: size, errorSize: errorSize}
			dir := t.TempDir()
			before := hostCache(t, dir, host)
			s := before.Open(accounts.ID{1}, Location{URL: fileLink(1) + "?token=1"}, nil)
			if _, err := s.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
				t.Fatal(err)
			}
			s.Release()
			if err := before.Close(); err != nil {
				t.Fatal(err)
			}

			after := hostCache(t, dir, host)
			kept := after.Usage().Used
			if kept == 0 {
				t.Fatal("nothing kept")
			}
			host.down.Store(true)
			location := c.location
			location.URL = fileLink(1) + "?token=2"
			s = after.Open(accounts.ID{1}, location, nil)
			defer s.Release()
			_, err := s.ReadAt(t.Context(), make([]byte, 10), 0)
			if !refused(err) {
				t.Fatalf("the error file: %v", err)
			}
			if used := after.Usage().Used; used != kept {
				t.Fatalf("%d bytes kept after the error file, %d before", used, kept)
			}

			host.down.Store(false)
			s.Retry()
			got := make([]byte, 10)
			if _, err := s.ReadAt(t.Context(), got, 5); err != nil {
				t.Fatal(err)
			}
			want := make([]byte, 10)
			_, _ = pattern{}.ReadAt(want, 5)
			if !bytes.Equal(got, want) {
				t.Error("content differs")
			}
			if known, _ := s.KnownSize(); known != size {
				t.Errorf("size %d", known)
			}
		})
	}
}

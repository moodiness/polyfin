package source

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// memoryFile is a file read at offsets, counting its reads.
type memoryFile struct {
	*bytes.Reader
	reads *atomic.Int32
}

func (f memoryFile) ReadAt(p []byte, off int64) (int, error) {
	f.reads.Add(1)
	return f.Reader.ReadAt(p, off)
}

func (memoryFile) Close() error { return nil }

// A file read at offsets, as over SSH, answers ranges as a server does,
// and is read through the cache in blocks: a seek reads only the blocks
// missing.
func TestFileOpenerAnswersRanges(t *testing.T) {
	data := make([]byte, 5*blockSize+123)
	for i := range data {
		data[i] = byte(i % 251)
	}
	var opens, reads atomic.Int32
	opener := FileOpener(func(_ context.Context, target string) (RandomFile, int64, error) {
		if !strings.HasSuffix(target, "/movie.mkv") {
			return nil, 0, fs.ErrNotExist
		}
		opens.Add(1)
		return memoryFile{bytes.NewReader(data), &reads}, int64(len(data)), nil
	})
	for _, test := range []struct {
		ranges string
		status int
		body   []byte
		header string
	}{
		{"bytes=10-19", http.StatusPartialContent, data[10:20], "bytes 10-19/" + strconv.Itoa(len(data))},
		{"bytes=" + strconv.Itoa(len(data)-5) + "-", http.StatusPartialContent, data[len(data)-5:], ""},
		{"bytes=-3", http.StatusPartialContent, data[len(data)-3:], ""},
		{"bytes=0-1,5-6", http.StatusOK, data, ""},
		{"", http.StatusOK, data, ""},
		{"bytes=" + strconv.Itoa(len(data)) + "-", http.StatusRequestedRangeNotSatisfiable, nil, "bytes */" + strconv.Itoa(len(data))},
	} {
		response, err := opener.Open(t.Context(), http.MethodGet, "sftp://host/movie.mkv", http.Header{"Range": {test.ranges}}, false)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != test.status || test.body != nil && !bytes.Equal(body, test.body) ||
			test.header != "" && response.Header.Get("Content-Range") != test.header {
			t.Errorf("%q: %d %q, %d bytes, %v", test.ranges, response.StatusCode, response.Header.Get("Content-Range"), len(body), err)
		}
	}
	if response, err := opener.Open(t.Context(), http.MethodGet, "sftp://host/other.mkv", http.Header{}, false); err != nil ||
		response.StatusCode != http.StatusNotFound {
		t.Errorf("a missing file: %v", err)
	}

	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: "sftp://host/movie.mkv", Opener: opener}, nil)
	defer s.Release()
	read := func(off int64) {
		t.Helper()
		got := make([]byte, 1000)
		if _, err := s.ReadAt(t.Context(), got, off); err != nil || !bytes.Equal(got, data[off:off+1000]) {
			t.Fatalf("at %d: %v", off, err)
		}
	}
	opens.Store(0)
	read(4 * blockSize)
	read(5)
	before := opens.Load()
	read(4*blockSize + 10)
	read(100)
	if after := opens.Load(); before != 2 || after != before {
		t.Errorf("opens: %d then %d", before, after)
	}
}

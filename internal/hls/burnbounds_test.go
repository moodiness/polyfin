package hls

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	// shareExcess is how much more than its bound an oversized file of a
	// subtitleShare holds.
	shareExcess = 64 << 20
	// shareSlack is what the buffers of a connection may hold of a file
	// whose reader stopped: the share counts what it gave, and the
	// system's buffers take a few MiB after the reader's last block.
	shareSlack = 32 << 20
)

// subtitleShare stands for a library's share: it serves files written as
// they are asked for, and counts the bytes it gave of each.
type subtitleShare struct {
	*httptest.Server
	running sync.WaitGroup
	given   map[string]*atomic.Int64
}

type countedWriter struct {
	w     io.Writer
	count *atomic.Int64
}

func (c countedWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.count.Add(int64(n))
	return n, err
}

// newSubtitleShare serves files, each by the function writing it, which
// stops at the first error: the reader left.
func newSubtitleShare(t *testing.T, files map[string]func(io.Writer) error) *subtitleShare {
	t.Helper()
	s := &subtitleShare{given: map[string]*atomic.Int64{}}
	for name := range files {
		s.given[name] = &atomic.Int64{}
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.running.Add(1)
		defer s.running.Done()
		name := strings.TrimPrefix(r.URL.Path, "/")
		write := files[name]
		if write == nil {
			http.NotFound(w, r)
			return
		}
		_ = write(countedWriter{w, s.given[name]})
	}))
	t.Cleanup(s.Close)
	return s
}

// gave is how many bytes of the file the share gave, once it has stopped
// giving any.
func (s *subtitleShare) gave(name string) int64 {
	s.running.Wait()
	return s.given[name].Load()
}

// repeated writes n bytes after head: block, again and again.
func repeated(head string, block []byte, n int64) func(io.Writer) error {
	return func(w io.Writer) error {
		if _, err := io.WriteString(w, head); err != nil {
			return err
		}
		for left := n; left > 0; {
			part := block[:min(int64(len(block)), left)]
			if _, err := w.Write(part); err != nil {
				return err
			}
			left -= int64(len(part))
		}
		return nil
	}
}

// pgsOfSize writes a PGS file of n bytes: the sentinel tests' rectangle,
// then segments that are no presentation, at time zero.
func pgsOfSize(n int64) func(io.Writer) error {
	return func(w io.Writer) error {
		rectangle := sentinelRectangle.PGS()
		if _, err := w.Write(rectangle); err != nil {
			return err
		}
		segment := make([]byte, pgsSegment+0xFFFF)
		copy(segment, "PG")
		segment[10] = 0x17
		for left := n - int64(len(rectangle)); left > 0; {
			size := min(int64(len(segment)), left)
			// What is left after it holds a segment's header.
			if rest := left - size; rest > 0 && rest < pgsSegment {
				size -= pgsSegment
			}
			binary.BigEndian.PutUint16(segment[11:], uint16(size-pgsSegment))
			if _, err := w.Write(segment[:size]); err != nil {
				return err
			}
			left -= size
		}
		return nil
	}
}

// paddedIndex is the sentinel tests' .idx file followed by comment lines,
// n bytes in all.
func paddedIndex(idx string, n int) string {
	const line = "# padding\n"
	padding := n - len(idx)
	whole := padding/len(line) - 1
	last := padding - whole*len(line)
	return idx + strings.Repeat(line, whole) + "#" + strings.Repeat("x", last-2) + "\n"
}

// A subtitle file larger than a movie's is neither read nor copied past
// its bound: the share is left there, and nothing of the copy stays in
// the folder but the .idx file as read of a pair whose .sub file is past
// its bound, which FFmpeg then reads itself as a stream, as it does a PGS
// file past it; a VobSub pair whose .idx file is past its own is not
// burned in, as FFmpeg would hold all of it.
func TestBurnCopyStopsReadingAtItsBounds(t *testing.T) {
	t.Parallel()
	idx, sub := sentinelRectangle.VobSub()
	zeros := make([]byte, 64<<10)
	comments := bytes.Repeat([]byte("# padding\n"), 4096)
	for _, c := range []struct {
		name     string
		files    map[string]func(io.Writer) error
		original string
		// read is the file read past its bound, bound that bound, and
		// unread a file that is then not asked for.
		read    string
		bound   int64
		unread  string
		refused bool
		// asRead is a pair whose .idx file is copied as read.
		asRead bool
	}{
		{name: "an .idx file", original: "Movie.idx", read: "Movie.idx", bound: maxIndexBytes, unread: "Movie.sub", refused: true,
			files: map[string]func(io.Writer) error{
				"Movie.idx": repeated(idx, comments, maxIndexBytes+shareExcess-int64(len(idx))),
				"Movie.sub": repeated(string(sub), nil, 0),
			}},
		{name: "a .sub file", original: "Movie.idx", read: "Movie.sub", bound: maxBurnBytes, asRead: true,
			files: map[string]func(io.Writer) error{
				"Movie.idx": repeated(idx, nil, 0),
				"Movie.sub": repeated(string(sub), zeros, maxBurnBytes+shareExcess-int64(len(sub))),
			}},
		{name: "a PGS file", original: "Movie.sup", read: "Movie.sup", bound: maxBurnBytes,
			files: map[string]func(io.Writer) error{"Movie.sup": pgsOfSize(maxBurnBytes + shareExcess)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			share := newSubtitleShare(t, c.files)
			dir := t.TempDir()
			original := share.URL + "/" + c.original
			file, sub, err := writeBurnCopy(t.Context(), original, filepath.Join(dir, "burn"), time.Hour)
			if !errors.Is(err, errNoSentinel) || errors.Is(err, errIndexTooLarge) != c.refused {
				t.Errorf("writeBurnCopy: %v, want errNoSentinel, and errIndexTooLarge: %v", err, c.refused)
			}
			if c.asRead {
				if got, _ := os.ReadFile(file); file != filepath.Join(dir, "burn.idx") || string(got) != idx || sub != share.URL+"/Movie.sub" {
					t.Errorf("FFmpeg reads %s with %s, want the .idx file as read and the .sub file itself", file, sub)
				}
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			} else if file != "" || sub != "" {
				t.Errorf("FFmpeg reads %s with %s, want no file", file, sub)
			}
			gave := share.gave(c.read)
			t.Logf("the share gave %d bytes of %s past its bound of %d", gave-c.bound, c.read, c.bound)
			if gave > c.bound+shareSlack {
				t.Errorf("the share gave %d bytes of %s, want its bound of %d and what the connection held", gave, c.read, c.bound)
			}
			if c.unread != "" && share.gave(c.unread) != 0 {
				t.Errorf("the share gave %d bytes of %s, want it not asked for", share.gave(c.unread), c.unread)
			}
			if entries, _ := os.ReadDir(dir); len(entries) > 0 {
				t.Errorf("left %v behind", entries)
			}
			if !c.refused {
				return
			}
			// The Manager gives FFmpeg no file, and the conversion burns
			// none in.
			m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			r := &Remux{Encode: &VideoEncoding{BurnFile: original}}
			file, _, ok := m.sentinelCopy(t.Context(), r, filepath.Join(dir, "burn"))
			if ok || file != "" {
				t.Errorf("FFmpeg is given %q, want no file", file)
			}
			if burning(r, file, "").Encode.Burns() {
				t.Error("the conversion still burns a file in")
			}
			if entries, _ := os.ReadDir(dir); len(entries) > 0 {
				t.Errorf("left %v behind", entries)
			}
		})
	}
}

// Files of exactly their bounds are copied whole, with the sentinel the
// small ones get.
func TestBurnCopyTakesFilesAtItsBounds(t *testing.T) {
	t.Parallel()
	at := sentinelTime(30 * time.Second)
	// ends is the file's size, its first head bytes and its last tail.
	ends := func(t *testing.T, name string, head, tail int) (int64, []byte, []byte) {
		t.Helper()
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		first, last := make([]byte, head), make([]byte, tail)
		if _, err := f.ReadAt(first, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := f.ReadAt(last, info.Size()-int64(tail)); err != nil {
			t.Fatal(err)
		}
		return info.Size(), first, last
	}
	t.Run("a VobSub pair", func(t *testing.T) {
		t.Parallel()
		idx, sub := sentinelRectangle.VobSub()
		padded := paddedIndex(idx, maxIndexBytes)
		if len(padded) != maxIndexBytes {
			t.Fatalf("an .idx file of %d bytes, want %d", len(padded), maxIndexBytes)
		}
		share := newSubtitleShare(t, map[string]func(io.Writer) error{
			"Movie.idx": repeated(padded, nil, 0),
			"Movie.sub": repeated(string(sub), make([]byte, 64<<10), maxBurnBytes-int64(len(sub))),
		})
		dir := t.TempDir()
		copied, _, err := writeBurnCopy(t.Context(), share.URL+"/Movie.idx", filepath.Join(dir, "burn"), at)
		if err != nil {
			t.Fatal(err)
		}
		marked, packs, err := vobSubSentinel(idx, maxBurnBytes, at)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(copied); copied != filepath.Join(dir, "burn.idx") || string(got) != marked+padded[len(idx):] {
			t.Errorf("copied to %s, %d bytes, not the small file's index of %d and the padding of %d", copied, len(got), len(marked), len(padded)-len(idx))
		}
		size, head, tail := ends(t, filepath.Join(dir, "burn.sub"), len(sub), len(packs))
		if size != maxBurnBytes+int64(len(packs)) || !bytes.Equal(head, sub) || !bytes.Equal(tail, packs) {
			t.Errorf("a .sub file of %d bytes, want the %d of the file and the sentinel's packs", size, maxBurnBytes)
		}
		if gave := share.gave("Movie.idx") + share.gave("Movie.sub"); gave != maxIndexBytes+maxBurnBytes {
			t.Errorf("the share gave %d bytes, want the files whole", gave)
		}
	})
	t.Run("a PGS file", func(t *testing.T) {
		t.Parallel()
		share := newSubtitleShare(t, map[string]func(io.Writer) error{"Movie.sup": pgsOfSize(maxBurnBytes)})
		dir := t.TempDir()
		copied, _, err := writeBurnCopy(t.Context(), share.URL+"/Movie.sup", filepath.Join(dir, "burn"), at)
		if err != nil {
			t.Fatal(err)
		}
		// The sentinel of the rectangle alone: a presentation and its end.
		rectangle := sentinelRectangle.PGS()
		small := bytes.Buffer{}
		if err := copyPGS(&small, bytes.NewReader(rectangle), at); err != nil {
			t.Fatal(err)
		}
		sentinel := small.Bytes()[len(rectangle):]
		size, head, tail := ends(t, copied, len(rectangle), len(sentinel))
		if copied != filepath.Join(dir, "burn.sup") || size != maxBurnBytes+int64(len(sentinel)) || !bytes.Equal(head, rectangle) ||
			!bytes.Equal(tail, sentinel) {
			t.Errorf("copied to %s, %d bytes, want the %d of the file and the sentinel", copied, size, maxBurnBytes)
		}
		if gave := share.gave("Movie.sup"); gave != maxBurnBytes {
			t.Errorf("the share gave %d bytes, want the file whole", gave)
		}
	})
}

// A copy the share cuts short leaves nothing in the folder, as before the
// copies had a bound, but the .idx file as read of a whole .idx file.
func TestBurnCopyCutShortLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	idx, sub := sentinelRectangle.VobSub()
	files := map[string][]byte{"Cut.sup": sentinelRectangle.PGS(), "Cut.idx": []byte(idx), "Cut.sub": sub, "Whole.idx": []byte(idx), "Whole.sub": sub}
	share := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		data := files[name]
		if name == "Whole.idx" {
			_, _ = w.Write(data)
			return
		}
		// The length of the whole file, then half of it.
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data[:len(data)/2])
	}))
	defer share.Close()
	for _, original := range []string{"Cut.sup", "Cut.idx", "Whole.idx"} {
		dir := t.TempDir()
		copied, _, err := writeBurnCopy(t.Context(), share.URL+"/"+original, filepath.Join(dir, "burn"), time.Hour)
		if err == nil {
			t.Errorf("%s: copied to %s, want an error", original, copied)
		}
		if original == "Whole.idx" {
			if got, _ := os.ReadFile(copied); copied != filepath.Join(dir, "burn.idx") || string(got) != idx {
				t.Errorf("%s: copied to %s, want the .idx file as read", original, copied)
			}
			if err := os.Remove(copied); err != nil {
				t.Fatal(err)
			}
		}
		if entries, _ := os.ReadDir(dir); len(entries) > 0 {
			t.Errorf("%s: left %v behind", original, entries)
		}
	}
}

// readCount counts the bytes read from a reader.
type readCount struct {
	r    io.Reader
	read int64
}

func (c *readCount) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += int64(n)
	return n, err
}

// A bounded reader gives a file of its bound whole, and fails on a larger
// one after one more byte of it.
func TestBoundedReadsOneBytePastItsBound(t *testing.T) {
	t.Parallel()
	const bound = 100_000
	over := errors.New("over")
	for _, size := range []int{0, 1, bound - 1, bound, bound + 1, bound + 70_000} {
		source := &readCount{r: bytes.NewReader(make([]byte, size))}
		data, err := io.ReadAll(&bounded{r: source, left: bound, over: over})
		switch {
		case size <= bound && (err != nil || len(data) != size):
			t.Errorf("%d bytes: %d read, %v, want them all", size, len(data), err)
		case size > bound && (!errors.Is(err, over) || len(data) > bound || source.read != bound+1):
			t.Errorf("%d bytes: %d given, %d read from the source, %v, want the bound, one more and the error", size, len(data), source.read, err)
		}
	}
}

// The sentinel's lines follow each track's last picture where the tracks'
// lines alternate and the file's last line has no end.
func TestVobSubSentinelFollowsLinesAsTheyCome(t *testing.T) {
	t.Parallel()
	idx := "id: en, index: 0\ntimestamp: 00:00:01:000, filepos: 000000000\n" +
		"id: fr, index: 1\ntimestamp: 00:00:02:000, filepos: 000000800\n" +
		"id: en, index: 0\ntimestamp: 00:00:03:000, filepos: 000001000\n" +
		"id: fr, index: 1\r\ntimestamp: 00:00:04:000, filepos: 000001800"
	marked, packs, err := vobSubSentinel(idx, 0x2000, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(idx, "filepos: 000001000\n", "filepos: 000001000\n"+
		"timestamp: 00:00:10:000, filepos: 000002000\ntimestamp: 00:00:11:000, filepos: 000002800\n", 1) +
		"\ntimestamp: 00:00:10:000, filepos: 000003000\ntimestamp: 00:00:11:000, filepos: 000003800\n"
	if marked != want || len(packs) != 4*2048 {
		t.Errorf("index\n%q\nwant\n%q\nand %d bytes of packs, want 4 of 2048", marked, want, len(packs))
	}
}

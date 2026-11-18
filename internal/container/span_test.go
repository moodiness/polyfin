package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"
)

// recorded is a file held in memory that records the reads made of it.
// It ends at its data, whatever size its reader was told, and fails the
// reads past failAt with failure when one is set.
type recorded struct {
	data    []byte
	failAt  int64
	failure error
	reads   []Range
}

func (r *recorded) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	r.reads = append(r.reads, Range{Off: off, N: len(p)})
	if off < 0 || off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	available := r.data[off:]
	if r.failure != nil {
		available = r.data[off:max(off, min(r.failAt, int64(len(r.data))))]
	}
	n := copy(p, available)
	switch {
	case n == len(p):
		return n, nil
	case r.failure != nil:
		return n, r.failure
	default:
		return n, io.EOF
	}
}

// buffers is the size of the buffer each of reads was made in, when they
// are the steps of one span starting at start: a step is read at the end
// of what the buffer held, to the end of the buffer.
func buffers(reads []Range, start int64) []int64 {
	sizes := make([]int64, len(reads))
	for i, read := range reads {
		sizes[i] = read.Off - start + int64(read.N)
	}
	return sizes
}

// patterned returns n bytes that differ with their position.
func patterned(n int64) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i ^ i>>8 ^ i>>16)
	}
	return data
}

// stalling serves the head of a file and the sent bytes after it, as
// zeros, then holds the first read needing more until released, telling
// it on started: a host that stops sending.
type stalling struct {
	head     []byte
	sent     int64
	started  chan Range
	released chan struct{}
}

var errStalled = errors.New("the host sent nothing more")

func (f *stalling) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	if off+int64(len(p)) > int64(len(f.head))+f.sent {
		f.started <- Range{Off: off, N: len(p)}
		<-f.released
		return 0, errStalled
	}
	clear(p)
	if off < int64(len(f.head)) {
		copy(p, f.head[off:])
	}
	return len(p), nil
}

// A file's head declares attached files of the most that is read, and its
// host stops after sending some of them, or none: what is held for it
// when it stops is a step, or twice what the host sent, not what the head
// declares.
func TestSpansAreReadInStepsThatFollowWhatArrived(t *testing.T) {
	t.Parallel()
	head := slices.Concat(
		ebmlElement(idEBML, ebmlElement(idDocType, []byte("matroska"))),
		idBytes(idSegment), []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		ebmlElement(idTracks, trackEntry(1, trackTypeVideo)),
		idBytes(idAttachments), sizeBytes(maxAttachments),
	)
	size := int64(len(head)) + maxAttachments
	for _, test := range []struct {
		name string
		// sent is how much the host sends after the head; held is the
		// buffer the read waits in.
		sent, held int64
	}{
		// The head comes with the window the file is opened with.
		{"nothing after the head", window, spanStep},
		{"less than a step", spanStep - 1, spanStep},
		{"a step", spanStep, 2 * spanStep},
		{"three steps", 3 * spanStep, 4 * spanStep},
		{"a little more than eight steps", 8*spanStep + 5, 16 * spanStep},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			file := &stalling{head: head, sent: test.sent, started: make(chan Range), released: make(chan struct{})}
			ctx := context.Background()
			m, err := OpenMatroska(ctx, file, size)
			if err != nil {
				t.Fatalf("OpenMatroska: %v", err)
			}
			done := make(chan error, 1)
			go func() {
				_, err := m.Attachments(ctx)
				done <- err
			}()
			select {
			case waiting := <-file.started:
				held := buffers([]Range{waiting}, int64(len(head)))[0]
				if held != test.held || held > max(spanStep, 2*test.sent) {
					t.Errorf("the host sent %d bytes of %d: the read waits in a buffer of %d, want %d",
						test.sent, maxAttachments, held, test.held)
				}
			case err := <-done:
				t.Fatalf("the attached files were read without waiting for the host: %v", err)
			}
			close(file.released)
			if err := <-done; !errors.Is(err, errStalled) || errors.Is(err, ErrUnreadable) {
				t.Errorf("got %v, want the host's error", err)
			}
		})
	}
}

// A span read in steps holds the bytes one read gave: each byte is asked
// for once, a span of a step or less takes one read as before, and a
// longer one allocates less than three times its length in all, copying
// less than twice its length as its buffer doubles.
func TestSpansReadInStepsHoldTheSameBytes(t *testing.T) {
	t.Parallel()
	const off = 100
	for _, length := range []int64{1, window, spanStep - 1, spanStep, spanStep + 1, 2 * spanStep, 3*spanStep + 7, 8 * spanStep, 9*spanStep - 1} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			t.Parallel()
			// A window follows the span, so that a short one is not read
			// as the file's last window.
			r := &recorded{data: patterned(off + length + window)}
			f := &file{ctx: context.Background(), r: r, size: int64(len(r.data)), window: window}
			got, err := f.span(off, length)
			if err != nil || !bytes.Equal(got, r.data[off:off+length]) {
				t.Fatalf("got %d bytes, the same as the file's: %v; error %v", len(got), bytes.Equal(got, r.data[off:off+length]), err)
			}
			read := max(length, window)
			var asked, allocated, copied int64
			for i, size := range buffers(r.reads, off) {
				at := r.reads[i].Off - off
				if at != asked {
					t.Fatalf("read %d is at %d of the span, after %d bytes were asked for: %v", i, at, asked, r.reads)
				}
				if size > max(spanStep, 2*at) {
					t.Errorf("read %d is made in a buffer of %d bytes, after %d arrived", i, size, at)
				}
				asked += int64(r.reads[i].N)
				allocated += size
				copied += at
			}
			if asked != read {
				t.Errorf("%d bytes asked for, want %d", asked, read)
			}
			if read <= spanStep && len(r.reads) != 1 {
				t.Errorf("%d reads for %d bytes, want 1", len(r.reads), read)
			}
			if allocated >= 3*read || copied >= 2*read {
				t.Errorf("%d bytes allocated and %d copied for a span of %d", allocated, copied, read)
			}
			t.Logf("a span of %d bytes: %d reads, %d bytes allocated, %d copied", read, len(r.reads), allocated, copied)
			// The span is held as one read's is.
			reads := len(r.reads)
			if again, err := f.span(off+length/2, length-length/2); err != nil || !bytes.Equal(again, r.data[off+length/2:off+length]) || len(r.reads) != reads {
				t.Errorf("the span held: error %v, %d more reads", err, len(r.reads)-reads)
			}
		})
	}
}

// A file that ends before the span its reader asks for, or whose host
// fails, fails the span as one read did, with the length of the whole
// span in the error, and nothing of it is held; no more was allocated for
// it than a step or twice what it had.
func TestSpansOfFilesThatEndEarlyFailAsBefore(t *testing.T) {
	t.Parallel()
	const off, length = 100, 40 * spanStep
	refused := errors.New("HTTP 502")
	for _, test := range []struct {
		name string
		// had is how much of the span the file has, and failure what its
		// host answers past it, when it does not end there.
		had     int64
		failure error
		want    error
	}{
		{"ends in the first step", spanStep / 2, nil, io.ErrUnexpectedEOF},
		{"ends at a step", 2 * spanStep, nil, io.ErrUnexpectedEOF},
		{"ends in a later step", 5*spanStep + 9, nil, io.ErrUnexpectedEOF},
		{"fails in the first step", spanStep / 2, refused, refused},
		{"fails in a later step", 5*spanStep + 9, refused, refused},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			r := &recorded{data: patterned(off + test.had), failAt: off + test.had, failure: test.failure}
			if test.failure != nil {
				// The file goes on; its host fails.
				r.data = append(r.data, make([]byte, spanStep)...)
			}
			f := &file{ctx: context.Background(), r: r, size: off + length + window, window: window}
			_, err := f.span(off, length)
			if want := fmt.Sprintf("reading %d bytes at %d: %v", int64(length), off, test.want); !errors.Is(err, test.want) || errors.Is(err, ErrUnreadable) || err.Error() != want {
				t.Errorf("got %v, want %s", err, want)
			}
			if f.spans[0].data != nil || f.spans[1].data != nil {
				t.Errorf("a span is held after the failure")
			}
			sizes := buffers(r.reads, off)
			if last := sizes[len(sizes)-1]; last > max(spanStep, 2*test.had) {
				t.Errorf("the file had %d bytes of the span: its last read was made in a buffer of %d: %v", test.had, last, r.reads)
			}
		})
	}
}

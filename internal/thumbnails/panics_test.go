package thumbnails

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"math/bits"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// panicLines returns the lines telling that making images ended on a
// panic.
func panicLines(logged *logLines) []string {
	return slices.DeleteFunc(logged.all(), func(line string) bool {
		return !strings.Contains(line, "ended on a panic")
	})
}

// A panic while the images of a version are made after its play fails
// that version as a read that failed does, logged once with its title
// and its stack; the queue goes on with the next version.
func TestAPanicWhileMakingImagesFailsItsVersionAlone(t *testing.T) {
	t.Parallel()
	ts := newTestService(t, "")
	logged := &logLines{}
	ts.Logger = slog.New(slog.NewTextHandler(logged, nil))
	ts.mu.Lock()
	ts.analysis = forcedAnalysis
	ts.mu.Unlock()
	// made waits until the queue is done with a version.
	made := func(done <-chan struct{}) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("the version still waits")
		}
	}

	ts.source = &countingSource{data: fixture(t, "forced.mkv"), odd: func(int) error { panic("a reader's fault") }}
	made(ts.queueNow(testVersion, "Night Train (2021)"))
	if result := resultOf(t, ts, testVersion.ID); result.State != StateFailed || result.Reason != ReasonError {
		t.Errorf("the version whose images panicked: %+v", result)
	}
	if err, failed := ts.failed.Get(testVersion.ID); !failed || !strings.Contains(err.Error(), "a reader's fault") {
		t.Errorf("the version is not left alone: %v", err)
	}
	lines := panicLines(logged)
	if len(lines) != 1 || !strings.Contains(lines[0], "level=ERROR") || !strings.Contains(lines[0], `title="Night Train (2021)"`) ||
		!strings.Contains(lines[0], "a reader's fault") || !strings.Contains(lines[0], "stack=") {
		t.Errorf("logged %q", lines)
	}

	// The queue makes the images of the next version: a file that is no
	// video gets its result.
	next := testVersion
	next.ID, next.Item = accounts.ID{31}, accounts.ID{32}
	ts.source = &countingSource{data: []byte("not a video file at all")}
	made(ts.queueNow(next, "Next"))
	if result := resultOf(t, ts, next.ID); result.State != StateCannot || result.Reason != ReasonNoIndex {
		t.Errorf("the version after it: %+v", result)
	}
	if status := ts.Status(); status.Working || status.Waiting != 0 {
		t.Errorf("the queue after both: %+v", status)
	}
}

// A panic while the thumbnails of a version playing are extended leaves
// that version alone for a while, as a read that failed does, logged
// once; the pass is over, and the next passes do not try it again.
func TestAPanicWhileExtendingThumbnailsLeavesItsVersionAlone(t *testing.T) {
	t.Parallel()
	ts := newTestService(t, "")
	logged := &logLines{}
	ts.Logger = slog.New(slog.NewTextHandler(logged, nil))
	ts.mu.Lock()
	ts.analysis = forcedAnalysis
	ts.mu.Unlock()
	asked := 0
	ts.Cached = func(library.Version) (Cached, bool) {
		asked++
		panic("a reader's fault")
	}
	j := job{version: testVersion, title: "Night Train (2021)", host: hostOf(testVersion.URL)}
	for range 3 {
		ts.extendPlaying(t.Context(), j)
	}
	if err, failed := ts.unextended.Get(testVersion.ID); !failed || !strings.Contains(err.Error(), "a reader's fault") {
		t.Errorf("the version is not left alone: %v", err)
	}
	if asked != 1 {
		t.Errorf("the cache was asked %d times, want once", asked)
	}
	ts.Service.mu.Lock()
	extending := ts.extending
	ts.Service.mu.Unlock()
	if extending != (accounts.ID{}) {
		t.Error("the version is still marked as extended now")
	}
	lines := panicLines(logged)
	if len(lines) != 1 || !strings.Contains(lines[0], "level=ERROR") || !strings.Contains(lines[0], `title="Night Train (2021)"`) ||
		!strings.Contains(lines[0], "stack=") {
		t.Errorf("logged %q", lines)
	}
}

// ebml encodes an EBML element with an 8-byte size.
func ebml(id uint32, children ...[]byte) []byte {
	payload := slices.Concat(children...)
	head := binary.BigEndian.AppendUint32(nil, id)[4-(bits.Len32(id)+7)/8:]
	return slices.Concat(head, binary.BigEndian.AppendUint64(nil, 1<<56|uint64(len(payload))), payload)
}

func ebmlUint(id uint32, value uint64) []byte {
	return ebml(id, binary.BigEndian.AppendUint64(nil, value))
}

// keyframePastTheFile is a Matroska file of one H.264 track whose Cues,
// before its only Cluster, place a keyframe 1 MiB into that Cluster of 64
// bytes.
func keyframePastTheFile() []byte {
	const (
		header, docType, segment, tracks, trackEntry = 0x1A45DFA3, 0x4282, 0x18538067, 0x1654AE6B, 0xAE
		trackNumber, trackType, codecID              = 0xD7, 0x83, 0x86
		cues, cuePoint, cueTime, cuePositions        = 0x1C53BB6B, 0xBB, 0xB3, 0xB7
		cueTrack, cueCluster, cueRelative, cluster   = 0xF7, 0xF1, 0xF0, 0x1F43B675
	)
	described := ebml(tracks, ebml(trackEntry,
		ebmlUint(trackNumber, 1), ebmlUint(trackType, 1), ebml(codecID, []byte("V_MPEG4/ISO/AVC"))))
	index := func(at uint64) []byte {
		return ebml(cues, ebml(cuePoint, ebmlUint(cueTime, 0),
			ebml(cuePositions, ebmlUint(cueTrack, 1), ebmlUint(cueCluster, at), ebmlUint(cueRelative, 1<<20))))
	}
	at := uint64(len(described) + len(index(0)))
	return slices.Concat(
		ebml(header, ebml(docType, []byte("matroska"))),
		ebml(segment, described, index(at), ebml(cluster, make([]byte, 64))),
	)
}

// A file whose index places a keyframe past its end, played, gets no
// thumbnail from what the source cache holds of it: it is unreadable,
// and left alone as any file that cannot give images.
func TestAKeyframePlacedPastTheFileGivesNoThumbnailDuringThePlay(t *testing.T) {
	t.Parallel()
	data := keyframePastTheFile()
	origin := newHost(t, data)
	ts := newTestService(t, "")
	logged := &logLines{}
	ts.Logger = slog.New(slog.NewTextHandler(logged, nil))
	cache, err := source.New(t.TempDir(), func() source.Policy { return source.Policy{Limit: 1 << 30} }, httpOpener{origin.client}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	ts.cache = cache
	ts.mu.Lock()
	ts.analysis = media.Analysis{Format: "matroska,webm", Duration: time.Minute, Size: int64(len(data)),
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264", Width: 320, Height: 180}}}
	ts.mu.Unlock()
	version := library.Version{ID: accounts.ID{1}, Item: accounts.ID{2}, URL: origin.url, Addon: "Test"}
	ctx := t.Context()
	// The play reads the file: the source cache holds it.
	src := cache.Open(version.ID, source.Location{URL: version.URL}, nil)
	defer src.Release()
	if _, err := src.ReadAt(ctx, make([]byte, len(data)), 0); err != nil {
		t.Fatal(err)
	}
	if cached, ok := cache.Cached(version.ID); !ok {
		t.Fatal("the file played is not in the source cache")
	} else if size, known := cached.KnownSize(); !known || size != int64(len(data)) {
		t.Fatalf("size %d, known %t", size, known)
	}

	ts.extendPlaying(ctx, job{version: version, title: "Night Train (2021)", host: hostOf(version.URL)})
	if err, failed := ts.unextended.Get(version.ID); !failed || !errors.Is(err, container.ErrUnreadable) {
		t.Errorf("got %v, want the version left alone as unreadable", err)
	}
	if lines := panicLines(logged); len(lines) != 0 {
		t.Errorf("logged %q", lines)
	}
	if _, found, err := ts.trickplaySet(ctx, version.ID, 240); found || err != nil {
		t.Errorf("thumbnails kept: %t, %v", found, err)
	}
}

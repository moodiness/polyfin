package container

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// memory is a file held in memory, counting the reads made of it and the
// bytes they took, as each would be a round trip to a remote file.
type memory struct {
	data  []byte
	calls int
	read  int64
}

func (m *memory) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	m.calls++
	if off < 0 || off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n := copy(p, m.data[off:])
	m.read += int64(n)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// sparse is a large file of which only parts are written, the rest
// reading as zeros: the media the index readers must not need.
type sparse struct {
	parts []part
	size  int64
	calls int
	read  int64
}

type part struct {
	off  int64
	data []byte
}

func (s *sparse) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	s.calls++
	if off < 0 || off >= s.size {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), s.size-off))
	clear(p[:n])
	for _, part := range s.parts {
		end := part.off + int64(len(part.data))
		if part.off < off+int64(n) && end > off {
			from := max(part.off, off)
			copy(p[from-off:n], part.data[from-part.off:])
		}
	}
	s.read += int64(n)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// seconds parses times as ffprobe prints them.
func seconds(t *testing.T, values ...string) []time.Duration {
	t.Helper()
	times := make([]time.Duration, len(values))
	for i, value := range values {
		s, err := strconv.ParseFloat(value, 64)
		if err != nil {
			t.Fatal(err)
		}
		times[i] = time.Duration(s * float64(time.Second))
	}
	return times
}

// checkTimes compares times to those ffprobe printed, which it rounds to
// the microsecond.
func checkTimes(t *testing.T, got, want []time.Duration) {
	t.Helper()
	equal := len(got) == len(want)
	for i := 0; equal && i < len(got); i++ {
		equal = (got[i] - want[i]).Abs() <= time.Microsecond
	}
	if !equal {
		t.Errorf("got %v, want %v", got, want)
	}
}

// keyframesForced are the keyframes of the fixtures made with keyframes
// forced at 0, 1.3, 2.9, 3.1, 6, 7.7, 11 and 12.5 s, moved to the next
// frame at 24 fps, and every 48 frames since the last one.
var keyframesForced = []string{"0.000000", "1.333333", "2.916667", "3.125000", "5.125000", "6.000000", "7.708333", "9.708333", "11.000000", "12.500000", "14.500000"}

func TestKeyframes(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		// The audio track comes first, and Matroska times are rounded to
		// the millisecond. ffmpeg writes a CuePoint for every keyframe.
		{"forced.mkv", []string{"0.000000", "1.333000", "2.917000", "3.125000", "5.125000", "6.000000", "7.708000", "9.708000", "11.000000", "12.500000", "14.500000"}},
		// The Cues come before the Clusters.
		{"front.webm", []string{"0.000000", "2.500000", "4.500000", "7.000000", "9.500000", "12.000000", "14.500000"}},
		// B-frames give composition offsets, which an edit list shifts
		// back to zero.
		{"faststart.mp4", keyframesForced},
		{"tail.mp4", keyframesForced},
		{"negative.mp4", keyframesForced},
		// An empty edit delays the video by 1.5 s.
		{"delayed.mp4", []string{"1.500000", "1.541667", "2.916667", "3.125000", "5.125000", "6.000000", "7.708333", "9.708333", "11.000000", "12.500000", "14.500000"}},
		// The edit starts between frames, after the first keyframe, which
		// is kept with a negative time; the earlier ones are dropped.
		{"trimmed.mp4", []string{"-1.083333", "0.916667", "1.791667", "3.500000", "5.500000", "6.791667", "8.291667", "10.291667"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := fixture(t, test.name)
			want := seconds(t, test.want...)

			// A fixture is smaller than a window: the first read holds it.
			r := &memory{data: data}
			times, err := Keyframes(context.Background(), r, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			checkTimes(t, times, want)
			if r.calls != 1 {
				t.Errorf("%d reads, want 1", r.calls)
			}

			// With windows of 4 KiB, a fixture stands for a large file:
			// the index is still reached in a few reads.
			r = &memory{data: data}
			times, err = keyframes(context.Background(), r, int64(len(data)), 4<<10)
			if err != nil {
				t.Fatal(err)
			}
			checkTimes(t, times, want)
			t.Logf("%d reads of 4 KiB windows", r.calls)
			if r.calls > 5 {
				t.Errorf("%d reads of 4 KiB windows, want at most 5", r.calls)
			}
		})
	}
}

func TestKeyframesNoIndex(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		// Samples are described in fragments next to them.
		{"fragmented MP4", fixture(t, "fragmented.mp4")},
		// Written to a pipe, the file has no Cues and its SeekHead lists
		// none: the Clusters must not be walked to find keyframes.
		{"Matroska without Cues", fixture(t, "piped.mkv")},
		{"text", fixture(t, "README.md")},
		{"empty", nil},
		{"Matroska with an unknown-size element before the Clusters", matroskaFile(ebmlElement(idInfo), []byte{0xEC, 0xFF})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := &memory{data: test.data}
			_, err := Keyframes(context.Background(), r, int64(len(test.data)))
			if !errors.Is(err, ErrNoIndex) {
				t.Errorf("got %v, want ErrNoIndex", err)
			}
			if r.calls > 1 {
				t.Errorf("%d reads, want at most 1", r.calls)
			}
		})
	}
}

func TestKeyframesTruncated(t *testing.T) {
	// Both files keep their index after the media.
	for _, name := range []string{"forced.mkv", "tail.mp4"} {
		data := fixture(t, name)
		for _, size := range []int{len(data) / 8, len(data) / 2, len(data) - 100, len(data) - 1} {
			t.Run(name+"/"+strconv.Itoa(size), func(t *testing.T) {
				_, err := Keyframes(context.Background(), &memory{data: data[:size]}, int64(size))
				if err == nil || errors.Is(err, ErrNoIndex) {
					t.Errorf("got %v, want an error about the file", err)
				}
			})
		}
	}
}

// TestKeyframesCorrupted alters the fixtures, a byte at a time every few:
// Keyframes may fail, but must not panic, and what it returns must be
// ascending.
func TestKeyframesCorrupted(t *testing.T) {
	names := []string{"forced.mkv", "front.webm", "faststart.mp4", "tail.mp4", "delayed.mp4", "trimmed.mp4"}
	for _, name := range names {
		original := fixture(t, name)
		data := slices.Clone(original)
		for i := 0; i < len(data); i += 23 {
			for _, value := range []byte{0x00, 0xFF, original[i] ^ 0x80} {
				data[i] = value
				times, err := Keyframes(context.Background(), &memory{data: data}, int64(len(data)))
				if err == nil && !slices.IsSorted(times) {
					t.Fatalf("%s with byte %d set to %#x: times not ascending: %v", name, i, value, times)
				}
			}
			data[i] = original[i]
		}
	}
}

func TestKeyframesCuesNotInSeekHead(t *testing.T) {
	// The SeekHead's entry for the Cues becomes a Void element, as when a
	// muxer could not come back to write it: the Cues are found right
	// after the last Cluster.
	data := fixture(t, "forced.mkv")
	id := bytes.Index(data, []byte{0x53, 0xAB, 0x84, 0x1C, 0x53, 0xBB, 0x6B})
	at := id - 3
	if id < 3 || !bytes.Equal(data[at:at+2], []byte{0x4D, 0xBB}) {
		t.Fatal("no Seek for the Cues")
	}
	// The Seek's size fits in one byte; the Void's header is as long.
	size := int(data[at+2] & 0x7F)
	copy(data[at:], append([]byte{0xEC, 0x80 | byte(size+1)}, make([]byte, size+1)...))

	times, err := Keyframes(context.Background(), &memory{data: data}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	checkTimes(t, times, seconds(t, "0.000000", "1.333000", "2.917000", "3.125000", "5.125000", "6.000000", "7.708000", "9.708000", "11.000000", "12.500000", "14.500000"))
}

func TestKeyframesSignedCompositionOffsets(t *testing.T) {
	// The first ctts version is meant for unsigned offsets, yet ffmpeg
	// reads them as signed: the first keyframe's becomes -512 in one.
	data := fixture(t, "negative.mp4")
	at := bytes.Index(data, []byte("ctts"))
	if at < 0 {
		t.Fatal("no ctts")
	}
	data[at+4] = 0
	if first := binary.BigEndian.Uint32(data[at+12:]); first != 1 {
		t.Fatalf("first ctts entry counts %d samples, want 1", first)
	}
	binary.BigEndian.PutUint32(data[at+16:], uint32(0xFFFFFE00))

	times, err := Keyframes(context.Background(), &memory{data: data}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	// Printed by ffprobe for the altered file.
	checkTimes(t, times, seconds(t, "-0.083333", "1.291667", "2.875000", "3.083333", "5.083333", "5.958333", "7.666667", "9.666667", "10.958333", "12.458333", "14.458333"))
}

func TestKeyframesLargeFiles(t *testing.T) {
	const gap = 40 << 30
	tests := []struct {
		name string
		file *sparse
		want []time.Duration
	}{
		{"moov after 40 GiB of mdat", mp4AfterMedia(t, gap), seconds(t, keyframesForced...)},
		{"Cues listed by a SeekHead at the end", largeMatroska(gap, testCues, int64(len(testCues))), []time.Duration{0, 2 * time.Second}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			times, err := Keyframes(context.Background(), test.file, test.file.size)
			if err != nil {
				t.Fatal(err)
			}
			checkTimes(t, times, test.want)
			if test.file.calls > 2 || test.file.read > 2*window {
				t.Errorf("%d reads of %d bytes, want at most 2 of a window each", test.file.calls, test.file.read)
			}
		})
	}
}

func TestKeyframesHostile(t *testing.T) {
	everySample := mp4File(nil, fullBox("stts", 0, u32(1, 3, 30)))
	edited := mp4File(
		mp4Box("edts", fullBox("elst", 0, u32(2, 500, 0xFFFFFFFF, 1<<16, 1000, 30, 1<<16))),
		fullBox("stts", 0, u32(1, 3, 30)))
	allSync := mp4File(nil, fullBox("stts", 0, u32(1, 0xFFFFFFFF, 1)))
	huge := mp4File(
		mp4Box("edts", fullBox("elst", 0, u32(1, 1000, 3, 1<<16))),
		fullBox("stts", 0, u32(2, 0xFFFFFFFF, 1, 0xFFFFFFFF, 1)),
		fullBox("stss", 0, u32(1, 5)))
	// The Cues claim 1 GiB, more than is allocated for an index.
	oversized := largeMatroska(1<<20, slices.Concat(idBytes(idCues), sizeBytes(1<<30)), 12+(1<<30))

	tests := []struct {
		name string
		file Reader
		size int64
		want []time.Duration
		err  error
	}{
		// Without stss every sample is a sync sample.
		{"every sample", &memory{data: everySample}, int64(len(everySample)), []time.Duration{0, time.Second / 3, 2 * time.Second / 3}, nil},
		// An empty edit of 0.5 s comes before the media from 30 ticks;
		// the sample before is dropped.
		{"edit list", &memory{data: edited}, int64(len(edited)), []time.Duration{500 * time.Millisecond, 500*time.Millisecond + time.Second/3}, nil},
		{"billions of sync samples", &memory{data: allSync}, int64(len(allSync)), nil, errInvalid},
		// Billions of samples are walked by runs.
		{"billions of samples", &memory{data: huge}, int64(len(huge)), []time.Duration{time.Second / 90}, nil},
		{"Cues of 1 GiB", oversized, oversized.size, nil, errInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			times, err := Keyframes(context.Background(), test.file, test.size)
			if !errors.Is(err, test.err) {
				t.Fatalf("got %v, want %v", err, test.err)
			}
			if !slices.Equal(times, test.want) {
				t.Errorf("got %v, want %v", times, test.want)
			}
		})
	}
}

// mp4AfterMedia returns tail.mp4 with gap bytes more of media, and an mdat
// size of 64 bits.
func mp4AfterMedia(t *testing.T, gap int64) *sparse {
	data := fixture(t, "tail.mp4")
	mdat := bytes.Index(data, []byte("mdat")) - 4
	moov := bytes.Index(data, []byte("moov")) - 4
	if mdat < 0 || moov < mdat {
		t.Fatal("tail.mp4 is not ftyp, mdat then moov")
	}
	header := make([]byte, 16)
	binary.BigEndian.PutUint32(header, 1)
	copy(header[4:], "mdat")
	binary.BigEndian.PutUint64(header[8:], uint64(16+moov-mdat-8)+uint64(gap))
	head := slices.Concat(data[:mdat], header, data[mdat+8:moov])
	at := int64(len(head)) + gap
	return &sparse{
		parts: []part{{0, head}, {at, data[moov:]}},
		size:  at + int64(len(data)-moov),
	}
}

// testCues has CuePoints for an audio track 1 and video tracks 2 and 3,
// out of order and repeated: the first video track's are at 0 and 2 s.
var testCues = ebmlElement(idCues,
	cuePoint(0, 2),
	cuePoint(50_000, 1),
	cuePoint(20_000, 1, 2),
	cuePoint(20_000, 2),
	cuePoint(70_000, 3),
)

// largeMatroska returns a Matroska file whose Segment, of unknown size,
// has a SeekHead pointing to Info, Tracks and a second SeekHead after a
// Cluster of gap bytes, which points to Cues before it. cues is written
// at the start of a span of cuesLength bytes.
func largeMatroska(gap int64, cues []byte, cuesLength int64) *sparse {
	header := slices.Concat(
		ebmlElement(idEBML, ebmlElement(idDocType, []byte("matroska"))),
		[]byte{0x18, 0x53, 0x80, 0x67, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	)
	info := ebmlElement(idInfo, ebmlUint(idTimestampScale, 100_000))
	tracks := ebmlElement(idTracks, trackEntry(1, 2), trackEntry(2, 1), trackEntry(3, 1))
	cluster := slices.Concat(idBytes(idCluster), sizeBytes(gap))

	// Seek entries have a fixed length, whatever their position.
	first := func(second int64) []byte {
		return ebmlElement(idSeekHead,
			seek(idInfo, 0), seek(idTracks, 0), seek(idSeekHead, second))
	}
	infoAt := int64(len(first(0)))
	tracksAt := infoAt + int64(len(info))
	cuesAt := tracksAt + int64(len(tracks)) + int64(len(cluster)) + gap
	secondAt := cuesAt + cuesLength
	second := ebmlElement(idSeekHead, seek(idCues, cuesAt))
	head := slices.Concat(header, ebmlElement(idSeekHead,
		seek(idInfo, infoAt), seek(idTracks, tracksAt), seek(idSeekHead, secondAt)), info, tracks, cluster)
	segment := int64(len(header))
	return &sparse{
		parts: []part{{0, head}, {segment + cuesAt, cues}, {segment + secondAt, second}},
		size:  segment + secondAt + int64(len(second)),
	}
}

// matroskaFile returns a small Matroska file of the given top-level
// elements.
func matroskaFile(elements ...[]byte) []byte {
	return slices.Concat(
		ebmlElement(idEBML, ebmlElement(idDocType, []byte("matroska"))),
		ebmlElement(idSegment, elements...),
	)
}

func seek(id uint32, position int64) []byte {
	return ebmlElement(idSeek, ebmlElement(idSeekID, idBytes(id)), ebmlUint(idSeekPosition, uint64(position)))
}

func trackEntry(number, kind uint64) []byte {
	return ebmlElement(idTrackEntry, ebmlUint(idTrackNumber, number), ebmlUint(idTrackType, kind))
}

func cuePoint(time uint64, tracks ...uint64) []byte {
	children := [][]byte{ebmlUint(idCueTime, time)}
	for _, track := range tracks {
		children = append(children, ebmlElement(idCueTrackPositions, ebmlUint(idCueTrack, track)))
	}
	return ebmlElement(idCuePoint, children...)
}

// ebmlElement encodes an element with an 8-byte size, which keeps lengths
// fixed while positions are worked out.
func ebmlElement(id uint32, children ...[]byte) []byte {
	payload := slices.Concat(children...)
	return slices.Concat(idBytes(id), sizeBytes(int64(len(payload))), payload)
}

func ebmlUint(id uint32, value uint64) []byte {
	return ebmlElement(id, binary.BigEndian.AppendUint64(nil, value))
}

func idBytes(id uint32) []byte {
	return binary.BigEndian.AppendUint32(nil, id)[4-(bits.Len32(id)+7)/8:]
}

func sizeBytes(size int64) []byte {
	return binary.BigEndian.AppendUint64(nil, 1<<56|uint64(size))
}

// mp4File returns an MP4 file with a video track of timescale 90, in a
// movie of timescale 1000, of the given sample table boxes.
func mp4File(edts []byte, stbl ...[]byte) []byte {
	mvhd := fullBox("mvhd", 0, u32(0, 0, 1000, 0))
	mdhd := fullBox("mdhd", 0, u32(0, 0, 90, 0))
	hdlr := fullBox("hdlr", 0, u32(0), []byte("vide"), u32(0, 0, 0), []byte{0})
	trak := mp4Box("trak", edts, mp4Box("mdia", mdhd, hdlr, mp4Box("minf", mp4Box("stbl", stbl...))))
	return slices.Concat(mp4Box("ftyp", []byte("isom"), u32(0)), mp4Box("moov", mvhd, trak))
}

func mp4Box(typ string, children ...[]byte) []byte {
	payload := slices.Concat(children...)
	return slices.Concat(u32(uint32(8+len(payload))), []byte(typ), payload)
}

func fullBox(typ string, version byte, children ...[]byte) []byte {
	return mp4Box(typ, append([]byte{version, 0, 0, 0}, slices.Concat(children...)...))
}

func u32(values ...uint32) []byte {
	var b []byte
	for _, value := range values {
		b = binary.BigEndian.AppendUint32(b, value)
	}
	return b
}

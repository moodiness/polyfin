package container

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// declaredMP4 is the head of an MP4 file whose tables declare count
// samples of 1 byte in one chunk at offset, with one run of stts and one
// size in stsz: a few hundred bytes whatever the count. stss, when not
// nil, lists the sync samples; without it every sample is one.
func declaredMP4(count, offset uint32, stss []byte) []byte {
	entry := mp4Box("avc1", make([]byte, visualEntry), mp4Box("avcC", []byte{1, 100, 0, 31}))
	boxes := [][]byte{
		fullBox("stsd", 0, u32(1), entry),
		fullBox("stts", 0, u32(1, count, 1)),
		fullBox("stsz", 0, u32(1, count)),
		fullBox("stsc", 0, u32(1, 1, count, 1)),
		fullBox("stco", 0, u32(1, offset)),
	}
	if stss != nil {
		boxes = append(boxes, stss)
	}
	return mp4File(nil, boxes...)
}

// declaredFile is declaredMP4 with its samples after the head, in a file
// of which only the head is written: the index readers read nothing else.
func declaredFile(count uint32, stss []byte) (*sparse, int64) {
	head := declaredMP4(count, 0, stss)
	head = declaredMP4(count, uint32(len(head)), stss)
	return &sparse{parts: []part{{0, head}}, size: int64(len(head)) + int64(count)}, int64(len(head))
}

// An index is read up to maxKeyframes keyframes, and never more than its
// file has bytes: past either, it is an invalid index, and nothing is
// built from the count it declares.
func TestAnMP4IndexDeclaringMoreKeyframesThanAFileHasIsInvalid(t *testing.T) {
	t.Parallel()
	// One more than maxKeyframes.
	const declared = 1<<20 + 1
	everySample, _ := declaredFile(declared, nil)
	numbers := make([]uint32, 0, declared+1)
	numbers = append(numbers, declared)
	for number := range uint32(declared) {
		numbers = append(numbers, number+1)
	}
	listed, _ := declaredFile(declared, fullBox("stss", 0, u32(numbers...)))
	// 4096 samples, all sync, in a file that ends with its head.
	short := declaredMP4(4096, 0, nil)

	tests := []struct {
		name string
		file Reader
		size int64
	}{
		{"every sample of more than the bound", everySample, everySample.size},
		{"more sync samples listed than the bound", listed, listed.size},
		{"more samples than the file has bytes", &memory{data: short}, int64(len(short))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			if times, err := Keyframes(ctx, test.file, test.size); !errors.Is(err, errInvalid) || times != nil {
				t.Errorf("Keyframes: %d times and %v, want none and errInvalid", len(times), err)
			}
			if keyframes, err := KeyframeOffsets(ctx, test.file, test.size); !errors.Is(err, errInvalid) || keyframes != nil {
				t.Errorf("KeyframeOffsets: %d keyframes and %v, want none and errInvalid", len(keyframes), err)
			}
			if v, err := OpenVideo(ctx, test.file, test.size); !errors.Is(err, errInvalid) || v != nil {
				t.Errorf("OpenVideo: %v, want errInvalid", err)
			}
		})
	}
}

// The longest index read: every one of maxKeyframes samples a keyframe.
func TestAnMP4IndexOfAsManyKeyframesAsAFileMayHaveReadsWhole(t *testing.T) {
	t.Parallel()
	file, head := declaredFile(maxKeyframes, nil)
	ctx := context.Background()
	times, err := Keyframes(ctx, file, file.size)
	if err != nil {
		t.Fatal(err)
	}
	keyframes, err := KeyframeOffsets(ctx, file, file.size)
	if err != nil {
		t.Fatal(err)
	}
	if len(times) != maxKeyframes || len(keyframes) != maxKeyframes {
		t.Fatalf("%d times and %d keyframes, want %d each", len(times), len(keyframes), maxKeyframes)
	}
	// A sample every tick of the track's timescale, 90 a second, each of
	// 1 byte.
	for _, i := range []int{0, 1, 90, maxKeyframes - 1} {
		at, err := duration(int64(i), 90)
		if err != nil {
			t.Fatal(err)
		}
		if times[i] != at || keyframes[i] != (Keyframe{Time: at, Offset: head + int64(i)}) {
			t.Errorf("keyframe %d: at %v, %+v; want %v at offset %d", i, times[i], keyframes[i], at, head+int64(i))
		}
	}
	if file.read > 2*window {
		t.Errorf("%d bytes read, want at most two windows", file.read)
	}
}

// The times and places of an MP4 file's keyframes are read into the
// slices a Video keeps: none is grown, nor copied from another.
func TestAnMP4FilesKeyframesStayInTheSlicesRead(t *testing.T) {
	t.Parallel()
	const count = 1000
	file, head := declaredFile(count, nil)
	v, err := OpenVideo(context.Background(), file, file.size)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Keyframes) != count || len(v.frames) != count {
		t.Fatalf("%d times and %d places, want %d each", len(v.Keyframes), len(v.frames), count)
	}
	if cap(v.Keyframes) != count || cap(v.frames) != count {
		t.Errorf("room for %d times and %d places, want %d each: the slices were grown", cap(v.Keyframes), cap(v.frames), count)
	}
	if last := v.frames[count-1]; last != (frameAt{off: head + count - 1, n: 1}) {
		t.Errorf("the last keyframe is at %+v", last)
	}
}

// Keyframes come in the order of their times, one per time, the first
// sample of those shown at once, each with its own place, when samples
// are not shown in the order they are stored in.
func TestAnMP4FilesKeyframesComeInTheOrderOfTheirTimes(t *testing.T) {
	t.Parallel()
	entry := mp4Box("avc1", make([]byte, visualEntry), mp4Box("avcC", []byte{1, 100, 0, 31}))
	// Four sync samples a second apart, of 10, 20, 30 and 40 bytes; the
	// first is shown two seconds late, when the third is.
	build := func(offset uint32) []byte {
		return mp4File(nil,
			fullBox("stsd", 0, u32(1), entry),
			fullBox("stts", 0, u32(1, 4, 90)),
			fullBox("ctts", 0, u32(2, 1, 180, 3, 0)),
			fullBox("stsz", 0, u32(0, 4, 10, 20, 30, 40)),
			fullBox("stsc", 0, u32(1, 1, 4, 1)),
			fullBox("stco", 0, u32(1, offset)))
	}
	head := int64(len(build(0)))
	data := append(build(uint32(head)), make([]byte, 100)...)
	want := []Keyframe{{time.Second, head + 10}, {2 * time.Second, head}, {3 * time.Second, head + 60}}

	ctx := context.Background()
	keyframes, err := KeyframeOffsets(ctx, &memory{data: data}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keyframes, want) {
		t.Errorf("got %+v, want %+v", keyframes, want)
	}
	v, err := OpenVideo(ctx, &memory{data: data}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(v.Keyframes, []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}) ||
		!slices.Equal(v.frames, []frameAt{{off: head + 10, n: 20}, {off: head, n: 10}, {off: head + 60, n: 40}}) {
		t.Errorf("got %v at %+v", v.Keyframes, v.frames)
	}
}

// With one size for every sample, stsz only declares how many there are:
// a sample's place is computed from its number, whatever the count, and
// no size is read to reach it. With a table of sizes, those before the
// sample in its chunk are read, which the table holds.
func TestAnMP4SamplesPlaceIsComputedNotWalkedTo(t *testing.T) {
	t.Parallel()
	const (
		every = 1<<32 - 1
		at    = 1000
		size  = int64(1) << 40
	)
	chunk := fullBox("stco", 0, u32(1, at))[8:]
	for _, declared := range []uint32{1 << 21, every} {
		stsz := fullBox("stsz", 0, u32(4, declared))[8:]
		stsc := fullBox("stsc", 0, u32(1, 1, declared, 1))[8:]
		frames, walked, err := sampleRanges([]uint64{0, 5, 1 << 20}, stsz, stsc, chunk, false, size)
		if err != nil {
			t.Fatalf("%d samples declared: %v", declared, err)
		}
		if want := []frameAt{{off: at, n: 4}, {off: at + 20, n: 4}, {off: at + 4<<20, n: 4}}; !slices.Equal(frames, want) || walked != 0 {
			t.Fatalf("%d samples declared: got %+v after %d sizes read, want %+v after none", declared, frames, walked, want)
		}
	}

	// The last of the 2^32-1 samples a file may declare.
	stsz := fullBox("stsz", 0, u32(4, every))[8:]
	stsc := fullBox("stsc", 0, u32(1, 1, every, 1))[8:]
	frames, walked, err := sampleRanges([]uint64{every - 1}, stsz, stsc, chunk, false, size)
	if err != nil {
		t.Fatal(err)
	}
	if want := []frameAt{{off: at + 4*(every-1), n: 4}}; !slices.Equal(frames, want) || walked != 0 {
		t.Fatalf("got %+v after %d sizes read, want %+v after none", frames, walked, want)
	}
	// Its place is checked against the file: it ends a file of this size,
	// and is past the end of one a byte shorter; so is every sample of a
	// chunk starting past the end.
	end := at + 4*int64(every)
	if frames, _, err := sampleRanges([]uint64{every - 1}, stsz, stsc, chunk, false, end); err != nil || len(frames) != 1 {
		t.Errorf("a file ending with the sample: %v", err)
	}
	for name, size := range map[string]int64{"a byte shorter": end - 1, "ending before the chunk": at} {
		if _, walked, err := sampleRanges([]uint64{every - 1}, stsz, stsc, chunk, false, size); !errors.Is(err, errInvalid) || walked != 0 {
			t.Errorf("a file %s: %v after %d sizes read, want errInvalid after none", name, err, walked)
		}
	}
	// A chunk offset of 64 bits that is no place in a file.
	if _, _, err := sampleRanges([]uint64{0}, stsz, stsc, fullBox("co64", 0, u32(1, 0x80000000, 0))[8:], true, size); !errors.Is(err, errInvalid) {
		t.Errorf("a chunk at a negative offset: %v, want errInvalid", err)
	}

	// Two chunks of four samples, of 1 to 8 bytes: the third sample of
	// the second chunk comes after two of its sizes.
	table := fullBox("stsz", 0, u32(0, 8, 1, 2, 3, 4, 5, 6, 7, 8))[8:]
	frames, walked, err = sampleRanges([]uint64{6}, table, fullBox("stsc", 0, u32(1, 1, 4, 1))[8:], fullBox("stco", 0, u32(2, 100, 200))[8:], false, size)
	if err != nil {
		t.Fatal(err)
	}
	if want := []frameAt{{off: 200 + 5 + 6, n: 7}}; !slices.Equal(frames, want) || walked != 3 {
		t.Errorf("got %+v after %d sizes read, want %+v after 3", frames, walked, want)
	}

	// Through the index readers: the first and the last of 2^32-1 samples
	// are sync samples, in a file large enough to hold them all.
	file, head := declaredFile(every, fullBox("stss", 0, u32(2, 1, every)))
	keyframes, err := KeyframeOffsets(context.Background(), file, file.size)
	if err != nil {
		t.Fatal(err)
	}
	if len(keyframes) != 2 || keyframes[0].Offset != head || keyframes[1].Offset != head+every-1 {
		t.Errorf("got %+v, want the samples at %d and %d", keyframes, head, head+every-1)
	}
}

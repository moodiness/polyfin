package container

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"testing"
)

// idrFrame reports whether an H.264 frame, its NAL units each preceded by
// a 4-byte length as Matroska and MP4 store them, is exactly whole and
// holds an IDR slice.
func idrFrame(frame []byte) bool {
	idr := false
	for len(frame) > 0 {
		if len(frame) < 5 {
			return false
		}
		n := binary.BigEndian.Uint32(frame)
		if uint64(n) > uint64(len(frame)-4) || n == 0 {
			return false
		}
		idr = idr || frame[4]&0x1F == 5
		frame = frame[4+n:]
	}
	return idr
}

// vp9Keyframe reports whether a VP9 frame of profile 0 is a keyframe.
func vp9Keyframe(frame []byte) bool {
	return len(frame) > 4 && frame[0]>>6 == 2 && frame[0]&0x04 == 0 && frame[1] == 0x49 && frame[2] == 0x83 && frame[3] == 0x42
}

// Every keyframe the index lists is read alone and whole, the codec as a
// decoder needs it, and its times are those the HLS keyframe index reads.
func TestVideoFrames(t *testing.T) {
	for _, tc := range []struct {
		file, codec string
		key         func([]byte) bool
	}{
		{"forced.mkv", "V_MPEG4/ISO/AVC", idrFrame},
		{"subtitles.mkv", "V_MPEG4/ISO/AVC", idrFrame},
		{"front.webm", "V_VP9", vp9Keyframe},
		{"faststart.mp4", "V_MPEG4/ISO/AVC", idrFrame},
		{"tail.mp4", "V_MPEG4/ISO/AVC", idrFrame},
		{"delayed.mp4", "V_MPEG4/ISO/AVC", idrFrame},
		{"trimmed.mp4", "V_MPEG4/ISO/AVC", idrFrame},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data := fixture(t, tc.file)
			ctx := context.Background()
			v, err := OpenVideo(ctx, &memory{data: data}, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if v.CodecID != tc.codec {
				t.Errorf("codec %q, want %q", v.CodecID, tc.codec)
			}
			if tc.codec == "V_MPEG4/ISO/AVC" && (len(v.CodecPrivate) < 7 || v.CodecPrivate[0] != 1) {
				t.Errorf("no avcC: %x", v.CodecPrivate)
			}
			if len(v.Settings) == 0 {
				t.Error("no Video element")
			}
			want, err := Keyframes(ctx, &memory{data: data}, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			checkTimes(t, v.Keyframes, want)
			all := make([]int, len(v.Keyframes))
			for i := range all {
				all[i] = i
			}
			f := &fetcher{data: data}
			var read []int
			err = v.ReadFrames(ctx, f, all, func(index int, frame []byte) error {
				read = append(read, index)
				if !tc.key(frame) {
					t.Errorf("keyframe %d at %v is not one: %d bytes", index, v.Keyframes[index], len(frame))
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(read, all) {
				t.Errorf("read %v, want %v", read, all)
			}
			// A request each, and one more for the first Matroska frame
			// at most, which learns how long Cluster headers are.
			if f.calls > len(all)+1 {
				t.Errorf("%d requests for %d keyframes", f.calls, len(all))
			}
		})
	}
}

// A source serving several ranges at once is asked for several keyframes
// with each request; one that does not, a keyframe each.
func TestVideoFramesBatched(t *testing.T) {
	data := fixture(t, "forced.mkv")
	ctx := context.Background()
	v, err := OpenVideo(ctx, &memory{data: data}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	all := make([]int, len(v.Keyframes))
	for i := range all {
		all[i] = i
	}
	batched := &rangeFetcher{fetcher: fetcher{data: data}}
	frames := map[int][]byte{}
	if err := v.ReadFrames(ctx, batched, all, func(index int, frame []byte) error {
		frames[index] = frame
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if batched.batches != 1 || batched.calls > 1 {
		t.Errorf("%d keyframes: %d batches and %d single requests", len(all), batched.batches, batched.calls)
	}
	single := &rangeFetcher{fetcher: fetcher{data: data}, unsupported: true}
	if err := v.ReadFrames(ctx, single, all, func(index int, frame []byte) error {
		if !slices.Equal(frame, frames[index]) {
			t.Errorf("keyframe %d differs", index)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if single.calls > len(all)+1 {
		t.Errorf("%d requests for %d keyframes", single.calls, len(all))
	}
	// Some keyframes only, in order.
	var got []int
	if err := v.ReadFrames(ctx, &fetcher{data: data}, []int{1, 4, 6}, func(index int, frame []byte) error {
		got = append(got, index)
		if !slices.Equal(frame, frames[index]) {
			t.Errorf("keyframe %d differs", index)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int{1, 4, 6}) {
		t.Errorf("read %v", got)
	}
}

func TestVideoWithoutIndex(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"fragmented.mp4", "piped.mkv"} {
		data := fixture(t, name)
		if _, err := OpenVideo(ctx, &memory{data: data}, int64(len(data))); !errors.Is(err, ErrNoIndex) {
			t.Errorf("%s: %v", name, err)
		}
	}
	text := []byte("not a media file at all")
	if _, err := OpenVideo(ctx, &memory{data: text}, int64(len(text))); !errors.Is(err, ErrNoIndex) {
		t.Errorf("text: %v", err)
	}
}

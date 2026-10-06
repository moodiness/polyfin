package container

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// Each keyframe comes with where a player starting from it reads: the
// Cluster holding it in a Matroska file, the sample in an MP4 file. Its
// times are those the HLS keyframe index reads, whatever the codec.
func TestKeyframeOffsets(t *testing.T) {
	cluster := []byte{0x1F, 0x43, 0xB6, 0x75}
	for _, file := range []string{"forced.mkv", "subtitles.mkv", "hevc.mkv", "front.webm", "faststart.mp4", "tail.mp4", "trimmed.mp4"} {
		t.Run(file, func(t *testing.T) {
			data := fixture(t, file)
			ctx := context.Background()
			keyframes, err := KeyframeOffsets(ctx, &memory{data: data}, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			want, err := Keyframes(ctx, &memory{data: data}, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if len(keyframes) != len(want) {
				t.Fatalf("%d keyframes, want %d", len(keyframes), len(want))
			}
			for i, k := range keyframes {
				if k.Time != want[i] {
					t.Errorf("keyframe %d at %v, want %v", i, k.Time, want[i])
				}
				if i > 0 && k.Offset < keyframes[i-1].Offset {
					t.Errorf("keyframe %d at offset %d, before the previous one's %d", i, k.Offset, keyframes[i-1].Offset)
				}
				if k.Offset <= 0 || k.Offset >= int64(len(data)) {
					t.Fatalf("keyframe %d at offset %d of %d", i, k.Offset, len(data))
				}
				if data[0] == 0x1A && !bytes.HasPrefix(data[k.Offset:], cluster) {
					t.Errorf("keyframe %d does not point to a Cluster: %x", i, data[k.Offset:k.Offset+4])
				}
			}
			if data[0] != 0x1A {
				v, err := OpenVideo(ctx, &memory{data: data}, int64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				for i, k := range keyframes {
					if k.Offset != v.frames[i].off {
						t.Errorf("keyframe %d at %d, its sample at %d", i, k.Offset, v.frames[i].off)
					}
				}
			}
		})
	}
	if _, err := KeyframeOffsets(context.Background(), &memory{data: []byte("not a media file")}, 16); !errors.Is(err, ErrNoIndex) {
		t.Errorf("a file of no container: %v", err)
	}
}

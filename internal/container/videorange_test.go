package container

import (
	"context"
	"errors"
	"testing"
)

// cuedKeyframe is a Matroska file whose Cues, before its only Cluster,
// list one keyframe of video track 1 in that Cluster, relative bytes into
// its data. The Cluster, of payload zeros, ends the file: left is how
// many bytes the file holds from the Cluster's start.
func cuedKeyframe(payload int, relative uint64) (data []byte, left uint64) {
	tracks := ebmlElement(idTracks, ebmlElement(idTrackEntry,
		ebmlUint(idTrackNumber, 1), ebmlUint(idTrackType, trackTypeVideo), ebmlElement(idCodecID, []byte("V_MPEG4/ISO/AVC"))))
	cues := func(cluster uint64) []byte {
		return ebmlElement(idCues, ebmlElement(idCuePoint,
			ebmlUint(idCueTime, 0),
			ebmlElement(idCueTrackPositions,
				ebmlUint(idCueTrack, 1), ebmlUint(idCueClusterPosition, cluster), ebmlUint(idCueRelativePosition, relative))))
	}
	// Sizes and integers take 8 bytes: lengths do not depend on values.
	at := uint64(len(tracks) + len(cues(0)))
	cluster := ebmlElement(idCluster, make([]byte, payload))
	return matroskaFile(tracks, cues(at), cluster), uint64(len(cluster))
}

// clamped is a Fetcher that keeps the lengths it was asked, and answers
// what the file holds of a range, nothing for one outside it or of a
// negative length, without an error.
type clamped struct {
	data  []byte
	asked []int
}

func (f *clamped) Fetch(_ context.Context, off int64, n int) ([]byte, error) {
	f.asked = append(f.asked, n)
	size := int64(len(f.data))
	from := max(0, min(off, size))
	return f.data[from:max(from, min(off+int64(n), size))], nil
}

// panicOf returns what fn panicked with, nil when it returned.
func panicOf(fn func()) (cause any) {
	defer func() { cause = recover() }()
	fn()
	return nil
}

// A keyframe the Cues place at or past the end of the file, in its
// Cluster, makes the file unreadable when its index is opened, as one
// whose Cluster is past it does; the last byte of the file is a place.
func TestAKeyframePlacedPastTheFileIsUnreadable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, left := cuedKeyframe(64, 0)
	for _, test := range []struct {
		name     string
		relative uint64
		refused  bool
	}{
		{"the last byte", left - 1, false},
		{"the end of the file", left, true},
		{"far past it", 1 << 20, true},
		{"2^63, negative once signed", 1 << 63, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			data, _ := cuedKeyframe(64, test.relative)
			video, err := OpenVideo(ctx, &memory{data: data}, int64(len(data)))
			switch {
			case test.refused && !errors.Is(err, ErrUnreadable):
				t.Fatalf("got %v, want ErrUnreadable", err)
			case !test.refused && (err != nil || len(video.Keyframes) != 1):
				t.Fatalf("got %v", err)
			}
		})
	}
}

// A keyframe placed so close to the end of the file that nothing is left
// to read where its block would be is unreadable: no Fetcher is asked a
// length of zero or less for it, and none answering such a length with
// nothing makes the read fail another way.
func TestKeyframeReadsAskNoLengthBelowOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Past minFrameGuess into its Cluster, the keyframe is read from
	// where it is said to be: a Cluster header further than the file ends.
	_, left := cuedKeyframe(minFrameGuess+1024, 0)
	data, _ := cuedKeyframe(minFrameGuess+1024, left-1)
	video, err := OpenVideo(ctx, &memory{data: data}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	f := &clamped{data: data}
	cause := panicOf(func() {
		err = video.ReadFrames(ctx, f, []int{0}, func(int, []byte) error { return nil })
	})
	if cause != nil {
		t.Fatalf("reading the keyframe panicked: %v", cause)
	}
	if !errors.Is(err, ErrUnreadable) || len(f.asked) != 0 {
		t.Errorf("got %v after asking lengths %v, want ErrUnreadable and no request", err, f.asked)
	}

	// A read is never passed on with a negative length.
	f = &clamped{data: data}
	cause = panicOf(func() {
		_, err = video.Frames(f).fetch(ctx, 0, -1)
	})
	if cause != nil {
		t.Fatalf("fetching a negative length panicked: %v", cause)
	}
	if !errors.Is(err, ErrUnreadable) || len(f.asked) != 0 {
		t.Errorf("got %v after asking lengths %v, want ErrUnreadable and no request", err, f.asked)
	}
}

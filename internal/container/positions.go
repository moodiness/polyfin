package container

import (
	"bytes"
	"context"
	"errors"
	"time"
)

// Keyframe is a keyframe of a file's first video track: its time, and
// where the data a player starting from it reads first starts in the file,
// the Cluster holding it in a Matroska file, the sample itself in an MP4
// file.
type Keyframe struct {
	Time   time.Duration
	Offset int64
}

// KeyframeOffsets returns the keyframes of the first video track of a
// Matroska, WebM or MP4 file with where each starts, ascending by time and
// one per time, as its index lists them: the bytes a remux starting from
// one, or reaching the next, reads. ErrNoIndex when the file has none.
// Unlike OpenVideo, the codec and how the frames are stored do not matter.
// size is the file size.
func KeyframeOffsets(ctx context.Context, r Reader, size int64) ([]Keyframe, error) {
	f := &file{ctx: ctx, r: r, size: size, window: window}
	head, err := f.span(0, min(size, 8))
	if err != nil {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(head, ebmlMagic):
		return matroskaOffsets(f)
	case len(head) == 8 && topLevelBoxes[string(head[4:8])]:
		v, err := mp4VideoFrames(f)
		if errors.Is(err, ErrUnsupportedCodec) {
			return nil, ErrNoIndex
		}
		if err != nil {
			return nil, err
		}
		keyframes := make([]Keyframe, len(v.Keyframes))
		for i, at := range v.Keyframes {
			keyframes[i] = Keyframe{Time: at, Offset: v.frames[i].off}
		}
		return keyframes, nil
	}
	return nil, ErrNoIndex
}

// matroskaOffsets reads the CuePoints of a Matroska file's first video
// track, with the Clusters they point to.
func matroskaOffsets(f *file) ([]Keyframe, error) {
	m, err := openMatroska(f)
	if err != nil {
		return nil, err
	}
	number, err := m.videoTrack()
	if err != nil {
		return nil, err
	}
	cues, err := m.cuesData(f, number)
	if err != nil {
		return nil, err
	}
	points, err := videoCues(cues, number, m.scale)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, ErrNoIndex
	}
	keyframes := make([]Keyframe, len(points))
	for i, point := range points {
		off := m.segment.data + int64(point.cluster)
		if point.cluster >= uint64(f.size) || off >= f.size {
			return nil, m.outside(point.cluster, "Cluster of a keyframe")
		}
		keyframes[i] = Keyframe{Time: point.time, Offset: off}
	}
	return keyframes, nil
}

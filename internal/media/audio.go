package media

import (
	"context"
	"fmt"
	"slices"
)

// ProbeAudio analyzes an audio file at url, such as a music track, which
// ffprobe reads over HTTP: a file holding audio, with or without cover
// art, whereas Probe takes videos only. Audio needs much less read than
// video to find its one track.
func (p Prober) ProbeAudio(ctx context.Context, url string) (Analysis, error) {
	data, err := p.run(ctx, url, "-probesize", "2M", "-analyzeduration", "5M")
	if err != nil {
		return Analysis{}, err
	}
	return ParseAudio(data)
}

// ParseAudio reads ffprobe's JSON output of an audio file: one without an
// audio track is not one.
func ParseAudio(data []byte) (Analysis, error) {
	analysis, err := parse(data)
	if err != nil {
		return Analysis{}, err
	}
	if !slices.ContainsFunc(analysis.Streams, func(s Stream) bool { return s.Type == "audio" }) {
		return Analysis{}, fmt.Errorf("%w: no audio track", ErrNotMedia)
	}
	return analysis, nil
}

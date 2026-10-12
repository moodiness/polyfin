package playback

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// A version of a local folder may not be one file: a DVD is a folder of
// files FFmpeg reads with its dvdvideo demuxer. FFmpeg and ffprobe read
// such a version from its input (see media.Input), on the server's disk,
// rather than through the source cache. Nothing serves its bytes: it plays
// only over HLS, cut into segments on its keyframes when the disc's own
// tables tell them, as a Blu-ray disc's do, so that its video may be
// copied; cut every few seconds otherwise, as an MPEG-TS file is, its
// video converted (see hls.NewGridPlan).

// ErrNoFile reports a version that is not one file, whose bytes nothing
// serves: it plays only over HLS.
var ErrNoFile = errors.New("the version is not one file: it plays only over HLS")

// Inputs finds, by key, what FFmpeg reads the versions of local folders
// that are not one file from (see localfiles.Service).
type Inputs interface {
	// Input is what FFmpeg reads the version from.
	Input(ctx context.Context, key string) (media.Input, error)
	// Keyframes are the version's keyframe times, on the timeline FFmpeg
	// reads it on from zero, read from the disc's own tables; an error
	// when they cannot be.
	Keyframes(ctx context.Context, key string) ([]time.Duration, error)
	// Identity tells the file of a local folder a key names, that of a
	// version read from its input or of one read as one file (see
	// LocalFiles), as its folder's last scan found it: path, its folder
	// and its path there, and state, its size and modification time,
	// which change with what it holds.
	Identity(ctx context.Context, key string) (path, state string, err error)
}

// LocalInputs reads the versions of local folders that are not one file
// from the inputs inputs finds by key. It returns the address the keys are
// appended to: the URL of such a version, which nothing serves, and by
// which ReadsInput knows it.
func (s *Service) LocalInputs(inputs Inputs) string {
	s.loopback.mu.Lock()
	s.loopback.inputs = inputs
	s.loopback.mu.Unlock()
	return s.inputBase()
}

// inputBase is the address the keys of local inputs are appended to (see
// LocalInputs).
func (s *Service) inputBase() string {
	return "http://" + s.loopback.listener.Addr().String() + "/input/" + s.loopback.fileKey + "/"
}

// ReadsInput reports whether FFmpeg reads a version from an input of its
// own rather than as one file (see LocalInputs).
func (s *Service) ReadsInput(version library.Version) bool {
	key, ok := strings.CutPrefix(version.URL, s.inputBase())
	return ok && key != ""
}

// inputKey is the key of a version read from its input, and what finds
// its input.
func (s *Service) inputKey(version library.Version) (Inputs, string, error) {
	s.loopback.mu.Lock()
	inputs := s.loopback.inputs
	s.loopback.mu.Unlock()
	key, ok := strings.CutPrefix(version.URL, s.inputBase())
	if !ok || inputs == nil {
		return nil, "", fmt.Errorf("%w: no input", ErrNoFile)
	}
	return inputs, key, nil
}

// input finds what FFmpeg reads a version from, ReadsInput being true.
func (s *Service) input(ctx context.Context, version library.Version) (media.Input, error) {
	inputs, key, err := s.inputKey(version)
	if err != nil {
		return media.Input{}, err
	}
	return inputs.Input(ctx, key)
}

// inputKeyframes returns the keyframe times of a version read from its
// input: remembered, saved, or read from the disc's own tables, and then
// saved as a file's index is. A version whose keyframes cannot be read,
// such as a DVD, is not read again for a while.
func (s *Service) inputKeyframes(ctx context.Context, version library.Version) ([]time.Duration, error) {
	if times, ok := s.indexes.Get(version.ID); ok {
		return times, nil
	}
	if err, failed := s.unindexed.Get(version.ID); failed {
		return nil, err
	}
	result, err, _ := s.flight.Do("keyframes "+version.ID.String(), func() (any, error) {
		ctx := context.WithoutCancel(ctx)
		if times, ok := s.savedKeyframes(ctx, version.ID); ok {
			return times, nil
		}
		inputs, key, err := s.inputKey(version)
		var times []time.Duration
		if err == nil {
			times, err = inputs.Keyframes(ctx, key)
		}
		if err == nil && len(times) == 0 {
			err = errors.New("the disc lists no keyframe")
		}
		if err != nil {
			s.unindexed.Put(version.ID, err)
			return nil, err
		}
		s.saveKeyframes(ctx, version.ID, times)
		s.logger.Debug("Read a disc's keyframes", "addon", version.Addon, "keyframes", len(times))
		return times, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]time.Duration), nil
}

// analyzeInput is Analyze for a version read from its input: ffprobe reads
// the input itself. A version that cannot be analyzed is not tried again
// for a while. A disc on the server's disk stands in for nothing, whatever
// its length (see ErrStandIn).
func (s *Service) analyzeInput(ctx context.Context, version library.Version) (media.Analysis, error) {
	if err, failed := s.failures.Get(version.ID); failed {
		return media.Analysis{}, err
	}
	result, err, _ := s.flight.Do("analyze "+version.ID.String(), func() (any, error) {
		// Shared and kept as Analyze's are, even once the request that
		// started it is canceled.
		ctx := context.WithoutCancel(ctx)
		started := time.Now()
		in, err := s.input(ctx, version)
		var analysis media.Analysis
		if err == nil {
			analysis, err = s.ffprobe().ProbeInput(ctx, in)
		}
		if err != nil {
			s.failures.Put(version.ID, err)
			return nil, err
		}
		// A disc read as one file tells its tracks' languages and its
		// length through its own tables.
		for i, stream := range analysis.Streams {
			if code := in.Languages[stream.ID]; stream.Language == "" && code != "" {
				analysis.Streams[i].Language = addonLanguage(code)
			}
		}
		if in.Duration > 0 {
			analysis.Duration = in.Duration
		}
		// A DVD's navigation packets, which FFmpeg's dvdvideo demuxer reads
		// itself, are no track.
		analysis.Streams = slices.DeleteFunc(analysis.Streams, func(stream media.Stream) bool {
			return stream.Type == "data" && stream.Codec == "dvd_nav_packet"
		})
		// ffprobe tells neither the size nor the bitrate of a disc: those of
		// the files its version plays stand for them.
		if analysis.Size == 0 {
			analysis.Size = version.Size
		}
		if analysis.Bitrate == 0 && analysis.Size > 0 && analysis.Duration > 0 {
			analysis.Bitrate = int64(float64(analysis.Size*8) / analysis.Duration.Seconds())
		}
		return s.keepAnalysis(ctx, version, analysis, started), nil
	})
	if err != nil {
		return media.Analysis{}, err
	}
	return result.(media.Analysis), nil
}

// probeInput is what ffprobe reads a version from until release: its
// input, or its source, through the source cache on the loopback
// interface.
func (s *Service) probeInput(ctx context.Context, version library.Version) (in media.Input, release func(), err error) {
	if s.ReadsInput(version) {
		in, err := s.input(ctx, version)
		return in, func() {}, err
	}
	src := s.open(version)
	target, unregister := s.loopback.register(src)
	return media.Input{Path: target}, func() {
		unregister()
		src.Release()
	}, nil
}

// ProbesInput reports whether ffprobe reads a video from in, an input of
// FFmpeg's own, such as a disc image read with the bluray protocol.
func (s *Service) ProbesInput(ctx context.Context, in media.Input) bool {
	_, err := s.ffprobe().ProbeInput(ctx, in)
	return err == nil
}

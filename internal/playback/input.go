package playback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// A version of a local folder may not be one file: a DVD is a folder of
// files FFmpeg reads with its dvdvideo demuxer. FFmpeg and ffprobe read
// such a version from its input (see media.Input), on the server's disk,
// rather than through the source cache. Nothing serves its bytes: it plays
// only over HLS, cut every few seconds as an MPEG-TS file is, its video
// converted (see hls.NewGridPlan).

// ErrNoFile reports a version that is not one file, whose bytes nothing
// serves: it plays only over HLS.
var ErrNoFile = errors.New("the version is not one file: it plays only over HLS")

// LocalInputs reads the versions of local folders that are not one file
// from the inputs input finds by key (see localfiles.Service.Input). It
// returns the address the keys are appended to: the URL of such a version,
// which nothing serves, and by which ReadsInput knows it.
func (s *Service) LocalInputs(input func(ctx context.Context, key string) (media.Input, error)) string {
	s.loopback.mu.Lock()
	s.loopback.input = input
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

// input finds what FFmpeg reads a version from, ReadsInput being true.
func (s *Service) input(ctx context.Context, version library.Version) (media.Input, error) {
	s.loopback.mu.Lock()
	input := s.loopback.input
	s.loopback.mu.Unlock()
	key, ok := strings.CutPrefix(version.URL, s.inputBase())
	if !ok || input == nil {
		return media.Input{}, fmt.Errorf("%w: no input", ErrNoFile)
	}
	return input(ctx, key)
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

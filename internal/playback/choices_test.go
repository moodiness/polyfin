package playback

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// An analysis that outlasts the settings' timeout is given up, of a file
// as of a live stream, and the version is not tried again for a while.
func TestAnalysesStopAtTheSettingsTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ffprobe")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	// The channel answers with a live playlist: its start is checked
	// before ffprobe reads it. The file is read with ranges, as its
	// keyframe index is read during the analysis.
	s := newService(t, &fileSource{data: []byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nsegment.ts\n")}, path, nil)
	settings := s.settings()
	settings.AnalysisTimeout = accounts.MinAnalysisTimeout
	s.settings = func() accounts.Settings { return settings }
	file := library.Version{ID: accounts.ID{9}, URL: "https://93.184.216.34/slow.mkv"}
	channel := library.Version{ID: accounts.ID{10}, URL: "https://93.184.216.34/slow.m3u8"}
	analyses := []func() error{
		func() error { _, err := s.Analyze(t.Context(), file); return err },
		func() error { _, err := s.AnalyzeLive(t.Context(), channel); return err },
	}
	var errs [2]error
	var took [2]time.Duration
	var wg sync.WaitGroup
	for i, analyze := range analyses {
		wg.Go(func() {
			started := time.Now()
			errs[i] = analyze()
			took[i] = time.Since(started)
		})
	}
	wg.Wait()
	// The default, 45 s, would still be waiting.
	limit := time.Duration(accounts.MinAnalysisTimeout) * time.Second
	for i, err := range errs {
		if !errors.Is(err, context.DeadlineExceeded) || took[i] < limit || took[i] > 3*limit {
			t.Errorf("analysis %d: %v after %s, want the timeout after %s", i, err, took[i], limit)
		}
	}
	if !s.Failed(file.ID) || !s.Failed(channel.ID) {
		t.Error("a version given up is tried again at once")
	}
}

// Converted video is scaled down to fit the height cap's 16:9 frame,
// keeping its shape, at the bitrate of that height; under the cap, nothing
// changes.
func TestConvertedVideoIsScaledDownToTheHeightCap(t *testing.T) {
	can := Capabilities{Encoders: []string{"libx264"}, ToneMapping: true}
	video := func(width, height int) MediaStream {
		return MediaStream{Codec: "mpeg2video", VideoRange: "SDR", Width: new(width), Height: new(height)}
	}
	h264 := func(width, height int, bitrate int64) *VideoConversion {
		return &VideoConversion{Codec: "h264", Encoder: "libx264", Width: width, Height: height, Bitrate: bitrate}
	}
	for _, test := range []struct {
		name      string
		maxHeight int
		limit     int64
		video     MediaStream
		want      *VideoConversion
	}{
		{"no cap", 0, 0, video(1920, 1080), h264(1920, 1080, 10_000_000)},
		{"720p", 720, 0, video(1920, 1080), h264(1280, 720, 5_000_000)},
		{"480p", 480, 0, video(1920, 1080), h264(852, 480, 2_000_000)},
		{"a wide picture", 480, 0, video(1920, 800), h264(854, 354, 2_000_000)},
		{"an upright picture", 720, 0, video(1080, 1920), h264(404, 720, 5_000_000)},
		{"a source under the cap", 720, 0, video(640, 360), h264(640, 360, 10_000_000)},
		{"a source at the cap", 720, 0, video(1280, 720), h264(1280, 720, 10_000_000)},
		{"a cap above what Polyfin converts to", 2160, 0, video(3840, 2160), h264(1920, 1080, 10_000_000)},
		{"a limit lower than the cap's bitrate", 720, 2_000_000, video(1920, 1080), h264(960, 540, 2_000_000)},
	} {
		if got := ConvertVideo("h264", test.limit, test.maxHeight, test.video, can); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: %+v, want %+v", test.name, got, test.want)
		}
	}
	// HDR tone mapped on the processor stays at 720p at most.
	hdr := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", Width: new(3840), Height: new(2160)}
	for maxHeight, height := range map[int]int{1080: 720, 480: 480} {
		if got := ConvertVideo("h264", 0, maxHeight, hdr, can); got == nil || got.Height != height || !got.ToneMap {
			t.Errorf("HDR at most %dp: %+v", maxHeight, got)
		}
	}
	// The encoding FFmpeg runs takes the size of the conversion.
	stream := media.Stream{Index: 0, Type: "video", Codec: "mpeg2video", Width: 1920, Height: 800, AverageRate: 24}
	remux := Remux{ConvertVideo: ConvertVideo("h264", 0, 480, video(1920, 800), can)}
	var encoding hls.Remux
	remux.convert(&encoding, stream, Tuning{})
	if e := encoding.Encode; e == nil || e.Width != 854 || e.Height != 354 || e.Bitrate != 2_000_000 {
		t.Errorf("encoding: %+v", e)
	}
}

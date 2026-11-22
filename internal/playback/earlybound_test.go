package playback

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// A play started ahead of its Play has FFmpeg read the source within the
// bytes of its first segment: up to where the next segment starts, and a
// second of the version or 1 MiB more, in the cache's 1 MiB blocks. The
// host is asked nothing past it; past it, FFmpeg is served what the cache
// holds, as the index at the end of a Matroska file that reading its
// keyframes left there, and waits for the rest until the play taking the
// encoding over lifts the bound.
func TestAnEarlyPlayReadsTheSourceWithinItsFirstSegment(t *testing.T) {
	t.Parallel()
	const size = 16 << 20
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	// A keyframe every 10 seconds, 4 MiB apart: 0.4 MiB a second.
	offsets := []container.Keyframe{{Time: 0, Offset: 0}, {Time: 10 * time.Second, Offset: 4 << 20},
		{Time: 20 * time.Second, Offset: 8 << 20}, {Time: 30 * time.Second, Offset: 12 << 20}}
	times := make([]time.Duration, len(offsets))
	for i, k := range offsets {
		times[i] = k.Time
	}
	analysis := media.Analysis{Format: "matroska,webm", Duration: 40 * time.Second, Size: size,
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "aac"}}}
	for _, tc := range []struct {
		name string
		fast bool
		end  int64
	}{
		// The next segment starts on the keyframe at 4 MiB: 1 MiB more,
		// more than a second's 0.4 MiB.
		{"cut on its keyframes", false, 5 << 20},
		// The next segment, a fast one, starts at 2 s, 0.8 MiB in: 1 MiB
		// more, up to the end of the block.
		{"with fast segments", true, 2 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host := &rangedSource{fileSource: fileSource{data: data}}
			s := newService(t, host, "ffprobe-not-installed", nil)
			version := library.Version{ID: accounts.ID{1}, URL: "https://93.184.216.34/movie.mkv", Filename: "movie.mkv", Size: size}
			s.analyses.Put(version.ID, analysis)
			s.indexes.Put(version.ID, times)
			s.offsets.Put(version.ID, offsets)
			remux := Remux{Session: "session", Version: version, Audio: 1, Format: hls.FMP4, FastStart: tc.fast, early: true}
			if tc.fast {
				remux.ConvertVideo = &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1280, Height: 720, Bitrate: 4_000_000}
			}
			r, release, err := s.remuxOpener(remux)(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if r.Plan.HasFast() != tc.fast || r.Lift == nil {
				t.Fatalf("fast segments %v, a bound %v; want %v and true", r.Plan.HasFast(), r.Lift != nil, tc.fast)
			}
			// The index at the file's end, which reading the keyframes read.
			src := s.open(version)
			_, err = src.ReadAt(t.Context(), make([]byte, 100), size-100)
			src.Release()
			if err != nil {
				t.Fatal(err)
			}
			host.mu.Lock()
			read := len(host.ranges)
			host.mu.Unlock()

			if got, err := fetchRange(t.Context(), r.Input, 0, tc.end-1); err != nil || !bytes.Equal(got, data[:tc.end]) {
				t.Fatalf("the first segment's bytes: %d, %v", len(got), err)
			}
			if got, err := fetchRange(t.Context(), r.Input, size-100, size-1); err != nil || !bytes.Equal(got, data[size-100:]) {
				t.Fatalf("the index at the end: %d, %v", len(got), err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
			defer cancel()
			if got, err := fetchRange(ctx, r.Input, tc.end, tc.end); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("a byte past the bound: %v, %v, want it held", got, err)
			}
			host.mu.Lock()
			asked := host.ranges[read:]
			host.mu.Unlock()
			for _, value := range asked {
				_, last, _ := strings.Cut(strings.TrimPrefix(value, "bytes="), "-")
				if end, err := strconv.ParseInt(last, 10, 64); err != nil || end >= tc.end {
					t.Errorf("asked %q of the host, past the bound at %d", value, tc.end)
				}
			}

			r.Lift()
			if got, err := fetchRange(t.Context(), r.Input, tc.end, tc.end); err != nil || !bytes.Equal(got, data[tc.end:tc.end+1]) {
				t.Errorf("once lifted, the byte past the bound: %v, %v", got, err)
			}
		})
	}
}

// fetchRange asks target for its bytes from first to last, included.
func fetchRange(ctx context.Context, target string, first, last int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Range", "bytes="+strconv.FormatInt(first, 10)+"-"+strconv.FormatInt(last, 10))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}

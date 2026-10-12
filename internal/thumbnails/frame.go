package thumbnails

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Frame makes the image of the video at input, a path or an address FFmpeg
// reads, as a chapter's image is made: the keyframe at or before at, at
// most chapterWidth pixels wide, tone mapped to SDR when hdr is set, as a
// JPEG. The home videos of local folders show it when no image beside
// them does.
func Frame(ctx context.Context, ffmpeg, input string, at time.Duration, hdr bool) ([]byte, error) {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-noaccurate_seek",
		"-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64), "-i", input,
		"-map", "0:v:0", "-vf", chapterFilter(hdr), "-frames:v", "1", "-f", "yuv4mpegpipe", "pipe:1"}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = &limitedBuffer{b: stderr, limit: 4 << 10}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start FFmpeg: %w", err)
	}
	img, readErr := firstImage(stdout)
	// FFmpeg must not block on a pipe no one reads.
	_, _ = io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()
	switch {
	case readErr != nil:
		return nil, readErr
	case img == nil && waitErr != nil:
		return nil, fmt.Errorf("%w: %w: %s", errNoImage, waitErr, strings.TrimSpace(stderr.String()))
	case img == nil:
		return nil, errNoImage
	}
	return encodeJPEG(img)
}

package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ErrNoKeyframes reports a file whose first video track has no keyframe
// FFmpeg can start from, or whose timestamps go back, as a recording
// joined from several broadcasts may: its keyframes cut no segments.
var ErrNoKeyframes = errors.New("no usable keyframes")

// maxScannedKeyframes bounds the keyframes a scan returns, for a file
// that claims every one of a huge number of frames is one.
const maxScannedKeyframes = 1 << 23

// timestampsBack is how far back a keyframe's time may be from the one
// before it, as decoding order puts them, before its file's timestamps
// count as going back.
const timestampsBack = time.Second

// Keyframes reads a whole file, such as an MPEG-TS file, which has no
// keyframe index, and returns the times of its first video track's
// keyframes, ascending and without duplicates, as FFmpeg marks its packets
// (the K flag), from the start FFmpeg gives the file: what -start_at_zero
// shifts its timestamps by. Jellyfin reads keyframes the same way, in its
// scheduled keyframe extraction (FfProbeKeyframeExtractor.cs L28-L33 in
// src/Jellyfin.MediaEncoding.Keyframes/FfProbe at v12.4). ctx bounds the
// read, which Timeout does not: it takes as long as reading the file does.
func (p Prober) Keyframes(ctx context.Context, url string) ([]time.Duration, error) {
	command := exec.CommandContext(ctx, p.Path, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags:format=start_time", "-of", "csv=p=1", "-i", url)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	times, start, parsed := parseKeyframes(stdout)
	if parsed != nil {
		_ = command.Process.Kill()
	}
	waited := command.Wait()
	switch {
	case ctx.Err() != nil:
		return nil, fmt.Errorf("ffprobe: %w", ctx.Err())
	case parsed != nil:
		return nil, parsed
	case waited != nil:
		message := strings.TrimSpace(stderr.String())
		if i := strings.LastIndexByte(message, '\n'); i >= 0 {
			message = message[i+1:]
		}
		return nil, fmt.Errorf("%w: ffprobe: %v: %s", ErrUnreadable, waited, strings.ReplaceAll(message, url, "<source>"))
	case len(times) == 0:
		return nil, ErrNoKeyframes
	}
	for i := range times {
		times[i] -= start
	}
	slices.Sort(times)
	return slices.Compact(times), nil
}

// parseKeyframes reads ffprobe's CSV lines of packets and of the format's
// start: the times of the keyframe packets, in the order read, and the
// start, zero when FFmpeg tells none. Times are rounded to microseconds,
// as keyframe indexes are kept.
func parseKeyframes(r io.Reader) (times []time.Duration, start time.Duration, err error) {
	lines := bufio.NewScanner(r)
	var last time.Duration
	for lines.Scan() {
		fields := strings.Split(lines.Text(), ",")
		switch {
		case fields[0] == "format" && len(fields) >= 2:
			if at, ok := secondsOf(fields[1]); ok {
				start = at
			}
		case fields[0] == "packet" && len(fields) >= 3 && strings.HasPrefix(fields[2], "K"):
			at, ok := secondsOf(fields[1])
			if !ok {
				continue
			}
			if len(times) > 0 && at < last-timestampsBack {
				return nil, 0, fmt.Errorf("%w: a keyframe at %s follows one at %s", ErrNoKeyframes, at, last)
			}
			if len(times) == maxScannedKeyframes {
				return nil, 0, fmt.Errorf("%w: more than %d", ErrNoKeyframes, maxScannedKeyframes)
			}
			if len(times) == 0 || at > last {
				last = at
			}
			times = append(times, at)
		}
	}
	if err := lines.Err(); err != nil {
		return nil, 0, fmt.Errorf("ffprobe: %w", err)
	}
	return times, start, nil
}

// secondsOf parses a time ffprobe writes in seconds, rounded to the
// microsecond; false for N/A.
func secondsOf(value string) (time.Duration, bool) {
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(math.Round(seconds*1e6)) * time.Microsecond, true
}

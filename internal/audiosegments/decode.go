package audiosegments

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// duration asks ffprobe how long the file at address lasts.
func duration(ctx context.Context, ffprobe, address string) (time.Duration, error) {
	cmd := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", address)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || seconds <= 0 {
		return 0, errors.New("ffprobe tells no duration")
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// listen fingerprints the first audio track of the file at address from
// start, for length, as FFmpeg decodes it to mono samples at sampleRate.
// FFmpeg runs at the lowest priority, as it reads in the background.
func listen(ctx context.Context, ffmpeg, address string, start, length time.Duration) (Fingerprint, error) {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
	if start > 0 {
		args = append(args, "-ss", seconds(start))
	}
	args = append(args, "-i", address, "-map", "0:a:0", "-t", seconds(length),
		"-ac", "1", "-ar", strconv.Itoa(sampleRate), "-f", "s16le", "-")
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Fingerprint{}, err
	}
	if err := cmd.Start(); err != nil {
		return Fingerprint{}, err
	}
	lowerPriority(cmd.Process.Pid)
	f, readErr := fingerprint(out, start)
	if readErr != nil {
		_ = cmd.Process.Kill()
	}
	if err := cmd.Wait(); err != nil {
		return Fingerprint{}, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return f, readErr
}

// seconds writes a time as FFmpeg reads it.
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

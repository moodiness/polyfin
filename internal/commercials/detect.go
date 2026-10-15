package commercials

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

const (
	// A piece between two black and silent moments lasts shortestPiece to
	// longestPiece when it may be a commercial, and a break lasts
	// shortestBreak to longestBreak in all.
	shortestPiece = 5 * time.Second
	longestPiece  = 90 * time.Second
	shortestBreak = 30 * time.Second
	longestBreak  = 8 * time.Minute
	// joined is the gap below which two black and silent moments are one,
	// as a fade flickers.
	joined = time.Second
	// shortestBlack and shortestSilence are the shortest black and silent
	// stretches FFmpeg reports.
	shortestBlack   = "0.1"
	shortestSilence = "0.1"
	// silence is the level below which the audio is silent.
	silence = "-50dB"
	// readWidth is the width the video is scaled down to once decoded:
	// telling black frames needs no more.
	readWidth = 64
)

// span is a stretch of a recording, from its start.
type span struct {
	start, end time.Duration
}

// detect reads the recording at path, lasting length, with FFmpeg at the
// lowest priority, and returns the moments its first video track is black
// and its first audio track silent at once, by start.
func detect(ctx context.Context, ffmpeg, path string, length time.Duration) ([]span, error) {
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-hide_banner", "-nostats", "-loglevel", "info",
		"-i", path, "-map", "0:v:0", "-map", "0:a:0",
		"-vf", "scale="+strconv.Itoa(readWidth)+":-2:flags=neighbor,blackdetect=d="+shortestBlack,
		"-af", "silencedetect=n="+silence+":d="+shortestSilence,
		"-f", "null", "-")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	media.LowerPriority(cmd.Process.Pid)
	black, silent, last := parse(stderr, length)
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, last)
	}
	return overlaps(black, silent), nil
}

// parse reads what blackdetect and silencedetect report, the black and
// the silent stretches, from FFmpeg's log; a stretch still under way at
// the end ends at length. last is the last line of the log, which tells
// a failure.
func parse(log io.Reader, length time.Duration) (black, silent []span, last string) {
	// What is left past a line too long is read to the end, so that
	// FFmpeg never waits on its log.
	defer io.Copy(io.Discard, log)
	scanner := bufio.NewScanner(log)
	silenceStart := time.Duration(-1)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			last = strings.TrimSpace(line)
		}
		switch {
		case strings.Contains(line, "blackdetect"):
			start, okStart := value(line, "black_start:")
			end, okEnd := value(line, "black_end:")
			if okStart && okEnd && start < end {
				black = append(black, span{start, end})
			}
		case strings.Contains(line, "silencedetect"):
			if start, ok := value(line, "silence_start:"); ok {
				silenceStart = max(start, 0)
			} else if end, ok := value(line, "silence_end:"); ok && silenceStart >= 0 {
				if silenceStart < end {
					silent = append(silent, span{silenceStart, end})
				}
				silenceStart = -1
			}
		}
	}
	if silenceStart >= 0 && silenceStart < length {
		silent = append(silent, span{silenceStart, length})
	}
	return black, silent, last
}

// value reads the time in seconds that follows key in line.
func value(line, key string) (time.Duration, bool) {
	_, after, found := strings.Cut(line, key)
	if !found {
		return 0, false
	}
	fields := strings.Fields(after)
	if len(fields) == 0 {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

// overlaps returns the stretches both black and silent, by start.
func overlaps(black, silent []span) []span {
	byStart := func(a, b span) int { return cmp.Compare(a.start, b.start) }
	slices.SortFunc(black, byStart)
	slices.SortFunc(silent, byStart)
	var both []span
	for i, j := 0, 0; i < len(black) && j < len(silent); {
		start, end := max(black[i].start, silent[j].start), min(black[i].end, silent[j].end)
		if start < end {
			both = append(both, span{start, end})
		}
		if black[i].end < silent[j].end {
			i++
		} else {
			j++
		}
	}
	return both
}

// breaks finds the commercial breaks among the black and silent moments
// of a recording, by start. The recording's start counts as one such
// moment, and moments less than joined apart as one. A break is a run of
// pieces between moments, each lasting shortestPiece to longestPiece, of
// shortestBreak to longestBreak in all, from the moment before it to the
// moment after it, those included, so that skipping it lands on the
// programme.
func breaks(moments []span) []span {
	bounds := []span{{0, 0}}
	for _, m := range moments {
		if last := &bounds[len(bounds)-1]; m.start-last.end < joined {
			last.end = max(last.end, m.end)
			continue
		}
		bounds = append(bounds, m)
	}
	var found []span
	// run is the index of the moment the run of pieces starts after, -1
	// outside a run.
	run := -1
	closeRun := func(last int) {
		if run < 0 {
			return
		}
		if total := bounds[last].start - bounds[run].end; total >= shortestBreak && total <= longestBreak {
			found = append(found, span{bounds[run].start, bounds[last].end})
		}
		run = -1
	}
	for i := 1; i < len(bounds); i++ {
		if piece := bounds[i].start - bounds[i-1].end; piece >= shortestPiece && piece <= longestPiece {
			if run < 0 {
				run = i - 1
			}
			continue
		}
		closeRun(i - 1)
	}
	closeRun(len(bounds) - 1)
	return found
}

// duration asks ffprobe how long the file at path lasts.
func duration(ctx context.Context, ffprobe, path string) (time.Duration, error) {
	cmd := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
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

// exists reports whether a file is at path.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

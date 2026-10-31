package hls

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// HEVCQuality is the quality factor a file is converted to HEVC with when
// the settings leave it to Polyfin, Jellyfin's for HEVC.
const HEVCQuality = 28

// slotWait is how often a file conversion waiting for a free conversion
// slot looks again (see ConvertFile).
const slotWait = 15 * time.Second

// FileConversion converts a whole file into another, its video to HEVC at
// the same size, and its other tracks copied as they are, as recordings
// are made smaller. Video fills in the encoder, a GPU's or libx265, with
// the video's Width, Height and FrameRate, the deinterlacing, and the
// settings' Preset and Quality; Bitrate is the source's, which caps the
// copy's, 0 when unknown. The quality factor is on x265's scale, which
// QSV's and AMF's share, and VideoToolbox's is drawn from (see
// videoToolboxQuality).
type FileConversion struct {
	Input, Output string
	Video         VideoEncoding
}

// args is FFmpeg's command line of the conversion, which reports its
// progress on its standard output. Every track is kept, in its order, with
// the metadata and chapters, so that the copy holds what the original
// does. The video is 8-bit 4:2:0, deinterlaced when asked, at the
// source's size: the filters of a conversion on its GPU, or in memory.
func (c FileConversion) args() []string {
	v := &c.Video
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-progress", "pipe:1"}
	args = append(args, v.inputs()...)
	args = append(args, "-i", c.Input, "-map", "0", "-c", "copy", "-c:v", v.Encoder, "-vf", v.filters())
	quality := cmp.Or(v.Quality, HEVCQuality)
	switch encoderFamily(v.Encoder) {
	case familySoftware:
		args = append(args, "-preset", cmp.Or(v.Preset, "veryfast"), "-crf", strconv.Itoa(quality), "-x265-params", "log-level=error")
	case familyNVENC:
		args = append(args, "-preset", cmp.Or(nvencPresets[v.Preset], "p4"), "-rc", "vbr", "-cq", strconv.Itoa(quality), "-b:v", "0")
	case familyVideoToolbox:
		args = append(args, "-global_quality", strconv.Itoa(videoToolboxQuality(quality)))
		if speed := videoToolboxSpeed[v.Preset]; speed != "" {
			args = append(args, "-prio_speed", speed)
		}
	case familyQSV:
		// Intelligent constant quality, on the same scale as x265's.
		args = append(args, "-global_quality", strconv.Itoa(quality))
		if preset := qsvPresets[v.Preset]; preset != "" {
			args = append(args, "-preset", preset)
		}
	case familyAMF:
		args = append(args, "-rc", "cqp", "-qp_i", strconv.Itoa(quality), "-qp_p", strconv.Itoa(quality))
		if preset := amfQualities[v.Preset]; preset != "" {
			args = append(args, "-quality", preset)
		}
	default:
		if v.Hardware != nil && v.Hardware.QVBR && v.Bitrate > 0 {
			args = append(args, "-rc_mode", "QVBR", "-global_quality", strconv.Itoa(quality), "-b:v", strconv.FormatInt(v.Bitrate/2, 10))
		} else {
			args = append(args, "-rc_mode", "CQP", "-qp", strconv.Itoa(quality))
		}
		if level := vaapiCompression[v.Preset]; level != "" {
			args = append(args, "-compression_level", level)
		}
	}
	// The source's bitrate caps the copy's: it is never meant to be
	// larger.
	if v.Bitrate > 0 {
		args = append(args, "-maxrate", strconv.FormatInt(v.Bitrate, 10), "-bufsize", strconv.FormatInt(v.Bitrate*2, 10))
	}
	return append(args, "-profile:v", "main", "-f", "matroska", c.Output)
}

// ConvertFile runs a file conversion, at the processor's lowest priority,
// until it ends or ctx does. It counts as one of the conversions of video
// the limit bounds (see LimitConversions), and waits until the playbacks
// leave one free: it never takes a playback's place. Progress, when set,
// is told how far into the source it has written.
func (m *Manager) ConvertFile(ctx context.Context, c FileConversion, progress func(time.Duration)) error {
	if err := m.takeFileSlot(ctx); err != nil {
		return err
	}
	defer func() {
		m.mu.Lock()
		m.files--
		m.mu.Unlock()
	}()
	cmd := exec.CommandContext(ctx, m.ffmpegPath(), c.args()...)
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	media.LowerPriority(cmd.Process.Pid)
	readProgress(out, progress)
	err = cmd.Wait()
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		return fmt.Errorf("FFmpeg failed: %w: %s", err, bytes.TrimSpace(stderr.bytes()))
	}
	return nil
}

// takeFileSlot waits until a file conversion may start, as a conversion
// streamed as one file would (see admitsAlone), and counts it.
func (m *Manager) takeFileSlot(ctx context.Context) error {
	wait := m.slotWait
	if wait <= 0 {
		wait = slotWait
	}
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return ErrStopped
		}
		if m.admitsAlone(Key{Converts: true}) {
			m.files++
			m.mu.Unlock()
			return nil
		}
		m.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-m.done:
			timer.Stop()
			return ErrStopped
		case <-timer.C:
		}
	}
}

// DecodedLength decodes the video of the file at path to its end, at the
// processor's lowest priority, and reports how far it went: an error when
// FFmpeg met one. The other tracks are left out: a conversion copies them
// as they are.
func (m *Manager) DecodedLength(ctx context.Context, path string) (time.Duration, error) {
	cmd := exec.CommandContext(ctx, m.ffmpegPath(), "-hide_banner", "-nostdin", "-loglevel", "error", "-xerror", "-progress", "pipe:1",
		"-i", path, "-map", "0:v", "-f", "null", "-")
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	media.LowerPriority(cmd.Process.Pid)
	var decoded time.Duration
	readProgress(out, func(at time.Duration) { decoded = at })
	if err := cmd.Wait(); err != nil {
		return decoded, fmt.Errorf("FFmpeg failed: %w: %s", err, bytes.TrimSpace(stderr.bytes()))
	}
	if met := bytes.TrimSpace(stderr.bytes()); len(met) > 0 {
		return decoded, fmt.Errorf("FFmpeg met errors: %s", met)
	}
	return decoded, nil
}

// readProgress reads what FFmpeg's -progress writes, telling progress, if
// set, each output time, until FFmpeg closes it.
func readProgress(r io.Reader, progress func(time.Duration)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, value, _ := strings.Cut(scanner.Text(), "=")
		if key != "out_time_us" || progress == nil {
			continue
		}
		if us, err := strconv.ParseInt(value, 10, 64); err == nil && us >= 0 {
			progress(time.Duration(us) * time.Microsecond)
		}
	}
	_, _ = io.Copy(io.Discard, r)
}

// Uses reports whether an encoding of version runs, or keeps its video,
// for a playback: its file is read.
func (m *Manager) Uses(version string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.encodings {
		if k.Version == version {
			return true
		}
	}
	for k := range m.kept {
		if k.Version == version {
			return true
		}
	}
	for p := range m.progressives {
		if p.key.Version == version {
			return true
		}
	}
	return false
}

package hls

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// progressiveKeyframes is how often converted video streamed as one file
// has a keyframe, as -force_key_frames takes it: every 3 seconds, so that
// a player seeks in a file saved from it as in a remux's segments.
const progressiveKeyframes = "expr:gte(t,n_forced*3)"

// progressive is a conversion streamed as one file (see Progressive).
type progressive struct {
	key    Key
	cancel context.CancelFunc
	// opened is set, with remux, once its input is open; stopped once the
	// manager stopped it. Both are guarded by Manager.mu.
	opened, stopped bool
	remux           Remux
	// position is how far into the version FFmpeg has written, in
	// nanoseconds.
	position atomic.Int64
}

// Progressive streams the encoding open prepares as one file in its
// Format, fragmented MP4 or MPEG-TS, from start on, to w as FFmpeg writes
// it: FFmpeg goes as fast as w takes what it writes. Unlike an HLS
// encoding it needs no keyframe index, and keeps nothing on disk. Its
// output starts at zero whatever start is.
//
// It counts toward the conversions LimitConversions bounds while it runs,
// when key converts the video, on its own even next to the user's other
// plays of the version (see admitsAlone): one past the limit is refused
// with ErrBusy before its input is opened. Stop, for its play session, and
// Close end it with ErrStopped. A failed input (see Remux.Failed) ends it
// with its failure; a ctx ending, with ctx's error.
func (m *Manager) Progressive(ctx context.Context, key Key, open Opener, start time.Duration, w io.Writer) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrStopped
	}
	if !m.admitsAlone(key) {
		m.mu.Unlock()
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := &progressive{key: key, cancel: cancel}
	if m.progressives == nil {
		m.progressives = map[*progressive]struct{}{}
	}
	m.progressives[p] = struct{}{}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.progressives, p)
		m.mu.Unlock()
	}()

	remux, release, err := open(ctx)
	m.mu.Lock()
	stopped := p.stopped
	if err == nil {
		p.opened, p.remux = true, remux
	}
	m.mu.Unlock()
	if err != nil {
		if stopped {
			return ErrStopped
		}
		return err
	}
	defer release()
	if stopped {
		return ErrStopped
	}
	if remux.Failed != nil {
		go func() {
			select {
			case <-remux.Failed:
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	cmd := exec.CommandContext(ctx, m.ffmpeg, remux.progressiveArgs(start)...)
	cmd.WaitDelay = 5 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	cmd.Stdout = w
	// FFmpeg's progress comes on file descriptor 3 (see progressiveArgs).
	progress, written, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.ExtraFiles = []*os.File{written}
	p.position.Store(int64(start))
	err = cmd.Start()
	_ = written.Close()
	if err != nil {
		_ = progress.Close()
		return fmt.Errorf("start FFmpeg: %w", err)
	}
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		defer progress.Close()
		p.watch(progress, start)
	}()
	err = cmd.Wait()
	<-watched
	m.mu.Lock()
	stopped = p.stopped
	m.mu.Unlock()
	switch {
	case remux.Failed != nil && isClosed(remux.Failed):
		return remux.Failure()
	case stopped:
		return ErrStopped
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		return fmt.Errorf("FFmpeg failed: %w: %s", err, bytes.TrimSpace(stderr.bytes()))
	}
	return nil
}

// isClosed reports whether c is closed.
func isClosed(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// stopProgressives stops the conversions streamed as one file that match
// selects: they end with ErrStopped. The caller holds m.mu.
func (m *Manager) stopProgressives(match func(Key) bool) {
	for p := range m.progressives {
		if match(p.key) {
			p.stopped = true
			p.cancel()
		}
	}
}

// watch follows FFmpeg's progress, key=value lines every half second, to
// keep how far into the version it has written: start, where it began,
// and its output time, which starts at zero.
func (p *progressive) watch(r io.Reader, start time.Duration) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, value, _ := strings.Cut(scanner.Text(), "=")
		if key != "out_time_us" {
			continue
		}
		if us, err := strconv.ParseInt(value, 10, 64); err == nil && us >= 0 {
			p.position.Store(int64(start + time.Duration(us)*time.Microsecond))
		}
	}
	// FFmpeg would wait for the pipe to be read.
	_, _ = io.Copy(io.Discard, r)
}

// progressiveArgs is FFmpeg's command line streaming the version as one
// file from start. Like a remux's, timestamps are read as the source
// tells them, so that a subtitle file burned in lines up with the video
// (see burnGraph), then shifted to start at zero in the output.
func (r Remux) progressiveArgs(start time.Duration) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-progress", "pipe:3"}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start.Seconds(), 'f', 6, 64))
	}
	if r.Encode != nil {
		args = append(args, r.Encode.inputs()...)
	}
	args = append(args, r.InputOptions...)
	args = append(args, "-copyts", "-i", r.Input)
	video := "0:" + strconv.Itoa(r.Video)
	if r.Encode != nil && r.Encode.Burns() {
		if r.Encode.BurnFile != "" {
			args = append(args, "-i", r.Encode.BurnFile)
		}
		args = append(args, "-filter_complex", r.Encode.burnGraph(r.Video, start, ""))
		video = "[video]"
	}
	args = append(args, "-map", video)
	if r.Audio >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(r.Audio))
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy")
	if r.Encode != nil {
		args = append(args, r.Encode.encoderArgs(progressiveKeyframes, "")...)
	}
	if r.VideoTag != "" {
		args = append(args, "-tag:v", r.VideoTag)
	}
	args = append(args, r.audioArgs()...)
	if r.Audio >= 0 && r.ADTS && r.AudioCodec == "" && r.Format == FMP4 {
		// AAC copied from MPEG-TS is reframed as MP4 stores it.
		args = append(args, "-bsf:a", "aac_adtstoasc")
	}
	args = append(args, r.threadArgs()...)
	args = append(args, "-avoid_negative_ts", "make_zero")
	if r.Format == TS {
		return append(args, "-muxdelay", "0", "-muxpreload", "0", "-f", "mpegts", "pipe:1")
	}
	// An MP4 written as it is made: fragmented, a fragment per keyframe,
	// its header waiting for the first, as some codecs, such as E-AC-3,
	// describe themselves in their first packet.
	return append(args, "-f", "mp4", "-movflags", "+frag_keyframe+empty_moov+delay_moov+default_base_moof", "pipe:1")
}

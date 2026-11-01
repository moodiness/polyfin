package hls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/process"
)

// track is the audio a kept video plays (see KeepVideo): one track of the
// version, copied or converted, cut at the video's segment times by an
// FFmpeg of its own, which serves every variant of the playback. It reads
// the version as the video's FFmpeg does, through the source cache, and
// makes no segment the video has not begun (see advance), nor further
// ahead than the video does.
type track struct {
	m   *Manager
	dir string
	// session and audio are the play session that asked for the track
	// last, and the track's index it asked, guarded by keeping.mu.
	session string
	audio   int
	remux   Remux
	release func()

	mu sync.Mutex
	// changed is closed, then replaced, whenever a segment is ready, a job
	// ends, a player asks for another segment, or the video moves on.
	changed chan struct{}
	ready   []bool
	// init is set once the initialization segment of fragmented MP4 is
	// made, or MPEG-TS's program map entry of the audio, entry, is known.
	init      bool
	entry     []byte
	job       *trackJob
	requested int
	// video is the segment the video began last, which the track does not
	// pass: the one being made, or one a player asked for.
	video int
	// runs counts the FFmpeg processes started, which tests read.
	runs    int
	stopped bool
}

// trackJob is one run of the audio's FFmpeg, from segment start onwards.
type trackJob struct {
	start int
	// next is the segment being made, once begun.
	next   int
	begun  bool
	cancel context.CancelFunc
	done   bool
	err    error
	// data are the MPEG-TS packets of the segment being made; samples, its
	// fMP4 samples, the first decoded at decode.
	data    []byte
	samples []fragmentSample
	decode  uint64
	// progressed is when FFmpeg last read or wrote, as far as it tells,
	// or else when the job started.
	progressed time.Time
}

// newTrack prepares the track remux plays, from what open returned for
// it: its input, and its audio, copied or converted. The video is left
// to the kept video's FFmpeg.
func (m *Manager) newTrack(key Key, remux Remux, release func()) (*track, error) {
	remux.Encode, remux.Subtitles, remux.Extracted, remux.VideoTag = nil, nil, nil, ""
	dir, err := os.MkdirTemp(m.dir, "audio-")
	if err != nil {
		return nil, err
	}
	t := &track{m: m, dir: dir, session: key.Session, audio: key.Audio, remux: remux, release: release,
		changed: make(chan struct{}), ready: make([]bool, remux.Plan.Len())}
	return t, nil
}

// sameTrack reports whether two remuxes make the same audio.
func sameTrack(a, b Remux) bool {
	return a.Audio == b.Audio && a.ADTS == b.ADTS && a.AudioCodec == b.AudioCodec && a.AudioChannels == b.AudioChannels &&
		a.AudioBitrate == b.AudioBitrate && a.AudioFilter == b.AudioFilter && slices.Equal(a.AudioDecoder, b.AudioDecoder) &&
		a.Format == b.Format && a.Plan.Len() == b.Plan.Len()
}

func (t *track) path(n int) string {
	if n < 0 {
		return filepath.Join(t.dir, "init.mp4")
	}
	return filepath.Join(t.dir, strconv.Itoa(n)+".audio")
}

// signal wakes whoever waits on the track. The caller holds t.mu.
func (t *track) signal() {
	close(t.changed)
	t.changed = make(chan struct{})
}

// advance records that the video began segment n.
func (t *track) advance(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n != t.video {
		t.video = n
		t.signal()
	}
}

// follow starts the track where its playback is: the video began segment
// n, and the player asked for segment requested last.
func (t *track) follow(n, requested int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.video, t.requested = n, requested
	t.signal()
}

// programEntry is the program map entry of the audio in MPEG-TS.
func (t *track) programEntry() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.entry
}

// await opens the audio of segment n, or the initialization segment of
// fragmented MP4 for n = -1, once made; nil when the track ends before
// segment n, or for the initialization segment of MPEG-TS. The video's
// segment n is ready: the track may make it.
func (t *track) await(ctx context.Context, n int) (*os.File, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	since := time.Now()
	if n >= 0 {
		t.requested = n
		t.video = max(t.video, n)
		t.prune(n)
		t.signal()
	}
	for {
		if t.stopped {
			return nil, ErrStopped
		}
		if n < 0 && t.init || n >= 0 && t.ready[n] {
			if n < 0 && t.remux.Format != FMP4 {
				return nil, nil
			}
			return os.Open(t.path(n))
		}
		j := t.job
		start := n
		if n < 0 {
			// The initialization segment comes with any segment: start
			// where the player last asked.
			start = t.requested
		}
		switch {
		case j != nil && j.done && j.err != nil && !errors.Is(j.err, errStale) && j.start == start:
			// Reported once: the next request tries again.
			t.job = nil
			return nil, j.err
		case j != nil && j.done && j.err == nil && n >= j.start && n >= j.next:
			// The track ends before this segment.
			return nil, nil
		case j == nil || j.done || n >= 0 && (n < j.next || n > j.next+reach):
			t.start(start)
		}
		if err := t.waitProgress(ctx, since); err != nil {
			return nil, err
		}
	}
}

// waitProgress waits for a change, or reports ErrStalled once the running
// job made no progress for the stall timeout, counted from since at the
// earliest. The caller holds t.mu, held again when it returns.
func (t *track) waitProgress(ctx context.Context, since time.Time) error {
	last := since
	if j := t.job; j != nil && j.progressed.After(last) {
		last = j.progressed
	}
	left := time.Until(last.Add(t.m.stall))
	if left <= 0 {
		return ErrStalled
	}
	timer := time.NewTimer(left)
	defer timer.Stop()
	changed := t.changed
	t.mu.Unlock()
	defer t.mu.Lock()
	select {
	case <-changed:
	case <-timer.C:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// wait waits for a change, holding t.mu again when it returns.
func (t *track) wait(ctx context.Context) error {
	changed := t.changed
	t.mu.Unlock()
	defer t.mu.Lock()
	select {
	case <-changed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// prune removes the segments far from segment n, as the video's are (see
// encoding.prune). The caller holds t.mu.
func (t *track) prune(n int) {
	far := t.m.aheadLimit(t.remux.Plan, n) + reach
	for i, ready := range t.ready {
		if ready && (i < n-behind || i > far) {
			t.ready[i] = false
			if err := os.Remove(t.path(i)); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.m.logger.Warn("A segment could not be removed", "error", err)
			}
		}
	}
}

// start replaces the running job with one starting at segment n. The
// caller holds t.mu.
func (t *track) start(n int) {
	if t.job != nil {
		t.job.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &trackJob{start: n, next: n, cancel: cancel, progressed: time.Now()}
	t.job = j
	go t.run(ctx, j)
}

// run runs FFmpeg for a job and files the audio it writes into segments.
func (t *track) run(ctx context.Context, j *trackJob) {
	cmd := exec.CommandContext(ctx, t.m.ffmpegPath(), t.remux.trackArgs(j.start)...)
	cmd.WaitDelay = 5 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	output, err := cmd.StdoutPipe()
	// FFmpeg's progress comes on pipe:3 (see trackArgs).
	var outputs *process.Outputs
	if err == nil {
		outputs, err = process.NewOutputs(cmd, 1)
	}
	if err == nil {
		err = cmd.Start()
		outputs.Started(cmd, err)
	}
	if err != nil {
		t.finish(j, fmt.Errorf("start FFmpeg: %w", err))
		return
	}
	progress := outputs.Readers[0]
	t.mu.Lock()
	t.runs++
	t.mu.Unlock()
	var watching sync.WaitGroup
	watching.Go(func() {
		defer progress.Close()
		watchProgress(progress, func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			j.progressed = time.Now()
		})
	})
	t.m.logger.Debug("Started the audio of a kept conversion", "from", j.start, "stream", t.remux.Audio)
	take := func(p audioPiece) error { return t.take(ctx, j, p) }
	if t.remux.Format == TS {
		err = splitAudioTS(output, take)
	} else {
		err = splitAudioMP4(output, take)
	}
	replaced := errors.Is(err, errStale) || ctx.Err() != nil
	if err != nil && !replaced {
		j.cancel()
	}
	_, _ = io.Copy(io.Discard, output)
	waitErr := cmd.Wait()
	watching.Wait()
	switch {
	case replaced:
		err = errStale
	case err == nil && waitErr != nil:
		err = fmt.Errorf("FFmpeg failed: %w: %s", waitErr, bytes.TrimSpace(stderr.bytes()))
	}
	if err != nil && !errors.Is(err, errStale) {
		t.m.logger.Warn("The audio of a kept conversion failed", "from", j.start, "error", err)
	}
	t.finish(j, err)
}

// take files a piece of the audio FFmpeg writes into the segment its
// time falls in. A piece before the job's first segment is left out.
func (t *track) take(ctx context.Context, j *trackJob, p audioPiece) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != j || t.stopped {
		return errStale
	}
	j.progressed = time.Now()
	if p.init {
		if t.init {
			return nil
		}
		if t.remux.Format == FMP4 {
			if err := writeFile(t.path(-1), p.data); err != nil {
				return err
			}
		} else {
			t.entry = p.data
		}
		t.init = true
		t.signal()
		return nil
	}
	plan := t.remux.Plan
	if !j.begun {
		if p.at < plan.Start(j.next)-tolerance {
			return nil
		}
		j.begun = true
	}
	for j.next+1 < plan.Len() && p.at >= plan.Start(j.next+1)-tolerance {
		if err := t.close(j); err != nil {
			return err
		}
		// Not past the video, nor further ahead than it goes.
		for j.next > t.video || j.next > t.m.aheadLimit(plan, t.requested) {
			if err := t.wait(ctx); err != nil {
				return errStale
			}
			if t.job != j || t.stopped {
				return errStale
			}
		}
	}
	if t.remux.Format == TS {
		j.data = append(j.data, p.data...)
		return nil
	}
	if len(j.samples) == 0 {
		j.decode = p.decode
	}
	j.samples = append(j.samples, p.samples...)
	return nil
}

// close completes segment j.next and moves the job to the next. The caller
// holds t.mu.
func (t *track) close(j *trackJob) error {
	data := j.data
	if t.remux.Format == FMP4 && len(j.samples) > 0 {
		data = audioFragment(uint32(j.next+1), j.decode, j.samples)
	}
	if err := writeFile(t.path(j.next), data); err != nil {
		return err
	}
	j.data, j.samples = nil, nil
	t.ready[j.next] = true
	j.next++
	t.signal()
	return nil
}

// finish records the end of a job; one that reached the end of its output
// completes its last segment.
func (t *track) finish(j *trackJob, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil && t.job == j && !t.stopped && j.begun {
		err = t.close(j)
	}
	j.done, j.err = true, err
	if t.job == j {
		t.signal()
	}
}

// stop ends the track and removes its segments.
func (t *track) stop() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	if t.job != nil {
		t.job.cancel()
	}
	t.signal()
	t.mu.Unlock()
	t.release()
	if err := os.RemoveAll(t.dir); err != nil {
		t.m.logger.Warn("The audio segments of a kept conversion could not be removed", "error", err)
	}
}

// trackArgs is FFmpeg's command line making the audio of a kept video
// alone, from segment n: read as the video's FFmpeg reads it, converted as
// it would be along the video (see args), timestamped alike, and written a
// frame at a time, so that the track cuts it at the video's segment
// times. FFmpeg reports its progress on pipe 3.
func (r Remux) trackArgs(n int) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-progress", "pipe:3"}
	seek, _ := r.seekArgs(n, r.Lookback)
	args = append(args, seek...)
	args = append(args, r.InputOptions...)
	args = append(args, r.decoderArgs()...)
	args = append(args, "-copyts", "-i", r.Input, "-map", "0:"+strconv.Itoa(r.Audio), "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy")
	args = append(args, r.audioArgs()...)
	args = append(args, "-bsf:a", r.audioFilters())
	args = append(args, r.threadArgs()...)
	args = append(args, "-avoid_negative_ts", "disabled", "-output_ts_offset", strconv.FormatFloat(timestampOffset.Seconds(), 'f', -1, 64))
	if r.Format == TS {
		// A PES per frame, as with the video (see args).
		return append(args, "-muxdelay", "0", "-muxpreload", "0", "-mpegts_start_pid", strconv.Itoa(audioPID), "-f", "mpegts", "pipe:1")
	}
	return append(args, "-use_editlist", "0", "-f", "mp4", "-movflags", "+frag_every_frame+empty_moov+delay_moov+default_base_moof+frag_discont", "pipe:1")
}

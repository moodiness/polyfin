package hls

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Tracks are converted for players that cannot take them as they are:
// streamed progressively, or cut into HLS segments of AudioSegment, as
// Jellyfin cuts audio. Every audio frame starts anew, so segments start
// exactly every AudioSegment, without a keyframe index. FFmpeg reads at
// most audioReadRate times as fast as the audio plays after a first
// audioBurst, so that a long audiobook is not converted far ahead of the
// listener, and starts again from the segment asked for when a player
// seeks further than audioReach ahead.
const (
	AudioSegment   = 3 * time.Second
	audioReadRate  = "10"
	audioBurst     = "30"
	audioReach     = 20
	audioPoll      = 50 * time.Millisecond
	audioKeptAhead = 200
	// audioKeptBehind is how many segments are kept before the one asked
	// for, for a player that asks again.
	audioKeptBehind = 10
)

// AudioPlan cuts a track lasting duration into segments of AudioSegment.
func AudioPlan(duration time.Duration) Plan {
	starts := []time.Duration{0}
	for at := AudioSegment; at < duration; at += AudioSegment {
		starts = append(starts, at)
	}
	return Plan{starts: starts, duration: duration}
}

// Audio is what a track is converted to: the FFmpeg encoder (aac,
// libmp3lame, libopus, flac, pcm_s16le…), its bitrate (zero for lossless
// codecs), sample rate and channels, zero keeping the source's, and the
// filter graph it goes through, such as a downmix to stereo, empty for
// none.
type Audio struct {
	// Input is the URL FFmpeg reads the track from, with InputOptions
	// before it, such as those of the HLS demuxer.
	Input        string
	InputOptions []string
	Encoder      string
	Bitrate      int64
	SampleRate   int
	Channels     int
	Filter       string
}

// args are FFmpeg's options converting the track's first audio stream.
func (a Audio) args() []string {
	args := []string{"-map", "0:a:0", "-map_metadata", "-1", "-map_chapters", "-1", "-vn", "-sn", "-dn", "-c:a", a.Encoder}
	if a.Bitrate > 0 {
		args = append(args, "-b:a", strconv.FormatInt(a.Bitrate, 10))
	}
	if a.SampleRate > 0 {
		args = append(args, "-ar", strconv.Itoa(a.SampleRate))
	}
	if a.Channels > 0 {
		args = append(args, "-ac", strconv.Itoa(a.Channels))
	}
	if a.Filter != "" {
		args = append(args, "-af", a.Filter)
	}
	return args
}

// Convert streams a track converted to format, an FFmpeg muxer (mp3, adts,
// ogg, flac, wav, ipod for MP4 audio…), from start, to w, as it is made.
func (m *Manager) Convert(ctx context.Context, audio Audio, format string, start time.Duration, w io.Writer) error {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start.Seconds(), 'f', 3, 64))
	}
	args = append(args, audio.InputOptions...)
	args = append(args, "-i", audio.Input)
	args = append(args, audio.args()...)
	if format == "ipod" || format == "mp4" {
		// An MP4 written as it is made: fragmented.
		args = append(args, "-movflags", "+frag_keyframe+empty_moov+default_base_moof")
	}
	args = append(args, "-f", format, "pipe:1")
	cmd := exec.CommandContext(ctx, m.ffmpeg, args...)
	cmd.WaitDelay = 5 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	cmd.Stdout = w
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("FFmpeg failed: %w: %s", err, bytes.TrimSpace(stderr.bytes()))
	}
	return nil
}

// AudioOpener prepares an audio encoding: the conversion and its plan, and
// a function releasing its input once the encoding stops.
type AudioOpener func(ctx context.Context) (Audio, Plan, func(), error)

// AudioKey identifies an audio encoding: what a play session plays.
type AudioKey struct {
	Session string
	Format  Format
}

func (k AudioKey) name() string {
	sum := sha256.Sum256([]byte("audio\x00" + k.Session + "\x00" + k.Format.Extension()))
	return hex.EncodeToString(sum[:12])
}

// audioEncoding is one conversion of a track into segments for a play
// session.
type audioEncoding struct {
	m      *Manager
	key    AudioKey
	dir    string
	opened chan struct{}
	audio  Audio
	plan   Plan
	free   func()
	err    error

	mu      sync.Mutex
	job     *audioJob
	used    time.Time
	stopped bool
	// pruned is the first segment not removed for being far behind the
	// player.
	pruned int
}

// audioJob is one run of FFmpeg, from segment start onwards.
type audioJob struct {
	start  int
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

// AudioInit opens the initialization segment of a fragmented MP4 audio
// encoding.
func (m *Manager) AudioInit(ctx context.Context, key AudioKey, open AudioOpener) (*os.File, error) {
	if key.Format != FMP4 {
		return nil, ErrNotFound
	}
	return m.audioFile(ctx, key, open, -1)
}

// AudioSegment opens segment n of an audio encoding, starting or moving
// FFmpeg to it when needed.
func (m *Manager) AudioSegment(ctx context.Context, key AudioKey, open AudioOpener, n int) (*os.File, error) {
	return m.audioFile(ctx, key, open, n)
}

func (m *Manager) audioFile(ctx context.Context, key AudioKey, open AudioOpener, n int) (*os.File, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrStopped
	}
	if m.audios == nil {
		m.audios = map[AudioKey]*audioEncoding{}
	}
	e := m.audios[key]
	if e == nil {
		e = &audioEncoding{m: m, key: key, dir: filepath.Join(m.dir, key.name()), opened: make(chan struct{}), used: time.Now()}
		m.audios[key] = e
		m.mu.Unlock()
		e.open(ctx, open)
	} else {
		m.mu.Unlock()
	}
	select {
	case <-e.opened:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if e.err != nil {
		m.mu.Lock()
		if m.audios[key] == e {
			delete(m.audios, key)
		}
		m.mu.Unlock()
		return nil, e.err
	}
	if n >= e.plan.Len() {
		return nil, ErrNotFound
	}
	return e.await(ctx, n)
}

func (e *audioEncoding) open(ctx context.Context, open AudioOpener) {
	defer close(e.opened)
	audio, plan, free, err := open(context.WithoutCancel(ctx))
	if err != nil {
		e.err = err
		return
	}
	if err := os.MkdirAll(e.dir, 0o700); err != nil {
		free()
		e.err = err
		return
	}
	e.audio, e.plan, e.free = audio, plan, free
}

func (e *audioEncoding) path(n int) string {
	if n < 0 {
		return filepath.Join(e.dir, "init.mp4")
	}
	return filepath.Join(e.dir, strconv.Itoa(n)+"."+e.key.Format.Extension())
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// await opens segment n, or the initialization segment for n = -1, once
// FFmpeg made it. A segment comes whole: FFmpeg writes each under a
// temporary name first. The initialization segment is complete once the
// job's first segment is.
func (e *audioEncoding) await(ctx context.Context, n int) (*os.File, error) {
	ticker := time.NewTicker(audioPoll)
	defer ticker.Stop()
	for {
		e.mu.Lock()
		e.used = time.Now()
		if e.stopped {
			e.mu.Unlock()
			return nil, ErrStopped
		}
		for ; e.pruned < n-audioKeptBehind; e.pruned++ {
			_ = os.Remove(e.path(e.pruned))
		}
		j := e.job
		ready := exists(e.path(n))
		if n < 0 {
			ready = ready && j != nil && exists(e.path(j.start))
		}
		if ready {
			e.mu.Unlock()
			return os.Open(e.path(n))
		}
		target := max(n, 0)
		switch {
		case j == nil:
			e.start(target)
		case jobDone(j) && j.err != nil && j.start == target:
			// Reported once: the next request tries again.
			e.job = nil
			e.mu.Unlock()
			return nil, j.err
		case jobDone(j) && n >= j.start:
			// FFmpeg reached the end before this segment.
			e.mu.Unlock()
			return nil, ErrNotFound
		case n >= 0 && (n < j.start || n > e.made(j)+audioReach) || jobDone(j):
			e.start(target)
		}
		e.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func jobDone(j *audioJob) bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

// made is the last segment the job made. The caller holds e.mu.
func (e *audioEncoding) made(j *audioJob) int {
	n := j.start - 1
	for n+1 < e.plan.Len() && exists(e.path(n+1)) {
		n++
	}
	return n
}

// start replaces the running job with one starting at segment n, after
// removing the segments it would write again. The caller holds e.mu.
func (e *audioEncoding) start(n int) {
	if e.job != nil {
		e.job.cancel()
		<-e.job.done
	}
	for i := n; i < e.plan.Len() && i < n+audioKeptAhead; i++ {
		_ = os.Remove(e.path(i))
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &audioJob{start: n, cancel: cancel, done: make(chan struct{})}
	e.job = j
	go e.run(ctx, j)
}

// run runs FFmpeg's HLS muxer from segment start, writing segments as
// they are made, timestamped from the track's start.
func (e *audioEncoding) run(ctx context.Context, j *audioJob) {
	defer close(j.done)
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-readrate", audioReadRate, "-readrate_initial_burst", audioBurst}
	if j.start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(e.plan.Start(j.start).Seconds(), 'f', 6, 64))
	}
	args = append(args, e.audio.InputOptions...)
	args = append(args, "-copyts", "-i", e.audio.Input)
	args = append(args, e.audio.args()...)
	args = append(args, "-avoid_negative_ts", "disabled", "-f", "hls", "-hls_time", strconv.FormatFloat(AudioSegment.Seconds(), 'f', -1, 64),
		"-hls_playlist_type", "vod", "-hls_flags", "temp_file+independent_segments", "-start_number", strconv.Itoa(j.start),
		"-hls_segment_filename", filepath.Join(e.dir, "%d."+e.key.Format.Extension()))
	if e.key.Format == FMP4 {
		args = append(args, "-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4")
	} else {
		args = append(args, "-hls_segment_type", "mpegts")
	}
	args = append(args, filepath.Join(e.dir, "ffmpeg.m3u8"))
	cmd := exec.CommandContext(ctx, e.m.ffmpeg, args...)
	cmd.WaitDelay = 5 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	started := time.Now()
	err := cmd.Run()
	switch {
	case ctx.Err() != nil:
		err = errStale
		e.m.logger.Debug("An audio conversion was stopped", "from", j.start, "duration", time.Since(started))
	case err != nil:
		err = fmt.Errorf("FFmpeg failed: %w: %s", err, bytes.TrimSpace(stderr.bytes()))
		e.m.logger.Warn("An audio conversion failed", "from", j.start, "error", err)
	default:
		e.m.logger.Debug("An audio conversion reached the end", "from", j.start, "duration", time.Since(started))
	}
	j.err = err
}

func (e *audioEncoding) idle() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return time.Since(e.used) > idleTimeout
}

// stop ends the encoding and removes its segments.
func (e *audioEncoding) stop() {
	<-e.opened
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return
	}
	e.stopped = true
	j := e.job
	e.mu.Unlock()
	if j != nil {
		j.cancel()
		<-j.done
	}
	if e.err != nil {
		return
	}
	e.free()
	if err := os.RemoveAll(e.dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		e.m.logger.Warn("The segments of an audio conversion could not be removed", "error", err)
	}
}

// stopAudios stops the audio encodings match selects.
func (m *Manager) stopAudios(match func(AudioKey, *audioEncoding) bool) {
	m.mu.Lock()
	var stopping []*audioEncoding
	for key, e := range m.audios {
		if match(key, e) {
			stopping = append(stopping, e)
			delete(m.audios, key)
		}
	}
	m.mu.Unlock()
	for _, e := range stopping {
		e.stop()
	}
}

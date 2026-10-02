package hls

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	// ahead is how many segments FFmpeg makes past the last one asked for
	// before it waits: a player buffers about a minute ahead.
	ahead = 10
	// reach is how far past the segment being made a request waits for it,
	// rather than starting FFmpeg again from the segment asked for.
	reach = 3
	// behind is how many segments are kept before the last one asked for,
	// for a player that asks again for the segment it plays. Further back,
	// FFmpeg makes them again from the source cache: a remuxed 4K segment
	// weighs about 60 MB.
	behind = 3
	// idleTimeout stops an encoding no player asked anything of for that
	// long, such as a paused one; it starts again on the next request.
	idleTimeout = 3 * time.Minute
	// tolerance absorbs the rounding between the keyframe index and the
	// timestamps in FFmpeg's output.
	tolerance = 2 * time.Millisecond
)

var (
	// ErrNotFound reports a segment the version does not have.
	ErrNotFound = errors.New("no such segment")
	// ErrStopped reports an encoding stopped while a player waited for it.
	ErrStopped = errors.New("the encoding was stopped")
	errStale   = errors.New("the job was replaced")
)

// Remux is what an encoding reads and produces.
type Remux struct {
	// Input is the URL FFmpeg reads the version from.
	Input string
	// Video and Audio are FFmpeg stream indexes; Audio is -1 for none.
	Video, Audio int
	// VideoTag is the sample entry of the video in MP4, such as hvc1 for
	// HEVC, which Apple players require; empty keeps FFmpeg's.
	VideoTag string
	Format   Format
	Plan     Plan
}

// Opener prepares an encoding: what it remuxes, and a function releasing
// its input once the encoding stops.
type Opener func(ctx context.Context) (Remux, func(), error)

// Key identifies an encoding: what a play session plays.
type Key struct {
	Session string
	Audio   int
	Format  Format
}

func (k Key) name() string {
	sum := sha256.Sum256([]byte(k.Session + "\x00" + strconv.Itoa(k.Audio) + "\x00" + k.Format.Extension()))
	return hex.EncodeToString(sum[:12])
}

// Manager runs the encodings players ask for.
type Manager struct {
	ffmpeg string
	dir    string
	logger *slog.Logger
	done   chan struct{}

	mu        sync.Mutex
	encodings map[Key]*encoding
	closed    bool
}

// NewManager returns a manager running FFmpeg from ffmpegPath and keeping
// segments under dir, which it empties first: segments are not reused
// across runs.
func NewManager(ffmpegPath, dir string, logger *slog.Logger) (*Manager, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{ffmpeg: ffmpegPath, dir: dir, logger: logger, done: make(chan struct{}), encodings: map[Key]*encoding{}}
	go m.stopIdle()
	return m, nil
}

// Close stops every encoding.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	close(m.done)
	m.mu.Unlock()
	m.stopWhere(func(Key, *encoding) bool { return true })
}

// Stop stops the encodings of a play session.
func (m *Manager) Stop(session string) {
	m.stopWhere(func(key Key, _ *encoding) bool { return key.Session == session })
}

func (m *Manager) stopWhere(match func(Key, *encoding) bool) {
	m.mu.Lock()
	var stopping []*encoding
	for key, e := range m.encodings {
		if match(key, e) {
			stopping = append(stopping, e)
			delete(m.encodings, key)
		}
	}
	m.mu.Unlock()
	for _, e := range stopping {
		e.stop()
	}
}

func (m *Manager) stopIdle() {
	ticker := time.NewTicker(idleTimeout / 6)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.stopWhere(func(_ Key, e *encoding) bool { return e.idle() })
		}
	}
}

// Init opens the initialization segment of a fragmented MP4 encoding.
func (m *Manager) Init(ctx context.Context, key Key, open Opener) (*os.File, error) {
	e, err := m.encoding(ctx, key, open)
	if err != nil {
		return nil, err
	}
	if e.remux.Format != FMP4 {
		return nil, ErrNotFound
	}
	return e.await(ctx, -1)
}

// Segment opens segment n of an encoding, starting or moving FFmpeg to it
// when needed.
func (m *Manager) Segment(ctx context.Context, key Key, open Opener, n int) (*os.File, error) {
	e, err := m.encoding(ctx, key, open)
	if err != nil {
		return nil, err
	}
	if n < 0 || n >= e.remux.Plan.Len() {
		return nil, ErrNotFound
	}
	return e.await(ctx, n)
}

// encoding returns the encoding of key, opening it on first use.
func (m *Manager) encoding(ctx context.Context, key Key, open Opener) (*encoding, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrStopped
	}
	e := m.encodings[key]
	if e == nil {
		e = &encoding{m: m, key: key, dir: filepath.Join(m.dir, key.name()), opened: make(chan struct{}),
			changed: make(chan struct{}), used: time.Now()}
		m.encodings[key] = e
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
		if m.encodings[key] == e {
			delete(m.encodings, key)
		}
		m.mu.Unlock()
		return nil, e.err
	}
	return e, nil
}

// encoding is one remux of a version for a play session.
type encoding struct {
	m   *Manager
	key Key
	dir string
	// opened is closed once remux, release and err are set.
	opened  chan struct{}
	remux   Remux
	release func()
	err     error

	mu sync.Mutex
	// changed is closed, then replaced, whenever a segment is ready, a job
	// ends, or a player asks for another segment.
	changed   chan struct{}
	ready     []bool
	init      bool
	job       *job
	requested int
	used      time.Time
	stopped   bool
}

func (e *encoding) open(ctx context.Context, open Opener) {
	defer close(e.opened)
	remux, release, err := open(context.WithoutCancel(ctx))
	if err != nil {
		e.err = err
		return
	}
	if err := os.MkdirAll(e.dir, 0o700); err != nil {
		release()
		e.err = err
		return
	}
	e.remux, e.release = remux, release
	e.ready = make([]bool, remux.Plan.Len())
}

// signal wakes whoever waits on the encoding. The caller holds e.mu.
func (e *encoding) signal() {
	close(e.changed)
	e.changed = make(chan struct{})
}

// wait waits for a change, holding e.mu again when it returns.
func (e *encoding) wait(ctx context.Context) error {
	changed := e.changed
	e.mu.Unlock()
	defer e.mu.Lock()
	select {
	case <-changed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *encoding) idle() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return time.Since(e.used) > idleTimeout
}

func (e *encoding) path(n int) string {
	if n < 0 {
		return filepath.Join(e.dir, "init.mp4")
	}
	return filepath.Join(e.dir, strconv.Itoa(n)+"."+e.remux.Format.Extension())
}

// await opens segment n, or the initialization segment for n = -1, once
// it is made.
func (e *encoding) await(ctx context.Context, n int) (*os.File, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.used = time.Now()
	if n >= 0 {
		e.requested = n
		e.prune(n)
		e.signal()
	}
	for {
		if e.stopped {
			return nil, ErrStopped
		}
		if (n < 0 && e.init) || (n >= 0 && e.ready[n]) {
			return os.Open(e.path(n))
		}
		j := e.job
		start := n
		if n < 0 {
			// The initialization segment comes with any segment: start
			// where the player last asked, if it did.
			start = e.requested
		}
		switch {
		case j != nil && j.done && j.err != nil && !errors.Is(j.err, errStale) && j.start == start:
			// Reported once: the next request tries again.
			e.job = nil
			return nil, j.err
		case j != nil && j.done && j.err == nil && n >= j.start && n >= j.next:
			// FFmpeg reached the end of the version before this segment.
			return nil, ErrNotFound
		case j == nil || j.done || (n >= 0 && (n < j.next || n > j.next+reach)):
			e.start(start)
		}
		if err := e.wait(ctx); err != nil {
			return nil, err
		}
	}
}

// prune removes the segments far from segment n. The caller holds e.mu.
func (e *encoding) prune(n int) {
	for i, ready := range e.ready {
		if ready && (i < n-behind || i > n+ahead+reach) {
			e.ready[i] = false
			if err := os.Remove(e.path(i)); err != nil && !errors.Is(err, os.ErrNotExist) {
				e.m.logger.Warn("A segment could not be removed", "error", err)
			}
		}
	}
}

// stop ends the encoding and removes its segments.
func (e *encoding) stop() {
	<-e.opened
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return
	}
	e.stopped = true
	if e.job != nil {
		e.job.cancel()
	}
	e.signal()
	e.mu.Unlock()
	if e.err != nil {
		return
	}
	e.release()
	if err := os.RemoveAll(e.dir); err != nil {
		e.m.logger.Warn("The segments of an encoding could not be removed", "error", err)
	}
}

// job is one run of FFmpeg, from segment start onwards.
type job struct {
	start int
	// next is the segment being made.
	next   int
	cancel context.CancelFunc
	done   bool
	err    error
	// file receives the segment being made, once its first keyframe came.
	file *os.File
}

// start replaces the running job with one starting at segment n. The
// caller holds e.mu.
func (e *encoding) start(n int) {
	if e.job != nil {
		e.job.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{start: n, next: n, cancel: cancel}
	e.job = j
	go e.run(ctx, j)
}

// run runs FFmpeg for a job and files what it writes into segments.
func (e *encoding) run(ctx context.Context, j *job) {
	cmd := exec.CommandContext(ctx, e.m.ffmpeg, e.remux.args(j.start)...)
	cmd.WaitDelay = 5 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	output, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		e.finish(j, fmt.Errorf("start FFmpeg: %w", err))
		return
	}
	started := time.Now()
	e.m.logger.Debug("Started a remux", "from", j.start)
	take := func(p piece) error { return e.take(ctx, j, p) }
	if e.remux.Format == TS {
		err = splitTS(output, take)
	} else {
		err = splitMP4(output, take)
	}
	// A job replaced or stopped from outside is stale; one whose output
	// could not be filed failed, and stops FFmpeg itself.
	replaced := errors.Is(err, errStale) || ctx.Err() != nil
	if err != nil && !replaced {
		j.cancel()
	}
	// FFmpeg is waited for once its output is drained or abandoned.
	_, _ = io.Copy(io.Discard, output)
	waitErr := cmd.Wait()
	switch {
	case replaced:
		err = errStale
	case err == nil && waitErr != nil:
		err = fmt.Errorf("FFmpeg failed: %w: %s", waitErr, bytes.TrimSpace(stderr.bytes()))
	}
	switch {
	case errors.Is(err, errStale):
		e.m.logger.Debug("A remux was stopped", "from", j.start, "duration", time.Since(started))
	case err != nil:
		e.m.logger.Warn("A remux failed", "from", j.start, "error", err)
	default:
		e.m.logger.Debug("A remux reached the end", "from", j.start, "duration", time.Since(started))
	}
	e.finish(j, err)
}

// take files a piece of FFmpeg's output.
func (e *encoding) take(ctx context.Context, j *job, p piece) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.job != j || e.stopped {
		return errStale
	}
	if p.init {
		if e.init {
			return nil
		}
		if err := writeFile(e.path(-1), p.data); err != nil {
			return err
		}
		e.init = true
		e.signal()
		return nil
	}
	plan := e.remux.Plan
	if j.file == nil {
		// FFmpeg may start on a keyframe before the segment: what comes
		// before it is not part of the job.
		if !p.keyframe || p.at < plan.Start(j.next)-tolerance {
			return nil
		}
		return e.begin(j, p.data)
	}
	if p.keyframe && j.next+1 < plan.Len() && p.at >= plan.Start(j.next+1)-tolerance {
		if err := e.close(j); err != nil {
			return err
		}
		// Keep at most ahead segments past the last one asked for.
		for j.next > e.requested+ahead {
			if err := e.wait(ctx); err != nil {
				return errStale
			}
			if e.job != j || e.stopped {
				return errStale
			}
		}
		return e.begin(j, p.data)
	}
	_, err := j.file.Write(p.data)
	return err
}

// begin starts writing segment j.next. The caller holds e.mu.
func (e *encoding) begin(j *job, data []byte) error {
	file, err := os.CreateTemp(e.dir, "partial-*")
	if err != nil {
		return err
	}
	j.file = file
	_, err = file.Write(data)
	return err
}

// close completes segment j.next and moves the job to the next. The caller
// holds e.mu.
func (e *encoding) close(j *job) error {
	file := j.file
	if file == nil {
		return nil
	}
	j.file = nil
	err := file.Close()
	if err == nil {
		err = os.Rename(file.Name(), e.path(j.next))
	}
	if err != nil {
		_ = os.Remove(file.Name())
		return err
	}
	e.ready[j.next] = true
	j.next++
	e.signal()
	return nil
}

// finish records the end of a job; one that reached the end of its output
// completes its last segment.
func (e *encoding) finish(j *job, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err == nil && e.job == j && !e.stopped {
		err = e.close(j)
	}
	if j.file != nil {
		_ = j.file.Close()
		_ = os.Remove(j.file.Name())
		j.file = nil
	}
	j.done, j.err = true, err
	if e.job == j {
		e.signal()
	}
}

func writeFile(path string, data []byte) error {
	partial := path + ".partial"
	if err := os.WriteFile(partial, data, 0o600); err != nil {
		return err
	}
	return os.Rename(partial, path)
}

// args is FFmpeg's command line for a job starting at segment n. Segments
// keep the source's timestamps, shifted by timestampOffset, so that those
// of different jobs follow each other.
func (r Remux) args(n int) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	if n > 0 {
		args = append(args, "-ss", strconv.FormatFloat(r.Plan.seekTime(n).Seconds(), 'f', 6, 64))
	}
	args = append(args, "-copyts", "-i", r.Input, "-map", "0:"+strconv.Itoa(r.Video))
	if r.Audio >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(r.Audio))
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy")
	if r.VideoTag != "" {
		args = append(args, "-tag:v", r.VideoTag)
	}
	if r.Audio >= 0 {
		// Audio before zero, such as encoder priming, is not played.
		args = append(args, "-bsf:a", `noise=drop=lt(pts\,0)`)
	}
	args = append(args, "-avoid_negative_ts", "disabled", "-output_ts_offset", strconv.FormatFloat(timestampOffset.Seconds(), 'f', -1, 64))
	if r.Format == TS {
		return append(args, "-muxdelay", "0", "-muxpreload", "0", "-f", "mpegts", "pipe:1")
	}
	// A fragment per keyframe, with the real decode times: without an edit
	// list, presentation times are those of the source, for video as for
	// audio. The movie header waits for the first fragment, as some codecs,
	// such as E-AC-3, describe themselves in their first packet.
	return append(args, "-use_editlist", "0", "-f", "mp4", "-movflags", "+frag_keyframe+empty_moov+delay_moov+default_base_moof+frag_discont", "pipe:1")
}

// tail keeps the end of what a process writes.
type tail struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, p...)
	if extra := len(t.data) - t.limit; extra > 0 {
		t.data = t.data[extra:]
	}
	return len(p), nil
}

func (t *tail) bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return bytes.Clone(t.data)
}

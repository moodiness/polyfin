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
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/subtitles"
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
	// settle is how long after FFmpeg wrote a part of the video the
	// subtitle cues shown there count as extracted. FFmpeg converts them in
	// threads of its own, which may lag behind the video it copies, most
	// when they start; nothing tells a cue still on its way from no cue.
	settle = 500 * time.Millisecond
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
	// InputOptions are FFmpeg's options for reading a live Input, such as
	// those of its HLS demuxer.
	InputOptions []string
	// Video and Audio are FFmpeg stream indexes; Audio is -1 for none.
	Video, Audio int
	// ADTS marks live AAC audio, framed as MPEG-TS carries it, which MP4
	// stores otherwise when it is copied.
	ADTS bool
	// AudioCodec is the encoder the audio is converted with, empty to copy
	// it; AudioChannels and AudioBitrate, what it is converted to, the
	// bitrate zero for lossless codecs.
	AudioCodec    string
	AudioChannels int
	AudioBitrate  int64
	// VideoTag is the sample entry of the video in MP4, such as hvc1 for
	// HEVC, which Apple players require; empty keeps FFmpeg's.
	VideoTag string
	// Encode converts the video; nil copies it.
	Encode *VideoEncoding
	Format Format
	Plan   Plan
	// Subtitles are the FFmpeg indexes of the text subtitle streams
	// extracted into Extracted while remuxing, from the bytes FFmpeg reads
	// anyway.
	Subtitles []int
	Extracted Extracted
}

// VideoEncoding is what a job converts the video to: 8-bit, progressive,
// with a keyframe at the start of every segment, so that the segments of
// a conversion follow those of a remux of the same version.
type VideoEncoding struct {
	// Encoder is libx264, libx265 or one of Hardware's; Level, the codec
	// level it declares.
	Encoder, Level string
	Width, Height  int
	// Bitrate is the average the encoder aims for, in bits per second.
	Bitrate int64
	// FrameRate is the source's, frames a second.
	FrameRate float64
	// ToneMap converts HDR to SDR; Deinterlace, interlaced video to
	// progressive.
	ToneMap, Deinterlace bool
	// Burn is the FFmpeg index of an image subtitle stream burned into the
	// video, nil for none.
	Burn *int
	// Hardware is the GPU decoding and encoding the video, nil for none.
	Hardware *Hardware
}

// filters is the filter chain of the video: 8-bit, at the size asked, in
// SDR, as the encoder takes it.
func (v *VideoEncoding) filters() string {
	return v.convert() + "," + v.Hardware.output()
}

// convert is the filter chain bringing the video to the size asked, in
// SDR, before the pixel format the encoder takes. On a GPU that tone maps,
// libplacebo scales and tone maps in one pass, applying the Dolby Vision
// metadata FFmpeg's decoder exports, and with the BT.2390 curve, which keeps
// midtones brighter than the processor's Hable.
func (v *VideoEncoding) convert() string {
	var filters []string
	if v.Deinterlace {
		filters = append(filters, "yadif")
	}
	size := "w=" + strconv.Itoa(v.Width) + ":h=" + strconv.Itoa(v.Height)
	switch {
	case v.toneMapsOnGPU():
		filters = append(filters, "libplacebo="+size+":format=yuv420p:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv:tonemapping=bt.2390")
	case v.ToneMap:
		// To linear light in floating point, to BT.709 primaries, tone
		// mapped, then to the BT.709 transfer and matrix in limited range.
		filters = append(filters, "scale="+size, "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=tv")
	default:
		filters = append(filters, "scale="+size)
	}
	return strings.Join(filters, ",")
}

// burnGraph is the filter graph burning subtitle stream burn into video
// stream video, as output [video]. The subtitle's canvas, the size of the
// video it was made for, is scaled to the width of the converted video
// keeping its shape, and laid at the bottom: a video cropped since keeps
// the subtitles near its bottom edge. It is laid after the conversion to
// SDR, which would dim its colors, and in memory, before a GPU encoder
// takes the frames.
//
// FFmpeg repeats the canvas for every packet read from the source, which
// is over a thousand a second with TrueHD audio: the canvas is first
// brought to the video's frame rate, or scaling the repeats makes the
// conversion several times slower.
func (v *VideoEncoding) burnGraph(video, burn int) string {
	return "[0:" + strconv.Itoa(video) + "]" + v.convert() + ",format=yuv420p[converted];" +
		"[0:" + strconv.Itoa(burn) + "]fps=" + strconv.FormatFloat(v.FrameRate, 'f', -1, 64) + ",scale=" + strconv.Itoa(v.Width) + ":-2[subtitle];" +
		"[converted][subtitle]overlay=x=0:y=main_h-overlay_h:eof_action=pass," + v.Hardware.output() + "[video]"
}

// args are FFmpeg's encoder options for a job starting at segment n.
func (v *VideoEncoding) args(plan Plan, n int) []string {
	// The encoder compares times in its frame rate's time base, where a
	// keyframe's exact time can fall just before the time asked: each is
	// asked a fraction of a frame early.
	rate := v.FrameRate
	if rate <= 0 {
		rate = 24
	}
	lead := 0.4 / rate
	times := make([]string, 0, plan.Len()-n)
	for k := n; k < plan.Len(); k++ {
		times = append(times, strconv.FormatFloat(max(plan.Start(k).Seconds()-lead, 0), 'f', 4, 64))
	}
	return v.encoderArgs(strings.Join(times, ","))
}

// encoderArgs are FFmpeg's encoder options, with keyframes forced as
// -force_key_frames takes them.
func (v *VideoEncoding) encoderArgs(keyframes string) []string {
	args := []string{"-c:v", v.Encoder,
		"-b:v", strconv.FormatInt(v.Bitrate, 10), "-maxrate", strconv.FormatInt(v.Bitrate*3/2, 10), "-bufsize", strconv.FormatInt(v.Bitrate*2, 10),
		"-force_key_frames", keyframes}
	if v.Burn == nil {
		args = append(args, "-vf", v.filters())
	}
	profile := "main"
	if strings.HasPrefix(v.Encoder, "h264") {
		profile = "high"
	}
	switch v.Encoder {
	case "libx264":
		args = append(args, "-preset", "veryfast", "-sc_threshold", "0", "-profile:v", profile, "-level:v", v.Level)
	case "libx265":
		args = append(args, "-preset", "veryfast", "-sc_threshold", "0", "-profile:v", profile, "-x265-params", "log-level=error:level-idc="+v.Level)
	case "h264_nvenc", "hevc_nvenc":
		// A forced keyframe is an IDR frame, as a segment's first must be.
		args = append(args, "-preset", "p4", "-rc", "vbr", "-forced-idr", "1", "-profile:v", profile, "-level:v", v.Level)
	default:
		args = append(args, "-rc_mode", "VBR", "-profile:v", profile, "-level:v", v.Level)
	}
	return args
}

// Opener prepares an encoding: what it remuxes, and a function releasing
// its input once the encoding stops.
type Opener func(ctx context.Context) (Remux, func(), error)

// Key identifies an encoding: what a play session plays.
type Key struct {
	Session string
	Audio   int
	Format  Format
	// User is who plays, which bounds the live encodings each user runs.
	User string
	// Version is the version played, and Converts marks an encoding that
	// converts the video: the playbacks doing so may be bounded, a playback
	// being what a user plays of a version (see LimitConversions).
	Version  string
	Converts bool
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
	can    capabilities
	// hardware is the GPU DetectHardware chose, set before encoding starts.
	hardware *Hardware
	// conversions returns how many playbacks may have their video
	// converted at once, 0 or less for no limit; nil sets no limit.
	conversions func() int

	mu        sync.Mutex
	encodings map[Key]*encoding
	lives     map[Key]*live
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
	m := &Manager{ffmpeg: ffmpegPath, dir: dir, logger: logger, done: make(chan struct{}), can: probe(ffmpegPath),
		encodings: map[Key]*encoding{}, lives: map[Key]*live{}}
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
	m.stopLives(func(Key, *live) bool { return true })
}

// Stop stops the encodings of a play session.
func (m *Manager) Stop(session string) {
	m.stopWhere(func(key Key, _ *encoding) bool { return key.Session == session })
	m.stopLives(func(key Key, _ *live) bool { return key.Session == session })
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
			m.stopLives(func(_ Key, l *live) bool { return l.idle() })
		}
	}
}

// LimitConversions bounds the playbacks whose video is converted at once to
// what limit returns, 0 for no limit, files and live alike. It is read
// whenever an encoding would convert the video of one more, so a change
// applies at once; encodings already running go on.
func (m *Manager) LimitConversions(limit func() int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversions = limit
}

// MayConvert reports whether an encoding converting the video of version
// for user may start now: see admits.
func (m *Manager) MayConvert(user, version string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.admits(Key{User: user, Version: version, Converts: true}, nil)
}

// admits reports whether an encoding of key may start: it copies the
// video, or the playbacks whose video is converted are fewer than the
// limit. A playback is what a user plays of a version, whatever the play
// session: apps start a new one to switch audio track or quality, and stop
// the old one once the new one plays. The playback key belongs to, which
// counts already, and the live encoding leaving, about to be replaced, are
// left out. The caller holds m.mu.
func (m *Manager) admits(key Key, leaving *live) bool {
	if !key.Converts || m.conversions == nil {
		return true
	}
	limit := m.conversions()
	if limit <= 0 {
		return true
	}
	type playback struct{ user, version string }
	converting := map[playback]bool{}
	count := func(k Key) {
		if k.Converts && (k.User != key.User || k.Version != key.Version) {
			converting[playback{k.User, k.Version}] = true
		}
	}
	for k := range m.encodings {
		count(k)
	}
	for k, l := range m.lives {
		if l != leaving {
			count(k)
		}
	}
	return len(converting) < limit
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

// Subtitles waits until the extracted subtitles of an encoding cover
// segment n, starting or moving FFmpeg to it when needed.
func (m *Manager) Subtitles(ctx context.Context, key Key, open Opener, n int) error {
	e, err := m.encoding(ctx, key, open)
	if err != nil {
		return err
	}
	if n < 0 || n >= e.remux.Plan.Len() {
		return ErrNotFound
	}
	if len(e.remux.Subtitles) == 0 {
		return nil
	}
	return e.awaitCovered(ctx, n)
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
		if !m.admits(key, nil) {
			m.mu.Unlock()
			return nil, ErrBusy
		}
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

// awaitCovered waits until the extracted subtitles cover segment n. The
// job making it, or about to, is waited for; another starts from n.
func (e *encoding) awaitCovered(ctx context.Context, n int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.used = time.Now()
	plan := e.remux.Plan
	for {
		if e.stopped {
			return ErrStopped
		}
		if e.remux.Extracted.Covers(plan.Start(n), plan.End(n)) {
			return nil
		}
		j := e.job
		switch {
		case j != nil && j.done && j.err != nil && !errors.Is(j.err, errStale) && j.start == n:
			e.job = nil
			return j.err
		case j == nil || j.done || n < j.start || n > j.next+reach:
			e.start(n)
		}
		// A player may fetch subtitles ahead of the video: FFmpeg goes on
		// until it reaches them.
		if n > e.requested {
			e.requested = n
			e.signal()
		}
		if err := e.wait(ctx); err != nil {
			return err
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

// run runs FFmpeg for a job and files what it writes into segments, and
// the cues of the subtitles it extracts.
func (e *encoding) run(ctx context.Context, j *job) {
	cmd := exec.CommandContext(ctx, e.m.ffmpeg, e.remux.args(j.start)...)
	cmd.WaitDelay = 5 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	output, err := cmd.StdoutPipe()
	// Each extracted subtitle stream comes on a pipe of its own, from file
	// descriptor 3 on.
	var readers []*os.File
	for range e.remux.Subtitles {
		if err != nil {
			break
		}
		var r, w *os.File
		if r, w, err = os.Pipe(); err == nil {
			readers = append(readers, r)
			cmd.ExtraFiles = append(cmd.ExtraFiles, w)
		}
	}
	if err == nil {
		err = cmd.Start()
	}
	for _, w := range cmd.ExtraFiles {
		_ = w.Close()
	}
	if err != nil {
		for _, r := range readers {
			_ = r.Close()
		}
		e.finish(j, fmt.Errorf("start FFmpeg: %w", err))
		return
	}
	var extracting sync.WaitGroup
	for i, r := range readers {
		stream := e.remux.Subtitles[i]
		extracting.Go(func() {
			defer r.Close()
			if err := subtitles.Scan(r, func(cue subtitles.Cue) { e.remux.Extracted.Add(stream, cue) }); err != nil {
				e.m.logger.Warn("A subtitle could not be extracted", "stream", stream, "error", err)
				// FFmpeg would wait for the pipe to be read.
				_, _ = io.Copy(io.Discard, r)
			}
		})
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
	// The cues FFmpeg wrote before it ended are added before the job
	// finishes and covers them.
	extracting.Wait()
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
	// The cues starting before the segment just made are counted as
	// extracted once they had time to come, leaving that segment's length
	// as a margin too.
	if j.next > j.start {
		from, to := e.remux.Plan.Start(j.start), e.remux.Plan.Start(j.next)
		time.AfterFunc(settle, func() {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.cover(from, to)
			e.signal()
		})
	}
	j.next++
	e.signal()
	return nil
}

// cover records that the subtitles a job extracts cover [from, to).
func (e *encoding) cover(from, to time.Duration) {
	if len(e.remux.Subtitles) > 0 {
		e.remux.Extracted.Cover(from, to)
	}
}

// finish records the end of a job; one that reached the end of its output
// completes its last segment, and has extracted every cue from its start.
func (e *encoding) finish(j *job, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err == nil && e.job == j && !e.stopped {
		if err = e.close(j); err == nil {
			plan := e.remux.Plan
			e.cover(plan.Start(j.start), plan.End(plan.Len()-1))
		}
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
// of different jobs follow each other. Extracted subtitles keep the
// source's own.
func (r Remux) args(n int) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	if n > 0 {
		// Converted audio starts where the demuxer does, on the keyframe,
		// as copied streams do, instead of at the time asked.
		args = append(args, "-noaccurate_seek", "-ss", strconv.FormatFloat(r.Plan.seekTime(n).Seconds(), 'f', 6, 64))
	}
	if r.Encode != nil {
		args = append(args, r.Encode.inputs()...)
	}
	args = append(args, "-copyts", "-i", r.Input)
	video := "0:" + strconv.Itoa(r.Video)
	if r.Encode != nil && r.Encode.Burn != nil {
		args = append(args, "-filter_complex", r.Encode.burnGraph(r.Video, *r.Encode.Burn))
		video = "[video]"
	}
	args = append(args, "-map", video)
	if r.Audio >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(r.Audio))
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy")
	if r.Encode != nil {
		args = append(args, r.Encode.args(r.Plan, n)...)
	}
	if r.VideoTag != "" {
		args = append(args, "-tag:v", r.VideoTag)
	}
	if r.Audio >= 0 && r.AudioCodec != "" {
		args = append(args, "-c:a", r.AudioCodec, "-ac", strconv.Itoa(r.AudioChannels))
		if r.AudioBitrate > 0 {
			args = append(args, "-b:a", strconv.FormatInt(r.AudioBitrate, 10))
		}
	}
	if r.Audio >= 0 {
		// Audio before zero, such as encoder priming, is not played.
		args = append(args, "-bsf:a", `noise=drop=lt(pts\,0)`)
	}
	args = append(args, "-avoid_negative_ts", "disabled", "-output_ts_offset", strconv.FormatFloat(timestampOffset.Seconds(), 'f', -1, 64))
	if r.Format == TS {
		args = append(args, "-muxdelay", "0", "-muxpreload", "0", "-f", "mpegts", "pipe:1")
	} else {
		// A fragment per keyframe, with the real decode times: without an
		// edit list, presentation times are those of the source, for video
		// as for audio. The movie header waits for the first fragment, as
		// some codecs, such as E-AC-3, describe themselves in their first
		// packet.
		args = append(args, "-use_editlist", "0", "-f", "mp4", "-movflags", "+frag_keyframe+empty_moov+delay_moov+default_base_moof+frag_discont", "pipe:1")
	}
	// Each subtitle stream is written as WebVTT, cue by cue, on its pipe.
	for i, stream := range r.Subtitles {
		args = append(args, "-map", "0:"+strconv.Itoa(stream), "-c:s", "webvtt", "-flush_packets", "1", "-f", "webvtt", "pipe:"+strconv.Itoa(3+i))
	}
	return args
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

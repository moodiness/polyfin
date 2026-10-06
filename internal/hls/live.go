package hls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// liveSegment is the length FFmpeg cuts live streams into, in seconds,
	// as Jellyfin does for live TV; copied video is cut on its keyframes.
	liveSegment = 3
	// liveWindow is how many segments the live playlist lists. Older ones
	// are deleted: a channel watched for hours keeps a minute of segments.
	liveWindow = 10
	// liveIdle stops a live encoding no player asked anything of for that
	// long. Players reload a live playlist every few seconds while they
	// play, so a player that went quiet has left.
	liveIdle = time.Minute
	// liveInitTime is the length of the first segments, in seconds, so
	// that a player starts a second after FFmpeg does.
	liveInitTime = 1
	// A run that ends, cleanly or not, is started again: after 1, 2, then
	// 4 seconds when the runs before it ended within liveQuickRun of their
	// start, at most liveRestarts times within liveRestartWindow, past
	// which the encoding gives up until a player asks again after it.
	liveQuickRun      = 10 * time.Second
	liveRestarts      = 5
	liveRestartWindow = 2 * time.Minute
	// liveWait bounds how long a request waits for the first segment.
	liveWait     = time.Minute
	livePlaylist = "live.m3u8"
	liveInit     = "init.mp4"
	// userLives is how many channels one user plays at once through
	// FFmpeg, as many as a household's screens; serverLives, how many the
	// server runs in all.
	userLives   = 4
	serverLives = 16
)

// ErrInterrupted reports a live stream whose input ended: a live stream
// has no end, so it is read again.
var ErrInterrupted = errors.New("the live stream was interrupted")

// ErrBusy reports an encoding refused because the server runs as many as
// it may: live encodings, or playbacks whose video is converted.
var ErrBusy = errors.New("too many encodings")

// liveFile matches the files of a live encoding a player may fetch.
var liveFile = regexp.MustCompile(`^(\d+\.(ts|mp4)|init(-\d+)?\.mp4)$`)

// live is a live stream that FFmpeg converts into HLS segments as it
// comes, for a play session. Unlike a remux, it has no plan: FFmpeg's own
// HLS muxer cuts the stream, and its playlist is served as it goes.
type live struct {
	m   *Manager
	key Key
	dir string

	mu      sync.Mutex
	opened  bool
	remux   Remux
	release func()
	// run is the current or last run of FFmpeg, nil before the first;
	// restarts are when runs started again lately, quick how many runs in
	// a row ended within liveQuickRun.
	run      *liveRun
	restarts []time.Time
	quick    int
	next     int
	used     time.Time
	// created orders a user's live encodings, the oldest replaced first.
	created time.Time
	// stopped is set once the encoding is stopped for good.
	stopped bool
}

// liveRun is one run of FFmpeg.
type liveRun struct {
	started, ended time.Time
	cancel         context.CancelFunc
	// done is closed when FFmpeg exits, err set before.
	done chan struct{}
	err  error
}

// LivePlaylist returns the media playlist of the live encoding of key,
// starting FFmpeg when it does not run, once the playlist lists a segment.
// uri names each file the playlist refers to.
func (m *Manager) LivePlaylist(ctx context.Context, key Key, open Opener, uri func(name string) string) ([]byte, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrStopped
	}
	l := m.lives[key]
	var replaced *live
	if l == nil {
		// Each live encoding reads its source as it comes, until the app
		// leaves: a user's newest replaces their oldest past userLives, and
		// none starts past serverLives. Recordings count too, but are never
		// replaced: past userLives with recordings only, none starts.
		mine, oldest := m.userLives(key.User)
		switch {
		case mine >= userLives && oldest == nil:
			m.mu.Unlock()
			return nil, ErrBusy
		case mine >= userLives:
			replaced = oldest
		case len(m.lives)+len(m.recordings) >= serverLives:
			m.mu.Unlock()
			return nil, ErrBusy
		}
		// Nor does one converting the video of a playback past the limit.
		if !m.admits(key, replaced) {
			m.mu.Unlock()
			return nil, ErrBusy
		}
		if replaced != nil {
			delete(m.lives, replaced.key)
		}
		now := time.Now()
		l = &live{m: m, key: key, dir: filepath.Join(m.dir, "live-"+key.name()), used: now, created: now}
		m.lives[key] = l
	}
	m.mu.Unlock()
	if replaced != nil {
		replaced.stop()
	}
	ctx, cancel := context.WithTimeout(ctx, liveWait)
	defer cancel()
	for {
		// A run that ended is started again (see ensure): the playlist
		// keeps listing the last segments meanwhile.
		run, err := l.ensure(ctx, open)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(l.dir, livePlaylist))
		if err == nil && bytes.Contains(data, []byte("#EXTINF")) {
			return rewriteLive(data, uri), nil
		}
		select {
		case <-run.done:
			// A stream that never played fails at once: the next request
			// starts it again.
			if run.err != nil && !errors.Is(run.err, ErrInterrupted) && !l.played() {
				return nil, run.err
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// LiveSegment opens a file of the live encoding of key: a segment its
// playlist lists, or the initialization segment of fragmented MP4.
func (m *Manager) LiveSegment(key Key, name string) (*os.File, error) {
	m.mu.Lock()
	l := m.lives[key]
	m.mu.Unlock()
	if l == nil || !liveFile.MatchString(name) {
		return nil, ErrNotFound
	}
	l.mu.Lock()
	l.used = time.Now()
	stopped := l.stopped
	l.mu.Unlock()
	if stopped {
		return nil, ErrStopped
	}
	file, err := os.Open(filepath.Join(l.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return file, err
}

// ensure opens the encoding's input on first use and starts FFmpeg unless
// it runs. A run that ended, as a live stream should not, is started again
// once its backoff passed (see liveQuickRun): it returns the ended run
// until then. Past liveRestarts within liveRestartWindow, it fails with
// the last run's error, and so does the next request until the window
// moved on. After a run that failed quickly the input is opened again,
// which analyzes the stream again.
func (l *live) ensure(ctx context.Context, open Opener) (*liveRun, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.used = time.Now()
	if l.stopped {
		return nil, ErrStopped
	}
	reopen := false
	run := l.run
	if run != nil {
		select {
		case <-run.done:
		default:
			return run, nil
		}
		ended := time.Now()
		if run.ended.Sub(run.started) < liveQuickRun {
			if wait := liveBackoff(l.quick); ended.Before(run.ended.Add(wait)) {
				return run, nil
			}
			reopen = run.err != nil && !errors.Is(run.err, ErrInterrupted)
		}
		l.restarts = slices.DeleteFunc(l.restarts, func(at time.Time) bool { return ended.Sub(at) > liveRestartWindow })
		if len(l.restarts) >= liveRestarts {
			return nil, fmt.Errorf("%w after %d restarts: %w", ErrStopped, len(l.restarts), run.err)
		}
		l.restarts = append(l.restarts, ended)
		l.m.logger.Info("A live stream is read again", "reason", run.err, "restarts", len(l.restarts))
	}
	if reopen && l.opened {
		l.release()
		l.opened = false
	}
	if !l.opened {
		remux, release, err := open(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}
		l.remux, l.release, l.opened = remux, release, true
	}
	return l.start()
}

// liveBackoff is how long after a run that ended quickly, the quick-th in
// a row, the next one starts: 1, 2, then 4 seconds.
func liveBackoff(quick int) time.Duration {
	return time.Second << min(max(quick-1, 0), 2)
}

// start runs FFmpeg into the encoding's directory, emptied on the first
// run only, numbering segments on from the last run's, so that a player
// that reloads the playlist does not see the stream go back; a run after
// the first starts its playlist with a discontinuity, and writes its own
// initialization segment. The caller holds l.mu.
func (l *live) start() (*liveRun, error) {
	if l.run == nil {
		if err := os.RemoveAll(l.dir); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	previous := l.run
	run := &liveRun{started: time.Now(), cancel: cancel, done: make(chan struct{})}
	l.run = run
	args := l.remux.runArgs(l.dir, l.next, previous != nil)
	go func() {
		defer cancel()
		cmd := exec.CommandContext(ctx, l.m.ffmpeg, args...)
		cmd.WaitDelay = 5 * time.Second
		stderr := &tail{limit: 4096}
		cmd.Stderr = stderr
		l.m.logger.Debug("Started a live encoding", "from", l.next)
		err := cmd.Run()
		switch {
		case ctx.Err() != nil:
			err = ErrStopped
			l.m.logger.Debug("A live encoding was stopped")
		case err != nil:
			err = fmt.Errorf("FFmpeg failed: %w: %s", err, bytes.TrimSpace(stderr.bytes()))
			l.m.logger.Warn("A live encoding failed", "error", err)
		default:
			// A live stream does not end: its input was interrupted.
			err = ErrInterrupted
			l.m.logger.Debug("A live stream was interrupted")
		}
		l.mu.Lock()
		l.next = max(l.next, lastSegment(l.dir)+1)
		run.ended = time.Now()
		if run.ended.Sub(run.started) < liveQuickRun {
			l.quick++
		} else {
			l.quick = 0
		}
		run.err = err
		close(run.done)
		l.mu.Unlock()
	}()
	return run, nil
}

// lastSegment is the number of the last segment in a directory, -1 for
// none.
func lastSegment(dir string) int {
	last := -1
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		name, _, _ := strings.Cut(entry.Name(), ".")
		if n, err := strconv.Atoi(name); err == nil {
			last = max(last, n)
		}
	}
	return last
}

// played reports whether a run wrote a segment.
func (l *live) played() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.next > 0 || lastSegment(l.dir) >= 0
}

func (l *live) idle() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return time.Since(l.used) > liveIdle
}

// stop ends the encoding and removes its segments.
func (l *live) stop() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.stopped = true
	run := l.run
	l.mu.Unlock()
	if run != nil {
		run.cancel()
		<-run.done
	}
	if l.release != nil {
		l.release()
	}
	if err := os.RemoveAll(l.dir); err != nil {
		l.m.logger.Warn("The segments of a live encoding could not be removed", "error", err)
	}
}

func (m *Manager) stopLives(match func(Key, *live) bool) {
	m.mu.Lock()
	var stopping []*live
	for key, l := range m.lives {
		if match(key, l) {
			stopping = append(stopping, l)
			delete(m.lives, key)
		}
	}
	m.mu.Unlock()
	for _, l := range stopping {
		l.stop()
	}
}

// liveMap is the initialization segment an FFmpeg playlist names.
var liveMap = regexp.MustCompile(`URI="init(-\d+)?\.mp4"`)

// rewriteLive names the files FFmpeg's playlist refers to by uri.
func rewriteLive(playlist []byte, uri func(name string) string) []byte {
	var out bytes.Buffer
	for line := range strings.Lines(string(playlist)) {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			line = liveMap.ReplaceAllStringFunc(line, func(attribute string) string {
				return `URI="` + uri(attribute[len(`URI="`):len(attribute)-1]) + `"`
			})
		case !strings.HasPrefix(line, "#"):
			line = uri(line)
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// liveArgs is FFmpeg's command line for the first run of a live encoding
// writing into dir, its first segment numbered start (see runArgs).
func (r Remux) liveArgs(dir string, start int) []string {
	return r.runArgs(dir, start, false)
}

// runArgs is FFmpeg's command line for a run of a live encoding writing
// into dir, its first segment numbered start; restart marks a run after
// the first. The playlist never ends: a live stream that does is read
// again.
func (r Remux) runArgs(dir string, start int, restart bool) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	if r.Encode != nil {
		args = append(args, r.Encode.inputs()...)
	}
	args = append(args, r.InputOptions...)
	args = append(args, "-i", r.Input, "-map", "0:"+strconv.Itoa(r.Video))
	if r.Audio >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(r.Audio))
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy")
	if r.Encode != nil {
		// A keyframe starts every segment of converted video.
		args = append(args, r.Encode.encoderArgs("expr:gte(t,n_forced*"+strconv.Itoa(liveSegment)+")")...)
	}
	if r.VideoTag != "" {
		args = append(args, "-tag:v", r.VideoTag)
	}
	if r.Audio >= 0 && r.AudioCodec != "" {
		args = append(args, r.audioArgs()...)
	} else if r.Audio >= 0 && r.ADTS && r.Format == FMP4 {
		args = append(args, "-bsf:a", "aac_adtstoasc")
	}
	args = append(args, r.threadArgs()...)
	flags := "delete_segments+independent_segments+temp_file+omit_endlist"
	init := liveInit
	if restart {
		flags += "+discont_start"
		init = "init-" + strconv.Itoa(start) + ".mp4"
	}
	args = append(args, "-f", "hls", "-hls_init_time", strconv.Itoa(liveInitTime), "-hls_time", strconv.Itoa(liveSegment),
		"-hls_list_size", strconv.Itoa(liveWindow), "-hls_flags", flags, "-start_number", strconv.Itoa(start))
	if r.Format == FMP4 {
		args = append(args, "-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", init)
	} else {
		args = append(args, "-hls_segment_type", "mpegts")
	}
	return append(args, "-hls_segment_filename", filepath.Join(dir, "%d."+r.Format.Extension()), filepath.Join(dir, livePlaylist))
}

// audioArgs are FFmpeg's options converting the audio, none when it is
// copied.
func (r Remux) audioArgs() []string {
	if r.Audio < 0 || r.AudioCodec == "" {
		return nil
	}
	args := []string{"-c:a", r.AudioCodec, "-ac", strconv.Itoa(r.AudioChannels)}
	if r.AudioBitrate > 0 {
		args = append(args, "-b:a", strconv.FormatInt(r.AudioBitrate, 10))
	}
	if r.AudioFilter != "" {
		args = append(args, "-af", r.AudioFilter)
	}
	return args
}

// threadArgs bound the threads of a conversion; a copy needs none.
func (r Remux) threadArgs() []string {
	if r.Threads <= 0 || r.Encode == nil && (r.Audio < 0 || r.AudioCodec == "") {
		return nil
	}
	return []string{"-threads", strconv.Itoa(r.Threads)}
}

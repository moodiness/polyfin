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
	// liveRetry is how long after a run starts a failed run may start
	// again for the next request, so that a source that refuses does not
	// make every player request start FFmpeg.
	liveRetry = 10 * time.Second
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

// ErrBusy reports a live encoding refused because the server runs as many
// as it may.
var ErrBusy = errors.New("too many live encodings")

// liveFile matches the files of a live encoding a player may fetch.
var liveFile = regexp.MustCompile(`^(\d+\.(ts|mp4)|init\.mp4)$`)

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
	// run is the current or last run of FFmpeg, nil before the first.
	run  *liveRun
	next int
	used time.Time
	// created orders a user's live encodings, the oldest replaced first.
	created time.Time
	// stopped is set once the encoding is stopped for good.
	stopped bool
}

// liveRun is one run of FFmpeg.
type liveRun struct {
	started time.Time
	cancel  context.CancelFunc
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
		// none starts past serverLives.
		var oldest *live
		mine := 0
		for k, other := range m.lives {
			if k.User == key.User {
				mine++
				if oldest == nil || other.created.Before(oldest.created) {
					oldest = other
				}
			}
		}
		switch {
		case mine >= userLives:
			replaced = oldest
			delete(m.lives, oldest.key)
		case len(m.lives) >= serverLives:
			m.mu.Unlock()
			return nil, ErrBusy
		}
		now := time.Now()
		l = &live{m: m, key: key, dir: filepath.Join(m.dir, "live-"+key.name()), used: now, created: now}
		m.lives[key] = l
	}
	m.mu.Unlock()
	if replaced != nil {
		replaced.stop()
	}
	run, err := l.ensure(ctx, open)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, liveWait)
	defer cancel()
	for {
		data, err := os.ReadFile(filepath.Join(l.dir, livePlaylist))
		if err == nil && bytes.Contains(data, []byte("#EXTINF")) {
			return rewriteLive(data, uri), nil
		}
		select {
		case <-run.done:
			if run.err != nil {
				return nil, run.err
			}
			// FFmpeg reached the end of the stream: its playlist is final.
			if data, err := os.ReadFile(filepath.Join(l.dir, livePlaylist)); err == nil {
				return rewriteLive(data, uri), nil
			}
			return nil, ErrNotFound
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
// it runs, ran to the end of the stream, or failed moments ago.
func (l *live) ensure(ctx context.Context, open Opener) (*liveRun, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.used = time.Now()
	if l.stopped {
		return nil, ErrStopped
	}
	if !l.opened {
		remux, release, err := open(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}
		l.remux, l.release, l.opened = remux, release, true
	}
	run := l.run
	if run != nil {
		select {
		case <-run.done:
			if run.err == nil || time.Since(run.started) < liveRetry {
				return run, nil
			}
		default:
			return run, nil
		}
	}
	return l.start()
}

// start runs FFmpeg again into an empty directory, numbering segments on
// from the last run's, so that a player that reloads the playlist does not
// see the stream go back. The caller holds l.mu.
func (l *live) start() (*liveRun, error) {
	if err := os.RemoveAll(l.dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &liveRun{started: time.Now(), cancel: cancel, done: make(chan struct{})}
	l.run = run
	args := l.remux.liveArgs(l.dir, l.next)
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
			l.m.logger.Debug("A live stream ended")
		}
		l.mu.Lock()
		l.next = max(l.next, lastSegment(l.dir)+1)
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

// rewriteLive names the files FFmpeg's playlist refers to by uri.
func rewriteLive(playlist []byte, uri func(name string) string) []byte {
	var out bytes.Buffer
	for line := range strings.Lines(string(playlist)) {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			line = strings.Replace(line, `URI="`+liveInit+`"`, `URI="`+uri(liveInit)+`"`, 1)
		case !strings.HasPrefix(line, "#"):
			line = uri(line)
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// liveArgs is FFmpeg's command line for a live encoding writing into dir,
// its first segment numbered start.
func (r Remux) liveArgs(dir string, start int) []string {
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
		args = append(args, "-c:a", r.AudioCodec, "-ac", strconv.Itoa(r.AudioChannels))
		if r.AudioBitrate > 0 {
			args = append(args, "-b:a", strconv.FormatInt(r.AudioBitrate, 10))
		}
	} else if r.Audio >= 0 && r.ADTS && r.Format == FMP4 {
		args = append(args, "-bsf:a", "aac_adtstoasc")
	}
	args = append(args, "-f", "hls", "-hls_time", strconv.Itoa(liveSegment), "-hls_list_size", strconv.Itoa(liveWindow),
		"-hls_flags", "delete_segments+independent_segments+temp_file", "-start_number", strconv.Itoa(start))
	if r.Format == FMP4 {
		args = append(args, "-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", liveInit)
	} else {
		args = append(args, "-hls_segment_type", "mpegts")
	}
	return append(args, "-hls_segment_filename", filepath.Join(dir, "%d."+r.Format.Extension()), filepath.Join(dir, livePlaylist))
}

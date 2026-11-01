package hls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A playout airs a channel Polyfin makes itself (see package virtualtv):
// one programme after the other, each from the moment the schedule has it
// at, as one live HLS stream. Each piece it airs, a programme or a slate
// standing in for one that does not play, is a run of FFmpeg that converts
// it in real time into segments numbered on from the last run's; the
// playlist is Polyfin's own, which lists the last segments of the runs and
// starts a new discontinuity at each piece. One playout airs a channel,
// whoever watches it: players and recordings read its playlist.
const (
	// playoutKept is how many segments a playout keeps on disk, for the
	// players a little behind the playlist.
	playoutKept = 2 * liveWindow
	// playoutRunList is how many segments the playlist of one run lists,
	// read every playoutPoll: more than FFmpeg makes meanwhile.
	playoutRunList = 30
	playoutPoll    = 300 * time.Millisecond
	// playoutLead is how far a playout's pieces run ahead of the clock:
	// its first piece bursts that much at once, for the first playlist's
	// lead, and the next ones keep it.
	playoutLead = liveLead + 2*time.Second
	// playoutCatchUp bounds how far a playout's pieces may fall behind the
	// clock, as when FFmpeg converts slower than real time: past it, the
	// next piece is what airs now.
	playoutCatchUp = 30 * time.Second
	// playoutRetry is how long after failing to find what to air a
	// playout asks again.
	playoutRetry = 5 * time.Second
	// playoutEarly is how much shorter than its length a piece may end and
	// still have aired whole.
	playoutEarly = 1500 * time.Millisecond
	// playoutRunPlaylist is the playlist FFmpeg writes for each run.
	playoutRunPlaylist = "run.m3u8"
	// slateSize is the picture of a slate, black and silent.
	slateSize = "1280x720"
)

// ErrEndedEarly reports a piece whose input ended before its length: a
// programme shorter than its schedule.
var ErrEndedEarly = errors.New("the programme ended before its time")

// PlayoutPiece is what a playout airs next: Remux's input from Offset on,
// its video converted (Remux.Encode) and its audio converted to AAC, or a
// black and silent slate, for Length.
type PlayoutPiece struct {
	Remux          Remux
	Offset, Length time.Duration
	Slate          bool
}

// PlayoutNext returns what a playout airs from at on, with what to release
// once it aired. failure tells how the piece before ended, when it did
// not air whole: ErrEndedEarly, or the error of its run. An error that is
// ErrStopped ends the playout; another is tried again a little later.
type PlayoutNext func(ctx context.Context, at time.Time, failure error) (PlayoutPiece, func(), error)

// playoutSegment is a segment a playout made: its number, its length in
// seconds, and how many pieces started before its own.
type playoutSegment struct {
	number   int
	duration float64
	piece    int
}

type playout struct {
	m    *Manager
	key  string
	dir  string
	name string
	next PlayoutNext

	mu sync.Mutex
	// segments are the last segments made, the oldest first; number is
	// the next one's, and elapsed how long the segments made so far last
	// in all, where the next run's timestamps start.
	segments []playoutSegment
	number   int
	elapsed  time.Duration
	// changed is closed, and replaced, when a segment is made, when the
	// playout fails to find what to air (err), and when it stops.
	changed chan struct{}
	err     error
	used    time.Time
	// answered is set once a playlist was answered (see ready).
	answered bool
	stopped  bool
	cancel   context.CancelFunc
	done     chan struct{}
}

// Playout returns the media playlist of the playout of key, a channel,
// starting it when it does not run; next tells what it airs (see
// PlayoutNext), name names it in the log, and uri names each segment the
// playlist lists. The first playlist waits for the segments the player
// asks for and a lead, as a live encoding's does (see LivePlaylist).
// Playouts count among the server's live encodings, and among the
// conversions of video.
func (m *Manager) Playout(ctx context.Context, key, name string, next PlayoutNext, uri func(name string) string, minSegments int) ([]byte, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrStopped
	}
	p := m.playouts[key]
	if p == nil {
		limit := m.limit()
		if len(m.lives)+len(m.recordings)+len(m.playouts) >= serverLives || limit > 0 && m.converting(nil, nil) >= limit {
			m.mu.Unlock()
			return nil, ErrBusy
		}
		run, cancel := context.WithCancel(context.Background())
		p = &playout{m: m, key: key, dir: filepath.Join(m.dir, "playout-"+key), name: name, next: next, changed: make(chan struct{}),
			used: time.Now(), cancel: cancel, done: make(chan struct{})}
		m.playouts[key] = p
		if err := errors.Join(os.RemoveAll(p.dir), os.MkdirAll(p.dir, 0o700)); err != nil {
			delete(m.playouts, key)
			m.mu.Unlock()
			cancel()
			return nil, err
		}
		go p.run(run)
		go p.watchIdle()
	}
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, liveWait)
	defer cancel()
	for {
		p.mu.Lock()
		p.used = time.Now()
		stopped, err, changed := p.stopped, p.err, p.changed
		var data []byte
		if p.ready(minSegments) {
			data = playoutPlaylist(p.segments, uri)
		}
		empty := len(p.segments) == 0
		p.mu.Unlock()
		switch {
		case data != nil:
			return data, nil
		case stopped:
			return nil, ErrStopped
		case empty && err != nil:
			return nil, err
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// PlayoutSegment opens a segment of the playout of key.
func (m *Manager) PlayoutSegment(key, name string) (*os.File, error) {
	m.mu.Lock()
	p := m.playouts[key]
	m.mu.Unlock()
	if p == nil || !liveFile.MatchString(name) || !strings.HasSuffix(name, ".ts") {
		return nil, ErrNotFound
	}
	p.mu.Lock()
	p.used = time.Now()
	stopped := p.stopped
	p.mu.Unlock()
	if stopped {
		return nil, ErrStopped
	}
	file, err := os.Open(filepath.Join(p.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return file, err
}

// StopPlayout stops the playout of key, if it runs: its channel changed.
func (m *Manager) StopPlayout(key string) {
	m.stopPlayouts(func(k string) bool { return k == key })
}

func (m *Manager) stopPlayouts(match func(string) bool) {
	m.mu.Lock()
	var stopping []*playout
	for key, p := range m.playouts {
		if match(key) {
			stopping = append(stopping, p)
			delete(m.playouts, key)
		}
	}
	m.mu.Unlock()
	for _, p := range stopping {
		p.stop()
	}
}

// ready reports whether the playlist may be answered: the first once it
// lists the segments the player asks for, and liveLeadSegments of liveLead
// in all; the next ones once it lists one. The caller holds p.mu.
func (p *playout) ready(minSegments int) bool {
	listed := p.segments[max(0, len(p.segments)-liveWindow):]
	if p.answered {
		return len(listed) > 0
	}
	var length float64
	for _, s := range listed {
		length += s.duration
	}
	if len(listed) < min(max(minSegments, liveLeadSegments), liveWindow) || time.Duration(length*float64(time.Second)) < liveLead {
		return false
	}
	p.answered = true
	return true
}

// playoutPlaylist is the live playlist of a playout's segments: its last
// liveWindow, a discontinuity starting each piece but the first listed,
// counted by the discontinuity sequence, which tells players the pieces
// that left the playlist.
func playoutPlaylist(segments []playoutSegment, uri func(name string) string) []byte {
	listed := segments[max(0, len(segments)-liveWindow):]
	target := liveSegment
	for _, s := range listed {
		target = max(target, int(math.Ceil(s.duration)))
	}
	var b bytes.Buffer
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", target)
	if len(listed) > 0 {
		fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n#EXT-X-DISCONTINUITY-SEQUENCE:%d\n", listed[0].number, listed[0].piece)
	}
	for i, s := range listed {
		if i > 0 && s.piece != listed[i-1].piece {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n%s\n", s.duration, uri(strconv.Itoa(s.number)+".ts"))
	}
	return b.Bytes()
}

// run airs piece after piece until the playout stops, from the moment it
// started on. Pieces are timed by what they aired, ahead of the clock by
// playoutLead (see burst), and caught up when they fall behind it by more
// than playoutCatchUp.
func (p *playout) run(ctx context.Context) {
	defer close(p.done)
	at := time.Now()
	var failure error
	for ctx.Err() == nil {
		piece, release, err := p.next(ctx, at, failure)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, ErrStopped) {
				p.m.logger.Info("A channel's playout stopped", "channel", p.name, "error", err)
				go p.m.StopPlayout(p.key)
				return
			}
			p.m.logger.Warn("A channel's playout found nothing to air", "channel", p.name, "error", err)
			p.fail(err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(playoutRetry):
			}
			at, failure = time.Now(), nil
			continue
		}
		aired, err := p.air(ctx, piece, burst(at, time.Now()))
		release()
		if ctx.Err() != nil {
			return
		}
		switch {
		case err != nil:
			p.m.logger.Warn("A programme of a channel failed to air", "channel", p.name, "error", err)
			failure = err
		case aired < piece.Length-playoutEarly:
			failure = ErrEndedEarly
		default:
			failure = nil
			aired = piece.Length
		}
		at = at.Add(aired)
		if now := time.Now(); now.Sub(at) > playoutCatchUp {
			p.m.logger.Info("A channel's playout fell behind the clock, and skips to what airs now", "channel", p.name, "behind", now.Sub(at).Round(time.Second))
			at = now
		}
		if err != nil && aired == 0 {
			// A piece that failed at once is not tried again in a loop.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

// burst is how much of its next piece, starting at at, a playout reads at
// once: what keeps it playoutLead ahead of now, at most playoutCatchUp.
func burst(at, now time.Time) time.Duration {
	return min(max(playoutLead-at.Sub(now), 0), playoutCatchUp)
}

// fail records that the playout found nothing to air.
func (p *playout) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
	close(p.changed)
	p.changed = make(chan struct{})
}

// air runs FFmpeg for a piece, collecting the segments it makes, and
// returns how long they last.
func (p *playout) air(ctx context.Context, piece PlayoutPiece, burst time.Duration) (time.Duration, error) {
	runList := filepath.Join(p.dir, playoutRunPlaylist)
	if err := os.Remove(runList); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	p.mu.Lock()
	args := piece.args(p.dir, p.number, burst, p.elapsed)
	p.mu.Unlock()
	cmd := exec.CommandContext(ctx, p.m.ffmpegPath(), args...)
	cmd.WaitDelay = 5 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	ticker := time.NewTicker(playoutPoll)
	defer ticker.Stop()
	var aired time.Duration
	started := false
	for {
		select {
		case err := <-exited:
			aired += p.collect(runList, &started)
			if err != nil && ctx.Err() == nil {
				err = fmt.Errorf("FFmpeg failed: %w: %s", err, cutAddresses(string(bytes.TrimSpace(stderr.bytes()))))
			}
			return aired, err
		case <-ticker.C:
			aired += p.collect(runList, &started)
		}
	}
}

// collect adds the segments a run's playlist lists that the playout does
// not have yet, the first of the run starting a piece, and returns how
// long they last; the oldest are deleted past playoutKept.
func (p *playout) collect(runList string, started *bool) time.Duration {
	data, err := os.ReadFile(runList)
	if err != nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var added time.Duration
	duration := -1.0
	for line := range strings.Lines(string(data)) {
		if !strings.HasSuffix(line, "\n") {
			// FFmpeg is still writing it.
			break
		}
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "#EXTINF:"); ok {
			duration, _ = strconv.ParseFloat(strings.TrimSuffix(value, ","), 64)
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		raw, ok := strings.CutSuffix(filepath.Base(line), ".ts")
		n, err := strconv.Atoi(raw)
		if !ok || err != nil || n < p.number || duration < 0 {
			duration = -1
			continue
		}
		piece := 0
		if len(p.segments) > 0 {
			piece = p.segments[len(p.segments)-1].piece
			if !*started {
				piece++
			}
		}
		*started = true
		p.segments = append(p.segments, playoutSegment{number: n, duration: duration, piece: piece})
		p.number = n + 1
		length := time.Duration(duration * float64(time.Second))
		added += length
		p.elapsed += length
		duration = -1
	}
	if added == 0 {
		return 0
	}
	for len(p.segments) > playoutKept {
		_ = os.Remove(filepath.Join(p.dir, strconv.Itoa(p.segments[0].number)+".ts"))
		p.segments = p.segments[1:]
	}
	p.err = nil
	close(p.changed)
	p.changed = make(chan struct{})
	return added
}

// watchIdle stops the playout once nothing asked anything of it for
// liveIdle: players reload a live playlist every few seconds.
func (p *playout) watchIdle() {
	ticker := time.NewTicker(liveIdle / 6)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.mu.Lock()
			idle := time.Since(p.used) > liveIdle
			p.mu.Unlock()
			if idle {
				p.m.stopPlayouts(func(key string) bool { return key == p.key && p.m.playouts[key] == p })
				return
			}
		}
	}
}

// stop ends the playout and removes its segments.
func (p *playout) stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
	p.cancel()
	<-p.done
	if err := os.RemoveAll(p.dir); err != nil {
		p.m.logger.Warn("The segments of a channel's playout could not be removed", "error", err)
	}
}

// ffmpegSeconds writes a length in seconds, as FFmpeg reads them.
func ffmpegSeconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

// args is FFmpeg's command line airing the piece into dir: its first
// segment numbered start, reading burst of it at once and then in real
// time, its timestamps starting at offset, where the run before ended.
// Every segment starts with a keyframe, every liveSegment seconds; a piece
// without audio gets silence, so that every segment carries the same
// tracks.
func (piece PlayoutPiece) args(dir string, start int, burst, offset time.Duration) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	pace := []string{"-readrate", "1", "-readrate_initial_burst", ffmpegSeconds(burst)}
	silence := append(append([]string{"-f", "lavfi"}, pace...), "-i", "anullsrc=r=48000:cl=stereo")
	keyframes := "expr:gte(t,n_forced*" + strconv.Itoa(liveSegment) + ")"
	stereo := []string{"-c:a", "aac", "-ac", "2", "-b:a", "128000"}
	r := piece.Remux
	switch {
	case piece.Slate || r.Encode == nil:
		args = append(args, "-f", "lavfi")
		args = append(args, pace...)
		args = append(args, "-i", "color=c=black:s="+slateSize+":r=25")
		args = append(args, silence...)
		args = append(args, "-t", ffmpegSeconds(piece.Length), "-map", "0:v", "-map", "1:a",
			"-c:v", "libx264", "-preset", "ultrafast", "-tune", "stillimage", "-pix_fmt", "yuv420p", "-force_key_frames", keyframes)
		args = append(args, stereo...)
	default:
		args = append(args, r.Encode.inputs()...)
		args = append(args, r.InputOptions...)
		args = append(args, r.decoderArgs()...)
		args = append(args, pace...)
		if piece.Offset > 0 {
			args = append(args, "-ss", ffmpegSeconds(piece.Offset))
		}
		args = append(args, "-i", r.Input)
		if r.Audio < 0 {
			args = append(args, silence...)
		}
		args = append(args, "-t", ffmpegSeconds(piece.Length), "-map", "0:"+strconv.Itoa(r.Video))
		if r.Audio >= 0 {
			args = append(args, "-map", "0:"+strconv.Itoa(r.Audio))
		} else {
			args = append(args, "-map", "1:a")
		}
		args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn")
		args = append(args, r.Encode.encoderArgs(keyframes, "")...)
		if r.Audio >= 0 && r.AudioCodec != "" {
			args = append(args, r.audioArgs()...)
		} else {
			args = append(args, stereo...)
		}
		args = append(args, r.threadArgs()...)
	}
	return append(args, "-output_ts_offset", ffmpegSeconds(offset),
		"-f", "hls", "-hls_time", strconv.Itoa(liveSegment), "-hls_list_size", strconv.Itoa(playoutRunList),
		"-hls_flags", "independent_segments+temp_file+omit_endlist", "-start_number", strconv.Itoa(start),
		"-hls_segment_type", "mpegts", "-hls_segment_filename", filepath.Join(dir, "%d.ts"), filepath.Join(dir, playoutRunPlaylist))
}

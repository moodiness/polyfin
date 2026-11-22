package hls

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/diskspace"
)

// Live TV pause and rewind. A live encoding keeps the segments FFmpeg makes
// in a buffer, which every player watching its channel shares (see
// Key.channel), up to LiveBuffer.Length. FFmpeg's own playlist lists the
// last liveWindow segments only: the buffer takes each one in as it comes
// (see takeIn), and Polyfin writes each player's playlist from it. A
// player's playlist starts where that player joined, a lead behind the
// newest segment (see window), and grows as the buffer does: a player
// starting a live stream at the first segment its playlist lists, as
// jellyfin-web's does, still starts near the live edge, and a player paused
// finds the segments it left once it plays on.

const (
	// liveLinger is how long a live encoding no one watches keeps its
	// buffer, FFmpeg stopped and its source released, before it is
	// removed: a player that went quiet and comes back to its play session
	// within it finds what it left, and FFmpeg starts again behind it.
	liveLinger = 5 * time.Minute
	// bufferEvery is how often the buffer of a running encoding takes in
	// the segments FFmpeg made: FFmpeg lists the last liveWindow only, of
	// a second each at first (see liveInitTime).
	bufferEvery = time.Second
)

// LiveBuffer is what live encodings keep for pause and rewind (see
// BufferLives).
type LiveBuffer struct {
	// Length is how much of each channel watched is kept: 0 keeps its
	// live window only.
	Length time.Duration
	// Bytes bounds the buffers of all channels together, which also keep
	// within half the free space of the segments folder.
	Bytes int64
}

// BufferLives sets what live encodings keep, which buffer returns, read
// as each segment comes: a change applies at once.
func (m *Manager) BufferLives(buffer func() LiveBuffer) {
	m.buffer.Store(&buffer)
}

// bufferSettings is what live encodings keep (see BufferLives).
func (m *Manager) bufferSettings() LiveBuffer {
	if buffer := m.buffer.Load(); buffer != nil {
		return (*buffer)()
	}
	return LiveBuffer{}
}

// bufferedSegment is a segment of a live encoding's buffer.
type bufferedSegment struct {
	// name is its file, number what FFmpeg numbered it.
	name     string
	number   int
	duration time.Duration
	size     int64
	// init is the initialization segment it follows, empty for MPEG-TS.
	init string
	// discontinuity marks the first segment of a run after another;
	// discontinuities counts them up to this segment, itself included.
	discontinuity   bool
	discontinuities int
}

// segmentBuffer is what a live encoding keeps of the segments FFmpeg made.
type segmentBuffer struct {
	segments []bufferedSegment
	// first is the media sequence number of segments[0]: segments are
	// numbered on as they come, whatever FFmpeg named them.
	first int
	// last is the number FFmpeg gave the last segment taken in, -1 before
	// the first.
	last          int
	length        time.Duration
	bytes         int64
	discontinuity int
	// target is the playlists' target duration, in seconds, which never
	// goes down, as players expect it not to change.
	target int
	// discontinue marks the next segment taken in as the first of a run
	// after another; runFirst is the sequence number the current run's
	// segments start at, where players joining start (see window); init
	// is the current run's initialization segment, for fragmented MP4.
	discontinue bool
	runFirst    int
	init        string
	// doomed are the files dropped from the buffer, removed once the next
	// segment comes in: a player may still be fetching them.
	doomed []string
}

// end is the sequence number the next segment taken in gets.
func (b *segmentBuffer) end() int {
	return b.first + len(b.segments)
}

// viewer is a play session watching a live encoding's channel.
type viewer struct {
	user, device string
	// joined is when it started watching, used when it last asked for a
	// file or reported its progress (see KeepWatching).
	joined, used time.Time
	// from is the sequence number its playlists start at, set once the
	// first is answered.
	from     int
	answered bool
}

// active reports whether the viewer asked for something within liveIdle:
// players reload a live playlist every few seconds while they play, and
// apps report their progress while paused.
func (v *viewer) active(now time.Time) bool {
	return now.Sub(v.used) <= liveIdle
}

// watched reports whether a viewer of the encoding is active.
func (l *live) watched(now time.Time) bool {
	l.vmu.Lock()
	defer l.vmu.Unlock()
	for _, v := range l.viewers {
		if v.active(now) {
			return true
		}
	}
	return false
}

// keepsLocked reports whether the encoding keeps a buffer worth lingering
// for once no one watches it. The caller holds l.mu.
func (l *live) keepsLocked() bool {
	return l.m.bufferSettings().Length > 0 && len(l.buffer.segments) > 0
}

// takeIn adds the segments FFmpeg's playlist lists past the last one taken
// in to the buffer, removes the files dropped before, and trims it. It
// reports whether it took one in. The caller holds l.mu.
func (l *live) takeIn() bool {
	data, err := os.ReadFile(filepath.Join(l.dir, livePlaylist))
	if err != nil {
		return false
	}
	b := &l.buffer
	added := false
	init, duration := "", time.Duration(0)
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			if found := liveMap.FindStringSubmatch(line); found != nil {
				init = found[1]
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			value, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			duration = 0
			if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
				duration = time.Duration(seconds * float64(time.Second))
			}
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			number, ok := segmentNumber(line)
			if !ok || number <= b.last {
				continue
			}
			info, err := os.Stat(filepath.Join(l.dir, line))
			if err != nil {
				continue
			}
			if !added {
				l.removeDoomed()
				added = true
			}
			s := bufferedSegment{name: line, number: number, duration: duration, size: info.Size(), init: init, discontinuity: b.discontinue}
			if s.discontinuity {
				b.discontinuity++
			}
			s.discontinuities = b.discontinuity
			b.segments = append(b.segments, s)
			b.discontinue = false
			b.last, b.length, b.bytes = number, b.length+duration, b.bytes+s.size
			b.target = max(b.target, int(math.Round(duration.Seconds())), 1)
		}
	}
	if added {
		l.trim()
	}
	return added
}

// segmentNumber is the number of a segment FFmpeg named.
func segmentNumber(name string) (int, bool) {
	if !liveFile.MatchString(name) || strings.HasPrefix(name, "init") {
		return 0, false
	}
	stem, _, _ := strings.Cut(name, ".")
	n, err := strconv.Atoi(stem)
	return n, err == nil
}

// trim drops the oldest segments past the buffer's length, keeping the
// live window at least. The caller holds l.mu.
func (l *live) trim() {
	keep := l.m.bufferSettings().Length
	b := &l.buffer
	for len(b.segments) > liveWindow && (keep <= 0 || b.length-b.segments[0].duration >= keep) {
		l.dropOldest()
	}
}

// dropOldest drops the oldest segment of the buffer, and the
// initialization segment of an earlier run once no segment follows it, and
// returns their size. Their files are removed once the next segment comes
// in. The caller holds l.mu.
func (l *live) dropOldest() int64 {
	b := &l.buffer
	s := b.segments[0]
	b.segments = slices.Delete(b.segments, 0, 1)
	b.first++
	b.length -= s.duration
	b.bytes -= s.size
	b.doomed = append(b.doomed, s.name)
	if s.init != "" && s.init != b.init && (len(b.segments) == 0 || b.segments[0].init != s.init) {
		b.doomed = append(b.doomed, s.init)
	}
	return s.size
}

// removeDoomed removes the files dropped from the buffer. One that cannot
// be removed, held open on Windows, goes with the encoding's folder. The
// caller holds l.mu.
func (l *live) removeDoomed() {
	for _, name := range l.buffer.doomed {
		if err := os.Remove(filepath.Join(l.dir, name)); err != nil && !os.IsNotExist(err) {
			l.m.logger.Debug("A segment of a live buffer could not be removed", "error", err)
		}
	}
	l.buffer.doomed = nil
}

// droppable is how much of the buffer may go for the bound on all
// buffers: what it keeps past its live window. The caller holds l.mu.
func (l *live) droppable() time.Duration {
	b := &l.buffer
	if len(b.segments) <= liveWindow {
		return 0
	}
	var window time.Duration
	for _, s := range b.segments[len(b.segments)-liveWindow:] {
		window += s.duration
	}
	return b.length - window
}

// window is where the playlist of v starts, and whether it may be
// answered. A viewer's first playlist starts at the current run's
// segments, as near the newest as gives the segments the app asks for
// (minSegments) and a lead of liveLeadSegments of liveLead, which it waits
// for (see LivePlaylist); once a run ended, one segment is enough, as a
// source read again owes no lead. Its next playlists start at the same
// segment, or the oldest one kept past it, and list every segment since:
// the player may go back to where it joined, the buffer's length at most,
// and finds what it paused on. The caller holds l.mu.
func (l *live) window(v *viewer, minSegments int) (int, bool) {
	b := &l.buffer
	end := b.end()
	if v.answered {
		start := max(v.from, b.first)
		return start, start < end
	}
	need := min(max(minSegments, liveLeadSegments), liveWindow)
	start, count, lead := end, 0, time.Duration(0)
	for start > max(b.runFirst, b.first) && (count < need || lead < liveLead) {
		start--
		count++
		lead += b.segments[start-b.first].duration
	}
	if count == 0 || (count < need || lead < liveLead) && l.ended == 0 {
		return 0, false
	}
	v.from, v.answered = start, true
	return start, true
}

// playlist writes the media playlist of the buffer's segments from the
// sequence number start on, naming each file by uri. The caller holds
// l.mu.
func (l *live) playlist(start int, uri func(name string) string) []byte {
	b := &l.buffer
	segments := b.segments[start-b.first:]
	version := 6
	if segments[0].init != "" {
		version = 7
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "#EXTM3U\n#EXT-X-VERSION:%d\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n", version, b.target, start)
	if before := segments[0].discontinuities - boolInt(segments[0].discontinuity); before > 0 {
		fmt.Fprintf(&out, "#EXT-X-DISCONTINUITY-SEQUENCE:%d\n", before)
	}
	out.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")
	init := ""
	for _, s := range segments {
		if s.discontinuity {
			out.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		if s.init != "" && s.init != init {
			init = s.init
			fmt.Fprintf(&out, "#EXT-X-MAP:URI=%q\n", uri(init))
		}
		fmt.Fprintf(&out, "#EXTINF:%.6f,\n%s\n", s.duration.Seconds(), uri(s.name))
	}
	return out.Bytes()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// follow takes the segments run makes in as they come, until it ends, and
// keeps all buffers within their bound.
func (l *live) follow(run *liveRun) {
	ticker := time.NewTicker(bufferEvery)
	defer ticker.Stop()
	for {
		select {
		case <-run.done:
			return
		case <-ticker.C:
		}
		l.mu.Lock()
		added := l.takeIn()
		l.mu.Unlock()
		if added {
			l.m.capBuffers()
		}
	}
}

// capBuffers keeps the buffers of all live encodings together within
// LiveBuffer.Bytes, and within half of what the segments folder's disk
// leaves them: the free space and what they take already. The longest
// buffer gives its oldest segment first, so that channels keep as many
// minutes each; none goes below its live window. A live encoding whose
// input is being opened, which holds its lock, is left for the next pass.
func (m *Manager) capBuffers() {
	limit := m.bufferSettings().Bytes
	if limit <= 0 || !m.capping.TryLock() {
		return
	}
	defer m.capping.Unlock()
	lives := m.liveList()
	var total int64
	for _, l := range lives {
		if l.mu.TryLock() {
			total += l.buffer.bytes
			l.mu.Unlock()
		}
	}
	measure := m.diskFree
	if measure == nil {
		measure = diskspace.Measure
	}
	if free, _, _, ok := measure(m.dir); ok {
		limit = min(limit, (free+total)/2)
	}
	for total > limit {
		var longest *live
		var length time.Duration
		for _, l := range lives {
			if !l.mu.TryLock() {
				continue
			}
			if d := l.droppable(); d > length {
				longest, length = l, d
			}
			l.mu.Unlock()
		}
		if longest == nil || !longest.mu.TryLock() {
			return
		}
		if longest.droppable() > 0 {
			total -= longest.dropOldest()
		}
		longest.mu.Unlock()
	}
}

// KeepWatching tells that a play session still watches its channel, as an
// app reports its progress while paused, when its player may ask for
// nothing: the channel's buffer goes on growing for it (see liveIdle).
func (m *Manager) KeepWatching(session string) {
	now := time.Now()
	for _, l := range m.liveList() {
		l.vmu.Lock()
		if v := l.viewers[session]; v != nil {
			v.used = now
		}
		l.vmu.Unlock()
	}
}

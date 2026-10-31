package hls

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// keepFor is how long the converted video of a play session that stopped
// is kept for another session to take over: apps changing the audio track
// stop the old session, then play the new one within seconds.
const keepFor = 30 * time.Second

// KeepVideo has converted video kept apart from its audio track while
// keep returns true, read as each request comes: an app changing the
// track, which plays a new session from the same position, gets the video
// already converted, and only the new track is made.
//
// A playback's kept video is known by its user, device, version, segment
// format and conversion (Key.Video) and variant, not by its play session
// or audio track: one FFmpeg converts the video alone, and a track (see
// track) makes the audio played, for every variant. Each segment served
// joins the two (see joinTS and joinInit). When the session stops, the
// video is kept keepFor, its FFmpeg waiting where it was: a new session
// asking for the same video takes it over, with the segments made, and
// the track it asks for starts at the segment it asks for, the old
// track's FFmpeg stopping. A session that never stopped is never taken
// over: a second app on the same device playing the same video at once
// converts its own. A kept video no session took over stops, and its
// segments are removed; so does one kept before for the same user,
// device and version, once another is kept or converted.
//
// Kept video counts once toward the conversions at once, being the
// user's playback of the version. Copied video is not kept: it costs no
// conversion.
func (m *Manager) KeepVideo(keep func() bool) {
	m.keepVideo.Store(&keep)
}

// keeping is what the variants of a kept video share: the session it
// plays for, and the audio track it plays.
//
// Locks are taken in the order keeping.mu, Manager.mu, encoding.mu,
// track.mu, each optional.
type keeping struct {
	// key is the key of the playback's first variant.
	key Key
	// released is set once session stopped, and timer then ends the
	// keeping keepFor later, unless a session took it over.
	released atomic.Bool

	mu      sync.Mutex
	session string
	timer   *time.Timer
	ended   bool
	track   *track
}

// keptVideo is the key of the kept video key plays.
func (k Key) keptVideo() Key {
	return Key{Audio: -1, Format: k.Format, User: k.User, Device: k.Device, Version: k.Version, Converts: true, Video: k.Video, Variant: k.Variant}
}

// kept reports whether a key names kept video, rather than what a play
// session plays.
func (k Key) kept() bool {
	return k.Session == "" && k.Video != ""
}

// sameDevice reports whether two keys are of the same user, device and
// version.
func (k Key) sameDevice(other Key) bool {
	return k.User == other.User && k.Device == other.Device && k.Version == other.Version
}

// keeps reports whether the encoding of key is kept video: kept video is
// on, key converts the video it describes, without fast segments, its
// session has no encoding of its own already, and no other session plays
// the same video that did not stop.
func (m *Manager) keeps(key Key) bool {
	keep := m.keepVideo.Load()
	if keep == nil || !(*keep)() || !key.Converts || key.Video == "" || key.Session == "" || key.FastStart {
		return false
	}
	m.mu.Lock()
	_, own := m.encodings[key]
	k := m.kept[key.keptVideo().playback()]
	m.mu.Unlock()
	if own {
		return false
	}
	if k == nil || k.released.Load() {
		return true
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.session == "" || k.session == key.Session
}

// videoAlone is open, the audio left out: the kept video's FFmpeg makes
// the video alone. Its input is never switched for another (see
// Remux.Switch): its audio's FFmpeg reads the version on its own.
func videoAlone(open Opener) Opener {
	return func(ctx context.Context) (Remux, func(), error) {
		r, release, err := open(ctx)
		r.Audio, r.ADTS, r.AudioCodec, r.AudioChannels, r.AudioBitrate, r.AudioFilter = -1, false, "", 0, 0, ""
		r.Switch = nil
		return r, release, err
	}
}

// keepPlayback records a kept video's playback when key is the first of
// its variants opened, and returns the kept videos of the same user,
// device and version that stopped, which it replaces. The caller holds
// m.mu.
func (m *Manager) keepPlayback(key Key) []*keeping {
	if !key.kept() {
		return nil
	}
	playback := key.playback()
	if m.kept[playback] != nil {
		return nil
	}
	var stale []*keeping
	for other, k := range m.kept {
		if other.sameDevice(key) && k.released.Load() {
			stale = append(stale, k)
		}
	}
	m.kept[playback] = &keeping{key: playback}
	return stale
}

// keptFile opens segment n of the kept video key plays, or its
// initialization segment for n = -1, joined with the audio of key's
// track. The session of key takes the kept video over.
func (m *Manager) keptFile(ctx context.Context, key Key, open Opener, n int) (*os.File, error) {
	// A kept video that ended just as the session took it over is opened
	// again.
	for range 2 {
		e, err := m.encoding(ctx, key.keptVideo(), videoAlone(open))
		if err != nil {
			return nil, err
		}
		if n < 0 && e.remux.Format != FMP4 || n >= e.remux.Plan.Len() {
			return nil, ErrNotFound
		}
		m.mu.Lock()
		k := m.kept[e.key.playback()]
		m.mu.Unlock()
		if k == nil {
			continue
		}
		t, err := k.claim(ctx, m, e, key, open)
		if errors.Is(err, errStale) {
			continue
		}
		if err != nil {
			return nil, err
		}
		video, err := e.await(ctx, n)
		if err != nil || t == nil {
			return video, err
		}
		audio, err := t.await(ctx, n)
		switch {
		case err != nil:
			_ = video.Close()
			return nil, err
		case audio == nil && n < 0:
			_ = video.Close()
			return nil, errors.New("the audio has no initialization segment")
		case audio == nil:
			// The track ends before this segment.
			return video, nil
		}
		defer video.Close()
		defer audio.Close()
		return m.joined(e.remux.Format, n, video, audio, t)
	}
	return nil, ErrStopped
}

// claim has key's session take the kept video over, and returns the track
// it plays, nil for none: the one playing when it is the same, else a new
// one, the old one stopping.
func (k *keeping) claim(ctx context.Context, m *Manager, e *encoding, key Key, open Opener) (*track, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.ended {
		return nil, errStale
	}
	if k.released.Load() {
		k.released.Store(false)
		k.timer.Stop()
		m.logger.Debug("A new play session took a kept conversion over", "variant", key.Variant)
	}
	k.session = key.Session
	t := k.track
	if t != nil && t.session == key.Session && t.audio == key.Audio {
		return t, nil
	}
	var next *track
	if key.Audio >= 0 {
		remux, release, err := open(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}
		switch {
		case remux.Audio < 0:
			release()
		case t != nil && sameTrack(t.remux, remux):
			release()
			t.session, t.audio = key.Session, key.Audio
			return t, nil
		default:
			if next, err = m.newTrack(key, remux, release); err != nil {
				release()
				return nil, err
			}
		}
	}
	k.track = next
	e.playback.follow(next)
	if t != nil {
		m.logger.Debug("The audio track of a kept conversion changed", "from", t.audio, "to", key.Audio)
		t.stop()
	}
	return next, nil
}

// release keeps the video of a play session that stopped for keepFor, for
// another session to take over, and ends those kept before for the same
// user, device and version.
func (m *Manager) release(session string) {
	m.mu.Lock()
	keepings := make([]*keeping, 0, len(m.kept))
	for _, k := range m.kept {
		keepings = append(keepings, k)
	}
	m.mu.Unlock()
	var released []*keeping
	for _, k := range keepings {
		k.mu.Lock()
		if !k.ended && k.session == session && !k.released.Load() {
			k.released.Store(true)
			k.timer = time.AfterFunc(time.Duration(m.keepFor.Load()), func() { m.expire(k) })
			released = append(released, k)
		}
		k.mu.Unlock()
	}
	for _, k := range released {
		m.logger.Debug("Kept a conversion for a new play session", "variant", k.key.Variant)
		for _, other := range keepings {
			if other != k && other.key.sameDevice(k.key) {
				m.expire(other)
			}
		}
	}
}

// expire ends a kept video no session took over since its session
// stopped: its FFmpeg stops, and its segments are removed.
func (m *Manager) expire(k *keeping) {
	k.mu.Lock()
	if k.ended || !k.released.Load() {
		k.mu.Unlock()
		return
	}
	k.ended = true
	k.timer.Stop()
	t := k.track
	k.track = nil
	m.mu.Lock()
	var stopping []*encoding
	for key, e := range m.encodings {
		if key.playback() == k.key {
			stopping = append(stopping, e)
			delete(m.encodings, key)
		}
	}
	if m.kept[k.key] == k {
		delete(m.kept, k.key)
	}
	m.mu.Unlock()
	k.mu.Unlock()
	m.logger.Debug("Stopped a kept conversion no play session took over")
	for _, e := range stopping {
		e.stop()
	}
	if t != nil {
		t.stop()
	}
}

// sweepKept ends the kept videos whose encodings all stopped, idle, failed
// or closed, and their tracks.
func (m *Manager) sweepKept() {
	m.mu.Lock()
	var ended []*keeping
	for playback, k := range m.kept {
		alive := false
		for key := range m.encodings {
			if key.playback() == playback {
				alive = true
				break
			}
		}
		if !alive {
			delete(m.kept, playback)
			ended = append(ended, k)
		}
	}
	m.mu.Unlock()
	for _, k := range ended {
		k.mu.Lock()
		k.ended = true
		if k.timer != nil {
			k.timer.Stop()
		}
		t := k.track
		k.track = nil
		k.mu.Unlock()
		if t != nil {
			t.stop()
		}
	}
}

// joined returns segment n of the kept video, or its initialization
// segment for n = -1, joined with its audio, as a file of its own, already
// removed.
func (m *Manager) joined(format Format, n int, video, audio *os.File, t *track) (*os.File, error) {
	v, err := io.ReadAll(video)
	if err != nil {
		return nil, err
	}
	a, err := io.ReadAll(audio)
	if err != nil {
		return nil, err
	}
	var data []byte
	switch {
	case n < 0:
		data, err = joinInit(v, a)
	case format == TS:
		data, err = joinTS(v, a, t.programEntry())
	default:
		data = append(v, a...)
	}
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(m.dir, "joined-*")
	if err != nil {
		return nil, err
	}
	// The file stays readable once removed.
	_ = os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

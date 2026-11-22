package hls

import (
	"context"
	"time"
)

// earlyIdle is how long an encoding started ahead of its play waits for
// it (see Early): a user who opened a title and did not press Play within
// it has likely moved on. It stops between earlyIdle and earlyIdle plus
// the idle check's period (see stopIdle) after it started.
const earlyIdle = 30 * time.Second

// Early starts, ahead of a play, the encoding of key, which the play
// session key names asks for once its app plays: FFmpeg's start, and its
// initialization segment and first segment, read from the source and
// remuxed or converted, after which FFmpeg ends, its read of the source
// with it. An input read within the bytes of that first segment (see
// Remux.Lift) has FFmpeg read nothing more of it meanwhile. The first
// request for key takes it over, as if it had started it: the input is
// read on, the segments made are served, FFmpeg starts again from the
// next, and the encoding counts toward the conversions at once. Until then
// it counts toward no limit and is not listed among those running (see
// Running).
//
// A user's device has one at most: a new one, or a request of that user
// and device for another encoding (another audio track or quality, or
// another title), stops the one before. One no play took over stops after
// earlyIdle, and so does any of its session's when that session stops
// (see Stop and StopEarly). None starts while the server converts video,
// for a playback, a file, a channel or ahead of another play: ErrBusy.
// An encoding of key under way already is left as it is. Early returns
// once the encoding opened, and FFmpeg started.
func (m *Manager) Early(ctx context.Context, key Key, open Opener) error {
	if key.Session == "" {
		return ErrNotFound
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrStopped
	}
	if m.encodings[key] != nil {
		m.mu.Unlock()
		return nil
	}
	before := m.dropEarly(func(k Key, _ *encoding) bool { return k.User == key.User && k.Device == key.Device })
	if m.busyConverting() {
		m.mu.Unlock()
		stopEncodings(before)
		return ErrBusy
	}
	now := time.Now()
	e := &encoding{m: m, key: key, opened: make(chan struct{}), changed: make(chan struct{}), quit: make(chan struct{}),
		playback: &playback{used: now}, early: true, earlyAt: now}
	m.encodings[key] = e
	m.mu.Unlock()
	stopEncodings(before)
	e.open(ctx, open)
	if err := e.waitOpen(ctx); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// A play may have taken it over and started FFmpeg meanwhile.
	if !e.stopped && e.job == nil {
		e.start(e.requested)
		m.logger.Debug("Started an encoding ahead of its play", "version", key.Version, "converts", key.Converts)
	}
	return nil
}

// StopEarly stops the encodings of a play session started ahead of its
// play that no player took over yet (see Early).
func (m *Manager) StopEarly(session string) {
	m.stopWhere(func(k Key, e *encoding) bool { return e.early && k.Session == session })
}

// expireEarly stops the encodings started ahead of a play that no player
// took over within earlyIdle, as of now.
func (m *Manager) expireEarly(now time.Time) {
	m.stopWhere(func(k Key, e *encoding) bool {
		expired := e.early && now.Sub(e.earlyAt) >= earlyIdle
		if expired {
			m.logger.Debug("Stopped an encoding started ahead of a play that did not come", "version", k.Version)
		}
		return expired
	})
}

// dropEarly forgets the encodings started ahead of their play, not taken
// over, that match, and returns them, for the caller to stop once it let
// m.mu go. The caller holds m.mu.
func (m *Manager) dropEarly(match func(Key, *encoding) bool) []*encoding {
	var dropped []*encoding
	for k, e := range m.encodings {
		if e.early && match(k, e) {
			dropped = append(dropped, e)
			delete(m.encodings, k)
		}
	}
	return dropped
}

// busyConverting reports whether the server converts video: for a
// playback, a file, a channel it airs, or ahead of a play. The caller
// holds m.mu.
func (m *Manager) busyConverting() bool {
	if m.converting(nil, nil) > 0 {
		return true
	}
	for k, e := range m.encodings {
		if e.early && (k.Converts || e.converts) {
			return true
		}
	}
	return false
}

// takeOver makes an encoding started ahead of its play the play's. The
// caller holds m.mu.
func (e *encoding) takeOver() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.early = false
	e.m.logger.Debug("A play took over the encoding started ahead of it", "version", e.key.Version, "after", time.Since(e.earlyAt).Round(time.Millisecond))
}

// liftInput lets FFmpeg read on the input of an encoding a play took over,
// read within its first segment's bytes ahead of the play (see
// Remux.Lift), once it is open; an encoding stopped first lifts nothing.
func (e *encoding) liftInput() {
	select {
	case <-e.opened:
	case <-e.quit:
		return
	}
	e.mu.Lock()
	input := e.input
	e.mu.Unlock()
	if input != nil && input.Lift != nil {
		input.Lift()
	}
}

// stopEncodings stops encodings no longer known to the manager.
func stopEncodings(encodings []*encoding) {
	for _, e := range encodings {
		e.stop()
	}
}

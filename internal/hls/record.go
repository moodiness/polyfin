package hls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// userLives counts the live encodings and recordings of user, with the
// oldest of their live encodings, which a new one may replace. The caller
// holds m.mu.
func (m *Manager) userLives(user string) (int, *live) {
	var oldest *live
	mine := 0
	for k, other := range m.lives {
		if k.User == user {
			mine++
			if oldest == nil || other.created.Before(oldest.created) {
				oldest = other
			}
		}
	}
	for k := range m.recordings {
		if k.User == user {
			mine++
		}
	}
	return mine, oldest
}

// Record copies the stream open opens into the MPEG-TS file at path, as it
// comes, until ctx ends or the stream does. A recording takes one of the
// live slots of key.User like a live encoding (see LivePlaylist): past
// userLives, it replaces the user's oldest live encoding, and it starts
// neither past serverLives nor while key records already, which ErrBusy
// reports. It returns nil once the stream ended, ctx's error once ctx
// ended, and FFmpeg's error otherwise.
func (m *Manager) Record(ctx context.Context, key Key, open Opener, path string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrStopped
	}
	if _, running := m.recordings[key]; running {
		m.mu.Unlock()
		return ErrBusy
	}
	mine, oldest := m.userLives(key.User)
	var replaced *live
	switch {
	case mine >= userLives && oldest == nil:
		m.mu.Unlock()
		return ErrBusy
	case mine >= userLives:
		replaced = oldest
		delete(m.lives, replaced.key)
	case len(m.lives)+len(m.recordings) >= serverLives:
		m.mu.Unlock()
		return ErrBusy
	}
	m.recordings[key] = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.recordings, key)
		m.mu.Unlock()
	}()
	if replaced != nil {
		replaced.stop()
	}
	remux, release, err := open(ctx)
	if err != nil {
		return err
	}
	defer release()
	cmd := exec.CommandContext(ctx, m.ffmpeg, remux.recordArgs(path)...)
	// FFmpeg is asked to stop, so that it writes what it holds.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 10 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	m.logger.Debug("Started a recording")
	err = cmd.Run()
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		return fmt.Errorf("FFmpeg failed: %w: %s", err, bytes.TrimSpace(stderr.bytes()))
	}
	return nil
}

// ErrRecording reports a recording file FFmpeg could not finish.
var ErrRecording = errors.New("the recording could not be finished")

// Finish joins the MPEG-TS parts of a recording, in order, into one
// Matroska file at path, its streams copied: a file with an index, which
// players seek in and Polyfin cuts into HLS segments.
func (m *Manager) Finish(ctx context.Context, parts []string, path string) error {
	list, err := os.CreateTemp("", "polyfin-parts-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(list.Name())
	for _, part := range parts {
		// The concat demuxer's list quotes names between single quotes.
		quoted := "'" + strings.ReplaceAll(part, "'", `'\''`) + "'"
		if _, err := fmt.Fprintln(list, "file "+quoted); err != nil {
			_ = list.Close()
			return err
		}
	}
	if err := list.Close(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, m.ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-f", "concat", "-safe", "0", "-i", list.Name(), "-map", "0", "-c", "copy", "-f", "matroska", path)
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("%w: %w: %s", ErrRecording, err, bytes.TrimSpace(stderr.bytes()))
	}
	return nil
}

// recordArgs is FFmpeg's command line copying the stream into path as
// MPEG-TS, which stays readable however the recording stops.
func (r Remux) recordArgs(path string) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}
	args = append(args, r.InputOptions...)
	args = append(args, "-i", r.Input, "-map", "0:"+strconv.Itoa(r.Video))
	if r.Audio >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(r.Audio))
	}
	return append(args, "-map_metadata", "-1", "-c", "copy", "-f", "mpegts", path)
}

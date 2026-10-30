package hls

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// What FFmpeg 9 writes when the NVIDIA driver is older than 570: its API
// is older than 13.0 too, which the FFmpeg for older drivers needs.
const driverOlderThan570 = `[h264_nvenc @ 0x14f60a3c0] Driver does not support the required nvenc API version. Required: 13.1 Found: 12.2
[h264_nvenc @ 0x14f60a3c0] The minimum required Nvidia driver for nvenc is 610.00 or newer
Conversion failed!`

// lockedLog is a log the Manager's goroutines write while a test reads it.
type lockedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// fakeOlderFFmpeg is the FFmpeg for older drivers, a script encoding
// everything, downloaded when release is closed, or failing with err.
type fakeOlderFFmpeg struct {
	path    string
	release chan struct{}
	err     error

	mu      sync.Mutex
	ready   bool
	fetches int
}

func newFakeOlderFFmpeg(t *testing.T, ready bool) *fakeOlderFFmpeg {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return &fakeOlderFFmpeg{path: path, ready: ready, release: make(chan struct{})}
}

func (f *fakeOlderFFmpeg) FFmpegReady() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.path, f.ready
}

func (f *fakeOlderFFmpeg) FetchFFmpeg(ctx context.Context) (string, error) {
	f.mu.Lock()
	f.fetches++
	f.mu.Unlock()
	select {
	case <-f.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if f.err != nil {
		return "", f.err
	}
	f.mu.Lock()
	f.ready = true
	f.mu.Unlock()
	return f.path, nil
}

// downloads counts the downloads asked for.
func (f *fakeOlderFFmpeg) downloads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fetches
}

// eventually waits, 30 s at most, for done to hold.
func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// logged waits, 30 s at most, for a line of log holding every part: the
// background download and detection log after they publish their state.
func logged(t *testing.T, log *lockedLog, parts ...string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for logLine(log.String(), parts...) == "" {
		if time.Now().After(deadline) {
			t.Fatalf("no line with %q in the log: %s", parts, log.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// An NVIDIA driver that NVENC's API 13.0 takes, though the Manager's own
// FFmpeg asks for 13.1, has the FFmpeg for older drivers downloaded,
// video converted on the processor meanwhile, then the GPU taken with
// that FFmpeg, which the Manager runs while that GPU is chosen. No driver
// is then told too old.
func TestAnOlderNVIDIADriverTakesTheFFmpegForOlderDrivers(t *testing.T) {
	var log lockedLog
	m := failingFFmpeg(t, "linux", driverTooOld, &log)
	defer m.Close()
	older := newFakeOlderFFmpeg(t, false)
	m.UseOlderDriverFFmpeg(older)
	m.SelectHardware("nvenc", "")
	if m.Hardware() != nil || m.NVIDIADriver() != "" || m.OlderDriverFFmpeg().State != "downloading" || m.ffmpegPath() != m.ffmpeg {
		t.Fatalf("while downloading: GPU %+v, driver %q, %+v, FFmpeg %s", m.Hardware(), m.NVIDIADriver(), m.OlderDriverFFmpeg(), m.ffmpegPath())
	}
	close(older.release)
	eventually(t, "the GPU's detection with the FFmpeg for older drivers", func() bool { return m.Hardware() != nil })
	hw := m.Hardware()
	if hw.Method != "cuda" || hw.FFmpeg != older.path || strings.Join(hw.Encoders, ",") != "h264_nvenc,hevc_nvenc" {
		t.Errorf("GPU: %+v", hw)
	}
	if m.ffmpegPath() != older.path || m.OlderDriverFFmpeg().State != "used" || m.NVIDIADriver() != "" {
		t.Errorf("FFmpeg %s, %+v, driver %q", m.ffmpegPath(), m.OlderDriverFFmpeg(), m.NVIDIADriver())
	}
	logged(t, &log, "level=INFO", "Video is converted on the GPU, with the FFmpeg for NVIDIA drivers older than 610", older.path)
	// Without a GPU, the Manager runs its own FFmpeg again.
	m.SelectHardware("none", "")
	if m.ffmpegPath() != m.ffmpeg || m.OlderDriverFFmpeg().State != "" {
		t.Errorf("without a GPU: FFmpeg %s, %+v", m.ffmpegPath(), m.OlderDriverFFmpeg())
	}
}

// Downloaded already, the FFmpeg for older drivers is taken at once.
func TestTheFFmpegForOlderDriversDownloadedIsTakenAtOnce(t *testing.T) {
	var log lockedLog
	m := failingFFmpeg(t, "windows", driverTooOld, &log)
	defer m.Close()
	older := newFakeOlderFFmpeg(t, true)
	m.UseOlderDriverFFmpeg(older)
	m.SelectHardware("auto", "")
	if hw := m.Hardware(); hw == nil || hw.FFmpeg != older.path || older.downloads() != 0 {
		t.Errorf("GPU %+v after %d downloads", hw, older.downloads())
	}
}

// A driver older than 570, whose API the FFmpeg for older drivers does not
// take either, downloads nothing: the driver needed is that FFmpeg's.
func TestADriverOlderThan570NeedsDriver570(t *testing.T) {
	var log lockedLog
	m := failingFFmpeg(t, "linux", driverOlderThan570, &log)
	defer m.Close()
	older := newFakeOlderFFmpeg(t, false)
	m.UseOlderDriverFFmpeg(older)
	m.SelectHardware("nvenc", "")
	if m.Hardware() != nil || m.NVIDIADriver() != OlderDriverMinimum || m.OlderDriverFFmpeg().State != "" || older.downloads() != 0 {
		t.Errorf("GPU %+v, driver %q, %+v, %d downloads", m.Hardware(), m.NVIDIADriver(), m.OlderDriverFFmpeg(), older.downloads())
	}
}

// A download that fails is told, and tried again when the GPU is chosen
// again, as when the settings are saved.
func TestAFailedDownloadIsToldAndTriedAgain(t *testing.T) {
	var log lockedLog
	m := failingFFmpeg(t, "linux", driverTooOld, &log)
	defer m.Close()
	older := newFakeOlderFFmpeg(t, false)
	older.err = errors.New("the download does not match its pinned checksum")
	close(older.release)
	m.UseOlderDriverFFmpeg(older)
	m.SelectHardware("nvenc", "")
	eventually(t, "the failed download", func() bool { return m.OlderDriverFFmpeg().State == "failed" })
	if status := m.OlderDriverFFmpeg(); status.Error != older.err.Error() || m.NVIDIADriver() != "" {
		t.Errorf("%+v, driver %q", status, m.NVIDIADriver())
	}
	logged(t, &log, "level=WARN", "could not be downloaded", "pinned checksum")
	older.err = nil
	m.SelectHardware("nvenc", "")
	eventually(t, "the GPU's detection after the second download", func() bool { return m.Hardware() != nil })
	if older.downloads() != 2 {
		t.Errorf("%d downloads", older.downloads())
	}
}

// NVENC failing with the FFmpeg for older drivers too is told with why.
func TestNVENCFailingWithTheFFmpegForOlderDriversIsTold(t *testing.T) {
	var log lockedLog
	m := failingFFmpeg(t, "linux", driverTooOld, &log)
	defer m.Close()
	older := newFakeOlderFFmpeg(t, true)
	if err := os.WriteFile(older.path, []byte("#!/bin/sh\necho 'OpenEncodeSessionEx failed: no encode device' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	m.UseOlderDriverFFmpeg(older)
	m.SelectHardware("nvenc", "")
	status := m.OlderDriverFFmpeg()
	if m.Hardware() != nil || status.State != "unusable" || !strings.HasPrefix(status.Error, "FFmpeg failed") || !strings.Contains(status.Error, "no encode device") ||
		m.NVIDIADriver() != "" {
		t.Errorf("GPU %+v, %+v, driver %q", m.Hardware(), status, m.NVIDIADriver())
	}
	if logLine(log.String(), "level=WARN", "hwaccel=nvenc", "with the FFmpeg for NVIDIA drivers older than 610", "no encode device") == "" {
		t.Errorf("log: %s", log.String())
	}
}

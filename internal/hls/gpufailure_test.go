package hls

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// What FFmpeg 9 writes when NVENC fails at startup, -loglevel error: with
// an NVIDIA driver older than its NVENC needs, and on a computer with the
// NVIDIA driver but without its NVENC library, as a container given the
// GPU without the video capability.
const (
	driverTooOld = `[h264_nvenc @ 0x14f60a3c0] Driver does not support the required nvenc API version. Required: 13.1 Found: 13.0
[h264_nvenc @ 0x14f60a3c0] The minimum required Nvidia driver for nvenc is 610.00 or newer
[vost#0:0/h264_nvenc @ 0x14f6098c0] Error while opening encoder - maybe incorrect parameters such as bit_rate, rate, width or height.
[vf#0:0 @ 0x14f609b40] Error sending frames to consumers: Function not implemented
[vf#0:0 @ 0x14f609b40] Task finished with error code: -38 (Function not implemented)
[vf#0:0 @ 0x14f609b40] Terminating thread with return code -38 (Function not implemented)
[vost#0:0/h264_nvenc @ 0x14f6098c0] Could not open encoder before EOF
[out#0/null @ 0x14f608f00] Nothing was written into output file, because at least one of its streams received no packets.
Conversion failed!`
	noEncodeLibrary = `[h264_nvenc @ 0x5581c2a4e0] Cannot load libnvidia-encode.so.1
[h264_nvenc @ 0x5581c2a4e0] The minimum required Nvidia driver for nvenc is 610.00 or newer
[vost#0:0/h264_nvenc @ 0x5581c29fc0] Error while opening encoder - maybe incorrect parameters such as bit_rate, rate, width or height.
Conversion failed!`
)

// failingFFmpeg is a Manager on system whose FFmpeg, a script, fails every
// encode: NVENC's writing nvenc to its standard error, the others' that
// no device is there. It logs into log.
func failingFFmpeg(t *testing.T, system, nvenc string, log *bytes.Buffer) *Manager {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake FFmpeg is a shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncase \"$*\" in\n*_nvenc*)\n\tcat >&2 <<'EOF'\n"+nvenc+"\nEOF\n\texit 1 ;;\nesac\n"+
		"echo 'No device available for the encoder' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return &Manager{system: system, ffmpeg: script, dir: t.TempDir(), logger: slog.New(slog.NewTextHandler(log, nil)),
		detected: map[string]*Hardware{}, done: make(chan struct{}), can: capabilities{encoders: everyEncoder, filters: everyFilter}}
}

// logLine is the line of log holding every part, empty for none.
func logLine(log string, parts ...string) string {
	for line := range strings.Lines(log) {
		found := true
		for _, part := range parts {
			found = found && strings.Contains(line, part)
		}
		if found {
			return line
		}
	}
	return ""
}

// An NVIDIA driver older than FFmpeg's NVENC needs is logged with FFmpeg's
// reason and NVENC's method, at the level of the line telling what video
// is converted on, and Polyfin tells the driver version needed, for
// System › Health, for as long as NVENC is the choice or among those
// Automatic tried.
func TestAnNVIDIADriverTooOldIsLoggedAndTold(t *testing.T) {
	for _, tc := range []struct{ system, want, level string }{
		{"windows", "auto", "level=INFO"},
		{"linux", "nvenc", "level=WARN"},
	} {
		var log bytes.Buffer
		m := failingFFmpeg(t, tc.system, driverTooOld, &log)
		m.SelectHardware(tc.want, "/dev/dri/renderD128")
		got := m.NVIDIADriver()
		m.SelectHardware("none", "")
		none := m.NVIDIADriver()
		m.Close()
		if line := logLine(log.String(), "hwaccel=nvenc", "Driver does not support the required nvenc API version",
			"The minimum required Nvidia driver for nvenc is 610.00 or newer"); !strings.HasPrefix(line, "time=") || !strings.Contains(line, tc.level) {
			t.Errorf("%s, %s: no %s line with FFmpeg's reason: %s", tc.system, tc.want, tc.level, log.String())
		}
		if m.Hardware() != nil || got != "610.00" || none != "" {
			t.Errorf("%s, %s: GPU %+v, driver needed %q, then %q without a GPU", tc.system, tc.want, m.Hardware(), got, none)
		}
	}
}

// NVENC failing for another reason, here its library missing, which FFmpeg
// follows with the minimum driver too, is logged with FFmpeg's reason,
// and tells no driver too old.
func TestOtherNVENCFailuresAreLoggedOnly(t *testing.T) {
	var log bytes.Buffer
	m := failingFFmpeg(t, "linux", noEncodeLibrary, &log)
	m.SelectHardware("auto", "/dev/dri/renderD128")
	m.Close()
	if logLine(log.String(), "level=INFO", "hwaccel=nvenc", "Cannot load libnvidia-encode.so.1") == "" {
		t.Errorf("no line with FFmpeg's reason: %s", log.String())
	}
	if logLine(log.String(), "level=INFO", "hwaccel=vaapi", "/dev/dri/renderD128", "No device available for the encoder") == "" {
		t.Errorf("no line with VAAPI's reason: %s", log.String())
	}
	if driver := m.NVIDIADriver(); driver != "" {
		t.Errorf("driver needed %q", driver)
	}
}

// What FFmpeg 9 writes, aborting, when VAAPI starts on a system without
// X11's library, as Polyfin's arm64 image, before it opens the device.
const noX11 = `implib-gen: libX11.so.6: failed to load library 'libX11.so.6' via dlopen: libX11.so.6: cannot open shared object file: No such file or directory
ffmpeg: libX11.so.6.tramp.S:42: _libX11_so_6_tramp_resolve: Assertion ` + "`0 && \"Assertion in generated code\"'" + ` failed.`

// On arm64, where Polyfin's image has no VAAPI, VAAPI asked for on a
// system without libva is told as not offered, without trying FFmpeg;
// with libva, and on amd64 whatever the system, FFmpeg's own reason is
// logged.
func TestVAAPIIsNotOfferedOnArm64WithoutLibva(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake FFmpeg is a shell script")
	}
	for _, tc := range []struct {
		arch        string
		libva       bool
		want, tried string
	}{
		{"arm64", false, "VAAPI is not offered here: libva is not installed", ""},
		{"arm64", true, "libX11.so.6", "vaapi"},
		{"amd64", false, "libX11.so.6", "vaapi"},
	} {
		dir := t.TempDir()
		script, tried := filepath.Join(dir, "ffmpeg"), filepath.Join(dir, "tried")
		if err := os.WriteFile(script, []byte("#!/bin/sh\ncase \"$*\" in\n*_vaapi*)\n\techo vaapi >> "+tried+"\n\tcat >&2 <<'EOF'\n"+noX11+
			"\nEOF\n\texit 134 ;;\nesac\necho 'No device available for the encoder' >&2\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		libraries := t.TempDir()
		if tc.libva {
			if err := os.WriteFile(filepath.Join(libraries, "libva.so.2"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		var log bytes.Buffer
		m := &Manager{system: "linux", arch: tc.arch, libraryDirs: []string{libraries}, ffmpeg: script, dir: t.TempDir(),
			logger: slog.New(slog.NewTextHandler(&log, nil)), detected: map[string]*Hardware{}, done: make(chan struct{}),
			can: capabilities{encoders: everyEncoder, filters: everyFilter}}
		m.SelectHardware("vaapi", "/dev/dri/renderD129")
		m.Close()
		ran, _ := os.ReadFile(tried)
		if line := logLine(log.String(), "level=WARN", "hwaccel=vaapi", tc.want); line == "" || strings.Contains(string(ran), "vaapi") != (tc.tried != "") {
			t.Errorf("%s, libva %v: FFmpeg tried %q, log %s", tc.arch, tc.libva, ran, log.String())
		}
	}
}

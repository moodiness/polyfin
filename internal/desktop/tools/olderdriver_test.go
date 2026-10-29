package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The FFmpeg for older NVIDIA drivers is downloaded, checked and unpacked
// into its own folder, beside Polyfin Desktop's FFmpeg, and neither
// download removes the other's: both are folders ffmpeg-<version> of the
// tools folder otherwise. A download that does not match its checksum is
// refused.
func TestTheFFmpegForOlderDriversKeepsToItsFolder(t *testing.T) {
	main := tarXZOf(t, []entry{{name: "ffmpeg-n9.0.2-linux64-gpl-shared-9.0/bin/ffmpeg", content: "the FFmpeg for API 13.1"},
		{name: "ffmpeg-n9.0.2-linux64-gpl-shared-9.0/bin/ffprobe", content: "ffprobe"}})
	older := tarXZOf(t, []entry{{name: "ffmpeg-n9.0.2-linux64-gpl-shared-9.0-nvenc13.0/bin/ffmpeg", content: "the FFmpeg for API 13.0"},
		{name: "ffmpeg-n9.0.2-linux64-gpl-shared-9.0-nvenc13.0/bin/ffprobe", content: "ffprobe"},
		{name: "ffmpeg-n9.0.2-linux64-gpl-shared-9.0-nvenc13.0/lib/libavcodec.so.63", content: "libavcodec"}})
	server := newFake(t, map[string][]byte{"/main.tar.xz": main, "/older.tar.xz": older})
	dir := t.TempDir()
	mainFile := pin(server.URL+"/main.tar.xz", main, "tar.xz")
	mainFile.Strip = 1
	desktop := Tools{Dir: dir, Manifest: Manifest{Version: "9.0.2", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{mainFile}}}
	olderFile := pin(server.URL+"/older.tar.xz", older, "tar.xz")
	olderFile.Strip = 1
	manifest := Manifest{Version: "9.0.2-nvenc13.0", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{olderFile}}
	nvenc13 := OlderDriver(dir, manifest)

	if _, ok := nvenc13.FFmpegReady(); ok {
		t.Fatal("ready before its download")
	}
	if _, err := desktop.Ensure(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	path, err := nvenc13.FetchFFmpeg(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "the FFmpeg for API 13.0" || filepath.Dir(filepath.Dir(filepath.Dir(path))) != filepath.Join(dir, "nvenc13") {
		t.Errorf("%s holds %q", path, got)
	}
	if ready, ok := nvenc13.FFmpegReady(); !ok || ready != path {
		t.Errorf("ready: %s %v", ready, ok)
	}
	if _, err := os.Stat(filepath.Join(dir, "nvenc13", "ffmpeg-9.0.2-nvenc13.0", "lib", "libavcodec.so.63")); err != nil {
		t.Errorf("its libraries: %v", err)
	}
	// Polyfin Desktop's FFmpeg, downloaded again, leaves it in place.
	if err := os.RemoveAll(filepath.Join(dir, "ffmpeg-9.0.2")); err != nil {
		t.Fatal(err)
	}
	if paths, err := desktop.Ensure(t.Context(), nil); err != nil || read(t, paths.FFmpeg) != "the FFmpeg for API 13.1" {
		t.Fatalf("Polyfin Desktop's FFmpeg: %+v %v", paths, err)
	}
	if _, ok := nvenc13.FFmpegReady(); !ok {
		t.Error("the FFmpeg for older drivers was removed")
	}

	tampered := nvenc13
	tampered.Dir = t.TempDir()
	tampered.Manifest.Files = []File{olderFile}
	tampered.Manifest.Files[0].SHA256 = mainFile.SHA256
	if _, err := tampered.FetchFFmpeg(t.Context()); !errors.Is(err, ErrChecksum) {
		t.Errorf("a download that does not match: %v", err)
	}
	if _, ok := tampered.FFmpegReady(); ok {
		t.Error("a download that does not match is ready")
	}
}

// A manifest file named by POLYFIN_NVENC13_TOOLS replaces the pinned
// downloads, as for tests against a local fake.
func TestTheVariableReplacesTheOlderDriverPins(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nvenc13.json")
	if err := os.WriteFile(file, []byte(`{"version":"9.0.2-nvenc13.0","ffmpeg":"bin/ffmpeg","ffprobe":"bin/ffprobe",
		"files":[{"url":"http://127.0.0.1:1/older.tar.xz","sha256":"`+fill("b")+`","size":3,"format":"tar.xz","strip":1}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(OlderDriverVariable, file)
	m, ok, err := OlderDriverManifest("darwin", "arm64")
	if err != nil || !ok || m.Version != "9.0.2-nvenc13.0" || m.Files[0].Strip != 1 {
		t.Errorf("OlderDriverManifest = %+v %v %v", m, ok, err)
	}
}

func fill(digit string) string {
	s := ""
	for range 64 {
		s += digit
	}
	return s
}

// Every system whose NVIDIA driver runs NVENC has the FFmpeg for older
// drivers pinned, built from the same FFmpeg as the Docker image's: the
// release named after its build, holding its version.
func TestTheFFmpegForOlderDriversIsPinnedWithTheImagesBuild(t *testing.T) {
	tag, rest, _ := strings.Cut(strings.TrimPrefix(btbn, "https://github.com/BtbN/FFmpeg-Builds/releases/download/"), "/")
	for _, system := range []string{"linux/amd64", "linux/arm64", "windows/amd64"} {
		goos, goarch, _ := strings.Cut(system, "/")
		m, ok, err := OlderDriverManifest(goos, goarch)
		if err != nil || !ok {
			t.Errorf("%s has no pinned FFmpeg for older drivers: %v", system, err)
			continue
		}
		if err := m.validate(); err != nil {
			t.Errorf("%s: %v", system, err)
		}
		want := "https://github.com/moodiness/polyfin/releases/download/ffmpeg-nvenc13.0-" + tag + "/" + rest
		for _, f := range m.Files {
			if !strings.HasPrefix(f.URL, want) || !strings.HasSuffix(strings.TrimSuffix(strings.TrimSuffix(f.URL, ".zip"), ".tar.xz"), "-9.0-nvenc13.0") ||
				f.Size <= 0 || f.Strip != 1 {
				t.Errorf("%s: %+v, want a build of %s", system, f, want)
			}
		}
	}
	// NVENC runs on no other system.
	for _, system := range []string{"windows/arm64", "darwin/arm64", "darwin/amd64"} {
		goos, goarch, _ := strings.Cut(system, "/")
		if _, ok, _ := OlderDriverManifest(goos, goarch); ok {
			t.Errorf("%s has an FFmpeg for older NVIDIA drivers", system)
		}
	}
}

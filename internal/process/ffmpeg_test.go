package process

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testFFmpeg is the FFmpeg of POLYFIN_TEST_FFMPEG; the test is skipped
// without one.
func testFFmpeg(t *testing.T) string {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	return ffmpeg
}

// FFmpeg writes its progress to pipe:3, which reads it to its end.
func TestFFmpegWritesItsProgressToAnOutput(t *testing.T) {
	cmd := exec.Command(testFFmpeg(t), "-hide_banner", "-nostdin", "-loglevel", "error", "-progress", "pipe:3",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-t", "1", "-f", "null", "-")
	outputs, err := NewOutputs(cmd, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = cmd.Start()
	outputs.Started(cmd, err)
	if err != nil {
		t.Fatal(err)
	}
	progress := readAll(t, outputs.Readers[0])
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(progress, "progress=end") {
		t.Errorf("progress %q", progress)
	}
}

// FFmpeg recording a source that flows stops at its first interrupt and
// ends its file, as a recording's does.
func TestFFmpegStopsOnItsInterrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.ts")
	cmd := exec.Command(testFFmpeg(t), "-hide_banner", "-nostdin", "-loglevel", "error", "-progress", "pipe:1", "-re",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-c:v", "mpeg2video", "-f", "mpegts", path)
	interrupt, err := Interruptible(cmd)
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Once FFmpeg wrote some of the recording, it is interrupted.
	lines := bufio.NewScanner(stdout)
	for lines.Scan() && !strings.HasPrefix(lines.Text(), "frame=") {
	}
	if err := interrupt(); err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	go func() {
		for lines.Scan() {
		}
		ended <- cmd.Wait()
	}()
	select {
	case <-ended:
	case <-time.After(time.Minute):
		_ = cmd.Process.Kill()
		t.Fatal("FFmpeg did not stop at its interrupt")
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Errorf("the recording: %v %v", info, err)
	}
}

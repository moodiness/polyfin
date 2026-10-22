package hls

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A text subtitle is drawn onto the converted video by libass, with the
// path escaped for FFmpeg's filter graph.
func TestTextBurnGraph(t *testing.T) {
	t.Parallel()
	v := VideoEncoding{Encoder: "libx264", Width: 640, Height: 360, BurnText: "/data/cache/burned-subtitles/a:b.srt", FontsDir: "/usr/share/fonts"}
	graph := v.burnGraph(0, 0, "")
	for _, want := range []string{"subtitles=filename=/data/cache/burned-subtitles/a\\:b.srt", ":fontsdir=/usr/share/fonts", ":force_style=FontName=DejaVu Sans\\,Bold=1", "[video]"} {
		if !strings.Contains(graph, want) {
			t.Errorf("graph %q lacks %q", graph, want)
		}
	}
	v.StyledText = true
	if strings.Contains(v.burnGraph(0, 0, ""), "force_style") {
		t.Error("an ASS script's styles are overridden")
	}
	if !v.Burns() {
		t.Error("a text subtitle is not burned")
	}
	// FFmpeg runs the graph, when it is installed with libass.
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "s.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:02,000\nHello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v = VideoEncoding{Encoder: "libx264", Width: 640, Height: 360, BurnText: srt}
	out := filepath.Join(dir, "out.mp4")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24", "-t", "1",
		"-filter_complex", v.textBurnGraph(0, ""), "-map", "[video]", "-c:v", "libx264", "-preset", "ultrafast", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, output)
	}
}

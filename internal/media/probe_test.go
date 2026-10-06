package media

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseReadsWhatTheRecordedClipsLack(t *testing.T) {
	analysis, err := Parse([]byte(`{
		"format": {"filename": "http://127.0.0.1:41234/source/abc", "format_name": "matroska,webm", "duration": "7200.5", "size": "40000000000", "bit_rate": "44444444"},
		"streams": [
			{"index": 0, "codec_type": "video", "codec_name": "hevc", "profile": "Main 10", "pix_fmt": "yuv420p10le", "width": 3840, "height": 2160,
			 "r_frame_rate": "24000/1001", "avg_frame_rate": "24000/1001", "disposition": {"default": 1},
			 "tags": {"BPS": "39000000", "language": "eng"},
			 "side_data_list": [
				{"side_data_type": "DOVI configuration record", "dv_version_major": 1, "dv_version_minor": 0, "dv_profile": 8, "dv_level": 6,
				 "rpu_present_flag": 1, "el_present_flag": 0, "bl_present_flag": 1, "dv_bl_signal_compatibility_id": 1},
				{"side_data_type": "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"}
			 ]},
			{"index": 1, "codec_type": "audio", "codec_name": "eac3", "channels": 6, "sample_rate": "48000", "disposition": {"forced": 0},
			 "tags": {"language": "fre", "title": "VFF"}},
			{"index": 2, "codec_type": "attachment", "codec_name": "ttf", "codec_tag_string": "[0][0][0][0]",
			 "tags": {"filename": "Sign.ttf", "mimetype": "application/x-truetype-font", "comment": "Signs"}}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !analysis.Remote || analysis.Duration != 2*time.Hour+500*time.Millisecond || analysis.Size != 40_000_000_000 {
		t.Errorf("format: %+v", analysis)
	}
	video := analysis.Streams[0]
	if video.Bitrate != 39_000_000 || video.BitDepth != 10 || !video.Default || video.FrameRate < 23.97 || video.FrameRate > 23.98 {
		t.Errorf("video: %+v", video)
	}
	want := DolbyVision{VersionMajor: 1, Profile: 8, Level: 6, Compatibility: 1, RPU: true, BL: true}
	if video.DolbyVision == nil || *video.DolbyVision != want || !video.HDR10Plus {
		t.Errorf("dynamic range: %+v, HDR10+ %v", video.DolbyVision, video.HDR10Plus)
	}
	if audio := analysis.Streams[1]; audio.Language != "fre" || audio.Title != "VFF" || audio.SampleRate != 48000 {
		t.Errorf("audio: %+v", audio)
	}
	// Fonts an ASS track uses are listed to apps by file name and type.
	if font := analysis.Streams[2]; font.Type != "attachment" || font.FileName != "Sign.ttf" ||
		font.MimeType != "application/x-truetype-font" || font.Comment != "Signs" {
		t.Errorf("attachment: %+v", font)
	}
}

func TestParseRefusesWhatIsNotAVideo(t *testing.T) {
	for name, output := range map[string]string{
		"a web page":       `{"format": {"format_name": "html"}, "streams": []}`,
		"music with cover": `{"format": {"format_name": "mp3"}, "streams": [{"index": 0, "codec_type": "audio", "codec_name": "mp3"}, {"index": 1, "codec_type": "video", "codec_name": "mjpeg", "disposition": {"attached_pic": 1}}]}`,
		"not ffprobe":      `Invalid data found when processing input`,
	} {
		if _, err := Parse([]byte(output)); !errors.Is(err, ErrNotMedia) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The clip Jellyfin's chapters were recorded from, as upstream ffprobe sees
// it.
func TestParseReadsChapters(t *testing.T) {
	probe, err := os.ReadFile(filepath.Join("..", "jellyfin", "testdata", "jellyfin-12.2", "chapters", "probe.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	want := []Chapter{
		{Start: 0, End: 2500400 * time.Microsecond, Title: "Opening"},
		{Start: 2500400 * time.Microsecond, End: 5 * time.Second},
		{Start: 5 * time.Second, End: 7500 * time.Millisecond, Title: "00:05:00.000"},
		{Start: 7500 * time.Millisecond, End: 10 * time.Second, Title: "7"},
	}
	if !slices.Equal(analysis.Chapters, want) {
		t.Errorf("chapters:\n got %+v\nwant %+v", analysis.Chapters, want)
	}
}

// Probe asks ffprobe for the chapters of a source it reads over HTTP.
func TestProbeFindsChapters(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	metadata := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(metadata, []byte(";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1000\ntitle=Intro\n"+
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=1000\nEND=2000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(dir, "clip.mkv")
	command := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10:duration=2",
		"-i", metadata, "-map", "0:v", "-map_chapters", "1", "-c:v", "mpeg4", clip)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, output)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, clip) }))
	defer server.Close()
	prober := Prober{Path: filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), Timeout: time.Minute}
	analysis, err := prober.Probe(t.Context(), server.URL+"/clip.mkv")
	if err != nil {
		t.Fatal(err)
	}
	want := []Chapter{{Start: 0, End: time.Second, Title: "Intro"}, {Start: time.Second, End: 2 * time.Second}}
	if !slices.Equal(analysis.Chapters, want) {
		t.Errorf("chapters:\n got %+v\nwant %+v", analysis.Chapters, want)
	}
}

// A live analysis reads about a second and never reconnects: a stream that
// ends while it is read fails at once; a file's analysis reconnects.
func TestLiveAnalysesNeverReconnect(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	path := filepath.Join(dir, "ffprobe")
	script := "#!/bin/sh\necho \"$*\" > " + args + "\necho '{\"streams\": [{\"index\": 0, \"codec_type\": \"video\", \"codec_name\": \"h264\"}], \"format\": {}}'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	prober := Prober{Path: path, Timeout: 5 * time.Second}
	read := func() string {
		data, err := os.ReadFile(args)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if _, err := prober.ProbeLive(t.Context(), "http://127.0.0.1:1/live.ts"); err != nil {
		t.Fatal(err)
	}
	if live := read(); strings.Contains(live, "-reconnect") || !strings.Contains(live, "-probesize 1M -analyzeduration 1M") {
		t.Errorf("a live analysis: %s", live)
	}
	if _, err := prober.Probe(t.Context(), "http://127.0.0.1:1/movie.mkv"); err != nil {
		t.Fatal(err)
	}
	if file := read(); !strings.Contains(file, "-reconnect 1") {
		t.Errorf("a file's analysis: %s", file)
	}
}

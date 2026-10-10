package playback

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// fakeFrameProbe is an ffprobe script answering a first frame with the
// mastering display, or failing, and counting its runs in a file.
func fakeFrameProbe(t *testing.T, fails bool) (path string, runs func() int) {
	t.Helper()
	dir := t.TempDir()
	count := filepath.Join(dir, "runs")
	script := "#!/bin/sh\necho run >> " + count + "\n"
	if fails {
		script += "exit 1\n"
	} else {
		script += `echo '{"frames": [{"stream_index": 0, "side_data_list": [{"side_data_type": "Mastering display metadata"}]}]}'` + "\n"
	}
	path = filepath.Join(dir, "ffprobe")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path, func() int {
		data, _ := os.ReadFile(count)
		return strings.Count(string(data), "run")
	}
}

// An analysis made before Polyfin recorded the mastering display has the
// first frame of its HDR10 video read once, only when Intel's GPU would
// tone map it, and keeps the answer; a frame that cannot be read leaves
// the version on the processor without being read again at once.
func TestTheMasteringDisplayIsLearnedOnceForIntel(t *testing.T) {
	intel := &hls.Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, ToneMapping: true}
	nvidia := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true}
	onCPU := func(hw *hls.Hardware) *VideoConversion {
		return &VideoConversion{Codec: "h264", Encoder: hw.Encoders[0], ToneMap: true, ToneMapOnCPU: true, Hardware: hw}
	}
	old := media.Analysis{Format: "matroska,webm", Streams: []media.Stream{{Index: 0, Type: "video", Codec: "hevc", ColorTransfer: "smpte2084"}}}

	ffprobe, runs := fakeFrameProbe(t, false)
	s := newService(t, &fakeSource{body: []byte("\x1a\x45\xdf\xa3")}, ffprobe, nil)
	version := library.Version{ID: accounts.ID{7}, URL: "https://93.184.216.34/movie.mkv"}
	data, _ := json.Marshal(old)
	if _, err := s.db.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
		t.Fatal(err)
	}
	for _, conversion := range []*VideoConversion{nil, onCPU(nvidia), {Codec: "h264", Encoder: "h264_vaapi", ToneMap: true, Hardware: intel}} {
		if _, ok := s.LearnMasteringDisplay(t.Context(), version, old, conversion); ok || runs() != 0 {
			t.Errorf("%+v: learned %v after %d runs", conversion, ok, runs())
		}
	}
	learned, ok := s.LearnMasteringDisplay(t.Context(), version, old, onCPU(intel))
	if got := learned.Streams[0].MasteringDisplay; !ok || got == nil || !*got || runs() != 1 || old.Streams[0].MasteringDisplay != nil {
		t.Fatalf("learned %v: %v after %d runs", ok, got, runs())
	}
	s.analyses.Delete(version.ID)
	if stored, _ := s.Analyzed(t.Context(), version.ID); stored.Streams[0].MasteringDisplay == nil || !*stored.Streams[0].MasteringDisplay {
		t.Errorf("not kept with the analysis: %+v", stored.Streams[0])
	}
	if _, ok := s.LearnMasteringDisplay(t.Context(), version, learned, onCPU(intel)); ok || runs() != 1 {
		t.Errorf("read again: %v after %d runs", ok, runs())
	}

	failing, failed := fakeFrameProbe(t, true)
	s = newService(t, &fakeSource{body: []byte("\x1a\x45\xdf\xa3")}, failing, nil)
	for range 2 {
		if got, ok := s.LearnMasteringDisplay(t.Context(), version, old, onCPU(intel)); ok || got.Streams[0].MasteringDisplay != nil {
			t.Errorf("a frame that could not be read: %v %+v", ok, got.Streams[0])
		}
	}
	if failed() != 1 {
		t.Errorf("a failing frame read %d times", failed())
	}
}

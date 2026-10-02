package media

import (
	"errors"
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
			 "tags": {"language": "fre", "title": "VFF"}}
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

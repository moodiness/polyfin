package media

import (
	"errors"
	"testing"
	"time"
)

func TestParseAudioTakesMusicFilesThatParseRefuses(t *testing.T) {
	output := []byte(`{"format": {"filename": "http://127.0.0.1:41234/source/abc", "format_name": "mp3", "duration": "212.4", "bit_rate": "320000"},
		"streams": [{"index": 0, "codec_type": "audio", "codec_name": "mp3", "channels": 2, "sample_rate": "44100"},
			{"index": 1, "codec_type": "video", "codec_name": "mjpeg", "disposition": {"attached_pic": 1}}]}`)
	if _, err := Parse(output); !errors.Is(err, ErrNotMedia) {
		t.Errorf("Parse took a music file: %v", err)
	}
	analysis, err := ParseAudio(output)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Format != "mp3" || analysis.Duration != 212*time.Second+400*time.Millisecond || analysis.Bitrate != 320_000 ||
		len(analysis.Streams) != 2 || analysis.Streams[0].Codec != "mp3" || analysis.Streams[0].SampleRate != 44100 || !analysis.Streams[1].AttachedPicture {
		t.Errorf("analysis: %+v", analysis)
	}
}

func TestParseAudioRefusesWhatHoldsNoAudio(t *testing.T) {
	for name, output := range map[string]string{
		"a web page":     `{"format": {"format_name": "html"}, "streams": []}`,
		"a silent video": `{"format": {"format_name": "mp4"}, "streams": [{"index": 0, "codec_type": "video", "codec_name": "h264"}]}`,
		"garbage":        `not json`,
	} {
		if _, err := ParseAudio([]byte(output)); !errors.Is(err, ErrNotMedia) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

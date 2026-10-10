package admin

import (
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
)

func TestSettingsConversion(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		ffmpeg = "ffmpeg-not-installed"
	}
	encoder, err := hls.NewManager(ffmpeg, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(encoder.Close)
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health.Encoder = encoder
	})
	administrator := api.signedIn("administrator", true)
	keys := []string{"encoderPreset", "h264Quality", "hevcQuality", "allowHevcEncoding", "hardwareAcceleration", "hardwareDecodingCodecs",
		"toneMapping", "toneMappingAlgorithm", "toneMappingPeak", "toneMappingDesat", "gpuToneMapping", "processorToneMappingHeight",
		"deinterlaceMethod", "deinterlaceDoubleRate",
		"downmixAlgorithm", "downmixBoost", "maxAudioChannels", "audioBitratePerChannel", "encodingThreads", "aheadSeconds"}
	tuning := func(body map[string]any) map[string]any {
		got := map[string]any{}
		for _, key := range keys {
			got[key] = body[key]
		}
		return got
	}
	defaults := map[string]any{"encoderPreset": "auto", "h264Quality": 0.0, "hevcQuality": 0.0, "allowHevcEncoding": false, "hardwareAcceleration": "auto",
		"hardwareDecodingCodecs": []any{"h264", "hevc", "hevc_10bit", "vp9", "av1", "mpeg2video", "vc1"}, "toneMapping": true, "toneMappingAlgorithm": "auto",
		"toneMappingPeak": 0.0, "toneMappingDesat": 0.0, "gpuToneMapping": true, "processorToneMappingHeight": 0.0,
		"deinterlaceMethod": "yadif", "deinterlaceDoubleRate": false, "downmixAlgorithm": "None",
		"downmixBoost": 1.0, "maxAudioChannels": 0.0, "audioBitratePerChannel": 0.0, "encodingThreads": 0.0, "aheadSeconds": 120.0}
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	if got := tuning(body); !reflect.DeepEqual(got, defaults) {
		t.Errorf("defaults: %v", got)
	}
	// What conversions run on shows, read only: no GPU here, and the
	// software encoders FFmpeg has.
	detected, _ := body["conversionHardware"].(map[string]any)
	if _, found := detected["default"]; found || detected["gpu"] != nil {
		t.Errorf("conversion hardware: %v", detected)
	}
	if want := slices.Contains(encoder.Encoders(), "libx264"); slices.Contains(detected["encoders"].([]any), any("libx264")) != want {
		t.Errorf("software encoders: %v, libx264 %v", detected["encoders"], want)
	}
	// The height Automatic gives HDR tone mapped on the processor: 720p
	// until a timing chooses, none here, and nothing without the filters.
	wantHeight := 0.0
	if encoder.HasFilters("zscale", "tonemap") {
		wantHeight = 720
	}
	if detected["toneMappingHeight"] != wantHeight {
		t.Errorf("tone mapped height: %v, want %v", detected["toneMappingHeight"], wantHeight)
	}

	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	changed := map[string]any{"encoderPreset": "slow", "h264Quality": 21.0, "hevcQuality": 26.0, "allowHevcEncoding": true, "hardwareAcceleration": "none",
		"hardwareDecodingCodecs": []any{"h264", "av1"}, "toneMapping": false, "toneMappingAlgorithm": "reinhard", "toneMappingPeak": 1000.0,
		"toneMappingDesat": 0.5, "gpuToneMapping": false, "processorToneMappingHeight": 1440.0,
		"deinterlaceMethod": "bwdif", "deinterlaceDoubleRate": true, "downmixAlgorithm": "Rfc7845", "downmixBoost": 2.5,
		"maxAudioChannels": 2.0, "audioBitratePerChannel": 96.0, "encodingThreads": 8.0, "aheadSeconds": 300.0}
	put := maps.Clone(base)
	maps.Copy(put, changed)
	if status, body, _ := administrator.call(http.MethodPut, "/settings", put); status != http.StatusOK || !reflect.DeepEqual(tuning(body), changed) {
		t.Fatalf("saving: %d %v", status, tuning(body))
	}
	if got := api.store.Settings(); got.EncoderPreset != "slow" || got.DownmixBoost != 2.5 || !slices.Equal(got.HardwareDecodingCodecs, []string{"h264", "av1"}) {
		t.Errorf("stored: %+v", got)
	}
	// A page or script older than them leaves them as they are.
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || !reflect.DeepEqual(tuning(body), changed) {
		t.Errorf("saving without them: %d %v", status, tuning(body))
	}
	for _, tc := range []struct {
		key   string
		value any
		code  string
	}{
		{"encoderPreset", "placebo", "invalid_encoder_preset"},
		{"h264Quality", 52, "invalid_video_quality"},
		{"hevcQuality", -1, "invalid_video_quality"},
		{"hardwareAcceleration", "qsv", "invalid_hardware_acceleration"},
		{"hardwareAcceleration", "", "invalid_hardware_acceleration"},
		{"hardwareDecodingCodecs", []string{"vp8"}, "invalid_hardware_decoding_codecs"},
		{"toneMappingAlgorithm", "gamma", "invalid_tone_mapping_algorithm"},
		{"toneMappingPeak", 50, "invalid_tone_mapping_peak"},
		{"toneMappingDesat", 11, "invalid_tone_mapping_desat"},
		{"processorToneMappingHeight", 480, "invalid_processor_tone_mapping_height"},
		{"processorToneMappingHeight", 4320, "invalid_processor_tone_mapping_height"},
		{"deinterlaceMethod", "w3fdif", "invalid_deinterlace_method"},
		{"downmixAlgorithm", "Stereo", "invalid_downmix_algorithm"},
		{"downmixBoost", 4, "invalid_downmix_boost"},
		{"maxAudioChannels", 8, "invalid_max_audio_channels"},
		{"audioBitratePerChannel", 1000, "invalid_audio_bitrate_per_channel"},
		{"encodingThreads", 65, "invalid_encoding_threads"},
		{"aheadSeconds", 29, "invalid_ahead_seconds"},
		{"aheadSeconds", 601, "invalid_ahead_seconds"},
	} {
		refused := maps.Clone(base)
		refused[tc.key] = tc.value
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%s %v: %d %v", tc.key, tc.value, status, body)
		}
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); !reflect.DeepEqual(tuning(body), changed) {
		t.Errorf("a refused value changed the settings: %v", tuning(body))
	}
}

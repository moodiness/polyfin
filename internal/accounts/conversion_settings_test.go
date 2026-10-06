package accounts

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

// The settings tuning conversions start as Polyfin converted before they
// were settings, and the administrator's setup keeps them.
func TestConversionSettingsDefaultToWhatPolyfinDidBefore(t *testing.T) {
	store := newStore(t)
	check := func(when string, got Settings) {
		t.Helper()
		if got.EncoderPreset != "auto" || got.H264Quality != 0 || got.HevcQuality != 0 || got.AllowHevcEncoding || got.HardwareAcceleration != "auto" ||
			!slices.Equal(got.HardwareDecodingCodecs, HardwareDecodingCodecs) || !got.ToneMapping || got.ToneMappingAlgorithm != "auto" ||
			got.ToneMappingPeak != 0 || got.ToneMappingDesat != 0 || got.DeinterlaceMethod != "yadif" || got.DeinterlaceDoubleRate ||
			got.DownmixAlgorithm != "None" || got.DownmixBoost != DefaultDownmixBoost || got.MaxAudioChannels != 0 ||
			got.AudioBitratePerChannel != 0 || got.EncodingThreads != 0 || got.AheadSegments != DefaultAheadSegments {
			t.Errorf("%s: %+v", when, got)
		}
	}
	check("defaults", store.Settings())
	if DefaultDownmixBoost != 1 || DefaultAheadSegments != 10 {
		t.Errorf("default constants: boost %v, %d segments ahead", DefaultDownmixBoost, DefaultAheadSegments)
	}
	if _, err := store.CreateFirstAdministrator(t.Context(), "admin", "correct horse", "fr"); err != nil {
		t.Fatal(err)
	}
	check("after the setup", store.Settings())
}

func TestConversionSettingsRoundTripAndStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	for _, tc := range []struct {
		name   string
		change func(*Settings)
		err    error
	}{
		{"an unknown preset", func(s *Settings) { s.EncoderPreset = "placebo" }, ErrInvalidEncoderPreset},
		{"an empty preset", func(s *Settings) { s.EncoderPreset = "" }, ErrInvalidEncoderPreset},
		{"a negative H.264 quality", func(s *Settings) { s.H264Quality = -1 }, ErrInvalidVideoQuality},
		{"an HEVC quality above 51", func(s *Settings) { s.HevcQuality = MaxVideoQuality + 1 }, ErrInvalidVideoQuality},
		{"an unknown GPU", func(s *Settings) { s.HardwareAcceleration = "qsv" }, ErrInvalidHardwareAcceleration},
		{"no GPU choice, which followed POLYFIN_HWACCEL", func(s *Settings) { s.HardwareAcceleration = "" }, ErrInvalidHardwareAcceleration},
		{"an unknown decoded codec", func(s *Settings) { s.HardwareDecodingCodecs = []string{"h264", "vp8"} }, ErrInvalidHardwareDecodingCodecs},
		{"a decoded codec twice", func(s *Settings) { s.HardwareDecodingCodecs = []string{"h264", "h264"} }, ErrInvalidHardwareDecodingCodecs},
		{"an unknown curve", func(s *Settings) { s.ToneMappingAlgorithm = "gamma" }, ErrInvalidToneMappingAlgorithm},
		{"a peak below 100 nits", func(s *Settings) { s.ToneMappingPeak = MinToneMappingPeak - 1 }, ErrInvalidToneMappingPeak},
		{"a peak above 10000 nits", func(s *Settings) { s.ToneMappingPeak = MaxToneMappingPeak + 1 }, ErrInvalidToneMappingPeak},
		{"a negative desaturation", func(s *Settings) { s.ToneMappingDesat = -0.1 }, ErrInvalidToneMappingDesat},
		{"too much desaturation", func(s *Settings) { s.ToneMappingDesat = MaxToneMappingDesat + 0.1 }, ErrInvalidToneMappingDesat},
		{"an unknown deinterlacer", func(s *Settings) { s.DeinterlaceMethod = "w3fdif" }, ErrInvalidDeinterlaceMethod},
		{"an unknown downmix", func(s *Settings) { s.DownmixAlgorithm = "dave750" }, ErrInvalidDownmixAlgorithm},
		{"a boost below 0.5", func(s *Settings) { s.DownmixBoost = MinDownmixBoost - 0.1 }, ErrInvalidDownmixBoost},
		{"a boost above 3", func(s *Settings) { s.DownmixBoost = MaxDownmixBoost + 0.1 }, ErrInvalidDownmixBoost},
		{"8 audio channels", func(s *Settings) { s.MaxAudioChannels = 8 }, ErrInvalidMaxAudioChannels},
		{"an audio bitrate below 32 kb/s", func(s *Settings) { s.AudioBitratePerChannel = MinAudioBitratePerChannel - 1 }, ErrInvalidAudioBitrate},
		{"an audio bitrate above 320 kb/s", func(s *Settings) { s.AudioBitratePerChannel = MaxAudioBitratePerChannel + 1 }, ErrInvalidAudioBitrate},
		{"negative threads", func(s *Settings) { s.EncodingThreads = -1 }, ErrInvalidEncodingThreads},
		{"too many threads", func(s *Settings) { s.EncodingThreads = MaxEncodingThreads + 1 }, ErrInvalidEncodingThreads},
		{"no segment ahead", func(s *Settings) { s.AheadSegments = MinAheadSegments - 1 }, ErrInvalidAheadSegments},
		{"too many segments ahead", func(s *Settings) { s.AheadSegments = MaxAheadSegments + 1 }, ErrInvalidAheadSegments},
	} {
		changed := store.Settings()
		tc.change(&changed)
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}
	defaults := newStore(t).Settings()
	if got := store.Settings(); !reflect.DeepEqual(got, defaults) {
		t.Errorf("a refused update changed the settings: %+v", got)
	}
	// Every other value is kept across a restart; the codecs decoded on
	// the GPU come back in their usual order.
	for _, change := range []func(*Settings){
		func(s *Settings) {
			s.EncoderPreset, s.H264Quality, s.HevcQuality, s.AllowHevcEncoding, s.HardwareAcceleration = "veryslow", MinVideoQuality, MaxVideoQuality, true, "vaapi"
			s.HardwareDecodingCodecs = []string{"vc1", "h264"}
			s.ToneMapping, s.ToneMappingAlgorithm, s.ToneMappingPeak, s.ToneMappingDesat = false, "mobius", MinToneMappingPeak, 0.5
			s.DeinterlaceMethod, s.DeinterlaceDoubleRate = "bwdif", true
			s.DownmixAlgorithm, s.DownmixBoost, s.MaxAudioChannels, s.AudioBitratePerChannel = "Dave750", MinDownmixBoost, 2, MinAudioBitratePerChannel
			s.EncodingThreads, s.AheadSegments = MaxEncodingThreads, MinAheadSegments
		},
		func(s *Settings) {
			s.EncoderPreset, s.HardwareAcceleration, s.HardwareDecodingCodecs = "ultrafast", "none", []string{}
			s.ToneMappingAlgorithm, s.ToneMappingPeak, s.ToneMappingDesat = "bt2390", MaxToneMappingPeak, MaxToneMappingDesat
			s.DownmixAlgorithm, s.DownmixBoost, s.MaxAudioChannels, s.AudioBitratePerChannel = "Ac4", MaxDownmixBoost, 6, MaxAudioBitratePerChannel
			s.EncodingThreads, s.AheadSegments = 0, MaxAheadSegments
		},
	} {
		changed := store.Settings()
		change(&changed)
		saved, err := store.UpdateSettings(ctx, changed)
		if err != nil {
			t.Fatalf("%+v: %v", changed, err)
		}
		if len(changed.HardwareDecodingCodecs) == 2 {
			changed.HardwareDecodingCodecs = []string{"h264", "vc1"}
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings(); !reflect.DeepEqual(got, changed) || !reflect.DeepEqual(saved, changed) {
			t.Errorf("after reopening: %+v, saved %+v, want %+v", got, saved, changed)
		}
	}
	// The database refuses what the store refuses, whoever writes it.
	for _, column := range []string{"encoder_preset = 'placebo'", "h264_quality = 52", "hevc_quality = -1", "hardware_acceleration = 'qsv'", "hardware_acceleration = ''",
		"hardware_decoding_codecs = '{vp8}'", "tone_mapping_algorithm = 'gamma'", "tone_mapping_peak = 99", "tone_mapping_desat = 11",
		"deinterlace_method = 'w3fdif'", "downmix_algorithm = 'dave750'", "downmix_boost = 0.4", "max_audio_channels = 8",
		"audio_bitrate_per_channel = 16", "encoding_threads = 65", "ahead_segments = 0"} {
		if _, err := store.db.Exec(ctx, "UPDATE settings SET "+column); err == nil {
			t.Errorf("the database took %s", column)
		}
	}
}

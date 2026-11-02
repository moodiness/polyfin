package playback

import (
	"errors"
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
)

// In fragmented MP4, the player keeps the audio's configuration from the
// initialization segment it has: a version whose audio would be converted
// into the HE-AAC the play copied cannot carry the play on, as the encoder
// writes AAC-LC alone. MPEG-TS, whose AAC frames each tell their own, and
// AAC-LC copied in fragmented MP4, can.
func TestFragmentedMP4ConvertsAudioIntoAACLCAlone(t *testing.T) {
	t.Parallel()
	other := media.Analysis{Streams: []media.Stream{{Index: 1, Type: "audio", Codec: "ac3", Channels: 2, SampleRate: 48_000}}}
	for name, c := range map[string]struct {
		profile string
		format  hls.Format
		carries bool
	}{
		"HE-AAC in fMP4":   {"HE-AAC", hls.FMP4, false},
		"HE-AACv2 in fMP4": {"HE-AACv2", hls.FMP4, false},
		"HE-AAC in TS":     {"HE-AAC", hls.TS, true},
		"AAC-LC in fMP4":   {"LC", hls.FMP4, true},
	} {
		first := media.Analysis{Streams: []media.Stream{{Index: 1, Type: "audio", Codec: "aac", Profile: c.profile, Channels: 2, SampleRate: 48_000}}}
		carry := &carry{remux: Remux{Audio: 1, Format: c.format}, analysis: first}
		var r hls.Remux
		err := carry.audio(&r, other, hls.Remux{}, Tuning{})
		if carried := err == nil; carried != c.carries || err != nil && !errors.Is(err, errNotAlike) {
			t.Errorf("%s: %v, want carried %v", name, err, c.carries)
		}
		if err == nil && r.AudioCodec != "aac" {
			t.Errorf("%s: converted into %q", name, r.AudioCodec)
		}
	}
}

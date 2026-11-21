package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

// An HLS request checks the audio track its URL names as PlaybackInfo
// would have: a track the URL's codecs do not take, its MPEG-TS segments
// do not carry, or with more channels than the URL allows is converted
// though the URL does not ask for it, and refused for a user who may not
// have audio converted. A track that fits is copied, and a one-file
// stream, whose container carries any codec, copies what its codecs take.
func TestHLSRequestConvertsAudioItCannotCopy(t *testing.T) {
	t.Parallel()
	p := playing(t)
	analysis := media.Analysis{Format: "matroska,webm", Duration: 15 * time.Second, Size: 21, Bitrate: 25_000, Remote: true,
		Streams: []media.Stream{
			{Index: 0, Type: "video", Codec: "h264", Profile: "High", Level: 10, Width: 64, Height: 64, FrameRate: 24, AverageRate: 24, PixelFormat: "yuv420p", BitDepth: 8},
			{Index: 1, Type: "audio", Codec: "dts", Default: true, Channels: 6, SampleRate: 48000, ChannelLayout: "5.1"},
			{Index: 2, Type: "audio", Codec: "ac3", Channels: 6, SampleRate: 48000, ChannelLayout: "5.1"},
			{Index: 3, Type: "audio", Codec: "aac", Profile: "LC", Channels: 2, SampleRate: 48000, ChannelLayout: "stereo"},
		}}
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", p.versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	p.keyframed(t, p.versions[0])
	opened, _ := accounts.ParseID(p.movie)
	// The movie's subtitle file comes first: DTS is stream 2, AC-3 3 and
	// AAC 4.
	remux := func(who accounts.User, query string) (remuxRequest, bool) {
		t.Helper()
		return p.handler.remuxFor(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/videos/"+p.movie+"/main.m3u8?"+query, nil),
			who, opened, p.versions[0].ID, "session")
	}
	for _, tc := range []struct {
		name, query string
		converted   bool
	}{
		{"DTS in MPEG-TS", "SegmentContainer=ts&AudioCodec=aac,ac3&AudioStreamIndex=2", true},
		{"DTS the codecs take, in MPEG-TS", "SegmentContainer=ts&AudioCodec=aac,ac3,dts&AudioStreamIndex=2", true},
		{"AC-3", "SegmentContainer=ts&AudioCodec=aac,ac3&AudioStreamIndex=3", false},
		{"AAC", "SegmentContainer=ts&AudioCodec=aac,ac3&AudioStreamIndex=4", false},
		{"AC-3 over the channels", "SegmentContainer=ts&AudioCodec=aac,ac3&AudioStreamIndex=3&TranscodingMaxAudioChannels=2", true},
		{"AC-3 within the channels", "SegmentContainer=ts&AudioCodec=aac,ac3&AudioStreamIndex=3&TranscodingMaxAudioChannels=6", false},
		{"DTS in one file", "AudioCodec=aac,ac3,dts&AudioStreamIndex=2", false},
		{"DTS in one file the codecs do not take", "AudioCodec=aac,ac3&AudioStreamIndex=2", true},
	} {
		req, ok := remux(p.user, tc.query)
		if !ok {
			t.Fatalf("%s: refused", tc.name)
		}
		if converted := req.remux.ConvertAudio != nil; converted != tc.converted {
			t.Errorf("%s: converted %v, want %v (%+v)", tc.name, converted, tc.converted, req.remux.ConvertAudio)
		}
	}

	if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{AudioTranscoding: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	user, err := p.store.User(t.Context(), p.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := remux(user, "SegmentContainer=ts&AudioCodec=aac,ac3&AudioStreamIndex=2"); ok {
		t.Error("a user who may not have audio converted got DTS in MPEG-TS")
	}
	if req, ok := remux(user, "SegmentContainer=ts&AudioCodec=aac,ac3&AudioStreamIndex=3"); !ok || req.remux.ConvertAudio != nil {
		t.Errorf("a user who may not have audio converted, AC-3: %v %+v", ok, req.remux.ConvertAudio)
	}
}

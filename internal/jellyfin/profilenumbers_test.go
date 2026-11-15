package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Request bodies are read as Jellyfin's JSON settings read them: strings
// also from numbers and booleans, numbers also from strings holding one,
// booleans strictly.
func TestBodiesAreReadAsJellyfinReadsThem(t *testing.T) {
	t.Parallel()
	type body struct {
		Name   string
		Values []string
		Prefs  map[string]string
		Count  int
		Ticks  *int64
		Rating float64
		Number json.Number
		On     bool
		Loose  looseInt
	}
	var got body
	err := unmarshalBody([]byte(`{"name":2,"Values":[1,true,"x",-0.50],"Prefs":{"a":1e3},"Count":"3","Ticks":"-5",
		"Rating":"7.5","Number":1,"On":true,"Loose":"4"}`), &got)
	if err != nil || got.Name != "2" || strings.Join(got.Values, " ") != "1 true x -0.50" || got.Prefs["a"] != "1e3" ||
		got.Count != 3 || got.Ticks == nil || *got.Ticks != -5 || got.Rating != 7.5 || got.Number != "1" || !got.On || got.Loose != (looseInt{4, true}) {
		t.Errorf("read %+v, %v", got, err)
	}
	// A nullable number takes an empty string as null.
	empty := body{Ticks: new(int64(1))}
	if err := unmarshalBody([]byte(`{"Ticks":""}`), &empty); err != nil || empty.Ticks != nil {
		t.Errorf("empty string: %v, %v", empty.Ticks, err)
	}
	for _, refused := range []string{
		`{"On":"true"}`, `{"On":1}`, `{"Count":" 3"}`, `{"Count":"3 "}`, `{"Count":"3x"}`, `{"Count":"1.5"}`, `{"Count":""}`,
		`{"Count":"null"}`, `{"Ticks":"0x10"}`, `{"Name":{}}`, `{"Name":[1]}`, `{"Name":2`, `{"Name":2} {}`,
	} {
		if err := unmarshalBody([]byte(refused), &body{}); err == nil {
			t.Errorf("%s read", refused)
		}
	}
}

// reproProfile is the device profile of the issue's report, its
// transcoding profile's numbers sent as given.
func reproProfile(channels, segments any) map[string]any {
	return map[string]any{
		"Name":               "repro",
		"DirectPlayProfiles": []any{map[string]any{"Container": "mp4,mkv", "Type": "Video", "VideoCodec": "h264", "AudioCodec": "aac"}},
		"TranscodingProfiles": []any{map[string]any{"Container": "ts", "Type": "Video", "AudioCodec": "aac", "VideoCodec": "h264",
			"Context": "Streaming", "Protocol": "hls", "MaxAudioChannels": channels, "MinSegments": segments, "SegmentLength": 3,
			"BreakOnNonKeyFrames": true}},
		"CodecProfiles":    []any{},
		"SubtitleProfiles": []any{},
	}
}

// playDecision is how the app is told to play a source, without the play
// session that differs between requests.
type playDecision struct {
	DirectPlay, DirectStream, Transcoding bool
	Container, SubProtocol, URL           string
}

func decisionOf(t *testing.T, source MediaSourceInfo) playDecision {
	t.Helper()
	if source.TranscodingUrl != "" {
		target, err := url.Parse(source.TranscodingUrl)
		if err != nil {
			t.Fatal(err)
		}
		query := target.Query()
		query.Del("PlaySessionId")
		target.RawQuery = query.Encode()
		source.TranscodingUrl = target.String()
	}
	return playDecision{source.SupportsDirectPlay, source.SupportsDirectStream, source.SupportsTranscoding,
		source.TranscodingContainer, source.TranscodingSubProtocol, source.TranscodingUrl}
}

// An app that sends numbers where jellyfin-web sends strings, as in a
// transcoding profile's MaxAudioChannels and MinSegments, plays as
// jellyfin-web does.
func TestProfileNumbersPlayAsStrings(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.remuxable(t)
	ask := func(profile map[string]any) playDecision {
		t.Helper()
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
			map[string]any{"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000, "DeviceProfile": profile})
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
			t.Fatalf("%d %s", status, data)
		}
		return decisionOf(t, response.MediaSources[0])
	}
	numbers, texts := ask(reproProfile(2, 1)), ask(reproProfile("2", "1"))
	// The Opus audio becomes AAC over HLS, with the profile's numbers.
	if numbers != texts || numbers.DirectPlay || !numbers.Transcoding ||
		!strings.Contains(numbers.URL, "TranscodingMaxAudioChannels=2") || !strings.Contains(numbers.URL, "MinSegments=1") {
		t.Errorf("numbers %+v\nstrings %+v", numbers, texts)
	}
	// A session's capabilities carry the same profile.
	if status, body := p.call(http.MethodPost, "/Sessions/Capabilities/Full", app("tv", p.token),
		map[string]any{"PlayableMediaTypes": []string{"Video"}, "DeviceProfile": reproProfile(2, 1)}); status != http.StatusNoContent {
		t.Errorf("capabilities: %d %s", status, body)
	}
}

// A codec profile condition's Value is a string an app may send as a
// number.
func TestConditionValuesMayBeNumbers(t *testing.T) {
	t.Parallel()
	p := playing(t)
	ask := func(width any) playDecision {
		t.Helper()
		profile := map[string]any{
			"DirectPlayProfiles":  []any{map[string]any{"Type": "Video", "Container": "mp4", "VideoCodec": "h264", "AudioCodec": "aac"}},
			"TranscodingProfiles": []any{map[string]any{"Type": "Video", "Container": "ts", "Protocol": "hls", "VideoCodec": "h264", "AudioCodec": "aac", "Context": "Streaming"}},
		}
		if width != nil {
			profile["CodecProfiles"] = []any{map[string]any{"Type": "Video", "Codec": "h264",
				"Conditions": []any{map[string]any{"Condition": "LessThanEqual", "Property": "Width", "Value": width, "IsRequired": false}}}}
		}
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
			map[string]any{"UserId": p.user.ID.String(), "MediaSourceId": p.versions[1].ID.String(), "MaxStreamingBitrate": 120_000_000,
				"DeviceProfile": profile})
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil {
			t.Fatalf("%d %s", status, data)
		}
		if len(response.MediaSources) == 0 {
			return playDecision{}
		}
		return decisionOf(t, response.MediaSources[0])
	}
	if free := ask(nil); !free.DirectPlay {
		t.Fatalf("without a condition: %+v", free)
	}
	// The video is 640 wide: past the condition, it does not play as it is.
	number, text := ask(320), ask("320")
	if number != text || number.DirectPlay {
		t.Errorf("number %+v\nstring %+v", number, text)
	}
}

// A body Polyfin cannot read, as Jellyfin cannot, is still refused, and
// logged once, with the app and the device but not the body.
func TestUnreadablePlaybackInfoIsLogged(t *testing.T) {
	t.Parallel()
	p := playing(t)
	const message = "A PlaybackInfo request could not be read"
	for i, body := range []string{
		`{"DeviceProfile":{"Name":"marker","TranscodingProfiles":[{"MaxAudioChannels":{"Channels":2}}]}}`,
		`{"DeviceProfile":{"Name":"marker","TranscodingProfiles":[{"BreakOnNonKeyFrames":"true"}]}}`,
		`{"DeviceProfile":{"Name":"marker","MaxStreamingBitrate":"marker"}}`,
		`{"DeviceProfile":{"Name":"marker"`,
	} {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, p.url+"/Items/"+p.movie+"/PlaybackInfo", strings.NewReader(body))
		request.Header.Set("Authorization", app("tv", p.token))
		request.Header.Set("Content-Type", "application/json")
		response, err := testClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		answer, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(answer), "The JSON value could not be converted.") {
			t.Errorf("%s: %d %s", body, response.StatusCode, answer)
		}
		logged := p.warnings.attrsOf(message)
		if len(logged) != i+1 {
			t.Fatalf("%s: logged %d times: %v", body, len(logged), p.warnings.logged())
		}
		attrs := logged[i]
		if attrs["app"] != "Test App" || attrs["device"] != "tv" || attrs["error"] == "" {
			t.Errorf("%s: logged %v", body, attrs)
		}
		for key, value := range attrs {
			if strings.Contains(value, "marker") {
				t.Errorf("%s: the body is logged in %s: %s", body, key, value)
			}
		}
	}
}
